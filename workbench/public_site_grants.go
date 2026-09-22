package workbench

import (
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/types"
)

// public_site_grants.go — the `default` row: what EVERY peer that can
// dial this one may do without being named.
//
// # Why there is a separate file for four resource patterns
//
// Because this is the one grant in the system with no grantee.
// `readHandshakePolicyGrants` tries `hex(identityHash)`, then the
// Base58 peer-id, then **`default`** — so a row written here is
// assembled into the inbound grant set of a peer nobody approved, on a
// connection nobody accepted individually. Every argument that makes a
// per-peer grant reviewable ("who is this, and what did I agree to give
// them") is unavailable here by construction.
//
// AP90 is the precedent and it is three weeks old at time of writing:
// `SyncSenderGrants` carried `Resources: ["*"]` on three of four entries
// under a doc comment claiming minimality, and *"share this folder"*
// authorized every entity and every mounted file on the machine.
// Measured across the wire, a file from an unshared folder came back
// with its content hash attached. **That one named one peer. This one
// names everybody**, so the same mistake here is the same mistake
// multiplied by the reachable network.

// PublicPolicyPeer is the V7 v7.62 §8 policy table's catch-all pattern.
//
// It is a literal, not a peer-id, and it is the third and last pattern
// `readHandshakePolicyGrants` tries. Named rather than spelled inline so
// a surface can recognise the row and label it — an operator auditing
// authorization must not have to know that one of the peer-ids in the
// list is not a peer-id.
const PublicPolicyPeer = "default"

// PublicSiteGrants is what a peer must grant `default` in order to serve
// a published site to a reader it has never met.
//
// The prefix is the one the signed published-root COMMITS TO — the same
// string `publish.MintOpts.Prefix` was given — because that is exactly
// the key set a verifying consumer walks. Deriving it from anything
// else (a site id, a convention, a constant) would let the grant and the
// root disagree, and the two disagreements are opposite failures: a
// grant narrower than the root is a walk that dies partway with the
// publisher looking like it is withholding (§6.5.3), and a grant wider
// than the root is a disclosure nobody asked for.
//
// # The two `system/` resources are the part a site-shaped grant gets wrong
//
// A published site's bytes live under the prefix. The thing that makes
// them *verifiable* does not: the published-root sits at
// `system/peer/published-root` and its signature at
// `system/signature/{hex}`, both outside the prefix they commit to. A
// grant scoped to the site alone produces a peer that serves every page
// and **cannot be verified at all**, which presents to the reader as
// "this publisher has never published" — the most misleading of the
// three states it could present as, because it accuses the publisher of
// something it did not do. Measured as its own arm in
// `publish/live_and_static_test.go`.
//
// # What `system/content:get` discloses here, stated plainly
//
// **This grant gives every peer that can dial us `system/content:get`
// over the bare `system/content` namespace, and today that is not
// scoped to the site.** It is the same hole AP90 names, escalated from
// one named peer to anybody:
//
//   - `EXTENSION-CONTENT` §6.4.1 specifies a namespace-scoped topology
//     in which `get` "consults the tree binding and serves only when the
//     hash is bound under the requested namespace", and makes it a MUST
//     for multi-party deployments.
//   - core-go implements the *ingest* half of that binding
//     (`bindHashTreePresence`) and **not the get half** — `handleGet` is
//     a bare store lookup. And `local/files` chunks mounted files
//     directly without ever calling `system/content:ingest`, so for file
//     bytes the binding is unreachable whatever this tier does. Both
//     halves are routed.
//
// So what actually holds the line right now is that **a hash you cannot
// discover is a blob you cannot ask for**, and the tree grant below is
// what discloses hashes. That is why it is derived per publish and why
// widening it is not a small change. A reader who learns a hash by some
// other route can fetch that blob from a publishing peer. Said out loud
// here, and measured rather than asserted, in
// `shellboot/public_site_scope_probe_test.go`.
//
// There is no narrower grant available at this layer: a verifying
// consumer must fetch the trie's interior nodes and every committed
// leaf, all of which are content-addressed, and the resource dimension
// the handler checks is a namespace rather than a hash set.
func PublicSiteGrants(publishedPrefix string) ([]types.GrantEntry, error) {
	prefix := strings.Trim(strings.TrimSpace(publishedPrefix), "/")
	if prefix == "" {
		// The whole tree. `MintRoot` accepts an empty prefix — a root
		// over everything is legal — but a PUBLIC grant over everything
		// is `Resources: ["*"]` wearing a different spelling, and it
		// would hand a stranger this peer's devices, folders, offers,
		// mounts and every ingested document. Refuse at the derivation
		// rather than let a caller pass "" through by accident; an
		// operator who genuinely means it can write the row by hand and
		// own the sentence.
		return nil, fmt.Errorf(
			"a public site grant needs the prefix the published root commits to, and %q is the "+
				"whole tree — that would give every peer that can dial this one a read of this "+
				"peer's devices, folders, offers and every mounted file, not a site; "+
				"publish a narrower prefix (e.g. %q)", publishedPrefix, SitesSubpath+"/")
	}
	if strings.Contains(prefix, "*") {
		return nil, fmt.Errorf(
			"a public site grant is derived from a concrete published prefix, and %q contains a "+
				"wildcard — the pattern is built here so that the grant and the signed root "+
				"cannot disagree about which keys are public", publishedPrefix)
	}
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources: types.CapabilityScope{Include: []string{
				// The bare prefix and its subtree. Written bare and never
				// also as `/*/…`: under §PR-8 canonicalization a bare
				// pattern becomes `/{ourPeerID}/…`, coverage requires
				// EVERY Include member to match, and a `/*/…` form would
				// make the whole entry uncoverable — silently dropping
				// the authority the bare form grants. Same reasoning as
				// SyncSenderGrants; see access_policy.go.
				prefix,
				prefix + "/*",
				// Without these two the site serves and cannot be
				// verified. See the doc comment.
				"system/peer/published-root",
				"system/signature/*",
			}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}, nil
}

