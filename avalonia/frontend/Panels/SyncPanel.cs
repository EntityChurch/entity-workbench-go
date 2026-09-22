using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Automation;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// SyncPanel — "Sync" — the whole sharing job on one surface, in the order
// an operator does it.
//
// # Why this exists when four panels already touch this job
//
// Counted, not estimated, on 2026-09-04:
//
//	Shared Folders   10 buttons
//	Sharing Status    5
//	Local Files       5
//	Files             2
//
// Five panels touch one job (those four plus Peer Connections), and the
// shell has eighteen verbs for it. **Every one was added for a real
// reason, and most are the scar tissue of a defect this project actually
// hit** — which is the trap: each was locally justified and the sum is
// unusable. The operator's report was that sharing a file meant clicking
// buttons across several windows and pressing refresh repeatedly, and
// that shipping it in that state would be indefensible.
//
// This panel is not a fifth surface for the same job. It is the two
// gestures and nothing else:
//
//	pick a folder -> pick a peer -> Share
//	a card appears -> pick a directory -> Accept
//
// Everything that serves neither gesture stays where it is. The other
// panels are not deleted — Sharing Status answers "is what I declared
// actually working", which is a real question you reach for AFTER
// something breaks, and Local Files manages mounts. They move to
// Diagnostics; this one is what a first-timer opens.
//
// # The landscape, since this is a solved problem elsewhere
//
// Syncthing is the closest model and its config is the design we
// converged on independently in S6: a folder has an `id` that is **the
// same string on every device**, a `path` that is per-device and "not
// sent to other devices", a per-folder device list, and a per-device
// TYPE (`sendreceive` / `sendonly` / `receiveonly`). Its UI is two
// columns — folders and devices — with a pending share arriving as a
// card you Add or Ignore. Dropbox has no pairing step at all because a
// server mediates, which we cannot copy. rsync has no relationship layer
// to learn from.
//
// So: one folder list (ours and received together, because after S6 they
// ARE one kind of thing), offers as cards at the top, one primary button.
//
// # No Refresh button, and that is the point
//
// This panel holds a `SharingRegisterWake` subscription against the
// declared state — `app/workbench/{folders,devices,syncs}/` and
// `app/share/records/` — so it redraws itself. Twelve of the app's
// fifteen panels already did this; the three sharing panels were the
// only ones that did not, which is precisely why they grew Refresh
// buttons.
//
// The ONE thing that cannot be tree-driven is a peer's offers TO us:
// those live in their tree and reading them is a dispatched remote read
// (AP11). So they are fetched on open, on a wake, and on demand — and
// the panel says when it last asked rather than implying they are live.
//
// # Render is wired to the wake; Reconcile is not
//
// A reconcile pass DIALS every declared device and writes to the tree.
// Wiring it to a wake would make a dialer out of a panel an operator
// leaves open, and it would wake itself forever. `StatusRender` reads
// records and observes the substrate — no write, no dial — and that is
// what the wake drives.
public sealed class SyncPanel : UserControl, IPanelPreferredHeight
{
    // Chrome floor (AP64). A panel that does not declare one claims the
    // 200px default, and three such panels sum to less than the viewport
    // — each then gets ~297px, less than its own fixed chrome, clipped,
    // with nothing to scroll. The interface's own doc told implementers
    // to skip this; a default that is wrong for every caller is a bug
    // with a docstring.
    public double PreferredSlotMinHeight => 560;

    private readonly long _peerHandle;
    private readonly IPanelHost _host;

    private readonly TextBlock _identity;
    private readonly TextBlock _note;
    private readonly ObservableCollection<OfferVm> _offers = new();
    private readonly ObservableCollection<FolderVm> _folders = new();
    private readonly TextBlock _offersEmpty;
    private readonly TextBlock _foldersEmpty;
    private readonly Control _offersSection;
    private readonly Control _problemsSection;
    private readonly ObservableCollection<string> _problems = new();
    private readonly Button _shareBtn;

    private readonly StackPanel _shareForm;
    private readonly TextBox _shareDir;
    private readonly ComboBox _sharePeer;
    private readonly ObservableCollection<DeviceChoice> _sharePeers = new();
    private readonly List<DeviceChoice> _knownDevices = new();

    private long _wakeRegistration = -1;
    private SharingWake? _wakeCallback;
    private GCHandle _wakeHandle;
    private delegate void SharingWake(long handle);

    private bool _busy;
    private bool _closed;

    // This peer's own dialable address, from ShareRender. Carried so a
    // confirmation can name a real address rather than telling the
    // operator to go and look one up on another panel.
    private string _listenAddr = "";

    // AutoLoadOnOpen is on in the app and off under test.
    //
    // Opening the panel fetches offers, which is a DISPATCHED REMOTE READ
    // — and no suite in this repo may reach anything outside the tree.
    // Worse for a test, the fetch completes asynchronously and then
    // replaces the row collections, silently wiping whatever the test
    // seeded a moment earlier; that is a race a test cannot see, which
    // would pass or fail on dispatcher timing and be blamed on the
    // assertion. Same shape and reason as SharePanel.AutoDialOnConnect
    // and SharingStatusPanel.AutoReconcileOnOpen.
    internal static bool AutoLoadOnOpen = true;

    // --- Test/driver surface ---------------------------------------------
    //
    // The drivers below call the panel's OWN handlers — the same methods
    // the buttons invoke — rather than reimplementing the sequence.
    //
    // That is not a style preference. `FileExplorerPanel` shipped
    // `OnRowSelected` throwing on every file an operator clicked, at
    // 138/138 green, because its end-to-end test drove data to the row
    // and stopped one method call short of the code that renders it
    // (AP61). A driver that rebuilds the flow tests the driver.
    public int FolderCount => _folders.Count;
    public int OfferCount => _offers.Count;
    public string NoteText => _note.Text ?? "";
    public bool OffersSectionVisible => _offersSection.IsVisible;

    // The reconciler's diagnosis as the operator reads it. A panel's prose
    // is a surface with no reader in the suite (AP71) — this is the
    // reader, and it exists because the last defect here was a correct
    // sentence that reached no pixel.
    public bool ProblemsSectionVisible => _problemsSection.IsVisible;
    public IReadOnlyList<string> ProblemsForTests => _problems;

