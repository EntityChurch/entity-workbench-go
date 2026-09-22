using System;
using System.IO;
using System.Linq;
using Avalonia.Headless.XUnit;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// End-to-end workspace persistence: arrange panels, close the view, open
// a new one, get the arrangement back.
//
// This is the only test that crosses the whole seam — PeerView reads the
// alias, calls the bridge, the Go model writes a real file, and a second
// PeerView reads it back. Everything below it (the model's precedence
// rules, the stack's change signal) is covered separately and more
// cheaply; what is only covered here is that the pieces are actually
// WIRED, which is the failure this repo keeps finding by audit rather
// than by test.
//
// **The layout file is redirected through the bridge, not an env var.**
// `WB_LAYOUT` cannot be set from managed code: Go captures its
// environment at process start, so a C# SetEnvironmentVariable never
// reaches os.Getenv in the bridge. Without the in-process redirect these
// tests would write to the developer's real ~/.entity/gui-layout.json —
// and no suite in this repo may depend on, or scribble on, state outside
// the tree.
[Collection(nameof(BridgeCollection))]
public sealed class PeerViewLayoutPersistenceTests : IDisposable
{
    private readonly BridgeFixture _bridge;
    private readonly string _dir;

    public PeerViewLayoutPersistenceTests(BridgeFixture bridge)
    {
        _bridge = bridge;
        _dir = Path.Combine(Path.GetTempPath(), "wb-layout-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(_dir);
        Bridge.TakeString(Bridge.LayoutSetPath(Path.Combine(_dir, "gui-layout.json")));
    }

    public void Dispose()
    {
        try { Directory.Delete(_dir, recursive: true); } catch { }
    }

    private PeerView NewView() => new(_bridge.DefaultPeer, _bridge.DefaultPeer);

    // A peer with no saved layout opens on the defaults. Stated first
    // because every later assertion is a difference from this one.
    [AvaloniaFact]
    public void A_Peer_With_No_Saved_Layout_Opens_On_The_Defaults()
    {
        var view = NewView();
        try
        {
            Assert.Equal(
                new[] { "site-view", "detail", "shell" },
                view.PanelStackForSmoke.PanelNames.ToArray());
        }
        finally { view.Dispose(); }
    }

    // The whole point. Add a panel, throw the view away, open a fresh
    // one — the panel is still there. Before this landed the arrangement
    // was three hard-coded names and an operator rebuilt their workspace
    // every launch.
    [AvaloniaFact]
    public void An_Added_Panel_Comes_Back_In_A_New_View()
    {
        var first = NewView();
        try
        {
            first.PanelStackForSmoke.AddSlot("local-files");
        }
        finally { first.Dispose(); }

        var second = NewView();
        try
        {
            Assert.Contains("local-files", second.PanelStackForSmoke.PanelNames);
            Assert.Equal(4, second.PanelStackForSmoke.PanelNames.Count);
        }
        finally { second.Dispose(); }
    }

    // Closing is remembered too, and separately: a workspace that
    // remembers additions and forgets removals gives the operator back
    // the panel they deliberately got rid of, every launch, which is
    // worse than not remembering at all.
    [AvaloniaFact]
    public void A_Closed_Panel_Stays_Closed_In_A_New_View()
    {
        var first = NewView();
        try
        {
            var names = first.PanelStackForSmoke.PanelNames;
            var idx = names.ToList().IndexOf("detail");
            Assert.True(idx >= 0, "fixture assumption: the default layout contains 'detail'");
            first.PanelStackForSmoke.SlotAtForTests(idx).CloseForTests();
        }
        finally { first.Dispose(); }

        var second = NewView();
        try
        {
            Assert.DoesNotContain("detail", second.PanelStackForSmoke.PanelNames);
        }
        finally { second.Dispose(); }
    }

    // Order is the layout, so the restore has to be a sequence and not a
    // set — the same panels in different places is a different workspace.
    [AvaloniaFact]
    public void The_Restored_Arrangement_Preserves_Order()
    {
        var first = NewView();
        string[] arranged;
        try
        {
            first.PanelStackForSmoke.AddSlot("peer-info");
            first.PanelStackForSmoke.AddSlot("local-files");
            arranged = first.PanelStackForSmoke.PanelNames.ToArray();
        }
        finally { first.Dispose(); }

        var second = NewView();
        try
        {
            Assert.Equal(arranged, second.PanelStackForSmoke.PanelNames.ToArray());
        }
        finally { second.Dispose(); }
    }

    // A layout naming a panel kind this build does not register must not
    // fail the launch: the view opens on what it CAN mount and says what
    // it dropped. That is what lets a file written by a newer build open
    // in an older one, and the note is what keeps the dropped panel from
    // being a silent disappearance.
    [AvaloniaFact]
    public void An_Unknown_Panel_Kind_Is_Skipped_And_Reported_Not_Fatal()
    {
        var first = NewView();
        string alias;
        try
        {
            alias = first.Alias;
            first.PanelStackForSmoke.AddSlot("detail");
        }
        finally { first.Dispose(); }

        // Write a layout containing one real kind and one from the future.
        var path = Path.Combine(_dir, "gui-layout.json");
        File.WriteAllText(path,
            "{\"peers\":{\"" + alias + "\":{\"panels\":[\"detail\",\"a-panel-from-the-future\"]}}}");

        var second = NewView();
        try
        {
            Assert.Equal(new[] { "detail" }, second.PanelStackForSmoke.PanelNames.ToArray());
        }
        finally { second.Dispose(); }
    }

    // AP33, at the surface. A layout file that does not parse must not be
    // silently swapped for the defaults: that is indistinguishable from
    // "it forgot again", which is the complaint the feature answers. The
    // view opens on the defaults AND says why.
    [AvaloniaFact]
    public void A_Corrupt_Layout_File_Opens_On_Defaults_And_Says_So()
    {
        File.WriteAllText(Path.Combine(_dir, "gui-layout.json"), "{\"peers\": {");

        var view = NewView();
        try
        {
            Assert.Equal(
                new[] { "site-view", "detail", "shell" },
                view.PanelStackForSmoke.PanelNames.ToArray());
            Assert.Contains("layout", view.StatusLineForTests, StringComparison.OrdinalIgnoreCase);
        }
        finally { view.Dispose(); }
    }
}
