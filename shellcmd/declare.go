package shellcmd

// declare.go — turning the two operator gestures into the two declared
// records the reconciler works from.
//
// These run INSIDE the existing `share` and `accept` verbs rather than
// replacing them. That is deliberate and it is the cheap half of the
// migration: the verbs keep doing exactly what they did, and they now
// also leave behind a durable statement of what the operator wanted. The
// reconciler can then re-establish it after a restart, after a policy
// change, or after the other machine comes back — none of which the verbs
// can do for themselves, because a verb runs once and a relationship
// lasts.
//
// Nothing here is allowed to fail a share or an accept for a cosmetic
// reason; a failure to declare IS returned, because a share that
// establishes substrate nothing remembers wanting is the state this whole
// design exists to end.

import (
	"fmt"
	"strings"
	"time"

	"entity-workbench-go/workbench"
)

// declareLocalShare records that we share `root` with `peerID`, and that
// we know `peerID`.
//
// Idempotent in both records: re-sharing the same folder with the same
// peer rewrites the same content, and sharing it with a SECOND peer adds
// an entry rather than replacing the first — the same cumulative-audience
// rule the offer record already follows, expressed where it is now the
// authority rather than a copy of it.
//
// It returns the folder record as it was BEFORE the change, so a caller
// whose later step fails can put it back exactly. Restoring "the record
// without this peer" would be wrong whenever the peer was already in it
// under some other state, and a share that fails half-way must not
// quietly rewrite a relationship it did not create.
func (ws *ShellWorkspace) declareLocalShare(root, peerID, alias string, nowMillis uint64) (prior workbench.FolderData, hadPrior bool, err error) {
	st := ws.Local.Peer.Store()
	if err := ws.declareDevice(peerID, alias, nowMillis); err != nil {
		return workbench.FolderData{}, false, err
	}

	// The id is the SHARED one — the same string this folder has on every
	// peer that participates in it. Before S6 this was the bare root, so
	// our record and theirs had no field in common and there was no
	// object either side could point at (workbench.FolderID).
	id := workbench.FolderID(ws.Local.Peer.PeerID(), root)

	f, hadPrior := workbench.LoadFolder(st, id)
	prior = f
	if !hadPrior {
		path, _ := workbench.MountFilesystemRoot(st, root)
		f = workbench.FolderData{
			ID:     id,
			Label:  root,
			Kind:   "files",
			Path:   path,
			Root:   root,
			Origin: "local",
			Mode:   workbench.FolderModeBoth,
		}
	}
	// Refresh the path every time. A folder can be unmounted and remounted
	// somewhere else, and a record that kept the first answer would send a
	// later reader — or a later remount — to a directory that is no longer
	// the one being shared.
	if path, ok := workbench.MountFilesystemRoot(st, root); ok {
		f.Path = path
	}
	f = f.WithPeerState(peerID, workbench.FolderStateOffered, millisOr(nowMillis), "")
	if err := workbench.SaveFolder(st, f); err != nil {
		return prior, hadPrior, err
	}
	return prior, hadPrior, nil
}

// undeclareLocalShare puts the folder record back the way
// declareLocalShare found it, and re-derives the policy row from what is
// left.
//
// The re-derivation is the part that is easy to get wrong. The row is a
// UNION across every folder naming this peer, so removing the share must
// not remove the ROW — the same peer may still be delivering a folder we
// accepted from them, and dropping their grant would break a second
// relationship because a first one failed to record an offer.
func (ws *ShellWorkspace) undeclareLocalShare(root, peerID string, prior workbench.FolderData, hadPrior bool) {
	st := ws.Local.Peer.Store()
	if hadPrior {
		_ = workbench.SaveFolder(st, prior)
	} else {
		workbench.RemoveFolder(st, workbench.FolderID(ws.Local.Peer.PeerID(), root))
	}
	_, _ = ws.ApplyDeclaredPolicy(peerID, "share unwound: "+root)
}

// declareWithdrawnShare records that `root` is no longer offered to
// `peerID`.
//
// Without this, `unshare` does not stick. It removes the policy row, and
// then the next reconcile — at the following `status`, or the next launch
// — reads a folder record that still says `offered` for that peer and
// writes the grant straight back. A withdrawal that survives until the
// next restart and then silently reverses itself is worse than one that
// fails loudly, and it is the exact failure mode the control loop exists
// to remove: **an operator gesture that changes the substrate without
// changing the declaration is a wizard step wearing a reconciler's
// clothes.**
//
// The entry is marked withdrawn rather than deleted, per the state's
// definition in workbench/desired_state.go: a removed entry reads as "we
// never shared this", and "it stopped working" then has no answer.
func (ws *ShellWorkspace) declareWithdrawnShare(root, peerID string) error {
	st := ws.Local.Peer.Store()
	f, ok := workbench.LoadFolder(st, workbench.FolderID(ws.Local.Peer.PeerID(), root))
	if !ok {
		return nil // never declared; nothing to withdraw
	}
	if _, shared := f.PeerState(peerID); !shared {
		return nil
	}
	f = f.WithPeerState(peerID, workbench.FolderStateWithdrawn, millisOr(0), "")
	return workbench.SaveFolder(st, f)
}

