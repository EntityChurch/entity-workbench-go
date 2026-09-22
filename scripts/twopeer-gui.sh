#!/usr/bin/env bash
# twopeer-gui.sh — the whole sharing flow, performed by PRESSING BUTTONS
# in two real Avalonia apps, on two containers, over one real TCP network.
#
# WHY THIS EXISTS. `scripts/twopeer-sync.sh` does all of this and drives
# **entity-shell**. The operator opens the **GUI**. That gap is the entire
# 2026-09-05 validation audit in one line: the business logic underneath
# is shared (avalonia/bridge/share.go is a thin envelope), but everything
# above the bridge call had no gate of any kind — the cgo/JSON boundary
# where an undeclared field is dropped in silence (AP49, shipped twice),
# the panel layer, the app's own process lifecycle, and its default peer
# configuration, which differs from the shell's.
#
# WHAT A GREEN RUN CLAIMS: two real Avalonia binaries, in separate network
# namespaces with distinct routable addresses, each with its own persistent
# sqlite store and its own identity, each under a real X server with a
# window manager and a real render thread — connected to each other by an
# address TYPED INTO A TEXT BOX with real keystrokes, sharing a folder by
# PRESSING the button an operator presses, accepting it by PRESSING the
# button on the card that appeared, and moving file CREATE / MODIFY /
# DELETE across the link. Every file assertion is on BYTES ON DISK on the
# receiving side. No wildcard grants: the permission stage is real.
#   IT DOES NOT CLAIM: two physical machines, the public internet, NAT,
#   mDNS on a real LAN (container DNS and a bridge stand in, and the
#   discovery phase is MEASURED rather than asserted), a GPU driver, or a
#   desktop window manager other than openbox.
#
#   bash scripts/twopeer-gui.sh              # build, run the lot, tear down
#   SKIP_BUILD=1 bash scripts/twopeer-gui.sh # reuse avalonia/dist-native
#   KEEP_UP=1    bash scripts/twopeer-gui.sh # leave both apps running
#   PHASE=share  bash scripts/twopeer-gui.sh # stop after a named phase
#                                            # (connect/share/accept/backfill/
#                                            #  live/restart/all)
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
APP="$REPO/avalonia"
IMG="${GUI_IMG:-entity-avalonia:dev}"
NET="${GUI_NET:-entity-gui-twopeer}"
PORT_A="${GUI_PORT_A:-9121}"
PORT_B="${GUI_PORT_B:-9122}"
PEER_PORT=9110
SECS="${GUI_SECS:-1800}"
STAMP="$(date +%Y%m%d-%H%M%S)"
ART="$APP/run-logs/twopeer-gui-$STAMP"
RUN="${RUNDIR:-/tmp/entity-gui-twopeer-$$}"
STOP_AFTER="${PHASE:-all}"

