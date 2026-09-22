using System;
using System.IO;
using System.Runtime.CompilerServices;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Media;

namespace EntityAvalonia.Panels;

// Rows — the ONE way this frontend builds a list/content row.
//
// # Why this type exists (AP46)
//
// Avalonia's `FuncDataTemplate<T>` declares its lambda parameter as a
// NON-NULLABLE `T`. It calls that lambda with `null` anyway, during
// container teardown: clearing an ObservableCollection that a
// virtualizing panel has realized runs
//
//   ObservableCollection.Clear
//     -> VirtualizingStackPanel.RecycleElementOnItemRemoved
//     -> ItemsControl.ClearContainerForItemOverride
//     -> ValueStore.ClearValue          (the container's ContentTemplate)
//     -> TemplateBinding.PublishValue
//     -> ContentPresenter.ContentChanged
//     -> FuncDataTemplate.Build(data: null)
//     -> YOUR LAMBDA, with row == null
//
// So `<Nullable>enable</Nullable>` is on, the code is warning-clean,
// and the very first field access in the row builder is a null
// dereference. The type system was told a lie by the framework and
// cannot help; **every** row-template site in the tree had the
// identical latent fault, all nineteen of them, and none of them was
// detectable by reading the row builder.
//
// That is what makes this a factory and not a null check. A null check
// is a fix for one row builder. The failure is a property of the
// FRAMEWORK CALL, so the guard belongs at the framework call — once —
// and the enforcement point is `RowTemplateDisciplineTests`, which
// fails the build if a raw `FuncDataTemplate` construction reappears
// anywhere in the frontend outside this file.
//
// # What the guard does, and what it deliberately does not do
//
//   * `row == null` -> no visual. This is teardown, not an error; it is
//     not logged, because logging it would fire on every list refresh.
//   * the builder throws -> a red, selectable one-line row naming the
//     exception and the CALL SITE (captured by [CallerFilePath], so a
//     report says `BrowserPanel.cs:215` without a stack trace).
//
// It does NOT swallow the fault silently. A row that failed to build is
// rendered as a visibly broken row, so a screenshot shows it and the
// breadcrumb ring carries it. The alternative we had until 2026-08-31 —
// let it reach `Dispatcher.UnhandledException`, which by policy did not
// set `Handled` — turned a cosmetic row-build failure into SIGABRT and
// an 872 MB core dump, three times in one session. See
// `CrashDiagnostics.InstallDispatcher` for the other half of that fix.
//
// # Scope
//
// `where T : class` covers every row type in this frontend (records and
// classes; `string` and `object` included). A struct row type would not
// have the null hazard at all, but a data template over a struct has
// its own unboxing pitfalls — if one is ever needed, give it its own
// factory rather than relaxing this constraint.
public static class Rows
{
    // Of builds a data template whose row builder cannot take the
    // process down.
    //
    // The signature is deliberately shaped like the framework
    // constructor it replaces — same two-argument lambda, same
    // `supportsRecycling` — so migrating a call site was a one-token
    // edit and no row builder had to change.
    public static FuncDataTemplate<T> Of<T>(
        Func<T, INameScope, Control?> build,
        bool supportsRecycling = false,
        [CallerFilePath] string callerFile = "",
        [CallerLineNumber] int callerLine = 0)
        where T : class
    {
        var site = Site(callerFile, callerLine);
        // The single sanctioned construction in the frontend. The
        // discipline test allows this line and only this line.
        return new FuncDataTemplate<T>(
            (row, scope) => Guarded(row, scope, build, site),
            supportsRecycling);
    }

    // Guarded is the whole point of this file.
    //
    // `row` is typed `T?` here and `T` at the framework boundary. That
    // disagreement is not sloppiness — it is the bug, written down.
    private static Control? Guarded<T>(
        T? row, INameScope scope, Func<T, INameScope, Control?> build, string site)
        where T : class
    {
        // Teardown. Avalonia is clearing a recycled container and asked
        // us to render nothing; rendering nothing is the correct answer.
        if (row is null) return null;

        try
        {
            return build(row, scope);
        }
        catch (Exception ex)
        {
            // Breadcrumb first: PanelLog records unconditionally, so this
            // survives into a crash dump even with WB_PANEL_LOG unset.
            SafeLog(site, typeof(T).Name, ex);
            return FailedRow(site, ex);
        }
    }

    // FailedRow must not itself throw — it runs on the exception path of
    // the render pipeline, and a second fault here would be unguarded.
    // Everything it touches is a literal or a static brush.
    private static Control FailedRow(string site, Exception ex)
    {
        try
        {
            return new SelectableTextBlock
            {
                Text = "row failed to render — " + ex.GetType().Name + " at " + site,
                Foreground = Brushes.IndianRed,
                FontSize = 11,
                TextWrapping = TextWrapping.Wrap,
            };
        }
        catch
        {
            return new Border();
        }
    }

    private static void SafeLog(string site, string rowType, Exception ex)
    {
        try
        {
            PanelLog.Write("row",
                $"template {site} threw building {rowType}: {ex.GetType().Name}: {ex.Message}");
        }
        catch
        {
            // A logger that throws must not escalate a contained row
            // fault into an uncontained one.
        }
    }

    private static string Site(string file, int line)
    {
        try
        {
            return (string.IsNullOrEmpty(file) ? "?" : Path.GetFileName(file)) + ":" + line;
        }
        catch
        {
            return "?:" + line;
        }
    }
}
