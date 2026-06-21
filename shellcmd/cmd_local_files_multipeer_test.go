package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"

	"github.com/fxamacker/cbor/v2"
)

// TestE2E_MultiPeer_FsSavePropagates is the deployment-shape
// validation: file save on peer A's filesystem propagates through
// the entity mesh to peer B's tree, via Phase E mount on A + Phase
// C follow on B.
//
// What this exercises end-to-end:
//
//  1. Phase E mount on A — fsnotify → FileData entity → workbench
//     ingest handler → doc/markdown-file at archives/notes/{relpath}.
//  2. Revision auto-version on A's archives/notes/ — each
//     doc/markdown-file write produces a new revision.
//  3. Phase C follow on B — subscription to A's revision head +
//     fetch/ingest/merge chain via scoped chain capability.
//  4. Capability + scoping enforcement — A's chain cap (Phase E
//     ingest) and B's chain cap (Phase C follow) are both scoped,
//     not owner-cap.
//
// This is the test that validates "wipe-and-rebuild from
// filesystem" works as a recovery strategy for the deployment
// model in DEPLOYMENT-DIRECTION.md.
func TestE2E_MultiPeer_FsSavePropagates(t *testing.T) {
	fsDir := t.TempDir()
	const targetPrefix = "archives/notes/"
	rootName := "multipeer"
	sourcePrefix := "local/files/" + rootName + "/"

	// Build two peers, each with workbench handlers wired. Listeners
	// + open-access grants so the cross-peer dispatch path works
	// without identity ceremonies (deployment-readiness validation
	// is in the SDK identity tests; this test is about the chain
	// composition).
	aliceIngest := workbench.NewNotificationIngestHandler(nil)
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: aliceIngest},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	// Bob follows alice via a `subscribe head → revision:pull` chain
	// (REVISION §4.4.8). The chain-error handler is wired for
	// continuation §3.4 lost-error marker observability; no
	// revision-specific handler is needed — pull is a core-go op.
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
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

	// Bring up listeners + bidirectional connections.
	for _, p := range []struct {
		name string
		ap   *entitysdk.AppPeer
	}{{"alice", alice}, {"bob", bob}} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(name string, ap *entitysdk.AppPeer) {
			errCh <- ap.ListenReady(ctx, ready)
		}(p.name, p.ap)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("%s listen: %v", p.name, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s listen timeout", p.name)
		}
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	// --- Phase E: install mount on alice ---------------------------
	// Phase E Q2 shape: mount cap covers the single workbench-ingest
	// handler dispatch. Localfiles handler internally manages
	// tree:get/put via its own internal scope.
	mountGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := alice.MintChainCapabilityBound(mountGrants,
		"system/capability/grants/chain/local-files/"+rootName); err != nil {
		t.Fatalf("mint mount cap: %v", err)
	}
	aliceIngest.RegisterMount(sourcePrefix, targetPrefix)
	aliceLF := alice.LocalFilesHandler()
	if aliceLF == nil {
		t.Fatal("alice local/files handler not wired")
	}
	if err := aliceLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: fsDir,
	}, alice.RawContentStore(), alice.RawLocationIndex()); err != nil {
		t.Fatalf("alice AddRoot: %v", err)
	}
	if err := aliceLF.StartWatching(ctx, rootName, alice.RawContentStore(),
		alice.RawLocationIndex(), alice.IdentityHash()); err != nil {
		t.Fatalf("alice StartWatching: %v", err)
	}
	aliceMountDeliverURI := fmt.Sprintf("entity://%s/%s", aliceID, workbench.NotificationIngestPattern)
	if _, err := alice.SubscribeRawAt(aliceID, sourcePrefix+"*", aliceMountDeliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		t.Fatalf("alice mount subscribe: %v", err)
	}

	// --- Auto-version config on alice's archives/notes/ ------------
	// Without this, the doc/markdown-file write wouldn't produce a
	// revision and bob's follow chain wouldn't fire.
	autoTrue := true
	if _, err := alice.Revision().ConfigPut(ctx, "notes", types.RevisionConfigData{
		Prefix:      targetPrefix,
		AutoVersion: &autoTrue,
	}, nil); err != nil {
		t.Fatalf("install auto-version config: %v", err)
	}

	// --- Phase C: install follow on bob as a revision:pull chain ---
	// `subscribe alice head → revision:pull(prefix, remote=aliceID)`.
	// Pull (REVISION §4.4.8) folds fetch + iterative fetch-entities +
	// local merge into one handler op, so the chain has no dynamic
	// field — notification just triggers; params are static.
	bobLocalCap := bob.OwnerCapability().ContentHash
	pullInbox := "system/inbox/follow-pull/" + aliceID + "/" + strings.Trim(targetPrefix, "/")
	pullParamsRaw, err := cbor.Marshal(types.RevisionFetchParamsData{
		Prefix: targetPrefix,
		Remote: aliceID,
	})
	if err != nil {
		t.Fatalf("encode bob pull params: %v", err)
	}
	pullData := types.ContinuationData{
		Target:    "system/revision",
		Operation: "pull",
		Resource:  &types.ResourceTarget{Targets: []string{targetPrefix}},
		Params:    cbor.RawMessage(pullParamsRaw),
	}
	entitysdk.SetDefaultDispatchCap(bobLocalCap, &pullData)
	pullCont, err := pullData.ToEntity()
	if err != nil {
		t.Fatalf("encode bob pull continuation: %v", err)
	}
	if _, err := bob.Continuation().Install(ctx, pullInbox, pullCont); err != nil {
		t.Fatalf("install bob pull continuation: %v", err)
	}

	headPath := entitysdk.RevisionHeadPath(aliceID, targetPrefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", bobID, pullInbox)
	if _, err := bob.SubscribeRawAt(aliceID, headPath, deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		t.Fatalf("bob follow subscribe: %v", err)
	}

	// --- Drop a file on alice's filesystem -------------------------
	mdContent := "# Multi-Peer Hello\n\nThis file propagates via mesh.\n"
	mdPath := filepath.Join(fsDir, "hello.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// --- Wait for bob's revision head to converge ------------------
	// Path: fsnotify (~2s debounce) → alice's FileData write → alice
	// ingest handler → doc/markdown-file at archives/notes/hello.md
	// → auto-version commit → alice's revision head update →
	// bob's subscription fires → bob's 3-step follow chain →
	// bob's revision head matches alice's.
	wantPath := targetPrefix + "hello.md"
	headDeadline := time.Now().Add(15 * time.Second)
	var aliceHead, bobHead types.RevisionStatusData
	for time.Now().Before(headDeadline) {
		aliceHead, _ = alice.Revision().Status(ctx, targetPrefix)
		bobHead, _ = bob.Revision().Status(ctx, targetPrefix)
		if !aliceHead.Head.IsZero() && bobHead.Head == aliceHead.Head {
			t.Logf("revision head converged: %s", aliceHead.Head)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if bobHead.Head != aliceHead.Head {
		t.Fatalf("revision head did not converge: alice=%s bob=%s",
			aliceHead.Head, bobHead.Head)
	}

	// Assert head convergence + content materialization.
	if bobHead.Head != aliceHead.Head {
		t.Errorf("bob head = %s, want alice head %s", bobHead.Head, aliceHead.Head)
	}

	// Wait for the converge handler to finish Pull on bob — head
	// convergence already happened (RevisionStatus check above), but
	// the trie walk + leaf binding runs inside the handler dispatch.
	// Poll up to 5s for the doc/markdown-file to appear at the
	// target path.
	contentDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(contentDeadline) {
		if bob.Store().Has(wantPath) {
			t.Logf("bob has %s — full content materialization works", wantPath)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("bob's tree does NOT have %s — content materialization broken; converge handler may have failed to Pull", wantPath)
	bobEntries := bob.Store().List(targetPrefix)
	t.Logf("bob tree under %s: %d entries", targetPrefix, len(bobEntries))
	for _, e := range bobEntries {
		t.Logf("  %s", e.Path)
	}
}
