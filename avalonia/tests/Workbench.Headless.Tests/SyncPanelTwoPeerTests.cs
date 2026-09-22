using System;
using System.IO;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;
using Xunit.Abstractions;

namespace EntityAvalonia.Tests;

// SyncPanelTwoPeerTests — the flow an operator actually performs, driven
// through the PANEL, across two real peers on loopback.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why this exists and why the single-peer tests are not enough
//
// `SyncPanelTests` asserts the panel's structure. It cannot tell you
// whether sharing a folder with another machine works, because there is
// no other machine in it — and every symptom the operator reported was
// about the relationship between two peers, not about one.
//
// It is also the only automated thing in this repo that reads the
// panel's PROSE. A verb's printed guidance is a surface with no reader
// in the suite (AP71): `share` went on telling the operator on the other
// machine to run a step that a release had deleted, for as long as that
// release had shipped, with every gate green. The panel's cards are the
// same class of surface. So this test asserts on what the card SAYS, not
// only on what the model holds.
//
// # Own peers, not the fixture's
//
// `BridgeFixture.DefaultPeer` is shared by the whole assembly and other
// tests write real declarations to it (AP70). A two-peer flow needs to
// know that the folder it sees is the folder it shared, so both peers
// here are created for this test and destroyed with it.
//
// # This does NOT test the permission stage
//
// Both peers are created with `open_access`, which is a wildcard grant —
// per AP63 that means this file says **nothing** about authorization.
// It measures the panel driving the data path. The permission stage is
// gated without wildcards in `shellboot/flow_e2e_test.go`, and the two
// must not be confused: a green run here does not mean an operator's
// share will authorize.
//
// Loopback only. No suite in this repo may reach the public internet.
[Collection(nameof(BridgeCollection))]
public class SyncPanelTwoPeerTests : IDisposable
{
    private readonly ITestOutputHelper _out;
    private readonly long _alice;
    private readonly long _bob;
    private readonly string _aliceDir;
    private readonly string _bobDir;
    private readonly string _tmp;

    public SyncPanelTwoPeerTests(ITestOutputHelper output)
    {
        _out = output;
        _tmp = Path.Combine(Path.GetTempPath(), "sync-panel-" + Guid.NewGuid().ToString("N")[..8]);
        _aliceDir = Path.Combine(_tmp, "photos");
        _bobDir = Path.Combine(_tmp, "from-alice");
        Directory.CreateDirectory(_aliceDir);
        File.WriteAllText(Path.Combine(_aliceDir, "one.md"), "# one\n\nalice wrote this.\n");
        File.WriteAllText(Path.Combine(_aliceDir, "two.md"), "# two\n\nand this.\n");

        _alice = CreatePeer("alice");
        _bob = CreatePeer("bob");
    }

    private static long CreatePeer(string alias)
    {
        // ListenAddr 127.0.0.1:0 — a real listener on an ephemeral
        // loopback port. open_access for the reason in the header.
        var cfg = JsonSerializer.Serialize(new
        {
            alias,
            listen = "127.0.0.1:0",
            open_access = true,
        });
        var reply = Bridge.TakeString(Bridge.PeerCreate(cfg));
        using var doc = JsonDocument.Parse(reply);
        Assert.True(doc.RootElement.GetProperty("ok").GetBoolean(),
            $"PeerCreate({alias}) failed: {reply}");
        return doc.RootElement.GetProperty("handle").GetInt64();
    }

