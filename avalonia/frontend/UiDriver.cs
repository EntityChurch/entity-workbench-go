using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
using System.Threading;
using Avalonia;
using Avalonia.Automation;
using Avalonia.Controls;
// Avalonia.Input for InputExtensions.InputHitTest — it hangs off
// IInputElement, not off Visual, so the VisualTree using is not enough.
using Avalonia.Input;
using Avalonia.Threading;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;

namespace EntityAvalonia;

// UiDriver — an automation server inside the running app, so something
// outside the process can find a control, press it, and read what the
// window then says.
//
// # Why this exists at all
//
// There is no Selenium for Avalonia on Linux, and that is a measurement
// rather than an impression: Avalonia 11.2.3's X11 backend ships **no
// AT-SPI bridge** (`strings Avalonia.X11.dll | grep -ci atspi` is 0, and
// Avalonia.FreeDesktop carries none either), so the accessibility tree —
// which is how dogtail/pyatspi would drive any other GTK/Qt app — does
// not exist to be driven. Appium and FlaUI are Windows-only. Building the
// driver IS the standard answer for this stack; it is not a workaround
// for not having found the tool.
//
// # What was missing, precisely
//
// This repo already had: a container with real X11 + Skia + a render
// thread (`run-xvfb-smoke.sh`), real pointer input through the real X
// server (the xdotool click fuzz), crash capture, and a two-container
// harness driving two peers over real TCP with real grants
// (`scripts/twopeer-sync.sh`).
//
// The one absent piece was a way to *command the app and read it back*.
// `SmokeDriver` is compiled in, selected by an env var, fires once and
// reports nothing but an exit code — and it calls the model methods
// UNDER the controls, so by construction it cannot see the panel layer.
// The headless xunit suite can press a real button, but has no X11 and
// no render thread and lives in the test process. So the operator's own
// path — the GUI, two machines, real grants, across a restart — had
// never been run by anything but the operator.
//
// # The division of labour, and why clicks are xdotool's job
//
// This driver RESOLVES and READS. It does not synthesise input.
//
//	resolve   "the Share button"  ->  screen rectangle   (this process)
//	press     screen rectangle    ->  X server           (xdotool)
//
// Injecting a fabricated pointer event into Avalonia's pipeline would be
// cheaper and would also be the thing under test lying about itself: the
// 2026-08-21 SIGSEGV arrived BEFORE the panel's own first log line, in
// the dispatch that a fabricated event skips. So the driver's only job is
// turning a name into coordinates, which is the one thing an outside
// harness cannot work out for itself, and the press goes through the same
// X server the click fuzz uses. If xdotool is not installed the command
// FAILS — it does not quietly fall back to calling the handler, because a
// harness that silently stops testing input is worse than one that stops.
//
// # Off by default, and loud when on
//
// A process that accepts commands on a socket is not something to be in
// by accident. `WB_UI_DRIVER` is unset in every shipped path; when it is
// set the app says so on stderr and in the crash trail, so a run log can
// never leave you wondering whether the session was under automation.
//
// # Protocol
//
// Request:  one line, TAB-separated: VERB<TAB>ARG<TAB>ARG
// Response: one line of JSON, always carrying "ok".
//
// Tab-separated because the harness is bash talking over /dev/tcp with no
// tools in the container to help it, and because every argument we pass
// (selectors, directory paths, panel prose) contains spaces and quotes
// and non-ASCII, and none of them contain tabs.
//
//	ping                          is anyone home, and which build
//	tree [maxNodes]               the visual tree — the authoring tool
//	find    <sel>                 every match, with screen rectangles
//	where   <sel>                 the one match's rectangle (fails on 0 or >1)
//	text    <sel>                 that control's own text
//	alltext <sel>                 that control's whole subtree, joined
//	wait    <sel> [ms]            until it exists and is visible
//	waitgone <sel> [ms]           until it is gone
//	waittext <sel> <substr> [ms]  until its subtree text contains substr
//	select  <sel> <text|index>    choose a row; NOT real input, and says so
//	click   <sel>                 real X11 press+release at its centre
//	type    <sel> <text>          click to focus, then real keystrokes
//	settext <sel> <text>          NOT real input; response says so
//	key     <keys>                xdotool key spec, e.g. Return
//	panels                        what each slot is showing
//	panel   <slot> <name>         switch a slot to a registered panel
//	close                         shut the window down the operator's way
//	quit                          close this connection
//
// Selector grammar, smallest thing that addresses this app's controls:
//
//	Button                 by control type
//	Button[2]              the third one, in visual-tree order
//	#shareGo               by AutomationProperties.AutomationId
//	Button#shareGo         both
//	Button:Share a folder  type + text CONTAINS (case-insensitive)
//	Button:=Share          type + text EXACTLY equals
//	:=Share                text exactly, any type
//
// AutomationIds are the stable form and should win wherever a control is
// worth addressing twice. Text selectors work today against a frontend
// that has none, which is what makes this adoptable incrementally rather
// than as a 50-file prerequisite — and the ids, when added, are real
// accessibility metadata rather than test scaffolding.
public static class UiDriver
{
    private static MainWindow? _window;
    private static PeerView? _peer;
    private static TcpListener? _listener;

