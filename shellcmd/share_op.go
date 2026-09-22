package shellcmd

// Share / Accept as workspace OPERATIONS — the seam that turns
// discover → share → permission → mount → sync from five disconnected
// capabilities into one flow.
//
// # What was missing, stage by stage
//
// Every stage's machinery existed. None of them were joined, and two were
// reachable from nothing at all:
//
//	discover     SDK has DiscoverPeers / ReadDiscoveredCandidates and
//	             shellboot auto-announces on the listener's scheme. The
//	             Avalonia panel consumed it; the shell had no verb.
//	share        entitysdk/share.go implements APP-CONVENTION-SHARE in
//	             full — AuthorShare, ShareGrants, PrefixTarget, audience
//	             entries, the withdrawal notice. Called from share_test.go
//	             and nothing else, in either renderer.
//	permission   `system/capability/policy/{peer}` is the kernel's
//	             per-peer handshake policy table. ZERO uses in this repo.
//	             What we had instead was shellboot's OpenAccess flag — a
//	             process-wide wildcard its own doc calls development-only —
//	             which is the only reason cross-peer sync ever worked here.
//	mount        done.
//	sync         done (M2), but the receiver had to be told a peer-id and
//	             a root name out of band.
//
// # Three measured facts these operations are built around
//
// From `shellboot/policy_probe_test.go`, none of them obvious:
//
//  1. **A sync is MUTUAL authorization.** Both peers need a policy entry
//     naming the other. Grant one direction and you get an accepted
//     subscription and no files — which is indistinguishable from a
//     working share until someone looks in the folder.
//  2. **The grant is fixed at HANDSHAKE.** Writing a policy while already
//     connected changes nothing. Both Share and Accept therefore
//     re-establish the connection, and report whether they managed to.
//  3. **The peer-id-keyed policy path works**, so an operator who has a
//     peer-id from `peers` needs nothing else.
//
// # Why the policy table and not AuthorShare's minted tokens
//
// `entitysdk.ShareWithdrawalNotice` states that a
// `system/capability:request`-minted token is **not recallable**: it is
// returned inline with no tree write, so the granter never holds its hash
// and `revoke` cannot name it. Building `unshare` on that would make the
// verb unable to do the thing it is named after. A policy entry is a tree
// write we own, so removing it means the next handshake does not carry
// the grant — a revocation that is real, with a stated limit (it does not
// reach into a connection that already holds one).

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// ShareRequest is the renderer-neutral input to a share.
type ShareRequest struct {
	// Root is the local mount root being offered.
	Root string
	// Peer is who it is offered to — an alias or a bare peer-id.
	Peer string
	// Title is an optional human label; defaults to the root name.
	Title string
	// NowMillis is the authoring timestamp. Passed in rather than read
	// from the clock so a caller can make the record deterministic.
	NowMillis uint64
}

// ShareOutcome reports what the share established.
type ShareOutcome struct {
	Root         string
	TargetPrefix string
	PeerID       string
	PeerAlias    string
	PolicyPath   string
	GrantSummary string
	Audience     []string

	// Reconnected reports whether the connection was re-established so
	// the new grant is actually in force.
	//
	// This field exists because the alternative is a share that reports
	// success and does nothing until an unrelated restart. If it is
	// false, ReconnectNote says why, and the operator has to act.
	Reconnected   bool
	ReconnectNote string
}