    // ReadForTests runs the panel's own read path. fetchOffers is a
    // DISPATCHED REMOTE READ per declared peer — real network, so a
    // caller opts in explicitly rather than getting it by default.
    internal Task ReadForTests(bool fetchOffers) => RefreshAsync(fetchOffers);

    // ShareForTests drives gesture one through the real handler, by
    // filling in the same controls an operator fills in.
    internal Task ShareForTests(string directory, string peerId)
    {
        _shareDir.Text = directory;
        _sharePeers.Clear();
        var choice = new DeviceChoice(peerId, peerId);
        _sharePeers.Add(choice);
        _sharePeer.SelectedItem = choice;
        return ShareAsync();
    }

    // AcceptForTests drives gesture two through the real handler, on a
    // row the panel actually built. It takes an INDEX into the rendered
    // offers rather than a peer/root pair, so a test cannot accept an
    // offer the panel never displayed — which is the failure the offer
    // list exists to prevent.
    internal Task AcceptForTests(int offerIndex, string directory)
        => AcceptAsync(_offers[offerIndex], directory);

    // OfferSummaryForTests is what the operator reads on the card. The
    // panel's prose is a surface with no reader in the suite (AP71) —
    // this is the reader.
    internal string OfferSummaryForTests(int i)
        => $"{_offers[i].PeerLabel} is offering “{_offers[i].Display}”";

    internal string SuggestedDirectoryForTests(int i) => _offers[i].SuggestedDirectory;

    internal string FolderRowForTests(int i)
        => $"{_folders[i].Arrow}  {_folders[i].Label}  |  {_folders[i].Detail}"
           + (_folders[i].Problem.Length > 0 ? $"  |  {_folders[i].Problem}" : "");

    public SyncPanel(long peerHandle, IPanelHost host)
    {
        _peerHandle = peerHandle;
        _host = host;

        _identity = new TextBlock
        {
            FontFamily = new FontFamily("monospace"),
            FontSize = 11,
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
        };

        _note = new TextBlock
        {
            TextWrapping = TextWrapping.Wrap,
            FontSize = 12,
            Margin = new Thickness(0, 6, 0, 0),
            IsVisible = false,
        };

        _shareBtn = new Button
        {
            Content = "Share a folder…",
            HorizontalAlignment = HorizontalAlignment.Left,
            Margin = new Thickness(0, 8, 0, 0),
        };
        _shareBtn.Click += (_, __) => BeginShare();

        _shareDir = new TextBox
        {
            Watermark = "the folder on this machine, e.g. /home/you/photos",
            FontSize = 11,
        };
        _sharePeer = new ComboBox
        {
            ItemsSource = _sharePeers,
            HorizontalAlignment = HorizontalAlignment.Stretch,
            Margin = new Thickness(0, 4, 0, 0),
        };
        var shareGo = new Button { Content = "Share", Margin = new Thickness(0, 6, 0, 0) };
        shareGo.Click += (_, __) => _ = ShareAsync();

        _shareForm = new StackPanel { Margin = new Thickness(0, 6, 0, 0), IsVisible = false };
        _shareForm.Children.Add(_shareDir);
        _shareForm.Children.Add(_sharePeer);
        _shareForm.Children.Add(shareGo);

        // AutomationIds — a stable name for every control an outside
        // driver has to press, so a scenario reads `#sync.share.go`
        // rather than `Button:Share`, which also matches "Share a
        // folder…" and would silently press the wrong one.
        //
        // These are `AutomationProperties`, not a private test channel:
        // the same metadata a screen reader consumes. Avalonia 11.2 ships
        // no AT-SPI bridge on X11 so nothing reads them today, but that
        // is a gap in the toolkit rather than a reason to invent a
        // parallel vocabulary we would have to migrate off later.
        AutomationProperties.SetAutomationId(_identity, "sync.identity");
        AutomationProperties.SetAutomationId(_note, "sync.note");
        AutomationProperties.SetAutomationId(_shareBtn, "sync.share.begin");
        AutomationProperties.SetAutomationId(_shareForm, "sync.share.form");
        AutomationProperties.SetAutomationId(_shareDir, "sync.share.dir");
        AutomationProperties.SetAutomationId(_sharePeer, "sync.share.peer");
        AutomationProperties.SetAutomationId(shareGo, "sync.share.go");

        _offersEmpty = Muted("nothing is being offered to you right now");
        _foldersEmpty = Muted("no shared folders yet — press “Share a folder…” to start");

        var offersList = BoundedList(_offers, OfferTemplate(), 180);
        _offersSection = Section("Offered to you", offersList, _offersEmpty);
        // Hidden until something arrives. An empty "Offered to you"
        // section on every launch is chrome that teaches the operator to
        // skip the region where the one time-sensitive thing appears.
        _offersSection.IsVisible = false;

        var foldersList = BoundedList(_folders, FolderTemplate(), 320);
        var foldersSection = Section("Shared folders", foldersList, _foldersEmpty);

        // Problems, directly under the identity line and above everything
        // else on the panel.
        //
        // Placement is the feature. The reconciler has always produced a
        // correct plain-language diagnosis of exactly the failure an
        // operator hits — a refused dial, a folder with no mount — and it
        // went to stderr, i.e. to a run log nobody opens. On 2026-09-08 an
        // operator sat in THIS panel for 45 minutes while the sentence
        // naming their problem was being computed on every pass.
        //
        // Not in the Sharing Status panel alone: that is a diagnostic
        // surface, and a diagnosis only helps in the panel where the flow
        // lives. Bounded like every other list here (AP64), so a peer
        // storm degrades to scrolling rather than to an unreachable Share
        // button.
        var problemsList = BoundedList(_problems, ProblemTemplate(), 140);
        _problemsSection = Section("Needs attention", problemsList, Muted(""));
        _problemsSection.IsVisible = false;
        AutomationProperties.SetAutomationId(_problemsSection, "sync.problems");

        var body = new StackPanel { Spacing = 4 };
        body.Children.Add(_identity);
        body.Children.Add(_problemsSection);
        body.Children.Add(_offersSection);
        body.Children.Add(foldersSection);
        body.Children.Add(_shareBtn);
        body.Children.Add(_shareForm);
        body.Children.Add(_note);

        Content = new ScrollViewer
        {
            HorizontalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Disabled,
            VerticalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
            Padding = new Thickness(10),
            Content = body,
        };

        OpenWake();
        if (AutoLoadOnOpen)
        {
            _ = RefreshAsync(fetchOffers: true);
            StartOffersPoll();
        }
    }

