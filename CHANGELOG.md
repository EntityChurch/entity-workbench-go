# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Figures.** A site's `assets/` subgraph is read, and the site convention's embed
  directive (`::embed[caption]{ref=assets/figures/x.png}`) renders as a picture in the
  desktop browser instead of as literal text. On the live federation that is 665
  committed figures on one site that were previously unreachable. Asset references are
  resolved only inside the site's own signed subgraph — an external URL, an absolute
  path, a `data:` URI or a `..` escape is refused, so a page body cannot make the
  renderer fetch something the publisher never committed.
- **Tables** render as tables, with links and figures inside cells still working.
- **HTML pages** — a page published as `format: html` is lowered to readable text with
  a note saying what was lost, instead of being parsed as markdown and drawn as its own
  source. Bodies are capped for display, with the full verified size still reported on
  the trust chain. `docs/architecture/HTML-PAGE-RENDERING.md` records why an embedded
  browser engine is not the answer today and what is.
- **A content cache** in the consume stack, keyed by content hash and by signed root
  hash. Bytes that have been fetched and proved once are not fetched again.
- **Share a folder with another machine.** The whole flow reaches both frontends:
  `share`, `offers`, `accept`, `direction`, `shares`, `unshare` in the shell, and a **Sync**
  panel in the desktop app that is two gestures — pick a folder and a peer and share it,
  or pick a directory on an arriving offer and accept it. Accepting takes the directory
  you choose, and refuses a non-empty one unless you say otherwise, because the sender's
  writes would overwrite what is already there. The two machines do **not** have to name
  the folder the same thing. A folder can be one-way in either direction or two-way, set
  per machine with `direction`, and both sides must declare two-way for the reverse leg
  to exist — the command says so, on the machine you are looking at, and names the
  command to run on the other one.
- **A file explorer.** Mount a local directory and browse what it produced: files with
  their kinds, a preview pane, and both numbers for a mount — how many files were
  admitted and how many became documents — so a directory of photographs and one README
  no longer reports as a single figure that hides the difference.
- **Publishing from the application.** A `publish` verb and a publish bar on the Local
  Site panel mint a signed root over a prefix and, optionally, emit the static directory
  for a host. `publish -public` also grants every peer the read a verifier needs;
  `-private` withdraws it. The prefix is sticky after the first publish, because after
  the first publish something is known and a constant default would quietly change what
  your peer commits to.
- **Reading a site live from the machine that published it.** When a publisher is
  reachable, the browser asks them directly instead of fetching a copy from a host. It is
  the same verification either way — the same recomputed hashes, the same signature, the
  same fail-closed walk — so what the live road buys is not fresher bytes but the removal
  of a third party who could serve an old signed root and say nothing. Every reading says
  which road answered and what that reading is worth.
- **A feed.** `post` writes an entry; `feed` shows yours; `follow`, `unfollow`, `follows`
  and `timeline` read other peers'; `ref` resolves a reference and reports which of four
  outcomes it got, including the difference between *the publisher withdrew this* and
  *this root never covered that part of their tree*. There is a Feed panel with the three
  actions kept apart by what they cost: listing who you follow contacts nobody, reading a
  timeline dials every publisher you follow, and catching up also moves your saved reading
  positions. Following requires nothing of the person followed and does not tell them.
  **Publishing your feed so that readers can tell who wrote each entry is a button**, in the
  same panel, beside a line that says whether anybody can currently read what you posted —
  and whether a reader fetching it as static files could attribute a single entry of it.
  That publish commits to your entries, the index, and the signature attributing each
  entry, and to nothing else: not your folders, not the machines you have paired with, not
  your documents.
- **Conflicts are noticed, listed and undoable.** When a delivered file lands on a version
  you had edited, the replaced version is recorded and `conflicts` lists it; `resolve`
  puts yours back. Previously nothing was destroyed and nothing said so, which is the same
  thing as a loss. The default converges on the sender's version and records what it
  replaced; a folder can instead be set to keep both, and **the folder owner's rule is the
  one that applies** on every machine.