// Share offers a mounted folder to one peer: writes the handshake policy
// that authorizes them, publishes the offer record that tells them what
// is on offer, and re-establishes the connection so the grant is live.
func (ws *ShellWorkspace) Share(req ShareRequest) (ShareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ShareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	root := sanitizeRootName(strings.TrimSpace(req.Root))
	if root == "" {
		return ShareOutcome{}, fmt.Errorf("no root name given")
	}
	peerID, alias, err := ws.resolvePeerRef(req.Peer)
	if err != nil {
		return ShareOutcome{}, err
	}
	if peerID == local.PeerID() {
		return ShareOutcome{}, fmt.Errorf("cannot share with this peer itself (%s)", peerID)
	}

	// The folder must actually be mounted here. Sharing a root we do not
	// publish would write an offer nobody can consume and a grant that
	// authorizes reading a prefix with nothing under it.
	if !hasLocalRoot(local, root) {
		return ShareOutcome{}, fmt.Errorf(
			"no local mount named %q to share — run `mount <dir> <prefix>` first", root)
	}
	targetPrefix := ""
	if b, ok := workbench.LoadMountBinding(local.Store(), root); ok {
		targetPrefix = b.TargetPrefix
	}

	// The offer's audience is cumulative across calls: sharing the same
	// folder with a second peer must not un-share it from the first.
	audience := []string{peerID}
	if existing, ok := findOffer(local.Store(), root); ok {
		audience = mergeAudience(existing.Audience, peerID)
	}

	// Record the DECLARATION before touching the substrate. From here on
	// the durable answer to "does this peer have this folder" is one
	// record, and the policy row below is derivable from it — which is
	// what lets a restart, or any later pass of the reconciler, put the
	// substrate back without an operator repeating this sequence.
	//
	// Declared first, and on purpose: if the policy write fails, having
	// declared the intent is the state we want to be left in. The reverse
	// order leaves authority granted for something nothing remembers
	// wanting.
	priorFolder, hadPriorFolder, err := ws.declareLocalShare(root, peerID, alias, req.NowMillis)
	if err != nil {
		return ShareOutcome{}, err
	}

	// DERIVED from the declaration, never written directly — see
	// ApplyDeclaredPolicy. Writing `SyncSenderGrants` here would replace
	// the whole row, and a peer we have also accepted a folder FROM needs
	// the receiver half of it: the second of the two verbs to run used to
	// revoke the first, so two-way sharing between one pair of machines
	// authorized in one direction only (AP68).
	if _, err := ws.ApplyDeclaredPolicy(peerID, "share: "+root); err != nil {
		ws.undeclareLocalShare(root, peerID, priorFolder, hadPriorFolder)
		return ShareOutcome{}, err
	}

	title := req.Title
	if title == "" {
		title = root
	}
	if err := workbench.SaveShareOffer(local.Store(), workbench.ShareOffer{
		Root:            root,
		Title:           title,
		TargetPrefix:    targetPrefix,
		Audience:        audience,
		CreatedAtMillis: req.NowMillis,
	}); err != nil {
		// Unwind the DECLARATION, and let the policy row be re-derived from
		// what is left — an authorization with no offer beside it is
		// authority nobody asked for and nothing records the reason for.
		//
		// Not `RemoveAccessPolicy`, which is what this did while the verb
		// owned the row: the row is now a union, so deleting it would also
		// revoke a folder we accepted FROM this peer and break a working
		// relationship because an unrelated one failed to record an offer.
		ws.undeclareLocalShare(root, peerID, priorFolder, hadPriorFolder)
		return ShareOutcome{}, err
	}

	reconnected, note := ws.refreshGrantConnection(peerID)

	return ShareOutcome{
		Root:          root,
		TargetPrefix:  targetPrefix,
		PeerID:        peerID,
		PeerAlias:     alias,
		PolicyPath:    workbench.AccessPolicyPrefix + peerID,
		GrantSummary:  workbench.SummarizeGrants(workbench.SyncSenderGrants()),
		Audience:      audience,
		Reconnected:   reconnected,
		ReconnectNote: note,
	}, nil
}

// UnshareOutcome reports what withdrawal actually did — and what it did
// not, which is the part a surface must not omit.
type UnshareOutcome struct {
	Root          string
	PeerID        string
	PolicyRemoved bool
	OfferRemoved  bool
	// StillOffered is the remaining audience for this root, so a surface
	// can say "withdrawn from A, still shared with B" rather than
	// implying the folder is now private.
	StillOffered []string
	// Caveat is the sentence a surface MUST print. Withdrawal stops the
	// NEXT handshake carrying the grant; it does not reach into a live
	// connection that already holds one.
	Caveat string
}

// Unshare withdraws one peer's access to one root.
func (ws *ShellWorkspace) Unshare(root, peer string) (UnshareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return UnshareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	root = sanitizeRootName(strings.TrimSpace(root))
	peerID, _, err := ws.resolvePeerRef(peer)
	if err != nil {
		return UnshareOutcome{}, err
	}
	if root == "" {
		return UnshareOutcome{}, fmt.Errorf("unshare needs a root name")
	}

	out := UnshareOutcome{
		Root:   root,
		PeerID: peerID,
		Caveat: "the grant is removed from the policy table, so the NEXT handshake " +
			"will not carry it; a connection that already holds it keeps it until " +
			"it is re-established.",
	}

	// Narrow the offer's audience rather than deleting the record, unless
	// this was the last member.
	if existing, ok := findOffer(local.Store(), root); ok {
		remaining := make([]string, 0, len(existing.Audience))
		for _, a := range existing.Audience {
			if a != peerID {
				remaining = append(remaining, a)
			}
		}
		out.StillOffered = remaining
		if len(remaining) == 0 {
			out.OfferRemoved = workbench.RemoveShareOffer(local.Store(), root)
		} else {
			existing.Audience = remaining
			if err := workbench.SaveShareOffer(local.Store(), existing); err != nil {
				return out, err
			}
		}
	}

	// Withdraw the DECLARATION first. Removing only the policy row leaves
	// a folder record still saying this peer is offered the folder, and
	// the next reconcile writes the grant straight back — so the
	// withdrawal would quietly reverse itself at the next `status` or the
	// next launch.
	if err := ws.declareWithdrawnShare(root, peerID); err != nil {
		return out, err
	}

	// Then re-derive the row. This drops the peer's policy exactly when
	// nothing declared authorizes them any more, which is a stronger and
	// narrower test than the one it replaces: `peerIsStillOffered` reads
	// only OFFERS, so unsharing the last folder from a peer we also
	// receive one FROM used to delete their delivery grant as well.
	removed, err := ws.WithdrawDeclaredPolicy(peerID, "unshare: "+root)
	if err != nil {
		return out, err
	}
	out.PolicyRemoved = removed
	return out, nil
}

