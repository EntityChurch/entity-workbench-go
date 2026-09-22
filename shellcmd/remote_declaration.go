package shellcmd

import (
	"fmt"
	"strings"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// remote_declaration.go — THE OWNER LOOKS, RATHER THAN WAITING TO BE TOLD.
//
// # The question this answers
//
// A shared folder is ONE object across two peers (S6, workbench.FolderID),
// and each peer holds its own record of it. Two facts live only in the
// OTHER peer's record, and the owner needs both to run the reverse leg of
// a `Mode: both` folder:
//
//   - WHICH ROOT THEIR COPY IS MOUNTED UNDER. `accept` takes a directory,
//     so the receiver's mount root is whatever they named it —
//     `FolderData.LocalRoot`, in their tree. Two peers not having to agree
//     on a filesystem path is a stated requirement of this product; it is
//     the entire reason `accept` grew a directory argument.
//   - WHETHER THEY ARE PUBLISHING. Direction is a per-peer property of one
//     folder, so `both` on our side means "we will accept their changes"
//     and says nothing about whether they send any.
//
// Before this, the reconciler assumed the answer to the first (our root
// name) and never asked the second. The consequence is the failure this
// codebase names most often and keeps re-shipping: a subscription to
// `local/files/{OUR root}/*` on a peer whose files are under
// `local/files/{THEIR root}/*` is ACCEPTED, reports healthy, and delivers
// nothing for as long as it exists.
//
// # Why a read and not a channel
//
// The obvious framing is that the receiver must TELL the owner — a
// receiver→owner message carrying acceptance, their local root, progress.
// That channel is genuinely owed for other reasons (conflict propagation
// needs it). It is not needed here, and building it here would have been
// the more expensive way to get a worse answer:
//
//   - the reverse leg only exists when the receiver PUBLISHES, and a
//     publishing peer has already granted us `system/tree:get` over their
//     namespace (workbench.SyncSenderGrants). The authority is present
//     exactly when the question is worth asking, by construction;
//   - a pushed fact is a snapshot that goes stale silently. A read is
//     true at the moment the reconciler acts on it, which is the moment
//     that matters;
//   - a push needs a new field in `app/share/*`, which is
//     APP-CONVENTION-SHARE's namespace and therefore a cross-impl
//     coordination rather than a local edit.
//
// This is a DISPATCHED read of a peer-qualified path, i.e. a remote read
// (AP11): `Get("/{them}/app/workbench/folders/{id}")` routes to that peer
// and answers about THEIR tree. `ShellWorkspace.Offers` is the same shape
// and has worked this way since S3.
//
// # What it deliberately does not do
//
// **It does not reconnect.** `refreshGrantConnection` disconnects before
// dialling, and the reconciler runs on a timer — a pass that re-handshakes
// every peer it wants to ask a question about is an outage generator, and
// the rule this repo already holds is that a POLICY change and only a
// policy change forces a re-handshake. So an unauthorized read is reported
// as what it is and retried next pass, on the connection the operator's
// own actions established.

// RemoteFolderView is what a peer's own record says about a folder we both
// participate in, read over the wire.
//
// Every field is HEARSAY in the strict sense — it is a fact about another
// machine, observed, not one we can compute. The struct exists so that
// stays visible at every call site: a reader who has a RemoteFolderView in
// hand cannot mistake it for local state, which is precisely the confusion
// that put another peer's acceptance decision inside our own declaration
// record and then had the reconciler branch on it.
type RemoteFolderView struct {
	// PeerID is whose record this is.
	PeerID string
	// FolderID is the shared identifier — the same string on both peers.
	FolderID string

	// Found is false when that peer holds no record for this folder at
	// all, which is the ordinary state before they have accepted. It is
	// NOT an error and must not be reported as one.
	Found bool

	// Root is the mount root THEIR copy lives under: their
	// FolderData.ReceivingRoot(). This is the answer the reverse leg
	// needs, and it is empty unless Found.
	Root string

	// Mode is their declared direction for this folder, defaulted through
	// EffectiveMode so a pre-S6 record reads as what its author meant.
	Mode string

	// Publishes is whether they send their changes for this folder. When
	// false there is no reverse leg to build — not because we refuse, but
	// because they have not offered one, and the two are different
	// sentences to put in front of an operator.
	Publishes bool

	// OurState is what their record says about US: offered / accepted /
	// declined / withdrawn, or empty when they name us not at all.
	//
	// This is the fact that "acceptance does not travel" is about. It
	// travels fine; nobody was looking.
	OurState string
}

// Summary is a one-line rendering for an operator-facing note. Never
// empty, because a blank explanation beside a folder that is not syncing
// is indistinguishable from no explanation at all.
func (v RemoteFolderView) Summary() string {
	if !v.Found {
		return "they hold no record of this folder yet"
	}
	s := fmt.Sprintf("their copy is at root %q, direction %q", v.Root, v.Mode)
	if v.OurState != "" {
		s += ", they have us as " + v.OurState
	}
	return s
}

// ObserveRemoteFolder reads another peer's own record of a shared folder.
//
// A remote read (AP11). Returns Found=false with a nil error when that peer
// simply has no such record — the normal state before they accept — and an
// error only when the read itself could not be performed, which is a
// different thing and reaches the operator differently.
func (ws *ShellWorkspace) ObserveRemoteFolder(peerID, folderID string) (RemoteFolderView, error) {
	view := RemoteFolderView{PeerID: peerID, FolderID: folderID}
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return view, fmt.Errorf("workspace has no local peer")
	}
	peerID = strings.TrimSpace(peerID)
	folderID = strings.TrimSpace(folderID)
	if peerID == "" || folderID == "" {
		return view, fmt.Errorf("need both a peer and a folder id")
	}
	local := ws.Local.Peer
	if peerID == local.PeerID() {
		return view, fmt.Errorf("that is this peer — read the local record instead")
	}

	path := "/" + peerID + "/" + workbench.FolderPrefix + folderID
	ent, ok, err := local.Get(path)
	if err != nil {
		return view, fmt.Errorf("read %s's record of folder %s: %w", peerID, folderID, err)
	}
	if !ok {
		return view, nil
	}
	if ent.Type != workbench.FolderType {
		return view, fmt.Errorf("%s holds a %q at %s, not a folder declaration",
			peerID, ent.Type, workbench.FolderPrefix+folderID)
	}
	var f workbench.FolderData
	if derr := ecf.Decode(ent.Data, &f); derr != nil {
		return view, fmt.Errorf("decode %s's record of folder %s: %w", peerID, folderID, derr)
	}
	if f.ID == "" {
		f.ID = folderID
	}

	view.Found = true
	view.Root = f.ReceivingRoot()
	view.Mode = f.EffectiveMode()
	view.Publishes = f.Publishes()
	if ps, ok := f.PeerState(local.PeerID()); ok {
		view.OurState = ps.State
	}
	return view, nil
}

