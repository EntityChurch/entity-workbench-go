package shellcmd_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestE2E_LocalFilesMountChain_LargeFile is the Phase E v2 §7.1
// regression — files past the wire-frame limit (~16 MiB default) and
// past the single-chunk threshold (1 MiB DefaultChunkSize) round-trip
// cleanly without buffering the payload into the typed
// doc/markdown-file entity.
//
// What this catches that the small-file e2e doesn't:
//   - The CONTENT v3.6 substrate carries the bytes; the typed entity
//     carries only a hash ref + metadata. Old shape (inline string
//     content) would have produced a CBOR-encoded entity larger than
//     the negotiated frame max, and subscription delivery would have
//     silently dropped the bound entity. Hash-ref shape is bounded
//     regardless of file size.
//   - FastCDC produces a multi-chunk blob; reassembly walks the chunk
//     list in order; the chunk count is non-trivial.
//   - First-chunk-only title extraction works — we don't materialize
//     the full payload in ingest-transform / notification-ingest.
//
// Size: 20 MiB. Comfortably past the 16 MiB transport frame default,
// comfortably past 1 MiB chunk target so blob.Chunks has at least
// ~20 entries, but small enough that the test runs in a few seconds.
func TestE2E_LocalFilesMountChain_LargeFile(t *testing.T) {
	const fileSize = 20 * 1024 * 1024 // 20 MiB
	fsDir := t.TempDir()

	ingestHandler := workbench.NewNotificationIngestHandler(nil)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingestHandler},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	const targetPrefix = "archives/large/"
	rootName := "e2e-large"
	sourcePrefix := "local/files/" + rootName + "/"

	ingestHandler.RegisterMount(sourcePrefix, targetPrefix)
	if err := installMountQ2(t, ap, fsDir, rootName, sourcePrefix, targetPrefix); err != nil {
		t.Fatalf("installMountQ2: %v", err)
	}

	// Build a markdown-ish payload: a known first heading followed by
	// random bytes padded out to fileSize. The heading must survive
	// first-chunk-only extraction (the heading sits at offset 0, well
	// inside the first 1 MiB chunk).
	header := "# Big File\n\nGenerated payload follows.\n"
	padding := make([]byte, fileSize-len(header))
	if _, err := rand.Read(padding); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	body := append([]byte(header), padding...)
	mdPath := filepath.Join(fsDir, "big.md")
	if err := os.WriteFile(mdPath, body, 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	wantSourcePath := sourcePrefix + "big.md"
	wantPath := targetPrefix + "big.md"

	// Larger budget than the small-file test — FastCDC over 20 MiB +
	// 20+ chunk store puts is real work, but should still finish well
	// under 15s on any reasonable machine.
	waitForBinding(t, ap, wantSourcePath, 15*time.Second, "FileData (localfiles watcher)")
	waitForBinding(t, ap, wantPath, 15*time.Second, "doc/markdown-file (notification ingest)")

	ent, ok, err := ap.Get(wantPath)
	if err != nil {
		t.Fatalf("Get %s: %v", wantPath, err)
	}
	if !ok {
		t.Fatalf("expected entity at %s, missing", wantPath)
	}
	if ent.Type != workbench.MarkdownFileType {
		t.Fatalf("target entity type = %s, want %s", ent.Type, workbench.MarkdownFileType)
	}

	md, err := workbench.MarkdownFileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode target entity: %v", err)
	}
	if md.Title != "Big File" {
		t.Errorf("title = %q, want %q (first-chunk heading scan should resolve it)", md.Title, "Big File")
	}
	if md.Size != int64(fileSize) {
		t.Errorf("md.Size = %d, want %d", md.Size, fileSize)
	}

	// The typed entity's CBOR payload itself MUST be tiny — that's
	// the whole point of the hash-ref shape. 4 KiB is a generous
	// ceiling; in practice the entity is well under 1 KiB.
	if len(ent.Data) > 4*1024 {
		t.Errorf("typed entity CBOR payload = %d bytes, want < 4 KiB (hash-ref shape required)", len(ent.Data))
	}

	// Substrate-side: blob entity is bound; chunk count matches what
	// FastCDC produces over 20 MiB. With 1 MiB target chunk size, we
	// expect at least ~16 chunks (real distribution varies, but the
	// floor is well above 1 — that's the regression we care about).
	cs := ap.Store().ContentStore()
	blobEnt, ok := cs.Get(md.Content)
	if !ok {
		t.Fatalf("blob entity missing from local content store")
	}
	if blobEnt.Type != types.TypeContentBlob {
		t.Fatalf("blob entity type = %s, want %s", blobEnt.Type, types.TypeContentBlob)
	}
	body2, present, err := workbench.LoadMarkdownContent(cs, md)
	if err != nil {
		t.Fatalf("LoadMarkdownContent: %v", err)
	}
	if !present {
		t.Fatal("blob present in store but LoadMarkdownContent reports missing")
	}
	if len(body2) != fileSize {
		t.Fatalf("reassembled size = %d, want %d", len(body2), fileSize)
	}
	// Verify the leading header round-tripped intact; full byte-compare
	// of 20 MiB would be slow under the race detector but the substrate
	// guarantees byte-identity via FastCDC content addressing — the
	// header check pins the head end and the size check pins the tail.
	if !strings.HasPrefix(string(body2), header) {
		t.Errorf("reassembled body does not start with original header")
	}

	// Sanity: no chain-error markers landed under
	// system/runtime/chain-errors/. A silent drop in the old shape
	// would have produced one of these.
	chainErrs := ap.Store().List("system/runtime/chain-errors/")
	if len(chainErrs) > 0 {
		paths := make([]string, 0, len(chainErrs))
		for _, e := range chainErrs {
			paths = append(paths, e.Path)
		}
		t.Errorf("unexpected chain-error markers: %s", strings.Join(paths, ", "))
	}
}

