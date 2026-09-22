#!/usr/bin/env bash
# gui-drive.sh — press buttons in the REAL Avalonia app from outside the
# process, and assert on what the window then says.
#
# WHY THIS EXISTS. Every gate in this repo either drives the GUI's model
# from inside a test process (the headless xunit suite: real input
# dispatch, no X11, no render thread, wildcard grants) or drives a
# DIFFERENT BINARY entirely (`scripts/twopeer-sync.sh`: two containers,
# real TCP, real grants — and entity-shell, which is not what the
# operator runs). The app's own `SmokeDriver` is compiled in, selected by
# an env var, fires once, reports an exit code, and calls the model
# methods UNDER the controls. So nothing could press the operator's
# button and read the operator's screen, and every session where they
# opened the GUI found bugs no gate could have caught.
#
# There is no Selenium for this stack and that is measured, not assumed:
# Avalonia 11.2.3's X11 backend ships no AT-SPI bridge, so the
# accessibility tree that dogtail/pyatspi would drive does not exist.
# `avalonia/frontend/UiDriver.cs` is the answer — an automation server in
# the app that resolves a control to a SCREEN RECTANGLE, with the press
# itself performed by xdotool against the real X server.
#
# WHAT A GREEN RUN CLAIMS: the real shipped binary, under a real X server
# with a window manager, with its real render thread, was driven by real
# pointer and keyboard input at coordinates it reported for named
# controls; the panel state changed as a result; the prose an operator
# reads was read back out of the process; and the app shut down through
# its own Closing handler afterwards.
#   IT DOES NOT CLAIM: two peers, a network, permissions, a restart, or
#   anything about sharing. This is the DRIVER's proof, not the flow's.
#   `scripts/twopeer-gui.sh` is the one that owes those, and it is not
#   written yet.
#
#   bash scripts/gui-drive.sh              # build, run, drive, tear down
#   SKIP_BUILD=1 bash scripts/gui-drive.sh # reuse avalonia/dist-native
#   KEEP_UP=1 bash scripts/gui-drive.sh    # leave the container running
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
APP="$REPO/avalonia"
IMG="${GUI_IMG:-entity-avalonia:dev}"
CTR="${GUI_CTR:-entity-gui-drive}"
PORT="${GUI_PORT:-9111}"
SECS="${GUI_SECS:-180}"
STAMP="$(date +%Y%m%d-%H%M%S)"
# Artifacts go to run-logs/, NEVER dist-native/ (AP54): `extract` runs
# `rm -rf dist-native` and every build target runs `extract`, so a log
# written there is destroyed by the next build — which is how the
# 2026-09-01 crash's complete breadcrumb stream was lost.
ART="$APP/run-logs/gui-drive-$STAMP"

