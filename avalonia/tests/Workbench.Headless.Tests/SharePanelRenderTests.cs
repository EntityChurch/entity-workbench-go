using System;
using System.Linq;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Input;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Can the operator actually CLICK the thing the app told them to click?
//
// This suite exists because of a defect that every other test was
// structurally unable to see. `SharePanel` put its lists in containers
// capped at 130px. A share row is a title, a path, a per-peer line, a
// button row and an address line — about 145px once ListBoxItem padding
// is counted — so "Complete connection" rendered below the fold of its
// own container. The accepting peer's panel said *press Complete
// connection on the other machine*; on the other machine there was no
// such button to be seen.
//
// Every SharePanel test passed throughout, because they all asserted on
// DTOs and view-model counts. A control's existence in a collection is
// not a claim that a person can reach it, and this is the same lesson as
// AP61 (a test that drives data to the row and never selects one) and of
// AP46's transferable half (constructing the world is not running it) —
// arriving here at the level of geometry rather than of state.
//
// The rule the assertions encode: for a control to count as shipped, it
// must be laid out INSIDE the bounds of the panel that owns it. Not
// present in a tree, not bound to a command — on screen.
[Collection(nameof(BridgeCollection))]
public sealed class SharePanelRenderTests
{
    private readonly BridgeFixture _bridge;

    public SharePanelRenderTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public void The_Complete_Connection_Button_Is_Laid_Out_Inside_The_Panel()
    {
        var panel = new SharePanel(_bridge.DefaultPeer);
        // A realistic slot, not a generous one: the panel declares a
        // 720px floor and the stack gives it that, so this is the size it
        // actually gets with other panels open.
        var window = new Window { Content = panel, Width = 900, Height = 720 };
        try
        {
            window.Show();
            HeadlessPump.Flush();

            panel.SeedShareForTests(
                root: "downloads", title: "Downloads", prefix: "local/files/downloads/",
                peerId: "12D3KooWRemotePeerIdentifier", alias: "laptop",
                address: "192.168.1.31:9000");

            // Show() is not a layout pass. Without this the containers
            // are never realized and the row is not built at all — the
            // trap AP46's transferable half names, and the reason a test
            // can "pass" against a panel that renders nothing.
            window.UpdateLayout();
            HeadlessPump.Flush();

            var button = FindButton(panel, "Complete connection");
            Assert.True(button != null,
                "no \"Complete connection\" button was built for a share that has an "
                + "audience — the accepting peer is told to press it by name");

            Assert.True(button!.Bounds.Width > 0 && button.Bounds.Height > 0,
                $"the button has zero size ({button.Bounds}) — it is in the tree and "
                + "occupies no pixels, which is indistinguishable from absent");

            // Measured against EVERY CLIPPING ANCESTOR, not against the
            // panel.
            //
            // Two earlier versions of this assertion were wrong in
            // opposite directions, and both are worth recording because
            // the second one nearly shipped as a pass.
            //
            // The first compared the button's y-offset within the panel
            // against the panel's height. That is vacuous: a container
            // with a MaxHeight scrolls its content, so a control below
            // ITS fold still reports an ordinary position inside the
            // panel. It passed against the bug it was written for.
            //
            // The second used window.InputHitTest, on the reasoning that
            // "can a person click it" is the real question. It is — but
            // in this headless environment the pointer lands on a
            // ScrollContentPresenter for EVERY button in this panel,
            // including ones with no list above them, whether or not
            // anything is clipped. It reported red for the fixed code and
            // for the broken code alike, which is not a test, it is a
            // coin that always lands the same way.
            //
            // What is left is the honest check: for each ancestor that
            // clips, is the control inside that ancestor's box? That is
            // decidable from geometry, it is deterministic, and it is
            // verified below to go red when a real cap is reintroduced.
            Assert.True(IsWithinAllClippingAncestors(button), Diagnose(window, panel, button));
        }
        finally
        {
            window.Close();
            panel.Dispose();
        }
    }

