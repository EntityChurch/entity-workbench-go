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

	// OutboundRoute reports that WE hold a connection to this peer, i.e.
	// that we can dispatch to them. **A renderer must not draw
	// `connected` without it.**
	//
	// `connected` alone is the connection POOL, which holds sessions in
	// both directions and tags neither. An inbound-only peer — they
	// dialled us, we never dialled them — is `connected: true` while
	// nothing we write can leave the machine, and that is the most likely
	// half-broken state there is, because a dial-by-address authorizes
	// only the dialer (AP63). An operator watched that exact row say
	// "connected" for 45 minutes on 2026-09-08 while the run log
	// correctly said the outbound connection had never come up.
	//
	// This is the field an undeclared-DTO bug would silently drop, so
	// StatusEnvelopeTests asserts it arrives (AP49).
	OutboundRoute bool `json:"outboundRoute"`

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

	// ReceiveFrom / SyncingWith are the DECLARATION and the OBSERVATION
	// of the same question: whose changes should reach this folder, and
	// whose actually can.
	//
	// Both cross the boundary because the renderer must be able to show
	// the gap, and until 2026-09-10 neither did — the model computed
	// SyncingWith and the bridge dropped it (AP49), so the folder row had
	// nothing but our own declared direction to draw from and rendered
	// `↔ two-way` for a folder that was pulling from nobody.
	ReceiveFrom []string `json:"receiveFrom"`
	SyncingWith []string `json:"syncingWith"`

	// Problems is `FolderStatus.problems()` verbatim — the SAME sentences
	// the shell prints and a reconcile pass produces.
	//
	// Carried rather than re-derived, because the renderer deciding when
	// a folder is in trouble is a second implementation of that judgement
	// and it was wrong: it showed a problem only for an unmounted folder
	// or an accepted-but-not-syncing received one, so a folder we OWN
	// could not report anything at all, however broken.
	Problems []string `json:"folderProblems"`

	Peers []statusFolderPeerDTO `json:"peers"`
}

// statusDeliveryDTO is this peer's subscription-delivery saturation.
//
// It reaches a pixel here for the first time. `ReconcileOutcome.Delivery`
// has been computed on every reading since the load work and this DTO did
// not declare it, so `System.Text.Json` dropped it in total silence
// (AP49) — the one number that explains "I copied a big directory in and
// it stopped part way" was being maintained, carried across the model
// boundary, and thrown away one field short of the screen.
type statusDeliveryDTO struct {
	// Available is false when the peer exposes no subscription engine.
	// A renderer must say *not measured* rather than drawing a zero:
	// "no drops" and "nothing counted them" are different claims and the
	// second one is usually the whole problem.
	Available bool `json:"available"`
	// Dropped is the lifetime count of notifications this peer discarded
	// because a delivery shard was full. A PROCESS counter, reset by a
	// restart — it says nothing about drops in an earlier run.
	Dropped uint64 `json:"dropped"`
	// QueueDepth is the current total depth across delivery shards.
	QueueDepth int `json:"queueDepth"`
	// Summary is the operator-facing sentence, or "" when there is
	// nothing to say. Composed in Go so the shell and the GUI cannot
	// describe saturation differently.
	Summary string `json:"summary"`
}

// statusCatchUpDTO is the periodic backfill supervisor: whether it is
// running, how fast, and what the last pass did.
//
// Also reaching a pixel for the first time. The supervisor is what turns
// a dropped delivery into a delay instead of a loss, and until now it was
// reachable from `entity-shell`'s `status` and `catchup` and from nothing
// in the GUI — which is the frontend the operator who hit the failure was
// using.
type statusCatchUpDTO struct {
	// Running is whether a supervisor is attached in this process. False
	// means nothing will pass again on its own, and a folder that stopped
	// part way stays that way.
	Running bool `json:"running"`
	// HavePass distinguishes "no files were recovered" from "no pass has
	// run yet". Rendering the second as the first is how a stalled
	// supervisor reads as a healthy one.
	HavePass bool `json:"havePass"`
	// IntervalSeconds is the CURRENT wait, which is adaptive — it drops
	// to the floor while passes are recovering files and backs off toward
	// the ceiling once they stop. "Checked every 5 s because it is still
	// finding things" and "checked twice an hour because it has been
	// quiet since Tuesday" are the same table without this.
	IntervalSeconds float64 `json:"intervalSeconds"`
	MinSeconds      float64 `json:"minSeconds"`
	MaxSeconds      float64 `json:"maxSeconds"`

	Folders        int     `json:"folders"`
	Recovered      int     `json:"recovered"`
	AlreadyCurrent int     `json:"alreadyCurrent"`
	Failed         int     `json:"failed"`
	DurationMillis float64 `json:"durationMillis"`
	AtMillis       uint64  `json:"atMillis"`
	Summary        string  `json:"summary"`
}