FAILURES=()
CHECKS=0
log() { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
ok()  { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s\n' "$*" >&2; }
bad() { CHECKS=$((CHECKS+1)); FAILURES+=("$*"); printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31mFATAL:\033[0m %s\n' "$*" >&2; exit 1; }

command -v podman >/dev/null || die "podman is required"
mkdir -p "$ART"

cleanup() {
  if [ "${KEEP_UP:-0}" = "1" ]; then
    log "KEEP_UP=1 — container left running. Tear down with: podman rm -f $CTR"
    return
  fi
  podman rm -f "$CTR" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- build ------------------------------------------------------------
if [ "${SKIP_BUILD:-0}" != "1" ]; then
  log "building the image and extracting dist-native (this is the slow part)"
  make -C "$APP" build extract >"$ART/build.log" 2>&1 \
    || { tail -30 "$ART/build.log" >&2; die "build failed — see $ART/build.log"; }
fi
[ -x "$APP/dist-native/entity-avalonia" ] || die "no dist-native/entity-avalonia (drop SKIP_BUILD=1)"
cp "$APP/run-xvfb-smoke.sh" "$APP/dist-native/"
chmod +x "$APP/dist-native/run-xvfb-smoke.sh"

# --- run --------------------------------------------------------------
# The app is launched by run-xvfb-smoke.sh so this harness inherits every
# guarantee that script already enforces — Xvfb, the crash-dump env, the
# frame captures, and the render-thread alt-stack gate that exits 3 if
# the 2026-09-02 stack-overflow mitigation silently stopped installing.
#
# WB_SMOKE_WM=1 because a window manager owns focus, and a driver that
# types into an unfocused window tests nothing.
log "starting entity-avalonia under Xvfb + openbox, UI driver on :$PORT"
podman rm -f "$CTR" >/dev/null 2>&1 || true
#
# WB_SMOKE_OUT points at a bind mount of the artifact directory, so the
# run writes its log, frames and crash record straight to run-logs/ and
# nothing has to be copied out afterwards. The default would put them in
# dist-native/xvfb-smoke-out — which is AP54's trap, since `extract` does
# `rm -rf dist-native` and every build target runs `extract`, so the next
# build silently destroys the log of the run you are investigating.
podman run -d --name "$CTR" \
  -p "127.0.0.1:$PORT:$PORT" \
  -v "$APP/dist-native:/app:Z" \
  -v "$ART:/out:Z" \
  -e WB_SMOKE_OUT=/out \
  -e WB_UI_DRIVER="0.0.0.0:$PORT" \
  -e WB_SMOKE_WM=1 \
  -w /app --entrypoint bash "$IMG" \
  /app/run-xvfb-smoke.sh "$SECS" >/dev/null || die "could not start $CTR"

# --- the wire ---------------------------------------------------------
# One connection for the whole session, held open on fd 3. bash's
# /dev/tcp is the whole client: the runtime image has no curl and no nc,
# and the host is not required to grow a dependency either.
#
# THE CONNECTION MUST BE PROVED, NOT OPENED. Rootless podman's port
# forwarder accepts a TCP connection on a published port whether or not
# anything inside the container is listening yet, so `exec 3<>/dev/tcp`
# succeeds instantly against a dead end — and the first read then blocks
# until the app's own exit timer fires three minutes later. That is
# exactly how the first run of this harness behaved: it sat silent for
# the whole session and reported nothing at all, which reads as a hang in
# the app rather than a race in the harness. A successful `ping` is the
# only evidence the socket reaches the driver.
#
# AND THE PROBE RUNS IN A SUBSHELL, which is not a style choice. A failed
# redirection on `exec` is FATAL to a non-interactive shell: bash exits
# on the spot, ignoring `if`, `||` and the surrounding loop. So the
# obvious `if exec 3<>/dev/tcp/…; then` retry loop cannot retry — the
# first refused connection kills the harness silently, mid-line, with the
# container still running and no verdict printed. Measured: that is
# precisely what the first two runs of this script did, and from the
# outside it looked like the app hanging rather than the harness dying.
# Inside `( )` the fatal exit is confined to the subshell.
wire_ready() {
  (
    exec 3<>"/dev/tcp/127.0.0.1/$PORT" || exit 1
    printf 'ping\n' >&3
    local reply
    IFS= read -r -t 3 reply <&3 || exit 1
    case "$reply" in *'"ok":true'*) exit 0 ;; *) exit 1 ;; esac
  ) 2>/dev/null
}

open_conn() {
  for _ in $(seq 1 60); do
    if wire_ready; then
      exec 3<>"/dev/tcp/127.0.0.1/$PORT"
      return 0
    fi
    if ! podman inspect -f '{{.State.Running}}' "$CTR" 2>/dev/null | grep -q true; then
      log "container exited early; last 40 log lines:"
      podman logs "$CTR" 2>&1 | tail -40 >&2
      return 1
    fi
    sleep 1
  done
  return 1
}

# drv VERB [ARG...] — send one tab-separated command, return one JSON
# line. Every exchange is transcribed, so a failure is re-readable
# without re-running.
#
# The read is BOUNDED. An unbounded one turns any driver defect into a
# harness that produces no output and no verdict, which is worse than a
# red run — a red run at least says which command died.
drv() {
  local IFS=$'\t' line reply
  line="$*"
  if ! printf '%s\n' "$line" >&3 2>/dev/null; then
    echo '{"ok":false,"error":"driver connection is gone (the app may have exited)"}'
    return 1
  fi
  if ! IFS= read -r -t "${DRV_TIMEOUT:-30}" reply <&3; then
    printf '>> %s\n<< (no reply within %ss)\n' "$line" "${DRV_TIMEOUT:-30}" >> "$ART/driver.log"
    echo '{"ok":false,"error":"no reply from the UI driver"}'
    return 1
  fi
  printf '>> %s\n<< %s\n' "$line" "$reply" >> "$ART/driver.log"
  printf '%s' "$reply"
}

# expect_ok LABEL VERB [ARG...] — the command must report ok:true.
expect_ok() {
  local label="$1"; shift
  local r; r=$(drv "$@")
  case "$r" in
    *'"ok":true'*) ok "$label"; printf '%s' "$r" ;;
    *) bad "$label — $r"; printf '%s' "$r" ;;
  esac
}

# expect_fail LABEL VERB [ARG...] — the command must REFUSE. Used for the
# driver's own guarantees: an ambiguous selector has to fail rather than
# quietly pick the first match, because "quietly picks one" is how a
# harness goes green while pressing the wrong button.
expect_fail() {
  local label="$1" want="$2"; shift 2
  local r; r=$(drv "$@")
  case "$r" in
    *'"ok":true'*) bad "$label — expected a refusal, got: $r" ;;
    *"$want"*)     ok "$label" ;;
    *)             bad "$label — refused, but not for the stated reason: $r" ;;
  esac
}

