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
