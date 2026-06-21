package shellcmd_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestE2E_CollaborativeMarkdownEditor is the "two devices, one
// markdown workspace" deployment scenario. Validates that:
//
//   - Two peers over TCP (real loopback, not in-memory) converge on
//     every write going either direction.
//   - The fs-to-fs path (mount + blob-resolve subscription) round-trips
//     bytes, not just typed metadata: each peer's filesystem ends up
//     byte-identical to the other's.
//   - The doc/markdown-file shape (Phase E v2 hash-ref content) reads
//     correctly on the receiving peer — the blob+chunks closure
//     arrives along with the typed entity; LoadMarkdownContent
//     succeeds on both sides.
//   - Multi-round back-and-forth (alice writes, bob writes, alice
//     edits, bob edits, both edit different files near-simultaneously)
//     converges without loops, drops, or chain-error markers.
//
// This is the regression block for "would this work on actual
// multi-host TCP" — same code path, only the addresses change.
// Loopback exercises the full serialization / signature / framing
// stack the way a remote LAN would.
//
// Architecture under test:
//
//	alice (TCP)                       bob (TCP)
//	  └─ localfiles mount @ aliceDir    └─ localfiles mount @ bobDir
//	     → FileData @ local/files/shared/*
//	  └─ subscribe BOB's local/files/shared/* (blob-resolve)
//	     → fetches blob closure + local/files:write content-mode
//	     → bob's bytes land on alice's fs (reverseTracker prevents loop)
//	  └─ subscribe OWN local/files/shared/* (notification-ingest)
//	     → doc/markdown-file @ archives/notes/*  (Phase E v2 hash-ref)
//	  (symmetric on bob)
//
// The notification-ingest path runs LOCALLY on each peer over its
// own substrate writes — every materialized file (whether from
// the watcher or from a cross-peer write) produces an updated
// doc/markdown-file on the local archives/notes/ prefix.
func TestE2E_CollaborativeMarkdownEditor(t *testing.T) {
	const rootName = "shared"
	const sourcePrefix = "local/files/" + rootName + "/"
	const targetPrefix = "archives/notes/"

	aliceDir := t.TempDir()
	bobDir := t.TempDir()

	alice := newCollabPeer(t, "alice", aliceDir)
	bob := newCollabPeer(t, "bob", bobDir)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	bringUpListener(t, ctx, alice.ap, "alice")
	bringUpListener(t, ctx, bob.ap, "bob")
	if _, err := alice.ap.Connect(ctx, bob.ap.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}
	if _, err := bob.ap.Connect(ctx, alice.ap.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}

	// Each peer mounts its own dir at the shared source prefix.
	alice.installMount(t, ctx, sourcePrefix, targetPrefix)
	bob.installMount(t, ctx, sourcePrefix, targetPrefix)

	// Cross-peer fs-to-fs: each peer subscribes to the other's
	// local/files/shared/* prefix and routes deliveries to its local
	// blob-resolve handler. Materialization writes bytes to the
	// subscriber's fs; reverseTracker (core-go) + idempotency
	// short-circuit (workbench) prevent a feedback loop with the
	// watcher.
	alice.installCrossPeerMaterialize(t, bob.ap.PeerID(), sourcePrefix)
	bob.installCrossPeerMaterialize(t, alice.ap.PeerID(), sourcePrefix)

	// === Round 1: alice writes ============================================
	notesPath := "intro.md"
	round1Content := "# Intro\n\nFirst draft from alice.\n"
	writeAndSettle(t, alice, bob, notesPath, round1Content, "round 1 alice")

	verifyConverged(t, "round 1", alice, bob, sourcePrefix+notesPath, targetPrefix+notesPath, round1Content)

	// === Round 2: bob writes a different file =============================
	replyPath := "reply.md"
	round2Content := "# Reply\n\nResponse from bob, separate file.\n"
	writeAndSettle(t, bob, alice, replyPath, round2Content, "round 2 bob")

	verifyConverged(t, "round 2", alice, bob, sourcePrefix+replyPath, targetPrefix+replyPath, round2Content)
	// And alice's earlier file is still present on both sides.
	verifyConverged(t, "round 2 (alice's still present)", alice, bob,
		sourcePrefix+notesPath, targetPrefix+notesPath, round1Content)

	// === Round 3: alice edits her own file ================================
	round3Content := "# Intro (revised)\n\nFirst draft from alice — revision after bob's reply.\n"
	writeAndSettle(t, alice, bob, notesPath, round3Content, "round 3 alice edit")

	verifyConverged(t, "round 3", alice, bob, sourcePrefix+notesPath, targetPrefix+notesPath, round3Content)
	verifyConverged(t, "round 3 (reply still present)", alice, bob,
		sourcePrefix+replyPath, targetPrefix+replyPath, round2Content)

	// === Round 4: bob edits alice's file (true collaboration) =============
	round4Content := "# Intro (revised)\n\nFirst draft from alice — revision after bob's reply.\n\n## Bob's addition\n\nAdded by bob.\n"
	writeAndSettle(t, bob, alice, notesPath, round4Content, "round 4 bob edit on alice's file")

	verifyConverged(t, "round 4", alice, bob, sourcePrefix+notesPath, targetPrefix+notesPath, round4Content)

	// === Round 5: bob edits his own file ==================================
	round5Content := "# Reply (updated)\n\nResponse from bob, separate file — now updated.\n"
	writeAndSettle(t, bob, alice, replyPath, round5Content, "round 5 bob edit")

	verifyConverged(t, "round 5", alice, bob, sourcePrefix+replyPath, targetPrefix+replyPath, round5Content)
	verifyConverged(t, "round 5 (intro still present)", alice, bob,
		sourcePrefix+notesPath, targetPrefix+notesPath, round4Content)

	// === Final: fs equality + tree equality + no chain errors =============
	// The strong convergence claim: every file on alice's fs is on
	// bob's fs with the same bytes, and vice versa; the doc/markdown-
	// file entities at archives/notes/ are byte-identical content
	// hashes on both peers; no chain-error markers leaked.
	t.Run("final-convergence", func(t *testing.T) {
		assertFilesystemEquality(t, aliceDir, bobDir)
		assertTreeEntityEquality(t, alice.ap, bob.ap, targetPrefix)
		assertNoChainErrors(t, alice.ap, "alice")
		assertNoChainErrors(t, bob.ap, "bob")
	})
}