- **A shared folder keeps its history.** Recording is switched on for folders that receive,
  so `history query` can show every version of a path and, for each, whether it was your
  edit or their delivery — which is the question worth asking on a shared folder.
- **Recovery from a burst.** A large copy into a shared folder used to deliver part of it
  and stop, silently, with both machines reporting healthy. A catch-up pass now re-derives
  what is missing by asking the sender what they hold; it runs on a timer that speeds up
  while it is finding things and backs off when it is not, and **Catch up now** is a button.
  The pass reports what it recovered — a number that should be zero and is worth watching.

### Changed

- **The browser is roughly an order of magnitude faster to navigate.** Measured against
  the live federation: opening a page 7.3 s → 2.4 s, following a link 6.2 s → 0.36 s,
  going back 5.9 s → 0.23 s. The verification is unchanged; what changed is that a
  content-addressed body is no longer re-downloaded and re-hashed on every click, and
  that a trie walk runs concurrently rather than one node at a time.
- **The pinned registry is shown in the browser chrome**, with the two facts about it
  that were being computed and silently dropped before reaching the screen: whether the
  pin was nominated by the origin itself (trust-on-first-use) and whether the layout was
  re-based onto a co-hosted peer.
- **The page no longer resets while you read it.** Lists and the page body are rebuilt
  only when they change, scroll position is remembered per page, and the trust chain is
  dimmed during a navigation rather than blanked — the chain beside a page always
  describes that page.
- **The desktop app is the same peer every time you open it.** It now keeps its identity,
  its store and its listener by default, and announces itself on the local network. Before
  this it was a new peer on every launch, which silently invalidated every grant, mount and
  accepted share from the previous session — so debugging a two-machine share by relaunching
  destroyed the state being debugged. `--ephemeral` asks for the old throwaway behaviour.
- **Sharing panels update themselves.** The three that had Refresh buttons now watch the
  tree like every other panel. A Refresh button on data the tree can announce was a missing
  subscription wearing a control.
- **The desktop app remembers its panel arrangement** between launches, and says so when a
  saved layout could not be read rather than quietly opening on the defaults.
- **A peer is only shown as connected when we can actually reach it.** A session the far
  peer opened to us is no longer reported as a working connection, because that is exactly
  the state in which sharing is half-broken.

### Fixed

- **A published root whose sequence number goes backwards is refused.** A verifying
  reader now spans a session, so an origin replaying a previous publish page by page —
  every response correctly signed — is detected rather than invisible.
- Markdown link-reference definitions no longer render a spurious "unsupported block"
  notice; inline HTML inside a markdown page is lowered to its text rather than dropped.
- **Sharing one folder granted a read of the whole machine.** The permission written when
  you shared a folder covered every entity and every mounted file on the peer, not the
  folder. Measured across the wire: a file from an unshared folder came back, with its
  content hash. The permission is now derived from the folder being shared.
- **Deletes never propagated**, and the acknowledgement said they had. A delete was
  dispatched at the sender's path on the receiver, which only coincides when both machines
  happen to name the folder identically.
- **Every subscription was dead after a restart.** The engine's index was rebuilt from the
  tree by nothing, so a restarted peer resumed its watcher, reported its mounts healthy and
  never produced another document.
- **The first change after a restart is late, not lost.** It arrives on the next catch-up
  pass with no action from you. This was recorded as a permanent loss for five days on the
  strength of a test window shorter than the mechanism's own retry interval.
- **`--identity` could not create an identity**, so the example every document carried could
  not work on a first run. `--new-identity` is the creating form.
- **Peers discovered on the local network were invisible to everything that consumed
  discovery.** Three consumers joined on a field that the protocol leaves empty until later
  in the handshake, so they silently matched nothing, for every peer, on every pass.
