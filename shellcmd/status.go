package shellcmd

// status.go — reading what is declared and what is actually true,
// WITHOUT running the loop.
//
// # Why this exists beside Reconcile rather than inside it
//
// `cmd_reconcile.go` argues, correctly for a shell, that a read-only
// status verb has the same blind spot as the five verbs it replaced: it
// reads records, and every restart defect this product shipped was a case
// of the records being intact while the derived runtime state was gone.
// So `status` runs the loop, and the reading is true *because* a pass made
// it true.
//
// A GUI panel cannot copy that. **A reconcile pass dials** — it calls
// maintain-peer for every declared device — so a panel that reconciled on
// every refresh or every wake would be a control loop on a UI timer, and
// an operator who left it open would be dialing every declared peer
// forever. That is not a status surface; it is a dialer with a table in
// it.
//
// Hence two entry points with one shape:
//
//   - [ShellWorkspace.StatusSnapshot] — reads declarations and observes the
//     substrate. No write, no dial, no reconnect. Safe on a wake, safe to
//     call repeatedly, and it reports `Reconciled: false` so a surface can
//     say what it is: a reading, not a verification.
//   - [ShellWorkspace.Reconcile] — one pass. Explicit: on open, on a
//     re-check, after a mutation. Reports `Reconciled: true`.
//
// Both return the same [ReconcileOutcome], so a renderer draws one thing
// and the only difference on screen is whether the pass ran. Getting that
// distinction into the OUTCOME rather than into the caller's memory is
// deliberate: "this reading was verified by a pass" is a property of the
// reading, and a surface that has to remember which function it called in
// order to caption the table will eventually caption it wrong.
//
// # What is knowable, and what this file refuses to claim
//
// **Outbound authority is exactly knowable.** What we grant a peer is our
// own `system/capability/policy/{peer}` row; we wrote it, so
// [DeviceStatus.OutboundGrant] is a fact.
//
// **Inbound authority is NOT knowable.** Whether *they* have authorized
// *us* lives in their capability table, which we cannot read. It is only
// ever OBSERVED — through deliveries arriving, or through chain-errors.
// So nothing here renders inbound as a health state, and no field in this
// file says "they authorized us". What a received folder carries instead
// is a count of what has actually landed ([FolderStatus.FilesPresent]) and
// whether the subscription exists, which are both things this peer can
// check for itself.
//
// The distinction is not pedantry. A green dot for inbound authority would
// be wrong in exactly the case that matters — they revoked us and we have
// not tried since — and it would be wrong while looking authoritative,
// which is the failure mode AP45 names.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// StatusSnapshot reads declared state and observes the substrate without
// running the control loop.
//
// It writes nothing, dials nothing and reconnects nobody, so it is safe
// from a wake, from a timer, and from a panel that refreshes while an
// operator watches it. The returned outcome has `Reconciled` false and
// never carries Actions — a read cannot have changed anything, and an
// Actions list that a read could populate would make the field mean two
// things.
//
// Problems here are strictly *observations*: a record that will not
// decode, a folder with no mount, an accepted folder with no
// subscription. They are the same sentences a pass would produce for the
// same state, because they describe the state rather than the pass.
func (ws *ShellWorkspace) StatusSnapshot() (ReconcileOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ReconcileOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	st := local.Store()
	out := ReconcileOutcome{}

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

	// The maintained set is a LOCAL dispatch — `system/network:status`
	// against our own peer — so reading it costs no network and cannot
	// block on another machine. It is the difference between "we
	// connected once" and "something is keeping this alive", and it is
	// the single field that tells a restarted app from a working one.
	maintained := ws.maintainedSet()

	for _, d := range devices {
		ds := ws.observeDevice(d)
		ds.Maintained = maintained[d.PeerID]
		// Direction BEFORE the maintenance note, for reconcileDevice's
		// reason: the useful sentence is the one about the route, and the
		// maintenance note would otherwise be the only thing an operator
		// reads for a peer that is half-connected.
		if p := ds.directionProblem(ws.publishesTo(d.PeerID)); p != "" {
			out.Problems = append(out.Problems, p)
		}
		if !ds.Maintained && !d.Paused {
			// Named rather than left blank: a device with no maintenance
			// session is not being reconnected by anything, and after a
			// restart that is the normal state until a pass runs. The
			// remedy is the Re-check button, so say so.
			ds.Note = "no maintenance session — nothing is reconnecting this peer until a re-check runs"
		}
		out.Devices = append(out.Devices, ds)
	}

	for _, f := range folders {
		fs := ws.observeFolder(f)
		out.Problems = append(out.Problems, fs.problems()...)
		out.Folders = append(out.Folders, fs)
	}

	sort.Strings(out.Problems)
	return out, nil
}

