#!/usr/bin/env bash
# threepeer-sync.sh — the topologies two peers cannot express.
#
# WHY THIS EXISTS. Every sharing gate in this repo runs exactly two
# peers. Two peers can only ever be one edge, so the whole class of
# questions that begins "and then the third machine…" has never been
# asked: whether one publisher can serve two receivers without the
# second share clobbering the first's grant, whether a file that
# ARRIVED at a peer propagates onward from it, and what a bidirectional
# pair does when a change can come back.
#
# Those are not variations on the two-peer case. Each one exercises a
# mechanism the two-peer harness structurally cannot reach:
#
#   FAN-OUT  A -> B and A -> C, one folder, two receivers.
#            Reaches: the per-peer policy row under a SECOND writer.
#            `system/capability/policy/{peer}` is a union with exactly
#            one writer (AP68); sharing the same folder with a second
#            peer is the first time two declarations touch two rows
#            from one reconcile pass.
#
#   CHAIN    A -> B -> C, where B forwards what it received.
#            Reaches: whether an INGESTED file is observable to the
#            receiving peer's own watcher. A file that arrives lands on
#            B's disk inside a mounted directory, so B's watcher should
#            see it exactly as it would a local write — but nothing has
#            ever checked, and "B's folder is mode=receive so it does
#            not republish" is a statement about the RECEIVED folder,
#            not about a folder B mounts and shares itself.
#
#   LOOP     A <-> B, mode=both, over a real network.
#            MEASURED, NOT ASSERTED — see PHASE 5. `Mode: both` is
#            shipped and has never been exercised two-way across a
#            network, so this harness has no business claiming to know
#            what it should do. It records what happens.
#
# WHAT A GREEN RUN CLAIMS: three peers in three network namespaces with
# distinct routable addresses and persistent sqlite stores, a fan-out to
# two receivers with both receiving, and a two-hop chain delivering a
# file the middle peer never authored — every assertion on BYTES ON DISK
# at the far end.
#   IT DOES NOT CLAIM: the GUI (this drives entity-shell), physical
#   machines, NAT, mDNS (container DNS stands in), more than three
#   peers, or any statement about `Mode: both`, which is measured and
#   reported rather than asserted.
#
#   bash scripts/threepeer-sync.sh
#   KEEP_UP=1 bash scripts/threepeer-sync.sh
#   PHASE=chain bash scripts/threepeer-sync.sh   # stop after a phase
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
NET="${SYNC_NET:-entity-threepeer}"
IMG="${SYNC_IMG:-docker.io/library/alpine:3.20}"
BIN="${SHELL_BIN:-/tmp/entity-shell-threepeer}"
RUNDIR="${RUNDIR:-/tmp/entity-threepeer-$$}"
PORT=9000
PEERS="peer-a peer-b peer-c"

# Collect and report (AP15). A run that aborts at the first red reports
# one defect when the tree may be wrong in four ways, and the count it
# yields is a lower bound.
FAILURES=()
CHECKS=0
NOTES=()