// statusHistoryLimitDTO is one path whose change recording was stopped
// because its chain outgrew its budget.
type statusHistoryLimitDTO struct {
	Path        string `json:"path"`
	Root        string `json:"root"`
	ConfigName  string `json:"configName"`
	Transitions uint64 `json:"transitions"`
	Budget      uint64 `json:"budget"`
	AtMillis    uint64 `json:"atMillis"`
	Summary     string `json:"summary"`
}

// statusRecordingDTO is whether anything is COUNTING recording growth,
// and what it has seen.
//
// The counters matter less than `running`. A guard that is not attached
// reports no limits and is indistinguishable, on screen, from a peer with
// no runaway paths — and the difference is a disk that fills overnight.
type statusRecordingDTO struct {
	Running     bool                    `json:"running"`
	Budget      uint64                  `json:"budget"`
	Paths       int                     `json:"paths"`
	Transitions uint64                  `json:"transitions"`
	Tripped     int                     `json:"tripped"`
	Limits      []statusHistoryLimitDTO `json:"limits"`
}

// statusConflictDTO is one file where a delivery replaced a local edit.
type statusConflictDTO struct {
	Key  string `json:"key"`
	Path string `json:"path"`
	Root string `json:"root"`
	// Recoverable gates the "restore mine" verb. A record whose replaced
	// version was never kept — the path had no change recording at the
	// time — is a NOTIFICATION, and offering a restore against it would
	// offer something that fails.
	Recoverable  bool   `json:"recoverable"`
	KeepBothPath string `json:"keepBothPath"`
	AtMillis     uint64 `json:"atMillis"`
	Summary      string `json:"summary"`
}

// statusConflictsDTO is the peer's conflict state: what is waiting for a
// decision, and whether the burst limiter has stopped delivery.
type statusConflictsDTO struct {
	Unresolved []statusConflictDTO `json:"unresolved"`
	// Storming means this peer has DELIBERATELY stopped materializing
	// deliveries. Neither healthy nor broken, and said nowhere else — so
	// a surface that renders only the list would show an operator an
	// empty table while files stop arriving.
	Storming      bool    `json:"storming"`
	Detected      int     `json:"detected"`
	Refused       int     `json:"refused"`
	Limit         int     `json:"limit"`
	WindowSeconds float64 `json:"windowSeconds"`
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

	// The three peer-wide facts. Not per-row on purpose: saturation is a
	// property of the engine every subscription shares, the supervisor
	// passes over all folders at once, and the recording guard counts
	// paths across mounts. Attributing any of them to a folder row would
	// invent an attribution the substrate does not have.
	Delivery  statusDeliveryDTO  `json:"delivery"`
	CatchUp   statusCatchUpDTO   `json:"catchUp"`
	Recording statusRecordingDTO `json:"recording"`
	Conflicts statusConflictsDTO `json:"conflicts"`
}

