using System;
using System.IO;
using System.Text.Json;
using Avalonia;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Themes.Fluent;
using Avalonia.X11;
using EntityAvalonia.Panels;

namespace EntityAvalonia;

public static class Program
{
    // Resolved config — built from argv in Main, consumed by MainWindow
    // when it formats the status line. The JSON blob itself is what
    // crosses the FFI boundary to BridgeInit.
    public static BridgeConfig Config { get; private set; } = new();
    public static string ConfigJson { get; private set; } = "";

    // DefaultListenAddr — the port this app listens on when nobody said.
    //
    // A fixed port rather than an ephemeral one because a fixed port is
    // something an operator can write in a firewall rule and type into
    // another machine. It falls back to an ephemeral port when taken (see
    // ListenFallback), so a second instance on one machine still starts —
    // which is the case that would otherwise make a fixed default hostile.
    public const string DefaultListenAddr = "0.0.0.0:9110";

    private const string Usage = @"Usage:
  entity-avalonia [flags]

By default this is a PERSISTENT, REACHABLE peer: the same peer-id every
launch, an on-disk store, an inbound listener, and an mDNS announcement so
peers on your LAN can find it without being told an address. Sharing a
folder is not possible without all four, and until 2026-09-03 the default
had none of them — every launch was a brand-new peer-id, so every grant,
offer and share from the previous session named a peer that no longer
existed.

Flags:
  --identity NAME      Use an EXISTING named identity from
                       ~/.entity/identities/. Startup FAILS if there is no
                       such identity — a peer-id is what every grant, mount
                       and offer on another machine names, so a typo must
                       not quietly become a different peer.
                       With no flag: ""default"", created on first launch.
  --new-identity NAME  Same, but create it if it does not exist. This
                       brings a NEW PEER into being, with a new peer-id
                       and an empty tree; nothing the old one owned
                       follows it.
  --alias NAME         Alias for the in-process peer in the shell
                       (default: --identity name, or ""self"")
  --storage KIND       Storage backend: ""sqlite"" (default) or ""memory""
  --storage-path PATH  SQLite DB path (default:
                       ~/.entity/peers/NAME/store.db).
  --listen ADDR        TCP listener for inbound peer connections
                       (default: " + DefaultListenAddr + @"). An address given
                       here is taken literally: if it is in use, startup
                       fails rather than silently moving to another port.
  --no-listen          Outbound only. Nobody can reach this peer, and no
                       folder can be shared TO it.
  --advertise URL      The dial URL published as this peer's transport
                       profile. Defaults to this host's LAN address plus
                       the bound port — set it behind NAT or a proxy.
  --ephemeral          Throwaway peer: in-memory store, fresh keypair,
                       no listener. Everything is lost on exit. This was
                       the default until 2026-09-03.
  --open-access        DEV: grant wildcard caps to connecting peers.
  -h, --help           Show this message and exit.
";

    [System.STAThread]
    public static int Main(string[] args)
    {
        // FIRST thing, before argv parsing and before Avalonia exists.
        // A crash during startup is exactly as undiagnosable as one an
        // hour in, and this costs nothing when nothing goes wrong.
        CrashDiagnostics.Install();

        // Immediately after, and before anything can go wrong: say which
        // build this is. Every crash artifact, every operator screenshot and
        // every "is the fix in?" question needs this line, and until now the
        // only way to answer was to guess from a timestamp on dist-native/.
        // It goes to BOTH stderr (the run log) and the crash trail, because
        // the two are read in different situations.
        Console.Error.WriteLine("entity-avalonia: " + BuildInfo.Line);
        CrashDiagnostics.Breadcrumb("build", BuildInfo.Line);

        if (!ParseArgs(args, out var avaloniaArgs))
        {
            Console.Error.Write(Usage);
            return 2;
        }

        ConfigJson = JsonSerializer.Serialize(Config);

        if (Config.OpenAccess)
        {
            Console.Error.WriteLine(
                "entity-avalonia: WARNING — running with --open-access; all connecting " +
                "peers receive wildcard capabilities (dev only)");
        }

        return BuildAvaloniaApp().StartWithClassicDesktopLifetime(avaloniaArgs);
    }

