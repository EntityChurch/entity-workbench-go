using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Input;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// ShellPanel is the per-panel dispatch surface. PHASE-I-DESKTOP-RENDERER
// PLAN §I.5 line: "Each Panel owns its own per-panel Shell instance
// (per shellcmd.NewShellInWorkspace — the workspace is per-process,
// shells are per-panel)."
//
// Each ShellPanel owns:
//
//   - Its own shellcmd.Shell handle (via Bridge.ShellOpen). Closed on
//     Dispose.
//   - Its own command history (Up/Down arrow cycling).
//   - Its own Tab-completion cycle state.
//   - Its own prompt label tracking the shell's WD.
//
// Workspace-level state (connections, aliases, identity) is shared
// across all shells belonging to the same peer; peer-status changes
// propagate via the host's RequestPeerStatusRefresh hook.
//
// Patterns applied:
//   P0 breadcrumb — PanelLog at every long-running op
//   P2 persistent containers — input + prompt + scrollback constructed
//      once; never re-parented
//   P4 bounded scrollback — defensive cap on _scrollbackRows so a
//      command flood can't blow inline-recursion limits (AP8)
public sealed class ShellPanel : UserControl, IDisposable
{
    private readonly long _peerHandle;
    private readonly IPanelHost _host;
    private readonly long _handle;

    private readonly TextBlock _promptLabel = null!;
    private readonly TextBox _dispatchInput = null!;
    private readonly ListBox _scrollback = null!;
    private readonly ObservableCollection<DispatchLineVm> _scrollbackRows = new();
    private readonly List<string> _history = new();
    private int _historyIdx;

    // Tab-completion cycle state.
    private List<string>? _completionCycle;
    private int _completionIndex;
    private string _completionPrefix = "";

    // P4: hard cap on scrollback rows so a runaway `ls` (e.g. a tree
    // with tens of thousands of entries dumped at once) can't exhaust
    // memory or stall layout. Older rows drop off the top.
    private const int MaxScrollbackRows = 5000;

    private bool _disposed;

    // Test surface.
    internal int ScrollbackRowCountForTests => _scrollbackRows.Count;
    internal long HandleForTests => _handle;
    internal void SubmitLineForTests(string line)
    {
        if (_dispatchInput == null) return;
        _dispatchInput.Text = line;
        SubmitLine();
    }

    public ShellPanel(long peerHandle, IPanelHost host)
    {
        _peerHandle = peerHandle;
        _host = host;
        PanelLog.Write("shell", $"Mount peerHandle={peerHandle}");

        var openReply = Bridge.TakeString(Bridge.ShellOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"shell open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            PanelLog.Write("shell", $"Mount open-failed reply={openReply}");
            return;
        }

        _promptLabel = new TextBlock
        {
            Text = Bridge.TakeString(Bridge.ShellPromptForHandle(_handle)),
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Opacity = 0.7,
            VerticalAlignment = VerticalAlignment.Center,
            Margin = new Thickness(0, 0, 6, 0),
        };

        _dispatchInput = new TextBox
        {
            Watermark = "command — try `ls`, `put demo/a test/x \"hi\"`, `peer ls`",
            FontFamily = new FontFamily("monospace"),
            FontSize = 14,
            VerticalAlignment = VerticalAlignment.Center,
        };
        _dispatchInput.KeyDown += OnDispatchInputKeyDown;

        _scrollback = new ListBox
        {
            ItemsSource = _scrollbackRows,
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = new FuncDataTemplate<DispatchLineVm>((vm, _) =>
                new SelectableTextBlock
                {
                    Text = vm.Text,
                    Foreground = ColorForKind(vm.Kind),
                    TextWrapping = TextWrapping.NoWrap,
                }, supportsRecycling: true),
        };

        var inputRow = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("Auto,*"),
            Margin = new Thickness(0, 6, 0, 0),
        };
        Grid.SetColumn(_promptLabel, 0);
        Grid.SetColumn(_dispatchInput, 1);
        inputRow.Children.Add(_promptLabel);
        inputRow.Children.Add(_dispatchInput);

