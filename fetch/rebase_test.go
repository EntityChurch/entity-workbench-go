package fetch_test

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/fetch"
)

// rebase_test.go — one origin, several peers.
//
// The well-known `{origin}/transport-profile` is a COLD-START entry
// point (§6.5.3 Mode A2), not a claim that the origin serves one peer.
// We read it as the latter and it cost us the whole naming leg against
// the live federation: `entitychurchregistry.org` features its SITE peer
// in that object while the REGISTRY peer — the entire reason the domain
// exists — sits beside it under its own peer-rooted prefix. Every
// `-registry`/`-name` invocation refused with "advertises X; you pinned
// Y", and the refusal read like a security property rather than the
// blind spot it was.
//
// Measured live 2026-08-30. These are the offline pins; the live run is
// a separate target, because a suite in `test-native` must not go red
// for a domain's reasons.
//
// Tier: contract pin.

const (
	// The two peers co-hosted on entitychurchregistry.org, verbatim.
	featuredPeer = "2KEbBKupL9RZsV4Jx8zYvK6g4EnikvCPSZL2zPEafybVkR"
	registryPeer = "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb"
)

// liveProfile is the endpoint block entitychurchregistry.org actually
// serves at its origin root, transcribed from the wire.
func liveProfile() types.HTTPPollProfileData {
	return types.HTTPPollProfileData{
		PeerID: featuredPeer,
		Endpoint: types.TransportEndpoint{
			TreeURLPrefix:     "/" + featuredPeer,
			ContentURLPrefix:  "/content",
			ManifestURLPrefix: "/" + featuredPeer + "/system/peer/published-root",
			ContentLayout:     types.ContentLayoutSharded24,
			TreeLeafSuffix:    ".bin",
			TreeListingSuffix: ".list",
		},
	}
}

// TestRebaseReachesASecondPeerOnTheSameOrigin is the regression. The
// wanted URLs are the ones that actually answered 200 on the live
// origin — the by-name pointer that our refusal had made unreachable.
func TestRebaseReachesASecondPeerOnTheSameOrigin(t *testing.T) {
	const origin = "https://entitychurchregistry.org"

	l, err := fetch.LayoutFromProfile(origin, liveProfile())
	if err != nil {
		t.Fatalf("LayoutFromProfile: %v", err)
	}
	if l.PeerID != featuredPeer {
		t.Fatalf("the origin features %s, got %s", featuredPeer, l.PeerID)
	}

	r, err := l.RebaseTo(registryPeer)
	if err != nil {
		t.Fatalf("RebaseTo: %v", err)
	}

	if r.PeerID != registryPeer {
		t.Errorf("PeerID: got %s, want %s", r.PeerID, registryPeer)
	}
	// Provenance is a third state and must survive, or the CLI reports a
	// re-based layout as one the origin advertised.
	if r.RebasedFrom != featuredPeer {
		t.Errorf("RebasedFrom: got %q, want %q", r.RebasedFrom, featuredPeer)
	}
	if l.RebasedFrom != "" {
		t.Errorf("RebaseTo mutated its receiver: RebasedFrom=%q", l.RebasedFrom)
	}

	// The peer-rooted positions move.
	wantLeaf := origin + "/" + registryPeer +
		"/system/registry/binding/by-name/billslab.com.bin"
	if got := r.TreeLeafURL("system/registry/binding/by-name/billslab.com"); got != wantLeaf {
		t.Errorf("by-name pointer:\n  got  %s\n  want %s", got, wantLeaf)
	}
	wantManifest := origin + "/" + registryPeer + "/system/peer/published-root"
	if got := r.ManifestURL(); got != wantManifest {
		t.Errorf("manifest:\n  got  %s\n  want %s", got, wantManifest)
	}

	// The origin-level layout does NOT move: content is a shared store,
	// deduped across every peer the origin hosts. Rebasing it would send
	// us to a content prefix that does not exist.
	if r.Endpoint.ContentURLPrefix != l.Endpoint.ContentURLPrefix {
		t.Errorf("content prefix moved: %q → %q",
			l.Endpoint.ContentURLPrefix, r.Endpoint.ContentURLPrefix)
	}
	if r.Endpoint.ContentLayout != l.Endpoint.ContentLayout ||
		r.Endpoint.TreeLeafSuffix != l.Endpoint.TreeLeafSuffix ||
		r.Endpoint.TreeListingSuffix != l.Endpoint.TreeListingSuffix {
		t.Error("an origin-level layout field moved under rebase")
	}
}

