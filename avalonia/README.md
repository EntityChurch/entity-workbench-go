# avalonia

The Avalonia (.NET) desktop renderer for entity-workbench-go.
Driven by a Go c-shared library carrying the existing
`entitysdk` + `shellboot` + `shellcmd` stack — same surface the
standalone `entity-shell` REPL uses, just with a graphical front-end.

**Status:** elevated from POC. Bridge contract is firm,
three spikes proven (hello round-trip, Store.Watch streaming via C
callback, shellcmd.Registry.Dispatch round-trip). The renderer
itself is still spike-shaped — see
`docs/architecture/PHASE-I-DESKTOP-RENDERER-PLAN.md` for the
multi-week build to panel parity with `console/` and `canvas/`.

Not deprecating the existing renderers — they keep working through
model-layer changes, and parity discipline is the architectural
forcing function.

## Layout

```
avalonia/
  bridge/        Go c-shared library (-buildmode=c-shared)
    main.go      exported symbols: BridgeInit, Hello, Dispatch,
                 WatchSubscribe, WatchUnsubscribe, FreeString
    go.mod       sibling-replaces into ../../entitysdk etc.
  frontend/      Avalonia 11 app, plain code (no XAML)
    Program.cs   AppBuilder entry
    MainWindow.cs  three-spike panel (placeholder until Phase I lands)
    Bridge.cs    P/Invoke wrapper around libbridge.so
    frontend.csproj
  Containerfile  multi-stage Fedora 43 build (Go + .NET SDK)
  Makefile       podman build / run / extract targets
  bridge_smoke.c throwaway C runner that validates the bridge
                 independently of Avalonia
```

## Build + run (podman)

The build context must include both `entity-core-go` and
`entity-workbench-go` siblings. The Makefile handles that — invoke
from this directory:

```
make build      # build the multi-stage image
make extract    # copy dist/ out to ./dist-native (host-runnable)
make host-run   # extract (if needed) + launch on host
make run        # run inside container with X11 forward (cookie-fragile on Wayland)
```

`make host-run` is the recommended dev loop today; container X11
forwarding through Mutter+XWayland needs more work and isn't on the
critical path (the renderer ships as a native binary, not a
container, in normal use).

## Bridge contract

| C symbol               | Signature                                              | Notes                                                 |
|------------------------|--------------------------------------------------------|-------------------------------------------------------|
| `BridgeInit`           | `() → const char*`                                     | NULL on success, error message (must free) on failure |
| `BridgeShutdown`       | `() → void`                                            | Close peer + cancel watches                           |
| `Hello`                | `() → const char*`                                     | Returns greeting (must free) — spike-1 sentinel        |
| `Dispatch`             | `(const char* argv_json) → const char*`                | argv as JSON array; returns `{ok,error?,result?}`     |
| `WatchSubscribe`       | `(const char* pattern, void* cb) → int64_t`            | Returns handle or -1; cb invoked per event            |
| `WatchUnsubscribe`     | `(int64_t handle) → void`                              | Stop the watch                                        |
| `FreeString`           | `(const char* p) → void`                               | Free strings returned by other calls                  |

Watch callback signature: `void cb(int64_t handle, const char* event_json)`.
**The callback MUST copy the JSON string before returning** — Go
frees it the moment `invoke_watch` returns.

The single-peer POC pre-cds the shell to `/@poc/` in `BridgeInit`
so relative paths (`put demo/a ...`) and watch patterns (`demo/*`)
align without the caller having to issue `cd` first. The real
frontend (Phase I) will plumb `shellboot.Config` properly and the
shell's WD will be user-driven.

## Validating the bridge without Avalonia

`bridge_smoke.c` dlopens `libbridge.so` and exercises every export
end-to-end — useful when iterating on the bridge half of the
contract:

```
make extract
cd dist-native
gcc ../bridge_smoke.c -o /tmp/smoke -ldl
LD_LIBRARY_PATH=. /tmp/smoke
```

Should print all three spikes green, ending with `Shutdown ok`.

## What this validates (or doesn't)

Validates:
- Go c-shared loaded by .NET via P/Invoke (build + link + run).
- Bidirectional flow: C# calls Go, Go invokes C# callbacks back.
- `shellcmd.Registry.Dispatch` works from a non-REPL frontend.
- `Store.Watch` events stream out the boundary at usable latency.
- Avalonia 11.2.x on Linux (Fedora 43 runtime) with X11/XWayland.

Does not validate:
- Cross-platform builds — macOS, Windows, Android, iOS all targets
  Avalonia supports; out of scope until Phase I stage 6+.
- Signed/notarized macOS distribution.
- Long-running stability (no leak tests).
- Real SDK identity bundles (POC uses ephemeral memory peer).
- Panel parity with existing `canvas/` / `console/` renderers
  (see Phase I plan for the sequencing).