    // Bound address, kept so the startup banner can state the port that
    // was actually bound rather than the one that was asked for.
    private static string _bound = "";

    public static bool IsRunning => _listener is not null;

    // MaybeStart is called from MainWindow's construction, on the same
    // timer that starts SmokeDriver. Returns false — silently, that is
    // the normal case — when WB_UI_DRIVER is unset.
    //
    // WB_UI_DRIVER accepts "9111" (loopback only) or "0.0.0.0:9111". A
    // container harness needs the second form: podman publishes from the
    // container's own interface, so a driver bound to 127.0.0.1 inside
    // the container is unreachable from the host no matter what -p says.
    public static bool MaybeStart(MainWindow window, PeerView peer)
    {
        var spec = Environment.GetEnvironmentVariable("WB_UI_DRIVER");
        if (string.IsNullOrWhiteSpace(spec)) return false;

        if (!TryParseEndpoint(spec, out var ep, out var why))
        {
            Console.Error.WriteLine($"entity-avalonia: WB_UI_DRIVER={spec} is not an address ({why}) — driver NOT started");
            return false;
        }

        _window = window;
        _peer = peer;

        try
        {
            _listener = new TcpListener(ep);
            _listener.Start();
            _bound = _listener.LocalEndpoint.ToString() ?? ep.ToString();
        }
        catch (SocketException e)
        {
            Console.Error.WriteLine($"entity-avalonia: UI driver could not bind {ep}: {e.Message}");
            _listener = null;
            return false;
        }

        // Said on stderr AND in the crash trail. A session under
        // automation must never be mistakable for an operator's session
        // when someone reads the log afterwards.
        Console.Error.WriteLine($"entity-avalonia: UI DRIVER LISTENING on {_bound} — this process accepts automation commands");
        CrashDiagnostics.Breadcrumb("ui-driver", "listening on " + _bound);
        PanelLog.Write("ui-driver", "listening on " + _bound);

        var t = new Thread(AcceptLoop) { IsBackground = true, Name = "ui-driver" };
        t.Start();
        return true;
    }

    private static bool TryParseEndpoint(string spec, out IPEndPoint ep, out string why)
    {
        ep = new IPEndPoint(IPAddress.Loopback, 0);
        why = "";
        spec = spec.Trim();
        if (int.TryParse(spec, NumberStyles.Integer, CultureInfo.InvariantCulture, out var bare))
        {
            if (bare is < 1 or > 65535) { why = "port out of range"; return false; }
            ep = new IPEndPoint(IPAddress.Loopback, bare);
            return true;
        }
        var idx = spec.LastIndexOf(':');
        if (idx <= 0) { why = "expected PORT or HOST:PORT"; return false; }
        if (!int.TryParse(spec[(idx + 1)..], out var port) || port is < 1 or > 65535)
        {
            why = "port out of range";
            return false;
        }
        var host = spec[..idx];
        if (!IPAddress.TryParse(host, out var ip)) { why = $"'{host}' is not an IP literal"; return false; }
        ep = new IPEndPoint(ip, port);
        return true;
    }

    // One client at a time, on purpose. The harness is a serial script;
    // concurrent automation of one window is not a thing anybody wants,
    // and serialising here means every command's effect is settled before
    // the next is read.
    private static void AcceptLoop()
    {
        while (_listener is not null)
        {
            TcpClient client;
            try { client = _listener.AcceptTcpClient(); }
            catch (SocketException) { return; }
            catch (ObjectDisposedException) { return; }

            try { Serve(client); }
            catch (IOException) { /* the harness hung up */ }
            catch (Exception e) { PanelLog.Write("ui-driver", "client error: " + e.Message); }
            finally { try { client.Close(); } catch { /* already gone */ } }
        }
    }

    private static void Serve(TcpClient client)
    {
        client.NoDelay = true;
        using var stream = client.GetStream();
        using var reader = new StreamReader(stream, new UTF8Encoding(false));
        using var writer = new StreamWriter(stream, new UTF8Encoding(false)) { AutoFlush = true, NewLine = "\n" };

        string? line;
        while ((line = reader.ReadLine()) is not null)
        {
            if (line.Length == 0) continue;
            string reply;
            try { reply = Execute(line); }
            catch (Exception e)
            {
                // Never let a driver bug take the app down: the whole
                // point of this process is to survive long enough to be
                // asked what it is showing.
                reply = Err($"{e.GetType().Name}: {e.Message}");
            }
            if (reply.Length == 0) return;   // `quit`
            writer.WriteLine(reply);
        }
    }

    // --- Commands ----------------------------------------------------------

