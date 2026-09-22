package shellcmd

import (
	"context"
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
	}
	lines = append(lines, backfillLines(out.Backfill, out.BackfillSkipped)...)
	return LinesResult(lines), nil
}

// backfillLines renders a catch-up pass for a terminal.
//
// It leads with the count because that is the operator's actual
// question — "did my files come across" — and every previous version of
// this output answered a different one. The note it replaced said
// "files already in their mount arrive when they next change", which
// was true of the implementation, described the product as not doing
// the thing it is named after, and stood for long enough that an
// operator reasonably concluded the whole feature was broken.
func backfillLines(res BackfillResult, skipped bool) []string {
	if skipped {
		return []string{"", "backfill skipped — only files changed from now on will arrive."}
	}
	lines := []string{"", "existing files: " + res.Summary()}
	for _, e := range res.Errors {
		lines = append(lines, "  ! "+e)
	}
	if res.Failed > len(res.Errors) {
		lines = append(lines, fmt.Sprintf("  ! ... and %d more", res.Failed-len(res.Errors)))
	}
	if res.Unreachable {
		// Distinct from a failure count: nothing about their folder was
		// observed, so there is nothing to retry per-file.
		lines = append(lines,
			"  the subscription IS established — changes they make from now on will still",
			"  arrive; run `resync` once the grant has reached this peer.")
		return lines
	}
	if res.Failed > 0 {
		// The most common cause by a wide margin, and the one an
		// operator cannot guess: their grant has not reached us yet, so
		// the content fetch is refused. Naming the retry is worth more
		// than naming the error class.
		lines = append(lines,
			"  a failure here is usually their grant not having reached this peer yet —",
			"  the subscription is established regardless; run `resync` to retry.")
	}
	return lines
}

func cmdResync(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: resync <peer> <root>")
	}
	res, err := sh.Resync(args[0], args[1])
	if err != nil {
		return Result{}, err
	}
	lines := []string{fmt.Sprintf("re-pulled %s from %s", args[1], args[0])}
	lines = append(lines, backfillLines(res, false)...)
	if res.Complete() && res.Failed == 0 && res.Materialized == 0 && res.AlreadyCurrent > 0 {
		// A positive confirmation, which the operator otherwise has no
		// way to obtain: "no errors" and "nothing happened" render
		// identically, and this is the run where they differ.
		lines = append(lines, "",
			"everything the remote folder holds is already here — the sync is current.")
	}
	return LinesResult(lines), nil
}

// cmdCatchUp runs one pass over every received folder and reports it.
//
// The verb exists for `resync`'s reason: everything this flow
// establishes is deliberately durable, so a second run is
// indistinguishable from a stale one unless something reports a positive
// confirmation. It also gives the supervisor a shipped surface — a loop
// running in the background that no operator can ask about or trigger is
// a model with no surface (D23), and here it is worse than usual because
// the condition it heals is itself invisible.
func cmdCatchUp(sh *Shell, args []string) (Result, error) {
	res, err := sh.CatchUp(context.Background())
	if err != nil {
		return Result{}, err
	}
	lines := []string{res.Summary()}
	for _, p := range res.Problems {
		lines = append(lines, "  problem: "+p)
	}
	if res.Folders > 0 && res.Recovered > 0 {
		lines = append(lines, "",
			fmt.Sprintf("%d file(s) had been silently lost — a burst of changes on the "+
				"sending peer outran its delivery queue, which drops rather than "+
				"blocks. Nothing was lost from their disk and nothing is lost from "+
				"yours; this pass closed the gap.", res.Recovered))
	}
	if last, ok := sh.LastCatchUp(); ok && last.AtMillis > 0 {
		lines = append(lines, "",
			"the background supervisor is running; its last pass: "+last.Summary())
	} else {
		lines = append(lines, "",
			"no background supervisor is running in this process — this was a "+
				"one-off pass.")
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
