using System;
using System.IO;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 coverage for what the panel is OBLIGED to say about its own
// trust root.
//
// # The defect these exist for, and why it was invisible
//
// AP45: a pin taken from an origin's unsigned `entity-deployment.json`
// is trust-on-first-use — the origin picked its own trust root — and
// **every surface must say so**. `BrowseModel` computes
// `RegistryPinFromOrigin` and `RegistryRebasedFrom` for exactly that
// purpose, and the bridge marshals the whole view, so both fields were
// on the wire.
//
// `BrowserPanel.View` did not declare either property. System.Text.Json
// dropped them silently — no warning, no exception, no missing-member
// diagnostic — so the GUI, which is the surface an operator actually
// uses, met none of the obligation while entity-shell met all of it.
// A dropped DTO field fails in the one way a missing method cannot: the
// sentence simply never appears, and every test that did not look for it
// stayed green.
//
// So these tests assert on the WORDS. That is unusual and it is the
// point: the obligation is a sentence, not a code path.
[Collection(nameof(BridgeCollection))]
public sealed class BrowserPanelProvenanceTests
{
    private const string RegistryPeer = "2KBLkCxvkgobuauPA6zPfKarpuRRnnWHL98n8Gv1GNmybr";

    private readonly BridgeFixture _bridge;

    public BrowserPanelProvenanceTests(BridgeFixture bridge) => _bridge = bridge;

    private static string FixtureRoot()
    {
        var candidates = new[]
        {
            Path.Combine("..", "..", "..", "..", "..", "fetch", "testdata", "crossimpl-rust-federation"),
            Path.Combine("/src", "entity-workbench-go", "fetch", "testdata", "crossimpl-rust-federation"),
        };
        foreach (var c in candidates)
        {
            if (Directory.Exists(c)) return Path.GetFullPath(c);
        }
        throw new DirectoryNotFoundException("the frozen crossimpl-rust-federation fixture is missing");
    }

