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
