package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// access_policy.go — per-peer authorization, which is the step between
// "we are connected" and "you may have my files".
//
// # The mechanism, and why it is this one
//
// `system/capability/policy/{pattern}` is the V7 v7.62 §8 policy table.
// `protocol.ConnectHandler.AssembleInboundGrants` reads it at
// authenticate-response and **unions** the matching entry's grants with
// the §4.4 floor. `readHandshakePolicyGrants` tries three patterns in
// order: `hex(identityHash)`, then the **Base58 peer-id**, then
// `default`.
//
// The middle one is what makes this usable. An operator who has just run
// `peers` holds a peer-id and nothing else, so a mechanism keyed only on
// an identity hash would need a lookup step nobody would perform.
// Measured working: `shellboot/policy_probe_test.go`.
//
// Nothing in this repo wrote one of these until 2026-09-02. The
// alternative we were using was `shellboot.Config.OpenAccess` — a
// process-wide wildcard, documented "development use only", which is the
// only reason cross-peer sync appeared to work at all.
//
// # Why the policy table rather than a minted token
//
// `entitysdk.AuthorShare` mints a cross-peer chain capability per
// grantee, and `ShareWithdrawalNotice` says plainly what that costs:
// a `system/capability:request`-minted token is **NOT recallable** —
// returned inline with no tree write, so the granter never holds its hash
// and `revoke`, which is keyed by hash, cannot name it. If that token
// were the authorization, `unshare` could not end access and would be
// lying by its own name.
//
// A policy entry is a tree write we own. Removing it means the next
// handshake does not carry the grant. That is a revocation we can
// actually perform, so it is the one the verbs stand on.
//
// # Three properties of this table that the verbs have to respect
//
// All three were measured, none is obvious, and each one costs a
// confusing afternoon if you meet it in the field instead of here:
//
//  1. **A sync is MUTUAL authorization.** The receiver dispatches into
//     the sender to subscribe and to fetch blobs; the sender's
//     subscription engine dispatches the notification back into the
//     receiver's `workbench/blob-resolve`. Both cross a capability
//     boundary, so both peers need an entry naming the other. Granting
//     one direction gets you an accepted subscription and no files —
//     which looks exactly like a working share.
//  2. **The grant is fixed at HANDSHAKE.** A policy written while two
//     peers are already connected changes nothing until they reconnect.
//     Every verb that writes one must re-establish the connection or say
//     that it has not taken effect.
//  3. **Advertisement discipline drops grants for unregistered
//     handlers.** An entry naming a handler this peer does not register
//     is silently filtered at connect time, so the grant set below is
//     only meaningful on a peer with the workbench handlers wired.

// AccessPolicyPrefix is the kernel's handshake policy table. `system/*`
// and therefore spec-adjacent: we WRITE entries here, we do not define
// the shape. `types.CapabilityPolicyEntryData` is the kernel's type.
const AccessPolicyPrefix = "system/capability/policy/"

// SyncSenderGrants is what the peer PUBLISHING a folder must grant the
// peer receiving it.
//
// This set is not invented. It is the minimum established by
// `shellcmd/cmd_stage3_cap_delegation_test.go`, which pairs a positive
// case with a negative one that drops `system/content:get` and confirms
// materialization then fails — so each entry is load-bearing by
// measurement rather than by reasoning.
//
//   - system/subscription:* — the receiver subscribes to our prefix.
//   - system/content:get — the receiver drives EnsureClosure to pull the
//     blob's chunks across. Dropping this one gets you a live
//     subscription and no bytes.
//   - local/files:read + system/tree:get — the subscription engine's own
//     pattern-match traversal over the matching tree paths.
//
// Resources are bare "*" and never also "/*/*": under §PR-8
// canonicalization "*" becomes "/{ourPeerID}/*", a peer can only
// advertise coverage of its own namespace, and coverage requires EVERY
// Include member to match — so adding "/*/*" would make the whole entry
// uncoverable and silently drop the authority "*" alone grants.
func SyncSenderGrants() []types.GrantEntry {
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/subscription"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"read"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
	}
}

// SyncReceiverGrants is what the peer RECEIVING a folder must grant the
// peer publishing it: the right to deliver a subscription notification
// into our blob-resolve handler.
//
// This is the direction every cross-peer test in this repo missed,
// because the receiver always ran with `OpenAccessGrants()` — including
// the test written specifically to close the capability-delegation gap,
// which scopes the sender and leaves the receiver wildcard on purpose.
// One entry, one operation, and without it a share is accepted and
// nothing ever arrives.
func SyncReceiverGrants() []types.GrantEntry {
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{BlobResolvePattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
	}
}

// AccessPolicy is one peer's authorization as a surface should show it.
type AccessPolicy struct {
	PeerID string
	Grants []types.GrantEntry
	Notes  string
	// Summary is a one-line human rendering of what the grants permit,
	// because a surface that lists a peer's authorization and cannot say
	// what it authorizes has not shown the operator anything they can
	// act on.
	Summary string

	// IsSelf marks the peer's own seed entry.
	//
	// The kernel writes one at construction (V7 v7.74 §6.9a, keyed on the
	// peer's own identity hash) granting `*:*`. It is correct and
	// load-bearing, and it is also the single most alarming row an access
	// listing can show: an operator scanning for "who can read my files"
	// finds a wildcard grant to a 66-character hex string. A surface that
	// does not name it as the peer itself has invented a security scare,
	// and one that silently hides it is concealing a real grant.
	// Distinguish it and say which it is.
	IsSelf bool
}

