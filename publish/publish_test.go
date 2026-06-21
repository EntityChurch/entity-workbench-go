package publish_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
)

// TestPublish_FirstForm covers the Amendment 5 happy path:
// manifest at {out}/manifest, content at content/{2}/{2}/{wire},
// tree bindings at {peer_id}/{path}.bin, AND the new listing objects:
// {peer_id}/{path}.list at every interior prefix, {peer_id}.list peer
// root, peers.list universal-tree-root.
func TestPublish_FirstForm(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	seeds := map[string]map[string]string{
		"docs/index":           {"title": "Welcome", "body": "first form"},
		"docs/intro":           {"title": "Intro", "body": "second"},
		"docs/chapter-1/start": {"title": "Chapter 1", "body": "third"},
	}
	hashes := make(map[string]hash.Hash, len(seeds))
	for p, data := range seeds {
		h, err := ap.Store().Put(p, "test/note", data)
		if err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
		hashes[p] = h
	}

	out := t.TempDir()
	const origin = "https://test-origin.example"
	res, err := publish.Publish(context.Background(), publish.Opts{
		Peer:      ap,
		Prefix:    "docs/",
		OutputDir: out,
		OriginURL: origin,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Paths != len(seeds) {
		t.Errorf("Paths = %d, want %d", res.Paths, len(seeds))
	}

	// Content round-trip for docs/index (sharded-2-4, full 33-byte wire hex).
	want := hashes["docs/index"]
	wireHex := hex.EncodeToString(want.Bytes())
	if len(wireHex) != 66 {
		t.Fatalf("expected 66-char wire hex, got %d (%q)", len(wireHex), wireHex)
	}
	raw, err := os.ReadFile(filepath.Join(out, "content", wireHex[0:2], wireHex[2:4], wireHex))
	if err != nil {
		t.Fatalf("read emitted content for docs/index: %v", err)
	}

	// Mode-A pre-image invariant: SHA-256(file_bytes) == h.EffectiveDigest().
	// (Under v7.69 multi-hash the Hash struct's Digest is [MaxDigestSize]byte,
	// so we compare slices via EffectiveDigest, not the raw arrays.)
	rehash := sha256.Sum256(raw)
	if !bytes.Equal(rehash[:], want.EffectiveDigest()) {
		t.Errorf("body re-hash mismatch for docs/index:\n  SHA-256(file)  = %x\n  hash.Digest    = %x\n(emit must be ecf.EncodeHashable, not ecf.Encode)",
			rehash[:], want.EffectiveDigest())
	}

	// Tree binding at {peer_id}/docs/index.bin — Amendment 6: the body
	// is the 2-key bare system/hash pointer ECF({type:"system/hash",
	// data: H}), NOT the dereferenced entity. (Returning the entity at
	// every path-keyed URL would multiply copies bound to the same hash,
	// defeating V7 §1.7 dedup on static CDNs.) Asserts:
	//   - body decodes as a 2-key ECF entity (no content_hash)
	//   - entity.type == "system/hash"
	//   - entity.data is the CBOR-bstr of the 33-byte bound hash
	//   - the bound hash matches the LocationIndex entry
	// The second-hop CONTENT_GET is implicit: the bound hash IS the
	// content-shard key already asserted above.
	treePath := filepath.Join(out, ap.PeerID(), "docs", "index.bin")
	treeBytes, err := os.ReadFile(treePath)
	if err != nil {
		t.Fatalf("read tree binding for docs/index: %v", err)
	}
	pointer := decodeHashPointer(t, treeBytes)
	if pointer != want {
		t.Errorf("emitted hash pointer = %s, want %s", pointer, want)
	}

	// --- Manifest (Amendment 5 three-prefix endpoint + listing suffix) ---
	manifestRaw, err := os.ReadFile(filepath.Join(out, "manifest"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifestEnt entity.Entity
	if err := ecf.Decode(manifestRaw, &manifestEnt); err != nil {
		t.Fatalf("decode manifest entity: %v", err)
	}
	if manifestEnt.Type != types.TypePeerTransportHTTPPoll {
		t.Errorf("manifest.type = %q, want %q", manifestEnt.Type, types.TypePeerTransportHTTPPoll)
	}
	var md types.HTTPPollProfileData
	if err := cbor.Unmarshal(manifestEnt.Data, &md); err != nil {
		t.Fatalf("decode manifest data: %v", err)
	}
	if md.TransportType != "http-poll" {
		t.Errorf("manifest.transport_type = %q, want http-poll", md.TransportType)
	}
	if md.PeerID != ap.PeerID() {
		t.Errorf("manifest.peer_id = %q, want %q", md.PeerID, ap.PeerID())
	}
	if md.Endpoint.TreeURLPrefix != origin {
		t.Errorf("manifest.endpoint.tree_url_prefix = %q, want %q", md.Endpoint.TreeURLPrefix, origin)
	}
	if md.Endpoint.ContentURLPrefix != origin+"/content" {
		t.Errorf("manifest.endpoint.content_url_prefix = %q, want %q", md.Endpoint.ContentURLPrefix, origin+"/content")
	}
	if md.Endpoint.ManifestURLPrefix != origin+"/manifest" {
		t.Errorf("manifest.endpoint.manifest_url_prefix = %q, want %q (Amendment 5 §6.5.3)", md.Endpoint.ManifestURLPrefix, origin+"/manifest")
	}
	if md.Endpoint.ContentLayout != types.ContentLayoutSharded24 {
		t.Errorf("manifest.endpoint.content_layout = %q, want %q", md.Endpoint.ContentLayout, types.ContentLayoutSharded24)
	}
	if md.Endpoint.TreeLeafSuffix != publish.DefaultTreeLeafSuffix {
		t.Errorf("manifest.endpoint.tree_leaf_suffix = %q, want %q", md.Endpoint.TreeLeafSuffix, publish.DefaultTreeLeafSuffix)
	}
	if md.Endpoint.TreeListingSuffix != publish.DefaultTreeListingSuffix {
		t.Errorf("manifest.endpoint.tree_listing_suffix = %q, want %q (Amendment 5 §6.5.3)", md.Endpoint.TreeListingSuffix, publish.DefaultTreeListingSuffix)
	}
	if md.Endpoint.TreeLeafSuffix == md.Endpoint.TreeListingSuffix {
		t.Errorf("manifest.endpoint: leaf and listing suffixes MUST differ (Amendment 5)")
	}
	if len(md.SupportedOps) != 3 ||
		md.SupportedOps[0] != types.OpTreeGet ||
		md.SupportedOps[1] != types.OpContentGet ||
		md.SupportedOps[2] != types.OpManifestGet {
		t.Errorf("manifest.supported_ops = %v, want [%s %s %s]",
			md.SupportedOps, types.OpTreeGet, types.OpContentGet, types.OpManifestGet)
	}
	if md.Freshness != "static-immutable+signed-pointer" {
		t.Errorf("manifest.freshness = %q, want static-immutable+signed-pointer", md.Freshness)
	}

	// --- Listings (Amendment 5 §6.5.3.1) ---

	// All-peers root listing: peers.list at the origin root, naming
	// this publisher's peer-id as the only peer-segment seen.
	allPeers := decodeListing(t, filepath.Join(out, "peers.list"))
	if allPeers.Path != "" {
		t.Errorf("peers.list path = %q, want \"\"", allPeers.Path)
	}
	if _, ok := allPeers.Entries[ap.PeerID()]; !ok {
		t.Errorf("peers.list: missing peer-id %q in entries (got %v)", ap.PeerID(), keysOf(allPeers.Entries))
	}
	if allPeers.Count != uint64(len(allPeers.Entries)) {
		t.Errorf("peers.list: count=%d but entries=%d", allPeers.Count, len(allPeers.Entries))
	}

	// Peer-root listing: {peer_id}.list, naming "docs" as a child with
	// has_children=true. listing.path carries the peer-id WITHOUT a
	// leading slash, matching core-go ext/httplive serveTreeListing
	// (poll.go:560 — `strings.TrimPrefix(prefix, "/")`).
	peerRoot := decodeListing(t, filepath.Join(out, ap.PeerID()+".list"))
	if peerRoot.Path != ap.PeerID() {
		t.Errorf("peer-root listing.path = %q, want %q (no leading slash; matches core-go ext/httplive convention)", peerRoot.Path, ap.PeerID())
	}
	docsEntry, ok := peerRoot.Entries["docs"]
	if !ok {
		t.Fatalf("peer-root listing missing 'docs' (got %v)", keysOf(peerRoot.Entries))
	}
	if hasChildren := asBool(docsEntry, "has_children"); !hasChildren {
		t.Errorf("peer-root listing: docs.has_children = false, want true")
	}

	// Listing at /{peer_id}/docs: should name index, intro (leaves), chapter-1 (parent).
	docsListing := decodeListing(t, filepath.Join(out, ap.PeerID(), "docs.list"))
	for _, name := range []string{"index", "intro", "chapter-1"} {
		if _, ok := docsListing.Entries[name]; !ok {
			t.Errorf("docs listing missing %q (got %v)", name, keysOf(docsListing.Entries))
		}
	}
	if !hasHash(docsListing.Entries["index"]) {
		t.Errorf("docs listing: index entry missing hash (leaf)")
	}
	if asBool(docsListing.Entries["chapter-1"], "has_children") != true {
		t.Errorf("docs listing: chapter-1.has_children = false, want true")
	}
	if asBool(docsListing.Entries["index"], "has_children") != false {
		t.Errorf("docs listing: index.has_children = true, want false (no children)")
	}

	// Listing at /{peer_id}/docs/chapter-1: should name 'start' as a leaf.
	chListing := decodeListing(t, filepath.Join(out, ap.PeerID(), "docs", "chapter-1.list"))
	if !hasHash(chListing.Entries["start"]) {
		t.Errorf("chapter-1 listing: start entry missing hash (leaf)")
	}

	// next_page is nil on every page (single-page renderer in v0).
	if allPeers.NextPage != nil || peerRoot.NextPage != nil || docsListing.NextPage != nil || chListing.NextPage != nil {
		t.Errorf("expected NextPage=nil on all single-page listings")
	}

	// Result count: peers + peer-root + docs + docs/chapter-1 = 4.
	if res.Listings != 4 {
		t.Errorf("res.Listings = %d, want 4", res.Listings)
	}
}

// TestPublish_FilterHooks exercises IncludePath + IncludeType, the
// sketch-level extension seams.
func TestPublish_FilterHooks(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	if _, err := ap.Store().Put("docs/keep", "type/keep", map[string]string{"k": "1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ap.Store().Put("docs/drop-path", "type/keep", map[string]string{"k": "2"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ap.Store().Put("docs/drop-type", "type/drop", map[string]string{"k": "3"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out := t.TempDir()
	res, err := publish.Publish(context.Background(), publish.Opts{
		Peer:      ap,
		Prefix:    "docs/",
		OutputDir: out,
		IncludePath: func(p string) bool {
			return filepath.Base(p) != "drop-path"
		},
		IncludeType: func(t string) bool { return t != "type/drop" },
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if res.Paths != 2 {
		t.Errorf("Paths after IncludePath = %d, want 2", res.Paths)
	}
	if res.Entities != 1 {
		t.Errorf("Entities after IncludeType = %d, want 1", res.Entities)
	}
}

// decodeHashPointer reads a .bin file as the Amendment 6 system/hash
// pointer body — a 2-key bare ECF entity `{type: "system/hash", data: H}`
// — and returns the bound hash. Validates the 2-key shape, the type, and
// the 33-byte data length. Fatal on any divergence (Amendment 6 is
// normative; one-hop wire entities here are non-conformant per V7 §1.7
// dedup).
func decodeHashPointer(t *testing.T, body []byte) hash.Hash {
	t.Helper()
	// 2-key bare ECF — produced by ecf.EncodeHashable — has no
	// content_hash field. Decode directly as a CBOR map of string→raw.
	var fields map[string]cbor.RawMessage
	if err := cbor.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode .bin as CBOR map: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf(".bin body must be 2-key bare pointer (Amendment 6); got %d keys: %v", len(fields), keysOfRaw(fields))
	}
	var typeName string
	if err := cbor.Unmarshal(fields["type"], &typeName); err != nil {
		t.Fatalf("decode .bin type field: %v", err)
	}
	if typeName != "system/hash" {
		t.Fatalf(".bin pointer type = %q, want %q (Amendment 6)", typeName, "system/hash")
	}
	var data []byte
	if err := cbor.Unmarshal(fields["data"], &data); err != nil {
		t.Fatalf("decode .bin data field: %v", err)
	}
	if len(data) != 33 {
		t.Fatalf(".bin pointer data = %d bytes, want 33 (algorithm byte + 32-byte digest)", len(data))
	}
	h, err := hash.FromBytes(data)
	if err != nil {
		t.Fatalf("parse bound hash from pointer data: %v", err)
	}
	return h
}

func keysOfRaw(m map[string]cbor.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// decodeListing reads a .list file and decodes the system/tree/listing
// entity it carries. Fatal on any failure — listings are normative.
func decodeListing(t *testing.T, path string) types.ListingData {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read listing %s: %v", path, err)
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		t.Fatalf("decode listing entity %s: %v", path, err)
	}
	if ent.Type != types.TypeTreeListing {
		t.Fatalf("listing %s: entity.type = %q, want %q", path, ent.Type, types.TypeTreeListing)
	}
	ld, err := types.ListingDataFromEntity(ent)
	if err != nil {
		t.Fatalf("listing %s: ListingDataFromEntity: %v", path, err)
	}
	return ld
}

func asBool(entry interface{}, key string) bool {
	m, ok := entry.(map[interface{}]interface{})
	if ok {
		v, _ := m[key].(bool)
		return v
	}
	if m2, ok := entry.(map[string]interface{}); ok {
		v, _ := m2[key].(bool)
		return v
	}
	return false
}

func hasHash(entry interface{}) bool {
	if m, ok := entry.(map[interface{}]interface{}); ok {
		_, has := m["hash"]
		return has
	}
	if m, ok := entry.(map[string]interface{}); ok {
		_, has := m["hash"]
		return has
	}
	return false
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
