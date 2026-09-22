using System;
using System.Collections.Generic;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Threading;

namespace EntityAvalonia;

// CrashDiagnostics — the always-on forensic surface for a crash class
// that leaves nothing behind.
//
// WHY THIS EXISTS (measured, 2026-08-21). Two `entity-avalonia` SIGSEGVs
// on the same day produced, between them: no managed minidump (createdump
// did not fire even though run-with-dump.sh exports
// DOTNET_DbgEnableMiniDump=1), no managed stack (the DAC refuses to load
// against a systemd-coredump ELF core — `Failed to load data access
// module, 0x80004002`), and no breadcrumb naming the last user action
// (`make crash` decodes libbridge.so Go symbols, and neither faulting
// thread had a single libbridge frame). The `si_code` on both dumps is
// 128 / SI_KERNEL with `si_addr = 0`, i.e. the signal was RE-RAISED —
// so even the register context is not the fault site. Four independent
// forensic channels, four blanks.
//
// A crash we cannot characterize is a crash we cannot fix, and the
// operator cannot be the instrument (they do not have time to sit and
// manually reproduce). So the app records its own last moments:
//
//   1. A ring of the last N breadcrumbs — ALWAYS kept in memory, even
//      when WB_PANEL_LOG is unset — AND written through, line by line,
//      to ~/.entity/crash/entity-avalonia-<pid>.trail as each one is
//      recorded. PanelLog feeds it. Printing is opt-in; RECORDING is
//      not, because the crash decides when we needed it.
//   2. Global input breadcrumbs on the TopLevel, TUNNELING so they run
//      BEFORE the target's own handler. This is the specific blind spot
//      that cost us the 13:16 dump: the user clicked a link, the fault
//      landed before `SiteViewPanel.NavigateTo`'s first log line, and
//      the run log therefore ended eight seconds before the crash with
//      no hint that a click had ever happened.
//   3. Managed exception handlers (AppDomain / Dispatcher / Task) that
//      dump the exception, its stack, AND the breadcrumb ring.
//   4. A durable file under ~/.entity/crash/. It lives there and not
//      beside the binary because `make extract` does `rm -rf` on
//      dist-native, so anything written there is deleted by the next
//      build — which is exactly what happened to the 2026-09-01 crash's
//      `run.log`, and that log had the whole breadcrumb stream in it.
//      (Both `make up` and `make host-run` now also tee stderr to
//      avalonia/run-logs/, outside the same `rm -rf`.)
//
// LIMIT, stated plainly: a hard SIGSEGV is NOT a managed exception and
// will not run any handler here. What this class guarantees for that
// case is the durable TRAIL — every breadcrumb, on disk, up to the last
// one recorded before the signal. It converts "no information" into
// "the last action before it died", and it converts every *managed*
// fault into a full symbolized stack with zero operator effort.
//
// That guarantee was VACUOUS until 2026-09-01. The sentence above used
// to say "the breadcrumb ring on disk up to the last flushed line", and
// the only thing that ever flushed the ring was WriteFatal — i.e. the
// managed-fault handler, i.e. precisely the path the SIGSEGV case does
// not take. Measured cost of the gap: the 2026-09-01 SIGSEGV's crash
// log ends seventeen hours before the fault. **A forensic guarantee is
// a claim about a code path; trace the path before writing the
// sentence.**
//
// SECOND LIMIT, CLOSED 2026-09-02 — and it took a real crash to close,
// because the sentence that used to sit here described the bug
// correctly and then declined to fix it.
//
// sigaltstack is PER-THREAD and must be called ON the thread it
// covers. Install() runs on the UI thread, so EnlargeAltStack covered
// the UI thread ONLY; Avalonia's render thread kept the PAL's stock
// 16 KB — the size already measured insufficient for this process's
// handler chain (click fuzz: 6/8 seeds crashed at 16 KB, 0/8 at 1 MB).
//
// On 2026-09-02 that thread took a SIGSEGV inside the compositor's
// visual walk and the kernel said what happened in one line:
//
//     kernel: signal: entity-avalonia[3746566] overflowed sigaltstack
//
// LWP 3746566 is not the process pid (3746552) — it is a secondary
// thread carrying ServerCompositionContainerVisual::Update and
// libSkiaSharp frames, i.e. the render thread. The handler chain ran
// off the end of 16 KB and the process died unrecoverably: no
// minidump, no crash log, and a breadcrumb trail that just stops.
//
// That also settles the question this file's own header had left open
// as a hypothesis — "createdump is on and produces nothing for this
// fault class", explained as "there is no stack left for createdump to
// run on". The kernel line is the measurement. It was never tested
// because nobody read the journal.
//
// InstallRenderThreadAltStack() closes it: a one-shot hook on
// IRenderTimer.Tick, which is raised ON the render thread, so the
// sigaltstack call lands where it has to. The external createdump path
// (run-with-dump.sh) stays armed regardless — it covers every thread
// this hook cannot reach, and "one thread is covered now" is not
// "every thread is".
public static class CrashDiagnostics
{
    private const int RingCapacity = 96;