log()  { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
ok()   { CHECKS=$((CHECKS+1)); printf '\033[1;32m  ok\033[0m  %s\n' "$*" >&2; }
bad()  { CHECKS=$((CHECKS+1)); FAILURES+=("$*"); printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; }
# note() is for a MEASUREMENT, not an assertion. It never fails the run.
# Kept visually distinct so a reader cannot mistake one for the other —
# a measurement dressed as a check is how an undesigned behaviour gets
# recorded as a passing requirement.
note() { NOTES+=("$*"); printf '\033[1;36m  ..\033[0m  %s\n' "$*" >&2; }
die()  { printf '\033[1;31mFATAL:\033[0m %s\n' "$*" >&2; exit 1; }

report() {
  echo >&2
  log "================ RESULT ================"
  # An empty result set satisfies "nothing failed". It is the oldest way
  # a harness lies, and this repo shipped one on 2026-09-06 that printed
  # its green banner after a fatal abort.
  if [ "$CHECKS" -eq 0 ]; then
    log "NO CHECKS RAN — this is a failure, not a pass."
    return 1
  fi
  log "$CHECKS checks, ${#FAILURES[@]} failed, ${#NOTES[@]} measured (not asserted)"
  if [ ${#NOTES[@]} -gt 0 ]; then
    for n in "${NOTES[@]}"; do printf '\033[1;36m  ..\033[0m %s\n' "$n" >&2; done
  fi
  if [ ${#FAILURES[@]} -gt 0 ]; then
    for f in "${FAILURES[@]}"; do printf '\033[1;31m  x\033[0m %s\n' "$f" >&2; done
    echo >&2
    for c in $PEERS; do log "$c log tail:"; out "$c" 2>/dev/null | tail -20 >&2; done
    return 1
  fi
  log "THREE-PEER GREEN: fan-out to two receivers, and a two-hop chain."
  return 0
}

command -v podman >/dev/null || die "podman is required"

cleanup() {
  if [ "${KEEP_UP:-0}" = "1" ]; then
    log "KEEP_UP=1 — leaving all three peers up. Tear down with:"
    log "  podman rm -f $PEERS; podman network rm $NET"
    return
  fi
  log "tearing down"
  # shellcheck disable=SC2086
  podman rm -f $PEERS >/dev/null 2>&1 || true
  podman network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- build ------------------------------------------------------------
# CGO off so the binary runs in a bare image; modernc.org/sqlite is pure
# Go, so the persistent backend survives it.
log "building entity-shell static (CGO off, linux/amd64)"
( cd "$REPO/shell" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BIN" ./cmd/entity-shell ) \
  || die "build failed"

# Directory plan. Each topology gets its OWN source folder and its own
# receiving directories, so a failure in one cannot be explained by
# state the other left behind — which matters more here than with two
# peers, because a shared folder name across topologies would make the
# fan-out and the chain indistinguishable in the logs.
#
#   fan-out:  a/photos  ->  b/fan-photos   and  c/fan-photos
#   chain:    a/docs    ->  b/chain-docs   ->   c/chain-docs
mkdir -p "$RUNDIR"/{a,b,c} "$RUNDIR"/photos "$RUNDIR"/docs
mkdir -p "$RUNDIR"/b-fan "$RUNDIR"/c-fan "$RUNDIR"/b-chain "$RUNDIR"/c-chain
# Per-peer directory that is /sym INSIDE every container and a different
# host directory for each. It exists so PHASE 6 can run the Mode:both
# case with SYMMETRIC root names — the discriminator for the two-root-
# names trap, which is invisible whenever both sides happen to agree.
mkdir -p "$RUNDIR"/sym-peer-a "$RUNDIR"/sym-peer-b "$RUNDIR"/sym-peer-c
chmod -R 777 "$RUNDIR"

# --- network ----------------------------------------------------------
podman network rm "$NET" >/dev/null 2>&1 || true
podman network create "$NET" >/dev/null || die "could not create network $NET"
log "network $NET up"

start_peer() {
  local name="$1" dir="$2"
  podman run -d --name "$name" --network "$NET" --hostname "$name" \
    -v "$BIN:/entity-shell:z,ro" \
    -v "$dir:/data:z" \
    -v "$RUNDIR/photos:/photos:z" \
    -v "$RUNDIR/docs:/docs:z" \
    -v "$RUNDIR/b-fan:/b-fan:z" \
    -v "$RUNDIR/c-fan:/c-fan:z" \
    -v "$RUNDIR/b-chain:/b-chain:z" \
    -v "$RUNDIR/c-chain:/c-chain:z" \
    -v "$RUNDIR/sym-$name:/sym:z" \
    -e HOME=/data \
    "$IMG" sh -c "
      mkfifo /tmp/in 2>/dev/null;
      sleep infinity > /tmp/in &
      exec /entity-shell -alias $name -storage sqlite \
        -storage-path /data/store.db -listen 0.0.0.0:$PORT < /tmp/in > /data/out.log 2>&1
    " >/dev/null || die "could not start $name"
}

# say CONTAINER CMD... — send one command, wait for the REPL to finish it
# by counting prompts. Polling rather than sleeping is what keeps this
# from being flaky on a loaded machine.
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

out() { podman exec "$1" sh -c "cat /data/out.log 2>/dev/null"; }

# folder_id PEER ROOT — the id `direction` takes, which is
# `{owner-peer-id}.{root}` and is deliberately not a name an operator
# would guess. Read it out of the verb's own listing rather than
# reconstructing it here: rebuilding an id in the harness is how a test
# starts passing against a build whose id format has moved.
#
# The log accumulates, so this reads only the lines the `direction` call
# just appended.
folder_id() {
  local c="$1" root="$2" before
  before=$(podman exec "$c" sh -c "wc -l < /data/out.log 2>/dev/null || echo 0")
  say "$c" "direction" >/dev/null 2>&1
  out "$c" | tail -n "+$((before + 1))" | grep -oE '[A-Za-z0-9]+\.'"$root"'$' | head -1
}

expect() {
  local c="$1" pat="$2" label="$3"
  if out "$c" | grep -qE "$pat"; then ok "$label"; else
    bad "$label — [$c] log has no /$pat/"
  fi
}

# The assertion that actually matters: bytes on disk at the far end. A
# tree-count would pass while the operator's folder was empty.
wait_for_file() {
  local path="$1" want="$2" label="$3" i content
  for i in $(seq 1 90); do
    if [ -f "$path" ]; then
      content=$(cat "$path" 2>/dev/null)
      [ "$content" = "$want" ] && { ok "$label"; return 0; }
    fi
    sleep 0.5
  done
  if [ -f "$path" ]; then
    bad "$label — file exists but content is '$(cat "$path" 2>/dev/null)', wanted '$want'"
  else
    bad "$label — $path never appeared (45s)"
  fi
  return 1
}

# measure_file — the same wait, reported as a MEASUREMENT. For the
# behaviours this harness deliberately does not claim to know.
measure_file() {
  local path="$1" label="$2" i
  for i in $(seq 1 60); do
    [ -f "$path" ] && { note "$label — arrived: '$(cat "$path" 2>/dev/null)'"; return 0; }
    sleep 0.5
  done
  note "$label — did NOT arrive within 30s"
  return 0
}

# connect_pair X Y — both directions. A dial-by-address authorizes the
# DIALER only (AP63 / §4.4: sendReciprocalGrant is gated on
# EstablishedViaRendezvousKey), so a one-way dial gives an accepted
# subscription and an empty folder. With three peers this is three
# pairs, not one, and forgetting a pair is the easiest way to produce a
# "chain is broken" result that is really a missing edge.
connect_pair() {
  local x="$1" y="$2"
  say "$x" "connect ${y#peer-} $y:$PORT"
  say "$y" "connect ${x#peer-} $x:$PORT"
}

# redial_pair X Y — grants are assembled at HANDSHAKE, so a policy
# written on a live connection is inert until the connection is
# re-established, and the peer that DISPATCHES is the peer that must
# reconnect. Both sides, every time a policy changed.
redial_pair() {
  local x="$1" y="$2"
  say "$x" "disconnect ${y#peer-}"
  say "$y" "disconnect ${x#peer-}"
  sleep 1
  say "$x" "connect ${y#peer-} $y:$PORT"
  say "$y" "connect ${x#peer-} $x:$PORT"
  sleep 2
}

# ================= PHASE 1 — three peers up ==========================
log "PHASE 1 — three peers, three containers, three persistent stores"
start_peer peer-a "$RUNDIR/a"
start_peer peer-b "$RUNDIR/b"
start_peer peer-c "$RUNDIR/c"
sleep 4
for c in $PEERS; do
  if podman exec "$c" sh -c "grep -q 'Local peer' /data/out.log"; then
    ok "$c came up with a peer"
  else
    bad "$c did not start — log follows"; out "$c" | tail -20 >&2
  fi
done

A_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' peer-a)
B_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' peer-b)
C_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' peer-c)
log "peer-a=$A_IP peer-b=$B_IP peer-c=$C_IP"
if [ "$A_IP" != "$B_IP" ] && [ "$B_IP" != "$C_IP" ] && [ "$A_IP" != "$C_IP" ]; then
  ok "all three peers have distinct routable addresses"
else
  bad "two peers share an address — they are not isolated"
fi

# All three pairs. The triangle of CONNECTIONS is set up even for the
# fan-out, because B and C must be able to reach each other for the
# chain phase and re-dialling later is cheaper than diagnosing a
# missing edge.
log "PHASE 1b — connect all three pairs, both directions each"
connect_pair peer-a peer-b
connect_pair peer-a peer-c
connect_pair peer-b peer-c
sleep 2
expect peer-a "peer-b|connected|handshake|b " "peer-a has dialled peer-b"
expect peer-a "peer-c|connected|handshake|c " "peer-a has dialled peer-c"
expect peer-b "peer-c|connected|handshake|c " "peer-b has dialled peer-c"

# ================= PHASE 2 — FAN-OUT =================================
# One folder, one publisher, TWO receivers. The mechanism under test is
# the per-peer policy row: `share photos with c` must not disturb the
# row that `share photos with b` already produced, and the reconciler
# has exactly one writer for both.
log "PHASE 2 — FAN-OUT: peer-a shares one folder with BOTH b and c"
echo "fan-one"    > "$RUNDIR/photos/one.txt"
echo "fan-two"    > "$RUNDIR/photos/two.txt"
mkdir -p "$RUNDIR/photos/sub"
echo "fan-nested" > "$RUNDIR/photos/sub/three.txt"
chmod -R 777 "$RUNDIR/photos"

say peer-a "mount /photos archives/photos"
sleep 2
expect peer-a "photos" "peer-a reports the photos mount"

say peer-a "share photos with b"
sleep 2
say peer-a "share photos with c"
sleep 2

say peer-b "offers a"
say peer-c "offers a"
expect peer-b "photos" "peer-b sees the offer"
expect peer-c "photos" "peer-c sees the offer — the SECOND receiver was told too"

say peer-b "accept a photos /b-fan"
sleep 3
say peer-c "accept a photos /c-fan"
sleep 3

# Both policies were written on live connections. Re-dial both pairs.
log "PHASE 2b — re-dial a<->b and a<->c (grants assemble at handshake)"
redial_pair peer-a peer-b
redial_pair peer-a peer-c

log "PHASE 2c — the backfill, at BOTH receivers"
wait_for_file "$RUNDIR/b-fan/one.txt"        "fan-one"    "fan-out: b got one.txt"
wait_for_file "$RUNDIR/b-fan/two.txt"        "fan-two"    "fan-out: b got two.txt"
wait_for_file "$RUNDIR/b-fan/sub/three.txt"  "fan-nested" "fan-out: b got the nested file"
wait_for_file "$RUNDIR/c-fan/one.txt"        "fan-one"    "fan-out: c got one.txt"
wait_for_file "$RUNDIR/c-fan/two.txt"        "fan-two"    "fan-out: c got two.txt"
wait_for_file "$RUNDIR/c-fan/sub/three.txt"  "fan-nested" "fan-out: c got the nested file"

log "PHASE 2d — a LIVE change must reach both receivers, not just the first"
echo "fan-live" > "$RUNDIR/photos/live.txt"
chmod 666 "$RUNDIR/photos/live.txt"
wait_for_file "$RUNDIR/b-fan/live.txt" "fan-live" "fan-out: live change reached b"
wait_for_file "$RUNDIR/c-fan/live.txt" "fan-live" "fan-out: live change reached c"

# The grant-clobber check, stated directly. If the second share had
# overwritten the first peer's policy row rather than unioning with it,
# the FIRST receiver is the one that goes quiet — so the assertion that
# matters is a change reaching B *after* C was added.
echo "fan-after-c" > "$RUNDIR/photos/after-c.txt"
chmod 666 "$RUNDIR/photos/after-c.txt"
wait_for_file "$RUNDIR/b-fan/after-c.txt" "fan-after-c" \
  "fan-out: b still receives after c was added (the policy row is a union, not a clobber)"

[ "${PHASE:-all}" = "fanout" ] && { report; exit $?; }

# ================= PHASE 3 — CHAIN ===================================
# A -> B -> C. The question is whether a file that ARRIVED at B is
# observable to B's own watcher and can therefore be published onward.
# `accept` creates the mount, so B is already publishing-capable for
# that directory; what has never been tested is whether ingest-written
# bytes look like a local write to the layer above.
log "PHASE 3 — CHAIN: peer-a -> peer-b -> peer-c"
echo "chain-origin" > "$RUNDIR/docs/origin.txt"
chmod -R 777 "$RUNDIR/docs"

say peer-a "mount /docs archives/docs"
sleep 2
say peer-a "share docs with b"
sleep 2
say peer-b "accept a docs /b-chain"
sleep 3
redial_pair peer-a peer-b

wait_for_file "$RUNDIR/b-chain/origin.txt" "chain-origin" "chain hop 1: a -> b delivered"

# Now B forwards. This is a NEW folder object originated by B over the
# directory `accept` mounted — not a re-publication of A's folder, which
# is mode=receive and deliberately does not republish. The distinction
# is the whole design answer to "does a chain loop": B publishes its own
# mount, so the forward is an explicit operator act rather than an
# emergent property of receiving.
log "PHASE 3b — peer-b forwards what it received to peer-c"
say peer-b "share b-chain with c"
sleep 2
say peer-c "offers b"
expect peer-c "b-chain" "peer-c sees peer-b's onward offer"
say peer-c "accept b b-chain /c-chain"
sleep 3
redial_pair peer-b peer-c

wait_for_file "$RUNDIR/c-chain/origin.txt" "chain-origin" \
  "chain hop 2: a file peer-b never authored reached peer-c"

log "PHASE 3c — a change at the ORIGIN must traverse two hops"
echo "chain-live" > "$RUNDIR/docs/live.txt"
chmod 666 "$RUNDIR/docs/live.txt"
wait_for_file "$RUNDIR/b-chain/live.txt" "chain-live" "chain: live change reached b (hop 1)"
wait_for_file "$RUNDIR/c-chain/live.txt" "chain-live" \
  "chain: live change traversed BOTH hops to c"

[ "${PHASE:-all}" = "chain" ] && { report; exit $?; }

# ================= PHASE 4 — the chain does not run backwards ========
# B received A's folder as mode=receive, so a write made at B must NOT
# travel back to A. This is the safety half of the chain: if it fails,
# a receiver silently becomes a publisher and an operator's edit
# overwrites the origin.
log "PHASE 4 — a write at the MIDDLE must not travel back to the origin"
echo "written-at-b" > "$RUNDIR/b-chain/from-b.txt"
chmod 666 "$RUNDIR/b-chain/from-b.txt"
sleep 12
if [ -f "$RUNDIR/docs/from-b.txt" ]; then
  bad "a receive-only folder published upstream: from-b.txt appeared at the origin"
else
  ok "receive-only holds: a write at peer-b did not reach peer-a"
fi

# ================= PHASE 5 — Mode: both, MEASURED ====================
# `Mode: both` is shipped and has never been exercised two-way over a
# network. This harness therefore does NOT assert what it should do —
# it records what it does, so the next session designs against a
# measurement instead of an assumption. Turning these into assertions
# without a design answer would encode whatever the code happens to do
# today as the requirement.
log "PHASE 5 — Mode: both, over a real network (MEASURED, not asserted)"
BCHAIN_ID="$(folder_id peer-b b-chain)"
if [ -z "$BCHAIN_ID" ]; then
  note "could not resolve peer-b's folder id for b-chain — Mode:both not exercised"
else
  note "peer-b's b-chain folder id is $BCHAIN_ID"
  say peer-b "direction $BCHAIN_ID both"
  sleep 2
  say peer-b "status"
  redial_pair peer-b peer-c
fi

echo "both-from-b" > "$RUNDIR/b-chain/both-b.txt"
chmod 666 "$RUNDIR/b-chain/both-b.txt"
measure_file "$RUNDIR/c-chain/both-b.txt" "Mode:both (b only) — b's own write travelling to c"

echo "both-from-c" > "$RUNDIR/c-chain/both-c.txt"
chmod 666 "$RUNDIR/c-chain/both-c.txt"
measure_file "$RUNDIR/b-chain/both-c.txt" "Mode:both (b only) — c's write travelling back to b"

# THE OTHER HALF, and it is the part that makes the measurement mean
# something. Mode is a PER-PEER declaration about one peer's own copy:
# setting `both` on b says b will publish AND receive, and says nothing
# about c, whose copy is still the mode `accept` gave it. So a one-sided
# `both` is expected to move exactly one way, and the interesting
# question is whether a two-sided one moves both.
#
# Measuring the second arm is what separates "two-way sync is broken"
# from "two-way sync is a two-sided declaration" — two very different
# findings, and the first one is what a single-arm measurement would
# have been written up as.
log 'PHASE 5b — now declare both on the OTHER side too'
# NOTE THE ROOT NAME. peer-c accepted into the directory /c-chain, but a
# received folder's ID is keyed on the SENDER's root — FolderID(owner,
# root) with owner=peer-b and root=b-chain — because it names one object
# across both peers (S6/AP72). `c-chain` is only LocalRoot.
#
# So an operator who accepted into a directory of their own choosing and
# then runs `direction` sees an id built from a root they never typed.
# The first version of this script looked up `c-chain` and found
# nothing, which is the two-root-names trap landing on the operator
# surface rather than in the code.
CCHAIN_ID="$(folder_id peer-c b-chain)"
if [ -z "$CCHAIN_ID" ]; then
  note "could not resolve peer-c's folder id for the received b-chain — the second arm did not run"
else
  note "peer-c's received-folder id is $CCHAIN_ID (keyed on peer-b's root, not on c-chain)"
  say peer-c "direction $CCHAIN_ID both"
  sleep 2
  redial_pair peer-b peer-c
  echo "both-from-c2" > "$RUNDIR/c-chain/both-c2.txt"
  chmod 666 "$RUNDIR/c-chain/both-c2.txt"
  measure_file "$RUNDIR/b-chain/both-c2.txt" "Mode:both (BOTH sides) — c's write travelling back to b"
fi

# Echo check: if a change loops, the origin's copy keeps being rewritten.
# A stable byte count after a settle window is the cheap tell.
sleep 8
B_COUNT=$(find "$RUNDIR/b-chain" -type f 2>/dev/null | wc -l)
C_COUNT=$(find "$RUNDIR/c-chain" -type f 2>/dev/null | wc -l)
sleep 8
B_COUNT2=$(find "$RUNDIR/b-chain" -type f 2>/dev/null | wc -l)
C_COUNT2=$(find "$RUNDIR/c-chain" -type f 2>/dev/null | wc -l)
if [ "$B_COUNT" = "$B_COUNT2" ] && [ "$C_COUNT" = "$C_COUNT2" ]; then
  ok "the pair CONVERGED — file counts stable over 8s (b=$B_COUNT2 c=$C_COUNT2)"
else
  bad "file counts still moving after settle: b $B_COUNT->$B_COUNT2, c $C_COUNT->$C_COUNT2 (possible echo loop)"
fi

# ================= PHASE 6 — the discriminator =======================
# PHASE 5 measured `Mode: both` failing to carry a receiver's write back
# to the publisher even with both sides declaring it. That result has
# two very different explanations and the difference decides who owns
# the bug:
#
#   (a) the receive-side subscription is not established at all, or
#   (b) it IS established, on the wrong root name.
#
# (b) is the standing trap in this codebase: a received folder has TWO
# root names — the sender's (which keys the binding and the folder id)
# and the local directory the operator chose. AGENTS.md says to read it
# through ReceivingRoot() rather than Root, "because Root is right in
# the symmetric case and silently wrong in exactly the case the field
# exists for, which survives every test whose two peers happen to agree
# on a name". PHASE 5 deliberately used DIFFERENT names (b-chain vs
# c-chain), so if that is the mechanism, this phase — where both sides
# use the SAME root name — will behave differently.
#
# A probe that does not reproduce the failing shape refutes nothing
# (AP43), so the point of running both arms is that either outcome is
# informative: same result on both means the subscription is missing;
# different results name the root as the cause.
log "PHASE 6 — Mode: both with SYMMETRIC root names (the discriminator)"
echo "sym-origin" > "$RUNDIR/sym-peer-a/origin.txt"
chmod -R 777 "$RUNDIR/sym-peer-a"

say peer-a "mount /sym archives/sym"
sleep 2
say peer-a "share sym with c"
sleep 2
# peer-c accepts into ITS /sym — a different host directory, the same
# basename, so sender-root and local-root agree.
say peer-c "accept a sym /sym"
sleep 3
redial_pair peer-a peer-c
wait_for_file "$RUNDIR/sym-peer-c/origin.txt" "sym-origin" "symmetric: a -> c delivered"

SYM_A_ID="$(folder_id peer-a sym)"
SYM_C_ID="$(folder_id peer-c sym)"
if [ -z "$SYM_A_ID" ] || [ -z "$SYM_C_ID" ]; then
  note "could not resolve both symmetric folder ids (a=$SYM_A_ID c=$SYM_C_ID) — arm did not run"
else
  note "symmetric folder id on a=$SYM_A_ID  on c=$SYM_C_ID"
  say peer-a "direction $SYM_A_ID both"
  say peer-c "direction $SYM_C_ID both"
  sleep 2
  redial_pair peer-a peer-c
  echo "sym-from-c" > "$RUNDIR/sym-peer-c/from-c.txt"
  chmod 666 "$RUNDIR/sym-peer-c/from-c.txt"
  measure_file "$RUNDIR/sym-peer-a/from-c.txt" \
    "Mode:both SYMMETRIC roots — c's write travelling back to a"

  # Which leg is missing? `receiveFromPeers` (shellcmd/reconcile.go)
  # says a LOCAL folder that Receives() should pull from every peer
  # whose state is Accepted — i.e. peer-a should hold a sync binding to
  # peer-c once it is `both`. Record whether it does, because "the
  # declaration did not take" and "the declaration took and the pull
  # leg is unimplemented" are different bugs with different owners, and
  # a measurement that cannot tell them apart sends the next session to
  # the wrong file.
  SYNCS_BEFORE=$(podman exec peer-a sh -c "wc -l < /data/out.log")
  say peer-a "syncs"
  if out peer-a | tail -n "+$((SYNCS_BEFORE + 1))" | grep -q "$(echo "$SYM_C_ID" | cut -d. -f1)"; then
    note "peer-a DOES hold a sync binding naming peer-c — the pull leg exists and did not deliver"
  else
    note "peer-a holds NO sync binding naming peer-c — the owner-side receive leg was never established"
  fi
  DIR_BEFORE=$(podman exec peer-a sh -c "wc -l < /data/out.log")
  say peer-a "direction"
  note "peer-a's declared mode for sym: $(out peer-a | tail -n "+$((DIR_BEFORE + 1))" | grep -B1 "\.sym$" | head -1 | tr -s ' ' | sed 's/^ *//')"
fi

report