    // OffersPollInterval — how often the panel asks known peers what they
    // are offering us, while it is open.
    //
    // A POLL, in a panel whose defining feature is that it does not need
    // Refresh buttons. The distinction is the one AP73 draws: everything
    // else this panel shows is OUR tree, so it is watched. A peer's offer
    // TO US lives in THEIR tree, and reading it is a dispatched remote
    // read (AP11) that no local subscription can ever fire on. AP73's own
    // text names this as the single honest exception.
    //
    // Without it the receiving operator is told nothing, ever. Measured
    // by scripts/twopeer-gui.sh on 2026-09-06 with two real GUIs on a
    // real network: peer-a shares a folder, peer-a's panel confirms it,
    // and peer-b's Sync panel — open the whole time, on the machine the
    // share was addressed to — stays empty indefinitely, because nothing
    // in peer-b's tree moved. The share had worked; the only surface
    // that could say so never asked.
    //
    // Fifteen seconds because it is a READ over a connection that already
    // exists, not a dial: `ShareOffers` dispatches to a pooled peer and a
    // peer that is not reachable simply fails and is skipped. That is why
    // this is safe where wiring RECONCILE to a timer would not be — a
    // pass dials every declared device and writes to the tree, and an
    // operator leaving this panel open overnight must not turn their
    // window into a dialer.
    private const int OffersPollIntervalSeconds = 15;
    private DispatcherTimer? _offersTimer;

    private void StartOffersPoll()
    {
        _offersTimer = new DispatcherTimer
        {
            Interval = TimeSpan.FromSeconds(OffersPollIntervalSeconds),
        };
        _offersTimer.Tick += (_, __) =>
        {
            if (_closed || _busy) return;
            _ = RefreshAsync(fetchOffers: true);
        };
        _offersTimer.Start();
    }

    // --- Reactivity --------------------------------------------------------

    private void OpenWake()
    {
        _wakeCallback = OnSharingWake;
        _wakeHandle = GCHandle.Alloc(_wakeCallback);
        var ptr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        var reply = Bridge.TakeString(Bridge.SharingRegisterWake(_peerHandle, ptr));
        _wakeRegistration = ParseRegistration(reply);
    }

    // Runs on a Go-owned goroutine. It must not touch a control here —
    // every UI mutation goes through the dispatcher.
    private void OnSharingWake(long handle)
    {
        if (_closed) return;
        Dispatcher.UIThread.Post(() =>
        {
            if (_closed) return;
            // fetchOffers: false. A wake means OUR declared state moved,
            // which says nothing about what a peer is offering, and
            // dispatching a remote read on every tree event would turn a
            // folder rename into a burst of network calls.
            _ = RefreshAsync(fetchOffers: false);
        });
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        _closed = true;
        _offersTimer?.Stop();
        _offersTimer = null;
        if (_wakeRegistration >= 0)
        {
            Bridge.TakeString(Bridge.SharingUnregisterWake(_peerHandle, _wakeRegistration));
            _wakeRegistration = -1;
        }
        if (_wakeHandle.IsAllocated) _wakeHandle.Free();
        _wakeCallback = null;
        base.OnDetachedFromVisualTree(e);
    }

    // --- Reading -----------------------------------------------------------

    private async Task RefreshAsync(bool fetchOffers)
    {
        // StatusRender reads records and observes the substrate. It does
        // not dial and does not write, which is what makes it safe here.
        var json = Bridge.TakeString(Bridge.StatusRender(_peerHandle));
        var view = Parse<StatusView>(json);
        if (view is null || !view.OK)
        {
            SetNote(view?.Error ?? "could not read this peer's sharing state", isError: true);
            return;
        }

        _identity.Text = $"{view.LocalAlias}  {Short(view.LocalPeerId)}";

        // The reconciler's own diagnosis, in the panel that owns the flow.
        // Hidden when there is nothing wrong: a permanently-present
        // "Needs attention" heading is chrome, and chrome is what an
        // operator learns to skip.
        _problems.Clear();
        foreach (var p in view.Problems ?? new List<string>()) _problems.Add(p);
        _problemsSection.IsVisible = _problems.Count > 0;

        // The peers we can ACT on are the reachable ones, not the
        // declared ones.
        //
        // This was the first defect running the two-peer flow found, and
        // it broke both gestures at once. `StatusRender.devices` is the
        // DECLARATIONS — peers we already have a relationship with — and
        // connecting deliberately does not create one
        // (`RememberDeviceAddress` updates, never creates: dialling a
        // peer to look at its tree must not enroll it in a relationship
        // the loop then maintains forever). So a peer you have just
        // connected to is not in that list.
        //
        // Which meant: you could not share with someone until you had
        // already shared with them, and — worse, because it is silent —
        // a receiver saw NO offer from a peer they had not already
        // shared something with. That is the FIRST share between two
        // machines, i.e. the only case that matters on day one.
        //
        // `ShareRender.peers` is `ws.Peers()`: the connection pool
        // merged with mDNS discovery. Union it with the declarations, so
        // a declared peer that is currently asleep does not vanish from
        // the list.
        var peersJson = Bridge.TakeString(Bridge.ShareRender(_peerHandle));
        var peersView = Parse<ShareView>(peersJson);
        if (peersView is not null) _listenAddr = DialableAddress(peersView);

        var seen = new HashSet<string>(StringComparer.Ordinal);
        _knownDevices.Clear();
        foreach (var d in view.Devices)
        {
            if (!seen.Add(d.PeerID)) continue;
            _knownDevices.Add(new DeviceChoice(
                d.PeerID, string.IsNullOrWhiteSpace(d.Label) ? d.PeerID : d.Label));
        }
        foreach (var p in peersView?.Peers ?? new List<SharePeerDto>())
        {
            if (string.IsNullOrWhiteSpace(p.PeerID) || !seen.Add(p.PeerID)) continue;
            _knownDevices.Add(new DeviceChoice(
                p.PeerID, string.IsNullOrWhiteSpace(p.Alias) ? p.PeerID : p.Alias));
        }

        _folders.Clear();
        foreach (var f in view.Folders) _folders.Add(FolderVm.From(f));
        _foldersEmpty.IsVisible = _folders.Count == 0;

        if (fetchOffers) await FetchOffersAsync(view);
    }

