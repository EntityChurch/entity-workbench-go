# Crash forensics & POSIX signals

What a .NET/Linux crash actually reports versus what it means, alternate signal stacks, and the cgo lifetime rule. Open DOCTRINE-CRASH-FORENSICS.md alongside this.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **An async cgo export must copy every C-owned argument into Go memory BEFORE launching the
  goroutine** (AP31). A `*C.char` belongs to the .NET marshaller and is freed when the P/Invoke
  returns; reading it on the goroutine is a use-after-free that **does not crash** — it reads as
  the empty string and surfaces as a plausible user error. The synchronous exports beside it are
  safe for a reason that does not transfer.
- **On a .NET Linux crash, read `si_code` before you believe `si_addr`** (AP34). CoreCLR's
  handler **re-raises** any fault it cannot classify, so the signal that reaches the coredump
  carries `si_code 128` (SI_KERNEL) and `si_addr 0` — and a register context that is the
  handler's, not the fault's. Both 2026-08-21 desktop dumps read as null dereferences on that
  basis and were nothing of the kind. The real fault (`si_code 2`, SEGV_ACCERR, `si_addr =
  rsp-8`, rip on a `call`) is only visible by catching the FIRST SIGSEGV live —
  `make -C avalonia smoke-xvfb-click GDB=1`, which runs under gdb with `nopass`. Corollary:
  **a systemd ELF core cannot give you a managed stack** (the DAC rejects it, `0x80004002`),
  and `dist-native/tools/dotnet-dump` cannot run on the host at all — it is
  framework-dependent beside a self-contained publish. `make -C avalonia crash` now does the
  managed half inside the builder image.
- **A FIELD THAT PRINTS IS NOT A FIELD THAT ANSWERS** (D25, AP55) — the same failure as AP34, one
  field over, and the reason D25 is ratified. `coredumpctl info` **truncates** a backtrace, so a
  run of identical return addresses is a *lower bound* and can never say "unbounded". On
  2026-09-01 we read 45 such frames as a runaway recursion and shipped a depth bound for it; the
  same core says uniform 448-byte stride, exactly 45 frames, 19.2 KB, in a thread with ≥1 MB of
  stack — **an ordinary tree walk.** Run **`make -C avalonia crash-stack`** (now inside
  `make crash`) before believing any classification: it prints stride, uniformity, span, depth
  below the thread descriptor, and the verdict with its numbers. Two things it will tell you that
  nothing else does — a **uniform** stride is a single recursing call site while a mixed one is
  not a recursion at all, and a recorded `rsp` **absent from the core** means the fault went
  through CoreCLR's handler on an alternate signal stack systemd does not dump, so the registers
  describe memory the core lacks. Prefer a derived measurement to a printed count, always.
- **The UI thread's alternate signal stack is 1 MB, on purpose, and it is load-bearing.** The
  PAL default is **16 KB**, which is not enough for this process's handler chain under real
  pointer input: the click fuzz crashed **6/8 seeds** at 16 KB and **0/8** at 1 MB, same
  binary, same seeds. Installed at startup by `CrashDiagnostics.EnlargeAltStack`;
  `WB_ALTSTACK_BYTES=0` restores stock, which is the only way to re-measure the bug. Only the
  UI thread is covered by *that* call — a crash on another managed thread would look identical.
  **The render thread is covered too as of 2026-09-02, and it took the crash to do it** (AP66).
  `sigaltstack` is per-thread and must be called *on* the thread it covers, so something has to
  run there; Avalonia 11.2 offers no tidy hook (`IRenderTimer.Tick` is **internal**, `Compositor`
  callbacks run on the UI thread, `AvaloniaLocator.Current` is gone). **`AltStackProbe` is the
  answer**: a zero-size, hit-test-invisible control whose `ICustomDrawOperation` is executed by
  the compositor during the render pass — the same call stack the fault was taken in. It
  re-invalidates until the install lands, because a control that is never dirty is never drawn and
  the crashing session had been idle for 17 s. **Gate: `run-xvfb-smoke.sh` exits 3 if the run log
  lacks the render-thread line** — the only harness that can see it, since the headless suite has
  no render thread and there is no cross-thread query for another thread's alt stack. Other
  managed threads are still uncovered, so `run-with-dump.sh` stays non-optional.
  **Read `journalctl` FIRST on any hard crash.** The 2026-09-02 death was named outright by
  `kernel: signal: entity-avalonia[<tid>] overflowed sigaltstack`, which also converted §0J's
  "createdump produces nothing for this fault class" from an untested hypothesis into a
  measurement. Note the tid there is the **thread**, not the pid — if they differ, the fault is
  not on the main thread and every per-thread mitigation you installed at startup missed it.
  **That kernel line is not reliable, though — its absence proves nothing.** The 2026-09-06
  crashes were the same fault class with no journal line at all, because the message is printed
  at signal-frame setup and this overflow happened *inside* an already-running handler.
- **EVERY thread now gets a 1 MB alt stack, and the mechanism is an `LD_PRELOAD` interposer —
  not the managed installs** (`avalonia/altstack-preload.c`, loaded by `run-with-dump.sh` and
  `run-xvfb-smoke.sh`). The per-thread managed installs reach the UI thread and the render
  thread and **cannot reach anything else**, because `sigaltstack` must be called ON the thread
  it covers and no managed hook runs on a fresh thread-pool, finalizer or timer thread. On
  2026-09-06 the GUI died twice in four `make twopeer-gui` runs on exactly such a thread, with
  **16 PAL-shaped stock 16 KiB alt stacks still present per core** while both enumerated threads were correctly
  covered and every gate was green (AP74). Measured from the cores' `PT_LOAD` headers: 12,288
  bytes usable, **13,088 consumed**, 800 into the guard page, one 6,960-byte frame, *identical
  in both cores*. The extra demand is the **CPU** — this AVX-512 host reports
  `sysconf(_SC_MINSIGSTKSZ)` 3376 against a compile-time `SIGSTKSZ` of 8192 — so the overflow
  is **deterministic** and only the arrival of a signal is intermittent. `WB_ALTSTACK_BYTES=0`
  disables the interposer *and* the managed installs, which keeps the A/B honest.
  **The gate is a population sample, not an install count**: the app starts one ordinary thread
  and asks the kernel what it got (`CrashDiagnostics.ProbeAltStackCoverage`);
  `AltStackCoverageTests` asserts on it, and `run-xvfb-smoke.sh` exits 3 without
  `coverage=ALL-THREADS`. **A dlopen'd library does not interpose** — `DllImport` resolves
  `libaltstack.so` happily in a process that never preloaded it, so "the symbol is there" is not
  the check; the interposer's own call counter is.
