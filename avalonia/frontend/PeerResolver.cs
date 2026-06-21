using System;

namespace EntityAvalonia;

// PanelScope declares how a panel type binds to a peer at spawn time.
// Per PHASE-I-MULTI-PEER-PLAN.md §4.7 (manifest-driven scope).
//
// Resolved by PeerResolver.ResolveForPanel; panels never compute the
// binding themselves — the resolver is the single chokepoint.
public enum PanelScope
{
    // System panels bind to the system peer (the boot peer that owns
    // the application-state namespace). Settings, Peer Manager, Event
    // Log live here. They don't follow the active peer.
    System,

    // Peer panels bind to whichever peer the resolver names as active
    // for the panel's host (typically the tab's peer). Tree, Detail,
    // Shell live here. Binding fixes at construction; later changes to
    // the active peer don't rebind existing panels (per godot's
    // GUIDE-PEER-COMPOSITIONS rule).
    Peer,

    // Explicit panels bind to a specific peer named in the spawn args.
    // Reserved for "pin this panel to peer X regardless of active" use
    // cases. Not used in S3 — listed here so the resolver vocabulary
    // is complete.
    Explicit,
}

// PeerResolver owns peer-lookup for a single host context (e.g. one
// PeerView). The godot HostNamespace port (per SIBLING-FRONTEND-SURVEY
// §3.3, godot core/host_namespace.gd:79–110).
//
// Why this seam matters:
//  1. Testable independently — only depends on Bridge surface.
//  2. Single chokepoint for resolution logic; future fallback rules
//     (e.g. "explicit peer no longer in roster → empty-slot marker")
//     land in one file.
//  3. Per-host overrides without touching global state — each tab
//     constructs its own resolver with `_activePeerOverride` set to
//     its peer's handle; system panels still resolve to the system
//     peer (override doesn't apply to System scope).
//
// Lifetime: one per PeerView (per tab) + one for the MainWindow's
// system-scope chrome. Constructed once, lives as long as the host.
//
// NOT a singleton, NOT DI-injected. Per-window-context state.
public sealed class PeerResolver
{
    private readonly long _systemPeerHandle;
    private long _activePeerOverride;

    public PeerResolver(long systemPeerHandle, long activePeerOverride = 0)
    {
        _systemPeerHandle = systemPeerHandle;
        _activePeerOverride = activePeerOverride;
    }

    // ResolveForPanel returns the peer handle a panel of the given
    // scope should bind to, considering this resolver's override.
    //
    // For System scope: always the system peer (override doesn't apply).
    // For Peer scope: the override if set, else the system peer as
    //                 fallback (in the absence of a per-tab override,
    //                 panels default to system — that's the
    //                 MainWindow-level resolver's behavior).
    // For Explicit scope: the explicitPeerHandle parameter; throws if 0.
    public long ResolveForPanel(PanelScope scope, long explicitPeerHandle = 0)
    {
        return scope switch
        {
            PanelScope.System => _systemPeerHandle,
            PanelScope.Peer => _activePeerOverride != 0 ? _activePeerOverride : _systemPeerHandle,
            PanelScope.Explicit => explicitPeerHandle != 0
                ? explicitPeerHandle
                : throw new InvalidOperationException(
                    "PeerResolver: Explicit scope requires a non-zero explicitPeerHandle"),
            _ => throw new ArgumentOutOfRangeException(nameof(scope), scope, null),
        };
    }

    // SetActivePeerOverride changes the override for subsequent
    // resolutions in this host. Panels already constructed don't
    // rebind — per the spawn-time-binding invariant.
    public void SetActivePeerOverride(long peerHandle) =>
        _activePeerOverride = peerHandle;

    public void ClearActivePeerOverride() => _activePeerOverride = 0;

    public long SystemPeerHandle => _systemPeerHandle;
    public long ActivePeerHandle => _activePeerOverride != 0 ? _activePeerOverride : _systemPeerHandle;
    public bool HasOverride => _activePeerOverride != 0;
}