    // The control arm. If the seeded share rendered no row at all, the
    // test above would fail on the null check for a reason that has
    // nothing to do with clipping — this pins that the row itself is
    // built and carries the peer, so a failure upstairs is unambiguous.
    [AvaloniaFact]
    public void A_Seeded_Share_Renders_Its_Peer_And_Its_Address()
    {
        var panel = new SharePanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 900, Height = 720 };
        try
        {
            window.Show();
            HeadlessPump.Flush();
            panel.SeedShareForTests("downloads", "Downloads", "local/files/downloads/",
                "12D3KooWRemotePeerIdentifier", "laptop", "192.168.1.31:9000");
            window.UpdateLayout();
            HeadlessPump.Flush();

            Assert.Equal(1, panel.ShareCount);

            var texts = panel.GetVisualDescendants().OfType<TextBlock>()
                .Select(t => t.Text ?? "").ToList();
            Assert.Contains(texts, t => t.Contains("laptop"));
            // The address is shown, not just held: the operator needs to
            // see which endpoint the button will dial before pressing it.
            Assert.Contains(texts, t => t.Contains("192.168.1.31:9000"));
        }
        finally
        {
            window.Close();
            panel.Dispose();
        }
    }

    // "Stop sharing" sits below "Complete connection" in the same row, so
    // it is the deeper of the two and fails first if a cap comes back.
    [AvaloniaFact]
    public void Every_Button_In_A_Share_Row_Is_Reachable()
    {
        var panel = new SharePanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 900, Height = 720 };
        try
        {
            window.Show();
            HeadlessPump.Flush();
            panel.SeedShareForTests("downloads", "Downloads", "local/files/downloads/",
                "12D3KooWRemotePeerIdentifier", "laptop", "192.168.1.31:9000");
            window.UpdateLayout();
            HeadlessPump.Flush();

            var clipped = panel.GetVisualDescendants().OfType<Button>()
                .Where(b => b.Bounds.Height > 0 && b.IsEffectivelyVisible)
                .Where(b => !IsWithinAllClippingAncestors(b))
                .Select(b => b.Content?.ToString() ?? "(unnamed)")
                .ToList();

            // Collected, not fail-fast (AP15): the first clipped control
            // is a lower bound on how many there are, and the count is
            // the thing that tells you whether a cap came back or one
            // row simply grew.
            Assert.True(clipped.Count == 0,
                "buttons laid out past the bottom of the panel: " + string.Join(", ", clipped));
        }
        finally
        {
            window.Close();
            panel.Dispose();
        }
    }

    // Diagnose says WHY a control is unreachable, because "not clickable"
    // has several causes with the same symptom and guessing between them
    // is how an afternoon goes. It names the position, the panel, what
    // the pointer actually hit, and every clipping ancestor between.
    private static string Diagnose(Window window, Control panel, Control control)
    {
        var inPanel = control.TranslatePoint(new Point(0, 0), panel);
        var centre = control.TranslatePoint(
            new Point(control.Bounds.Width / 2, control.Bounds.Height / 2), window);
        var hit = centre.HasValue ? window.InputHitTest(centre.Value) as Visual : null;

        // Every clipping ancestor WITH its own offset in panel space and
        // the control's offset inside it — the two numbers that say
        // whether the control falls outside that particular box. A list
        // of sizes alone cannot, which cost a round trip.
        var clippers = control.GetVisualAncestors()
            .OfType<Control>()
            .Where(a => a.ClipToBounds)
            .Select(a =>
            {
                var aInPanel = a.TranslatePoint(new Point(0, 0), panel);
                var ctrlInA = control.TranslatePoint(new Point(0, 0), a);
                var inside = ctrlInA.HasValue
                    && ctrlInA.Value.Y >= -0.5
                    && ctrlInA.Value.Y + control.Bounds.Height <= a.Bounds.Height + 0.5;
                return $"{a.GetType().Name} @panelY="
                    + $"{(aInPanel.HasValue ? aInPanel.Value.Y.ToString("F0") : "?")} "
                    + $"size={a.Bounds.Width:F0}x{a.Bounds.Height:F0} "
                    + $"ctrlY={(ctrlInA.HasValue ? ctrlInA.Value.Y.ToString("F0") : "?")} "
                    + (inside ? "OK" : "*** OUTSIDE ***");
            })
            .ToList();

        return $"\"{(control as ContentControl)?.Content}\" is in the visual tree and NOT clickable.\n"
            + $"  button bounds : {control.Bounds}\n"
            + $"  y within panel: {(inPanel.HasValue ? inPanel.Value.Y.ToString("F0") : "n/a")}\n"
            + $"  panel size    : {panel.Bounds.Width:F0}x{panel.Bounds.Height:F0}\n"
            + $"  hit-test point: {(centre.HasValue ? centre.Value.ToString() : "n/a")}\n"
            + $"  pointer landed: {hit?.GetType().Name ?? "(null)"}\n"
            + "  clipping ancestors:\n    " + (clippers.Count == 0 ? "(none)" : string.Join("\n    ", clippers));
    }

    private static Button? FindButton(Visual root, string label)
        => root.GetVisualDescendants().OfType<Button>()
            .FirstOrDefault(b => (b.Content?.ToString() ?? "") == label);

    // IsWithinAllClippingAncestors reports whether the control lies
    // inside the box of every ancestor that clips.
    //
    // A control outside any one of them is invisible to the operator no
    // matter how sane its coordinates look from further out — which is
    // the whole failure mode: a height-capped container scrolls its
    // content, so the control keeps a perfectly ordinary position in the
    // panel and owns no visible pixel. Checking each clipper in turn is
    // what distinguishes that from a control that simply sits low.
    private static bool IsWithinAllClippingAncestors(Control control)
    {
        foreach (var a in control.GetVisualAncestors().OfType<Control>())
        {
            if (!a.ClipToBounds) continue;
            var inA = control.TranslatePoint(new Point(0, 0), a);
            if (!inA.HasValue) return false;
            if (inA.Value.Y < -0.5) return false;
            if (inA.Value.Y + control.Bounds.Height > a.Bounds.Height + 0.5) return false;
        }
        return true;
    }
}
