package shellcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// The shell commands in this file bridge a host filesystem
// directory to a revision-tracked tree prefix. They're DELIBERATELY
// not named after the underlying `local/files` handler — that
// handler does the lower-level watch/read/write/list/delete
// operations against a single mount; the bridge commands here
// compose it with the workbench's ingest-transform handler, a
// scoped chain capability, and on_error routing to produce a
// higher-level "mount this directory, propagate changes through
// the revision system" behavior.
//
// Top-level commands (registered in commands.go):
//
//	mount <fs-dir> <tree-prefix>
//	    Install the bridge. Builds the 3-step continuation chain
//	    (read → transform → put), persists the localfiles
//	    RootConfig, starts the fsnotify watcher, installs the
//	    subscription that drives the chain.
//
//	unmount <name>
//	    Tear down the bridge — abandon the chain continuations.
//	    (Subscription cleanup is a Phase E v1 limitation; see
//	    cmdUnmount for the TODO.)
//
//	mounts
//	    List currently-mounted bridges from the tree-resident
//	    `system/config/local/files/*` config.

// defaultMountExclude is the canonical "don't ingest these" list
// applied to every mount unless the caller passes -exclude to
// override. Covers VCS metadata, package-manager artifacts, common
// build outputs, and large binary artifacts that the watcher would
// otherwise read into FileData entities and OOM the peer on. The list
// is filename-only (filepath.Match patterns); paths are matched by
// basename at each level, so ".git" prunes the entire directory
// subtree.
var defaultMountExclude = []string{
	".git",
	".svn",
	".hg",
	"node_modules",
	"target",
	"dist",
	"build",
	"vendor",
	".venv",
	"__pycache__",
	"*.exe",
	"*.bin",
	"*.so",
	"*.dylib",
	"*.dll",
	"*.a",
	"*.o",
	"*.class",
	"*.pyc",
}

