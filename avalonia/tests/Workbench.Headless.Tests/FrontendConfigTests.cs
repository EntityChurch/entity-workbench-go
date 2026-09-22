using System.Text.Json;
using Xunit;

namespace EntityAvalonia.Tests;

/// <summary>
/// The C# half of "does the catch-up supervisor reach the GUI".
///
/// Tier: unit (TESTING-STRATEGY §1) — no bridge, no window.
///
/// <para>
/// The chain is three links: this frontend decides a default, serializes
/// it, and the bridge unmarshals the blob into shellboot.Config, which
/// turns reconcile_on_start into a running catch-up supervisor. The Go
/// end of it is gated in shellboot/frontend_catchup_test.go, against the
/// same JSON shape asserted here.
/// </para>
///
/// <para>
/// The middle link is the one that fails silently. An undeclared or
/// renamed field is DISCARDED with no warning by System.Text.Json on the
/// way out and by encoding/json on the way in — AP49, which shipped here
/// once already when BrowserPanel.View dropped two provenance fields the
/// model computed and the bridge sent. Nothing downstream notices,
/// because a peer with no supervisor behaves exactly like a peer whose
/// deliveries never dropped.
/// </para>
/// </summary>
public class FrontendConfigTests
{
    /// <summary>
    /// A default launch — no flags at all, which is what an operator who
    /// double-clicks the app gets — must ask for reconcile-on-start.
    ///
    /// That flag carries two things, and the second is the one this test
    /// is really about: the startup reconcile (re-establish declared
    /// relationships) and the catch-up supervisor (recover files the
    /// sender's subscription queue dropped on the floor). Without it a
    /// large copy into a shared folder stops part way, permanently, with
    /// no error on either side.
    /// </summary>
    [Fact]
    public void A_Default_Launch_Asks_For_Reconcile_On_Start()
    {
        Assert.True(Program.ParseArgs(System.Array.Empty<string>(), out _));
        Assert.True(Program.Config.ReconcileOnStart);
    }

    /// <summary>
    /// The control arm. --ephemeral is a throwaway peer with a fresh
    /// keypair and an in-memory store: it has no declarations to
    /// re-establish and no folder that could be behind, so it must not
    /// run a background loop. Without this arm the assertion above is
    /// satisfied by a field that is unconditionally true.
    /// </summary>
    [Fact]
    public void An_Ephemeral_Launch_Does_Not()
    {
        Assert.True(Program.ParseArgs(new[] { "--ephemeral" }, out _));
        Assert.False(Program.Config.ReconcileOnStart);
    }

    /// <summary>
    /// The field must cross the FFI boundary under the name the Go side
    /// reads. This asserts on the serialized bytes rather than on the
    /// property, because the property is not what travels.
    ///
    /// The literal here is deliberately the exact JSON fragment, not a
    /// round-trip through a C# mirror of shellboot.Config: a mirror would
    /// be renamed by the same edit that renamed the original and the test
    /// would keep passing. shellboot/frontend_catchup_test.go asserts the
    /// far side of the same string.
    /// </summary>
    [Fact]
    public void Reconcile_On_Start_Survives_Serialization_Under_The_Name_Go_Reads()
    {
        Assert.True(Program.ParseArgs(System.Array.Empty<string>(), out _));
        var json = JsonSerializer.Serialize(Program.Config);

        Assert.Contains("\"reconcile_on_start\":true", json);
    }

    /// <summary>
    /// An ephemeral launch is still LONG-RUNNING, and that is the whole
    /// point of the second flag.
    ///
    /// <para>
    /// reconcile_on_start answers "do I have declarations to
    /// re-establish", which for a throwaway peer is correctly no. The
    /// catch-up supervisor needs a different question answered — "am I
    /// going to be around to take another pass" — and for a desktop
    /// application that is yes in every configuration it has. Until these
    /// were one flag, --ephemeral was the ONE configuration in which a
    /// burst lost files permanently, because there is no next launch to
    /// recover them in.
    /// </para>
    /// </summary>
    [Fact]
    public void An_Ephemeral_Launch_Is_Still_Long_Running()
    {
        Assert.True(Program.ParseArgs(new[] { "--ephemeral" }, out _));
        Assert.False(Program.Config.ReconcileOnStart);
        Assert.True(Program.Config.LongRunning);
    }

    /// <summary>
    /// And a default launch is too — so the flag is not accidentally
    /// scoped to the ephemeral branch it was added for.
    /// </summary>
    [Fact]
    public void A_Default_Launch_Is_Long_Running()
    {
        Assert.True(Program.ParseArgs(System.Array.Empty<string>(), out _));
        Assert.True(Program.Config.LongRunning);
    }

    /// <summary>
    /// Same AP49 hazard as reconcile_on_start, asserted the same way: on
    /// the serialized bytes, under the name the Go side reads.
    /// shellboot/frontend_catchup_test.go asserts the far side, against
    /// the ephemeral blob — the case this flag exists for.
    /// </summary>
    [Fact]
    public void Long_Running_Survives_Serialization_Under_The_Name_Go_Reads()
    {
        Assert.True(Program.ParseArgs(new[] { "--ephemeral" }, out _));
        var json = JsonSerializer.Serialize(Program.Config);

        Assert.Contains("\"long_running\":true", json);
        Assert.Contains("\"reconcile_on_start\":false", json);
    }

    /// <summary>
    /// ParseArgs resets Config, so calling it twice yields the second
    /// call's configuration and not the union of both. It mutated a
    /// static in place until 2026-09-07 — harmless while Main was the
    /// only caller, and exactly the kind of accumulated state that makes
    /// a test suite's results depend on execution order (AP70).
    /// </summary>
    [Fact]
    public void ParseArgs_Does_Not_Accumulate_Across_Calls()
    {
        Assert.True(Program.ParseArgs(new[] { "--open-access", "--alias", "first" }, out _));
        Assert.True(Program.Config.OpenAccess);

        Assert.True(Program.ParseArgs(System.Array.Empty<string>(), out _));
        Assert.False(Program.Config.OpenAccess);
        Assert.Equal("", Program.Config.Alias);
    }
}
