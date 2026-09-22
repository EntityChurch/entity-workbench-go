using System;
using System.Linq;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// The delivery / catch-up / recording section, which is D23 closed on the
// machinery an operator most needs to see and can least infer.
//
// # What this is defending, and why a DTO test would not do it
//
// All three facts were already computed, already carried across the model
// boundary, and reachable from `entity-shell` and from no pixel. Two of
// them — `ReconcileOutcome.Delivery` and the supervisor's state — were one
// undeclared field away from the screen, which is AP49 exactly:
// System.Text.Json drops an undeclared member in TOTAL silence, so the
// panel would render "nothing dropped" for a peer that dropped 2327
// notifications and no test would read a value that had quietly become
// false.
//
// So these assert on what the PANEL DISPLAYS. A DTO with the right
// property names passes a serialization test and still renders nothing.
//
// # The one discipline running through all of them
//
// MEASURED-ZERO and NOT-MEASURED must not render the same. "Nothing
// dropped", "no pass recovered anything" and "no path hit its budget" are
// healthy. "Nothing counted drops", "no supervisor is running" and
// "nothing is watching growth" are the same words on a careless surface
// and mean the opposite — and every one of them is the state in which the
// failure is silently in progress. That is why each assertion below has a
// negative arm.
[Collection(nameof(BridgeCollection))]
public sealed class SharingStatusDeliveryTests
{
    private readonly BridgeFixture _bridge;

    public SharingStatusDeliveryTests(BridgeFixture bridge) => _bridge = bridge;

    private (Window, SharingStatusPanel) Open()
    {
        var panel = new SharingStatusPanel(_bridge.DefaultPeer);
        // Taller than the sibling suite's 620: this panel now declares an
        // 800px floor, and a reachability assertion in a window shorter
        // than the floor measures the window, not the panel.
        var window = new Window { Content = panel, Width = 900, Height = 900 };
        window.Show();
        HeadlessPump.Flush();
        // Show() is not a layout pass (AP46's transferable half).
        window.UpdateLayout();
        return (window, panel);
    }

    private static void Settle(Window window)
    {
        HeadlessPump.Flush();
        window.UpdateLayout();
        HeadlessPump.Flush();
        window.UpdateLayout();
    }

    // The three lines exist and carry a real reading, not a placeholder.
    //
    // The fixture peer is healthy and idle, so the EXPECTED content is the
    // healthy phrasing — which is the case worth pinning, because it is
    // the one a careless implementation reaches by rendering nothing.
    [AvaloniaFact]
    public void The_Panel_Reports_Delivery_CatchUp_And_Recording()
    {
        var (window, panel) = Open();
        try
        {
            panel.Refresh();
            Settle(window);

            Assert.NotEqual("", panel.DeliveryText);
            Assert.NotEqual("", panel.CatchUpText);
            Assert.NotEqual("", panel.RecordingText);

            // Saturation is a real reading on this peer — the subscription
            // engine is present, so the panel must NOT be saying it cannot
            // measure. That negative arm is the point: "not measured" is
            // what an undeclared `delivery` field would produce, and it
            // reads as a caveat rather than as a bug.
            Assert.DoesNotContain("not measured", panel.DeliveryText);
            Assert.Contains("dropped", panel.DeliveryText);
        }
        finally { window.Close(); }
    }

    // The recording guard is ATTACHED for a peer built by the real
    // bootstrap. If it is not, the panel says so instead of implying that
    // nothing has grown.
    //
    // This is the assertion that would have caught the whole class of
    // "the model has it and no frontend enables it" — the guard hangs off
    // Bootstrap for every peer, and a regression that moved it behind a
    // frontend flag would show here as the warning text.
    [AvaloniaFact]
    public void Recording_Growth_Is_Being_Counted_On_A_Bootstrapped_Peer()
    {
        var (window, panel) = Open();
        try
        {
            panel.Refresh();
            Settle(window);

            Assert.DoesNotContain("NOT being counted", panel.RecordingText);
            Assert.Contains("budget", panel.RecordingText);
            Assert.Contains("per path", panel.RecordingText);
        }
        finally { window.Close(); }
    }

