# Doctrine — crash forensics on the Avalonia/.NET/cgo substrate

Canonical. Living doc — edit in place.

**Open this at the START of a crash investigation, not after the obvious
things fail.** Its entire value is that the intuitive order is wrong and
costs days: this repo spent a month on one crash, produced four honest
negative results, and every one of them was taken with an instrument that
could not reach the bug.

The charter (`DISCIPLINE-CHARTER.md`) says *what* must hold. The substrate
model (`MODEL-AVALONIA-RUNTIME.md`) says what the platform *does*. This
says *how to proceed* when the process dies and leaves nothing.

---

## §0 The cycle this names

> **Reach → capture → classify → bisect → mitigate → gate.**

The recurring failure it exists to prevent is inverting the first two.
Given a coredump, the reflex is to start reading it. But a dump you cannot
reproduce is a single sample of unknown representativeness, and on this
substrate it is very likely *mislabelled* (§3). **Reproduction is not the
last step of the investigation; it is the first**, because everything
downstream — bisection, mitigation, and the regression gate you finish
with — is gated on being able to run the thing again.

Each step below owns exactly one lever.

---

## §1 Reach — build the instrument before reading anything

**Lever: does any harness we own execute the region the bug lives in?**

Ask it as a *region* question, against the seven boundaries of
`MODEL-AVALONIA-RUNTIME.md §6`, and answer it in writing. The four
negatives that cost a month were all Boundary A/D (layout, window
geometry). The bug was Boundary G (signals), reachable only through real
input.

Concretely, this repo's instruments and what each **cannot** do:

| Instrument | Reaches | Cannot reach |
|---|---|---|
| `make -C avalonia test` (headless) | models, panel mount, envelope decode, real Skia raster | X11 backend at all; input dispatch; the window manager |
| `smoke-xvfb-{driver,site,handlers,connections,program}` | real X11 paint, real dispatcher, model-driven churn | **input dispatch, hit-testing, focus transfer** — every one of these calls the model method *under* the control |
| `smoke-xvfb-window` | window geometry, iconify with a WM | input; also compositing WMs (openbox is not mutter/kwin) |
| **`smoke-xvfb-click`** | **real pointer input via xdotool → hit-test → handlers**, and since 2026-09-01 real **drags** (press → motion stream → release, `DRAG_PCT`), so pointer capture and scrollbar thumbs are in reach | keyboard-only paths; multi-touch; gestures needing modifier keys held |
| `make crash-hunt` | the above, swept over seeds, unattended | same limits, more samples |

**If no row reaches the region, building the missing instrument is the
task.** It is cheaper than it looks — `smoke-xvfb-click` was one package
(`xdotool`), ~40 lines of shell, and two make targets, and it hit on the
first seed after a month of misses.

**Every negative result is written with its reach in the same sentence**
(D24). "Survived 25 collapse cycles" is not a finding; "…and this harness
emits no X11 input, so it cannot reach input dispatch" is.

Determinism rules for the instrument itself:
- **Seed it, and log every event with its coordinates.** "It crashes
  randomly" is not actionable; "seed 7, click 143 at 812,455" is.
- **Do not trust a seed to reproduce.** This crash is timing-dependent:
  the same seed hit ~65% of runs. Sweep seeds; report the *rate*.
- **Write artifacts outside `dist-native/`** or copy them immediately —
  `extract` does `rm -rf` on it, so the next make invocation deletes the
  evidence.

---

## §2 Capture — the first signal, not the dump

**Lever: is the artifact you are about to read the actual fault?**

On this substrate, usually not. Run the reproducer under gdb and stop on
the **first** SIGSEGV with `nopass`:

```bash
make -C avalonia smoke-xvfb-click GDB=1 CLICK_SEED=7
```

`nopass` is the whole point: it keeps gdb from delivering the signal
onward, so the runtime never gets to handle, fail, and re-raise it. You
see the fault's own `rip`, `rsp` and `si_*`.

Two configuration traps that will waste a run:
- **Filter the runtime's own signals** or you stop on the wrong one:
  `handle all nostop noprint pass`, then re-enable SIGSEGV/SIGBUS. SIG34
  (`SIGRTMIN`) is CoreCLR injecting thread suspension and fires constantly.
- **gdb needs `--cap-add=SYS_PTRACE`** under podman.

Also capture, in the same run: `info proc mappings` (to identify which
mapping `rsp` is in — this is what distinguishes a managed-stack overflow
from a signal-stack one), and `info threads`.