// maintainedSet is the peers the kernel currently holds a maintenance
// session for, keyed by peer-id.
//
// A failure to read it is deliberately NOT a problem line. The network
// status op is local, so a failure means something is wrong with this
// peer's own dispatch rather than with any declared relationship — and
// reporting it per-device would print the same internal error once per
// row while attributing it to peers that have nothing to do with it. The
// devices simply report `Maintained: false`, which is the honest answer
// when we could not establish otherwise.
func (ws *ShellWorkspace) maintainedSet() map[string]bool {
	out := map[string]bool{}
	nc := ws.Local.Peer.Network()
	if nc == nil {
		return out
	}
	rows, err := nc.MaintainedPeers(context.Background())
	if err != nil {
		return out
	}
	for _, r := range rows {
		out[r.PeerID] = true
	}
	return out
}

// observeDevice builds the part of a device's status that is READ rather
// than established: identity, address, pool connectivity, and what our
// own policy row grants them.
//
// Shared with the reconcile pass so the two surfaces cannot describe the
// same device differently. `Maintained` is deliberately not set here —
// the two callers learn it by different means (a pass gets it from
// maintain-peer's own result; a snapshot reads the session list), and a
// helper that guessed would be wrong for one of them.
func (ws *ShellWorkspace) observeDevice(d workbench.DeviceData) DeviceStatus {
	local := ws.Local.Peer
	st := DeviceStatus{
		PeerID:         d.PeerID,
		Label:          d.ShortLabel(),
		Address:        dialHostPort(d.PreferredAddress()),
		Paused:         d.Paused,
		AddedAtMillis:  d.AddedAtMillis,
		LastSeenMillis: d.LastSeenMillis,
	}
	for _, c := range local.ConnectedPeers() {
		if c.PeerID == d.PeerID {
			st.Connected = true
			break
		}
	}
	// Direction, which the pool cannot express. See DeviceStatus.
	// OutboundRoute for why a bare Connected is not an answer.
	st.OutboundRoute = ws.hasOutboundConnection(d.PeerID)
	// Outbound authority, exactly knowable: this is our row, which we
	// wrote. "(nothing)" is a real answer and a different one from "we
	// could not look" — a peer we know and grant nothing is a peer whose
	// deliveries we refuse, and that is precisely the state an operator
	// is trying to see here.
	if pol, ok := workbench.LoadAccessPolicy(local.Store(), d.PeerID); ok {
		st.OutboundGrant = pol.Summary
		if st.OutboundGrant == "" {
			st.OutboundGrant = workbench.SummarizeGrants(pol.Grants)
		}
	} else {
		st.OutboundGrant = "(nothing)"
	}
	if d.Paused {
		st.Note = "paused — no connection is maintained until it is resumed"
	}
	return st
}

