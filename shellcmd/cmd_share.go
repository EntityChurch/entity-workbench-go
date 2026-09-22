package shellcmd

import (
	"fmt"
	"strings"
	"time"
)

// The flow verbs: peers → share → offers → accept, plus shares/unshare
// and the `access` view of who is authorized.
//
// Flag parsing and phrasing only. Everything that decides what happens is
// in share_op.go, so a panel calls the same functions instead of building
// an argv.
//
// Two things every one of these verbs is careful to print, because both
// were measured and neither is guessable:
//
//   - whether the connection was re-established. The grant is assembled
//     during the handshake, so a policy written while connected does not
//     take effect until the next one — a share that says "done" and
//     changes nothing is the worst version of this feature.
//   - that a sync needs BOTH sides. `share` names the command the other
//     operator has to run, because half an authorization produces an
//     accepted subscription and an empty folder.

func cmdPeers(sh *Shell, args []string) (Result, error) {
	_ = args
	peers, err := sh.Peers()
	if err != nil {
		return Result{}, err
	}
	if len(peers) == 0 {
		return LinesResult([]string{
			"no peers visible",
			"",
			"  - discovery announces on the listener's scheme, so a peer with no",
			"    -listen address is not announcing and will not be found;",
			"  - mDNS is link-local, so peers on different networks never appear",
			"    here and must be reached with `connect <alias> <addr>`.",
		}), nil
	}
	lines := make([]string, 0, len(peers)+2)
	lines = append(lines, fmt.Sprintf("peers: %d", len(peers)))
	for _, p := range peers {
		who := p.PeerID
		if p.Alias != "" {
			who = fmt.Sprintf("%s (%s)", p.Alias, p.PeerID)
		}
		addr := p.Address
		if addr == "" {
			addr = "(no dialable address announced)"
		}
		lines = append(lines, fmt.Sprintf("  %-10s %-24s %s", p.Source, addr, who))
	}
	return LinesResult(lines), nil
}

func cmdShare(sh *Shell, args []string) (Result, error) {
	// `share ls` is the listing; anything else is root + peer.
	if len(args) >= 1 && (args[0] == "ls" || args[0] == "list") {
		return cmdShares(sh, args[1:])
	}
	var positional []string
	title := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-title", "--title":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-title needs a value")
			}
			title = args[i+1]
			i++
		case "with", "to":
			// `share notes with alice` reads better than `share notes alice`
			// and costs one line to accept.
			continue
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		return LinesResult(strings.Split(shareUsageDetail, "\n")), nil
	}

	out, err := sh.Share(ShareRequest{
		Root:      positional[0],
		Peer:      positional[1],
		Title:     title,
		NowMillis: uint64(time.Now().UnixMilli()),
	})
	if err != nil {
		return Result{}, err
	}

	who := out.PeerID
	if out.PeerAlias != "" {
		who = fmt.Sprintf("%s (%s)", out.PeerAlias, out.PeerID)
	}
	lines := []string{
		fmt.Sprintf("sharing %s with %s", out.Root, who),
		fmt.Sprintf("  granted:   %s", out.GrantSummary),
		fmt.Sprintf("  policy:    %s", out.PolicyPath),
		fmt.Sprintf("  audience:  %s", strings.Join(out.Audience, ", ")),
	}
	lines = append(lines, grantEffectLines(out.Reconnected, out.ReconnectNote)...)
	lines = append(lines,
		"",
		// The other half. A sync is mutual authorization and the operator
		// on the far side has to run this, or their folder stays empty
		// with no error anywhere.
		//
		// ONE command, and it names a DIRECTORY. Until 2026-09-03 this
		// printed a `mount` line first and an `accept` with no directory
		// after it — the pre-S3 flow, still being handed to operators
		// after `accept` had learned to create the mount itself. Worse
		// than merely stale: an `accept` with no directory falls back to
		// a mount named after OUR folder, which is exactly the unwritten
		// coupling S3 removed, so following this text reintroduced the
		// defect the release fixed.
		"they now run:",
		fmt.Sprintf("    accept %s %s <a-directory-on-their-disk>",
			sh.Local.Peer.PeerID(), out.Root),
		"",
		"  That one command creates the directory, bridges it, authorizes",
		"  our deliveries, subscribes, and pulls across whatever is already",
		"  in the folder. They do not need to mount anything first.",
	)
	return LinesResult(lines), nil
}

