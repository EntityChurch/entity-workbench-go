#!/usr/bin/env bash
# Boots entity-avalonia inside Xvfb (virtual framebuffer) so the real
# X11 + Skia paint path runs without needing a display. Captures a
# screenshot before the app exits and reports its exit code.
#
# Why: headless Avalonia tests don't exercise the same dispatcher /
# layout depth as a real X11 message loop. This wrapper closes that
# gap as a smoke test — boots the app, lets it render its main
# window + initial paint, captures the frame, kills it. Any X11-
# specific paint/layout bug surfaces here, even when headless tests
# pass.
#
# Usage:
#   ./run-xvfb-smoke.sh [seconds]
#
# Args:
#   seconds — how long to let the app run before closing (default 15).
#
# Env overrides:
#   WB_SMOKE_SCREEN — Xvfb geometry (default 1280x1024x24)
#   WB_SMOKE_OUT    — output dir for screenshot + run.log
#                     (default ./xvfb-smoke-out)
#
# Exit codes:
#   0  — app booted, rendered, exited 0 inside the timer window
#   1  — Xvfb failed to start
#   2  — app crashed (non-zero exit before timer)
#   3  — screenshot capture failed
#
# Artifacts (in $WB_SMOKE_OUT):
#   run.log       — entity-avalonia's full stderr+stdout
#   screenshot.png — frame grab from Xvfb before app close

set -uo pipefail

SECONDS_TO_RUN="${1:-15}"
SCREEN="${WB_SMOKE_SCREEN:-1280x1024x24}"
OUT_DIR="${WB_SMOKE_OUT:-$(pwd)/xvfb-smoke-out}"
mkdir -p "$OUT_DIR"

DISPLAY_NUM=99
LOG="$OUT_DIR/run.log"
SHOT="$OUT_DIR/screenshot.png"

echo "==> Xvfb smoke run"
echo "    geometry: $SCREEN"
echo "    timer:    ${SECONDS_TO_RUN}s"
echo "    out:      $OUT_DIR"

# Start Xvfb on :99. -nolisten tcp keeps it scoped to local Unix
# sockets; -ac disables host access control (we own the display).
Xvfb ":$DISPLAY_NUM" -screen 0 "$SCREEN" -nolisten tcp -ac > "$OUT_DIR/xvfb.log" 2>&1 &
XVFB_PID=$!
trap 'kill $XVFB_PID 2>/dev/null || true' EXIT

# Wait for Xvfb to bind its Unix socket. The socket appearing under
# /tmp/.X11-unix means the server is listening; we don't need
# xdpyinfo (which isn't packaged in fedora 43's minimal X stack).
export DISPLAY=":$DISPLAY_NUM"
ready=0
for _ in $(seq 1 25); do
    if [ -S "/tmp/.X11-unix/X$DISPLAY_NUM" ]; then
        ready=1
        break
    fi
    sleep 0.2
done
if [ "$ready" -ne 1 ]; then
    echo "ERROR: Xvfb didn't come up. xvfb.log:"
    cat "$OUT_DIR/xvfb.log"
    exit 1
fi
echo "    Xvfb ready on :$DISPLAY_NUM"

# Optional window manager. WB_SMOKE_WM=1 starts openbox on the virtual
# display before the app launches.
#
# **This exists for one reason: minimize is a window-MANAGER operation.**
# Setting WindowState.Minimized asks the WM to iconify; with no WM on the
# display nothing acts on it, and the driver's own counter reports zero
# transitions (correctly — see SmokeDriver.StartWindowCycle). The open
# managed stack overflow fires on minimize on a real desktop, and a bare
# Xvfb cannot reach it by construction. Off by default: every other smoke
# target is a paint test that a WM would only add reparenting noise to.
if [ -n "${WB_SMOKE_WM:-}" ]; then
    if command -v openbox >/dev/null 2>&1; then
        openbox > "$OUT_DIR/wm.log" 2>&1 &
        WM_PID=$!
        trap 'kill $WM_PID 2>/dev/null || true; kill $XVFB_PID 2>/dev/null || true' EXIT
        sleep 1
        echo "    window manager: openbox (pid $WM_PID) — minimize can now take effect"
    else
        echo "    WARNING: WB_SMOKE_WM set but openbox is not installed; minimize will be a no-op"
    fi
fi

