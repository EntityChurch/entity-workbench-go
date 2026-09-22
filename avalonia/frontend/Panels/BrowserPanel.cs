using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Controls.Templates;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// BrowserPanel renders wb.BrowseModel — the consume-side **browser**:
// pin a name authority, see what it carries, go somewhere, and see the
// chain that put the bytes on screen.
//
// # What this is, next to PublisherVerifyPanel
//
// `PublisherVerifyPanel` is the inspector: you already know an origin
// and you want to know whether it is lying. It renders the chain
// INSTEAD of the site, deliberately. That surface answered one question
// and could not answer the three a person actually has — *what is out
// there*, *take me there*, *where am I*.
//
// This panel answers those, and the design problem it exists to solve is
// that **the answer looks the same whether it was verified or not**. A
// page is a page. So the chain is not behind a button and not on another
// tab: it is the right-hand column, always visible, showing the
// provenance of the bytes in the middle column and no others.
//
// # The contrast with entity-browser-rust's Site Browser, made concrete
//
// Theirs renders the pages with trust in the chrome — a banner, a state
// on the window. That is the reader's browser and it is the right shape
// for a reader. Ours puts the ten-step chain beside the page because the
// user we are building for is the person deciding whether to believe it.
//
// Two rules the model enforces and this panel must not undo:
//
//   1. A step that could not be established is shown FAILING, never
//      omitted. A rail that renders six green rows and stops looks green
//      at a glance.
//   2. No green verdict is rendered as the bare word "verified". The
//      freshness line under the rail always names a moment.
//
// Layout:
//
//   +--------------------------------------------------------------+
//   | [<] [>] [ address.......................... ] [Go]           |
//   | registry 2KBLk… @ origin · pinned by hand · as of 2026-…     |
//   +-------------+-----------------------------+------------------+
//   | REGISTRY    | Site title                  | TRUST            |
//   |  docs.…     | breadcrumbs                 |  ok registry pin |
//   |  lab.…      | -------------------------   |  ok association  |
//   |  ! ghost.…  | body                        |  ...             |
//   | SITES       |                             |  freshness bound |
//   |  demo *     |                             |                  |
//   +-------------+-----------------------------+------------------+
//
// Patterns applied (GUIDE-AVALONIA-PANEL-PATTERNS.md):
//
//   P0 breadcrumb    — PanelLog at every bridge call
//   P2 persistent    — every named control created once, never detached
//   P3′ operation-triggered wake — this panel wakes when a navigation or
//        an enumeration FINISHES, not on tree events. The single-flight
//        guard is on the START side and lives in the bridge, per
//        operation: enumerating and navigating do not block each other,
//        two navigations do.
//   P4 bounded body  — per-block split; a long page would otherwise hit
//        Skia's paint recursion (AP8)
//   P6 pinned wake   — explicit GCHandle.Alloc on the wake delegate
//
// No peer handle: a Mode A2 consumer is not a peer (§6.5.3). The
// registry factory passes one and this panel ignores it, visibly.
public sealed class BrowserPanel : UserControl, IDisposable
{
    // A published site's body is authored content and can be long. Same
    // rule as SiteViewPanel: no single SelectableTextBlock over ~500
    // inlines.
    private const int MaxInlinesPerBlock = 500;

    private readonly long _handle;

    private readonly TextBox _addressBox;
    private readonly Button _backButton;
    private readonly Button _forwardButton;
    private readonly Button _goButton;

    private readonly TextBox _registryOriginBox;
    private readonly TextBox _registryPeerBox;
    private readonly TextBox _pinTreeBox;
    private readonly TextBox _pinContentBox;
    private readonly TextBox _pinManifestBox;
    private readonly TextBox _pinLayoutBox;
    private readonly TextBox _targetOriginBox;
    private readonly Button _pinButton;
    private readonly TextBlock _registryLine;
    private readonly TextBlock _chromeLine;

    private readonly ListBox _nameList;
    private readonly ObservableCollection<NameRow> _names = new();
    private readonly TextBlock _namesAuthorityLine;
    private readonly TextBlock _namesNoteLine;

    private readonly ListBox _siteList;
    private readonly ObservableCollection<string> _sites = new();

    private readonly TextBlock _titleLine;
    private readonly TextBlock _crumbLine;
    private readonly StackPanel _bodyStack;
    private readonly TextBlock _errorLine;
    private readonly TextBlock _defaultedLine;
    // _noticeLine reports an action that did NOT change the page — an
    // external link, today. Separate from _errorLine on purpose: an error
    // clears the page, and clicking a link out of the system must not
    // blank the page the user is reading.
    private readonly TextBlock _noticeLine;
    // _bodyNoteLine says what the DISPLAY did to the body — lowered from
    // HTML, capped at MaxDisplayBytes. Never confusable with the page:
    // it sits above it, and the trust chain still reports the full
    // verified byte count.
    private readonly TextBlock _bodyNoteLine;

    private readonly ItemsControl _stepList;
    private readonly ObservableCollection<StepRow> _steps = new();
    private readonly TextBlock _freshnessLine;
    private readonly TextBlock _navigatingLine;

    private Bridge.TreeWakeCallback? _wakeCallback;
    private GCHandle _wakeCallbackHandle;
    private bool _disposed;

    // ---- the anti-churn state (2026-08-31) ---------------------------
    //
    // An operator's report: *"it just jumps around the page … I click a
    // link, it resets and scrolls me up to the top."* Refresh() used to
    // Clear() and rebuild _names, _sites, _steps and the whole body on
    // every wake, whether or not any of them had changed, and
    // MarkNavigating() blanked the trust rail for the entire duration of
    // a navigation. Three columns re-laid out, twice, per click.
    //
    // The fix is not a diffing framework. It is four remembered strings:
    // a list whose signature is unchanged is not touched at all, and an
    // untouched ObservableCollection produces no layout pass and no
    // container recycling (which is also AP46's blast radius, reduced).
    private string _namesSig = "\u0000";
    private string _sitesSig = "\u0000";
    private string _stepsSig = "\u0000";
    private string _bodySig = "\u0000";
    private string _registrySig = "\u0000";

    // _scrollByAddress remembers where the reader was on each page, so
    // Back returns to the paragraph they left rather than to the top.
    // Keyed by the canonical address, which is what history stores.
    private readonly Dictionary<string, double> _scrollByAddress = new();
    private string _shownAddress = "";
    private ScrollViewer? _centerScroll;

    // _assetGeneration invalidates in-flight figure fetches. A figure
    // resolved on a worker lands after the page it belongs to may have
    // been replaced; without this the image would be written into a
    // control that is no longer in the tree (harmless) or, worse, a slow
    // asset from page A would appear on page B.
    private long _assetGeneration;

    // _ops mirrors the bridge's completed-operation counter. A wake can
    // arrive for an operation already drawn (two can complete between two
    // dispatcher ticks), and "the Go button is enabled again" is NOT a
    // completion signal — the goroutine may not have entered the
    // operation yet when the first Render lands.
    private long _ops;

    // Test seam: counts body rebuilds so a headless test can assert the
    // page was re-rendered rather than merely re-fetched.
    public static int BodyRecreateCountForTests;