    private static string Execute(string line)
    {
        var parts = line.Split('\t');
        var verb = parts[0].Trim().ToLowerInvariant();
        string Arg(int i) => i < parts.Length ? parts[i] : "";
        int ArgInt(int i, int dflt)
            => int.TryParse(Arg(i), out var v) ? v : dflt;

        PanelLog.Write("ui-driver", "<- " + line.Replace('\t', ' '));

        switch (verb)
        {
            case "quit":
                return "";

            case "ping":
                return Ok(new Dictionary<string, object?>
                {
                    ["build"] = BuildInfo.Line,
                    ["bound"] = _bound,
                });

            case "tree":
                return OnUi(() =>
                {
                    var max = ArgInt(1, 500);
                    var nodes = Walk().Take(max).Select(Describe).ToList();
                    return Ok(new Dictionary<string, object?> { ["count"] = nodes.Count, ["nodes"] = nodes });
                });

            case "find":
                return OnUi(() =>
                {
                    var m = Match(Arg(1));
                    return Ok(new Dictionary<string, object?>
                    {
                        ["count"] = m.Count,
                        ["matches"] = m.Select(Describe).ToList(),
                    });
                });

            case "where":
                return OnUi(() =>
                {
                    if (!One(Arg(1), out var c, out var err)) return err;
                    return Ok(Describe(c));
                });

            // hittest — what does the framework think is at this control's
            // centre? The diagnostic for a refused click, and the reason
            // `click`'s own refusal now carries the same fields inline:
            // "something else is on top" is unactionable without knowing
            // WHAT, and a stack of z-ordered visuals at the point answers
            // it in one exchange instead of a screenshot and a guess.
            case "hittest":
                return OnUi(() => HitTest(Arg(1)));

            case "text":
                return OnUi(() =>
                {
                    if (!One(Arg(1), out var c, out var err)) return err;
                    return Ok(new Dictionary<string, object?> { ["text"] = TextOf(c) ?? "" });
                });

            case "alltext":
                return OnUi(() =>
                {
                    if (!One(Arg(1), out var c, out var err)) return err;
                    return Ok(new Dictionary<string, object?> { ["text"] = SubtreeText(c) });
                });

            case "wait":
                return Wait(Arg(1), ArgInt(2, 10000));

            case "waitgone":
                return WaitGone(Arg(1), ArgInt(2, 10000));

            case "select":
                return OnUi(() => SelectItem(Arg(1), Arg(2)));

            case "waittext":
                return WaitText(Arg(1), Arg(2), ArgInt(3, 10000));

            case "click":
                return Click(Arg(1));

            case "type":
                return Type(Arg(1), Arg(2));

            case "settext":
                return SetText(Arg(1), Arg(2));

            case "key":
                return Key(Arg(1));

            // close asks the window to shut down the way the operator's
            // close button does, through the Closing handler that calls
            // Bridge.Shutdown. A harness that instead kills the container
            // learns nothing about shutdown, and shutdown is where a
            // GUI's peer state is committed — so the scenario would be
            // unable to distinguish "it worked" from "it worked until
            // the process ended".
            case "close":
                Dispatcher.UIThread.Post(() => _window?.Close());
                return Ok(new Dictionary<string, object?> { ["closing"] = true });

            case "panels":
                return OnUi(PanelsState);

            case "panel":
                return OnUi(() => SwitchPanel(Arg(1), Arg(2)));

            default:
                return Err($"unknown verb '{verb}'");
        }
    }

    // --- Waiting -----------------------------------------------------------
    //
    // Polled from the DRIVER thread, marshalling one cheap query per poll,
    // rather than blocking the UI thread for the timeout. A wait that
    // stalls the dispatcher would prevent the very state change it is
    // waiting for — which is the classic form of this bug, and it presents
    // as "the harness times out on something that works by hand".

    private static string Wait(string sel, int timeoutMs)
    {
        var sw = Stopwatch.StartNew();
        while (true)
        {
            var n = OnUiValue(() => Match(sel).Count(Visible));
            if (n > 0)
                return Ok(new Dictionary<string, object?> { ["count"] = n, ["waitedMs"] = sw.ElapsedMilliseconds });
            if (sw.ElapsedMilliseconds >= timeoutMs)
                return Err($"timed out after {timeoutMs}ms waiting for '{sel}'");
            Thread.Sleep(100);
        }
    }

    // WaitGone is not the negation of Wait and cannot be written as one by
    // the harness. "It is not there yet" and "it has gone away" are the
    // same observation at different times, so a scenario that checks a
    // card is absent immediately after pressing Accept passes before the
    // panel has even refreshed. Waiting for the disappearance is the only
    // form that distinguishes the two.
    private static string HitTest(string sel)
    {
        if (!One(sel, out var c, out var err)) return err;
        var top = TopLevel.GetTopLevel(c);
        if (top is null) return Err($"'{sel}' is not in a window");
        var local = c.TranslatePoint(new Point(c.Bounds.Width / 2, c.Bounds.Height / 2), top);
        if (local is null) return Err($"'{sel}' has no position relative to the window");
        var p = local.Value;

        var stack = new List<string>();
        foreach (var v in top.GetVisualsAt(p))
        {
            stack.Add(v is Control vc
                ? $"{vc.GetType().Name}{(string.IsNullOrEmpty(AutomationProperties.GetAutomationId(vc)) ? "" : "#" + AutomationProperties.GetAutomationId(vc))}"
                : v.GetType().Name);
            if (stack.Count >= 12) break;
        }
        var topmost = top.InputHitTest(p) as Visual;

        return Ok(new Dictionary<string, object?>
        {
            ["x"] = Math.Round(p.X, 1),
            ["y"] = Math.Round(p.Y, 1),
            ["client"] = $"{top.ClientSize.Width:F0}x{top.ClientSize.Height:F0}",
            ["target"] = $"{c.GetType().Name} {c.Bounds.Width:F0}x{c.Bounds.Height:F0}",
            ["topmost"] = topmost is Control tc ? $"{tc.GetType().Name}:{Trim(TextOf(tc) ?? "", 30)}" : topmost?.GetType().Name ?? "(none)",
            ["visualsAt"] = stack,
            ["targetInStack"] = stack.Count > 0 && top.GetVisualsAt(p).Any(v => ReferenceEquals(v, c)),
        });
    }

