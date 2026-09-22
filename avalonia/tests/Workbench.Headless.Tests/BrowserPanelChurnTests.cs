using System;
using System.IO;
using System.Linq;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 coverage for the panel's REDRAW behaviour, which had no
// coverage at all and which an operator hit before any of it was
// measured.
//
// # The report
//
// *"When I click something it looks like … it resets and scrolls me up
// to the top of the page … the way it randomly moves the screen around,
// I haven't figured out the logic."*
//
// There was no logic. `Refresh()` ran on every wake and unconditionally
// `Clear()`ed and rebuilt the name list, the site list, the trust rail
// and the entire page body — whether or not any of them had changed —
// and `MarkNavigating()` blanked the rail for the whole duration of a
// navigation, which before the content cache was six to fifteen seconds.
// Three columns re-laid out per click, with the middle one losing its
// scroll offset because its content had been replaced.
//
// # Why these assert on COUNTS and not on appearance
//
// "It jumps" is not directly assertable in a headless renderer. What is
// assertable is the cause: how many times the body was rebuilt, and
// whether the rail was emptied. `BodyRecreateCountForTests` already
// existed for exactly this and nothing had ever used it to pin the
// negative — that a redundant Refresh rebuilds NOTHING.
[Collection(nameof(BridgeCollection))]
public sealed class BrowserPanelChurnTests
{
    private const string RegistryPeer = "2KBLkCxvkgobuauPA6zPfKarpuRRnnWHL98n8Gv1GNmybr";

    private readonly BridgeFixture _bridge;

