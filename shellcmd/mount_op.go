package shellcmd

// Mount / Unmount as workspace OPERATIONS rather than shell verbs.
//
// This file is the extraction AGENTS.md asks for: *"DRY the integration,
// not the renderer. Shared shell↔workspace wiring lives once in
// shellcmd/integration.go; renderers add one line of wiring. Same closure
// in two renderers = extract it."* Until now the whole mount pipeline —
// validate the target, mint the scoped chain capability, register the
// source→target mapping, persist the RootConfig, subscribe, start the
// watcher, record the subscription — lived inside `cmdMount`, reachable
// only by typing a sentence at a shell prompt. The Avalonia Local Files
// panel could list mounts and could not make one, which is D23's shape
// with the model half-built rather than unbuilt: `make reachability`
// passes because the model *has* a surface, and a read-only surface for a
// read-write model is invisible to that sweep.
//
// The split is deliberate about where the seam falls. Everything that
// decides *what happens* is here. Everything that decides *how it is
// said* — flag parsing, usage text, the line-oriented result — stays in
// cmd_local_files.go. A renderer that wants a form with a directory
// picker and a checkbox calls Mount; it does not build an argv.
//
// Ordering is load-bearing and is preserved verbatim from the verb,
// including the two comments that explain why:
//
//   - subscribe BEFORE StartWatching, because the watcher's initial scan
//     writes synchronously and anything written before the subscription
//     is live never reaches the target prefix;
//   - unwind in reverse on every failure, so a half-mount does not leave
//     an ingest registration or a live subscription behind.
//
// The one thing that is NOT preserved is the conflict path's shape. The
// verb built a formatted multi-line error and returned it as a string,
// which is fine for a terminal and useless to a panel that wants to show
// the operator what is in the way and offer to proceed. It is now a typed
// MountConflict carrying the validation result, whose Error() renders the
// exact text the verb used to produce — so the shell's output is
// unchanged and a renderer can reach the structure behind it.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// DefaultMountExclude is the canonical "don't ingest these" list applied
// to a mount whose caller did not name an exclude set. Exported because a
// renderer offering a mount form has to be able to SHOW the operator what
// they are getting by default — a filter applied silently is a filter the
// operator will later be surprised by, and the shell at least prints it
// back in the mount result.
//
// Filename-only (filepath.Match) patterns, matched per path level, so
// ".git" prunes the whole subtree.
func DefaultMountExclude() []string {
	return append([]string(nil), defaultMountExclude...)
}

// MountRequest is the renderer-neutral input to a mount.
type MountRequest struct {
	// FilesystemDir is the host directory to bridge. Relative paths are
	// resolved against the process working directory, which is the
	// caller's business and not ours to second-guess.
	FilesystemDir string

	// TargetPrefix is the tree prefix the ingested documents land under.
	// A missing trailing "/" is added.
	TargetPrefix string

	// Include is the admit-list of filename globs; empty admits
	// everything not excluded.
	Include []string

	// Exclude is the deny-list. Nil AND ExcludeSet false means "apply
	// DefaultMountExclude". A caller who genuinely wants no exclusions at
	// all sets ExcludeSet with a nil/empty Exclude — the distinction
	// exists because "said nothing" and "said none" are different
	// intentions and collapsing them would silently ingest a .git
	// directory.
	Exclude    []string
	ExcludeSet bool

	// Force proceeds past a target-prefix type conflict.
	Force bool

	// ReadOnly suppresses tree→filesystem writeback for this mount:
	// changes on disk flow into the tree, and nothing the tree receives
	// is written back out. Syncthing calls this shape "Send Only" and it
	// is the most-used non-default folder type there.
	//
	// The field existed in the kernel's RootConfigData from the start and
	// this struct hard-coded `false`, so the whole mode was expressible
	// in the substrate and unreachable from every surface we ship — the
	// M1 finding's shape ("a capability that exists in a dependency is
	// not a capability of your product until something in your tree calls
	// it") with a field in place of a function, which is why the D23
	// sweep could not see it.
	ReadOnly bool

	// PublishDescriptors emits system/content/descriptor entities on read
	// (DOMAIN-LOCAL-FILES §10.5 V3). Same story: a persisted field with
	// no way to set it.
	PublishDescriptors bool
}

