package shellcmd

// reconcile.go — the control loop that makes the substrate match what the
// operator declared.
//
// # The thing this replaces
//
// `Share` / `Accept` / `CompleteShare` are a WIZARD: each one performs a
// fixed sequence of substrate mutations at the moment of a button press,
// and each one encodes, inside that press, the four authorization facts
// AP63 cost a day to learn. That shape has three consequences the
// operator met all three of:
//
//   - **Order-dependent.** Share before the peer is reachable, or accept
//     before a mount exists, and the step is simply lost. There is no
//     path back except doing the whole sequence again in the right order,
//     on two machines.
//   - **Not idempotent.** Running a step twice is not obviously safe, so
//     nothing runs them again on your behalf.
//   - **Dead after a restart.** A wizard has no representation of what it
//     established, so nothing can re-establish it. Every restart defect in
//     this flow is that sentence.
//
// A reconciler has none of those properties, and it is not a bigger idea
// — it is a smaller one. There is no sequence. There is a declaration
// (workbench.DeviceData + workbench.FolderData) and a function that makes
// the world match it, which is safe to run at startup, after any change,
// on a connection event, or on a timer.
//
// # What it owns, that used to be spread across three verbs
//
//  1. **The per-peer policy row is a UNION over folders, in both
//     directions**, computed once and written once. This is not tidiness;
//     the split version is a live defect. `Share` writes
//     SyncSenderGrants to `system/capability/policy/{peer}` and `Accept`
//     writes SyncReceiverGrants to the SAME PATH, and SaveAccessPolicy
//     replaces rather than merges — correctly, by its own documented
//     contract, which assumes one caller with one intent. With two
//     callers, sharing a folder to a peer AND accepting one from the same
//     peer means the second write silently revokes the first, so two-way
//     sharing between one pair of peers cannot authorize. Every two-way
//     test in this repo runs under `OpenAccessGrants()` and therefore
//     cannot see it — AP63, one instance further on than the one that
//     named AP63.
//  2. **Reconnect is the KERNEL's**, via `system/network:maintain-peer`,
//     which connects, installs the §4.1 reconnect continuation graph,
//     retries forever with derived backoff, and restores subscriptions on
//     reconnect. Nothing in this repo called it — ten references, nine in
//     its own file and one in its own test — while three of our verbs
//     hand-rolled a single `Connect` and called it a relationship.
//  3. **A policy change forces a re-handshake, and ONLY a change does.**
//     Grants are assembled at handshake, so a policy written on a live
//     connection is inert. But reconnecting on every pass would make the
//     loop an outage generator. The rule is: write, then reconnect,
//     exactly when the desired grants differ from the stored ones. At
//     startup no reconnect is needed at all — the process has no
//     connections yet, so the first handshake reads current policy.
//
// # What it deliberately does NOT do
//
// It does not create mounts for folders that have none, and it does not
// delete anything. Creating a mount means choosing a directory on
// somebody's disk; deleting means removing their files or their access.
// Both are operator decisions, so the loop REPORTS them as problems with
// the action named, and a surface offers the action. A reconciler that
// silently creates and destroys is one nobody will run at startup.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// ReconcileOutcome is what one pass did and what it found.
//
// Actions and Problems are both plain lines meant for an operator. The
// split is: an Action is something that CHANGED, a Problem is something
// that could not be made true and why. A pass over a settled system
// produces neither, which is the signal that everything is as declared.
type ReconcileOutcome struct {
	Devices  []DeviceStatus
	Folders  []FolderStatus
	Actions  []string
	Problems []string

	// Reconciled distinguishes a pass from a READ of the same records
	// (see status.go). A read is cheap and safe on a wake; only a pass
	// dials, writes policy, or establishes a subscription — so only a
	// pass turns "declared" into "verified".
	//
	// It is a field on the outcome rather than a fact the caller
	// remembers, because "this reading was verified" is a property of the
	// reading. A surface that had to recall which function produced its
	// table in order to caption it will eventually caption it wrong, and
	// the wrong caption is the confident one.
	Reconciled bool
}

// Settled reports whether the pass found nothing to do and nothing wrong.
func (o ReconcileOutcome) Settled() bool {
	return len(o.Actions) == 0 && len(o.Problems) == 0
}