# Hand the timer to the app so it self-closes inside the window.
# We don't kill from outside — that would skip the Closing handler
# (Bridge.Shutdown) and produce dirty exits.
export WB_SMOKE_EXIT_AFTER_SEC="$SECONDS_TO_RUN"
export WB_PANEL_LOG=1
export DOTNET_EnableDiagnostics=1
export DOTNET_DbgEnableMiniDump=1
export DOTNET_DbgMiniDumpType=4
export DOTNET_DbgMiniDumpName="$OUT_DIR/managed.%d.dmp"
export LD_LIBRARY_PATH=.
# Keep the crash record inside the run's artifact dir rather than the
# default ~/.entity/crash, so a harness run is self-contained.
export WB_CRASH_DIR="$OUT_DIR/crash"

# WB_SMOKE_INGEST / WB_SMOKE_CYCLE_PATHS / WB_SMOKE_CYCLE_GAP_MS are
# passed through from the caller's environment. The driver inside
# the app reads them. See SmokeDriver.cs.
if [ -n "${WB_SMOKE_INGEST:-}" ]; then
    echo "    smoke driver: WB_SMOKE_INGEST=$WB_SMOKE_INGEST cycles=${WB_SMOKE_CYCLE_PATHS:-50} gap=${WB_SMOKE_CYCLE_GAP_MS:-150}ms"
fi

# Launch the app in the background so we can capture screenshots
# at intervals during the run. The last few survive on disk; if
# the app crashes mid-run they show what was visible at each
# capture point.
# WB_SMOKE_GDB=1 runs the app under gdb and stops at the FIRST SIGSEGV.
#
# This exists because the signal that reaches the coredump is not the
# one that matters. CoreCLR's handler runs, fails to convert the fault,
# and RE-RAISES — so every dump we have carries si_code 128 (SI_KERNEL)
# with si_addr 0 and a register context that is the handler's, not the
# fault's. `nopass` keeps gdb from delivering the signal onward, so we
# stop on the ORIGINAL fault with the true rip/si_addr intact.
if [ -n "${WB_SMOKE_GDB:-}" ]; then
    echo "    running under gdb (stop at first SIGSEGV)"
    gdb -batch -nx -q \
        -ex "set pagination off" \
        -ex "set confirm off" \
        -ex "handle all nostop noprint pass" \
        -ex "handle SIG34 SIG35 SIG36 SIG37 SIG38 nostop print pass" \
        -ex "handle SIGINT stop print nopass" \
        -ex "handle SIGSEGV stop print nopass" \
        -ex "handle SIGBUS stop print nopass" \
        -ex "run" \
        -ex "echo \n===== TRUE FAULT SITE =====\n" \
        -ex "print \$_siginfo" \
        -ex "info registers rip rsp rbp rax rbx rcx rdx rsi rdi" \
        -ex "echo \n===== BACKTRACE =====\n" \
        -ex "bt 60" \
        -ex "echo \n===== DISASM AROUND RIP =====\n" \
        -ex "x/8i \$rip" \
        -ex "echo \n===== THREADS =====\n" \
        -ex "info threads" \
        -ex "echo \n===== STACK EXTENT =====\n" \
        -ex "info proc mappings" \
        -ex "dump binary memory $OUT_DIR/stack.bin \$rsp \$rsp+2097152" \
        --args ./entity-avalonia > "$LOG" 2>&1 &
    APP_PID=$!
else
    ./entity-avalonia > "$LOG" 2>&1 &
    APP_PID=$!
fi

