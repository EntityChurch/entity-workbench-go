#!/usr/bin/env bash
# entity-state.sh — what state this product has left on your machine, and
# how to get back to virgin.
#
# WHY THIS EXISTS. An operator debugging a share could not answer "am I
# looking at a bug or at cruft from the last twenty runs?" — because
# nothing in the product could tell them where its state lived, and
# nothing could remove it. Measured on the developer machine 2026-09-05:
# **30 GB**, 1841 peer directories, 1590 identity files, 2451 log files
# totalling 27 GB, and 823 MB of crash dumps. None of it is pruned by
# anything, ever, and `forget --all` deliberately touches none of it.
#
# THE INVENTORY IS THE DEFAULT. This script does nothing destructive
# unless you type `reset`, and `reset` names every path before it removes
# it. A tool that wipes state on the strength of a flag is how somebody
# loses a keypair they needed.
#
#   bash scripts/entity-state.sh              # inventory (default, read-only)
#   bash scripts/entity-state.sh reset        # wipe, with confirmation
#   bash scripts/entity-state.sh reset --yes  # wipe, no prompt (CI/harness)
#   bash scripts/entity-state.sh reset --keep-identities
#
# WHAT RESET DOES NOT TOUCH, on purpose:
#   - Any directory you mounted. Those are YOUR files; a received folder's
#     bytes are yours too (the same rule `forget` follows). This script
#     removes the peer's record of a mount, never the mount's contents.
#   - Anything outside ~/.entity and this repo's own containers.
set -uo pipefail

ENTITY_HOME="${ENTITY_HOME:-$HOME/.entity}"
ACTION="${1:-inventory}"
shift || true
ASSUME_YES=0
KEEP_IDENTITIES=0
for a in "$@"; do
  case "$a" in
    --yes|-y) ASSUME_YES=1 ;;
    --keep-identities) KEEP_IDENTITIES=1 ;;
  esac
done

log()  { printf '\033[1m::\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m !\033[0m %s\n' "$*" >&2; }

# size PATH — human size, or "-" when absent. Never fails the script.
size() { [ -e "$1" ] && du -sh "$1" 2>/dev/null | cut -f1 || echo "-"; }
count() { [ -d "$1" ] && ls -A "$1" 2>/dev/null | wc -l | tr -d ' ' || echo 0; }

# The complete set. Anything this product writes outside these paths is a
# bug in the product, not an omission here — which is the point of naming
# them in one place: the list is checkable.
#
#   peers/         one sqlite store per peer identity. THE TREE lives here:
#                  declarations, mounts, offers, capability policy, every
#                  ingested document. Deleting a peer dir un-declares
#                  everything that peer knew.
#   identities/    Ed25519 keypairs. THIS IS THE IRREVERSIBLE ONE — a peer
#                  is its key, so removing it does not reset that peer, it
#                  ends it. Every grant anyone else wrote naming it is dead.
#   logs/          run logs. Unbounded, nothing rotates them.
#   crash/         minidumps + breadcrumb trails. Unbounded.
#   gui-layout.json    which panels are open, per peer ALIAS.
#   browser.json       registry origin + peer-id pin for the Browser panel.
#   peer-manager.json  the GUI's multi-peer roster.
#   shell/history      REPL history.
PATHS_STATE=(peers logs crash gui-layout.json browser.json peer-manager.json shell)
PATHS_KEYS=(identities keys)

inventory() {
  log "entity state under $ENTITY_HOME"
  if [ ! -d "$ENTITY_HOME" ]; then
    log "nothing there — this machine is already virgin"
    return 0
  fi
  printf '\n  %-22s %8s %10s  %s\n' "PATH" "ENTRIES" "SIZE" "WHAT IT IS" >&2
  printf '  %-22s %8s %10s  %s\n' "----" "-------" "----" "----------" >&2
  describe peers        "the tree: declarations, mounts, offers, policy, documents"
  describe identities   "KEYPAIRS — removing one ends that peer, it does not reset it"
  describe logs         "run logs; nothing rotates these"
  describe crash        "minidumps + breadcrumb trails; nothing prunes these"
  describe gui-layout.json  "which panels are open (per peer ALIAS)"
  describe browser.json     "Browser panel's registry pin"
  describe peer-manager.json "the GUI's peer roster"
  describe shell            "REPL history"
  printf '\n' >&2
  log "TOTAL $(size "$ENTITY_HOME")"

  # Containers this repo creates. Named explicitly: a wildcard over
  # `podman ps` would sweep up a neighbour's work, and this script runs on
  # a machine that has plenty of it.
  local ours
  ours=$(podman ps -a --format '{{.Names}}' 2>/dev/null | grep -E '^(peer-a|peer-b|peer-c|peer-d)$' || true)
  if [ -n "$ours" ]; then
    printf '\n' >&2
    warn "leftover harness containers still running or stopped:"
    printf '%s\n' "$ours" | sed 's/^/     /' >&2
  fi
  local nets
  nets=$(podman network ls --format '{{.Name}}' 2>/dev/null | grep -E '^entity-(twopeer|go-fed)$' || true)
  [ -n "$nets" ] && { warn "leftover harness networks:"; printf '%s\n' "$nets" | sed 's/^/     /' >&2; }
}

describe() {
  local p="$ENTITY_HOME/$1" what="$2" n s
  n=$(count "$p"); s=$(size "$p")
  [ -f "$p" ] && n="(file)"
  printf '  %-22s %8s %10s  %s\n' "$1" "$n" "$s" "$what" >&2
}

do_reset() {
  inventory
  printf '\n' >&2
  if [ "$KEEP_IDENTITIES" = "1" ]; then
    log "RESET will remove: ${PATHS_STATE[*]}"
    log "RESET will KEEP:   ${PATHS_KEYS[*]} (--keep-identities)"
    log "Keeping identities means this machine is the SAME peer afterwards,"
    log "so grants other machines wrote for it stay valid."
  else
    log "RESET will remove: ${PATHS_STATE[*]} ${PATHS_KEYS[*]}"
    warn "THIS INCLUDES KEYPAIRS. Every peer on this machine becomes a NEW peer."
    warn "Every grant another machine wrote naming these peers is dead, and"
    warn "those machines must forget and re-share. Use --keep-identities to"
    warn "reset the tree and stay the same peer."
  fi
  warn "Directories you mounted are NOT touched. Your files are your files."

  if [ "$ASSUME_YES" != "1" ]; then
    printf '\n  type RESET to proceed: ' >&2
    local answer; read -r answer
    [ "$answer" = "RESET" ] || { log "aborted — nothing removed"; return 1; }
  fi

  local targets=("${PATHS_STATE[@]}")
  [ "$KEEP_IDENTITIES" = "1" ] || targets+=("${PATHS_KEYS[@]}")
  for t in "${targets[@]}"; do
    if [ -e "$ENTITY_HOME/$t" ]; then
      rm -rf "${ENTITY_HOME:?}/$t"
      log "removed $t"
    fi
  done

  for c in peer-a peer-b peer-c peer-d; do
    podman rm -f "$c" >/dev/null 2>&1 && log "removed container $c"
  done
  for n in entity-twopeer entity-go-fed; do
    podman network rm "$n" >/dev/null 2>&1 && log "removed network $n"
  done

  printf '\n' >&2
  log "reset complete — state now:"
  inventory
}

case "$ACTION" in
  inventory|"") inventory ;;
  reset)        do_reset ;;
  *) printf 'usage: %s [inventory|reset] [--yes] [--keep-identities]\n' "$0" >&2; exit 2 ;;
esac