    private static readonly object _gate = new();
    private static readonly Queue<string> _ring = new(RingCapacity);
    private static bool _installed;
    private static string? _crashLogPath;
    private static bool _fatalWritten;

    // ---- the durable trail ------------------------------------------
    //
    // The in-memory ring reaches disk only through WriteFatal, and
    // WriteFatal only runs for a MANAGED fault. A hard SIGSEGV, a
    // runtime FailFast and a stack overflow all run no handler at all,
    // so for exactly the crash class this file was written for, the ring
    // dies with the process.
    //
    // That was not a theoretical gap. In the 2026-09-01 SIGSEGV the
    // crash log's breadcrumbs end at 21:52 and the process died at 14:33
    // the next day — SEVENTEEN HOURS of user actions, including whatever
    // one killed it, held only in memory. The class comment below
    // promised "the breadcrumb ring on disk up to the last flushed line"
    // and nothing flushed it; the guarantee was vacuous.
    //
    // So every breadcrumb is now written through to its own file as it
    // is recorded. Line-buffered onto the fd (bufferSize 1 + AutoFlush),
    // so the bytes are in the page cache before the call returns and
    // only a machine crash can lose them. Cost is one small write per
    // user action against a class of crash that otherwise leaves
    // nothing.
    private const long MaxTrailBytes = 8L << 20;

    private static StreamWriter? _trail;
    private static string? _trailPath;
    private static long _trailBytes;
    private static bool _trailStopped;

    public static string? TrailPath => _trailPath;

    // Trace mode logs first-chance exceptions too. Off by default: a
    // healthy Avalonia session throws and catches a fair number of them
    // internally, so this is a debugging instrument, not a default.
    private static bool _trace;

    public static string? CrashLogPath => _crashLogPath;

    // Install wires the process-wide handlers. Idempotent; safe to call
    // before Avalonia starts (it does not touch the dispatcher).
    public static void Install()
    {
        lock (_gate)
        {
            if (_installed) return;
            _installed = true;
        }

        _trace = !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WB_CRASH_TRACE"));
        _crashLogPath = ResolveCrashLogPath();
        // Before the first Breadcrumb call below, so the trail carries
        // the whole session including startup.
        OpenTrail();

        AppDomain.CurrentDomain.UnhandledException += (_, e) =>
            WriteFatal("AppDomain.UnhandledException" + (e.IsTerminating ? " (terminating)" : ""),
                e.ExceptionObject as Exception, e.ExceptionObject);

        System.Threading.Tasks.TaskScheduler.UnobservedTaskException += (_, e) =>
        {
            WriteFatal("TaskScheduler.UnobservedTaskException", e.Exception, null);
            // Observing it keeps a background fault from escalating into
            // a process kill on a runtime configured to do so. We have
            // already recorded it; escalating adds nothing.
            e.SetObserved();
        };

        if (_trace)
        {
            AppDomain.CurrentDomain.FirstChanceException += (_, e) =>
            {
                // First-chance is noisy AND re-entrant: writing to the
                // ring can itself throw. Keep it to one cheap line, and
                // print it — a managed exception thrown just before a
                // hard fault is the strongest lead there is, and it has
                // to survive the signal.
                Panels.PanelLog.Write("first-chance",
                    e.Exception.GetType().Name + ": " + Truncate(e.Exception.Message, 160));
            };
        }

        Breadcrumb("crash-diag", "installed" + (_trace ? " (trace on)" : "") +
            (_crashLogPath != null ? " log=" + _crashLogPath : " log=<none>"));
        Breadcrumb("crash-diag", "trail=" + (_trailPath ?? "<none>"));
        Breadcrumb("crash-diag", "external diagnostics: " + ArmingReport() +
            " pid=" + Environment.ProcessId);
        // Announced on stderr too, unconditionally. A reporter who
        // launched the binary directly rather than through
        // run-with-dump.sh needs to know the process is running blind
        // BEFORE it crashes, not after.
        try
        {
            Console.Error.WriteLine("entity-avalonia: external diagnostics: " + ArmingReport()
                + " (all three are set by run-with-dump.sh, which `make gui` and `make gui-run` both use)");
        }
        catch { }

        // Report, then optionally replace, this thread's alternate
        // signal stack. Main() runs on the UI thread, so installing here
        // protects the thread the crash lands on.
        Panels.PanelLog.Write("altstack", "at startup: " + ReportAltStack());

        // DEFAULT ON. The PAL gives the UI thread a 16 KB alternate
        // signal stack (measured), and 16 KB is not enough for the
        // handler chain this process actually runs: the click fuzz
        // crashed 5 seeds out of 5 on the stock size and 0 out of 5 at
        // 1 MB, same seeds, same binary.
        //
        // The cost is a 1 MB PRIVATE|ANONYMOUS mapping that commits
        // lazily — only the pages a signal handler actually touches are
        // ever backed — against a fatal, unrecoverable process kill.
        // That trade is not close.
        //
        // WB_ALTSTACK_BYTES overrides the size; 0 disables entirely and
        // restores the stock behaviour, which is what the A/B control
        // arm uses. Keep that escape hatch: it is the only way to
        // re-measure the bug once this is in.
        long want = AltStackWantBytes();
        string outcome;
        if (want > 0)
        {
            var installed = EnlargeAltStack(want);
            Panels.PanelLog.Write("altstack", $"enlarge -> {installed}");
            Panels.PanelLog.Write("altstack", "after: " + ReportAltStack());
            outcome = installed;
        }
        else
        {
            outcome = "DISABLED by WB_ALTSTACK_BYTES=0 (stock 16 KB — the crashing config)";
            Panels.PanelLog.Write("altstack", "enlargement " + outcome);
        }

        // Announced unconditionally, for the same reason the render mode
        // is (Program.BuildAvaloniaApp): it is the first thing a crash
        // report needs and the last thing a reporter thinks to include.
        // WB_PANEL_LOG is off on the documented fast loop (`make gui-run`),
        // so a PanelLog line alone would be invisible exactly when it
        // matters.
        try
        {
            Console.Error.WriteLine("entity-avalonia: alt signal stack = " + outcome);
        }
        catch { }

        // Then the question the line above does NOT answer: is every
        // OTHER thread covered? The 2026-09-06 crash was on a thread
        // this class never touches, so "the UI thread is enlarged" was
        // a true sentence beside a fatal gap. Measured, not assumed —
        // see ProbeAltStackCoverage.
        try
        {
            var cov = ProbeAltStackCoverage();
            Panels.PanelLog.Write("altstack", "coverage: " + cov.Detail);
            Console.Error.WriteLine("entity-avalonia: alt signal stack coverage: " + cov.Detail);
        }
        catch (Exception ex)
        {
            Panels.PanelLog.Write("altstack", "coverage probe threw: " + ex.Message);
        }
    }