    // FetchOffersAsync asks every declared peer what it is offering.
    //
    // A DISPATCHED REMOTE READ per peer, on a thread-pool worker: it goes
    // to another machine and a peer that is asleep is the normal case,
    // not the exceptional one.
    private async Task FetchOffersAsync(StatusView view)
    {
        // _knownDevices, not view.Devices: an offer arrives from someone
        // we may have no declaration for yet, and asking only declared
        // peers made the first share between two machines invisible to
        // the receiver. See the note in RefreshAsync.
        var paused = new HashSet<string>(StringComparer.Ordinal);
        foreach (var d in view.Devices)
        {
            if (d.Paused) paused.Add(d.PeerID);
        }

        // SNAPSHOT the peer list before the loop.
        //
        // Each iteration awaits a dispatched remote read, and the panel
        // is wake-driven — so a tree write during that await re-enters
        // RefreshAsync and rebuilds `_knownDevices` underneath a live
        // enumeration. `Accept` writes to the tree, which makes this
        // reliable rather than rare: it threw on the accept path the
        // first time the two-peer flow ran. In the app the throw is
        // contained, but only MaxContainedUiFaults (8) times — the ninth
        // takes the process down.
        var targets = _knownDevices.ToArray();
        var found = new List<OfferVm>();
        foreach (var choice in targets)
        {
            if (paused.Contains(choice.PeerId)) continue;
            var peerId = choice.PeerId;
            var label = choice.Label;
            var json = await Task.Run(() =>
                Bridge.TakeString(Bridge.ShareOffers(_peerHandle, peerId)));
            var reply = Parse<OffersReply>(json);
            if (reply is null || !reply.OK) continue;
            foreach (var o in reply.Offers)
            {
                // Only what we have not already taken up. An offer we
                // accepted is a FOLDER now, and showing it in both places
                // is the "same thing in two lists" confusion this panel
                // exists to end.
                if (HaveFolderFrom(peerId, o.Root)) continue;
                found.Add(new OfferVm(peerId, label, o.Root, o.Title));
            }
        }
        if (_closed) return;

        // REBUILD ONLY ON A REAL CHANGE. The offers list is now polled
        // every OffersPollIntervalSeconds, and each row carries a TextBox
        // the operator types their receiving directory into — so an
        // unconditional rebuild would silently discard a half-typed path
        // every fifteen seconds, on the one control in this panel where
        // the operator is expected to type something long.
        //
        // This is AP49's second half at a timer's tempo: an unconditional
        // redraw is a correctness surface, not a cosmetic one. The same
        // reasoning already gates BrowserPanel's three lists.
        var keys = new List<string>(found.Count);
        foreach (var o in found) keys.Add(o.PeerId + "/" + o.Root);
        var signature = string.Join(" ", keys);
        if (signature == _offersSignature) return;
        _offersSignature = signature;

        _offers.Clear();
        foreach (var o in found) _offers.Add(o);
        _offersEmpty.IsVisible = _offers.Count == 0;
        _offersSection.IsVisible = _offers.Count > 0;
    }

    // The offer set as last rendered. "" is not the same as "no offers":
    // the empty SET has a signature of "" too, which is correct — both
    // mean the rendered list is already right.
    private string _offersSignature = "";

    private bool HaveFolderFrom(string peerId, string root)
    {
        foreach (var f in _folders)
        {
            if (f.Origin == peerId && f.Root == root) return true;
        }
        return false;
    }

    // --- Gesture one: share ------------------------------------------------

    // BeginShare reveals the inline form. Inline rather than a dialog:
    // the gesture is "pick a folder, pick a peer, Share", and a modal
    // that hides the folder list while you choose makes the operator
    // remember what they were looking at.
    private void BeginShare()
    {
        _shareForm.IsVisible = !_shareForm.IsVisible;
        if (!_shareForm.IsVisible) return;
        _ = BeginShareAsync();
    }

    // BeginShareAsync RE-READS THE REACHABLE PEERS before offering the
    // list, and that read is the fix for a dead end an operator cannot
    // escape.
    //
    // This panel is wake-driven and holds no Refresh button on purpose —
    // but the wake watches the DECLARATION prefixes, and connecting to a
    // peer is deliberately not a declaration (`RememberDeviceAddress`
    // updates, never creates: dialling a machine to look at its tree must
    // not enroll it in a relationship the loop then maintains). So a
    // connection writes nothing to the tree, nothing wakes, and the peer
    // list stays exactly as it was when the panel opened.
    //
    // Which is empty, on every fresh launch. Measured by
    // scripts/twopeer-gui.sh on 2026-09-06: open the app, connect to a
    // peer in the Connections panel, press "Share a folder…" — the
    // combo is empty and the panel answers "Which peer?", with no
    // control anywhere that would populate it. The operator's only way
    // out is to restart the app, and the flow the product is named after
    // is unreachable until they work that out.
    //
    // This is AP73 in the mirror. That rule says a Refresh button on tree
    // data is a bug report about a missing subscription; the converse is
    // that state which is NOT in the tree — the connection pool, mDNS —
    // cannot be subscribed to, so a surface reading it must re-read at
    // the moment of use. Opening this form is that moment. It is a read
    // (`StatusRender` + `ShareRender`), never a pass: no dial, no write.
    private async Task BeginShareAsync()
    {
        await RefreshAsync(fetchOffers: false);
        if (_closed || !_shareForm.IsVisible) return;

        _sharePeers.Clear();
        foreach (var d in _knownDevices) _sharePeers.Add(d);
        _sharePeer.SelectedIndex = _sharePeers.Count > 0 ? 0 : -1;
        if (_sharePeers.Count == 0)
        {
            SetNote(
                "No peers are known yet. Connect to one first — a share is a relationship "
                + "with a machine, so there has to be a machine.", isError: false);
        }
    }

