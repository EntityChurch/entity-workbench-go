#!/usr/bin/env bash
# conflict-semantics.sh — what ACTUALLY happens when two peers change the
# same file, measured against the behaviour the spec says to expect.
#
# WHY THIS EXISTS. `twopeer-sync.sh` PHASE 8b already exercised a
# concurrent edit and recorded the outcome with the comment *"there is no
# specified behaviour to assert against yet"*. **That sentence is false**,
# and it is why this sat parked as an open design question for days.
#
# DOMAIN-LOCAL-FILES §1.1a (v1.3 Amendment 4) specifies it exactly, and
# names our own case as what validated it ("workbench-go's WB-25 case,
# Stage 4 round-3"). Three guarantees:
#
#   1. TREE CONVERGENCE — both peers' writes produce blob entities at
#      distinct content hashes; the tree records both as distinct chain
#      events; subscriptions deliver both.
#   2. FILESYSTEM PARTIAL SWAP — the reverse-write pipeline applies the
#      most-recently-arrived write to disk; the on-disk file reflects the
#      last arrival.
#   3. NO AUTOMATIC MERGE — bytes are not three-way merged at the
#      substrate level. "Operators wanting merge semantics layer it via
#      the revision extension."
#
# So last-write-wins at the filesystem surface is INTENDED, matches the
# rsync / git-checkout mental model, and is not a defect. **The question
# this harness answers is guarantee 1**, because it is the one that
# decides what we are even looking at:
#
#   - if BOTH blobs are in the tree, the losing bytes are RECOVERABLE and
#     what we lack is conflict *surfacing* — a UI/detection feature we can
#     build, and the industry-standard answer (Syncthing, Dropbox, OneDrive
#     and iCloud all keep a renamed second copy) maps onto the spec's own
#     `keep-both` strategy.
#   - if only ONE blob survives, the bytes are genuinely gone and that is
#     a substrate-level finding to route.
#
# Those are very different reports and "a concurrent edit silently
# destroys the receiver's file" was written without distinguishing them.
#
# WHAT A GREEN RUN CLAIMS: the on-disk outcome of each concurrent-change
# case on both peers, and whether the superseded bytes are still
# addressable in the loser's own tree.
#   IT DOES NOT CLAIM: anything about the revision extension's merge
#   framework, which this flow never invokes (we do not commit versions,
#   so nothing ever merges). See §5 of the packet.
#
#   bash scripts/conflict-semantics.sh
#   KEEP_UP=1 bash scripts/conflict-semantics.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
NET="${SYNC_NET:-entity-conflict}"
IMG="${SYNC_IMG:-docker.io/library/alpine:3.20}"
BIN="${SHELL_BIN:-/tmp/entity-shell-conflict}"
RUNDIR="${RUNDIR:-/tmp/entity-conflict-$$}"
PORT=9000

