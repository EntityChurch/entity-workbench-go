using System;
using System.Collections.Generic;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// The stack must actually SCROLL when the panels in it do not fit.
//
// This is a regression suite for a shipped defect an operator hit before
// any test did: with three panels open, panel content was clipped and the
// scrollbar could not move, so the only way to use the app was to close
// panels until one was left.
//
// The cause was not in the scrolling machinery, which was correct.
// PanelStack sizes its Grid to max(viewport, sum-of-slot-minimums), and a
// slot's minimum is whatever its panel declares through
// IPanelPreferredHeight. ProgramPanel was the ONLY implementer, so every
// other panel claimed the 200px default: three of them summed to 608px
// against a ~900px viewport, the Grid was pinned to the viewport, and
// each panel got ~297px — less than its own fixed chrome. Content was
// clipped with nothing to scroll, while the stack's scrollbar rendered
// permanently (Visible + AllowAutoHide=false) and inert.
//
// So the assertions here are about the two things that actually broke,
// and neither is "a ScrollViewer exists":
//
//   1. Every registered panel declares a floor (the discipline gate). A
//      new panel that forgets is the defect coming back, and it comes
//      back invisibly — one panel alone always looks fine.
//   2. With a realistic window and three panels, the scroll viewer's
//      extent EXCEEDS its viewport, which is what "the scrollbar can
//      move" means mechanically.
//
// Note the AP15 shape in (1): it collects every offender and reports them
// together rather than failing on the first, because the count a
// fail-fast loop yields is a lower bound.
[Collection(nameof(BridgeCollection))]
public sealed class PanelStackScrollTests
{
    private readonly BridgeFixture _bridge;

    public PanelStackScrollTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    // The discipline gate. A panel that does not declare a floor sizes
    // itself by the 200px stack default, which is smaller than the chrome
    // of every panel in this app — so this is not a style rule, it is the
    // condition the clipping bug needs in order to exist.
    [AvaloniaFact]
    public void Every_Registered_Panel_Declares_A_Height_Floor()
    {
        var missing = new List<string>();
        foreach (var entry in PanelRegistry.All())
        {
            var slot = new PanelSlot(_bridge.DefaultPeer, new TestHost(), entry.Name);
            try
            {
                // PreferredSlotMinHeight is 0 when the mounted panel does
                // not implement IPanelPreferredHeight.
                if (slot.PreferredSlotMinHeight <= 0) missing.Add(entry.Name);
            }
            finally { slot.Dispose(); }
        }

        Assert.True(missing.Count == 0,
            "these panels declare no PreferredSlotMinHeight and will be clipped as soon as a "
            + "second panel is open: " + string.Join(", ", missing));
    }

    // The mechanical assertion: three ordinary panels in a normal window
    // produce a scrollable stack. Extent > Viewport is what lets the
    // scrollbar thumb move; before the floors landed they were equal.
    [AvaloniaFact]
    public void Three_Panels_In_A_Normal_Window_Make_The_Stack_Scrollable()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(),
            "peer-connections", "local-files", "shell");
        var window = new Window { Content = stack, Width = 1100, Height = 900 };
        try
        {
            window.Show();
            HeadlessPump.Flush();
            window.UpdateLayout();
            HeadlessPump.Flush();

            var scroll = stack.ScrollViewerForTests;
            Assert.True(scroll.Extent.Height > scroll.Viewport.Height,
                $"the stack is not scrollable: extent {scroll.Extent.Height} <= "
                + $"viewport {scroll.Viewport.Height}. Three panels whose declared floors sum "
                + "to more than the window must overflow, or their content is clipped with "
                + "nothing to scroll — the shipped bug this suite exists for.");
        }
        finally
        {
            window.Close();
            stack.Dispose();
        }
    }

    // The control arm. Without it the test above passes for the wrong
    // reason — a stack that ALWAYS overflows would satisfy it, and that is
    // its own bug (a single panel would scroll a viewport it fits in).
    [AvaloniaFact]
    public void One_Panel_In_A_Normal_Window_Does_Not_Scroll()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail");
        var window = new Window { Content = stack, Width = 1100, Height = 900 };
        try
        {
            window.Show();
            HeadlessPump.Flush();
            window.UpdateLayout();
            HeadlessPump.Flush();

            var scroll = stack.ScrollViewerForTests;
            Assert.True(scroll.Extent.Height <= scroll.Viewport.Height + 1,
                $"a single panel should fill the viewport, not overflow it: extent "
                + $"{scroll.Extent.Height} vs viewport {scroll.Viewport.Height}");
        }
        finally
        {
            window.Close();
            stack.Dispose();
        }
    }

    // The panel an operator was staring at when they reported this. Its
    // fixed chrome — header, listen line, two bounded lists, three input
    // rows, a status line and a caption — is ~540px, so a 200px floor put
    // the Connect button outside the slot. Pinned by name because it is
    // the worst offender and the one whose regression is most costly:
    // connecting is the first step of every cross-peer flow.
    [AvaloniaFact]
    public void PeerConnections_Declares_Enough_Room_For_Its_Own_Chrome()
    {
        var slot = new PanelSlot(_bridge.DefaultPeer, new TestHost(), "peer-connections");
        try
        {
            Assert.True(slot.PreferredSlotMinHeight >= 520,
                $"peer-connections declares {slot.PreferredSlotMinHeight}px, which is less than "
                + "its own fixed chrome — the connect controls will be clipped");
        }
        finally { slot.Dispose(); }
    }

    // Per-file rather than shared: every stack test file carries its own
    // and hoisting them is a separate tidy-up, not this change's business.
    private sealed class TestHost : IPanelHost
    {
        public event Action<string>? SelectedPath;
        public string? CurrentSelectedPath { get; private set; }
        public void PublishSelectedPath(string path)
        {
            CurrentSelectedPath = path;
            SelectedPath?.Invoke(path);
        }
        public void RequestPeerStatusRefresh() { }
    }
}
