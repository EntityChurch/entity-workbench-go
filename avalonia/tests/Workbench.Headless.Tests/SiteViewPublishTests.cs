using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// SiteViewPublishTests — the GUI half of W4.
//
// The wire gate is `shellboot/public_site_scope_probe_test.go`: it is
// what establishes that a stranger can verify the site and read nothing
// else. These tests establish something the Go suites structurally
// cannot — that the facts reach a pixel.
//
// **The arm that matters is the disclosure sentence**, and the reason is
// AP49: an undeclared field crosses `System.Text.Json` and is dropped in
// total silence. On this surface the two fields most likely to be dropped
// are `publicPresent` and `problems`, and a panel that drops them renders
// "published, private, nothing wrong" for a site that is open to the
// world. That is the worst available failure for a surface whose only job
// is to say otherwise, and no Go test can see it — the Go side is correct
// either way.
//
// AP70 applies with teeth here: `BridgeFixture.DefaultPeer` is SHARED
// across the whole assembly, and this test writes the `default` policy
// row on it. So the publish/unpublish pair lives in ONE test that
// restores the row rather than in two that could interleave, and the
// final assertion is that it was restored.
[Collection(nameof(BridgeCollection))]
public sealed class SiteViewPublishTests
{
    private readonly BridgeFixture _bridge;

    public SiteViewPublishTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    // The read arm. Opening the panel must say something true about what
    // is published WITHOUT publishing anything — `PublishRender` mints
    // nothing, which is why it is safe here and on a wake.
    [AvaloniaFact]
    public void Opening_The_Panel_Reports_What_Is_Published_Without_Publishing()
    {
        var panel = new SiteViewPanel(_bridge.DefaultPeer, host: null, siteID: "demo");
        var window = new Window { Content = panel, Width = 900, Height = 700 };
        window.Show();
        window.UpdateLayout();

        var line = panel.PublishLineForTests;
        Assert.False(string.IsNullOrWhiteSpace(line));
        Assert.DoesNotContain("reading what is published", line);
        Assert.DoesNotContain("failed", line);

        // A read never claims a mint. If this line ever says "published
        // <prefix> — N keys, seq S" in the minted phrasing off an open,
        // the render export has started signing roots on panel open,
        // which would announce a new release of the site every time a
        // window was focused.
        Assert.DoesNotContain("keys, seq", line);
    }

    // The disclosure arm, and the AP49 gate.
    //
    // Publish → make public → assert the panel SAYS so → make private →
    // assert it stops saying so. Every assertion is on rendered text,
    // because the whole point is that the operator can see it.
    [AvaloniaFact]
    public async Task Making_A_Site_Public_Says_So_On_The_Panel_And_Making_It_Private_Stops()
    {
        var panel = new SiteViewPanel(_bridge.DefaultPeer, host: null, siteID: "demo");
        var window = new Window { Content = panel, Width = 900, Height = 700 };
        window.Show();
        window.UpdateLayout();

        // Anti-vacuity: it must be private to begin with, or "it says
        // private at the end" proves nothing about the button.
        Assert.Contains("private", panel.PublishAccessForTests);

        await panel.PublishForTests(1);

        var access = panel.PublishAccessForTests;
        Assert.Contains("PUBLIC", access);
        Assert.Contains("any peer that can dial this one", access);

        var line = panel.PublishLineForTests;
        Assert.Contains("published", line);
        Assert.DoesNotContain("failed", line);

        try
        {
            // The problems block is the AP84 obligation on this panel.
            // The fixture peer binds no listener, so the publish is
            // reachable by nobody and the panel MUST say so — a public
            // site that nothing can dial is exactly the state an operator
            // would otherwise stare at wondering why their friend sees
            // nothing.
            Assert.Contains("not listening", panel.PublishProblemsForTests);
        }
        finally
        {
            // Restore the shared fixture peer. AP70: the `default` row is
            // real state on a peer the rest of the assembly uses.
            await panel.PublishForTests(-1);
        }

        Assert.Contains("private", panel.PublishAccessForTests);
        Assert.DoesNotContain("PUBLIC", panel.PublishAccessForTests);
    }
}