    public static AppBuilder BuildAvaloniaApp()
    {
        var builder = AppBuilder.Configure<App>()
            .UsePlatformDetect()
            .WithInterFont()
            .LogToTrace();

        // RENDER MODE — software by default since 2026-08-19, GPU by opt-in.
        //
        // GPU rendering (mesa hardware GL) intermittently SIGSEGVs on some
        // drivers. The render itself is proven crash-free in software Skia (the
        // headless ProgramPanelStressTests rasterize a 64×64 grid 400× with no
        // GPU, and the Xvfb smoke runs use llvmpipe software GL), so the fault
        // is the driver path, not our paint code.
        //
        // **The default is inverted for release**, and this was an open product
        // call in STATUS rather than an oversight. The reasoning: we do not own
        // the faulting code and cannot fix it, we cannot reliably detect a bad
        // driver from in-process (auto-detect would mean fingerprinting mesa
        // versions, which is a guess that ages badly), and the two outcomes are
        // not symmetric — the cost of software render is frames per second on a
        // desktop app that is mostly static text and small grids, while the cost
        // of the GPU path on an affected driver is a hard crash that takes the
        // user's session with it. A slow window beats a dead one.
        //
        // Reversible in one env var, both directions:
        //   WB_GPU_RENDER=1       — opt back into hardware GL (what to try first
        //                           when someone reports sluggish rendering)
        //   WB_SOFTWARE_RENDER=1  — still honored; it is now the default, so
        //                           setting it is a no-op kept for the scripts
        //                           and docs that already pass it
        //
        // The chosen mode is announced on stderr because it is the first thing
        // a crash report needs and the last thing a reporter thinks to include.
        var gpuOptIn = !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WB_GPU_RENDER"));
        if (gpuOptIn)
        {
            Console.Error.WriteLine(
                "entity-avalonia: render mode = GPU (hardware GL, WB_GPU_RENDER set) — " +
                "if this session crashes in the driver, unset it to fall back to software");
        }
        else
        {
            builder = builder.With(new X11PlatformOptions
            {
                RenderingMode = new[] { X11RenderingMode.Software },
            });
            Console.Error.WriteLine(
                "entity-avalonia: render mode = software Skia (default; set WB_GPU_RENDER=1 for hardware GL)");
        }
        return builder;
    }

    // ParseArgs strips our flags out of args and passes the rest through
    // to Avalonia (so things like --help-avalonia or future avalonia
    // flags still work). Unknown args go through too — Avalonia ignores
    // unknown by default.
    private static bool ParseArgs(string[] args, out string[] remaining)
    {
        _ephemeral = false;
        _listenExplicit = false;
        var passthrough = new System.Collections.Generic.List<string>();
        for (int i = 0; i < args.Length; i++)
        {
            var a = args[i];
            switch (a)
            {
                case "-h":
                case "--help":
                    remaining = passthrough.ToArray();
                    Console.Write(Usage);
                    Environment.Exit(0);
                    return true;
                case "--identity":
                    if (!TakeValue(args, ref i, a, out var ident)) { remaining = Array.Empty<string>(); return false; }
                    Config.Identity = ident;
                    break;
                case "--new-identity":
                    if (!TakeValue(args, ref i, a, out var newIdent)) { remaining = Array.Empty<string>(); return false; }
                    Config.Identity = newIdent;
                    Config.CreateIdentity = true;
                    break;
                case "--alias":
                    if (!TakeValue(args, ref i, a, out var alias)) { remaining = Array.Empty<string>(); return false; }
                    Config.Alias = alias;
                    break;
                case "--storage":
                    if (!TakeValue(args, ref i, a, out var sk)) { remaining = Array.Empty<string>(); return false; }
                    Config.Storage = sk;
                    break;
                case "--storage-path":
                    if (!TakeValue(args, ref i, a, out var sp)) { remaining = Array.Empty<string>(); return false; }
                    Config.StoragePath = sp;
                    break;
                case "--listen":
                    if (!TakeValue(args, ref i, a, out var ln)) { remaining = Array.Empty<string>(); return false; }
                    Config.Listen = ln;
                    // An address the operator typed is honoured exactly. The
                    // ephemeral fallback exists for the default nobody chose;
                    // applying it here would hide a port conflict and produce
                    // a peer unreachable at the address they wrote down.
                    Config.ListenFallback = false;
                    _listenExplicit = true;
                    break;
                case "--no-listen":
                    Config.Listen = "";
                    Config.ListenFallback = false;
                    _listenExplicit = true;
                    break;
                case "--advertise":
                    if (!TakeValue(args, ref i, a, out var adv)) { remaining = Array.Empty<string>(); return false; }
                    Config.Advertise = adv;
                    break;
                case "--ephemeral":
                    _ephemeral = true;
                    break;
                case "--open-access":
                    Config.OpenAccess = true;
                    break;
                default:
                    passthrough.Add(a);
                    break;
            }
        }
        remaining = passthrough.ToArray();
        ApplyDefaults();
        return true;
    }

