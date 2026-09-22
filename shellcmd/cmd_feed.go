package shellcmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// cmd_feed.go — the operator surface over this peer's own feed.
//
// Two verbs, one gesture each: `post` writes one entry, `feed` shows what this
// peer has posted and whether anybody can read it. That split is the same one
// `publish` / `publish status` makes, for the same reason — an act and a read
// are different operations and a surface that refreshes must not be the one
// that writes.
//
// Until this landed the feed vocabulary was reachable from no verb and no
// pixel, which is D23's violation in the shape the reachability sweep cannot
// see: it asks whether a `workbench/*_model.go` has a surface, and these are
// SDK types.

const postUsage = `post <text>                 — write one entry into this peer's feed
post -reply <hash> <text>   — reply to an entry this peer holds

The body is carried inline as an ` + "`app/embed/text/plain`" + ` payload and is
bounded at 16384 bytes (APP-CONVENTION-EMBED §3). Every entry is signed
individually at authoring time: a detached ` + "`system/signature`" + ` is what
attributes an entry once it leaves this tree, and nothing downstream can
supply one afterwards.

POSTING IS NOT PUBLISHING. A post writes four keys into this peer's tree
and moves none of them into the signed root, so a reader sees nothing new
until ` + "`publish`" + ` runs again. This verb says so every time the root is
behind.

A REPLY NOTIFIES NOBODY. It is published in THIS peer's namespace like
any other entry; the peer you replied to learns of it only if they poll
for entries naming theirs, or if they have granted you delivery. There is
no unsolicited inbox in this system (FEED §3), which is the same property
that means there is no spam route.`

const feedUsage = `feed [-limit N]             — this peer's own feed, newest first

Shows what is posted, whether a signed root covers it, whether that root
still commits to what the tree holds, and who is authorized to read it.
It publishes nothing and dials nobody.

This is NOT a reader for somebody else's feed — it reads this peer's own
tree with this peer's own authority. Following another peer needs the
walk, the four live-reference outcomes and per-entry attribution, none of
which this seat has built.`

func cmdPost(sh *Shell, args []string) (Result, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return MessageResult(postUsage), nil
	}

	var req FeedPostRequest
	rest := args
	if args[0] == "-reply" {
		if len(args) < 3 {
			return Result{}, fmt.Errorf("post: -reply needs an entry hash and a body\n%s", postUsage)
		}
		h, err := parseEntryHash(args[1])
		if err != nil {
			return Result{}, fmt.Errorf("post: %w", err)
		}
		req.ReplyTo = h
		rest = args[2:]
	}
	req.Text = strings.Join(rest, " ")

	out, err := sh.ShellWorkspace.Post(context.Background(), req)
	if err != nil {
		return Result{}, err
	}
	return LinesResult(renderPost(out)), nil
}

func cmdFeed(sh *Shell, args []string) (Result, error) {
	limit := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "help", "-h", "--help":
			return MessageResult(feedUsage), nil
		case "-limit":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("feed: -limit needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return Result{}, fmt.Errorf("feed: -limit takes a non-negative count, got %q", args[i+1])
			}
			limit = n
			i++
		default:
			return Result{}, fmt.Errorf("feed: unknown flag %q\n%s", args[i], feedUsage)
		}
	}

	out, err := sh.ShellWorkspace.Feed(context.Background(), limit)
	if err != nil {
		return Result{}, err
	}
	return LinesResult(renderFeed(out)), nil
}

// parseEntryHash takes the hex form `post` and `feed` print.
//
// An entry's identity IS its content hash (§2.2.1 pins every reference to
// one), so the hex is the address an operator has. Refused rather than
// truncated when it is the wrong length: a short hash that happens to decode
// would name different bytes.
func parseEntryHash(s string) (hash.Hash, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return hash.Hash{}, fmt.Errorf("%q is not hex — `feed` prints each entry's hash, which is its identity", s)
	}
	h, err := hash.FromBytes(raw)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("%q is not an entity hash: %w", s, err)
	}
	return h, nil
}

