using System.Text.Json;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// SyncPanelProblemsTests — the gate on "the diagnosis reaches the panel
// that owns the flow".
//
// Tier: headless-UI (TESTING-STRATEGY §3).
//
// # The defect this exists for
//
// On 2026-09-08 an operator's outbound connection never came up. The
// reconciler diagnosed it correctly at startup, in plain language, naming
// the address and the consequence — and put it on stderr, i.e. in
// `avalonia/run-logs/`, where an operator does not look. They sat in THIS
// panel for 45 minutes.
//
// There were TWO independent reasons the sentence could not arrive, and
// only fixing both helps:
//
//   1. `StatusView` here did not declare `problems`, so System.Text.Json
//      dropped it in silence at the boundary (AP49 — the same failure as
//      the browser's provenance fields).
//   2. Nothing rendered it even if it had arrived.
//
// The first is what these tests are mostly about, because an undeclared
// field fails nothing: no exception, no warning, no test, just a value
// that is quietly always the default.
[Collection(nameof(BridgeCollection))]
public class SyncPanelProblemsTests
{
    private readonly BridgeFixture _fx;

    public SyncPanelProblemsTests(BridgeFixture fx) => _fx = fx;

    private static (Window w, SyncPanel p) Settle(long handle)
    {
        var panel = new SyncPanel(handle, new TestHost());
        var window = new Window { Width = 900, Height = 700, Content = panel };
        window.Show();
        window.UpdateLayout();
        return (window, panel);
    }

    // The AP49 gate. It deserializes the panel's OWN DTO rather than
    // asserting on a rendered row, because that is exactly where the
    // value was being lost — a renderer test would have gone on passing
    // with the field undeclared, since an empty list renders as nothing
    // and "nothing wrong" is the normal case.
    [AvaloniaFact]
    public void The_Status_Envelope_Does_Not_Drop_The_Problems_Field()
    {
        const string json = """
        {
          "ok": true,
          "localPeerId": "2KLUxPXmcQx1",
          "localAlias": "me",
          "devices": [],
          "folders": [],
          "problems": [
            "desk-2: could not open our own connection to this peer — nothing we write to a shared folder will reach them — could not dial 192.168.68.160:9000 (connection refused)"
          ]
        }
        """;

        var view = JsonSerializer.Deserialize<SyncPanel.StatusView>(json);

        Assert.NotNull(view);
        Assert.Single(view!.Problems);
        Assert.Contains("192.168.68.160:9000", view.Problems[0]);
    }

    // THE FOLDER ROW MUST NOT CLAIM TWO-WAY IT DOES NOT HAVE.
    //
    // An operator's report, 2026-09-10, verbatim: *"showing two-way,
    // showing no problem, meanwhile the whole files aren't getting
    // delivered."* Every clause was reproducible, and the row was the
    // surface saying it. Three separate causes, all of them here:
    //
    //   - the arrow came from `mode` alone, which is OUR DECLARATION and
    //     says nothing about whether any subscription exists;
    //   - the renderer decided for itself when a folder was in trouble,
    //     and its rule could not fire for a folder we OWN at all;
    //   - `syncingWith` and the model's own problem sentences were not
    //     declared on this DTO, so they were dropped in silence (AP49).
    //
    // Deserializing the DTO rather than reading a rendered row, for this
    // file's stated reason: an undeclared field renders as *nothing is
    // wrong*, which is the worst available failure for a surface whose
    // only job is to say otherwise.
    [AvaloniaFact]
    public void A_Two_Way_Folder_With_No_Reverse_Leg_Renders_As_Unestablished_And_Says_Why()
    {
        const string json = """
        {
          "id": "2KLUxPXmcQx1.shared",
          "label": "shared",
          "local": true,
          "root": "shared",
          "localRoot": "shared",
          "path": "/home/me/shared",
          "origin": "local",
          "mode": "both",
          "mounted": true,
          "syncing": false,
          "accepted": false,
          "note": "",
          "filesPresent": 14,
          "filesIngested": 14,
          "filesObservable": true,
          "receiveFrom": ["2KOtherPeer"],
          "syncingWith": [],
          "folderProblems": [
            "folder \"shared\" is set to \"both\" but nothing is arriving from 2KOtherPeer — no subscription to their copy exists."
          ],
          "peers": []
        }
        """;

        var dto = JsonSerializer.Deserialize<SyncPanel.FolderDto>(json);
        Assert.NotNull(dto);

        // The two lists must both survive the boundary, or the row has
        // only the declaration to draw from — which is the defect.
        Assert.Single(dto!.ReceiveFrom);
        Assert.Empty(dto.SyncingWith);
        Assert.Single(dto.Problems);

        var vm = SyncPanel.FolderVm.From(dto);

        // NOT a bare "↔". The folder is declared two-way and is pulling
        // from nobody, and a row that draws those identically is the
        // thing the operator was looking at.
        Assert.NotEqual("↔", vm.Arrow);

        // And it must SAY so. A dimmed arrow alone is a puzzle.
        Assert.NotEqual("", vm.Problem);
        Assert.Contains("2KOtherPeer", vm.Problem);

        // The file count is REAL and is not the answer. 14 of 14 readable
        // is true of the owner's own mount and says nothing about
        // delivery — this is the "14 files under management" that read as
        // reassurance while nothing was crossing.
        Assert.Contains("14", vm.Detail);
    }