    private static string WaitGone(string sel, int timeoutMs)
    {
        var sw = Stopwatch.StartNew();
        while (true)
        {
            var n = OnUiValue(() => Match(sel).Count(Visible));
            if (n == 0)
                return Ok(new Dictionary<string, object?> { ["waitedMs"] = sw.ElapsedMilliseconds });
            if (sw.ElapsedMilliseconds >= timeoutMs)
                return Err($"timed out after {timeoutMs}ms; '{sel}' is still showing {n} control(s)");
            Thread.Sleep(100);
        }
    }

    // SelectItem picks a row in a ComboBox or list, and reports
    // realInput:false because it is NOT the operator's gesture.
    //
    // The honest gesture is: click the ComboBox, which opens a popup, then
    // click the item. Avalonia hosts that popup in its own PopupRoot —
    // a separate TopLevel that this driver's walk, rooted at the main
    // window, does not reach. Rather than pretend, the field says so, and
    // any scenario using this is covering the panel's LOGIC and not its
    // combo box. Reaching popup roots is the follow-on work; it is not
    // done, and nothing here should read as though it were.
    private static string SelectItem(string sel, string want)
    {
        if (!One(sel, out var c, out var err)) return err;
        if (c is not Avalonia.Controls.Primitives.SelectingItemsControl sic)
            return Err($"'{sel}' is a {c.GetType().Name}, not something with a selection");

        var items = new List<object?>();
        if (sic.ItemsSource is not null)
            foreach (var o in sic.ItemsSource) items.Add(o);
        if (items.Count == 0) return Err($"'{sel}' has no items to choose from");

        int idx = -1;
        if (int.TryParse(want, out var n) && n >= 0 && n < items.Count) idx = n;
        else
        {
            for (int i = 0; i < items.Count; i++)
            {
                if ((items[i]?.ToString() ?? "").Contains(want, StringComparison.OrdinalIgnoreCase))
                {
                    idx = i;
                    break;
                }
            }
        }
        if (idx < 0)
        {
            var have = string.Join(" | ", items.Take(8).Select(o => Trim(o?.ToString() ?? "", 40)));
            return Err($"'{sel}' has no item matching '{want}' — it offers: {have}");
        }

        sic.SelectedIndex = idx;
        return Ok(new Dictionary<string, object?>
        {
            ["index"] = idx,
            ["item"] = Trim(items[idx]?.ToString() ?? "", 120),
            ["realInput"] = false,
        });
    }

    private static string WaitText(string sel, string needle, int timeoutMs)
    {
        var sw = Stopwatch.StartNew();
        var last = "";
        while (true)
        {
            last = OnUiValue(() =>
            {
                var m = Match(sel);
                return m.Count == 0 ? "" : string.Join(" ", m.Select(SubtreeText));
            });
            if (last.Contains(needle, StringComparison.OrdinalIgnoreCase))
                return Ok(new Dictionary<string, object?> { ["text"] = last, ["waitedMs"] = sw.ElapsedMilliseconds });
            if (sw.ElapsedMilliseconds >= timeoutMs)
                // The text we DID see is the whole diagnostic. A bare
                // "timed out" sends the reader back to a screenshot.
                return Err($"timed out after {timeoutMs}ms; '{sel}' says: {Trim(last, 400)}");
            Thread.Sleep(100);
        }
    }

    // --- Input -------------------------------------------------------------