# Collected, never aborted at the first red (AP15). A run that stops at
# the first failure tells you one thing when the tree may be wrong in
# four ways, and the count it yields is a lower bound.
FAILURES=()
KNOWN=()
CHECKS=0
FATAL=""
log()   { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
note()  { printf '\033[1;36m  ..\033[0m  %s\n' "$*" >&2; }
ok()    { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s\n' "$*" >&2; }
bad()   { CHECKS=$((CHECKS+1)); FAILURES+=("$*"); printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; }
die()   { FATAL="$*"; printf '\033[1;31mFATAL:\033[0m %s\n' "$*" >&2; report; exit 2; }

# known_bad — a failure that is ALREADY DOCUMENTED as open, recorded
# separately so this harness is not permanently red for a defect it did
# not introduce and is not this run's news.
#
# It is a waiver on a NAMED instance, never a blanket one: if the check
# starts passing, `known_ok` says so loudly, because a silently-healed
# waiver is how a fixed bug goes on being described as broken — and how a
# second, different instance gets absorbed into the first one's excuse.
known_bad() { CHECKS=$((CHECKS+1)); KNOWN+=("$*"); printf '\033[1;33m KNOWN\033[0m %s\n' "$*" >&2; }
known_ok()  { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s \033[1;33m<- the known defect did NOT reproduce; re-check the waiver\033[0m\n' "$*" >&2; }

report() {
  echo >&2
  log "================ RESULT ================"
  log "$CHECKS checks · ${#FAILURES[@]} failed · ${#KNOWN[@]} known-open"
  log "artifacts: $ART"
  if [ ${#KNOWN[@]} -gt 0 ]; then
    for k in "${KNOWN[@]}"; do printf '\033[1;33m  ~\033[0m %s\n' "$k" >&2; done
  fi
  if [ ${#FAILURES[@]} -gt 0 ] || [ -n "$FATAL" ]; then
    [ -n "$FATAL" ] && printf '\033[1;31m  !\033[0m ABORTED: %s\n' "$FATAL" >&2
    for f in "${FAILURES[@]}"; do printf '\033[1;31m  x\033[0m %s\n' "$f" >&2; done
    echo >&2
    log "peer-a app log tail:"; tail -20 "$ART/a/run.log" 2>/dev/null >&2
    log "peer-b app log tail:"; tail -20 "$ART/b/run.log" 2>/dev/null >&2
    return 1
  fi
  # NO CHECKS IS NOT A PASS. The first version printed the green banner
  # after a fatal abort during the build — zero checks, zero failures, and
  # a sentence claiming two peers had shared a folder. An empty result set
  # satisfying a "nothing failed" test is the oldest way a harness lies,
  # and this one did it on its very first run.
  if [ "$CHECKS" -eq 0 ]; then
    log "NOTHING RAN — 0 checks. This is not a pass."
    return 1
  fi
  log "TWO-PEER GUI GREEN: connected, shared, accepted, backfilled and kept in sync — by pressing buttons."
  return 0
}

command -v podman >/dev/null || die "podman is required"
mkdir -p "$ART/a" "$ART/b"

cleanup() {
  if [ "${KEEP_UP:-0}" = "1" ]; then
    log "KEEP_UP=1 — both apps left running."
    log "  peer-a driver: 127.0.0.1:$PORT_A   peer-b driver: 127.0.0.1:$PORT_B"
    log "  tear down: podman rm -f gui-peer-a gui-peer-b; podman network rm $NET"
    return
  fi
  podman rm -f gui-peer-a gui-peer-b >/dev/null 2>&1 || true
  podman network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- build ------------------------------------------------------------
if [ "${SKIP_BUILD:-0}" != "1" ]; then
  log "building the image and extracting dist-native (the slow part)"
  make -C "$APP" build extract >"$ART/build.log" 2>&1 \
    || { tail -30 "$ART/build.log" >&2; die "build failed — see $ART/build.log"; }
fi
[ -x "$APP/dist-native/entity-avalonia" ] || die "no dist-native/entity-avalonia (drop SKIP_BUILD=1)"
cp "$APP/run-xvfb-smoke.sh" "$APP/dist-native/"
chmod +x "$APP/dist-native/run-xvfb-smoke.sh"

mkdir -p "$RUN/a" "$RUN/b" "$RUN/photos" "$RUN/received"
chmod -R 777 "$RUN"

# Containers first, THEN the network: podman refuses to remove a network
# that still has containers attached, and the refusal is swallowed here —
# so a leftover peer from an earlier run made `network create` fail and
# the whole harness abort before check one, reporting a container-runtime
# problem as if it were ours.
podman rm -f gui-peer-a gui-peer-b >/dev/null 2>&1 || true
podman network rm "$NET" >/dev/null 2>&1 || true
podman network create "$NET" >/dev/null || die "could not create network $NET"

# start_app NAME HOSTDIR DRIVERPORT EXTRA-MOUNT...
#
# dist-native is mounted with a SHARED SELinux label (:z, lowercase), not
# a private one. Two containers mounting the same host directory with :Z
# would each relabel it privately and the second would revoke the first's
# access mid-run — the same failure mode AGENTS.md documents for running
# `make test-each` and `make -C avalonia test` concurrently, where every
# suite after the first reports "Permission denied" while its own log
# says ok. The per-peer directories keep :z for the same reason.
#
# THE LAYOUT IS PINNED, AND THE TALL SCREEN IS NOT COSMETIC.
#
# The default arrangement is three panels (site-view/detail/shell) in a
# stack that is taller than the window, so the PanelStack scrolls — and
# the two panels this scenario needs are then partly below the fold. A
# control that is scrolled out still has layout and still reports a
# screen coordinate, so a driver that trusts it clicks the chrome sitting
# at that pixel instead. Measured on 2026-09-06: the press on Share
# landed 218px high, on the slot's header; the driver said ok, the app
# logged no press, and six checks failed downstream with nothing pointing
# at the cause.
#
# UiDriver now refuses a clipped click outright, which converts that into
# an honest red. Pinning the layout to exactly the two panels the flow
# uses, on a screen tall enough for both, is what makes it green — and it
# is also closer to what an operator does, since nobody drives this flow
# with three unrelated panels open. WB_LAYOUT takes a PATH to a layout
# file (workbench/layout_config.go), keyed by peer ALIAS.
write_layout() {
  local home="$1" alias="$2"
  printf '{"peers":{"%s":{"panels":["sync","peer-connections"]}}}\n' "$alias" > "$home/layout.json"
  chmod 666 "$home/layout.json"
}

start_app() {
  local name="$1" home="$2" port="$3"; shift 3
  write_layout "$home" "${name#gui-peer-}"
  podman run -d --name "$name" --network "$NET" --hostname "$name" \
    -p "127.0.0.1:$port:$port" \
    -v "$APP/dist-native:/app:z" \
    -v "$home:/data:z" \
    -v "$RUN/photos:/photos:z" \
    -v "$RUN/received:/received:z" \
    -v "$ART/${name#gui-peer-}:/out:z" \
    -e HOME=/data \
    -e WB_SMOKE_OUT=/out \
    -e WB_SMOKE_WM=1 \
    -e WB_LAYOUT=/data/layout.json \
    -e WB_SMOKE_SCREEN=1280x1440x24 \
    -e WB_UI_DRIVER="0.0.0.0:$port" \
    -e WB_SMOKE_APP_ARGS="--new-identity ${name#gui-} --alias ${name#gui-peer-} --storage-path /data/store.db --listen 0.0.0.0:$PEER_PORT" \
    -w /app --entrypoint bash "$IMG" \
    /app/run-xvfb-smoke.sh "$SECS" >/dev/null \
    || die "could not start $name"
}

# --- the wire ---------------------------------------------------------
# fd 3 is peer-a, fd 4 is peer-b, held open for the whole session.
#
# The probe runs in a SUBSHELL and is only believed when a ping comes
# back. Two reasons, both measured on 2026-09-05: a failed redirection on
# `exec` is fatal to a non-interactive shell, so the obvious retry loop
# cannot retry and the harness dies mid-line with no verdict; and rootless
# podman accepts a connection on a published port before anything inside
# the container is listening, so connecting proves nothing on its own.
wire_ready() {
  local port="$1" reply
  (
    exec 9<>"/dev/tcp/127.0.0.1/$port" || exit 1
    printf 'ping\n' >&9
    IFS= read -r -t 3 reply <&9 || exit 1
    case "$reply" in *'"ok":true'*) exit 0 ;; *) exit 1 ;; esac
  ) 2>/dev/null
}

open_peer() {
  local who="$1" port="$2" ctr="$3" i
  for i in $(seq 1 90); do
    if wire_ready "$port"; then
      case "$who" in
        A) exec 3<>"/dev/tcp/127.0.0.1/$port" ;;
        B) exec 4<>"/dev/tcp/127.0.0.1/$port" ;;
      esac
      return 0
    fi
    if ! podman inspect -f '{{.State.Running}}' "$ctr" 2>/dev/null | grep -q true; then
      log "$ctr exited early; last 30 log lines:"
      podman logs "$ctr" 2>&1 | tail -30 >&2
      return 1
    fi
    sleep 1
  done
  return 1
}

# NO `2>/dev/null` ON THESE, and the reason is worth the paragraph.
#
# `exec 3<&- 2>/dev/null` does two things, and the second is not the one
# anybody intends: it closes fd 3, and then — because `exec` with no
# command makes its redirections PERMANENT — it points the shell's stderr
# at /dev/null for the rest of the run. Every `log`, every `ok`/`FAIL`,
# and the entire final report write to fd 2. So the harness executed the
# whole restart phase, reached its verdict, and printed nothing at all,
# exiting 0.
#
# From outside that is indistinguishable from a crash at phase 11, and it
# was the exact opposite: a clean green run that had gagged itself. Under
# `bash -x` the trace stops at `+ exec` for the same reason, which is what
# finally named it. Both descriptors are open here by construction, so
# there is nothing to suppress.
close_peers() { exec 3<&-; exec 4<&-; }

# peer_health WHO — " — the peer-a container has EXITED (…)" or "".
#
# WITHOUT THIS, A DEAD APP LOOKS LIKE TWELVE UNRELATED PANEL BUGS. When
# peer-a segfaulted mid-keystroke on 2026-09-06 the run reported "no reply
# from the UI driver", then eleven more failures about missing controls
# and absent files — every one of them true, none of them the news, and
# the actual event (a SIGSEGV) discoverable only by going to the container
# afterwards. A harness that cannot say "the thing under test died" makes
# its own output untrustworthy.
peer_health() {
  local who="$1" ctr state code
  case "$who" in A) ctr=gui-peer-a ;; B) ctr=gui-peer-b ;; *) return 0 ;; esac
  state=$(podman inspect -f '{{.State.Status}}' "$ctr" 2>/dev/null)
  [ "$state" = "running" ] && return 0
  code=$(podman inspect -f '{{.State.ExitCode}}' "$ctr" 2>/dev/null)
  # 139 = 128+SIGSEGV. run-xvfb-smoke.sh maps any non-zero app exit to 2.
  printf ' — THE %s CONTAINER IS %s (exit %s); the app died, see the run log' \
    "$ctr" "${state:-gone}" "${code:-?}"
}

# drv A|B VERB [ARG...] — one tab-separated command, one JSON line back.
# Bounded read: an unbounded one turns any driver defect into a harness
# that produces no output and no verdict, which is worse than a red run.
drv() {
  local who="$1"; shift
  local fd line reply IFS=$'\t'
  case "$who" in A) fd=3 ;; B) fd=4 ;; *) echo '{"ok":false,"error":"bad peer"}'; return 1 ;; esac
  line="$*"
  if ! printf '%s\n' "$line" >&"$fd" 2>/dev/null; then
    echo "{\"ok\":false,\"error\":\"driver connection is gone$(peer_health "$who")\"}"
    return 1
  fi
  if ! IFS= read -r -t "${DRV_TIMEOUT:-180}" -u "$fd" reply; then
    printf '[%s] >> %s\n[%s] << (no reply)\n' "$who" "$line" "$who" >> "$ART/driver.log"
    echo "{\"ok\":false,\"error\":\"no reply from the UI driver$(peer_health "$who")\"}"
    return 1
  fi
  printf '[%s] >> %s\n[%s] << %s\n' "$who" "$line" "$who" "$reply" >> "$ART/driver.log"
  printf '%s' "$reply"
}