    // AltStackWantBytes is the one place the size is decided, so the UI
    // thread and the render thread cannot drift apart — and so
    // WB_ALTSTACK_BYTES=0 still disables BOTH, which is what makes the
    // A/B control arm meaningful. If the two install sites read the
    // knob separately, "restore stock behaviour" would only half work
    // and the re-measurement would quietly be of a third configuration.
    private static long AltStackWantBytes()
    {
        long want = 1L << 20;
        var overrideBytes = Environment.GetEnvironmentVariable("WB_ALTSTACK_BYTES");
        if (!string.IsNullOrEmpty(overrideBytes) && long.TryParse(overrideBytes, out var n)) want = n;
        return want;
    }

    // Threads whose alternate signal stack this process has enlarged.
    // Exposed because a test asserting "the render thread is covered"
    // has nothing else to read: sigaltstack has no cross-thread query,
    // so the only evidence available is that the install ran there.
    private static int _altStackThreadsEnlarged;
    private static int _renderAltStackInstalled;

    public static int AltStackThreadsEnlarged => Volatile.Read(ref _altStackThreadsEnlarged);

    // RenderAltStackOutcome is the render thread's install result, or
    // null while it has not run yet. A surface that wants to say
    // whether this process is actually covered reads this, not a
    // hopeful sentence in a doc.
    public static string? RenderAltStackOutcome { get; private set; }

    // EnlargeAltStackHere enlarges the CALLING thread's alternate signal
    // stack, and is the render thread's entry point.
    //
    // sigaltstack is per-thread and must be called ON the thread it
    // covers, so there is no way to do this for the render thread from
    // here — something has to run there. `AltStackProbe` is that
    // something: a zero-size control whose custom draw operation is
    // executed by the compositor during the render pass, which is
    // precisely the call stack the 2026-09-02 SIGSEGV was taken in.
    //
    // Why not the render timer: IRenderTimer.Tick is internal in
    // Avalonia 11.2. Why not a hook on Compositor: its update callbacks
    // run on the UI thread. A custom draw operation is the only public
    // surface in this version that is guaranteed to execute on the
    // render thread.
    //
    // Idempotent and cheap to call repeatedly — it returns immediately
    // once a thread has been covered — because the draw path invokes it
    // per frame and a P/Invoke per frame on the render thread is a cost
    // with no second answer at the end of it.
    public static void EnlargeAltStackHere(string who)
    {
        if (Volatile.Read(ref _renderAltStackInstalled) != 0) return;
        long want = AltStackWantBytes();
        if (want <= 0)
        {
            if (Interlocked.Exchange(ref _renderAltStackInstalled, 1) != 0) return;
            RenderAltStackOutcome = "DISABLED by WB_ALTSTACK_BYTES=0 (stock 16 KB — the crashing config)";
            Panels.PanelLog.Write("altstack", who + ": " + RenderAltStackOutcome);
            return;
        }
        if (Interlocked.Exchange(ref _renderAltStackInstalled, 1) != 0) return;

        string before, installed, after;
        try
        {
            before = ReportAltStack();
            installed = EnlargeAltStack(want);
            after = ReportAltStack();
            Interlocked.Increment(ref _altStackThreadsEnlarged);
        }
        catch (Exception ex)
        {
            RenderAltStackOutcome = who + " install threw: " + ex.Message;
            Panels.PanelLog.Write("altstack", RenderAltStackOutcome);
            return;
        }

        RenderAltStackOutcome = installed;
        Panels.PanelLog.Write("altstack",
            $"{who} (managed tid {Environment.CurrentManagedThreadId}): before: {before}");
        Panels.PanelLog.Write("altstack", $"{who}: enlarge -> {installed}");
        Panels.PanelLog.Write("altstack", $"{who}: after: {after}");
        try
        {
            Console.Error.WriteLine("entity-avalonia: render-thread alt signal stack = " + installed);
        }
        catch { }
    }

