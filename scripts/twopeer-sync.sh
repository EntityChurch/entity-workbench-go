#!/usr/bin/env bash
# twopeer-sync.sh — the full sharing lifecycle, two peers, two containers,
# one real TCP network. Nobody's laptop required.
#
# WHY THIS EXISTS. Every gate in this repo for the sharing flow ran two
# peers IN ONE PROCESS, on loopback, inside one Go test binary. That is
# enough to establish the protocol works and it cannot see: a restart, a
# real listener bind, DNS between hosts, a file appearing on a disk one
# peer owns and the other does not, or anything that only goes wrong once
# the two halves stop sharing a heap. So the product's own operator was
# the integration test, on two physical machines, by hand — which is not
# a test, because it cannot be re-run and it produces no artifact.
#
# WHAT A GREEN RUN CLAIMS: two peers in separate network namespaces with
# distinct routable addresses, persistent sqlite stores on separate
# volumes, discovering each other by container DNS over a real TCP hop,
# establishing a share, and moving file CREATE / MODIFY / DELETE across
# it — then surviving a shutdown and restart of both, with a change made
# after the restart still arriving.
#   IT DOES NOT CLAIM: two physical machines, the public internet, NAT
#   traversal, mDNS discovery (container DNS stands in), or the GUI. The
#   GUI's panels are covered by the headless suite; this is the substrate
#   underneath them.
#
#   bash scripts/twopeer-sync.sh          # up -> exercise -> down
#   KEEP_UP=1 bash scripts/twopeer-sync.sh
#   PHASE=restart bash scripts/twopeer-sync.sh   # stop after a named phase
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
NET="${SYNC_NET:-entity-twopeer}"
IMG="${SYNC_IMG:-docker.io/library/alpine:3.20}"
BIN="${SHELL_BIN:-/tmp/entity-shell-twopeer}"
RUNDIR="${RUNDIR:-/tmp/entity-twopeer-$$}"
PORT=9000

# Every failure is collected and reported at the end rather than aborting
# at the first one (AP15): a run that stops at the first red tells you one
# thing when the tree may be wrong in four ways, and the count it yields
# is a lower bound.
FAILURES=()
CHECKS=0