# field JSON KEY — pull one scalar out of a response line.
field() { printf '%s' "$1" | grep -o "\"$2\":[^,}]*" | head -1 | cut -d: -f2- | sed 's/^"//; s/"$//'; }

log "waiting for the app to answer"
open_conn || die "the UI driver never answered on 127.0.0.1:$PORT (see $ART/)"

# --- the scenario -----------------------------------------------------

R=$(expect_ok "the app answers, and says which build it is" ping)
log "build: $(field "$R" build)"

expect_ok "the panel slots can be read" panels >/dev/null
expect_ok "a slot switches to the Sync panel" panel first sync >/dev/null
expect_ok "the Sync panel is on screen" wait SyncPanel 20000 >/dev/null

# The prose reader (AP71). A verb's printed guidance is a surface with no
# reader in the suite, and a panel's is the same class of surface: the
# only reason we know `share` spent a release telling operators to run a
# step that had been deleted is that somebody ran it and read the output.
R=$(expect_ok "the panel's own text can be read from outside the process" alltext SyncPanel)
log "panel says: $(field "$R" text)"

# The driver's own guarantee, asserted rather than assumed. "Share"
# matches both "Share a folder…" and the form's "Share".
expect_fail "an AMBIGUOUS selector is refused, not guessed at" "matches" where "Button:Share"

# Before the click: the share form exists and is hidden.
R=$(expect_ok "the share form's button resolves" find "#sync.share.go")
case "$R" in
  *'"visible":false'*) ok "the share form starts hidden" ;;
  *) bad "the share form should start hidden — $R" ;;
esac

# --- the actual proof -------------------------------------------------
# A real X11 press, at coordinates the app itself reported for a control
# addressed by name.
R=$(expect_ok "REAL CLICK on “Share a folder…”" click "#sync.share.begin")
log "clicked at $(field "$R" x),$(field "$R" y) (real X11 input: $(field "$R" realInput))"

R=$(expect_ok "the share form opened as a RESULT of that click" find "#sync.share.go")
case "$R" in
  *'"visible":true'*) ok "the form is now visible" ;;
  *) bad "the click did not open the form — $R" ;;
