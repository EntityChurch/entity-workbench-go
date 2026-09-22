# Build & environment

Toolchain pins, the sibling-checkout requirement, what each entry-point target does, and the build-output traps that make you measure the wrong binary.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **Go pinned to 1.25.1** (forced by core-go's `ext/go.mod` `go 1.25.0`) — the Makefile
  pins it. Per AGENTS-STANDARD, never set `GOTOOLCHAIN=` inline.
- **Sibling `../entity-core-go/` is required.** Every `go.mod` uses `replace` directives
  resolving to `../../entity-core-go/core` and `../../entity-core-go/ext`; without the
  sibling, `go build` fails at module resolution. README documents the layout.

- **`make lint` is `go vet` only — it does not check formatting.** Nothing gates gofmt, so
  drift accumulates silently (60 files at the 2026-08-18 audit). Run `make fmt` as its own
  commit, never folded into a feature diff.
- **`make textual` refuses a raw C0 control byte in a tracked source** (AP51). Seventeen of them
  — NUL, SOH, STX typed *literally* into C# string literals as separators — made
  `BrowserPanel.cs` **binary to git**: `Bin 36537 -> 64640 bytes`, no diff, no blame, no merge,
  on the 1513-line centre of that session's work. Nothing about the program was wrong, so every
  test stayed green. **Do not reach for `grep` here**: GNU grep matches on NUL-terminated C
  strings, so a NUL cannot appear in a pattern and `grep -P '\x00'` reports a file clean that
  `od -c` shows it sitting in — a false negative shaped like a pass. The fix is always escapes
  (`"\u0000"`), never a `.gitattributes` binary marker, which keeps the bytes and discards the
  diff. The tell is a source file `file(1)` calls `data`.
- `make test ARGS="-run X -v"` — single test / forwarded flags (`ARGS=` is the only
  passthrough syntax `make` accepts).
- `make test-sdk` / `test-shell` / `test-shellcmd` / `test-workbench` — per-package suites.
- **`bin/` is build output, not the tree — rebuild before you measure behaviour with it.**
  On 2026-08-31 `bin/entity-shell` was six days stale and still carried the pre-AP44 refusal,
  so a live check "reproduced" a bug that had been fixed and was one sentence from being
  reported as a regression in a shipped surface. `bin/entity-fetch` *had* been rebuilt, which
  made it worse: two binaries from the same tree disagreed, which reads as a code difference
  between `fetch` and `shellcmd`. The tell is a refusal message you cannot `grep` in the
  source — if the string is not in the tree, you are running an old binary. `make build` first,
  every time, before attributing anything to source.
- **Every build/test target now refuses early if the sibling kernel is missing** (`preflight`,
  added 2026-08-24, AP41). `doctor` had that check from the day it was written and **nothing
  called it**, so cloning this repo on its own produced forty lines of module-resolution spew and
  no cause — measured by someone doing exactly that. The predicate lives once as
  `SIBLING_PRESENT` and is shared with `doctor`. **No suite in this repo can regress
  this**: a suite that runs at all is running in a tree where the sibling resolved, so if you
  touch it, test it the only way that works — `make preflight PARENT=/tmp/no-such-parent`.
- **Entry points, for when you need to actually run the thing:** `make doctor` (prerequisites
  + the sibling kernel — run this first on a strange machine), `make run` (build + REPL;
  `ARGS=` for one-shot), `make gui` / `gui-run` / `gui-build` / `gui-test` (root-level
  passthroughs to `avalonia/`), `make demo` (scripted CLI tour in a throwaway HOME — the
  fastest end-to-end validation that the shipped binary works).
- **`make gui` rebuilds the image; `make gui-run` does not.** Use `gui` after any Go or C#
  change, `gui-run` to launch what is already extracted. Both forward the app's own flags —
  `make gui-run ARGS="--new-identity second-peer"` (double-dash: the .NET frontend does
  not use Go's `flag` spelling). **With no flags the GUI is a PERSISTENT, REACHABLE peer** —
  same peer-id, on-disk SQLite, a listener on `0.0.0.0:9110`, an mDNS announcement — and that
  is the configuration to run; `--ephemeral` asks for the throwaway one. **This bullet said the
  opposite until 2026-09-08**, five months of sessions copied `--identity me` out of it, and
  `--identity` LOADS an existing identity and **fails** when there is none — so the example
  every doc inherited could not work on a first run. `--new-identity` is the creating form.
  `avalonia/README.md` is the full entry-path doc.
  **Both now launch through `run-with-dump.sh`, and the rebuild is the only difference left.**
  Until 2026-09-01 `gui-run` exec'd the bare binary, so whether a session had diagnostics
  depended on which target you happened to type and nothing recorded which. The app now prints
  `external diagnostics: minidump=… perfmap=…` at startup and writes it to the crash trail; **if
  that line says OFF, relaunch armed before analysing anything.**
- **Run logs live in `avalonia/run-logs/`, NOT in `dist-native/`** (AP54). `extract` does
  `rm -rf dist-native`, and every build target runs `extract`, so a log written there is deleted
  by the next build — which is what happened to the 2026-09-01 crash's stderr, and it had
  `WB_PANEL_LOG=1`, meaning the complete breadcrumb stream up to the fault was on disk and gone
  before anyone looked. `DOCTRINE-CRASH-FORENSICS` §1 already said *"write artifacts outside
  `dist-native/`"*; we had written that about smoke artifacts and put the run log inside anyway.
  Logs are timestamped, never overwritten: the interesting run is rarely the most recent one.
- `make go ARGS="..."` — escape hatch; `ARGS` carries the subcommand (`vet ./...`,
  `mod tidy`, `env`, …).
- **Avalonia builds go through podman, always** — `cd avalonia && make build && make
  extract`, then `make host-run`. The host has no .NET and never needs it; never `dnf
  install dotnet`. Bridge-only smoke check: `cd avalonia/bridge && CGO_ENABLED=1 go build
  -buildmode=c-shared -o /tmp/libbridge-test.so .`.