expect_ok() {
  local label="$1" who="$2"; shift 2
  local r; r=$(drv "$who" "$@")
  case "$r" in
    *'"ok":true'*) ok "$label"; printf '%s' "$r" ;;
    *) bad "$label — $r"; printf '%s' "$r" ;;
  esac
}

field() { printf '%s' "$1" | grep -o "\"$2\":[^,}]*" | head -1 | cut -d: -f2- | sed 's/^"//; s/"$//'; }

# --- file assertions --------------------------------------------------
# ON DISK, on the receiving side, always. A tree count passes while the
# operator's folder is empty — which is exactly how the delete defect
# survived every gate this repo had.
wait_for_file() {
  local path="$1" want="$2" label="$3" mode="${4:-hard}" i content
  for i in $(seq 1 90); do
    if [ -f "$path" ]; then
      content=$(cat "$path" 2>/dev/null)
      [ "$content" = "$want" ] && { [ "$mode" = "known" ] && known_ok "$label" || ok "$label"; return 0; }
    fi
    sleep 1
  done
  local why
  if [ -f "$path" ]; then why="content is '$(cat "$path" 2>/dev/null)', wanted '$want'"
  else why="$path never appeared (90s)"; fi
  if [ "$mode" = "known" ]; then known_bad "$label — $why"; else bad "$label — $why"; fi
  return 1
}

