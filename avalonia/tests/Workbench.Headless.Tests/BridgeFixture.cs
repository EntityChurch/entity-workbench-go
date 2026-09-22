using System;
using System.Runtime.InteropServices;
using System.Text.Json;
using EntityAvalonia;
using Xunit;

namespace EntityAvalonia.Tests;

// BridgeFixture boots libbridge.so EXACTLY ONCE across the whole test
// assembly via xUnit's ICollectionFixture. Reasons:
//
//   1. The spike flagged that cgo callbacks may thread-lock to the
//      first goroutine↔OS-thread mapping observed. HeadlessUnitTestSession
//      spins up a fresh dispatcher thread per test, so a per-test
//      BridgeInit risks drift. One init, one peer handle, every test
//      reuses it.
//
//   2. BridgeInit is the heavy step (peer boot, identity material,
//      seed entities). Amortizing it across tests keeps each
//      [AvaloniaFact] cheap.
//
// Tests that need a fresh peer can still use Bridge.PeerCreate to
// spawn an additional peer scoped to that test; the system peer
// from this fixture stays untouched.
public sealed class BridgeFixture : IDisposable
{
    public long DefaultPeer { get; }

    public BridgeFixture()
    {
        // No test in this assembly may reach the public internet.
        //
        // BrowserPanel pins a configured registry at construction so the
        // shipped app opens on something usable. Left on here, every
        // BrowserPanel test would race that pin against its own fixture
        // pin — and on 2026-08-31 it did exactly that: six tests failed
        // with the LIVE registry's peer-id where the frozen fixture's
        // was expected, because the container had egress. A suite whose
        // result depends on a remote host is not measuring this tree.
        //
        // The policy itself (precedence, defaults, the deployment-file
        // adoption) is covered where it lives, in
        // `workbench/browse_config_test.go`.
        EntityAvalonia.Panels.BrowserPanel.AutoPinOnOpen = false;

        // Same rule, one panel over: no suite in this assembly dials
        // anything by itself. SharePanel completes the reciprocal dial
        // automatically when a peer it shares with reconnects, which is
        // correct in the app and is a suite reaching the network here.
        // In-process flag rather than an env var — Go captures its
        // environment at process start, so a C# SetEnvironmentVariable
        // never reaches the bridge.
        EntityAvalonia.Panels.SharePanel.AutoDialOnConnect = false;

        // Same rule, and one extra reason. SharingStatusPanel runs a real
        // reconcile pass when it opens — correct in the app, since opening
        // it IS the operator asking whether their sharing works, and a
        // pass is the only thing that can answer. Here it would dial. It
        // would also land ASYNCHRONOUSLY and REPLACE the panel's row
        // collections, wiping whatever a test seeded moments earlier: a
        // race decided by dispatcher timing and blamed on the assertion.
        //
        // Assembly-wide rather than per-test, because PanelStackScrollTests
        // constructs every registered panel and would otherwise dial on
        // whatever the last test happened to leave this static set to.
        EntityAvalonia.Panels.SharingStatusPanel.AutoReconcileOnOpen = false;

        var config = new BridgeConfig
        {
            Identity = "",
            Alias = "test",
            Storage = "memory",
            StoragePath = "",
            Listen = "",
            OpenAccess = false,
        };
        var json = JsonSerializer.Serialize(config);
        var errPtr = Bridge.Init(json);
        if (errPtr != IntPtr.Zero)
        {
            var err = Marshal.PtrToStringAnsi(errPtr) ?? "(null)";
            Bridge.FreeString(errPtr);
            throw new InvalidOperationException($"BridgeInit failed: {err}");
        }
        DefaultPeer = Bridge.DefaultPeer();
        if (DefaultPeer == 0)
        {
            throw new InvalidOperationException("BridgeInit returned but DefaultPeer is 0");
        }
    }

    public void Dispose()
    {
        // BridgeShutdown is a best-effort drain; per the bridge contract
        // it doesn't reliably teardown all peers. The test process
        // exiting will reap the rest.
        try { Bridge.Shutdown(); } catch { }
    }
}

[CollectionDefinition(nameof(BridgeCollection))]
public sealed class BridgeCollection : ICollectionFixture<BridgeFixture> { }