    private static bool _ephemeral;
    private static bool _listenExplicit;

    // ApplyDefaults turns the parsed flags into the configuration the peer
    // is actually built from. It runs after parsing, never during, so the
    // order flags appear on the command line cannot change the result.
    //
    // THE DEFAULT IS A SERVICE, NOT A DEMO. Until 2026-09-03 it was the
    // reverse — memory store, ephemeral keypair, no listener — and the
    // consequence was not "some features are off". The whole tree is
    // peer-id-namespaced, so a fresh keypair per launch means the app was a
    // DIFFERENT PEER every time it started: mounts, grants, offers, accepted
    // shares and roster entries from the last session all named a peer-id
    // that no longer existed, and every one of them silently did nothing.
    // An operator debugging a two-machine share by restarting the app —
    // which is what anyone does — was destroying the state they were
    // debugging, on both machines, on every launch.
    //
    // `--ephemeral` keeps that behaviour for a throwaway peer, which is a
    // real and useful thing; it just is not what someone gets by default.
    private static void ApplyDefaults()
    {
        if (_ephemeral)
        {
            if (string.IsNullOrEmpty(Config.Storage)) Config.Storage = "memory";
            if (!_listenExplicit) { Config.Listen = ""; Config.ListenFallback = false; }
            return;
        }

        // Persistent store. shellboot substitutes the "default" identity for
        // an empty one under sqlite (and creates it on first use), so the
        // identity name has ONE definition and it is not duplicated here.
        if (string.IsNullOrEmpty(Config.Storage)) Config.Storage = "sqlite";

        if (!_listenExplicit && string.IsNullOrEmpty(Config.Listen))
        {
            Config.Listen = DefaultListenAddr;
            Config.ListenFallback = true;
        }

        // Re-establish every declared relationship at startup. This is the
        // answer to "why do I have to press connect again after every
        // launch", and it is on for a persistent peer because a persistent
        // peer is the only kind that HAS declarations to re-establish.
        Config.ReconcileOnStart = true;
    }

    private static bool TakeValue(string[] args, ref int i, string flag, out string value)
    {
        if (i + 1 >= args.Length)
        {
            Console.Error.WriteLine($"entity-avalonia: {flag} requires a value");
            value = "";
            return false;
        }
        value = args[++i];
        return true;
    }
}

// BridgeConfig field names match the JSON tags in ../bridge/main.go
// bridgeConfig. The serializer needs explicit property names because
// the Go side reads `identity`/`alias`/etc., not the C# PascalCase.
public class BridgeConfig
{
    [System.Text.Json.Serialization.JsonPropertyName("identity")]
    public string Identity { get; set; } = "";

