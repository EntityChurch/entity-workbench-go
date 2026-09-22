package shellboot_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/workbench"
)

// WHO MAY DISPATCH AT OUR DELIVERY ENTRY POINT? — `BY-12` on the
// architecture seat's ledger, handed to us to measure.
//
// # Why this exists
//
// `TestProbe_SyncLegHasNoRollbackFloor` measured that the delivery
// handler has no ordering check: an authentic (path, entity, hash)
// triple replayed after a newer one overwrites it, status 200, no error.
// It was careful to say what it did NOT measure — *that an unauthorized
// remote party can trigger it* — because the probe never crossed the
// wire.
//
// The specification seat then ruled `A-33` and wrote that gap into the
// ruling as an explicit assumption: **"the interim defence of this leg
// is the authorization on its dispatch entry point — which is an
// assumption"**, ours to measure, and it *"changes the severity of the
// whole item in either direction"*. So this is the other half, and it is
// the half somebody else's text now leans on.
//
// # What is under test, and what is deliberately NOT
//
// One question: **does the capability layer refuse a dispatch at
// `workbench/blob-resolve:receive` from a peer the receiver has not
// authorized?** Two dialers, differing in exactly one thing — whether a
// `system/capability/policy/{peer}` row names them.
//
// There is no folder, no share and no sync here **on purpose**. The
// replay's *effect* is already measured; adding a real sync would mean
// the two arms differed in half a dozen ways and the grant would stop
// being the isolated variable. What is left is the entry point itself.
//
// ⚠ **`OpenAccess` is off on the receiver, and that is the whole
// experiment.** Every cross-peer test in this repo that uses
// `newSyncTestPeer` runs with `OpenAccess: true`, which authorizes
// everything and therefore measures nothing about permission (AP63). A
// version of this probe built on that harness would report "a stranger
// can deliver" and be measuring the fixture.
func TestProbe_WhoMayDispatchADelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// NO OpenAccess. The receiver authorizes by policy row or not at all.
	receiverAP, receiverWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "by12-receiver", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap receiver: %v", err)
	}
	defer receiverAP.Close()
	if receiverWS.BlobResolve == nil {
		t.Fatal("the receiver has no blob-resolve handler, so there is no entry point to probe")
	}

	stranger, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "by12-stranger", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap stranger: %v", err)
	}
	defer stranger.Close()

	authorized, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "by12-authorized", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap authorized: %v", err)
	}
	defer authorized.Close()

	// The one difference between the two dialers. Exactly the grant
	// `shellcmd`'s accept path writes for a real sender.
	if _, err := receiverAP.Store().Put(
		"system/capability/policy/"+authorized.PeerID(), types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: authorized.PeerID(),
			Grants: []types.GrantEntry{{
				Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
				Operations: types.CapabilityScope{Include: []string{"receive"}},
				Resources:  types.CapabilityScope{Include: []string{"*"}},
			}},
			Notes: "BY-12 probe: this peer may deliver",
		}); err != nil {
		t.Fatalf("write delivery policy row: %v", err)
	}

	// AP63: grants are assembled at HANDSHAKE, so both rows have to exist
	// before either dial.
	bringUpListener(t, ctx, receiverAP, "by12-receiver")
	bringUpListener(t, ctx, stranger, "by12-stranger")
	bringUpListener(t, ctx, authorized, "by12-authorized")

	for _, d := range []struct {
		name string
		ap   *entitysdk.AppPeer
	}{{"stranger", stranger}, {"authorized", authorized}} {
		conn, err := d.ap.Connect(ctx, receiverAP.Addr().String())
		if err != nil {
			t.Fatalf("%s could not dial the receiver: %v", d.name, err)
		}
		defer conn.Close()
	}

	strangerStatus, strangerCode, strangerErr := dispatchDelivery(t, stranger, receiverAP.PeerID())
	authStatus, authCode, authErr := dispatchDelivery(t, authorized, receiverAP.PeerID())

	t.Logf("BY-12 — dispatch at entity://{receiver}/%s:receive\n"+
		"  stranger   (no policy row): status=%d code=%q err=%v\n"+
		"  authorized (policy row)   : status=%d code=%q err=%v",
		workbench.BlobResolvePattern, strangerStatus, strangerCode, strangerErr,
		authStatus, authCode, authErr)

	strangerRefused := strangerStatus == 403 || strangerCode == "capability_denied" ||
		(strangerErr != nil && strings.Contains(strangerErr.Error(), "capability"))

	if !strangerRefused {
		// This is the finding that would RAISE the severity of the
		// rollback item, and it must be loud. Do not soften it into a
		// log line.
		t.Errorf("A PEER WITH NO POLICY ROW REACHED THE DELIVERY ENTRY POINT "+
			"(status=%d code=%q err=%v).\n"+
			"  The A-33 ruling's interim defence of this leg is the authorization on this "+
			"entry point, stated there as an assumption. If this arm fails, the assumption is "+
			"false: the missing rollback floor is remotely triggerable by an unauthorized party, "+
			"not only by an authorized sender or by out-of-order delivery.\n"+
			"  Route it before doing anything else.", strangerStatus, strangerCode, strangerErr)
	}

	// ANTI-VACUITY, and it is the arm that does the work. Without it this
	// probe is satisfied by a receiver that refuses EVERYTHING — a broken
	// handler, a listener that never came up, a URI typo — and would
	// report the reassuring answer for the worst possible reason.
	authRefusedOnCapability := authStatus == 403 || authCode == "capability_denied"
	if authRefusedOnCapability {
		t.Fatalf("anti-vacuity: the AUTHORIZED peer was also refused on capability "+
			"(status=%d code=%q err=%v). The two arms differ only in the policy row, so this "+
			"probe is measuring something other than authorization and its negative arm is "+
			"worth nothing.", authStatus, authCode, authErr)
	}

	if strangerRefused && !authRefusedOnCapability {
		t.Logf("MEASURED: the delivery entry point is capability-gated. A peer the receiver has " +
			"not named cannot dispatch `receive` at all, while a peer it has named gets past the " +
			"capability layer and is answered by the handler.\n" +
			"  ⇒ the sync leg's missing rollback floor is reachable by an AUTHORIZED sender and " +
			"by accidental out-of-order delivery, and NOT by a stranger. That is the reading " +
			"A-33's interim defence assumed, now performed.\n" +
			"  What this still does NOT establish: anything about a sender whose authorization " +
			"was legitimate and has since been withdrawn, and anything about the subscription " +
			"engine's own delivery path, which runs under the subscription's dispatch capability " +
			"rather than under the peer's policy row.")
	}
}