// AcceptOutcome reports what accepting an offer established.
type AcceptOutcome struct {
	PeerID    string
	PeerAlias string
	Root      string

	// LocalRoot is the mount root the files land in. Equal to Root in
	// the symmetric case; different whenever the operator chose a
	// directory whose name is not what the sender called theirs.
	LocalRoot string

	// Mounted is the mount this accept CREATED, or nil when it received
	// into one that already existed.
	//
	// A pointer, so that "reused an existing mount" and "made a new one"
	// are distinguishable at a glance. They are different events for the
	// operator: one of them just bridged a directory on their disk, and
	// a surface that reports both identically is hiding the act it
	// should be confirming.
	Mounted *MountOutcome

	PolicyPath    string
	GrantSummary  string
	Sync          SyncOutcome
	Reconnected   bool
	ReconnectNote string

	// PublisherMustDial is the step the RECEIVER cannot perform and the
	// flow does not work without.
	//
	// Over a dial-by-address connection, authorization is one-directional
	// by design. `Connection.sendReciprocalGrant` is gated on
	// `EstablishedViaRendezvousKey()`, and the kernel states the reason:
	// *"a dial-by-address is asymmetric — one party requested service —
	// and §6.6's one-directional mint stands alone there."* The dialer
	// gains authority to originate to the acceptor; the acceptor gains
	// nothing.
	//
	// So accepting gets us authority to subscribe and fetch, and gives
	// the publisher nothing — and DELIVERY runs publisher→us. They must
	// dial us once, after this policy exists, or their notifications are
	// refused 403 and the folder stays empty with no error on our side.
	//
	// Carried as a field rather than left to prose because it is the one
	// step in the whole flow that neither verb can complete on its own.
	PublisherMustDial string
}

// AcceptRequest is the renderer-neutral input to an accept.
type AcceptRequest struct {
	// Peer is who is offering — an alias or a bare peer-id.
	Peer string

	// Root is THEIR mount root name, as `offers` reports it.
	Root string

	// Directory is where the files should land on this machine. When
	// set, Accept mounts it — creating the directory if it is not there
	// — and receives into it. When empty, Accept requires a mount
	// already named Root, which is what it always required.
	//
	// This field is the whole of step 6 of the nine-step flow. Until it
	// existed, `sync` refused without a local mount (correctly — a sync
	// that 404s on every delivery is worse than one that refuses), and
	// the only way to satisfy that was a separate `mount` step whose
	// directory basename had to match, by an unstated coupling, whatever
	// the SENDER had called their folder.
	Directory string

	// AllowNonEmpty proceeds when Directory already contains files.
	//
	// The refusal it overrides is not fussiness. Incoming files
	// overwrite same-named local files and remote deletes propagate, so
	// pointing an accept at a directory that already has contents in it
	// is a destructive act — and the operator's mental model at that
	// moment is "choose somewhere to put these", not "choose something
	// to merge with". Naming the flag after the thing it permits, rather
	// than `Force`, keeps that visible at the call site.
	AllowNonEmpty bool
}

// NonEmptyDirectory is returned when Accept was asked to receive into a
// directory that already has files in it.
//
// Typed, for MountConflict's reason: a shell prints it, and a panel wants
// to say "this folder has 12 items in it" beside a button that proceeds
// anyway.
type NonEmptyDirectory struct {
	Path    string
	Entries int
}

func (e *NonEmptyDirectory) Error() string {
	return fmt.Sprintf(
		"%s already has %d item(s) in it — accepting into a folder that is not empty lets "+
			"incoming files overwrite same-named local ones, and lets their deletes remove "+
			"yours. Choose an empty folder, or accept it anyway if this is the right one.",
		e.Path, e.Entries)
}

// AsNonEmptyDirectory reports whether err is (or wraps) a
// NonEmptyDirectory, and hands back the typed value.
func AsNonEmptyDirectory(err error) (*NonEmptyDirectory, bool) {
	var ne *NonEmptyDirectory
	if errors.As(err, &ne) {
		return ne, true
	}
	return nil, false
}

