package shellcmd

// Sync / Unsync as workspace OPERATIONS — the receiving half of a
// cross-peer folder sync, and the surface the engine never had.
//
// # What this closes
//
// `workbench.BlobResolveHandler` is the whole cross-peer materialization
// pipeline: unwrap the subscription delivery, pull the changed file
// entity out of the payload, drive the blob closure across from the
// source peer under a cap-checked sequencer, then dispatch
// `local/files:write` locally so the bytes land on disk. It has twelve
// test files behind it — one-way, bidirectional, burst writes, a 4 MB
// file, capability delegation, late join, self-loop.
//
// Every one of those registrations was in a `_test.go`. `shellboot`
// registered two handlers and this was not one of them, and
// `subscription` is `ls|inspect|rm` with no create, so nothing a user
// could run established the relationship that drives it. The engine was
// built, tested, and reachable from nothing — D23's shape at the handler
// layer, where `make reachability` does not look because the sweep asks
// whether a *model* has a surface.
//
// # The seam
//
// Same split as mount_op.go: everything that decides *what happens* is
// here, everything that decides *how it is said* stays in the verb. A
// renderer offering a "sync a folder from a peer" affordance calls Sync;
// it does not build an argv.
//
// # Ordering
//
// Load-bearing, and it is the mirror of Mount's:
//
//  1. resolve the remote peer (an alias is a local convenience; the
//     subscription needs the peer-id);
//  2. require a LOCAL mount at the target — the materialization step
//     dispatches `local/files:write`, which needs a root whose
//     filesystem directory the bytes can land in. Without it the sync
//     would establish cleanly and then 404 on every delivery, which is
//     the failure shape this repo has now shipped twice;
//  3. mint the narrowest chain capability — the single `receive` op the
//     subscription dispatches at blob-resolve;
//  4. register the source→target mapping on the handler;
//  5. persist OUR half, so a restart restores the routing;
//  6. subscribe to the remote prefix with include_payload.
//
// Every failure after step 4 unwinds what preceded it.

