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

// SharedScope is one folder's contribution to a peer's authorization: the
// root OUR copy lives under, and the folder's shared identity.
//
// Both fields, and neither is derivable from the other here. **The root is
// this peer's, and the id is the OWNER's** — a folder we received and
// republish under `both` sits at our own `LocalRoot` while keeping the
// originator's id (`FolderID(owner, their-root)`), because the id is what
// makes it one object across peers (S6/AP72).
//
// Deriving either one inside this function was the first version and it
// was wrong in exactly the asymmetric case the id exists for: it built
// `FolderID(self, f.Root)` and `local/files/{f.Root}`, which on the
// receiving peer names an id nobody holds and a directory that does not
// exist. Both reverse-leg tests caught it. Read `ReceivingRoot()`, never
// `Root` — the two agree in the symmetric case, which is what makes the
// mistake survive a suite that only tests matching names.
type SharedScope struct {
	// LocalRoot is the local-files mount root on THIS peer.
	LocalRoot string
	// FolderID is the cross-peer folder identity.
	FolderID string
}

// SyncSenderGrants is what the peer PUBLISHING a folder must grant the
// peer receiving it.
//
// The HANDLER list is not invented. It is the minimum established by
// `shellcmd/cmd_stage3_cap_delegation_test.go`, which pairs a positive
// case with a negative one that drops `system/content:get` and confirms
// materialization then fails — so each handler is load-bearing by
// measurement rather than by reasoning.
//
//   - system/subscription:* — the receiver subscribes to our prefix.
//   - system/content:get — the receiver drives EnsureClosure to pull the
//     blob's chunks across. Dropping this one gets you a live
//     subscription and no bytes.
//   - local/files:read + system/tree:get — the subscription engine's own
//     pattern-match traversal over the matching tree paths.
//
// Resource patterns are written bare and never also as "/*/*": under §PR-8
// canonicalization a bare pattern becomes "/{ourPeerID}/…", a peer can
// only advertise coverage of its own namespace, and coverage requires
// EVERY Include member to match — so adding a "/*/…" form would make the
// whole entry uncoverable and silently drop the authority the bare form
// grants.
//
// # The resources are scoped to the shared folders, and were `*` until 2026-09-10
//
// Three of these four entries carried `Resources: ["*"]`, and the
// reconciler writes this set into `system/capability/policy/{peer}`
// verbatim. So the gesture *"share this folder"* authorized the receiving
// peer to read **the entire tree and every mounted file on the machine**.
//
// Measured, not reasoned: `shellboot/share_scope_probe_test.go` shares one
// folder and then reads a file from an unshared one across the wire. Before
// this change it came back — 126 bytes including the `content` hash, which
// is precisely what `system/content:get` needs. Share one folder, enumerate
// everything, fetch anything.
//
// **Why the doc comment above did not catch it.** It says this set is "the
// minimum established by cmd_stage3_cap_delegation_test.go", and that is
// true of the HANDLER LIST and false of everything else: that test's
// negative arm drops `system/content:get` *entirely* and confirms
// materialization then fails. Dropping a whole handler shows the handler is
// necessary. It says nothing about whether its resources are minimal. **A
// grant minimized along one axis reads as a minimized grant** — the
// sentence was written about the rows and got read as being about the
// cells.
//
// # `system/content` IS scopeable, we are not scoping it, and the spec
// # calls what we are doing security-defective
//
// **An earlier version of this comment said the content handler "cannot be
// scoped per folder in this or any implementation" and called that a
// property to design around. That was WRONG**, asserted without reading
// `EXTENSION-CONTENT` §6.4, and it is the worst shape of wrong: a confident
// architectural sentence that closes a question permanently (AP45). It was
// caught by the operator, who knows the capability system, and not by us.
//
// What §6.4 actually specifies:
//
//   - The dispatch's resource target is a **namespace path**, not a hash —
//     `content.EnsureClosure(…, namespace)` puts it in
//     `ResourceTarget{Targets: [namespace]}` and the hashes travel in
//     params (`ext/content/sequencer.go`). So the cap layer scopes on a
//     namespace the caller names.
//   - §6.4.2 binds each hash into the tree at `{namespace}/{hex(H)}`.
//     Lookup is one `tree:get` probe. **The tree is the capability
//     boundary** — which is the whole design, and the thing the earlier
//     comment talked itself out of.
//   - §6.4.1 defines two topologies. **Namespace-scoped is the production
//     default and a MUST for any multi-party deployment**, where get
//     "consults the tree binding and serves only when the hash is bound
//     under the requested namespace". Single-trust-domain — get resolves
//     any hash for any cap-holding caller — is opt-in, MUST NOT be the
//     default, and the spec says in terms that **"multi-party deployments
//     operating under single-trust-domain topology are out-of-spec and
//     security-defective."**
//
// **We are that sentence.** We pass the bare `"system/content"` default
// namespace (`workbench/blob_resolve.go`), we never call
// `system/content:ingest`, so nothing is bound at `{namespace}/{hex(H)}`
// anywhere in our tree, and two laptops owned by different people are
// unambiguously multi-party.
//
// # What is ALSO true, and why this is not fixable here alone
//
// core-go implements the **ingest** half — `bindHashTreePresence` writes
// the §6.4.2 binding — and **not the get half**: `handleGet` is a bare
// `hctx.Store.Get(hreq)` with no namespace consult, and `requireResource`
// only checks that a target was *supplied*. So today the namespace is a
// label the cap layer checks against your grant, after which any hash is
// served. Scoping our grant alone would narrow which label we may claim,
// not which bytes we may get. Routed.
//
// # So, until both halves land: the TREE grant is the operative boundary
//
// Stated as the current fact rather than as a law of nature. With get
// unenforced, an unshared file is confidential because its hash is
// undiscoverable, and the tree is what discloses hashes — so narrowing the
// tree grant is what actually holds the line right now. **A second
// implementation that grants a wide tree scope has re-opened this**, and one
// that implements §6.4.1 properly does not need to rely on it at all.
//
// Resource dimension 3 is only checked *when the execute carries a
// resource* (`core/capability/check.go`), so this narrowing is worth
// nothing unless the dispatches actually carry one. Verified that all three
// do: `tree.CreateGetRequest` returns a target, `local/files` reads carry
// the path, and a subscribe carries its pattern
// (`entitysdk/subscription.go`).
//
// Patterns canonicalize against the GRANTER's peer-id under §PR-8, targets
// against the request path, which is why these are written bare and never
// as `/*/…`.
func SyncSenderGrants(folders []SharedScope) []types.GrantEntry {
	// Both the bare prefix and its subtree: the prefix itself is the
	// listing target, the subtree is every file under it.
	var fileRes, subRes []string
	for _, f := range folders {
		p := LocalFilesSourcePrefix + f.LocalRoot
		fileRes = append(fileRes, p, p+"/*")
		subRes = append(subRes, p+"/*")
	}

	// The tree grant covers the shared files PLUS the folder declarations
	// for exactly the folders shared with this peer — the reverse leg reads
	// the counterpart's record over the wire (`ObserveRemoteFolder`), and
	// without it a two-way folder cannot learn which directory the other
	// side keeps it in.
	//
	// Scoped to the specific folder ids rather than `app/workbench/folders/*`:
	// that prefix holds every folder this peer shares with ANYONE, and a
	// receiver has no business enumerating the others. It is only metadata,
	// which is exactly the argument that gets a leak shipped.
	treeRes := append([]string{}, fileRes...)
	for _, f := range folders {
		treeRes = append(treeRes, FolderPrefix+f.FolderID)
	}
	// The offer records. `offers` is a dispatched read of the counterpart's
	// `app/share/records/*` (AP11), so a receiver that cannot read this
	// prefix cannot see what we offered them — measured: the flow test
	// 403s at `offers` the moment the tree grant stops being `*`.
	//
	// Prefix-wide rather than per-record, and that IS a disclosure: this
	// peer's offers to OTHER peers are readable. Narrowing it needs the
	// record id at grant time, which the reconciler does not hold, and the
	// offer is a label rather than an authority (APP-CONVENTION-SHARE
	// §2.2), so nothing is authorized by seeing one. Named here rather
	// than left silent: it is the one wildcard left in this set.
	treeRes = append(treeRes, ShareOfferPrefix+"*")

	if len(folders) == 0 {
		// No folders shared with this peer means no authority. Returning
		// the wildcard set here would make "share nothing" the most
		// permissive state in the system.
		return nil
	}

	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/subscription"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: subRes},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"read"}},
			Resources:  types.CapabilityScope{Include: fileRes},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: treeRes},
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

	// IsPublic marks the `default` catch-all row — the one entry in this
	// table whose grantee is not a peer.
	//
	// IsSelf's argument, one row over and pointing the other way. The
	// self entry is alarming and harmless; this one is the opposite: it
	// reads as an ordinary entry with an unremarkable name, and it
	// authorizes **every peer that can dial this one**. A listing that
	// prints `default` beside two peer-ids, with no more emphasis than
	// they get, has told an operator that three peers are authorized
	// when the true answer is "two peers and the network".
	IsPublic bool
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
			PeerID:   peerID,
			Grants:   d.Grants,
			Notes:    d.Notes,
			Summary:  SummarizeGrants(d.Grants),
			IsSelf:   selfIdentityHex != "" && peerID == selfIdentityHex,
			IsPublic: peerID == PublicPolicyPeer,
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