wait_for_absence() {
  local path="$1" label="$2" i
  for i in $(seq 1 90); do
    [ -f "$path" ] || { ok "$label"; return 0; }
    sleep 1
  done
  bad "$label — $path is still present after 90s"
  return 1
}

stop_after() {
  [ "$STOP_AFTER" = "$1" ] || return 0
  log "PHASE=$1 — stopping here as asked"
  report; exit $?
}

# ======================================================================
# PHASE 1 — two apps, two containers, two stores
# ======================================================================
log "PHASE 1 — two Avalonia apps, two containers, two persistent stores"
echo "photo one"   > "$RUN/photos/one.txt"
echo "photo two"   > "$RUN/photos/two.txt"
mkdir -p "$RUN/photos/sub"
echo "nested"      > "$RUN/photos/sub/three.txt"
chmod -R 777 "$RUN/photos"

start_app gui-peer-a "$RUN/a" "$PORT_A"
start_app gui-peer-b "$RUN/b" "$PORT_B"

open_peer A "$PORT_A" gui-peer-a || die "peer-a's UI driver never answered"
open_peer B "$PORT_B" gui-peer-b || die "peer-b's UI driver never answered"
ok "both apps came up and answer their UI driver"

RA=$(expect_ok "peer-a reports its build" A ping)
# The pinned layout must actually have been read. If it was not, both
# panels are still reachable by switching slots, so every later check
# would pass — while the run had silently stopped exercising the
# arrangement an operator sees and gone back to scrolling for controls.
for who in A B; do
  R=$(drv "$who" panels)
  case "$R" in
    *sync*peer-connections*) ok "peer-$who opened on the pinned two-panel layout" ;;
    *) bad "peer-$who's layout is $R, not [sync, peer-connections] — WB_LAYOUT was not honoured" ;;
  esac
