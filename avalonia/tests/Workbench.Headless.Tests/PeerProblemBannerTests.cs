using System;
using System.Linq;
using System.Text.Json;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia;
using Xunit;

namespace EntityAvalonia.Tests;

// The peer problem banner — the surface that did not exist on 2026-09-10,
// and whose absence is the whole reason an operator spent a morning
// believing a working app was eating their files.
//
// WHAT ACTUALLY HAPPENED. The reconciler diagnosed the fault correctly at
// startup and in full: "could not open our own connection to this peer —
// nothing we write to a shared folder will reach them". That went to
// stderr, i.e. to `avalonia/run-logs/`. The operator's saved layout held
// `tree-view` and `peer-connections`; neither can say anything about
// sharing, and the Sharing Status panel — where we had put every warning
// the day before — was not open. So the app's own correct diagnosis
// reached no pixel, while two other surfaces actively reassured: the peer
// status line said "1 remote" and the Nearby row said "Connected", both
// derived from `Workspace.Conns`, which is an ADDRESS BOOK and survives
// the far peer being switched off.
//
// The generalisable rule, and the reason this file exists rather than one
// more assertion inside a panel test: **a diagnosis whose visibility
// depends on which panels the operator happens to have open is not a
// surface.** D23/AP73 aimed at the application instead of at a model.
//
// The two tests below defend the two independent ways this goes quiet
// again: the banner not rendering what it is given, and the bridge field
// it reads being renamed out from under it (AP49 — System.Text.Json drops
// an undeclared member in silence, and a dropped `problems` renders as
// "everything is fine", which is the worst available failure for a
// surface whose only job is to say otherwise).
[Collection(nameof(BridgeCollection))]
public sealed class PeerProblemBannerTests
{
    private readonly BridgeFixture _bridge;

    public PeerProblemBannerTests(BridgeFixture bridge) => _bridge = bridge;

    private (Window, PeerView) Open()
    {
        var view = new PeerView(_bridge.DefaultPeer, _bridge.DefaultPeer);
        var window = new Window { Content = view, Width = 1000, Height = 700 };
        window.Show();
        HeadlessPump.Flush();
        // Show() is not a layout pass (AP46's transferable half).
        window.UpdateLayout();
        return (window, view);
    }

    // The banner renders a problem it is given, and hides when there are
    // none. The negative arm is the load-bearing one: a banner stuck
    // visible would be ignored within a day, and a banner stuck hidden is
    // the defect this file is about.
    [AvaloniaFact]
    public void A_Problem_Reaches_The_Screen_And_An_Empty_List_Hides_The_Banner()
    {
        var (window, view) = Open();
        using (window as IDisposable)
        {
            view.ApplyProblemsForTests(Array.Empty<string>(), reconciled: true);
            window.UpdateLayout();
            Assert.False(view.ProblemBannerVisibleForTests,
                "with no problems the banner must be hidden — a permanent banner is a banner nobody reads");

            const string real =
                "2kluxpxmcqx1: could not open our own connection to this peer — "
                + "nothing we write to a shared folder will reach them";
            view.ApplyProblemsForTests(new[] { real }, reconciled: true);
            window.UpdateLayout();

            Assert.True(view.ProblemBannerVisibleForTests,
                "a reconciler problem must be visible without opening any particular panel");
            var text = view.ProblemBannerTextForTests;
            Assert.Contains("could not open our own connection", text);
            // The consequence sentence is the half that tells the operator
            // which machine to walk to. Losing it turns a diagnosis back
            // into a symptom.
            Assert.Contains("will reach them", text);
        }
    }

    // A read is captioned as a read. `StatusRender` observes and never
    // dials, so it reports what the last pass established — and "verified
    // just now" is a different claim. Carrying it in the outcome rather
    // than remembering which export was called is the rule the Sharing
    // Status panel already follows; this is the same rule one level up.
    [AvaloniaFact]
    public void An_Unreconciled_Reading_Does_Not_Claim_To_Have_Just_Verified()
    {
        var (window, view) = Open();
        using (window as IDisposable)
        {
            view.ApplyProblemsForTests(new[] { "peer x: unreachable" }, reconciled: false);
            window.UpdateLayout();
            var stale = view.ProblemBannerTextForTests;
            Assert.Contains("last pass", stale);
            Assert.DoesNotContain("just now", stale);

            view.ApplyProblemsForTests(new[] { "peer x: unreachable" }, reconciled: true);
            window.UpdateLayout();
            Assert.Contains("just now", view.ProblemBannerTextForTests);
        }
    }

    // AP49: the banner reads exactly two fields off StatusRender. If either
    // is renamed on the Go side the banner reports "no problems" for a peer
    // that has them, silently, and every test above still passes because
    // they drive the renderer directly. This is the only arm that crosses
    // the bridge.
    [AvaloniaFact]
    public void The_Status_Envelope_Carries_Problems_And_Reconciled()
    {
        var reply = Bridge.TakeString(Bridge.StatusRender(_bridge.DefaultPeer));
        using var doc = JsonDocument.Parse(reply);
        var root = doc.RootElement;

        Assert.True(root.TryGetProperty("ok", out _), $"no `ok` in StatusRender envelope: {reply}");
        Assert.True(root.TryGetProperty("problems", out var problems),
            $"the banner reads `problems` and it is not in the envelope: {reply}");
        Assert.True(root.TryGetProperty("reconciled", out var reconciled),
            $"the banner reads `reconciled` and it is not in the envelope: {reply}");

        // Shape, not contents: the fixture peer is shared and other tests
        // write real declarations to it (AP70), so asserting on the number
        // of problems here would be asserting on everything every other
        // test in the assembly has ever done to this peer.
        Assert.Equal(JsonValueKind.Array, problems.ValueKind);
        Assert.True(reconciled.ValueKind is JsonValueKind.True or JsonValueKind.False);
    }
}
