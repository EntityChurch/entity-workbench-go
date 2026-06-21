//go:build perfreview

package perfreview

// Comprehensive inspect probe: content stream + path tap + chain-
// error enumeration in one test. Shows what diagnostic surface is
// possible TODAY with no core-go changes.
//
// The content stream captures EVERY entity that lands in wb-go's
// content store during the run. Filter by type to focus. This is
// the highest-leverage primitive — it shows everything the chain
// produces, including entities that never get bound to observable
// paths (or that get bound and then unbound during cleanup).

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

func TestCrossImpl_InspectFullSurface(t *testing.T) {
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

	// Content stream — captures every NEW entity in the store.
	stream := &inspect.ContentStream{}
	defer stream.Stop()

	follower, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{
			peer.WithConnectionGrants(peer.OpenAccessGrants()),
			stream.PeerOption(),
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
	t.Logf("follower=%s source=%s (%s)", localID, sourceID, targetImpl)

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
		Action: "set", Name: "inspect-full", Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	mergePath := fmt.Sprintf("system/inbox/follow/%s/%smerge", sourceID, prefix)
	fetchPath := fmt.Sprintf("system/inbox/follow/%s/%sfetch", sourceID, prefix)

	// Snapshot the content-stream baseline AFTER setup so we don't
	// count the boot entities (cap chains, transport, revision
	// config, identity bundle, etc.).
	baseline := stream.Count()

	// Filter to chain-relevant types: protocol errors, envelopes,
	// continuation results, snapshots, inbox deliveries. Everything
	// else is noise (trie nodes, cap signatures, etc.).
	// (Empty filter means "everything"; we narrow only for the dump.)

	// Tap at the merge inbox to see Python's deliver_async results
	// directly.
	mergeTap, err := inspect.InstallTap(follower, mergePath)
	if err != nil {
		t.Fatalf("install merge tap: %v", err)
	}
	defer mergeTap.Close()

	// Install fetch step only; merge step IS the tap.
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

	t.Logf("\n=== Inspect: full surface vs %s ===", targetImpl)
	t.Logf("content-stream events:        %d total (%d baseline boot, %d run-window)",
		stream.Count(), baseline, stream.Count()-runStart)
	t.Logf("merge-tap captures:           %d", mergeTap.Count())
	t.Logf("chain-error-lost markers:     %d", len(inspect.FindChainErrors(follower)))

	// Histogram by type — the high-leverage chain-debug view.
	hist := stream.CountByType()
	type kv struct {
		k string
		v int
	}
	var rows []kv
	for k, v := range hist {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v != rows[j].v {
			return rows[i].v > rows[j].v
		}
		return rows[i].k < rows[j].k
	})

	t.Logf("\n--- content-store histogram (all entities stored during run) ---")
	for _, r := range rows {
		if r.v >= 2 || isChainRelevantType(r.k) {
			t.Logf("  %6d × %s", r.v, r.k)
		}
	}

	// Focus on protocol errors — these are the smoking guns.
	errors := stream.EntitiesOfType("system/protocol/error")
	t.Logf("\n--- system/protocol/error entities: %d ---", len(errors))
	for i, h := range errors {
		if i >= 3 {
			t.Logf("  ... (%d total)", len(errors))
			break
		}
		d := inspect.DumpEntityByHash(follower, h)
		if d == nil {
			continue
		}
		t.Logf("  #%d hash=%s", i, h.String())
		t.Logf("       data=%+v", d.Data)
	}

	// Confirm: merge-tap captures should report the same shapes the
	// content-stream histogram surfaces (both routes to the same
	// truth from different angles — tap from path side, stream from
	// content side).
	deliveryStatuses := map[uint]int{}
	for _, c := range mergeTap.Captures() {
		if c.IsDelivery {
			deliveryStatuses[c.DeliveryStatus]++
		}
	}
	t.Logf("\n--- merge-tap delivery status histogram ---")
	for status, count := range deliveryStatuses {
		t.Logf("  %d × status=%d", count, status)
	}
}

func isChainRelevantType(t string) bool {
	switch {
	case t == "system/protocol/error",
		t == "system/protocol/inbox/delivery",
		t == "system/envelope",
		t == "system/tree/snapshot",
		t == "system/revision/entry",
		t == "system/runtime/tap/ack":
		return true
	}
	return false
}
