using System.Linq;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;

namespace Workbench.Headless.Tests;

// PanelLogCoalesceTests — a repeating breadcrumb must not drown the log
// or the crash ring.
//
// Tier: unit (TESTING-STRATEGY §1).
//
// # The defect this gates
//
// An operator sent a 4,700-line run log on 2026-09-08 asking why their
// folder had stopped syncing one way. **4,489 lines were four render
// breadcrumbs repeating**; `peer-connections: NearbyRender h=1` alone was
// 2,383 of them, one every 1.2 seconds, carrying nothing but a handle that
// never changes. The answer was two `connection refused` lines buried
// under 95% noise.
//
// The half nobody can see is worse: `CrashDiagnostics` keeps a BOUNDED
// ring so a crash can be explained by what preceded it, and a panel that
// renders on every store write evicts the whole causal history in seconds.
//
// # Why the assertions are ON A UNIQUE TAG
//
// `CrashDiagnostics.Snapshot()` is a PROCESS-GLOBAL ring and xunit runs
// these classes in parallel with everything else in the assembly, so an
// assertion phrased over "the entries added since I started" is really an
// assertion over what every other test happened to log meanwhile. The
// first version of this file did exactly that and failed at 6 entries
// where it expected 3 — AP70's shape, in the breadcrumb ring rather than
// in a peer's tree. Each test uses a tag no other test writes and filters
// the ring to it.
public class PanelLogCoalesceTests
{
    private static string[] Mine(string tag) =>
        CrashDiagnostics.Snapshot().Where(l => l.Contains(tag)).ToArray();

    [Fact]
    public void A_Repeating_Breadcrumb_Is_Emitted_On_A_Power_Of_Two_Ladder()
    {
        const string tag = "panellog-test-ladder";
        PanelLog.ResetForTests();

        // The real shape, at a tenth of the real volume.
        for (var i = 0; i < 240; i++) PanelLog.Write(tag, "NearbyRender h=1");

        var mine = Mine(tag);

        // 1,2,4,8,16,32,64,128 -> 8 entries for 240 writes.
        Assert.True(mine.Length == 8,
            $"240 identical breadcrumbs produced {mine.Length} ring entries; expected 8 " +
            "(the 1st, 2nd, 4th ... 128th). A ring that grows with a repeating render " +
            "loses the causal history a crash investigation needs.\n  " +
            string.Join("\n  ", mine));

        // The count is carried ON the line — a reader has to be able to see
        // that the loop is running, which a silently dropped line does not
        // tell them.
        Assert.DoesNotContain("(x", mine[0]);
        Assert.Contains("(x2)", mine[1]);
        Assert.Contains("(x128)", mine[7]);
    }

    [Fact]
    public void A_RARE_Event_Is_Never_Suppressed()
    {
        const string tag = "panellog-test-rare";
        PanelLog.ResetForTests();

        // The property a time-window rate limiter does not have, and the
        // reason this is a counter and not a clock: something that happened
        // twice must be visible as having happened twice. In a crash
        // investigation "it fired once" and "it fired twice in a burst" are
        // different facts, and a limiter can hide the second.
        PanelLog.Write(tag, "Mount h=1");
        PanelLog.Write(tag, "Mount h=1");

        var mine = Mine(tag);
        Assert.True(mine.Length == 2,
            $"two occurrences produced {mine.Length} entries; a rare event must never " +
            "be suppressed.\n  " + string.Join("\n  ", mine));
        Assert.Contains("(x2)", mine[1]);
    }

    [Fact]
    public void Different_Tags_With_The_Same_Text_Are_Counted_Separately()
    {
        const string tagA = "panellog-test-splitA";
        const string tagB = "panellog-test-splitB";
        PanelLog.ResetForTests();

        // Anti-vacuity: a counter keyed on the MESSAGE alone passes every
        // assertion above and silently merges two panels' breadcrumbs into
        // one ladder — which is a false causal story, not less noise. Both
        // panels here render, and both must be visible as rendering.
        for (var i = 0; i < 4; i++)
        {
            PanelLog.Write(tagA, "Render h=1");
            PanelLog.Write(tagB, "Render h=1");
        }

        // 1,2,4 -> 3 entries each.
        Assert.True(Mine(tagA).Length == 3 && Mine(tagB).Length == 3,
            $"the two tags produced {Mine(tagA).Length} and {Mine(tagB).Length} entries; " +
            "expected 3 each. The ladder key is (tag, message), not message.");
    }

    [Fact]
    public void Interleaving_Does_Not_Defeat_It()
    {
        const string tag = "panellog-test-interleave";
        PanelLog.ResetForTests();

        // THE REGRESSION THAT MATTERS, and the one the first implementation
        // failed. Collapsing runs of CONSECUTIVE identical lines is the
        // obvious fix and it is nearly useless here: measured on the
        // operator's actual 4,700-line file it saved 14%, because the four
        // offenders interleave — NearbyRender, tree-view Render,
        // NearbyRender, ... — so nothing ever repeats consecutively.
        //
        // If someone replaces this with consecutive-run coalescing, this
        // test fails with 16 entries instead of 8.
        for (var i = 0; i < 8; i++)
        {
            PanelLog.Write(tag, "NearbyRender h=1");
            PanelLog.Write(tag, "Render h=1");
        }

        var mine = Mine(tag);
        Assert.True(mine.Length == 8,
            $"interleaved repeats produced {mine.Length} entries; expected 8 (1,2,4,8 for " +
            "each of the two messages). Consecutive-run coalescing gives 16 here and " +
            "saved 14% on the real log — the messages never repeat back to back.\n  " +
            string.Join("\n  ", mine));
    }
}
