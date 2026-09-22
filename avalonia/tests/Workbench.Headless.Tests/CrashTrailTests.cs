using System;
using System.IO;
using System.Linq;
using EntityAvalonia;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-1 gate for the durable breadcrumb trail.
//
// # What went wrong, and why a test is the right answer
//
// CrashDiagnostics kept the last 96 breadcrumbs in an in-memory ring and
// wrote them to disk from WriteFatal — the MANAGED fault handler. The
// class comment stated the guarantee as "the breadcrumb ring on disk up
// to the last flushed line". Nothing flushed it. So for the exact crash
// class the file was built for — a hard SIGSEGV, which runs no managed
// handler at all — the breadcrumbs died with the process.
//
// Measured cost: in the 2026-09-01 SIGSEGV the crash log's last
// breadcrumb is timestamped seventeen hours before the fault, because
// the only WriteFatal that ever ran was for an unrelated background
// DBus exception the evening before. Every user action in between,
// including whichever one killed it, was in memory only.
//
// # Why this test, specifically
//
// The obvious test — "provoke a fault and check the log" — tests
// WriteFatal, which was never broken. The defect is in the *absence* of
// writes on the ordinary path, so the assertion has to be that a
// breadcrumb reaches disk **with nothing going wrong at all**. That is
// what `Breadcrumbs_Reach_Disk_With_No_Fault` asserts, and it fails
// against the pre-fix class.
//
// The second test pins the mechanism rather than the outcome. A
// StreamWriter with default buffering would pass a check performed
// after process exit but lose the last 4 KB — which is to say, exactly
// the breadcrumbs that describe a crash — when the process is killed by
// a signal. So the file has to be readable *while the writer is still
// open*, which is only true if each line goes through write(2).
public class CrashTrailTests
{
    // Install() is process-wide and idempotent, so the first caller wins
    // and every test here reads the trail that caller opened. The Lazy
    // makes the ordering explicit instead of leaving it to xunit's
    // collection order, and redirects the trail into a temp directory so
    // a test run does not append to the operator's real crash log.
    //
    // WB_CRASH_DIR is read by managed code in this same process, so
    // SetEnvironmentVariable does reach it. That is NOT true of the Go
    // bridge's env vars, which are captured once at process start — see
    // AGENTS.md on WB_NO_AUTOPIN. The distinction is the reason this
    // works and BrowserPanel.AutoPinOnOpen had to be an in-process flag.
    private static readonly Lazy<string> Trail = new(() =>
    {
        var dir = Path.Combine(Path.GetTempPath(),
            "wb-crashtrail-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        Environment.SetEnvironmentVariable("WB_CRASH_DIR", dir);
        CrashDiagnostics.Install();
        return CrashDiagnostics.TrailPath ?? "";
    });

    private static string TrailPathOrSkip() => Trail.Value;

    private static string ReadTrail(string path)
    {
        // Read while the writer is still open, sharing the handle. If
        // the content only appears after the process exits, the
        // write-through is not happening and a signal would lose the
        // tail — which is the entire defect these tests cover.
        using var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                      FileShare.ReadWrite);
        using var sr = new StreamReader(fs);
        return sr.ReadToEnd();
    }

    [Fact]
    public void Breadcrumbs_Reach_Disk_With_No_Fault()
    {
        var trail = TrailPathOrSkip();
        Assert.False(string.IsNullOrEmpty(trail),
            "CrashDiagnostics.Install() did not open a trail file. Without it the " +
            "breadcrumb ring is memory-only and a hard signal loses every crumb.");

        var marker = "trail-probe-" + Guid.NewGuid().ToString("N");
        CrashDiagnostics.Breadcrumb("test", marker);

        Assert.Contains(marker, ReadTrail(trail));
    }

    [Fact]
    public void Trail_Records_Whether_External_Diagnostics_Were_Armed()
    {
        var trail = TrailPathOrSkip();
        Assert.False(string.IsNullOrEmpty(trail));

        var text = ReadTrail(trail);

        // The arming line is not cosmetic. A missing managed minidump
        // reads identically whether createdump was switched off or
        // whether it ran and found nothing, and those two send the next
        // session to different questions. Every crash artifact has to
        // say which one it was.
        Assert.Contains("external diagnostics:", text);
        Assert.Contains("minidump=", text);
        Assert.Contains("perfmap=", text);
    }

    [Fact]
    public void Ring_And_Trail_Agree_On_What_Was_Recorded()
    {
        var trail = TrailPathOrSkip();
        Assert.False(string.IsNullOrEmpty(trail));

        var marker = "ring-vs-trail-" + Guid.NewGuid().ToString("N");
        CrashDiagnostics.Breadcrumb("test", marker);

        Assert.Contains(CrashDiagnostics.Snapshot(), l => l.Contains(marker));
        Assert.Contains(marker, ReadTrail(trail));
    }
}
