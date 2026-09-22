using System.Linq;
using System.Text;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Presenters;
using Avalonia.Controls.Primitives;
using Avalonia.VisualTree;

namespace EntityAvalonia.Tests;

// Reachable — can an operator actually press this control?
//
// # Why this is one file and not a copy per test class
//
// It was three copies (`SharingStatusPanelTests`, `SharingStatusDeliveryTests`,
// `SharePanelRenderTests`), byte-identical, sharing one blind spot that took a
// real red suite to find. A subtle predicate duplicated three ways is the same
// hazard this repository keeps recording about diagnostic SENTENCES: the moment
// one copy is improved the others quietly disagree, and nothing says so.
//
// # The distinction it exists to draw, and why the first version got it wrong
//
// AP64's defect was a panel whose content did not fit its slot **with nothing
// to scroll** — the stack's always-visible scrollbar sat inert while every
// panel was clipped, and the only workaround an operator found was closing
// panels until one was left. "Is this control inside every clipping ancestor's
// bounds" was written as the test for it, and for that defect it is right.
//
// ⛔ **It is wrong as a general reachability test, and on 2026-09-16 it failed
// in the direction that costs a session.** `SharingStatusPanel` puts its whole
// body in a working `ScrollViewer` precisely so an under-estimated height floor
// *degrades to scrolling rather than to an unreachable button*. Add one line
// per folder row, populate the shared fixture peer with real folders — as the
// full suite does and a filtered run does not — and buttons fall below that
// viewport. They are **one scroll away**, which is the designed behaviour, and
// the predicate called them unclickable.
//
// # The rule, and the part that is not obvious
//
// Below the fold of a container the operator can SCROLL is reachable; clipped
// by a container that cannot scroll is not. *Nothing to scroll* is the whole
// difference, and it is the phrase AP64 turns on.
//
// ⭐ **Walking every ancestor is the wrong shape for that rule**, and the first
// attempt did exactly that and still failed. A control inside an unscrolled
// viewport has a layout position that is only true WHILE it is unscrolled, so
// measuring it against ancestors ABOVE the viewport measures a position the
// operator is about to change. Measured: a button 560px down a 600px stack in a
// 100px `ScrollViewer` correctly passed both scroll containers and was then
// rejected by the `Window` — `y=894` against `h=768`.
//
// So when the subject leaves a scrollable viewport, the question becomes *is
// the VIEWPORT reachable* and the subject is substituted. That is also what
// keeps AP64 caught at the right level: if a panel's own ScrollViewer is
// clipped out of its slot, scrolling it reveals nothing, and the substituted
// check fails exactly as it should.
//
// # What it still does NOT claim
//
// Nothing about hit-testing, z-order, opacity or an overlay sitting on top —
// this is layout only. A control can satisfy this and still be covered. For
// input that genuinely dispatches, drive `MouseDown`/`MouseUp` at hit-tested
// coordinates (`ProgramPanelInputTests` is the worked example), or go to
// `smoke-xvfb-click` for the platform below Avalonia.
internal static class Reachable
{
    // IsClickable is layout reachability: non-zero size, and not clipped away
    // by anything the operator cannot scroll past.
    public static bool IsClickable(Control control) =>
        control.Bounds.Width > 0
        && control.Bounds.Height > 0
        && IsWithinAllClippingAncestors(control);

    // IsWithinAllClippingAncestors is the layout half alone, kept separate so
    // a caller that has already asserted on size gets a failure message about
    // the right thing.
    public static bool IsWithinAllClippingAncestors(Control control) =>
        Reaches(control, depth: 0);

    // depth bounds the substitution. Nested scroll containers are legitimate
    // and rare; an unbounded walk here would be a recursion driven by a visual
    // tree a test did not build.
    private static bool Reaches(Control control, int depth)
    {
        if (depth > 8) return false;

        foreach (var a in control.GetVisualAncestors().OfType<Control>())
        {
            if (!a.ClipToBounds) continue;
            var inA = control.TranslatePoint(new Point(0, 0), a);
            if (!inA.HasValue) return false;

            var above = inA.Value.Y < -0.5;
            var below = inA.Value.Y + control.Bounds.Height > a.Bounds.Height + 0.5;
            if (!above && !below) continue;

            // Outside this ancestor. If the ancestor is a viewport with
            // somewhere to scroll to, the control is reachable within it and
            // the remaining question is whether the VIEWPORT is reachable —
            // every ancestor above it is measuring an unscrolled position.
            var sv = ScrollOwnerOf(a);
            if (sv == null) return false;
            return Reaches(sv, depth + 1);
        }
        return true;
    }

    // ScrollOwnerOf returns the scroll container THIS clipping ancestor is the
    // viewport of, when it currently has more content than viewport.
    //
    // **Deliberately narrow.** It matches only a `ScrollViewer` or its
    // `ScrollContentPresenter`, never "some `ScrollViewer` exists further up" —
    // a `Border` with `ClipToBounds` inside a scroll container clips its child
    // for its own reasons, and scrolling the outer container reveals nothing.
    // Widening it to any ancestor would make the predicate one that almost
    // never fails, which is worse than the blind spot it replaced: a gate that
    // cannot fire is a monument.
    //
    // The extent check carries the same weight. A `ScrollViewer` whose content
    // already fits has nothing to scroll, so a control outside its bounds is
    // outside for some other reason and is genuinely unreachable — AP64 exactly.
    private static ScrollViewer? ScrollOwnerOf(Control clipper)
    {
        var sv = clipper as ScrollViewer;
        if (sv == null && clipper is ScrollContentPresenter)
        {
            sv = clipper.GetVisualAncestors().OfType<ScrollViewer>().FirstOrDefault();
        }
        if (sv == null) return null;
        return sv.Extent.Height > sv.Viewport.Height + 0.5 ? sv : null;
    }

    // DescribeClippers is for failure messages: which ancestors clip, where the
    // subject sits inside each, and what any scroll container reports. A
    // reachability failure with no chain in it sends the reader to the panel,
    // and the panel is usually fine — which is how this predicate's own defect
    // stayed hidden through three copies.
    public static string DescribeClippers(Control control)
    {
        var sb = new StringBuilder();
        foreach (var a in control.GetVisualAncestors().OfType<Control>())
        {
            var inA = control.TranslatePoint(new Point(0, 0), a);
            sb.Append($"[{a.GetType().Name} clip={a.ClipToBounds} h={a.Bounds.Height:0.#} "
                + $"y={(inA.HasValue ? inA.Value.Y : double.NaN):0.#}");
            var sv = a as ScrollViewer
                ?? (a is ScrollContentPresenter
                    ? a.GetVisualAncestors().OfType<ScrollViewer>().FirstOrDefault()
                    : null);
            if (sv != null)
            {
                sb.Append($" ext={sv.Extent.Height:0.#} vp={sv.Viewport.Height:0.#}"
                    + $" scrollable={sv.Extent.Height > sv.Viewport.Height + 0.5}");
            }
            sb.Append("] ");
        }
        return sb.ToString();
    }
}