// Accept is the receiving side's one step: put a mount where the files
// will land, write the delivery policy that lets the publisher reach our
// blob-resolve handler, re-establish the connection so it is in force,
// then sync and catch up.
//
// The actions are one verb because they are not independently useful. A
// policy with no sync grants a stranger a handler they will never call; a
// sync with no policy is accepted and delivers nothing; and a sync with
// no mount 404s on every delivery while reporting itself healthy.
func (ws *ShellWorkspace) Accept(req AcceptRequest) (AcceptOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return AcceptOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	peerID, alias, err := ws.resolvePeerRef(req.Peer)
	if err != nil {
		return AcceptOutcome{}, err
	}
	root := sanitizeRootName(strings.TrimSpace(req.Root))
	if root == "" {
		return AcceptOutcome{}, fmt.Errorf("accept needs a root name (try `offers %s`)", req.Peer)
	}

	// Put the receiving mount in place BEFORE anything durable is
	// written. It is the step most likely to fail for an ordinary reason
	// — a path that does not exist, a folder with files already in it, a
	// basename that collides with another mount — and every one of those
	// is better met before this peer has granted anybody anything.
	localRoot, mounted, err := ws.prepareReceivingMount(root, req)
	if err != nil {
		return AcceptOutcome{}, err
	}

	// Declare before establishing, same as Share — see declareLocalShare.
	if err := ws.declareAcceptedFolder(peerID, alias, root, localRoot); err != nil {
		return AcceptOutcome{}, err
	}

	// Derived from the declaration, for the reason given in Share: this
	// row is a union across both directions, and writing only the
	// receiver half here revoked the sender half of a folder we share
	// back to the same peer (AP68).
	if _, err := ws.ApplyDeclaredPolicy(peerID, "accept: deliveries for "+root); err != nil {
		return AcceptOutcome{}, err
	}

	reconnected, note := ws.refreshGrantConnection(peerID)

	syncOut, err := ws.Sync(SyncRequest{Remote: peerID, Root: root, TargetRoot: localRoot})
	if err != nil {
		// Leave the policy in place on failure and say so in the error:
		// the operator is about to fix the cause and retry, and removing
		// the grant would make the retry fail for a second, different
		// reason. The mount is left in place for the same reason, and
		// because it is a directory on their disk that they named.
		return AcceptOutcome{}, fmt.Errorf(
			"%w\n(the delivery grant for %s was written and is left in place; "+
				"re-run accept once the cause above is fixed)", err, peerID)
	}

	return AcceptOutcome{
		PeerID:        peerID,
		PeerAlias:     alias,
		Root:          root,
		LocalRoot:     localRoot,
		Mounted:       mounted,
		PolicyPath:    workbench.AccessPolicyPrefix + peerID,
		GrantSummary:  workbench.SummarizeGrants(workbench.SyncReceiverGrants()),
		Sync:          syncOut,
		Reconnected:   reconnected,
		ReconnectNote: note,
		PublisherMustDial: fmt.Sprintf(
			"connect %s <this-peer's host:port>   (run on %s)", local.PeerID(), peerID),
	}, nil
}

// prepareReceivingMount makes sure there is somewhere for the incoming
// files to land, and reports which local mount root that is.
//
// Three cases, and the distinction between the last two is the whole
// safety story:
//
//   - **No directory named.** The caller is using the old shape, so a
//     mount named after THEIR root must already exist. Unchanged.
//   - **A directory that is already mounted.** Reuse it. The operator
//     made the decision to bridge that directory when they mounted it,
//     and re-accepting after a restart, or accepting a second folder
//     into the same place, must not ask again or refuse.
//   - **A directory that is not mounted.** Mount it — and refuse first
//     if it already has files in it, because from here on their deletes
//     remove the operator's files and their writes overwrite them.
//
// Creating the directory when it is absent is deliberate and is not the
// same kind of act: the operator typed a path that does not exist, which
// only means one thing.
func (ws *ShellWorkspace) prepareReceivingMount(theirRoot string, req AcceptRequest) (localRoot string, mounted *MountOutcome, err error) {
	local := ws.Local.Peer
	dir := strings.TrimSpace(req.Directory)

	if dir == "" {
		if !hasLocalRoot(local, theirRoot) {
			return "", nil, fmt.Errorf(
				"no local mount named %q to receive into — give a directory "+
					"(`accept <peer> %s <directory>`) and it will be created and mounted for you",
				theirRoot, theirRoot)
		}
		return theirRoot, nil, nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s: %w", dir, err)
	}
	info, statErr := os.Stat(absDir)
	switch {
	case os.IsNotExist(statErr):
		if err := os.MkdirAll(absDir, 0o755); err != nil {
			return "", nil, fmt.Errorf("create %s: %w", absDir, err)
		}
	case statErr != nil:
		return "", nil, fmt.Errorf("stat %s: %w", absDir, statErr)
	case !info.IsDir():
		return "", nil, fmt.Errorf("%s is not a directory", absDir)
	}

	localRoot = sanitizeRootName(filepath.Base(absDir))
	if localRoot == "" {
		return "", nil, fmt.Errorf("could not derive a usable mount name from %s", absDir)
	}

	// Is this directory already mounted, under this name?
	if existing, ok := workbench.MountFilesystemRoot(local.Store(), localRoot); ok {
		if existing != absDir {
			// A DIFFERENT directory holds this basename. Refusing is the
			// only honest answer: mounting would collide, and silently
			// receiving into the other directory would put a stranger's
			// files somewhere the operator did not choose.
			return "", nil, fmt.Errorf(
				"the mount name %q is already taken by %s — accepting into %s would collide "+
					"with it; rename or move the folder you are accepting into",
				localRoot, existing, absDir)
		}
		return localRoot, nil, nil
	}

	if !req.AllowNonEmpty {
		n, err := countDirEntries(absDir)
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", absDir, err)
		}
		if n > 0 {
			return "", nil, &NonEmptyDirectory{Path: absDir, Entries: n}
		}
	}

	out, err := ws.Mount(MountRequest{
		FilesystemDir: absDir,
		TargetPrefix:  "archives/" + localRoot + "/",
	})
	if err != nil {
		return "", nil, fmt.Errorf("mount %s to receive into: %w", absDir, err)
	}
	return out.RootName, &out, nil
}