// DeviceStatus is the observed state of one declared peer.
type DeviceStatus struct {
	PeerID string
	Label  string
	// Address is the address the loop will dial, bare host:port, or "".
	Address string
	// Maintained reports that the kernel's reconnect graph is installed
	// for this peer — the difference between "we connected once" and
	// "this relationship is being kept alive".
	Maintained bool
	// Connected is the pool's view right now.
	Connected bool
	// Paused mirrors the declaration.
	Paused bool
	// Note explains a false Maintained.
	Note string

	// OutboundGrant is what OUR policy row grants this peer, summarized —
	// "(nothing)" when we grant them nothing at all.
	//
	// Exactly knowable, because it is our row and we wrote it. There is
	// deliberately no inbound counterpart: what THEY grant US is in their
	// capability table, which we cannot read, and it is only ever
	// observed through deliveries arriving. A field here claiming to know
	// it would be wrong in exactly the case that matters — they revoked
	// us and we have not tried since — while looking authoritative.
	OutboundGrant string

	// AddedAtMillis / LastSeenMillis are Unix MILLIseconds, from the
	// declaration. The unit is in the name because these cross cgo and
	// JSON, where a doc comment is invisible and a mistaken unit has
	// already killed this process once (AP61).
	AddedAtMillis  uint64
	LastSeenMillis uint64
}

// FolderPeerStatus is one peer's participation in a folder, as a surface
// needs it: identified, named, stated, and timed.
//
// Structured rather than the pre-rendered "peer (state)" line this
// replaced. A renderer that has to split a string back apart in order to
// colour the state or offer an action against the peer will get one of
// the two wrong, and a peer-id contains no separator that makes the split
// safe anyway.
type FolderPeerStatus struct {
	PeerID string
	Label  string
	State  string
	// AtMillis is when this state was last set. Unix milliseconds.
	AtMillis uint64
	Note     string
}

// FolderStatus is the observed state of one declared folder.
type FolderStatus struct {
	ID    string
	Label string
	// Local distinguishes a folder we own from one we received.
	Local bool
	// Mounted reports that the local-files mount backing this folder
	// exists. A folder with no mount produces nothing and receives
	// nothing, and every surface that reads the TREE will still describe
	// it as fine — which is how a dead mount stayed invisible for a month.
	Mounted bool
	// Syncing reports that a received folder has its subscription and
	// binding in place.
	Syncing bool
	// PeerStates is one entry per peer this folder is shared with.
	PeerStates []FolderPeerStatus
	Note       string

	// Root is the mount root this folder is bound to — for a RECEIVED
	// folder, the ORIGINATING peer's root name, because that is what the
	// subscription and the sync binding are keyed on.
	Root string
	// LocalRoot is the mount root on THIS peer that the bytes live in.
	// Equal to Root in the symmetric case; different the moment an
	// operator receives `downloads` into `~/from-alice`.
	LocalRoot string
	// Path is the local directory the declaration remembers. It is what
	// makes a remount possible after the mount that knew is gone.
	Path string
	// Origin is "local" for a folder we own, else the peer we received it
	// from.
	Origin string
	// Mode is send / receive / both.
	Mode string

	// Accepted is set for a RECEIVED folder we have accepted. A folder
	// that is merely offered is not supposed to have anything
	// established for it, so a surface must not read its missing mount
	// as a fault.
	Accepted bool
	// AcceptedAtMillis is when we accepted it. Unix milliseconds.
	AcceptedAtMillis uint64

	// FilesPresent is the SOURCE layer: one entity per file the mount
	// admits, i.e. what is on disk. FilesIngested is how many of those
	// have become documents at the target prefix.
	//
	// Both, never one (AP59). A single count reads whichever layer cannot
	// fail — which is how a directory of 400 photographs and one README
	// displayed "401 entities" with exactly one openable document.
	FilesPresent  int
	FilesIngested int
	// FilesObservable is false when there is no mount to count. A
	// renderer says *unknown* rather than a confident 0: "nothing has
	// arrived" and "there is nowhere for it to arrive" are different
	// claims and this folder's whole problem is usually the second one.
	FilesObservable bool
}