// cmdMount: wire a filesystem dir to a tree prefix via the
// Phase E ingest pipeline.
//
// Usage:
//
//	mount <fs-dir> <tree-prefix> [-include "*.md,*.txt"] [-exclude "vendor,*.log"]
//
// Filters: -include and -exclude take comma-separated filepath.Match
// globs. Include defaults to empty (admit everything not excluded);
// Exclude defaults to a curated VCS/build-artifact list (see
// defaultMountExclude). Passing -exclude REPLACES the defaults — if
// you want to extend them, restate the defaults explicitly in your
// flag. (This is intentional: callers should know exactly what's
// filtered.)
//
// Revision auto-versioning is a separate concern handled by
// `revision config put <name> <prefix> -auto` — run that after the
// mount when you want writes under the target prefix to produce
// revisions. The two verbs are intentionally separate because
// "filesystem-sync this prefix" and "version-track this prefix" are
// orthogonal: you might want one without the other.
//
// End to end:
//  1. Validate the filesystem dir exists.
//  2. Pick a root name (slug from dir basename) and target prefix
//     (caller-supplied), and confirm neither collides with an
//     existing mount.
//  3. Mint a scoped chain capability authorizing only the
//     notification-ingest receive op.
//  4. Register the workbench source→target mapping.
//  5. Persist the RootConfig (with include/exclude) in the tree.
//  6. Subscribe on local/files/{root}/* with delivery to the
//     workbench notification-ingest handler.
//  7. Start the fsnotify watcher — initial scan + event loop
//     respect include + exclude.
//
// Restart-equivalence: the RootConfig in the tree carries the
// filters, so a reopen reload sees the same set of files.
func cmdMount(sh *Shell, args []string) (Result, error) {
	// Sub-verb dispatch. Backward-compat: the legacy positional form
	// `mount <fs-dir> <tree-prefix>` keeps working as the first arg
	// is a path, not one of the reserved sub-verb literals.
	if len(args) >= 1 {
		switch args[0] {
		case "sweep":
			return cmdMountSweep(sh, args[1:])
		case "include":
			return cmdMountFilterUpdate(sh, args[1:], "include")
		case "exclude":
			return cmdMountFilterUpdate(sh, args[1:], "exclude")
		case "filter":
			return cmdMountFilterShow(sh, args[1:])
		}
	}
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: mount <fs-dir> <tree-prefix> [-include PATTERNS] [-exclude PATTERNS] [-force]\n   or: mount sweep <root> [-add]\n   or: mount include|exclude <root> PATTERNS\n   or: mount filter <root>")
	}

	// Strip flags out of args so the positional parse below stays
	// minimal. -include / -exclude take a single comma-separated value.
	var includePatterns, excludePatterns []string
	excludeSet := false
	force := false
	positional := args[:0:0]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-include":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-include requires a value")
			}
			includePatterns = splitFilterPatterns(args[i+1])
			i++
		case "-exclude":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-exclude requires a value")
			}
			excludePatterns = splitFilterPatterns(args[i+1])
			excludeSet = true
			i++
		case "-force":
			force = true
		default:
			positional = append(positional, a)
		}
	}
	if !excludeSet {
		excludePatterns = append([]string(nil), defaultMountExclude...)
	}
	for _, p := range positional {
		if p == "-auto" {
			return Result{}, fmt.Errorf("-auto is not a mount flag; run `revision config put <name> <prefix> -auto` after the mount to enable auto-versioning")
		}
		if strings.HasPrefix(p, "-") {
			return Result{}, fmt.Errorf("unknown flag: %s (mount accepts -include, -exclude, -force)", p)
		}
	}
	if len(positional) < 2 {
		return Result{}, fmt.Errorf("usage: mount <fs-dir> <tree-prefix> [-include PATTERNS] [-exclude PATTERNS]")
	}
	if len(positional) > 2 {
		return Result{}, fmt.Errorf("too many arguments (got %d positional): expected <fs-dir> <tree-prefix>", len(positional))
	}
	fsDir, targetPrefix := positional[0], positional[1]

	// Normalize.
	absDir, err := filepath.Abs(fsDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve fs dir: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return Result{}, fmt.Errorf("stat %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("%s is not a directory", absDir)
	}
	if !strings.HasSuffix(targetPrefix, "/") {
		targetPrefix += "/"
	}

	// Derive a stable root name from the dir basename. Multiple mounts
	// of different dirs with the same basename would collide; that's
	// a future-flag (v1 errors at AddRoot's overlap check below).
	rootName := sanitizeRootName(filepath.Base(absDir))
	if rootName == "" {
		return Result{}, fmt.Errorf("could not derive a usable root name from %s", absDir)
	}

	local := sh.Local.Peer
	lfHandler := local.LocalFilesHandler()
	if lfHandler == nil {
		return Result{}, fmt.Errorf("local/files extension is disabled on this peer")
	}
	ingestHandler := sh.NotificationIngest
	if ingestHandler == nil {
		return Result{}, fmt.Errorf("workbench notification-ingest handler not wired on this shell (Phase E Q2 dependency)")
	}

	localID := local.PeerID()
	sourcePrefix := "local/files/" + rootName + "/"

	// Phase E v2 §7.4 — pre-mount validation. Walk the target prefix
	// and warn if any binding has an unexpected type. Workbench owns
	// doc/markdown-file at the target today; other types (hand-put
	// markdown content, leftovers from a different extension, stale
	// state from a different mount shape) signal a conflict that the
	// operator should consciously override.
	expectedTypes := []string{workbench.MarkdownFileType}
	vr := workbench.ValidateMountTarget(local, sourcePrefix, targetPrefix, expectedTypes)
	if vr.HasConflict() && !force {
		var b strings.Builder
		fmt.Fprintf(&b, "mount aborted: %d existing binding(s) at %s with unexpected type(s):\n", vr.TargetTotal, targetPrefix)
		for _, t := range vr.ForeignTypeOrder() {
			fmt.Fprintf(&b, "  %d  %s\n", vr.TargetForeign[t], t)
		}
		fmt.Fprintf(&b, "  (%d matching %s already present)\n", vr.TargetExpected, workbench.MarkdownFileType)
		if vr.SourceTotal > 0 {
			fmt.Fprintf(&b, "  also: %d binding(s) under %s from a prior mount\n", vr.SourceTotal, sourcePrefix)
		}
		fmt.Fprintf(&b, "re-run with -force to mount anyway")
		return Result{}, fmt.Errorf("%s", b.String())
	}

	// Mint a scoped chain capability authorizing only the single op
	// the subscription dispatches: workbench/ingest-from-notification
	// receive. Narrowest possible cap — the handler's internal scope
	// does the tree:get/put work under its own grant.
	//
	// Why a single-handler shape here (not a 3-step chain): the
	// notification's URI is qualified (`/{peerID}/local/files/...`)
	// while system/tree:get wants an unqualified path. Chain
	// `resource_extract` is dotted-path navigation, not string
	// transformation — it can't strip the peer-id prefix. Plus
	// step 3's target path is `{target_prefix}+{relpath}` which
	// requires either threading both prefixes through Params or
	// a string-transform step. The single-handler shape solves
	// URI normalization and path mapping in one place. See
	// `workbench/notification_ingest.go` docstring for the full
	// reckoning.
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		},
	}
	capPath := "system/capability/grants/chain/local-files/" + rootName
	if _, err := local.MintChainCapabilityBound(grants, capPath); err != nil {
		return Result{}, fmt.Errorf("mint chain cap: %w", err)
	}

	// Register the source→target mapping with the workbench ingest
	// handler. The handler holds this in-memory; the
	// system/config/local/files/{rootName} entity (written by
	// AddRoot below) is the durable record that drives reload at
	// peer startup.
	ingestHandler.RegisterMount(sourcePrefix, targetPrefix)

	ctx := context.Background()
	rootCfg := localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: absDir,
		ReadOnly:       false,
		Exclude:        excludePatterns,
		Include:        includePatterns,
	}
	if err := lfHandler.AddRoot(rootName, rootCfg, local.RawContentStore(), local.RawLocationIndex()); err != nil {
		ingestHandler.UnregisterMount(sourcePrefix)
		return Result{}, fmt.Errorf("add localfiles root: %w", err)
	}

	// Subscribe BEFORE starting the watcher. The watcher's initial
	// scan writes entities to sourcePrefix synchronously; if the
	// subscription isn't live by then, those writes are missed and
	// pre-existing files at mount time never reach the target prefix.
	// Single hop, no continuation chain.
	deliverURI := fmt.Sprintf("entity://%s/%s", localID, workbench.NotificationIngestPattern)
	// "deleted" included so fs-unlink cascades through to the
	// notification-ingest delete branch (workbench application logic
	// — removes the workbench-owned doc/markdown-file at the target
	// prefix when its source FileData goes away).
	sub, err := local.SubscribeRawAt(localID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated", "deleted"}})
	if err != nil {
		ingestHandler.UnregisterMount(sourcePrefix)
		return Result{}, fmt.Errorf("subscribe to source prefix: %w", err)
	}

	if err := lfHandler.StartWatching(ctx, rootName, local.RawContentStore(),
		local.RawLocationIndex(), local.IdentityHash()); err != nil {
		_ = sub.Close()
		ingestHandler.UnregisterMount(sourcePrefix)
		return Result{}, fmt.Errorf("start watcher: %w", err)
	}

	// Record the subscription so unmount can cancel it.
	sh.registerMountSub(rootName, sub)

	includeDisplay := "(all)"
	if len(includePatterns) > 0 {
		includeDisplay = strings.Join(includePatterns, ", ")
	}
	excludeDisplay := strings.Join(excludePatterns, ", ")
	if excludeDisplay == "" {
		excludeDisplay = "(none)"
	}

	return LinesResult([]string{
		fmt.Sprintf("mounted %s → %s (root=%s)", absDir, targetPrefix, rootName),
		fmt.Sprintf("  source:    %s", sourcePrefix),
		fmt.Sprintf("  target:    %s", targetPrefix),
		fmt.Sprintf("  include:   %s", includeDisplay),
		fmt.Sprintf("  exclude:   %s", excludeDisplay),
		fmt.Sprintf("  chain cap: %s", capPath),
		fmt.Sprintf("  handler:   %s", workbench.NotificationIngestPattern),
		fmt.Sprintf("  sub:       %s", sub.ID()),
	}), nil
}