// MountOutcome describes what a successful mount established. Every
// field is a fact about the mount that now exists, so a surface can
// report the mount rather than repeat the request back.
type MountOutcome struct {
	RootName       string
	FilesystemRoot string
	SourcePrefix   string
	TargetPrefix   string
	Include        []string
	Exclude        []string
	CapabilityPath string
	HandlerPattern string
	SubscriptionID string
	// ReadOnly is echoed back because it changes what the mount DOES,
	// and a surface that accepted the flag without confirming it leaves
	// the operator to infer a write policy from silence.
	ReadOnly bool
}

// MountConflict is returned when the target prefix already holds
// bindings of a type this mount does not own and Force was not set.
//
// It is a typed error rather than a formatted string because the two
// consumers need different things from the same fact: the shell prints
// it, and a panel wants to list the offending types and offer a "mount
// anyway" button. Error() reproduces the shell's original text exactly,
// so the terminal surface did not change when this became structured.
type MountConflict struct {
	TargetPrefix string
	SourcePrefix string
	// ExpectedTypes is the set the mount owns — plural since the ingest
	// registry, and rendered as a set rather than a single name because
	// collapsing five types to one in the message would tell an operator
	// their code files were foreign to a mount that writes code files.
	ExpectedTypes []string
	Result        workbench.MountValidationResult
}

func (e *MountConflict) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mount aborted: %d existing binding(s) at %s with unexpected type(s):\n",
		e.Result.TargetTotal, e.TargetPrefix)
	for _, t := range e.Result.ForeignTypeOrder() {
		fmt.Fprintf(&b, "  %d  %s\n", e.Result.TargetForeign[t], t)
	}
	fmt.Fprintf(&b, "  (%d matching %s already present)\n",
		e.Result.TargetExpected, strings.Join(e.ExpectedTypes, " / "))
	if e.Result.SourceTotal > 0 {
		fmt.Fprintf(&b, "  also: %d binding(s) under %s from a prior mount\n",
			e.Result.SourceTotal, e.SourcePrefix)
	}
	fmt.Fprintf(&b, "re-run with -force to mount anyway")
	return b.String()
}

// AsMountConflict reports whether err is (or wraps) a MountConflict, and
// hands back the typed value. Convenience for renderers that would
// otherwise each write the errors.As dance.
func AsMountConflict(err error) (*MountConflict, bool) {
	var mc *MountConflict
	if errors.As(err, &mc) {
		return mc, true
	}
	return nil, false
}