// remoteRootForReverseLeg resolves which root on `peerID` our reverse-leg
// subscription must name, for a folder WE own.
//
// Returns ok=false with a reason when the leg must not be established this
// pass. The reason is written for an operator, because every one of these
// cases used to present as a healthy row and an empty folder.
//
// # Why a failure here refuses instead of falling back
//
// The tempting fallback is "assume their root matches ours" — which is
// what the code did before, and which is right in the symmetric case. It
// is not a safe default, because the binding it creates is DURABLE and
// silent: a subscription to the wrong prefix establishes cleanly, reports
// healthy, and delivers nothing until somebody deletes it by hand. Not
// establishing it is recoverable by the next pass, which is thirty seconds
// away. Between a wrong answer that persists and no answer that retries,
// a control loop should always take the second.
func (ws *ShellWorkspace) remoteRootForReverseLeg(f workbench.FolderData, peerID string) (root string, ok bool, reason string) {
	view, err := ws.ObserveRemoteFolder(peerID, f.ID)
	if err != nil {
		// A 403 HERE IS ITSELF THE ANSWER, and saying only "could not
		// read" would waste it. The read needs `system/tree:get`, which
		// their side grants us exactly when their folder PUBLISHES
		// (workbench.SyncSenderGrants) — so a peer that refuses this read
		// is, in the overwhelmingly common case, a peer that has not
		// declared two-way. Stated as the likely cause and not as a fact,
		// because a stale handshake produces the same refusal and the two
		// send an operator to different places.
		if entitysdk.IsForbidden(err) {
			return "", false, fmt.Sprintf(
				"%s did not let us read their record of this folder, so we cannot tell "+
					"which directory they keep it in and will not guess. The usual cause "+
					"is that they have not asked for two-way — run `direction %s both` on "+
					"their machine; if they already have, their grant has not reached us "+
					"yet and reconnecting will fix it (%v)", peerID, f.ID, err)
		}
		return "", false, fmt.Sprintf(
			"could not read %s's record of this folder, so we do not know which "+
				"directory they keep it in and will not guess (%v) — retried on the "+
				"next pass", peerID, err)
	}
	if !view.Found {
		return "", false, fmt.Sprintf(
			"%s has not accepted this folder yet, so there is nothing of theirs to "+
				"receive", peerID)
	}
	if !view.Publishes {
		// NAME BOTH STEPS. Whoever declares `both` FIRST gets this
		// refusal, correctly — the other side has not asked yet — and the
		// pass that would establish the leg is the one that just ran. So
		// an operator who does only what the first half of this sentence
		// says ends up with two correct declarations and no leg, which is
		// the failure mode this whole area keeps producing. The second
		// step is cheap and `status` is the verb that re-runs the loop.
		return "", false, fmt.Sprintf(
			"%s has this folder set to %q, so they are not sending their changes — "+
				"two-way needs BOTH sides to ask for it. Run `direction %s both` on "+
				"their machine, then `status` here to establish the leg",
			peerID, view.Mode, f.ID)
	}
	if strings.TrimSpace(view.Root) == "" {
		return "", false, fmt.Sprintf(
			"%s's record names no mount root for this folder", peerID)
	}
	return view.Root, true, ""
}