    // BeginShareForTests drives the button's OWN handler, including the
    // re-read. `ShareForTests` below deliberately does not: it injects a
    // peer straight into the combo, which is why the empty-list defect
    // survived a two-peer panel test that exercised everything after it.
    internal Task BeginShareForTests()
    {
        _shareForm.IsVisible = true;
        return BeginShareAsync();
    }

    internal int SharePeerChoiceCountForTests => _sharePeers.Count;

    // ShareAsync is ONE gesture over two substrate steps: bridging the
    // directory into the tree (a mount) and offering it to a peer.
    //
    // The operator does not say "mount" — that is a mechanism that leaked
    // into the UI, and it is why the flow had a step whose purpose nobody
    // could explain. `accept` already creates its own mount for the same
    // reason. Note this is the surface creating one, never the
    // reconciler: the loop deliberately refuses to create a mount,
    // because that writes to a path on somebody's disk and is an
    // operator's decision.
    private async Task ShareAsync()
    {
        if (_busy) return;
        var dir = (_shareDir.Text ?? "").Trim();
        if (dir.Length == 0) { SetNote("Which folder?", isError: true); return; }
        if (_sharePeer.SelectedItem is not DeviceChoice peer)
        {
            SetNote("Which peer?", isError: true);
            return;
        }

        _busy = true;
        try
        {
            var name = System.IO.Path.GetFileName(dir.TrimEnd('/', '\\'));
            if (name.Length == 0) { SetNote($"“{dir}” has no folder name", isError: true); return; }

            var mountJson = await Task.Run(() => Bridge.TakeString(Bridge.LocalFilesMount(
                _peerHandle, dir, $"archives/{name}/", "", "", 1, 0, 0)));
            var mount = Parse<MountReply>(mountJson);
            string root;
            if (mount is { OK: true })
            {
                root = mount.RootName;
            }
            else
            {
                // An existing mount for this directory is the ordinary
                // case on a second share, not a failure — the operator is
                // adding a peer to a folder they already share. Anything
                // else is reported as-is.
                if (mount?.Conflict is null)
                {
                    SetNote(mount?.Error ?? "could not bridge that folder", isError: true);
                    return;
                }
                root = name;
            }

            var shareJson = await Task.Run(() => Bridge.TakeString(
                Bridge.ShareCreate(_peerHandle, root, peer.PeerId, name)));
            var reply = Parse<ShareCreateReply>(shareJson);
            if (reply is null || !reply.OK)
            {
                SetNote(reply?.Error ?? "could not share that folder", isError: true);
                return;
            }
            SetNote(ShareConfirmation(reply, peer.Label), isError: false);
            _shareForm.IsVisible = false;
            _shareDir.Text = "";
        }
        finally { _busy = false; }
    }

    // --- Gesture two: accept ----------------------------------------------

    private async Task AcceptAsync(OfferVm offer, string directory)
    {
        if (_busy) return;
        _busy = true;
        try
        {
            // Network-bound: it authorizes, subscribes and backfills.
            var json = await Task.Run(() => Bridge.TakeString(
                Bridge.ShareAccept(_peerHandle, offer.PeerId, offer.Root, directory, 0)));
            var reply = Parse<ShareAcceptReply>(json);
            if (reply is null || !reply.OK)
            {
                SetNote(reply?.Error ?? "accept failed", isError: true);
                return;
            }
            SetNote(AcceptConfirmation(reply, offer.PeerLabel), isError: false);
            await RefreshAsync(fetchOffers: true);
        }
        finally { _busy = false; }
    }

    // DialableAddress is an address the OTHER machine could actually
    // use, or "" when we do not have one.
    //
    // A wildcard or port-0 address is worse than none: the SDK refuses to
    // ADVERTISE one for that reason, and printing one to an operator is
    // the same failure moved up to the UI — a fact-shaped string that
    // cannot work. The first version of this confirmation printed
    // "reachable at 127.0.0.1:0", which is the configured value, not a
    // bound port.
    private static string DialableAddress(ShareView v)
    {
        if (!string.IsNullOrWhiteSpace(v.Advertised)) return v.Advertised;
        var a = (v.ListenAddr ?? "").Trim();
        if (a.Length == 0) return "";
        if (a.EndsWith(":0", StringComparison.Ordinal)) return "";
        if (a.StartsWith("0.0.0.0", StringComparison.Ordinal)
            || a.StartsWith("[::]", StringComparison.Ordinal)
            || a.StartsWith(":", StringComparison.Ordinal)) return "";
        return a;
    }

    // --- What the operator is told -----------------------------------------
    //
    // Composed here from STRUCTURED fields, because the bridge replies
    // carry no prose and inventing a `note` field on them (which the
    // first version of this panel did) yields the empty string — the
    // gesture completes and says nothing at all. That was found by
    // running the two-peer flow and reading the output, which is the
    // only thing that finds this class (AP71).
    //
    // Three fields here that a paraphrase drops, each of which turns a
    // reported success into a lie when it is missing:
    //
    //   reconnected        a grant written on a live connection is inert
    //                      until the connection is re-established.
    //   publisherMustDial  the one step neither side's verb can perform.
    //   backfill           whether the files that were ALREADY there
    //                      came across, which is the operator's actual
    //                      question.

    private string ShareConfirmation(ShareCreateReply r, string peerLabel)
    {
        var who = string.IsNullOrWhiteSpace(r.PeerAlias) ? peerLabel : r.PeerAlias;
        var text = $"Shared “{r.Root}” with {who}.";
        if (!r.Reconnected && !string.IsNullOrWhiteSpace(r.ReconnectNote))
        {
            // NOT cosmetic. The grant set is assembled at handshake, so
            // this is the difference between a share that is in force and
            // one that will be after some unrelated restart.
            text += $" {r.ReconnectNote}";
        }
        text += $" They accept it on their machine — it will appear in their Sync panel.";
        return text;
    }