    // THE test: alice shares through her panel, bob sees it on his,
    // accepts through his, and alice's files are on bob's disk.
    [AvaloniaFact]
    public async Task A_Folder_Shared_On_One_Panel_Is_Accepted_On_The_Other_And_The_Files_Arrive()
    {
        SyncPanel.AutoLoadOnOpen = false;

        var (aw, ap) = Open(_alice);
        var (bw, bp) = Open(_bob);
        try
        {
            Connect(_alice, _bob, "bob");
            Connect(_bob, _alice, "alice");

            // --- Gesture one: alice shares --------------------------------
            await ap.ShareForTests(_aliceDir, PeerIdOf(_bob));
            _out.WriteLine($"[alice, after Share] {ap.NoteText}");
            Assert.False(ap.NoteText.Contains("could not", StringComparison.OrdinalIgnoreCase),
                $"share failed: {ap.NoteText}");
            // The gesture must SAY something. The first version of this
            // panel read a `note` field the bridge reply does not have,
            // so Share and Accept both completed in total silence — the
            // operator pressed the button and nothing on screen changed
            // to confirm it. Nothing but running the flow finds that
            // (AP71), and nothing but this assertion keeps it fixed.
            Assert.False(string.IsNullOrWhiteSpace(ap.NoteText),
                "Share completed and told the operator nothing");
            Assert.Contains("photos", ap.NoteText);

            await ap.ReadForTests(fetchOffers: false);
            Assert.True(ap.FolderCount >= 1, "alice's panel shows no folder after sharing one");
            _out.WriteLine($"[alice, folder row] {ap.FolderRowForTests(0)}");

            // --- Gesture two: bob sees it and accepts ---------------------
            await WaitFor(async () =>
            {
                await bp.ReadForTests(fetchOffers: true);
                return bp.OfferCount > 0;
            }, "bob's panel never showed alice's offer");

            // The card's PROSE, which nothing else in the suite reads.
            var card = bp.OfferSummaryForTests(0);
            _out.WriteLine($"[bob, offer card] {card}");
            Assert.Contains("is offering", card);

            // The suggested directory is qualified BY PEER. Two peers
            // sharing a same-named folder must not land on one path,
            // where the second accept refuses as non-empty and reads as
            // our bug.
            var suggested = bp.SuggestedDirectoryForTests(0);
            _out.WriteLine($"[bob, suggested directory] {suggested}");
            Assert.Contains("entity-shared", suggested);

            // Accept into a DIFFERENTLY NAMED directory on purpose: the
            // two roots are only the same by coincidence, and a fixture
            // where they agree cannot fail on the field that exists for
            // the case where they do not (FolderData.LocalRoot).
            await bp.AcceptForTests(0, _bobDir);
            _out.WriteLine($"[bob, after Accept] {bp.NoteText}");
            Assert.False(bp.NoteText.Contains("failed", StringComparison.OrdinalIgnoreCase),
                $"accept failed: {bp.NoteText}");
            Assert.False(string.IsNullOrWhiteSpace(bp.NoteText),
                "Accept completed and told the operator nothing");
            // WHERE the files went is the operator's first question, and
            // this surface had no answer for it at all.
            Assert.Contains(_bobDir, bp.NoteText);

            // --- The only assertion that matters: the bytes -------------
            await WaitFor(() => Task.FromResult(
                    File.Exists(Path.Combine(_bobDir, "one.md")) &&
                    File.Exists(Path.Combine(_bobDir, "two.md"))),
                $"alice's files never reached {_bobDir}. " +
                $"Contents: {DirListing(_bobDir)}");

            Assert.Equal("# one\n\nalice wrote this.\n",
                File.ReadAllText(Path.Combine(_bobDir, "one.md")));

            await bp.ReadForTests(fetchOffers: false);
            _out.WriteLine($"[bob, folder row] {bp.FolderRowForTests(0)}");
        }
        finally { bw.Close(); aw.Close(); }
    }

    // A peer connected AFTER the panel opened must appear in the share
    // form. This is the first gesture's dead end, and it shipped.
    //
    // The panel is wake-driven and has no Refresh button, by design. But
    // the wake watches the DECLARATION prefixes, and connecting to a peer
    // writes no declaration — `RememberDeviceAddress` updates, never
    // creates, precisely so that dialling a machine to look at its tree
    // does not enroll it in a maintained relationship. So nothing wakes,
    // and the peer list stays as it was when the panel opened: empty.
    //
    // The operator's experience, measured by scripts/twopeer-gui.sh on
    // 2026-09-06 against two real GUIs on a real network: connect to the
    // other machine, press "Share a folder…", get an empty dropdown and
    // "Which peer?", with no control on the surface that would fill it.
    // Restarting the app is the only escape, and the flow the product is
    // named after is unreachable until you guess that.
    //
    // WHY THE TEST ABOVE COULD NOT SEE IT: `ShareForTests` injects the
    // peer straight into the combo and calls ShareAsync. It drives the
    // second half of gesture one and skips the half that was broken —
    // which is exactly the shape AP61 names, a test that drives data to
    // the row and never selects one. This test drives the BUTTON'S own
    // handler instead.
    [AvaloniaFact]
    public async Task A_Peer_Connected_After_The_Panel_Opened_Is_Offered_In_The_Share_Form()
    {
        SyncPanel.AutoLoadOnOpen = false;

        var (aw, ap) = Open(_alice);
        try
        {
            // The panel is open and current BEFORE any peer exists — the
            // ordinary case, since the app restores its layout at startup
            // and the operator connects afterwards.
            await ap.ReadForTests(fetchOffers: false);

            Connect(_alice, _bob, "bob");

            await ap.BeginShareForTests();

            Assert.True(ap.SharePeerChoiceCountForTests > 0,
                "the share form offered no peers after one was connected; the panel says " +
                $"\"{ap.NoteText}\" and an operator has no control that would populate it");
        }
        finally { aw.Close(); }
    }

