using System;
using System.Collections.Generic;
using System.Linq;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// The layout SIGNAL — the half of workspace persistence that lives in
// the stack.
//
// PeerView writes the file; PanelStack is what tells it something
// changed. A missed signal is not a crash and not a visible defect: it
// is a workspace that remembers some of what the operator did and not
// the rest, discovered a session later when the wrong panels come back.
// That is exactly the class of bug no one reports precisely, so it is
// pinned per mutation rather than in one "it saves" test.
[Collection(nameof(BridgeCollection))]
public sealed class PanelStackLayoutTests
{
    private readonly BridgeFixture _bridge;

    public PanelStackLayoutTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    // Order is the layout. A set restores the same panels in the wrong
    // places, which is a different workspace — so this asserts the
    // sequence, not the membership.
    [AvaloniaFact]
    public void PanelNames_Reports_The_Arrangement_In_Display_Order()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail", "peer-info", "shell");
        try
        {
            Assert.Equal(new[] { "detail", "peer-info", "shell" }, stack.PanelNames.ToArray());
        }
        finally { stack.Dispose(); }
    }

    [AvaloniaFact]
    public void Adding_A_Panel_Signals_The_Layout_Changed()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail");
        var seen = new List<IReadOnlyList<string>>();
        stack.LayoutChanged += () => seen.Add(stack.PanelNames);
        try
        {
            stack.AddSlot("peer-info");
            Assert.Single(seen);
            Assert.Equal(new[] { "detail", "peer-info" }, seen[0].ToArray());
        }
        finally { stack.Dispose(); }
    }

    [AvaloniaFact]
    public void Closing_A_Panel_Signals_The_Layout_Changed()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail", "peer-info");
        var seen = new List<IReadOnlyList<string>>();
        stack.LayoutChanged += () => seen.Add(stack.PanelNames);
        try
        {
            stack.SlotAtForTests(0).CloseForTests();
            Assert.Single(seen);
            Assert.Equal(new[] { "peer-info" }, seen[0].ToArray());
        }
        finally { stack.Dispose(); }
    }

    // The one that is easy to miss. Swapping a slot in place changes
    // nothing about the slot COUNT, so a naive "save when slots change"
    // would drop it — and the operator's workspace would then remember
    // the panels they added and forget the ones they changed their mind
    // about, which is a stranger failure to diagnose than forgetting
    // everything.
    [AvaloniaFact]
    public void Swapping_A_Panel_In_Place_Signals_The_Layout_Changed()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail", "peer-info");
        var seen = new List<IReadOnlyList<string>>();
        stack.LayoutChanged += () => seen.Add(stack.PanelNames);
        try
        {
            stack.SlotAtForTests(0).SwitchTo("shell");
            Assert.NotEmpty(seen);
            Assert.Equal(new[] { "shell", "peer-info" }, stack.PanelNames.ToArray());
        }
        finally { stack.Dispose(); }
    }

    // A stack the operator emptied reports an empty arrangement rather
    // than throwing or reporting stale names. What to DO about an empty
    // layout is the model's decision (it declines to persist one, so the
    // peer falls back next launch) — the stack's job is only to report
    // the truth.
    [AvaloniaFact]
    public void An_Emptied_Stack_Reports_An_Empty_Arrangement()
    {
        var stack = new PanelStack(_bridge.DefaultPeer, new TestHost(), "detail");
        try
        {
            stack.SlotAtForTests(0).CloseForTests();
            Assert.Empty(stack.PanelNames);
        }
        finally { stack.Dispose(); }
    }

    // Minimal IPanelHost stub — these tests need no cross-panel signal
    // forwarding. Same shape as the one in PanelStackTests; nested rather
    // than shared because every stack test file carries its own and
    // hoisting them is a separate tidy-up, not this change's business.
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
