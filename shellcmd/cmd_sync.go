package shellcmd

import (
	"fmt"
	"strings"
)

// The `sync` verb — the receiving half of a cross-peer folder sync.
//
// Flag parsing and phrasing only; everything that decides what happens
// lives in sync_op.go, so the Avalonia panel calls the same function
// rather than building an argv (AGENTS.md: *DRY the integration, not the
// renderer*).
//
//	sync <peer> <root> [-as <local-root>]
//	    Subscribe to <peer>'s local/files/<root>/* and materialize every
//	    file it publishes into the local mount of the same name. `-as`
//	    covers the case where the two peers mounted the same folder under
//	    different names.
//
//	unsync <peer> <root>
//	    Cancel it. Leaves the mount and everything already received.
//
//	syncs
//	    List what is established.
//
// The asymmetry with `mount` is deliberate and worth stating: a mount
// creates a local bridge, a sync attaches to a REMOTE one. `sync`
// therefore refuses when no local mount of the target name exists,
// instead of quietly creating one — a sync that invented its own
// destination directory would be choosing where an operator's files land.

func cmdSync(sh *Shell, args []string) (Result, error) {
	var positional []string
	targetRoot := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-as", "--as":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-as needs a local root name")
			}
			targetRoot = args[i+1]
			i++
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		// The long form, not a one-line usage string. The two things an
		// operator gets wrong here — that a mount must exist first, and
		// that this is the RECEIVING side — are both facts a terse
		// "usage:" line has no room for.
		return LinesResult(strings.Split(syncUsageDetail, "\n")), nil
	}

	out, err := sh.Sync(SyncRequest{
		Remote:     positional[0],
		Root:       positional[1],
		TargetRoot: targetRoot,
	})
	if err != nil {
		return Result{}, err
	}

	who := out.RemotePeerID
	if out.RemoteAlias != "" {
		who = fmt.Sprintf("%s (%s)", out.RemoteAlias, out.RemotePeerID)
	}
	lines := []string{
		fmt.Sprintf("syncing %s from %s", out.Root, who),
		fmt.Sprintf("  their prefix:  %s", out.SourcePrefix),
		fmt.Sprintf("  our prefix:    %s", out.TargetPrefix),
		fmt.Sprintf("  handler:       %s", out.HandlerPattern),
		fmt.Sprintf("  capability:    %s", out.CapabilityPath),
		fmt.Sprintf("  subscription:  %s", out.SubscriptionID),
		"",
		// Said out loud because the alternative is an operator watching
		// an empty directory and concluding the feature is broken. The
		// subscription fires on CHANGE; it does not replay history.
		"note: this delivers files the remote peer changes from now on.",
		"      Files already in their mount arrive when they next change,",
		"      or immediately if they re-mount (the watcher's initial scan",
		"      re-writes every entity it admits).",
	}
	return LinesResult(lines), nil
}

func cmdUnsync(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: unsync <peer> <root>")
	}
	out, err := sh.Unsync(args[0], args[1])
	if err != nil {
		return Result{}, err
	}
	if !out.Found {
		return MessageResult(fmt.Sprintf(
			"no sync of %q from %s was established (nothing to do)",
			out.Root, out.RemotePeerID)), nil
	}
	lines := []string{fmt.Sprintf("stopped syncing %s from %s", out.Root, out.RemotePeerID)}
	if out.SubscriptionCloseErr != nil {
		// Reported, not promoted to a returned error: the routing is
		// gone either way, so the unsync succeeded in the sense that
		// matters, and failing the whole verb here would be alarming in
		// the wrong direction (same reasoning as UnmountOutcome).
		lines = append(lines, fmt.Sprintf(
			"  warning: subscription close reported: %v", out.SubscriptionCloseErr))
	}
	lines = append(lines,
		"  the local mount and everything already received are untouched.")
	return LinesResult(lines), nil
}

func cmdSyncs(sh *Shell, args []string) (Result, error) {
	_ = args
	rows, problems := sh.Syncs()
	if len(rows) == 0 && len(problems) == 0 {
		return MessageResult("no syncs established"), nil
	}
	lines := make([]string, 0, len(rows)+len(problems)+2)
	lines = append(lines, fmt.Sprintf("inbound syncs: %d", len(rows)))
	for _, r := range rows {
		who := r.RemotePeerID
		if r.RemoteAlias != "" {
			who = r.RemoteAlias
		}
		// "restored" rather than "dead": a sync whose handle this
		// process does not hold is one the kernel is driving after a
		// restart, and calling that inactive would be a surface
		// asserting something it does not know.
		state := "restored"
		if r.Live {
			state = "live"
		}
		lines = append(lines, fmt.Sprintf("  %-20s %-12s %s -> %s  [%s]",
			r.Root, who, r.SourcePrefix, r.TargetPrefix, state))
	}
	if len(problems) > 0 {
		lines = append(lines, "", "bindings that did not decode:")
		for _, p := range problems {
			lines = append(lines, "  "+p)
		}
	}
	return LinesResult(lines), nil
}

// syncUsageDetail is the long-form help, kept beside the verb so the
// registration line stays short.
var syncUsageDetail = strings.TrimSpace(`
sync <peer> <root> [-as <local-root>]

Attach to a folder another peer has mounted. Their watcher publishes file
entities under local/files/<root>/; this subscribes to that prefix with
the payload included, pulls each changed file's blob closure across, and
writes it into your own mount of the same name.

Requires a local mount to receive into — run 'mount <dir> <prefix>' first.
A sync writes into a mount; it does not create one, because choosing where
someone's files land is not a default.
`)
