package shellcmd

import (
	"context"
	"fmt"
	"strings"

	"entity-workbench-go/workbench"
)

// cmd_publish.go — the operator surface over publishing.
//
// Until this landed the only way to publish anything from this tree was
// `entity-publish`, a separate binary that opens a peer's store off disk
// — so the act was reachable from no shell verb and no pixel, on a
// product whose whole consume side is built out. That is D23's shape,
// and it is why this verb and the *Local Site* panel's Publish section
// are one change.

const publishUsage = `publish [flags]              — sign a root over this peer's sites and choose who may read it
publish status               — what is published, who may read it, and whether anyone can arrive

Flags:
  -prefix P     the tree prefix the signed root commits to (default "` + `sites/` + `")
  -site ID      sugar for -prefix sites/ID/ — note a peer has exactly ONE
                published root, so this REPLACES what it committed to before
  -out DIR      also emit the static corridor into DIR, for upload to a
                web server
  -origin URL   the HTTP origin DIR will be served from; baked into the
                emitted transport profile, so it is wanted now and not at
                upload time
  -public       write the ` + "`default`" + ` policy row: any peer that can dial
                this one may read the published prefix and verify it
  -private      remove that row
  -feed         publish this peer's FEED: the entries, the index, and the
                signature that attributes each entry. Commits to those and
                to nothing else, at the peer root — which is the only prefix
                that contains both ` + "`app/feed/`" + ` and ` + "`system/signature/`" + `
  -whole-peer   acknowledge that the prefix spans the system boundary, so
                the root commits to this peer's own declarations as well as
                to the thing you meant to publish. REFUSED without it, and
                the refusal names what is on offer

Publishing is one act — signing a root over a prefix — and the flags pick
which projections of it you want. With no flags it signs and tells you
nobody can read it yet.

Note this shell defaults to an IN-MEMORY store: ` + "`-identity NAME`" + ` alone
gives you the right peer-id and an empty tree, so a site authored on disk
will not be there to publish. Start with ` + "`-storage sqlite`" + `.`

func cmdPublish(sh *Shell, args []string) (Result, error) {
	if len(args) > 0 {
		switch args[0] {
		case "status":
			out, err := sh.ShellWorkspace.PublishStatus(context.Background())
			if err != nil {
				return Result{}, err
			}
			return LinesResult(renderPublish(out)), nil
		case "help", "-h", "--help":
			return MessageResult(publishUsage), nil
		}
	}

	var req PublishRequest
	need := func(i int, flag string) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("publish: %s needs a value", flag)
		}
		return args[i+1], nil
	}
	for i := 0; i < len(args); i++ {
		var err error
		switch args[i] {
		case "-prefix":
			req.Prefix, err = need(i, "-prefix")
			i++
		case "-site":
			var id string
			id, err = need(i, "-site")
			i++
			if err == nil {
				req.Prefix = workbench.SitesSubpath + "/" + strings.Trim(id, "/") + "/"
			}
		case "-out":
			req.OutputDir, err = need(i, "-out")
			i++
		case "-origin":
			req.OriginURL, err = need(i, "-origin")
			i++
		case "-public":
			req.Public = true
		case "-private":
			req.Unpublic = true
		case "-whole-peer":
			req.AllowWholePeer = true
		case "-feed":
			req.Feed = true
		default:
			return Result{}, fmt.Errorf("publish: unknown flag %q\n%s", args[i], publishUsage)
		}
		if err != nil {
			return Result{}, err
		}
	}
	if req.OriginURL != "" && req.OutputDir == "" {
		// -origin with no -out is almost always a live/static mix-up, and
		// silently ignoring it would let an operator believe they had
		// configured an origin. The origin is a property of the static
		// directory; there is nowhere else for it to go.
		return Result{}, fmt.Errorf(
			"publish: -origin describes where a static directory will be served from, and no " +
				"directory was asked for — add -out DIR, or drop -origin (a live read needs no " +
				"origin: a reader dials this peer)")
	}

	out, err := sh.ShellWorkspace.Publish(context.Background(), req)
	if err != nil {
		return Result{}, err
	}
	return LinesResult(renderPublish(out)), nil
}