// dispatchDelivery sends one delivery-shaped dispatch at another peer's
// blob-resolve handler and reports how far it got.
//
// The notification is deliberately well-formed but points at nothing: a
// subscription id this receiver never issued, and an entity that is
// genuinely its own pre-image so the failure cannot be a hash check. If
// the capability layer lets it through, the handler answers on the
// merits and the status is not 403 — which is exactly the discrimination
// this probe needs, and it reaches it without arranging a real folder.
func dispatchDelivery(t *testing.T, from *entitysdk.AppPeer, targetPeerID string) (uint, string, error) {
	t.Helper()

	payload, err := ecf.Encode(map[string]string{"probe": "by12"})
	if err != nil {
		t.Fatalf("encode probe payload: %v", err)
	}
	body, err := entity.NewEntity("test/scalar", payload)
	if err != nil {
		t.Fatalf("build probe entity: %v", err)
	}
	h := body.ContentHash

	notif, err := types.SubscriptionNotificationData{
		SubscriptionID: "by12-not-a-real-subscription",
		Event:          "updated",
		URI:            "/" + from.PeerID() + "/local/files/by12/probe.txt",
		Hash:           h,
	}.ToEntity()
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}

	uri := "entity://" + targetPeerID + "/" + workbench.BlobResolvePattern
	resp, err := from.Executor().ExecuteWithIncluded(uri, "receive", notif, nil,
		map[hash.Hash]entity.Entity{h: body})
	if err != nil {
		return 0, entitysdk.CodeOf(err), err
	}
	if resp == nil {
		return 0, "", nil
	}
	code := ""
	if e := entitysdk.ErrorFromResponse(resp); e != nil {
		code = e.Code
	}
	return resp.Status, code, nil
}
