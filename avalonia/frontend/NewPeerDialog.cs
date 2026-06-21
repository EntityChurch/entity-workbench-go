using System;
using System.Text.Json;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Media;

namespace EntityAvalonia;

// NewPeerDialog is the modal Window that gathers the bridgeConfig
// fields for a new in-process peer. PHASE-I-MULTI-PEER-PLAN.md §5.2.
//
// Submit calls Bridge.PeerCreate(json) directly and exposes the
// resulting peer handle (0 on failure) via the CreatedPeerHandle
// property. Caller (MainWindow's "+" button) reads it after
// ShowDialog returns and opens the corresponding tab.
//
// Defaults: ephemeral identity, memory storage, no listen. Minimal-
// viable peer. Power users edit any field; common case is "alias only,
// hit Enter."
public sealed class NewPeerDialog : Window
{
    public long CreatedPeerHandle { get; private set; }
    public string CreatedAlias { get; private set; } = "";
    public string? ErrorMessage { get; private set; }

    private readonly TextBox _identity;
    private readonly TextBox _alias;
    private readonly ComboBox _storage;
    private readonly TextBox _storagePath;
    private readonly TextBox _listen;
    private readonly CheckBox _openAccess;
    private readonly TextBlock _errorLabel;

    public NewPeerDialog()
    {
        Title = "New peer";
        // Explicit ClientSize — SizeToContent was the prior fix for
        // the "one line visible" symptom but relies on the WM honoring
        // MinWidth/MinHeight, which Mutter+XWayland doesn't always do.
        // Fixed size with CanResize=true is more predictable across
        // compositors; the content fits comfortably in 560x540 with
        // room for the error label if it shows.
        Width = 560;
        Height = 540;
        MinWidth = 480;
        MinHeight = 420;
        CanResize = true;
        WindowStartupLocation = WindowStartupLocation.CenterOwner;
        Background = new SolidColorBrush(Color.FromRgb(0x2b, 0x2d, 0x33));

        _identity = MakeTextBox(watermark: "(empty = ephemeral keypair)");
        _alias = MakeTextBox(watermark: "alias (defaults to identity or \"self\")");
        _storage = new ComboBox
        {
            ItemsSource = new[] { "memory", "sqlite" },
            SelectedIndex = 0,
            FontSize = 13,
            HorizontalAlignment = HorizontalAlignment.Stretch,
            MinWidth = 200,
        };
        _storagePath = MakeTextBox(watermark: "sqlite path (blank → ~/.entity/peers/{identity}/store.db)");
        _listen = MakeTextBox(watermark: "TCP listen addr (blank = no inbound)");
        _openAccess = new CheckBox
        {
            Content = "open-access (DEV: wildcard caps to connecting peers)",
            FontSize = 12,
            Margin = new Thickness(0, 8, 0, 0),
        };
        _errorLabel = new TextBlock
        {
            Text = "",
            Foreground = Brushes.IndianRed,
            FontSize = 12,
            Margin = new Thickness(0, 8, 0, 0),
            TextWrapping = TextWrapping.Wrap,
            IsVisible = false,
        };

        var createBtn = new Button { Content = "Create peer", IsDefault = true };
        createBtn.Click += (_, _) => DoCreate();
        var cancelBtn = new Button { Content = "Cancel", IsCancel = true, Margin = new Thickness(8, 0, 0, 0) };
        cancelBtn.Click += (_, _) => Close();

        var buttonBar = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            HorizontalAlignment = HorizontalAlignment.Right,
            Margin = new Thickness(0, 16, 0, 0),
        };
        buttonBar.Children.Add(createBtn);
        buttonBar.Children.Add(cancelBtn);

        // Grid with explicit Auto rows — predictable layout, no
        // StackPanel-collapsing-height surprises. Min column width
        // ensures textboxes have room.
        var form = new Grid
        {
            Margin = new Thickness(16),
            RowDefinitions = new RowDefinitions(
                "Auto,Auto,Auto,Auto,Auto,Auto,Auto,Auto,Auto,Auto,Auto,Auto"),
            ColumnDefinitions = new ColumnDefinitions("*"),
            MinWidth = 480,
        };
        var rows = new Control[]
        {
            MakeLabel("identity"),
            _identity,
            MakeLabel("alias"),
            _alias,
            MakeLabel("storage"),
            _storage,
            MakeLabel("storage path"),
            _storagePath,
            MakeLabel("listen"),
            _listen,
            _openAccess,
            _errorLabel,
        };
        for (int i = 0; i < rows.Length; i++)
        {
            Grid.SetRow(rows[i], i);
            form.Children.Add(rows[i]);
        }

        var dock = new DockPanel { LastChildFill = true };
        DockPanel.SetDock(buttonBar, Dock.Bottom);
        dock.Children.Add(buttonBar);
        dock.Children.Add(form);

        Content = dock;
        Opened += (_, _) => _alias.Focus();
    }

    private void DoCreate()
    {
        var cfg = new BridgeConfig
        {
            Identity = _identity.Text?.Trim() ?? "",
            Alias = _alias.Text?.Trim() ?? "",
            Storage = (_storage.SelectedItem as string) ?? "memory",
            StoragePath = _storagePath.Text?.Trim() ?? "",
            Listen = _listen.Text?.Trim() ?? "",
            OpenAccess = _openAccess.IsChecked == true,
        };
        var json = JsonSerializer.Serialize(cfg);
        var reply = Bridge.TakeString(Bridge.PeerCreate(json));

        // Envelope: {ok, handle} or {ok:false, error}.
        try
        {
            using var doc = JsonDocument.Parse(reply);
            var root = doc.RootElement;
            if (root.TryGetProperty("ok", out var ok) && ok.GetBoolean()
                && root.TryGetProperty("handle", out var h))
            {
                CreatedPeerHandle = h.GetInt64();
                CreatedAlias = !string.IsNullOrEmpty(cfg.Alias)
                    ? cfg.Alias
                    : (!string.IsNullOrEmpty(cfg.Identity) ? cfg.Identity : "self");
                Close();
                return;
            }
            if (root.TryGetProperty("error", out var err))
            {
                ErrorMessage = err.GetString();
                ShowError(ErrorMessage ?? "(unknown error)");
                return;
            }
            ShowError($"unexpected reply: {reply}");
        }
        catch (Exception ex)
        {
            ShowError($"reply parse failed: {ex.Message}\n{reply}");
        }
    }

    private void ShowError(string msg)
    {
        _errorLabel.Text = msg;
        _errorLabel.IsVisible = true;
    }

    private static TextBlock MakeLabel(string text) => new()
    {
        Text = text,
        FontSize = 11,
        Opacity = 0.65,
        Margin = new Thickness(0, 6, 0, 0),
    };

    private static TextBox MakeTextBox(string watermark) => new()
    {
        Watermark = watermark,
        FontFamily = new FontFamily("monospace"),
        FontSize = 13,
    };
}
