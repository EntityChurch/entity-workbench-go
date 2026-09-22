package shellcmd

// Forget — drop this peer's memory of another peer, so a share flow can
// be tested from a known-clean state.
//
// # Why this exists
//
// Everything the flow establishes is deliberately durable: the policy
// survives a restart, the sync binding is restored at startup, the alias
// and the discovered candidate persist, and the files that arrived are
// the operator's files now. Each of those is correct on its own and the
// sum of them is a system that cannot be re-tested. An operator running
// the flow a second time cannot tell a working handshake from a stale
// grant that was already there, and the first symptom of that is not a
// failure — it is a SUCCESS they cannot trust, which is worse, because
// it makes every subsequent measurement uninterpretable.
//
// So this is a testing and recovery affordance, and it is deliberately
// narrow in one direction: it forgets RELATIONSHIPS, never CONTENT.
//
// # What it will not do
//
// It does not delete a single received file, and it does not unmount
// anything. Bytes that arrived are the operator's files — the same rule
// Unsync already holds to — and a "start clean" verb that quietly
// deleted a folder would be the most destructive thing in this
// codebase. A clean *content* slate is a directory the operator removes
// themselves, deliberately, with a tool that is named after deleting.
//
// It also does not touch this peer's own identity. "Forget peer X" and
// "become a different peer" are different acts with different blast
// radii, and the second one is a launch-time choice (`--identity`,
// or an ephemeral peer with no flag at all) rather than something a
// running session should do to itself while handles are open.

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"entity-workbench-go/workbench"
)

// ForgetOutcome is what was actually dropped. Every field is an action
// that occurred, so a surface can report the work rather than assert
// that a peer is "forgotten" — the operator's next question is always
// which half was still there.
type ForgetOutcome struct {
	PeerID    string
	PeerAlias string

	// SyncsStopped are roots this peer was receiving FROM the target.
	SyncsStopped []string

	// OffersWithdrawn are roots this peer was offering TO the target.
	OffersWithdrawn []string

	// PolicyRemoved reports the target's authorization entry going away.
	PolicyRemoved bool

	// Disconnected reports the live connection being evicted.
	Disconnected bool

	// Problems are failures that did not stop the pass. Forget is
	// best-effort by design: a half-forgotten peer is a worse state
	// than either end of the range, so a failure in one stage must not
	// abandon the others.
	Problems []string
}

// Clean reports whether every stage succeeded.
func (o ForgetOutcome) Clean() bool { return len(o.Problems) == 0 }

// Summary renders one line, and always states the problem count,
// because the entire purpose of this verb is to establish a state the
// operator can trust — a summary that reads as success while half the
// grants survive defeats the verb.
func (o ForgetOutcome) Summary() string {
	parts := []string{}
	if n := len(o.SyncsStopped); n > 0 {
		parts = append(parts, fmt.Sprintf("%d sync(s) stopped", n))
	}
	if n := len(o.OffersWithdrawn); n > 0 {
		parts = append(parts, fmt.Sprintf("%d offer(s) withdrawn", n))
	}
	if o.PolicyRemoved {
		parts = append(parts, "authorization removed")
	}
	if o.Disconnected {
		parts = append(parts, "disconnected")
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing was remembered about this peer")
	}
	if n := len(o.Problems); n > 0 {
		parts = append(parts, fmt.Sprintf("%d PROBLEM(S)", n))
	}
	return strings.Join(parts, ", ")
}

