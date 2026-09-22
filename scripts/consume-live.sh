#!/usr/bin/env bash
# consume-live.sh — drive our consumer at the LIVE public federation.
#
# What a green run claims, precisely:
#
#   - `{origin}/transport-profile` decodes as a `system/peer/transport/
#     http-poll` entity and its endpoint block validates.
#   - The registry peer's `system/peer/published-root` verifies: the
#     signature is ed25519 over the root, against the key the PINNED
#     peer-id carries. The origin is trusted for nothing.
#   - Its `system/registry/binding/by-name/` prefix is enumerated by
#     WALKING that signed root — not by reading the served `.list`. The
#     walk and the menu are reconciled and any disagreement is printed.
#   - Every name resolves per EXTENSION-REGISTRY §6a.4: signature, the
#     `binding.name == asked` association check, a finite unexpired ttl,
#     and a revocation probe inside the signed key set.
#   - One name is followed all the way through hop 2 — the binding's
#     transport becomes a layout, the TARGET peer's own signed root is
#     verified under a DIFFERENT key, and a page's bytes are checked
#     against what that root committed for its path.
#
# What it does NOT claim:
#
#   - Nothing here is fresh. Every artifact is "verified as of that
#     publisher's published_at". A quiet publisher and a withholding
#     origin are indistinguishable from a consumer (§6.5.3.1), and the
#     only bound on a withheld revocation is the binding's ttl.
#   - It is not a conformance run. It exercises OUR reader against THEIR
#     bytes; a divergence here is a finding to route, not a verdict.
#   - It reaches the public internet, which is why it is deliberately
#     OUTSIDE `test-native`. A sweep target that can go red for a
#     domain's reasons teaches people to ignore the sweep.
#
# The registry pin is the one fact supplied out of band, exactly as a
# deployment's shipped resolver-config would supply it. It is NOT read
# from the origin — that is the whole point of a pin.
set -euo pipefail

REGISTRY_ORIGIN="${REGISTRY_ORIGIN:-https://entitychurchregistry.org}"
REGISTRY_PEER="${REGISTRY_PEER:-2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb}"

# The follow-through leg. A name plus a path that name's target commits.
FOLLOW_NAME="${FOLLOW_NAME:-entitycoreprotocol.org}"
FOLLOW_PATH="${FOLLOW_PATH:-sites/entity-core-protocol-main/pages/index}"

BIN="${BIN:-./bin/entity-fetch}"

if [ ! -x "$BIN" ]; then
	echo "consume-live: $BIN not built — run 'make fetch-build' first" >&2
	exit 2
fi

echo "== consume-live =="
echo "   registry origin : $REGISTRY_ORIGIN"
echo "   registry pin    : $REGISTRY_PEER  (out of band; not read from the origin)"
echo

echo "-- leg 1: enumerate, by WALKING the signed root --"
"$BIN" -registry "$REGISTRY_ORIGIN" -registry-peer "$REGISTRY_PEER" -names
echo

echo "-- leg 2: resolve every enumerated name (§6a.4 in full) --"
# Read the WALK's rows, not the served menu — the `ok` lines are the
# committed key set. (The -json form pretty-prints across lines, so a
# one-line extraction of it silently yields nothing.)
names="$("$BIN" -registry "$REGISTRY_ORIGIN" -registry-peer "$REGISTRY_PEER" -names |
	awk '/^[[:space:]]+ok[[:space:]]/ {print $2}')"

if [ -z "$names" ]; then
	echo "consume-live: the walk committed no names — that is a finding, not a pass" >&2
	exit 1
fi

# Collect, then report. A loop that exits at the first failure yields a
# count that is a lower bound (AP15).
failed=0
total=0
for n in $names; do
	total=$((total + 1))
	if "$BIN" -registry "$REGISTRY_ORIGIN" -registry-peer "$REGISTRY_PEER" -name "$n" >/tmp/consume-live-$n.log 2>&1; then
		printf '  ok    %s\n' "$n"
	else
		printf '  FAIL  %s  (see /tmp/consume-live-%s.log)\n' "$n" "$n"
		failed=$((failed + 1))
	fi
done
echo

echo "-- leg 3: follow one name through hop 2 to verified page bytes --"
if ! "$BIN" -registry "$REGISTRY_ORIGIN" -registry-peer "$REGISTRY_PEER" \
	-name "$FOLLOW_NAME" -path "$FOLLOW_PATH"; then
	echo "consume-live: the follow-through leg failed" >&2
	failed=$((failed + 1))
fi
echo

if [ "$failed" -ne 0 ]; then
	echo "consume-live: $failed of $total name(s) plus the follow leg did not hold" >&2
	exit 1
fi
echo "consume-live: $total name(s) resolved and one followed to verified bytes."
echo "Every claim above is 'as of published_at' — never simply verified."