func renderPost(out FeedPostOutcome) []string {
	e := out.Entry
	lines := []string{
		fmt.Sprintf("posted — entry %s", hex.EncodeToString(e.Hash.Bytes())),
		fmt.Sprintf("  page %d, %d %s on it, %d in the feed", e.Page, e.PageEntries,
			pluralOf(e.PageEntries, "entry", "entries"), e.Entries),
	}
	if e.NewPage && e.Page > 0 {
		// Worth a line of its own: it is the only moment §4.3's cost model
		// is visible, and a sealed page is a page a reader can cache forever.
		lines = append(lines, fmt.Sprintf("  this post opened page %d; page %d is sealed and will not be "+
			"rewritten", e.Page, e.Page-1))
	}
	lines = append(lines, "  signed — "+e.SignaturePath)
	if !out.ReplyTo.IsZero() {
		// `FEED-R9`, said at the moment the word "reply" is used. The verb
		// did not claim a notification before this, so it was not yet a
		// violation — and an operator's reasonable reading of "reply" is
		// exactly what the MUST NOT exists to head off, so silence was the
		// wrong amount to say.
		lines = append(lines,
			"  this reply notifies nobody: it is published in THIS peer's namespace, and the peer",
			"  you replied to sees it only if they poll for entries naming theirs, or have granted",
			"  you delivery — inbound delivery is granted, never ambient")
	}
	lines = append(lines, feedReachLines(out.Reach, true)...)
	return lines
}

func renderFeed(out FeedOutcome) []string {
	r := out.Readout
	var lines []string
	if !r.Published {
		lines = append(lines, "this peer has posted nothing — `post <text>` writes the first entry")
		lines = append(lines, feedReachLines(out.Reach, false)...)
		return lines
	}

	lines = append(lines, fmt.Sprintf("%d %s across %d %s", len(r.Entries),
		pluralOf(len(r.Entries), "entry", "entries"), r.Pages, pluralOf(int(r.Pages), "page", "pages")))
	if r.Truncated {
		lines = append(lines, "  (limited — older entries are in the feed and were not read)")
	}
	lines = append(lines, "")
	for _, e := range r.Entries {
		stamp := "?"
		if e.CreatedAt > 0 {
			stamp = time.UnixMilli(int64(e.CreatedAt)).Format("2006-01-02 15:04")
		}
		mark := ""
		if e.IsReply {
			mark = " (reply)"
		}
		if !e.Signed {
			// Rendered, never hidden: an unsigned entry is a real entry that
			// cannot be attributed once it travels, and dropping it would
			// make this list disagree with the feed's own index.
			mark += " ⚠ UNSIGNED"
		}
		lines = append(lines, fmt.Sprintf("  %s  %s%s", stamp, firstLine(e.Text), mark))
		lines = append(lines, fmt.Sprintf("            %s  page %d", hex.EncodeToString(e.Hash.Bytes()), e.Page))
	}

	lines = append(lines, feedReachLines(out.Reach, false)...)
	if len(out.Problems) > 0 {
		lines = append(lines, "", "what is not done")
		for _, p := range out.Problems {
			lines = append(lines, "  - "+wrapDetail(p))
		}
	}
	return lines
}

// feedReachLines is the one rendering of who can read this feed, shared by both
// verbs so a post and a read cannot describe the same peer differently.
//
// `terse` is what `post` wants: after an act, the only interesting part is what
// the act did NOT do.
func feedReachLines(reach FeedReach, terse bool) []string {
	lines := []string{"", "can anyone read it"}
	switch {
	case !reach.Published:
		lines = append(lines, "  no signed root — nothing published; `publish -prefix "+FeedPrefix+"`")
	case !reach.CoversFeed:
		lines = append(lines, fmt.Sprintf("  the published root commits to %q, not to %q", reach.Prefix, FeedPrefix))
	case !reach.Current:
		lines = append(lines, "  the published root is BEHIND this tree — posts since the last `publish` are")
		lines = append(lines, "  in no reader's view, and look identical to not having posted")
		if !terse {
			lines = append(lines, "    published  "+reach.RootHash)
			lines = append(lines, "    tree now   "+reach.RootNow)
		}
	default:
		lines = append(lines, fmt.Sprintf("  published %q and the root commits to this tree as it stands", reach.Prefix))
	}
	if reach.Published && reach.CoversFeed {
		if reach.Public.Ours {
			lines = append(lines, "  ANY PEER THAT CAN DIAL THIS ONE may read it — `publish -private` closes that")
		} else if reach.Public.Present {
			lines = append(lines, "  a `default` policy row exists that `publish` did not write: "+reach.Public.Summary)
		} else {
			lines = append(lines, "  nobody, unless you have named them with `share` — `publish -public` opens it")
		}
	}
	return lines
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if len(s) > 72 {
		s = s[:69] + "…"
	}
	return s
}

// pluralOf is the irregular form; `plural` (cmd_inspect.go) already covers
// the -s case and two spellings of one helper is worse than one of each.
func pluralOf(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
