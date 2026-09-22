using System;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using Avalonia.Input;
using Avalonia.VisualTree;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Reaching a DRAG.
//
// 2026-09-01: the GUI took SIGSEGV (pid 1722846, core present) on a single
// pointer interaction with the tree's vertical scrollbar. The breadcrumb
// says `input: press @361,810 on Border` and nothing after it.
//
// Per DOCTRINE-CRASH-FORENSICS §1 the first question is which instrument
// reaches the region, answered in writing. The doctrine's own table says
// `smoke-xvfb-click` **cannot reach drag gestures** — it presses and
// releases at one point, so every existing input harness in this repo
// stops exactly short of the thing the operator did. That is the same
// shape as the month-long miss the doctrine was written about: four honest
// negatives taken with instruments that could not execute the region.
//
// The headless harness CAN reach it. `MouseMove` exists alongside
// `MouseDown`/`MouseUp` and runs the genuine route — hit test, pointer
// capture, class **and** instance handlers — so a press, a sequence of
// moves under capture, and a release is a real drag through Avalonia's
// own input stack. That is cheaper than building the xvfb equivalent and
// is the right first instrument for a suspect ABOVE the platform
// (ScrollBar/Thumb/ScrollViewer logic). If these pass, the suspect moves
// below Avalonia and xvfb + gdb is the next instrument, not this one.
//
// A crash here does not fail an assertion — it takes the test host down.
// That is the intended signal: the run goes red with the process dying,
// which is exactly the fidelity we want from a reproduction attempt.
[Collection(nameof(BridgeCollection))]
public sealed class TreeViewScrollDragTests
{
    private readonly BridgeFixture _bridge;

    public TreeViewScrollDragTests(BridgeFixture bridge) => _bridge = bridge;

    // Enough rows that the vertical scrollbar is real and its thumb is
    // small enough to have somewhere to travel.
    private const int RowCount = 200;

    [AvaloniaFact]
    public async Task Dragging_The_Vertical_Scrollbar_Thumb_Does_Not_Crash()
    {
        var panel = await MountedTreeWithRows();
        var window = (Window)panel.Parent!;

        var thumb = FindVerticalThumb(panel);
        Assert.True(thumb is not null,
            "no vertical ScrollBar thumb was realized — the test cannot reach the region it " +
            "exists for, and a pass would be vacuous. Ensure the row set overflows the viewport.");

        var start = CenterInWindow(thumb!, window);

        // Press on the thumb, then move under capture. The moves are the
        // part no existing harness in this repo performs.
        window.MouseMove(start);
        window.MouseDown(start, MouseButton.Left);
        for (var dy = 10; dy <= 200; dy += 10)
        {
            window.MouseMove(new Point(start.X, start.Y + dy));
            HeadlessPump.Flush();
        }
        window.MouseUp(new Point(start.X, start.Y + 200), MouseButton.Left);
        HeadlessPump.Flush();

        // Reaching here at all is the finding. Assert the drag did
        // something so the test cannot pass by silently missing the thumb.
        var sv = panel.ListForTests.GetVisualDescendants().OfType<ScrollViewer>().FirstOrDefault();
        Assert.True(sv is not null, "no ScrollViewer under the list");
        Assert.True(sv!.Offset.Y > 0,
            $"the drag did not scroll (offset {sv.Offset.Y}) — the gesture missed the thumb, " +
            "so this run reached nothing");
    }

    // The operator was adjusting the scrollbar, which in practice means
    // several drags in both directions rather than one clean sweep. A
    // reversal re-enters the thumb's drag-delta path with a negative
    // delta, which the single-direction case above never exercises.
    [AvaloniaFact]
    public async Task Dragging_The_Thumb_Back_And_Forth_Does_Not_Crash()
    {
        var panel = await MountedTreeWithRows();
        var window = (Window)panel.Parent!;

        var thumb = FindVerticalThumb(panel);
        Assert.True(thumb is not null, "no vertical ScrollBar thumb was realized");

        var start = CenterInWindow(thumb!, window);
        window.MouseMove(start);
        window.MouseDown(start, MouseButton.Left);

        for (var sweep = 0; sweep < 6; sweep++)
        {
            var down = sweep % 2 == 0;
            for (var step = 1; step <= 12; step++)
            {
                var dy = (down ? step : 12 - step) * 15;
                window.MouseMove(new Point(start.X, start.Y + dy));
                HeadlessPump.Flush();
            }
        }

        window.MouseUp(new Point(start.X, start.Y), MouseButton.Left);
        HeadlessPump.Flush();
    }