- **A remembered address is now a hypothesis, not the answer.** A peer that has moved is
  found by its live announcement and the working address is written back; previously a
  stored address was tried forever while the peer sat on the network announcing a different
  one.
- **Sites written through the SDK were invisible to the application that reads them** — the
  writer and both readers disagreed about where a site lives.
- **A publish over a prefix with nothing in it** produced a well-formed, correctly signed,
  entirely empty origin. It is refused, and the refusal names what the tree actually holds.
- **A large folder could never finish transferring.** The bounded catch-up walk restarted
  from the beginning every pass, so beyond its limit the same files were re-examined forever
  and the rest were unreachable. It resumes where it stopped.
- **Several crashes in the desktop app.** The cause was signal-stack exhaustion on threads
  that had no enlarged stack installed — the platform default is far too small for this
  process's handler chain under real pointer input, and the size needed turns out to depend
  on the CPU. A separate hazard in list-row templates could end the process on an ordinary
  list update. An earlier diagnosis of one of these as runaway recursion was wrong and was
  withdrawn: the evidence for it was a truncated backtrace read as an unbounded one.
- **A file's timestamp could end the process.** Every file row threw on a unit mismatch, and
  the ninth click killed the app.

### Known limitations

- The first change to a shared folder after a peer restarts arrives on the next catch-up
  pass rather than immediately. No upper bound is promised; **Pull now** forces it.
- The incoming leg of a sync carries no rollback witness, so an out-of-order delivery is
  followed rather than refused. The statically published road does refuse one. Every
  folder reading says which of the two it is.
- A folder's conflict rule can be set from the shell and from no control in the desktop app.
- Reading feeds is one implementation against one publisher; there is no mirror, and no
  verb removes an entry once posted.
- Version history is recorded and never pruned. A file rewritten continuously will grow it
  until a per-path budget stops recording for that path, which keeps the oldest versions
  and loses the newest — the folder reading says when this has happened and what to do.

## [0.9.0]

The first release since `v0.8.0`. The theme is **reach**: several arcs that were
complete at the model layer but had no surface a person could touch now have one,
and the CDN corridor gained its second end.

### Added

- **The consume leg of the CDN corridor — a journey, not an inspector.** A name now
  resolves the whole way: registry → binding → transports → the target's signed
  published root → a verified walk → a page. Three surfaces drive it — the
  `registry` / `browse` / `open` verbs in `entity-shell`, the **Browser** panel in the
  desktop app, and `entity-fetch -registry`.
- **A publisher that emits a real signed root.** `entity-publish` writes a signed
  `system/peer/published-root` and uploads the full closure of `root_hash`; the
  http-poll transport profile it advertises is what a consumer enters through, so no
  URL is derived by convention on either side.
- **Peer liveness read from the tree.** `peer status` in the shell and a liveness
  surface in the desktop app, both backed by the `system/peer/status` entity rather
  than by a connection-pool snapshot — so `suspect` is expressible and a transition
  carries its reason.
- **Transport self-publication.** A listening peer advertises its own transport
  profile under its own peer-id, wired into the shell's listener bind.
- **The handler browser and the name arc**, each reachable from a menu or a verb
  rather than existing only as a model.
- **`programs/`** — a module of its own (extracted from `workbench/`) holding the
  generic compute host, the program descriptors, and the Life / Snake / Asteroids /
  heavyfield programs, with a standard controller input model and an interactive Life.
- **A front door.** `make doctor` (is this machine set up?), `make run`, `make gui`,
  `make gui-run`, `make demo`, and `make test-each` — the last runs every Go suite to
  completion and prints a pass/fail table instead of stopping at the first failure.
- **`make reachability`** — a sweep that fails when a model has no user-reachable
  surface, and **`make crossimpl-go`**, which stands up the Go reference
  implementation's federation publisher in a container and drives our verifying
  consumer at it over a real network.

### Changed

