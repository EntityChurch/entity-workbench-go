package entitysdk_test

// J-4 — the FEED joint fixture, produced against.
//
// entity-browser-rust built `tests/fixtures/feed-joint/` (their `dev` @
// 2e24636) and routed it in
// ROUTING-2026-09-12-d-workbench-go-THE-FEED-JOINT-FIXTURE-IS-BUILT-AND-THE-PEER-ID-IS-IN-THE-BYTES.md.
// `J-5` allocated the halves: **they produce the rig, we produce the bytes.**
// This file is our half — the same authored input driven through OUR encoder,
// compared row by row against their computed expectations.
//
// Tier: cross-implementation conformance (TESTING-STRATEGY).
//
// WHY THIS IS THE GATE THAT MATTERS FOR W5. Every other test of `feed.go` and
// `embed.go` in this tree is ours against ours. This is the only one where a
// disagreement is possible, and `APP-CONVENTION-FEED`'s vocabulary has two
// seats and — until this change — one implementation.
//
// ⛔ WHAT DOES NOT CARRY OVER FROM THE SITE FIXTURE, and both failures are
// silent — you get plausible entities with different bytes and nothing says
// why:
//
//   - **The peer id IS in the bytes here.** `FEED-R1` makes an entry's `author`
//     equal the namespace it is read under, and every index page is a list of
//     `pin(author, entry_hash)`. The site fixture's README says "the peer id
//     and the keypair do not matter"; neither sentence survives.
//   - **So the fixture pins a SEED, not a peer id.** A literal peer id would
//     leave `FEED-R2`'s detached signature out of the comparison entirely —
//     you cannot sign as a peer whose key you do not hold. Ed25519 signing is
//     deterministic (RFC 8032), so a seed makes the peer id, the identity
//     entity and every signature byte reproducible.
//   - **Only the INDEX keys are the convention's** (§4.2). §2 makes the
//     cross-impl contract the type tag rather than the path, so where an entry
//     lives is a local choice — which is why `index_root` is a comparand and
//     their `feed_root_ours` is not.
//
// The ORDER of the steps is the one thing they asked back for when they
// accepted the split: **a per-key report before any root comparison**, because
// a bare root mismatch costs a session to localize.
//
// A DIVERGENCE IN STEPS 1–3 IS ROUTED, NOT CORRECTED. Neither seat can rule
// this convention, and correcting our side to match theirs without establishing
// which is right converts a cohort disagreement into a silent divergence with
// one repo's name on it.

import (
	"bytes"
	hexenc "encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/tree"

	"entity-workbench-go/entitysdk"
)

const rustFeedFixture = "testdata/crossimpl-rust-feed"

// ---------- the authored input (feed.json) ----------

type jsonHash struct {
	Literal string // 66-char wire hex
	Entry   string // {"entry": "<name>"} — an EARLIER entry
}

