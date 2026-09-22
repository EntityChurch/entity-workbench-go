package shellcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// cmd_ref.go — resolve one reference and say which outcome it got.
//
// # Why this verb exists at all
//
// `APP-CONVENTION-FEED` §2.2.2 gives a live reference four outcomes and
// **[MUST]s that the reader be able to tell which one it got**
// (`FEED-R7`). The normative half is the *ability to tell*, and a fact
// that reaches a resolver and no screen has not reached a reader — which
// is D23 at field granularity, and the shape AP49 takes one boundary
// further out, where a dropped field renders as *"everything is fine"*.
// `workbench.ResolveRef` is the model; this is the surface, in the same
// change.
//
// **It is also the first thing in this tree that resolves a reference at
// all.** Until now `entitysdk.LiveRef` had a constructor and no consumer.
//
// # What it does NOT do
//
// It follows nothing. A reference is one hop — resolve it, report it —
// and a verb that then chased `reply.root` would be a feed reader, which
// this seat has not built and which is where the four reader-side MUSTs
// in §40 of `AGENTS.md` are still ungated.

const refUsage = `ref <entity+ref://...>      — resolve one reference and say which outcome it got

Takes either shape (APP-CONVENTION-REFERENCE §3.1):

  a PIN     entity+ref://<peer>/?hash=<hex>        these exact bytes
  a LIVE    entity+ref://<peer>/<path>[?seen=<hex>]  whatever is there now

The outcome is the answer, not a side note. A live reference has four
of them (FEED §2.2.2) and only one is a failure:

  current            the path resolves; it is what the linker saw
  moved              the path resolves to something ELSE — the ordinary
                     case, and the one a view MUST surface (FEED-R7)
  fell-back-to-seen  the path is gone; the linker's hash still names
                     bytes, and any store that has them satisfies it
  dangling           nothing resolves and nothing falls back
  not-committed      the publisher's root commits to a different part of
                     its tree, so it says nothing about this path

The road is the live one and there is no other: a reference names a
publisher and carries no origin, so resolving one means asking them. It
never dials a guess — this peer must already be connected, or be the
publisher itself.`

func cmdRef(sh *Shell, args []string) (Result, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return MessageResult(refUsage), nil
	}
	if len(args) > 1 {
		return Result{}, fmt.Errorf("ref: one reference at a time, got %d arguments — a reference "+
			"URI carries no spaces (§3.3 percent-encodes every segment)\n%s", len(args), refUsage)
	}

	ref, err := entitysdk.ParseRefURI(args[0])
	if err != nil {
		if errors.Is(err, entitysdk.ErrRefNotAReferenceURI) {
			// REF-R20 makes this a CLASSIFICATION and not a fault: a
			// string that is not an `entity+ref://` is an ordinary
			// external link, and a resolver that re-anchored it would
			// produce a well-formed wrong location and be unable to
			// report that it had. Said plainly here, with the two shapes,
			// because the operator typed it.
			return Result{}, fmt.Errorf("ref: %q is not an entity+ref:// reference — it names "+
				"something outside this system, so there is nothing here to resolve it against\n%s",
				args[0], refUsage)
		}
		return Result{}, fmt.Errorf("ref: %w", err)
	}
	out, err := bareBrowserOf(sh).ResolveReference(context.Background(), ref)
	if err != nil {
		return Result{}, fmt.Errorf("ref: %w", err)
	}
	return LinesResult(renderRefOutcome(out)), nil
}

// renderRefOutcome prints the outcome first and the bytes second.
//
// **The order is the point.** What came back is only meaningful together
// with which of §2.2.2's rows produced it: the same entity is *"the
// document you linked to"* under `current` and *"a different document
// than the one you linked to"* under `moved`, and a surface that leads
// with the body has already answered the question wrongly by omission.
func renderRefOutcome(out workbench.RefOutcome) []string {
	kind := "live reference"
	if out.Ref.IsPinned() {
		kind = "pinned reference"
	}
	uri, err := out.Ref.URI()
	if err != nil {
		// Can only happen for an atom built in process; a parsed one
		// round-trips by construction (REF-R8).
		uri = fmt.Sprintf("%s at %s%s", out.Ref.Tag, out.Ref.Peer, out.Ref.Path)
	}
	lines := []string{
		kind,
		"  " + uri,
		"",
		"  outcome    " + string(out.Row),
	}
	lines = append(lines, refWrap(out.Note)...)

	lines = append(lines, "")
	if !out.Resolved.IsZero() {
		label := "resolves   "
		if out.Ref.IsPinned() {
			label = "pin        "
		}
		lines = append(lines, "  "+label+out.Resolved.String())
	}
	if !out.Seen.IsZero() {
		mark := ""
		if out.Moved {
			// FEED-R7's fact, on the line the comparison is about. Said
			// twice on purpose — once in the outcome and once beside the
			// hashes — because these two lines are what an operator
			// copies into a bug report.
			mark = "   ← NOT what the path resolves to now"
		}
		lines = append(lines, "  linker saw "+out.Seen.String()+mark)
	}
	if out.Have {
		lines = append(lines, fmt.Sprintf("  body       %s, from %s", out.Entity.Type, out.Provenance))
	}
	if out.Prefix != "" {
		lines = append(lines, fmt.Sprintf("  root scope %q", out.Prefix))
	}
	return lines
}

// refWrap lays the outcome's sentence out under the label column, so the
// note reads as part of the `outcome` row rather than as loose prose
// underneath the block.
func refWrap(s string) []string {
	const (
		indent = "             " // len("  outcome    ")
		width  = 84 - len(indent)
	)
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) > width:
			out = append(out, indent+line)
			line = w
		default:
			line += " " + w
		}
	}
	if line != "" {
		out = append(out, indent+line)
	}
	return out
}