log()  { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
ok()   { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s\n' "$*" >&2; }
bad()  { CHECKS=$((CHECKS+1)); FAILURES+=("$*"); printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mFATAL:\033[0m %s\n' "$*" >&2; exit 1; }

report() {
  echo >&2
  log "================ RESULT ================"
  log "$CHECKS checks, ${#FAILURES[@]} failed"
  if [ ${#FAILURES[@]} -gt 0 ]; then
    for f in "${FAILURES[@]}"; do printf '\033[1;31m  x\033[0m %s\n' "$f" >&2; done
    echo >&2
    log "peer-a log tail:"; out peer-a 2>/dev/null | tail -25 >&2
    log "peer-b log tail:"; out peer-b 2>/dev/null | tail -25 >&2
    return 1
  fi
  log "TWO-PEER SYNC GREEN across create, modify, delete and a restart."
  return 0
}


command -v podman >/dev/null || die "podman is required"

cleanup() {
  if [ "${KEEP_UP:-0}" = "1" ]; then
    log "KEEP_UP=1 — leaving both peers up. Tear down with:"
    log "  podman rm -f peer-a peer-b; podman network rm $NET"
    return
  fi
  log "tearing down"
  podman rm -f peer-a peer-b >/dev/null 2>&1 || true
  podman network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- build ------------------------------------------------------------
# CGO off: the binary has to run in a bare image. modernc.org/sqlite is
# pure Go, so the persistent backend survives this.
log "building entity-shell static (CGO off, linux/amd64)"
( cd "$REPO/shell" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BIN" ./cmd/entity-shell ) \
  || die "build failed"

mkdir -p "$RUNDIR/a" "$RUNDIR/b" "$RUNDIR/photos" "$RUNDIR/received"
chmod -R 777 "$RUNDIR"

# --- network ----------------------------------------------------------
podman network rm "$NET" >/dev/null 2>&1 || true
podman network create "$NET" >/dev/null || die "could not create network $NET"
log "network $NET up"

# start_peer NAME HOSTDIR — a long-lived shell with a live listener,
# driven through a FIFO. `sleep infinity > fifo` holds the write end open
# so the REPL does not see EOF between commands and exit.
start_peer() {
  local name="$1" dir="$2"
  podman run -d --name "$name" --network "$NET" --hostname "$name" \
    -v "$BIN:/entity-shell:z,ro" \
    -v "$dir:/data:z" \
    -v "$RUNDIR/photos:/photos:z" \
    -v "$RUNDIR/received:/received:z" \
    -e HOME=/data \
    "$IMG" sh -c "
      mkfifo /tmp/in 2>/dev/null;
      sleep infinity > /tmp/in &
      exec /entity-shell -alias $name -storage sqlite \
        -storage-path /data/store.db -listen 0.0.0.0:$PORT < /tmp/in > /data/out.log 2>&1
    " >/dev/null || die "could not start $name"
}

# say CONTAINER CMD... — send one command and wait for the REPL to finish
# it, by counting prompts in the log. Polling the log rather than sleeping
# is what keeps this from being flaky on a loaded machine.
say() {
  local c="$1"; shift
  local before after i
  before=$(podman exec "$c" sh -c "grep -c 'entity:/ >' /data/out.log 2>/dev/null || echo 0")
  podman exec "$c" sh -c "printf '%s\n' \"$*\" > /tmp/in"
  for i in $(seq 1 100); do
    after=$(podman exec "$c" sh -c "grep -c 'entity:/ >' /data/out.log 2>/dev/null || echo 0")
    [ "$after" -gt "$before" ] && return 0
    sleep 0.2
  done
  bad "[$c] command timed out: $*"
  return 1
}

# out CONTAINER — the whole log so far.
out() { podman exec "$1" sh -c "cat /data/out.log 2>/dev/null"; }

# expect CONTAINER PATTERN LABEL — assert the log contains PATTERN.
expect() {
  local c="$1" pat="$2" label="$3"
  if out "$c" | grep -qE "$pat"; then ok "$label"; else
    bad "$label — [$c] log has no /$pat/"
  fi
}

# log_lines CONTAINER — how many lines the log has right now. Paired with
# expect_after to scope an assertion to ONE command's output.
log_lines() { podman exec "$1" sh -c "wc -l < /data/out.log 2>/dev/null || echo 0"; }

# expect_after CONTAINER MARK PATTERN LABEL — assert PATTERN appears in
# the log BELOW line MARK.
#
# `expect` greps the whole log, which is right for "did this peer ever
# report X" and WRONG for "what did that command just print": a filename
# mentioned by an earlier phase satisfies it, so the assertion passes
# without the command under test having produced anything. Every listing
# assertion needs this form.
expect_after() {
  local c="$1" mark="$2" pat="$3" label="$4"
  if out "$c" | tail -n +"$((mark + 1))" | grep -qE "$pat"; then ok "$label"; else
    bad "$label — [$c] nothing matching /$pat/ in the output of that command"
  fi
}

# ---- files_match — the assertion that actually matters ---------------
# Compares what peer B has on DISK against what peer A has on disk. A
# tree-count assertion would pass while the operator's folder was empty,
# which is exactly the class of bug this harness exists to catch.
wait_for_file() {
  local path="$1" want="$2" label="$3" i content
  for i in $(seq 1 60); do
    if [ -f "$path" ]; then
      content=$(cat "$path" 2>/dev/null)
      [ "$content" = "$want" ] && { ok "$label"; return 0; }
    fi
    sleep 0.5
  done
  if [ -f "$path" ]; then
    bad "$label — file exists but content is '$(cat "$path" 2>/dev/null)', wanted '$want'"
  else
    bad "$label — $path never appeared (30s)"
  fi
  return 1
}

wait_for_absence() {
  local path="$1" label="$2" i
  for i in $(seq 1 60); do
    [ -f "$path" ] || { ok "$label"; return 0; }
    sleep 0.5
  done
  bad "$label — $path is still present after 30s"
  return 1
}

# --- phase 1: bring both peers up -------------------------------------
log "PHASE 1 — two peers, two containers, two stores"
start_peer peer-a "$RUNDIR/a"
start_peer peer-b "$RUNDIR/b"
sleep 3
for c in peer-a peer-b; do
  if podman exec "$c" sh -c "grep -q 'Local peer' /data/out.log"; then
    ok "$c came up with a peer"
  else
    bad "$c did not start — log follows"; out "$c" | tail -20 >&2
  fi
done

A_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' peer-a)
B_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' peer-b)
log "peer-a=$A_IP peer-b=$B_IP (distinct namespaces, real addresses)"
[ "$A_IP" != "$B_IP" ] && ok "the two peers have distinct routable addresses" \
  || bad "both peers report the same address — they are not isolated"

# --- phase 2: mount a folder with content already in it ---------------
# Pre-existing content on purpose: a subscription is a future tense
# (AP65), so a folder that already has files is the case that used to
# transfer nothing.
log "PHASE 2 — mount, with files already present"
echo "first" > "$RUNDIR/photos/one.txt"
echo "second" > "$RUNDIR/photos/two.txt"
mkdir -p "$RUNDIR/photos/sub"
echo "nested" > "$RUNDIR/photos/sub/three.txt"
chmod -R 777 "$RUNDIR/photos"

# The root name is the DIRECTORY BASENAME, not the tree prefix — so the
# directory is called photos and every later verb names `photos`.
say peer-a "mount /photos archives/photos"
sleep 2
say peer-a "mounts"
expect peer-a "photos" "peer-a reports the mount"

# --- phase 3: connect both directions ---------------------------------
# BOTH peers dial. A dial-by-address authorizes the DIALER only
# (AP63/§4.4), so a one-way dial gives an accepted subscription and an
# empty folder.
log "PHASE 3 — connect, both directions"
say peer-b "connect a peer-a:$PORT"
say peer-a "connect b peer-b:$PORT"
sleep 1
expect peer-b "peer-a|connected|handshake|a " "peer-b dialed peer-a"

# --- phase 4: share and accept ----------------------------------------
log "PHASE 4 — share and accept"
say peer-a "share photos with b"
sleep 2
say peer-b "offers a"
say peer-b "accept a photos /received"
sleep 3
say peer-b "status"

# RE-DIAL, and it is not belt-and-braces. Grants are assembled at
# HANDSHAKE (AP63), so a policy written on a live connection is inert
# until that connection is re-established — and `accept` is what writes
# peer-b's policy authorizing peer-a to deliver. The connections opened
# in phase 3 predate it, so without this the backfill succeeds (it runs
# on authority the RECEIVER holds and pulls) while every subsequent live
# change is refused. Both sides, because the peer that DISPATCHES is the
# peer that must reconnect.
log "PHASE 4b — re-dial, because grants are assembled at handshake"
say peer-a "disconnect b"
say peer-b "disconnect a"
sleep 1
say peer-a "connect b peer-b:$PORT"
say peer-b "connect a peer-a:$PORT"
sleep 3

# --- phase 5: the backfill --------------------------------------------
log "PHASE 5 — the files that were already there"
wait_for_file "$RUNDIR/received/one.txt"       "first"  "backfill: one.txt arrived"
wait_for_file "$RUNDIR/received/two.txt"       "second" "backfill: two.txt arrived"
wait_for_file "$RUNDIR/received/sub/three.txt" "nested" "backfill: nested file arrived"

# --- phase 5b: the receiver's TREE, not just its disk ------------------
# Every assertion above this point is about bytes on disk, and for a long
# time that was every assertion in this harness. It is not enough: a
# delivered file has to become an ENTITY on the receiver, or there is
# nothing for conflict detection to compare against, nothing for
# `revision` to version, and blob-resolve's already-current check can
# never fire.
#
# Ask the SHELL, not the store. The alpine image ships no sqlite3, and
# copying the store out is worse than useless — file-backed stores open
# WAL, so a `podman cp store.db` without its `-wal` sidecar answers a
# question about a database that is missing every recent write, and
# reports the absence as zero rows with no error. That is AP76, and it
# cost a milestone. `shellboot/receive_binds_e2e_test.go` asserts that
# `ls` agrees with the location index, which is what makes asking the
# shell here sound.
log "PHASE 5b — the receiver's TREE, not just its disk"
#
# `@peer-b/` and not a bare relative path: at the REPL root the working
# directory is `/`, which is the CONNECTION list and not a peer
# namespace, so `ls local/files/...` resolves to `/local/files/...` and
# fails with `no connection for path`. The alias form is the pinned
# sigil (AGENTS.md); the peer's own alias is what it was started with.
mark=$(log_lines peer-b)
say peer-b "ls @peer-b/local/files/received/"
expect_after peer-b "$mark" "one\.txt" "tree: the receiver BOUND one.txt, not only wrote it"
expect_after peer-b "$mark" "two\.txt"  "tree: a second delivered file is bound"

# --- phase 6: create --------------------------------------------------
log "PHASE 6 — a file is ADDED on peer-a"
echo "added-after-share" > "$RUNDIR/photos/added.txt"
chmod 666 "$RUNDIR/photos/added.txt"
wait_for_file "$RUNDIR/received/added.txt" "added-after-share" "create: added.txt propagated"

# --- phase 7: modify --------------------------------------------------
log "PHASE 7 — a file is MODIFIED on peer-a"
echo "modified-content" > "$RUNDIR/photos/one.txt"
wait_for_file "$RUNDIR/received/one.txt" "modified-content" "modify: one.txt updated"

# --- phase 8: delete --------------------------------------------------
log "PHASE 8 — a file is REMOVED on peer-a"
rm -f "$RUNDIR/photos/two.txt"
wait_for_absence "$RUNDIR/received/two.txt" "delete: two.txt removed downstream"

[ "${PHASE:-all}" = "prerestart" ] && { report; exit 0; }

# --- phase 8b: RENAME, and a concurrent edit ---------------------------
# Two cases every mature sync product has an explicit answer for and this
# one has never been asked.
#
# RENAME has no event of its own — a filesystem watcher sees unlink+create
# — so the question is whether the new name arrives AND the old one goes.
# CONCURRENT EDIT is the conflict case: both sides change the same file
# while the link is up. Dropbox and Syncthing both keep a conflicted copy;
# what this product does is undefined and undocumented, and "undefined" on
# an operator's files is the worst outcome in the catalogue.
log "PHASE 8b — rename, and a concurrent edit"
mv "$RUNDIR/photos/one.txt" "$RUNDIR/photos/renamed.txt"
wait_for_file "$RUNDIR/received/renamed.txt" "modified-content" "rename: new name arrived"
wait_for_absence "$RUNDIR/received/one.txt" "rename: old name removed"

echo "edited-on-A" > "$RUNDIR/photos/conflict.txt"
chmod 666 "$RUNDIR/photos/conflict.txt"
wait_for_file "$RUNDIR/received/conflict.txt" "edited-on-A" "conflict: seed file replicated"
# Now change it on BOTH sides, receiver last.
echo "edited-on-A-again" > "$RUNDIR/photos/conflict.txt"
echo "edited-on-B" > "$RUNDIR/received/conflict.txt"
sleep 8
CONFLICT_A=$(cat "$RUNDIR/photos/conflict.txt" 2>/dev/null)
CONFLICT_B=$(cat "$RUNDIR/received/conflict.txt" 2>/dev/null)
log "conflict outcome: sender='$CONFLICT_A' receiver='$CONFLICT_B'"
ls "$RUNDIR/received/" | grep -i conflict | sed 's/^/     received: /' >&2
# THE PREVIOUS COMMENT HERE SAID "there is no specified behaviour to
# assert against yet". THAT WAS FALSE, and it parked this question for
# days — a dismissal written into a comment, where review never re-reads
# it (D27).
#
# DOMAIN-LOCAL-FILES §1.1a specifies it exactly, and credits our own
# WB-25 case with validating it: last-arrival wins at the filesystem
# surface, BOTH writes recorded in the tree at distinct chain positions,
# no automatic merge, and EXTENSION-REVISION is where collaborative-edit
# semantics live.
#
# It IS asserted here now, and the reason it was not is worth recording
# because it was wrong twice over. The old comment said this harness only
# reads the disk (fixed — PHASE 5b reads the tree through the shell) and
# that `make conflict-semantics` had found the receiving peer binds no
# file entity at all. THAT FINDING WAS AN ARTIFACT of reading a `podman
# cp` of a live WAL SQLite store without its sidecar (AP76). The receiver
# binds correctly, and the chain below is the proof an operator can see.
ok "conflict: on-disk outcome recorded"

# The tree-side half of §1.1a: BOTH writes at distinct chain positions,
# so the edit that lost on disk is still addressable. The reconciler
# installs the recording config for a folder that receives
# (shellcmd/folder_history.go), so nothing here turns it on — if this
# fails because no transitions were recorded, that is the finding.
#
# `@peer-b/` for PHASE 5b's reason. Two positions minimum: one authored
# by the WATCHER (peer-b's own edit) and one by the delivery.
mark=$(log_lines peer-b)
say peer-b "history query @peer-b/local/files/received/conflict.txt"
if out peer-b | tail -n +"$((mark + 1))" | grep -qE "no transitions recorded"; then
  bad "conflict: nothing was recorded — the overwritten edit is unrecoverable"
else
  n=$(out peer-b | tail -n +"$((mark + 1))" | grep -cE "(created|updated)")
  if [ "$n" -lt 2 ]; then
    bad "conflict: only $n chain position(s); §1.1a requires both writes recorded"
  else
    ok "conflict: both writes are on the chain ($n positions) — the losing edit is addressable"
  fi
  # A count alone is satisfied by four DELIVERIES. The claim is that both
  # SIDES are represented, and the chain says which is which: a local
  # edit is ingested by the watcher, a delivered one is dispatched by
  # blob-resolve. Without this arm the check above passes on a chain in
  # which peer-b's own edit was never recorded at all.
  if out peer-b | tail -n +"$((mark + 1))" | grep -q "local/files:watch"; then
    ok "conflict: peer-b's OWN edit is on the chain (local/files:watch)"
  else
    bad "conflict: no watcher-authored position — peer-b's own edit was not recorded"
  fi
  if out peer-b | tail -n +"$((mark + 1))" | grep -q "local/files:write"; then
    ok "conflict: the DELIVERED edit is on the chain (local/files:write)"
  else
    bad "conflict: no delivery-authored position on the chain"
  fi
fi

# restart_and_check WHO LABEL N — restart one or both peers, re-dial, then
# assert a change made AFTER the restart arrives. Factored because the
# three permutations differ only in who goes down, and the interesting
# answer is whether they differ in OUTCOME.
#
# Each permutation writes TWO files: the documented open defect
# (FIRST-CHANGE-AFTER-RESTART-IS-LOST, 2026-09-03) is that the FIRST
# change after a restart is silently dropped while every one after it
# lands, so a single file cannot tell "lost the first" from "lost
# everything" from "working".
restart_and_check() {
  local who="$1" label="$2" n="$3"
  log "PHASE $label"
  case "$who" in
    a)    podman rm -f peer-a >/dev/null 2>&1; sleep 1; start_peer peer-a "$RUNDIR/a" ;;
    b)    podman rm -f peer-b >/dev/null 2>&1; sleep 1; start_peer peer-b "$RUNDIR/b" ;;
    both) podman rm -f peer-a peer-b >/dev/null 2>&1; sleep 1
          start_peer peer-a "$RUNDIR/a"; start_peer peer-b "$RUNDIR/b" ;;
  esac
  sleep 5
  for c in peer-a peer-b; do
    podman exec "$c" sh -c "grep -q 'Local peer' /data/out.log" \
      && ok "[$label] $c is up" || bad "[$label] $c did not come back"
  done

  # Re-dial from whichever side went down. A connection is process
  # memory; the peer that DISPATCHES is the peer that must reconnect.
  say peer-b "connect a peer-a:$PORT"
  say peer-a "connect b peer-b:$PORT"
  sleep 3

  echo "${label}-first" > "$RUNDIR/photos/${n}-first.txt"
  chmod 666 "$RUNDIR/photos/${n}-first.txt"
  sleep 4
  echo "${label}-second" > "$RUNDIR/photos/${n}-second.txt"
  chmod 666 "$RUNDIR/photos/${n}-second.txt"

  wait_for_file "$RUNDIR/received/${n}-first.txt"  "${label}-first" \
    "[$label] FIRST change after restart arrived"
  wait_for_file "$RUNDIR/received/${n}-second.txt" "${label}-second" \
    "[$label] SECOND change after restart arrived"
}

# --- phase 9a/9b: ONE peer restarts, the other stays up ---------------
# The asymmetric cases, and they are the ones an operator actually hits:
# nobody reboots both laptops at once. They are also where the two roles
# differ — the sender holds the watcher and the receiver holds the
# subscription, and each is derived runtime state over a persistent store
# (D26/AP62) rebuilt by a different code path.
restart_and_check b "9a RECEIVER restarts, sender stays up" r1
restart_and_check a "9b SENDER restarts, receiver stays up" r2

# --- phase 9: restart both --------------------------------------------
# The whole point. Everything above runs in a process that has been alive
# the entire time; a mount, a subscription and a sync binding are all
# derived runtime state over a persistent store (D26/AP62), and each one
# has silently failed to survive a restart at least once in this repo.
log "PHASE 9 — stop BOTH peers, restart both, same stores"
podman rm -f peer-a peer-b >/dev/null 2>&1
sleep 1
start_peer peer-a "$RUNDIR/a"
start_peer peer-b "$RUNDIR/b"
sleep 5
for c in peer-a peer-b; do
  if podman exec "$c" sh -c "grep -q 'Local peer' /data/out.log"; then
    ok "$c restarted"
  else
    bad "$c did not restart"; out "$c" | tail -20 >&2
  fi
done

say peer-a "mounts"
expect peer-a "photos" "the mount survived the restart"
say peer-b "syncs"
expect peer-b "photos" "the sync binding survived the restart"

# Re-dial: a connection is process memory and the reconciler re-establishes
# it, but the addresses are what we typed, so make it explicit here.
say peer-b "connect a peer-a:$PORT"
say peer-a "connect b peer-b:$PORT"
sleep 3

# --- phase 10: the first change after a restart -----------------------
# This is the known-open defect (FIRST-CHANGE-AFTER-RESTART-IS-LOST,
# 2026-09-03): restart a peer and the NEXT file changed never arrives,
# while every one after it does. It is measured here rather than
# described, and BOTH files are asserted so the run distinguishes "the
# first one is lost" from "everything is lost".
log "PHASE 10 — the first change after a restart, and the second"
echo "after-restart-1" > "$RUNDIR/photos/post1.txt"
chmod 666 "$RUNDIR/photos/post1.txt"
sleep 4
echo "after-restart-2" > "$RUNDIR/photos/post2.txt"
chmod 666 "$RUNDIR/photos/post2.txt"

wait_for_file "$RUNDIR/received/post1.txt" "after-restart-1" \
  "FIRST change after restart arrived"
wait_for_file "$RUNDIR/received/post2.txt" "after-restart-2" \
  "SECOND change after restart arrived"

# --- phase 11: resync recovers ----------------------------------------
# resync exists for testability and this is the case it was built for: it
# is the only positive confirmation the flow offers, because "no errors"
# and "nothing happened" render identically.
log "PHASE 11 — resync recovers whatever the restart lost"
say peer-b "resync a photos"
sleep 3
wait_for_file "$RUNDIR/received/post1.txt" "after-restart-1" \
  "resync recovered the lost change"

report
exit $?