// Mount bridges a host directory to a tree prefix through the ingest
// pipeline, and is the single implementation behind both the `mount`
// verb and any renderer's mount affordance.
//
// Steps, in the order they must happen:
//
//  1. resolve + stat the directory (a mount of a file, or of nothing, is
//     a refusal, not a mount of an empty set);
//  2. derive a stable root name from the basename;
//  3. validate the target prefix and refuse a type conflict unless Force;
//  4. mint the narrowest chain capability that authorizes the single op
//     the subscription dispatches;
//  5. register the source→target mapping with the ingest handler;
//  6. persist the RootConfig (the durable record reload reads);
//  7. subscribe — BEFORE the watcher, see the file header;
//  8. start the watcher;
//  9. record the subscription so Unmount can cancel it.
//
// Every failure after step 5 unwinds what preceded it.
func (ws *ShellWorkspace) Mount(req MountRequest) (MountOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return MountOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	lfHandler := local.LocalFilesHandler()
	if lfHandler == nil {
		return MountOutcome{}, fmt.Errorf("local/files extension is disabled on this peer")
	}
	ingestHandler := ws.NotificationIngest
	if ingestHandler == nil {
		return MountOutcome{}, fmt.Errorf("workbench notification-ingest handler not wired on this workspace")
	}

	if strings.TrimSpace(req.FilesystemDir) == "" {
		return MountOutcome{}, fmt.Errorf("no filesystem directory given")
	}
	if strings.TrimSpace(req.TargetPrefix) == "" {
		return MountOutcome{}, fmt.Errorf("no tree prefix given")
	}

	absDir, err := filepath.Abs(req.FilesystemDir)
	if err != nil {
		return MountOutcome{}, fmt.Errorf("resolve fs dir: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return MountOutcome{}, fmt.Errorf("stat %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return MountOutcome{}, fmt.Errorf("%s is not a directory", absDir)
	}

	targetPrefix := req.TargetPrefix
	if !strings.HasSuffix(targetPrefix, "/") {
		targetPrefix += "/"
	}

	// Derive a stable root name from the dir basename. Two different
	// directories with the same basename collide; AddRoot's overlap check
	// is what refuses that today.
	rootName := sanitizeRootName(filepath.Base(absDir))
	if rootName == "" {
		return MountOutcome{}, fmt.Errorf("could not derive a usable root name from %s", absDir)
	}
	sourcePrefix := "local/files/" + rootName + "/"

	excludePatterns := req.Exclude
	if !req.ExcludeSet {
		excludePatterns = DefaultMountExclude()
	}
	includePatterns := req.Include

	// Pre-mount validation: walk the target prefix and refuse if any
	// binding carries a type this mount does not own. Workbench owns the
	// whole `doc/*` document set there; anything else means the operator
	// is about to mount over someone else's state.
	//
	// This is the registry's set, not the single doc/markdown-file it
	// used to be, and the difference is not cosmetic: once ingest
	// classifies by extension, a mount of a mixed directory writes
	// doc/code-file and doc/image-file alongside the markdown, so the
	// pre-registry expected-set would have made a mount CONFLICT WITH
	// WHAT ITS OWN PREDECESSOR WROTE — remount the same directory and
	// the validator refuses on the evidence of the first mount's
	// success.
	expectedTypes := workbench.DocEntityTypes()
	vr := workbench.ValidateMountTarget(local, sourcePrefix, targetPrefix, expectedTypes)
	if vr.HasConflict() && !req.Force {
		return MountOutcome{}, &MountConflict{
			TargetPrefix:  targetPrefix,
			SourcePrefix:  sourcePrefix,
			ExpectedTypes: expectedTypes,
			Result:        vr,
		}
	}

	// Narrowest possible capability: the single receive op the
	// subscription dispatches. The handler's own grant does the
	// tree:get/put work inside its scope.
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		},
	}
	capPath := "system/capability/grants/chain/local-files/" + rootName
	if _, err := local.MintChainCapabilityBound(grants, capPath); err != nil {
		return MountOutcome{}, fmt.Errorf("mint chain cap: %w", err)
	}

	// In-memory source→target mapping. The RootConfig written below is
	// the durable half that drives reload at peer startup.
	ingestHandler.RegisterMount(sourcePrefix, targetPrefix)

	rootCfg := localfiles.RootConfigData{
		Prefix:             sourcePrefix,
		FilesystemRoot:     absDir,
		ReadOnly:           req.ReadOnly,
		Exclude:            excludePatterns,
		Include:            includePatterns,
		PublishDescriptors: req.PublishDescriptors,
	}
	if err := lfHandler.AddRoot(rootName, rootCfg, local.RawContentStore(), local.RawLocationIndex()); err != nil {
		ingestHandler.UnregisterMount(sourcePrefix)
		return MountOutcome{}, fmt.Errorf("add localfiles root: %w", err)
	}

	// Persist OUR half beside the kernel's. AddRoot has just written the
	// kernel's record; without this the source→target mapping exists
	// only in the ingest handler's memory, and a restart brings the
	// watcher back with nothing to route its events to. See
	// workbench/mount_binding.go.
	if err := workbench.SaveMountBinding(local.Store(), workbench.MountBindingData{
		Root:         rootName,
		SourcePrefix: sourcePrefix,
		TargetPrefix: targetPrefix,
	}); err != nil {
		ingestHandler.UnregisterMount(sourcePrefix)
		return MountOutcome{}, fmt.Errorf("persist mount binding: %w", err)
	}

	// Subscribe BEFORE starting the watcher. The watcher's initial scan
	// writes entities to sourcePrefix synchronously; if the subscription
	// is not live by then, those writes are missed and pre-existing files
	// never reach the target prefix.
	//
	// "deleted" is included so an fs unlink cascades into the ingest
	// handler's delete branch and removes the workbench-owned document at
	// the target prefix.
	localID := local.PeerID()
	deliverURI := fmt.Sprintf("entity://%s/%s", localID, workbench.NotificationIngestPattern)
	sub, err := local.SubscribeRawAt(localID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated", "deleted"}})
	if err != nil {
		workbench.RemoveMountBinding(local.Store(), rootName)
		ingestHandler.UnregisterMount(sourcePrefix)
		return MountOutcome{}, fmt.Errorf("subscribe to source prefix: %w", err)
	}

	if err := lfHandler.StartWatching(context.Background(), rootName, local.RawContentStore(),
		local.RawLocationIndex(), local.IdentityHash()); err != nil {
		_ = sub.Close()
		workbench.RemoveMountBinding(local.Store(), rootName)
		ingestHandler.UnregisterMount(sourcePrefix)
		return MountOutcome{}, fmt.Errorf("start watcher: %w", err)
	}

	ws.registerMountSub(rootName, sub)

	return MountOutcome{
		RootName:       rootName,
		FilesystemRoot: absDir,
		SourcePrefix:   sourcePrefix,
		TargetPrefix:   targetPrefix,
		Include:        includePatterns,
		Exclude:        excludePatterns,
		CapabilityPath: capPath,
		HandlerPattern: workbench.NotificationIngestPattern,
		SubscriptionID: sub.ID(),
		ReadOnly:       req.ReadOnly,
	}, nil
}