    // AutoPinOnOpen controls the start-up registry pin. TRUE in the
    // shipped app — that is the whole point of it.
    //
    // It is a static rather than a constructor argument because the
    // suppression a test needs is assembly-wide: the panel is built from
    // a registry factory in production and from a dozen places in the
    // tests, and a parameter would have to be threaded through all of
    // them and would be forgotten in the next one written.
    //
    // Why not the WB_NO_AUTOPIN environment variable, which exists and
    // does the same job: **Go captures its environment once at process
    // start**, so a `Environment.SetEnvironmentVariable` from a test does
    // not reach `os.Getenv` in the bridge at all — on .NET/Unix it does
    // not even reach the native `environ`. The env var is for launching
    // the app; this flag is for in-process control. A test that set the
    // variable and believed it had opted out would still hit the network
    // and would be measuring the public internet.
    public static bool AutoPinOnOpen = true;

    public BrowserPanel(long peerHandle, IPanelHost? host = null)
    {
        _ = peerHandle; // see the class note: a verifying consumer is not a peer.

        var openReply = Bridge.TakeString(Bridge.BrowseOpen());
        _handle = ParseHandle(openReply);

        // Every control is constructed even on the failure path; only
        // Content is replaced. An early return leaving fields null turns
        // a later Refresh into a NullReference on the UI thread, which in
        // this runtime is a process death rather than an exception
        // (MODEL-AVALONIA-RUNTIME, the X11/Skia boundary).
        _addressBox = new TextBox
        {
            Watermark = "entitychurchregistry.org   (a domain, a name, or a peer-id)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
        };
        _addressBox.KeyDown += (_, e) =>
        {
            if (e.Key == Avalonia.Input.Key.Enter) Go();
        };
        _backButton = new Button { Content = "◀", FontSize = 13, IsEnabled = false, Padding = new Thickness(10, 2) };
        _forwardButton = new Button { Content = "▶", FontSize = 13, IsEnabled = false, Padding = new Thickness(10, 2) };
        _goButton = new Button { Content = "Go", FontSize = 13, Padding = new Thickness(14, 2) };
        _backButton.Click += (_, _) => Navigate(Bridge.BrowseBack, "back");
        _forwardButton.Click += (_, _) => Navigate(Bridge.BrowseForward, "forward");
        _goButton.Click += (_, _) => Go();

        _registryOriginBox = new TextBox
        {
            Watermark = "registry origin  (https://host or http://10.89.3.2:8099)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
        };
        _registryPeerBox = new TextBox
        {
            Watermark = "registry peer-id — THE PIN. For an identity-form id the pin IS the key.",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
        };
        _pinTreeBox = MonoBox("pin tree_url_prefix (optional)");
        _pinContentBox = MonoBox("pin content_url_prefix (optional)");
        _pinManifestBox = MonoBox("pin manifest_url_prefix (optional)");
        _pinLayoutBox = MonoBox("pin content_layout, e.g. sharded-2-4 (optional)");
        _targetOriginBox = MonoBox("target origin override (optional)");
        _pinButton = new Button { Content = "Pin registry", FontSize = 12, Padding = new Thickness(12, 3) };
        _pinButton.Click += (_, _) => Pin();

        _registryLine = new TextBlock
        {
            Text = "no registry pinned — a browser must not invent a name authority",
            FontSize = 11,
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 2, 0, 4),
        };

        // The chrome line sits directly under the address bar and is
        // where the reader's eye already is. It carries the pin in one
        // line, because the left column is a place you look and the
        // chrome is a place you see.
        _chromeLine = new TextBlock
        {
            Text = "",
            FontSize = 11,
            FontFamily = new FontFamily("monospace"),
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(2, 0, 0, 6),
        };

