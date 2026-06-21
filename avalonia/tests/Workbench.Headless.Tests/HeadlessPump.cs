using System;
using System.Threading.Tasks;
using Avalonia.Headless;
using Avalonia.Threading;

namespace EntityAvalonia.Tests;

// Shared pumping helpers. The headless dispatcher needs explicit
// kicks to drain wakes that arrive from non-UI threads (cgo callbacks
// from Go goroutines, in our case) AND to advance the render-timer
// scheduler (per Avalonia issue #15447, ForceRenderTimerTick is the
// documented escape).
public static class HeadlessPump
{
    // Pump the dispatcher until `predicate` returns true or `timeout`
    // elapses. Returns true if the predicate fired before the deadline.
    public static async Task<bool> WaitUntil(Func<bool> predicate, TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        while (!predicate() && DateTime.UtcNow < deadline)
        {
            AvaloniaHeadlessPlatform.ForceRenderTimerTick();
            Dispatcher.UIThread.RunJobs();
            await Task.Delay(10);
        }
        return predicate();
    }

    // Drain currently-queued jobs + tick the render timer once. Use
    // after a synchronous mutation (e.g. SetSearch) when you want
    // layout to settle before asserting, but don't need to wait for
    // a particular condition.
    public static void Flush()
    {
        AvaloniaHeadlessPlatform.ForceRenderTimerTick();
        Dispatcher.UIThread.RunJobs();
    }
}