esac

# The panel's prose, driven by the click. This peer is alone in a
# container, so BeginShare's "no peers" branch is the correct one — and
# it is a sentence, composed in the panel, that nothing else in this repo
# has ever read back.
expect_ok "the panel EXPLAINS why it cannot share yet" \
  waittext "#sync.note" "No peers are known yet" 5000 >/dev/null

# Real keystrokes into a real TextBox, read back through the driver.
expect_ok "REAL TYPING into the folder box" type "#sync.share.dir" "/tmp/photos" >/dev/null
R=$(expect_ok "the typed text is what the control holds" text "#sync.share.dir")
if [ "$(field "$R" text)" = "/tmp/photos" ]; then
  ok "keystrokes landed in the control: /tmp/photos"
else
  bad "the box holds '$(field "$R" text)', not '/tmp/photos'"
fi

# CONTROL ARM. Without it, "the form is visible" could be satisfied by a
# form that was always visible and a click that did nothing at all.
expect_ok "REAL CLICK again (the toggle's other half)" click "#sync.share.begin" >/dev/null
R=$(expect_ok "the form closed again" find "#sync.share.go")
case "$R" in
  *'"visible":false'*) ok "control arm: the second click hid the form" ;;
  *) bad "control arm FAILED — the form did not hide, so the first click may have proved nothing: $R" ;;
esac

# A frame of the driven window, captured while it is still up.
#
# run-xvfb-smoke.sh's own final screenshot is taken only if the app is
# still alive when its timer expires, and this harness closes the window
# long before that — so without this the one artifact a human would
# actually look at is missing from every successful run.
if podman exec -e DISPLAY=:99 "$CTR" import -window root /out/driven.png >/dev/null 2>&1; then
  ok "a screenshot of the driven window was captured"
else
  bad "could not capture a screenshot of the driven window"
fi

# --- shutdown ---------------------------------------------------------
# Through the window's own Closing handler, which is where Bridge.Shutdown
# runs. Killing the container instead would say nothing about shutdown.
expect_ok "the window closes the operator's way" close >/dev/null
# ONE close, not two. `exec 3<>` opens a single read-write descriptor, so
# `exec 3<&-` closes it outright and the `exec 3>&-` that used to follow
# was a redirection error on an already-closed fd — fatal to a
# non-interactive shell, which killed the harness after its last check
# and before it printed the verdict. Every check had passed; the run
# still reported failure and said nothing about why.
exec 3<&-

log "waiting for the container to exit"
RC=$(podman wait --condition exited "$CTR" 2>/dev/null || echo "timeout")
if [ "$RC" = "0" ]; then
  ok "the app exited 0 after being driven (and the render-thread alt-stack gate passed)"
else
  bad "the app exited $RC — see $ART/run.log"
fi

if compgen -G "$ART/frames/*.png" >/dev/null; then
  ok "the session rendered frames throughout ($(ls "$ART"/frames/*.png | wc -l) captured)"
else
  bad "no frames were captured — the run may not have rendered at all"
fi

# --- report -----------------------------------------------------------
echo >&2
log "================ RESULT ================"
log "$CHECKS checks, ${#FAILURES[@]} failed"
log "artifacts: $ART"
if [ ${#FAILURES[@]} -gt 0 ]; then
  for f in "${FAILURES[@]}"; do printf '\033[1;31m  x\033[0m %s\n' "$f" >&2; done
  echo >&2
  log "app log tail:"; tail -30 "$ART/run.log" 2>/dev/null >&2
  exit 1
fi
# An empty result set must never satisfy "nothing failed" — see the same
# guard in twopeer-gui.sh, which printed a green banner after aborting
# during its build.
if [ "$CHECKS" -eq 0 ]; then log "NOTHING RAN — 0 checks. This is not a pass."; exit 1; fi
log "THE GUI CAN BE DRIVEN: named control -> screen rectangle -> real X11 input -> read back."
exit 0