// renderPublish is the one renderer for both the act and the read.
//
// Shared on purpose, the same way `status` shares `observeDevice`
// between its pass and its read: two surfaces describing one peer are
// read side by side, and any difference between them reads to an
// operator as a change in the peer rather than a difference in the
// code. The ONE thing that differs is the first line, and it differs
// because [PublishOutcome.Minted] says so.
func renderPublish(out PublishOutcome) []string {
	var lines []string
	if out.Minted {
		lines = append(lines, fmt.Sprintf("published %q — %d keys, seq %d",
			out.Prefix, out.Bindings, out.Seq))
	} else if out.RootHash == "" {
		lines = append(lines, "this peer has published nothing")
	} else {
		lines = append(lines, fmt.Sprintf("published root: %q — seq %d", out.Prefix, out.Seq))
		lines = append(lines, fmt.Sprintf("  %d keys are bound under that prefix right now", out.Bindings))
	}
	if out.RootHash != "" {
		lines = append(lines, "  root  "+out.RootHash)
		lines = append(lines, "  peer  "+out.PeerID)
	}
	// WHICH SET, always, when it is not the scan. After `A-38` the prefix
	// no longer implies the content: a root over the peer root is 9 keys
	// or 399 depending on a choice nothing else on screen records, and
	// the difference is whether this peer's folder paths and its peers'
	// addresses are inside the signed set. Printing the prefix and the
	// count without the set is printing two numbers that do not add up.
	if out.ContentSet == workbench.PublishContentFeed {
		lines = append(lines,
			"  set   the feed: entries, index, and the signature attributing each entry",
			"        (the peer root is the BOUND — this root commits to those keys and to",
			"        nothing else under it)")
	}

	lines = append(lines, "", "who may read it")
	switch {
	case out.RootHash == "" && !out.Public.Present:
		// Said separately from the private case below, because that one
		// claims "this root is signed and in the tree" — which is exactly
		// the sentence a peer with no root must not print, and which it
		// did print until the flow was actually run (AP71).
		lines = append(lines, "  nothing, because nothing is published yet. `publish` signs a root",
			"  over this peer's sites; it does not authorize anybody.")
	case !out.Public.Present:
		lines = append(lines, "  nobody, unless you have named them with `share` — this root is signed",
			"  and in the tree, and no peer is authorized to ask for it. `publish -public`",
			"  opens it to every peer that can dial this one.")
	case out.Public.Ours:
		lines = append(lines, fmt.Sprintf("  ANY PEER THAT CAN DIAL THIS ONE may read %q and verify it.", out.Public.Prefix),
			"  That is the `default` row in the capability policy table; `access` lists it,",
			"  and `publish -private` removes it.")
	default:
		lines = append(lines, "  a `default` policy row exists that this verb did not write:",
			"    "+out.Public.Summary)
	}

	lines = append(lines, "", "can anyone arrive")
	if out.Reach.Listening == "" {
		lines = append(lines, "  not listening — no peer can dial this one")
	} else {
		lines = append(lines, "  listening on "+out.Reach.Listening)
	}
	if len(out.Reach.Advertised) == 0 {
		lines = append(lines, "  advertised: nothing — a reader that knows only the peer-id cannot",
			"  find an address for this peer")
	} else {
		lines = append(lines, "  advertised: "+strings.Join(out.Reach.Advertised, ", "))
		lines = append(lines, "              (which of these a reader picks is the reader's choice,",
			"               ranked by §6.5.1a D1 on the machine doing the dialling)")
	}

	if out.StaticDir != "" {
		lines = append(lines, "", "static projection")
		lines = append(lines, fmt.Sprintf("  %d paths → %s (%d bytes)", out.StaticPaths, out.StaticDir, out.StaticBytes))
		if out.StaticOrigin != "" {
			lines = append(lines, "  to be served from "+out.StaticOrigin)
		}
	}

	if len(out.Problems) > 0 {
		lines = append(lines, "", "what is not done")
		for _, p := range out.Problems {
			lines = append(lines, "  - "+wrapDetail(p))
		}
	}
	return lines
}
