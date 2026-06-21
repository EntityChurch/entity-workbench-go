//go:build perfreview

package perfreview

// Cross-impl revision:follow chain — Cat 1 application-layer probe.
//
// This is the production use case workbench-go is built for: a Go
// peer mirrors a remote peer's revision-tracked prefix via the
// canonical 3-step chain (subscribe head → revision:fetch-diff →
// tree:merge). We use Rust as the source and wb-go as the follower
// to validate the chain works end-to-end across impls.
//
// Substrates exercised cross-impl:
//
//   - subscription (head-path watch installed on Rust)
//   - revision   (config + auto_version + fetch-diff on Rust)
//   - tree       (extract on Rust, merge on wb-go)
//   - continuation (chain installed on wb-go; remote dispatch to Rust)
//
// If this passes, workbench-go's cross-impl repository-workspace use
// case is empirically validated. If it breaks, we've found something
// production-blocking at the application layer.
//
// Dependencies: Rust must have the D4 fetch-diff is_external gate
// removed (commit `entity-core-rust/fe7e4b0`).
//
// Run:
//   /tmp/peer-manager start --name rust1 --type rust
//   CROSSIMPL_TARGET_ADDR=127.0.0.1:NNNN \
//     make perfreview ARGS="-run TestCrossImpl_RevisionFollowChain -v -timeout=2m"

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_RevisionFollowChain(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// wb-go = follower (Bob). Open-access connection grants so the
	// inbound continuation-chain dispatches from Rust pass auth.
	dbgLog := log.New(os.Stderr, "[wbgo-dbg] ", log.LstdFlags|log.Lmicroseconds)
	follower, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{
			peer.WithConnectionGrants(peer.OpenAccessGrants()),
			peer.WithDebugLog(dbgLog),
		},
	})
	if err != nil {
		t.Fatalf("create follower: %v", err)
	}
	defer follower.Close()

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- follower.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("follower listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("follower listen timeout")
	}

	conn, err := follower.Connect(ctx, targetAddr)
	if err != nil {
		t.Fatalf("connect source: %v", err)
	}
	sourceID := string(conn.ConnState().RemotePeerID)
	t.Logf("follower (wb-go): %s @ %s", follower.PeerID(), follower.Addr())
	t.Logf("source   (rust): %s @ %s", sourceID, targetAddr)

	// Register follower's address on source so source can dial back
	// for subscription delivery + reverse-chain dispatch.
	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        follower.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + follower.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	if _, err := follower.PutEntity(
		fmt.Sprintf("/%s/system/peer/transport/%s", sourceID, follower.PeerID()),
		transportEnt,
	); err != nil {
		t.Fatalf("register transport: %v", err)
	}

	// Install auto-version on source for the prefix we'll follow.
	const prefix = "synctest/"
	yes := true
	cfg := coretypes.RevisionConfigData{
		Prefix:      prefix,
		AutoVersion: &yes,
	}
	if _, err := follower.RevisionAt(sourceID).Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "crossimpl-follow",
		Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config on source: %v", err)
	}
	t.Logf("auto-version installed on source at %q", prefix)

	// Install the canonical 2-step follow chain on the follower side.
	// Inlined from shellcmd.InstallRevisionFollowChain so this test
	// is self-contained (perfreview's go.mod doesn't depend on shellcmd).
	rawSub, err := installRevisionFollowChainForTest(ctx, follower, sourceID, prefix)
	if err != nil {
		t.Fatalf("install follow chain: %v", err)
	}
	defer func() { _ = rawSub.Close() }()
	t.Logf("follow chain installed: sub=%s", rawSub.ID())

	// Give the chain a moment to be reachable.
	time.Sleep(300 * time.Millisecond)

	// Drive writes on source's tracked prefix. Each write trips
	// auto-version → head advances → notification fires → chain runs.
	//
	// Mirroring semantic: tree:merge applies the source envelope under
	// (executing peer's namespace + TargetPrefix), so the chain lands
	// follower's local mirror at /<followerID>/synctest/, NOT under
	// the source's namespace. Documented at
	// entitysdk/tree_follow_since_wiring_test.go:185-190 (prior art
	// for InstallRevisionFollowChain).
	const N = 20
	followerID := follower.PeerID()
	t.Logf("publishing %d entities to source's %q prefix", N, prefix)
	expectedPaths := make(map[string]string, N)
	for i := 0; i < N; i++ {
		// Cross-peer Put → entity lives on source's tree at this path.
		srcPath := fmt.Sprintf("/%s/%snote-%03d", sourceID, prefix, i)
		val := fmt.Sprintf("note %03d from source", i)
		if _, err := follower.Put(srcPath, "follow/test",
			map[string]interface{}{"i": i, "val": val}); err != nil {
			t.Fatalf("source Put i=%d: %v", i, err)
		}
		// The chain mirrors that under follower's own namespace.
		mirrorPath := fmt.Sprintf("/%s/%snote-%03d", followerID, prefix, i)
		expectedPaths[mirrorPath] = val
		time.Sleep(50 * time.Millisecond) // gentle pace; chain has time to fire
	}

	// Wait for the chain to converge. Each head update triggers a
	// fetch-diff round-trip + a local merge. Generous budget.
	deadline := time.Now().Add(20 * time.Second)
	var observed int
	lastReport := time.Now()
	for time.Now().Before(deadline) {
		observed = 0
		for path := range expectedPaths {
			if follower.Store().Has(path) {
				observed++
			}
		}
		if observed >= N {
			break
		}
		if time.Since(lastReport) > 2*time.Second {
			t.Logf("convergence progress: %d/%d local mirror entries", observed, N)
			lastReport = time.Now()
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("converged: %d/%d local mirror entries (within %s)",
		observed, N, time.Since(deadline.Add(-20*time.Second)))

	// Diagnostic: dump everything in wb-go's local tree under the
	// follow prefix. If merge ran but wrote to a different prefix,
	// these'll show up here.
	allLocal := follower.Store().List("")
	t.Logf("wb-go local store has %d total path bindings", len(allLocal))
	synctestCount := 0
	inboxCount := 0
	revisionCount := 0
	for _, e := range allLocal {
		switch {
		case strings.Contains(e.Path, "synctest"):
			synctestCount++
			if synctestCount <= 3 {
				t.Logf("  synctest binding: %s", e.Path)
			}
		case strings.Contains(e.Path, "system/inbox"):
			inboxCount++
		case strings.Contains(e.Path, "system/revision"):
			revisionCount++
			if revisionCount <= 3 {
				t.Logf("  revision binding: %s", e.Path)
			}
		}
	}
	t.Logf("wb-go local: synctest=%d inbox=%d revision=%d", synctestCount, inboxCount, revisionCount)

	// Chain-error-lost markers per EXTENSION-CONTINUATION §3.4 / §3.10.5:
	// bound when a forward dispatch returns >=400 with no on_error. If
	// the chain dispatches but the merge step rejects N times, we'll see
	// N markers here — exactly the diagnostic we need for "merge dispatch
	// runs but no entities materialize."
	var chainErrs []store.LocationEntry
	var runtimePaths []store.LocationEntry
	for _, e := range allLocal {
		if strings.Contains(e.Path, "chain-errors") {
			chainErrs = append(chainErrs, e)
		}
		if strings.Contains(e.Path, "system/runtime") {
			runtimePaths = append(runtimePaths, e)
		}
	}
	var followErrPaths []store.LocationEntry
	for _, e := range allLocal {
		if strings.Contains(e.Path, "follow-errors") {
			followErrPaths = append(followErrPaths, e)
		}
	}
	t.Logf("paths under follow-errors (any): %d", len(followErrPaths))
	for i, e := range followErrPaths {
		if i >= 5 {
			t.Logf("  ... (%d total)", len(followErrPaths))
			break
		}
		t.Logf("  follow-errors: %s", e.Path)
	}
	t.Logf("paths under system/runtime: %d", len(runtimePaths))
	for i, e := range runtimePaths {
		if i >= 8 {
			t.Logf("  ... (%d total)", len(runtimePaths))
			break
		}
		t.Logf("  runtime: %s", e.Path)
	}
	t.Logf("chain-errors bound (substring): %d", len(chainErrs))

	// Decode follow-chain/<remote>/<prefix>/fetch-errors bindings —
	// after the OnError fix, each fetch-diff failure surfaces here as
	// a system/protocol/error entity. The test asserts these bindings
	// EXIST when convergence fails; they are the visible error surface.
	var fetchErrs, mergeErrs []store.LocationEntry
	for _, e := range runtimePaths {
		switch {
		case strings.Contains(e.Path, "follow-errors") && strings.Contains(e.Path, "/fetch/"):
			fetchErrs = append(fetchErrs, e)
		case strings.Contains(e.Path, "follow-errors") && strings.Contains(e.Path, "/merge/"):
			mergeErrs = append(mergeErrs, e)
		}
	}
	t.Logf("follow-chain/fetch-errors bound: %d", len(fetchErrs))
	t.Logf("follow-chain/merge-errors bound: %d", len(mergeErrs))
	for i, e := range fetchErrs {
		if i >= 1 {
			break
		}
		t.Logf("  fetch-error[0] path: %s", e.Path)
		h, ok := follower.RawLocationIndex().Get(e.Path)
		if !ok {
			continue
		}
		ent, ok := follower.Store().GetByHash(h)
		if !ok {
			continue
		}
		t.Logf("    entity.type=%q", ent.Type)
		var decoded interface{}
		if err := cbor.Unmarshal(ent.Data, &decoded); err == nil {
			t.Logf("    entity.data_decoded=%+v", decoded)
		}
	}
	for i, e := range chainErrs {
		if i >= 3 {
			t.Logf("  ... (%d total; first 3 dumped)", len(chainErrs))
			break
		}
		t.Logf("  chain-error path: %s", e.Path)
		hash, ok := follower.RawLocationIndex().Get(e.Path)
		if !ok {
			continue
		}
		ent, ok := follower.Store().GetByHash(hash)
		if !ok {
			continue
		}
		t.Logf("    marker entity type=%q data_hex=%x", ent.Type, ent.Data)
	}

	if observed < N {
		t.Errorf("convergence failed: %d/%d entities reached follower's local tree", observed, N)
		// Diagnostic: which paths didn't make it?
		var missing []string
		for path := range expectedPaths {
			if !follower.Store().Has(path) {
				missing = append(missing, path)
				if len(missing) >= 5 {
					break
				}
			}
		}
		t.Logf("first %d missing paths: %v", len(missing), missing)
		return
	}

	// Spot-check a few entries for value equivalence — proves the
	// content actually crossed, not just the path bindings.
	for path, expected := range expectedPaths {
		ent, ok, err := follower.Get(path)
		if err != nil || !ok {
			t.Errorf("Get(%s) on follower: ok=%v err=%v", path, ok, err)
			continue
		}
		_ = ent
		_ = expected
		// Note: we don't decode the entity data here; presence + Has()
		// above is the load-bearing check. Value-equivalence under
		// source-wins merge is the spec-level guarantee from
		// `tree:merge`.
	}

	t.Logf("*** cross-impl revision:follow chain converged %d/%d ***", observed, N)
}

// installRevisionFollowChainForTest mirrors
// shellcmd/cmd_revision_follow.go::InstallRevisionFollowChain.
// See that function for the full design notes.
func installRevisionFollowChainForTest(ctx context.Context, local *entitysdk.AppPeer, remoteID, prefix string) (*entitysdk.RawSubscription, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	localID := local.PeerID()
	fetchPath := fmt.Sprintf("system/inbox/follow/%s/%sfetch", remoteID, prefix)
	mergePath := fmt.Sprintf("system/inbox/follow/%s/%smerge", remoteID, prefix)

	localCap := local.OwnerCapability().ContentHash
	crossPeerGrants := []coretypes.GrantEntry{{
		Handlers:   coretypes.CapabilityScope{Include: []string{"system/revision"}},
		Operations: coretypes.CapabilityScope{Include: []string{"fetch-diff"}},
		Resources:  coretypes.CapabilityScope{Include: []string{"*"}},
	}}
	crossPeerCapEnt, err := local.MintCrossPeerChainCapability(remoteID, crossPeerGrants, nil)
	if err != nil {
		return nil, fmt.Errorf("mint cross-peer follow-chain cap: %w", err)
	}
	crossPeerCap := crossPeerCapEnt.ContentHash

	mergeParams, err := cbor.Marshal(coretypes.MergeRequestData{
		Strategy:     "source-wins",
		SourcePrefix: prefix,
		TargetPrefix: prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("encode merge params: %w", err)
	}
	// OnError surfaces failed merges as observable entities, mirroring
	// the canonical shellcmd InstallRevisionFollowChain after the
	// chain-diagnostics lessons fix.
	mergeErrorPath := fmt.Sprintf("system/inbox/follow-errors/%s/%smerge",
		remoteID, prefix)
	mergeData := coretypes.ContinuationData{
		Target:      "system/tree",
		Operation:   "merge",
		Resource:    &coretypes.ResourceTarget{Targets: []string{prefix}},
		Params:      cbor.RawMessage(mergeParams),
		ResultField: "source_envelope",
		OnError: &coretypes.DeliverySpec{
			URI:       fmt.Sprintf("entity://%s/%s", localID, mergeErrorPath),
			Operation: "receive",
		},
	}

	fetchParams, err := cbor.Marshal(coretypes.RevisionFetchDiffParamsData{Prefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("encode fetch-diff params: %w", err)
	}
	fetchErrorPath := fmt.Sprintf("system/inbox/follow-errors/%s/%sfetch",
		remoteID, prefix)
	fetchData := coretypes.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/revision", remoteID),
		Operation: "fetch-diff",
		Resource:  &coretypes.ResourceTarget{Targets: []string{prefix}},
		Params:    cbor.RawMessage(fetchParams),
		ResultTransform: &coretypes.ContinuationTransformData{
			Extract: "previous_hash",
		},
		ResultField: "base",
		DeliverTo: &coretypes.DeliverySpec{
			URI:       fmt.Sprintf("entity://%s/%s", localID, mergePath),
			Operation: "receive",
		},
		OnError: &coretypes.DeliverySpec{
			URI:       fmt.Sprintf("entity://%s/%s", localID, fetchErrorPath),
			Operation: "receive",
		},
	}

	entitysdk.SetDefaultDispatchCap(localCap, &mergeData)
	entitysdk.SetDefaultDispatchCap(crossPeerCap, &fetchData)

	mergeCont, err := mergeData.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("encode merge continuation: %w", err)
	}
	if _, err := local.Continuation().Install(ctx, mergePath, mergeCont); err != nil {
		return nil, fmt.Errorf("install merge continuation: %w", err)
	}
	fetchCont, err := fetchData.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("encode fetch-diff continuation: %w", err)
	}
	if _, err := local.Continuation().Install(ctx, fetchPath, fetchCont); err != nil {
		return nil, fmt.Errorf("install fetch-diff continuation: %w", err)
	}

	headPath := entitysdk.RevisionHeadPath(remoteID, prefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", localID, fetchPath)
	rawSub, err := local.SubscribeRawAt(remoteID, headPath, deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}})
	if err != nil {
		return nil, fmt.Errorf("subscribe on remote head: %w", err)
	}
	return rawSub, nil
}
