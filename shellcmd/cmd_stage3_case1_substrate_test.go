package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"

	"github.com/fxamacker/cbor/v2"
)

// TestStage3_Case1_SubstrateExplicit is the smallest possible
// cross-peer substrate exercise: alice writes a file → bob fetches
// the file entity from alice's tree → bob pulls the blob (+chunks)
// via ContentClient → bob materializes to local disk via
// local/files:write content-mode.
//
// This is "Case 1: Sequential one-way" from STAGE-3-DESIGN-RESPONSE
// §4.2 in its most explicit shape: every cross-peer step is invoked
// directly so we can see exactly which substrate surfaces work. No
// subscriptions, no revision-follow chain — just the substrate
// dance. Subsequent test cases automate this via subscription +
// chain composition.
//
// What this validates end-to-end on the v1.3-Amendment-2 substrate:
//
//  1. The file entity on alice carries Content: hash (substrate
//     migration v1.2+).
//  2. Bob can fetch the file entity cross-peer via system/tree:get
//     (cap-checked at alice's dispatcher).
//  3. Bob can fetch the blob + chunks via system/content:get against
//     alice's namespace (cap-checked at alice's dispatcher; v3.5
//     §6.2 path_required MUST honored).
//  4. The substrate's content-addressed dedup property holds — the
//     same blob hash is present on both peers' content stores after
//     transfer.
//  5. Bob can materialize the blob to local disk via local/files:write
//     content-mode (no bytes traverse the wire — only the hash).
//  6. The file lands on bob's local filesystem with the original
//     content (byte-for-byte).
//
// What this DOES NOT test (next round): subscription-driven
// triggering, conflict resolution, large-file streaming, the L12
// 503 retry path.
func TestStage3_Case1_SubstrateExplicit(t *testing.T) {
	aliceDir := t.TempDir()
	bobDir := t.TempDir()
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	// --- Build two peers. Open-access grants so cross-peer dispatch
	//     works without cap-minting ceremonies; this test is about
	//     substrate composition, not authorization (separate concern
	//     covered in existing cross_peer_identity_test).
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		// Bob also needs the workbench notification-ingest handler
		// wired so that his local mount's reverse-write subscription
		// (set up by AddRoot) finds something on the inbox path. For
		// this explicit test we don't actually exercise the inbox
		// path — we dispatch :write directly — but the handler must
		// be registered so peer construction doesn't panic on the
		// missing reference.
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Bring up listeners + bidirectional connections.
	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	aliceID := alice.PeerID()

	// --- Alice mounts her dir at sourcePrefix. Watcher writes
	//     FileData entities to her tree as files appear on disk.
	aliceLF := alice.LocalFilesHandler()
	if aliceLF == nil {
		t.Fatal("alice local/files handler not wired")
	}
	if err := aliceLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: aliceDir,
	}, alice.RawContentStore(), alice.RawLocationIndex()); err != nil {
		t.Fatalf("alice AddRoot: %v", err)
	}
	if err := aliceLF.StartWatching(ctx, rootName, alice.RawContentStore(),
		alice.RawLocationIndex(), alice.IdentityHash()); err != nil {
		t.Fatalf("alice StartWatching: %v", err)
	}

	// --- Bob mounts HIS dir at the same source prefix. The mount
	//     registers the root config (so :write can target the prefix)
	//     and gives him a place to materialize files. We don't start
	//     bob's watcher because he's the sink in this one-way case
	//     and any auto-rechunk loop would muddy the test signal.
	bobLF := bob.LocalFilesHandler()
	if bobLF == nil {
		t.Fatal("bob local/files handler not wired")
	}
	if err := bobLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: bobDir,
	}, bob.RawContentStore(), bob.RawLocationIndex()); err != nil {
		t.Fatalf("bob AddRoot: %v", err)
	}

	// --- Alice writes hello.md to her filesystem. The watcher will
	//     pick this up within the debounce window and bind a
	//     local/files/file entity at sourcePrefix+"hello.md".
	mdContent := "# Stage 3 case 1\n\nSubstrate-explicit cross-peer file sync.\n"
	mdPath := filepath.Join(aliceDir, "hello.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0600); err != nil {
		t.Fatalf("alice write file: %v", err)
	}

	// Wait for alice's watcher to bind the FileData entity.
	wantSourcePath := sourcePrefix + "hello.md"
	if !pollUntilBound(alice, wantSourcePath, 5*time.Second) {
		t.Fatalf("alice's watcher never bound %s", wantSourcePath)
	}

	// --- Bob fetches alice's file entity via cross-peer tree:get.
	aliceFileURI := fmt.Sprintf("entity://%s/%s", aliceID, wantSourcePath)
	fileEnt, ok, err := bob.Get(aliceFileURI)
	if err != nil {
		t.Fatalf("bob fetch alice's file entity: %v", err)
	}
	if !ok {
		t.Fatalf("bob's tree:get on %s returned not-found", aliceFileURI)
	}
	if fileEnt.Type != localfiles.TypeFile {
		t.Fatalf("expected %s, got %s", localfiles.TypeFile, fileEnt.Type)
	}
	file, err := localfiles.FileDataFromEntity(fileEnt)
	if err != nil {
		t.Fatalf("decode FileData: %v", err)
	}
	if file.Content.IsZero() {
		t.Fatalf("file entity has zero Content hash — substrate migration not in effect?")
	}
	t.Logf("bob fetched alice's file entity: path=%s size=%d content_hash=%s",
		file.Path, file.Size, file.Content.String())

	// --- Pre-fetch state: bob's content store should NOT have the
	//     blob yet (we just connected; subscriptions/syncs aren't
	//     wired).
	bobCS := bob.RawContentStore()
	if _, hadBlob := bobCS.Get(file.Content); hadBlob {
		t.Logf("note: bob already had the blob (unexpected for case 1; sync may have leaked through connection handshake)")
	}

	// --- Bob pulls the blob closure via ContentClient against alice.
	//     This is the §7.2 algorithm: get blob manifest → identify
	//     missing chunks → batch-fetch → done.
	if err := bob.ContentAt(aliceID).FetchBlobClosure(ctx, file.Content); err != nil {
		t.Fatalf("bob FetchBlobClosure(alice, %s): %v", file.Content.String(), err)
	}

	// --- Post-fetch: bob's content store NOW has the blob.
	if _, hadBlob := bobCS.Get(file.Content); !hadBlob {
		t.Fatalf("after FetchBlobClosure, bob's content store does not have blob %s", file.Content.String())
	}
	t.Logf("bob's content store has the blob after FetchBlobClosure")

	// --- Bob materializes to disk via local/files:write content-mode.
	//     This is the L10-clean materialization trigger per arch's
	//     recommended pattern: cap-checked write op that resolves
	//     the blob hash locally and atom-writes the file.
	bobFileTreePath := sourcePrefix + "hello.md"
	writeReq := localfiles.WriteRequestData{
		Content: &file.Content,
	}
	writeReqRaw, err := ecf.Encode(writeReq)
	if err != nil {
		t.Fatalf("ecf.Encode write request: %v", err)
	}
	writeReqEnt, err := entity.NewEntity(localfiles.TypeWriteRequest, cbor.RawMessage(writeReqRaw))
	if err != nil {
		t.Fatalf("build write request entity: %v", err)
	}
	resource := &types.ResourceTarget{Targets: []string{bobFileTreePath}}
	writeResp, err := bob.Executor().ExecuteOnResource("local/files", "write", writeReqEnt, resource)
	if err != nil {
		t.Fatalf("bob local/files:write: %v", err)
	}
	if writeResp.Status != 200 {
		t.Fatalf("bob :write returned status %d", writeResp.Status)
	}

	// --- The file should now exist on bob's filesystem with the
	//     original content (byte-for-byte).
	bobFSPath := filepath.Join(bobDir, "hello.md")
	gotBytes, err := os.ReadFile(bobFSPath)
	if err != nil {
		t.Fatalf("read bob's hello.md: %v", err)
	}
	if string(gotBytes) != mdContent {
		t.Errorf("bob's hello.md content mismatch:\n  got:  %q\n  want: %q",
			string(gotBytes), mdContent)
	} else {
		t.Logf("bob's filesystem has the file with matching content (%d bytes)", len(gotBytes))
	}

	// --- Content-addressed dedup: both peers' content stores hold
	//     the same blob hash (the L10 substrate property — the
	//     blob_hash is computed identically because the bytes are
	//     identical, and FastCDC is determined).
	aliceCS := alice.RawContentStore()
	aliceBlob, aliceHas := aliceCS.Get(file.Content)
	bobBlob, bobHas := bobCS.Get(file.Content)
	if !aliceHas || !bobHas {
		t.Fatalf("dedup check: alice has blob = %v, bob has blob = %v", aliceHas, bobHas)
	}
	if aliceBlob.ContentHash != bobBlob.ContentHash {
		t.Errorf("blob hash divergence: alice=%s bob=%s",
			aliceBlob.ContentHash, bobBlob.ContentHash)
	}
}

// bringUpListener starts the peer's listener and waits for ready or
// errors out on timeout.
func bringUpListener(t *testing.T, ctx context.Context, ap *entitysdk.AppPeer, name string) {
	t.Helper()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ap.ListenReady(ctx, ready)
	}()
	select {
	case <-ready:
		return
	case err := <-errCh:
		t.Fatalf("%s listen: %v", name, err)
	case <-time.After(2 * time.Second):
		t.Fatalf("%s listen timeout", name)
	}
}

// pollUntilBound waits for the given tree path to have a binding on
// the peer, returning true on success and false on timeout.
func pollUntilBound(ap *entitysdk.AppPeer, path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Avoid Store.Has — use the peer's tree:get path which
		// matches the actual dispatch shape.
		_, ok, _ := ap.Get(path)
		if ok {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// keep the imports honest for IDE / golint.
var _ = strings.HasPrefix