// Reconcile runs one pass. Safe to call at startup, after any change to
// the declaration, and repeatedly: a settled system produces an outcome
// with no actions and no problems, and in particular writes no policy and
// forces no reconnect.
func (ws *ShellWorkspace) Reconcile(ctx context.Context) (ReconcileOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ReconcileOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	st := local.Store()
	out := ReconcileOutcome{Reconciled: true}

	devices, devProblems := workbench.LoadDevices(st)
	folders, folderProblems := workbench.LoadFolders(st)
	for _, p := range devProblems {
		out.Problems = append(out.Problems, "device record "+p)
	}
	for _, p := range folderProblems {
		out.Problems = append(out.Problems, "folder record "+p)
	}

	// 1. Addresses. Discovery is the only source that improves on its own,
	// so fold whatever it currently knows into the declaration before
	// anything tries to dial. An empty discovery result never erases what
	// we already had (DeviceData.WithAddress refuses an empty address) —
	// a peer that is asleep must not cost us the address we reached it at
	// yesterday.
	devices = ws.refreshDeviceAddresses(devices, &out)

	// 2. Policy. Computed as a union across every folder, per peer, and
	// written only when it differs from what is stored.
	desired := desiredGrantsByPeer(local.PeerID(), folders)
	changedPolicy := ws.reconcilePolicies(desired, &out)

	// 3. Connections. maintain-peer is idempotent per peer (the handler
	// keys a session by peer-id and re-entry is a no-op), so this runs
	// unconditionally; the reconnect for a changed policy happens first,
	// so maintain sees an already-connected peer and only installs the
	// graph.
	for _, d := range devices {
		out.Devices = append(out.Devices, ws.reconcileDevice(ctx, d, changedPolicy, &out))
	}

	// 4. Folders: report what exists, establish what can be established
	// without making a decision that belongs to a human.
	for _, f := range folders {
		out.Folders = append(out.Folders, ws.reconcileFolder(f, &out))
	}

	sort.Strings(out.Actions)
	sort.Strings(out.Problems)
	return out, nil
}

// refreshDeviceAddresses folds the current discovery snapshot into the
// device records. Returns the updated set so the rest of the pass dials
// the fresh address rather than the one it was called with.
func (ws *ShellWorkspace) refreshDeviceAddresses(devices []workbench.DeviceData, out *ReconcileOutcome) []workbench.DeviceData {
	local := ws.Local.Peer
	if !local.DiscoveryEnabled() {
		return devices
	}
	seen := map[string]string{}
	for _, cand := range local.ReadDiscoveredCandidates() {
		if addr := entitysdk.DialAddressForCandidate(cand); addr != "" && cand.PeerID != "" {
			seen[cand.PeerID] = addr
		}
	}
	for i, d := range devices {
		addr, ok := seen[d.PeerID]
		if !ok || addr == d.PreferredAddress() {
			continue
		}
		next := d.WithAddress(addr)
		if err := workbench.SaveDevice(local.Store(), next); err != nil {
			out.Problems = append(out.Problems,
				fmt.Sprintf("%s: could not record the discovered address %s: %v", d.ShortLabel(), addr, err))
			continue
		}
		devices[i] = next
		out.Actions = append(out.Actions,
			fmt.Sprintf("%s: learned address %s from discovery", next.ShortLabel(), addr))
	}
	return devices
}

// desiredGrantsByPeer computes, for every peer named by any folder, the
// full set of grants that peer needs from us.
//
// The union is the whole point. A peer we publish to needs the sender
// grants; a peer we receive from needs the receiver grant; a peer in both
// relationships needs both, and the two verbs that write this row today
// each write only their own half over the top of the other's.
func desiredGrantsByPeer(selfPeerID string, folders []workbench.FolderData) map[string][]types.GrantEntry {
	needSender := map[string]bool{}
	needReceiver := map[string]bool{}

	for _, f := range folders {
		if f.IsLocal() {
			// We publish it. Every peer it is offered to or accepted by
			// must be able to subscribe and pull the blob closure.
			//
			// `offered` counts, not only `accepted`: the grant is what
			// makes accepting POSSIBLE. Waiting for their acceptance
			// before authorizing it is the deadlock the nine-step flow
			// worked around by making the operator press things in a
			// particular order on two machines.
			for _, p := range f.SharedWith {
				if p.PeerID == "" || p.PeerID == selfPeerID {
					continue
				}
				switch p.State {
				case workbench.FolderStateOffered, workbench.FolderStateAccepted:
					needSender[p.PeerID] = true
				}
			}
			continue
		}
		// We received it. Only an ACCEPTED folder authorizes the origin
		// to deliver into our blob-resolve handler — an offer we have not
		// accepted must not grant a stranger a handler.
		if ps, ok := f.PeerState(f.Origin); ok && ps.State == workbench.FolderStateAccepted {
			needReceiver[f.Origin] = true
		}
	}

	out := map[string][]types.GrantEntry{}
	for peerID := range needSender {
		out[peerID] = append(out[peerID], workbench.SyncSenderGrants()...)
	}
	for peerID := range needReceiver {
		out[peerID] = append(out[peerID], workbench.SyncReceiverGrants()...)
	}
	return out
}

