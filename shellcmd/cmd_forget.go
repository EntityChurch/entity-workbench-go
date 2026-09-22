package shellcmd

import (
	"fmt"
	"strings"
)

func cmdForget(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return LinesResult([]string{
			"usage: forget <peer>      drop every relationship with one peer",
			"       forget --all       drop every relationship with every peer",
			"",
			"Removes what this peer REMEMBERS about another: inbound syncs, outbound",
			"offers, their authorization, and the live connection. The next handshake",
			"is then a real one rather than a replay of a grant that was already there.",
			"",
			"It deletes NO files and unmounts nothing. Received files are yours; a",
			"folder you want emptied is one you empty yourself.",
			"",
			"It does not change this peer's own identity — relaunch with a different",
			"--identity for that.",
		}), nil
	}

	if args[0] == "--all" || args[0] == "-a" || args[0] == "all" {
		outs, err := sh.ForgetAll()
		if err != nil {
			return Result{}, err
		}
		if len(outs) == 0 {
			return MessageResult("nothing remembered about any peer — already clean"), nil
		}
		lines := []string{fmt.Sprintf("forgot %d peer(s):", len(outs))}
		problems := 0
		for _, o := range outs {
			who := o.PeerID
			if o.PeerAlias != "" {
				who = fmt.Sprintf("%s (%s)", o.PeerAlias, o.PeerID)
			}
			lines = append(lines, fmt.Sprintf("  %s — %s", who, o.Summary()))
			for _, p := range o.Problems {
				problems++
				lines = append(lines, "      ! "+p)
			}
		}
		lines = append(lines, "", forgetCaveat(problems == 0))
		return LinesResult(lines), nil
	}

	out, err := sh.Forget(args[0])
	if err != nil {
		return Result{}, err
	}
	who := out.PeerID
	if out.PeerAlias != "" {
		who = fmt.Sprintf("%s (%s)", out.PeerAlias, out.PeerID)
	}
	lines := []string{fmt.Sprintf("forgot %s", who), "  " + out.Summary()}
	if len(out.SyncsStopped) > 0 {
		lines = append(lines, "  syncs stopped:     "+strings.Join(out.SyncsStopped, ", "))
	}
	if len(out.OffersWithdrawn) > 0 {
		lines = append(lines, "  offers withdrawn:  "+strings.Join(out.OffersWithdrawn, ", "))
	}
	for _, p := range out.Problems {
		lines = append(lines, "  ! "+p)
	}
	lines = append(lines, "", forgetCaveat(out.Clean()))
	return LinesResult(lines), nil
}

// forgetCaveat states the two things that are still true after a
// forget, because both of them look like a bug to an operator who has
// just asked for a clean slate and is about to re-run the flow.
func forgetCaveat(clean bool) string {
	base := "files already received are untouched, and the other peer still remembers YOU —\n" +
		"run `forget` on that machine too before treating the next run as a clean test."
	if clean {
		return base
	}
	return "SOME STAGES FAILED (above) — this peer is only partly forgotten, so the next\n" +
		"run is not a clean test. " + base
}
