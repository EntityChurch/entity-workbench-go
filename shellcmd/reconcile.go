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

	// Delivery is this peer's subscription-delivery saturation — see
	// delivery_health.go. Carried on the outcome because a peer that has
	// dropped notifications is publishing an incomplete folder to
	// everyone subscribed to it, which is a fact about the WHOLE reading
	// and not about any one device or folder row.
	Delivery DeliveryHealth

	// HistoryLimits are the paths whose change recording this peer
	// STOPPED because their chain outgrew its budget — see
	// history_budget.go. Carried on the outcome for Delivery's reason: it
	// is a standing fact about the peer rather than about one row, and it
	// is the one condition in this whole loop where the system is working
	// exactly as designed AND an operator has lost something they may
	// want. Each one is also appended to Problems, so a surface that
	// renders only the prose still says it.
	HistoryLimits []workbench.HistoryLimitData

	// Conflicts are the files where a delivery replaced a local edit and
	// the operator has not decided yet — see conflict_op.go. Carried for
	// HistoryLimits' reason: a standing state of the peer rather than a
	// property of one row, and the one condition in this loop where
	// everything worked exactly as designed and somebody still lost
	// something they may want back.
	Conflicts []workbench.ConflictData

	// ConflictHealth is the burst limiter's state. Storming means this
	// peer has deliberately stopped materializing deliveries, which is
	// neither healthy nor broken and is said nowhere else.
	ConflictHealth workbench.ConflictHealth
}