// countDirEntries counts what is in a directory without reading it all
// into memory — a receiving folder an operator points at by mistake can
// be their home directory.
func countDirEntries(dir string) (int, error) {
	f, err := os.Open(dir)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// DialableAddressFor reports an address that can actually be dialled to
// reach a peer, and where it came from ("connection-table" or
// "discovery"), or ("", "") when nothing is known.
//
// Exported so a surface can show the reciprocal-dial step as a state it
// can act on — with an address already resolved, or with a field to type
// one into — rather than discovering at click time that there was never
// anything to dial. The strength of the claim differs by source, which
// is why the source is returned rather than just the string.
func (ws *ShellWorkspace) DialableAddressFor(peerID string) (string, string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return "", ""
	}
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID && pc.Address != "" {
			return pc.Address, "connection-table"
		}
	}
	if addr := ws.dialableAddressFor(peerID); addr != "" {
		return addr, "discovery"
	}
	return "", ""
}

// CompleteShareOutcome reports the reciprocal dial — the last step of a
// share, and the only one that cannot be performed by the peer who needs
// it done.
type CompleteShareOutcome struct {
	PeerID    string
	PeerAlias string
	// Address is what was actually dialled, so a surface can show the
	// operator which of the several possible sources won.
	Address string
	// AddressSource is "given", "connection-table" or "discovery".
	// Distinguished because they are different strengths of claim: an
	// address we dialled before is known-good, an mDNS announcement is
	// what the peer says about itself, and a typed one is the operator's
	// assertion.
	AddressSource string
	Connected     bool

	// NeedsAddress is the one failure a surface must handle differently
	// from an error: there is nothing wrong, we simply do not know where
	// this peer listens. The remedy is an address field, not a retry.
	NeedsAddress bool
	Note         string
}