    // Click scrolls the target into view, refuses a point OUTSIDE THE
    // WINDOW, and reports — without refusing on — what the framework
    // thinks is at that point.
    //
    // The bounds check is a hard gate and needs to be. A control's screen
    // rectangle is defined whether or not it is inside the viewport: a
    // button scrolled out of a ScrollViewer still has layout, still has
    // bounds, and `PointToScreen` still returns a plausible coordinate,
    // which may be off the window entirely. xdotool would click it, the
    // app would react to something else or to nothing, and the harness
    // would report a successful press of a button nobody touched. A
    // driver that can click the wrong thing and say `ok` manufactures
    // green.
    //
    // THE HIT TEST IS ADVISORY, AND THAT IS A MEASUREMENT, NOT A
    // COMPROMISE. The first version refused unless `InputHitTest` at the
    // intended point returned the target or a descendant. It was a FALSE
    // REFUSAL and it blocked the entire two-peer run on 2026-09-06.
    // Measured, with a reproducer:
    //
    //	panel first sync                -> hit test: AccessText:"Share a folder…"   (agrees)
    //	panel first peer-connections
    //	panel first sync                -> hit test: ScrollContentPresenter         (disagrees)
    //	xdotool mousemove 435 272 click 1 -> the form opens, the note updates
    //
    // After a panel is swapped out of a slot and back in, `InputHitTest`
    // answers with an ancestor for a control that real X11 input reaches
    // perfectly well, and it never recovers. Avalonia's actual input
    // routing and this extension are not the same machinery, so treating
    // the extension as an oracle for the router is wrong.
    //
    // A false refusal is the worse failure. It reads as rigor, it is
    // always attributed to the other side, and it leaves no wrong answer
    // to catch — the same shape as AP44, where four refusals phrased as
    // security properties made the cohort's only public registry
    // unreachable. So the observation is reported in every response and
    // the harness prints it; if a click ever does land on the wrong
    // control, the evidence is already in the transcript.
    private static string Click(string sel)
    {
        var prepared = OnUiValue(() =>
        {
            if (!One(sel, out var c, out var err)) return err;
            EnsureInViewport(c);
            return "";
        });
        if (prepared.Length > 0) return prepared;
        Settle();
        // A second pass: scrolling an outer viewer moves the inner one's
        // content, so one round is not always enough in a nested stack.
        OnUiValue(() =>
        {
            if (One(sel, out var c, out _)) EnsureInViewport(c);
            return "";
        });
        Settle();

        var resolved = OnUiValue<(bool ok, string err, PixelPoint pt, string label, bool agrees, string what)>(() =>
        {
            if (!One(sel, out var c, out var err)) return (false, err, default, "", false, "");
            if (!c.IsEffectivelyVisible || c.Bounds.Width <= 0 || c.Bounds.Height <= 0)
                return (false, Err($"'{sel}' is not visible"), default, "", false, "");
            if (ClippedAt(c, new Point(c.Bounds.Width / 2, c.Bounds.Height / 2)))
                return (false, Err(
                    $"'{sel}' is scrolled out of view and could not be scrolled back — " +
                    "clicking its coordinates would press whatever chrome is there instead"),
                    default, "", false, "");

            var top = TopLevel.GetTopLevel(c);
            if (top is null) return (false, Err($"'{sel}' is not in a window"), default, "", false, "");

            var local = c.TranslatePoint(new Point(c.Bounds.Width / 2, c.Bounds.Height / 2), top);
            if (local is null)
                return (false, Err($"'{sel}' has no position relative to the window"), default, "", false, "");

            var p = local.Value;
            var size = top.ClientSize;
            if (p.X < 0 || p.Y < 0 || p.X > size.Width || p.Y > size.Height)
                return (false, Err(
                    $"'{sel}' sits at ({p.X:F0},{p.Y:F0}) which is outside the {size.Width:F0}x{size.Height:F0} " +
                    "window - it is laid out off-screen and BringIntoView did not reach it"),
                    default, "", false, "");

            var hit = top.InputHitTest(p) as Visual;
            var onTarget = false;
            for (var v = hit; v is not null; v = v.GetVisualParent())
            {
                if (ReferenceEquals(v, c)) { onTarget = true; break; }
            }
            var what = hit is Control hc
                ? $"{hc.GetType().Name}:{Trim(TextOf(hc) ?? "", 40)}"
                : hit?.GetType().Name ?? "(nothing)";

            return (true, "", top.PointToScreen(p), Trim(TextOf(c) ?? c.GetType().Name, 60), onTarget, what);
        });
        if (!resolved.ok) return resolved.err;

        var centre = resolved.pt;
        var run = Xdotool("mousemove", centre.X.ToString(CultureInfo.InvariantCulture),
                                       centre.Y.ToString(CultureInfo.InvariantCulture),
                          "click", "1");
        if (run is not null) return Err(run);

        Settle();
        return Ok(new Dictionary<string, object?>
        {
            ["x"] = centre.X,
            ["y"] = centre.Y,
            ["label"] = resolved.label,
            ["realInput"] = true,
            // Advisory, never a gate. See the note above Click: this
            // disagrees with real input routing after a panel swap, and
            // refusing on it blocked a whole two-peer run for a click
            // that demonstrably works.
            ["hitTestAgrees"] = resolved.agrees,
            ["hitTestSays"] = resolved.what,
        });
    }

    private static string Type(string sel, string text)
    {
        var clicked = Click(sel);
        if (!clicked.Contains("\"ok\":true", StringComparison.Ordinal)) return clicked;

        // Select-all then type, so `type` REPLACES rather than appending
        // to whatever the control was pre-filled with. An offer card's
        // directory box arrives pre-filled with a suggestion, and a
        // harness that appends to it produces a path nobody can read and
        // a failure nobody can explain.
        var sa = Xdotool("key", "--clearmodifiers", "ctrl+a");
        if (sa is not null) return Err(sa);
        var run = Xdotool("type", "--clearmodifiers", "--delay", "20", "--", text);
        if (run is not null) return Err(run);

        Settle();
        return Ok(new Dictionary<string, object?> { ["typed"] = text, ["realInput"] = true });
    }

    private static string Key(string keys)
    {
        var run = Xdotool("key", "--clearmodifiers", keys);
        if (run is not null) return Err(run);
        Settle();
        return Ok(new Dictionary<string, object?> { ["key"] = keys, ["realInput"] = true });
    }

