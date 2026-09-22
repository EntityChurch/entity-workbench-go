namespace EntityAvalonia.Panels;

// IPanelPreferredHeight lets a panel tell PanelStack how much vertical space it
// actually needs. **Every panel implements it.** That is a correction, and the
// paragraph below is kept because the reasoning it records is what made the
// original guidance — "implement it only when the default genuinely does not
// work" — look reasonable while it broke the app.
//
// Why this exists: the compute-program game panels (Snake, Life, Asteroids) draw
// a SQUARE world, so their drawing scales with min(width, height). In a stack of
// three slots the row is wide but short, which meant the square collapsed to the
// slot's leftover height (~135px in a 1280x1024 window) and threw away all the
// width. The board was legible only if the user hand-dragged a splitter. A panel
// that draws a fixed-aspect scene is the first content in this app whose size is
// a real requirement rather than a preference — so it needed a way to say so.
//
// **Why every panel now implements it (2026-09-02).** ProgramPanel was the only
// implementer for six weeks, so every other panel claimed the 200px stack
// default. PanelStack sizes its Grid to max(viewport, sum-of-minimums), so with
// three ordinary panels open the sum was 608px against a ~900px viewport: the
// Grid was pinned to the viewport, each row got ~297px, and panels whose fixed
// chrome alone is taller than that had their content clipped with nothing to
// scroll. Meanwhile the stack's scrollbar is configured Visible +
// AllowAutoHide=false, so an operator saw a scrollbar that could not move and
// correctly concluded the app does not scroll. The only workaround was to close
// panels until one was left.
//
// So the floor is not an optional refinement — a panel that does not declare one
// is a panel that lies about how much room it needs, and the lie is invisible
// until someone opens a second panel. The numbers below are derived from the
// chrome each panel builds, and they are MINIMUMS: over-declaring costs a little
// early scrolling, under-declaring costs reachability. Prefer over.
//
// The declaration is a floor, not a guarantee, so it is paired with the other
// half of the rule: a panel must not put an unbounded list inside a docked
// region. Bound it (MaxHeight) so it scrolls within itself. Then an
// under-estimate degrades to "scroll inside the panel" rather than "the button
// is gone".
//
// Contract:
//   - The value is a MINIMUM, not a fixed height. The slot still star-shares the
//     viewport and the GridSplitter still moves freely above this floor.
//   - PanelStack clamps it to [SlotMinHeight, SlotMaxHeight]. It can never pin a
//     row to zero, so the layout-recursion SIGSEGV mitigation in PanelStack's
//     class doc stays intact.
//   - Exceeding the viewport is fine and intended: the stack's Grid.Height grows
//     to the summed minimums and the outer ScrollViewer engages. The user scrolls
//     rather than squinting.
public interface IPanelPreferredHeight
{
    // The minimum height, in DIPs, this panel needs to be usable.
    double PreferredSlotMinHeight { get; }
}
