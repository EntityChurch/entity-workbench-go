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
	"strings"
	"time"

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
	// peer was rewritten as a result. Grants are assembled at HANDSHAKE
	// (AP63), so the new authority is inert until the connection is
	// re-established — which is why this verb now does it (Reconnected).
	//
	// This comment used to end *"and the reconciler has already forced
	// the reconnect."* It had not: this verb neither reconnected nor
	// reconciled, and nothing else did it on the operator's behalf. So
	// `direction ... both` on two machines wrote two correct
	// declarations and established nothing until some later pass
	// happened to run, which is indistinguishable from the feature not
	// working. A false sentence in a doc comment is worse than none:
	// it is the sentence a reader checks the behaviour against.
	PolicyChanged bool

	// Reconnected reports that we re-established our outbound connection
	// to at least one peer named by this folder, so the rewritten grant
	// is actually in force.
	//
	// The peer that DISPATCHES is the peer that must reconnect (AP63):
	// re-deriving OUR policy row changes what THEY may do, and they use
	// their own pooled connection — so this refresh is what lets US
	// dispatch under whatever they granted in return, which is exactly
	// what the reverse leg's remote read needs.
	Reconnected bool

	// ReconnectNote says why not, when we could not. Never empty when
	// PolicyChanged is true and Reconnected is false.
	ReconnectNote string

	// Established names what a reconcile pass built as a result — the
	// answer to "did anything actually happen". Empty is a real answer
	// and is not the same as a failure.
	Established []string

	// Problems is what the pass could not do, verbatim. The reverse leg
	// refusing because the OTHER side has not declared two-way lands
	// here, and it is the single most useful line this verb can print:
	// direction is per-peer, both sides must ask, and nothing else tells
	// an operator that they are only half done.
	Problems []string

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

	// RE-ESTABLISH, THEN RECONCILE — the same two steps `share` and
	// `accept` have always taken, and the reason this verb was the odd
	// one out is simply that nobody ran the flow (AP71).
	//
	// A policy change and ONLY a policy change forces a re-handshake; a
	// mode change that rewrote nothing skips both steps, because
	// reconnecting on every invocation is an outage generator.
	if res.PolicyChanged {
		var notes []string
		for _, p := range f.SharedWith {
			if p.PeerID == "" || p.PeerID == ws.Local.Peer.PeerID() {
				continue
			}
			ok, note := ws.refreshGrantConnection(p.PeerID)
			if ok {
				res.Reconnected = true
				continue
			}
			if note != "" {
				notes = append(notes, p.PeerID+": "+note)
			}
		}
		if !res.Reconnected {
			res.ReconnectNote = strings.Join(notes, "; ")
			if res.ReconnectNote == "" {
				res.ReconnectNote = "this folder names no other peer to reconnect to"
			}
			res.Caveat = "authorization changed and we could not re-establish the connection, " +
				"so it is in force from the next handshake either side makes"
		}
	}

	// One pass, so the operator's declaration becomes substrate now
	// rather than at some later moment they cannot observe. A verb runs
	// the loop on purpose — the argument against it is about TIMERS and
	// wakes, where a pass turns a passive surface into a dialer.
	if res.Changed {
		out, rerr := ws.ReconcileWithTimeout(30 * time.Second)
		if rerr != nil {
			res.Problems = append(res.Problems, "could not reconcile: "+rerr.Error())
		} else {
			res.Established = out.Actions
			res.Problems = append(res.Problems, out.Problems...)
		}
	}
	if mode == workbench.FolderModeSend && !f.IsLocal() {
		res.Caveat = "send-only: changes they make are no longer applied here, " +
			"and the files already received are left exactly as they are"
	}
	return res, nil
}