    // SetText writes the property directly. It is NOT input, the response
    // says so in a field a harness can assert on, and it exists only for
    // the case where a real keystroke is not what is being tested — a long
    // absolute path typed at 20ms/char is thirty seconds of nothing.
    //
    // Anything that claims to test the input path must use `type`.
    private static string SetText(string sel, string text) => OnUi(() =>
    {
        if (!One(sel, out var c, out var err)) return err;
        if (c is not TextBox tb) return Err($"'{sel}' is a {c.GetType().Name}, not a TextBox");
        tb.Text = text;
        return Ok(new Dictionary<string, object?> { ["text"] = text, ["realInput"] = false });
    });

    // Xdotool returns null on success, or the error string to report.
    //
    // A missing xdotool is an ERROR and never a fallback. A harness whose
    // input silently stopped being real input would keep passing while
    // covering nothing, which is the exact failure this whole driver
    // exists to end.
    private static string? Xdotool(params string[] args)
    {
        var psi = new ProcessStartInfo("xdotool")
        {
            RedirectStandardError = true,
            RedirectStandardOutput = true,
            UseShellExecute = false,
        };
        foreach (var a in args) psi.ArgumentList.Add(a);

        try
        {
            using var p = Process.Start(psi);
            if (p is null) return "xdotool did not start";
            if (!p.WaitForExit(10000))
            {
                try { p.Kill(true); } catch { /* already gone */ }
                return "xdotool timed out after 10s";
            }
            if (p.ExitCode != 0)
                return $"xdotool exited {p.ExitCode}: {Trim(p.StandardError.ReadToEnd(), 200)}";
            return null;
        }
        catch (System.ComponentModel.Win32Exception)
        {
            return "xdotool is not installed in this image — real input is unavailable, "
                 + "and this command will not fall back to invoking the handler directly";
        }
    }

    // Settle lets the click's consequences run before the response is
    // written. A Background-priority no-op returns after layout and
    // render have been serviced, which is what makes the harness's next
    // `text` read the post-click state rather than racing it.
    private static void Settle()
    {
        try
        {
            Dispatcher.UIThread.InvokeAsync(() => { }, DispatcherPriority.Background)
                .GetTask().Wait(5000);
        }
        catch (Exception) { /* a shutting-down dispatcher is not our problem */ }
    }

    // --- Panels ------------------------------------------------------------

    private static string PanelsState()
    {
        var stack = _peer?.PanelStackForSmoke;
        if (stack is null) return Err("no peer view");
        var names = new List<string>();
        for (int i = 0; i < stack.SlotCountForTests; i++)
            names.Add(stack.SlotAtForTests(i).CurrentPanelName);
        return Ok(new Dictionary<string, object?> { ["slots"] = names });
    }

    // SwitchPanel drives the slot's own SwitchTo, the same method the
    // slot's picker calls. It is not the operator's gesture — opening the
    // picker and choosing a row is — and a harness that wants to test the
    // picker should click it. This is here so a scenario about SHARING
    // does not spend its first ten steps on panel chrome.
    private static string SwitchPanel(string slotSpec, string panel)
    {
        var stack = _peer?.PanelStackForSmoke;
        if (stack is null) return Err("no peer view");
        var n = stack.SlotCountForTests;
        if (n == 0) return Err("no panel slots are open");

        int idx;
        switch (slotSpec.Trim().ToLowerInvariant())
        {
            case "first": case "top": case "middle": idx = 0; break;
            case "last": case "bottom": idx = n - 1; break;
            default:
                if (!int.TryParse(slotSpec, out idx)) return Err($"'{slotSpec}' is not a slot");
                break;
        }
        if (idx < 0 || idx >= n) return Err($"slot {idx} out of range (0..{n - 1})");

        stack.SlotAtForTests(idx).SwitchTo(panel);
        var got = stack.SlotAtForTests(idx).CurrentPanelName;
        if (!string.Equals(got, panel, StringComparison.Ordinal))
            // SwitchTo is tolerant of an unknown name by design (a layout
            // written by a newer build must still open). Silence here
            // would hand the harness a slot showing something else.
            return Err($"slot {idx} is showing '{got}', not '{panel}' — is that panel registered?");
        return Ok(new Dictionary<string, object?> { ["slot"] = idx, ["panel"] = got });
    }

    // --- The visual tree ---------------------------------------------------

    private static IEnumerable<Control> Walk()
    {
        if (_window is null) yield break;
        var stack = new Stack<Visual>();
        stack.Push(_window);
        while (stack.Count > 0)
        {
            var v = stack.Pop();
            if (v is Control c) yield return c;
            // Pushed in reverse so the yielded order is the visual order,
            // which is what makes `Button[2]` mean the same control twice
            // running.
            var kids = v.GetVisualChildren().ToList();
            for (int i = kids.Count - 1; i >= 0; i--) stack.Push(kids[i]);
        }
    }