// observeFolder builds a folder's status from the declaration and the
// substrate, establishing nothing.
//
// Same sharing argument as observeDevice: the reconcile pass calls this
// first and then acts, so a folder cannot be described one way by the
// panel and another way by the shell.
func (ws *ShellWorkspace) observeFolder(f workbench.FolderData) FolderStatus {
	local := ws.Local.Peer
	st := local.Store()

	fs := FolderStatus{
		ID:        f.ID,
		Label:     f.DisplayLabel(),
		Local:     f.IsLocal(),
		Root:      f.Root,
		LocalRoot: f.ReceivingRoot(),
		Path:      f.Path,
		Origin:    f.Origin,
		// EffectiveMode, never the raw field. See FolderStatus.Mode.
		Mode: f.EffectiveMode(),
	}
	for _, p := range f.SharedWith {
		fs.PeerStates = append(fs.PeerStates, FolderPeerStatus{
			PeerID:   p.PeerID,
			Label:    ws.DeviceLabelFor(p.PeerID),
			State:    p.State,
			AtMillis: p.AtMillis,
			Note:     p.Note,
		})
	}
	sort.Slice(fs.PeerStates, func(i, j int) bool { return fs.PeerStates[i].PeerID < fs.PeerStates[j].PeerID })

	// ReceivingRoot(), never Root. The subscription is keyed on the
	// SENDER's name and the mount is named after the directory the
	// operator picked; checking Root would report "no mount" for every
	// folder received into a differently-named directory.
	fs.Mounted = hasLocalRoot(local, fs.LocalRoot)
	if !f.IsLocal() {
		_, fs.Syncing = workbench.LoadSyncBinding(st, f.Origin, f.Root)
		if ps, ok := f.PeerState(f.Origin); ok {
			fs.Accepted = ps.State == workbench.FolderStateAccepted
			fs.AcceptedAtMillis = ps.AtMillis
		}
	}
	// DECLARED, then OBSERVED, as two separate lists.
	fs.ReceiveFrom = receiveFromPeers(f, local.PeerID())
	sort.Strings(fs.ReceiveFrom)

	// Every peer we ACTUALLY pull from.
	//
	// **Matched on the binding's TARGET, never by looking one up under
	// our own root name.** The reverse-leg binding is keyed on the
	// SENDER's root — theirs — because that is the prefix being watched,
	// and the receiver's mount is named after whatever directory the
	// operator chose. `LoadSyncBinding(st, peerID, f.Root)` therefore
	// missed every working two-way folder between two machines that had
	// named the directory differently, and the row said "syncing with
	// nobody" while bytes were arriving.
	//
	// The target prefix is ours and is exactly what identifies the
	// binding as belonging to THIS folder, so this is a local read with
	// no guess in it. Gated by
	// TestStatus_ReverseLegIsSeenWhenTheirRootDiffers.
	//
	// The transferable rule: **a fix that changes what a record is keyed
	// on has to move every reader of that key, and the readers that
	// merely DISPLAY are the ones nobody thinks of.**
	want := "local/files/" + fs.LocalRoot + "/"
	bindings, _ := workbench.LoadSyncBindings(st)
	pulling := map[string]bool{}
	for _, b := range bindings {
		if b.TargetPrefix == want {
			pulling[b.RemotePeerID] = true
		}
	}
	for _, peerID := range fs.ReceiveFrom {
		if pulling[peerID] {
			fs.SyncingWith = append(fs.SyncingWith, peerID)
		}
	}
	sort.Strings(fs.SyncingWith)

	fs.FilesPresent, fs.FilesIngested, fs.FilesObservable = mountFileCounts(st, fs.LocalRoot)

	// WHOSE rule applies, and whether we hold it. A LOCAL read: the
	// question is what this peer knows, not what the other one currently
	// says, so a status panel can answer it without becoming a dialer.
	if owner := f.OwnerOf(local.PeerID()); owner == "" || owner == local.PeerID() {
		// We own it. Our declaration IS the subject's rule.
		fs.OwnerRuleKnown = true
		fs.OwnerConflictPolicy = f.ConflictPolicy()
	} else if o, heard := workbench.LoadObservedFolder(st, f.ID); heard {
		fs.OwnerRuleKnown = true
		fs.OwnerConflictPolicy = o.OwnerConflictPolicy()
	}

	// A-33: a leg with no witness says so. Only where there IS an
	// incoming leg — a folder we publish and never pull from has nothing
	// arriving, and describing its absent leg as undefended is a
	// confident answer to a question nobody asked.
	if !fs.Local || len(fs.ReceiveFrom) > 0 {
		fs.RollbackWitness = WitnessNotSupported
	}

	return fs
}

// problems renders the observations that need an operator, as the same
// sentences a reconcile pass produces for the same state.
//
// On the status type rather than in the snapshot loop because the
// reconcile pass produces these too, and two copies of a diagnostic
// sentence drift the moment one of them is improved.
func (fs FolderStatus) problems() []string {
	switch {
	case fs.Local && !fs.Mounted:
		return []string{fmt.Sprintf(
			"folder %q has no mount (expected root %q at %s) — remount it, or remove the folder",
			fs.Label, fs.Root, fs.Path)}
	case fs.Local:
		// A local folder that only PUBLISHES is fine once it is mounted.
		// One that also RECEIVES has a second half, and until 2026-09-10
		// this branch returned nil unconditionally — so an owner's folder
		// could not report a problem of any kind, however broken.
		//
		// That is what an operator hit: they asked for two-way, nothing
		// was established, and the row said `↔` and nothing else. The
		// missing leg is observable from local state — we declared which
		// peers we pull from, and we can see which subscriptions exist —
		// so a READ can say it without dialing anybody.
		return fs.missingReverseLegs()
	case !fs.Accepted:
		// A received folder we have not accepted is not supposed to have
		// anything established for it.
		return nil
	case !fs.Mounted:
		return []string{fmt.Sprintf(
			"folder %q from %s is accepted but has no mount (expected root %q at %s) — "+
				"every delivery will answer 404 until it exists",
			fs.Label, fs.Origin, fs.LocalRoot, fs.Path)}
	case !fs.Syncing:
		return []string{fmt.Sprintf(
			"folder %q from %s is accepted and mounted but has no subscription — "+
				"a re-check establishes it",
			fs.Label, fs.Origin)}
	case !fs.OwnerRuleKnown:
		// Established and receiving, and we do not hold the rule for what
		// happens when their change lands on an edit of yours. Deliveries
		// still arrive; a COLLISION is held rather than resolved, so this
		// is not "broken" and must not be worded as if it were.
		//
		// Said at all because the alternative is an operator watching one
		// file fail to arrive with every row green — the shape this whole
		// panel exists to stop.
		return []string{fmt.Sprintf(
			"folder %q from %s is established, but we have not been able to read their "+
				"rule for what happens when their change lands on one of your edits — "+
				"that rule is theirs, because a shared folder has one. Files still "+
				"arrive; a COLLISION is held until we can read it, and a re-check with "+
				"that peer reachable clears it",
			fs.Label, fs.Origin)}
	}
	return nil
}

