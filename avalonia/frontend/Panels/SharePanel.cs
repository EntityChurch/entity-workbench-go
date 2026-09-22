using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// SharePanel is the folder-sharing flow as one surface: discover a peer,
// offer them a mounted folder, see what they are offering you, accept it,
// and see what is arriving.
//
// **Why it is one panel and not five.** The five stages are not
// independently useful, and three of the four facts that make the flow
// work are invisible from any single stage:
//
//   * A sync is MUTUAL authorization. Both peers need a policy entry
//     naming the other. Grant one direction and you get an accepted
//     subscription and an empty folder — which looks exactly like a
//     working share until someone opens the folder.
//   * The grant set is assembled at HANDSHAKE, so a policy written on a
//     live connection is inert until the connection is re-established.
//     Share and Accept both reconnect and both report whether they
//     managed to; this panel renders that, because "shared" and "shared
//     and in force" are different states.
//   * A dial-by-address authorizes the DIALER ONLY. So after you accept,
//     the publisher must dial YOU once, or their deliveries are refused
//     and nothing arrives — with no error on this side. That instruction
//     is the loudest thing on the panel after an accept, because it is
//     the one step neither side's verb can perform for you.
//
// Splitting these across panels would put the operator in the position of
// knowing the rules in order to sequence the buttons. The panel sequences
// them instead: the sections are numbered in the order you perform them.
//
// **No flow logic lives here.** Every button is one `Bridge.Share*` call,
// which is one `ShellWorkspace` method, which is the same method the
// shell verb calls (shellcmd/share_op.go, sync_op.go). This is the
// "renderers are thin I/O" rule — and specifically the AP57 fix, which
// says to extract the operation so the verb and the panel share it rather
// than reimplementing it in the renderer. The operations were already
// extracted; until now nothing in the GUI called them.
//
// **The two network calls run off the UI thread.** `ShareOffers` and
// `ShareAccept` reconnect and dispatch to a remote peer, so they can take
// seconds. They are synchronous cgo exports (see Bridge.cs for why) and
// are wrapped in Task.Run here, the same shape PeerConnectionsPanel uses
// for its dial.
public sealed class SharePanel : UserControl, IPanelPreferredHeight, IDisposable
{
    // Chrome floor: this peer's identity block + four numbered sections,
    // each with a control row and a bounded list. Taller than any other
    // panel because it is a whole workflow rather than one view; the
    // stack scrolls to it (see IPanelPreferredHeight).
    public double PreferredSlotMinHeight => 720;

    // Lists are NOT height-capped. They were, at 130px, and that cap ate
    // the "Complete connection" button: a share row is a title, a path, a
    // per-peer line, a button row and an address line — about 145px with
    // ListBoxItem padding — so the buttons sat below the fold of their
    // own container, inside a list too short to scroll usefully. The
    // operator's report was that the button the other panel told them to
    // press did not exist. It existed and was clipped, which is worse,
    // because there is nothing to look for.
    //
    // The cap was there to stop one long list pushing the next section
    // off the panel. That job belongs to the panel's own ScrollViewer,
    // which this panel has: content grows, the panel scrolls, everything
    // stays reachable. A fixed cap solves the wrong half of the problem
    // and creates an unreachable control to do it.

    private readonly long _peerHandle;

    private readonly SelectableTextBlock _identityLine;
    private readonly TextBlock _dialHint;

    private readonly ComboBox _peerPicker;
    private readonly ComboBox _mountPicker;
    private readonly TextBox _titleBox;

    private readonly Button _shareBtn;
    private readonly Button _offersBtn;
    private readonly SelectableTextBlock _status;

    private readonly ObservableCollection<PeerVm> _peers = new();
    private readonly ObservableCollection<MountVm> _mounts = new();
    private readonly ObservableCollection<ShareVm> _shares = new();
    private readonly ObservableCollection<OfferVm> _offers = new();
    private readonly ObservableCollection<SyncVm> _syncs = new();

    private readonly TextBlock _peersEmpty;
    private readonly TextBlock _sharesEmpty;
    private readonly TextBlock _offersEmpty;
    private readonly TextBlock _syncsEmpty;

    private string _localPeerId = "";

    // The address the operator supplies for a peer we cannot otherwise
    // reach, asked for at SHARE time rather than three screens later.
    private readonly TextBox _shareAddrBox;
    private readonly TextBlock _shareAddrHint;

    // Connections wake + the auto-dial bookkeeping behind it.
    //
    // This is what removes the third manual step. When the receiver
    // accepts, their Accept re-establishes their connection to us, so we
    // observe a connection change at exactly the moment their delivery
    // grant becomes real — which is the one moment our reciprocal dial
    // needs to happen and the one thing neither verb could time.
    //
    // Bounded to ONCE PER PEER for the panel's lifetime, and only for
    // peers in our OWN share audience. Both bounds matter: a dial is a
    // disconnect + reconnect, and two panels that each auto-dial on the
    // other's connection change would ping-pong. A peer we are not
    // sharing with is never dialled from here.
    private long _connsHandle = -1;
    private ConnWake? _connWakeCallback;
    private GCHandle _connWakeHandle;
    private readonly HashSet<string> _autoDialled = new();
    private bool _disposed;

    private delegate void ConnWake(long handle);

    // Test surface: auto-dial is off by default under test so a suite
    // does not dial anything, and the set is inspectable so the
    // once-per-peer bound can be asserted rather than assumed.
    internal static bool AutoDialOnConnect = true;
    internal int AutoDialledCountForTests => _autoDialled.Count;

    // SeedShareForTests puts a share into the panel without needing a
    // second live peer, so the RENDERING of a share row can be gated.
    //
    // It seeds the view-model collection the real ItemTemplate reads, so
    // the row is built by BuildShareRow exactly as it is in the app —
    // which is the code that mattered: the buttons were present, correct,
    // and clipped inside a height-capped list, and every existing test
    // passed because they all asserted on data rather than on whether a
    // control ended up somewhere a person could click.
    internal void SeedShareForTests(string root, string title, string prefix,
        string peerId, string alias, string address)
    {
        _shares.Add(new ShareVm(root, title, prefix,
            new List<string> { peerId },
            new List<AudienceVm> { new(peerId, alias, true, address, "connection-table") }));
        _sharesEmpty.IsVisible = false;
    }

