using System;
using System.Linq;
using System.Text.Json;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.VisualTree;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// FeedPanel — the surface `follow` / `timeline` / `ref` did not have.
//
// `FEED-R7` is a **[MUST]** that a reader be able to tell
// `APP-CONVENTION-FEED` §2.2.2's four resolution outcomes apart. W6 built
// the resolver and gave it a shell verb; a resolver whose outcome a
// renderer drops satisfies that MUST nowhere, which is D23 at field
// granularity and AP49's shape one boundary out.
//
// # The arm that matters is the envelope arm
//
// Two of the three tests below drive the panel directly and would both
// keep passing if `row`, `moved`, `attributed` or `listed` were renamed
// on the Go side — `System.Text.Json` drops an undeclared member in
// total silence, and on this surface a dropped field renders as
// *attributed*, *current* and *fine*. That is the worst available failure
// for a panel whose job is to say otherwise, so the third test reads the
// raw envelope and asserts the names.
//
// ⚠ **AP70: the fixture peer is SHARED across the assembly**, and other
// tests write real state to it. So nothing here asserts over *the panel*
// — only over rows this test created, or over the envelope's SHAPE.
[Collection(nameof(BridgeCollection))]
public sealed class FeedPanelTests
{
    private readonly BridgeFixture _bridge;

    public FeedPanelTests(BridgeFixture bridge) => _bridge = bridge;