// reconcilePolicies writes each peer's policy row when it differs from
// what is stored, and reports which peers changed so the connection can
// be re-established for them and for nobody else.
func (ws *ShellWorkspace) reconcilePolicies(desired map[string][]types.GrantEntry, out *ReconcileOutcome) map[string]bool {
	st := ws.Local.Peer.Store()
	changed := map[string]bool{}
	for peerID, grants := range desired {
		wrote, existed, err := writePolicyIfChanged(st, peerID, grants,
			"reconciled: "+workbench.SummarizeGrants(grants))
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("policy for %s: %v", peerID, err))
			continue
		}
		if !wrote {
			continue
		}
		changed[peerID] = true
		verb := "wrote"
		if existed {
			verb = "updated"
		}
		out.Actions = append(out.Actions,
			fmt.Sprintf("%s authorization for %s (%s)", verb, peerID, workbench.SummarizeGrants(grants)))
	}
	return changed
}

// ApplyDeclaredPolicy recomputes ONE peer's policy row from the folder
// declarations and writes it when it differs. Reports whether the row
// changed.
//
// This is the single writer AP68 asks for, and it is exported to the
// package's verbs for one reason: `share` and `accept` must not write
// their own half of the row.
//
// `system/capability/policy/{peer}` is ONE row per peer and
// `SaveAccessPolicy` REPLACES it. `Share` needs the sender grants there
// and `Accept` needs the receiver grants, so whichever ran second used to
// silently revoke the other — and a pair of machines that share a folder
// each way is the first thing anyone with a laptop and a desktop sets up.
// The reconciler computed the union correctly and healed it, but only on
// its next pass: at the following `status`, or the next launch. Between
// the two verbs and that pass, one direction was dead, which is a window
// an operator lands in by doing the two gestures the product is for.
//
// So the verbs call this instead. The row is derived from the
// declarations they have just written, which makes the union the only
// form it is ever in, rather than a correction applied later.
func (ws *ShellWorkspace) ApplyDeclaredPolicy(peerID, note string) (bool, error) {
	grants, st, err := ws.declaredGrantsFor(peerID)
	if err != nil {
		return false, err
	}
	if len(grants) == 0 {
		// Nothing declared authorizes this peer any more. Deliberately NOT
		// a removal: withdrawing access is an operator's decision to make
		// (`unshare`, via WithdrawDeclaredPolicy), and a verb that
		// half-failed must not take it as a side effect.
		return false, nil
	}
	wrote, _, err := writePolicyIfChanged(st, peerID, grants, note)
	return wrote, err
}

// WithdrawDeclaredPolicy re-derives peerID's row after a withdrawal, and
// REMOVES it when the declarations no longer authorize anything at all.
//
// Separate from ApplyDeclaredPolicy because "no grants left" means
// different things in the two situations, and collapsing them breaks one
// of the two. On a withdrawal, removing the row IS the intent. On a
// failed share, the same emptiness is an accident of the moment and
// deleting a peer's authority because an unrelated write failed is a
// worse outcome than leaving a row the next reconcile will correct.
//
// What it replaces is worth naming, because it read as careful and was
// not: `unshare` used to remove the row whenever the peer appeared in no
// remaining OFFER. Offers only describe the outgoing direction, so
// unsharing the last folder from a peer we also RECEIVE one from deleted
// their delivery grant too — the incoming folder then 404s on every
// delivery, with the operator having asked to stop sharing something
// else entirely.
func (ws *ShellWorkspace) WithdrawDeclaredPolicy(peerID, note string) (removed bool, err error) {
	grants, st, err := ws.declaredGrantsFor(peerID)
	if err != nil {
		return false, err
	}
	if len(grants) == 0 {
		return workbench.RemoveAccessPolicy(st, peerID), nil
	}
	if _, _, err := writePolicyIfChanged(st, peerID, grants, note); err != nil {
		return false, err
	}
	return false, nil
}