// splitFilterPatterns parses a comma-separated glob list, trimming
// whitespace around each entry and dropping empties.
func splitFilterPatterns(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cmdUnmount tears down a mount: cancel subscription, abandon the
// chain continuations. Idempotent — missing pieces are ignored.
// Does NOT delete the persisted RootConfig (that requires explicit
// cleanup since other tooling may want to inspect the last-mounted
// state); a future v2 may add `unmount --purge`.
func cmdUnmount(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: unmount <root-name>")
	}
	rootName := args[0]
	sourcePrefix := "local/files/" + rootName + "/"
	if sh.NotificationIngest != nil {
		sh.NotificationIngest.UnregisterMount(sourcePrefix)
	}
	subErr := sh.closeMountSub(rootName)
	// The fsnotify watcher is still not stopped: core-go's
	// localfiles.Handler exposes StartWatching but no StopWatching /
	// RemoveRoot. Stopping it needs an additive core-go affordance —
	// trivial there (handler.go already holds h.watchers[rootName] and
	// watcher.Stop()); tracked as an upstream candidate alongside the
	// other localfiles items.
	// Note this is bounded, not unbounded: StartWatching stops and
	// replaces any existing watcher for the same root
	// (../entity-core-go/ext/localfiles/handler.go:75-78), so a
	// remount does not leak a second watcher — the bound is one live
	// watcher per distinct root name, not one per unmount.
	if subErr != nil {
		return MessageResult(fmt.Sprintf("unmounted %s (ingest + subscription cleared; subscription close reported: %v; watcher stop pending core-go StopWatching)", rootName, subErr)), nil
	}
	return MessageResult(fmt.Sprintf("unmounted %s (ingest registration + subscription cleared; watcher stop pending core-go StopWatching affordance)", rootName)), nil
}

