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

// S4's gates. The panel is the missing half of the declared-state layer,
// so what has to be pinned is not "it draws rows" — it is the three
// claims the panel is allowed to make and the two it is not.
//
// # What each test here is defending
//
//  1. **The envelope arrives intact.** Every field in this panel means
//     "something is wrong"; System.Text.Json drops an undeclared member in
//     total silence (AP49), and a dropped one renders as "everything is
//     fine". That is the worst available failure for a surface whose only
//     job is to say otherwise, so the fields are asserted to arrive rather
//     than assumed.
//  2. **A read is captioned as a read.** Only a reconcile pass dials,
//     writes policy or establishes a subscription. A panel that captioned
//     a records-read as a verification would make exactly the claim the
//     render/reconcile split exists to avoid.
//  3. **Inbound authority is never rendered as a health state.** What
//     they grant us is in their capability table, which this peer cannot
//     read. The panel must say so in words, not draw a dot.
//  4. **The remount button is offered exactly when the loop is stuck on
//     a missing mount** — and is reachable, not merely present.
//  5. **The panel declares a height floor** (AP64).
[Collection(nameof(BridgeCollection))]
public sealed class SharingStatusPanelTests
{
    private readonly BridgeFixture _bridge;

    public SharingStatusPanelTests(BridgeFixture bridge) => _bridge = bridge;