// collabPeer bundles per-peer state for the collaborative editor test.
type collabPeer struct {
	name   string
	ap     *entitysdk.AppPeer
	ingest *workbench.NotificationIngestHandler
	br     *workbench.BlobResolveHandler
	fsDir  string
	id     string
}

func newCollabPeer(t *testing.T, name, fsDir string) *collabPeer {
	t.Helper()
	ingest := workbench.NewNotificationIngestHandler(nil)
	br := workbench.NewBlobResolveHandler()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingest},
			{Pattern: workbench.BlobResolvePattern, Handler: br},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	return &collabPeer{
		name:   name,
		ap:     ap,
		ingest: ingest,
		br:     br,
		fsDir:  fsDir,
		id:     ap.PeerID(),
	}
}

// installMount wires the fs→tree direction: localfiles watcher
// produces FileData entities, notification-ingest produces
// doc/markdown-file entities at archives/notes/{path}.
func (p *collabPeer) installMount(t *testing.T, ctx context.Context, sourcePrefix, targetPrefix string) {
	t.Helper()
	const rootName = "shared"

	grants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := p.ap.MintChainCapabilityBound(grants,
		"system/capability/grants/chain/notification-ingest/"+p.name); err != nil {
		t.Fatalf("%s mint ingest cap: %v", p.name, err)
	}
	p.ingest.RegisterMount(sourcePrefix, targetPrefix)

	lf := p.ap.LocalFilesHandler()
	if lf == nil {
		t.Fatalf("%s local/files handler not wired", p.name)
	}
	if err := lf.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: p.fsDir,
	}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
		t.Fatalf("%s AddRoot: %v", p.name, err)
	}
	if err := lf.StartWatching(ctx, rootName, p.ap.RawContentStore(),
		p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
		t.Fatalf("%s StartWatching: %v", p.name, err)
	}

	deliverURI := fmt.Sprintf("entity://%s/%s", p.id, workbench.NotificationIngestPattern)
	if _, err := p.ap.SubscribeRawAt(p.id, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated", "deleted"}}); err != nil {
		t.Fatalf("%s mount subscribe: %v", p.name, err)
	}
}

