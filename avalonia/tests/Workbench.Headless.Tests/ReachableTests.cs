using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Headless.XUnit;
using Avalonia.Layout;
using Xunit;

namespace EntityAvalonia.Tests;

// ReachableTests — the gate on the gate.
//
// `Reachable` is the predicate three panel suites use to decide whether a
// button an operator needs is actually pressable. It was widened on
// 2026-09-16 to stop reporting *below the fold of a working ScrollViewer* as
// unreachable, and a widened predicate is one step from a predicate that
// never fails. **A gate whose success condition cannot occur is not a gate,
// it is a monument** — so both arms are here, and the NEGATIVE one is the
// one that matters.
//
// Layout only; see `Reachable`'s header for what it does not claim.
public class ReachableTests
{
    private static (Window w, Button b) Clipped(bool scrollable)
    {
        // A button pushed well below its container's bounds. The container
        // clips, so the button has a position and owns no visible pixel.
        var tall = new StackPanel { Height = 600 };
        tall.Children.Add(new Border { Height = 560 });
        var button = new Button { Content = "press me", Height = 24 };
        tall.Children.Add(button);

        Control host;
        if (scrollable)
        {
            // A real ScrollViewer with more content than viewport: the
            // operator scrolls and the button is there.
            host = new ScrollViewer
            {
                Height = 100,
                Content = tall,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            };
        }
        else
        {
            // AP64's shape: a fixed, clipping container with NOTHING to
            // scroll. The button is in the tree and cannot be reached by any
            // gesture at all.
            host = new Border { Height = 100, ClipToBounds = true, Child = tall };
        }

        var window = new Window { Width = 400, Height = 200, Content = host };
        window.Show();
        window.UpdateLayout();
        return (window, button);
    }

    // ⛔ The arm that keeps this honest. Clipped, nothing to scroll, no
    // gesture reaches it — which is precisely the defect an operator hit
    // before AP64 was written, and the reason they had to close panels until
    // one was left.
    [AvaloniaFact]
    public void A_Control_Clipped_With_Nothing_To_Scroll_Is_Not_Reachable()
    {
        var (window, button) = Clipped(scrollable: false);
        try
        {
            Assert.False(Reachable.IsClickable(button),
                "a button clipped by a fixed container with nothing to scroll reported as "
                + "clickable — the predicate three panel suites rely on can no longer fail, "
                + "and AP64 would ship again with every test green");
        }
        finally { window.Close(); }
    }

    // The arm that was wrong before 2026-09-16. Same button, same offset, one
    // difference: the container scrolls. `SharingStatusPanel` puts its whole
    // body in one of these on purpose, so that an under-estimated height floor
    // degrades to scrolling instead of to an unreachable control — and the old
    // predicate called that failure.
    [AvaloniaFact]
    public void A_Control_Below_The_Fold_Of_A_Scrollable_Container_Is_Reachable()
    {
        var (window, button) = Clipped(scrollable: true);
        try
        {
            Assert.True(Reachable.IsClickable(button),
                "a button below the fold of a working ScrollViewer reported as unreachable. "
                + "It is one scroll away, which is the designed degradation, and calling it "
                + "a defect turns a correct panel red for adding a line of honest text"
                + "\nDIAG: " + Reachable.DescribeClippers(button));
        }
        finally { window.Close(); }
    }

    // Zero-size is not reachable however scrollable its ancestors are:
    // `IsClickable` folds in the size check so a caller cannot satisfy it
    // with a control that occupies no pixel.
    [AvaloniaFact]
    public void A_Zero_Size_Control_Is_Not_Reachable()
    {
        var button = new Button { Content = "", Width = 0, Height = 0 };
        var window = new Window
        {
            Width = 400,
            Height = 200,
            Content = new ScrollViewer { Content = button },
        };
        window.Show();
        window.UpdateLayout();
        try
        {
            Assert.False(Reachable.IsClickable(button),
                "a zero-size control reported as clickable — present is not shipped");
        }
        finally { window.Close(); }
    }
}
