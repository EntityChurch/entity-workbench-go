//go:build perfreview

package perfreview

// Merge-inbox dispatch tap. Captures the raw bytes wb-go receives on
// the merge inbox path so we can byte-diff what Rust sends vs what
// Python sends for the same revision:follow chain. This is the
// minimum diagnostic to close F-CIMP-7 (revision:follow vs Python
// fails 19/20 with snapshot_not_found from tree:merge).
//
// Strategy:
//   - Install fetch-diff continuation at the fetch inbox path
//   - At the merge inbox path, register a TAP handler (not a merge
//     continuation). The tap captures req.Params (the InboxDelivery
//     entity) at a debug path under system/runtime/merge-tap/,
//     returns 200, and lets us inspect after.
//   - Run writes → fetch chain fires → tap receives 20 dispatches
//   - Dump the tapped entities, decoded as CBOR maps
//
// Two runs (one per impl) → diff the bytes → know the bug.

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_MergeInboxTap(t *testing.T) {
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
	t.Logf("follower (wb-go): %s", localID)
	t.Logf("source   (%s):    %s", targetImpl, sourceID)

	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        localID,
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + follower.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
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
		Action: "set", Name: "tap-probe", Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	mergePath := fmt.Sprintf("system/inbox/follow/%s/%smerge", sourceID, prefix)
	fetchPath := fmt.Sprintf("system/inbox/follow/%s/%sfetch", sourceID, prefix)

	// Tap handler at the merge inbox path. Captures req.Params and
	// stores at /<localID>/system/runtime/merge-tap/<seq>/<request_id>.
	var seq atomic.Int64
	tap, err := follower.RegisterHandler(entitysdk.HandlerSpec{
		Pattern: mergePath,
		Name:    "merge-tap",
		Operations: map[string]coretypes.HandlerOperationSpec{
			"receive": {InputType: "primitive/any"},
		},
	}, func(_ context.Context, req *handler.Request) (*handler.Response, error) {
		n := seq.Add(1)
		// Bind a tap entry at a path we can browse later.
		tapPath := fmt.Sprintf("system/runtime/merge-tap/%05d/%s",
			n, req.Context.RequestID)
		// Store the entire incoming Params entity as-is.
		storedHash, perr := req.Context.Store.Put(req.Params)
		if perr == nil {
			_, _ = req.Context.TreeSet(tapPath, storedHash, "receive")
		}
		// Return a benign success response so the chain framework
		// doesn't keep retrying. We're intentionally breaking the
		// merge step for this diagnostic run.
		resultRaw, _ := cbor.Marshal(map[string]interface{}{"tapped": true, "seq": n})
		result, _ := entity.NewEntity("system/runtime/merge-tap/ack", resultRaw)
		return &handler.Response{Status: 200, Result: result}, nil
	})
	if err != nil {
		t.Fatalf("register merge tap: %v", err)
	}
	defer tap.Close()

	// Install ONLY the fetch step + subscription. No merge continuation
	// — the tap intercepts at the merge inbox path instead.
	localCap := follower.OwnerCapability().ContentHash
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
	}
	entitysdk.SetDefaultDispatchCap(crossPeerCap, &fetchData)
	fetchCont, _ := fetchData.ToEntity()
	_ = localCap // not used in fetch
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

	// Let the chain settle.
	time.Sleep(5 * time.Second)

	captured := seq.Load()
	t.Logf("\n=== merge-tap captures vs %s ===", targetImpl)
	t.Logf("total dispatches captured: %d (expected 20)", captured)

	tapEntries := follower.Store().List("")
	var taps []string
	for _, e := range tapEntries {
		if !contains(e.Path, "merge-tap/") || contains(e.Path, "/ack") {
			continue
		}
		taps = append(taps, e.Path)
	}
	t.Logf("tap-bound paths: %d", len(taps))

	// Dump the first 2 captured dispatches: full hex of req.Params.Data
	// and decoded CBOR structure.
	for i, tapPath := range taps {
		if i >= 2 {
			t.Logf("  ... (%d total tap entries; first 2 dumped)", len(taps))
			break
		}
		t.Logf("--- tap[%d] %s ---", i, tapPath)
		h, ok := follower.RawLocationIndex().Get(tapPath)
		if !ok {
			t.Logf("  no hash bound")
			continue
		}
		ent, ok := follower.Store().GetByHash(h)
		if !ok {
			t.Logf("  hash %s not in content store", h)
			continue
		}
		t.Logf("  entity.type=%q", ent.Type)
		t.Logf("  entity.data_len=%d", len(ent.Data))
		t.Logf("  entity.data_hex=%x", ent.Data)
		var decoded interface{}
		if err := cbor.Unmarshal(ent.Data, &decoded); err == nil {
			t.Logf("  entity.data_decoded=%+v", decoded)
		}
		// If it's an InboxDelivery, decode + dump the inner result.
		if ent.Type == coretypes.TypeInboxDelivery {
			delivery, derr := coretypes.InboxDeliveryDataFromEntity(ent)
			if derr == nil {
				t.Logf("  delivery.OriginalRequestID=%s", delivery.OriginalRequestID)
				t.Logf("  delivery.Status=%d", delivery.Status)
				t.Logf("  delivery.Result_len=%d", len(delivery.Result))
				t.Logf("  delivery.Result_hex=%x", delivery.Result)
				var innerDecoded interface{}
				if err := cbor.Unmarshal(delivery.Result, &innerDecoded); err == nil {
					t.Logf("  delivery.Result_decoded=%+v", innerDecoded)
				}
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