    // The offer must LEAVE bob's offer list once he has accepted it.
    //
    // An accepted offer is a FOLDER now. Showing it in both places is the
    // "same thing in two lists" confusion this panel exists to end, and
    // it is also how an operator ends up accepting twice into two
    // directories — which is the multi-destination mess they reported.
    [AvaloniaFact]
    public async Task An_Accepted_Offer_Stops_Being_An_Offer()
    {
        SyncPanel.AutoLoadOnOpen = false;

        var (aw, ap) = Open(_alice);
        var (bw, bp) = Open(_bob);
        try
        {
            Connect(_alice, _bob, "bob");
            Connect(_bob, _alice, "alice");

            await ap.ShareForTests(_aliceDir, PeerIdOf(_bob));
            await WaitFor(async () =>
            {
                await bp.ReadForTests(fetchOffers: true);
                return bp.OfferCount > 0;
            }, "bob never saw the offer");

            // Control arm: it IS there before the accept. Without this
            // the assertion below passes against a panel that never
            // shows an offer at all — which is the likelier false green.
            Assert.True(bp.OfferCount > 0);
            Assert.True(bp.OffersSectionVisible);

            await bp.AcceptForTests(0, _bobDir);
            await bp.ReadForTests(fetchOffers: true);

            Assert.Equal(0, bp.OfferCount);
            Assert.False(bp.OffersSectionVisible,
                "the offers section is still on screen with nothing in it");
        }
        finally { bw.Close(); aw.Close(); }
    }

    private static (Window w, SyncPanel p) Open(long handle)
    {
        var panel = new SyncPanel(handle, new TestHost());
        var window = new Window { Width = 900, Height = 700, Content = panel };
        window.Show();
        window.UpdateLayout();
        return (window, panel);
    }

    // Connect dials `to` from `from`. BOTH directions are dialled: a
    // sync is mutual, and a dial-by-address authorizes the DIALER only
    // (AP63) — one direction gives you an accepted subscription and an
    // empty folder.
    private static void Connect(long from, long to, string alias)
    {
        var connsHandle = ParseHandle(Bridge.TakeString(Bridge.ConnectionsOpen(from)));
        Assert.True(connsHandle >= 0, "ConnectionsOpen failed");
        var addr = ListenAddrOf(to);
        Assert.False(string.IsNullOrWhiteSpace(addr),
            "the target peer advertised no listen address — BringUpListener bound nothing");
        var reply = Bridge.TakeString(Bridge.ConnectionsConnect(connsHandle, alias, addr));
        Bridge.ConnectionsClose(connsHandle);
        // Carry the reply into the message. A connection failure that
        // says only "sub-string found" makes the reader re-run the suite
        // with a print statement to learn what every failure already
        // knew.
        Assert.False(reply.Contains("\"ok\":false"),
            $"dialling {addr} failed: {reply}");
    }

    private static string PeerIdOf(long handle)
    {
        var json = Bridge.TakeString(Bridge.PeerList());
        using var doc = JsonDocument.Parse(json);
        // {ok, peers:[...]} — an envelope, not a bare array.
        foreach (var e in doc.RootElement.GetProperty("peers").EnumerateArray())
        {
            if (e.GetProperty("handle").GetInt64() != handle) continue;
            return e.TryGetProperty("peer_id", out var v) ? (v.GetString() ?? "") : "";
        }
        return "";
    }

    // ListenAddrOf reads `scheme`, not `addr`, to decide whether the peer
    // is up: core-go sets `p.listener` only on ListenReady, so `Addr()`
    // is nil for a WebSocket listener that is bound and serving. That
    // reported `listening: false` for every ws peer until 2026-08-18.
    private static string ListenAddrOf(long handle)
    {
        var json = Bridge.TakeString(Bridge.PeerListenAddr(handle));
        using var doc = JsonDocument.Parse(json);
        if (!doc.RootElement.TryGetProperty("result", out var r)) return "";
        if (!r.TryGetProperty("listening", out var up) || !up.GetBoolean()) return "";
        return r.TryGetProperty("addr", out var a) ? (a.GetString() ?? "") : "";
    }

    private static long ParseHandle(string json)
    {
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.TryGetProperty("handle", out var h) ? h.GetInt64() : -1;
    }

    // WaitFor polls because the flow crosses two peers, a subscription
    // and a filesystem watcher. The deadline is generous on purpose: a
    // timeout here should mean "it never happened", not "the machine was
    // busy" — a flaky gate on this flow would be worse than none.
    private static async Task WaitFor(Func<Task<bool>> cond, string message)
    {
        var deadline = DateTime.UtcNow.AddSeconds(60);
        while (DateTime.UtcNow < deadline)
        {
            if (await cond()) return;
            await Task.Delay(250);
        }
        Assert.Fail(message);
    }

    private static string DirListing(string dir)
        => Directory.Exists(dir)
            ? string.Join(", ", Directory.GetFileSystemEntries(dir))
            : "(the directory does not exist)";

    public void Dispose()
    {
        if (_bob != 0) Bridge.TakeString(Bridge.PeerDestroy(_bob));
        if (_alice != 0) Bridge.TakeString(Bridge.PeerDestroy(_alice));
        try { Directory.Delete(_tmp, recursive: true); } catch (IOException) { }
    }

    private sealed class TestHost : IPanelHost
    {
        public event Action<string>? SelectedPath;
        public string? CurrentSelectedPath { get; private set; }
        public void PublishSelectedPath(string path)
        {
            CurrentSelectedPath = path;
            SelectedPath?.Invoke(path);
        }
        public void RequestPeerStatusRefresh() { }
    }
}