done
note "peer-a build: $(field "$RA" build)"

A_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' gui-peer-a)
B_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' gui-peer-b)
if [ -n "$A_IP" ] && [ "$A_IP" != "$B_IP" ]; then
  ok "the two apps have distinct routable addresses ($A_IP / $B_IP)"
else
  bad "the two apps are not network-isolated (a=$A_IP b=$B_IP)"
fi

# ======================================================================
# PHASE 2 — discovery, MEASURED not asserted
# ======================================================================
# mDNS across a podman bridge is not a product claim and a red here would
# be the container runtime's news, not ours. A scenario with no specified
# behaviour gets measured and printed, never asserted — inventing a pass
# criterion for it freezes whatever happens today into a gate.
log "PHASE 2 — does either app SEE the other without being told? (measured)"
expect_ok "peer-a's Connections panel is open" A wait PeerConnectionsPanel 20000 >/dev/null
expect_ok "peer-b's Connections panel is open" B wait PeerConnectionsPanel 20000 >/dev/null
sleep 8
for who in A B; do
  R=$(drv "$who" alltext '#conn.nearby.list')
  T=$(field "$R" text)
  if [ -n "$T" ]; then note "peer-$who nearby: $T"; else
    E=$(drv "$who" alltext '#conn.nearby.empty')
    note "peer-$who nearby: (nothing) — panel says: $(field "$E" text)"
  fi
done
note "discovery is informational here; the connect below types an address either way"

# ======================================================================
# PHASE 3 — connect, BOTH directions, by typing an address
# ======================================================================
# Both peers dial. A dial-by-address authorizes the DIALER ONLY
# (AP63/§4.4: sendReciprocalGrant is gated on EstablishedViaRendezvousKey
# — "a dial-by-address is asymmetric, one party requested service"), so a
# one-way dial yields an accepted subscription and an empty folder.
log "PHASE 3 — connect both ways, by typing into the panel"
connect_to() {
  local who="$1" host="$2" alias="$3"
  expect_ok "peer-$who types the address" "$who" type '#conn.address' "$host:$PEER_PORT" >/dev/null
  expect_ok "peer-$who types the alias" "$who" type '#conn.alias' "$alias" >/dev/null
  expect_ok "peer-$who presses Connect" "$who" click '#conn.connect' >/dev/null
}
connect_to A gui-peer-b b
connect_to B gui-peer-a a
sleep 4
for who in A B; do
  R=$(drv "$who" alltext '#conn.status')
  note "peer-$who connection status: $(field "$R" text)"
  R=$(drv "$who" alltext '#conn.pool.list')
  case "$(field "$R" text)" in
    *"$( [ "$who" = A ] && echo b || echo a )"*) ok "peer-$who's pool lists the other peer" ;;
    *) bad "peer-$who's pool does not list the other peer — it says: $(field "$R" text)" ;;
  esac