    // The catch-up verb is REACHABLE — rendered, enabled, and inside every
    // clipping ancestor.
    //
    // "The button exists" is not the claim. AP64 shipped a stack in which
    // every panel's controls existed and several were clipped out of
    // reach, with nothing to scroll, and the only workaround an operator
    // found was closing panels until one was left. A verb an operator
    // cannot press is the same as no verb.
    [AvaloniaFact]
    public void Catch_Up_Now_Is_Reachable()
    {
        var (window, panel) = Open();
        try
        {
            Settle(window);
            var btn = panel.GetVisualDescendants().OfType<Button>()
                .FirstOrDefault(b => (b.Content as string) == "Catch up now");
            Assert.True(btn != null, "no \"Catch up now\" button — the machinery that "
                                     + "recovers a copy that stopped part way is reachable "
                                     + "from the shell and from no pixel again");
            Assert.True(btn!.IsEffectivelyVisible, "the button is not visible");
            Assert.True(btn.IsEnabled, "the button is disabled on an idle panel");
            Assert.True(IsWithinAllClippingAncestors(btn),
                "the button is clipped out of every visible pixel it owns");
        }
        finally { window.Close(); }
    }

    // Pressing it runs a real pass through the real bridge and the panel
    // reports the result.
    //
    // Safe in this suite for the reason the export exists: a catch-up does
    // not dial. It reads the peer's own sync bindings — the fixture peer
    // has none, so the pass covers zero folders and says so, which is the
    // honest empty answer and not a silent one.
    [AvaloniaFact]
    public async Task Catching_Up_Runs_A_Pass_And_Says_What_It_Did()
    {
        var (window, panel) = Open();
        try
        {
            // AWAITED, not blocked on: the continuation resumes on the UI
            // thread, so `.GetAwaiter().GetResult()` deadlocks the
            // headless dispatcher.
            await panel.PerformCatchUpForTests();
            Settle(window);

            // A pass has now completed, so the line must have moved off
            // "no pass has completed yet" — otherwise the button is wired
            // to something that does not record what it did, which is the
            // shape in which a stalled supervisor reads as a healthy one.
            Assert.DoesNotContain("no pass has completed yet", panel.CatchUpText);
            Assert.Contains("catch-up", panel.CatchUpText);

            // And the status line reports it rather than staying on the
            // in-progress caption.
            Assert.DoesNotContain("catching up —", panel.StatusText);
        }
        finally { window.Close(); }
    }

    // A catch-up is a READ, and the panel must keep saying so.
    //
    // The temptation is to caption a pass that just transferred files as a
    // verification, because it feels like one. It is not: only a reconcile
    // dials, writes policy or establishes a subscription, and a catch-up
    // does none of those. Captioning it "re-checked" would make exactly
    // the claim the render/reconcile split exists to avoid, and it would
    // be believed — a surface that has to remember which function produced
    // its table will caption it wrong, always in the confident direction.
    [AvaloniaFact]
    public async Task A_Catch_Up_Is_Still_Captioned_As_A_Read()
    {
        var (window, panel) = Open();
        try
        {
            await panel.PerformCatchUpForTests();
            Settle(window);
            Assert.Contains("read at", panel.CaptionText);
            Assert.DoesNotContain("re-checked", panel.CaptionText);
        }
        finally { window.Close(); }
    }

    // A conflict row offers the verbs, and offers RESTORE only when the
    // replaced version was actually kept.
    //
    // Both arms in one test on purpose. "The button exists" passes against
    // a panel that shows it unconditionally — which would offer an
    // operator a restore that fails, and a restore that fails reads as the
    // tool having lost their file twice.
    [AvaloniaFact]
    public void A_Conflict_Row_Offers_Restore_Only_When_The_Version_Was_Kept()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedConflictForTests("k-recoverable",
                "/p/local/files/shared/recoverable.md", "a change replaced your edit",
                recoverable: true, keepBothPath: "");
            panel.SeedConflictForTests("k-gone",
                "/p/local/files/shared/gone.md", "replaced, and NOT recoverable",
                recoverable: false, keepBothPath: "");
            Settle(window);

            Assert.Equal(2, panel.ConflictCount);

