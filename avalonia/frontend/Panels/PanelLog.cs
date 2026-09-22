using System;
using System.IO;

namespace EntityAvalonia.Panels;

// PanelLog: minimal stderr-buffered breadcrumb log. The bug class
// we've been chasing (stack-overflow-style crash with no managed
// dump) prevents .NET from writing any post-mortem trace. The only
// thing we get is the last line written to stderr BEFORE the crash.
// Pre-allocating + flushing every line means the last successful
// operation is captured.
//
// Enable by setting the WB_PANEL_LOG environment variable to any
// non-empty value. Off by default (zero overhead) so the test
// suite isn't slowed.
//
// # Repeats are SUPPRESSED ON A POWER-OF-TWO SCHEDULE, and the shape was
// # chosen by measuring the real log rather than by reasoning about it
//
// An operator sent a 4,700-line run log on 2026-09-08 asking why their
// folder had stopped syncing one way. **4,489 lines were four render
// breadcrumbs repeating** — `peer-connections: NearbyRender h=1` alone was
// 2,383 of them, one every 1.2 seconds, carrying nothing but a handle that
// never changes. The answer to their question was two `connection refused`
// lines, with the consequence written out in the message, buried under 95%
// noise. They stopped reading before reaching it, which is the correct
// response to that file.
//
// The half nobody can see is worse: `CrashDiagnostics` keeps a BOUNDED
// ring precisely so a crash can be explained by what preceded it, and a
// panel that renders on every store write evicts the entire causal history
// within seconds. Every crash investigation this project has run depended
// on that ring.
//
// **The obvious fix does not work, and it was written before it was
// measured.** Collapsing runs of CONSECUTIVE identical lines takes that
// log from 4,700 to 4,061 — a 14% saving — because the four offenders
// INTERLEAVE: `NearbyRender, tree-view Render, NearbyRender, …`. Nothing
// repeats consecutively, so consecutive-run coalescing catches almost
// nothing. Measured on the operator's actual file before shipping, which
// is the only reason it is not in this file today.
//
// What works is per-KEY suppression on a power-of-two schedule: the 1st,
// 2nd, 4th, 8th, 16th … occurrence of a given (tag, message) is emitted,
// carrying its occurrence number, and the rest are dropped. On the same
// file: **4,700 lines to 251.**
//
//	[panel 19:31:12.445] peer-connections: NearbyRender h=1
//	[panel 19:31:13.208] peer-connections: NearbyRender h=1 (x2)
//	[panel 19:31:15.361] peer-connections: NearbyRender h=1 (x4)
//	...
//	[panel 19:38:41.882] peer-connections: NearbyRender h=1 (x2048)
//
// Three properties this shape has and a time window does not. **A rare
// event is never suppressed** — anything that happens once or twice is
// logged once or twice, and "twice" is a fact a rate limiter can hide.
// **There is no clock**, so it is deterministic and gated without one.
// And **the count is carried on the line**, so a reader learns the loop is
// running at all, which a dropped line does not tell them.
//
// This does NOT excuse logging a render with no content in it. Fixing the
// flood at the sink means every future panel gets it for free; naming what
// actually changed is still the panel's job, and a breadcrumb whose whole
// payload is a handle that never varies is not a breadcrumb.
public static class PanelLog
{
    private static readonly bool _enabled;
    private static readonly TextWriter _out;

    // Occurrences per (tag, message). Guarded by _gate — Write is called
    // from the UI thread, the render thread and thread-pool workers, and
    // an interleaved counter would misreport rather than merely race.
    private static readonly object _gate = new();
    private static readonly System.Collections.Generic.Dictionary<string, long> _seen = new();

    // A bound on the key set, because a message that embeds a hash or a
    // timestamp is a NEW key every time and would otherwise leak. Past the
    // bound the table is cleared rather than evicted one-by-one: the worst
    // case of forgetting is that a noisy key restarts its ladder and logs a
    // few more lines, and paying for an LRU to avoid that would be spending
    // real complexity on the failure mode that is already the safe one.
    private const int MaxTrackedKeys = 4096;

    static PanelLog()
    {
        // WB_CRASH_TRACE implies panel logging: a first-chance trace that
        // records nowhere is not a trace.
        var v = Environment.GetEnvironmentVariable("WB_PANEL_LOG");
        var t = Environment.GetEnvironmentVariable("WB_CRASH_TRACE");
        _enabled = !string.IsNullOrEmpty(v) || !string.IsNullOrEmpty(t);
        _out = Console.Error;
    }

    public static bool Enabled => _enabled;

    public static void Write(string tag, string message)
    {
        long n;
        lock (_gate)
        {
            if (_seen.Count >= MaxTrackedKeys) _seen.Clear();
            var key = tag + "\u0000" + message;
            _seen.TryGetValue(key, out n);
            n++;
            _seen[key] = n;
        }

        // 1, 2, 4, 8, ... — n is a power of two.
        if ((n & (n - 1)) != 0) return;

        var line = n == 1 ? message : $"{message} (x{n})";

        // RECORD unconditionally, PRINT on opt-in. The ring is in-memory
        // and costs a string + an enqueue; the crash decides when we
        // needed it, and by then WB_PANEL_LOG can no longer be set.
        // (2026-08-21: two SIGSEGVs, and the only reason we had any
        // breadcrumbs at all was that the operator happened to launch
        // via `make up`, which exports WB_PANEL_LOG. `make gui-run` does
        // not.)
        CrashDiagnostics.Breadcrumb(tag, line);

        if (!_enabled) return;
        var ts = DateTime.UtcNow.ToString("HH:mm:ss.fff");
        _out.WriteLine($"[panel {ts}] {tag}: {line}");
        _out.Flush();
    }

    // ResetForTests clears the occurrence table. It is process-global, so
    // without this a test's first Write continues whatever ladder an
    // earlier test left behind.
    internal static void ResetForTests()
    {
        lock (_gate) _seen.Clear();
    }
}