    // An operator-supplied pin is NOT trust-on-first-use, and the panel
    // must not say it is. This is the control: without it, a panel that
    // printed the TOFU warning unconditionally would pass the test below
    // while telling every operator something false.
    [AvaloniaFact]
    public void An_Operator_Supplied_Pin_Is_Not_Labelled_TOFU()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        using var panel = new BrowserPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 1200, Height = 700 };
        window.Show();
        panel.PinForTests(
            origin.Prefix, RegistryPeer,
            "/registry/" + RegistryPeer, "/registry/content",
            "/registry/" + RegistryPeer + "/system/peer/published-root", "sharded-2-4");

        var all = panel.RegistryTextForTests + " " + panel.ChromeTextForTests;
        Assert.Contains(RegistryPeer, panel.RegistryTextForTests, StringComparison.Ordinal);
        Assert.DoesNotContain("trust-on-first-use", all, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("TOFU", all, StringComparison.OrdinalIgnoreCase);
        window.Close();
    }

    // The pin is visible in the chrome, not only in a side column.
    //
    // It WAS in the side column and nowhere else, below an expanded
    // seven-field pin form — i.e. below the fold of a 300px column. The
    // operator's report was that the browser had stopped showing which
    // registry it was pinned to and looked like it was "picking this data
    // up out of nowhere". It was on screen; it was under a form.
    [AvaloniaFact]
    public void The_Pin_Is_Visible_In_The_Chrome()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        using var panel = new BrowserPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 1200, Height = 700 };
        window.Show();
        panel.PinForTests(
            origin.Prefix, RegistryPeer,
            "/registry/" + RegistryPeer, "/registry/content",
            "/registry/" + RegistryPeer + "/system/peer/published-root", "sharded-2-4");

        var chrome = panel.ChromeTextForTests;
        Assert.Contains("pinned to", chrome, StringComparison.OrdinalIgnoreCase);
        Assert.Contains(origin.Prefix, chrome, StringComparison.Ordinal);
        // The peer-id is abbreviated in the chrome, so assert on a prefix
        // long enough to be that peer and no other.
        Assert.Contains(RegistryPeer[..12], chrome, StringComparison.Ordinal);
        window.Close();
    }

    // A pin adopted from the origin's own `entity-deployment.json` MUST
    // be labelled trust-on-first-use — AP45's obligation, asserted as
    // the sentence it is.
    //
    // Driven through the pure function rather than a live origin: the
    // frozen federation serves no `transport-profile` (conformant, per
    // NETWORK §6.5.4, and why every other test here pins the layout by
    // hand), so the TOFU path cannot be reached against it at all. An
    // obligation whose only gate needs a fixture that does not exist is
    // an obligation with no gate — which is how this one came to be
    // missing.
    [AvaloniaFact]
    public void A_Pin_The_Origin_Nominated_Is_Labelled_Trust_On_First_Use()
    {
        var (detail, chrome) = BrowserPanel.RegistryProvenanceText(
            RegistryPeer, "https://entitychurchregistry.org",
            discovered: true, rebasedFrom: null, pinFromOrigin: true, fresh: "2026-08-30T12:00:00Z");

        Assert.Contains("trust-on-first-use", detail, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("the origin chose which key that is", detail, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("entity-deployment.json", detail, StringComparison.OrdinalIgnoreCase);
        // And in the chrome too: a side column is a place you look, the
        // chrome is a place you see.
        Assert.Contains("TOFU", chrome, StringComparison.OrdinalIgnoreCase);
    }

    // The THIRD provenance state is not collapsed into the second.
    //
    // A re-based layout is neither "discovered" nor "pinned": the origin
    // advertised a layout for a different peer and we substituted the
    // peer-id segment. Reporting it as "discovered" tells the operator
    // the origin advertised something it did not.
    [AvaloniaFact]
    public void A_Rebased_Layout_Is_Not_Reported_As_Discovered()
    {
        var (detail, chrome) = BrowserPanel.RegistryProvenanceText(
            RegistryPeer, "https://entitychurchregistry.org",
            discovered: true, rebasedFrom: "2KOtherPeerFeaturedByTheOrigin",
            pinFromOrigin: false, fresh: "2026-08-30T12:00:00Z");

        Assert.Contains("RE-BASED", detail, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("2KOtherPeerFeaturedByTheOrigin", detail, StringComparison.Ordinal);
        Assert.Contains("re-based", chrome, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("layout discovered from", detail, StringComparison.OrdinalIgnoreCase);
    }

    // THE DEFECT ITSELF: the render envelope's provenance fields must
    // survive deserialization.
    //
    // Both were computed by the model and marshalled by the bridge. The
    // `View` class simply did not declare them, and System.Text.Json
    // drops an undeclared member in silence — no warning, no exception,
    // no diagnostic of any kind. Every other test stayed green because
    // none of them looked at a value that had quietly become `false`.
    //
    // This is the only shape of assertion that catches it, and it is
    // cheap, which is the point: a field that crosses a boundary and is
    // never read is indistinguishable from a field that is never sent.
    [AvaloniaFact]
    public void The_Render_Envelope_Does_Not_Drop_The_Provenance_Fields()
    {
        const string json = """
        {"ok":true,"ops":1,"view":{
          "Registry":"2KBLk","RegistryOrigin":"https://x",
          "RegistryDiscovered":true,
          "RegistryRebasedFrom":"2KOther",
          "RegistryPinFromOrigin":true,
          "Body":{"Format":"html","Text":"lowered text","IsMarkdown":false,"FullBytes":8267316}
        }}
        """;
        var view = BrowserPanel.ParseViewForTests(json);
        Assert.True(view != null, "the envelope did not deserialize at all");

        Assert.True(BrowserPanel.ViewPinFromOriginForTests(view),
            "RegistryPinFromOrigin was dropped by the DTO — the model computes it, the bridge " +
            "sends it, and AP45 makes saying so an obligation. A field the DTO does not declare " +
            "is discarded silently.");
        Assert.Equal("2KOther", BrowserPanel.ViewRebasedFromForTests(view));

        // Body is the same class of defect one field over: without it the
        // panel cannot tell an HTML page from a markdown one, which is
        // how an 8.27 MB pre-rendered paper was fed to Markdig.
        Assert.False(BrowserPanel.ViewBodyIsMarkdownForTests(view),
            "Body.IsMarkdown was dropped — the panel would parse lowered HTML as markdown");
        Assert.Equal("lowered text", BrowserPanel.ViewBodyTextForTests(view));
    }
}
