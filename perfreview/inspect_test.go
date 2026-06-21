//go:build perfreview

package perfreview

// End-to-end exercise of perfreview/inspect helpers + diagnosis of
// the F-CIMP-7 OnError dispatch behavior. Installs:
//
//   - A tap at the merge inbox path (catches direct fetch-step
//     deliveries)
//   - A tap at the fetch-error inbox path (catches OnError fires
//     from the fetch step — should see Python's base_not_a_version
//     errors here cleanly)
//   - A tap at the merge-error inbox path (catches OnError fires
//     from the merge step)
//
// Then enumerates chain-error markers and dumps representative
// entries. Single test, comprehensive surface, no theorizing.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/inspect"
)

func TestCrossImpl_InspectChainTaps(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	targetImpl := os.Getenv("CROSSIMPL_TARGET_IMPL")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required")
	}
	if targetImpl == "" {
		targetImpl = "unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	follower, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{
			peer.WithConnectionGrants(peer.OpenAccessGrants()),
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
		t.Fatalf("connect: %v", err)
	}
	sourceID := string(conn.ConnState().RemotePeerID)
	localID := follower.PeerID()

	transportEnt, _ := coretypes.TCPProfileData{
		PeerID:        localID,
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + follower.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if _, err := follower.PutEntity(
		fmt.Sprintf("/%s/system/peer/transport/%s", sourceID, localID),
		transportEnt,
	); err != nil {
		t.Fatalf("register transport: %v", err)
	}

	const prefix = "synctest/"
	yes := true
	cfg := coretypes.RevisionConfigData{Prefix: prefix, AutoVersion: &yes}
	if _, err := follower.RevisionAt(sourceID).Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set", Name: "inspect-probe", Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	mergePath := fmt.Sprintf("system/inbox/follow/%s/%smerge", sourceID, prefix)
	fetchPath := fmt.Sprintf("system/inbox/follow/%s/%sfetch", sourceID, prefix)
	fetchErrorPath := fmt.Sprintf("system/inbox/follow-errors/%s/%sfetch", sourceID, prefix)
	mergeErrorPath := fmt.Sprintf("system/inbox/follow-errors/%s/%smerge", sourceID, prefix)

	// Three taps — covering all three observable surfaces.
	mergeTap, err := inspect.InstallTap(follower, mergePath)
	if err != nil {
		t.Fatalf("install merge tap: %v", err)
	}
	defer mergeTap.Close()

	fetchErrTap, err := inspect.InstallTap(follower, fetchErrorPath)
	if err != nil {
		t.Fatalf("install fetch-error tap: %v", err)
	}
	defer fetchErrTap.Close()

	mergeErrTap, err := inspect.InstallTap(follower, mergeErrorPath)
	if err != nil {
		t.Fatalf("install merge-error tap: %v", err)
	}
	defer mergeErrTap.Close()

	// Install ONLY the fetch step continuation. The merge step is
	// the tap (intentionally breaks merge for observability — we
	// just want to see what arrives).
	crossPeerCapEnt, err := follower.MintCrossPeerChainCapability(sourceID,
		[]coretypes.GrantEntry{{
			Handlers:   coretypes.CapabilityScope{Include: []string{"system/revision"}},
			Operations: coretypes.CapabilityScope{Include: []string{"fetch-diff"}},
			Resources:  coretypes.CapabilityScope{Include: []string{"*"}},
		}}, nil)
	if err != nil {
		t.Fatalf("mint cap: %v", err)
	}
	crossPeerCap := crossPeerCapEnt.ContentHash

	fetchParams, _ := cbor.Marshal(coretypes.RevisionFetchDiffParamsData{Prefix: prefix})
	fetchData := coretypes.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/revision", sourceID),
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
	entitysdk.SetDefaultDispatchCap(crossPeerCap, &fetchData)
	fetchCont, _ := fetchData.ToEntity()
	if _, err := follower.Continuation().Install(ctx, fetchPath, fetchCont); err != nil {
		t.Fatalf("install fetch continuation: %v", err)
	}

	headPath := entitysdk.RevisionHeadPath(sourceID, prefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", localID, fetchPath)
	rawSub, err := follower.SubscribeRawAt(sourceID, headPath, deliverURI, "receive",
		entitysdk.SubscribeOpts{
			Events: []string{"created", "updated"},
			Limits: &coretypes.SubscriptionLimitsData{RateLimit: ptrU64(1_000_000)},
		})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = rawSub.Close() }()

	time.Sleep(300 * time.Millisecond)

	const N = 20
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("/%s/%snote-%03d", sourceID, prefix, i)
		if _, err := follower.Put(path, "follow/test",
			map[string]interface{}{"i": i, "val": fmt.Sprintf("note-%03d", i)}); err != nil {
			t.Fatalf("put i=%d: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(5 * time.Second)

	t.Logf("\n=== Inspect surface vs %s ===", targetImpl)
	t.Logf("merge tap captures:        %d", mergeTap.Count())
	t.Logf("fetch-error tap captures:  %d", fetchErrTap.Count())
	t.Logf("merge-error tap captures:  %d", mergeErrTap.Count())

	chainErrs := inspect.FindChainErrors(follower)
	t.Logf("chain-error-lost markers:  %d", len(chainErrs))

	// Dump first capture from each tap that has any.
	dumpFirst := func(label string, captures []inspect.Capture) {
		if len(captures) == 0 {
			t.Logf("%s: no captures", label)
			return
		}
		t.Logf("--- %s [0] ---", label)
		t.Logf("%s", inspect.PrettyPrint(captures[0]))
	}
	dumpFirst("merge", mergeTap.Captures())
	dumpFirst("fetch-error", fetchErrTap.Captures())
	dumpFirst("merge-error", mergeErrTap.Captures())

	// If we got fetch-error captures, we've validated the OnError
	// route. Their decoded result should be Python's
	// base_not_a_version error.
	if fetchErrTap.Count() > 0 {
		t.Logf("\n*** OnError surface is working: %d fetch-step errors captured cleanly ***",
			fetchErrTap.Count())
	}

	// Browse for follow-errors paths that might have other shapes.
	followErrs := inspect.FindUnder(follower, "follow-errors")
	t.Logf("\nfollow-errors path bindings on follower: %d", len(followErrs))
	for i, e := range followErrs {
		if i >= 5 {
			t.Logf("  ... (%d total)", len(followErrs))
			break
		}
		t.Logf("  %s", e.Path)
	}
}