    public BrowserPanelChurnTests(BridgeFixture bridge) => _bridge = bridge;

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
        throw new DirectoryNotFoundException(
            "the frozen crossimpl-rust-federation fixture is not reachable from the test image; " +
            "fix the path rather than skipping.");
    }

    private static (Window window, BrowserPanel panel) Pinned(FixtureOrigin origin, long peer)
    {
        var panel = new BrowserPanel(peer);
        var window = new Window { Content = panel, Width = 1200, Height = 700 };
        window.Show();
        panel.PinForTests(
            origin.Prefix,
            RegistryPeer,
            "/registry/" + RegistryPeer,
            "/registry/content",
            "/registry/" + RegistryPeer + "/system/peer/published-root",
            "sharded-2-4");
        return (window, panel);
    }

    // A Refresh that changes nothing must rebuild nothing.
    //
    // This is the churn defect stated as its own negative. Before the
    // signature guards, ten redundant Refreshes were ten full body
    // rebuilds and ten teardown-and-rebuild cycles of three lists — and
    // a wake can arrive for an operation already drawn, so redundant
    // Refreshes are the normal case, not a corner.
    [AvaloniaFact]
    public void A_Redundant_Refresh_Rebuilds_Nothing()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        var (window, panel) = Pinned(origin, _bridge.DefaultPeer);
        using (panel)
        {
            panel.GoForTests("docs.entitychurch.org");
            Assert.Equal("", panel.ErrorTextForTests);

            var before = BrowserPanel.BodyRecreateCountForTests;
            for (int i = 0; i < 10; i++)
            {
                panel.Refresh();
                window.UpdateLayout();
            }
            var rebuilds = BrowserPanel.BodyRecreateCountForTests - before;

            Assert.True(rebuilds == 0,
                $"ten redundant Refreshes rebuilt the page body {rebuilds} times. Each rebuild " +
                "discards the visual tree and resets the centre ScrollViewer to the top, which is " +
                "the 'it jumps around / scrolls me back up' report verbatim.");
        }
        window.Close();
    }

    // The name list survives a navigation untouched.
    //
    // Names change when the registry is re-enumerated and at no other
    // time; a navigation must not tear the left column down and rebuild
    // it. This also shrinks AP46's blast radius — a list that is not
    // Clear()ed never runs container teardown.
    [AvaloniaFact]
    public void Navigating_Does_Not_Rebuild_The_Name_List()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        var (window, panel) = Pinned(origin, _bridge.DefaultPeer);
        using (panel)
        {
            var namesBefore = panel.NameRowsForTests.ToList();
            Assert.NotEmpty(namesBefore);

            panel.GoForTests("docs.entitychurch.org");
            window.UpdateLayout();

            Assert.Equal(namesBefore, panel.NameRowsForTests.ToList());
        }
        window.Close();
    }

    // The trust rail is DIMMED during a navigation, never emptied.
    //
    // # Why this is a correction and not a relaxation
    //
    // The rule it replaces — "a stale chain beside fresh bytes is the one
    // lie this panel exists to prevent" — is right, and was being applied
    // one step too early. The page does not become fresh when a
    // navigation STARTS; it is swapped in Refresh, atomically with the
    // new chain. In between, the reader is still looking at the previous
    // page and the previous chain is exactly what describes it. Clearing
    // the rail there deleted a true statement and left a page with no
    // provenance beside it for the whole navigation.
    //
    // So the invariant this asserts is the real one: **page and chain are
    // replaced together, or neither is.**
    [AvaloniaFact]
    public void The_Rail_Is_Never_Empty_Beside_A_Page()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        var (window, panel) = Pinned(origin, _bridge.DefaultPeer);
        using (panel)
        {
            panel.GoForTests("docs.entitychurch.org");
            Assert.True(panel.StepCountForTests > 0);
            Assert.True(panel.BodyBlockCountForTests > 0);

            var stepsBefore = panel.StepNamesForTests.ToList();

            // Start a second navigation and inspect the moment after the
            // start-side bookkeeping has run.
            panel.MarkNavigatingForTests();
            window.UpdateLayout();

            Assert.True(panel.StepCountForTests > 0,
                "the rail was emptied at the start of a navigation, leaving the page that is still " +
                "on screen with no provenance beside it for the duration");
            Assert.Equal(stepsBefore, panel.StepNamesForTests.ToList());
            Assert.True(panel.RailIsDimmedForTests,
                "the rail describes the OLD page during a navigation, so it must be visibly " +
                "provisional — an undimmed stale rail is the lie the old clearing was guarding against");
            Assert.True(panel.NavigatingNoticeVisibleForTests,
                "the rail was dimmed with no explanation; an unexplained greyed-out trust column " +
                "is worse than a cleared one");
        }
        window.Close();
    }

    // A REFUSED navigation still clears the page, and that rule is
    // untouched by the above.
    //
    // The distinction is exact: a page whose chain FAILED must go,
    // because there is no true pairing to preserve. A page whose chain is
    // merely being re-checked stays with its own chain. Without this
    // test, "stop clearing the rail" could be over-applied into "stop
    // clearing anything", which would put unverified bytes on screen.
    [AvaloniaFact]
    public void A_Refused_Navigation_Still_Clears_The_Page()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        var (window, panel) = Pinned(origin, _bridge.DefaultPeer);
        using (panel)
        {
            panel.GoForTests("docs.entitychurch.org");
            Assert.True(panel.BodyBlockCountForTests > 0);

            panel.GoForTests("no-such-name.entitychurch.org");
            window.UpdateLayout();

            Assert.NotEqual("", panel.ErrorTextForTests);
            Assert.Equal(0, panel.BodyBlockCountForTests);
            Assert.Equal("", panel.TitleTextForTests);
        }
        window.Close();
    }

    // The address box is not rewritten under the reader's cursor.
    //
    // It used to be assigned on every Refresh, so what a user had typed
    // ("billslab.com") was replaced with the canonical form
    // ("entity://billslab.com/billslab-main/index") and the caret moved.
    [AvaloniaFact]
    public void A_Redundant_Refresh_Does_Not_Touch_The_Address_Box()
    {
        using var origin = new FixtureOrigin(FixtureRoot());
        var (window, panel) = Pinned(origin, _bridge.DefaultPeer);
        using (panel)
        {
            panel.GoForTests("docs.entitychurch.org");
            var shown = panel.AddressTextForTests;
            Assert.NotEqual("", shown);

            for (int i = 0; i < 5; i++) panel.Refresh();
            Assert.Equal(shown, panel.AddressTextForTests);
        }
        window.Close();
    }
}
