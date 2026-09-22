package main

// Sharing-status bridge surface — the declared-state layer, which until
// now the GUI could not see at all.
//
// **Why this file exists.** `grep '^//export ' avalonia/bridge/*.go` for
// devices, folders, declarations or the reconciler returned NOTHING
// before this. The whole of S2 — two declared records and an idempotent
// control loop — was reachable from one shell verb and from no pixel.
// Worse, the GUI *runs* a reconcile pass at every startup
// (`shellboot.reconcileAtStartup`) and reports its problems to **stderr**,
// which for a desktop app is a file in `avalonia/run-logs/` that nobody
// has a reason to open. So an operator whose accepted folder had lost its
// mount was told so, correctly, somewhere they would never look, while
// every panel on screen looked fine.
//
// `make reachability` cannot raise this. It asks whether a
// `workbench/*_model.go` has a surface; the reconciler is `shellcmd`'s and
// it has a verb (`status`), so the question of whether the GUI can reach
// it is never put. This is the AP57/D23-at-the-handler-layer shape again,
// and the honest description of this file is not "a nice panel" — it is
// the missing half of S2.
//
// **Render and reconcile are separate exports, deliberately.** The shell's
// `status` verb runs the loop on purpose: a read-only report has the same
// blind spot as the five verbs it replaced. A panel cannot copy that,
// because **a reconcile pass dials** — it calls maintain-peer for every
// declared device — and a panel refreshes. Wake-driven reconciling turns a
// status panel into a dialer that an operator leaves running. So:
//
//	StatusRender     — reads records + observes the substrate. No write,
//	                   no dial. Safe on a wake, safe on a timer.
//	StatusReconcile  — one pass. Explicit only: on open, on Re-check, and
//	                   after a mutation this panel performed.
//
// Both marshal the same DTO, and it carries `reconciled` so the surface
// says which it is looking at rather than implying the stronger one.
//
// **Synchronous exports, like the rest of the share surface.** The async
// cgo shape carries AP31: a `*C.char` belongs to the .NET marshaller and
// is freed when the P/Invoke returns, so reading it on a goroutine is a
// use-after-free that reads as the empty string and surfaces as a
// plausible user error. `StatusReconcile` can take seconds — it dials —
// so the caller runs it on a thread-pool worker, which is what
// `SharingStatusPanel` does and what `SharePanel` already did.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"context"
	"strings"
	"time"

	"entity-workbench-go/shellcmd"
)

// --- DTOs -------------------------------------------------------------
//
// Flat and explicit, per AP49: an undeclared field is discarded by
// System.Text.Json in TOTAL silence, and this panel's entire content is
// fields that mean "something is wrong". A dropped one renders as
// "everything is fine", which is the worst available failure for a
// surface whose only job is to say otherwise.

type statusDeviceDTO struct {
	PeerID     string `json:"peerId"`
	Label      string `json:"label"`
	Address    string `json:"address"`
	Maintained bool   `json:"maintained"`
	Connected  bool   `json:"connected"`
	Paused     bool   `json:"paused"`
	Note       string `json:"note"`

	// OutboundGrant is what WE grant THEM, summarized. Exactly knowable,
	// because it is our own policy row.
	//
	// There is deliberately no inbound counterpart. What they grant us
	// lives in their capability table, which we cannot read; it is only
	// ever observed, through deliveries arriving. A field here would let
	// a renderer draw a green dot for inbound authority, and that dot is
	// wrong in exactly the case that matters — they revoked us and we
	// have not tried since.
	OutboundGrant string `json:"outboundGrant"`

	AddedAtMillis  uint64 `json:"addedAtMillis"`
	LastSeenMillis uint64 `json:"lastSeenMillis"`
}

type statusFolderPeerDTO struct {
	PeerID   string `json:"peerId"`
	Label    string `json:"label"`
	State    string `json:"state"`
	AtMillis uint64 `json:"atMillis"`
	Note     string `json:"note"`
}

type statusFolderDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Local     bool   `json:"local"`
	Root      string `json:"root"`
	LocalRoot string `json:"localRoot"`
	Path      string `json:"path"`
	Origin    string `json:"origin"`
	Mode      string `json:"mode"`

	Mounted          bool   `json:"mounted"`
	Syncing          bool   `json:"syncing"`
	Accepted         bool   `json:"accepted"`
	AcceptedAtMillis uint64 `json:"acceptedAtMillis"`
	Note             string `json:"note"`

	// FilesPresent / FilesIngested are the two sides of the mount's lossy
	// stage, carried separately because one count reads the layer that
	// cannot fail (AP59). FilesObservable is false when there is no mount
	// to count at all — the renderer must say *unknown* rather than a
	// confident 0, since "nothing arrived" and "there is nowhere for it
	// to arrive" are different claims and the second is usually the whole
	// problem.
	FilesPresent    int  `json:"filesPresent"`
	FilesIngested   int  `json:"filesIngested"`
	FilesObservable bool `json:"filesObservable"`

	Peers []statusFolderPeerDTO `json:"peers"`
}

type statusRenderDTO struct {
	OK bool   `json:"ok"`
	Er string `json:"error"`

	// Reconciled says whether a PASS produced this reading or whether it
	// is a read of the records. Only a pass dials, writes policy or
	// establishes a subscription, so only a pass turns "declared" into
	// "verified" — and a surface that captioned a read as a verification
	// would be making the exact claim this whole split exists to avoid.
	Reconciled bool `json:"reconciled"`

	// LocalPeerID / LocalAlias name WHICH PEER this table is about. The
	// GUI hosts several and nothing on screen used to say which one a
	// panel belonged to; for this panel that is not cosmetic, because
	// every row is a statement about one peer's relationships.
	LocalPeerID string `json:"localPeerId"`
	LocalAlias  string `json:"localAlias"`

	Devices  []statusDeviceDTO `json:"devices"`
	Folders  []statusFolderDTO `json:"folders"`
	Actions  []string          `json:"actions"`
	Problems []string          `json:"problems"`
	Settled  bool              `json:"settled"`
}

func statusOutcomeToDTO(localPeerID, localAlias string,
	out shellcmd.ReconcileOutcome) statusRenderDTO {

	dto := statusRenderDTO{
		OK:          true,
		Reconciled:  out.Reconciled,
		LocalPeerID: localPeerID,
		LocalAlias:  localAlias,
		Devices:     []statusDeviceDTO{},
		Folders:     []statusFolderDTO{},
		Actions:     nz(out.Actions),
		Problems:    nz(out.Problems),
		Settled:     out.Settled(),
	}
	for _, d := range out.Devices {
		dto.Devices = append(dto.Devices, statusDeviceDTO{
			PeerID:         d.PeerID,
			Label:          d.Label,
			Address:        d.Address,
			Maintained:     d.Maintained,
			Connected:      d.Connected,
			Paused:         d.Paused,
			Note:           d.Note,
			OutboundGrant:  d.OutboundGrant,
			AddedAtMillis:  d.AddedAtMillis,
			LastSeenMillis: d.LastSeenMillis,
		})
	}
	for _, f := range out.Folders {
		row := statusFolderDTO{
			ID:               f.ID,
			Label:            f.Label,
			Local:            f.Local,
			Root:             f.Root,
			LocalRoot:        f.LocalRoot,
			Path:             f.Path,
			Origin:           f.Origin,
			Mode:             f.Mode,
			Mounted:          f.Mounted,
			Syncing:          f.Syncing,
			Accepted:         f.Accepted,
			AcceptedAtMillis: f.AcceptedAtMillis,
			Note:             f.Note,
			FilesPresent:     f.FilesPresent,
			FilesIngested:    f.FilesIngested,
			FilesObservable:  f.FilesObservable,
			// Empty, not nil: `[]` deserializes to a list you can iterate
			// and `null` to one that throws on first use.
			Peers: []statusFolderPeerDTO{},
		}
		for _, p := range f.PeerStates {
			label := p.Label
			if label == "" {
				label = p.PeerID
			}
			row.Peers = append(row.Peers, statusFolderPeerDTO{
				PeerID:   p.PeerID,
				Label:    label,
				State:    p.State,
				AtMillis: p.AtMillis,
				Note:     p.Note,
			})
		}
		dto.Folders = append(dto.Folders, row)
	}
	return dto
}