FAILURES=(); CHECKS=0; NOTES=()
log()  { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
ok()   { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s\n' "$*" >&2; }
bad()  { CHECKS=$((CHECKS+1)); FAILURES+=("$*"); printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; }
note() { NOTES+=("$*"); printf '\033[1;36m  ..\033[0m  %s\n' "$*" >&2; }
die()  { printf '\033[1;31mFATAL:\033[0m %s\n' "$*" >&2; exit 1; }

report() {
  echo >&2; log "================ RESULT ================"
  [ "$CHECKS" -eq 0 ] && { log "NO CHECKS RAN — that is a failure, not a pass."; return 1; }
  log "$CHECKS checks, ${#FAILURES[@]} failed, ${#NOTES[@]} measured"
  for n in "${NOTES[@]}"; do printf '\033[1;36m  ..\033[0m %s\n' "$n" >&2; done
  if [ ${#FAILURES[@]} -gt 0 ]; then
    for f in "${FAILURES[@]}"; do printf '\033[1;31m  x\033[0m %s\n' "$f" >&2; done
    return 1
  fi
  log "CONFLICT SEMANTICS CHARACTERISED — see the measurements above."
  return 0
}

command -v podman >/dev/null || die "podman is required"
cleanup() {
  [ "${KEEP_UP:-0}" = "1" ] && { log "KEEP_UP=1 — peers left up (podman rm -f peer-a peer-b)"; return; }
  podman rm -f peer-a peer-b >/dev/null 2>&1 || true
  podman network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log "building entity-shell static"
( cd "$REPO/shell" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BIN" ./cmd/entity-shell ) \
  || die "build failed"

mkdir -p "$RUNDIR"/{a,b,photos,received}
chmod -R 777 "$RUNDIR"

podman network rm "$NET" >/dev/null 2>&1 || true
podman network create "$NET" >/dev/null || die "network create failed"

# EACH PEER SEES ONLY ITS OWN DIRECTORY.
#
# The first version of this harness bind-mounted BOTH $RUNDIR/photos and
# $RUNDIR/received into BOTH containers, which is what twopeer-sync.sh
# does. That is fine there and it is NOT fine here: it gives the RECEIVER
# a real /photos directory that is the PUBLISHER's source, so a
# mis-resolved mount can land on the publisher's own bytes and the
# resulting measurement is an artifact of the fixture.
#
# It produced exactly that: peer-b's root config came back as
# `filesystem_root: /photos`, which reads as a serious mount defect and
# could equally have been the harness handing it a directory it would
# never have on a real machine. A fixture that gives a peer access it
# could not have cannot be used to characterise what that peer does.
start_peer() {
  local name="$1" dir="$2" own="$3" mnt="$4"
  podman run -d --name "$name" --network "$NET" --hostname "$name" \
    -v "$BIN:/entity-shell:z,ro" -v "$dir:/data:z" \
    -v "$own:$mnt:z" \
    -e HOME=/data "$IMG" sh -c "
      mkfifo /tmp/in 2>/dev/null; sleep infinity > /tmp/in &
      exec /entity-shell -alias $name -storage sqlite \
        -storage-path /data/store.db -listen 0.0.0.0:$PORT < /tmp/in > /data/out.log 2>&1
    " >/dev/null || die "could not start $name"
}
say() {
  local c="$1"; shift; local before after i
  before=$(podman exec "$c" sh -c "grep -c 'entity:/ >' /data/out.log 2>/dev/null || echo 0")
  podman exec "$c" sh -c "printf '%s\n' \"$*\" > /tmp/in"
  for i in $(seq 1 100); do
    after=$(podman exec "$c" sh -c "grep -c 'entity:/ >' /data/out.log 2>/dev/null || echo 0")
    [ "$after" -gt "$before" ] && return 0
    sleep 0.2
  done
  bad "[$c] timed out: $*"; return 1
}
out() { podman exec "$1" sh -c "cat /data/out.log 2>/dev/null"; }
# say_out CONTAINER CMD... — run a command and return ONLY its new output.
say_out() {
  local c="$1"; shift; local before
  before=$(podman exec "$c" sh -c "wc -l < /data/out.log 2>/dev/null || echo 0")
  say "$c" "$@" >/dev/null 2>&1
  out "$c" | tail -n "+$((before + 1))"
}
wait_for_file() {
  local path="$1" want="$2" label="$3" i
  for i in $(seq 1 80); do
    [ -f "$path" ] && [ "$(cat "$path" 2>/dev/null)" = "$want" ] && { ok "$label"; return 0; }
    sleep 0.5
  done
  bad "$label — got '$(cat "$path" 2>/dev/null)', wanted '$want'"; return 1
}

# ---- setup: A publishes /photos, B receives into /received -----------
log "SETUP — two peers, one shared folder"
start_peer peer-a "$RUNDIR/a" "$RUNDIR/photos" /photos
start_peer peer-b "$RUNDIR/b" "$RUNDIR/received" /received
sleep 4
echo "v0" > "$RUNDIR/photos/doc.txt"
echo "v0" > "$RUNDIR/photos/del.txt"
chmod -R 777 "$RUNDIR/photos"

say peer-a "mount /photos archives/photos"; sleep 2
say peer-b "connect a peer-a:$PORT"; say peer-a "connect b peer-b:$PORT"; sleep 1
say peer-a "share photos with b"; sleep 2
say peer-b "accept a photos /received"; sleep 3
say peer-a "disconnect b"; say peer-b "disconnect a"; sleep 1
say peer-a "connect b peer-b:$PORT"; say peer-b "connect a peer-a:$PORT"; sleep 3
wait_for_file "$RUNDIR/received/doc.txt" "v0" "baseline replicated to the receiver"

# Locate the file entity in EACH peer's tree. The two peers use different
# root names on purpose in the general case, so the path is discovered
# rather than assumed.
log "locating the file entity in each peer's tree"
A_TREE=$(say_out peer-a "find / doc.txt" | grep -oE '[^ ]*doc\.txt' | head -3 | tr '\n' ' ')
B_TREE=$(say_out peer-b "find / doc.txt" | grep -oE '[^ ]*doc\.txt' | head -3 | tr '\n' ' ')
note "peer-a tree paths for doc.txt: ${A_TREE:-<none found>}"
note "peer-b tree paths for doc.txt: ${B_TREE:-<none found>}"

# ============ CASE 1 — EDIT vs EDIT, receiver edits first ============
# The common real case: the operator edits their copy of a received
# folder while the publisher edits theirs. Publisher's write arrives
# last, so per §1.1a guarantee 2 the publisher's bytes are what the
# receiver ends up holding on disk.
#
# THE MEASUREMENT THAT MATTERS is guarantee 1: after that, are the
# receiver's own bytes still addressable in the receiver's tree?
log "CASE 1 — edit vs edit (receiver edits, then publisher edits)"
echo "edited-on-B" > "$RUNDIR/received/doc.txt"
sleep 4
B_AFTER_OWN=$(cat "$RUNDIR/received/doc.txt" 2>/dev/null)
note "case1: receiver's own edit on disk before the publisher's write: '$B_AFTER_OWN'"

echo "edited-on-A" > "$RUNDIR/photos/doc.txt"
sleep 10
A_DISK=$(cat "$RUNDIR/photos/doc.txt" 2>/dev/null)
B_DISK=$(cat "$RUNDIR/received/doc.txt" 2>/dev/null)
note "case1 ON DISK: publisher='$A_DISK'  receiver='$B_DISK'"

if [ "$B_DISK" = "edited-on-A" ]; then
  ok "case1: last arrival wins at the FS surface (DOMAIN-LOCAL-FILES §1.1a guarantee 2)"
elif [ "$B_DISK" = "edited-on-B" ]; then
  bad "case1: the publisher's write did NOT reach the receiver — this is a delivery bug, not a conflict"
else
  bad "case1: receiver holds unexpected content '$B_DISK'"
fi

# Guarantee 1 — is the superseded content still in the loser's tree?
log "CASE 1b — are the receiver's superseded bytes still ADDRESSABLE in its own tree?"
B_CHAIN=""
for p in $B_TREE; do
  c=$(say_out peer-b "inspect chain $p" 2>/dev/null)
  [ -n "$c" ] && B_CHAIN="$B_CHAIN$c"
done
CHAIN_EVENTS=$(printf '%s' "$B_CHAIN" | grep -ciE 'revision|chain|event|entry|hash' || true)
note "case1b: receiver chain inspection produced $CHAIN_EVENTS matching lines"
if printf '%s' "$B_CHAIN" | grep -q .; then
  note "case1b: chain excerpt: $(printf '%s' "$B_CHAIN" | tr '\n' ' ' | cut -c1-300)"
fi
# The decisive test, done by content rather than by reading a chain
# format: can the receiver still find its own superseded bytes anywhere
# in its tree? grep searches entity CONTENT, so a surviving blob answers.
B_GREP=$(say_out peer-b "grep / edited-on-B -l" 2>/dev/null | grep -vE '^\s*$|entity:/ >' | head -5)
if printf '%s' "$B_GREP" | grep -q '[a-z]'; then
  ok "case1b: the receiver's superseded bytes ARE still addressable — divergence is recorded, not destroyed"
  note "case1b: found at: $(printf '%s' "$B_GREP" | tr '\n' ' ' | cut -c1-200)"
else
  bad "case1b: the receiver's superseded bytes are GONE from its own tree — guarantee 1 does not hold here"
fi

# ============ CASE 2 — EDIT vs DELETE ================================
log "CASE 2 — receiver edits while the publisher DELETES the same file"
wait_for_file "$RUNDIR/received/del.txt" "v0" "case2: baseline present at the receiver"
echo "edited-on-B" > "$RUNDIR/received/del.txt"
sleep 3
rm -f "$RUNDIR/photos/del.txt"
sleep 10
if [ -f "$RUNDIR/received/del.txt" ]; then
  note "case2: the delete did NOT remove the receiver's edited copy (content='$(cat "$RUNDIR/received/del.txt")')"
else
  note "case2: the delete REMOVED the receiver's edited copy — the edit signal is dropped"
fi
B_GREP2=$(say_out peer-b "grep / edited-on-B -l" 2>/dev/null | grep -vE '^\s*$|entity:/ >' | head -5)
if printf '%s' "$B_GREP2" | grep -q '[a-z]'; then
  note "case2: the receiver's edited bytes remain addressable in its tree"
else
  note "case2: the receiver's edited bytes are NOT addressable after the delete"
fi

# ============ CASE 3 — the control arm ===============================
# A receiver-only edit with NO competing write must simply survive. If
# this fails, everything above is measuring a delivery bug rather than a
# conflict.
log "CASE 3 — control: a receiver edit with no competing write must survive"
echo "solo-on-B" > "$RUNDIR/received/solo.txt"
sleep 8
if [ "$(cat "$RUNDIR/received/solo.txt" 2>/dev/null)" = "solo-on-B" ]; then
  ok "case3: a receiver-only file is left alone (so cases 1-2 are conflicts, not deletions)"
else
  bad "case3: a receiver-only file did not survive — cases 1-2 are not measuring what they claim"
fi
if [ -f "$RUNDIR/photos/solo.txt" ]; then
  bad "case3: a receive-only folder published upstream — solo.txt reached the publisher"
else
  ok "case3: receive-only holds — the receiver's own file did not travel upstream"
fi

report
