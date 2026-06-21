package entitysdk

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// TestSubscriptionExtensionDeliversThroughDispatch confirms that
// enabling the subscription extension via PeerConfig produces a peer
// where dispatched puts fire the full reactive pipeline: emit →
// subscription engine → dispatched delivery EXECUTE → stock inbox
// handler tree-write.
//
// Step 2 acceptance test for SDK-ALIGNMENT §7.5 — proves the plumbing
// end-to-end. The ergonomic subscription bridge (step 3,
// AppPeer.Subscribe) will replace the direct-engine subscribe call
// below with a dispatched Execute that carries Included.
func TestSubscriptionExtensionDeliversThroughDispatch(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Extensions: ExtensionsConfig{
			Subscription: &SubscriptionConfig{},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	engine := ap.subscriptionEngine()
	if engine == nil {
		t.Fatal("subscription engine was not wired")
	}

	identity := ap.peer.Identity()
	cs := ap.peer.Store()
	kp := ap.peer.Keypair()

	// Mint a capability token authorizing receive on our inbox path,
	// plus its signature.
	now := uint64(time.Now().UnixMilli())
	expires := now + 3600000
	inboxURI := "system/inbox/sdk-step2-test"
	capData := types.CapabilityTokenData{
		Grants: []types.GrantEntry{
			{
				Handlers:   types.CapabilityScope{Include: []string{"system/inbox/*"}},
				Resources:  types.CapabilityScope{Include: []string{inboxURI}},
				Operations: types.CapabilityScope{Include: []string{"receive"}},
			},
		},
		Granter:   types.SingleSigGranter(identity.ContentHash),
		Grantee:   identity.ContentHash,
		CreatedAt: now,
		ExpiresAt: &expires,
	}
	capEntity, err := capData.ToEntity()
	if err != nil {
		t.Fatalf("cap ToEntity: %v", err)
	}
	tokenHash, err := cs.Put(capEntity)
	if err != nil {
		t.Fatalf("cs.Put(cap): %v", err)
	}

	sig := kp.Sign(capEntity.ContentHash.Bytes())
	sigEntity, err := types.SignatureData{
		Target:    capEntity.ContentHash,
		Signer:    identity.ContentHash,
		Algorithm: "ed25519",
		Signature: sig,
	}.ToEntity()
	if err != nil {
		t.Fatalf("sig ToEntity: %v", err)
	}
	if _, err := cs.Put(sigEntity); err != nil {
		t.Fatalf("cs.Put(sig): %v", err)
	}

	// Subscribe directly through the engine — Executor doesn't carry
	// Included yet; the bridge (step 3) closes that gap.
	subReq := types.SubscriptionRequestData{
		DeliverTo:    types.DeliverySpec{URI: inboxURI, Operation: "receive"},
		DeliverToken: tokenHash,
	}
	subReqEntity, err := subReq.ToEntity()
	if err != nil {
		t.Fatalf("subReq ToEntity: %v", err)
	}

	resp, err := engine.HandleSubscribe(context.Background(), &handler.Request{
		Path:      "system/subscription",
		Operation: "subscribe",
		Params:    subReqEntity,
		Context: &handler.HandlerContext{
			AuthorHash:    identity.ContentHash,
			LocalPeerID:   kp.PeerID(),
			Store:         cs,
			LocationIndex: ap.peer.LocationIndex(),
			Resource:      &types.ResourceTarget{Targets: []string{"workspace/data/*"}},
			Included: map[hash.Hash]entity.Entity{
				capEntity.ContentHash: capEntity,
				sigEntity.ContentHash: sigEntity,
				identity.ContentHash:  identity,
			},
		},
	})
	if err != nil || resp.Status != 200 {
		t.Fatalf("subscribe: status=%d err=%v", resp.Status, err)
	}

	// Dispatched L1 put on a matching path. Traverses:
	//   ap.Put → system/tree handler → store.Put → emit → subscription
	//   engine's tree-event sink → pattern match → engine.Deliver →
	//   dispatcher EXECUTE to inboxURI → inbox handler writes
	//   notification at inboxURI/{uuid}.
	if _, err := ap.Put("workspace/data/article-1", "test/doc",
		map[string]interface{}{"title": "hello"}); err != nil {
		t.Fatalf("L1 Put: %v", err)
	}

	// Poll for delivery — async, up to 2 seconds.
	li := ap.peer.LocationIndex()
	deadline := time.Now().Add(2 * time.Second)
	var entries []string
	for time.Now().Before(deadline) {
		found := li.List(inboxURI + "/")
		if len(found) > 0 {
			for _, e := range found {
				entries = append(entries, e.Path)
			}
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if len(entries) == 0 {
		t.Fatal("no notification delivered to inbox path — dispatched pipeline broken")
	}
	if len(entries) != 1 {
		t.Errorf("got %d deliveries, want 1: %v", len(entries), entries)
	}
	if !strings.Contains(entries[0], inboxURI) {
		t.Errorf("delivery path %q does not contain inboxURI %q", entries[0], inboxURI)
	}
}