func cmdUnshare(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: unshare <root> <peer>")
	}
	out, err := sh.Unshare(args[0], args[1])
	if err != nil {
		return Result{}, err
	}
	lines := []string{fmt.Sprintf("withdrew %s from %s", out.Root, out.PeerID)}
	if out.PolicyRemoved {
		lines = append(lines, "  their access policy is removed (they are in no other share)")
	} else {
		lines = append(lines,
			"  their access policy is KEPT — they are still in the audience of another share")
	}
	if len(out.StillOffered) > 0 {
		lines = append(lines, fmt.Sprintf("  %s is still shared with: %s",
			out.Root, strings.Join(out.StillOffered, ", ")))
	}
	// Never omitted: withdrawal is about the next handshake.
	lines = append(lines, "  note: "+out.Caveat)
	return LinesResult(lines), nil
}

func cmdShares(sh *Shell, args []string) (Result, error) {
	_ = args
	offers, problems := sh.SharesOffered()
	if len(offers) == 0 && len(problems) == 0 {
		return MessageResult("sharing nothing"), nil
	}
	lines := make([]string, 0, len(offers)+len(problems)+2)
	lines = append(lines, fmt.Sprintf("shared folders: %d", len(offers)))
	for _, o := range offers {
		lines = append(lines, fmt.Sprintf("  %-20s -> %s", o.Root, strings.Join(o.Audience, ", ")))
	}
	if len(problems) > 0 {
		lines = append(lines, "", "records that did not decode:")
		for _, p := range problems {
			lines = append(lines, "  "+p)
		}
	}
	return LinesResult(lines), nil
}

func cmdOffers(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: offers <peer>   (what that peer is sharing with you)")
	}
	offers, err := sh.Offers(args[0])
	if err != nil {
		return Result{}, err
	}
	if len(offers) == 0 {
		return LinesResult([]string{
			fmt.Sprintf("%s is offering you nothing", args[0]),
			"",
			"  An offer is a label, not an authority — if they ran `share` and you",
			"  see nothing here, check you are connected and that their share named",
			"  THIS peer: " + sh.Local.Peer.PeerID(),
		}), nil
	}
	lines := make([]string, 0, len(offers)+3)
	lines = append(lines, fmt.Sprintf("%s is offering you: %d", args[0], len(offers)))
	for _, o := range offers {
		lines = append(lines, fmt.Sprintf("  %-20s %s", o.Root, o.Title))
	}
	lines = append(lines, "",
		fmt.Sprintf("to take one:  mount <a-local-dir> archives/<root>/   then   accept %s <root>", args[0]))
	return LinesResult(lines), nil
}

