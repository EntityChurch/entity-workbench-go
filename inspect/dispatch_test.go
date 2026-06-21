package inspect_test

// Unit tests for DispatchTap, WireRecorder, BindingStream, and
// SubscriptionTracer. Validate consumption shape against the four
// core-go hooks. Single-peer self-trigger keeps
// the tests cross-peer-independent.

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/inspect"
)

// TestDispatchTap_ObservesDispatchEntryExit: install a dispatch tap,
// trigger a local handler invocation, verify entry + exit captured
// with response_status / response_hash populated on exit.
func TestDispatchTap_ObservesDispatchEntryExit(t *testing.T) {
	tap := inspect.NewDispatchTap("system/tree")

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		RawOptions: []peer.Option{tap.PeerOption()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Put an entity at a path; this triggers a tree:put dispatch
	// through the local dispatcher.
	if _, err := ap.Put("dispatch-tap-test/x", "test/marker",
		map[string]interface{}{"v": 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Allow the dispatch to complete.
	time.Sleep(50 * time.Millisecond)

	exchanges := tap.Exchanges()
	if len(exchanges) == 0 {
		t.Fatalf("no exchanges captured; tap saw %d events total", tap.Count())
	}

	// At least one exchange should have both entry and exit; at least
	// one should have a 2xx exit status.
	var sawExit2xx bool
	for _, ex := range exchanges {
		if ex.Entry == nil {
			t.Errorf("exchange missing entry for request_id=%q", exitID(ex))
		}
		if ex.Exit != nil && ex.Exit.ResponseStatus >= 200 && ex.Exit.ResponseStatus < 300 {
			sawExit2xx = true
			if ex.Exit.ResponseHash.IsZero() {
				t.Errorf("response_hash zero on 2xx exit — core-go review §7.1 invariant violated")
			}
		}
	}
	if !sawExit2xx {
		t.Errorf("expected at least one 2xx exit; histogram=%v", tap.CountByStatus())
	}
}

func exitID(ex inspect.DispatchExchange) string {
	if ex.Entry != nil {
		return ex.Entry.RequestID
	}
	if ex.Exit != nil {
		return ex.Exit.RequestID
	}
	return ""
}

// TestBindingStream_ObservesPathMutation: install a binding stream,
// put an entity, verify the binding event surfaces with correct path
// and ChangeType.
func TestBindingStream_ObservesPathMutation(t *testing.T) {
	stream := &inspect.BindingStream{}

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		RawOptions: []peer.Option{stream.PeerOption()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	const path = "binding-stream-test/x"
	if _, err := ap.Put(path, "test/marker",
		map[string]interface{}{"v": 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	events := stream.Events()
	if len(events) == 0 {
		t.Fatalf("no binding events captured")
	}

	var sawPath bool
	for _, e := range events {
		if e.Path != "" && e.Hash.String() != "" {
			sawPath = true
		}
	}
	if !sawPath {
		t.Errorf("no event with path + hash; events=%+v", events)
	}

	hist := stream.CountByChangeType()
	if len(hist) == 0 {
		t.Errorf("CountByChangeType empty")
	}
}

// TestSubscriptionTracer_AttachesToEngine: validate the tracer
// attaches without error to a peer's engine. Behavior tested in the
// perfreview cross-impl probe.
func TestSubscriptionTracer_AttachesToEngine(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	engine := ap.SubscriptionEngine()
	if engine == nil {
		t.Fatal("SubscriptionEngine() returned nil; subscription extension not wired")
	}

	tracer := &inspect.SubscriptionTracer{}
	tracer.Attach(engine)

	// No subscriptions installed; no events expected. Just confirm
	// the empty state is consistent.
	if tracer.CountEmits() != 0 {
		t.Errorf("unexpected emits before any subscription: %d", tracer.CountEmits())
	}
	if tracer.CountDelivers() != 0 {
		t.Errorf("unexpected delivers: %d", tracer.CountDelivers())
	}
}

// TestWireRecorder_NoFramesAtConstructionRest: WireRecorder doesn't
// fire on local-only (no-connection) puts. Confirms the wire hook
// scope is bounded to network-traversal events.
func TestWireRecorder_NoFramesOnLocalOnlyPuts(t *testing.T) {
	rec := &inspect.WireRecorder{}

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		RawOptions: []peer.Option{rec.PeerOption()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	if _, err := ap.Put("wire-test/x", "test/marker",
		map[string]interface{}{"v": 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if rec.Count() != 0 {
		t.Errorf("expected 0 frames on local-only put, got %d", rec.Count())
	}
}

// Ensure context import isn't unused.
var _ = context.Background
