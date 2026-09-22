package shellcmd

// mode_op.go — setting a folder's direction.
//
// # Why this file exists at all
//
// `FolderData.Mode` shipped with two writers, two readers, and nothing
// branching on it: both readers put it in a status DTO. It was
// faithfully stored, faithfully displayed, and consulted by no code at
// any layer — a config field that was a fiction (AP67's second
// instance). The tell, worth carrying: **a field whose only readers are
// serializers.** No test at any layer could fail on it, because
// round-tripping a value is exactly what it did correctly.
//
// S6 made the reconciler branch on it. That closes half the gap; the
// other half is this file. A model that something reads but nothing can
// set is the read-only-surface-over-a-read-write-model violation from
// the other side (AP57) — the operator would have a direction that
// governs their bytes and no way to choose it.
//
// The operation lives here, once, so the verb and the panel share it
// rather than the renderer reimplementing the rules
// (shellcmd/mount_op.go is the worked example this follows).

import (
	"fmt"

	"entity-workbench-go/workbench"
)

// SetFolderModeRequest names one folder and the direction to put it in.
type SetFolderModeRequest struct {
	// FolderID is the shared folder id (workbench.FolderID). A caller
	// holding a root name and a peer builds it rather than passing two
	// strings, so this cannot be called with a mismatched pair.
	FolderID string
	// Mode is free text; it goes through workbench.NormalizeMode, which
	// refuses anything that is not one of the three.
	Mode string
}

// SetFolderModeResult reports what changed, and — the part a caller
// needs and cannot derive — whether the change requires the other side
// to do anything before it takes effect.
type SetFolderModeResult struct {
	FolderID string
	Label    string
	Previous string
	Mode     string
	// Changed is false when the folder was already in that mode. A
	// no-op is reported as a no-op rather than as a success, so a
	// surface does not claim to have done something it did not.
	Changed bool
	// PolicyChanged reports that the authorization we grant the other
	// peer was rewritten as a result. When it is true the grant is only
	// in force at the NEXT handshake — grants are assembled at handshake
	// (AP63) — and the reconciler has already forced the reconnect.
	PolicyChanged bool
	// Caveat is the sentence a surface must show when the change is not
	// yet fully in effect. Empty when there is nothing to say.
	Caveat string
}

// SetFolderMode changes one folder's direction and re-derives everything
// downstream of it.
//
// It DECLARES and then reconciles, in that order, like every other verb
// that changes what the operator wants. Writing the policy row here
// instead would be the AP68 shape: the row has exactly one writer, and
// "the loop corrects it a pass later" is not the same as "nothing else
// writes it".
func (ws *ShellWorkspace) SetFolderMode(req SetFolderModeRequest) (SetFolderModeResult, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return SetFolderModeResult{}, fmt.Errorf("workspace has no local peer")
	}
	mode, err := workbench.NormalizeMode(req.Mode)
	if err != nil {
		return SetFolderModeResult{}, err
	}
	st := ws.Local.Peer.Store()
	f, ok := workbench.LoadFolder(st, req.FolderID)
	if !ok {
		return SetFolderModeResult{}, fmt.Errorf("no folder %q is declared on this peer", req.FolderID)
	}

	res := SetFolderModeResult{
		FolderID: f.ID,
		Label:    f.DisplayLabel(),
		Previous: f.Mode,
		Mode:     mode,
	}
	if f.Mode == mode {
		return res, nil
	}
	f.Mode = mode
	if err := workbench.SaveFolder(st, f); err != nil {
		return res, err
	}
	res.Changed = true

	// Re-derive the authorization for every peer this folder names. The
	// union is recomputed from the declarations, so a peer that is also
	// in a second relationship with us keeps that half of its row.
	for _, p := range f.SharedWith {
		if p.PeerID == "" || p.PeerID == ws.Local.Peer.PeerID() {
			continue
		}
		changed, err := ws.ApplyDeclaredPolicy(p.PeerID, "folder "+f.ID+" set to "+mode)
		if err != nil {
			return res, fmt.Errorf("re-deriving authorization for %s: %w", p.PeerID, err)
		}
		if changed {
			res.PolicyChanged = true
		}
	}

	if res.PolicyChanged {
		res.Caveat = "authorization changed; it is in force from the next handshake with that peer"
	}
	if mode == workbench.FolderModeSend && !f.IsLocal() {
		res.Caveat = "send-only: changes they make are no longer applied here, " +
			"and the files already received are left exactly as they are"
	}
	return res, nil
}