// UnmountOutcome reports what teardown managed to do, INCLUDING what it
// could not. SubscriptionCloseErr is carried rather than swallowed or
// promoted to a returned error: the ingest registration is gone either
// way, so the unmount succeeded in the sense that matters, and a surface
// that reported total failure because a subscription close complained
// would be lying in the more alarming direction.
type UnmountOutcome struct {
	RootName             string
	SubscriptionCloseErr error

	// WatcherStillRunning is always true, and says so on purpose.
	// core-go's localfiles.Handler exposes StartWatching and no
	// StopWatching / RemoveRoot, so the fsnotify watcher for this root
	// keeps running until the process exits. It is BOUNDED, not leaking:
	// StartWatching stops and replaces any existing watcher for the same
	// root name, so the ceiling is one live watcher per distinct root
	// ever mounted, not one per unmount.
	//
	// This field exists so a renderer states the limitation instead of
	// implying a clean teardown. An unmount that silently leaves a
	// watcher running is a surface asserting something it does not know
	// (AP45), and the operator finds out when their edits keep landing.
	WatcherStillRunning bool
}

// Unmount tears a mount down: drop the ingest mapping and cancel the
// subscription. Idempotent — an unknown root name is not an error,
// because "make sure this is not mounted" is a reasonable thing to ask
// twice.
//
// It deliberately does NOT delete the persisted RootConfig. The config is
// the record of what was last mounted and other tooling reads it; a
// purging variant is a separate verb, not a default.
func (ws *ShellWorkspace) Unmount(rootName string) (UnmountOutcome, error) {
	if strings.TrimSpace(rootName) == "" {
		return UnmountOutcome{}, fmt.Errorf("no root name given")
	}
	sourcePrefix := "local/files/" + rootName + "/"
	if ws.NotificationIngest != nil {
		ws.NotificationIngest.UnregisterMount(sourcePrefix)
	}
	// Drop the workbench binding too, or the next startup restores a
	// mapping for a mount the operator unmounted — an unmount that
	// undoes itself on restart is worse than one that does nothing,
	// because it happens later and out of sight.
	//
	// Note the asymmetry with the kernel's RootConfig, which is
	// deliberately left in place: that record says what was last
	// mounted and other tooling reads it. Ours says what is routing
	// right now, and nothing should be.
	if ws.Local != nil && ws.Local.Peer != nil {
		workbench.RemoveMountBinding(ws.Local.Peer.Store(), rootName)
	}
	subErr := ws.closeMountSub(rootName)
	return UnmountOutcome{
		RootName:             rootName,
		SubscriptionCloseErr: subErr,
		WatcherStillRunning:  true,
	}, nil
}