    // Test/driver surface. Headless tests set the pickers and call the
    // Perform* methods rather than synthesizing clicks — the click route
    // itself is covered by the real-input harness, and AP32 warns against
    // settling on a derived UI property as a completion signal.
    public int PeerCount => _peers.Count;
    public int MountCount => _mounts.Count;
    public int ShareCount => _shares.Count;
    public int OfferCount => _offers.Count;
    public int SyncCount => _syncs.Count;
    public string StatusText => _status.Text ?? "";
    public string IdentityText => _identityLine.Text ?? "";
    public string SelectedPeerId
    {
        get => (_peerPicker.SelectedItem as PeerVm)?.PeerId ?? "";
        set
        {
            foreach (var p in _peers)
            {
                if (p.PeerId != value) continue;
                _peerPicker.SelectedItem = p;
                return;
            }
        }
    }
    public string SelectedMountRoot
    {
        get => (_mountPicker.SelectedItem as MountVm)?.Root ?? "";
        set
        {
            foreach (var m in _mounts)
            {
                if (m.Root != value) continue;
                _mountPicker.SelectedItem = m;
                return;
            }
        }
    }
    public string TitleText { get => _titleBox.Text ?? ""; set => _titleBox.Text = value; }

    public SharePanel(long peerHandle)
    {
        _peerHandle = peerHandle;

        _identityLine = new SelectableTextBlock
        {
            FontFamily = new FontFamily("monospace"),
            FontSize = 11,
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
        };
        // Selectable on purpose: the peer-id and the listen address are
        // the two things the operator has to get onto the OTHER machine,
        // and a value you cannot copy is a value you retype wrong.
        _dialHint = new TextBlock
        {
            FontSize = 11,
            Opacity = 0.55,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 2, 0, 0),
        };

        _peerPicker = new ComboBox
        {
            ItemsSource = _peers,
            MinWidth = 260,
            FontSize = 12,
            ItemTemplate = Rows.Of<PeerVm>((vm, _) => new TextBlock
            {
                Text = vm.Display,
                FontFamily = new FontFamily("monospace"),
                FontSize = 12,
            }),
        };
        _peerPicker.SelectionChanged += (_, _) => UpdateDialHint();

        _mountPicker = new ComboBox
        {
            ItemsSource = _mounts,
            MinWidth = 200,
            FontSize = 12,
            ItemTemplate = Rows.Of<MountVm>((vm, _) => new TextBlock
            {
                Text = vm.Display,
                FontFamily = new FontFamily("monospace"),
                FontSize = 12,
            }),
        };

        _titleBox = new TextBox
        {
            Watermark = "title (optional — defaults to the folder name)",
            FontSize = 12,
            MinWidth = 220,
        };

        // Asked here, at share time, because this is where the operator
        // has already chosen the peer — and because the sharing side is
        // precisely the side that usually cannot work the address out.
        // The receiver dialled us, so our connection entry for them is
        // their ephemeral source port (V7 §6.7.1: MUST NOT be treated as
        // dialable). Discovering that at the END of the flow, on the
        // other machine, is what made this feel like three disconnected
        // chores.
        _shareAddrBox = new TextBox
        {
            Watermark = "their host:port",
            FontSize = 12,
            MinWidth = 160,
            IsVisible = false,
        };
        _shareAddrHint = new TextBlock
        {
            FontSize = 11,
            FontStyle = FontStyle.Italic,
            Opacity = 0.6,
            TextWrapping = TextWrapping.Wrap,
            IsVisible = false,
            Margin = new Thickness(0, 0, 0, 4),
        };

        _shareBtn = new Button { Content = "Share folder", FontSize = 12 };
        ToolTip.SetTip(_shareBtn,
            "Authorize this peer to read the folder, publish the offer, and re-establish "
            + "the connection so the grant is actually in force.");
        _shareBtn.Click += (_, _) => _ = PerformShareAsync();

        _offersBtn = new Button { Content = "Check their offers", FontSize = 12 };
        ToolTip.SetTip(_offersBtn,
            "Ask the selected peer what they are sharing with you. Reaches the network.");
        _offersBtn.Click += (_, _) => _ = LoadOffersAsync();

        _status = new SelectableTextBlock
        {
            Text = "",
            FontSize = 12,
            TextWrapping = TextWrapping.Wrap,
            Foreground = Brushes.Gainsboro,
            Margin = new Thickness(12, 6, 12, 10),
        };

        _peersEmpty = Hint("No peers yet. Connect to one in Peer Connections, or launch both "
            + "peers with --listen so they announce on the LAN.");
        _sharesEmpty = Hint("Not sharing anything yet.");
        _offersEmpty = Hint("Pick a peer and press \"Check their offers\".");
        _syncsEmpty = Hint("Not receiving any folder yet.");

        var body = new StackPanel { Orientation = Orientation.Vertical };
        body.Children.Add(BuildIdentitySection());
        body.Children.Add(BuildShareOutSection());
        body.Children.Add(BuildOffersSection());
        body.Children.Add(BuildReceivingSection());

        // Status docked to the BOTTOM and the body in a fill ScrollViewer:
        // the operator's last action is the thing they are reading, and a
        // status line that scrolls away with the body is a status line
        // they have to go looking for. This also makes the panel
        // internally scrollable regardless of whether the declared floor
        // above is generous enough — an under-estimate degrades to
        // scrolling rather than to an unreachable button.
        var root = new DockPanel { LastChildFill = true };
        DockPanel.SetDock(_status, Dock.Bottom);
        root.Children.Add(_status);
        root.Children.Add(new ScrollViewer
        {
            Content = body,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
            VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
        });
        Content = root;

