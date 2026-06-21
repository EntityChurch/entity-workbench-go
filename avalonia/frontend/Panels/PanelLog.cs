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
public static class PanelLog
{
    private static readonly bool _enabled;
    private static readonly TextWriter _out;

    static PanelLog()
    {
        var v = Environment.GetEnvironmentVariable("WB_PANEL_LOG");
        _enabled = !string.IsNullOrEmpty(v);
        _out = Console.Error;
    }

    public static bool Enabled => _enabled;

    public static void Write(string tag, string message)
    {
        if (!_enabled) return;
        var ts = DateTime.UtcNow.ToString("HH:mm:ss.fff");
        _out.WriteLine($"[panel {ts}] {tag}: {message}");
        _out.Flush();
    }
}