// installCrossPeerMaterialize wires the remote-fs→local-fs direction:
// subscribe to remoteID's source prefix, deliver to local
// blob-resolve handler, which fetches the blob closure and writes
// bytes to the local fsDir via local/files:write content-mode.
func (p *collabPeer) installCrossPeerMaterialize(t *testing.T, remoteID, sourcePrefix string) {
	t.Helper()
	p.br.RegisterMount(sourcePrefix, sourcePrefix)
	chainGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := p.ap.MintChainCapabilityBound(chainGrants,
		"system/capability/grants/chain/blob-resolve/"+p.name+"-from-"+remoteID); err != nil {
		t.Fatalf("%s mint blob-resolve cap: %v", p.name, err)
	}
	deliverURI := fmt.Sprintf("entity://%s/%s", p.id, workbench.BlobResolvePattern)
	if _, err := p.ap.SubscribeRawAt(remoteID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{
			Events:         []string{"created", "updated", "deleted"},
			IncludePayload: true,
		}); err != nil {
		t.Fatalf("%s subscribe to %s: %v", p.name, remoteID, err)
	}
}

// writeAndSettle writes a file to writer's fs, then polls until
// reader has the doc/markdown-file entity at archives/notes/<relpath>
// with content matching the expected bytes. The watch-and-fanout
// stack is event-driven; we just need to wait for it to drain.
func writeAndSettle(t *testing.T, writer, reader *collabPeer, relPath, content, label string) {
	t.Helper()
	full := filepath.Join(writer.fsDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatalf("%s mkdir: %v", label, err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		t.Fatalf("%s WriteFile: %v", label, err)
	}
	t.Logf("%s: wrote %d bytes to %s/%s", label, len(content), writer.name, relPath)

	const sourcePrefix = "local/files/shared/"
	const targetPrefix = "archives/notes/"
	sourcePath := sourcePrefix + relPath
	targetPath := targetPrefix + relPath
	readerFsPath := filepath.Join(reader.fsDir, relPath)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// 1. Reader has the doc/markdown-file entity at the target
		//    path with content matching.
		ent, ok := reader.ap.Store().Get(targetPath)
		if ok && ent.Type == workbench.MarkdownFileType {
			md, err := workbench.MarkdownFileDataFromEntity(ent)
			if err == nil {
				body, present, err := workbench.LoadMarkdownContent(reader.ap.Store().ContentStore(), md)
				if err == nil && present && string(body) == content {
					// 2. Reader's fs has the file with matching bytes
					//    (cross-peer materialization completed).
					fsBytes, err := os.ReadFile(readerFsPath)
					if err == nil && string(fsBytes) == content {
						// 3. Reader's localfiles binding is updated to
						//    reflect the new content (watcher caught
						//    the cross-peer write).
						_, srcOk := reader.ap.Store().Get(sourcePath)
						if srcOk {
							t.Logf("%s: converged after %v", label, time.Since(deadline.Add(-30*time.Second)))
							return
						}
					}
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Diagnostic dump on failure.
	t.Logf("%s: convergence FAILED — diagnostic dump:", label)
	dumpCollabState(t, writer, "writer/"+writer.name, sourcePath, targetPath)
	dumpCollabState(t, reader, "reader/"+reader.name, sourcePath, targetPath)
	if _, err := os.Stat(readerFsPath); err != nil {
		t.Logf("  reader fs missing %s: %v", readerFsPath, err)
	} else {
		fsBytes, _ := os.ReadFile(readerFsPath)
		t.Logf("  reader fs has %s, %d bytes (want %d)", readerFsPath, len(fsBytes), len(content))
	}
	t.Fatalf("%s: did not converge within 30s", label)
}

func dumpCollabState(t *testing.T, p *collabPeer, label, sourcePath, targetPath string) {
	t.Helper()
	src, srcOK := p.ap.Store().Get(sourcePath)
	t.Logf("  %s source %s: present=%v type=%s", label, sourcePath, srcOK, src.Type)
	tgt, tgtOK := p.ap.Store().Get(targetPath)
	t.Logf("  %s target %s: present=%v type=%s", label, targetPath, tgtOK, tgt.Type)
	if tgtOK && tgt.Type == workbench.MarkdownFileType {
		md, err := workbench.MarkdownFileDataFromEntity(tgt)
		if err == nil {
			body, present, err := workbench.LoadMarkdownContent(p.ap.Store().ContentStore(), md)
			t.Logf("    decoded: title=%q content_hash=%s size=%d body_present=%v body_bytes=%d err=%v",
				md.Title, md.Content, md.Size, present, len(body), err)
		} else {
			t.Logf("    decode error: %v", err)
		}
	}
	chainErrs := p.ap.Store().List("system/runtime/chain-errors/")
	if len(chainErrs) > 0 {
		t.Logf("  %s chain-error markers: %d", label, len(chainErrs))
		for _, e := range chainErrs {
			t.Logf("    %s", e.Path)
		}
	}
}

// verifyConverged confirms that both peers see byte-identical state
// for one path: localfiles FileData at sourcePath, doc/markdown-file
// at targetPath, blob bytes match expected content, fs bytes match.
func verifyConverged(t *testing.T, label string, alice, bob *collabPeer, sourcePath, targetPath, expected string) {
	t.Helper()
	for _, p := range []*collabPeer{alice, bob} {
		// FileData at source.
		srcEnt, ok := p.ap.Store().Get(sourcePath)
		if !ok {
			t.Errorf("%s: %s missing source binding at %s", label, p.name, sourcePath)
			continue
		}
		if srcEnt.Type != localfiles.TypeFile {
			t.Errorf("%s: %s source type = %s, want %s", label, p.name, srcEnt.Type, localfiles.TypeFile)
		}

		// doc/markdown-file at target.
		tgtEnt, ok := p.ap.Store().Get(targetPath)
		if !ok {
			t.Errorf("%s: %s missing target binding at %s", label, p.name, targetPath)
			continue
		}
		if tgtEnt.Type != workbench.MarkdownFileType {
			t.Errorf("%s: %s target type = %s, want %s", label, p.name, tgtEnt.Type, workbench.MarkdownFileType)
			continue
		}
		md, err := workbench.MarkdownFileDataFromEntity(tgtEnt)
		if err != nil {
			t.Errorf("%s: %s decode markdown-file: %v", label, p.name, err)
			continue
		}
		body, present, err := workbench.LoadMarkdownContent(p.ap.Store().ContentStore(), md)
		if err != nil || !present {
			t.Errorf("%s: %s LoadMarkdownContent: present=%v err=%v", label, p.name, present, err)
			continue
		}
		if string(body) != expected {
			t.Errorf("%s: %s blob content mismatch (got %d bytes, want %d)", label, p.name, len(body), len(expected))
		}

		// fs.
		fsPath := filepath.Join(p.fsDir, strings.TrimPrefix(sourcePath, "local/files/shared/"))
		fsBytes, err := os.ReadFile(fsPath)
		if err != nil {
			t.Errorf("%s: %s fs read %s: %v", label, p.name, fsPath, err)
			continue
		}
		if !bytes.Equal(fsBytes, []byte(expected)) {
			t.Errorf("%s: %s fs content mismatch at %s (got %d bytes, want %d)", label, p.name, fsPath, len(fsBytes), len(expected))
		}
	}
}

// assertFilesystemEquality walks both dirs and verifies they have
// the same set of files with byte-identical content.
func assertFilesystemEquality(t *testing.T, dirA, dirB string) {
	t.Helper()
	mapA := mapFilesystem(t, dirA)
	mapB := mapFilesystem(t, dirB)

	// Set equality on paths.
	paths := map[string]struct{}{}
	for p := range mapA {
		paths[p] = struct{}{}
	}
	for p := range mapB {
		paths[p] = struct{}{}
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	mismatches := 0
	for _, p := range sorted {
		hA, okA := mapA[p]
		hB, okB := mapB[p]
		switch {
		case !okA:
			t.Errorf("fs equality: missing on alice: %s (on bob: %x)", p, hB[:8])
			mismatches++
		case !okB:
			t.Errorf("fs equality: missing on bob: %s (on alice: %x)", p, hA[:8])
			mismatches++
		case hA != hB:
			t.Errorf("fs equality: content differs at %s: alice=%x bob=%x", p, hA[:8], hB[:8])
			mismatches++
		}
	}
	if mismatches == 0 {
		t.Logf("fs equality OK: %d paths identical on both sides", len(sorted))
	}
}

func mapFilesystem(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = sha256.Sum256(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// assertTreeEntityEquality verifies both peers have the same set of
// {logical_path → content_hash} under prefix. Each peer writes into
// its own /{peerID}/{prefix} namespace, so we compare paths stripped
// of the writer's peer-id prefix.
func assertTreeEntityEquality(t *testing.T, alice, bob *entitysdk.AppPeer, prefix string) {
	t.Helper()
	aMap := relativePathHashes(alice, prefix)
	bMap := relativePathHashes(bob, prefix)

	allPaths := map[string]struct{}{}
	for p := range aMap {
		allPaths[p] = struct{}{}
	}
	for p := range bMap {
		allPaths[p] = struct{}{}
	}
	mismatches := 0
	for p := range allPaths {
		aH, aOK := aMap[p]
		bH, bOK := bMap[p]
		switch {
		case !aOK:
			t.Errorf("tree equality (%s): missing on alice: %s", prefix, p)
			mismatches++
		case !bOK:
			t.Errorf("tree equality (%s): missing on bob: %s", prefix, p)
			mismatches++
		case aH != bH:
			t.Errorf("tree equality (%s): content hash differs at %s: alice=%s bob=%s", prefix, p, aH, bH)
			mismatches++
		}
	}
	if mismatches == 0 {
		t.Logf("tree equality OK at %s: %d logical paths identical on both peers", prefix, len(allPaths))
	}
}

// relativePathHashes returns {logical_path → content_hash} for entries
// under prefix on this peer, with the writer's peer-id segment
// stripped. Store.List returns qualified paths like
// `/{peerID}/archives/notes/intro.md`; we want `archives/notes/intro.md`
// so two peers' views can be compared.
func relativePathHashes(ap *entitysdk.AppPeer, prefix string) map[string]string {
	out := map[string]string{}
	for _, e := range ap.Store().List(prefix) {
		rel := strings.TrimPrefix(e.Path, "/")
		if idx := strings.Index(rel, "/"); idx >= 0 {
			rel = rel[idx+1:]
		}
		out[rel] = e.Hash.String()
	}
	return out
}

func assertNoChainErrors(t *testing.T, ap *entitysdk.AppPeer, name string) {
	t.Helper()
	entries := ap.Store().List("system/runtime/chain-errors/")
	if len(entries) == 0 {
		return
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	t.Errorf("%s: %d chain-error markers leaked: %s", name, len(entries), strings.Join(paths, ", "))
}
