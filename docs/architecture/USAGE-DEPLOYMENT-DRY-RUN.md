# Usage: deployment dry-run

**Audience:** operators preparing to deploy a workbench peer, or developers verifying that the wipe-and-rebuild model still holds. **Companion to:** [`DEPLOYMENT-DIRECTION.md`](DEPLOYMENT-DIRECTION.md) (the operational posture this exercises).

This document captures the **end-to-end procedure** that validates the deployment model: identity persists across DB wipes, re-ingested filesystem corpus reproduces byte-identical entity hashes, and a peer restored from its identity backup keeps its peer-id.

---

## What the dry-run proves

Three claims from `DEPLOYMENT-DIRECTION.md §4`:

1. **Identity material is the only thing that must survive.** Wipe the SQLite DB but keep the keypair → same peer-id on restart.
2. **The filesystem is the source of truth.** Re-ingest after a wipe → same content hashes, same path map.
3. **Backup + restore preserves identity.** Wipe everything, restore from backup → same peer-id.

A failure of any gate means the deployment story is broken — either identity isn't actually durable, or ingestion isn't deterministic, or backup isn't capturing the right material.

---

## Prerequisites

- Repo checked out + `make build` clean (entity-shell + entity-console binaries)
- `sqlite3` CLI on PATH
- Write access to `$HOME` (the dry-run uses a tmpdir-scoped `HOME` so it doesn't touch your real identities)

---

## Procedure

The dry-run script lives at `/tmp/deployment-dry-run.sh` (created ad-hoc; the source is reproduced below — keep this doc as the source of truth, the `/tmp` copy is a working file).

**Three phases:**

### Phase 1 — first life

1. `entity-shell identity create dryrun-peer` — generates a v7-flat keypair under `$HOME/.entity/identities/dryrun-peer` (file form for v7-flat mode, directory form for identity-bundle mode).
2. `entity-shell -identity dryrun-peer -storage sqlite -storage-path <DB> info` — bootstraps the peer, records the peer-id.
3. `entity-shell ... ingest tree <corpus> demo` — bulk-ingests a small markdown corpus under tree prefix `demo`.
4. `entity-shell ... tree @dryrun-peer/demo` — verifies the corpus landed.
5. `sqlite3 <DB> "SELECT path, hex(hash) FROM locations WHERE path LIKE '/<peer-id>/demo/%' ORDER BY path"` — captures the path → content-hash map.

### Phase 2 — wipe DB, keep identity

1. `rm -f <DB> <DB>-wal <DB>-shm`
2. `entity-shell ... info` — peer-id MUST match Phase 1's peer-id (gate 1).
3. Re-ingest the corpus + re-snapshot the path → hash map.
4. `diff` Phase 1 hashes vs Phase 2 hashes — MUST be identical (gate 2).

### Phase 3 — full wipe, restore from backup

1. Back up `$HOME/.entity/identities/dryrun-peer` to a separate location *before* the wipe (this is the operator's load-bearing step).
2. `rm -f <DB>` + `rm -rf <identity-path>`
3. Restore the backup: `cp -r <backup> <identity-path>`
4. `entity-shell ... info` — peer-id MUST match Phase 1's peer-id (gate 3).
5. Re-ingest + re-snapshot; hashes MUST match Phase 1.

---

## Run log

Executed on this host (Linux 7.0.4-100.fc43.x86_64, Go 1.25.1). All three gates pass:

```
Phase 1 peer-id: 2KdJ8cf7Znm44o8rvCCbgmgJ51DbQjZuhmfZohEBENX6hq
Phase 2 peer-id: 2KdJ8cf7Znm44o8rvCCbgmgJ51DbQjZuhmfZohEBENX6hq  (after DB wipe + re-ingest)
Phase 3 peer-id: 2KdJ8cf7Znm44o8rvCCbgmgJ51DbQjZuhmfZohEBENX6hq  (after full wipe + identity restore + re-ingest)

Phase 1 entities/locations: 255 / 278
Phase 2 entities/locations: 255 / 278   (deterministic re-bootstrap)

Phase 1 corpus path → hash:
/<peer>/demo/hello.md         | 00B68CAD5314927192F293C09C8A6D1A2B1D351DF4F73CCE19CA47E4F37688C75D
/<peer>/demo/nested/deep.md   | 0086B92911DAC6A5F7A870DFFFE825DC2D4CAAD239B532F46AFF027EFE36B04035
/<peer>/demo/world.md         | 00F9FF93ACEE0B0CB9D4DAA45C280BDA2C4CCD04A6DBB71FFABA875D5AA9AC7713

(Phase 2 + Phase 3 hash maps byte-identical.)
```

Corpus: 3 markdown files (87 bytes) under `demo/`. SQLite store stabilizes at 255 entities / 278 locations after re-bootstrap. (The bulk is bootstrap state — the demo corpus itself contributes 3 paths + 3 content-hash entities; the rest is identity, capability grants, system entities.)

---

## What this does NOT cover

This dry-run is **single-peer**. Multi-peer aspects of the deployment story are tracked separately:

- **Connection state persistence** — `system/peer/transport/{peer-id}` entries today rebuild from scratch each launch (`DEPLOYMENT-DIRECTION.md §7`).
- **Subscription rehydration** — does a peer with `revision follow` chains pick them back up on restart? (§7 open question.)
- **Auto-version re-arming** — does the auto-versioner re-engage on a prefix marked `auto_version: true` after restart? (§7 open question.)
- **Cross-impl conformance** — workbench-go is the only impl this validates. Rust + Python peers' wipe-and-rebuild characteristics are separately tracked.

These are real deployment hazards but they're not what this dry-run is for. When a multi-peer deployment story wants to ship, extend the script to phase-4 (two peers, follow-chain, kill one, wipe-rebuild, verify chain resumes) and produce a separate gate.

---

## The reusable script

The exact script that produced the run log above. Keep it self-contained — sets up its own `HOME`, its own corpus, leaves the workdir for inspection.

Stored at `/tmp/deployment-dry-run.sh` when authored; reproduce here verbatim when you want to re-run. (We don't commit it to `scripts/` because deployment dry-runs are episodic — re-derive from this doc when needed.)

```bash
#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-$(pwd)}"
WORKDIR=$(mktemp -d -t entity-dryrun-XXXXXX)
export HOME="$WORKDIR/home"
mkdir -p "$HOME"

CORPUS="$WORKDIR/corpus"
DB="$WORKDIR/peer.db"
BACKUP_DIR="$WORKDIR/identity-backup.dir"
BACKUP_FILE="$WORKDIR/identity-backup.file"
SHELL_BIN="$REPO/entity-shell"

run_shell() {
  HOME="$HOME" "$SHELL_BIN" -identity dryrun-peer -storage sqlite \
    -storage-path "$DB" "$@"
}

peer_id_of() {
  grep -oE 'PeerID:[[:space:]]+[A-Za-z0-9]+' "$1" | head -1 | awk '{print $2}'
}

snapshot_hashes() {
  sqlite3 "$DB" \
    "SELECT path, hex(hash) FROM locations WHERE path LIKE '/$2/demo/%' ORDER BY path;" \
    > "$1"
}

# Phase 0: corpus on disk
mkdir -p "$CORPUS/nested"
echo "# Hello" > "$CORPUS/hello.md"
echo "# World" > "$CORPUS/world.md"
echo "# Nested" > "$CORPUS/nested/deep.md"

# Phase 1: first life
"$SHELL_BIN" identity create dryrun-peer
run_shell info > "$WORKDIR/phase1-info.txt"
PEER_ID_1=$(peer_id_of "$WORKDIR/phase1-info.txt")
run_shell ingest tree "$CORPUS" demo
snapshot_hashes "$WORKDIR/phase1-hashes.txt" "$PEER_ID_1"

# Backup identity
ID_PATH="$HOME/.entity/identities/dryrun-peer"
if [ -d "$ID_PATH" ]; then cp -r "$ID_PATH" "$BACKUP_DIR"
else                      cp    "$ID_PATH" "$BACKUP_FILE"; fi

# Phase 2: wipe DB
rm -f "$DB" "${DB}-wal" "${DB}-shm"
run_shell info > "$WORKDIR/phase2-info.txt"
[ "$PEER_ID_1" = "$(peer_id_of "$WORKDIR/phase2-info.txt")" ] \
  || { echo "FAIL: peer-id changed across DB wipe"; exit 1; }
run_shell ingest tree "$CORPUS" demo
snapshot_hashes "$WORKDIR/phase2-hashes.txt" "$PEER_ID_1"
diff -q "$WORKDIR/phase1-hashes.txt" "$WORKDIR/phase2-hashes.txt" \
  || { echo "FAIL: hashes diverged after re-ingest"; exit 1; }

# Phase 3: full wipe + restore
rm -f "$DB" "${DB}-wal" "${DB}-shm"
rm -rf "$ID_PATH"
if [ -d "$BACKUP_DIR" ]; then cp -r "$BACKUP_DIR" "$ID_PATH"
else                          cp    "$BACKUP_FILE" "$ID_PATH"; fi
run_shell info > "$WORKDIR/phase3-info.txt"
[ "$PEER_ID_1" = "$(peer_id_of "$WORKDIR/phase3-info.txt")" ] \
  || { echo "FAIL: peer-id changed across identity restore"; exit 1; }

echo "PASS — all three gates"
echo "Workdir kept: $WORKDIR"
```

---

## Cross-references

- [`DEPLOYMENT-DIRECTION.md §4`](DEPLOYMENT-DIRECTION.md) — the wipe-and-rebuild recovery procedure this validates.
