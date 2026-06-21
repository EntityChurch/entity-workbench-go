using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Smoke test for the headless harness. Proves end-to-end:
//   - libbridge.so loads in a test process (P/Invoke works)
//   - TreeOpen → wake callback fires from a Go goroutine
//   - The wake marshals to HeadlessUnitTestSession's dispatcher
//   - RerenderFromBridge runs on the dispatcher and populates _rows
//   - xUnit assertions see post-callback state
//
// If THIS test passes, every other [AvaloniaFact] in this assembly
// can rely on the same pattern. If it doesn't, the cgo thread-locking
// risk the spike flagged is real and we need a different fixture
// scope strategy.
[Collection(nameof(BridgeCollection))]
public sealed class TreeViewPanelSmokeTests
{
    private readonly BridgeFixture _bridge;

    public TreeViewPanelSmokeTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public async Task Panel_Opens_Wakes_And_Renders_At_Least_One_Row()
    {
        var panel = new TreeViewPanel(_bridge.DefaultPeer);

        // Host in a window so layout runs — ListBox.ItemsSource needs a
        // live visual tree to materialize containers. Window size is
        // arbitrary; just enough to make the ListBox non-zero-height.
        var window = new Window
        {
            Content = panel,
            Width = 400,
            Height = 600,
        };
        window.Show();

        // Wake → render pipeline is async: TreeOpen schedules a goroutine
        // wake which posts to the dispatcher. Pump jobs until _rows
        // populates or we time out. ForceRenderTimerTick is the
        // documented escape from issue #15447's render-timing gotcha.
        var deadline = DateTime.UtcNow.AddSeconds(5);
        while (panel.RowsCountForTests == 0 && DateTime.UtcNow < deadline)
        {
            AvaloniaHeadlessPlatform.ForceRenderTimerTick();
            Dispatcher.UIThread.RunJobs();
            await Task.Delay(10);
        }

        Assert.True(
            panel.RowsCountForTests > 0,
            $"expected at least one row from initial TreeRender; got {panel.RowsCountForTests}. " +
            "Most likely: BridgeInit didn't seed any tree entries, OR the wake never marshaled.");
    }
}