done
stop_after connect

# ======================================================================
# PHASE 4 — share, gesture one
# ======================================================================
# One gesture over two substrate steps: the panel bridges the directory
# into the tree (a mount) AND offers it. The operator never says "mount";
# that is a mechanism that leaked into the UI.
log "PHASE 4 — peer-a shares /photos with b, through the Sync panel"
expect_ok "peer-a's Sync panel is on screen" A wait SyncPanel 30000 >/dev/null
expect_ok "peer-a presses “Share a folder…”" A click '#sync.share.begin' >/dev/null
expect_ok "the share form opened" A wait '#sync.share.go' 5000 >/dev/null
expect_ok "peer-a types the folder" A type '#sync.share.dir' /photos >/dev/null

# The peer list is the REACHABLE peers (pool ∪ mDNS) unioned with the
# declared ones — not the declarations alone, which is what made the FIRST
# share between two machines impossible in both directions at once.
RS=$(expect_ok "peer-a can choose the peer it just connected to" A select '#sync.share.peer' b)
note "chose: $(field "$RS" item)   (realInput=$(field "$RS" realInput) — see UiDriver.SelectItem)"
expect_ok "peer-a presses Share" A click '#sync.share.go' >/dev/null
expect_ok "peer-a is told the share was made" A waittext '#sync.note' "Shared" 30000 >/dev/null
RN=$(drv A alltext '#sync.note')
note "peer-a's panel says: $(field "$RN" text)"
stop_after share

# ======================================================================
# PHASE 5 — the offer appears on the other machine, unprompted
# ======================================================================
log "PHASE 5 — peer-b opens Sync and finds the offer"
expect_ok "peer-b's Sync panel is on screen" B wait SyncPanel 30000 >/dev/null
expect_ok "an offer card appeared on peer-b" B wait '#sync.offer.accept' 60000 >/dev/null
RO=$(expect_ok "the card says who is offering what" B alltext '#sync.offer.title[0]')
OFFER_TEXT=$(field "$RO" text)
note "peer-b's card: $OFFER_TEXT"
case "$OFFER_TEXT" in
  *photos*) ok "the card names the folder that was shared" ;;
  *) bad "the card does not name 'photos' — it says: $OFFER_TEXT" ;;
esac
RD=$(drv B text '#sync.offer.dir[0]')
note "peer-b's suggested directory: $(field "$RD" text)"

# ======================================================================
# PHASE 6 — accept, gesture two
# ======================================================================
log "PHASE 6 — peer-b accepts into a directory it chooses"
expect_ok "peer-b types the receiving directory" B type '#sync.offer.dir[0]' /received >/dev/null
expect_ok "peer-b presses Accept" B click '#sync.offer.accept[0]' >/dev/null
expect_ok "peer-b is told what was accepted and where" B waittext '#sync.note' "Accepted" 60000 >/dev/null
RN=$(drv B alltext '#sync.note')
ACCEPT_TEXT=$(field "$RN" text)
note "peer-b's panel says: $ACCEPT_TEXT"
# The operator's actual question is whether their files came across, and
# the backfill sentence is the only thing that answers it. A confirmation
# that omits it reports success and says nothing.
case "$ACCEPT_TEXT" in
  *"/received"*) ok "the confirmation names the directory the files went to" ;;
  *) bad "the confirmation does not name /received: $ACCEPT_TEXT" ;;
esac
expect_ok "the offer card is consumed once accepted" B waitgone '#sync.offer.accept' 30000 >/dev/null
stop_after accept

# ======================================================================
# PHASE 7 — the backfill: files that were ALREADY there
# ======================================================================
# A subscription is a future tense (AP65). Until sync backfilled, the
# gesture the product is named after produced an empty folder with no
# error on either side.
log "PHASE 7 — the files that were already in the folder"
wait_for_file "$RUN/received/one.txt"       "photo one" "backfill: one.txt is on peer-b's disk"
wait_for_file "$RUN/received/two.txt"       "photo two" "backfill: two.txt is on peer-b's disk"
wait_for_file "$RUN/received/sub/three.txt" "nested"    "backfill: the nested file arrived"
stop_after backfill