// declaredGrantsFor is the union one peer is owed by the declarations.
func (ws *ShellWorkspace) declaredGrantsFor(peerID string) ([]types.GrantEntry, *workbench.Store, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	folders, _ := workbench.LoadFolders(local.Store())
	return desiredGrantsByPeer(local.PeerID(), folders)[peerID], local.Store(), nil
}

// writePolicyIfChanged is the one place a policy row is written.
//
// It writes only on a real difference, which is what keeps the loop from
// being an outage generator: a rewritten row means a reconnect, because
// grants are assembled at handshake. Reports whether it wrote, and
// whether a row was already there (the caller words "wrote" vs
// "updated" from it).
func writePolicyIfChanged(st *workbench.Store, peerID string, grants []types.GrantEntry, note string) (wrote, existed bool, err error) {
	existing, existed := workbench.LoadAccessPolicy(st, peerID)
	if existed && grantsEquivalent(existing.Grants, grants) {
		return false, existed, nil
	}
	if err := workbench.SaveAccessPolicy(st, peerID, grants, note); err != nil {
		return false, existed, err
	}
	return true, existed, nil
}

// dialedThisProcess records the peers this PROCESS has opened an
// outbound connection to.
//
// # Why a process-scoped set, and why it is the fix for a real defect
//
// Measured 2026-09-03 with two real peers: share a folder, accept it,
// watch files flow — then restart the SENDING peer. Its writes stop
// reaching the receiver, and `status` reports **"settled — everything
// declared is established."** A manual `connect` on the sender restores
// delivery instantly.
//
// The cause is that **a connection has a direction for authority and none
// for display.** A dial-by-address authorizes the DIALER only (AP63), so
// delivery from us to them runs over the connection WE opened. After a
// restart we have opened none — but the receiver's own connection is
// still in our pool, so `ConnectedPeers()` says "connected", maintain-peer
// finds a live session and is satisfied, and every surface agrees the
// relationship is fine while nothing can be dispatched over it.
//
// So our outbound connection is **derived runtime state that must be
// re-established at open**, exactly like the subscription engine's path
// index (AP62) and like maintain-peer's own session map, which
// `AGENTS.md` already records as re-issued per launch. This set is what
// makes "once per process" expressible: empty at startup, so the first
// pass dials every declared peer we have an address for, and later passes
// skip them — because reconnecting on every pass would make the loop an
// outage generator, which is the same reason the policy write is
// conditional.
//
// Not persisted, deliberately. Persisting it would reinstate the bug: the
// whole point is that the fact expires when the process does.
func (ws *ShellWorkspace) markDialed(peerID string) {
	if ws.dialedThisProcess == nil {
		ws.dialedThisProcess = map[string]bool{}
	}
	ws.dialedThisProcess[peerID] = true
}

func (ws *ShellWorkspace) hasDialedThisProcess(peerID string) bool {
	return ws.dialedThisProcess[peerID]
}

// ensureOutboundRoute opens OUR connection to a peer, once per process.
//
// Reports whether it dialled and what to say about it. A failure is not
// an error to unwind anything for: the declaration is right, the peer is
// simply not there, and the next pass tries again.
func (ws *ShellWorkspace) ensureOutboundRoute(ctx context.Context, d workbench.DeviceData, st *DeviceStatus, out *ReconcileOutcome) {
	if ws.hasDialedThisProcess(d.PeerID) {
		return
	}
	addr := st.Address
	if addr == "" {
		// Nothing to dial. Named rather than silently skipped: for a
		// folder we PUBLISH, this is the difference between working and
		// not, and it reads as "connected" everywhere else.
		if ws.publishesTo(d.PeerID) {
			st.Note = "connected, but this peer has never given us an address to dial — " +
				"our own outbound connection is what carries deliveries, so nothing we " +
				"write will reach them until one of us dials the other"
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s: we publish a folder to this peer and have no address to dial them at — "+
					"run `connect <alias> <host:port>`, or have them dial us", st.Label))
		}
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := ws.Local.Peer.Connect(dialCtx, addr); err != nil {
		st.Note = fmt.Sprintf("could not dial %s: %v", addr, err)
		out.Problems = append(out.Problems, fmt.Sprintf(
			"%s: could not open our own connection to %s (%v) — until it succeeds, "+
				"anything we write to a shared folder will not reach them",
			st.Label, addr, err))
		return
	}
	ws.markDialed(d.PeerID)
	// Deliberately NOT ws.rememberAddress: that invents a shell alias
	// from the peer-id when none exists, and the loop doing so made the
	// operator's own `connect b <addr>` fail with *"already bound to
	// alias 2k7u4nx2"*. The address is already in the DECLARATION, which
	// is where a durable fact belongs; dialableAddressFor reads it from
	// there.
	st.Connected = true
	out.Actions = append(out.Actions,
		fmt.Sprintf("%s: opened our outbound connection to %s so deliveries can reach them",
			st.Label, addr))
}

