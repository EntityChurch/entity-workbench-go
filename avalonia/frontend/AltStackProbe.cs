using System;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using Avalonia.Platform;
using Avalonia.Rendering.SceneGraph;
using Avalonia.Threading;

namespace EntityAvalonia;

// AltStackProbe carries CrashDiagnostics onto Avalonia's RENDER THREAD.
//
// # Why a control exists to do this
//
// `sigaltstack` is per-thread and must be called ON the thread it
// covers. `CrashDiagnostics.Install()` runs on the UI thread, so until
// 2026-09-02 the render thread kept the PAL's stock 16 KB alternate
// signal stack — a size already measured insufficient for this
// process's signal handler chain (click fuzz: 6/8 seeds crashed at
// 16 KB, 0/8 at 1 MB).
//
// On 2026-09-02 the render thread took a SIGSEGV inside the
// compositor's visual walk and the kernel recorded the consequence in
// one line:
//
//     kernel: signal: entity-avalonia[3746566] overflowed sigaltstack
//
// The handler chain ran off the end of 16 KB, so the process died
// unrecoverably: no minidump, no crash log, and a breadcrumb trail that
// simply stops. Nothing above the signal layer can contain that.
//
// # Why a custom draw operation and not something tidier
//
// There is no public API in Avalonia 11.2 that hands you the render
// thread:
//
//   - `IRenderTimer.Tick` is raised there, and is **internal**.
//   - `Compositor`'s update callbacks run on the **UI** thread.
//   - `AvaloniaLocator.Current` is gone in 11.x.
//
// An `ICustomDrawOperation` is serialized into the render command
// stream and executed by the compositor during the render pass — which
// is not merely *a* background thread but the exact call stack the
// crash was taken in (`ServerCompositionContainerVisual::RenderCore`
// appears in both). So the probe runs where the fault lands, by
// construction rather than by inference.
//
// # Why it keeps asking for frames
//
// A control that is never dirty is never drawn. The probe invalidates
// itself until the install has happened, then stops — otherwise a
// window that renders once and sits idle (which is the shape of the
// session that crashed: 17 seconds of no input before the fault) would
// leave the render thread uncovered forever.
public sealed class AltStackProbe : Control
{
    // Zero-size and hit-test invisible. It participates in the render
    // pass and in nothing else; it must not affect layout, and it must
    // never be a click target.
    public AltStackProbe()
    {
        Width = 0;
        Height = 0;
        IsHitTestVisible = false;
        // AP64: every panel declares its height contribution. This is not
        // a panel, but it IS in a layout, and a control that measures as
        // anything other than zero here would steal space from a docked
        // region for a thing the operator cannot see.
        MinWidth = 0;
        MinHeight = 0;
    }

    protected override Size MeasureOverride(Size availableSize) => new Size(0, 0);

    public override void Render(DrawingContext context)
    {
        base.Render(context);

        if (CrashDiagnostics.RenderThreadAltStackInstalled) return;

        // Bounds must be non-empty or the operation can be culled before
        // it is ever executed — and a probe that is optimized away is
        // indistinguishable from one that ran, which is the failure mode
        // this whole area keeps producing. One pixel, fully transparent.
        context.Custom(new InstallOp(new Rect(0, 0, 1, 1)));

        // Ask for another frame until it has actually happened. Posted
        // rather than called inline: InvalidateVisual during Render is a
        // re-entrant layout mutation.
        Dispatcher.UIThread.Post(() =>
        {
            if (!CrashDiagnostics.RenderThreadAltStackInstalled) InvalidateVisual();
        }, DispatcherPriority.Background);
    }

    private sealed class InstallOp : ICustomDrawOperation
    {
        public InstallOp(Rect bounds) => Bounds = bounds;

        public Rect Bounds { get; }

        // Never a hit-test target: this draws nothing and must not
        // intercept input for the control underneath it.
        public bool HitTest(Point p) => false;

        // Reference equality. Returning true for distinct instances
        // would let the renderer skip a repeat operation, which is the
        // one thing this probe depends on not happening.
        public bool Equals(ICustomDrawOperation? other) => ReferenceEquals(this, other);

        public void Dispose() { }

        // THE RENDER THREAD. Everything here runs on it.
        public void Render(ImmediateDrawingContext context)
        {
            // Draws nothing on purpose. The side effect is the point,
            // and a probe that also painted would make a rendering
            // artifact out of a diagnostic.
            CrashDiagnostics.EnlargeAltStackHere("render thread");
        }
    }
}
