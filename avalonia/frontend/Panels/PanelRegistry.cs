using System;
using System.Collections.Generic;
using Avalonia.Controls;

namespace EntityAvalonia.Panels;

// PanelRegistry is the central catalog of panel types. Each entry maps
// a stable name (used in slot dropdowns + persisted layout state, when
// that lands) to a factory that constructs the panel against a peer
// handle and a host.
//
// This is the first piece of panel management — without a registry,
// every panel had to be hard-coded into PeerView's layout. With it,
// adding a new panel is "register + it shows up in every slot's
// dropdown."
//
// Scope today: per-peer panels only. PanelScope.System panels (event
// log, peer manager) will get a sibling registry when there's a system-
// peer host to register against — same shape, different lifetime.
public static class PanelRegistry
{
    // Category groups panels in the slot picker. Without it every panel
    // lands in one flat list — 18 rows with no ordering principle, which
    // is what the picker was until 2026-08-30 and what an operator
    // called "just a big-ass list of panels".
    //
    // The categories are by QUESTION the panel answers, not by
    // subsystem, because that is how someone opening the picker is
    // actually thinking: what is out there / what is on this peer / what
    // is this thing doing / let me run something.
    public static class Category
    {
        public const string Network = "Network & the outside world";
        public const string ThisPeer = "This peer";
        public const string Diagnostics = "Diagnostics";
        public const string Programs = "Programs";
    }

    public sealed record Entry(
        string Name,
        string DisplayName,
        Func<long, IPanelHost, Control> Factory,
        string Category = PanelRegistry.Category.ThisPeer,
        string Blurb = "");

    private static readonly List<Entry> _entries = new();

    public static void Register(string name, string displayName, Func<long, IPanelHost, Control> factory)
        => Register(name, displayName, factory, Category.ThisPeer, "");

    public static void Register(string name, string displayName,
        Func<long, IPanelHost, Control> factory, string category, string blurb)
    {
        // Replace if already registered — supports hot-reloading
        // during dev without leaking old entries.
        _entries.RemoveAll(e => e.Name == name);
        _entries.Add(new Entry(name, displayName, factory, category, blurb));
    }

    // CategoriesInOrder is the display order of the groups. Explicit
    // rather than alphabetical: "Network" first because reaching the
    // outside world is what someone opening a fresh workspace wants.
    public static IReadOnlyList<string> CategoriesInOrder() => new[]
    {
        Category.Network, Category.ThisPeer, Category.Diagnostics, Category.Programs,
    };

    // ByCategory returns the entries in a category, registration order.
    public static IReadOnlyList<Entry> ByCategory(string category)
    {
        var outp = new List<Entry>();
        foreach (var e in _entries)
        {
            if (e.Category == category) outp.Add(e);
        }
        return outp;
    }

    public static IReadOnlyList<Entry> All() => _entries;

    public static Entry? Get(string name)
    {
        foreach (var e in _entries)
        {
            if (e.Name == name) return e;
        }
        return null;
    }

    public static Control Create(string name, long peerHandle, IPanelHost host)
    {
        var entry = Get(name);
        if (entry == null)
        {
            return new TextBlock
            {
                Text = $"(unknown panel: {name})",
                Margin = new Avalonia.Thickness(12),
            };
        }
        return entry.Factory(peerHandle, host);
    }
}

// IPanelHost is the surface a panel uses to participate in cross-
// panel signals — chiefly tree↔detail selection forwarding, but extensible.
// Implemented by PeerView.
public interface IPanelHost
{
    // SelectedPath fires when the user selects an entity in the tree
    // (or any other source that wants to broadcast a path). Detail
    // panels subscribe to drive their ShowEntity calls. Subscribers
    // unsubscribe on dispose.
    event Action<string> SelectedPath;

    // CurrentSelectedPath is the most recent SelectedPath value (or
    // null if nothing has been selected yet). A panel mounted into a
    // slot mid-session calls this to seed its initial state instead
    // of waiting for the next selection event.
    string? CurrentSelectedPath { get; }

    // PublishSelectedPath lets panels OTHER than the tree (e.g. the
    // markdown-files browser, query browser) broadcast a path so
    // subscribers (detail, markdown-view) react. Equivalent to the
    // tree's EntitySelected → host handoff but for arbitrary panels.
    void PublishSelectedPath(string path);

    // RequestPeerStatusRefresh asks the host to re-pull peer summary
    // and refresh its status bar / chrome. ShellPanel calls this
    // after every dispatch because `connect` / `disconnect` / `cd` /
    // identity commands mutate peer state shared across all shells
    // belonging to the same peer. Empty default: hosts that don't
    // surface peer status just no-op.
    void RequestPeerStatusRefresh();
}