// Settled reports whether the pass found nothing to do and nothing wrong.
//
// Saturation deliberately does NOT make a system unsettled. A drop is a
// past event on a process counter, not a present divergence between
// declared and actual, and a peer that dropped a notification an hour ago
// and has been idle since is settled by every meaning the loop has. It is
// reported through Problems by the caller that observed it, so it is
// visible without making "settled" a claim the loop cannot re-derive.
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
	// Connected is the pool's view right now — a session exists, in
	// EITHER direction. On its own it is NOT the answer to "can we reach
	// them"; see OutboundRoute.
	Connected bool

	// OutboundRoute reports that WE opened a connection to this peer in
	// this process, i.e. that we can actually dispatch to them.
	//
	// # Why this is a separate field and not a nicer word for Connected
	//
	// A connection has a direction for AUTHORITY and none for display. A
	// dial-by-address authorizes the DIALER only (AP63), so deliveries
	// from us to them ride the connection WE opened — and the pool holds
	// theirs and ours indistinguishably. An INBOUND-ONLY session (they
	// dialled us, we never dialled them) therefore renders as plain
	// "connected" while nothing we write can leave the machine.
	//
	// That is not a rare corner. It is the most likely half-broken state
	// there is, because a dial-by-address is asymmetric by design, and it
	// is what an operator hit on 2026-09-08: the panel said connected for
	// 45 minutes while the run log correctly said the outbound connection
	// had never come up. **The panel and the log contradicted each other
	// and the reassuring one was on screen.**
	//
	// So this is the same discipline the Sharing Status panel already
	// applies to inbound authority — state what is knowable, never draw a
	// health dot over what is not — moved one field across. A surface
	// rendering Connected without this one is reporting a fact it does
	// not have.
	//
	// Sourced from the POOL, via core-go's Connection.IsOutbound() (their
	// tracker row 13, adopted 2026-09-17). It used to be sourced from a set
	// of peers we remembered dialling, because Connections() concatenated
	// both directions without tagging them and IsConnected() conflates the
	// pool with the §6.11 reentry map — a workaround that was wrong in one
	// direction, since "we dialled them" stays true after the connection
	// drops and this field then asserted a route that no longer existed.
	OutboundRoute bool

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
	// SyncingWith is every peer we currently hold a subscription to for
	// this folder. For a received folder that is at most its origin and
	// Syncing says the same thing; for a folder we OWN and have set to
	// `both`, it is the peers whose changes we pull back, and there is
	// no single origin for Syncing to describe.
	//
	// Both fields, because Syncing is what every existing surface reads
	// and silently redefining it to mean "any peer" would make a
	// send-only folder shared with four peers report as syncing.
	SyncingWith []string
	// ReceiveFrom is every peer whose changes this folder is SUPPOSED to
	// pull — the declaration's side of the question SyncingWith answers
	// with observation.
	//
	// The two exist as a pair on purpose, and the pair is the whole point
	// of this type. A surface holding only the declaration renders what
	// the operator ASKED for and calls it the outcome; that is the defect
	// an operator reported on 2026-09-10 as *"showing two-way, showing no
	// problem, meanwhile the whole files aren't getting delivered."*
	// Carrying both makes the gap between them computable, which is what
	// problems() reports and what no renderer should have to derive.
	ReceiveFrom []string
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
	// Mode is send / receive / both, ALWAYS the EFFECTIVE mode and never
	// the raw field.
	//
	// An absent Mode means the pre-S6 behaviour — a local folder
	// publishes — and never `both`. Carrying the raw field here forced
	// every renderer to default it, and the Avalonia row defaulted a
	// blank to `"both"`: a pre-S6 record drew `↔ two-way` on screen while
	// the reconciler treated it as send-only. **A default that lives in a
	// renderer is a second answer to a question the model already
	// answers**, and the second answer is the one nobody tests.
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

	// OwnerRuleKnown reports whether this peer holds the OWNER's
	// reconciliation rule for this folder — what happens when their
	// change lands on a local edit.
	//
	// A shared folder names ONE rule and it is the owner's, so for a
	// RECEIVED folder this is hearsay we have to have obtained
	// (workbench.ObservedFolderData). Always true for a folder we own,
	// where our own declaration is the rule and there is nobody to ask.
	//
	// It is observed from LOCAL state — do we hold the observation — so a
	// read can report it without dialing anybody, which is why it lives
	// here and not only in the pass. When it is false a collision is HELD
	// rather than resolved, and an operator whose file has not arrived
	// needs that sentence from whichever surface they happen to have
	// open.
	OwnerRuleKnown bool

	// OwnerConflictPolicy is the rule that will actually be applied,
	// resolved through the same defaulting both sides use. Empty when
	// OwnerRuleKnown is false — and a renderer must not print a default
	// there, because "record" and "we do not know" are the two states
	// this field exists to keep apart.
	OwnerConflictPolicy string

	// RollbackWitness is how this folder's INCOMING leg is defended
	// against a stale delivery arriving after a newer one — and today,
	// for every folder, the answer is [WitnessNotSupported].
	//
	// **It is a stated property, not a problem, and the distinction is
	// the design.** Nothing is wrong with a folder that reports this;
	// every folder reports it, because the subscription-delivered leg
	// carries no ordering quantity at all. Putting it in `problems()`
	// would add one permanent line to every receiving folder, which is
	// how an operator learns to skip the problems list.
	//
	// It exists because the architecture seat ruled (`A-33`, 2026-09-12)
	// that where a leg carries no witness the receiving implementation
	// **MUST** report it as `not_supported` and **MUST NOT** present the
	// leg as rollback-protected. We were already meeting the second half
	// by saying nothing — and *saying nothing* is exactly how a reader
	// concludes a leg is fine. The static leg next door DOES refuse a
	// rollback (`fetch.ErrSeqRollback`), and a product with one defended
	// leg and one silent leg reads as a product with two defended legs.
	//
	// Empty for a folder with no incoming leg: a send-only folder has
	// nothing arriving, and reporting an undefended leg it does not have
	// is a different kind of wrong answer.
	RollbackWitness string
}

// WitnessNotSupported is the `A-33` value: this leg carries no quantity
// a receiver could order deliveries by.
//
// Spelled exactly as the ruling spells it. **It is deliberately not a
// bool** — `RollbackProtected: false` is a field a surface forgets to
// read and renders as absence, and absence is indistinguishable from
// "fine" (AP49's shape). A string that says `not_supported` cannot be
// rendered as nothing by accident.
//
// The fix is NOT ours: a witness on this leg is a subscription-tier
// change (arch's `BY-6`). Do not synthesize one locally — `modified_at`
// is minted by the filesystem rather than by the writer, so a floor over
// it refuses a restored backup forever while admitting the replay it was
// built for. Measured and ruled; see
// `shellboot/sync_rollback_probe_test.go`.
const WitnessNotSupported = "not_supported"