// UnmarshalJSON admits the two forms the README pins. **A forward reference is
// refused**, here as there: an entry's hash is a function of its bytes, so an
// entry naming a later one cannot be encoded, and a format that merely LOOKED
// like it allowed one would invite a two-pass resolver for a shape the
// convention cannot represent. (The refusal happens in resolve, which is where
// the ordering is known.)
func (h *jsonHash) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		h.Literal = s
		return nil
	}
	var o struct {
		Entry string `json:"entry"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	if o.Entry == "" {
		return fmt.Errorf("hash value is neither a wire-hex literal nor {\"entry\": \"<name>\"}")
	}
	h.Entry = o.Entry
	return nil
}

type jsonRef struct {
	Tag  string    `json:"tag"`
	Peer string    `json:"peer"` // SYMBOLIC — "author" / "forum"
	Hash *jsonHash `json:"hash"`
	Path string    `json:"path"`
	Seen *jsonHash `json:"seen"`
	At   *struct {
		Field []string `json:"field"`
	} `json:"at"`
	Via []struct {
		Tag   string `json:"tag"`
		Value string `json:"value"`
	} `json:"via"`
}

type jsonBody struct {
	MediaType string `json:"media_type"`
	Fallback  string `json:"fallback"`
	Payload   struct {
		Tag  string    `json:"tag"`
		UTF8 string    `json:"utf8"`
		Hash *jsonHash `json:"hash"`
	} `json:"payload"`
	// json.RawMessage, NOT map[string]interface{}: the README pins that a JSON
	// string is an ECF text value and a JSON integer an ECF UNSIGNED integer,
	// and Go's default decode turns every JSON number into a float64. An
	// implementation that stringifies — or floats — a number into the open bag
	// is meant to be caught by a HASH rather than by review, so the harness
	// must not quietly launder it first.
	Params json.RawMessage `json:"params"`
}

type jsonEntry struct {
	Name      string   `json:"name"`
	CreatedAt uint64   `json:"created_at"`
	Body      jsonBody `json:"body"`
	Reply     *struct {
		Root   jsonRef `json:"root"`
		Parent jsonRef `json:"parent"`
	} `json:"reply"`
	Context     *jsonRef  `json:"context"`
	Prev        *jsonHash `json:"prev"`
	Attachments []jsonRef `json:"attachments"`
}

type jsonFeed struct {
	Peers struct {
		Author string `json:"author"`
		Forum  string `json:"forum"`
	} `json:"peers"`
	PageSize int         `json:"page_size"`
	Entries  []jsonEntry `json:"entries"`
}

// ---------- their computed half (EXPECTED.json) ----------

type expectedFeed struct {
	AuthorPeerID string `json:"author_peer_id"`
	Entries      []struct {
		Name         string `json:"name"`
		Entry        string `json:"entry"`
		Signature    string `json:"signature"`
		SignatureKey string `json:"signature_key"`
	} `json:"entries"`
	IndexBindings map[string]string `json:"index_bindings"`
	IndexRoot     string            `json:"index_root"`
	PageCount     uint64            `json:"page_count"`
	PageSize      int               `json:"page_size"`
}

// ---------- the producer ----------

type producedFeed struct {
	authorKP   crypto.Keypair
	authorID   entity.Entity
	authorPeer string
	forumPeer  string
	entries    []entity.Entity
	byName     map[string]hash.Hash
	names      []string
	sigs       []entity.Entity
	sigKeys    []string
	index      entitysdk.FeedIndex
}

func loadFeedFixture(t *testing.T) (jsonFeed, expectedFeed) {
	t.Helper()
	var in jsonFeed
	raw, err := os.ReadFile(filepath.Join(rustFeedFixture, "feed.json"))
	if err != nil {
		t.Fatalf("read their feed.json: %v", err)
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("decode their feed.json: %v", err)
	}
	var exp expectedFeed
	raw, err = os.ReadFile(filepath.Join(rustFeedFixture, "EXPECTED.json"))
	if err != nil {
		t.Fatalf("read their EXPECTED.json: %v", err)
	}
	if err := json.Unmarshal(raw, &exp); err != nil {
		t.Fatalf("decode their EXPECTED.json: %v", err)
	}
	if len(in.Entries) == 0 || len(exp.Entries) != len(in.Entries) {
		t.Fatalf("anti-vacuity: %d authored entries, %d expected rows", len(in.Entries), len(exp.Entries))
	}
	return in, exp
}

func keypairFromSeedHex(t *testing.T, s string) crypto.Keypair {
	t.Helper()
	b, err := hexenc.DecodeString(s)
	if err != nil {
		t.Fatalf("decode seed: %v", err)
	}
	if len(b) != 32 {
		t.Fatalf("seed is %d bytes, want 32", len(b))
	}
	var seed [32]byte
	copy(seed[:], b)
	return crypto.FromSeed(seed)
}

// produce runs the README's steps 0–3 with OUR encoder and nothing of theirs.
func produce(t *testing.T, in jsonFeed) producedFeed {
	t.Helper()
	p := producedFeed{byName: map[string]hash.Hash{}}

	// Step 0 — the peer ids.
	p.authorKP = keypairFromSeedHex(t, in.Peers.Author)
	p.authorPeer = p.authorKP.PeerID().String()
	p.forumPeer = keypairFromSeedHex(t, in.Peers.Forum).PeerID().String()
	id, err := p.authorKP.IdentityEntity()
	if err != nil {
		t.Fatalf("author identity entity: %v", err)
	}
	p.authorID = id

	peerFor := func(symbol string) string {
		switch symbol {
		case "author":
			return p.authorPeer
		case "forum":
			return p.forumPeer
		default:
			t.Fatalf("fixture names peer symbol %q, which is neither author nor forum", symbol)
			return ""
		}
	}
	// Resolving a hash is where the forward-reference refusal lives, because
	// this is the only place that knows what has been built so far.
	resolveHash := func(h *jsonHash, where string) hash.Hash {
		t.Helper()
		if h == nil {
			t.Fatalf("%s: missing hash", where)
		}
		if h.Entry != "" {
			got, ok := p.byName[h.Entry]
			if !ok {
				t.Fatalf("%s: {\"entry\": %q} is a FORWARD reference (or a typo) — an entry's hash is a "+
					"function of its bytes, so an entry naming a later one cannot be encoded", where, h.Entry)
			}
			return got
		}
		got, err := hash.ParseHex(h.Literal)
		if err != nil {
			t.Fatalf("%s: parse wire-hex %q: %v", where, h.Literal, err)
		}
		return got
	}
	buildRef := func(r jsonRef, where string) entitysdk.EntityRef {
		t.Helper()
		var out entitysdk.EntityRef
		switch r.Tag {
		case "pin":
			out = entitysdk.PinnedRef(peerFor(r.Peer), resolveHash(r.Hash, where+".hash"))
		case "live":
			var seen hash.Hash
			if r.Seen != nil {
				seen = resolveHash(r.Seen, where+".seen")
			}
			out = entitysdk.LiveRef(peerFor(r.Peer), r.Path, seen)
		default:
			t.Fatalf("%s: reference tag %q is neither pin nor live", where, r.Tag)
		}
		if r.At != nil {
			out = out.WithAnchor(r.At.Field...)
		}
		for _, v := range r.Via {
			// An UNKNOWN hint kind is carried and never acted on (REF-R7/R8).
			// The fixture carries one on purpose.
			out = out.WithHints(entitysdk.RefHint{Tag: v.Tag, Value: v.Value})
		}
		return out
	}

	// Step 1 — the entries, in order.
	for _, e := range in.Entries {
		var payload entitysdk.EmbedPayload
		switch e.Body.Payload.Tag {
		case "inline":
			payload = entitysdk.InlinePayload([]byte(e.Body.Payload.UTF8))
		case "pointer":
			payload = entitysdk.PointerPayload(resolveHash(e.Body.Payload.Hash, e.Name+".body.payload.hash"))
		default:
			t.Fatalf("%s: payload tag %q", e.Name, e.Body.Payload.Tag)
		}
		body := entitysdk.NewEmbedNode(e.Body.MediaType, payload, e.Body.Fallback)
		if len(e.Body.Params) > 0 {
			body = body.WithParams(decodeECFParams(t, e.Name, e.Body.Params))
		}

		data := entitysdk.FeedEntryData{
			Author:    p.authorPeer,
			CreatedAt: e.CreatedAt,
			Body:      body,
		}
		if e.Reply != nil {
			data.Reply = &entitysdk.FeedReply{
				Root:   buildRef(e.Reply.Root, e.Name+".reply.root"),
				Parent: buildRef(e.Reply.Parent, e.Name+".reply.parent"),
			}
		}
		if e.Context != nil {
			c := buildRef(*e.Context, e.Name+".context")
			data.Context = &c
		}
		if e.Prev != nil {
			h := resolveHash(e.Prev, e.Name+".prev")
			data.Prev = &h
		}
		for i, a := range e.Attachments {
			data.Attachments = append(data.Attachments, buildRef(a, fmt.Sprintf("%s.attachments[%d]", e.Name, i)))
		}

		ent, err := data.ToEntity()
		if err != nil {
			t.Fatalf("%s: our encoder refused a conformant entry: %v", e.Name, err)
		}
		p.entries = append(p.entries, ent)
		p.names = append(p.names, e.Name)
		p.byName[e.Name] = ent.ContentHash

		// Step 2 — the detached signature (FEED-R2 / §1.1).
		sig, key, err := entitysdk.MintEntrySignature(&p.authorKP, p.authorID, ent.ContentHash)
		if err != nil {
			t.Fatalf("%s: mint signature: %v", e.Name, err)
		}
		p.sigs = append(p.sigs, sig)
		p.sigKeys = append(p.sigKeys, key)
	}

	// Step 3 — the index. The feed's own high-water mark is the head stamp.
	ixEntries := make([]entitysdk.FeedIndexEntry, 0, len(p.entries))
	var highWater uint64
	for i, ent := range p.entries {
		ixEntries = append(ixEntries, entitysdk.FeedIndexEntry{
			Hash:      ent.ContentHash,
			CreatedAt: in.Entries[i].CreatedAt,
		})
		if in.Entries[i].CreatedAt > highWater {
			highWater = in.Entries[i].CreatedAt
		}
	}
	ix, err := entitysdk.BuildFeedIndex(p.authorPeer, ixEntries, in.PageSize, highWater)
	if err != nil {
		t.Fatalf("build index: %v", err)
	}
	p.index = ix
	return p
}

// decodeECFParams turns the JSON bag into ECF values, preserving the text /
// unsigned-integer distinction the README pins. json.Number is the only way to
// see it: Go's default `interface{}` decode makes every JSON number a float64,
// which would re-encode as a CBOR float and change the hash.
func decodeECFParams(t *testing.T, name string, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	dec := json.NewDecoder(newBytesReader(raw))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("%s: decode params: %v", name, err)
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch tv := v.(type) {
		case json.Number:
			u, err := jsonNumberToUint(tv)
			if err != nil {
				t.Fatalf("%s: params[%q] = %s: %v (the fixture carries unsigned integers only)", name, k, tv, err)
			}
			out[k] = u
		case string:
			out[k] = tv
		default:
			t.Fatalf("%s: params[%q] has unexpected JSON type %T", name, k, v)
		}
	}
	return out
}

// ---------- the steps, as tests, in the order the protocol runs ----------

// Step 0. If this fails, nothing below means anything — it is a V7 §1.5
// peer-id construction difference and not a FEED one, and it would red every
// row at once.
func TestFeedJoint_Step0_PeerIDsFromTheSeeds(t *testing.T) {
	in, exp := loadFeedFixture(t)
	kp := keypairFromSeedHex(t, in.Peers.Author)
	if got := kp.PeerID().String(); got != exp.AuthorPeerID {
		t.Fatalf("author peer id from the pinned seed: got %s, want %s\n"+
			"STOP HERE — this is a V7 §1.5 peer-id construction difference, not a FEED one, and every "+
			"entry body and index page carries it", got, exp.AuthorPeerID)
	}
}

// Step 1, per entry, in order, before anything else is compared.
func TestFeedJoint_Step1_EntryHashes(t *testing.T) {
	in, exp := loadFeedFixture(t)
	p := produce(t, in)

	for i, want := range exp.Entries {
		if p.names[i] != want.Name {
			t.Fatalf("row %d: produced %q, expected %q — the fixtures are out of step", i, p.names[i], want.Name)
		}
		got := hexOf(p.entries[i].ContentHash)
		if got != want.Entry {
			t.Errorf("entry %q: hash diverges — ROUTE, do not correct locally\n  ours   %s\n  theirs %s",
				want.Name, got, want.Entry)
		}
	}
}

// Step 2 — the detached signatures, and the KEY as well as the hash. The key is
// a real expectation rather than a local choice: V7 §3.5's invariant pointer
// pins it at `system/signature/{entry_hash_hex}` under the author's namespace.
func TestFeedJoint_Step2_DetachedSignatures(t *testing.T) {
	in, exp := loadFeedFixture(t)
	p := produce(t, in)

	for i, want := range exp.Entries {
		if got := hexOf(p.sigs[i].ContentHash); got != want.Signature {
			t.Errorf("entry %q: signature entity hash diverges — ROUTE\n  ours   %s\n  theirs %s",
				want.Name, got, want.Signature)
		}
		if p.sigKeys[i] != want.SignatureKey {
			t.Errorf("entry %q: signature key: ours %s, theirs %s",
				want.Name, p.sigKeys[i], want.SignatureKey)
		}
	}
}

// Step 3 — the index, KEY BY KEY. This ordering is the thing the peer seat
// asked back for, and it is why the root comparison is a separate test that
// runs after: a bare root mismatch names the feed and not the page.
func TestFeedJoint_Step3_IndexBindingsKeyByKey(t *testing.T) {
	in, exp := loadFeedFixture(t)
	p := produce(t, in)

	if p.index.PageCount != exp.PageCount {
		t.Errorf("page count: ours %d, theirs %d (page_size %d over %d entries)",
			p.index.PageCount, exp.PageCount, in.PageSize, len(in.Entries))
	}
	ours := p.index.Bindings()

	// Compare the KEY SETS first. A key present on one side only is a
	// different failure from a key whose value differs, and it sends you to a
	// different part of the build.
	var oursKeys, theirsKeys []string
	for k := range ours {
		oursKeys = append(oursKeys, k)
	}
	for k := range exp.IndexBindings {
		theirsKeys = append(theirsKeys, k)
	}
	sort.Strings(oursKeys)
	sort.Strings(theirsKeys)
	if fmt.Sprint(oursKeys) != fmt.Sprint(theirsKeys) {
		t.Fatalf("index key sets differ — ROUTE\n  ours   %v\n  theirs %v", oursKeys, theirsKeys)
	}

	for _, k := range theirsKeys {
		got := hexOf(ours[k])
		if got != exp.IndexBindings[k] {
			t.Errorf("index key %q: binding diverges — ROUTE\n  ours   %s\n  theirs %s",
				k, got, exp.IndexBindings[k])
		}
	}
}

// Step 4 — the root, over the §4.2-pinned keys and nothing else. Their
// `feed_root_ours` is deliberately not compared: `app/feed/entries/{hex}` is
// their local placement and §2 leaves the path local, so it is their drift
// detector and not a comparand.
func TestFeedJoint_Step4_IndexRoot(t *testing.T) {
	in, exp := loadFeedFixture(t)
	p := produce(t, in)

	bs := make([]tree.Binding, 0, len(p.index.PageKeys)+1)
	for k, h := range p.index.Bindings() {
		bs = append(bs, tree.Binding{Path: k, Hash: h})
	}
	root, err := tree.BuildTrie(store.NewMemoryContentStore(), bs)
	if err != nil {
		t.Fatalf("build trie: %v", err)
	}
	if got := hexOf(root); got != exp.IndexRoot {
		t.Errorf("index root diverges — ROUTE, and read the per-key report above first\n  ours   %s\n  theirs %s",
			got, exp.IndexRoot)
	}
}

// THE ARITHMETIC, ASSERTED DIRECTLY — because the round trip cannot see it.
//
// §4.5: pages FILL oldest-first and entries WITHIN a page read newest-first.
// The two directions run against each other, and **getting BOTH backwards
// round-trips perfectly** — a harness that only re-reads its own index passes
// with the pair swapped. The joint fixture would catch it (their bytes carry
// one direction), but only while the fixture exists and only for five entries,
// so the property is pinned here on its own terms too.
//
// Why it matters, from §4.3 rule 1: fill newest-first and every publish shifts
// every entry one slot, so the whole archive is rewritten and every reader's
// cursor dies — the cost property defeated by the ordinary act of posting.
func TestFeedIndex_PagesFillOldestFirstAndReadNewestFirstWithinAPage(t *testing.T) {
	const author = "2K42FX8pASWDrXaAVsGXMNJbAkCVBuVwXf4RuwFNnmyYis"
	mk := func(b byte) hash.Hash {
		h, err := hash.ParseHex("00" + fmt.Sprintf("%064x", b))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	// Oldest → newest.
	entries := []entitysdk.FeedIndexEntry{
		{Hash: mk(1), CreatedAt: 100}, {Hash: mk(2), CreatedAt: 200},
		{Hash: mk(3), CreatedAt: 300}, {Hash: mk(4), CreatedAt: 400},
		{Hash: mk(5), CreatedAt: 500},
	}
	ix, err := entitysdk.BuildFeedIndex(author, entries, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	if ix.PageCount != 3 {
		t.Fatalf("five entries at page size 2 is three pages with a PARTIAL last one; got %d", ix.PageCount)
	}

	var page0 entitysdk.FeedIndexPageData
	decodeEntityData(t, ix.Pages[0], &page0)
	// Page 0 holds the OLDEST two...
	if len(page0.Entries) != 2 {
		t.Fatalf("page 0 holds %d entries", len(page0.Entries))
	}
	// ...listed NEWEST first, so entry 2 precedes entry 1.
	if page0.Entries[0].Hash == nil || *page0.Entries[0].Hash != mk(2) {
		t.Errorf("page 0 entry 0 is not the newer of the oldest pair — entries within a page are newest-first (§4.5)")
	}
	if page0.Entries[1].Hash == nil || *page0.Entries[1].Hash != mk(1) {
		t.Errorf("page 0 entry 1 is not the oldest entry — pages FILL oldest-first (§4.3 rule 1)")
	}

	// The partial last page, which a single-page fixture cannot exercise.
	var page2 entitysdk.FeedIndexPageData
	decodeEntityData(t, ix.Pages[2], &page2)
	if len(page2.Entries) != 1 {
		t.Errorf("the last page is partial and holds one entry; got %d", len(page2.Entries))
	}
	if page2.UpdatedAt != 500 {
		t.Errorf("a page's updated_at is the MAX created_at of the entries it carries; got %d", page2.UpdatedAt)
	}
	if page0.UpdatedAt != 200 {
		t.Errorf("page 0's updated_at should be 200 (its own newest), not the feed's; got %d", page0.UpdatedAt)
	}

	// The head omits `oldest` when nothing has been dropped (§4.2's default).
	var head entitysdk.FeedIndexHeadData
	decodeEntityData(t, ix.Head, &head)
	if head.Current != 2 {
		t.Errorf("head.current: got %d, want 2", head.Current)
	}
	if head.Oldest != nil {
		t.Errorf("head carries oldest=%d; §4.2 defaults it to 0 and a publisher who has dropped nothing "+
			"emits no key", *head.Oldest)
	}
}

func hexOf(h hash.Hash) string { return hexenc.EncodeToString(h.Bytes()) }

// ---------- small helpers ----------

func newBytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func jsonNumberToUint(n json.Number) (uint64, error) {
	return strconv.ParseUint(n.String(), 10, 64)
}

func decodeEntityData(t *testing.T, e entity.Entity, v interface{}) {
	t.Helper()
	if err := ecf.Decode(e.Data, v); err != nil {
		t.Fatalf("decode %s: %v", e.Type, err)
	}
}