// Forget drops every relationship this peer holds with one other peer.
//
// Order is load-bearing and is the reverse of how the flow was built:
// stop receiving, then withdraw what we offer, then remove their
// authorization, then evict the connection. Dropping the connection
// first would leave the subscription teardown trying to reach a peer it
// can no longer address, which turns a clean stop into a warning the
// operator then has to interpret.
func (ws *ShellWorkspace) Forget(peer string) (ForgetOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ForgetOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	peerID, alias, err := ws.resolvePeerRef(peer)
	if err != nil {
		return ForgetOutcome{}, err
	}
	if peerID == local.PeerID() {
		return ForgetOutcome{}, fmt.Errorf(
			"refusing to forget this peer's own identity (%s) — `forget` drops what this "+
				"peer remembers about ANOTHER peer; to run as somebody else, relaunch with "+
				"a different --identity", peerID)
	}

	out := ForgetOutcome{PeerID: peerID, PeerAlias: alias}

	// 1. Stop receiving from them. Unsync is idempotent and already
	//    declines to delete anything that arrived.
	bindings, problems := workbench.LoadSyncBindings(local.Store())
	for _, p := range problems {
		out.Problems = append(out.Problems, "read sync bindings: "+p)
	}
	for _, b := range bindings {
		if b.RemotePeerID != peerID {
			continue
		}
		res, err := ws.Unsync(peerID, b.Root)
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("unsync %s: %v", b.Root, err))
			continue
		}
		if res.SubscriptionCloseErr != nil {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"unsync %s: subscription close: %v", b.Root, res.SubscriptionCloseErr))
		}
		out.SyncsStopped = append(out.SyncsStopped, b.Root)
	}

	// 2. Withdraw what we offer them. Unshare handles the offer record
	//    and the policy together for that root, and reports the
	//    remaining audience — which is why this cannot just delete the
	//    policy wholesale and skip the loop.
	offers, offerProblems := workbench.LoadShareOffers(local.Store())
	for _, p := range offerProblems {
		out.Problems = append(out.Problems, "read share offers: "+p)
	}
	for _, o := range offers {
		if !o.OfferedTo(peerID) {
			continue
		}
		if _, err := ws.Unshare(o.Root, peerID); err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("unshare %s: %v", o.Root, err))
			continue
		}
		out.OffersWithdrawn = append(out.OffersWithdrawn, o.Root)
	}

	// 3. Remove whatever authorization survives the per-root
	//    withdrawals — the delivery permission `accept` wrote is not
	//    attached to any root we offer, so step 2 cannot have removed
	//    it, and leaving it behind is precisely the stale grant that
	//    makes a re-test unreadable.
	if workbench.RemoveAccessPolicy(local.Store(), peerID) {
		out.PolicyRemoved = true
	}

	// 4. Evict the connection last, and drop the alias with it.
	//
	//    The eviction is what makes the next handshake a real one. A
	//    grant is assembled AT the handshake (AP63), so a connection
	//    that outlives this call still carries every permission it was
	//    granted when it opened — the peer would be "forgotten" in the
	//    tree and fully authorized on the wire, which is the most
	//    misleading state this verb could possibly leave behind.
	local.Disconnect(peerID)
	out.Disconnected = true
	for a, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID && a != ws.Local.Alias {
			ws.removeConn(a)
		}
	}

	sort.Strings(out.SyncsStopped)
	sort.Strings(out.OffersWithdrawn)
	return out, nil
}

// ForgetAll drops every relationship with every remote peer.
//
// The clean-slate button. It reports per-peer outcomes rather than one
// aggregate, because "three peers forgotten" and "three peers forgotten,
// one of which still holds a grant" have to be distinguishable — the
// state this verb exists to establish is one the operator is about to
// trust a measurement to.
func (ws *ShellWorkspace) ForgetAll() ([]ForgetOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	selfID := local.PeerID()

	// Collect the union of every peer we remember anything about,
	// before mutating any of it. Iterating a live map while Forget
	// removes from it is the kind of bug that leaves exactly one peer
	// behind and looks like a flake.
	targets := map[string]bool{}
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID != "" && pc.PeerID != selfID {
			targets[pc.PeerID] = true
		}
	}
	bindings, _ := workbench.LoadSyncBindings(local.Store())
	for _, b := range bindings {
		if b.RemotePeerID != "" && b.RemotePeerID != selfID {
			targets[b.RemotePeerID] = true
		}
	}
	offers, _ := workbench.LoadShareOffers(local.Store())
	for _, o := range offers {
		for _, p := range o.Audience {
			if p != "" && p != selfID {
				targets[p] = true
			}
		}
	}
	// Pass our own identity hex so the kernel's §6.9a self-seed row is
	// MARKED rather than guessed at. Passing "" leaves every row
	// unmarked, and this loop would then treat our own wildcard grant
	// as a remote peer to forget — revoking our authority over our own
	// tree. The key is the identity hash in hex, not the Base58 peer-id,
	// so a `p.PeerID != selfID` test alone does not catch it.
	policies, _ := workbench.ListAccessPolicies(local.Store(),
		hex.EncodeToString(local.IdentityHash().Bytes()))
	for _, p := range policies {
		// The kernel seeds this peer's own wildcard row at construction
		// (V7 v7.74 §6.9a). Removing it would revoke our authority over
		// our own tree, which is not "forgetting a peer" by any reading.
		if p.PeerID != "" && p.PeerID != selfID && !p.IsSelf {
			targets[p.PeerID] = true
		}
	}

	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []ForgetOutcome
	for _, id := range ids {
		res, err := ws.Forget(id)
		if err != nil {
			res = ForgetOutcome{PeerID: id, Problems: []string{err.Error()}}
		}
		out = append(out, res)
	}
	return out, nil
}