# ---- click fuzz --------------------------------------------------
#
# Real X11 pointer input, injected with xdotool. **This is the rung
# every other smoke target skips.** smoke-xvfb-site drives the model
# directly (SiteViewPanel.NavigateForTests), which by construction
# cannot reach input dispatch, hit-testing, focus movement, or any
# handler that runs BEFORE a panel's own code — and the 2026-08-21
# SIGSEGV landed in exactly that gap: the operator clicked one link
# and the fault preceded `SiteViewPanel.NavigateTo`'s first log line.
# A driver that calls the method under the click cannot reproduce a
# bug in the click.
#
# Deterministic by seed, and every gesture is logged with its
# coordinates, so a crashing run is replayable rather than a story
# about randomness. Bash's $RANDOM is seeded from WB_SMOKE_CLICK_SEED.
#
# DRAGS, added 2026-09-01. Until today this loop only ever did
# `mousemove; click` — press and release at one point — and the
# doctrine's own capability table said so. A drag is a different code
# path in every layer that matters: it takes pointer CAPTURE, it
# delivers a stream of PointerMoved to a captured element rather than to
# whatever is under the cursor, and on a ScrollBar thumb it drives
# repeated layout and a compositor pass per move. None of that is
# reachable by a click.
#
# It matters because the 2026-09-01 SIGSEGV happened while the operator
# was dragging the tree's vertical scrollbar, and no instrument in this
# repo could perform that gesture against a real compositor. The
# headless harness can drag (TreeViewScrollDragTests) but has no X11
# backend and no render thread; this harness has both and could not
# drag. The crash lives in the intersection, which is precisely the
# region neither one covered.
#
# WB_SMOKE_DRAG_PCT of gestures become drags (default 40). Of those,
# half are near-VERTICAL — same x, large dy — because that is the shape
# of a scrollbar-thumb gesture and a uniformly random endpoint pair
# almost never produces one.
CLICKS="${WB_SMOKE_CLICK_FUZZ:-0}"
if [ "$CLICKS" -gt 0 ]; then
    if ! command -v xdotool >/dev/null 2>&1; then
        echo "ERROR: WB_SMOKE_CLICK_FUZZ set but xdotool is not installed in this image"
        kill "$APP_PID" 2>/dev/null || true
        exit 4
    fi
    CLICK_SEED="${WB_SMOKE_CLICK_SEED:-1}"
    CLICK_GAP_MS="${WB_SMOKE_CLICK_GAP_MS:-120}"
    DRAG_PCT="${WB_SMOKE_DRAG_PCT:-40}"
    DRAG_STEPS="${WB_SMOKE_DRAG_STEPS:-12}"
    CLICK_LOG="$OUT_DIR/clicks.log"
    : > "$CLICK_LOG"
    # Screen geometry, minus a margin so we stay inside the window
    # rather than clicking the root desktop.
    SCREEN_W="${SCREEN%%x*}"
    _rest="${SCREEN#*x}"
    SCREEN_H="${_rest%%x*}"
    echo "    click fuzz: $CLICKS gestures seed=$CLICK_SEED gap=${CLICK_GAP_MS}ms" \
         "drag=${DRAG_PCT}% steps=$DRAG_STEPS -> $CLICK_LOG"
    (
        RANDOM=$CLICK_SEED
        # Let the window map and do its first paint before poking it.
        sleep 3
        i=0
        while [ "$i" -lt "$CLICKS" ] && kill -0 "$APP_PID" 2>/dev/null; do
            x=$(( RANDOM % (SCREEN_W - 20) + 10 ))
            y=$(( RANDOM % (SCREEN_H - 20) + 10 ))
            roll=$(( RANDOM % 100 ))
            if [ "$roll" -lt "$DRAG_PCT" ]; then
                # Half the drags are near-vertical (scrollbar-thumb
                # shape); the rest go anywhere.
                if [ $(( RANDOM % 2 )) -eq 0 ]; then
                    x2=$(( x + (RANDOM % 7) - 3 ))
                    y2=$(( RANDOM % (SCREEN_H - 20) + 10 ))
                    kind="DRAG-V"
                else
                    x2=$(( RANDOM % (SCREEN_W - 20) + 10 ))
                    y2=$(( RANDOM % (SCREEN_H - 20) + 10 ))
                    kind="DRAG"
                fi
                echo "$i $kind $x $y -> $x2 $y2 steps=$DRAG_STEPS" >> "$CLICK_LOG"
                # press, walk, release. Each mousemove is a separate
                # xdotool call so the app sees a STREAM of motion to a
                # captured element, which is the whole point — a single
                # jump from press-point to release-point exercises
                # neither capture nor incremental layout.
                xdotool mousemove "$x" "$y" mousedown 1 >/dev/null 2>&1 || true
                s=1
                while [ "$s" -le "$DRAG_STEPS" ]; do
                    mx=$(( x + (x2 - x) * s / DRAG_STEPS ))
                    my=$(( y + (y2 - y) * s / DRAG_STEPS ))
                    xdotool mousemove "$mx" "$my" >/dev/null 2>&1 || true
                    s=$((s + 1))
                done
                xdotool mouseup 1 >/dev/null 2>&1 || true
            else
                echo "$i CLICK $x $y" >> "$CLICK_LOG"
                xdotool mousemove "$x" "$y" click 1 >/dev/null 2>&1 || true
            fi
            i=$((i + 1))
            sleep "$(awk "BEGIN{print $CLICK_GAP_MS/1000}")"
        done
        echo "click fuzz finished after $i gestures" >> "$CLICK_LOG"
    ) &
    CLICK_PID=$!
fi