    private (Window, SharingStatusPanel) Open()
    {
        // The open-time reconcile pass is disabled assembly-wide in
        // BridgeFixture — it dials, and it would land asynchronously and
        // replace the rows a test had just seeded. The one test that needs
        // a pass drives it explicitly.
        var panel = new SharingStatusPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 900, Height = 620 };
        window.Show();
        HeadlessPump.Flush();
        // Show() is not a layout pass (AP46's transferable half): without
        // this the list containers are never realized and no row is built
        // at all, so a test can "pass" against a panel that renders
        // nothing.
        window.UpdateLayout();
        return (window, panel);
    }

    private static void Settle(Window window, SharingStatusPanel panel)
    {
        HeadlessPump.Flush();
        window.UpdateLayout();
        HeadlessPump.Flush();
        window.UpdateLayout();
    }

    // The render envelope must not drop a field.
    //
    // Asserted through what the panel DISPLAYS rather than through the DTO
    // class, because a declared-but-unread member passes a DTO test and
    // still renders nothing — which is AP49's actual shape: the model
    // computed it, the bridge sent it, the panel never saw it, and no test
    // read a value that had quietly become false.
    [AvaloniaFact]
    public void A_Device_Row_Shows_Its_State_And_What_We_Grant_It()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedDeviceForTests("12D3KooWRemotePeerIdentifier", "s4-desk",
                maintained: false, connected: false, paused: false,
                outboundGrant: "system/subscription:create, workbench/blob-resolve:*");
            Settle(window, panel);

            // Row-scoped: the panel also renders the shared fixture peer's
            // REAL declarations, so a whole-panel assertion can pass on a
            // row this test did not create.
            var row = RowTextsFor(panel, "s4-desk");

            // The grant summary is the outbound half, and it is exact
            // knowledge: it is our own policy row.
            Assert.Contains(row, t => t.Contains("you grant them")
                                      && t.Contains("blob-resolve"));
            // "offline" is not enough. A peer nothing is retrying and a
            // peer being retried are different situations and only one of
            // them needs an operator — collapsing them is what hid a
            // restart defect for months.
            Assert.Contains(row, t => t.Contains("nothing is retrying"));
            // And the Pause verb is there, because a panel with no verb in
            // it is a read-only surface over a read-write model (AP57).
            Assert.NotNull(FindButtonInRow(panel, "s4-desk", "Pause"));
        }
        finally { window.Close(); }
    }

    // The claim the panel is NOT allowed to make.
    //
    // If this ever fails because someone added an inbound status field,
    // the fix is to delete the field, not to update the test: whether they
    // have authorized us is in their capability table, which we cannot
    // read, and a green dot there is wrong in exactly the case that
    // matters — they revoked us and we have not tried since.
    [AvaloniaFact]
    public void Inbound_Authority_Is_Stated_As_Unknowable_Not_Drawn_As_A_State()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedDeviceForTests("12D3KooWRemotePeerIdentifier", "s4-inbound",
                maintained: true, connected: true, paused: false,
                outboundGrant: "system/subscription:create");
            Settle(window, panel);

            var row = RowTextsFor(panel, "s4-inbound");

            Assert.Contains(row, t => t.Contains("what they grant you")
                                      && t.Contains("not knowable"));
            // And it points at the evidence that DOES exist, rather than
            // leaving "unknown" as a dead end.
            Assert.Contains(row, t => t.Contains("what they grant you")
                                      && t.Contains("actually arrived"));
            // The row is fully connected and maintained — the state in
            // which a green dot is most tempting — and STILL says nothing
            // about inbound authority.
            Assert.Contains(row, t => t.Contains("relationship is being maintained"));
        }
        finally { window.Close(); }
    }

    // A read must caption itself as a read.
    //
    // Both directions in one test, because a caption that never changes at
    // all satisfies either half on its own — and the failure that matters
    // is the weaker claim wearing the stronger one's clothes, which only
    // shows up as a difference between the two.
    [AvaloniaFact]
    public async Task A_Read_Is_Captioned_As_A_Read_And_A_Pass_As_A_Pass()
    {
        var (window, panel) = Open();
        try
        {
            // AWAITED, not blocked on: the continuation resumes on the UI
            // thread, so `.GetAwaiter().GetResult()` here deadlocks the
            // headless dispatcher (measured, in SharePanelAcceptDirectoryTests).
            await panel.PerformRecheckForTests();
            Settle(window, panel);
            Assert.Contains("re-checked", panel.CaptionText);
            Assert.DoesNotContain("Re-check now\" does that", panel.CaptionText);

            panel.Refresh();
            Settle(window, panel);
            Assert.Contains("read at", panel.CaptionText);
            Assert.Contains("Nothing has been re-established", panel.CaptionText);
        }
        finally { window.Close(); }
    }

    // Both sides of the mount's lossy stage, and an explicit unknown when
    // there is no mount (AP59). A confident "0 files" for a folder that
    // has nowhere to put them is the wrong answer to the question the
    // operator is actually asking.
    [AvaloniaFact]
    public void A_Folder_With_No_Mount_Reports_Unknown_Not_Zero()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedFolderForTests("s4-unmounted", "s4-unmounted", local: true,
                origin: "local", root: "s4-unmounted", localRoot: "s4-unmounted",
                mounted: false, syncing: false, accepted: false, path: "/tmp/s4-unmounted",
                filesPresent: 0, filesIngested: 0, filesObservable: false);
            Settle(window, panel);

            // ROW-SCOPED, and that is the whole correction. The first
            // version of this asserted over every TextBlock in the panel,
            // which also renders the REAL folders in the shared fixture
            // peer's tree — one of which another test in this assembly
            // accepts into a temp directory. So the negative assertion
            // failed on somebody else's row, and, worse, a sibling test's
            // positive assertion was passing on one. A seeded row must be
            // asserted against itself.
            var row = RowTextsFor(panel, "s4-unmounted");
            Assert.Contains(row, t => t.Contains("files: unknown")
                                      && t.Contains("no mount to count"));
            Assert.DoesNotContain(row, t => t.Contains("files: 0 on disk"));
            Assert.Contains(row, t => t.Contains("NO MOUNT"));
        }
        finally { window.Close(); }
    }

    [AvaloniaFact]
    public void A_Mounted_Folder_Reports_Both_Layers()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedFolderForTests("s4-photos", "s4-photos", local: true, origin: "local",
                root: "s4-photos", localRoot: "s4-photos",
                mounted: true, syncing: false, accepted: false, path: "/tmp/s4-photos",
                filesPresent: 401, filesIngested: 1, filesObservable: true);
            Settle(window, panel);

            // 401 and 1 both, never one number. A directory of 400
            // photographs and one README displayed "401 entities" with
            // exactly one openable document for a month.
            var row = RowTextsFor(panel, "s4-photos");
            Assert.Contains(row, t => t.Contains("401") && t.Contains("1 readable"));
        }
        finally { window.Close(); }
    }

    // The remount button is the surface's half of the loop's bargain: the
    // reconciler NAMES the action and refuses to take it, because creating
    // a mount writes to a directory on somebody's disk.
    //
    // Two arms in one test on purpose — offered when stuck, absent when
    // not — because "the button exists" passes against a panel that shows
    // it unconditionally, which would invite an operator to remount a
    // folder that is working.
    [AvaloniaFact]
    public void Remount_Is_Offered_Only_When_The_Loop_Is_Stuck_On_A_Missing_Mount()
    {
        var (window, panel) = Open();
        try
        {
            panel.SeedFolderForTests("s4-stuck", "s4-stuck", local: true, origin: "local",
                root: "s4-stuck", localRoot: "s4-stuck",
                mounted: false, syncing: false, accepted: false, path: "/tmp/s4-stuck",
                filesPresent: 0, filesIngested: 0, filesObservable: false);
            Settle(window, panel);

            var remount = FindButtonInRow(panel, "s4-stuck", "Remount");
            Assert.True(remount != null,
                "a declared folder with no mount offers no way to fix it — the reconciler "
                + "names this action and deliberately will not take it, so a surface must");

            // Present is not shipped. A control below the fold of its own
            // container has an ordinary position in the panel and owns no
            // visible pixel.
            Assert.True(remount!.Bounds.Width > 0 && remount.Bounds.Height > 0,
                $"the Remount button has zero size ({remount.Bounds})");
            Assert.True(IsWithinAllClippingAncestors(remount),
                "the Remount button is laid out outside a clipping ancestor — it is in the "
                + "tree and cannot be clicked");
        }
        finally { window.Close(); }

        // The other arm: a healthy folder must not offer it.
        var (window2, panel2) = Open();
        try
        {
            panel2.SeedFolderForTests("s4-healthy", "s4-healthy", local: true, origin: "local",
                root: "s4-healthy", localRoot: "s4-healthy",
                mounted: true, syncing: true, accepted: false, path: "/tmp/s4-healthy",
                filesPresent: 3, filesIngested: 3, filesObservable: true);
            Settle(window2, panel2);
            // Scoped to THIS panel: a real unmounted folder in the shared
            // fixture peer would otherwise supply a Remount button and turn
            // this arm into a coin flip on test order.
            Assert.DoesNotContain(RowTextsFor(panel2, "s4-healthy"), t => t.Contains("NO MOUNT"));
            Assert.Null(FindButtonInRow(panel2, "s4-healthy", "Remount"));
        }
        finally { window2.Close(); }
    }

    // A received folder shows BOTH names when they differ. The
    // subscription is keyed on the sender's root and the mount is named
    // after the directory the operator picked; a surface that showed one
    // name cannot explain where the files went.
    [AvaloniaFact]
    public void A_Received_Folder_Names_Both_Roots_When_They_Differ()
    {
        var (window, panel) = Open();
        try
        {
            // The two roots DIFFER, which is the case this test is named
            // after and which an earlier version did not seed at all — it
            // passed both blank and asserted on strings a real folder in
            // the shared fixture peer also produced.
            panel.SeedFolderForTests("s4-recv", "s4-recv", local: false,
                origin: "12D3KooWRemotePeerIdentifier",
                root: "their-downloads", localRoot: "s4-from-alice",
                mounted: true, syncing: true, accepted: true, path: "/home/me/s4-from-alice",
                filesPresent: 12, filesIngested: 12, filesObservable: true);
            Settle(window, panel);

            var row = RowTextsFor(panel, "s4-recv");
            Assert.Contains(row, t => t.Contains("received from"));
            // Both names, in one sentence. A surface that showed only one
            // cannot explain where the files went — which is the operator's
            // first question about a received folder.
            Assert.Contains(row, t => t.Contains("their-downloads")
                                      && t.Contains("s4-from-alice"));
            Assert.Contains(row, t => t.Contains("mounted and subscribed"));
            Assert.Contains(row, t => t.Contains("/home/me/s4-from-alice"));
        }
        finally { window.Close(); }
    }

    // AP64. A panel that forgets the floor claims the 200px default and
    // the stack then clips every panel on screen; PanelStackScrollTests
    // collects offenders, but declaring it is cheaper than discovering it
    // there.
    [AvaloniaFact]
    public void The_Panel_Declares_A_Height_Floor()
    {
        var (window, panel) = Open();
        try
        {
            var floor = Assert.IsAssignableFrom<IPanelPreferredHeight>(panel);
            Assert.True(floor.PreferredSlotMinHeight >= 400,
                $"declared floor {floor.PreferredSlotMinHeight} is below this panel's own chrome");
        }
        finally { window.Close(); }
    }

    // D23/AP57: the panel is registered, so it is reachable from the slot
    // picker rather than being a class only a test constructs. It carries
    // a category and a blurb, because a panel that registers without them
    // lands in "This peer" with no explanation — a bug, not a default.
    [AvaloniaFact]
    public void The_Panel_Is_Registered_With_A_Category_And_A_Blurb()
    {
        var entry = PanelRegistry.Get("sharing-status");
        Assert.True(entry != null, "SharingStatusPanel is not in the registry — it would be "
            + "a complete panel with no way for an operator to open it");
        // DIAGNOSTICS as of 2026-09-04, not Network. This panel answers
        // "is what I declared actually working" — a question you reach
        // for after something breaks, not one of the two gestures. The
        // Sync panel is the front door now, and leaving four panels in
        // Network is the surface sprawl an operator called out.
        //
        // The assertion the test's name is about is the pair below: a
        // panel that registers without a category and a blurb lands in
        // "This peer" with no explanation, which is a bug and not a
        // default. Which category is a product decision and moves.
        Assert.Equal(PanelRegistry.Category.Diagnostics, entry!.Category);
        Assert.False(string.IsNullOrWhiteSpace(entry.Blurb));
        // Named for its question. This must stay distinct from the Sync
        // panel's name now that one exists — two panels whose names do
        // not say which question they answer is the naming failure an
        // operator called incomprehensible.
        Assert.DoesNotContain("Sync", entry.DisplayName);
    }

    private static Button? FindButton(Visual root, string label)
        => root.GetVisualDescendants().OfType<Button>()
            .FirstOrDefault(b => (b.Content?.ToString() ?? "") == label);

    // RowFor finds the row built for one seeded item.
    //
    // Necessary, not fastidious. The panel reads the SHARED fixture peer's
    // real declarations on construction, and another test in this assembly
    // accepts a folder into a temp directory on that same peer — so the
    // panel legitimately renders rows this test did not create. A negative
    // assertion over the whole panel fails on one of those, and a positive
    // one can PASS on one, which is the worse of the two and is what an
    // earlier version of these tests was doing.
    private static StackPanel? RowFor(Visual root, string label)
        => root.GetVisualDescendants().OfType<StackPanel>()
            .FirstOrDefault(sp => sp.Children.Count > 0
                && sp.Children[0] is TextBlock tb
                // StartsWith, not equality: a device row's heading is the
                // label followed by the shortened peer-id, and a folder
                // row's is the label alone.
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
        return row == null ? null : FindButton(row, buttonLabel);
    }

    // Copied deliberately rather than shared: this is the geometry check
    // SharePanelRenderTests arrived at after two wrong versions (a
    // panel-relative comparison that a scrolling container makes vacuous,
    // and a hit-test that reports red for fixed and broken code alike).
    // A control outside ANY clipping ancestor owns no visible pixel.
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