---

## §3 Classify — read `si_code` before `si_addr`

**Lever: what kind of fault is this actually?**

**CoreCLR re-raises any fault it cannot classify.** When it does, the
signal that lands in the coredump is the *second* one, carrying:

```
si_code = 128 (SI_KERNEL),  si_addr = 0,  registers = the handler's
```

Read naively that is a null dereference. It is not; it is an artefact.
This misreading is AP34 and it cost this repo the initial diagnosis.

**But that signature has a second cause, and reading it as the first one
cost a further month.** Do not stop at `si_code` — the box below says
how to tell them apart, and it takes one query.

| Observation | Means |
|---|---|
| `si_code 128` (SI_KERNEL), `si_addr 0` | **two causes, and they are opposite.** See the box below — resolve it before doing anything else |
| `si_code 1` (SEGV_MAPERR) | genuinely unmapped address — a real wild pointer |
| `si_code 2` (SEGV_ACCERR) | **permission**, not absence — guard page. Almost always a stack overflow |
| `si_addr == rsp - 8`, rip on a `call` | the pushed return address hit a guard page: **stack overflow, confirmed** |
| `rsp` inside a `PROT_NONE` mapping | same, and the mapping's neighbours tell you *which* stack |

**`si_code 128` / `si_addr 0` has TWO causes and the discriminator is
`rsp`** (earned 2026-09-06, after this table's single reading cost a
second month):

1. **CoreCLR re-raised** a fault it could not classify. The registers
   are the handler's and the dump tells you nothing. This is AP34.
2. **The kernel could not build a signal frame at all**, so
   `force_sigsegv()` fired. This is what an alternate-signal-stack
   overflow looks like from the outside, and here **the registers ARE
   the fault site.**

Tell them apart by asking which mapping `rsp` is in:

| `rsp` is in | Means |
|---|---|
| a `PROT_NONE` page directly below a small rw- region | **case 2** — alt-stack overflow. The rw- region's size is the stack that blew |
| the thread's own stack (just below its `fs_base`) | **case 1** — ordinary re-raise |
| a registered alt stack, mid-region | **case 1** — the handler was running normally |

A cheap corroborator: on every thread EXCEPT the faulting one, `rsp`
sits a few hundred bytes below that thread's `fs_base`. If the faulting
thread is the only one where those two are in unrelated regions, you are
looking at case 2.

**And read `PT_LOAD`, not `info proc mappings`** (AP75). `info proc
mappings` on a core is served from the `NT_FILE` note, which lists only
**file-backed** mappings — so every stack, heap and guard page is absent
from it by construction and the tool answers *"not present in core"*
about memory the core records perfectly well. That reads exactly like a
wild pointer and is not. The authoritative record is the core's own
`PT_LOAD` program headers: `p_vaddr`/`p_memsz` for what was mapped,
`p_filesz` for what was dumped, `p_flags` for the permissions. Note
`p_filesz == 0` means *"mapped, contents not dumped"* — normal for every
file-backed executable page — and never *"absent"*.

The 2026-09-06 diagnosis was one `PT_LOAD` query: `rsp` 800 bytes into a
4 KiB `PROT_NONE` page, immediately below a 12 KiB rw- region, in both
cores, at the same offset. The session before it had read the same two
files with `info proc mappings` and recorded *"not a stack overflow"*.

Then ask **which stack**: if `rsp`'s region is a small (~16 KB) anonymous
mapping with a guard page, adjacent to libc/libpthread rather than to the
main `[stack]`, it is the **alternate signal stack**, and the overflow is
in signal handling. That is a different bug from a managed recursion, and
the runtime will not tell you: it prints no `Stack overflow.` and
`createdump` never fires, because there is no stack left to report on.

**And do not classify a recursion from a frame COUNT — measure the
stack.** `coredumpctl info` truncates a backtrace, so a run of identical
return addresses ending at systemd's limit says *"at least N"* and never
*"unbounded"*. On 2026-09-01 that distinction was the whole diagnosis:
45 printed frames read as a runaway recursion, and the same core says
uniform 448-byte stride, exactly 45 frames, 19.2 KB, in a thread with
≥1 MB of stack — an ordinary tree walk. **`make -C avalonia crash-stack`
prints this and now runs inside `make crash`.** It is AP55, and it is why
D25 exists.