    // Dragging past the control's bounds is what happens when someone
    // flicks a scrollbar: the pointer leaves the window while capture is
    // still held. Negative and beyond-extent coordinates are the classic
    // way a drag-delta calculation produces an out-of-range index.
    [AvaloniaFact]
    public async Task Dragging_The_Thumb_Outside_The_Window_Does_Not_Crash()
    {
        var panel = await MountedTreeWithRows();
        var window = (Window)panel.Parent!;

        var thumb = FindVerticalThumb(panel);
        Assert.True(thumb is not null, "no vertical ScrollBar thumb was realized");

        var start = CenterInWindow(thumb!, window);
        window.MouseMove(start);
        window.MouseDown(start, MouseButton.Left);

        foreach (var p in new[]
                 {
                     new Point(start.X, -500),                 // far above
                     new Point(start.X, 5000),                 // far below
                     new Point(-800, start.Y),                 // far left
                     new Point(5000, start.Y),                 // far right
                     new Point(double.MaxValue / 4, start.Y),  // absurd but finite
                 })
        {
            window.MouseMove(p);
            HeadlessPump.Flush();
        }

        window.MouseUp(start, MouseButton.Left);
        HeadlessPump.Flush();
    }

    // Releasing without ever having pressed, and pressing twice without a
    // release, are both states a real window manager can deliver when a
    // drag crosses a window boundary or the app loses focus mid-gesture.
    [AvaloniaFact]
    public async Task Unbalanced_Press_And_Release_On_The_Thumb_Does_Not_Crash()
    {
        var panel = await MountedTreeWithRows();
        var window = (Window)panel.Parent!;

        var thumb = FindVerticalThumb(panel);
        Assert.True(thumb is not null, "no vertical ScrollBar thumb was realized");
        var start = CenterInWindow(thumb!, window);

        window.MouseUp(start, MouseButton.Left);              // release with no press
        HeadlessPump.Flush();

        window.MouseDown(start, MouseButton.Left);
        window.MouseDown(start, MouseButton.Left);            // second press, no release
        HeadlessPump.Flush();

        window.MouseMove(new Point(start.X, start.Y + 60));
        HeadlessPump.Flush();

        window.MouseUp(start, MouseButton.Left);
        window.MouseUp(start, MouseButton.Left);              // double release
        HeadlessPump.Flush();
    }

    // --- harness ---------------------------------------------------

    private async Task<TreeViewPanel> MountedTreeWithRows()
    {
        for (var i = 0; i < RowCount; i++)
        {
            PutDataEntity($"scrolldrag_{i:D3}", $"row {i}");
        }

        var panel = new TreeViewPanel(_bridge.DefaultPeer);
        // Narrow and short on purpose: a small viewport against 200 rows
        // guarantees an overflowing scrollbar with a short thumb.
        var window = new Window { Content = panel, Width = 420, Height = 320 };
        window.Show();
        panel.SetSearchForTests("scrolldrag_");

        await HeadlessPump.WaitUntil(
            () => panel.RowsCountForTests > 40,
            TimeSpan.FromSeconds(10));

        // Show() is not a layout pass (AP46's transferable half): without
        // this the scrollbar template is never applied and the thumb does
        // not exist to be found.
        HeadlessPump.Flush();
        window.UpdateLayout();
        HeadlessPump.Flush();
        return panel;
    }

    private static Thumb? FindVerticalThumb(TreeViewPanel panel)
    {
        var bar = panel.ListForTests
            .GetVisualDescendants()
            .OfType<ScrollBar>()
            .FirstOrDefault(b => b.Orientation == Avalonia.Layout.Orientation.Vertical);
        return bar?.GetVisualDescendants().OfType<Thumb>().FirstOrDefault();
    }

    private static Point CenterInWindow(Visual v, Window window)
    {
        var local = new Point(v.Bounds.Width / 2, v.Bounds.Height / 2);
        return v.TranslatePoint(local, window) ?? local;
    }

    private void PutDataEntity(string path, string value)
    {
        var jsonArg = JsonSerializer.Serialize(value);
        var replyPtr = Bridge.DispatchLine(_bridge.DefaultPeer, $"put {path} data {jsonArg}");
        var reply = Marshal.PtrToStringAnsi(replyPtr) ?? "(null)";
        Bridge.FreeString(replyPtr);
        using var doc = JsonDocument.Parse(reply);
        if (!doc.RootElement.GetProperty("ok").GetBoolean())
        {
            throw new InvalidOperationException($"put failed for '{path}': {reply}");
        }
    }
}