    // Create the named identity if it does not exist. Set only by
    // --new-identity, never by --identity: loading names a peer that
    // exists, creating brings a new one into being, and the tree is
    // peer-id-namespaced so a typo under create-if-absent abandons
    // everything the intended peer owns. See shellboot.Config.
    [System.Text.Json.Serialization.JsonPropertyName("create_identity")]
    public bool CreateIdentity { get; set; }

    [System.Text.Json.Serialization.JsonPropertyName("alias")]
    public string Alias { get; set; } = "";

    [System.Text.Json.Serialization.JsonPropertyName("storage")]
    public string Storage { get; set; } = "";

    [System.Text.Json.Serialization.JsonPropertyName("storage_path")]
    public string StoragePath { get; set; } = "";

    [System.Text.Json.Serialization.JsonPropertyName("listen")]
    public string Listen { get; set; } = "";

    // Allow an ephemeral port when Listen is taken. Set only for the
    // DEFAULT address; an address the operator typed fails loudly instead.
    [System.Text.Json.Serialization.JsonPropertyName("listen_fallback")]
    public bool ListenFallback { get; set; }

    // The dial URL published as this peer's transport profile. Empty means
    // "derive it" — LAN address + bound port for a wildcard bind.
    [System.Text.Json.Serialization.JsonPropertyName("advertise")]
    public string Advertise { get; set; } = "";

    // Re-establish declared peers and folders once, in the background,
    // after the peer comes up. Off for an ephemeral peer, which by
    // definition has nothing declared to re-establish.
    [System.Text.Json.Serialization.JsonPropertyName("reconcile_on_start")]
    public bool ReconcileOnStart { get; set; }

    [System.Text.Json.Serialization.JsonPropertyName("open_access")]
    public bool OpenAccess { get; set; }
}