# Capture a screenshot every ~2s. Keep all frames; small + cheap.
# Numbered by capture-time so they sort chronologically (frame-00.png
# is t≈2s, frame-01.png is t≈4s, etc.).
shots_dir="$OUT_DIR/frames"
mkdir -p "$shots_dir"
frame_n=0
remaining=$SECONDS_TO_RUN
while [ "$remaining" -gt 2 ] && kill -0 "$APP_PID" 2>/dev/null; do
    sleep 2
    remaining=$((remaining - 2))
    if ! kill -0 "$APP_PID" 2>/dev/null; then
        echo "    app exited mid-capture at frame $frame_n"
        break
    fi
    frame_path="$shots_dir/frame-$(printf '%02d' "$frame_n").png"
    if import -display ":$DISPLAY_NUM" -window root "$frame_path" 2>>"$OUT_DIR/screenshot.err"; then
        echo "    frame $frame_n -> $frame_path"
    fi
    frame_n=$((frame_n + 1))
done

# Final screenshot pinned to the well-known name.
if kill -0 "$APP_PID" 2>/dev/null; then
    if import -display ":$DISPLAY_NUM" -window root "$SHOT" 2>>"$OUT_DIR/screenshot.err"; then
        echo "    final screenshot -> $SHOT"
    else
        echo "WARNING: final screenshot capture failed; see $OUT_DIR/screenshot.err"
    fi
fi

# Wait for the app to close (timer should fire any moment now).
wait "$APP_PID"
EXIT_CODE=$?

kill "${CLICK_PID:-}" 2>/dev/null || true

echo "==> entity-avalonia exited $EXIT_CODE"
if [ "$EXIT_CODE" -ne 0 ]; then
    echo "==> last 40 log lines:"
    tail -40 "$LOG"
    # The three channels that make a crash actionable instead of a
    # story. Print them here so a CI/loop run captures them without
    # anyone going to look.
    if [ -s "$OUT_DIR/clicks.log" ]; then
        echo "==> last 15 clicks (x y, replay with WB_SMOKE_CLICK_SEED=${WB_SMOKE_CLICK_SEED:-1}):"
        tail -15 "$OUT_DIR/clicks.log"
    fi
    for cl in "$OUT_DIR"/crash/*.log; do
        [ -e "$cl" ] || continue
        echo "==> crash diagnostics: $cl"
        cat "$cl"
    done
    for dmp in "$OUT_DIR"/managed.*.dmp; do
        [ -e "$dmp" ] || continue
        echo "==> managed minidump present: $dmp"
    done
    exit 2
fi

# The render thread's alternate signal stack — asserted, not hoped for.
#
# This is the ONLY place in the repo that can check it. sigaltstack is
# per-thread, the headless suite has no render thread at all, and there
# is no cross-thread query for the current alt stack — so the sole
# available evidence is that the install ran there, and the sole harness
# with a real compositor is this one.
#
# It matters because of what a regression here looks like: nothing. The
# app boots, renders, clicks, exits 0, and the render thread is silently
# back on the PAL's stock 16 KB — the configuration that produced the
# 2026-09-02 SIGSEGV, where the kernel logged "overflowed sigaltstack"
# and the process died with no minidump, no crash log and a breadcrumb
# trail that just stops. A green smoke run would say nothing about it.
#
# WB_ALTSTACK_BYTES=0 is the documented A/B control arm and legitimately
# disables the install, so it is exempted rather than allowed to fail.
if [ "${WB_ALTSTACK_BYTES:-}" != "0" ]; then
    if grep -q "render thread: enlarge -> installed" "$LOG"; then
        echo "    render-thread altstack: $(grep -m1 "render thread: enlarge ->" "$LOG" | sed 's/.*enlarge -> //')"
    else
        echo "==> FAIL: the render thread's alternate signal stack was never enlarged."
        echo "    The app ran and exited cleanly, so nothing else in this run reports it."
        echo "    The render thread is on the PAL stock 16 KB, which is the configuration"
        echo "    that crashed on 2026-09-02 (kernel: 'overflowed sigaltstack')."
        echo "    Look at AltStackProbe: it reaches the render thread through an"
        echo "    ICustomDrawOperation, and a custom draw op that is culled, never"
        echo "    scheduled, or attached outside the visual tree runs zero times."
        grep -i "altstack" "$LOG" | tail -10 || true
        exit 3
    fi
fi

echo "==> smoke run complete"
echo "    log:        $LOG"
echo "    screenshot: $SHOT"
exit 0