func statusOutcomeToDTO(localPeerID, localAlias string, ws *shellcmd.ShellWorkspace,
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
		Delivery: statusDeliveryDTO{
			Available:  out.Delivery.Available,
			Dropped:    out.Delivery.Dropped,
			QueueDepth: out.Delivery.QueueDepth,
			Summary:    out.Delivery.Summary(),
		},
		CatchUp:   catchUpToDTO(ws),
		Recording: recordingToDTO(ws, out),
		Conflicts: conflictsToDTO(out),
	}
	for _, d := range out.Devices {
		dto.Devices = append(dto.Devices, statusDeviceDTO{
			PeerID:         d.PeerID,
			Label:          d.Label,
			Address:        d.Address,
			Maintained:     d.Maintained,
			Connected:      d.Connected,
			OutboundRoute:  d.OutboundRoute,
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
			ReceiveFrom:      nz(f.ReceiveFrom),
			SyncingWith:      nz(f.SyncingWith),
			// One writer for the sentence (shellcmd/status.go), shared by
			// the shell, the pass and this panel, so the three cannot
			// describe the same fault differently.
			Problems: nz(f.Problems()),
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

// catchUpToDTO reads the supervisor's state off the workspace.
//
// Deliberately NOT off the outcome: `LastCatchUp` and `CatchUpInterval`
// are process memory about a running loop, and a reconcile pass does not
// take a catch-up pass. Folding them into ReconcileOutcome would make
// "the last catch-up" look like something this reading produced.
func catchUpToDTO(ws *shellcmd.ShellWorkspace) statusCatchUpDTO {
	dto := statusCatchUpDTO{
		MinSeconds: shellcmd.MinCatchUpInterval.Seconds(),
		MaxSeconds: shellcmd.MaxCatchUpInterval.Seconds(),
	}
	if ws == nil {
		return dto
	}
	iv, running := ws.CatchUpInterval()
	dto.Running = running
	dto.IntervalSeconds = iv.Seconds()

	last, have := ws.LastCatchUp()
	dto.HavePass = have
	if !have {
		return dto
	}
	dto.Folders = last.Folders
	dto.Recovered = last.Recovered
	dto.AlreadyCurrent = last.AlreadyCurrent
	dto.Failed = last.Failed
	dto.DurationMillis = float64(last.Duration.Microseconds()) / 1000
	dto.AtMillis = last.AtMillis
	dto.Summary = last.Summary()
	return dto
}

// recordingToDTO reads the growth guard's state, plus the durable limits
// the outcome already carries.
//
// The limits come off the OUTCOME and not from a second read, so the
// table and the `problems:` lines beside it are the same list. Two reads
// would be two answers the moment a limit trips between them, and the
// disagreement would look like a bug in the panel.
func recordingToDTO(ws *shellcmd.ShellWorkspace, out shellcmd.ReconcileOutcome) statusRecordingDTO {
	dto := statusRecordingDTO{Limits: []statusHistoryLimitDTO{}}
	if ws != nil {
		st := ws.HistoryBudget()
		dto.Running = st.Running
		dto.Budget = st.Budget
		dto.Paths = st.Paths
		dto.Transitions = st.Transitions
		dto.Tripped = st.Tripped
	}
	for _, l := range out.HistoryLimits {
		dto.Limits = append(dto.Limits, statusHistoryLimitDTO{
			Path:        l.Path,
			Root:        l.Root,
			ConfigName:  l.ConfigName,
			Transitions: l.Transitions,
			Budget:      l.Budget,
			AtMillis:    l.AtMillis,
			Summary:     l.Summary(),
		})
	}
	return dto
}

// conflictsToDTO carries the unresolved conflicts and the limiter state.
//
// Off the OUTCOME, so the table and the `problems:` lines beside it are
// the same list — two reads would be two answers the moment a conflict
// arrives between them, and the disagreement would look like a panel bug.
func conflictsToDTO(out shellcmd.ReconcileOutcome) statusConflictsDTO {
	dto := statusConflictsDTO{
		Unresolved:    []statusConflictDTO{},
		Storming:      out.ConflictHealth.Storming,
		Detected:      out.ConflictHealth.Detected,
		Refused:       out.ConflictHealth.Refused,
		Limit:         out.ConflictHealth.Limit,
		WindowSeconds: out.ConflictHealth.WindowSeconds,
	}
	for _, c := range out.Conflicts {
		dto.Unresolved = append(dto.Unresolved, statusConflictDTO{
			Key:          c.Key(),
			Path:         c.Path,
			Root:         c.Root,
			Recoverable:  c.Recoverable,
			KeepBothPath: c.KeepBothPath,
			AtMillis:     c.AtMillis,
			Summary:      c.Summary(),
		})
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
		statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, ws, out),
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
		statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, ws, out),
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

// panelCatchUpTimeout bounds one catch-up pass driven from the GUI.
//
// Longer than the reconcile timeout because the work is different in
// kind: a reconcile waits on peers that may be switched off, and a
// catch-up transfers files from ones that are not. A recovering pass over
// 2000 files ran ~700 ms and a settled one ~470 ms, so this is generous
// for anything an operator would recognise as a folder — but a first
// backfill of a large share is genuinely a transfer and must not be cut
// off at a number chosen for a status table.
const panelCatchUpTimeout = 2 * time.Minute

// StatusCatchUp runs ONE catch-up pass over every folder this peer
// receives, and returns the resulting reading.
//
// **Safe from a button and safe from a timer, unlike StatusReconcile** —
// and the difference is the whole reason this is a separate export. A
// catch-up uses the sync binding and the pooled connection; it does not
// dial, does not create a mount, does not delete, and does not write
// policy. It is a read of what the sender actually has, plus writes into
// a mount the operator already agreed to.
//
// It exists because the machinery that turns a dropped delivery into a
// delay rather than a loss was reachable from `entity-shell` and from no
// pixel — on the failure an operator is most likely to meet and least
// able to diagnose. `catchup` is the shell verb; this is the same call.
//
//export StatusCatchUp
func StatusCatchUp(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("StatusCatchUp", &result)
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelCatchUpTimeout)
	defer cancel()

	// RunCatchUpOnce and not CatchUp: it records the pass so the reading
	// below reports it, and it declines rather than stacking when the
	// supervisor is already mid-pass. A second concurrent pass over the
	// same folders would double the transfer and report half of it twice.
	if _, ran := ws.RunCatchUpOnce(ctx); !ran {
		// Not an error. A pass was already running, which is the healthy
		// case — fall through and render, so the panel shows whatever that
		// pass concludes rather than an alarm about a working supervisor.
		_ = ran
	}

	out, err := ws.StatusSnapshot()
	if err != nil {
		return marshalReply(statusRenderDTO{Er: err.Error()}, "status catch-up")
	}
	return marshalReply(
		statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, ws, out),
		"status catch-up")
}

// StatusResolveConflict decides one conflict and returns the resulting
// reading.
//
// keep is "mine", "theirs" or "both", and there is deliberately no
// default at this boundary either: the verb refuses an empty one and so
// does this, because a panel that silently picked for the operator would
// be the exact behaviour the whole feature exists to replace.
//
// Local work — a reassemble and a file write — so it neither dials nor
// blocks on another machine. It still goes on a thread-pool worker,
// because it writes a file whose size nobody here chose.
//
//export StatusResolveConflict
func StatusResolveConflict(peerHandle C.int64_t, key *C.char, keep *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("StatusResolveConflict", &result)
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	k := strings.TrimSpace(C.GoString(key))
	choice := strings.TrimSpace(C.GoString(keep))
	out, err := ws.ResolveConflict(k, choice)
	if err != nil {
		return marshalReply(statusRenderDTO{Er: err.Error()}, "status resolve conflict")
	}

	// Render afterwards, so the panel's table and the sentence it shows
	// come from one reading. Returning only a note would leave the row
	// on screen until the next refresh, which reads as the button having
	// done nothing.
	snap, err := ws.StatusSnapshot()
	if err != nil {
		return marshalReply(statusRenderDTO{Er: err.Error()}, "status resolve conflict")
	}
	dto := statusOutcomeToDTO(hp.AppPeer.PeerID(), ws.Local.Alias, ws, snap)
	dto.Actions = append(dto.Actions, out.Path+": "+out.Note)
	return marshalReply(dto, "status resolve conflict")
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

// StatusSetFolderDirection sets which way a shared folder flows on THIS
// peer: send, receive, or both.
//
// The whole operation is `ShellWorkspace.SetFolderMode`, shared with the
// `direction` verb. Nothing about the rules is reimplemented here —
// re-deriving the policy row from the declaration, and the fact that a
// changed grant is only in force at the next handshake, are exactly the
// things nobody rediscovers by reading a renderer.
//
// It exists because S6 made `FolderData.Mode` the field the reconciler
// branches on, and a setting that governs an operator's bytes with no way
// to choose it is AP57 from the other side.
//
//export StatusSetFolderDirection
func StatusSetFolderDirection(peerHandle C.int64_t, folderID *C.char, mode *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("StatusSetFolderDirection", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	res, err := ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: strings.TrimSpace(C.GoString(folderID)),
		Mode:     strings.TrimSpace(C.GoString(mode)),
	})
	if err != nil {
		return marshalReply(statusActionReplyDTO{Er: err.Error()}, "status set direction")
	}
	note := res.Label + " is already " + res.Mode + " — nothing changed."
	if res.Changed {
		switch res.Mode {
		case "send":
			note = res.Label + ": changes here are published to them; theirs are not applied here."
		case "receive":
			note = res.Label + ": their changes are applied here; nothing here is published to them."
		default:
			// "changes now flow both ways" — WHICH IS A PROMISE, and the
			// pass below is what decides whether it is true.
			//
			// This sentence shipped unconditionally. An operator pressed
			// *Make two-way*, got told changes flow both ways, and
			// nothing was established — because direction is per-peer and
			// the other machine had not asked. The declaration is ours to
			// state; the outcome is not, and stating one as the other is
			// the exact failure this panel exists to prevent.
			note = res.Label + ": set to two-way on this machine."
		}
	}
	if res.Caveat != "" {
		note += " " + res.Caveat
	}
	// WHAT THE PASS ACTUALLY DID. `SetFolderMode` reconnects and
	// reconciles, and its outcome is the only thing that can distinguish
	// "declared" from "working". Dropping it here would be AP49 on the
	// one field that carries the bad news.
	if len(res.Problems) > 0 {
		note += " NOT established yet: " + strings.Join(res.Problems, " · ")
	} else if len(res.Established) > 0 {
		note += " Established: " + strings.Join(res.Established, " · ")
	} else if res.Changed && res.Mode == "both" {
		// Neither built nor refused — say so rather than letting silence
		// read as success.
		note += " Nothing needed establishing."
	}
	return marshalReply(statusActionReplyDTO{OK: true, Note: note}, "status set direction")
}