    // RenderThreadAltStackInstalled reports whether the render thread has
    // actually been covered yet. It is false until the first frame is
    // drawn, which is a real state and not an error — a surface or a test
    // that treats "not yet" as "never" would be asserting on a race.
    public static bool RenderThreadAltStackInstalled =>
        Volatile.Read(ref _renderAltStackInstalled) != 0 && RenderAltStackOutcome != null;

    // How many distinct UI-thread faults we contain before we stop
    // containing. See InstallDispatcher for the reasoning.
    private const int MaxContainedUiFaults = 8;

    private static int _containedUiFaults;

    // Number of UI-thread exceptions contained this session. A test (or
    // a smoke run) asserts on this rather than on "the app is still up",
    // which is true whether or not anything went wrong.
    public static int ContainedUiFaults => Volatile.Read(ref _containedUiFaults);

    // InstallDispatcher hooks Avalonia's UI-thread exception event. It is
    // separate from Install() because the dispatcher does not exist until
    // the framework is up.
    //
    // # This handler used to let every UI fault kill the process
    //
    // The previous policy set no `e.Handled`, on the reasoning that
    // swallowing a UI fault "leaves the app running in an undefined
    // state and turns one diagnosable crash into a stream of downstream
    // mysteries". That reasoning is sound for a fault that mutates
    // shared state halfway. It is wrong as a blanket rule, and on
    // 2026-08-31 it cost a user their whole session three times: a row
    // template dereferenced a null the framework handed it (AP46), which
    // corrupts nothing, and the process took SIGABRT and wrote an 872 MB
    // core dump. The user's verdict — that a GUI dying on a cosmetic
    // fault is not a diagnostic strategy — is correct.
    //
    // So: contain, but bounded, which keeps the original concern intact
    // rather than overruling it.
    //
    //   * Every fault is still recorded in full by WriteFatal — same
    //     crash log, same breadcrumb ring, same detail as before. We
    //     lose no forensics.
    //   * The first `MaxContainedUiFaults` are contained and the session
    //     survives.
    //   * Past that, we stop setting Handled and let it take its course.
    //     A fault that keeps firing IS the "stream of downstream
    //     mysteries" the old comment feared, and by then the log has
    //     eight records of it — strictly more evidence than dying on the
    //     first one produced.
    //
    // WB_UI_FAULTS_FATAL=1 restores the old behaviour, which is the only
    // way to re-measure a fault as a hard crash.
    public static void InstallDispatcher()
    {
        try
        {
            var alwaysFatal = !string.IsNullOrEmpty(
                Environment.GetEnvironmentVariable("WB_UI_FAULTS_FATAL"));

            Dispatcher.UIThread.UnhandledException += (_, e) =>
            {
                var n = Interlocked.Increment(ref _containedUiFaults);
                WriteFatal($"Dispatcher.UnhandledException (#{n})", e.Exception, null);

                if (alwaysFatal || n > MaxContainedUiFaults)
                {
                    Breadcrumb("crash-diag",
                        alwaysFatal
                            ? "WB_UI_FAULTS_FATAL set — not containing"
                            : $"fault #{n} exceeds the containment budget of {MaxContainedUiFaults} — not containing");
                    return; // no Handled: let it terminate, as before.
                }

                e.Handled = true;
            };
        }
        catch (Exception ex)
        {
            Breadcrumb("crash-diag", "dispatcher hook failed: " + ex.Message);
        }
    }

    // AttachInput records every pointer press and key down on a TopLevel
    // BEFORE the target control's own handler sees it (Tunnel). This is
    // the "what did the user just do" channel.
    public static void AttachInput(TopLevel top)
    {
        try
        {
            top.AddHandler(InputElement.PointerPressedEvent, OnPointerPressed,
                RoutingStrategies.Tunnel, handledEventsToo: true);
            top.AddHandler(InputElement.KeyDownEvent, OnKeyDown,
                RoutingStrategies.Tunnel, handledEventsToo: true);
            Breadcrumb("input", "breadcrumbs attached to " + top.GetType().Name);
        }
        catch (Exception ex)
        {
            Breadcrumb("crash-diag", "input hook failed: " + ex.Message);
        }
    }