func nz(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// StatusRender reads the declared state and observes the substrate. It
// writes nothing and dials nobody, so it is safe from a wake.
//
//export StatusRender
func StatusRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("StatusRender", &result)
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	out, err := ws.StatusSnapshot()
	if err != nil {
		return marshalReply(statusRenderDTO{Er: err.Error()}, "status render")
	}
	return marshalReply(
		statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, out),
		"status render")
}

// panelReconcileTimeout bounds one pass driven from the GUI.
//
// Shorter than the 60 s startup pass and longer than the shell's 20 s: a
// person is watching this one, and a declared peer that is switched off
// is the normal case rather than the exceptional one. An unbounded pass
// would make a button appear to hang for as long as another machine
// stays asleep.
const panelReconcileTimeout = 25 * time.Second

// StatusReconcile runs ONE reconcile pass and returns the resulting
// state.
//
// This is the export that dials, writes policy where the declarations
// require it, and establishes a subscription for an accepted folder that
// has a mount and no sync. It must never be wired to a wake or a timer —
// see this file's header.
//
//export StatusReconcile
func StatusReconcile(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("StatusReconcile", &result)
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelReconcileTimeout)
	defer cancel()
	out, err := ws.Reconcile(ctx)
	if err != nil {
		return marshalReply(statusRenderDTO{Er: err.Error()}, "status reconcile")
	}
	return marshalReply(
		statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, out),
		"status reconcile")
}

// --- Mutations the panel offers --------------------------------------

type statusActionReplyDTO struct {
	OK bool   `json:"ok"`
	Er string `json:"error"`
	// Note is what happened, in the operator's terms. Carried rather than
	// composed in the renderer so the two frontends cannot describe the
	// same action differently.
	Note string `json:"note"`
}

// StatusPauseDevice pauses or resumes a declared peer.
//
// It writes the DECLARATION and nothing else: the reconciler stops
// maintaining the peer on its next pass because the record says so. A
// version that closed the connection directly would be a wizard step
// again — substrate changed, declaration unchanged, and the next pass
// undoes it. That is exactly how `unshare` silently reversed itself.
//
//export StatusPauseDevice
func StatusPauseDevice(peerHandle C.int64_t, peer *C.char, paused C.int) (result *C.char) {
	defer recoverToErrorEnvelope("StatusPauseDevice", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	who := strings.TrimSpace(C.GoString(peer))
	want := paused != 0
	d, err := ws.SetDevicePaused(who, want)
	if err != nil {
		return marshalReply(statusActionReplyDTO{Er: err.Error()}, "status pause")
	}
	note := d.ShortLabel() + " resumed — the next re-check maintains a connection to it again."
	if want {
		note = d.ShortLabel() + " paused. Nothing is disconnected now; the relationship simply " +
			"stops being kept alive from the next pass. Files already received stay where they are."
	}
	return marshalReply(statusActionReplyDTO{OK: true, Note: note}, "status pause")
}

// StatusRemountFolder re-creates a folder's mount at the directory its
// declaration remembers.
//
// This is the action the reconciler NAMES and refuses to take, because
// creating a mount means writing to a directory on somebody's disk and
// that is an operator's decision. The loop reports the problem with the
// action named; this is the surface's half of that bargain.
//
// The path is not a parameter. It comes from the declaration, which is
// the only thing left that can say where a folder's files are once the
// mount that knew is gone — and a remount that took a directory would let
// a mistyped one silently become the folder's new meaning on the
// receiving side of somebody else's writes.
//
//export StatusRemountFolder
func StatusRemountFolder(peerHandle C.int64_t, folderID *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("StatusRemountFolder", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	id := strings.TrimSpace(C.GoString(folderID))
	out, err := ws.RemountFolder(id)
	if err != nil {
		return marshalReply(statusActionReplyDTO{Er: err.Error()}, "status remount")
	}
	return marshalReply(statusActionReplyDTO{
		OK: true,
		Note: "mounted " + out.FilesystemRoot + " at root " + out.RootName +
			" → " + out.TargetPrefix + ". Run a re-check to establish the rest.",
	}, "status remount")
}
