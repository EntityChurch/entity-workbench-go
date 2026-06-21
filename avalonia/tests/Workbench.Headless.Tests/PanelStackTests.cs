using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 headless coverage for PanelStack (PHASE-I-DESKTOP-RENDERER
// PLAN §I.5). The "ship the layout framework" stage moved the right
// column from a fixed 3-row grid to a dynamic slot list; these tests
// pin the dynamic invariants:
//
//   - Initial panels mount in order; SlotCount reflects them.
//   - AddSlot appends to the bottom; SlotCount grows.
//   - Close (via the slot's close button) removes that slot;
//     adjacent slots survive; SlotCount shrinks.
//   - A rapid add/close cycle does not crash or hang. This is the
//     "no leaks over 10 minutes" §I.5 exit criterion's smoke shape —
//     we can't run 10 minutes in a unit test, but we can do 50
//     cycles in milliseconds and assert nothing dies.
//
// PanelStack uses real PanelRegistry entries, which require a real
// peer handle behind them — share the BridgeCollection fixture
// (libbridge boot, default peer) with the other Tier-3 tests.
[Collection(nameof(BridgeCollection))]
public sealed class PanelStackTests
{
    private readonly BridgeFixture _bridge;

    public PanelStackTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public void Initial_Panels_Mount_In_Order()
    {
        var host = new TestHost();
        var stack = new PanelStack(
            _bridge.DefaultPeer,
            host,
            "detail", "peer-info");
        try
        {
            Assert.Equal(2, stack.SlotCountForTests);
            Assert.Equal("detail", stack.SlotAtForTests(0).CurrentPanelName);
            Assert.Equal("peer-info", stack.SlotAtForTests(1).CurrentPanelName);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void AddSlot_Appends_At_Bottom()
    {
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host, "detail");
        try
        {
            Assert.Equal(1, stack.SlotCountForTests);

            stack.AddSlot("peer-info");
            Assert.Equal(2, stack.SlotCountForTests);
            Assert.Equal("peer-info", stack.SlotAtForTests(1).CurrentPanelName);

            stack.AddSlot("log-viewer");
            Assert.Equal(3, stack.SlotCountForTests);
            Assert.Equal("log-viewer", stack.SlotAtForTests(2).CurrentPanelName);

            // First slot unchanged after appends.
            Assert.Equal("detail", stack.SlotAtForTests(0).CurrentPanelName);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Close_Removes_Slot_And_Neighbors_Survive()
    {
        var host = new TestHost();
        var stack = new PanelStack(
            _bridge.DefaultPeer,
            host,
            "detail", "peer-info", "log-viewer");
        try
        {
            Assert.Equal(3, stack.SlotCountForTests);

            // Close the middle slot — peer-info. Expect detail (idx 0)
            // and log-viewer (was idx 2, now idx 1) to survive.
            var middle = stack.SlotAtForTests(1);
            middle.CloseForTests();

            Assert.Equal(2, stack.SlotCountForTests);
            Assert.Equal("detail", stack.SlotAtForTests(0).CurrentPanelName);
            Assert.Equal("log-viewer", stack.SlotAtForTests(1).CurrentPanelName);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Close_Last_Slot_Yields_Empty_Stack()
    {
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host, "detail");
        try
        {
            Assert.Equal(1, stack.SlotCountForTests);
            stack.SlotAtForTests(0).CloseForTests();
            Assert.Equal(0, stack.SlotCountForTests);

            // Empty stack is still functional — AddSlot brings it back.
            stack.AddSlot("peer-info");
            Assert.Equal(1, stack.SlotCountForTests);
            Assert.Equal("peer-info", stack.SlotAtForTests(0).CurrentPanelName);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Slot_Rows_Have_MinHeight_And_MaxHeight_Pinned()
    {
        // Structural mitigation against the layout-recursion
        // SIGSEGV. The splitter respects RowDefinition.
        // MinHeight/MaxHeight, so the zero-collapse → layout-recursion
        // chain is unreachable as long as these pins exist. Pinning
        // the invariants here ensures a future "just use star" revert
        // would fail the test instead of silently re-opening the
        // crash class.
        var host = new TestHost();
        var stack = new PanelStack(
            _bridge.DefaultPeer,
            host,
            "detail", "peer-info", "log-viewer");
        try
        {
            for (int i = 0; i < 3; i++)
            {
                var row = stack.SlotRowForTests(i);
                Assert.Equal(PanelStack.SlotMinHeight, row.MinHeight);
                Assert.Equal(PanelStack.SlotMaxHeight, row.MaxHeight);
            }
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Grid_Is_Hosted_In_ScrollViewer()
    {
        // The "many panels → scroll" half of the panel-sizing fix.
        // Pinning the ScrollViewer's presence prevents a regression
        // back to "panels squeeze into a fixed viewport."
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host, "detail");
        try
        {
            var sv = stack.ScrollViewerForTests;
            Assert.NotNull(sv);
            // Visible (not Auto) + AllowAutoHide=false + Grid Margin
            // = ScrollGutter together reserve a fixed gutter on the
            // right so the outer scrollbar doesn't overlay inner panel
            // scrollbars. Pinning all three here prevents a regression.
            Assert.Equal(
                Avalonia.Controls.Primitives.ScrollBarVisibility.Visible,
                sv.VerticalScrollBarVisibility);
            Assert.Equal(
                Avalonia.Controls.Primitives.ScrollBarVisibility.Disabled,
                sv.HorizontalScrollBarVisibility);
            Assert.False(sv.AllowAutoHide);
            // The Grid that the ScrollViewer wraps must carry a right
            // margin equal to ScrollGutter — that's what physically
            // moves inner content (and inner scrollbars) clear of the
            // outer scrollbar's overlay zone.
            var inner = Assert.IsType<Grid>(sv.Content);
            Assert.Equal(PanelStack.ScrollGutter, inner.Margin.Right);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Eight_Panels_Mount_Without_Crashing()
    {
        // Cross-check: with star rows + MinHeight + ScrollViewer +
        // viewport-bound Grid.Height, adding 8 panels (total
        // 8*200 + 7*4 = 1628 px content_min) must not throw or
        // crash regardless of viewport. Previously, with raw
        // star-weighted rows in an unbounded splitter context, deep
        // splitter drags risked the layout-engine recursion.
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host);
        try
        {
            for (int i = 0; i < 8; i++)
            {
                stack.AddSlot("detail");
            }
            Assert.Equal(8, stack.SlotCountForTests);
            for (int i = 0; i < 8; i++)
            {
                var row = stack.SlotRowForTests(i);
                Assert.Equal(PanelStack.SlotMinHeight, row.MinHeight);
            }
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void AddSlot_Preserves_Existing_RowDefinition_Identities()
    {
        // The user-visible contract for "adding a panel doesn't
        // reset sizes": the RowDefinition object instances for
        // existing slots must survive an AddSlot call. If AddSlot
        // rebuilt the grid, the RowDefinitions would be replaced with
        // fresh instances and any GridSplitter-applied size would
        // reset to default. By pinning instance identity, we pin
        // the "append-not-rebuild" behavior.
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host, "detail", "peer-info");
        try
        {
            var row0Before = stack.SlotRowForTests(0);
            var row1Before = stack.SlotRowForTests(1);

            stack.AddSlot("log-viewer");

            // Same RowDefinition instances for slots 0 and 1 — proves
            // they weren't recreated. Slot 2 is the new one.
            Assert.Same(row0Before, stack.SlotRowForTests(0));
            Assert.Same(row1Before, stack.SlotRowForTests(1));
            Assert.Equal(3, stack.SlotCountForTests);
        }
        finally
        {
            stack.Dispose();
        }
    }

    [AvaloniaFact]
    public void Rapid_Add_Close_Cycle_Does_Not_Crash()
    {
        // §I.5 exit criterion's "no leaks over 10 minutes" — the
        // honest unit-test shape: 50 add+close cycles. A leak in the
        // PanelSlot dispose path would either crash here (handle
        // double-free) or hang (orphaned wake goroutines blocking the
        // bridge). Both manifest as the assertion never running.
        var host = new TestHost();
        var stack = new PanelStack(_bridge.DefaultPeer, host, "detail");
        try
        {
            for (int i = 0; i < 50; i++)
            {
                stack.AddSlot("log-viewer");
                // Close the slot we just added (always at the bottom).
                stack.SlotAtForTests(stack.SlotCountForTests - 1)
                    .CloseForTests();
            }

            // After 50 add+close cycles, only the original "detail"
            // remains.
            Assert.Equal(1, stack.SlotCountForTests);
            Assert.Equal("detail", stack.SlotAtForTests(0).CurrentPanelName);
        }
        finally
        {
            stack.Dispose();
        }
    }

    // Minimal IPanelHost stub — most stack tests don't need any cross-
    // panel signal forwarding. Matches the shape used in
    // MarkdownViewPanelStressTests' TestHost.
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