- The desktop app defaults to **software rendering**; the GPU path is opt-in.
- The three legacy per-program panels were retired in favour of the generic host.
- `make build` / `test` / `lint` / `gui` now run a **preflight** that names the
  missing sibling checkout in one sentence instead of failing forty lines deep in
  module resolution.

### Fixed

- **A crash that had gone unreproduced for a month** in the desktop app under real
  pointer input: exhaustion of the UI thread's alternate signal stack, whose platform
  default is 16 KB. Measured at 6 of 8 seeds crashing before, 0 of 8 after.
- **Interactive Life's on-screen controller was inert from the day it shipped**,
  through two independent defects — the host sampled its input ports only at tick
  time, so a click shorter than a tick was never observed (1 of 6 presses landed), and
  the toolkit was silently discarding the panel's pointer handlers. Every value
  offered to a port is now observed by exactly one tick, in order.
- **Life's "Regen" was sliding one fixed pattern rather than generating a new one** —
  a linear hash under a power-of-two modulus makes bumping the generation counter
  arithmetically equivalent to shifting the cell index.
- **The query index did not survive a restart.** With a persistent store the tree
  came back and the index did not, so `find`, `grep` and `compute aggregate` were
  blind to everything written before the process started while `ls` listed it
  happily. The index is now rebuilt when the peer opens.
- **`compute aggregate` could not read a numeric field written by the shell's own
  `put`** — JSON numbers decode as floats and the aggregate accepted only integer
  kinds. Non-integral values are now refused rather than truncated.
- **An experimental alternate compute engine mishandled an error raised inside a
  collection operation.** In this system an error is an ordinary value, and what
  happens to one depends on where it lands: `map` *places* each result into its
  output array, so a failing element becomes an error value there and the
  operation succeeds; a filter predicate is *read*, so a failing one stops the
  filter. The alternate engine stopped in both cases, which made
  `length(map(items, f))` report a failure where the reference engine reports the
  item count. The same distinction was missing for `fold`'s accumulator (a
  function that ignores a failed accumulator is supposed to recover), for an error
  read back out of a collection, and at the point where a contained error is
  written out — it now reduces to its code alone, so two implementations that word
  the same failure differently still agree on the bytes. Found by the 300-case
  equivalence sweep, which is green for the first time; five new
  position-by-position vectors pin it, since a generator reaches this shape only by
  luck of the draw. *(Earlier releases of this file listed this as an open question
  with one failing case and no known cause. All three parts of that were wrong: there
  were three cases — the sweep stopped at the first — and the cause had in fact been
  correctly identified before being retracted on a probe that could not test it.)*
- A peer held ~20 MB for a delivery ring it never released.
- A persistent store now yields a persistent peer identity across restarts.
- The watch hub could send on a closed channel.

### Known limitations

Stated because they are real and reproducible, not because they are comfortable:

- **A sibling `entity-core-go` checkout is required to build.** Every module resolves
  the kernel through a local `replace`, so this repo must be cloned beside it —
  README § *Repository layout* has the shape, `make doctor` verifies it, and the build
  now refuses early with instructions. There is no published module path yet, so
  `go get` of the SDK is not available in this release.
- **A rare failure under full-suite load** in a bidirectional burst-write end-to-end
  test, traced to a terminal write loss below this layer and routed upstream; and one
  unidentified desktop-test flake observed once in eight runs.
- **117 short-SHA citations across the published documents do not resolve for a reader
  of the public history**, which is authored fresh at the release boundary. They are
  provenance notes on internal commits — not links you can follow — and replacing them
  with content-addressed citations is in progress. **72 of them are in
  `docs/STATUS.md`**, the rolling engineering log, which is published deliberately: it
  is written for the next working session first, and it cites the commits that session
  would look up. The remainder sit in the framework documents, where each pins the
  defect that earned a rule.

---

## [0.8.0]

- Initial public research-preview release.