import (
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// SyncRequest is the renderer-neutral input to a sync.
type SyncRequest struct {
	// Remote is the peer to sync FROM — an alias from the workspace's
	// connection table, or a bare peer-id. An alias is resolved here so
	// that a renderer showing a peer picker can pass what it displays.
	Remote string

	// Root is the mount root name on BOTH sides. The remote's source
	// prefix is `local/files/{root}/`; ours is the same unless
	// TargetRoot says otherwise.
	Root string

	// TargetRoot, when set, is the local mount root the bytes are
	// written into, for the case where the two peers mounted the same
	// folder under different names. Empty means "the same name", which
	// is the symmetric case and by far the common one.
	TargetRoot string
}

// SyncOutcome describes the relationship that now exists. Every field is
// a fact about the sync rather than an echo of the request, so a surface
// can report what was established.
type SyncOutcome struct {
	RemotePeerID   string
	RemoteAlias    string
	Root           string
	SourcePrefix   string
	TargetPrefix   string
	TargetRoot     string
	CapabilityPath string
	HandlerPattern string
	SubscriptionID string
}

// Sync establishes the receiving half of a folder sync from a remote
// peer, and is the single implementation behind both the `sync` verb and
// any renderer's sync affordance.
func (ws *ShellWorkspace) Sync(req SyncRequest) (SyncOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return SyncOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	blobResolve := ws.BlobResolve
	if blobResolve == nil {
		return SyncOutcome{}, fmt.Errorf(
			"workbench blob-resolve handler not wired on this workspace")
	}

	remote := strings.TrimSpace(req.Remote)
	if remote == "" {
		return SyncOutcome{}, fmt.Errorf("no remote peer given")
	}
	root := sanitizeRootName(strings.TrimSpace(req.Root))
	if root == "" {
		return SyncOutcome{}, fmt.Errorf("no root name given")
	}

	// An alias is a local convenience and the subscription needs the
	// peer-id, so resolve here rather than making every caller do it.
	// A bare peer-id passes through unchanged, which is what lets a
	// renderer pass either.
	remotePeerID := remote
	remoteAlias := ""
	if pc, ok := ws.Conns[remote]; ok && pc != nil {
		remotePeerID = pc.PeerID
		remoteAlias = pc.Alias
	} else if a, ok := ws.peerMap[remote]; ok {
		remoteAlias = a
	}
	if remotePeerID == local.PeerID() {
		return SyncOutcome{}, fmt.Errorf(
			"cannot sync from this peer to itself (%s) — a mount already does that", remotePeerID)
	}

	targetRoot := sanitizeRootName(strings.TrimSpace(req.TargetRoot))
	if targetRoot == "" {
		targetRoot = root
	}

	sourcePrefix := "local/files/" + root + "/"
	targetPrefix := "local/files/" + targetRoot + "/"

	// The local mount must exist FIRST. Materialization dispatches
	// `local/files:write` at targetPrefix+relpath, which needs a root
	// whose FilesystemRoot the bytes can be written into. Establishing a
	// sync without one produces a relationship that looks healthy and
	// 404s on every delivery — the exact shape of the mount-binding bug
	// and of the ingest-mapping bug before it, and the third time is
	// where it becomes a check rather than a comment.
	lfHandler := local.LocalFilesHandler()
	if lfHandler == nil {
		return SyncOutcome{}, fmt.Errorf("local/files extension is disabled on this peer")
	}
	if !hasLocalRoot(local, targetRoot) {
		return SyncOutcome{}, fmt.Errorf(
			"no local mount named %q to receive into — run `mount <dir> <prefix>` on a "+
				"directory first (a sync writes into a mount, it does not create one)",
			targetRoot)
	}

	// Narrowest possible capability: the single receive op the
	// subscription dispatches. The handler's own grant does the
	// content-fetch and local-write work inside its scope.
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		},
	}
	capPath := "system/capability/grants/chain/blob-resolve/" + targetRoot
	if _, err := local.MintChainCapabilityBound(grants, capPath); err != nil {
		return SyncOutcome{}, fmt.Errorf("mint chain cap: %w", err)
	}

	blobResolve.RegisterMount(sourcePrefix, targetPrefix)

	// include_payload is what makes this one dispatch instead of two:
	// the changed FileData arrives with the notification, so the handler
	// never has to reach back for a tree:get before it knows which blob
	// to pull.
	deliverURI := fmt.Sprintf("entity://%s/%s", local.PeerID(), workbench.BlobResolvePattern)
	sub, err := local.SubscribeRawAt(remotePeerID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{
			Events:         []string{"created", "updated"},
			IncludePayload: true,
		})
	if err != nil {
		blobResolve.UnregisterMount(sourcePrefix)
		return SyncOutcome{}, fmt.Errorf("subscribe to %s on %s: %w", sourcePrefix, remotePeerID, err)
	}

	// Persist last, with the subscription id in hand, so the record is
	// never written describing a subscription that does not exist.
	if err := workbench.SaveSyncBinding(local.Store(), workbench.SyncBindingData{
		RemotePeerID:   remotePeerID,
		Root:           root,
		SourcePrefix:   sourcePrefix,
		TargetPrefix:   targetPrefix,
		SubscriptionID: sub.ID(),
	}); err != nil {
		_ = sub.Close()
		blobResolve.UnregisterMount(sourcePrefix)
		return SyncOutcome{}, fmt.Errorf("persist sync binding: %w", err)
	}

	ws.registerSyncSub(remotePeerID, root, sub)

	return SyncOutcome{
		RemotePeerID:   remotePeerID,
		RemoteAlias:    remoteAlias,
		Root:           root,
		SourcePrefix:   sourcePrefix,
		TargetPrefix:   targetPrefix,
		TargetRoot:     targetRoot,
		CapabilityPath: capPath,
		HandlerPattern: workbench.BlobResolvePattern,
		SubscriptionID: sub.ID(),
	}, nil
}