    // Input breadcrumbs go through PanelLog, not straight to the ring.
    //
    // The ring alone is not enough and the first harness run proved it: a
    // hard SIGSEGV never runs WriteFatal, so an in-memory ring dies with
    // the process. PanelLog flushes each line to stderr, which is the
    // only channel that survives a signal — and the run log is exactly
    // where "what did the user just do" needs to appear.
    private static void OnPointerPressed(object? sender, PointerPressedEventArgs e)
    {
        try
        {
            var pt = e.GetCurrentPoint(null).Position;
            Panels.PanelLog.Write("input", $"press @{pt.X:F0},{pt.Y:F0} on {Describe(e.Source)}");
        }
        catch
        {
            // A breadcrumb must never be the thing that breaks the app.
            Panels.PanelLog.Write("input", "press <describe failed>");
        }
    }

    private static void OnKeyDown(object? sender, KeyEventArgs e)
    {
        try
        {
            Panels.PanelLog.Write("input", $"key {e.Key} mods={e.KeyModifiers} on {Describe(e.Source)}");
        }
        catch
        {
            Panels.PanelLog.Write("input", "key <describe failed>");
        }
    }

    // Describe names a control well enough to find it in source: type,
    // x:Name if set, and the text of a Button/TextBlock (truncated).
    private static string Describe(object? source)
    {
        if (source == null) return "<null>";
        var t = source.GetType().Name;
        if (source is not Control c) return t;

        var sb = new StringBuilder(t);
        if (!string.IsNullOrEmpty(c.Name)) sb.Append('#').Append(c.Name);

        string? text = c switch
        {
            Button b => b.Content as string,
            TextBlock tb => tb.Text,
            TextBox box => box.Text,
            ContentControl cc => cc.Content as string,
            _ => null,
        };
        if (!string.IsNullOrEmpty(text)) sb.Append(" \"").Append(Truncate(text!, 48)).Append('"');
        return sb.ToString();
    }

    // Breadcrumb records into the ring AND writes through to the trail
    // file. Cheap, allocation-light, never throws. PanelLog also calls
    // this, so panel breadcrumbs are captured whether or not
    // WB_PANEL_LOG is printing them.
    //
    // The write-through is the part that matters for a hard signal: see
    // the MaxTrailBytes block above for why the ring alone is not a
    // forensic channel.
    public static void Breadcrumb(string tag, string message)
    {
        try
        {
            var line = $"[{DateTime.UtcNow:HH:mm:ss.fff}] {tag}: {message}";
            lock (_gate)
            {
                if (_ring.Count >= RingCapacity) _ring.Dequeue();
                _ring.Enqueue(line);
                WriteTrailLocked(line);
            }
        }
        catch
        {
            // Intentionally empty — see the comment above.
        }
    }

    // WriteTrailLocked appends one line to the durable trail. Caller
    // holds _gate. Never throws: a diagnostic that takes the app down is
    // worse than no diagnostic.
    //
    // The size cap exists because this file grows with session length,
    // and a GUI left open for days is the normal case here (the crash
    // that motivated this had been up ~17 hours). At the cap we stop and
    // say so, rather than rotating: a rotation would discard the START
    // of the session, and the startup breadcrumbs — render mode, alt
    // stack size, which panels mounted — are the ones a crash report
    // needs and cannot reconstruct.
    private static void WriteTrailLocked(string line)
    {
        if (_trail == null || _trailStopped) return;
        try
        {
            if (_trailBytes >= MaxTrailBytes)
            {
                _trailStopped = true;
                _trail.WriteLine($"[trail] size cap {MaxTrailBytes} bytes reached — no further breadcrumbs recorded");
                return;
            }
            _trail.WriteLine(line);
            _trailBytes += line.Length + 1;
        }
        catch
        {
            // A full or unwritable disk must not be fatal. Stop trying.
            _trailStopped = true;
        }
    }

    // OpenTrail creates the write-through file. Called once from
    // Install(), before the first breadcrumb.
    //
    // bufferSize 1 + AutoFlush is deliberate and is the whole mechanism:
    // it puts each line through write(2) as it is recorded, so the bytes
    // survive a SIGSEGV that runs no handler. A default-buffered
    // StreamWriter would hold the last 4 KB — which is to say, exactly
    // the breadcrumbs describing the crash — in userspace memory that
    // dies with the process.
    private static void OpenTrail()
    {
        try
        {
            if (_crashLogPath == null) return;
            _trailPath = Path.ChangeExtension(_crashLogPath, ".trail");
            var fs = new FileStream(_trailPath, FileMode.Append, FileAccess.Write,
                FileShare.ReadWrite, bufferSize: 1, FileOptions.WriteThrough);
            _trail = new StreamWriter(fs) { AutoFlush = true };
        }
        catch
        {
            _trail = null;
            _trailPath = null;
        }
    }