public class App : Application
{
    public override void Initialize()
    {
        Styles.Add(new FluentTheme());
        // Match the project's terminal-first aesthetic (entity-shell,
        // entity-console, canvas all assume dark). Force dark so we
        // don't depend on the user's OS theme — colors should look the
        // same wherever the renderer ships.
        RequestedThemeVariant = Avalonia.Styling.ThemeVariant.Dark;

        // Register the panels available to PanelSlot dropdowns.
        // Order here = order in the slot picker menu.
        PanelRegistry.Register("detail", "Detail",
            (handle, host) => new DetailPanel(handle, host),
            PanelRegistry.Category.ThisPeer, "The selected entity, decoded.");
        PanelRegistry.Register("peer-info", "Peer Info",
            (handle, _) => new PeerInfoPanel(handle),
            PanelRegistry.Category.ThisPeer, "This peer's identity, storage and listener.");
        PanelRegistry.Register("log-viewer", "Log Viewer",
            (handle, _) => new LogViewerPanel(handle),
            PanelRegistry.Category.Diagnostics, "This process's log stream.");
        PanelRegistry.Register("markdown-view", "Markdown View",
            (handle, host) => new MarkdownViewPanel(handle, host),
            PanelRegistry.Category.ThisPeer, "Renders the selected markdown entity.");
        PanelRegistry.Register("markdown-files", "Markdown Files",
            (handle, host) => new MarkdownFilesPanel(handle, host),
            PanelRegistry.Category.ThisPeer, "Markdown entities in this peer's tree.");
        PanelRegistry.Register("query-browser", "Query Browser",
            (handle, host) => new QueryBrowserPanel(handle, host),
            PanelRegistry.Category.Diagnostics, "Query this peer's tree by type and path.");
        PanelRegistry.Register("handler-browser", "Handler Browser",
            (handle, host) => new HandlerBrowserPanel(handle, host),
            PanelRegistry.Category.Diagnostics, "Handlers this peer has registered, and their ops.");
        PanelRegistry.Register("site-view", "Local Site (this peer's own)",
            (handle, host) => new SiteViewPanel(handle, host),
            PanelRegistry.Category.ThisPeer,
            "A site published by THIS peer. Opens the bundled demo. For sites out on "
            + "the network, use Browser.");
        // The reader's surface is above; this is the operator's. Same
        // published bytes, opposite question — "what does it say" vs
        // "is it serving what it signed". Both shapes exist on purpose;
        // the contrast is the UX research.
        PanelRegistry.Register("publisher-verify", "Origin Inspector (is it serving what it signed?)",
            (handle, host) => new PublisherVerifyPanel(handle, host),
            PanelRegistry.Category.Network,
            "Point it at an origin URL and it reports which link of the verification chain "
            + "held and what each one proves. It shows the CHAIN, never a page — use Browser "
            + "to read the content.");
        // The browser is the journey; Publisher Verify is the inspector.
        // Both stay: they answer different questions about the same
        // bytes, and an operator debugging an origin does not want a
        // page in the way.
        PanelRegistry.Register("browser", "Browser — registry + sites on the network",
            (handle, host) => new BrowserPanel(handle, host),
            PanelRegistry.Category.Network,
            "START HERE for anything remote. Type a domain (entitychurchregistry.org), walk "
            + "its registry, and open any name it carries.");
        PanelRegistry.Register("shell", "Shell",
            (handle, host) => new ShellPanel(handle, host),
            PanelRegistry.Category.ThisPeer, "The entity-shell REPL against this peer.");
        // The filesystem side of this peer. Registered next to the other
        // "this peer" surfaces because a mount is a fact about THIS
        // machine's disk, not about the network.
        // The explorer is registered BEFORE the mount manager, because it
        // is the one an operator wants first: "show me my files" is the
        // question, and "which directories are wired to which prefixes"
        // is the administration behind it. The two are separate panels
        // for the reason the browser trio is — each answers ONE question,
        // and the last time three panels read the same bytes without
        // saying which question they answered, an operator called the set
        // incomprehensible and was right.
        PanelRegistry.Register("file-explorer", "Files (browse a mounted folder)",
            (handle, host) => new FileExplorerPanel(handle, host),
            PanelRegistry.Category.ThisPeer,
            "The contents of a mounted directory, folder by folder, with a preview. Every "
            + "file says whether it became a document you can open — and if not, why not.");
        PanelRegistry.Register("local-files", "Local Files (manage mounts)",
            (handle, _) => new LocalFilesPanel(handle),
            PanelRegistry.Category.ThisPeer,
            "Directories on this machine wired into the tree by `mount` — where each one "
            + "ingests to, its filters, and how many entities it holds. Watcher liveness is "
            + "not knowable from the tree, so it is reported as unknown rather than assumed.");
        PanelRegistry.Register("peer-connections", "Peer Connections",
            (handle, _) => new PeerConnectionsPanel(handle),
            PanelRegistry.Category.Network, "Who this peer is connected to, from the tree's liveness record.");
        // Sync is the front door for the whole sharing job: the two
        // gestures and nothing else.
        //
        // It exists because five panels touched this one job — Shared
        // Folders (10 buttons), Sharing Status (5), Local Files (5),
        // Files (2), plus Peer Connections — over eighteen shell verbs.
        // Every one was added for a real reason and most are the scar
        // tissue of a defect this project actually hit, which is exactly
        // the trap: each was locally justified and the sum is unusable.
        //
        // The other four are not deleted, they are DEMOTED. Each answers
        // a real question you reach for after something breaks; none of
        // them is what a first-timer should open.
        PanelRegistry.Register("sync", "Sync — share a folder with a peer",
            (handle, host) => new SyncPanel(handle, host),
            PanelRegistry.Category.Network,
            "Share a folder with another machine, and accept the folders they share with "
            + "you. Updates itself — the folders, the offers and the state all come from "
            + "the tree, so there is nothing to refresh.");
        // Registered next to Peer Connections because that is where an
        // operator lands looking for it: connecting is the step before
        // sharing, and until this panel existed the flow simply stopped
        // there. Every verb behind it was shell-only.
        //
        // Superseded by Sync for the ordinary flow. Kept because it
        // exposes per-stage controls (resync, forget, unsync) that Sync
        // deliberately does not, and those are what you reach for when a
        // share will not establish.
        PanelRegistry.Register("share", "Shared Folders (every control, one stage at a time)",
            (handle, _) => new SharePanel(handle),
            PanelRegistry.Category.Diagnostics,
            "Offer a mounted folder to a peer, see what they are offering you, accept it, "
            + "and see what is arriving. Both peers must dial each other.");
        // Named for the question it answers, not for the machinery behind
        // it. With Peer Connections ("who am I connected to") and Shared
        // Folders ("share and receive") already here, a third panel called
        // "Sync" would repeat the Browser / Local Site / Origin Inspector
        // naming failure an operator called incomprehensible.
        PanelRegistry.Register("sharing-status", "Sharing Status (declared vs. actual)",
            (handle, _) => new SharingStatusPanel(handle),
            PanelRegistry.Category.Diagnostics,
            "What you declared — peers and folders — beside what is actually established, "
            + "and what is stopping the rest. What you grant a peer is stated exactly; what "
            + "they grant you is not knowable from here, so it is shown as what has arrived.");
        // The generic host: ONE panel class, every program, mounted from
        // descriptors.
        //
        // The three legacy per-program panels ("Snake (compute)" / "Life
        // (compute)" / "Asteroids (compute)") were registered here alongside
        // these until 2026-08-20, so the two paths could be compared live.
        // The comparison is done: the generic path carries the display, the
        // input ports AND the program-specific status readout (POP / LEN /
        // SCORE, projected in the tree), and equality with the hard-coded
        // models is pinned by frozen state-hash vectors that no longer need
        // the legacy code to exist (programs/oracle_vectors_test.go).
        //
        // Note what is NOT here: three panel classes. The only per-program thing
        // is the string, and it is used for authoring only.
        PanelRegistry.Register("program-life", "Life (generic host)",
            (handle, _) => new ProgramPanel(handle, "life"),
            PanelRegistry.Category.Programs, "");
        PanelRegistry.Register("program-snake", "Snake (generic host)",
            (handle, _) => new ProgramPanel(handle, "snake"),
            PanelRegistry.Category.Programs, "");
        PanelRegistry.Register("program-asteroids", "Asteroids (generic host)",
            (handle, _) => new ProgramPanel(handle, "asteroids"),
            PanelRegistry.Category.Programs, "");

        // The sharding floor on a screen: 64×64 Life, past the single-eval budget
        // cliff, mounting only because the host runs the static-k shard family.
        // Same ProgramPanel class as above — it never learns the program is
        // sharded (the descriptor's shard block is the host's concern). This is
        // the visual validation of the host-as-compute-kernel floor.
        PanelRegistry.Register("program-life-big", "Life 64×64 (sharded host)",
            (handle, _) => new ProgramPanel(handle, "life-big"),
            PanelRegistry.Category.Programs, "");

        // Interactive Life: a d-pad cursor + toggle/regen/pause action buttons,
        // one key-set input port — the standard controller (controls.go),
        // mounted through the SAME generic ProgramPanel as every other program.
        PanelRegistry.Register("program-life-edit", "Life (interactive)",
            (handle, _) => new ProgramPanel(handle, "life-edit"),
            PanelRegistry.Category.Programs, "");
    }

    public override void OnFrameworkInitializationCompleted()
    {
        // The dispatcher exists by now, so the UI-thread fault channel
        // can be hooked. Do it BEFORE constructing MainWindow — panel
        // mount is itself a place a fault can land.
        CrashDiagnostics.InstallDispatcher();

        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime desktop)
        {
            var window = new MainWindow();
            desktop.MainWindow = window;
            // Global input breadcrumbs. Tunnelled, so a click is recorded
            // before the target handler runs — the 2026-08-21 dump died
            // in that exact window and left no trace of the click.
            CrashDiagnostics.AttachInput(window);
        }
        base.OnFrameworkInitializationCompleted();
    }
}
