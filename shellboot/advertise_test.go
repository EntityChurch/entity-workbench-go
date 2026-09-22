package shellboot_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/shellboot"
)

// TestCreate_AdvertisesTheListenerItBound is the real-session pin for
// §6.5.1a D1 self-publication: a peer created with a listener now tells
// the tree how to reach it, which is the precondition for every
// connectivity piece above it (a browser cannot maintain, meet, or
// report liveness against a peer that advertises no dial address).
//
// Tier: real-session (TESTING-STRATEGY) — it boots a peer through the
// same PeerManager.Create path a frontend uses, binds a real listener
// on an ephemeral loopback port, and reads the profile back out of the
// store. Not a unit test of the derivation helper.
func TestCreate_AdvertisesTheListenerItBound(t *testing.T) {
	m := shellboot.NewPeerManager("test-advertise")
	h, err := m.Create(shellboot.Config{
		// Port 0 is a real bind: the kernel picks the port. The
		// advertised URL therefore carries the pre-bind form, which is
		// the known limitation named in AdvertisedURL's doc — it is
		// asserted here rather than hidden, so a fix has a pin to move.
		ListenAddr:   "ws://127.0.0.1:19100/ws",
		AdvertiseURL: "",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	hp := m.Get(h)
	if hp == nil {
		t.Fatal("Get: hosted peer missing after Create")
	}
	defer m.Destroy(h)

	if hp.AdvertiseErr != nil {
		t.Fatalf("AdvertiseErr = %v, want nil for a concrete bind host", hp.AdvertiseErr)
	}
	if hp.AdvertisedURL != "ws://127.0.0.1:19100/ws" {
		t.Errorf("AdvertisedURL = %q, want the derived dial URL", hp.AdvertisedURL)
	}

	ap := hp.AppPeer
	idHash, err := types.ComputePeerIdentityHashFromPeerID(crypto.PeerID(ap.PeerID()))
	if err != nil {
		t.Fatalf("identity hash: %v", err)
	}
	path := "system/peer/transport/" + types.PeerIdentityHashHex(idHash) + "/primary-ws"
	ph, ok := ap.RawLocationIndex().Get(path)
	if !ok {
		t.Fatalf("no websocket profile bound at %s — the peer listens and says nothing", path)
	}
	ent, ok := ap.RawContentStore().Get(ph)
	if !ok {
		t.Fatalf("profile %s bound but not stored", ph)
	}
	data, err := types.WebSocketProfileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode websocket profile: %v", err)
	}
	if data.PeerID != ap.PeerID() {
		t.Errorf("profile.peer_id = %q, want %q", data.PeerID, ap.PeerID())
	}
	if data.Endpoint.URL != "ws://127.0.0.1:19100/ws" {
		t.Errorf("profile.endpoint.url = %q", data.Endpoint.URL)
	}
	if data.Freshness != "live" {
		t.Errorf("profile.freshness = %q, want live", data.Freshness)
	}
}

// TestCreate_WildcardBindAdvertisesAConcreteAddress.
//
// **This test asserted the opposite until 2026-09-03**, and the change is
// deliberate rather than a relaxation — so it is worth being exact about
// what was right in the old rule and what was wrong.
//
// Right: *a durable claim that peers can dial 0.0.0.0 is worse than
// silence.* Publishing the wildcard verbatim would be a profile nobody
// can use, and that is still forbidden — it is the assertion below.
//
// Wrong: concluding from that that a wildcard bind must publish NOTHING.
// A wildcard is the only sensible bind for a machine on a LAN, so the old
// rule meant the normal configuration published no transport profile at
// all — and a peer with no profile cannot be reconnected to by peer-id.
// `EnsureConnected` resolves a profile, finds none, and the relationship
// can only be revived by somebody re-typing an address. That manual step
// is the one the sharing redesign exists to delete, and this refusal was
// quietly guaranteeing it.
//
// So a wildcard now resolves to this host's LAN address
// (`shellboot.lanDialHost`). It is a GUESS — a multi-homed machine may be
// reachable on a different interface — and it is labelled as one wherever
// it is rendered; an operator who knows better passes AdvertiseURL, which
// still wins. A guess that is usually right and always visible beats a
// refusal that is always useless.
func TestCreate_WildcardBindAdvertisesAConcreteAddress(t *testing.T) {
	m := shellboot.NewPeerManager("test-advertise-wildcard")
	h, err := m.Create(shellboot.Config{ListenAddr: "0.0.0.0:19101"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	hp := m.Get(h)
	if hp == nil {
		t.Fatal("Get: hosted peer missing after Create")
	}
	defer m.Destroy(h)

	// The listener is up — advertising is a separate concern and its
	// failure must not take the peer down with it.
	if hp.ListenScheme != "tcp" {
		t.Errorf("ListenScheme = %q, want tcp — the listener should still have bound", hp.ListenScheme)
	}
	if hp.AdvertiseErr != nil {
		t.Fatalf("AdvertiseErr = %v; a wildcard bind on a host with any address should resolve", hp.AdvertiseErr)
	}
	if hp.AdvertisedURL == "" {
		t.Fatal("AdvertisedURL is empty — a peer with no transport profile cannot be reconnected to by peer-id")
	}
	// The part of the old rule that was right, and is now the assertion.
	for _, forbidden := range []string{"0.0.0.0", "[::]", "*"} {
		if strings.Contains(hp.AdvertisedURL, forbidden) {
			t.Errorf("AdvertisedURL = %q — it carries the wildcard %q, which nobody can dial",
				hp.AdvertisedURL, forbidden)
		}
	}
	if !strings.HasSuffix(hp.AdvertisedURL, ":19101") {
		t.Errorf("AdvertisedURL = %q, want the bound port 19101", hp.AdvertisedURL)
	}
}