// TestRebaseSubstitutesWholeSegmentsOnly guards the discriminator. A
// peer-id appearing INSIDE a segment — a bucket name, a CDN path — is
// not the same claim, exactly as treeBase()'s last-segment check holds.
func TestRebaseSubstitutesWholeSegmentsOnly(t *testing.T) {
	const origin = "https://cdn.example"

	for _, tc := range []struct {
		name, prefix, want string
	}{
		{
			name:   "peer-rooted — substituted",
			prefix: "/" + featuredPeer,
			want:   "/" + registryPeer,
		},
		{
			name:   "peer-id is a substring of a longer segment — left alone",
			prefix: "/mirror-" + featuredPeer + "-cache",
			want:   "/mirror-" + featuredPeer + "-cache",
		},
		{
			name:   "peer-id appears mid-path as a whole segment — substituted there too",
			prefix: "/" + featuredPeer + "/mirror",
			want:   "/" + registryPeer + "/mirror",
		},
		{
			name:   "origin-rooted, peer-agnostic (our own emission) — unchanged",
			prefix: origin,
			want:   origin,
		},
	} {
		p := liveProfile()
		p.Endpoint.TreeURLPrefix = tc.prefix
		p.Endpoint.ManifestURLPrefix = ""

		l, err := fetch.LayoutFromProfile(origin, p)
		if err != nil {
			t.Errorf("%s: LayoutFromProfile: %v", tc.name, err)
			continue
		}
		r, err := l.RebaseTo(registryPeer)
		if err != nil {
			t.Errorf("%s: RebaseTo: %v", tc.name, err)
			continue
		}
		if got := r.Endpoint.TreeURLPrefix; got != tc.want {
			t.Errorf("%s:\n  got  %s\n  want %s", tc.name, got, tc.want)
		}
	}
}

// TestRebaseToTheSamePeerIsIdentity — the common case is one peer per
// origin, and it must not acquire a re-based provenance label it did not
// earn. A layout that was genuinely discovered has to keep saying so.
func TestRebaseToTheSamePeerIsIdentity(t *testing.T) {
	l, err := fetch.LayoutFromProfile("https://entitycoreprotocol.org", liveProfile())
	if err != nil {
		t.Fatalf("LayoutFromProfile: %v", err)
	}
	r, err := l.RebaseTo(featuredPeer)
	if err != nil {
		t.Fatalf("RebaseTo: %v", err)
	}
	if r.RebasedFrom != "" {
		t.Errorf("a no-op rebase claimed provenance it does not have: %q", r.RebasedFrom)
	}
	if r.Endpoint.TreeURLPrefix != l.Endpoint.TreeURLPrefix {
		t.Error("a no-op rebase changed the tree prefix")
	}
}

// TestRebaseRefusesAnEmptyPeer — the pin is the one fact the operator
// supplies out of band, so an empty one is an error rather than a
// layout addressed at nothing.
func TestRebaseRefusesAnEmptyPeer(t *testing.T) {
	l, err := fetch.LayoutFromProfile("https://entitychurchregistry.org", liveProfile())
	if err != nil {
		t.Fatalf("LayoutFromProfile: %v", err)
	}
	if _, err := l.RebaseTo(""); err == nil {
		t.Error("RebaseTo(\"\") returned no error")
	}
}
