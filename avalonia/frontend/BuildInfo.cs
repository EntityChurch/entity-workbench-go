using System;
using System.Reflection;

namespace EntityAvalonia;

// BuildInfo answers "which build is this?" — the question that cost a full
// day on 2026-09-03, when a landed fix was reported as still broken because
// two machines were running binaries six days apart and nothing on screen or
// in the log said so.
//
// `entity-shell` has had a stamp since it was written (`-X main.version`,
// root Makefile) and the GUI had none, which is the surface an operator
// actually runs.
//
// TWO stamps, deliberately. The frontend's comes from the assembly's
// InformationalVersion and the bridge's from a Go ldflags symbol; both are
// injected by the same `podman build --build-arg BUILD_STAMP=...`, so they
// agree by construction. They can only disagree in dist-native/, where
// `extract` copies an executable and a .so that a hand-run `go build` may
// since have replaced — and that state is indistinguishable from "the fix
// did not land" unless something says it out loud.
public static class BuildInfo
{
    private static string? _bridge;

    // Frontend is the stamp compiled into this assembly. "dev" when the
    // build did not pass one (a bare `dotnet run` outside the container).
    public static string Frontend { get; } =
        typeof(BuildInfo).Assembly
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion
        is string v && !string.IsNullOrWhiteSpace(v) ? Normalize(v) : "dev";

    // Bridge is the stamp compiled into libbridge.so, read across the FFI
    // boundary on first use and cached. Returns "unavailable" if the export
    // is missing, which is itself the answer: the .so predates this field.
    public static string Bridge
    {
        get
        {
            if (_bridge != null) return _bridge;
            try
            {
                _bridge = EntityAvalonia.Bridge.TakeString(EntityAvalonia.Bridge.BuildStamp());
                if (string.IsNullOrWhiteSpace(_bridge)) _bridge = "unavailable";
            }
            catch (EntryPointNotFoundException)
            {
                _bridge = "unavailable (libbridge.so predates BridgeBuildStamp)";
            }
            catch (DllNotFoundException)
            {
                _bridge = "unavailable (libbridge.so not loaded)";
            }
            return _bridge;
        }
    }

    // Matches reports whether the two halves came from the same build. False
    // is the interesting case and the reason this type exists.
    public static bool Matches => Frontend == Bridge;

    // Line is the one-line form written to stderr at startup and shown in the
    // This-peer panel. It states the mismatch rather than leaving a reader to
    // compare two strings, because nobody compares two strings.
    public static string Line =>
        Matches
            ? $"build {Frontend}"
            : $"build {Frontend} (frontend) / {Bridge} (bridge) — MISMATCH: dist-native holds artifacts from two different builds; re-run `make -C avalonia build extract`";

    // The SDK appends "+<sha>" source-revision metadata to
    // InformationalVersion when the property is set without one. Our stamp
    // already ends in "+<tree-hash>", so a second "+" segment would be the
    // SDK's, not ours — keep the first two segments and drop the rest.
    private static string Normalize(string v)
    {
        var parts = v.Split('+');
        return parts.Length <= 2 ? v : parts[0] + "+" + parts[1];
    }
}