    // ArmingReport states which external diagnostics are actually on.
    //
    // This is recorded because nothing in a crash artifact said so, and
    // a blank minidump slot reads identically whether createdump was
    // disabled or whether it ran and failed. A crash report that cannot
    // distinguish "we had no instrument" from "the instrument found
    // nothing" sends the next session to the wrong question — and on
    // 2026-09-01 it sent this one there, to a written-up conclusion that
    // the crash had been taken unarmed. It had not; the environment
    // block in the coredump says DOTNET_DbgEnableMiniDump=1 and
    // createdump produced nothing regardless. One line at startup would
    // have closed that question before it was opened. See
    // run-with-dump.sh.
    private static string ArmingReport()
    {
        string On(string name) =>
            string.IsNullOrEmpty(Environment.GetEnvironmentVariable(name)) ? "OFF" : "on";
        return "minidump=" + On("DOTNET_DbgEnableMiniDump")
             + " perfmap=" + On("DOTNET_PerfMapEnabled")
             + " panel-log=" + On("WB_PANEL_LOG");
    }

    // Snapshot returns the current ring, oldest first.
    public static IReadOnlyList<string> Snapshot()
    {
        lock (_gate)
        {
            return new List<string>(_ring);
        }
    }

    private static void WriteFatal(string channel, Exception? ex, object? raw)
    {
        var sb = new StringBuilder();
        sb.AppendLine();
        sb.AppendLine("========== entity-avalonia FATAL ==========");
        sb.Append("when:    ").AppendLine(DateTime.UtcNow.ToString("yyyy-MM-dd HH:mm:ss.fff") + "Z");
        sb.Append("channel: ").AppendLine(channel);
        sb.Append("pid:     ").AppendLine(Environment.ProcessId.ToString());
        sb.Append("thread:  ").AppendLine(Environment.CurrentManagedThreadId +
            (Dispatcher.UIThread.CheckAccess() ? " (UI thread)" : " (background)"));
        sb.Append("render:  ").AppendLine(
            string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WB_GPU_RENDER"))
                ? "software Skia" : "GPU (WB_GPU_RENDER set)");

        if (ex != null)
        {
            sb.AppendLine("---- exception ----");
            sb.AppendLine(ex.ToString());
        }
        else if (raw != null)
        {
            sb.AppendLine("---- non-exception throw ----");
            sb.AppendLine(raw.ToString());
        }

        sb.AppendLine("---- breadcrumbs (oldest first) ----");
        foreach (var line in Snapshot()) sb.AppendLine(line);
        sb.AppendLine("========== end ==========");

        var text = sb.ToString();

        // stderr first: it is the channel a smoke harness or `make up`
        // is already capturing.
        try
        {
            Console.Error.Write(text);
            Console.Error.Flush();
        }
        catch { /* stderr can be gone during shutdown */ }

        // Then the durable file. Guard against a fault storm writing
        // megabytes: the first fatal is the one that matters.
        try
        {
            if (_crashLogPath != null)
            {
                bool first;
                lock (_gate) { first = !_fatalWritten; _fatalWritten = true; }
                if (first || _trace)
                {
                    File.AppendAllText(_crashLogPath, text);
                }
            }
        }
        catch { /* a diagnostic that throws is worse than no diagnostic */ }
    }

    // ---- alternate signal stack -------------------------------------
    //
    // Measured 2026-08-21, under gdb, on a reproducible crash:
    //
    //     Thread 1 received signal SIG34 (the runtime's thread-suspend
    //     injection), then immediately
    //     Thread 1 received signal SIGSEGV, si_code=2 (SEGV_ACCERR),
    //     si_addr = rsp-8, rip on a `call` instruction,
    //     and rsp inside a PROT_NONE page.
    //
    // A faulting `call` whose pushed return address lands in a guard
    // page is a stack overflow, and the mapping rsp sat in was NOT the
    // managed stack — it was the small anonymous region the PAL sets up
    // as the ALTERNATE SIGNAL STACK. So the overflow is signal-handler
    // stack exhaustion, which is why the runtime never printed
    // "Stack overflow." and why createdump never fired: by the time it
    // faults, the process has no stack left to report on.
    //
    // ReportAltStack prints what the thread actually has, so the theory
    // is checkable rather than inferred from a mapping table.
    // EnlargeAltStack replaces it with a larger one.
    [StructLayout(LayoutKind.Sequential)]
    private struct StackT
    {
        public IntPtr ss_sp;
        public int ss_flags;
        public IntPtr ss_size;
    }

    [DllImport("libc", SetLastError = true)]
    private static extern int sigaltstack(IntPtr newStack, IntPtr oldStack);

    [DllImport("libc", SetLastError = true)]
    private static extern IntPtr mmap(IntPtr addr, IntPtr length, int prot, int flags, int fd, IntPtr offset);

    private const int PROT_READ = 1, PROT_WRITE = 2;
    private const int MAP_PRIVATE = 0x02, MAP_ANONYMOUS = 0x20, MAP_STACK = 0x20000;

    public static string ReportAltStack()
    {
        try
        {
            var buf = Marshal.AllocHGlobal(Marshal.SizeOf<StackT>());
            try
            {
                if (sigaltstack(IntPtr.Zero, buf) != 0)
                    return "sigaltstack query failed errno=" + Marshal.GetLastWin32Error();
                var st = Marshal.PtrToStructure<StackT>(buf);
                return $"sp=0x{st.ss_sp.ToInt64():x} size={st.ss_size.ToInt64()} flags={st.ss_flags}";
            }
            finally { Marshal.FreeHGlobal(buf); }
        }
        catch (Exception ex)
        {
            return "sigaltstack query threw: " + ex.Message;
        }
    }

