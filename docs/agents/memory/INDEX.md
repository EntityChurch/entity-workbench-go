# Memory — index

This is what a session in this repo would otherwise rediscover the hard way. **`AGENTS.md` is how
to work here today; this is what we learned.** Open a topic when its subject is your subject —
these are reference, not reading.

## How to use it

**Find by symptom.** Every entry leads with what you would actually observe, states the mechanism
before the fix, and points at `file:line` rather than at prose. That ordering is deliberate: most
of these were expensive precisely because the symptom pointed at the wrong layer.

**Entries are superseded, not appended.** When something here is wrong, correct it in place — git
holds the history. Several entries deliberately keep a retracted claim visible with the
correction beside it, because the error is the transferable part; those say so.

**The rule that bounds this directory:**

> **An entry that could become a check SHOULD become one — and then it is deleted from here.**

Memory is where a finding waits *while it is still only prose*. It is not where findings retire.
So a maintenance pass is not "trim the files"; it is, per entry: *could a test, a lint rule, a
build assertion or a gate make this impossible instead of merely documented?* Many entries here
already name their gate — those are candidates for deletion once the gate is load-bearing, and
the ones that name a **mutation-checked** gate are the strongest candidates.

## The files

| file | covers |
|---|---|
| [`BUILD-AND-ENVIRONMENT.md`](BUILD-AND-ENVIRONMENT.md) | Toolchain pins, the sibling-checkout requirement, what each entry-point target does, and the build-output traps that make you measure the wrong binary. |
| [`TESTING-AND-SWEEPS.md`](TESTING-AND-SWEEPS.md) | How to get a true reading of the tree's state, which suites lie to you and why, the podman collider, the load-dependent failures, and the harness gates that reach real input. **Read before quoting any failure count.** |
| [`AVALONIA-GUI.md`](AVALONIA-GUI.md) | Panel construction, the framework traps that shipped defects at full green, the render/DTO boundary, and which panel answers which question. |
| [`CRASH-FORENSICS-AND-SIGNALS.md`](CRASH-FORENSICS-AND-SIGNALS.md) | What a .NET/Linux crash actually reports versus what it means, alternate signal stacks, and the cgo lifetime rule. Open `docs/architecture/DOCTRINE-CRASH-FORENSICS.md` alongside it. |
| [`FOLDER-SHARING-AND-SYNC.md`](FOLDER-SHARING-AND-SYNC.md) | The declare-then-reconcile control loop, direction, conflicts, delivery saturation and catch-up, connection reachability, and the verbs that drive all of it. The largest file here, and the area with the most operator-visible failure modes. |
| [`MOUNTS-AND-LOCAL-FILES.md`](MOUNTS-AND-LOCAL-FILES.md) | The two layers of a mount, what persists where, watcher liveness, history recording, and the tree-to-disk direction we do not run. |
| [`CAPABILITIES-AND-GRANTS.md`](CAPABILITIES-AND-GRANTS.md) | The four grant dimensions, what an absent one defaults to, the per-peer policy table, and the fixture wildcard that deletes a whole stage of the product from the suite. |
| [`SDK-AND-STORE.md`](SDK-AND-STORE.md) | Addressing, path qualification, what routes remotely versus locally, derived indexes that need rebuilding at open, and the SDK shape rules. |
| [`PUBLISHING-AND-CONSUMING.md`](PUBLISHING-AND-CONSUMING.md) | The CDN corridor and the live road: minting a signed root, the transport ladder, the verification stack both roads share, and the refusals that were wrong. |
| [`FEED-AND-SOCIAL-CONVENTIONS.md`](FEED-AND-SOCIAL-CONVENTIONS.md) | EMBED, FEED and the data-exchange mirror: what is built, what agrees cross-implementation, and which reader obligations have no gate here. |
| [`PROGRAMS-AND-COMPUTE.md`](PROGRAMS-AND-COMPUTE.md) | The `programs/` track, the input-port contract, determinism, and the Axis-1 admission corpus that is not wired into any target. |
| [`CROSS-REPO-ROUTING.md`](CROSS-REPO-ROUTING.md) | Delivery versus intention, the counterpart trackers, packet form, marking which sentences are measured, and pricing work against the substrate rather than our own tree. **Read before opening a packet.** |
| [`DOCS-AND-PUBLICATION.md`](DOCS-AND-PUBLICATION.md) | `CANONICAL-DOCS.toml` as a published artifact and a keep-list, and where project state actually lives. |

## Provenance

These files were split out of `AGENTS.md` on 2026-09-17, when it had reached 279 KB — nine times
the 30 KiB budget an agent context assumes, and past the point where several agents silently
truncate a project doc. **The content moved verbatim; nothing was rewritten, condensed or
dropped in the split**, which was the point: the findings were never the problem, the filing was.
`AGENTS.md` had been the only file whose name invited a session to put something there.

The promotion question above has **not** yet been asked of these entries. That is deliberate —
mixing a move with a rewrite arrives unreviewable — and it is the next pass.