    private string AcceptConfirmation(ShareAcceptReply r, string peerLabel)
    {
        var who = string.IsNullOrWhiteSpace(r.PeerAlias) ? peerLabel : r.PeerAlias;
        var where = string.IsNullOrWhiteSpace(r.MountedDir) ? "this machine" : r.MountedDir;
        var text = r.CreatedMount
            ? $"Accepted “{r.Root}” from {who} into {where}, which it bridged for you."
            : $"Accepted “{r.Root}” from {who} into {where}.";

        // The operator's actual question: did my files come across.
        if (r.Backfill.Ran && !string.IsNullOrWhiteSpace(r.Backfill.Summary))
        {
            text += $" {r.Backfill.Summary}.";
        }
        else if (r.Backfill.Unreachable)
        {
            text += " Their machine could not be reached, so nothing has been copied yet.";
        }

        if (!string.IsNullOrWhiteSpace(r.PublisherMustDial))
        {
            // NOT r.PublisherMustDial itself. That field is a literal
            // shell command with a placeholder to hand-fill —
            // `connect <peer-id> <this-peer's host:port>` — which is the
            // right rendering in `entity-shell` and unusable in a
            // window. Pasting it here is AP71's shape inside the GUI:
            // guidance that is correct for one surface, shipped to
            // another where nobody can act on it.
            //
            // The MEANING, in the tense that is actually true: a first
            // transfer needs no dial from the publisher — the backfill
            // above already ran on authority this accept granted. Their
            // dial is for FUTURE changes. `accept` used to say "until
            // they dial you this folder stays empty", which told
            // operators their files had not arrived while the files were
            // on disk.
            text += $" Files already in the folder have arrived. For {who}'s LATER changes"
                  + " to reach you, they need to connect to this peer once";
            if (!string.IsNullOrWhiteSpace(_listenAddr))
            {
                text += $" — this peer is reachable at {_listenAddr}";
            }
            text += ".";
        }
        if (!r.Reconnected && !string.IsNullOrWhiteSpace(r.ReconnectNote))
        {
            text += $" {r.ReconnectNote}";
        }
        return text;
    }

    // --- Direction ---------------------------------------------------------

    private async Task SetDirectionAsync(FolderVm folder, string mode)
    {
        if (_busy) return;
        _busy = true;
        try
        {
            var json = Bridge.TakeString(
                Bridge.StatusSetFolderDirection(_peerHandle, folder.Id, mode));
            var reply = Parse<ActionReply>(json);
            SetNote(reply is { OK: true } ? reply.Note : (reply?.Error ?? "could not set direction"),
                isError: reply is not { OK: true });
            // No explicit refresh: the write moved app/workbench/folders/,
            // so the wake brings us back. That is the whole point of the
            // subscription, and calling Refresh here as well would redraw
            // twice for one change.
        }
        finally { _busy = false; }
    }

    // --- Rows --------------------------------------------------------------

    // Rows.Of, never `new FuncDataTemplate<T>` (AP46). Avalonia types the
    // builder's parameter T and CALLS IT WITH NULL during container
    // teardown, so every raw construction is a null dereference that
    // <Nullable>enable</Nullable> cannot see. All nineteen sites in this
    // frontend shipped that bug, and the symptom was SIGABRT rather than
    // an exception.
    private IDataTemplate OfferTemplate() => Rows.Of<OfferVm>((row, _) =>
    {
        var title = new TextBlock
        {
            Text = $"{row.PeerLabel} is offering “{row.Display}”",
            TextWrapping = TextWrapping.Wrap,
            FontWeight = FontWeight.SemiBold,
        };
        var dir = new TextBox
        {
            Text = row.SuggestedDirectory,
            Watermark = "where the files should go",
            FontSize = 11,
            Margin = new Thickness(0, 4, 0, 0),
        };
        var accept = new Button { Content = "Accept" };
        accept.Click += (_, __) => _ = AcceptAsync(row, dir.Text ?? "");

        // Per-ROW ids are not unique, and that is correct: a driver
        // addresses the nth card as `#sync.offer.accept[0]`. Naming them
        // per-peer would make a scenario's selector depend on a peer-id
        // it cannot know before the run.
        AutomationProperties.SetAutomationId(title, "sync.offer.title");
        AutomationProperties.SetAutomationId(dir, "sync.offer.dir");
        AutomationProperties.SetAutomationId(accept, "sync.offer.accept");

        var buttons = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 4, 0, 0),
        };
        buttons.Children.Add(accept);