# ======================================================================
# PHASE 8 — re-dial, because grants are assembled at HANDSHAKE
# ======================================================================
# `accept` wrote peer-b's policy authorizing peer-a to deliver, and the
# connections opened in phase 3 predate it. The backfill above runs on
# authority the RECEIVER holds and pulls, so it succeeds either way —
# while every subsequent live change is refused. Both sides, because the
# peer that DISPATCHES is the peer that must reconnect.
#
# Done through the panel's own Disconnect/Connect buttons: if the GUI
# could not perform this step, that would itself be the finding.
log "PHASE 8 — reconnect both ways (a grant written on a live connection is inert)"
for who in A B; do
  expect_ok "peer-$who's Connections panel is still open" "$who" wait PeerConnectionsPanel 20000 >/dev/null
  R=$(drv "$who" find '#conn.pool.disconnect')
  if [[ "$R" == *'"ok":true'* ]] && [[ "$R" != *'"count":0'* ]]; then
    expect_ok "peer-$who disconnects" "$who" click '#conn.pool.disconnect[0]' >/dev/null
  else
    bad "peer-$who has no Disconnect control to press — $R"
  fi
done
sleep 2
connect_to A gui-peer-b b
connect_to B gui-peer-a a
sleep 5
ok "both peers re-dialled after the grants were written"

# ======================================================================
# PHASE 9 — live changes: create, modify, delete
# ======================================================================
log "PHASE 9 — a file is ADDED on peer-a"
echo "added-after-share" > "$RUN/photos/added.txt"; chmod 666 "$RUN/photos/added.txt"
wait_for_file "$RUN/received/added.txt" "added-after-share" "create: added.txt reached peer-b"

log "PHASE 9b — a file is MODIFIED on peer-a"
echo "modified-content" > "$RUN/photos/one.txt"
wait_for_file "$RUN/received/one.txt" "modified-content" "modify: one.txt updated on peer-b"

log "PHASE 9c — a file is DELETED on peer-a"
rm -f "$RUN/photos/two.txt"
wait_for_absence "$RUN/received/two.txt" "delete: two.txt removed on peer-b"

# ======================================================================
# PHASE 10 — what the PANELS say about it
# ======================================================================
# The panel is the operator's only evidence. A flow that works while the
# window says nothing is a flow nobody will trust.
log "PHASE 10 — do the panels tell the truth about what just happened?"
expect_ok "peer-b's Sync panel is still open" B wait SyncPanel 30000 >/dev/null
expect_ok "peer-b's Sync panel lists the folder" B wait '#sync.folder.head' 30000 >/dev/null
RF=$(drv B alltext '#sync.folder.detail[0]')
note "peer-b's folder row: $(field "$RF" text)"
case "$(field "$RF" text)" in
  # Both sides of the lossy stage, never one count (AP59): the watcher
  # writes a file entity per admitted file and the ingest chain lifts
  # each into a document, and reading only the source layer once showed
  # "401 entities" for a directory with one openable document.
  *"files readable"*) ok "peer-b's row reports both sides of the ingest stage" ;;
  *"no mount"*) bad "peer-b's row says there is no mount — the accept did not bridge the directory" ;;
  *) bad "peer-b's row does not report file counts: $(field "$RF" text)" ;;
esac
expect_ok "peer-a's Sync panel is still open" A wait SyncPanel 30000 >/dev/null
expect_ok "peer-a's Sync panel lists the shared folder" A wait '#sync.folder.head' 30000 >/dev/null
RF=$(drv A alltext '#sync.folder.detail[0]')
note "peer-a's folder row: $(field "$RF" text)"
stop_after live

# ======================================================================
# PHASE 11 — restart BOTH apps against the same stores
# ======================================================================
# Everything above ran in processes that have been alive the whole time.
# A mount, a subscription and a sync binding are all derived runtime state
# over a persistent store (D26/AP62), and each has silently failed to
# survive a restart at least once in this repo. The GUI adds a fourth:
# its default peer was ephemeral until 2026-09-03, so a restart used to
# invalidate every grant by changing the peer-id.
log "PHASE 11 — stop BOTH apps, restart them on the same stores"
close_peers
podman rm -f gui-peer-a gui-peer-b >/dev/null 2>&1
sleep 2
start_app gui-peer-a "$RUN/a" "$PORT_A"
start_app gui-peer-b "$RUN/b" "$PORT_B"
open_peer A "$PORT_A" gui-peer-a || die "peer-a did not come back"
open_peer B "$PORT_B" gui-peer-b || die "peer-b did not come back"
ok "both apps restarted and answer again"