// UnsyncOutcome reports what teardown managed to do, including what it
// could not — same reasoning as UnmountOutcome: the routing is gone
// either way, so a surface that reported total failure because a
// subscription close complained would be lying in the more alarming
// direction.
type UnsyncOutcome struct {
	RemotePeerID         string
	Root                 string
	Found                bool
	SubscriptionCloseErr error
}

// Unsync tears down one inbound sync: drop the blob-resolve mapping,
// cancel the subscription, remove the persisted binding.
//
// Idempotent — an unknown pair is not an error, because "make sure this
// is not syncing" is a reasonable thing to ask twice.
//
// It deliberately does NOT unmount the local root or delete anything
// already materialized. Bytes that arrived are the operator's files now,
// and a teardown that removed them would be a destructive act nobody
// asked for.
func (ws *ShellWorkspace) Unsync(remote, root string) (UnsyncOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return UnsyncOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	remotePeerID := strings.TrimSpace(remote)
	if pc, ok := ws.Conns[remotePeerID]; ok && pc != nil {
		remotePeerID = pc.PeerID
	}
	root = sanitizeRootName(strings.TrimSpace(root))
	if remotePeerID == "" || root == "" {
		return UnsyncOutcome{}, fmt.Errorf("unsync needs a remote peer and a root name")
	}

	out := UnsyncOutcome{RemotePeerID: remotePeerID, Root: root}

	binding, found := workbench.LoadSyncBinding(local.Store(), remotePeerID, root)
	out.Found = found

	if ws.BlobResolve != nil {
		src := binding.SourcePrefix
		if src == "" {
			src = "local/files/" + root + "/"
		}
		ws.BlobResolve.UnregisterMount(src)
	}

	if sub := ws.takeSyncSub(remotePeerID, root); sub != nil {
		if err := sub.Close(); err != nil {
			out.SubscriptionCloseErr = err
		}
	}

	workbench.RemoveSyncBinding(local.Store(), remotePeerID, root)
	return out, nil
}

// SyncRow is one established sync as a surface should show it.
type SyncRow struct {
	RemotePeerID string
	RemoteAlias  string
	Root         string
	SourcePrefix string
	TargetPrefix string
	// Live reports whether this process holds the subscription handle.
	// A restored sync is routed and persistent but its handle lives in
	// the kernel's engine rather than in this workspace, so `false` here
	// means "restored, not broken" and a surface must not call it dead.
	Live bool
}

// Syncs lists every established inbound sync, sorted, with the problems
// LoadSyncBindings could not decode.
func (ws *ShellWorkspace) Syncs() (rows []SyncRow, problems []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	bindings, problems := workbench.LoadSyncBindings(ws.Local.Peer.Store())
	for _, b := range bindings {
		alias := ""
		if a, ok := ws.peerMap[b.RemotePeerID]; ok {
			alias = a
		}
		_, live := ws.syncSubs[workbench.SyncBindingKey(b.RemotePeerID, b.Root)]
		rows = append(rows, SyncRow{
			RemotePeerID: b.RemotePeerID,
			RemoteAlias:  alias,
			Root:         b.Root,
			SourcePrefix: b.SourcePrefix,
			TargetPrefix: b.TargetPrefix,
			Live:         live,
		})
	}
	return rows, problems
}

// hasLocalRoot reports whether the local peer has a local/files root of
// this name. Reads the kernel's own config prefix, which is the record
// AddRoot writes and Load rehydrates.
func hasLocalRoot(local *entitysdk.AppPeer, root string) bool {
	for _, e := range local.Store().List(workbench.MountConfigPrefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		name, under := workbench.RelativeUnder(e.Path, workbench.MountConfigPrefix)
		if under && name == root {
			return true
		}
	}
	return false
}