func cmdAccept(sh *Shell, args []string) (Result, error) {
	var positional []string
	allowNonEmpty := false
	for _, a := range args {
		switch a {
		case "-anyway", "--anyway":
			allowNonEmpty = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) < 2 {
		return Result{}, fmt.Errorf("usage: accept <peer> <root> [<directory>] [-anyway]")
	}
	rest := positional
	dir := ""
	if len(rest) > 2 {
		dir = rest[2]
	}
	out, err := sh.Accept(AcceptRequest{
		Peer: rest[0], Root: rest[1], Directory: dir, AllowNonEmpty: allowNonEmpty,
	})
	if err != nil {
		// A non-empty target is the one refusal here an operator is
		// expected to overrule, so name the flag that does it rather than
		// leaving them to find it in the usage line.
		if ne, ok := AsNonEmptyDirectory(err); ok {
			return Result{}, fmt.Errorf("%w\nre-run with -anyway to accept into %s regardless",
				ne, ne.Path)
		}
		return Result{}, err
	}
	who := out.PeerID
	if out.PeerAlias != "" {
		who = fmt.Sprintf("%s (%s)", out.PeerAlias, out.PeerID)
	}
	lines := []string{
		fmt.Sprintf("accepted %s from %s", out.Root, who),
	}
	// State the mount FIRST when this accept made one: it is the only
	// part of the operation that touched the operator's disk, and it is
	// the answer to "where did my files go".
	if out.Mounted != nil {
		lines = append(lines,
			fmt.Sprintf("  files land in: %s  (mounted as %q)",
				out.Mounted.FilesystemRoot, out.Mounted.RootName))
	} else if out.LocalRoot != "" {
		lines = append(lines, fmt.Sprintf("  files land in: the existing mount %q", out.LocalRoot))
	}
	lines = append(lines,
		fmt.Sprintf("  granted them: %s", out.GrantSummary),
		fmt.Sprintf("  policy:       %s", out.PolicyPath),
		fmt.Sprintf("  their prefix: %s", out.Sync.SourcePrefix),
		fmt.Sprintf("  our prefix:   %s", out.Sync.TargetPrefix),
		fmt.Sprintf("  subscription: %s", out.Sync.SubscriptionID),
	)
	lines = append(lines, grantEffectLines(out.Reconnected, out.ReconnectNote)...)
	lines = append(lines, backfillLines(out.Sync.Backfill, out.Sync.BackfillSkipped)...)

	// The dial requirement is about FUTURE changes only, and saying so
	// is not a softening — it is the correction of a real error in this
	// output. The backfill above runs entirely on authority WE hold:
	// we dispatch to them for the listing and the blob closure, which
	// their `share` grant covers. Nothing in it needs them to dial us.
	//
	// What needs their dial is DELIVERY — they dispatch to us to say "a
	// file changed" — so the previous version of this text, which told
	// the operator the folder would stay empty until they dialled, was
	// describing a product that could not do a first transfer at all.
	// It is the sentence an operator reads while their files are
	// already on disk, and it taught them not to believe the surface.
	lines = append(lines, "",
		"ONE STEP LEFT, AND IT IS ON THEIR MACHINE:",
		"    "+out.PublisherMustDial,
		"",
		"  This is for CHANGES FROM NOW ON, not for the files above. A",
		"  dial-by-address authorizes the DIALER only, so they must dial you",
		"  once now that this grant exists. Until they do, their change",
		"  notifications are refused with no error on your side — and you can",
		"  always pull their current state yourself with `resync`.")
	return LinesResult(lines), nil
}

func cmdAccess(sh *Shell, args []string) (Result, error) {
	_ = args
	policies, problems := sh.AccessPolicies()
	if len(policies) == 0 && len(problems) == 0 {
		return LinesResult([]string{
			"no peer is authorized",
			"",
			"  Nothing can read this peer's folders. `share <root> <peer>` writes an",
			"  entry here; so does `accept`, in the other direction.",
		}), nil
	}
	lines := make([]string, 0, len(policies)+len(problems)+2)
	others, public := 0, false
	for _, p := range policies {
		switch {
		case p.IsSelf:
		case p.IsPublic:
			public = true
		default:
			others++
		}
	}
	// The public row is counted OUT of the peer total and said separately.
	// Folding it in would report "3 authorized peers" for two peers and
	// the network, and the arithmetic would be the reassuring half of a
	// wrong answer.
	head := fmt.Sprintf("authorized peers: %d (plus this peer's own entry)", others)
	if public {
		head += " — AND EVERY PEER THAT CAN DIAL THIS ONE, see the `default` row"
	}
	lines = append(lines, head)
	for _, p := range policies {
		label := p.PeerID
		switch {
		case p.IsSelf:
			// Named, never hidden. The kernel seeds this wildcard entry for
			// the peer itself; an operator auditing "who can read my
			// files" would otherwise find `*:*` granted to an unexplained
			// hex string and reasonably conclude they had been breached.
			label = p.PeerID + "   <- THIS PEER (kernel seed entry, not a grant to anyone else)"
		case p.IsPublic:
			// The opposite failure to IsSelf's, and the reason both are
			// marked: this row's name looks like one more peer, and it is
			// every peer. `publish -private` removes it.
			label = p.PeerID + "   <- NOT A PEER: every peer that can dial this one"
		}
		lines = append(lines, fmt.Sprintf("  %s", label))
		lines = append(lines, fmt.Sprintf("      %s", p.Summary))
		if p.Notes != "" {
			lines = append(lines, fmt.Sprintf("      (%s)", p.Notes))
		}
	}
	if len(problems) > 0 {
		lines = append(lines, "", "entries that did not decode:")
		for _, p := range problems {
			lines = append(lines, "  "+p)
		}
	}
	return LinesResult(lines), nil
}

// grantEffectLines renders whether the new grant is actually live.
//
// Shared by share and accept because the answer matters equally in both
// directions and getting it wrong in one place would be worse than not
// reporting it at all — an operator who reads "in force" once will assume
// it everywhere.
func grantEffectLines(reconnected bool, note string) []string {
	if reconnected {
		return []string{"  the connection was re-established, so the grant is in force now."}
	}
	return []string{
		"  NOT YET IN FORCE: " + note,
		"  (a grant is assembled during the handshake, so a policy written on a live",
		"   connection does nothing until that connection is re-established.)",
	}
}

var shareUsageDetail = strings.TrimSpace(`
share <root> with <peer> [-title TEXT]

Offer a folder you have mounted to another peer. Two things happen:

  1. a handshake policy is written authorizing that peer to subscribe to
     the folder and fetch its blobs — this is the permission;
  2. an offer record is published so they can find it with 'offers'.

A SYNC NEEDS BOTH SIDES. This authorizes them to read from you; they must
run 'accept' to authorize you to deliver to them. Until they do, their
subscription is accepted and no files ever arrive.

Related: shares, unshare, access, offers, accept, peers.
`)