// Problems is problems(), exported for a renderer.
//
// Exported rather than reimplemented on the other side of the bridge, and
// that is the whole reason it exists: the Avalonia folder row decided for
// itself when a folder was in trouble, and its rule — unmounted, or a
// received folder that is accepted and not syncing — meant a folder we
// OWN could not report anything at all. One writer for the sentence, read
// by the shell, by a reconcile pass and by every panel.
func (fs FolderStatus) Problems() []string { return fs.problems() }

// missingReverseLegs names every peer this folder is declared to pull
// from and is not pulling from.
//
// # Why a READ is allowed to say this
//
// Both halves are local facts. `ReceiveFrom` comes from our own
// declaration; `SyncingWith` comes from our own sync bindings. Nothing
// here dials, dispatches, or asks another machine anything — which is the
// constraint a status surface lives under, since a panel refreshes and a
// pass that dialled on every refresh would be a dialer with a table in it.
//
// # Why it does not say WHY
//
// The usual cause is that the other machine has not asked for two-way,
// and that is a fact in THEIR tree — a reconcile pass reads it and puts
// the specific sentence in `Note`. A read cannot, so it states the
// observation and names the action, rather than guessing at a cause it
// has not measured. *"Declared X, substrate lacks Y, press this"* is
// weaker than the pass's sentence and it is true, which is the trade a
// surface should always take.
func (fs FolderStatus) missingReverseLegs() []string {
	if len(fs.ReceiveFrom) == 0 {
		return nil
	}
	pulling := map[string]bool{}
	for _, p := range fs.SyncingWith {
		pulling[p] = true
	}
	var out []string
	for _, peerID := range fs.ReceiveFrom {
		if pulling[peerID] {
			continue
		}
		// The Note, when a pass has produced one, is the specific reason
		// and is strictly better than the generic sentence. Prefer it,
		// but never render NOTHING — silence here is the whole defect.
		if strings.TrimSpace(fs.Note) != "" {
			out = append(out, fmt.Sprintf("folder %q: %s", fs.Label, fs.Note))
			continue
		}
		out = append(out, fmt.Sprintf(
			"folder %q is set to %q but nothing is arriving from %s — no subscription "+
				"to their copy exists. Re-check to establish it; if it stays this way "+
				"they have not set this folder to two-way on their machine.",
			fs.Label, fs.Mode, peerID))
	}
	return out
}

// mountFileCounts reports both sides of the mount's lossy stage: how many
// files the mount admits (the SOURCE layer, one entity per file on disk)
// and how many of them have become documents at the target prefix.
//
// Both, never one (AP59). A mount of 400 photographs and one README has
// 401 in the source layer and one openable document, and a single count
// reads whichever layer cannot fail — which is how a folder that
// delivered nothing displayed a healthy number for a month.
//
// `observable` is false when there is no mount to count, so a renderer
// says *unknown* rather than printing a confident 0. Those are different
// claims and a surface that renders them identically is lying about one.
func mountFileCounts(st *workbench.Store, root string) (source, ingested int, observable bool) {
	if st == nil || root == "" {
		return 0, 0, false
	}
	ent, ok := st.Get(workbench.MountConfigPrefix + root)
	if !ok {
		return 0, 0, false
	}
	cfg, err := localfiles.RootConfigDataFromEntity(ent)
	if err != nil {
		return 0, 0, false
	}
	if cfg.Prefix != "" {
		source = len(st.List(cfg.Prefix))
	}
	if b, ok := workbench.LoadMountBinding(st, root); ok && b.TargetPrefix != "" {
		ingested = len(st.List(b.TargetPrefix))
	}
	return source, ingested, true
}