    private static List<Control> Match(string selector)
    {
        var sel = Parsed.From(selector);
        var all = new List<Control>();
        foreach (var c in Walk())
        {
            if (sel.Type.Length > 0 && !TypeMatches(c, sel.Type)) continue;
            if (sel.Id.Length > 0 &&
                !string.Equals(AutomationProperties.GetAutomationId(c) ?? "", sel.Id, StringComparison.Ordinal))
                continue;
            if (sel.HasText)
            {
                var t = TextOf(c);
                if (t is null) continue;
                if (sel.ExactText)
                {
                    if (!string.Equals(t.Trim(), sel.Text, StringComparison.OrdinalIgnoreCase)) continue;
                }
                else if (!t.Contains(sel.Text, StringComparison.OrdinalIgnoreCase)) continue;
            }
            all.Add(c);
        }
        if (sel.Index >= 0)
            return sel.Index < all.Count ? new List<Control> { all[sel.Index] } : new List<Control>();
        return all;
    }

    // One resolves to exactly one control, and REFUSES an ambiguous
    // selector rather than taking the first match.
    //
    // Taking the first is the tempting default and it is how a harness
    // ends up green while clicking the wrong button: "Share a folder…"
    // and "Share" both contain "Share", and a driver that quietly picks
    // one has made the scenario's meaning depend on visual-tree order.
    private static bool One(string sel, out Control control, out string err)
    {
        var m = Match(sel);
        if (m.Count == 1) { control = m[0]; err = ""; return true; }
        control = null!;
        if (m.Count == 0) { err = Err($"no control matches '{sel}'"); return false; }
        var names = string.Join(" | ", m.Take(5).Select(c => $"{c.GetType().Name}:{Trim(TextOf(c) ?? "", 40)}"));
        err = Err($"'{sel}' matches {m.Count} controls — narrow it with [n] or := : {names}");
        return false;
    }

    private static bool TypeMatches(Control c, string name)
    {
        for (var t = c.GetType(); t is not null && t != typeof(object); t = t.BaseType)
        {
            if (string.Equals(t.Name, name, StringComparison.OrdinalIgnoreCase)) return true;
        }
        return false;
    }

    // Visible means ON SCREEN, which is not the same as laid out.
    //
    // `IsEffectivelyVisible` and non-zero Bounds are both true for a
    // control that has been scrolled out of its ScrollViewer: it has
    // layout, it has a size, and `PointToScreen` returns a coordinate
    // pointing at whatever chrome now occupies that pixel. On 2026-09-06
    // that put a click on the Sync panel's Share button 218px above the
    // button, on the panel slot's header — the driver reported `ok`, the
    // app logged no press at all, and the run went on to fail six checks
    // downstream with no indication that the cause was one bad click.
    //
    // So every ancestor that clips is checked. This is pure layout
    // arithmetic on purpose: the compositor's own hit test would answer
    // the same question, and it is unreliable here (see Click).
    private static bool Visible(Control c)
        => c.IsEffectivelyVisible && c.Bounds.Width > 0 && c.Bounds.Height > 0
           && !ClippedAt(c, new Point(c.Bounds.Width / 2, c.Bounds.Height / 2));

    private static bool ClippedAt(Control c, Point ptInC)
    {
        for (Visual? v = c.GetVisualParent(); v is not null; v = v.GetVisualParent())
        {
            if (!v.ClipToBounds) continue;
            var p = c.TranslatePoint(ptInC, v);
            if (p is null) return true;
            var b = v.Bounds;
            if (p.Value.X < 0 || p.Value.Y < 0 || p.Value.X > b.Width || p.Value.Y > b.Height)
                return true;
        }
        return false;
    }

    // EnsureInViewport scrolls the control into view the way a person
    // would before reaching for it.
    //
    // `BringIntoView()` alone was not enough — it is a routed request an
    // ancestor may or may not act on, and in this app's nested
    // PanelStack-over-panel-ScrollViewer arrangement it left the target
    // clipped. So after asking politely, each ancestor ScrollViewer's
    // Offset is set directly, innermost first, which is arithmetic
    // nobody has to be persuaded to honour.
    private static void EnsureInViewport(Control c)
    {
        c.BringIntoView();
        for (Visual? v = c.GetVisualParent(); v is not null; v = v.GetVisualParent())
        {
            if (v is not ScrollViewer sv) continue;
            var p = c.TranslatePoint(new Point(0, 0), sv);
            if (p is null) continue;

            // Position of the control in the scroller's CONTENT space:
            // where it sits relative to the viewport, plus how far the
            // viewport has already been scrolled.
            var posX = p.Value.X + sv.Offset.X;
            var posY = p.Value.Y + sv.Offset.Y;
            var offX = sv.Offset.X;
            var offY = sv.Offset.Y;

            if (posY < offY) offY = posY;
            else if (posY + c.Bounds.Height > offY + sv.Viewport.Height)
                offY = posY + c.Bounds.Height - sv.Viewport.Height;

            if (posX < offX) offX = posX;
            else if (posX + c.Bounds.Width > offX + sv.Viewport.Width)
                offX = posX + c.Bounds.Width - sv.Viewport.Width;

            if (offX < 0) offX = 0;
            if (offY < 0) offY = 0;
            sv.Offset = new Vector(offX, offY);
        }
    }

    private static bool Centre(Control c, out PixelPoint pt)
    {
        pt = default;
        var top = TopLevel.GetTopLevel(c);
        if (top is null) return false;
        var local = c.TranslatePoint(new Point(c.Bounds.Width / 2, c.Bounds.Height / 2), top);
        if (local is null) return false;
        pt = top.PointToScreen(local.Value);
        return true;
    }