// RollbackWitnessNote is the one sentence every surface prints for it,
// and the empty string where there is no incoming leg to describe.
//
// One writer, for `problems()`' reason: two copies of a diagnostic drift
// the moment one of them is improved, and this one crosses cgo into a
// panel as well as into the shell.
func (fs FolderStatus) RollbackWitnessNote() string {
	if fs.RollbackWitness != WitnessNotSupported {
		return ""
	}
	return "rollback witness: not_supported — a delivery carries no ordering " +
		"quantity, so a stale one arriving after a newer one is applied. This leg " +
		"is NOT rollback-protected (the static publish leg is)."
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

	ws.noteSaturation(&out)
	ws.noteHistoryLimits(&out)
	ws.noteConflicts(&out)

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
	desired := desiredGrantsByPeer(local.PeerID(), folders, ws.publicSiteGrants())
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
	// entitysdk.CandidatePeerID, NEVER cand.PeerID. That field is empty on
	// every mDNS candidate this product has ever seen (§2.1: null until
	// IDENTIFY, and nothing calls PromoteSuccessor), so the `cand.PeerID
	// != ""` guard this line used to carry made the entire address refresh
	// unreachable — for every peer, on every pass, silently. See
	// CandidatePeerID's own comment for the measurement and for why
	// trusting the claim is correct at this one decision.
	seen := map[string]string{}
	for _, cand := range local.ReadDiscoveredCandidates() {
		pid := entitysdk.CandidatePeerID(cand)
		if pid == "" {
			continue
		}
		if addr := entitysdk.DialAddressForCandidate(cand); addr != "" {
			seen[pid] = addr
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
//
// # publicSite, and the reason a named peer needs it spelled out again
//
// **The kernel's policy resolution is FIRST MATCH WINS, not a union.**
// `readHandshakePolicyGrants` tries `hex(identityHash)`, then the Base58
// peer-id, then `default`, and RETURNS AT THE FIRST ONE IT FINDS. So a
// peer with a row of its own never reaches `default` — and the
// consequence is the opposite of intuition: **publishing a site publicly
// makes it readable by every peer in the world EXCEPT the ones you have
// shared a folder with.**
//
// That is the worst available shape of this bug. The peer most likely to
// be used to test a newly-published site is the other machine already
// paired with this one, so the feature presents as broken to the only
// reader an operator has, while working for everybody they cannot ask.
//
// So the site grants are unioned into every derived per-peer row. Note
// what is NOT done: an arbitrary hand-written `default` row is left
// alone. `default` means "peers not otherwise named" in the kernel, and
// redefining it as "everyone" for a row whose intent we cannot read
// would be widening somebody else's grant on their behalf. Only OUR
// public site grant is propagated, because "this site is public" is a
// statement about everyone and we know it is, having written it.
func desiredGrantsByPeer(selfPeerID string, folders []workbench.FolderData, publicSite []types.GrantEntry) map[string][]types.GrantEntry {
	// Peer -> the roots we publish TO them. A set of roots and not a bool,
	// because the sender grant is now scoped to exactly those folders
	// (workbench.SyncSenderGrants): a peer we share two folders with must
	// be authorized for both, and a peer we share one with must not be
	// authorized for the rest of the tree.
	needSender := map[string][]workbench.SharedScope{}
	needReceiver := map[string]bool{}

	// Direction comes from Mode, NOT from Origin. Before S6 this loop
	// branched on IsLocal(), which is "who created the folder" — an
	// immutable, binary fact standing in for a per-peer, three-valued
	// one. That is why `both` was inexpressible and why a bidirectional
	// share had to be two unrelated folders (workbench.FolderData.Mode).
	for _, f := range folders {
		for _, p := range f.SharedWith {
			if p.PeerID == "" || p.PeerID == selfPeerID {
				continue
			}
			// We publish to them: they need the sender grants so they can
			// subscribe and pull the blob closure.
			//
			// `offered` counts, not only `accepted`: the grant is what
			// makes accepting POSSIBLE. Waiting for their acceptance
			// before authorizing it is the deadlock the nine-step flow
			// worked around by making the operator press things in a
			// particular order on two machines.
			if f.Publishes() {
				switch p.State {
				case workbench.FolderStateOffered, workbench.FolderStateAccepted:
					needSender[p.PeerID] = appendScope(needSender[p.PeerID],
						workbench.SharedScope{LocalRoot: f.ReceivingRoot(), FolderID: f.ID})
				}
			}
			// We receive from them: they need the receiver grant so their
			// deliveries reach our blob-resolve handler.
			//
			// An offer we have not accepted must not grant a stranger a
			// handler — so on a folder somebody else originated, ACCEPTED
			// only. That is a property of OUR decision, which is why it is
			// asserted here rather than inherited from an Origin branch.
			//
			// On a folder WE own, `offered` is our own act of sharing and
			// carries the same weight as `accepted` would: we declared
			// `both`, we chose the peer, and their acceptance is recorded
			// in their tree with nothing to carry it back (see
			// receiveFromPeers for the full argument). Withholding the
			// receiver grant here would let the owner subscribe to the
			// receiver's copy and then 403 every delivery it pulled — the
			// half-built reverse leg, which is worse than none because it
			// fails at the last hop with a permission error that reads as
			// a security problem.
			if f.Receives() {
				switch {
				case p.State == workbench.FolderStateAccepted:
					needReceiver[p.PeerID] = true
				case f.IsLocal() && p.State == workbench.FolderStateOffered:
					needReceiver[p.PeerID] = true
				}
			}
		}
	}

	out := map[string][]types.GrantEntry{}
	for peerID, scopes := range needSender {
		// Sorted so the grant set is a pure function of the declarations.
		// Map iteration order is random, and an unstable Resources list
		// changes the row's content hash on every pass — which
		// writePolicyIfChanged reads as a change, which forces a
		// re-handshake, which turns the loop into an outage generator.
		sort.Slice(scopes, func(i, j int) bool {
			if scopes[i].LocalRoot != scopes[j].LocalRoot {
				return scopes[i].LocalRoot < scopes[j].LocalRoot
			}
			return scopes[i].FolderID < scopes[j].FolderID
		})
		out[peerID] = append(out[peerID], workbench.SyncSenderGrants(scopes)...)
	}
	for peerID := range needReceiver {
		out[peerID] = append(out[peerID], workbench.SyncReceiverGrants()...)
	}
	// Appended last and to every row, so the set stays a pure function of
	// its inputs — the row's content hash decides whether a re-handshake
	// happens, and an unstable ordering here is an outage generator.
	for peerID := range out {
		out[peerID] = append(out[peerID], publicSite...)
	}
	return out
}

// publicSiteGrantsForPeers is the public site grant, or nil when this
// peer is not serving one.
//
// Nil rather than an empty slice on purpose: it is appended to every
// derived row, and the difference between "no public site" and "a public
// site granting nothing" is the difference between leaving a row alone
// and rewriting it.
func (ws *ShellWorkspace) publicSiteGrants() []types.GrantEntry {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil
	}
	row, ok := workbench.LoadAccessPolicy(ws.Local.Peer.Store(), workbench.PublicPolicyPeer)
	if !ok || !workbench.IsPublicSiteGrant(row.Grants) {
		return nil
	}
	return row.Grants
}

// appendScope adds v to s when it is not already there.
//
// The same folder can name the same peer more than once across
// declarations, and a duplicate would duplicate every resource pattern
// derived from it — harmless to the capability check and not harmless to
// the row's content hash, which is what decides whether a re-handshake
// happens.
func appendScope(s []workbench.SharedScope, v workbench.SharedScope) []workbench.SharedScope {
	if v.LocalRoot == "" {
		return s
	}
	for _, e := range s {
		if e == v {
			return s
		}
	}
	return append(s, v)
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
	return desiredGrantsByPeer(local.PeerID(), folders, ws.publicSiteGrants())[peerID], local.Store(), nil
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

// hasOutboundConnection reports whether the pool currently holds a
// connection WE opened to this peer.
//
// # Why this replaced a set of peers we remember having dialled
//
// The defect it was written for is real and is worth keeping in view.
// Measured 2026-09-03 with two real peers: share a folder, accept it, watch
// files flow — then restart the SENDING peer. Its writes stop reaching the
// receiver, and `status` reports **"settled — everything declared is
// established."** A manual `connect` on the sender restores delivery
// instantly. The cause is that **a connection has a direction for authority
// and none for display**: a dial-by-address authorizes the DIALER only
// (AP63), so delivery from us to them runs over the connection WE opened,
// and after a restart we have opened none — while the receiver's own
// connection sits in our pool making `ConnectedPeers()` say "connected".
//
// The workaround was a process-scoped set of peers we had dialled, because
// core-go's `Connections()` concatenated both directions without tagging
// them and `IsConnected()` conflated the pool with the §6.11 reentry map.
// We routed that (`docs/outbox/CONNECTION-DIRECTION-AND-BILATERAL-REACH-2026-09-09.md`)
// and **core-go answered it**: `Connection.IsOutbound()` / `Direction()`,
// recorded once in `PerformConnect`, dialer-only (their tracker row 13).
//
// ⭐ **Adopting it is not tidying — the workaround was wrong in one
// direction.** *"We dialled this peer"* and *"we have a connection to this
// peer"* are different claims, and they diverge exactly when the outbound
// connection drops mid-session: the remembered fact stays true forever, so
// `OutboundRoute` went on reporting a route that no longer existed. That is
// the same false-reassurance this whole field exists to prevent, one layer
// in — a surface stating a fact it does not have. The pool answers the
// question that was always being asked.
func (ws *ShellWorkspace) hasOutboundConnection(peerID string) bool {
	if peerID == "" || ws.Local.Peer == nil {
		return false
	}
	return ws.Local.Peer.HasOutboundConnection(peerID)
}

// markDialFailed records that a dial to this peer has already failed in
// this process, so a pass does not re-walk the whole address ladder at ten
// seconds an address for a peer that is simply switched off.
//
// ⚠ It records FAILURE only, and that is the half that changed when
// `IsOutbound()` was adopted. The old set recorded success too, which meant
// a dropped outbound connection was never re-dialled — the loop knew the
// route was gone (once the display was honest) and declined to act on it.
// Now the pool answers *do we have a route*, this answers *is it worth
// trying again right now*, and a connection that drops is re-established by
// the next pass instead of waiting for an operator to type `connect`.
//
// Not persisted, deliberately: the whole point is that it expires with the
// process.
func (ws *ShellWorkspace) markDialFailed(peerID string) {
	if ws.dialFailedThisProcess == nil {
		ws.dialFailedThisProcess = map[string]bool{}
	}
	ws.dialFailedThisProcess[peerID] = true
}

func (ws *ShellWorkspace) hasDialFailedThisProcess(peerID string) bool {
	return ws.dialFailedThisProcess[peerID]
}

// ensureOutboundRoute opens OUR connection to a peer when the pool does not
// already hold one.
//
// Reports whether it dialled and what to say about it. A failure is not
// an error to unwind anything for: the declaration is right, the peer is
// simply not there, and a later process tries again.
func (ws *ShellWorkspace) ensureOutboundRoute(ctx context.Context, d workbench.DeviceData, st *DeviceStatus, out *ReconcileOutcome) {
	// Ask the pool, not our memory. Reconnecting on every pass would make
	// the loop an outage generator — which is what this guard is for — but
	// the condition that justifies skipping is *a live outbound connection
	// exists*, and until core-go's row 13 there was no way to ask.
	if ws.hasOutboundConnection(d.PeerID) {
		st.OutboundRoute = true
		return
	}
	if ws.hasDialFailedThisProcess(d.PeerID) {
		return
	}
	addrs := ws.dialLadderFor(d)
	if len(addrs) == 0 {
		// Nothing to dial. Named rather than silently skipped: for a
		// folder we PUBLISH, this is the difference between working and
		// not, and it reads as "connected" everywhere else.
		st.Note = "no address has ever been recorded for this peer, and it is not " +
			"announcing one on this network — run `connect <alias> <host:port>`, " +
			"or have them dial us"
		return
	}

	// Try EVERY address we know, not just the preferred one.
	//
	// A remembered address is a hypothesis about where a peer is; a live
	// announcement is an observation. Dialling only the stored one is what
	// let an operator's app spend 45 minutes refusing at a port the peer
	// had moved off, while that peer sat on the LAN announcing its real
	// one. Walking the ladder is also what makes trusting the mDNS claim
	// safe: a spoofed announcement costs one failed handshake and we fall
	// through to the address that works, rather than replacing it.
	var tried []string
	for _, addr := range addrs {
		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := ws.Local.Peer.Connect(dialCtx, addr)
		cancel()
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s (%v)", addr, err))
			continue
		}
		// Write the WORKING address back, so the next process starts from
		// a fact rather than from the guess that just failed. This is the
		// half that makes the correction durable: without it the ladder
		// re-derives the same answer every launch and the declaration
		// stays wrong forever.
		//
		// Deliberately NOT ws.rememberAddress: that invents a shell alias
		// from the peer-id when none exists, and the loop doing so made
		// the operator's own `connect b <addr>` fail with *"already bound
		// to alias 2k7u4nx2"*. RememberDeviceAddress updates the
		// DECLARATION only, which is where a durable fact belongs.
		if addr != d.PreferredAddress() {
			ws.RememberDeviceAddress(d.PeerID, addr)
			out.Actions = append(out.Actions, fmt.Sprintf(
				"%s: %s answered where %s did not — recorded it as the address to use",
				st.Label, addr, d.PreferredAddress()))
		}
		// maintain-peer needs the address that actually answered, not the
		// one observeDevice read out of the declaration a moment ago.
		if hp := dialHostPort(addr); hp != "" {
			st.Address = hp
		}
		st.Connected = true
		st.OutboundRoute = true
		out.Actions = append(out.Actions,
			fmt.Sprintf("%s: opened our outbound connection to %s so deliveries can reach them",
				st.Label, addr))
		return
	}

	// Every address failed. Record it so the next pass in this process does
	// not spend ten seconds an address re-establishing the same answer.
	ws.markDialFailed(d.PeerID)
	st.Note = "could not dial " + strings.Join(tried, "; ")
}

// directionProblem is the ONE writer of the sentence an operator needs
// when we cannot reach a peer, shared by the pass and the read.
//
// It is a method on the status rather than a line appended at the point
// of failure because the pass and the panel were describing this state
// differently — the pass said "could not dial X" into a run log, and the
// panel said "connected". A single function means the two surfaces cannot
// drift, and it means the READ can report the state even though a read
// never dials: the observation "a session exists and none of it is ours"
// needs no attempt to make it.
//
// `publishes` changes the WORDING and never the decision, and the
// difference was a real error while this was being written.
//
// The first version reported nothing unless we published a folder to the
// peer, on the reasoning that a folder we only RECEIVE needs their dial
// and not ours. **That is wrong, and AP63 already says so: a sync is
// MUTUAL.** The receiver dispatches out to subscribe and to pull the blob
// closure — the backfill runs entirely on authority the receiver holds —
// and the publisher dispatches back to deliver. So a peer we cannot reach
// breaks a folder we receive just as completely as one we publish; it
// simply breaks it in a way that looks like "nothing is arriving" rather
// than "nothing is being sent". Both need saying.
//
// What `publishes` buys is naming the consequence the operator will
// actually observe, because those two symptoms send someone to look in
// opposite places.
func (st DeviceStatus) directionProblem(publishes bool) string {
	if st.Paused || st.OutboundRoute {
		return ""
	}
	detail := ""
	if st.Note != "" {
		detail = " — " + st.Note
	}
	consequence := "we cannot fetch from them, so a folder we receive will stop updating"
	if publishes {
		consequence = "nothing we write to a shared folder will reach them"
	}
	if st.Connected {
		// The exact state that cost an operator 45 minutes: the pool says
		// connected because THEY dialled US, and a connection they opened
		// authorizes them, not us (AP63).
		return fmt.Sprintf(
			"%s: they can reach us, but we could not open our own connection to them — %s%s",
			st.Label, consequence, detail)
	}
	return fmt.Sprintf(
		"%s: could not open our own connection to this peer — %s%s",
		st.Label, consequence, detail)
}

// dialLadderFor is every address worth trying for a peer, best first, and
// it is the one place the discovery-vs-declaration precedence is decided.
//
// **Discovery first.** The stored address is the only source that survives
// a restart, which is why it exists — but "durable" is not "authoritative".
// A candidate in the discovery set was observed within
// DiscoveryCandidateMaxAge (15s), so it is a live statement about where the
// peer is listening right now, whereas the declaration may be months old.
// The declaration follows immediately behind, so a peer that is asleep, or
// on a LAN with no multicast, still gets dialled at the address that
// worked last time.
//
// Nothing here is trusted on its own: the dial authenticates against the
// peer-id we already declared, so the ladder can only ever waste a dial.
func (ws *ShellWorkspace) dialLadderFor(d workbench.DeviceData) []string {
	var out []string
	seen := map[string]bool{}
	add := func(addr string) {
		addr = strings.TrimSpace(addr)
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		out = append(out, addr)
	}
	add(ws.discoveredAddressFor(d.PeerID))
	for _, a := range d.Addresses {
		add(a)
	}
	return out
}

// publishesTo reports whether any declared folder is shared OUT to this
// peer — i.e. whether we ever need to dispatch to them.
func (ws *ShellWorkspace) publishesTo(peerID string) bool {
	folders, _ := workbench.LoadFolders(ws.Local.Peer.Store())
	for _, f := range folders {
		// Publishes(), not IsLocal(): a folder we ACCEPTED from them and
		// set to `both` also dispatches to them, and under the old test
		// it silently did not — the outbound route was never opened, so
		// our writes went nowhere with nothing reporting a fault.
		if !f.Publishes() {
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

	// Read the direction problem HERE, before maintain-peer overwrites
	// st.Note with its own failure. The two are different faults — "we
	// have no route to them" and "the reconnect graph did not install" —
	// and the second is a consequence of the first often enough that
	// letting it overwrite the cause is how the useful sentence gets lost.
	if p := st.directionProblem(ws.publishesTo(d.PeerID)); p != "" {
		out.Problems = append(out.Problems, p)
	}

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
	// Read the OWNER's reconciliation rule BEFORE observing, and before
	// any of the early returns below. A shared folder names ONE rule and
	// it is the owner's, and the handler that needs it at delivery time
	// cannot go and ask (remote_declaration.go).
	//
	// Before observeFolder because the pass may have just obtained it,
	// and a status computed first would report a gap this pass closed.
	// Above the accepted/mounted checks because whether we have heard the
	// owner's rule has nothing to do with whether our own mount is
	// healthy — and putting it below them would mean a folder that
	// becomes receivable later starts receiving before it has ever
	// learned the rule, which is precisely the window a collision is held
	// in.
	ownerRuleNote := ws.recordOwnerConflictRule(f)

	fs := ws.observeFolder(f)

	// The standing sentence is problems()'s, so the read and the pass
	// cannot describe this state differently. What the PASS additionally
	// knows is WHY the read failed just now, which no local observation
	// can supply — so it goes in the Note, which is where this type
	// already carries pass-specific detail.
	if ownerRuleNote != "" {
		fs.Note = ownerRuleNote
	}

	if f.IsLocal() {
		if !fs.Mounted {
			// Named, not fixed. Re-creating a mount means writing to a
			// path on somebody's disk that this record only remembers.
			fs.Note = "no local mount for root " + f.Root + " — nothing is published from this folder"
			out.Problems = append(out.Problems, fs.problems()...)
			return fs
		}
	} else {
		// A received folder. The mount is where the bytes land; without
		// it every delivery answers 404 no_mount_for_uri while the sync
		// row still lists as healthy, which is the exact failure this
		// product shipped twice.
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
	}

	if !f.Receives() {
		// Mounted and set to send-only: we publish our side and take
		// nothing back. Said out loud, because an operator looking at a
		// folder that is established and quiet needs to be able to tell
		// "configured that way" from "broken".
		fs.Note = "send-only — their changes are not applied here"
		return fs
	}

	// Turn on the chain BEFORE opening the subscription, because a
	// delivery that lands between the two is exactly the one nobody can
	// recover. See folder_history.go for why this is derived state and
	// not something `mount` writes.
	ws.reconcileFolderHistory(f, fs, out)

	// Subscribe to everyone we receive from. This IS establishable
	// without a human decision: the mount exists and the operator
	// already said yes, so subscribing carries out their decision
	// rather than making one.
	//
	// The loop is over PEERS rather than over the single origin because
	// a folder we own and set to `both` pulls from every peer that
	// accepted it, and it has no origin to key that on. A received
	// folder yields exactly its origin, so the previous behaviour is the
	// one-element case of this one.
	for _, peerID := range receiveFromPeers(f, ws.Local.Peer.PeerID()) {
		// WHICH ROOT ON THEIR MACHINE. For a folder we RECEIVED, the
		// source is the origin's own root and `f.Root` already holds it.
		// For the reverse leg of a folder we OWN, their copy is mounted
		// under whatever directory THEY named at `accept`, which is a fact
		// in THEIR tree — so we go and read it (remote_declaration.go).
		//
		// This used to be a KNOWN LIMIT comment saying the two names had
		// to agree. They do not have to agree: `accept` takes a directory
		// precisely so a receiver is never told a name out of band, and a
		// reverse leg that only worked when the names happened to collide
		// had not met that requirement. Measured in
		// shellboot/mode_both_asymmetric_roots_test.go, which fails with
		// the owner's root and passes with theirs, forward leg green in
		// both arms.
		sourceRoot := f.Root
		if f.IsLocal() {
			remoteRoot, ok, reason := ws.remoteRootForReverseLeg(f, peerID)
			if !ok {
				// NAMED, NOT GUESSED. Falling back to our own root here is
				// what produced a durable subscription to a prefix that
				// does not exist on the far side — accepted, healthy,
				// permanently empty. Saying so and retrying next pass is
				// strictly better than a wrong answer that persists.
				fs.Note = reason
				out.Problems = append(out.Problems,
					fmt.Sprintf("folder %q, receiving from %s: %s", fs.Label, peerID, reason))
				continue
			}
			sourceRoot = remoteRoot
		}

		// Idempotence is keyed on the SOURCE root, which is what the
		// binding itself is keyed on (workbench.SyncBindingKey). Checking
		// `f.Root` here would miss an established asymmetric leg every
		// pass and re-subscribe forever.
		if _, ok := workbench.LoadSyncBinding(ws.Local.Peer.Store(), peerID, sourceRoot); ok {
			continue
		}
		if _, err := ws.Sync(SyncRequest{
			Remote: peerID, Root: sourceRoot, TargetRoot: fs.LocalRoot,
		}); err != nil {
			fs.Note = "could not subscribe: " + err.Error()
			out.Problems = append(out.Problems,
				fmt.Sprintf("folder %q from %s: %v", fs.Label, peerID, err))
			continue
		}
		fs.SyncingWith = append(fs.SyncingWith, peerID)
		if peerID == f.Origin {
			fs.Syncing = true
		}
		out.Actions = append(out.Actions,
			fmt.Sprintf("folder %q: subscribed to %s and caught up", fs.Label, peerID))
	}
	sort.Strings(fs.SyncingWith)
	return fs
}

// receiveFromPeers is every peer whose changes to this folder we pull.
//
// Empty when the folder does not receive at all, so a caller does not
// have to check Receives() as well — the two answers cannot then
// disagree, which is the failure mode that put a subscription on a
// send-only folder in the first place.
//
// For a folder we RECEIVED it is the origin, and only when we accepted
// it. For one we OWN it is every peer that accepted it: an offer they
// have not taken up authorizes nothing and must not open a subscription
// to a peer who never agreed to publish to us.
func receiveFromPeers(f workbench.FolderData, selfPeerID string) []string {
	if !f.Receives() {
		return nil
	}
	if !f.IsLocal() {
		if ps, ok := f.PeerState(f.Origin); ok && ps.State == workbench.FolderStateAccepted {
			return []string{f.Origin}
		}
		return nil
	}
	var out []string
	for _, p := range f.SharedWith {
		if p.PeerID == "" || p.PeerID == selfPeerID {
			continue
		}
		// `offered` counts HERE and only here, and the reason is that the
		// word means two different things on the two branches above.
		//
		// On a RECEIVED folder (the branch above) `offered` is *they
		// offered and we have not accepted* — a stranger's proposal, and
		// gating on it is right.
		//
		// On a LOCAL folder it is *WE offered it to them*: our own act,
		// recorded by `declareLocalShare`. Whether they went on to accept
		// is written by `declareAcceptedFolder` in THEIR tree and there is
		// no channel that carries it back — so requiring `accepted` here
		// was requiring a fact that structurally cannot arrive, and
		// `Mode: both` could never run its reverse leg on any pair of
		// peers, ever. Measured on a live pair 2026-09-10: the owner's
		// record said `offered` for a receiver that had accepted hours
		// earlier and was receiving files fine in the forward direction.
		//
		// This is NOT branching on Origin for DIRECTION — direction is
		// still `f.Receives()`, checked at the top. It is reading a state
		// field whose meaning genuinely depends on which side wrote it,
		// which is why this function already had the two branches.
		//
		// The asymmetry mirrors `desiredGrantsByPeer`, which has admitted
		// `offered` on the publish side since S6 for the same reason: a
		// declaration about our own intent must not wait on a round trip
		// that does not exist.
		//
		// Cost when they never accepted: we subscribe to a peer that has
		// no mount for this root and nothing arrives. That is the same
		// no-op as an accepted peer with an empty folder.
		switch p.State {
		case workbench.FolderStateAccepted, workbench.FolderStateOffered:
			out = append(out, p.PeerID)
		}
	}
	sort.Strings(out)
	return out
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