// publishesTo reports whether any declared folder is shared OUT to this
// peer — i.e. whether we ever need to dispatch to them.
func (ws *ShellWorkspace) publishesTo(peerID string) bool {
	folders, _ := workbench.LoadFolders(ws.Local.Peer.Store())
	for _, f := range folders {
		if !f.IsLocal() {
			continue
		}
		for _, p := range f.SharedWith {
			if p.PeerID != peerID {
				continue
			}
			if p.State == workbench.FolderStateOffered || p.State == workbench.FolderStateAccepted {
				return true
			}
		}
	}
	return false
}

// reconcileDevice brings one peer's connection to the declared state.
func (ws *ShellWorkspace) reconcileDevice(ctx context.Context, d workbench.DeviceData, changedPolicy map[string]bool, out *ReconcileOutcome) DeviceStatus {
	local := ws.Local.Peer
	// The read half is shared with StatusSnapshot, so a pass and a read
	// cannot describe the same device differently — which they would, the
	// first time one of them grew a field.
	st := ws.observeDevice(d)
	if d.Paused {
		return st
	}

	// A changed policy is only in force at the NEXT handshake, so a live
	// connection has to be replaced. Done before maintain-peer so the
	// session it installs sits on top of a connection that already
	// carries the new grants.
	if changedPolicy[d.PeerID] && st.Connected {
		ok, note := ws.refreshGrantConnection(d.PeerID)
		if ok {
			out.Actions = append(out.Actions,
				fmt.Sprintf("%s: reconnected so the new authorization is in force", st.Label))
		} else {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %s", st.Label, note))
		}
	}

	// OUR outbound connection, once per process, BEFORE maintain-peer.
	//
	// maintain-peer is satisfied by any live session, including one the
	// other side opened — and a connection they opened does not authorize
	// us to dispatch over it (AP63). So this has to happen first, and it
	// has to happen even when everything already looks connected.
	ws.ensureOutboundRoute(ctx, d, &st, out)

	// The kernel's relationship lifecycle. This is the call that answers
	// "why do I have to press connect again after a restart".
	res, err := local.Network().MaintainPeer(ctx, d.PeerID, entitysdk.MaintainOpts{Address: st.Address})
	if err != nil {
		st.Note = err.Error()
		out.Problems = append(out.Problems,
			fmt.Sprintf("%s: not maintained (%v)%s", st.Label, err, addressHint(st.Address)))
		return st
	}
	st.Maintained = res.SessionID != ""
	if !st.Maintained {
		st.Note = "maintain-peer returned no session id"
	}
	return st
}

