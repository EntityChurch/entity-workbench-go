package shellcmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cmd_follow.go — the operator surface over somebody else's feed.
//
// Three verbs and one reading: `follow` / `unfollow` declare an interest,
// `follows` says what is declared and what is in the way, `timeline` reads.
//
// **`feed` reads OUR posts out of our own tree; `timeline` reads THEIRS
// through their signed root.** That is not a nuance — they are different
// operations with different trust arguments, and one verb doing both would
// have to describe them with one sentence.

const followUsage = `follow <peer> [-label NAME]   — follow a peer's feed
unfollow <peer>               — stop, and forget where you had read to
follows                       — who this peer follows, and what is in the way

Following asks nobody's permission and tells nobody. FEED §2.4 makes a
feed-follow a follow of a NAMESPACE: public, pull-only, needing no grant,
and the publisher does not learn that you did it. (That is what separates
it from following a SHARE, where the publisher authorized you and
therefore knows.)

So a follow is a private, durable note to yourself. Reading is what
needs a route to them — a connection this peer already holds — and
` + "`follows`" + ` says which subjects are currently out of reach.

-label is your own petname for the peer: local, never sent anywhere, and
the answer to "I cannot read a public key" that needs no naming
authority at all.`

const timelineUsage = `timeline [<peer>] [-limit N] [-new]

Reads the feeds this peer follows, through each publisher's signed root:
verify one signature, read the index head, read down, stop. Every entry
is checked against the namespace it was found under (FEED-R1) and
attributed only if a detached signature over its exact bytes verifies
(FEED-R4) — an entry nobody signed is shown as UNATTRIBUTED rather than
credited to whoever served it.

  <peer>     read one subject instead of every follow. It does not have
             to be followed.
  -limit N   at most N entries per publisher
  -new       read from your stored position and ADVANCE it. Without this
             the cursor is neither read nor written, so a plain timeline
             is safe to run twice.

Entries are grouped by publisher and NOT interleaved by timestamp: §9.3
leaves cross-publisher order undefined and §2.3.2 makes created_at a
display heuristic, so sorting by it would promote an unverifiable clock
to an ordering authority.`

func cmdFollow(sh *Shell, args []string) (Result, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return MessageResult(followUsage), nil
	}
	rest, flags, err := splitFlags(args, "label")
	if err != nil {
		return Result{}, fmt.Errorf("follow: %w\n%s", err, followUsage)
	}
	if len(rest) != 1 {
		return Result{}, fmt.Errorf("follow: one peer at a time\n%s", followUsage)
	}
	subject, via := resolvePeerRef(sh, rest[0])
	out, err := sh.ShellWorkspace.FeedFollow(FollowRequest{
		Subject: subject,
		Label:   flags["label"],
		// §2.4's `via`: the identifier as it was typed, for provenance
		// display. What the operator wrote, not what we resolved it to —
		// the resolved form is already the subject.
		Via: rest[0],
	})
	if err != nil {
		return Result{}, fmt.Errorf("follow: %w", err)
	}

	verb := "following"
	if out.Updated {
		verb = "updated the label on"
	}
	lines := []string{fmt.Sprintf("%s %s%s", verb, out.Follow.Subject, via)}
	if out.Follow.Label != "" {
		lines = append(lines, "  label      "+out.Follow.Label+"  (yours, local, never sent anywhere)")
	}
	lines = append(lines,
		"  they are not told, and nothing was asked of them — FEED §2.4 makes this pull-only")
	if !out.Reachable {
		lines = append(lines, "", "not readable yet", "  "+wrapDetail(out.Why))
	} else {
		lines = append(lines, "", "  `timeline "+out.Follow.Subject+"` reads it now")
	}
	return LinesResult(lines), nil
}

func cmdUnfollow(sh *Shell, args []string) (Result, error) {
	if len(args) != 1 || args[0] == "help" {
		return MessageResult(followUsage), nil
	}
	subject, via := resolvePeerRef(sh, args[0])
	existed, err := sh.ShellWorkspace.FeedUnfollow(subject)
	if err != nil {
		return Result{}, fmt.Errorf("unfollow: %w", err)
	}
	if !existed {
		return MessageResult("not following " + subject + via), nil
	}
	return LinesResult([]string{
		"stopped following " + subject + via,
		"  the read position went with it, so a later follow shows the feed rather than resuming",
		"  nothing was deleted on their side and they were never told either way",
	}), nil
}

func cmdFollows(sh *Shell, args []string) (Result, error) {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		return MessageResult(followUsage), nil
	}
	out, err := sh.ShellWorkspace.FeedFollows(context.Background())
	if err != nil {
		return Result{}, fmt.Errorf("follows: %w", err)
	}
	var lines []string
	if len(out.Rows) == 0 {
		lines = append(lines, "following nobody — `follow <peer>` starts")
	}
	for _, r := range out.Rows {
		name := r.Follow.Subject
		if r.Follow.Label != "" {
			name = r.Follow.Label + "  " + r.Follow.Subject
		}
		lines = append(lines, "  "+name)
		since := "?"
		if r.Follow.Since > 0 {
			since = time.UnixMilli(int64(r.Follow.Since)).Format("2006-01-02")
		}
		pos := "never read"
		if r.Read {
			// A position, not a claim about the publisher: it says where
			// THIS reader stopped, and nothing about when they posted.
			pos = fmt.Sprintf("read to page %d, entry %s", r.Cursor.Page,
				shortHex(r.Cursor.Applied.Bytes()))
		}
		lines = append(lines, "      since "+since+" · "+pos)
	}
	if len(out.Problems) > 0 {
		lines = append(lines, "", "what is in the way")
		for _, p := range out.Problems {
			lines = append(lines, "  - "+wrapDetail(p))
		}
	}
	return LinesResult(lines), nil
}