// SaveAccessPolicy writes (or replaces) the handshake policy for one peer.
//
// Replaces rather than merges, deliberately. Merging would make the
// stored authority a function of every call ever made, so an operator
// could not read the entry and know what it grants — and could not
// narrow it without deleting first. One caller, one statement of intent.
func SaveAccessPolicy(st *Store, peerID string, grants []types.GrantEntry, notes string) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if strings.TrimSpace(peerID) == "" {
		return fmt.Errorf("access policy needs a peer id")
	}
	if strings.Contains(peerID, "/") {
		return fmt.Errorf("peer id %q contains a path separator; it is one path segment", peerID)
	}
	if len(grants) == 0 {
		return fmt.Errorf("access policy for %s grants nothing — remove it instead of "+
			"writing an empty one, which reads as authorization that is present and inert", peerID)
	}
	if _, err := st.Put(AccessPolicyPrefix+peerID, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: peerID,
			Grants:      grants,
			Notes:       notes,
		}); err != nil {
		return fmt.Errorf("write access policy for %s: %w", peerID, err)
	}
	return nil
}

// LoadAccessPolicy reads one peer's policy.
func LoadAccessPolicy(st *Store, peerID string) (AccessPolicy, bool) {
	if st == nil || peerID == "" {
		return AccessPolicy{}, false
	}
	ent, ok := st.Get(AccessPolicyPrefix + peerID)
	if !ok || ent.Type != types.TypeCapPolicyEntry {
		return AccessPolicy{}, false
	}
	var d types.CapabilityPolicyEntryData
	if err := ecf.Decode(ent.Data, &d); err != nil {
		return AccessPolicy{}, false
	}
	return AccessPolicy{
		PeerID:  peerID,
		Grants:  d.Grants,
		Notes:   d.Notes,
		Summary: SummarizeGrants(d.Grants),
	}, true
}

// RemoveAccessPolicy drops one peer's policy. Reports whether something
// was there.
//
// The caller owes the operator one more sentence than this function can
// give them: removing the entry stops the NEXT handshake from carrying
// the grant, and does not close a connection that already holds one.
func RemoveAccessPolicy(st *Store, peerID string) bool {
	if st == nil || peerID == "" {
		return false
	}
	return st.Remove(AccessPolicyPrefix + peerID)
}

// ListAccessPolicies reads every policy entry, sorted by peer.
//
// Rows that do not decode come back as problems rather than being
// dropped: this table is an authorization surface, and an entry we
// cannot read is exactly the one an operator needs told about (AP33).
// selfIdentityHex, when non-empty, is the peer's own identity hash in
// hex — the key the kernel's seed entry is written under. Callers that
// have it get IsSelf set; callers that pass "" get every row unmarked,
// which is the honest degradation rather than a guess.
func ListAccessPolicies(st *Store, selfIdentityHex string) (policies []AccessPolicy, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(AccessPolicyPrefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		peerID, under := RelativeUnder(e.Path, AccessPolicyPrefix)
		if !under || peerID == "" || strings.Contains(peerID, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, peerID+": listed but not resolvable")
			continue
		}
		if ent.Type != types.TypeCapPolicyEntry {
			problems = append(problems, peerID+": unexpected type "+ent.Type)
			continue
		}
		var d types.CapabilityPolicyEntryData
		if err := ecf.Decode(ent.Data, &d); err != nil {
			problems = append(problems, peerID+": did not decode: "+err.Error())
			continue
		}
		policies = append(policies, AccessPolicy{
			PeerID:  peerID,
			Grants:  d.Grants,
			Notes:   d.Notes,
			Summary: SummarizeGrants(d.Grants),
			IsSelf:  selfIdentityHex != "" && peerID == selfIdentityHex,
		})
	}
	sort.Slice(policies, func(i, j int) bool { return policies[i].PeerID < policies[j].PeerID })
	sort.Strings(problems)
	return policies, problems
}

// SummarizeGrants renders a grant set as `handler:op,op` clauses.
//
// Deliberately mechanical rather than interpretive: it names exactly what
// is in the entry. A friendly summary ("can sync files") would be a
// second, drifting description of the authority, and the one place an
// operator checks what they granted is not the place to paraphrase.
func SummarizeGrants(grants []types.GrantEntry) string {
	if len(grants) == 0 {
		return "(nothing)"
	}
	parts := make([]string, 0, len(grants))
	for _, g := range grants {
		h := strings.Join(g.Handlers.Include, "|")
		if h == "" {
			h = "*"
		}
		ops := strings.Join(g.Operations.Include, ",")
		if ops == "" {
			ops = "*"
		}
		parts = append(parts, h+":"+ops)
	}
	sort.Strings(parts)
	return strings.Join(parts, "  ")
}