// CompleteShare performs the reciprocal dial from the SHARING side.
//
// # Why this operation exists at all
//
// Over a dial-by-address connection the kernel's reciprocal grant is
// gated on `EstablishedViaRendezvousKey()` — *"a dial-by-address is
// asymmetric — one party requested service"*. So when the receiver dials
// us to subscribe, they gain the right to originate to us and we gain
// nothing; and DELIVERY runs publisher→receiver. Until we dial them, our
// notifications are refused and their folder stays empty, with no error
// on either side.
//
// # Why the receiver cannot do it for us, and why we cannot do it silently
//
// The addresses are asymmetric too, and that asymmetry is the whole
// difficulty. The receiver always has ours (they dialled it). We
// frequently have nothing for them: `ConnectedPeers()` reports the
// OBSERVED remote address, which for their inbound connection is an
// ephemeral source port that looks dialable and is not (see
// dialableAddressFor). And we cannot read their self-published transport
// profile, because `SyncReceiverGrants` deliberately grants us exactly
// one thing — `workbench/blob-resolve:receive` — and reading their tree
// is not it. Widening that grant to make this automatic is a real option
// and a separate decision; it trades a scoped read on the receiver for
// removing this step.
//
// So the honest shape is: resolve an address if we can, and if we cannot,
// say so as a distinct outcome (`NeedsAddress`) rather than as a failure,
// so a surface can ask for one instead of reporting that something broke.
//
// A successful dial is REMEMBERED in the connection table, which is what
// makes this a one-time step per peer rather than a recurring chore.
func (ws *ShellWorkspace) CompleteShare(peer, address string) (CompleteShareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return CompleteShareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	peerID, alias, err := ws.resolvePeerRef(peer)
	if err != nil {
		return CompleteShareOutcome{}, err
	}
	if peerID == local.PeerID() {
		return CompleteShareOutcome{}, fmt.Errorf("cannot dial this peer itself (%s)", peerID)
	}

	out := CompleteShareOutcome{PeerID: peerID, PeerAlias: alias}

	addr := strings.TrimSpace(address)
	out.AddressSource = "given"
	if addr == "" {
		addr = ws.dialableAddressFor(peerID)
		// dialableAddressFor prefers the connection table over discovery,
		// so report which one answered rather than guessing.
		out.AddressSource = "discovery"
		for _, pc := range ws.Conns {
			if pc != nil && pc.PeerID == peerID && pc.Address == addr && addr != "" {
				out.AddressSource = "connection-table"
				break
			}
		}
	}
	if addr == "" {
		out.NeedsAddress = true
		out.AddressSource = ""
		out.Note = "no address is known for this peer — we have never dialled them and they " +
			"are not announcing on the local network. Their own app shows the address it " +
			"listens on."
		return out, nil
	}
	out.Address = addr

	// Disconnect first so the next handshake re-assembles the grant set.
	// This is the whole point of the operation: the policy the receiver
	// wrote is inert on a connection established before it existed.
	//
	// Ordering matters and was a live bug once: an earlier version of the
	// neighbouring reconnect tore the connection down BEFORE checking it
	// had somewhere to redial, which turned a share into an outage. The
	// address is resolved above, so by here there is something to dial.
	local.Disconnect(peerID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := local.Connect(ctx, addr)
	if err != nil {
		out.Note = fmt.Sprintf("dial %s failed: %v — the share is written and applies at "+
			"the next successful connection either side makes", addr, err)
		return out, nil
	}

	// Verify we reached the peer we meant to. An address can be stale or
	// simply wrong when it was typed, and connecting to the wrong peer
	// and reporting the share complete is worse than failing: the folder
	// stays empty and the surface says it should not be.
	if state := conn.ConnState(); state == nil || string(state.RemotePeerID) != peerID {
		got := "(no peer-id)"
		if state != nil && state.RemotePeerID != "" {
			got = string(state.RemotePeerID)
		}
		out.Note = fmt.Sprintf("dialled %s but reached %s, not this peer — check the address", addr, got)
		return out, nil
	}

	out.Connected = true
	ws.rememberAddress(peerID, addr)
	// And onto the declaration, so the address survives this process.
	// The GUI's "Complete connection" button is this path, and it was
	// losing the operator's address at exit exactly as `connect` was.
	ws.RememberDeviceAddress(peerID, addr)
	return out, nil
}

// rememberAddress records a known-good dial address for a peer so a
// later CompleteShare (or any reconnect) does not have to ask again.
// Updates an existing alias binding in place; otherwise binds a new one
// derived from the peer-id, and gives up quietly if that name is taken
// rather than inventing a second alias for a peer that already has one.
func (ws *ShellWorkspace) rememberAddress(peerID, addr string) {
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID {
			pc.Address = addr
			return
		}
	}
	short := peerID
	if len(short) > 8 {
		short = short[:8]
	}
	alias, err := NormalizeAlias(strings.ToLower(short))
	if err != nil || alias == "" || IsReservedAlias(alias) {
		return
	}
	if _, taken := ws.Conns[alias]; taken {
		return
	}
	ws.addConn(&PeerConn{
		Alias:   alias,
		Address: addr,
		PeerID:  peerID,
		Peer:    ws.Local.Peer,
	})
}

// Offers reads what a remote peer is publishing to us.
//
// A DISPATCHED read of a peer-qualified path, which is a remote read
// (AP11): `List("/{them}/app/share/records/")` routes to that peer and
// returns their tree, which is the point — we want their offers, not our
// mirror of them.
func (ws *ShellWorkspace) Offers(peer string) (offers []workbench.ShareOffer, err error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	peerID, _, err := ws.resolvePeerRef(peer)
	if err != nil {
		return nil, err
	}

	// Refresh OUR outbound connection first.
	//
	// The grant we hold on them was assembled during OUR handshake, and
	// reading their offers needs the `system/tree:get` their `share`
	// wrote into their policy table afterwards. Their own reconnect does
	// not help: we dispatch over the connection WE opened, and that one
	// still carries whatever we were granted when we opened it.
	//
	// The transferable rule, measured the hard way: **the peer that
	// DISPATCHES is the peer that must re-establish its connection.** A
	// reconnect performed by the grantER refreshes the reciprocal
	// direction, not the pooled outbound connection the grantEE actually
	// uses. Without this, `offers` returns 403 for a share that was
	// correctly written moments earlier.
	ws.refreshGrantConnection(peerID)

	prefix := "/" + peerID + "/" + workbench.ShareOfferPrefix
	entries, lerr := local.List(prefix)
	if lerr != nil {
		return nil, fmt.Errorf("read %s's offers: %w", peerID, lerr)
	}
	me := local.PeerID()
	for _, e := range entries {
		ent, ok, gerr := local.Get(e.Path)
		if gerr != nil || !ok {
			continue
		}
		offer, ok := workbench.DecodeRemoteShareOffer(e.Path, ent)
		if !ok {
			continue
		}
		// Only offers that name us. An offer to someone else is not ours
		// to see listed as available — and since the record is a label
		// rather than an authority, showing it would promise nothing the
		// grant backs.
		if offer.OfferedTo(me) {
			offers = append(offers, offer)
		}
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].Root < offers[j].Root })
	return offers, nil
}