        var card = new StackPanel { Margin = new Thickness(0, 4, 0, 6) };
        card.Children.Add(title);
        card.Children.Add(dir);
        card.Children.Add(buttons);
        return card;
    });

    private IDataTemplate FolderTemplate() => Rows.Of<FolderVm>((row, _) =>
    {
        var head = new TextBlock
        {
            Text = $"{row.Arrow}  {row.Label}",
            FontWeight = FontWeight.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        };
        var sub = new TextBlock
        {
            Text = row.Detail,
            FontSize = 11,
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
        };

        var stack = new StackPanel { Margin = new Thickness(0, 4, 0, 6) };
        stack.Children.Add(head);
        stack.Children.Add(sub);

        if (!string.IsNullOrEmpty(row.Problem))
        {
            stack.Children.Add(new TextBlock
            {
                Text = row.Problem,
                FontSize = 11,
                TextWrapping = TextWrapping.Wrap,
                Foreground = Brushes.IndianRed,
            });
        }

        // One verb per row, and it is the direction — the setting S6 made
        // real. A panel with no verb in it is AP57's tell.
        var cycle = new Button
        {
            Content = row.NextDirectionLabel,
            FontSize = 11,
            Margin = new Thickness(0, 4, 0, 0),
        };
        cycle.Click += (_, __) => _ = SetDirectionAsync(row, row.NextDirection);
        stack.Children.Add(cycle);

        AutomationProperties.SetAutomationId(head, "sync.folder.head");
        AutomationProperties.SetAutomationId(sub, "sync.folder.detail");
        AutomationProperties.SetAutomationId(cycle, "sync.folder.direction");

        return stack;
    });

    // Rows.Of, never `new FuncDataTemplate` (AP46): Avalonia calls the
    // builder with null during container teardown, and all nineteen raw
    // sites in this frontend shipped the same null dereference.
    private IDataTemplate ProblemTemplate() => Rows.Of<string>((text, _) =>
    {
        var block = new TextBlock
        {
            Text = text,
            FontSize = 11,
            TextWrapping = TextWrapping.Wrap,
            Foreground = Brushes.IndianRed,
            Margin = new Thickness(0, 2, 0, 0),
        };
        AutomationProperties.SetAutomationId(block, "sync.problem");
        return block;
    });

    // --- Chrome ------------------------------------------------------------

    private static TextBlock Muted(string text) => new()
    {
        Text = text,
        FontSize = 11,
        Opacity = 0.6,
        TextWrapping = TextWrapping.Wrap,
        Margin = new Thickness(0, 2, 0, 0),
    };

    // BoundedList caps its height on purpose. Never put an unbounded list
    // in a docked region (AP64): bounded, an under-estimated slot
    // degrades to scrolling rather than to a button nobody can reach.
    private static Control BoundedList<T>(ObservableCollection<T> items, IDataTemplate template, double maxHeight)
        => new ItemsControl
        {
            ItemsSource = items,
            ItemTemplate = template,
            MaxHeight = maxHeight,
        };

    private static Control Section(string heading, Control list, Control empty)
    {
        var panel = new StackPanel { Margin = new Thickness(0, 10, 0, 0) };
        panel.Children.Add(new TextBlock
        {
            Text = heading,
            FontWeight = FontWeight.Bold,
            FontSize = 12,
        });
        panel.Children.Add(empty);
        panel.Children.Add(list);
        return panel;
    }

    private void SetNote(string text, bool isError)
    {
        _note.Text = text;
        _note.IsVisible = !string.IsNullOrWhiteSpace(text);
        _note.Foreground = isError ? Brushes.IndianRed : Brushes.Gray;
    }

    private static string Short(string peerId)
        => string.IsNullOrEmpty(peerId) || peerId.Length <= 12 ? peerId : peerId[..12] + "…";

    private static long ParseRegistration(string json)
    {
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("registration", out var r)) return r.GetInt64();
        }
        catch (JsonException) { }
        return -1;
    }

    private static T? Parse<T>(string json) where T : class
    {
        try { return JsonSerializer.Deserialize<T>(json, JsonOpts); }
        catch (JsonException) { return null; }
    }

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // --- DTOs --------------------------------------------------------------
    //
    // EVERY field this panel reads is declared. An undeclared field is
    // dropped by System.Text.Json in total silence (AP49) — the model
    // computes it, the bridge sends it, and the renderer sees the default
    // with no warning of any kind. That has already shipped here once, in
    // the browser's provenance fields, and no test noticed because none
    // read a value that had quietly become false.

    // internal, not private: SyncPanelProblemsTests deserializes this DTO
    // directly. An undeclared field is dropped in silence at this
    // boundary (AP49), so the gate has to be able to see the shape rather
    // than a rendered row — an empty list renders as nothing, which is
    // also what "nothing is wrong" looks like.
    internal sealed class StatusView
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("localPeerId")] public string LocalPeerId { get; set; } = "";
        [JsonPropertyName("localAlias")] public string LocalAlias { get; set; } = "";
        [JsonPropertyName("devices")] public List<DeviceDto> Devices { get; set; } = new();
        [JsonPropertyName("folders")] public List<FolderDto> Folders { get; set; } = new();

        // **This field was not declared until 2026-09-09, and that is the
        // whole of defect 2c.** The reconciler produced a correct,
        // plain-language diagnosis — *"could not open our own connection to
        // 192.168.68.160:9000 (connection refused) — until it succeeds,
        // anything we write to a shared folder will not reach them"* —
        // StatusRender put it on the wire, and System.Text.Json dropped it
        // here in silence (AP49). So the panel that owns the flow could not
        // have shown the answer even if it had wanted to, and the sentence
        // went to a run log instead. An operator spent 45 minutes without
        // it while it was being computed for them every pass.
        //
        // Asserted by SyncPanelProblemsTests, because an undeclared field
        // fails nothing.
        [JsonPropertyName("problems")] public List<string> Problems { get; set; } = new();
    }

    // internal for the same reason StatusView is: it is reachable from a
    // property on an internal type, so C# requires at least that
    // accessibility.
    internal sealed class DeviceDto
    {
        [JsonPropertyName("peerId")] public string PeerID { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("paused")] public bool Paused { get; set; }
    }

    internal sealed class FolderDto
    {
        [JsonPropertyName("id")] public string Id { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("local")] public bool Local { get; set; }
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("localRoot")] public string LocalRoot { get; set; } = "";
        [JsonPropertyName("path")] public string Path { get; set; } = "";
        [JsonPropertyName("origin")] public string Origin { get; set; } = "";
        [JsonPropertyName("mode")] public string Mode { get; set; } = "";
        [JsonPropertyName("mounted")] public bool Mounted { get; set; }
        [JsonPropertyName("syncing")] public bool Syncing { get; set; }
        [JsonPropertyName("accepted")] public bool Accepted { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
        [JsonPropertyName("filesPresent")] public int FilesPresent { get; set; }
        [JsonPropertyName("filesIngested")] public int FilesIngested { get; set; }
        [JsonPropertyName("filesObservable")] public bool FilesObservable { get; set; }
        [JsonPropertyName("peers")] public List<FolderPeerDto> Peers { get; set; } = new();
    }

    internal sealed class FolderPeerDto
    {
        [JsonPropertyName("peerId")] public string PeerID { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("state")] public string State { get; set; } = "";
    }

    private sealed class OffersReply
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("offers")] public List<OfferDto> Offers { get; set; } = new();
    }

    private sealed class OfferDto
    {
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("title")] public string Title { get; set; } = "";
    }

    // ShareView is read for its PEER list only — the reachable peers,
    // from the connection pool merged with mDNS. The rest of
    // ShareRender's envelope is the legacy Shared Folders panel's.
    private sealed class ShareView
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("peers")] public List<SharePeerDto> Peers { get; set; } = new();
        // What the OTHER side needs typed at it, carried in the render
        // so an instruction can name a real address instead of a
        // placeholder the operator has to go and find.
        [JsonPropertyName("listenAddr")] public string ListenAddr { get; set; } = "";
        // What the peer actually PUBLISHES about itself. Preferred over
        // listenAddr, which is the CONFIGURED value — `127.0.0.1:0` asks
        // the kernel for an ephemeral port and is not an address anyone
        // can dial.
        [JsonPropertyName("advertisedUrl")] public string Advertised { get; set; } = "";
    }

    internal sealed class SharePeerDto
    {
        [JsonPropertyName("peerId")] public string PeerID { get; set; } = "";
        [JsonPropertyName("alias")] public string Alias { get; set; } = "";
        [JsonPropertyName("connected")] public bool Connected { get; set; }
        // "connected" or "discovery" — different claims. A connection is
        // a fact; an mDNS announcement is an advertisement.
        [JsonPropertyName("source")] public string Source { get; set; } = "";
    }

    private sealed class MountReply
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("rootName")] public string RootName { get; set; } = "";
        // Structured, not a sentence: a conflict here means the directory
        // is already bridged, which on a second share is the ordinary
        // case rather than a failure. shellcmd.MountConflict is a typed
        // error precisely so a surface can tell the two apart.
        [JsonPropertyName("conflict")] public JsonElement? Conflict { get; set; }
    }

    // DeviceChoice is a peer as the share form offers it. ToString is the
    // ComboBox's display text.
    internal sealed record DeviceChoice(string PeerId, string Label)
    {
        public override string ToString() => Label;
    }

    private sealed class ShareCreateReply
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("peerAlias")] public string PeerAlias { get; set; } = "";
        [JsonPropertyName("grantSummary")] public string GrantSummary { get; set; } = "";
        [JsonPropertyName("reconnected")] public bool Reconnected { get; set; }
        [JsonPropertyName("reconnectNote")] public string ReconnectNote { get; set; } = "";
    }

    private sealed class ShareAcceptReply
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("peerAlias")] public string PeerAlias { get; set; } = "";
        [JsonPropertyName("localRoot")] public string LocalRoot { get; set; } = "";
        [JsonPropertyName("mountedDir")] public string MountedDir { get; set; } = "";
        [JsonPropertyName("createdMount")] public bool CreatedMount { get; set; }
        [JsonPropertyName("reconnected")] public bool Reconnected { get; set; }
        [JsonPropertyName("reconnectNote")] public string ReconnectNote { get; set; } = "";
        [JsonPropertyName("publisherMustDial")] public string PublisherMustDial { get; set; } = "";
        [JsonPropertyName("backfill")] public BackfillDto Backfill { get; set; } = new();
    }

    internal sealed class BackfillDto
    {
        [JsonPropertyName("ran")] public bool Ran { get; set; }
        [JsonPropertyName("summary")] public string Summary { get; set; } = "";
        [JsonPropertyName("unreachable")] public bool Unreachable { get; set; }
        [JsonPropertyName("failed")] public int Failed { get; set; }
    }

    private sealed class ActionReply
    {
        [JsonPropertyName("ok")] public bool OK { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }

    // --- Row view-models ---------------------------------------------------

    internal sealed class OfferVm
    {
        public string PeerId { get; }
        public string PeerLabel { get; }
        public string Root { get; }
        public string Display { get; }
        // Qualified by PEER, so two peers offering a same-named folder do
        // not land on one directory — where the second accept refuses as
        // non-empty and reads as our bug.
        public string SuggestedDirectory { get; }

        public OfferVm(string peerId, string peerLabel, string root, string title)
        {
            PeerId = peerId;
            PeerLabel = peerLabel;
            Root = root;
            Display = string.IsNullOrWhiteSpace(title) ? root : title;
            var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
            SuggestedDirectory = System.IO.Path.Combine(home, "entity-shared", peerLabel, root);
        }
    }

    internal sealed class FolderVm
    {
        public string Id { get; init; } = "";
        public string Label { get; init; } = "";
        public string Origin { get; init; } = "";
        public string Root { get; init; } = "";
        public string Mode { get; init; } = "";
        public string Arrow { get; init; } = "";
        public string Detail { get; init; } = "";
        public string Problem { get; init; } = "";

        // The direction cycle: both -> send -> receive -> both.
        public string NextDirection => Mode switch
        {
            "both" => "send",
            "send" => "receive",
            _ => "both",
        };

        public string NextDirectionLabel => NextDirection switch
        {
            "send" => "Make send-only",
            "receive" => "Make receive-only",
            _ => "Make two-way",
        };

        public static FolderVm From(FolderDto f)
        {
            // The arrow is the DIRECTION, which after S6 is Mode and not
            // origin. Before S6 the reconciler branched on IsLocal()
            // everywhere it meant direction, so "both" was inexpressible
            // and a bidirectional share was two unrelated one-way pipes.
            var arrow = f.Mode switch
            {
                "send" => "→",
                "receive" => "←",
                _ => "↔",
            };

            var who = new List<string>();
            foreach (var p in f.Peers)
            {
                var name = string.IsNullOrWhiteSpace(p.Label) ? p.PeerID : p.Label;
                who.Add($"{name} ({p.State})");
            }
            var peers = who.Count == 0 ? "not shared with anyone yet" : string.Join(", ", who);

            // Both sides of the lossy stage, never one count (AP59). The
            // watcher writes one entity per admitted file; the ingest
            // chain lifts each into a document, and reading only the
            // source layer displayed "401 entities" for a directory with
            // exactly one openable document. FilesObservable false means
            // there is no mount to count — "nothing arrived" and "there
            // is nowhere for it to arrive" are different claims, and the
            // second is usually the whole problem.
            var files = f.FilesObservable
                ? $"{f.FilesIngested} of {f.FilesPresent} files readable"
                : "no mount — nothing to count";

            var where = string.IsNullOrWhiteSpace(f.Path) ? "(path unknown)" : f.Path;

            var problem = "";
            if (!f.Mounted) problem = f.Note.Length > 0 ? f.Note : "this folder has no mount";
            else if (!f.Local && f.Accepted && !f.Syncing) problem = f.Note;

            return new FolderVm
            {
                Id = f.Id,
                Label = string.IsNullOrWhiteSpace(f.Label) ? f.Root : f.Label,
                Origin = f.Origin,
                Root = f.Root,
                Mode = string.IsNullOrWhiteSpace(f.Mode) ? "both" : f.Mode,
                Arrow = arrow,
                Detail = $"{peers}  ·  {files}  ·  {where}",
                Problem = problem,
            };
        }
    }
}