// cmdMountSweep reconciles a mount's tree state against its
// filesystem state. Operator tool for cold recovery of drift
// accumulated when the watcher was offline.
//
// Usage:
//
//	mount sweep <root>           — remove tree entries whose fs file is gone
//	mount sweep <root> -add      — also ingest fs files missing from the tree
//
// Phase E v2 §7.2. Safe by construction: scoped to local/files/
// namespace + the mount's registered target prefix; never touches
// other extensions' namespaces; never mutates the content store
// (orphaned blobs remain — that's item 19, awaiting cross-team
// GC contract).
func cmdMountSweep(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: mount sweep <root> [-add]")
	}
	rootName := args[0]
	addMode := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-add":
			addMode = true
		default:
			return Result{}, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	res, err := workbench.SweepMount(sh.Local.Peer, sh.NotificationIngest, rootName)
	if err != nil {
		return Result{}, err
	}

	lines := []string{
		fmt.Sprintf("sweep %s: fs files=%d, tree paths=%d", rootName, res.FilesystemFiles, res.SourcePresent),
		fmt.Sprintf("  removed source bindings: %d", len(res.SourceRemoved)),
	}
	for _, rel := range res.SourceRemoved {
		lines = append(lines, fmt.Sprintf("    - %s", rel))
	}
	if len(res.TargetRemoved) > 0 {
		lines = append(lines, fmt.Sprintf("  removed target bindings: %d", len(res.TargetRemoved)))
		for _, rel := range res.TargetRemoved {
			lines = append(lines, fmt.Sprintf("    - %s", rel))
		}
	}

	if addMode {
		added, errs, err := workbench.IngestMissingFiles(sh.Local.Peer, rootName)
		if err != nil {
			return Result{}, err
		}
		lines = append(lines, fmt.Sprintf("  ingested missing files: %d", added))
		for _, e := range errs {
			lines = append(lines, "    ! "+e)
		}
	}

	return LinesResult(lines), nil
}

