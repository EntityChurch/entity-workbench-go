using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using Avalonia.Input;
using Avalonia.Threading;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Regression tests for the two bugs fixed in this session:
//
//   1. SelectableTextBlock in row labels intercepted pointer input
//      and broke single-click selection. Test: simulate a real
//      click at row coordinates, assert EntitySelected fired once
//      with the row's path.
//
//   2. RerenderFromBridge restoring SelectedIndex by path fired
//      SelectionChanged → EntitySelected → cascading panel reloads
//      on every wake. Test: select a row, trigger a wake that
//      doesn't change selection, assert EntitySelected DID NOT fire.
//
// Each test PUTs an entity at a unique path so it has a deterministic
// row to act on. Paths are uniquified per-test so the shared bridge
// fixture (which persists state across tests in this collection)
// doesn't surface flakes from order-dependent state.
[Collection(nameof(BridgeCollection))]
public sealed class TreeViewPanelRegressionTests
{
    private readonly BridgeFixture _bridge;

    public TreeViewPanelRegressionTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public async Task Clicking_Row_Fires_EntitySelected_Exactly_Once()
    {
        const string TestPath = "test_click_target";
        PutDataEntity(TestPath, "click test");

        var panel = new TreeViewPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 600 };
        window.Show();

        // Tree paths are peer-id-prefixed (/<peer-id>/<path>) and the
        // peer-id folder starts collapsed. Use search to surface the
        // entry — matches show regardless of expansion state.
        panel.SetSearchForTests(TestPath);

        var ready = await HeadlessPump.WaitUntil(
            () => FindRowEndingWith(panel, "/" + TestPath) >= 0,
            TimeSpan.FromSeconds(5));
        AssertReady(ready, panel, TestPath);

        var targetIdx = FindRowEndingWith(panel, "/" + TestPath);
        var fullPath = panel.GetRowPathForTests(targetIdx);
        Assert.True(panel.IsEntryForTests(targetIdx),
            $"row at '{fullPath}' should be entity-bearing");

        int fired = 0;
        string? lastPath = null;
        panel.EntitySelected += p => { fired++; lastPath = p; };

        // Materialize containers via layout pass.
        HeadlessPump.Flush();
        var listBox = panel.ListForTests;
        var container = listBox.ContainerFromIndex(targetIdx) as Control;
        Assert.NotNull(container);

        // Translate container center to window coords + click.
        var local = new Point(container!.Bounds.Width / 2, container.Bounds.Height / 2);
        var inWindow = container.TranslatePoint(local, window) ?? local;

        window.MouseMove(inWindow);
        window.MouseDown(inWindow, MouseButton.Left);
        window.MouseUp(inWindow, MouseButton.Left);
        HeadlessPump.Flush();

        Assert.Equal(targetIdx, panel.SelectedIndexForTests);
        Assert.Equal(1, fired);
        Assert.Equal(fullPath, lastPath);
    }

    [AvaloniaFact]
    public async Task TreeWake_DoesNotRefire_EntitySelected_When_Selection_Unchanged()
    {
        const string TestPath = "test_wake_target";
        PutDataEntity(TestPath, "wake test");

        var panel = new TreeViewPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 600 };
        window.Show();

        panel.SetSearchForTests(TestPath);

        var ready = await HeadlessPump.WaitUntil(
            () => FindRowEndingWith(panel, "/" + TestPath) >= 0,
            TimeSpan.FromSeconds(5));
        AssertReady(ready, panel, TestPath);

        var targetIdx = FindRowEndingWith(panel, "/" + TestPath);

        // Establish selection programmatically — Bug 1's test covers
        // the click path; here we only need a selected state to assert
        // Bug 2's wake-driven re-fire is suppressed.
        panel.ListForTests.SelectedIndex = targetIdx;
        HeadlessPump.Flush();

        // Subscribe AFTER selection is established. Any further
        // EntitySelected fire is the bug — RerenderFromBridge's
        // restore-selection path leaking through SelectionChanged.
        int fired = 0;
        panel.EntitySelected += _ => fired++;

        // Trigger a wake without user action by mutating the tree.
        // Adding a sibling entity fires a wake on the same prefix
        // and forces RerenderFromBridge, which is where Bug 2 lived.
        PutDataEntity("test_wake_sibling_" + Guid.NewGuid().ToString("N").Substring(0, 8), "sibling");

        // Pump enough for the wake → render to drain.
        await HeadlessPump.WaitUntil(() => false, TimeSpan.FromMilliseconds(500));

        Assert.Equal(0, fired);
    }

    // --- helpers -----------------------------------------------------

    // PutDataEntity dispatches `put <path> data "<jsonValue>"` against
    // the shared default peer. The DTO reply asserts ok so syntax
    // errors fail loudly rather than silently leaving the tree empty.
    private void PutDataEntity(string path, string jsonValue)
    {
        var jsonArg = JsonSerializer.Serialize(jsonValue);
        // The shell tokenizer expects a quoted string — JsonSerializer
        // already wraps the value in quotes so the literal arg passed
        // into the shell is e.g. `"click test"` (with the quotes).
        var line = $"put {path} data {jsonArg}";
        var replyPtr = Bridge.DispatchLine(_bridge.DefaultPeer, line);
        var reply = Marshal.PtrToStringAnsi(replyPtr) ?? "(null)";
        Bridge.FreeString(replyPtr);
        using var doc = JsonDocument.Parse(reply);
        if (!doc.RootElement.GetProperty("ok").GetBoolean())
        {
            throw new InvalidOperationException(
                $"put failed for line `{line}`: {reply}");
        }
    }

    private static int FindRowByPath(TreeViewPanel panel, string path)
    {
        for (int i = 0; i < panel.RowsCountForTests; i++)
        {
            if (panel.GetRowPathForTests(i) == path) return i;
        }
        return -1;
    }

    // FindRowEndingWith matches by path suffix. Tree paths are
    // peer-id-prefixed (/<peer-id>/<user-path>); tests don't know the
    // peer-id at compile time, so they match on the user-path suffix.
    private static int FindRowEndingWith(TreeViewPanel panel, string suffix)
    {
        for (int i = 0; i < panel.RowsCountForTests; i++)
        {
            if (panel.GetRowPathForTests(i).EndsWith(suffix)) return i;
        }
        return -1;
    }

    // AssertReady throws with a diagnostic dump of all rows when the
    // expected path doesn't appear. Without this, a failure here just
    // says "Assert.True failed" with no clue what the tree actually
    // contains.
    private static void AssertReady(bool ready, TreeViewPanel panel, string expectedPath)
    {
        if (ready) return;
        var rows = new List<string>();
        for (int i = 0; i < panel.RowsCountForTests; i++)
        {
            rows.Add($"  [{i}] hasEntry={panel.IsEntryForTests(i),-5} " +
                     $"path='{panel.GetRowPathForTests(i)}'");
        }
        throw new Xunit.Sdk.XunitException(
            $"expected row with path '{expectedPath}' to appear within timeout.\n" +
            $"actual tree state ({panel.RowsCountForTests} rows):\n" +
            string.Join("\n", rows));
    }
}