// DiscoveredPeer is one candidate as a surface should show it.
type DiscoveredPeer struct {
	PeerID    string
	Address   string
	Alias     string
	Connected bool
	// Source is how we know about this peer — "discovery" (announced on
	// the local network) or "connected" (we hold a connection). Stated
	// because they are different claims: one is an advertisement, the
	// other is a fact.
	Source string
}

// Peers is the discover stage: everything we can currently see, from
// mDNS announcements and from the connection pool, merged.
func (ws *ShellWorkspace) Peers() ([]DiscoveredPeer, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	byID := map[string]*DiscoveredPeer{}

	for _, c := range local.ConnectedPeers() {
		if c.PeerID == "" {
			continue
		}
		// A DIALABLE address, never the pool's observed one, when we have
		// one. `PeerInfo.Address` is the connection's remote address, and
		// for an INBOUND connection that is the dialer's ephemeral source
		// port — `192.168.68.160:50026`, which looks exactly like an
		// address and is refused by everything that tries it. The same
		// trap dialableAddressFor's own comment records; this surface was
		// still printing it, and it is the address an operator copies.
		addr := ws.dialableAddressFor(c.PeerID)
		if addr == "" {
			addr = c.Address
		}
		byID[c.PeerID] = &DiscoveredPeer{
			PeerID:    c.PeerID,
			Address:   addr,
			Alias:     ws.AliasFor(c.PeerID),
			Connected: true,
			Source:    "connected",
		}
	}

	if local.DiscoveryEnabled() {
		for _, cand := range local.ReadDiscoveredCandidates() {
			// entitysdk.CandidatePeerID, never cand.PeerID — the third
			// site of the same dead read. That field is empty on every
			// mDNS candidate (see CandidatePeerID), so `id == ""` was true
			// every time and **the `peers` verb has never listed a single
			// discovered peer** — it only ever showed the connection pool
			// under a heading that promised discovery. The GUI's Nearby
			// panel looked fine throughout, because the bridge reads the
			// TXT hint directly.
			id := entitysdk.CandidatePeerID(cand)
			if id == "" || id == local.PeerID() {
				continue
			}
			if existing, ok := byID[id]; ok {
				if existing.Address == "" {
					existing.Address = candidateAddress(cand)
				}
				continue
			}
			byID[id] = &DiscoveredPeer{
				PeerID:  id,
				Address: candidateAddress(cand),
				Alias:   ws.AliasFor(id),
				Source:  "discovery",
			}
		}
	}

	out := make([]DiscoveredPeer, 0, len(byID))
	for _, p := range byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Connected != out[j].Connected {
			return out[i].Connected
		}
		return out[i].PeerID < out[j].PeerID
	})
	return out, nil
}

// --- internals ------------------------------------------------------

// resolvePeerRef turns an alias or peer-id into (peerID, alias).
func (ws *ShellWorkspace) resolvePeerRef(ref string) (peerID, alias string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("no peer given")
	}
	if pc, ok := ws.Conns[ref]; ok && pc != nil {
		return pc.PeerID, pc.Alias, nil
	}
	return ref, ws.AliasFor(ref), nil
}