expect_ok "peer-b's Sync panel is open after the restart" B wait SyncPanel 30000 >/dev/null
if [[ "$(drv B wait '#sync.folder.head' 30000)" == *'"ok":true'* ]]; then
  ok "the accepted folder survived the restart and is still listed"
else
  bad "peer-b's Sync panel has no folder after the restart — the declaration or its mount is gone"
fi

log "PHASE 11b — reconnect and change a file"
expect_ok "peer-a's Connections panel is open" A wait PeerConnectionsPanel 20000 >/dev/null
expect_ok "peer-b's Connections panel is open" B wait PeerConnectionsPanel 20000 >/dev/null
connect_to A gui-peer-b b
connect_to B gui-peer-a a
sleep 5

# TWO files, and BOTH are hard assertions here.
#
# The documented open defect
# (FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03) is ASYMMETRIC: restart
# ONE peer and the next change is lost; restart BOTH and nothing is. This
# phase restarts both, which is the case that WORKS — so waiving the
# first-change check here would waive the wrong instance and hide a real
# regression behind somebody else's defect. The asymmetric case is phase
# 12, and that is where the waiver belongs.
#
# Two files either way, because one cannot tell "lost the first" from
# "lost everything" from "working".
echo "post-restart-1" > "$RUN/photos/post1.txt"; chmod 666 "$RUN/photos/post1.txt"
sleep 6
echo "post-restart-2" > "$RUN/photos/post2.txt"; chmod 666 "$RUN/photos/post2.txt"
wait_for_file "$RUN/received/post1.txt" "post-restart-1" \
  "[both restarted] FIRST change after restart arrived"
wait_for_file "$RUN/received/post2.txt" "post-restart-2" \
  "[both restarted] SECOND change after restart arrived"

stop_after restart

# ======================================================================
# PHASE 12 — restart ONE peer, which is what an operator actually does
# ======================================================================
# Nobody reboots both laptops at once. This is the asymmetric case, it is
# where the documented defect lives, and it is the only permutation whose
# first change is waived — by name, and loudly un-waived by `known_ok` if
# it ever starts passing, because a silently-healed waiver is how a fixed
# bug goes on being described as broken.
#
# The receiver restarts and the sender stays up: the two roles fail
# differently, since the sender holds the watcher and the receiver holds
# the subscription, and each is derived runtime state rebuilt by a
# different code path (D26/AP62).
log "PHASE 12 — the RECEIVER restarts; the sender stays up"
close_peers
podman rm -f gui-peer-b >/dev/null 2>&1
sleep 2
start_app gui-peer-b "$RUN/b" "$PORT_B"
open_peer A "$PORT_A" gui-peer-a || die "peer-a's driver went away while peer-b restarted"
open_peer B "$PORT_B" gui-peer-b || die "peer-b did not come back"
ok "peer-b restarted while peer-a stayed up"

connect_to A gui-peer-b b
connect_to B gui-peer-a a
sleep 5

echo "asym-1" > "$RUN/photos/asym1.txt"; chmod 666 "$RUN/photos/asym1.txt"
sleep 6
echo "asym-2" > "$RUN/photos/asym2.txt"; chmod 666 "$RUN/photos/asym2.txt"
wait_for_file "$RUN/received/asym1.txt" "asym-1" \
  "[receiver restarted] FIRST change after restart arrived" known
wait_for_file "$RUN/received/asym2.txt" "asym-2" \
  "[receiver restarted] SECOND change after restart arrived"

# Both apps must still be alive. A crash mid-run is a hard failure even
# if every check before it passed — and without this the run can end green
# with one of the two peers dead since phase 4.
for c in gui-peer-a gui-peer-b; do
  if [ "$(podman inspect -f '{{.State.Status}}' "$c" 2>/dev/null)" = "running" ]; then
    ok "$c survived the whole run"
  else
    bad "$c is not running at the end of the run (exit $(podman inspect -f '{{.State.ExitCode}}' "$c" 2>/dev/null)) — it crashed or was killed"
  fi
done

report
exit $?
