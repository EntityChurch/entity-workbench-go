using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 headless coverage for SiteViewPanel (PHASE-I-SITE-VIEW-PLAN §7).
// Closes the regression gate the SITE thread left owed.
//
// Each test opens a panel against siteID="demo" — SiteOpen self-seeds
// the demo manifest+pages via EnsureDemoSite when absent. That means
// every test starts from the same content shape regardless of the
// shared BridgeFixture's accumulated state.
//
// Three contracts under test:
//   1. Mount → render smoke. The bridge wakes back through the
//      goroutine, the dispatcher posts, the body materializes.
//      If THIS fails, the panel never came up at all.
//   2. P3 wake debounce. 5 rapid Navigates collapse to a single
//      RerenderFromBridge. Regression here means a Navigate burst
//      would stack up renders and either flicker or stall the UI.
//   3. Deep navigation + GoBack. Three-levels-deep target resolves,
//      breadcrumb trail extends, GoBack pops. Exercises the model's
//      depth-bounded nav-walk through the real bridge — the Go side
//      already pins the depth-32 cap, this asserts the C# panel
//      threads a deep stack cleanly.
[Collection(nameof(BridgeCollection))]
public sealed class SiteViewPanelTests
{
    private readonly BridgeFixture _bridge;

    public SiteViewPanelTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public async Task Panel_Mount_Renders_Demo_Site_Title_And_Body()
    {
        var panel = new SiteViewPanel(_bridge.DefaultPeer, host: null, siteID: "demo");
        var window = new Window { Content = panel, Width = 800, Height = 600 };
        window.Show();

        // Initial render is synchronous inside the constructor, but the
        // first body materialization happens through the dispatcher.
        // Wait until we see at least one body recreation and one nav
        // link from the manifest (Home / Guide / About / Theory).
        var ready = await HeadlessPump.WaitUntil(
            () => panel.BodyRecreateCountForTests >= 1
               && panel.RenderCallCountForTests >= 1,
            TimeSpan.FromSeconds(5));

        Assert.True(
            ready,
            $"expected demo render within 5s. " +
            $"RenderCallCount={panel.RenderCallCountForTests} " +
            $"BodyRecreateCount={panel.BodyRecreateCountForTests}. " +
            "Most likely: SiteOpen failed to seed demo OR wake never marshaled.");

        // Title pulls from manifest ("Entity Demo Site"). If we see
        // "(site)" instead, the render produced an empty SiteTitle —
        // probably the resolver bailed before resolving the manifest.
        Assert.Equal("Entity Demo Site", panel.SiteTitleForTests);

        // Root page has an empty breadcrumb trail by model contract
        // (workbench/site_model_test.go pins "breadcrumbs empty on
        // root"). Sidebar carries the section list — assert at least
        // one entry so we know the manifest's nav resolved.
        Assert.True(
            panel.SidebarCountForTests >= 1,
            $"expected ≥1 sidebar entry on root, got {panel.SidebarCountForTests}");
    }

    [AvaloniaFact]
    public async Task Rapid_Navigate_Debounces_To_Single_Rerender()
    {
        // P3 contract: a burst of Navigates fires OnChange on each
        // location change, which queues a wake per Navigate at the
        // Go side. The C# wake handler single-flights (_renderQueued)
        // and the 150ms DispatcherTimer collapses the burst into one
        // RerenderFromBridge. After settle, exactly one render delta.
        //
        // Pattern lifted from MarkdownViewPanelStressTests.Rapid_Publish
        // _Debounces_To_Single_LoadPath — same shape, different surface.
        var panel = new SiteViewPanel(_bridge.DefaultPeer, host: null, siteID: "demo");
        var window = new Window { Content = panel, Width = 800, Height = 600 };
        window.Show();

        // Let the initial render settle so the baseline is stable.
        await HeadlessPump.WaitUntil(
            () => panel.RenderCallCountForTests >= 1,
            TimeSpan.FromSeconds(5));
        HeadlessPump.Flush();
        var baseline = panel.RenderCallCountForTests;

        // Five distinct demo-page Navigates back-to-back. The model is
        // idempotent on Navigate-to-current (loc == current early-return),
        // so distinct targets are required to actually fire OnChange 5×.
        string[] targets = new[]
        {
            "./guide/intro",
            "./guide/install",
            "./guide/advanced/internals",
            "./about",
            "./theory",
        };
        foreach (var t in targets)
        {
            panel.NavigateForTests(t);
        }

        // Wait past the 150ms debounce window plus generous margin.
        // 400ms total — survives CI jitter, tight enough that a debounce
        // regression to multi-second values would fail loudly.
        await HeadlessPump.WaitUntil(() => false, TimeSpan.FromMilliseconds(400));

        var delta = panel.RenderCallCountForTests - baseline;
        Assert.Equal(1, delta);
    }

    [AvaloniaFact]
    public async Task Deep_Navigation_And_GoBack_Update_Breadcrumb_Trail()
    {
        // Demo's deepest page is guide/advanced/internals (3 levels).
        // Navigate there, observe breadcrumb trail grew, GoBack, observe
        // it shrank again. Validates the bridge ↔ model ↔ render path
        // under non-trivial Location depth — the Go-side depth-32 cap
        // already has unit coverage in workbench/site_model_test.go;
        // this is the C# panel-level smoke that the cap doesn't crash
        // the render under real nav depth.
        var panel = new SiteViewPanel(_bridge.DefaultPeer, host: null, siteID: "demo");
        var window = new Window { Content = panel, Width = 800, Height = 600 };
        window.Show();

        await HeadlessPump.WaitUntil(
            () => panel.BodyRecreateCountForTests >= 1,
            TimeSpan.FromSeconds(5));
        var rootCrumbs = panel.BreadcrumbCountForTests;
        var baselineRender = panel.RenderCallCountForTests;

        panel.NavigateForTests("./guide/advanced/internals");

        // Wait for the post-debounce render. WaitUntil polls on the
        // render counter so we don't depend on a precise sleep value.
        var navRendered = await HeadlessPump.WaitUntil(
            () => panel.RenderCallCountForTests > baselineRender,
            TimeSpan.FromSeconds(2));
        Assert.True(navRendered,
            $"expected RenderCallCount > {baselineRender} after Navigate; " +
            $"got {panel.RenderCallCountForTests}");

        // Three-level-deep page: trail should be longer than the root's.
        // Breadcrumb count is "label" + "separator" interleaved (every
        // crumb after the first prefixes a " > "), so deeper paths grow
        // the child count strictly. Just assert it grew.
        Assert.True(
            panel.BreadcrumbCountForTests > rootCrumbs,
            $"expected deeper breadcrumb trail than root ({rootCrumbs}); " +
            $"got {panel.BreadcrumbCountForTests}");

        var atDepthCrumbs = panel.BreadcrumbCountForTests;
        var atDepthRender = panel.RenderCallCountForTests;

        // GoBack pops history → back to root → trail shrinks.
        panel.GoBackForTests();

        var backRendered = await HeadlessPump.WaitUntil(
            () => panel.RenderCallCountForTests > atDepthRender,
            TimeSpan.FromSeconds(2));
        Assert.True(backRendered,
            $"expected RenderCallCount > {atDepthRender} after GoBack");

        Assert.True(
            panel.BreadcrumbCountForTests < atDepthCrumbs,
            $"expected shorter breadcrumb trail after GoBack " +
            $"(was {atDepthCrumbs}, now {panel.BreadcrumbCountForTests})");
    }

}