// refreshGrantConnection re-establishes the connection to a peer so a
// freshly written policy is actually in force.
//
// The grant a peer holds is assembled during the handshake, so a policy
// written afterwards has no effect until the next one. Measured:
// `shellboot/policy_probe_test.go::TestProbe_PolicyWrittenAfterConnect`
// — sync is refused 403 with both policies present and written late.
//
// Returns whether it managed it and why not. It does NOT return an error:
// failing to reconnect leaves the policy correctly written and the grant
// pending, which is a state worth reporting rather than one worth
// unwinding a share for.
func (ws *ShellWorkspace) refreshGrantConnection(peerID string) (bool, string) {
	local := ws.Local.Peer

	addr := ws.dialableAddressFor(peerID)
	if addr == "" {
		// **Deliberately does not disconnect.** An earlier version tore
		// the connection down first and then discovered it had nothing to
		// dial, which left the peer disconnected — strictly worse than
		// doing nothing, and it turned a share into an outage.
		return false, "no dialable address is known for this peer, so the connection " +
			"was left alone — the grant applies to the next connection either side " +
			"makes (`connect <alias> <host:port>` forces one now)"
	}

	local.Disconnect(peerID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := local.Connect(ctx, addr); err != nil {
		return false, fmt.Sprintf(
			"reconnect to %s failed (%v) — the policy is written and the grant "+
				"applies at the next successful connection", addr, err)
	}
	return true, ""
}

// dialableAddressFor returns an address that can actually be dialled to
// reach a peer, or "".
//
// **`AppPeer.ConnectedPeers()` is not a source for this**, which is the
// trap: its Address is the connection's OBSERVED remote address, and for
// an INBOUND connection that is the dialer's ephemeral source port. It
// looks exactly like a real address — `127.0.0.1:50026` — and dialling it
// is refused. Measured while writing the flow test, where the reconnect
// failed against a port that had never been listening.
//
// The two honest sources, in preference order:
//
//  1. the address the workspace recorded when WE dialled them, via the
//     `connect` verb. Known-dialable because it worked once.
//  2. the peer's own mDNS announcement, which advertises the address it
//     LISTENS on rather than one it happened to send from.
func (ws *ShellWorkspace) dialableAddressFor(peerID string) string {
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID && pc.Address != "" {
			return pc.Address
		}
	}
	// The DECLARATION, which is the only one of these three sources that
	// survives the process. Source 1 above is this session's connections
	// and source 3 below is a live announcement; without this, an address
	// the operator typed yesterday is gone today, and the reconciler has
	// nothing to dial with after a restart.
	if d, ok := workbench.LoadDevice(ws.Local.Peer.Store(), peerID); ok {
		if addr := d.PreferredAddress(); addr != "" {
			return addr
		}
	}
	return ws.discoveredAddressFor(peerID)
}

// discoveredAddressFor is the live-announcement arm, shared with the
// reconciler's dial ladder.
//
// **entitysdk.CandidatePeerID, never cand.PeerID.** The comparison this
// used to make — `cand.PeerID != peerID` — could not match anything: the
// field is empty on every mDNS candidate (see CandidatePeerID), so the
// discovery fallback was dead code wherever it appeared. It appeared in
// both places that needed it.
func (ws *ShellWorkspace) discoveredAddressFor(peerID string) string {
	local := ws.Local.Peer
	if !local.DiscoveryEnabled() {
		return ""
	}
	for _, cand := range local.ReadDiscoveredCandidates() {
		if entitysdk.CandidatePeerID(cand) != peerID {
			continue
		}
		if addr := entitysdk.DialAddressForCandidate(cand); addr != "" {
			return addr
		}
	}
	return ""
}

func findOffer(st *workbench.Store, root string) (workbench.ShareOffer, bool) {
	offers, _ := workbench.LoadShareOffers(st)
	for _, o := range offers {
		if o.Root == root {
			return o, true
		}
	}
	return workbench.ShareOffer{}, false
}

// peerIsStillOffered is deliberately GONE, not kept for a future caller.
//
// It answered "does this peer appear in any remaining offer", which was
// `Unshare`'s test for whether to drop their policy row — and offers
// describe only the OUTGOING direction, so it returned false for a peer
// actively delivering a folder to us and the row went with it. Its
// replacement asks the question that is actually being decided: does
// anything we have DECLARED still authorize this peer
// (`WithdrawDeclaredPolicy`). Leaving the old predicate in the file would
// leave the wrong question one autocomplete away from its right-looking
// name.

func mergeAudience(existing []string, add string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+1)
	for _, a := range append(append([]string(nil), existing...), add) {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func candidateAddress(c types.CandidateData) string {
	// The dial-address choice lives in the SDK, shared with the Avalonia
	// bridge — it is a substrate judgement about mDNS announcements, not
	// something either renderer owns.
	return entitysdk.DialAddressForCandidate(c)
}

// SharesOffered lists this peer's own outgoing offers.
func (ws *ShellWorkspace) SharesOffered() ([]workbench.ShareOffer, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	return workbench.LoadShareOffers(ws.Local.Peer.Store())
}

// AccessPolicies lists every peer this peer has authorized.
func (ws *ShellWorkspace) AccessPolicies() ([]workbench.AccessPolicy, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	// The peer's own identity hash in hex is the key the kernel's §6.9a
	// seed entry uses, and passing it is what lets a surface say "this
	// row is you" instead of showing a wildcard grant to an unexplained
	// hex string.
	return workbench.ListAccessPolicies(ws.Local.Peer.Store(),
		hex.EncodeToString(ws.Local.Peer.IdentityHash().Bytes()))
}
