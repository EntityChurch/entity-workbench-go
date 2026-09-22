package workbench

import (
	"strings"
	"testing"
)

// public_site_grants_test.go — the derivation, in isolation.
//
// The wire measurement is `shellboot/public_site_scope_probe_test.go`,
// and it is the one that matters: these assertions are about a slice of
// structs and prove nothing about what a peer will actually serve. They
// exist for the cases the wire probe cannot reach cheaply — an empty
// prefix, a wildcard, a round trip through the recogniser — and because
// a refusal with no test is a refusal nobody has read.

func TestPublicSiteGrants_RefusesTheWholeTree(t *testing.T) {
	// The one that matters. `MintRoot` will happily sign a root over the
	// whole tree, so "" reaches here by an ordinary route — and the grant
	// derived from it would be `Resources: ["*"]` wearing a different
	// spelling, handed to every peer that can dial this one.
	for _, prefix := range []string{"", " ", "/", "//"} {
		if _, err := PublicSiteGrants(prefix); err == nil {
			t.Errorf("PublicSiteGrants(%q) was accepted; it authorizes the entire tree "+
				"— devices, folders, offers and every mounted file — to anybody", prefix)
		}
	}
}

func TestPublicSiteGrants_RefusesAWildcard(t *testing.T) {
	if _, err := PublicSiteGrants("sites/*"); err == nil {
		t.Error("PublicSiteGrants accepted a caller-supplied wildcard; the pattern is built " +
			"here precisely so the grant and the signed root cannot disagree")
	}
}

func TestPublicSiteGrants_CarriesTheTwoVerificationPaths(t *testing.T) {
	// Not decoration. A grant scoped to the site alone yields a peer that
	// serves every page and cannot be verified at all, which presents to
	// a reader as "this publisher has never published" — an accusation
	// against the publisher for something the reader's own grant caused.
	grants, err := PublicSiteGrants("sites/")
	if err != nil {
		t.Fatal(err)
	}
	var tree []string
	for _, g := range grants {
		if len(g.Handlers.Include) == 1 && g.Handlers.Include[0] == "system/tree" {
			tree = g.Resources.Include
		}
	}
	for _, want := range []string{"sites", "sites/*", "system/peer/published-root", "system/signature/*"} {
		found := false
		for _, r := range tree {
			if r == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the tree grant does not cover %q — it covers %v", want, tree)
		}
	}
}

func TestPublicSiteGrants_DoNotReachTheDeclarationsOrTheFiles(t *testing.T) {
	// A resource-pattern assertion, which is NOT the same claim as the
	// wire probe's: it says nothing about how the capability check reads
	// these patterns. It is here to fail loudly if somebody widens the
	// set by editing a literal, which is how AP90 happened.
	grants, err := PublicSiteGrants("sites/")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range grants {
		for _, r := range g.Resources.Include {
			if r == "*" {
				t.Fatalf("a public site grant carries Resources [\"*\"] on handler %v. "+
					"AP90 is this exact line, and that one named ONE peer; this one names "+
					"everybody that can dial this machine.", g.Handlers.Include)
			}
			for _, forbidden := range []string{DevicePrefix, FolderPrefix, ShareOfferPrefix, LocalFilesSourcePrefix} {
				if strings.HasPrefix(r, strings.TrimSuffix(forbidden, "/")) {
					t.Errorf("a public site grant covers %q, which is under %q — "+
						"that is this peer's own business, not part of a published site",
						r, forbidden)
				}
			}
		}
	}
}

func TestPublicSitePrefix_RoundTripsAndIgnoresTheSystemPaths(t *testing.T) {
	// The recogniser is what a surface uses to say "the public grant
	// covers X and the published root commits to Y". A version that
	// returned the first `/*`-suffixed pattern reported `system/signature`
	// as the published prefix whenever the Include order moved — a
	// confidently wrong answer on an authorization surface.
	grants, err := PublicSiteGrants("sites/notes/")
	if err != nil {
		t.Fatal(err)
	}
	if got := PublicSitePrefix(grants); got != "sites/notes" {
		t.Errorf("PublicSitePrefix = %q, want %q", got, "sites/notes")
	}
	if !IsPublicSiteGrant(grants) {
		t.Error("IsPublicSiteGrant did not recognise its own output")
	}
}

func TestIsPublicSiteGrant_DoesNotClaimAStrangersRow(t *testing.T) {
	// The predicate decides whether a surface may offer to REMOVE the
	// `default` row. Claiming somebody's hand-written entry would delete
	// authorization they wrote deliberately, and SaveAccessPolicy
	// replaces rather than merges, so the friendly action is the
	// destructive one.
	if IsPublicSiteGrant(SyncReceiverGrants()) {
		t.Error("IsPublicSiteGrant claimed the sync receiver grant")
	}
	if IsPublicSiteGrant(nil) {
		t.Error("IsPublicSiteGrant claimed an empty grant set")
	}
	if IsPublicSiteGrant(SyncSenderGrants([]SharedScope{{LocalRoot: "r", FolderID: "f"}})) {
		t.Error("IsPublicSiteGrant claimed the sync sender grant")
	}
}