// SetDevicePaused pauses or resumes a declared device, and reports the
// record as it now stands.
//
// Pausing is a change to DESIRED state, so it is written to the
// declaration and nothing else: the reconciler stops maintaining the peer
// on its next pass because the record says so. A version that closed the
// connection directly would be a wizard step again — the substrate would
// change, the declaration would not, and the next pass would undo it.
// That is exactly how `unshare` silently reversed itself.
//
// It deliberately does NOT drop a live connection. Pausing means "stop
// keeping this alive", not "cut it now", and a peer that is mid-delivery
// when an operator pauses it should finish rather than fail. Forget is
// the verb for severing.
func (ws *ShellWorkspace) SetDevicePaused(peerID string, paused bool) (workbench.DeviceData, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return workbench.DeviceData{}, fmt.Errorf("workspace has no local peer")
	}
	st := ws.Local.Peer.Store()
	d, ok := workbench.LoadDevice(st, peerID)
	if !ok {
		return workbench.DeviceData{}, fmt.Errorf("no declared device %s", peerID)
	}
	if d.Paused == paused {
		return d, nil
	}
	d.Paused = paused
	if err := workbench.SaveDevice(st, d); err != nil {
		return workbench.DeviceData{}, err
	}
	return d, nil
}

// RemountFolder re-creates the local-files mount a folder declaration
// remembers, at the path recorded in the declaration.
//
// This is the action the reconciler NAMES and refuses to take. The loop
// will not create a mount, because creating one means writing to a
// directory on somebody's disk and that is an operator's decision — so it
// reports the problem with the action named, and a surface offers the
// action. This is that surface's half.
//
// The path comes from the declaration and never from a caller, which is
// the whole reason FolderData carries one: after the mount that knew is
// gone, the record is the only thing left that can say where a folder's
// files are. A remount that took a path as an argument would let a
// mis-typed directory become the folder's new meaning, silently, on the
// receiving side of somebody else's writes.
func (ws *ShellWorkspace) RemountFolder(folderID string) (MountOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return MountOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	st := ws.Local.Peer.Store()
	f, ok := workbench.LoadFolder(st, folderID)
	if !ok {
		return MountOutcome{}, fmt.Errorf("no declared folder %q", folderID)
	}
	root := f.ReceivingRoot()
	if hasLocalRoot(ws.Local.Peer, root) {
		return MountOutcome{}, fmt.Errorf("folder %q is already mounted at root %q", f.DisplayLabel(), root)
	}
	if f.Path == "" {
		// A refusal, not a guess. A folder record with no path is one
		// whose mount was gone before the record learned where it was,
		// and inventing a directory here would bridge a path the
		// operator never chose.
		return MountOutcome{}, fmt.Errorf(
			"folder %q does not record a directory — re-create the mount by hand and it will be picked up",
			f.DisplayLabel())
	}
	return ws.Mount(MountRequest{
		FilesystemDir: f.Path,
		TargetPrefix:  remountTargetPrefix(st, root),
	})
}

// remountTargetPrefix recovers the tree prefix a folder's documents used
// to land under, so a remount restores the SAME mapping rather than a new
// one derived from the directory name.
//
// The mount binding outlives the mount — `shellboot` restores it at
// startup and it is what the ingest handler routes on — so when it is
// present it is the authority. Falling back to the default when it is not
// is correct rather than lax: a folder with no binding never had a
// workbench-side mapping to preserve.
func remountTargetPrefix(st *workbench.Store, root string) string {
	if b, ok := workbench.LoadMountBinding(st, root); ok && b.TargetPrefix != "" {
		return b.TargetPrefix
	}
	return "doc/" + root + "/"
}

// DeviceLabelFor is the display name for a peer-id in the relationship
// surfaces, and it is deliberately NOT [ShellWorkspace.AliasFor].
//
// `AliasFor` reads `peerMap`, which is populated when a connection is
// made and empty otherwise — so after a restart, before anything has
// dialed, every row in a status table would be a bare peer-id. The
// DECLARED label is durable and is what the operator named this machine,
// so it wins; the connection alias is the fallback, and the peer-id the
// last resort. Never the empty string: a blank name in a relationship
// table is indistinguishable from a broken row.
func (ws *ShellWorkspace) DeviceLabelFor(peerID string) string {
	if peerID == "" {
		return ""
	}
	if ws != nil && ws.Local != nil && ws.Local.Peer != nil {
		if d, ok := workbench.LoadDevice(ws.Local.Peer.Store(), peerID); ok && d.Label != "" {
			return d.Label
		}
	}
	if alias := ws.AliasFor(peerID); alias != "" {
		return alias
	}
	return peerID
}