| Observation | Means |
|---|---|
| repeated return address, **uniform** stride | a single call site recursing — the stride is the frame size |
| repeated return address, **mixed** stride | one call site reached by several paths; usually not a recursion |
| chain depth ≪ 1 MB | **not** a stack overflow, whatever the frame count looked like |
| recorded `rsp` absent from the core | the fault went through CoreCLR's handler, which runs on an alternate signal stack systemd does not dump. The recorded registers describe the handler. Use `fs_base` to find the real stack |

Three forensic channels that look productive and are dead ends here —
know them so you do not spend a day each:
- **The DAC will not load against a systemd ELF core** (`0x80004002`).
  There is no managed stack to be had from one. Only `createdump`'s own
  minidump carries what the DAC needs.
- **`dist-native/tools/dotnet-dump` cannot run on the host.** It is
  framework-dependent and sits beside a *self-contained* publish. Managed
  analysis runs inside the builder image (`make -C avalonia crash`).
- **`make crash` decodes Go symbols**, and these crashes have **zero**
  `libbridge.so` frames. A clean Go decode is not evidence of anything.

Cheap discriminators worth recording every time, so runs are comparable:

```bash
coredumpctl info $PID | grep -icE 'gallium|GLX_mesa|libGL\.|swrast'   # 21-ish = mesa driver bug; 0 = ours
coredumpctl info $PID | grep -c 'libbridge\.so'                        # 0 = not the Go bridge
```

---

## §4 Bisect — one variable, measured, never argued

**Lever: which hypotheses are actually excluded?**

With a reproducer in hand this is cheap, so **prefer an experiment over an
argument** (D19). Each run is one env var against the same binary and the
same seeds. Record the ruled-out list — it is as valuable as the answer,
and it is what stops the next session re-running them.

From the 2026-08-21 hunt, all ruled out as the trigger:

| Hypothesis | Test | Result |
|---|---|---|
| Go async preemption signals | `GODEBUG=asyncpreemptoff=1` | 5/5 still crashed |
| Background GC suspension churn | `DOTNET_gcConcurrent=0` | still crashed |
| Tiered-compilation rejit | `DOTNET_TieredCompilation=0` | still crashed |
| mesa/GPU driver | GPU-module discriminator | 0 modules, both dumps |
| the Go bridge | libbridge frame count | 0 frames, every dump |
| GridSplitter zero-size layout recursion (the documented predecessor) | grep the crashing runs' input breadcrumbs | **0** splitter presses in any of them |

**Measure the thing you are about to assert.** The altstack size was
*inferred* from a mapping table and then *measured* with `sigaltstack(2)`
at startup: 16384 bytes. The measurement is what made the fix defensible.

---

## §5 Mitigate — and be explicit about what is not explained

**Lever: does the change remove the crash, and do you know why?**

These are two questions and they can have different answers. Ship the
mitigation when the first is yes; **say so plainly when the second is no.**

The A/B is the deliverable, not the anecdote — same binary, same seeds,
one variable:

| arm | crashes |
|---|---|
| stock 16 KB altstack (`WB_ALTSTACK_BYTES=0`) | 6 / 8 seeds |
| 1 MB altstack (default) | 0 / 8 seeds |

**Keep the escape hatch that restores the broken behaviour.** Once a fix
lands, it is the only way anyone can re-measure the bug — and a mitigation
whose effect cannot be re-demonstrated becomes folklore in one session.

**A PER-INSTANCE MITIGATION NEEDS A POPULATION GATE** (AP74, and this is
the expensive lesson of the whole alt-stack arc). The 2026-08-21 fix
covered the UI thread. On 2026-09-02 the render thread crashed and was
added. On 2026-09-06 the process died twice on a *third* thread, with
sixteen per core still on the stock size. Each fix was correct, each
gate was green, and each gate's subject was **the threads somebody had
enumerated** rather than the process. When you cannot enumerate the
population, stop enumerating: `altstack-preload.c` interposes
`sigaltstack(2)` itself, so every thread is covered at the moment the
runtime creates it. The gate starts one ordinary thread and asks the
kernel what it got — a sample of the population, not a count of installs.

**What consumes more than 16 KB is no longer unidentified**, and the
answer generalises. Measured on both 2026-09-06 cores: the kernel signal
frame plus CoreCLR's handler prologue is ~6.1 KiB, then **one** frame of
6,960 bytes (`rbp-rsp = 0x1b30`, identical in both) — not a recursion,
three return addresses on the whole stack. Total 13,088 bytes against
12,288 usable. The reason it does not fit is the **CPU**: on an AVX-512
host the XSAVE signal frame is far larger than the PAL's compile-time
`SIGSTKSZ` assumed. Check it before assuming this is a .NET bug:

```bash
grep -o 'avx512[a-z]*' /proc/cpuinfo | sort -u          # is the wide state there?
getconf -a 2>/dev/null | grep -i sigstksz               # or sysconf(_SC_MINSIGSTKSZ)
```

On this machine: compile-time `SIGSTKSZ` 8192, `MINSIGSTKSZ` 2048,
`sysconf(_SC_MINSIGSTKSZ)` **3376**. When those disagree that badly, the
stock alt stack is undersized for every signal the process takes.

**A 40-line reproducer beats the coredump here**, and it is worth
writing before shipping the fix: mmap 16 KiB, `mprotect` the low page
`PROT_NONE`, install it with `sigaltstack`, install a `SA_ONSTACK`
handler with a ~7 KiB frame, dereference NULL. It dies with `rsp` in the
guard page — the same structural signature as the production cores — and
it survives under the interposer, same binary. That A/B is what turns
"the mechanism is inferred" into "the mechanism is demonstrated".

---

## §6 Gate — close the reach gap permanently

**Lever: would this bug survive a repeat?**

- The instrument built in §1 becomes a named target in `AGENTS.md` beside
  the suites, not a script in someone's shell history.
- The classification lesson becomes an anti-pattern with an enforcement
  point (AP34 → `GDB=1` mode; AP35 → `smoke-xvfb-click`).
- The substrate finding becomes a row in `MODEL-AVALONIA-RUNTIME.md §6`
  and an invariant in §7. **A new boundary is a real discovery** — six
  boundaries described every previously-diagnosed bug and none described
  this one, which was the tell that the map was short a row rather than
  that the bug was exotic.
- The app carries its own black box: breadcrumbs recorded unconditionally,
  input recorded *tunnelling* (before the target handler, or a crash in
  dispatch leaves no trace of the click), a durable crash log outside the
  wiped build directory, and the load-bearing runtime config announced on
  stderr at startup because it is the first thing a report needs.

---

## §7 The order, as a checklist

1. Name the region against `MODEL-AVALONIA-RUNTIME.md §6`. Does an
   instrument reach it? If not, **build one** — that is the task.
2. Reproduce. Seed it, log every event, sweep seeds, report the **rate**.
3. Catch the **first** signal under gdb with `nopass`.
4. `si_code` before `si_addr`. Identify **which stack** `rsp` is in —
   from the core's **`PT_LOAD` headers**, never from `info proc
   mappings`, which cannot see anonymous memory and will call a mapped
   guard page absent (AP75). `PROT_NONE` immediately below a small rw-
   region is an alternate signal stack, and `si_code 128` over it means
   the kernel could not build a frame, not that anything was re-raised.
   Then run `make -C avalonia crash-stack` and read the GEOMETRY before
   believing any classification — a frame count from `coredumpctl info`
   is a lower bound, not a depth (D25/AP55).
5. Bisect by experiment, one variable, same seeds. Record the exclusions.
6. A/B the mitigation. Keep a switch that restores the bug.
7. Gate it, and state what remains unexplained.

**Step 0, added 2026-09-01 and it comes before all of them: establish
what the artifacts actually are, by measurement.** All launch targets now
share one wrapper and the app records its own arming
(`external diagnostics: minidump=… perfmap=… panel-log=…`, on stderr and
in the crash trail). **If that line says OFF, relaunch armed rather than
analysing harder.** If there is no such line — an older build — the
crashed process's **entire environment is inside the coredump**:

```bash
coredumpctl dump <pid> --output=/tmp/c
grep -ac DOTNET_DbgEnableMiniDump /tmp/c    # was createdump even on?
```

Run it before concluding anything about a missing dump. On 2026-09-01
that one command refuted a written-up finding that the crash had been
taken unarmed — it was armed, and **createdump produced nothing anyway**,
as on 2026-08-21. "The variable was unset" and "the runtime declined to
dump" are different bugs with different owners, and they are
indistinguishable from the outside. See AP54.

**Anti-order** (what this doctrine exists to stop): read the dump →
theorise → run a harness that cannot reach the region → record a negative
→ conclude the bug is rare → repeat next month. **A second anti-order,
earned 2026-09-01:** read a number off the dump → find it plausible →
ship a fix for it → write it into STATUS, the charter and a source
comment → have the same dump refute it an hour later.