// PublicSiteNote is the `notes` field written onto the public policy
// row, and it is written for the operator who finds this row in six
// months with no memory of asking for it.
//
// The prefix is in the sentence because the row is the only durable
// record of WHICH publish it was written for: re-publishing a narrower
// prefix moves what the signed root commits to and leaves this grant
// pointing at the old one, which is a stale disclosure rather than a
// broken site.
func PublicSiteNote(publishedPrefix string) string {
	return fmt.Sprintf("public: any peer may read %q and verify this peer's published root", publishedPrefix)
}

// IsPublicSiteGrant reports whether a policy row is one of ours.
//
// Compared on the SHAPE rather than on the note, because the note is
// prose and a hand-edited row is still a public grant. A surface uses
// this to decide whether it may offer to withdraw the row: offering to
// "unpublish" a hand-written `default` entry that grants something else
// entirely would delete authorization the operator wrote deliberately.
func IsPublicSiteGrant(grants []types.GrantEntry) bool {
	if len(grants) != 2 {
		return false
	}
	var sawTree, sawContent bool
	for _, g := range grants {
		switch {
		case len(g.Handlers.Include) == 1 && g.Handlers.Include[0] == "system/tree":
			for _, r := range g.Resources.Include {
				if r == "system/peer/published-root" {
					sawTree = true
				}
			}
		case len(g.Handlers.Include) == 1 && g.Handlers.Include[0] == "system/content":
			sawContent = true
		}
	}
	return sawTree && sawContent
}

// PublicSitePrefix recovers the published prefix a public grant was
// derived from, or "" if the row is not one of ours.
//
// Exists so a surface can say *"the public grant covers `sites/`, and
// the current published root commits to `sites/notes/`"* — the stale
// disclosure the note above names. Nothing else can answer that
// question: the grant and the root are two entities with no link
// between them, and the only thing that ever tied them together is the
// act that wrote both.
func PublicSitePrefix(grants []types.GrantEntry) string {
	for _, g := range grants {
		if len(g.Handlers.Include) != 1 || g.Handlers.Include[0] != "system/tree" {
			continue
		}
		for _, r := range g.Resources.Include {
			// Skip the two verification paths by name rather than relying
			// on Include order: the slice is built here today and a
			// hand-edited row may list them in any order, which would
			// otherwise report `system/signature` as the published prefix
			// — a confidently wrong answer on an authorization surface.
			if r == "system/signature/*" || r == "system/peer/published-root" {
				continue
			}
			if strings.HasSuffix(r, "/*") {
				return strings.TrimSuffix(r, "/*")
			}
		}
	}
	return ""
}