// cmdMountFilterUpdate replaces the runtime include OR exclude
// pattern set for an existing mount. Phase E v2 §7.3 workbench-
// application layer — see notification_ingest.go::SetMountFilters
// for the live-filter semantics + the FEEDBACK memo for the
// localfiles-side gap.
//
// Usage:
//
//	mount include <root> PATTERNS    — set include (comma-separated)
//	mount include <root> -reset      — clear include (= match all)
//	mount exclude <root> PATTERNS    — set exclude (comma-separated)
//	mount exclude <root> -reset      — clear exclude
func cmdMountFilterUpdate(sh *Shell, args []string, kind string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: mount %s <root> PATTERNS|-reset", kind)
	}
	rootName := args[0]
	sourcePrefix := "local/files/" + rootName + "/"

	if sh.NotificationIngest == nil {
		return Result{}, fmt.Errorf("workbench notification-ingest handler not wired on this shell")
	}
	curInclude, curExclude, ok := sh.NotificationIngest.MountFilters(sourcePrefix)
	if !ok {
		return Result{}, fmt.Errorf("no mount with root name %q", rootName)
	}

	var newPatterns []string
	if args[1] != "-reset" {
		newPatterns = splitFilterPatterns(args[1])
	}

	switch kind {
	case "include":
		curInclude = newPatterns
	case "exclude":
		curExclude = newPatterns
	}
	if !sh.NotificationIngest.SetMountFilters(sourcePrefix, curInclude, curExclude) {
		return Result{}, fmt.Errorf("could not update %q filters (mount gone?)", rootName)
	}

	includeDisplay := "(all)"
	if len(curInclude) > 0 {
		includeDisplay = strings.Join(curInclude, ",")
	}
	excludeDisplay := "(none)"
	if len(curExclude) > 0 {
		excludeDisplay = strings.Join(curExclude, ",")
	}
	return LinesResult([]string{
		fmt.Sprintf("mount %s %s filter updated (workbench-application layer)", kind, rootName),
		fmt.Sprintf("  include: %s", includeDisplay),
		fmt.Sprintf("  exclude: %s", excludeDisplay),
		"  note: localfiles watcher still emits FileData for non-matching files;",
		"        run `mount sweep " + rootName + "` to remove residue at the source prefix",
		"        if you tightened the filter and want to clean up.",
	}), nil
}

// cmdMountFilterShow reads back the current workbench filter for a mount.
func cmdMountFilterShow(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: mount filter <root>")
	}
	rootName := args[0]
	sourcePrefix := "local/files/" + rootName + "/"

	if sh.NotificationIngest == nil {
		return Result{}, fmt.Errorf("workbench notification-ingest handler not wired on this shell")
	}
	include, exclude, ok := sh.NotificationIngest.MountFilters(sourcePrefix)
	if !ok {
		return Result{}, fmt.Errorf("no mount with root name %q", rootName)
	}
	includeDisplay := "(all)"
	if len(include) > 0 {
		includeDisplay = strings.Join(include, ",")
	}
	excludeDisplay := "(none)"
	if len(exclude) > 0 {
		excludeDisplay = strings.Join(exclude, ",")
	}
	return LinesResult([]string{
		fmt.Sprintf("mount %s filter:", rootName),
		fmt.Sprintf("  include: %s", includeDisplay),
		fmt.Sprintf("  exclude: %s", excludeDisplay),
	}), nil
}

// cmdMounts shows currently-mounted bridges from the tree-resident
// config namespace. Reads system/config/local/files/* entries —
// note we read from the local/files handler's own config namespace
// (it's already persisted there by AddRoot), we don't keep a
// separate workbench-level mount list.
func cmdMounts(sh *Shell, args []string) (Result, error) {
	_ = args
	local := sh.Local.Peer
	entries := local.Store().List("system/config/local/files/")
	if len(entries) == 0 {
		return MessageResult("no mounts"), nil
	}
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, fmt.Sprintf("mounted roots: %d", len(entries)))
	for _, e := range entries {
		// Path shape: system/config/local/files/{root}
		root := strings.TrimPrefix(e.Path, "system/config/local/files/")
		lines = append(lines, fmt.Sprintf("  %-30s @ %s", root, e.Path))
	}
	return LinesResult(lines), nil
}

// sanitizeRootName produces a stable, filesystem-safe slug from a
// directory basename. Lowercases; replaces non-alphanumeric/hyphen
// runs with a single hyphen; trims leading/trailing hyphens.
func sanitizeRootName(s string) string {
	var b strings.Builder
	lastHyphen := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