// declareAcceptedFolder records that we have accepted `root` from
// `peerID`, into the local mount `localRoot`, and that we know `peerID`.
//
// The folder id is derived from the sync binding key, so this record and
// the binding the sync writes cannot name different things — the one
// field that joins them is computed, not typed twice.
//
// `localRoot` is recorded separately from `root` because they are only
// the same by coincidence. The subscription and the binding are keyed on
// the SENDER's name; the mount the bytes land in is named after the
// directory the operator picked. Writing one and inferring the other is
// what made the receiving mount have to be named after somebody else's
// folder.
func (ws *ShellWorkspace) declareAcceptedFolder(peerID, alias, root, localRoot string) error {
	st := ws.Local.Peer.Store()
	now := uint64(time.Now().UnixMilli())
	if err := ws.declareDevice(peerID, alias, now); err != nil {
		return err
	}
	if localRoot == "" {
		localRoot = root
	}

	id := workbench.ReceivedFolderID(peerID, root)
	f, ok := workbench.LoadFolder(st, id)
	if !ok {
		f = workbench.FolderData{
			ID:     id,
			Label:  root,
			Kind:   "files",
			Root:   root,
			Origin: peerID,
			// A received folder is receive-only until something says
			// otherwise. The safe default is the one that cannot
			// surprise an operator by publishing their disk back.
			Mode: workbench.FolderModeReceive,
		}
	}
	f.LocalRoot = localRoot
	if path, ok := workbench.MountFilesystemRoot(st, localRoot); ok {
		f.Path = path
	}
	f = f.WithPeerState(peerID, workbench.FolderStateAccepted, now, "")
	return workbench.SaveFolder(st, f)
}

// declareDevice records a peer as known, preserving anything already
// recorded about it.
//
// The address is taken from whatever the workspace can currently reach
// them at, because this runs at the one moment we are provably in contact
// — and an address learned here is what the reconciler dials after the
// next restart, when discovery may not have run yet.
func (ws *ShellWorkspace) declareDevice(peerID, alias string, nowMillis uint64) error {
	if strings.TrimSpace(peerID) == "" {
		return fmt.Errorf("cannot declare a device with no peer-id")
	}
	st := ws.Local.Peer.Store()
	d, ok := workbench.LoadDevice(st, peerID)
	if !ok {
		d = workbench.DeviceData{PeerID: peerID, AddedAtMillis: millisOr(nowMillis)}
	}
	// An alias is the operator's own name for this peer, so it wins over
	// a previously derived one — but an EMPTY alias never erases a label
	// they set earlier.
	if alias != "" && alias != peerID {
		d.Label = alias
	}
	if addr := ws.dialableAddressFor(peerID); addr != "" {
		d = d.WithAddress(addr)
	}
	return workbench.SaveDevice(st, d)
}

// RememberDeviceAddress records a known-good dial address on an EXISTING
// device declaration.
//
// # The defect this closes
//
// Measured 2026-09-03, two real peers, a folder shared and delivering:
// restart the sending peer and its writes stop reaching the receiver,
// while `status` reports **"settled — everything declared is
// established."** A manual `connect <alias> <host:port>` restores
// delivery instantly. That is the exact false-confidence failure the
// whole declare-then-reconcile design exists to remove, reproduced by
// the design's own status surface.
//
// The cause is one missing write. `connect` put the address in
// `ShellWorkspace.Conns`, which is process memory, and in the kernel's
// connection pool, which is also process memory. Nothing put it in the
// DECLARATION. So the address an operator typed — the one fact about
// that peer nobody else can supply — did not survive the process that
// received it, and the next reconcile had nothing to dial with.
//
// # Why it UPDATES and never CREATES
//
// A device declaration means "this is a peer I have a relationship
// with", and it is created by the two real gestures, `share` and
// `accept`. Connecting is a means, not a gesture: dialing a peer to look
// at its tree must not enroll it in a relationship the reconciler then
// maintains forever. So this is an update to a record that already
// exists, and a no-op otherwise — which is also why it returns nothing
// and swallows a missing device rather than reporting one.
func (ws *ShellWorkspace) RememberDeviceAddress(peerID, addr string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(addr) == "" {
		return
	}
	st := ws.Local.Peer.Store()
	d, ok := workbench.LoadDevice(st, peerID)
	if !ok {
		return
	}
	next := d.WithAddress(addr)
	if next.PreferredAddress() == d.PreferredAddress() {
		return
	}
	_ = workbench.SaveDevice(st, next)
}

// millisOr fills in the clock when a caller passed no timestamp. Callers
// that want determinism pass their own, which is why this is not read
// unconditionally.
func millisOr(nowMillis uint64) uint64 {
	if nowMillis != 0 {
		return nowMillis
	}
	return uint64(time.Now().UnixMilli())
}
