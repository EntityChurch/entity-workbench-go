package publish_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// bz11_verification_grant_test.go — `BZ-11`: the verification grant, and
// the half of `A-34` that is a `MUST NOT` rather than a `MUST`.
//
// # What arch ruled, and which half was measured
//
// `APP-CONVENTION-SEMANTIC-CONTENT-SITE` v0.5.2 §7, ruled 2026-09-13 on
// our diagnosis:
//
//   - **[MUST]** a grant intended to make a site verifiable covers the
//     published-root and signature locations as well as the content
//     prefix — *"the scope of a site's content and the scope of
//     verifying it are different sets"*. `workbench.PublicSiteGrants`
//     carries both and `publish/live_and_static_test.go` has the arm.
//   - **[MUST NOT]** report an authorization failure at those locations
//     **as an absent published root**. ⛔ This half had no gate anywhere
//     in this tree, and our own tracker said so in the honest form:
//     *"the grant half may already hold — that is a reading and not a
//     measurement."*
//
// # Why the MUST NOT is the one worth a file
//
// Because the trap is baited with good practice. An operator scoping a
// grant tightly — the thing we tell them to do everywhere else — writes
// a grant over the site prefix and nothing else, which is the most
// defensible-looking grant available. The reader then reports *"this
// publisher has never published"*: a false statement about the
// publisher, manufactured by the reader's own scoping, and it sends
// somebody to the wrong machine. That is the same shape as `A-36`
// (`a36_peer_root_probe_test.go`), one convention over — there the
// publisher's prefix manufactures a false statement about an author.
//
// The three arms are one scenario at three grant scopes, because the
// grant is the variable and a separate harness per arm would let the
// peers differ too.

// grantScope names the three grants under test.
type grantScope int

const (
	// scopeFull is what PublicSiteGrants derives: prefix + both
	// verification locations. The anti-vacuity arm.
	scopeFull grantScope = iota
	// scopeSiteOnly is the A-34 trap: the content prefix and nothing else.
	scopeSiteOnly
	// scopeNoSignature is the subtler half — the root is readable and its
	// signature is not, so verification fails one hop later than the
	// obvious case. A reader that only guards the root location reports
	// this one wrongly while passing scopeSiteOnly.
	scopeNoSignature
)

func (g grantScope) String() string {
	switch g {
	case scopeFull:
		return "prefix + published-root + signature"
	case scopeSiteOnly:
		return "prefix only (the A-34 trap)"
	default:
		return "prefix + published-root, no signature"
	}
}

func grantsFor(scope grantScope, prefix string) []types.GrantEntry {
	res := []string{strings.TrimSuffix(prefix, "/") + "/*"}
	switch scope {
	case scopeFull:
		res = append(res, "system/peer/published-root", "system/signature/*")
	case scopeNoSignature:
		res = append(res, "system/peer/published-root")
	}
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: res},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}
}

// verifyPairUnderGrant stands up a publisher that HAS published and a
// reader whose grant is the parameter, and returns the reader's attempt
// at a verified root.
//
// `published` is a parameter rather than always true because the third
// arm needs the genuinely-absent case: `ErrNoPublishedRoot` must still
// be reachable, or a reader that never returns it satisfies the MUST NOT
// by having deleted the only correct answer.
func verifyPairUnderGrant(t *testing.T, scope grantScope, published bool) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	const prefix = "sites/"

	publisher, err := entitysdk.CreatePeer(entitysdk.PeerConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("publisher CreatePeer: %v", err)
	}
	t.Cleanup(func() { publisher.Close() })

	reader, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("reader CreatePeer: %v", err)
	}
	t.Cleanup(func() { reader.Close() })

	if _, err := publisher.Store().Put(prefix+"demo/page", "app/site/page",
		map[string]any{"slug": "page", "body": "a published page"}); err != nil {
		t.Fatalf("seed page: %v", err)
	}
	if published {
		if _, err := publish.MintRoot(ctx, publish.MintOpts{Peer: publisher, Prefix: prefix}); err != nil {
			t.Fatalf("MintRoot: %v", err)
		}
	}

	policyPath := "system/capability/policy/" + reader.PeerID()
	if _, err := publisher.Store().Put(policyPath, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: reader.PeerID(),
			Grants:      grantsFor(scope, prefix),
			Notes:       "BZ-11 gate: " + scope.String(),
		}); err != nil {
		t.Fatalf("policy row: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- publisher.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("publisher listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publisher not listening after 5s")
	}

	conn, err := reader.Connect(ctx, publisher.Addr().String())
	if err != nil {
		t.Fatalf("reader Connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	c, err := workbench.NewPeerConsumer(reader, publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	_, err = c.VerifiedRoot(ctx)
	return err
}

// TestBZ11_AFullyScopedGrantVerifies is the anti-vacuity arm. Without
// it, every assertion below is satisfied by a reader that cannot verify
// anything at all.
func TestBZ11_AFullyScopedGrantVerifies(t *testing.T) {
	if err := verifyPairUnderGrant(t, scopeFull, true); err != nil {
		t.Fatalf("a grant carrying the prefix and both verification locations failed to verify: %v", err)
	}
}

// TestBZ11_AbsentIsStillReportableAsAbsent is the second anti-vacuity
// arm, and it is the one a careless fix breaks.
//
// The MUST NOT says do not report authorization failure AS an absent
// root. The cheapest way to satisfy that sentence is to stop returning
// `ErrNoPublishedRoot` at all — which destroys the honest answer for the
// publisher who genuinely has not published, and that is a third
// machine's worth of wrong diagnosis.
func TestBZ11_AbsentIsStillReportableAsAbsent(t *testing.T) {
	err := verifyPairUnderGrant(t, scopeFull, false)
	if err == nil {
		t.Fatal("a peer that never minted a root verified anyway")
	}
	if !errors.Is(err, fetch.ErrNoPublishedRoot) {
		t.Fatalf("a peer that has genuinely never published reports %v, not ErrNoPublishedRoot — "+
			"the honest answer has to stay reachable or the MUST NOT is met by deleting it", err)
	}
}

// TestBZ11_AnAuthorizationFailureIsNotReportedAsAnAbsentRoot is the
// `MUST NOT` itself, at both locations.
func TestBZ11_AnAuthorizationFailureIsNotReportedAsAnAbsentRoot(t *testing.T) {
	for _, scope := range []grantScope{scopeSiteOnly, scopeNoSignature} {
		t.Run(scope.String(), func(t *testing.T) {
			err := verifyPairUnderGrant(t, scope, true)
			if err == nil {
				t.Fatalf("a grant scoped %q verified a root it is not authorized to read", scope)
			}
			if errors.Is(err, fetch.ErrNoPublishedRoot) {
				t.Fatalf("⛔ SITE §7 [MUST NOT]: a grant scoped %q reports the publisher as having "+
					"never published. The publisher HAS published; the reader's own grant caused "+
					"this, and the sentence accuses the wrong machine.\n  got: %v", scope, err)
			}
			t.Logf("scope %q reports: %v", scope, err)
		})
	}
}
