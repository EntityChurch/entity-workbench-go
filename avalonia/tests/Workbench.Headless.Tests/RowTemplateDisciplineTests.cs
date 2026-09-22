using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.IO;
using System.Linq;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Headless.XUnit;
using Avalonia.Media;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-2 gate for AP46 — the row-template null hazard.
//
// # The defect these tests exist for
//
// Avalonia's `FuncDataTemplate<T>` types its lambda parameter as a
// non-nullable `T` and then calls it with `null` while tearing a
// recycled container down. Clearing an ObservableCollection that a
// virtualizing panel has realized is enough to trigger it. Every row
// builder in this frontend dereferenced its parameter on the first
// line, so all nineteen template sites carried the same latent null
// dereference — and because the runtime's policy was to let a UI-thread
// fault terminate, the observable symptom was SIGABRT, not an exception.
//
// # Why there are two tests and not one
//
// `Guard_Survives_Clear_Of_Realized_List` alone would be a test that
// passes for reasons nobody has established. `Raw_FuncDataTemplate_Still_Crashes`
// is the control: it drives the identical sequence through the
// unguarded framework constructor and asserts the fault DOES occur.
// Without it, a future Avalonia release could quietly stop passing null
// and the guarded test would keep passing while measuring nothing.
// If the control ever goes green, the hazard is gone upstream and this
// whole file — and `Rows` — can be reconsidered. Until then it is the
// proof that the probe reaches the mechanism.
//
// `No_Raw_FuncDataTemplate_In_Frontend` is the enforcement point. A
// discipline nothing checks is a comment.
public sealed class RowTemplateDisciplineTests
{
    private sealed record Row(string Name, bool Ok);

    // Builds the row the way every panel in this tree does: dereference
    // the parameter immediately, no null check, warning-clean under
    // <Nullable>enable</Nullable>.
    private static Control BuildRow(Row row) => new TextBlock
    {
        Text = (row.Ok ? "ok " : "!! ") + row.Name,
        Foreground = row.Ok ? Brushes.MediumSeaGreen : Brushes.IndianRed,
    };

    private static (Window, ObservableCollection<Row>) Realize(IDataTemplate template)
    {
        var items = new ObservableCollection<Row>();
        for (int i = 0; i < 40; i++) items.Add(new Row("name-" + i, i % 3 != 0));

        var list = new ListBox { ItemsSource = items, ItemTemplate = template, Height = 300 };
        var window = new Window { Content = list, Width = 400, Height = 300 };
        window.Show();
        // Containers must actually be realized — an unrealized list has
        // nothing to recycle and the teardown path never runs.
        window.UpdateLayout();
        HeadlessPump.Flush();
        return (window, items);
    }

    [AvaloniaFact]
    public void Guard_Survives_Clear_Of_Realized_List()
    {
        var (window, items) = Realize(Rows.Of<Row>((row, _) => BuildRow(row)));

        // This is the exact call that killed the process from
        // BrowserPanel.Refresh: clearing a realized, virtualized list.
        var ex = Record.Exception(() =>
        {
            items.Clear();
            window.UpdateLayout();
            HeadlessPump.Flush();
        });

        Assert.Null(ex);

        // And it must still work afterwards — a guard that leaves the
        // list unusable has only moved the failure.
        items.Add(new Row("after-clear", true));
        window.UpdateLayout();
        HeadlessPump.Flush();
        Assert.Single(items);
    }

    [AvaloniaFact]
    public void Raw_FuncDataTemplate_Still_Crashes()
    {
        // The control. Same sequence, unguarded constructor — the shape
        // every panel shipped until 2026-08-31.
        var (window, items) = Realize(
            new FuncDataTemplate<Row>((row, _) => BuildRow(row), supportsRecycling: false));

        var ex = Record.Exception(() =>
        {
            items.Clear();
            window.UpdateLayout();
            HeadlessPump.Flush();
        });

        Assert.NotNull(ex);
        Assert.IsType<NullReferenceException>(ex);
    }

    [AvaloniaFact]
    public void Guard_Contains_A_Throwing_Row_Builder()
    {
        // The null case is not the only way a row builder dies. A guard
        // that only handled null would still let a bad field kill the
        // process, so the contract is "the builder cannot take the
        // process down", not "null is handled".
        var template = Rows.Of<Row>((row, _) =>
            throw new InvalidOperationException("boom: " + row.Name));

        var ex = Record.Exception(() =>
        {
            var (window, _) = Realize(template);
            window.UpdateLayout();
            HeadlessPump.Flush();
        });

        Assert.Null(ex);
    }

    [Fact]
    public void No_Raw_FuncDataTemplate_In_Frontend()
    {
        var frontend = FindFrontendDir();
        var offenders = new List<string>();

        foreach (var file in Directory.EnumerateFiles(frontend, "*.cs", SearchOption.AllDirectories))
        {
            // Rows.cs holds the single sanctioned construction.
            if (Path.GetFileName(file) == "Rows.cs") continue;

            var lines = File.ReadAllLines(file);
            for (int i = 0; i < lines.Length; i++)
            {
                var line = lines[i];
                if (line.TrimStart().StartsWith("//")) continue;
                if (line.Contains("new FuncDataTemplate"))
                {
                    offenders.Add($"{Path.GetFileName(file)}:{i + 1}: {line.Trim()}");
                }
            }
        }

        // Collect ALL of them, then report. Failing on the first would
        // make the count a lower bound (AP15).
        Assert.True(offenders.Count == 0,
            "Row templates must be built with Rows.Of<T>(...), which guards the null the " +
            "framework passes during container teardown (AP46). Raw constructions found:\n  " +
            string.Join("\n  ", offenders));
    }

    [Fact]
    public void Every_Panel_Row_Template_Goes_Through_The_Guard()
    {
        // Anti-vacuity for the test above: if the frontend stopped using
        // row templates entirely, No_Raw_FuncDataTemplate would pass
        // while proving nothing.
        var frontend = FindFrontendDir();
        var sites = Directory
            .EnumerateFiles(frontend, "*.cs", SearchOption.AllDirectories)
            .Where(f => Path.GetFileName(f) != "Rows.cs")
            .SelectMany(File.ReadAllLines)
            .Count(l => l.Contains("Rows.Of<"));

        Assert.True(sites >= 15,
            $"expected the frontend's row templates to go through Rows.Of; found {sites}. " +
            "If templates were legitimately removed, lower this floor deliberately.");
    }

    private static string FindFrontendDir()
    {
        var candidates = new[]
        {
            Path.Combine("..", "..", "..", "..", "..", "frontend"),
            Path.Combine("/src", "entity-workbench-go", "avalonia", "frontend"),
        };
        foreach (var c in candidates)
        {
            if (Directory.Exists(c) && File.Exists(Path.Combine(c, "MainWindow.cs")))
                return Path.GetFullPath(c);
        }
        // A source-scanning gate that skips when it cannot find the
        // source has stopped measuring and not mentioned it.
        throw new DirectoryNotFoundException(
            "frontend/ not found from " + Directory.GetCurrentDirectory() +
            " — this gate cannot silently pass.");
    }
}