    private (Window, FeedPanel) Open()
    {
        var panel = new FeedPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 900, Height = 700 };
        window.Show();
        HeadlessPump.Flush();
        // Show() is not a layout pass (AP46's transferable half). Without
        // this the panel has no realized containers, so anything that
        // depends on a row existing is unreachable from the test while it
        // reports green.
        window.UpdateLayout();
        return (window, panel);
    }

    // AP64: every panel declares a height floor, and one that forgets
    // claims the 200px stack default — which is less than this panel's own
    // fixed chrome, so it renders clipped with an unreachable button and
    // nothing to scroll. Cheap to assert, and the failure it prevents was
    // found by an operator rather than by a test.
    [AvaloniaFact]
    public void The_Panel_Declares_A_Height_Floor_Above_The_Stack_Default()
    {
        var (window, panel) = Open();
        using (window as IDisposable)
        {
            Assert.True(panel.PreferredSlotMinHeight > 200,
                "a panel that does not declare its own floor gets 200px, which is less than this "
                + "panel's fixed chrome — the operator's only workaround is closing other panels");
        }
    }

    // The panel opens, renders, and carries VERBS.
    //
    // AP57: a read-only surface over a read-write model is D23's violation
    // in a form `make reachability` cannot see — the sweep asks whether a
    // model has *a* surface, and one render export satisfies it
    // completely. **The tell is a panel with no verb in it**, so the verb
    // buttons are asserted by name rather than assumed.
    [AvaloniaFact]
    public void The_Panel_Carries_Its_Three_Verbs()
    {
        var (window, panel) = Open();
        using (window as IDisposable)
        {
            var labels = panel.GetVisualDescendants()
                .OfType<Button>()
                .Select(b => b.Content?.ToString() ?? "")
                .ToList();

            Assert.Contains("Follow", labels);
            Assert.Contains("Read", labels);
            Assert.Contains("Catch up", labels);
            Assert.Contains("Resolve", labels);

            // The produce side. Until 2026-09-16 every verb on this panel
            // was a READER's: an operator could see what everyone else had
            // posted and could not make their own feed readable, which is
            // AP57's tell — a read-only surface over a read-write model,
            // the one shape `make reachability` cannot see.
            Assert.Contains("Publish feed", labels);

            // Read and Catch up are SEPARATE controls on purpose: one
            // dials, the other dials and moves durable read positions. A
            // single button with a checkbox beside it would make a
            // refresh able to change durable state depending on a toggle
            // the operator set an hour ago.
            Assert.Equal(1, labels.Count(l => l == "Read"));
            Assert.Equal(1, labels.Count(l => l == "Catch up"));
        }
    }

    // ⭐ The envelope arm. AP49.
    //
    // Every field asserted here is one whose ABSENCE renders as good news:
    // a missing `attributed` deserializes to false — which is the safe
    // direction — but a missing `row` or `moved` or `listed` erases the
    // distinction the panel exists to draw. Asserted on the RAW reply so
    // a Go-side rename fails here rather than silently downgrading the
    // surface.
    //
    // ⚠ **The comment above used to name `row`, `moved` and `listed` as
    // the dangerous category and then assert none of them.** That is
    // AP45's shape sitting inside an AP49 gate: a criterion stated in
    // prose, never applied, and invisible to review forever because it
    // reads as a description of what the test does. Corrected
    // 2026-09-15; the error is left visible because the next person to
    // add a field here will be reading this paragraph.
    //
    // What is covered here now, and what is NOT:
    //
    //   - `row` and `moved` ARE asserted — they ride on the ref envelope,
    //     which the malformed-reference call already returns in full, so
    //     they cost nothing and were simply missed.
    //   - `via` and `listed` are NOT, and cannot be from this fixture.
    //     `via` needs a source, i.e. a follow; `listed` needs an ENTRY,
    //     i.e. a reachable publisher that has posted and published — and
    //     no suite in this repo may reach the public internet, while
    //     writing a follow onto `BridgeFixture.DefaultPeer` writes real
    //     state every other test in the assembly then renders (AP70).
    //     They are gated on the Go side instead, at both layers and in
    //     both directions: `publish/feed_road_wiring_test.go` asserts
    //     `Via`/`Listed` with an enumeration control arm proving each can
    //     take its other value, and `shellboot/feed_timeline_test.go`
    //     asserts the sentence reaches the rendered source block.
    //
    // ⛔ So a Go-side rename of `via` or `listed` still downgrades the
    // GUI silently. That is a real hole, named rather than papered over,
    // and the honest fix is a bridge-envelope gate that can build a
    // populated timeline — which needs a second peer in this fixture.
    [AvaloniaFact]
    public void The_Feed_Envelopes_Do_Not_Drop_The_Fields_The_Panel_Reads()
    {
        // Follows: reachability moved onto the row precisely so a renderer
        // would not have to parse it back out of a prose problem string.
        var follows = Bridge.TakeString(Bridge.FeedFollowsRender(_bridge.DefaultPeer));
        using (var doc = JsonDocument.Parse(follows))
        {
            var root = doc.RootElement;
            Assert.True(root.TryGetProperty("rows", out _),
                "`rows` is gone from the follows envelope — the panel renders an empty list");
            Assert.True(root.TryGetProperty("problems", out _),
                "`problems` is gone — §2.4's privacy sentence belongs to no row, so losing this "
                + "field is the only way it reaches the operator disappearing");
        }

        // The timeline envelope. `sources` is FEED-R23: a view of more
        // than one publisher MUST declare what produced it, and a list of
        // entries with no provenance is exactly the view that MUST NOT
        // exist. Reading with no follows is fine — the shape is the
        // assertion, not the contents.
        var timeline = Bridge.TakeString(Bridge.FeedTimelineRead(_bridge.DefaultPeer, "", 10));
        using (var doc = JsonDocument.Parse(timeline))
        {
            var root = doc.RootElement;
            Assert.True(root.TryGetProperty("sources", out _),
                "`sources` is gone from the timeline envelope — FEED-R23 makes a provenance-free "
                + "multi-publisher view the one that MUST NOT exist, and this is how it becomes one");
            Assert.True(root.TryGetProperty("entries", out _));
            Assert.True(root.TryGetProperty("advanced", out _),
                "`advanced` is gone — the panel would then be unable to tell an operator whether "
                + "their saved read position actually moved");
        }

        // The reference envelope, which is the FEED-R7 one. Resolved
        // against a deliberately malformed reference: the outcome does not
        // matter here, the FIELD NAMES do, and a malformed ref keeps this
        // arm from dialling anything (no suite in this repo may reach the
        // public internet).
        var reference = Bridge.TakeString(
            Bridge.FeedResolveRef(_bridge.DefaultPeer, "not-a-reference"));
        using (var doc = JsonDocument.Parse(reference))
        {
            var root = doc.RootElement;
            // `error` is the publisher-fault channel and is what a
            // malformed input produces. Its presence is the proof that the
            // two are separate channels rather than one.
            Assert.True(root.TryGetProperty("error", out var err),
                "`error` is gone from the ref envelope — then a fault about the PUBLISHER and an "
                + "outcome about the REFERENCE arrive indistinguishable, which is the collapse "
                + "the typed outcome exists to prevent");
            Assert.False(string.IsNullOrEmpty(err.GetString()),
                "a malformed reference produced no error — then the panel renders a blank outcome");

            // `row` is §2.2.2's four outcomes and `moved` is the boolean
            // beside it. They are separate fields on purpose: a caller
            // switching on the enum can forget a case, and a caller
            // rendering provenance reads one boolean — so losing either
            // one collapses a distinction `FEED-R7` MUSTs a reader be
            // able to draw. `moved` is the worse loss of the two, because
            // absent deserializes to `false`, which is the ASSERTION that
            // the document has not changed since it was linked.
            Assert.True(root.TryGetProperty("row", out _),
                "`row` is gone from the ref envelope — §2.2.2's four outcomes then arrive as one, "
                + "and `FEED-R7` is a MUST about a reader being able to tell which one it got");
            Assert.True(root.TryGetProperty("moved", out _),
                "`moved` is gone from the ref envelope — and its absence deserializes to false, "
                + "which does not read as a missing field. It reads as a positive claim that the "
                + "document has not moved since somebody linked to it");
        }
    }

    // ⭐ The own-feed envelope arm. AP49, and this one is the reason the
    // section exists at all.
    //
    // `signatureNote` is `FEED-R2`'s attribution caveat and it had never
    // crossed this boundary in ANY form before 2026-09-16 — so a GUI
    // operator's feed could be unattributable to every static reader with
    // nothing on screen saying so. A rename on the Go side puts it back in
    // exactly that state, silently, because a missing string deserializes
    // to `""` and `""` is the value that means *attributable*. **The
    // dangerous fields here all fail SAFE-LOOKING**: absent `coversFeed`
    // and absent `current` are `false`, which over-warns; absent
    // `signatureNote` is the one that under-warns, and it is the field the
    // whole export was added for.
    //
    // Read against the shared fixture peer deliberately: the SHAPE is the
    // assertion and the contents are whatever that peer happens to hold
    // (AP70). This export dials nobody, so it is safe in a suite that may
    // not reach the network.
    [AvaloniaFact]
    public void The_Own_Feed_Envelope_Does_Not_Drop_The_Attribution_Fields()
    {
        var own = Bridge.TakeString(Bridge.FeedOwnRender(_bridge.DefaultPeer));
        using var doc = JsonDocument.Parse(own);
        var root = doc.RootElement;

        Assert.True(root.TryGetProperty("signatureNote", out _),
            "`signatureNote` is gone from the own-feed envelope — and its absence deserializes to "
            + "the empty string, which on this surface is not a missing field. It is the positive "
            + "claim that every entry is attributable to a static reader, which is the exact "
            + "sentence A-38 exists to make true or false");
        Assert.True(root.TryGetProperty("published", out _),
            "`published` is gone — the panel cannot then tell *no root is signed* from *a root is "
            + "signed and does not reach the feed*, and those send an operator to different actions");
        Assert.True(root.TryGetProperty("coversFeed", out _),
            "`coversFeed` is gone — this is the quietest of the three reach states: everything "
            + "looks published and the feed keys are not in the root");
        Assert.True(root.TryGetProperty("current", out _),
            "`current` is gone — posting without re-publishing is invisible to every reader on "
            + "both roads and looks exactly like not having posted");
        Assert.True(root.TryGetProperty("contentSet", out _),
            "`contentSet` is gone — after A-38 the prefix does not imply the set, so this is the "
            + "only field that says whether a peer-root root committed to 9 keys or to 399");
        Assert.True(root.TryGetProperty("posted", out _),
            "`posted` is gone — *never posted* and *a feed that is empty* are different facts and "
            + "only the first is a reason to say \"nothing here yet\"");
        Assert.True(root.TryGetProperty("problems", out _),
            "`problems` is gone — that is the operator's whole action list, and this panel is "
            + "where it renders (AP84: stderr is not a surface)");

        // And the publish envelope's half of the same change. A surface
        // MUST render the content set: 9 keys under `/` is only legible if
        // you already know a curated set exists.
        var publish = Bridge.TakeString(Bridge.PublishRender(_bridge.DefaultPeer));
        using var pdoc = JsonDocument.Parse(publish);
        Assert.True(pdoc.RootElement.TryGetProperty("contentSet", out _),
            "`contentSet` is gone from the publish envelope — the Local Site panel then reports a "
            + "binding count with nothing saying which choice produced it");
    }

    // ⭐ Three states, not two — the rule the conflict-rule control earned,
    // applied to attribution.
    //
    // `signatureNote` empty is a REAL ANSWER (A-38 ruling (D) applied) and
    // is ALSO what an unpublished peer produces, because there is no root
    // to be wrong about. A renderer that treats empty as *attributable*
    // therefore tells an operator who has published nothing that every
    // entry is attributable — confidently, and in the wrong direction.
    //
    // Driven through `ApplyOwnFeedForTests` rather than the bridge: two of
    // the three states are facts about a published root, and the fixture
    // peer is shared across the assembly (AP70), so the bridge cannot reach
    // them deterministically. The field names are covered above, against
    // what Go actually emits.
    [AvaloniaFact]
    public void An_Unpublished_Feed_Does_Not_Render_As_Attributable()
    {
        var (window, panel) = Open();
        using (window as IDisposable)
        {
            // (1) Posted, nothing published. The note is empty and must not
            //     be read as good news.
            panel.ApplyOwnFeedForTests(
                """
                {"posted":true,"entries":2,"published":false,"coversFeed":false,
                 "current":false,"contentSet":"","signatureNote":"","problems":[]}
                """);
            Assert.DoesNotContain("attributable", panel.OwnFeedAttributionForTests);
            Assert.Contains("nothing is published", panel.OwnFeedLineForTests);

            // (2) Published, covering, with the caveat. The caveat is the
            //     sentence, verbatim — a panel that paraphrased it would be
            //     the second place that wording lives.
            panel.ApplyOwnFeedForTests(
                """
                {"posted":true,"entries":2,"published":true,"coversFeed":true,"current":true,
                 "prefix":"app/feed/","contentSet":"",
                 "signatureNote":"a static reader cannot attribute any entry","problems":[]}
                """);
            Assert.Contains("a static reader cannot attribute any entry",
                panel.OwnFeedAttributionForTests);

            // (3) Published, covering, silent — and ONLY here may the panel
            //     say so. This is the arm that fails if a renderer
            //     short-circuits on the empty note.
            panel.ApplyOwnFeedForTests(
                """
                {"posted":true,"entries":2,"published":true,"coversFeed":true,"current":true,
                 "prefix":"/","contentSet":"feed","signatureNote":"","problems":[]}
                """);
            Assert.Contains("every entry is attributable", panel.OwnFeedAttributionForTests);
        }
    }

    // The button an operator has to be able to press.
    //
    // Reachability and not mere existence: a control that exists, has
    // layout and reports a screen coordinate is still one an operator
    // cannot click if it is clipped with nothing to scroll (AP64). This
    // panel's body IS in a working `ScrollViewer`, so below the fold is one
    // scroll away and reachable — which is the distinction `Reachable`
    // exists to draw, and the one the first version of that predicate got
    // wrong.
    [AvaloniaFact]
    public void The_Publish_Feed_Button_Is_Reachable()
    {
        var (window, panel) = Open();
        using (window as IDisposable)
        {
            var button = panel.GetVisualDescendants()
                .OfType<Button>()
                .FirstOrDefault(b => (b.Content?.ToString() ?? "") == "Publish feed");
            Assert.NotNull(button);
            Assert.True(Reachable.IsClickable(button!),
                "the only control that makes a feed readable is not clickable: "
                + Reachable.DescribeClippers(button!));
        }
    }
}
