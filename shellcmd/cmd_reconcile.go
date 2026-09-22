package shellcmd

import (
	"fmt"
	"time"
)

// `status` — what is declared, what is actually true, and the difference.
//
// This is the verb the flow did not have. `mounts`, `shares`, `syncs`,
// `access` and `peers` each answer one question about one record, and an
// operator asking the only question they ever ask — *is this working* —
// had to run five verbs and join the answers themselves. Worse, all five
// read the TREE, and every restart defect this product shipped was a case
// of the tree being intact while the derived runtime state was gone: so
// the five verbs agreed that everything was fine, on a peer where nothing
// could possibly work.
//
// `status` runs the reconciler, which is what makes it able to answer.
// A read-only report over the same records would be a sixth verb with the
// same blind spot.
//
// # Why running a control loop from a status verb is not a trick
//
// A reconcile pass over a settled system writes nothing, dials nothing
// new and reconnects nobody: it is safe by construction, and the run is
// what turns "declared" into "verified". A status command that CANNOT
// change anything also cannot notice anything that a stale record hides,
// which is the failure mode being fixed. The pass says what it did.

const reconcileTimeout = 20 * time.Second

func cmdStatus(sh *Shell, args []string) (Result, error) {
	_ = args
	if sh == nil || sh.ShellWorkspace == nil {
		return Result{}, fmt.Errorf("no workspace")
	}
	out, err := sh.ReconcileWithTimeout(reconcileTimeout)
	if err != nil {
		return Result{}, err
	}

	var lines []string

	if len(out.Devices) == 0 && len(out.Folders) == 0 {
		lines = append(lines,
			"nothing declared on this peer.",
			"",
			"  `share <root> with <peer>`  offers a folder you have mounted",
			"  `accept <peer> <root>`      takes up a folder offered to you",
			"",
			"Both record a durable declaration; from then on this peer",
			"re-establishes the relationship by itself at every start.")
		return LinesResult(lines), nil
	}

	if len(out.Devices) > 0 {
		lines = append(lines, fmt.Sprintf("peers: %d", len(out.Devices)))
		for _, d := range out.Devices {
			state := "offline"
			switch {
			case d.Paused:
				state = "paused"
			case d.Connected && d.Maintained:
				state = "connected"
			case d.Connected:
				state = "connected (not maintained)"
			case d.Maintained:
				// The distinction that matters after a restart: the
				// relationship is being kept alive and the peer is simply
				// not there yet. Reported as "retrying" rather than
				// "offline" because those are different situations and
				// only one of them needs an operator.
				state = "offline, retrying"
			}
			addr := d.Address
			if addr == "" {
				addr = "(no address; by peer-id only)"
			}
			lines = append(lines, fmt.Sprintf("  %-12s %-26s %s", state, addr, d.Label))
			// What we grant them, always — it is our own row, so it is
			// exact. There is no inbound counterpart on purpose: their
			// capability table is not readable from here, and the only
			// honest evidence is the folder file counts below.
			lines = append(lines, "               you grant: "+d.OutboundGrant)
			if d.Note != "" {
				lines = append(lines, "               "+d.Note)
			}
		}
		lines = append(lines, "")
	}

	if len(out.Folders) > 0 {
		lines = append(lines, fmt.Sprintf("folders: %d", len(out.Folders)))
		for _, f := range out.Folders {
			origin := "shared out"
			state := "mounted"
			if !f.Local {
				origin = "received"
				switch {
				case f.Mounted && f.Syncing:
					state = "syncing"
				case f.Mounted:
					state = "mounted, not syncing"
				default:
					state = "NO MOUNT"
				}
			} else if !f.Mounted {
				state = "NO MOUNT"
			}
			lines = append(lines, fmt.Sprintf("  %-10s %-22s %s", origin, state, f.Label))
			// Both sides of the mount's lossy stage, and an explicit
			// unknown when there is no mount to count (AP59): "nothing
			// arrived" and "there is nowhere for it to arrive" are
			// different claims, and for a folder in trouble it is
			// almost always the second.
			//
			// NOT "on disk", which is what this said and which collides
			// with the sweep's genuine filesystem walk. FilesPresent is
			// the source LAYER — what the watcher admitted into the tree.
			if f.FilesObservable {
				lines = append(lines, fmt.Sprintf("               %d file(s) admitted by the watcher, %d readable as documents",
					f.FilesPresent, f.FilesIngested))
			} else {
				lines = append(lines, "               files: unknown — there is no mount to count")
			}
			for _, p := range f.PeerStates {
				lines = append(lines, "               "+p.PeerID+" ("+p.State+")")
			}
			if f.Note != "" {
				lines = append(lines, "               "+f.Note)
			}
		}
		lines = append(lines, "")
	}

	if len(out.Actions) > 0 {
		lines = append(lines, "this pass changed:")
		for _, a := range out.Actions {
			lines = append(lines, "  "+a)
		}
		lines = append(lines, "")
	}
	if len(out.Problems) > 0 {
		lines = append(lines, "problems:")
		for _, p := range out.Problems {
			lines = append(lines, "  "+p)
		}
		lines = append(lines, "")
	}

	// The catch-up supervisor, said out loud. A background loop that
	// silently repairs things is only half a feature: an operator who
	// cannot see it running cannot tell "my files arrived because the
	// system healed itself" from "my files arrived and nothing was ever
	// wrong", and the second reading is the one that gets believed right
	// up until the loop is off.
	if last, ok := sh.LastCatchUp(); ok {
		lines = append(lines, last.Summary())
		if last.Recovered > 0 {
			lines = append(lines,
				"  (that many files had been silently lost to a delivery burst "+
					"and were recovered — run `catchup` to pass again now)")
		}
		// The RATE, because it is adaptive and the two regimes look
		// identical from a folder listing. "Checked every 5s because it is
		// still finding things" and "checked every 10 minutes because it
		// has been quiet since Tuesday" are the same table otherwise, and
		// an operator staring at a stale folder needs to know which.
		if iv, running := sh.CatchUpInterval(); running {
			state := "settled"
			if iv <= MinCatchUpInterval {
				state = "actively recovering"
			}
			lines = append(lines,
				fmt.Sprintf("  next pass in %s (%s; the rate adapts — it drops to %s "+
					"while files are being recovered and backs off to %s once they "+
					"are not)", iv.Round(time.Second), state,
					MinCatchUpInterval, MaxCatchUpInterval))
		} else {
			lines = append(lines,
				"  no supervisor is running in this process — nothing will pass again "+
					"on its own")
		}
		lines = append(lines, "")
	} else if len(out.Folders) > 0 {
		lines = append(lines,
			"no catch-up pass has run in this process — a folder that stopped "+
				"part way will stay that way until `catchup` or `resync`.",
			"")
	}

	// The recording growth guard, said out loud for the same reason as the
	// supervisor above and one step stronger: the limits themselves are
	// already in `problems:`, so the only thing left to say is whether
	// anything is COUNTING. A guard that is not attached reports no limits
	// and looks exactly like a peer with no runaway paths — which is the
	// reading that gets believed right up until the disk fills.
	if hb := sh.HistoryBudget(); hb.Running {
		lines = append(lines,
			fmt.Sprintf("change recording: %d version(s) recorded across %d path(s) "+
				"this session, budget %d per path",
				hb.Transitions, hb.Paths, hb.Budget))
		if hb.Tripped > 0 {
			lines = append(lines,
				fmt.Sprintf("  %d path(s) hit the budget in this session and stopped "+
					"recording — listed under problems above", hb.Tripped))
		}
		lines = append(lines, "")
	} else if len(out.Folders) > 0 {
		lines = append(lines,
			"change recording is NOT being counted in this process — a file that "+
				"something rewrites continuously will grow the tree without bound "+
				"and nothing here will notice.",
			"")
	}

	if out.Settled() {
		lines = append(lines, "settled — everything declared is established.")
	}
	return LinesResult(lines), nil
}
