package entitysdk

import (
	"strings"
	"testing"
	"time"
)

// drainSub waits up to d for the next event on sub; fails on timeout.
func drainSub(t *testing.T, sub *Subscription, d time.Duration) ChangeEvent {
	t.Helper()
	select {
	case ev, ok := <-sub.Events():
		if !ok {
			t.Fatal("subscription channel closed")
		}
		return ev
	case <-time.After(d):
		t.Fatalf("timed out waiting for subscription event after %s", d)
		return ChangeEvent{}
	}
}

// TestSubscribeTreeConformance asserts the subscription bridge writes
// all three tree entries §11.5.1 requires (interface, handler entity,
// and — if InternalScope is set — grant) at the inbox path, and
// removes them on Close. The bridge registers with InternalScope=nil
// (the body only writes to a channel), so no grant is expected.
// Mirrors the conformance check from PROPOSAL-SDK-HANDLER-REGISTRATION-
// AND-NOTIFICATION-BOUNDARY §5.6.
func TestSubscribeTreeConformance(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	sub, err := ap.Subscribe("workspace/tree-check/*", SubscribeOpts{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	inbox := "system/inbox/sdk-sub-" + sub.id

	li := ap.peer.LocationIndex()
	if !li.Has(inbox) {
		t.Errorf("handler entity missing at %s after Subscribe", inbox)
	}
	if !li.Has("system/handler/" + inbox) {
		t.Errorf("interface entity missing at system/handler/%s after Subscribe", inbox)
	}
	if li.Has("system/capability/grants/" + inbox) {
		t.Errorf("grant entity should NOT be written (InternalScope is nil)")
	}

	if err := sub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if li.Has(inbox) {
		t.Errorf("handler entity still present at %s after Close", inbox)
	}
	if li.Has("system/handler/" + inbox) {
		t.Errorf("interface entity still present at system/handler/%s after Close", inbox)
	}
}

// TestSubscribeDisabledReturns500 confirms the bridge refuses to run
// when the extension is explicitly disabled — no silent no-op.
//
// Subscription is default-on in the current convention; opt-out is
// via &SubscriptionConfig{Disabled: true}. Pre-flip this test
// passed nil and got "off"; the flip makes that case "on", so the
// test now sets Disabled explicitly.
func TestSubscribeDisabledReturns500(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	_, err = ap.Subscribe("workspace/*", SubscribeOpts{})
	if !IsSystemError(err) {
		t.Fatalf("want 500 when subscription extension disabled, got %v", err)
	}
}

// TestSubscribeEndToEnd is the step-3 acceptance test: the full
// dispatched path driven entirely through AppPeer.Subscribe — no
// direct engine access, no manual token minting by the caller.
// Put → subscription matches → delivery EXECUTE → bridge inbox
// handler → Go channel.
func TestSubscribeEndToEnd(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	sub, err := ap.Subscribe("workspace/data/*", SubscribeOpts{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	if sub.ID() == "" {
		t.Error("Subscribe returned empty subscription ID")
	}
	if sub.Pattern() != "workspace/data/*" {
		t.Errorf("Pattern() = %q, want workspace/data/*", sub.Pattern())
	}

	// L1 put on a matching path.
	if _, err := ap.Put("workspace/data/alpha", "test/doc",
		map[string]interface{}{"title": "alpha"}); err != nil {
		t.Fatal(err)
	}

	ev := drainSub(t, sub, 2*time.Second)
	if ev.EventType != ChangePut {
		t.Errorf("EventType = %q, want put", ev.EventType)
	}
	if !strings.Contains(ev.Path, "workspace/data/alpha") {
		t.Errorf("Path = %q, want containing workspace/data/alpha", ev.Path)
	}
	if ev.NewHash.IsZero() {
		t.Error("NewHash is zero — notification should carry the new binding hash")
	}
}

// TestSubscribePatternIsolation confirms the bridge routes each
// subscription's events to its own channel, and only matching events
// arrive. If this failed, two subscriptions would cross-contaminate —
// which would mean the per-subscription inbox path or longest-prefix
// handler match is broken.
func TestSubscribePatternIsolation(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	subA, err := ap.Subscribe("left/*", SubscribeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer subA.Close()

	subB, err := ap.Subscribe("right/*", SubscribeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer subB.Close()

	if _, err := ap.Put("left/one", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Put("right/one", "test/v", 2); err != nil {
		t.Fatal(err)
	}

	gotA := drainSub(t, subA, 2*time.Second)
	if !strings.Contains(gotA.Path, "left/one") {
		t.Errorf("subA got path %q, want left/one", gotA.Path)
	}
	gotB := drainSub(t, subB, 2*time.Second)
	if !strings.Contains(gotB.Path, "right/one") {
		t.Errorf("subB got path %q, want right/one", gotB.Path)
	}

	// No cross-contamination — each channel should be empty.
	select {
	case ev := <-subA.Events():
		t.Errorf("subA received unexpected event: %+v", ev)
	default:
	}
	select {
	case ev := <-subB.Events():
		t.Errorf("subB received unexpected event: %+v", ev)
	default:
	}
}

// TestSubscribeCloseStopsDeliveries confirms Close unsubscribes at the
// engine layer so subsequent puts don't land in the closed channel
// (which would panic) or a replaced handler path.
func TestSubscribeCloseStopsDeliveries(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	sub, err := ap.Subscribe("target/*", SubscribeOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Receive one event first to confirm it's live.
	if _, err := ap.Put("target/one", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	drainSub(t, sub, 2*time.Second)

	if err := sub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Channel should be drained and closed.
	_, ok := <-sub.Events()
	if ok {
		t.Error("channel should be closed after Close")
	}

	// Further puts don't panic (they'd write to a closed channel if
	// the handler weren't swapped out or unsubscribe didn't land).
	if _, err := ap.Put("target/two", "test/v", 2); err != nil {
		t.Fatal(err)
	}
	// Let any late deliveries try to fire.
	time.Sleep(100 * time.Millisecond)
}