        _namesAuthorityLine = new TextBlock
        {
            Text = "",
            FontSize = 11,
            Opacity = 0.7,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 0, 0, 4),
        };
        _namesNoteLine = new TextBlock
        {
            Text = "",
            FontSize = 11,
            Foreground = Brushes.Goldenrod,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 0, 0, 4),
        };
        _nameList = new ListBox
        {
            ItemsSource = _names,
            FontSize = 12,
            MaxHeight = 320,
            ItemTemplate = Rows.Of<NameRow>((row, _) => BuildNameView(row), supportsRecycling: false),
        };
        _nameList.SelectionChanged += (_, _) =>
        {
            if (_nameList.SelectedItem is NameRow row && row.Committed)
            {
                _addressBox.Text = row.Name;
                Go();
            }
        };

        _siteList = new ListBox { ItemsSource = _sites, FontSize = 12, MaxHeight = 160 };
        _siteList.SelectionChanged += (_, _) =>
        {
            if (_siteList.SelectedItem is string site && !string.IsNullOrEmpty(site))
            {
                _addressBox.Text = CurrentHost() + "/" + site;
                Go();
            }
        };

        _titleLine = new TextBlock
        {
            Text = "", FontWeight = FontWeight.Bold, FontSize = 18,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 2),
        };
        _crumbLine = new TextBlock
        {
            Text = "", FontSize = 11, Opacity = 0.65,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 8),
        };
        _errorLine = new TextBlock
        {
            Text = "", FontSize = 13, Foreground = Brushes.IndianRed,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 8),
        };
        _defaultedLine = new TextBlock
        {
            Text = "", FontSize = 11, Opacity = 0.7,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 8, 0, 0),
        };
        _noticeLine = new TextBlock
        {
            Text = "", FontSize = 11, Foreground = Brushes.Goldenrod,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 8),
            IsVisible = false,
        };
        _bodyNoteLine = new TextBlock
        {
            Text = "", FontSize = 11, Opacity = 0.8, Foreground = Brushes.Goldenrod,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 8),
            IsVisible = false,
        };
        _bodyStack = new StackPanel { Orientation = Orientation.Vertical };

        _stepList = new ItemsControl
        {
            ItemsSource = _steps,
            ItemTemplate = Rows.Of<StepRow>((row, _) => BuildStepView(row), supportsRecycling: false),
        };
        _freshnessLine = new TextBlock
        {
            Text = "", FontSize = 11, Opacity = 0.8,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 10, 0, 0),
        };
        // Shown while a navigation is in flight, above a DIMMED rail
        // that still describes the page still on screen. Without this
        // line the dimming would be unexplained, and an unexplained
        // greyed-out trust column is worse than a cleared one.
        _navigatingLine = new TextBlock
        {
            Text = "re-checking… the chain below still describes the page you are looking at",
            FontSize = 10, Opacity = 0.8, Foreground = Brushes.Goldenrod,
            TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 4),
            IsVisible = false,
        };

        if (_handle < 0)
        {
            Content = new TextBlock
            {
                Text = "browser bridge unavailable — " + openReply,
                Foreground = Brushes.IndianRed,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(12),
            };
            return;
        }

        Content = BuildLayout();

        _wakeCallback = OnWakeFromGo;
        // P6: the delegate must be pinned. Go holds this pointer for the
        // life of the handle and a collected delegate is a jump into
        // freed memory, not a managed exception.
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        Bridge.TakeString(Bridge.BrowseRegisterWake(_handle,
            Marshal.GetFunctionPointerForDelegate(_wakeCallback)));
        PanelLog.Write("browse", $"Open h={_handle}");

        // Open on a usable browser rather than an empty one. This is
        // async by construction (it fetches two well-known objects and
        // walks a signed root) and the wake registered above is what
        // brings the result in — so the window paints immediately and a
        // slow or unreachable origin costs a spinner, not a hang.
        //
        // Suppressed by WB_NO_AUTOPIN, and configured by
        // ~/.entity/browser.json — see workbench.LoadBrowseConfig.
        if (AutoPinOnOpen)
        {
            _namesAuthorityLine.Text = "pinning the configured registry…";
            Bridge.TakeString(Bridge.BrowseAutoPin(_handle));
        }
    }

    private static TextBox MonoBox(string watermark) => new TextBox
    {
        Watermark = watermark,
        FontFamily = new FontFamily("monospace"),
        FontSize = 11,
    };

    private Control BuildLayout()
    {
        var bar = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("Auto,Auto,*,Auto"),
            Margin = new Thickness(0, 0, 0, 4),
        };
        Grid.SetColumn(_backButton, 0);
        Grid.SetColumn(_forwardButton, 1);
        Grid.SetColumn(_addressBox, 2);
        Grid.SetColumn(_goButton, 3);
        bar.Children.Add(_backButton);
        bar.Children.Add(_forwardButton);
        bar.Children.Add(_addressBox);
        bar.Children.Add(_goButton);

        var pinPanel = new StackPanel { Orientation = Orientation.Vertical, Spacing = 3 };
        pinPanel.Children.Add(_registryOriginBox);
        pinPanel.Children.Add(_registryPeerBox);
        pinPanel.Children.Add(new TextBlock
        {
            Text = "A registry that serves no transport-profile is CONFORMANT (NETWORK §6.5.4 puts " +
                   "profile distribution out-of-band in v1). Pin its layout below when there is none — " +
                   "and know that a wrong pin and a withholding origin look identical from here.",
            FontSize = 10,
            Opacity = 0.6,
            TextWrapping = TextWrapping.Wrap,
        });
        pinPanel.Children.Add(_pinTreeBox);
        pinPanel.Children.Add(_pinContentBox);
        pinPanel.Children.Add(_pinManifestBox);
        pinPanel.Children.Add(_pinLayoutBox);
        pinPanel.Children.Add(_targetOriginBox);
        pinPanel.Children.Add(_pinButton);

        var left = new StackPanel { Orientation = Orientation.Vertical, Spacing = 2 };
        // ORDER MATTERS, and it was wrong.
        //
        // _registryLine — the identity of the trust root, the one fact
        // supplied out of band and the only thing trusted a priori —
        // used to sit BELOW an expanded seven-field pin form, i.e. below
        // the fold of a 300px column. The operator's report was that the
        // browser had stopped showing which registry it was pinned to and
        // looked like it was "picking this data up out of nowhere". It
        // was on screen; it was under a form nobody needs after start-up.
        //
        // So: identity first, form collapsed. The form is for the
        // uncommon act (re-pinning by hand); the identity is for every
        // moment the panel is open.
        left.Children.Add(SectionHeader("TRUST ROOT — the pin"));
        left.Children.Add(_registryLine);
        left.Children.Add(new Expander
        {
            Header = "Re-pin this browser",
            Content = pinPanel,
            IsExpanded = false,
            FontSize = 12,
            Margin = new Thickness(0, 2, 0, 0),
        });
        left.Children.Add(SectionHeader("NAMES"));
        left.Children.Add(_namesAuthorityLine);
        left.Children.Add(_namesNoteLine);
        left.Children.Add(_nameList);
        left.Children.Add(SectionHeader("SITES HERE"));
        left.Children.Add(new TextBlock
        {
            Text = "every site this publisher's SIGNED ROOT commits to",
            FontSize = 10, Opacity = 0.55, TextWrapping = TextWrapping.Wrap,
        });
        left.Children.Add(_siteList);

        var center = new StackPanel { Orientation = Orientation.Vertical };
        center.Children.Add(_errorLine);
        center.Children.Add(_noticeLine);
        center.Children.Add(_titleLine);
        center.Children.Add(_crumbLine);
        center.Children.Add(_bodyNoteLine);
        center.Children.Add(_bodyStack);
        center.Children.Add(_defaultedLine);

        var right = new StackPanel { Orientation = Orientation.Vertical, Spacing = 2 };
        right.Children.Add(SectionHeader("TRUST — for the bytes on screen"));
        right.Children.Add(new TextBlock
        {
            Text = "Every step below is satisfiable by an origin that is lying, except the walk. " +
                   "That is why this column is not a tick.",
            FontSize = 10, Opacity = 0.55, TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 0, 0, 6),
        });
        right.Children.Add(_navigatingLine);
        right.Children.Add(_stepList);
        right.Children.Add(_freshnessLine);

        var columns = new Grid { ColumnDefinitions = new ColumnDefinitions("300,*,340") };
        var leftScroll = new ScrollViewer { Content = left, Padding = new Thickness(0, 0, 8, 0) };
        _centerScroll = new ScrollViewer { Content = center, Padding = new Thickness(8, 0) };
        var rightScroll = new ScrollViewer { Content = right, Padding = new Thickness(8, 0, 0, 0) };
        Grid.SetColumn(leftScroll, 0);
        Grid.SetColumn(_centerScroll, 1);
        Grid.SetColumn(rightScroll, 2);
        columns.Children.Add(leftScroll);
        columns.Children.Add(_centerScroll);
        columns.Children.Add(rightScroll);

        var root = new DockPanel { Margin = new Thickness(10) };
        DockPanel.SetDock(bar, Dock.Top);
        DockPanel.SetDock(_chromeLine, Dock.Top);
        root.Children.Add(bar);
        root.Children.Add(_chromeLine);
        root.Children.Add(columns);
        return root;
    }

    private static TextBlock SectionHeader(string text) => new TextBlock
    {
        Text = text,
        FontSize = 11,
        FontWeight = FontWeight.SemiBold,
        Opacity = 0.8,
        Margin = new Thickness(0, 10, 0, 2),
    };

    private static Control BuildNameView(NameRow row)
    {
        var stack = new StackPanel { Orientation = Orientation.Vertical };
        var head = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 6 };
        head.Children.Add(new TextBlock
        {
            Text = row.Committed ? "ok" : "!!",
            FontFamily = new FontFamily("monospace"),
            FontSize = 11,
            Foreground = row.Committed ? Brushes.MediumSeaGreen : Brushes.IndianRed,
        });
        head.Children.Add(new TextBlock { Text = row.Name, FontSize = 12 });
        stack.Children.Add(head);
        if (!string.IsNullOrEmpty(row.Note))
        {
            stack.Children.Add(new TextBlock
            {
                Text = row.Note,
                FontSize = 10,
                Opacity = row.Committed ? 0.55 : 0.95,
                Foreground = row.Committed ? null : Brushes.IndianRed,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(18, 0, 0, 2),
            });
        }
        return stack;
    }

    private static Control BuildStepView(StepRow row)
    {
        var stack = new StackPanel { Orientation = Orientation.Vertical, Margin = new Thickness(0, 0, 0, 5) };
        var head = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 6 };
        head.Children.Add(new TextBlock
        {
            Text = row.Mark,
            FontFamily = new FontFamily("monospace"),
            FontSize = 11,
            Foreground = row.MarkBrush,
        });
        head.Children.Add(new TextBlock { Text = row.Name, FontSize = 12, FontWeight = FontWeight.SemiBold });
        stack.Children.Add(head);
        stack.Children.Add(new TextBlock
        {
            Text = row.Detail,
            FontFamily = new FontFamily("monospace"),
            FontSize = 10,
            Opacity = 0.7,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(16, 1, 0, 0),
        });
        if (!string.IsNullOrEmpty(row.Note))
        {
            stack.Children.Add(new TextBlock
            {
                Text = row.Note,
                FontSize = 10,
                Opacity = row.NoteIsError ? 0.95 : 0.6,
                Foreground = row.NoteIsError ? Brushes.IndianRed : null,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(16, 1, 0, 0),
            });
        }
        return stack;
    }

    private string CurrentHost() => _lastHost;
    private string _lastHost = "";

    private void Pin()
    {
        if (_disposed || _handle < 0) return;
        var json = JsonSerializer.Serialize(new PinConfig
        {
            Origin = _registryOriginBox.Text ?? "",
            PeerId = _registryPeerBox.Text ?? "",
            PinTree = _pinTreeBox.Text ?? "",
            PinContent = _pinContentBox.Text ?? "",
            PinManifest = _pinManifestBox.Text ?? "",
            PinLayout = _pinLayoutBox.Text ?? "",
            TargetOrigin = _targetOriginBox.Text ?? "",
        });
        var reply = Bridge.TakeString(Bridge.BrowsePin(_handle, json));
        PanelLog.Write("browse", $"Pin h={_handle} reply={reply}");

        var env = TryDecode(reply);
        if (env is { Ok: false })
        {
            _registryLine.Text = env.Error ?? "pin failed";
            _registryLine.Foreground = Brushes.IndianRed;
            return;
        }
        _registryLine.Foreground = null;
        _registryLine.Text = "pinned — nothing verified yet. A pin is a key, not a claim about an origin.";
        // Enumerating is the operation that actually checks anything.
        Bridge.TakeString(Bridge.BrowseNames(_handle));
        _namesAuthorityLine.Text = "walking the registry's signed root…";
    }

    private void Go() => GoTo(_addressBox.Text ?? "");

    // FollowLink is what a click on a rendered markdown link calls.
    //
    // The href goes to the bridge UNINTERPRETED. `support.md` is not a
    // file, `site:x` is not a URL scheme this panel knows, and
    // `../notes/x.md` is relative to something this panel does not track
    // — all three are resolved by workbench's ClassifyTarget, which is
    // byte-identical with entity-browser-rust by obligation. The one
    // thing done here is the single-flight/disabled check the buttons
    // also do.
    internal void FollowLink(string href)
    {
        if (_disposed || _handle < 0 || string.IsNullOrWhiteSpace(href)) return;
        var reply = Bridge.TakeString(Bridge.BrowseFollow(_handle, href));
        PanelLog.Write("browse", $"Follow h={_handle} href={href} reply={reply}");
        MarkNavigating();
    }

    private void GoTo(string addr)
    {
        if (_disposed || _handle < 0) return;
        if (string.IsNullOrWhiteSpace(addr)) return;
        _addressBox.Text = addr;
        var reply = Bridge.TakeString(Bridge.BrowseGo(_handle, addr));
        PanelLog.Write("browse", $"Go h={_handle} addr={addr} reply={reply}");
        MarkNavigating();
    }

    private void Navigate(Func<long, IntPtr> call, string what)
    {
        if (_disposed || _handle < 0) return;
        var reply = Bridge.TakeString(call(_handle));
        PanelLog.Write("browse", $"{what} h={_handle} reply={reply}");
        MarkNavigating();
    }

    private void MarkNavigating()
    {
        _goButton.IsEnabled = false;
        _errorLine.Text = "";
        _defaultedLine.Text = "";

        // Remember where the reader was, before anything moves.
        SaveScroll();

        // THE RAIL IS NOT CLEARED, and this is a correction to a rule
        // that was applied one step too early.
        //
        // The rule is right: a stale chain beside fresh bytes is the one
        // lie this panel exists to prevent. But the page does not become
        // fresh here — it is swapped in Refresh, atomically with the new
        // chain. Between the click and that moment the reader is still
        // looking at the PREVIOUS page, and the previous chain is exactly
        // what describes it. Clearing the rail here therefore removed a
        // true statement and left a page with no provenance beside it,
        // for the whole duration of a navigation — six to fifteen seconds
        // before the cache landed, and a 340px column collapsing to
        // nothing and back on every click.
        //
        // So the rail stays, dimmed and labelled, and the pair (page,
        // chain) is only ever replaced together.
        _stepList.Opacity = 0.45;
        _freshnessLine.Opacity = 0.45;
        _navigatingLine.IsVisible = true;
    }

    // SaveScroll records the reader's position on the page being left.
    private void SaveScroll()
    {
        if (_centerScroll == null || string.IsNullOrEmpty(_shownAddress)) return;
        _scrollByAddress[_shownAddress] = _centerScroll.Offset.Y;
    }

    // RestoreScroll puts a revisited page back where the reader left it,
    // and a newly-visited one at the top.
    //
    // Posting rather than setting: the new body's controls have not been
    // measured yet when Refresh runs, so the ScrollViewer's Extent is
    // still the old page's and an offset assigned now is clamped to it.
    // Background priority runs after the layout pass that follows.
    private void RestoreScroll(string address)
    {
        if (_centerScroll == null) return;
        var target = _scrollByAddress.TryGetValue(address, out var y) ? y : 0;
        Dispatcher.UIThread.Post(() =>
        {
            if (_disposed || _centerScroll == null) return;
            _centerScroll.Offset = new Vector(_centerScroll.Offset.X, target);
        }, DispatcherPriority.Background);
    }

    // OnWakeFromGo runs on a Go goroutine. Marshal to the UI thread
    // before touching a control — the visual tree is UI-thread-affine and
    // a cross-thread mutation is a crash, not an exception.
    private void OnWakeFromGo(long handle)
    {
        if (_disposed) return;
        Dispatcher.UIThread.Post(Refresh, DispatcherPriority.Background);
    }

    public void Refresh()
    {
        if (_disposed || _handle < 0) return;

        var json = Bridge.TakeString(Bridge.BrowseRender(_handle));
        BrowseEnvelope? env;
        try
        {
            env = JsonSerializer.Deserialize<BrowseEnvelope>(json, JsonOpts);
        }
        catch (Exception ex)
        {
            PanelLog.Write("browse", $"Render decode failed: {ex.Message}");
            _errorLine.Text = "(render decode failed — see the panel log)";
            _goButton.IsEnabled = true;
            return;
        }
        var v = env?.View;
        if (v == null)
        {
            _errorLine.Text = env?.Error ?? "(no view)";
            _goButton.IsEnabled = true;
            return;
        }
        _ops = env!.Ops;

        _goButton.IsEnabled = !v.Running;
        _backButton.IsEnabled = v.CanBack;
        _forwardButton.IsEnabled = v.CanForward;
        _lastHost = v.Host ?? "";

        RefreshRegistryIdentity(v);

        // Every list below is rebuilt ONLY if it changed. See the
        // _namesSig field note: an untouched ObservableCollection costs
        // no layout pass, and re-laying out three columns on every wake
        // is what "it jumps around" was.
        var namesSig = Signature(v.Names);
        if (namesSig != _namesSig)
        {
            _namesSig = namesSig;
            _names.Clear();
            foreach (var row in v.Names ?? new List<NameDto>())
            {
                var note = !string.IsNullOrEmpty(row.Err) ? row.Err
                    : row.Listed ? row.BindingHash ?? ""
                    : "committed by the signed root, omitted from the served menu";
                _names.Add(new NameRow(row.Name ?? "", row.Committed, note));
            }
        }
        _namesAuthorityLine.Text = v.NamesAuthority ?? "";
        _namesNoteLine.Text = v.NamesNote ?? "";
        _namesNoteLine.IsVisible = !string.IsNullOrEmpty(v.NamesNote);

        var sitesSig = string.Join("\u0001", v.Sites ?? new List<string>());
        if (sitesSig != _sitesSig)
        {
            _sitesSig = sitesSig;
            _sites.Clear();
            foreach (var s in v.Sites ?? new List<string>()) _sites.Add(s);
        }

        var stepsSig = Signature(v.Steps);
        if (stepsSig != _stepsSig)
        {
            _stepsSig = stepsSig;
            _steps.Clear();
            foreach (var s in v.Steps ?? new List<StepDto>())
            {
                var (mark, brush) = s.Status switch
                {
                    "ok" => ("ok  ", Brushes.MediumSeaGreen),
                    "failed" => ("FAIL", (IBrush)Brushes.IndianRed),
                    "skipped" => ("skip", Brushes.Gray),
                    _ => ("..  ", (IBrush)Brushes.Gray),
                };
                // A green step shows what it PROVES; a failed or skipped
                // one shows why. Neither is optional — the note is the
                // whole reason this column is not a tick.
                var note = !string.IsNullOrEmpty(s.Err) ? s.Err : s.Proves;
                _steps.Add(new StepRow(mark, brush, s.Name ?? "", s.Detail ?? "",
                    note ?? "", !string.IsNullOrEmpty(s.Err)));
            }
        }
        _freshnessLine.Text = v.Freshness ?? "";

        // The navigation finished, so the rail describes the bytes that
        // are about to be on screen. Un-dim it in the same Refresh that
        // swaps the body — page and chain move together or not at all.
        if (!v.Running)
        {
            _stepList.Opacity = 1.0;
            _freshnessLine.Opacity = 0.8;
            _navigatingLine.IsVisible = false;
        }

        _noticeLine.Text = v.Notice ?? "";
        _noticeLine.IsVisible = !string.IsNullOrEmpty(v.Notice);

        _errorLine.Text = v.Err ?? "";
        if (!string.IsNullOrEmpty(v.Err))
        {
            // A refused navigation clears the page. Leaving the previous
            // one up beside a failed chain is how a user reads unverified
            // bytes as verified ones.
            _titleLine.Text = "";
            _crumbLine.Text = "";
            _bodyNoteLine.IsVisible = false;
            _bodyStack.Children.Clear();
            _bodySig = "\u0000error";
            _shownAddress = "";
            BodyRecreateCountForTests++;
            return;
        }

        // The address bar is only rewritten when it actually differs AND
        // the reader is not typing in it. It used to be assigned on every
        // Refresh, which moved the caret and replaced what the user had
        // typed ("billslab.com") with the canonical form mid-keystroke.
        var address = v.Address ?? "";
        if (!string.IsNullOrEmpty(address) && _addressBox.Text != address && !_addressBox.IsFocused)
        {
            _addressBox.Text = address;
        }

        var content = v.Content;
        _titleLine.Text = content?.SiteTitle ?? "";
        _crumbLine.Text = BuildCrumbs(content);

        var body = v.Body;
        // The body signature includes the address, so navigating between
        // two pages with identical text still counts as a move (and gets
        // its own scroll position).
        var bodySig = address + "\u0001" + (body?.Text ?? "") + "\u0001" + (body?.Note ?? "");
        if (bodySig != _bodySig)
        {
            _bodySig = bodySig;
            SwapBody(content?.PageTitle ?? "", body);
            _shownAddress = address;
            RestoreScroll(address);
        }

        _defaultedLine.Text = v.SiteDefaulted
            ? $"This publisher offers {v.Sites?.Count ?? 0} sites and the address named none, so this " +
              $"is \"{v.Site}\" — first in byte order, not a front door anyone declared."
            : "";
    }

    // RefreshRegistryIdentity draws the pin in both places it belongs,
    // including the two provenance facts the DTO used to drop.
    //
    // # The fields that were on the wire and not on the screen
    //
    // The model computes `RegistryPinFromOrigin` and
    // `RegistryRebasedFrom` precisely so a surface can report them, and
    // AP45 makes saying so an obligation: a pin the ORIGIN nominated
    // (from its unsigned `entity-deployment.json`) is trust-on-first-use,
    // because the origin picked its own trust root. entity-shell renders
    // both. This panel's `View` class did not declare either property, so
    // System.Text.Json silently dropped them and the GUI — the surface an
    // operator actually uses — met none of the obligation.
    //
    // A dropped field in a DTO is invisible in exactly the way a missing
    // one is not: nothing warns, nothing fails, and the sentence simply
    // never appears.
    private void RefreshRegistryIdentity(View v)
    {
        var sig = $"{v.Registry}|{v.RegistryOrigin}|{v.RegistryDiscovered}|{v.RegistryFresh}|" +
                  $"{v.RegistryPinFromOrigin}|{v.RegistryRebasedFrom}";
        if (sig == _registrySig) return;
        _registrySig = sig;

        var (detail, chrome) = RegistryProvenanceText(
            v.Registry, v.RegistryOrigin, v.RegistryDiscovered,
            v.RegistryRebasedFrom, v.RegistryPinFromOrigin, v.RegistryFresh);

        _registryLine.Foreground = null;
        _registryLine.Text = detail;
        _chromeLine.Text = chrome;
        _chromeLine.Foreground =
            v.RegistryPinFromOrigin || string.IsNullOrEmpty(v.Registry) ? Brushes.Goldenrod : null;

        if (string.IsNullOrEmpty(v.Registry)) return;

        // Show the active pin in the re-pin form too, so it reads as the
        // current state rather than as an empty box with a watermark.
        if (!_registryOriginBox.IsFocused) _registryOriginBox.Text = v.RegistryOrigin ?? "";
        if (!_registryPeerBox.IsFocused) _registryPeerBox.Text = v.Registry ?? "";
    }

    // RegistryProvenanceText is a PURE function of the six facts the
    // model reports about the pin, returning (side-column detail, chrome
    // line).
    //
    // # Why it is pure, and separate
    //
    // AP45's obligation is a SENTENCE — "a pin taken from the origin is
    // trust-on-first-use and every surface must say so" — and a sentence
    // that only exists inside a method that also needs a live network, a
    // fixture origin serving a transport-profile, and a mounted window is
    // a sentence with no cheap gate on it. That is how it came to be
    // missing in the first place: the facts crossed the bridge, the DTO
    // dropped them, and no test looked because looking was expensive.
    //
    // Pulled out here it costs one call to assert, with no I/O, so the
    // obligation is checked the way it is stated.
    internal static (string Detail, string Chrome) RegistryProvenanceText(
        string? registry, string? origin, bool discovered,
        string? rebasedFrom, bool pinFromOrigin, string? fresh)
    {
        if (string.IsNullOrEmpty(registry))
        {
            const string none = "no registry pinned — a browser must not invent a name authority";
            return (none, none);
        }

        string layout;
        if (discovered && !string.IsNullOrEmpty(rebasedFrom))
        {
            // THREE provenance states, not two. Collapsing this one into
            // "discovered" tells the operator the origin advertised a
            // layout it did not advertise.
            layout = "layout RE-BASED onto your pin — this origin's profile features " +
                     rebasedFrom + ", and one origin may host several peers. The pinned root's " +
                     "own signature is what checks the substitution was right.";
        }
        else if (discovered)
        {
            layout = "layout discovered from the origin's transport-profile";
        }
        else
        {
            layout = "layout PINNED by hand — a wrong pin and a withholding origin look identical";
        }

        var tofu = pinFromOrigin
            ? "PIN OFFERED BY THE ORIGIN (entity-deployment.json), not by you — trust-on-first-use. " +
              "Everything below verifies under this key, but the origin chose which key that is. " +
              "Re-pin with a peer-id you got somewhere else if you have one."
            : "";

        var asOf = string.IsNullOrEmpty(fresh) ? "not walked yet" : "as of " + fresh;

        var detail = $"{registry}\n@ {origin}\n{layout} · {asOf}" +
                     (tofu == "" ? "" : "\n\n" + tofu);

        // One line where the eye already is. Short, and still honest
        // about TOFU — that is the one word that must not be buried in a
        // side column.
        var shortId = registry!.Length > 12 ? registry[..12] + "…" : registry;
        var chrome = $"pinned to {shortId} @ {origin} · {asOf}" +
                     (pinFromOrigin ? "  ⚠ pin offered by the origin (TOFU)" : "") +
                     (!string.IsNullOrEmpty(rebasedFrom) ? "  · layout re-based" : "");
        return (detail, chrome);
    }

    // ParseViewForTests deserializes a render envelope exactly as
    // Refresh does, so a test can pin WHICH FIELDS SURVIVE the DTO.
    //
    // That is the defect this exists for: RegistryPinFromOrigin and
    // RegistryRebasedFrom were marshalled by the bridge and dropped here
    // because the View class did not declare them. System.Text.Json
    // reports nothing for an undeclared member — no warning, no
    // exception — so the only way to catch it is to assert the value
    // arrives.
    internal static object? ParseViewForTests(string json)
        => JsonSerializer.Deserialize<BrowseEnvelope>(json, JsonOpts)?.View;

    internal static bool ViewPinFromOriginForTests(object? view)
        => view is View v && v.RegistryPinFromOrigin;

    internal static string ViewRebasedFromForTests(object? view)
        => view is View v ? v.RegistryRebasedFrom ?? "" : "";

    internal static bool ViewBodyIsMarkdownForTests(object? view)
        => view is View v && (v.Body?.IsMarkdown ?? false);

    internal static string ViewBodyTextForTests(object? view)
        => view is View v ? v.Body?.Text ?? "" : "";

    private static string Signature(List<NameDto>? rows)
    {
        if (rows == null) return "";
        var sb = new System.Text.StringBuilder();
        foreach (var r in rows) sb.Append(r.Name).Append('\u0002').Append(r.Committed)
            .Append('\u0002').Append(r.Listed).Append('\u0002').Append(r.Err).Append('\u0001');
        return sb.ToString();
    }

    private static string Signature(List<StepDto>? rows)
    {
        if (rows == null) return "";
        var sb = new System.Text.StringBuilder();
        foreach (var r in rows) sb.Append(r.Name).Append('\u0002').Append(r.Status)
            .Append('\u0002').Append(r.Detail).Append('\u0002').Append(r.Err).Append('\u0001');
        return sb.ToString();
    }

    private static string BuildCrumbs(ContentDto? content)
    {
        if (content?.Breadcrumbs == null || content.Breadcrumbs.Count == 0) return "";
        var parts = new List<string>();
        foreach (var c in content.Breadcrumbs) parts.Add(c.Label ?? "");
        return string.Join("  /  ", parts);
    }

    // SwapBody rebuilds the page body under P4's bounded-block rule.
    //
    // It takes the model's BodyView rather than a raw string, and the
    // difference is the fix for two separate defects:
    //
    //  * `IsMarkdown` — `SitePage.format` has always carried "html" for
    //    a pre-rendered document, and this panel could not see the field
    //    because its DTO never declared it. So HTML went into Markdig
    //    and rendered as its own source. HTML is now lowered to text in
    //    Go and arrives here flagged, and running lowered HTML through a
    //    markdown parser would re-interpret its punctuation, so the flag
    //    is obeyed rather than advisory.
    //
    //  * `Note` — the body may have been lowered or capped. Both are
    //    DISPLAY decisions and must never look like publisher decisions,
    //    so whatever happened is stated above the page. The trust chain
    //    keeps reporting the full verified byte count.
    private void SwapBody(string pageTitle, BodyDto? body)
    {
        // Any figure still in flight belongs to the page being replaced.
        _assetGeneration++;

        _bodyStack.Children.Clear();
        if (!string.IsNullOrEmpty(pageTitle))
        {
            _bodyStack.Children.Add(new TextBlock
            {
                Text = pageTitle,
                FontWeight = FontWeight.Bold,
                FontSize = 15,
                Margin = new Thickness(0, 0, 0, 6),
            });
        }

        _bodyNoteLine.Text = body?.Note ?? "";
        _bodyNoteLine.IsVisible = !string.IsNullOrEmpty(body?.Note);

        var text = body?.Text ?? "";
        if (string.IsNullOrEmpty(text))
        {
            _bodyStack.Children.Add(new TextBlock { Text = "(empty page)", Opacity = 0.5 });
            BodyRecreateCountForTests++;
            return;
        }

        if (body is { IsMarkdown: false })
        {
            // Plain text: no parser, monospace, and that is the whole
            // rendering. A format we did not interpret is shown as what
            // it is rather than as what it might be.
            _bodyStack.Children.Add(new SelectableTextBlock
            {
                Text = text,
                FontFamily = new FontFamily("monospace"),
                FontSize = 12.5,
                TextWrapping = TextWrapping.Wrap,
                Opacity = 0.92,
                Padding = new Thickness(2, 2, 12, 4),
            });
            BodyRecreateCountForTests++;
            return;
        }

        var inlines = MarkdownRenderer.BuildInlines(text, FollowLink, RequestAsset);
        int per = 0;
        var block = NewBlock();
        for (int i = 0; i < inlines.Count; i++)
        {
            block.Inlines!.Add(inlines[i]);
            per++;
            if (inlines[i] is LineBreak && per >= MaxInlinesPerBlock)
            {
                _bodyStack.Children.Add(block);
                block = NewBlock();
                per = 0;
            }
        }
        _bodyStack.Children.Add(block);
        BodyRecreateCountForTests++;
    }

    // RequestAsset fills one figure, off the UI thread.
    //
    // # Why a worker and not a synchronous call
    //
    // `Bridge.BrowseAsset` reads through the verifying consumer. After
    // the trie walk the bytes are usually a cache hit and this would
    // return in microseconds — but a miss is an HTTP round trip, and
    // this runs from inside a layout pass while building inlines. A
    // gallery page carries thirteen figures; thirteen serial round trips
    // on the UI thread is a frozen window, which is precisely the class
    // of defect this session started from.
    //
    // # Why the generation check is not optional
    //
    // A fetch that lands after the reader has moved on must not write
    // into the new page. Without the guard, a slow figure from the page
    // you left appears in the page you are reading — with the current
    // page's trust chain beside it. That is the exact lie this panel
    // exists to prevent, arriving by a back door.
    internal void RequestAsset(string reference, Image target)
    {
        if (_disposed || _handle < 0 || string.IsNullOrWhiteSpace(reference)) return;
        var generation = _assetGeneration;
        var handle = _handle;

        System.Threading.Tasks.Task.Run(() =>
        {
            string reply;
            try
            {
                reply = Bridge.TakeString(Bridge.BrowseAsset(handle, reference));
            }
            catch (Exception ex)
            {
                PanelLog.Write("browse", $"asset {reference} failed: {ex.Message}");
                return;
            }

            AssetEnvelope? env;
            try { env = JsonSerializer.Deserialize<AssetEnvelope>(reply, JsonOpts); }
            catch { env = null; }
            if (env is not { Ok: true } || string.IsNullOrEmpty(env.Bytes)) return;

            byte[] raw;
            try { raw = Convert.FromBase64String(env.Bytes!); }
            catch { return; }

            Dispatcher.UIThread.Post(() =>
            {
                if (_disposed || generation != _assetGeneration) return;
                try
                {
                    // Avalonia's Bitmap decodes raster formats only.
                    // **SVG is not among them** and there is no SVG
                    // package referenced by this project, so an SVG asset
                    // is left as its caption rather than crashing a
                    // decoder or drawing a broken-image glyph. billslab's
                    // figure corpus is overwhelmingly PNG with a handful
                    // of SVGs; adding Avalonia.Svg.Skia is the fix and is
                    // a dependency decision, not a bug fix.
                    if ((env.MediaType ?? "").Contains("svg", StringComparison.OrdinalIgnoreCase))
                    {
                        PanelLog.Write("browse", $"asset {reference} is SVG — not decodable here");
                        return;
                    }
                    using var ms = new System.IO.MemoryStream(raw);
                    target.Source = new Avalonia.Media.Imaging.Bitmap(ms);
                }
                catch (Exception ex)
                {
                    // A body that is not a decodable image. The caption
                    // stays; nothing else changes. Logged because "the
                    // publisher committed something that is not an
                    // image" is a real finding about a site.
                    PanelLog.Write("browse", $"asset {reference} did not decode: {ex.Message}");
                }
            }, DispatcherPriority.Background);
        });
    }

    private static SelectableTextBlock NewBlock() => new SelectableTextBlock
    {
        FontSize = 14,
        TextWrapping = TextWrapping.Wrap,
        Opacity = 0.92,
        Padding = new Thickness(2, 2, 12, 4),
    };

    private static long ParseHandle(string reply)
    {
        try
        {
            var env = JsonSerializer.Deserialize<OpenEnvelope>(reply, JsonOpts);
            if (env is { Ok: true }) return env.Handle;
        }
        catch
        {
            // fall through to the -1 sentinel
        }
        return -1;
    }

    private static BrowseEnvelope? TryDecode(string reply)
    {
        try { return JsonSerializer.Deserialize<BrowseEnvelope>(reply, JsonOpts); }
        catch { return null; }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_handle >= 0) Bridge.TakeString(Bridge.BrowseClose(_handle));
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
        _wakeCallback = null;
        PanelLog.Write("browse", $"Close h={_handle}");
    }

    // ---- test hooks (Tier 3) ----------------------------------------
    //
    // Navigation is asynchronous by construction (the bridge moves the
    // network work off the UI thread), so a test needs a way to start
    // one and settle. These poll Render rather than relying on the wake:
    // a headless test has no dispatcher loop pumping Go callbacks.

    public long HandleForTests => _handle;
    public string ErrorTextForTests => _errorLine?.Text ?? "";
    public string TitleTextForTests => _titleLine?.Text ?? "";
    public string FreshnessTextForTests => _freshnessLine?.Text ?? "";
    public string RegistryTextForTests => _registryLine?.Text ?? "";
    public string NamesAuthorityTextForTests => _namesAuthorityLine?.Text ?? "";
    public string NamesNoteTextForTests => _namesNoteLine?.Text ?? "";
    public int StepCountForTests => _steps.Count;
    public int BodyBlockCountForTests => _bodyStack?.Children.Count ?? 0;
    public string AddressTextForTests => _addressBox?.Text ?? "";
    public string ChromeTextForTests => _chromeLine?.Text ?? "";
    public string BodyNoteTextForTests => _bodyNoteLine?.Text ?? "";
    public bool RailIsDimmedForTests => _stepList != null && _stepList.Opacity < 1.0;
    public bool NavigatingNoticeVisibleForTests => _navigatingLine?.IsVisible ?? false;

    // MarkNavigatingForTests exposes the START side of a navigation on
    // its own, which is the only way to observe the window between the
    // click and the completion — the window in which the rail used to be
    // empty beside a page that was still on screen.
    public void MarkNavigatingForTests() => MarkNavigating();

    public IReadOnlyList<string> StepMarksForTests
    {
        get
        {
            var marks = new List<string>();
            foreach (var s in _steps) marks.Add(s.Mark.Trim());
            return marks;
        }
    }

    public IReadOnlyList<string> StepNamesForTests
    {
        get
        {
            var names = new List<string>();
            foreach (var s in _steps) names.Add(s.Name);
            return names;
        }
    }

    public IReadOnlyList<string> StepNotesForTests
    {
        get
        {
            var notes = new List<string>();
            foreach (var s in _steps) notes.Add(s.Note);
            return notes;
        }
    }

    public IReadOnlyList<string> NameRowsForTests
    {
        get
        {
            var rows = new List<string>();
            foreach (var n in _names) rows.Add((n.Committed ? "ok " : "!! ") + n.Name);
            return rows;
        }
    }

    public void PinForTests(string origin, string peerId, string tree, string content,
        string manifest, string layout, int timeoutMs = 8000)
    {
        _registryOriginBox.Text = origin;
        _registryPeerBox.Text = peerId;
        _pinTreeBox.Text = tree;
        _pinContentBox.Text = content;
        _pinManifestBox.Text = manifest;
        _pinLayoutBox.Text = layout;
        var before = _ops;
        Pin(); // Pin is synchronous; the enumeration it kicks off is not.
        Settle(before, timeoutMs);
    }

    public void GoForTests(string address, int timeoutMs = 8000)
    {
        var before = _ops;
        GoTo(address);
        Settle(before, timeoutMs);
    }

    public void BackForTests(int timeoutMs = 8000)
    {
        var before = _ops;
        Navigate(Bridge.BrowseBack, "back");
        Settle(before, timeoutMs);
    }

    // Settle waits for the bridge's completed-op counter to pass `before`.
    //
    // The button-enabled heuristic this replaced was a race in the
    // direction that makes a test pass while measuring nothing: Render
    // sets IsEnabled from `Running`, and `Running` is still false in the
    // window between the C call returning and the goroutine entering the
    // operation — so every assertion downstream ran against an empty view.
    private void Settle(long before, int timeoutMs)
    {
        var deadline = DateTime.UtcNow.AddMilliseconds(timeoutMs);
        while (DateTime.UtcNow < deadline)
        {
            Refresh();
            PumpLayout();
            if (_ops > before) return;
            System.Threading.Thread.Sleep(10);
        }
        Refresh();
        PumpLayout();
    }

    // PumpLayout runs a layout pass between Refreshes, and it is load-
    // bearing rather than cosmetic.
    //
    // `Refresh` clears `_names` and `_steps`. Clearing a list whose
    // containers a virtualizing panel has REALIZED runs the container
    // teardown path — which is where Avalonia hands a row template a
    // null and where AP46 lived. Clearing a list that was never laid out
    // recycles nothing and exercises none of it.
    //
    // So without this call, every test below mounts a Window, calls
    // Show(), populates the name list, clears it, and never once touches
    // the code path that killed the process in front of a user on
    // 2026-08-31. That is exactly what happened: 74/74 green across a
    // month, against a fault that fires on the first real navigation.
    // `Show()` is not a layout pass.
    private void PumpLayout()
    {
        try
        {
            UpdateLayout();
        }
        catch
        {
            // Not attached to a visual root (several tests construct the
            // panel bare). Nothing to realize, nothing to tear down.
        }
    }

    private sealed record StepRow(string Mark, IBrush MarkBrush, string Name, string Detail,
        string Note, bool NoteIsError);

    private sealed record NameRow(string Name, bool Committed, string Note);

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private sealed class PinConfig
    {
        [JsonPropertyName("origin")] public string Origin { get; set; } = "";
        [JsonPropertyName("peer_id")] public string PeerId { get; set; } = "";
        [JsonPropertyName("pin_tree")] public string PinTree { get; set; } = "";
        [JsonPropertyName("pin_content")] public string PinContent { get; set; } = "";
        [JsonPropertyName("pin_manifest")] public string PinManifest { get; set; } = "";
        [JsonPropertyName("pin_layout")] public string PinLayout { get; set; } = "";
        [JsonPropertyName("target_origin")] public string TargetOrigin { get; set; } = "";
    }

    private sealed class OpenEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("handle")] public long Handle { get; set; }
    }

    private sealed class BrowseEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
        [JsonPropertyName("view")] public View? View { get; set; }
        [JsonPropertyName("ops")] public long Ops { get; set; }
    }

    private sealed class View
    {
        [JsonPropertyName("Address")] public string? Address { get; set; }
        [JsonPropertyName("Host")] public string? Host { get; set; }
        [JsonPropertyName("Site")] public string? Site { get; set; }
        [JsonPropertyName("Page")] public string? Page { get; set; }
        [JsonPropertyName("Registry")] public string? Registry { get; set; }
        [JsonPropertyName("RegistryOrigin")] public string? RegistryOrigin { get; set; }
        [JsonPropertyName("RegistryDiscovered")] public bool RegistryDiscovered { get; set; }
        // These two were computed by the model, marshalled by the bridge,
        // and DROPPED here because this class did not declare them — so
        // the GUI met none of AP45's "a surface MUST say so" obligation
        // while entity-shell met all of it. A DTO field that is absent is
        // silently absent: nothing warns, nothing fails, the sentence
        // just never appears. See RefreshRegistryIdentity.
        [JsonPropertyName("RegistryRebasedFrom")] public string? RegistryRebasedFrom { get; set; }
        [JsonPropertyName("RegistryPinFromOrigin")] public bool RegistryPinFromOrigin { get; set; }
        [JsonPropertyName("RegistryFresh")] public string? RegistryFresh { get; set; }
        [JsonPropertyName("Names")] public List<NameDto>? Names { get; set; }
        [JsonPropertyName("NamesAuthority")] public string? NamesAuthority { get; set; }
        [JsonPropertyName("NamesNote")] public string? NamesNote { get; set; }
        [JsonPropertyName("Sites")] public List<string>? Sites { get; set; }
        [JsonPropertyName("SiteDefaulted")] public bool SiteDefaulted { get; set; }
        [JsonPropertyName("Content")] public ContentDto? Content { get; set; }
        // Body is Content's body PREPARED FOR DISPLAY. Content.BodyMarkdown
        // is now blanked by the bridge — see BrowseRender's note; an 8.27 MB
        // page does not cross cgo as a JSON string.
        [JsonPropertyName("Body")] public BodyDto? Body { get; set; }
        [JsonPropertyName("Steps")] public List<StepDto>? Steps { get; set; }
        [JsonPropertyName("Freshness")] public string? Freshness { get; set; }
        [JsonPropertyName("CanBack")] public bool CanBack { get; set; }
        [JsonPropertyName("CanForward")] public bool CanForward { get; set; }
        [JsonPropertyName("Running")] public bool Running { get; set; }
        [JsonPropertyName("Err")] public string? Err { get; set; }
        [JsonPropertyName("Notice")] public string? Notice { get; set; }
    }

    private sealed class NameDto
    {
        [JsonPropertyName("Name")] public string? Name { get; set; }
        [JsonPropertyName("Target")] public string? Target { get; set; }
        [JsonPropertyName("BindingHash")] public string? BindingHash { get; set; }
        [JsonPropertyName("Committed")] public bool Committed { get; set; }
        [JsonPropertyName("Listed")] public bool Listed { get; set; }
        [JsonPropertyName("Err")] public string? Err { get; set; }
    }

    // BodyDto mirrors workbench.BodyView.
    internal sealed class BodyDto
    {
        [JsonPropertyName("Format")] public string? Format { get; set; }
        [JsonPropertyName("Text")] public string? Text { get; set; }
        // IsMarkdown is OBEYED, not consulted. False means the text was
        // already lowered (HTML) or is a format we do not speak, and
        // running either through a markdown parser re-interprets
        // punctuation the publisher did not write as markup.
        [JsonPropertyName("IsMarkdown")] public bool IsMarkdown { get; set; }
        [JsonPropertyName("Truncated")] public bool Truncated { get; set; }
        [JsonPropertyName("FullBytes")] public int FullBytes { get; set; }
        [JsonPropertyName("Note")] public string? Note { get; set; }
    }

    private sealed class AssetEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
        [JsonPropertyName("media_type")] public string? MediaType { get; set; }
        [JsonPropertyName("bytes")] public string? Bytes { get; set; }
    }

    private sealed class ContentDto
    {
        [JsonPropertyName("SiteTitle")] public string? SiteTitle { get; set; }
        [JsonPropertyName("PageTitle")] public string? PageTitle { get; set; }
        [JsonPropertyName("BodyMarkdown")] public string? BodyMarkdown { get; set; }
        [JsonPropertyName("BodyFormat")] public string? BodyFormat { get; set; }
        [JsonPropertyName("Breadcrumbs")] public List<CrumbDto>? Breadcrumbs { get; set; }
    }

    private sealed class CrumbDto
    {
        [JsonPropertyName("Label")] public string? Label { get; set; }
    }

    private sealed class StepDto
    {
        [JsonPropertyName("Name")] public string? Name { get; set; }
        [JsonPropertyName("Status")] public string? Status { get; set; }
        [JsonPropertyName("Detail")] public string? Detail { get; set; }
        [JsonPropertyName("Proves")] public string? Proves { get; set; }
        [JsonPropertyName("Err")] public string? Err { get; set; }
    }
}
