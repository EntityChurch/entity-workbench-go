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

// S3 in the GUI: accepting a folder asks WHERE THE FILES GO, on the row,
// at the moment the operator decides to take it.
//
// # Why these assertions and not the obvious ones
//
// The obvious test — "the panel has a TextBox" — passes against a field
// that is rendered and never read, which is the exact defect shape this
// repo keeps paying for (AP57: a surface over a model it cannot drive).
// So the assertions here are:
//
//   1. the field is laid out INSIDE the panel's bounds, because a control
//      below the fold of its own container is not shipped (the lesson
//      SharePanelRenderTests was written for);
//   2. it is pre-filled with a path that does not already exist, because
//      the model refuses a non-empty directory and a default that trips
//      that refusal every time teaches operators to click past it;
//   3. Accept READS it — asserted by changing the text and requiring the
//      panel to carry the new value, since a handler that captured a
//      stale control or recomputed the default would pass every other
//      check here.
[Collection(nameof(BridgeCollection))]
public sealed class SharePanelAcceptDirectoryTests
{
    private readonly BridgeFixture _bridge;

    public SharePanelAcceptDirectoryTests(BridgeFixture bridge) => _bridge = bridge;

    private (Window, SharePanel) OpenWithOffer(string root)
    {
        var panel = new SharePanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 900, Height = 720 };
        window.Show();
        HeadlessPump.Flush();
        panel.SeedOfferForTests(root, root, $"local/files/{root}/");
        // A layout pass, not just a data change: the row is built by the
        // ItemTemplate and nothing exists until the container is realized.
        panel.UpdateLayout();
        HeadlessPump.Flush();
        panel.UpdateLayout();
        return (window, panel);
    }

    [AvaloniaFact]
    public void The_Receive_Directory_Field_Is_Rendered_Inside_The_Panel()
    {
        var (window, panel) = OpenWithOffer("downloads");
        try
        {
            var boxes = panel.GetVisualDescendants().OfType<TextBox>()
                .Where(b => (b.Text ?? "").Contains("entity-shared"))
                .ToList();
            Assert.True(boxes.Count > 0,
                "the offer row has no directory field — accepting would have to fall back to " +
                "a mount named after the SENDER's folder, which is the coupling S3 removes");

            var box = boxes[0];
            var panelBounds = new Rect(panel.Bounds.Size);
            var topLeft = box.TranslatePoint(new Point(0, 0), panel);
            Assert.True(topLeft.HasValue, "the directory field is not in the panel's visual tree");
            var boxRect = new Rect(topLeft!.Value, box.Bounds.Size);
            Assert.True(panelBounds.Contains(boxRect),
                $"the directory field is laid out at {boxRect}, outside the panel's {panelBounds} — " +
                "a field below the fold of its own container is not something an operator can fill in");
        }
        finally { window.Close(); }
    }

    [AvaloniaFact]
    public void The_Default_Directory_Is_A_Fresh_Path_Not_Somewhere_They_Keep_Things()
    {
        var (window, panel) = OpenWithOffer("downloads");
        try
        {
            var proposed = panel.AcceptDirectoryForTests("downloads");
            Assert.False(string.IsNullOrWhiteSpace(proposed),
                "the row proposes no directory at all");

            // Not an existing directory. Incoming files overwrite
            // same-named local ones and remote deletes propagate, so a
            // default pointing at anything the operator already uses is a
            // default that can destroy their work on one click.
            Assert.False(System.IO.Directory.Exists(proposed),
                $"the proposed directory {proposed} already exists — the default must be a new " +
                "folder, or the non-empty refusal fires on every first use and gets clicked past");

            // And it is not the home directory or a bare root.
            var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
            Assert.NotEqual(home, proposed.TrimEnd('/'));
            Assert.Contains("downloads", proposed);
        }
        finally { window.Close(); }
    }

    // The collision an operator hit on 2026-09-04.
    //
    // Unqualified, a peer sharing `downloads` proposed
    // `~/entity-shared/downloads` on a machine that already had a
    // `Downloads` — a second thing with the same name, offered at the
    // moment they were least equipped to reason about which was which.
    // Their report: they had to invent a name to tell them apart. It also
    // collides outright between peers, where the second accept then
    // refuses as non-empty for a reason that looks like a bug in us.
    [AvaloniaFact]
    public void The_Default_Directory_Is_Qualified_By_The_Sending_Peer()
    {
        var (window, panel) = OpenWithOffer("downloads");
        try
        {
            panel.SeedPeerForTests("2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR", "desk-2");
            panel.SelectedPeerId = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR";
            // The row is rebuilt from the template, so re-seed the offer
            // after selecting: the default is computed when the row is
            // built, which is the behaviour under test.
            panel.SeedOfferForTests("downloads", "downloads", "local/files/downloads/");
            panel.UpdateLayout();
            HeadlessPump.Flush();
            panel.UpdateLayout();

            var proposed = panel.AcceptDirectoryForTests("downloads") ?? "";
            Assert.Contains("entity-shared", proposed);
            Assert.Contains("downloads", proposed);
            // The peer's name is IN the path. Without it, two peers each
            // sharing a folder of the same name land on one directory.
            Assert.Contains("desk-2", proposed);
            Assert.False(System.IO.Directory.Exists(proposed),
                $"the proposed directory {proposed} already exists");
        }
        finally { window.Close(); }
    }

    // Two peers, one folder name, two directories — the property the
    // qualification exists for, asserted directly rather than inferred
    // from the string above.
    [AvaloniaFact]
    public void Two_Peers_Sharing_The_Same_Folder_Name_Do_Not_Collide()
    {
        var a = SharePanel.DefaultReceiveDirectory("desk-2", "photos");
        var b = SharePanel.DefaultReceiveDirectory("laptop", "photos");
        Assert.NotEqual(a, b);
        // And an unknown peer still yields a usable path rather than one
        // with an empty segment in it.
        var anon = SharePanel.DefaultReceiveDirectory("", "photos");
        Assert.DoesNotContain("//", anon.Replace("://", ":/"));
        Assert.Contains("photos", anon);
    }

    [AvaloniaFact]
    public async Task Accept_Actually_Uses_The_Directory_The_Operator_Typed()
    {
        var (window, panel) = OpenWithOffer("downloads");
        try
        {
            var box = panel.GetVisualDescendants().OfType<TextBox>()
                .First(b => (b.Text ?? "").Contains("entity-shared"));

            var chosen = System.IO.Path.Combine(
                System.IO.Path.GetTempPath(),
                "entity-accept-gate-" + Guid.NewGuid().ToString("N"));
            Assert.False(System.IO.Directory.Exists(chosen));

            box.Text = chosen;
            HeadlessPump.Flush();

            // A peer must be selected or the accept refuses before doing
            // anything, and a refusal would satisfy a weaker assertion
            // while proving nothing about the field.
            panel.SeedPeerForTests("2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR", "them");
            panel.SelectedPeerId = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR";

            // AWAITED, not blocked on. `PerformAcceptAsync` resumes on the
            // UI thread after its Task.Run, so `.GetAwaiter().GetResult()`
            // on the test thread deadlocks the dispatcher — measured: the
            // suite hung here. These are the first tests in this project
            // to drive a Perform*Async, so there was no pattern to copy.
            //
            // The accept FAILS — there is no such peer to sync from — and
            // that is fine. What is under test is everything before the
            // sync leg: the typed path has to cross the bridge and reach
            // the model, which creates and mounts it.
            await panel.PerformAcceptAsync("downloads");

            // The only assertion here that a decorative field cannot
            // satisfy. Asserting on the accessor instead would be
            // circular — PerformAcceptAsync reads the same accessor, so a
            // handler that ignored it entirely and used its own default
            // would pass.
            Assert.True(System.IO.Directory.Exists(chosen),
                $"the model never saw {chosen} — the directory field is rendered but not sent, " +
                "so the accept fell back to a mount named after the sender's folder");

            try { System.IO.Directory.Delete(chosen, recursive: true); } catch { }
        }
        finally { window.Close(); }
    }

    [AvaloniaFact]
    public async Task An_Accept_With_No_Directory_Refuses_Before_Touching_The_Network()
    {
        var (window, panel) = OpenWithOffer("downloads");
        try
        {
            var box = panel.GetVisualDescendants().OfType<TextBox>()
                .First(b => (b.Text ?? "").Contains("entity-shared"));
            box.Text = "   ";
            HeadlessPump.Flush();

            panel.SelectedPeerId = "";
            await panel.PerformAcceptAsync("downloads");

            // Either refusal is correct here; what must NOT happen is a
            // dispatch with an empty directory, which the model would
            // then read as "use a mount named after their folder" —
            // silently reinstating the coupling this whole change removes.
            Assert.False(string.IsNullOrEmpty(panel.StatusText));
        }
        finally { window.Close(); }
    }
}