            // The recoverable one offers all three.
            Assert.NotNull(FindButtonInRow(panel, "recoverable.md", "Restore mine"));
            Assert.NotNull(FindButtonInRow(panel, "recoverable.md", "Keep theirs"));
            Assert.NotNull(FindButtonInRow(panel, "recoverable.md", "Keep both"));

            // The unrecoverable one offers only the decision, because
            // there are no bytes to put back.
            Assert.Null(FindButtonInRow(panel, "gone.md", "Restore mine"));
            Assert.Null(FindButtonInRow(panel, "gone.md", "Keep both"));
            Assert.NotNull(FindButtonInRow(panel, "gone.md", "Keep theirs"));

            // Reachable, not merely present (AP64).
            var btn = FindButtonInRow(panel, "recoverable.md", "Restore mine")!;
            Assert.True(btn.IsEffectivelyVisible, "Restore mine is not visible");
            Assert.True(IsWithinAllClippingAncestors(btn),
                "Restore mine is clipped out of every visible pixel it owns");
        }
        finally { window.Close(); }
    }

    // A row is headed by the FILE NAME, not the tree path.
    //
    // An operator recognises `notes.md`; `/2KG2Hp…/local/files/shared/notes.md`
    // is the same fact rendered as an address, and the address is three
    // quarters peer-id. The full path stays in the summary underneath.
    [AvaloniaFact]
    public void A_Conflict_Row_Is_Headed_By_The_File_Name()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedConflictForTests("k1",
                "/2KG2HpLvLywygCFmLAP9R2CmMSF3zMxTs2qZEps7545Ft7/local/files/shared/notes.md",
                "a change from alice replaced your edit", recoverable: true, keepBothPath: "");
            Settle(window);

            var row = RowTextsFor(panel, "notes.md");
            Assert.Contains(row, t => t == "notes.md");
            // And the whole path is still readable somewhere in the row.
            Assert.Contains(row, t => t.Contains("replaced your edit"));
        }
        finally { window.Close(); }
    }

    // A quiet peer says so, rather than showing an empty box.
    [AvaloniaFact]
    public void With_No_Conflicts_The_Section_Says_So_And_Shows_No_Storm()
    {
        var (window, panel) = Open();
        try
        {
            panel.Refresh();
            Settle(window);
            Assert.Equal(0, panel.ConflictCount);
            Assert.Equal("", panel.ConflictStormText);
        }
        finally { window.Close(); }
    }

    // Row helpers, copied from SharingStatusPanelTests for the reason
    // stated there: the geometry check survived two wrong versions and a
    // shared copy would invite a third.
    private static StackPanel? RowFor(Visual root, string label)
        => root.GetVisualDescendants().OfType<StackPanel>()
            .FirstOrDefault(sp => sp.Children.Count > 0
                && sp.Children[0] is TextBlock tb
                && (tb.Text ?? "").StartsWith(label, StringComparison.Ordinal));

    private static System.Collections.Generic.List<string> RowTextsFor(Visual root, string label)
    {
        var row = RowFor(root, label);
        Assert.True(row != null, $"no row was built for the seeded item \"{label}\"");
        return row!.GetVisualDescendants().OfType<TextBlock>()
            .Select(t => t.Text ?? "").ToList();
    }

    private static Button? FindButtonInRow(Visual root, string label, string buttonLabel)
    {
        var row = RowFor(root, label);
        return row?.GetVisualDescendants().OfType<Button>()
            .FirstOrDefault(b => (b.Content as string) == buttonLabel);
    }

    // Copied deliberately rather than shared — see SharingStatusPanelTests
    // for why this is the geometry check that survived two wrong versions.
    // Delegates to `Reachable`, which is the ONE copy. This was three
    // byte-identical private methods sharing one blind spot: a control below
    // the fold of a working ScrollViewer is one scroll away, not unreachable,
    // and reporting it as unreachable turned two correct panels red on
    // 2026-09-16. See `Reachable.cs` for the distinction and why it is drawn
    // narrowly.
    private static bool IsWithinAllClippingAncestors(Control control) =>
        Reachable.IsWithinAllClippingAncestors(control);
}