        Refresh();
        OpenConnectionsWake();
    }

    // --- Sections ---------------------------------------------------------

    private Control BuildIdentitySection()
    {
        var stack = Section("This peer");
        stack.Children.Add(_identityLine);
        stack.Children.Add(_dialHint);
        return stack;
    }

    private Control BuildShareOutSection()
    {
        var stack = Section("1 · Share a folder with a peer");

        var row = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 2, 0, 4),
        };
        row.Children.Add(Label("folder"));
        row.Children.Add(_mountPicker);
        row.Children.Add(Label("with"));
        row.Children.Add(_peerPicker);
        stack.Children.Add(row);

        stack.Children.Add(_shareAddrHint);

        var row2 = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 0, 0, 6),
        };
        row2.Children.Add(_titleBox);
        row2.Children.Add(_shareAddrBox);
        row2.Children.Add(_shareBtn);
        row2.Children.Add(RowButton("Refresh", "Re-read peers, mounts, shares and syncs", Refresh));
        stack.Children.Add(row2);

        stack.Children.Add(_peersEmpty);
        stack.Children.Add(SubHeader("Currently shared out"));
        stack.Children.Add(_sharesEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _shares,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = Rows.Of<ShareVm>((vm, _) => BuildShareRow(vm)),
        });
        return stack;
    }

    private Control BuildOffersSection()
    {
        var stack = Section("2 · What they are offering you");
        var row = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 2, 0, 4),
        };
        row.Children.Add(_offersBtn);
        stack.Children.Add(row);
        stack.Children.Add(_offersEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _offers,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = Rows.Of<OfferVm>((vm, _) => BuildOfferRow(vm)),
        });
        return stack;
    }

    private Control BuildReceivingSection()
    {
        var stack = Section("3 · Folders you are receiving");
        stack.Children.Add(_syncsEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _syncs,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = Rows.Of<SyncVm>((vm, _) => BuildSyncRow(vm)),
        });
        return stack;
    }

    // --- Rows -------------------------------------------------------------

    private Control BuildShareRow(ShareVm vm)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 4, 4, 4), Spacing = 1 };
        stack.Children.Add(new TextBlock
        {
            Text = vm.Title,
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
        });
        stack.Children.Add(Line($"{vm.Root}  →  {vm.TargetPrefix}", Brushes.Gainsboro));

        // One block per peer in the audience, because the reciprocal dial
        // is per peer and so is withdrawing. A single "shared with A, B"
        // line plus one button set cannot express either.
        foreach (var who in vm.AudienceState)
        {
            stack.Children.Add(BuildAudienceBlock(vm.Root, who));
        }
        // Audience with no state: a share record naming peers we have no
        // information about. Rendered rather than dropped.
        if (vm.AudienceState.Count == 0 && vm.Audience.Count > 0)
            stack.Children.Add(Line(vm.AudienceLine, Brushes.DarkGray));
        return stack;
    }

    // BuildAudienceBlock renders one peer we shared with, and the state
    // of the step that decides whether the share does anything.
    //
    // The dial cannot be done by the peer who needs it: over a
    // dial-by-address connection the kernel's reciprocal grant is gated
    // on EstablishedViaRendezvousKey(), so the receiver's accept buys
    // them the right to subscribe and gives us nothing, while delivery
    // runs us→them. The first version of this panel handled that by
    // PRINTING A SHELL COMMAND for the operator to run on the other
    // machine, which is not a surface — it is the shell with extra steps,
    // and it was rightly called that. The button below is the same
    // operation performed here.
    private Control BuildAudienceBlock(string root, AudienceVm who)
    {
        var block = new StackPanel
        {
            Orientation = Orientation.Vertical,
            Spacing = 2,
            Margin = new Thickness(0, 4, 0, 0),
        };

        block.Children.Add(Line(
            $"shared with {who.Label}"
            + (who.Connected ? "  ·  connected" : "  ·  not connected"),
            Brushes.DarkGray));

        var verbs = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 6 };

        var peerId = who.PeerId;
        // The address box only appears when nothing is known — the case
        // that is NOT an error, just an unknown. When an address is known
        // the button uses it and the operator types nothing.
        var addrBox = new TextBox
        {
            Watermark = "their host:port",
            FontSize = 11,
            MinWidth = 160,
            IsVisible = !who.HasAddress,
        };

        var completeBtn = new Button
        {
            Content = "Complete connection",
            FontSize = 11,
            Padding = new Thickness(8, 2),
        };
        ToolTip.SetTip(completeBtn, who.HasAddress
            ? $"Dial {who.Address} so this peer's grant is in force and deliveries are accepted. "
              + $"Address from: {who.AddressSource}."
            : "We do not know where this peer listens. Enter their address once — it is remembered.");
        completeBtn.Click += (_, _) => _ = PerformCompleteAsync(peerId, addrBox);

        verbs.Children.Add(completeBtn);
        verbs.Children.Add(addrBox);
        verbs.Children.Add(RowButton(
            "Stop sharing",
            "Remove this peer's grant and drop them from the offer's audience.",
            () => PerformUnshare(root, peerId)));
        block.Children.Add(verbs);

        if (who.HasAddress)
            block.Children.Add(Line($"    will dial {who.Address}  ({who.AddressSource})", Brushes.DimGray));

        return block;
    }

    private Control BuildOfferRow(OfferVm vm)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 4, 4, 4), Spacing = 1 };
        stack.Children.Add(new TextBlock
        {
            Text = vm.Title,
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
        });
        stack.Children.Add(Line($"folder \"{vm.Root}\" · they publish it at {vm.TargetPrefix}",
            Brushes.Gainsboro));
        // An offer is a LABEL, not an authority — APP-CONVENTION-SHARE §2.2
        // forbids inferring authorization from it. Saying so here is the
        // difference between "Accept failed" reading as a bug and reading
        // as the two things being correctly separate.
        stack.Children.Add(Line(
            "an offer is a label, not permission — Accept is what asks for the bytes",
            Brushes.DimGray));

        var root = vm.Root;
        var verbs = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 3, 0, 0),
        };
        verbs.Children.Add(RowButton("Accept",
            "Authorize their deliveries, reconnect, and start receiving into your local mount "
            + "of the same name. You need that mount first.",
            () => _ = PerformAcceptAsync(root)));
        stack.Children.Add(verbs);
        return stack;
    }

    private Control BuildSyncRow(SyncVm vm)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 4, 4, 4), Spacing = 1 };
        stack.Children.Add(new TextBlock
        {
            Text = vm.Root,
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
        });
        stack.Children.Add(Line($"from {vm.PeerLabel}  →  {vm.TargetPrefix}", Brushes.Gainsboro));
        // "restored" is not "broken". SyncRow.Live means this process holds
        // the subscription handle; a sync restored from disk at startup is
        // routed by the kernel and reads as false. Its own doc comment says
        // a surface must not call that dead, so the word here is "restored".
        stack.Children.Add(Line(
            vm.Live ? "live in this session" : "restored at startup — routed by the kernel",
            Brushes.DarkGray));

        var peer = vm.PeerId;
        var root = vm.Root;
        var verbs = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 3, 0, 0),
        };
        verbs.Children.Add(RowButton("Stop receiving",
            "Cancel the subscription. The mount and everything already received stay.",
            () => PerformUnsync(peer, root)));
        stack.Children.Add(verbs);
        return stack;
    }

    // --- Operations -------------------------------------------------------

    // Refresh re-reads everything local: peers, mounts, shares, syncs.
    // Public so a headless test drives it without a wake it does not have.
    public void Refresh()
    {
        var reply = Bridge.TakeString(Bridge.ShareRender(_peerHandle));
        var dto = Decode<RenderEnvelope>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            SetStatus($"share render failed: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                Brushes.IndianRed);
            return;
        }

        _localPeerId = dto.LocalPeerId;
        var listen = string.IsNullOrEmpty(dto.AdvertisedUrl)
            ? (string.IsNullOrEmpty(dto.ListenAddr) ? "" : dto.ListenAddr)
            : dto.AdvertisedUrl;
        _identityLine.Text =
            $"{(string.IsNullOrEmpty(dto.LocalAlias) ? "this peer" : dto.LocalAlias)}   {dto.LocalPeerId}\n"
            + (string.IsNullOrEmpty(listen)
                // Stated, not omitted: with no listener nobody can dial back,
                // and the receiving half of a share is exactly a dial back.
                // An operator who cannot see this concludes the share is
                // broken rather than unreachable.
                ? "no listener — relaunch with --listen 0.0.0.0:PORT for another machine to reach you"
                : $"reachable at {listen}");

        var keepPeer = SelectedPeerId;
        var keepMount = SelectedMountRoot;

        _peers.Clear();
        foreach (var p in dto.Peers ?? new List<PeerDto>())
            _peers.Add(new PeerVm(p.PeerId, p.Alias, p.Address, p.Connected, p.Source,
                p.DialAddress, p.DialAddressSource));
        _peersEmpty.IsVisible = _peers.Count == 0;

        _mounts.Clear();
        foreach (var m in dto.Mounts ?? new List<MountDto>())
            _mounts.Add(new MountVm(m.Root, m.TargetPrefix, m.SharedWith));

        _shares.Clear();
        foreach (var s in dto.Shares ?? new List<OfferDto>())
        {
            var state = new List<AudienceVm>();
            foreach (var a in s.AudienceState ?? new List<AudienceDto>())
                state.Add(new AudienceVm(a.PeerId, a.Alias, a.Connected, a.Address, a.AddressSource));
            _shares.Add(new ShareVm(
                s.Root, s.Title, s.TargetPrefix, s.Audience ?? new List<string>(), state));
        }
        _sharesEmpty.IsVisible = _shares.Count == 0;

        _syncs.Clear();
        foreach (var s in dto.Syncs ?? new List<SyncDto>())
            _syncs.Add(new SyncVm(s.RemotePeerId, s.RemoteAlias, s.Root, s.TargetPrefix, s.Live));
        _syncsEmpty.IsVisible = _syncs.Count == 0;

        SelectedPeerId = keepPeer;
        SelectedMountRoot = keepMount;
        if (_peerPicker.SelectedItem == null && _peers.Count > 0) _peerPicker.SelectedIndex = 0;
        if (_mountPicker.SelectedItem == null && _mounts.Count > 0) _mountPicker.SelectedIndex = 0;
        UpdateDialHint();

        // Problems are rendered, never dropped. A share record that will
        // not decode is a thing the operator has; omitting it renders a
        // broken share as no share at all.
        if (dto.Problems is { Count: > 0 })
            SetStatus(string.Join("\n", dto.Problems), Brushes.Orange);
    }

    // PerformShareAsync offers the selected mount to the selected peer.
    // Async because Share re-establishes the connection, which dials.
    public async Task PerformShareAsync()
    {
        var root = SelectedMountRoot;
        var peer = SelectedPeerId;
        if (string.IsNullOrEmpty(root)) { SetStatus("pick a folder to share", Brushes.Orange); return; }
        if (string.IsNullOrEmpty(peer)) { SetStatus("pick a peer to share with", Brushes.Orange); return; }

        _shareBtn.IsEnabled = false;
        try
        {
            // Record the address BEFORE sharing, if the operator supplied
            // one. Share re-establishes the connection so its grant is in
            // force, and that reconnect needs somewhere to dial — without
            // this the share reports "not in force" for a reason the
            // operator just typed the answer to. It also means the
            // reciprocal dial after their accept can happen by itself.
            var addr = _shareAddrBox.Text?.Trim() ?? "";
            if (!string.IsNullOrEmpty(addr))
            {
                await Task.Run(() =>
                    Bridge.TakeString(Bridge.ShareComplete(_peerHandle, peer, addr)));
            }

            var title = TitleText.Trim();
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.ShareCreate(_peerHandle, root, peer, title)));
            var dto = Decode<ShareCreateReply>(reply, out var err);
            if (dto == null || !dto.Ok)
            {
                SetStatus($"share failed: {(dto?.Error is { Length: > 0 } e ? e : err)}", Brushes.IndianRed);
                return;
            }

            var lines = new List<string>
            {
                $"shared \"{dto.Root}\" with {Short(dto.PeerId)}"
                    + (string.IsNullOrEmpty(dto.PeerAlias) ? "" : $" ({dto.PeerAlias})"),
                $"granted: {dto.GrantSummary}",
            };
            // Reconnected is the difference between a share that works and
            // one that will work after some unrelated restart. Never
            // collapsed into "shared".
            lines.Add(dto.Reconnected
                ? "connection re-established — the grant is in force now"
                : "NOT re-established, so the grant is not yet in force: "
                  + (string.IsNullOrEmpty(dto.ReconnectNote) ? "reconnect to this peer" : dto.ReconnectNote));
            // No shell command here. This said "they now run: accept
            // <peer-id> <root>", which is the shell's phrasing handed to
            // someone sitting in front of a GUI — the same mistake as
            // printing the reciprocal dial, one screen earlier, and the
            // operator hit both in the same session.
            lines.Add($"On their machine: Shared Folders → \"Check their offers\" → Accept "
                + $"on \"{dto.Root}\". They need a local mount named \"{dto.Root}\" first.");
            SetStatus(string.Join("\n", lines), dto.Reconnected ? Brushes.PaleGreen : Brushes.Orange);
            Refresh();
        }
        finally
        {
            _shareBtn.IsEnabled = true;
        }
    }

    // PerformCompleteAsync runs the reciprocal dial. Network-bound, so it
    // goes to a worker; the address box is read on the UI thread first.
    public async Task PerformCompleteAsync(string peerId, TextBox? addrBox)
    {
        var typed = addrBox?.Text?.Trim() ?? "";
        SetStatus($"connecting to {Short(peerId)}…", Brushes.Gainsboro);
        var reply = await Task.Run(() =>
            Bridge.TakeString(Bridge.ShareComplete(_peerHandle, peerId, typed)));
        var dto = Decode<CompleteReply>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            SetStatus($"could not complete: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                Brushes.IndianRed);
            return;
        }

        // needsAddress is not a failure and is not styled as one. Nothing
        // is broken; we just do not know where they listen.
        if (dto.NeedsAddress)
        {
            if (addrBox != null) addrBox.IsVisible = true;
            SetStatus(
                $"{Short(dto.PeerId)}: {dto.Note}\n"
                + "Enter their address in the box on this row and press Complete connection. "
                + "Their app shows it under \"This peer\".",
                Brushes.Orange);
            return;
        }

        if (!dto.Connected)
        {
            SetStatus($"{Short(dto.PeerId)}: {dto.Note}", Brushes.IndianRed);
            return;
        }

        SetStatus(
            $"connected to {Short(dto.PeerId)} at {dto.Address} ({dto.AddressSource}).\n"
            + "Their grant is in force — deliveries for the folders you share with them are "
            + "now accepted, and this address is remembered.",
            Brushes.PaleGreen);
        Refresh();
    }

    public void PerformUnshare(string root, string peer)
    {
        var reply = Bridge.TakeString(Bridge.ShareRevoke(_peerHandle, root, peer));
        var dto = Decode<ShareRevokeReply>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            SetStatus($"unshare failed: {(dto?.Error is { Length: > 0 } e ? e : err)}", Brushes.IndianRed);
            return;
        }
        var lines = new List<string> { $"withdrew {Short(dto.PeerId)}'s access to \"{dto.Root}\"" };
        if (dto.StillOffered is { Count: > 0 })
            lines.Add($"still shared with {dto.StillOffered.Count} other peer(s)");
        // The caveat is carried as a field precisely so a renderer cannot
        // paraphrase it away. Withdrawal stops the NEXT handshake; it does
        // not reach into a live connection that already holds the grant.
        if (!string.IsNullOrEmpty(dto.Caveat)) lines.Add(dto.Caveat);
        SetStatus(string.Join("\n", lines), Brushes.Gainsboro);
        Refresh();
    }

    // LoadOffersAsync asks the selected peer what they are offering us.
    // Network: reconnects, then does a dispatched read of their tree.
    public async Task LoadOffersAsync()
    {
        var peer = SelectedPeerId;
        if (string.IsNullOrEmpty(peer)) { SetStatus("pick a peer first", Brushes.Orange); return; }

        _offersBtn.IsEnabled = false;
        try
        {
            SetStatus($"asking {Short(peer)} what they are sharing…", Brushes.Gainsboro);
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.ShareOffers(_peerHandle, peer)));
            var dto = Decode<OffersReply>(reply, out var err);
            _offers.Clear();
            if (dto == null || !dto.Ok)
            {
                _offersEmpty.Text = "could not read their offers — see the status line";
                _offersEmpty.IsVisible = true;
                // A 403 here is the common case and it is not a bug: they
                // have to share with us before we can read the record.
                SetStatus($"offers from {Short(peer)} failed: "
                    + $"{(dto?.Error is { Length: > 0 } e ? e : err)}\n"
                    + "if this is a permission refusal, they have not shared anything with you yet.",
                    Brushes.IndianRed);
                return;
            }
            foreach (var o in dto.Offers ?? new List<OfferDto>())
                _offers.Add(new OfferVm(o.Root, o.Title, o.TargetPrefix));
            _offersEmpty.Text = _offers.Count == 0
                ? $"{Short(peer)} is not offering you anything."
                : "";
            _offersEmpty.IsVisible = _offers.Count == 0;
            SetStatus(_offers.Count == 0
                ? $"{Short(peer)} has no offers for you."
                : $"{Short(peer)} is offering {_offers.Count} folder(s).", Brushes.Gainsboro);
        }
        finally
        {
            _offersBtn.IsEnabled = true;
        }
    }

    // PerformAcceptAsync takes an offer: writes the delivery grant,
    // reconnects, and syncs. Network-bound, hence async.
    public async Task PerformAcceptAsync(string root)
    {
        var peer = SelectedPeerId;
        if (string.IsNullOrEmpty(peer)) { SetStatus("pick a peer first", Brushes.Orange); return; }

        SetStatus($"accepting \"{root}\" from {Short(peer)}…", Brushes.Gainsboro);
        var reply = await Task.Run(() =>
            Bridge.TakeString(Bridge.ShareAccept(_peerHandle, peer, root)));
        var dto = Decode<AcceptReply>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            // The common failure is "no local mount to receive into", and
            // Accept deliberately leaves the delivery grant in place and
            // says so in its error. Passed through verbatim...
            var message = dto?.Error is { Length: > 0 } e ? e : err;
            // ...but the operation's own remedy is phrased for the shell
            // ("run `mount <dir> <prefix>`"), which is not something an
            // operator can do from this panel. A refusal whose fix names
            // a surface you are not on is a dead end, so the GUI adds the
            // GUI route. The wording of the refusal stays the model's —
            // this appends, it does not paraphrase.
            if (message.Contains("no local mount named"))
                message += $"\n\nIn the GUI: open a \"Local Files (manage mounts)\" panel and mount "
                    + $"a directory with the tree prefix ending in \"{root}\", then press Accept again. "
                    + "A sync writes into a mount; it does not create one.";
            SetStatus($"accept failed: {message}", Brushes.IndianRed);
            return;
        }

        var lines = new List<string>
        {
            $"accepted \"{dto.Root}\" from {Short(dto.PeerId)}",
            $"receiving into {dto.TargetPrefix}",
            $"granted: {dto.GrantSummary}",
        };
        if (!dto.Reconnected)
            lines.Add("connection NOT re-established, so your grant is not yet in force: "
                + (string.IsNullOrEmpty(dto.ReconnectNote) ? "reconnect to this peer" : dto.ReconnectNote));
        // The step nothing else prompts. Over a dial-by-address connection
        // the kernel's reciprocal grant is gated on
        // EstablishedViaRendezvousKey(), so accepting buys us the right to
        // subscribe and fetch and gives the publisher nothing — while
        // DELIVERY runs publisher→us. Without their dial the folder stays
        // empty and there is no error on this side to notice.
        //
        // `publisherMustDial` is a SHELL COMMAND and is deliberately not
        // shown. Printing it was this panel's first answer and it was the
        // wrong one: an instruction to go and type something elsewhere is
        // not a surface, and the operator has a button for it — on their
        // machine, in this same panel, on the row for this folder. Name
        // the button, not the command. The field stays in the DTO because
        // the shell renders it and the two share one operation.
        lines.Add("");
        lines.Add("Last step, on THEIR machine: open Shared Folders and press "
            + "\"Complete connection\" on this folder's row. Until they do, their "
            + "deliveries to you are refused and this folder stays empty.");
        SetStatus(string.Join("\n", lines), Brushes.PaleGreen);
        Refresh();
    }

    public void PerformUnsync(string peer, string root)
    {
        var reply = Bridge.TakeString(Bridge.ShareUnsync(_peerHandle, peer, root));
        var dto = Decode<UnsyncReply>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            SetStatus($"stop-receiving failed: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                Brushes.IndianRed);
            return;
        }
        var msg = dto.Found
            ? $"stopped receiving \"{dto.Root}\" from {Short(dto.PeerId)}"
            : $"no active sync for \"{dto.Root}\" from {Short(dto.PeerId)} — nothing to stop";
        if (!string.IsNullOrEmpty(dto.SubscriptionCloseError))
            msg += $"\nsubscription close reported: {dto.SubscriptionCloseError}";
        if (!string.IsNullOrEmpty(dto.Note)) msg += "\n" + dto.Note;
        SetStatus(msg, Brushes.Gainsboro);
        Refresh();
    }

    // --- Helpers ----------------------------------------------------------

    // UpdateDialHint restates, for the currently selected peer, the fact
    // that trips everybody: both sides must dial. Shown before the
    // operator acts rather than only in the post-accept status, because
    // the whole point is that it is not discoverable from the outcome.
    private void UpdateDialHint()
    {
        var vm = _peerPicker.SelectedItem as PeerVm;
        if (vm == null)
        {
            _dialHint.Text = "";
            _shareAddrBox.IsVisible = false;
            _shareAddrHint.IsVisible = false;
            return;
        }

        _dialHint.Text = vm.Connected
            ? $"connected to {Short(vm.PeerId)}."
            : $"{Short(vm.PeerId)} is announced but not connected.";

        // The address field appears only when we genuinely have nothing
        // to dial. When we do — because we dialled them once, or because
        // they are announcing on the LAN — the operator types nothing and
        // never learns this step exists.
        var needs = !vm.HasDialAddress;
        _shareAddrBox.IsVisible = needs;
        _shareAddrHint.IsVisible = needs;
        if (needs)
        {
            _shareAddrHint.Text =
                "We have no address for this peer — they dialled us, and the port we see is "
                + "their outgoing one, which cannot be dialled back. Enter the address their "
                + "app shows under \"This peer\". Asked once, then remembered.";
        }
    }

    // --- Auto-dial ---------------------------------------------------------

    // OpenConnectionsWake subscribes to connection changes so the
    // reciprocal dial can happen by itself.
    //
    // The receiver's Accept re-establishes their connection to us, which
    // is observable here and is the exact moment our dial has to happen:
    // their delivery grant exists from that point, and our outbound
    // connection has to be re-made for it to be assembled. Before this,
    // that timing was the operator's problem and there was no way for
    // them to know when it had arrived.
    private void OpenConnectionsWake()
    {
        if (!AutoDialOnConnect) return;
        _connsHandle = ParseHandle(Bridge.TakeString(Bridge.ConnectionsOpen(_peerHandle)));
        if (_connsHandle < 0) return;
        _connWakeCallback = OnConnectionsWake;
        _connWakeHandle = GCHandle.Alloc(_connWakeCallback);
        var ptr = Marshal.GetFunctionPointerForDelegate(_connWakeCallback);
        Bridge.TakeString(Bridge.ConnectionsRegisterWake(_connsHandle, ptr));
    }

    // Called from Go. Hop to the UI thread before touching anything.
    private void OnConnectionsWake(long handle)
    {
        if (_disposed) return;
        Dispatcher.UIThread.Post(() =>
        {
            if (_disposed) return;
            _ = AutoDialPendingAsync();
        });
    }

    // AutoDialPendingAsync completes the reciprocal dial for every peer
    // in our own share audience that we can reach and have not already
    // dialled this session.
    //
    // Three bounds, each load-bearing:
    //   - only peers in OUR audience, so a peer sharing TO us is never
    //     dialled from here and two panels cannot ping-pong;
    //   - once per peer, because a dial is a disconnect + reconnect and
    //     repeating it on every connection event would be an outage loop;
    //   - only when an address is already known, because the alternative
    //     is a silent failure, and the address question belongs in the
    //     share form where the operator is looking.
    private async Task AutoDialPendingAsync()
    {
        if (!AutoDialOnConnect || _disposed) return;

        var todo = new List<AudienceVm>();
        foreach (var share in _shares)
        {
            foreach (var who in share.AudienceState)
            {
                if (!who.HasAddress) continue;
                if (_autoDialled.Contains(who.PeerId)) continue;
                todo.Add(who);
            }
        }
        if (todo.Count == 0) return;

        foreach (var who in todo)
        {
            if (_disposed) return;
            // Marked before the attempt, not after: a failing dial that
            // is retried on every subsequent connection event is the
            // outage loop this bound exists to prevent. The manual
            // button remains for a deliberate retry.
            _autoDialled.Add(who.PeerId);
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.ShareComplete(_peerHandle, who.PeerId, "")));
            if (_disposed) return;
            var dto = Decode<CompleteReply>(reply, out _);
            if (dto is { Ok: true, Connected: true })
            {
                SetStatus(
                    $"{who.Label} accepted — connected back at {dto.Address} "
                    + $"({dto.AddressSource}). The folder is live; files you change will reach them.",
                    Brushes.PaleGreen);
                Refresh();
            }
        }
    }

    private static long ParseHandle(string reply)
    {
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean()) return -1;
            if (doc.RootElement.TryGetProperty("handle", out var h)) return h.GetInt64();
            if (doc.RootElement.TryGetProperty("result", out var r)
                && r.TryGetProperty("handle", out var rh)) return rh.GetInt64();
            return -1;
        }
        catch (JsonException)
        {
            return -1;
        }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_connsHandle >= 0)
        {
            Bridge.ConnectionsClose(_connsHandle);
            _connsHandle = -1;
        }
        if (_connWakeHandle.IsAllocated) _connWakeHandle.Free();
        _connWakeCallback = null;
    }

    private void SetStatus(string text, IBrush brush)
    {
        _status.Text = text;
        _status.Foreground = brush;
    }

    private static string Short(string peerId)
        => string.IsNullOrEmpty(peerId) || peerId.Length <= 12 ? peerId : peerId[..12] + "…";

    private static StackPanel Section(string header)
    {
        var stack = new StackPanel
        {
            Orientation = Orientation.Vertical,
            Margin = new Thickness(12, 8, 12, 2),
        };
        stack.Children.Add(new TextBlock
        {
            Text = header,
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
            Opacity = 0.85,
            Margin = new Thickness(0, 0, 0, 4),
        });
        return stack;
    }

    private static TextBlock SubHeader(string text) => new()
    {
        Text = text,
        FontSize = 11,
        Opacity = 0.6,
        Margin = new Thickness(0, 6, 0, 2),
    };

    private static TextBlock Hint(string text) => new()
    {
        Text = text,
        FontSize = 11,
        FontStyle = FontStyle.Italic,
        Opacity = 0.5,
        TextWrapping = TextWrapping.Wrap,
        Margin = new Thickness(0, 2, 0, 2),
    };

    private static TextBlock Label(string text) => new()
    {
        Text = text,
        FontSize = 12,
        Opacity = 0.6,
        VerticalAlignment = VerticalAlignment.Center,
    };

    private static TextBlock Line(string text, IBrush brush) => new()
    {
        Text = text,
        FontSize = 12,
        Foreground = brush,
        TextWrapping = TextWrapping.Wrap,
    };

    // AP37/P7: pointer input on a Button must never be wired with `+=`.
    // Click is the routed-command surface and is safe; it is the press
    // events Button marks handled in its own override.
    private static Button RowButton(string label, string tip, Action onClick)
    {
        var b = new Button { Content = label, FontSize = 11, Padding = new Thickness(8, 2) };
        ToolTip.SetTip(b, tip);
        b.Click += (_, _) => onClick();
        return b;
    }

    private static T? Decode<T>(string json, out string error) where T : class
    {
        error = "";
        try
        {
            var v = JsonSerializer.Deserialize<T>(json, JsonOpts);
            if (v == null) error = "empty reply";
            return v;
        }
        catch (JsonException ex)
        {
            error = ex.Message;
            return null;
        }
    }

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // --- View models ------------------------------------------------------

    private sealed record PeerVm(
        string PeerId, string Alias, string Address, bool Connected, string Source,
        string DialAddress, string DialAddressSource)
    {
        // HasDialAddress, not Address.Length — Address may be an inbound
        // connection's ephemeral source port, which looks like an address
        // and is refused. This is the one that decides whether the share
        // form asks a question.
        public bool HasDialAddress => !string.IsNullOrEmpty(DialAddress);

        // Source is rendered, not collapsed: "connected" is a fact and
        // "discovery" is an advertisement, and a picker that merges them
        // says a peer is reachable when nobody has reached it.
        public string Display =>
            (string.IsNullOrEmpty(Alias) ? Short(PeerId) : $"{Alias} ({Short(PeerId)})")
            + (Connected ? "  · connected" : $"  · {Source}");
    }

    private sealed record MountVm(string Root, string TargetPrefix, int SharedWith)
    {
        public string Display => SharedWith > 0 ? $"{Root}  (shared with {SharedWith})" : Root;
    }

    private sealed record ShareVm(
        string Root, string Title, string TargetPrefix,
        List<string> Audience, List<AudienceVm> AudienceState)
    {
        public string AudienceLine => Audience.Count == 0
            ? "no audience"
            : "shared with " + string.Join(", ", Audience.ConvertAll(Short));
    }

    private sealed record AudienceVm(
        string PeerId, string Alias, bool Connected, string Address, string AddressSource)
    {
        public bool HasAddress => !string.IsNullOrEmpty(Address);
        public string Label => string.IsNullOrEmpty(Alias)
            ? Short(PeerId)
            : $"{Alias} ({Short(PeerId)})";
    }

    private sealed record OfferVm(string Root, string Title, string TargetPrefix);

    private sealed record SyncVm(string PeerId, string Alias, string Root, string TargetPrefix, bool Live)
    {
        public string PeerLabel => string.IsNullOrEmpty(Alias) ? Short(PeerId) : $"{Alias} ({Short(PeerId)})";
    }

    // --- DTOs -------------------------------------------------------------
    //
    // AP49: an undeclared member is discarded by System.Text.Json in total
    // silence — the bug there was a provenance flag the model computed,
    // the bridge sent, and the panel never saw, with no test noticing
    // because none read a value that had quietly become false. Every field
    // the bridge sends is declared here and SharePanelTests asserts they
    // arrive.

    public sealed class RenderEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("localPeerId")] public string LocalPeerId { get; set; } = "";
        [JsonPropertyName("localAlias")] public string LocalAlias { get; set; } = "";
        [JsonPropertyName("listenAddr")] public string ListenAddr { get; set; } = "";
        [JsonPropertyName("advertisedUrl")] public string AdvertisedUrl { get; set; } = "";
        [JsonPropertyName("peers")] public List<PeerDto>? Peers { get; set; }
        [JsonPropertyName("mounts")] public List<MountDto>? Mounts { get; set; }
        [JsonPropertyName("shares")] public List<OfferDto>? Shares { get; set; }
        [JsonPropertyName("syncs")] public List<SyncDto>? Syncs { get; set; }
        [JsonPropertyName("problems")] public List<string>? Problems { get; set; }
    }

    public sealed class PeerDto
    {
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("alias")] public string Alias { get; set; } = "";
        [JsonPropertyName("address")] public string Address { get; set; } = "";
        [JsonPropertyName("connected")] public bool Connected { get; set; }
        [JsonPropertyName("source")] public string Source { get; set; } = "";
        [JsonPropertyName("dialAddress")] public string DialAddress { get; set; } = "";
        [JsonPropertyName("dialAddressSource")] public string DialAddressSource { get; set; } = "";
    }

    public sealed class MountDto
    {
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("sharedWith")] public int SharedWith { get; set; }
    }

    public sealed class OfferDto
    {
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("title")] public string Title { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("audience")] public List<string>? Audience { get; set; }
        [JsonPropertyName("createdAtMillis")] public ulong CreatedAtMillis { get; set; }
        [JsonPropertyName("audienceState")] public List<AudienceDto>? AudienceState { get; set; }
    }

    public sealed class AudienceDto
    {
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("alias")] public string Alias { get; set; } = "";
        [JsonPropertyName("connected")] public bool Connected { get; set; }
        [JsonPropertyName("address")] public string Address { get; set; } = "";
        [JsonPropertyName("addressSource")] public string AddressSource { get; set; } = "";
    }

    public sealed class CompleteReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("peerAlias")] public string PeerAlias { get; set; } = "";
        [JsonPropertyName("address")] public string Address { get; set; } = "";
        [JsonPropertyName("addressSource")] public string AddressSource { get; set; } = "";
        [JsonPropertyName("connected")] public bool Connected { get; set; }
        [JsonPropertyName("needsAddress")] public bool NeedsAddress { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }

    public sealed class SyncDto
    {
        [JsonPropertyName("remotePeerId")] public string RemotePeerId { get; set; } = "";
        [JsonPropertyName("remoteAlias")] public string RemoteAlias { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("sourcePrefix")] public string SourcePrefix { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("live")] public bool Live { get; set; }
    }

    public sealed class ShareCreateReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("peerAlias")] public string PeerAlias { get; set; } = "";
        [JsonPropertyName("policyPath")] public string PolicyPath { get; set; } = "";
        [JsonPropertyName("grantSummary")] public string GrantSummary { get; set; } = "";
        [JsonPropertyName("audience")] public List<string>? Audience { get; set; }
        [JsonPropertyName("reconnected")] public bool Reconnected { get; set; }
        [JsonPropertyName("reconnectNote")] public string ReconnectNote { get; set; } = "";
    }

    public sealed class ShareRevokeReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("policyRemoved")] public bool PolicyRemoved { get; set; }
        [JsonPropertyName("offerRemoved")] public bool OfferRemoved { get; set; }
        [JsonPropertyName("stillOffered")] public List<string>? StillOffered { get; set; }
        [JsonPropertyName("caveat")] public string Caveat { get; set; } = "";
    }

    public sealed class OffersReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("peer")] public string Peer { get; set; } = "";
        [JsonPropertyName("offers")] public List<OfferDto>? Offers { get; set; }
    }

    public sealed class AcceptReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("peerAlias")] public string PeerAlias { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("policyPath")] public string PolicyPath { get; set; } = "";
        [JsonPropertyName("grantSummary")] public string GrantSummary { get; set; } = "";
        [JsonPropertyName("sourcePrefix")] public string SourcePrefix { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("targetRoot")] public string TargetRoot { get; set; } = "";
        [JsonPropertyName("subscriptionId")] public string SubscriptionId { get; set; } = "";
        [JsonPropertyName("reconnected")] public bool Reconnected { get; set; }
        [JsonPropertyName("reconnectNote")] public string ReconnectNote { get; set; } = "";
        [JsonPropertyName("publisherMustDial")] public string PublisherMustDial { get; set; } = "";
    }

    public sealed class UnsyncReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("found")] public bool Found { get; set; }
        [JsonPropertyName("subscriptionCloseError")] public string SubscriptionCloseError { get; set; } = "";
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }
}