        var scrollbackScroll = new ScrollViewer
        {
            Content = _scrollback,
            HorizontalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
            VerticalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
        };

        var root = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(8),
        };
        DockPanel.SetDock(inputRow, Dock.Bottom);
        root.Children.Add(inputRow);
        root.Children.Add(scrollbackScroll);
        Content = root;
    }

    public void FocusInput() => _dispatchInput?.Focus();

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("shell", $"Dispose h={_handle}");
        if (_handle >= 0)
        {
            Bridge.ShellClose(_handle);
        }
    }

    private void OnDispatchInputKeyDown(object? sender, KeyEventArgs e)
    {
        if (e.Key != Key.Tab)
        {
            _completionCycle = null;
        }
        if (e.Key == Key.Enter)
        {
            SubmitLine();
            e.Handled = true;
            return;
        }
        if (e.Key == Key.Tab)
        {
            DoComplete();
            e.Handled = true;
            return;
        }
        if (e.Key == Key.Up)
        {
            if (_history.Count == 0) return;
            if (_historyIdx > 0) _historyIdx--;
            _dispatchInput.Text = _history[_historyIdx];
            _dispatchInput.CaretIndex = _dispatchInput.Text?.Length ?? 0;
            e.Handled = true;
            return;
        }
        if (e.Key == Key.Down)
        {
            if (_history.Count == 0) return;
            if (_historyIdx < _history.Count - 1)
            {
                _historyIdx++;
                _dispatchInput.Text = _history[_historyIdx];
            }
            else
            {
                _historyIdx = _history.Count;
                _dispatchInput.Text = "";
            }
            _dispatchInput.CaretIndex = _dispatchInput.Text?.Length ?? 0;
            e.Handled = true;
        }
    }

    private void SubmitLine()
    {
        if (_handle < 0) return;
        var line = (_dispatchInput.Text ?? "").Trim();
        if (line.Length > 0)
        {
            _history.Add(line);
            _historyIdx = _history.Count;
        }
        PanelLog.Write("shell", $"Dispatch h={_handle} line={line}");
        var reply = Bridge.TakeString(Bridge.ShellDispatchLine(_handle, line));
        DispatchRespDto? dto = null;
        try
        {
            dto = JsonSerializer.Deserialize<DispatchRespDto>(reply);
        }
        catch { }

        if (dto?.Lines != null)
        {
            if (dto.Lines.Count == 1 && dto.Lines[0].Text == "(clear)")
            {
                _scrollbackRows.Clear();
            }
            else
            {
                foreach (var l in dto.Lines)
                {
                    _scrollbackRows.Add(new DispatchLineVm(l.Text, l.Kind));
                }
                EnforceScrollbackCap();
            }
        }
        else
        {
            _scrollbackRows.Add(new DispatchLineVm($"<unparseable reply> {reply}", "error"));
            EnforceScrollbackCap();
        }

        if (!string.IsNullOrEmpty(dto?.Prompt))
        {
            _promptLabel.Text = dto.Prompt;
        }
        _dispatchInput.Text = "";

        // connect/disconnect/cd may have mutated peer-status. The host
        // owns the status bar; ask it to refresh. Hosts that don't
        // surface peer-status just no-op.
        _host.RequestPeerStatusRefresh();

        Dispatcher.UIThread.Post(() =>
        {
            if (_scrollbackRows.Count > 0)
            {
                _scrollback.ScrollIntoView(_scrollbackRows[_scrollbackRows.Count - 1]);
            }
        }, DispatcherPriority.Background);
    }

    private void EnforceScrollbackCap()
    {
        // P4 (a): cap with drop-oldest. The user's most-recent output
        // is what's visible / what they care about; drop the top.
        while (_scrollbackRows.Count > MaxScrollbackRows)
        {
            _scrollbackRows.RemoveAt(0);
        }
    }

    private void DoComplete()
    {
        if (_handle < 0) return;
        if (_completionCycle != null && _completionCycle.Count > 0)
        {
            _completionIndex = (_completionIndex + 1) % _completionCycle.Count;
            ApplyCycleCandidate();
            return;
        }

        var line = _dispatchInput.Text ?? "";
        var reply = Bridge.TakeString(Bridge.ShellComplete(_handle, line));
        CompleteRespDto? dto = null;
        try
        {
            dto = JsonSerializer.Deserialize<CompleteRespDto>(reply);
        }
        catch { }
        if (dto?.Candidates == null || dto.Candidates.Count == 0) return;

        _completionPrefix = line.Substring(0, dto.TokenStart);

        if (dto.Candidates.Count == 1)
        {
            _dispatchInput.Text = _completionPrefix + dto.Candidates[0] + " ";
            _dispatchInput.CaretIndex = _dispatchInput.Text.Length;
            _completionCycle = null;
            return;
        }

        _scrollbackRows.Add(new DispatchLineVm("  " + string.Join("  ", dto.Candidates), "null"));
        EnforceScrollbackCap();
        Dispatcher.UIThread.Post(() =>
        {
            if (_scrollbackRows.Count > 0)
            {
                _scrollback.ScrollIntoView(_scrollbackRows[_scrollbackRows.Count - 1]);
            }
        }, DispatcherPriority.Background);
        _completionCycle = dto.Candidates;
        _completionIndex = 0;
        ApplyCycleCandidate();
    }

    private void ApplyCycleCandidate()
    {
        if (_completionCycle == null || _completionCycle.Count == 0) return;
        _dispatchInput.Text = _completionPrefix + _completionCycle[_completionIndex];
        _dispatchInput.CaretIndex = _dispatchInput.Text.Length;
    }

    private static IBrush ColorForKind(string kind) => kind switch
    {
        "path" => new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff)),
        "hash" => new SolidColorBrush(Color.FromRgb(0xc8, 0xa8, 0xff)),
        "key" => new SolidColorBrush(Color.FromRgb(0xff, 0xd6, 0x7a)),
        "string" => new SolidColorBrush(Color.FromRgb(0xb5, 0xe8, 0x99)),
        "number" => new SolidColorBrush(Color.FromRgb(0xff, 0xb3, 0x86)),
        "error" => Brushes.IndianRed,
        "null" => new SolidColorBrush(Color.FromArgb(0xaa, 0xff, 0xff, 0xff)),
        _ => new SolidColorBrush(Color.FromArgb(0xdd, 0xff, 0xff, 0xff)),
    };

    private static long ParseHandle(string envelope)
    {
        try
        {
            using var doc = JsonDocument.Parse(envelope);
            var root = doc.RootElement;
            if (root.TryGetProperty("ok", out var ok) && ok.GetBoolean()
                && root.TryGetProperty("handle", out var h))
            {
                return h.GetInt64();
            }
        }
        catch { }
        return -1;
    }

    // --- DTOs --------------------------------------------------------

    private sealed class DispatchRespDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("lines")] public List<DispatchLineDto>? Lines { get; set; }
        [JsonPropertyName("prompt")] public string Prompt { get; set; } = "";
        [JsonPropertyName("error")] public string Error { get; set; } = "";
    }

    private sealed class DispatchLineDto
    {
        [JsonPropertyName("text")] public string Text { get; set; } = "";
        [JsonPropertyName("kind")] public string Kind { get; set; } = "";
    }

    private sealed class DispatchLineVm
    {
        public string Text { get; }
        public string Kind { get; }
        public DispatchLineVm(string text, string kind) { Text = text; Kind = kind; }
    }

    private sealed class CompleteRespDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("candidates")] public List<string>? Candidates { get; set; }
        [JsonPropertyName("tokenStart")] public int TokenStart { get; set; }
    }
}