    // ---- coverage: is EVERY thread protected, or only the two we
    //      can reach from managed code? ------------------------------
    //
    // MEASURED 2026-09-06, two independent coredumps from
    // `make twopeer-gui`. The process died on a thread that was neither
    // the UI thread nor the render thread — the two this class can
    // reach — with rsp 800 bytes inside the PROT_NONE guard page below
    // a 12 KiB usable alternate signal stack. Both cores were the same
    // shape to the byte: same rip (libcoreclr+0x3af3da), same frame
    // (rbp-rsp = 0x1b30 = 6,960 bytes), same 13,088 bytes used against
    // 12,288 available, same si_code 128 / si_addr 0.
    //
    // Sixteen threads in each core still carried the PAL's stock
    // 16 KiB, and three had already taken a signal on it.
    //
    // sigaltstack is per-thread and must be called ON the thread it
    // covers, and there is no managed hook that runs on a fresh
    // thread-pool, finalizer, timer, tiered-compilation or diagnostics
    // thread. So the coverage gap cannot be closed from here at all;
    // it is closed by libaltstack.so, an LD_PRELOAD interposer on
    // sigaltstack(2) that catches every thread at the moment the PAL
    // installs its own. run-with-dump.sh loads it.
    //
    // WHY THIS PROBE EXISTS RATHER THAN A PRINTED FLAG. Whether the
    // preload is actually in effect depends on the launch path, the
    // image contents and LD_PRELOAD surviving into the process — three
    // things that fail silently and all of which have failed before in
    // this repo (AP41: an instrument nothing calls is indistinguishable
    // from an instrument you do not have). So the app MEASURES it: it
    // starts one ordinary thread and asks the kernel what alternate
    // signal stack that thread actually got. A count is bookkeeping; a
    // thread the runtime made, asked directly, is evidence (D25).
    public readonly struct AltStackCoverage
    {
        // The symbol resolved. This is NOT the same as interposing, and
        // conflating the two produced a confidently wrong diagnostic the
        // first time this was written: DllImport("libaltstack.so")
        // dlopen's the file, which sits beside the binary with
        // LD_LIBRARY_PATH=. — so the symbol resolves happily in a
        // process that never preloaded it, and the report said
        // "IS loaded but did not enlarge — investigate" about a process
        // whose only fault was a missing LD_PRELOAD.
        //
        // **A dlopen'd library does not interpose.** Symbol interposition
        // is decided at load order, and a library brought in later by
        // dlopen is behind libc in every lookup that already resolved.
        public bool InterposerSymbolResolved { get; init; }

        // The interposer is actually in the sigaltstack path — derived
        // from its own call counter, not from the symbol resolving.
        public bool InterposerActive { get; init; }

        public long ProbeThreadBytes { get; init; }
        public long WantBytes { get; init; }
        public bool AllThreadsCovered { get; init; }
        public string Detail { get; init; }
    }

    private static AltStackCoverage? _coverage;

    // Coverage is the last probe result, or null before Install() ran.
    public static AltStackCoverage? Coverage => _coverage;

    // AltStackSizeHere returns the CALLING thread's alternate signal
    // stack size in bytes, or -1 if the query failed.
    public static long AltStackSizeHere()
    {
        try
        {
            var buf = Marshal.AllocHGlobal(Marshal.SizeOf<StackT>());
            try
            {
                if (sigaltstack(IntPtr.Zero, buf) != 0) return -1;
                return Marshal.PtrToStructure<StackT>(buf).ss_size.ToInt64();
            }
            finally { Marshal.FreeHGlobal(buf); }
        }
        catch { return -1; }
    }

    [DllImport("libaltstack.so", EntryPoint = "wb_altstack_present")]
    private static extern int wb_altstack_present();

    [DllImport("libaltstack.so", EntryPoint = "wb_altstack_enlarged")]
    private static extern ulong wb_altstack_enlarged();

    // The discriminator between "preloaded and interposing" and "merely
    // dlopen'd by the P/Invoke above". If the library is in the
    // sigaltstack path at all it has counted the runtime's own calls,
    // which happen long before this probe; if it was pulled in late by
    // DllImport it has counted none.
    [DllImport("libaltstack.so", EntryPoint = "wb_altstack_calls")]
    private static extern ulong wb_altstack_calls();

