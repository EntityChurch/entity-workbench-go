package entitysdk_test

import (
	"path/filepath"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestSubscription_SurvivesRestart measures whether a subscription is
// live after the process that created it has gone away.
//
// # Why this test exists
//
// `subscription.Engine`'s own doc comment states the contract:
//
//	Must be called after SetLocationIndex and before StartDelivery.
//	...
//	Rationale: subscriptions are classified as PERSISTENT extension
//	state, so the durable copy in the tree is authoritative and the
//	runtime index is a derived cache that must be rebuilt on boot.
//
// That is `Engine.Load()`. `entity-core-go`'s own daemon calls it
// (`cmd/entity-peer/main.go`). `entitysdk.assembleAppPeer` does
// `SetLocationIndex` → `Deliver` → `StartDelivery` and never called
// `Load` in between — the exact gap the comment describes, in the exact
// place it names.
//
// This is AP39's shape one extension over: **a persistent store does not
// make a derived runtime index persistent.** There the volatile index was
// the query index and the symptom was `find`/`grep` going blind to
// everything written before the process started. Here the volatile index
// is the subscription engine's, and the symptom is worse, because a
// subscription is not a read path — it is the thing that makes writes
// *cause* something. Every mount in this repo is driven by one.
//
// The assertion is deliberately on the ENGINE's registered set rather
// than on a delivery, because the engine is where the claim lives: the
// tree keeps the subscription entity either way, so a test that only
// re-read the tree would pass against the broken build.
func TestSubscription_SurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer.db")

	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	const prefix = "app/probe/"
	var qualifiedPattern string
	var subID string

	// --- Pass 1: create the subscription on a persistent peer ---------
	func() {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Keypair: &kp,
			Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("pass1 CreatePeer: %v", err)
		}
		defer ap.Close()

		id := ap.PeerID()
		deliverURI := "entity://" + id + "/workspace/app"

		sub, err := ap.SubscribeRawAt(id, prefix+"*", deliverURI, "receive",
			entitysdk.SubscribeOpts{Events: []string{"created", "updated"}})
		if err != nil {
			t.Fatalf("pass1 subscribe: %v", err)
		}
		subID = sub.ID()

		// Read the qualified pattern back off the stored entity rather
		// than reconstructing it. The engine's index is keyed on
		// `sub.Pattern` verbatim, so a hand-built key tests the
		// spelling of this test rather than the state of the engine —
		// which is exactly what the first version of it did.
		ent, ok := ap.Store().Get("system/subscription/" + subID)
		if !ok {
			t.Fatalf("pass1: subscription entity %s not in tree", subID)
		}
		sd, derr := types.SubscriptionDataFromEntity(ent)
		if derr != nil {
			t.Fatalf("pass1: decode subscription entity: %v", derr)
		}
		qualifiedPattern = sd.Pattern

		// The premise. If this is 0 the test is measuring nothing and
		// the pattern spelling is wrong, not the engine.
		if n := ap.SubscriptionEngine().SubscriberCountForPrefix(qualifiedPattern); n == 0 {
			t.Fatalf("pass1: engine reports 0 subscribers for %q immediately after "+
				"subscribing — the test's own premise is broken, not the engine",
				qualifiedPattern)
		}
	}()

	// --- Pass 2: reopen the same database -----------------------------
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Keypair: &kp,
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
	})
	if err != nil {
		t.Fatalf("pass2 CreatePeer: %v", err)
	}
	defer ap.Close()

	// The durable copy is in the tree — this is the half that always
	// worked, and asserting it first is what makes the failure below
	// legible as "the index did not rebuild" rather than "the write was
	// lost".
	if _, ok := ap.Store().Get("system/subscription/" + subID); !ok {
		t.Fatalf("pass2: subscription entity %s is not in the reopened tree — "+
			"the durable half did not survive, which is a different bug from "+
			"the one this test is about", subID)
	}

	if n := ap.SubscriptionEngine().SubscriberCountForPrefix(qualifiedPattern); n == 0 {
		t.Errorf("pass2: the subscription entity is in the tree and the engine "+
			"reports 0 subscribers for %q — the runtime index was not rebuilt "+
			"on open. Engine.Load() is the kernel's own answer and assembleAppPeer "+
			"does not call it.", qualifiedPattern)
	}
}