    private static Dictionary<string, object?> Describe(Control c)
    {
        var d = new Dictionary<string, object?>
        {
            ["type"] = c.GetType().Name,
            ["id"] = AutomationProperties.GetAutomationId(c) ?? "",
            ["name"] = c.Name ?? "",
            ["text"] = Trim(TextOf(c) ?? "", 200),
            ["visible"] = c.IsEffectivelyVisible,
            ["enabled"] = c.IsEffectivelyEnabled,
            ["w"] = Math.Round(c.Bounds.Width, 1),
            ["h"] = Math.Round(c.Bounds.Height, 1),
        };
        if (Centre(c, out var p)) { d["cx"] = p.X; d["cy"] = p.Y; }
        return d;
    }

    // TextOf is "what a person reads on this control", and returning null
    // means "this control has no text of its own" — which is different
    // from the empty string, and the difference decides whether a text
    // selector considers the control at all.
    private static string? TextOf(Control c) => c switch
    {
        TextBlock tb => tb.Text,
        TextBox tb => tb.Text,
        ComboBox cb => cb.SelectionBoxItem?.ToString(),
        ContentControl cc => cc.Content as string,
        _ => null,
    };

    // SubtreeText is WHAT AN OPERATOR CAN READ, so it skips anything that
    // is not effectively visible — the whole subtree, not just the node.
    //
    // The first version did not, and it read the Sync panel's "Offered to
    // you / nothing is being offered to you right now" section out of a
    // panel where that section is deliberately hidden until an offer
    // arrives. A prose assertion satisfied by text nobody can see is not
    // a weaker test, it is a wrong one: it would pass against a panel
    // that had stopped displaying the thing entirely.
    private static string SubtreeText(Control root)
    {
        var sb = new StringBuilder();
        var stack = new Stack<Visual>();
        stack.Push(root);
        while (stack.Count > 0)
        {
            var v = stack.Pop();
            if (v is Control skip && !skip.IsEffectivelyVisible) continue;
            if (v is Control c)
            {
                var t = TextOf(c);
                if (!string.IsNullOrWhiteSpace(t))
                {
                    if (sb.Length > 0) sb.Append(" · ");
                    sb.Append(t.Trim());
                }
            }
            var kids = v.GetVisualChildren().ToList();
            for (int i = kids.Count - 1; i >= 0; i--) stack.Push(kids[i]);
        }
        return sb.ToString();
    }

    // --- Plumbing ----------------------------------------------------------

    private static string OnUi(Func<string> f) => OnUiValue(f);

    private static T OnUiValue<T>(Func<T> f)
    {
        // The driver thread blocks here; the UI thread does the work.
        // Never the other way round.
        return Dispatcher.UIThread.InvokeAsync(f).GetTask().GetAwaiter().GetResult();
    }

    // Relaxed escaping, deliberately. The default encoder escapes every
    // non-alphanumeric to \uXXXX, so a panel's own prose — the thing this
    // driver exists to let somebody read — arrives in the transcript as
    // `“Share a folder…”` and a build stamp arrives as
    // `a326484-dirty+cd7141fe`. The response is still one line and
    // still valid JSON; it is now also legible to the person reading the
    // failure.
    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        WriteIndented = false,
        Encoder = System.Text.Encodings.Web.JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    private static string Ok(Dictionary<string, object?> fields)
    {
        var d = new Dictionary<string, object?> { ["ok"] = true };
        foreach (var kv in fields) d[kv.Key] = kv.Value;
        return JsonSerializer.Serialize(d, JsonOpts);
    }

    private static string Err(string message)
        => JsonSerializer.Serialize(new Dictionary<string, object?>
        {
            ["ok"] = false,
            ["error"] = message,
        }, JsonOpts);

    private static string Trim(string s, int max)
    {
        s = s.Replace('\n', ' ').Replace('\r', ' ').Replace('\t', ' ');
        return s.Length <= max ? s : s[..max] + "…";
    }

    // Parsed selector. See the grammar in the class doc.
    private readonly struct Parsed
    {
        public string Type { get; init; }
        public string Id { get; init; }
        public string Text { get; init; }
        public bool HasText { get; init; }
        public bool ExactText { get; init; }
        public int Index { get; init; }

        public static Parsed From(string selector)
        {
            selector ??= "";
            var text = "";
            var hasText = false;
            var exact = false;

            var colon = selector.IndexOf(':');
            var head = selector;
            if (colon >= 0)
            {
                head = selector[..colon];
                text = selector[(colon + 1)..];
                hasText = true;
                if (text.StartsWith('=')) { exact = true; text = text[1..]; }
            }

            var index = -1;
            var lb = head.IndexOf('[');
            if (lb >= 0 && head.EndsWith(']'))
            {
                if (int.TryParse(head[(lb + 1)..^1], out var n)) index = n;
                head = head[..lb];
            }

            var id = "";
            var hash = head.IndexOf('#');
            if (hash >= 0)
            {
                id = head[(hash + 1)..];
                head = head[..hash];
            }

            return new Parsed
            {
                Type = head.Trim(),
                Id = id.Trim(),
                Text = text,
                HasText = hasText,
                ExactText = exact,
                Index = index,
            };
        }
    }
}