func cmdTimeline(sh *Shell, args []string) (Result, error) {
	req := TimelineRequest{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "help", "-h", "--help":
			return MessageResult(timelineUsage), nil
		case "-new":
			req.New = true
		case "-limit":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("timeline: -limit needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return Result{}, fmt.Errorf("timeline: -limit takes a non-negative count, got %q", args[i+1])
			}
			req.Limit = n
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return Result{}, fmt.Errorf("timeline: unknown flag %q\n%s", args[i], timelineUsage)
			}
			if req.Subject != "" {
				return Result{}, fmt.Errorf("timeline: one subject at a time\n%s", timelineUsage)
			}
			req.Subject, _ = resolvePeerRef(sh, args[i])
		}
	}

	out, err := sh.ShellWorkspace.FeedTimeline(context.Background(), req, bareBrowserOf(sh))
	if err != nil {
		return Result{}, fmt.Errorf("timeline: %w", err)
	}
	return LinesResult(renderTimeline(out)), nil
}

// renderTimeline prints the entries and then what produced them.
//
// **The sources block is not a footer, it is `FEED-R23`**: a view of more
// than one publisher MUST declare what produced it — sources, mirrors and
// exclusions — and a publisher that could not be reached is an exclusion,
// so its row stays in the list with the reason instead of disappearing
// into a shorter timeline.
func renderTimeline(out TimelineOutcome) []string {
	var lines []string
	if len(out.Sources) == 0 {
		return []string{"following nobody — `follow <peer>` starts, or `timeline <peer>` reads one"}
	}

	if len(out.Merged) == 0 {
		lines = append(lines, "nothing to show")
		if out.Advanced {
			lines = append(lines, "  nothing new since this reader's last position — which is the "+
				"ordinary state of a feed, not an error")
		}
	}
	for _, e := range out.Merged {
		who := e.Subject
		if e.Label != "" {
			who = e.Label
		}
		stamp := "?"
		if e.Entry.CreatedAt > 0 {
			// Their clock, shown because a reader wants it and marked as
			// theirs because §2.3.2 makes it a display heuristic.
			stamp = time.UnixMilli(int64(e.Entry.CreatedAt)).Format("2006-01-02 15:04")
		}
		mark := ""
		if e.Entry.IsReply {
			mark = " (reply)"
		}
		if e.Entry.Rejected {
			// FEED-R1. The row stays and the body does not.
			lines = append(lines, fmt.Sprintf("  %s  %s ⛔ REJECTED%s", stamp, who, mark))
			lines = append(lines, "            "+wrapDetail(e.Entry.Problem))
			continue
		}
		if !e.Entry.Attributed {
			mark += " ⚠ UNATTRIBUTED"
		}
		lines = append(lines, fmt.Sprintf("  %s  %s%s", stamp, who, mark))
		if e.Entry.Problem != "" {
			lines = append(lines, "            "+wrapDetail(e.Entry.Problem))
			continue
		}
		lines = append(lines, "            "+firstLine(e.Entry.Text))
		lines = append(lines, "            "+shortHex(e.Entry.Hash.Bytes()))
	}

	lines = append(lines, "", "what produced this view")
	for _, s := range out.Sources {
		who := s.Subject
		if s.Label != "" {
			who = s.Label + " (" + s.Subject + ")"
		}
		if s.Err != "" {
			lines = append(lines, "  - "+who+" — EXCLUDED: "+wrapDetail(s.Err))
			continue
		}
		via := string(s.Read.Via)
		if via == "" {
			via = "nothing read"
		}
		lines = append(lines, fmt.Sprintf("  - %s — %d %s via the %s", who, len(s.Read.Entries),
			pluralOf(len(s.Read.Entries), "entry", "entries"), via))
		if s.Read.Freshness != "" {
			lines = append(lines, "      "+wrapDetail(s.Read.Freshness))
		}
		if s.Read.Truncated {
			lines = append(lines, "      this is not all of it — a limit stopped the read")
		}
		if s.Read.Resumed {
			lines = append(lines, "      resumed from a page number: the entry this reader held as its "+
				"position is gone")
		}
		for _, n := range s.Read.Notes {
			lines = append(lines, "      "+wrapDetail(n))
		}
	}
	if out.Advanced {
		lines = append(lines, "", "read positions were advanced (-new)")
	}
	return lines
}

// shortHex is a hash at reading length for a list column.
func shortHex(b []byte) string {
	s := hex.EncodeToString(b)
	if len(s) <= 24 {
		return s
	}
	return s[:24] + "…"
}
