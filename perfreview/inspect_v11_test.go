//go:build perfreview

package perfreview

// Cross-impl revision-follow probe using v1.1 parallel hooks
// (dispatch + wire + binding + subscription emit/deliver) instead of
// the original tap-as-handler shape. Validates the new core-go hook
// surfaces land correctly and replicate F-CIMP-7's clean histogram
// without intercepting the production handler flow.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/inspect"
)

func TestCrossImpl_InspectV11Hooks(t *testing.T) {
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

	// All four parallel hooks at construction. Note: DispatchTap filters
	// to follow-inbox dispatches; WireRecorder captures everything;
	// BindingStream + content stream observe local state changes.
	stream := &inspect.ContentStream{}
	bindings := &inspect.BindingStream{}
	wire := &inspect.WireRecorder{}
	mergeTap := inspect.NewDispatchTap("system/inbox/follow")

	follower, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{
			peer.WithConnectionGrants(peer.OpenAccessGrants()),
			stream.PeerOption(),
			bindings.PeerOption(),
			wire.PeerOption(),
			mergeTap.PeerOption(),
		},
	})
	if err != nil {
		t.Fatalf("create follower: %v", err)
	}
	defer follower.Close()

	// SubscriptionTracer attaches post-construction via engine accessor.
	subs := &inspect.SubscriptionTracer{}
	subs.Attach(follower.SubscriptionEngine())

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
		Action: "set", Name: "v11-hooks", Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	mergePath := fmt.Sprintf("system/inbox/follow/%s/%smerge", sourceID, prefix)
	fetchPath := fmt.Sprintf("system/inbox/follow/%s/%sfetch", sourceID, prefix)

	crossPeerCapEnt, err := follower.MintCrossPeerChainCapability(sourceID,
		[]coretypes.GrantEntry{{
			Handlers:   coretypes.CapabilityScope{Include: []string{"system/revision"}},
			Operations: coretypes.CapabilityScope{Include: []string{"fetch-diff"}},
			Resources:  coretypes.CapabilityScope{Include: []string{"*"}},
		}}, nil)
	if err != nil {
		t.Fatalf("mint cap: %v", err)
	}

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
	}
	entitysdk.SetDefaultDispatchCap(crossPeerCapEnt.ContentHash, &fetchData)
	fetchCont, _ := fetchData.ToEntity()
	if _, err := follower.Continuation().Install(ctx, fetchPath, fetchCont); err != nil {
		t.Fatalf("install fetch continuation: %v", err)
	}

	// Legacy tap at merge path acts as a synthetic sink that 200-acks the
	// chain (so it converges without wiring tree:merge end-to-end). The
	// point of this probe is to validate the NEW parallel hooks fire
	// correctly alongside; the chain sink is incidental.
	mergeSink, err := inspect.InstallTap(follower, mergePath)
	if err != nil {
		t.Fatalf("install merge sink tap: %v", err)
	}
	defer mergeSink.Close()

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
	runStart := stream.Count()
	bindStart := bindings.Count()

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

	stream.Stop()
	bindings.Stop()
	wire.Stop()
	mergeTap.Stop()
	subs.Stop()

	t.Logf("\n=== v1.1 hook surface vs %s ===", targetImpl)
	t.Logf("content-stream events:       %d total (%d run-window)", stream.Count(), stream.Count()-runStart)
	t.Logf("binding-stream events:       %d total (%d run-window)", bindings.Count(), bindings.Count()-bindStart)
	t.Logf("wire frames inbound+outbound: %d", wire.Count())
	t.Logf("dispatch-tap captures:        %d (filter=%q)", mergeTap.Count(), "system/inbox/follow")
	t.Logf("subscription emits:           %d (LOCAL emitter; 0 expected — we're the SUBSCRIBER)", subs.CountEmits())
	t.Logf("subscription delivers:        %d (LOCAL deliveries; 0 expected — source peer delivers)", subs.CountDelivers())
	t.Logf("merge-sink tap captures:     %d (legacy tap as chain sink)", mergeSink.Count())
	t.Logf("chain-error-lost markers:    %d", len(inspect.FindChainErrors(follower)))

	t.Logf("\n--- dispatch exit status histogram (filter=follow inbox) ---")
	for status, count := range mergeTap.CountByStatus() {
		t.Logf("  %d × status=%d", count, status)
	}

	t.Logf("\n--- subscription delivery status histogram ---")
	for status, count := range subs.CountByDeliveryStatus() {
		t.Logf("  %d × status=%d", count, status)
	}

	t.Logf("\n--- wire root-type histogram ---")
	wireHist := wire.CountByRootType()
	type kv struct {
		k string
		v int
	}
	var wrows []kv
	for k, v := range wireHist {
		wrows = append(wrows, kv{k, v})
	}
	sort.Slice(wrows, func(i, j int) bool { return wrows[i].v > wrows[j].v })
	for _, r := range wrows {
		t.Logf("  %5d × %s", r.v, r.k)
	}

	// Assertions: the four parallel hooks all observe activity for the
	// canonical follow chain. F-CIMP-7 is fixed → no chain errors. The
	// merge sink (legacy tap) captures every fetch result. The dispatch
	// hook observes BOTH the inbound deliveries to fetch/merge inboxes
	// AND the local dispatches the chain framework drives — so its
	// count is roughly mergeSink.Count() × 4 (entry+exit × 2 paths).
	if cnt := len(inspect.FindChainErrors(follower)); cnt > 0 {
		t.Errorf("expected 0 chain-error markers, got %d (F-CIMP-7 regression?)", cnt)
	}
	if wire.Count() == 0 {
		t.Errorf("WireRecorder saw no frames — cross-peer hook gap?")
	}
	if mergeTap.Count() == 0 {
		t.Errorf("DispatchTap saw no follow-inbox dispatches — hook gap?")
	}
	if bindings.Count()-bindStart == 0 {
		t.Errorf("BindingStream saw no run-window bindings — hook gap?")
	}
	if mergeSink.Count() == 0 {
		t.Errorf("Legacy merge-sink saw no captures — chain didn't reach merge")
	}
}