    // The positive control. With the leg established the same row must go
    // quiet — otherwise the assertion above is satisfied by a panel that
    // flags every folder, and a permanent warning is one an operator
    // learns to skip within a day.
    [AvaloniaFact]
    public void An_Established_Two_Way_Folder_Renders_Clean()
    {
        const string json = """
        {
          "id": "2KLUxPXmcQx1.shared", "label": "shared", "local": true,
          "root": "shared", "localRoot": "shared", "path": "/home/me/shared",
          "origin": "local", "mode": "both", "mounted": true, "syncing": false,
          "accepted": false, "note": "", "filesPresent": 14, "filesIngested": 14,
          "filesObservable": true,
          "receiveFrom": ["2KOtherPeer"],
          "syncingWith": ["2KOtherPeer"],
          "folderProblems": [],
          "peers": []
        }
        """;

        var vm = SyncPanel.FolderVm.From(JsonSerializer.Deserialize<SyncPanel.FolderDto>(json)!);
        Assert.Equal("↔", vm.Arrow);
        Assert.Equal("", vm.Problem);
    }

    // A BLANK MODE IS NOT TWO-WAY, and the renderer used to say it was.
    //
    // `EffectiveMode` reads an absent mode as the pre-S6 behaviour — a
    // local folder publishes — and AGENTS.md states that defaulting it to
    // `both` is the mistake that is not symmetric. The row defaulted a
    // blank to `"both"` anyway. Go now sends the resolved mode, so a
    // blank arriving here means something upstream is broken and the row
    // must not paper over it.
    [AvaloniaFact]
    public void A_Blank_Mode_Is_Never_Drawn_As_Two_Way()
    {
        const string json = """
        {
          "id": "x.shared", "label": "shared", "local": true, "root": "shared",
          "localRoot": "shared", "path": "/p", "origin": "local", "mode": "",
          "mounted": true, "syncing": false, "accepted": false, "note": "",
          "filesPresent": 0, "filesIngested": 0, "filesObservable": true,
          "receiveFrom": [], "syncingWith": [], "folderProblems": [], "peers": []
        }
        """;

        var vm = SyncPanel.FolderVm.From(JsonSerializer.Deserialize<SyncPanel.FolderDto>(json)!);
        Assert.NotEqual("↔", vm.Arrow);
        Assert.NotEqual("both", vm.Mode);
    }

    // The control arm: the section stays hidden when nothing is wrong. A
    // permanently-visible "Needs attention" heading is chrome, and chrome
    // is precisely what an operator learns to skip — which would
    // reintroduce the defect in a new form.
    [AvaloniaFact]
    public void A_Healthy_Peer_Shows_No_Attention_Section()
    {
        SyncPanel.AutoLoadOnOpen = false;
        var (w, p) = Settle(_fx.DefaultPeer);
        try
        {
            Assert.False(p.ProblemsSectionVisible);
        }
        finally
        {
            w.Close();
            SyncPanel.AutoLoadOnOpen = true;
        }
    }

    private sealed class TestHost : IPanelHost
    {
        public event System.Action<string>? SelectedPath;
        public string? CurrentSelectedPath { get; private set; }
        public void PublishSelectedPath(string path)
        {
            CurrentSelectedPath = path;
            SelectedPath?.Invoke(path);
        }
        public void RequestPeerStatusRefresh() { }
    }
}