// reconcileFolder reports one folder's substrate state and establishes
// what can be established without a human decision.
func (ws *ShellWorkspace) reconcileFolder(f workbench.FolderData, out *ReconcileOutcome) FolderStatus {
	// Same sharing argument as reconcileDevice: observe first, then act.
	// The diagnostics come from FolderStatus.problems() so the pass and
	// the read produce the SAME sentence for the same state — two copies
	// of a diagnostic drift the moment one of them is improved.
	fs := ws.observeFolder(f)

	if f.IsLocal() {
		if !fs.Mounted {
			// Named, not fixed. Re-creating a mount means writing to a
			// path on somebody's disk that this record only remembers.
			fs.Note = "no local mount for root " + f.Root + " — nothing is published from this folder"
			out.Problems = append(out.Problems, fs.problems()...)
		}
		return fs
	}

	// A received folder. The mount is where the bytes land; without it
	// every delivery answers 404 no_mount_for_uri while the sync row
	// still lists as healthy, which is the exact failure this product
	// shipped twice.
	if !fs.Accepted {
		state := "never offered to us"
		if ps, ok := f.PeerState(f.Origin); ok && ps.State != "" {
			state = ps.State
		}
		fs.Note = "not accepted (" + state + ") — nothing is established for it"
		return fs
	}
	if !fs.Mounted {
		fs.Note = "accepted, but there is no local mount at root " + fs.LocalRoot + " to receive into"
		out.Problems = append(out.Problems, fs.problems()...)
		return fs
	}
	if !fs.Syncing {
		// This one IS establishable: the mount exists and the operator
		// already said yes, so subscribing is carrying out their decision
		// rather than making one.
		if _, err := ws.Sync(SyncRequest{
			Remote: f.Origin, Root: f.Root, TargetRoot: fs.LocalRoot,
		}); err != nil {
			fs.Note = "could not subscribe: " + err.Error()
			out.Problems = append(out.Problems,
				fmt.Sprintf("folder %q from %s: %v", fs.Label, f.Origin, err))
			return fs
		}
		fs.Syncing = true
		out.Actions = append(out.Actions,
			fmt.Sprintf("folder %q: subscribed to %s and caught up", fs.Label, f.Origin))
	}
	return fs
}

// grantsEquivalent compares two grant sets for the purpose of deciding
// whether a write (and therefore a reconnect) is needed.
//
// Compared on a canonical rendering rather than field by field, and
// order-insensitively, because the union in desiredGrantsByPeer is built
// from map iteration: an order-sensitive comparison would report a
// difference on most passes, rewrite an identical policy, and reconnect
// every peer every time — a loop that generates outages while reporting
// that everything is as declared.
func grantsEquivalent(a, b []types.GrantEntry) bool {
	if len(a) != len(b) {
		return false
	}
	ka, kb := grantKeys(a), grantKeys(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

func grantKeys(g []types.GrantEntry) []string {
	out := make([]string, 0, len(g))
	for _, e := range g {
		out = append(out, strings.Join([]string{
			strings.Join(e.Handlers.Include, ","),
			strings.Join(e.Handlers.Exclude, ","),
			strings.Join(e.Operations.Include, ","),
			strings.Join(e.Operations.Exclude, ","),
			strings.Join(e.Resources.Include, ","),
			strings.Join(e.Resources.Exclude, ","),
		}, "|"))
	}
	sort.Strings(out)
	return out
}

// dialHostPort reduces a discovery/connection address to the bare
// host:port that maintain-peer requires.
//
// `Peer.RegisterRemote` REFUSES an address carrying a scheme — *"pass
// host:port only"* — and it builds a TCP profile, so a ws:// address is
// not merely differently spelled, it is a different transport this seam
// cannot express. A ws:// peer therefore gets no address here and is
// maintained by peer-id alone, which works when it has advertised its own
// transport profile and reports honestly when it has not.
func dialHostPort(addr string) string {
	addr = strings.TrimSpace(addr)
	switch {
	case addr == "":
		return ""
	case strings.HasPrefix(addr, "tcp://"):
		return strings.TrimPrefix(addr, "tcp://")
	case strings.Contains(addr, "://"):
		return "" // ws/wss/http — not expressible as a TCP profile
	default:
		return addr
	}
}

// addressHint appends the reason a maintain failure is most likely to be
// an addressing problem rather than a peer problem.
func addressHint(addr string) string {
	if addr != "" {
		return ""
	}
	return " — no address is known for this peer and it has not published a transport profile we can resolve"
}

// ReconcileWithTimeout is the startup form: one pass, bounded, so a peer
// that is unreachable delays the app by a known amount instead of an
// unknown one.
//
// The bound matters more than it looks. maintain-peer CONNECTS as part of
// its first call, and a declared peer that is switched off is the normal
// case, not the exceptional one — so an unbounded startup reconcile makes
// launching the app depend on whether another machine happens to be
// awake.
func (ws *ShellWorkspace) ReconcileWithTimeout(d time.Duration) (ReconcileOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return ws.Reconcile(ctx)
}