    // ProbeAltStackCoverage starts one thread and reads back the
    // alternate signal stack the runtime gave it. A freshly created
    // thread is the right sample precisely because it is the population
    // the crash came from: not the UI thread, not the render thread,
    // covered by nothing this class installs.
    public static AltStackCoverage ProbeAltStackCoverage()
    {
        long want = AltStackWantBytes();

        bool resolved;
        ulong enlarged = 0, calls = 0;
        try
        {
            resolved = wb_altstack_present() == 1;
            if (resolved)
            {
                enlarged = wb_altstack_enlarged();
                calls = wb_altstack_calls();
            }
        }
        catch
        {
            // DllNotFoundException / EntryPointNotFoundException — the
            // interposer is simply not on disk. That is a real and
            // reportable state, not an error.
            resolved = false;
        }

        // Interposing, as opposed to merely present. The runtime installs
        // an alternate signal stack on every thread it creates, so by the
        // time this runs a preloaded interposer has counted many calls
        // and a dlopen'd one has counted none.
        bool active = resolved && calls > 0;

        long probed = -1;
        try
        {
            var t = new Thread(() => probed = AltStackSizeHere())
            {
                IsBackground = true,
                Name = "altstack-coverage-probe",
            };
            t.Start();
            if (!t.Join(TimeSpan.FromSeconds(10))) probed = -2;
        }
        catch (Exception ex)
        {
            _coverage = new AltStackCoverage
            {
                InterposerSymbolResolved = resolved,
                InterposerActive = active,
                ProbeThreadBytes = -1,
                WantBytes = want,
                AllThreadsCovered = false,
                Detail = "probe thread failed: " + ex.Message,
            };
            return _coverage.Value;
        }

        // want <= 0 means WB_ALTSTACK_BYTES=0 — the control arm. Stock
        // size is then the CORRECT outcome, and reporting it as covered
        // would make the A/B meaningless.
        bool covered = want > 0 && probed >= want;

        string detail = probed switch
        {
            -2 => "probe thread did not finish within 10s",
            -1 => "probe thread could not query sigaltstack",
            _ when want <= 0 =>
                $"DISABLED by WB_ALTSTACK_BYTES=0 — probe thread has {probed} bytes (stock; the crashing config)",
            _ when covered =>
                $"coverage=ALL-THREADS probe-thread={probed} bytes, "
                + $"interposer={(active ? "active" : "NOT-ACTIVE")}, calls={calls}, enlarged={enlarged}",
            _ =>
                $"coverage=UI-AND-RENDER-ONLY probe-thread={probed} bytes (want {want}); "
                + (!resolved
                    ? "libaltstack.so is not present at all — is it in the image, beside the binary?"
                    : calls == 0
                        // The case that produced a wrong diagnosis the
                        // first time round. Name the cause, not the
                        // symptom: the file is findable, so it resolves,
                        // and that says nothing about interposition.
                        ? "libaltstack.so resolved but has seen 0 sigaltstack calls — it was dlopen'd "
                          + "by this P/Invoke rather than LD_PRELOAD'd, and a dlopen'd library does "
                          + "not interpose. Set LD_PRELOAD (run-with-dump.sh does)."
                        : $"libaltstack.so IS interposing (calls={calls}, enlarged={enlarged}) but the "
                          + "probe thread still came back short — investigate the interposer itself."),
        };

        _coverage = new AltStackCoverage
        {
            InterposerSymbolResolved = resolved,
            InterposerActive = active,
            ProbeThreadBytes = probed,
            WantBytes = want,
            AllThreadsCovered = covered,
            Detail = detail,
        };
        return _coverage.Value;
    }

    // EnlargeAltStack installs a bigger alternate signal stack for the
    // CURRENT thread. Must be called on the UI thread to protect it.
    // Returns a human-readable outcome; never throws.
    public static string EnlargeAltStack(long bytes)
    {
        try
        {
            var len = new IntPtr(bytes);
            var mem = mmap(IntPtr.Zero, len, PROT_READ | PROT_WRITE,
                MAP_PRIVATE | MAP_ANONYMOUS | MAP_STACK, -1, IntPtr.Zero);
            if (mem == IntPtr.Zero || mem.ToInt64() == -1)
                return "mmap failed errno=" + Marshal.GetLastWin32Error();

            var buf = Marshal.AllocHGlobal(Marshal.SizeOf<StackT>());
            try
            {
                Marshal.StructureToPtr(new StackT { ss_sp = mem, ss_flags = 0, ss_size = len }, buf, false);
                if (sigaltstack(buf, IntPtr.Zero) != 0)
                    return "sigaltstack install failed errno=" + Marshal.GetLastWin32Error();
                // Deliberately leaked: it must outlive every signal the
                // process will ever take.
                return $"installed {bytes} bytes at 0x{mem.ToInt64():x}";
            }
            finally { Marshal.FreeHGlobal(buf); }
        }
        catch (Exception ex)
        {
            return "enlarge threw: " + ex.Message;
        }
    }

    private static string ResolveCrashLogPath()
    {
        try
        {
            var dir = Environment.GetEnvironmentVariable("WB_CRASH_DIR");
            if (string.IsNullOrEmpty(dir))
            {
                var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
                if (string.IsNullOrEmpty(home)) return null!;
                dir = Path.Combine(home, ".entity", "crash");
            }
            Directory.CreateDirectory(dir);
            return Path.Combine(dir, $"entity-avalonia-{Environment.ProcessId}.log");
        }
        catch
        {
            return null!;
        }
    }

    private static string Truncate(string s, int max) =>
        s.Length <= max ? s : s.Substring(0, max) + "…";
}