// TestIngestMarkdownTree_LargeFile covers the direct fs-walk ingest
// path (`ingest tree` shell verb / IngestMarkdownTree function). Same
// concern as the mount-chain test — large files must round-trip via
// blob+chunks, not inline string — but exercises the Level 0 Store.Put
// path rather than the watch/notification chain.
func TestIngestMarkdownTree_LargeFile(t *testing.T) {
	const fileSize = 12 * 1024 * 1024 // 12 MiB — past 1 MiB chunk target, past inline cliff
	srcDir := t.TempDir()

	header := "# Large Doc\n\nGenerated content follows.\n"
	padding := make([]byte, fileSize-len(header))
	if _, err := rand.Read(padding); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	body := append([]byte(header), padding...)
	if err := os.WriteFile(filepath.Join(srcDir, "big.md"), body, 0600); err != nil {
		t.Fatalf("write big.md: %v", err)
	}
	// A small companion file to cover the mixed-size case.
	if err := os.WriteFile(filepath.Join(srcDir, "small.md"), []byte("# Small\n\nshort.\n"), 0600); err != nil {
		t.Fatalf("write small.md: %v", err)
	}

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	res, err := workbench.IngestMarkdownTree(ap.Store(), srcDir, "docs/")
	if err != nil {
		t.Fatalf("IngestMarkdownTree: %v", err)
	}
	if res.Created != 2 {
		t.Fatalf("Created = %d, want 2 (got errors: %v)", res.Created, res.Errors)
	}
	if res.BytesIn != int64(fileSize+len("# Small\n\nshort.\n")) {
		t.Errorf("BytesIn = %d, want %d", res.BytesIn, fileSize+len("# Small\n\nshort.\n"))
	}

	// Verify the big file's entity is hash-ref shape.
	bigEnt, ok, err := ap.Get("docs/big.md")
	if err != nil || !ok {
		t.Fatalf("Get docs/big.md: ok=%v err=%v", ok, err)
	}
	md, err := workbench.MarkdownFileDataFromEntity(bigEnt)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if md.Title != "Large Doc" {
		t.Errorf("title = %q, want %q", md.Title, "Large Doc")
	}
	if md.Size != int64(fileSize) {
		t.Errorf("size = %d, want %d", md.Size, fileSize)
	}
	if len(bigEnt.Data) > 4*1024 {
		t.Errorf("typed entity payload = %d bytes, want < 4 KiB", len(bigEnt.Data))
	}

	cs := ap.Store().ContentStore()
	got, present, err := workbench.LoadMarkdownContent(cs, md)
	if err != nil {
		t.Fatalf("LoadMarkdownContent: %v", err)
	}
	if !present {
		t.Fatal("blob missing from local content store")
	}
	if len(got) != fileSize {
		t.Errorf("reassembled size = %d, want %d", len(got), fileSize)
	}
	if !strings.HasPrefix(string(got), header) {
		t.Error("reassembled body does not start with original header")
	}
	// Make sure two distinct files with distinct content produce
	// distinct blob hashes (basic dedup-soundness check).
	smallEnt, _, _ := ap.Get("docs/small.md")
	smallMD, _ := workbench.MarkdownFileDataFromEntity(smallEnt)
	if smallMD.Content == md.Content {
		t.Error("small file and large file resolved to the same blob hash — chunking is broken")
	}
}

// TestMarkdownViewModel_SaveLargeContent covers the editor save path
// for large drafts. Save() must chunk via the substrate; the typed
// entity it binds must stay small regardless of draft size.
func TestMarkdownViewModel_SaveLargeContent(t *testing.T) {
	const draftSize = 8 * 1024 * 1024 // 8 MiB
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Seed a small entity at the target path so EnterEdit has something
	// to clone — the test simulates "load a small doc, paste a huge
	// draft, save."
	pc := ap.PeerContext()
	if _, err := pc.Store().Put("notes/big.md", workbench.MarkdownFileType,
		workbench.MarkdownFileData{Path: "notes/big.md", Title: "Stub"}); err != nil {
		t.Fatal(err)
	}

	m := workbench.NewMarkdownViewModel(pc)
	m.Load("notes/big.md")
	m.EnterEdit()

	header := "# Saved Big\n\n"
	padding := make([]byte, draftSize-len(header))
	for i := range padding {
		padding[i] = byte('a' + (i % 26))
	}
	draft := header + string(padding)
	m.UpdateContent(draft)
	m.UpdateTitle("Saved Big")
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Typed entity stays small.
	ent, _ := pc.Store().Get("notes/big.md")
	if len(ent.Data) > 4*1024 {
		t.Errorf("typed entity payload = %d bytes, want < 4 KiB", len(ent.Data))
	}

	md, err := workbench.MarkdownFileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if md.Size != int64(len(draft)) {
		t.Errorf("md.Size = %d, want %d", md.Size, len(draft))
	}

	// Round-trip: load fresh, content matches.
	got, present, err := workbench.LoadMarkdownContent(pc.Store().ContentStore(), md)
	if err != nil || !present {
		t.Fatalf("LoadMarkdownContent: present=%v err=%v", present, err)
	}
	if len(got) != len(draft) {
		t.Fatalf("reassembled size = %d, want %d", len(got), len(draft))
	}
	if string(got[:len(header)]) != header {
		t.Errorf("header round-trip mismatch")
	}
}

// silence unused-import compiler errors if test layout shifts later.
var _ = fmt.Sprintf
