using System;
using System.Linq;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// SyncPanelTests — the front door for the sharing job.
//
// Tier: headless-UI (TESTING-STRATEGY §3).
//
// # What these are actually for
//
// The panel exists because five surfaces touched one job. So the
// assertions that matter are about what it does NOT have as much as what
// it does: no Refresh button (it subscribes to the tree), and a verb in
// every folder row (a panel with no verb in it is AP57's tell).
//
// # The fixture peer is SHARED (AP70)
//
// Every headless panel test in this assembly runs against
// BridgeFixture.DefaultPeer — one peer for the whole assembly — and other
// tests write real declarations to it. So an assertion phrased over "the
// panel" is really phrased over everything every other test has ever done
// to that peer. Only the NEGATIVE assertions can catch contamination,
// because contamination only ever ADDS matching rows; the positive ones
// here are therefore deliberately about structure rather than about a
// particular row existing.
[Collection(nameof(BridgeCollection))]
public class SyncPanelTests
{
    private readonly BridgeFixture _fx;

    public SyncPanelTests(BridgeFixture fx) => _fx = fx;

    // Settle mounts the panel in a real window and runs a LAYOUT PASS.
    //
    // Show() is not a layout pass (AP46's transferable half): a test that
    // populates and clears without UpdateLayout never realizes a
    // container, so container teardown — where the row-template null
    // crash lives — is unreachable from the suite while it reports green.
    private static (Window w, SyncPanel p) Settle(long handle)
    {
        var panel = new SyncPanel(handle, new TestHost());
        var window = new Window { Width = 900, Height = 700, Content = panel };
        window.Show();
        window.UpdateLayout();
        return (window, panel);
    }

    [AvaloniaFact]
    public void The_Panel_Mounts_And_Reads_The_Peer_Without_Reaching_The_Network()
    {
        // AutoLoadOnOpen off: opening fetches OFFERS, which is a
        // dispatched remote read per declared peer. No suite in this repo
        // may reach anything outside the tree, and the async completion
        // would also replace the row collections underneath the
        // assertions — a race that presents as a flaky assertion rather
        // than as the timing bug it is.
        SyncPanel.AutoLoadOnOpen = false;
        var (w, p) = Settle(_fx.DefaultPeer);
        try
        {
            Assert.NotNull(p);
            // The offers section starts hidden. An empty "Offered to you"
            // block on every launch is chrome that teaches an operator to
            // skip the one region where a time-sensitive card appears.
            Assert.False(p.OffersSectionVisible);
        }
        finally { w.Close(); }
    }

    // THE point of the panel, asserted as an absence.
    //
    // The three sharing panels were the only ones in the app with no tree
    // subscription, and that is exactly why each grew a Refresh button —
    // 12 of 15 panels updated themselves and these did not. If a Refresh
    // button ever appears here, the subscription has been broken or
    // someone has papered over it; either way the fix is the
    // subscription, not the button.
    [AvaloniaFact]
    public void There_Is_No_Refresh_Button_Because_The_Panel_Subscribes_To_The_Tree()
    {
        SyncPanel.AutoLoadOnOpen = false;
        var (w, p) = Settle(_fx.DefaultPeer);
        try
        {
            var labels = p.GetVisualDescendants()
                .OfType<Button>()
                .Select(b => (b.Content as string ?? "").Trim())
                .ToList();

            Assert.DoesNotContain(labels, s =>
                s.Equals("Refresh", StringComparison.OrdinalIgnoreCase) ||
                s.Equals("Reload", StringComparison.OrdinalIgnoreCase) ||
                s.Equals("Re-check now", StringComparison.OrdinalIgnoreCase));

            // Control arm: the sweep can actually see this panel's
            // buttons. Without it the assertion above passes against a
            // panel with no buttons at all, or against a broken descendant
            // walk — which is the more likely way to get a false green.
            Assert.Contains(labels, s => s.StartsWith("Share a folder", StringComparison.Ordinal));
        }
        finally { w.Close(); }
    }

    // The share gesture is reachable and asks for both of its inputs.
    //
    // A panel whose primary button reveals nothing is the "model with no
    // shipped surface" failure one level in: the button exists, the
    // gesture does not.
    [AvaloniaFact]
    public void The_Share_Gesture_Asks_For_A_Folder_And_A_Peer()
    {
        SyncPanel.AutoLoadOnOpen = false;
        var (w, p) = Settle(_fx.DefaultPeer);
        try
        {
            var shareBtn = p.GetVisualDescendants().OfType<Button>()
                .First(b => (b.Content as string ?? "").StartsWith("Share a folder", StringComparison.Ordinal));

            // IsEffectivelyVisible, NOT IsVisible. IsVisible is the
            // control's OWN property and stays true for a child of a
            // hidden parent — the first version of this test asserted on
            // it and failed, correctly, against a form that was not on
            // screen. Only the effective flag answers "can the operator
            // see this", which is the question the test is named after.
            Assert.Empty(p.GetVisualDescendants().OfType<TextBox>()
                .Where(t => t.IsEffectivelyVisible));

            RaiseClick(shareBtn);
            w.UpdateLayout();

            // After: a directory field and a peer chooser, which are the
            // two things the gesture needs and the only two.
            Assert.NotEmpty(p.GetVisualDescendants().OfType<TextBox>()
                .Where(t => t.IsEffectivelyVisible));
            Assert.NotEmpty(p.GetVisualDescendants().OfType<ComboBox>()
                .Where(c => c.IsEffectivelyVisible));
        }
        finally { w.Close(); }
    }

    // Minimal IPanelHost stub — this panel forwards no cross-panel
    // signal, so the host is only here to satisfy the constructor.
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

    private static void RaiseClick(Button b)
    {
        // The panel wires Click, so the driver raises Click. This is NOT
        // the same as a real pointer dispatch and does not pretend to be:
        // where hit-testing or handler ORDER is the thing under test, the
        // suite uses Avalonia.Headless MouseDown/MouseUp at hit-tested
        // coordinates (see MarkdownLinkClickTests). Here the question is
        // whether the gesture reveals its inputs, which is the handler's
        // own behaviour.
        b.RaiseEvent(new Avalonia.Interactivity.RoutedEventArgs(Button.ClickEvent));
    }
}
