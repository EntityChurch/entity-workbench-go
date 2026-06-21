# Deployment Direction

**Owner:** workbench team. **Status:** Direction doc — long-lived,
updated as we learn from real deployments.

The operational posture for production-ising workbench peers. What
"deployed" means, what must survive, how we handle breaking changes,
what to back up, what to version. Companion to
`SHELL-DIRECTION.md` (the CLI direction).

This doc is opinionated about a deliberately narrow scope: single
long-running peer per host, multi-host mesh, no Kubernetes yet, no
container orchestration yet. When we have evidence those constraints
matter, we'll revisit.

---

## 1. What "deployed" means today

Today: a person on a host runs

```
entity-shell -identity peerA \
             -storage sqlite \
             -storage-path ~/.entity/peers/peerA/peer.db
```

and the peer stays up for some duration (interactive session, screen
session, systemd unit, doesn't matter). Other peers run the same
shape; the mesh forms when `connect` + `revision follow` get wired.

That's the operational unit: one binary, one DB file, one on-disk
identity. No service discovery, no orchestrator, no cluster
membership protocol beyond the peer-to-peer transport.

This is enough for the use cases on the roadmap (personal multi-
machine workspace, small collaborative mesh). When we outgrow it,
we'll know.

---

## 2. What must survive

There is exactly one artifact that must survive across host wipes,
binary upgrades, and breaking schema changes:

**The on-disk identity material** — the keypair (V7-only mode) or the
identity bundle (identity-aware mode). Without it, the peer's peer-id
changes; without a stable peer-id, follow chains break, capability
grants don't recognize the peer, and to the rest of the mesh you're
just a new participant who happens to share a hostname.

Everything else is reconstructible:

- **The SQLite content store** is derived from the local filesystem
  corpus (once Phase E lands — see §6 for the interim caveat).
- **Connection state** (`system/peer/transport/{peer-id}` entries) is
  regenerable from a list of known peers or rediscoverable through
  whatever bootstrap mechanism the mesh uses.
- **Revision history** is regenerable by re-ingesting + re-committing.
  Convergence sorts itself out via content addressing — peers that
  re-publish the same content produce the same hashes.
- **The binary itself** is just a build artifact.

This is the answer to "what happens when code changes." We don't ship
migration frameworks; we ship a wipe-and-rebuild recovery procedure.
The cost of a wipe is reconstruction time + re-establishing follow
chains, not data loss.

---

## 3. Terminology — identity vs keypair

The word "identity" is overloaded across four concepts. Pin them
down now or trip over them forever.

| Layer | Term | Refers to |
|---|---|---|
| V7 protocol | **identity entity** | The hash-addressed public-key entity, produced by `crypto.Keypair.IdentityEntity()`. Every peer has exactly one. ≈ the peer's public key wrapped as an entity. |
| On-disk material (V7-only mode) | **keypair file** | A bare Ed25519 keypair on disk at `~/.entity/identities/{name}`. Misnomered as "identity" in `crypto.LoadIdentity/SaveIdentity`. We call this a **keypair**, not an identity. |
| On-disk material (identity-aware mode) | **identity bundle** | A directory at `~/.entity/identities/{name}/` containing the controller cert + member quorum + keypair. This IS legitimately an identity in the identity-extension sense. |
| Extension | **identity extension** | The attestation + quorum + identity stack (`ext/identity/`). Provides certs, custody, named identities. |

**Convention going forward:**

- When we mean the V7 keypair, write "keypair" (or "peer keypair" if
  context is ambiguous).
- When we mean the identity-extension construct, write "identity"
  (or "identity bundle" when referring to the on-disk shape).
- When we mean the identity-extension feature surface, write
  "identity extension" or "identity stack."
- The shell `-identity NAME` flag is fine as-is — it's the
  auto-detecting user-facing dispatcher between the two on-disk
  shapes. Its docs should mention both forms.
- `crypto.LoadIdentity/SaveIdentity` — these load/save **keypairs**.
  We don't rename them unilaterally (core-go API, used cross-team);
  we'll file feedback to core-go suggesting `LoadKeypairByName /
  SaveKeypairByName` and deprecating the existing names.

When in doubt, ask: "is this thing a keypair, or does it also carry
certs + custody?" That answers which term to use.

---

## 4. The wipe-and-rebuild model

Recovery procedure:

```
1. Back up the identity material (keypair or bundle).
2. Wipe the peer's SQLite DB and any derived state.
3. Reinstall the binary (potentially a newer version).
4. Re-bind the identity from backup.
5. Re-ingest the local filesystem corpus.
6. Re-connect to known peers.
7. Re-install follow chains.
8. Convergence does the rest.
```

Step 1 is the load-bearing step. If you don't have the identity
material backed up, you can't be the same peer after the wipe.

Steps 5–8 are operationally expensive but not data-losing. The mesh
converges because content addressing makes re-publication of the
same content a no-op for peers that already have it.

**Operational guidance for operators:**

- **Back up identity material at the moment it's created.** Treat it
  like an SSH private key. The current shape:
  - V7-only mode: a single file at `~/.entity/identities/{name}`.
    Copy it; encrypt the copy; store the copy off-host.
  - Identity-aware mode: a directory at `~/.entity/identities/{name}/`.
    Tar + encrypt the directory; store off-host.
- **Don't trust the SQLite DB as canonical for anything not also in
  the filesystem corpus.** Anything authored exclusively through the
  shell (no filesystem mirror) is lost on a wipe. See §6.
- **Track which binary version a peer was last running.** When you
  wipe + rebuild, knowing the prior version tells you whether the
  re-ingest produces byte-identical entities (no version drift) or
  whether to expect type-tag differences (something changed in
  core-go or workbench).

---

## 5. Binary version and schema version

Two version axes matter operationally:

### 5.1 Binary version

Today: nothing. `entity-shell --version` doesn't exist.

Direction:

- Build-time stamp injected via `-ldflags "-X main.version=…"`.
  Source: short git hash + tag if present.
- `entity-shell -version` prints it and exits 0.
- The `info` REPL command also surfaces it.

When the binary version differs from what wrote the DB, log a clear
notice at startup. Don't fail — wipe-and-rebuild is the explicit
recovery — but make the drift visible.

### 5.2 Schema version

core-go's SQLite store sets `PRAGMA user_version = 1` on init (see
`entity-core-go/core/store/sqlite.go:18`). The schema can be bumped
when entity-type definitions or table layouts change in a backwards-
incompatible way.

Direction:

- On open, the workbench reads `PRAGMA user_version` via the SQLite
  store's `DB()` accessor.
- If the read version is greater than `core-go's compile-time
  `schemaVersion`, refuse to open with a clear error: "DB schema
  version N exceeds binary's supported version M; binary upgrade
  required." Don't silently corrupt.
- If the read version is less than the binary's supported version,
  the binary may know how to read older shapes (no migration today,
  so the answer for now is also "refuse to open"). When we add
  migrations, this branch becomes the migrate-on-open path.

This is a one-day item once we sequence it. Listed in the roadmap
pre-deployment hygiene tranche.

---

## 6. Phase E is on the deployment critical path

Between Phase D (today) and Phase E (next), there's a gap that
matters operationally:

- **Phase D state** — the SQLite DB persists. Content authored via
  the shell or via continuation chains lives there. If the DB is
  wiped, that content is gone unless it was also mirrored somewhere.
- **Phase E direction** — the filesystem is the source of truth; the
  SQLite tree is a derivative. Wipe the DB, re-ingest the filesystem,
  recover.

Between those two states, anything authored exclusively in-shell
without a filesystem origin is at risk of being unrecoverable on a
wipe. That's a deployment hazard.

**Implication:** treat Phase E as the deployment gate, not as a
feature. The pitch — "your filesystem is the source of truth,
wipe-and-rebuild is the recovery procedure" — depends on it.

Until Phase E lands, the operational guidance is: **don't put
authoritative content into a deployed peer that you couldn't
reconstruct.** Shell experiments, demos, ephemeral commits — fine.
Anything you'd be upset to lose — keep a filesystem mirror by hand.

---

## 7. Operational concerns still open

Tracked here, not yet decided. These get resolved either in the
roadmap punch list or in their own follow-up direction doc when
they grow.

- **Concurrent multi-process access.** Two shell processes against
  the same SQLite file: undefined today. SQLite supports it via
  WAL mode, which `NewSqliteStore` already enables, but we haven't
  pinned the behavior with a test. Either commit to "single writer
  at a time" and document it, or test multi-process WAL semantics
  and rely on them.
- **Connection state persistence.** `system/peer/transport/{peer-id}`
  entries currently rebuild from scratch each shell launch. For a
  long-running deployed peer, we want them to persist OR a startup
  procedure that re-establishes them from a known-peers list.
- **Auto-reconnect on restart.** Once connection state persists,
  the shell should attempt to reconnect to known peers on startup.
  Failure to reconnect should be logged, not fatal.
- **Subscription rehydration.** A peer with installed `revision
  follow` chains needs them to keep firing after a restart. The
  continuation install path writes to the tree, so the chain
  config is persistent. **Closed via Stage 5 round-2 helpers:**
  the subscription engine does NOT auto-rehydrate. The boot-time
  recovery sequence is
  `Connect → RestorePriorSubscriptions → ReconcileSinceLastSeen`
  per `GUIDE-CONTINUATIONS-WORKBENCH §6.5`. **Lane 5 sugar:**
  `AppPeer.RecoverAfterRestart(ctx, publisherPeerID, publisherAddr)`
  composes all three into a single call; returns a `RecoveryResult`
  with per-step error reporting. Soft-handles the case where the
  publisher's prefix isn't auto-versioned (returns a zero-entity
  ReconcileResult, not an error). Sidecar tracking entities make
  subscription recovery deterministic across restart.
- **Auto-version re-arming.** A prefix with `auto_version: true` —
  does the auto-versioner pick that config back up on reopen? The
  prefix is in the tree, so probably yes, but pin it with a test.
- **Long-running peer hygiene.** Memory growth over weeks, dispatch-
  loop edge cases that only show up with millions of events,
  SQLite file growth without vacuum. None of these are urgent now;
  they become urgent the day we deploy.
- **Stage 5 production ceilings (per-peer, mesh).** Cross-
  peer aggregate ceiling ~6,500/sec at N=8 post core-go H-G3
  (receiver-side write-amp fix `69bbc83` — envelope_ingest Has-then-Put
  guards + NotifyingContentStore.Put short-circuit). Was ~3,277/sec
  post H-G1+H-G2 and ~900/sec pre-fix — **~7.2× over the pre-fix
  baseline**. 4-spoke meshes at ≤500/sec hub rate are lossless;
  4-spoke at 1K/sec hub rate is now ~93% (was 87%). **F10 rule of
  thumb: K > 2N** where N is the number of subscribers and K is the
  deliveryLoop worker pool size — under N too close to K, FNV-32
  hash collisions produce ~2:1 per-spoke variance. Currently doc-only;
  arch decides whether to amend v3.15 §5.2. Less operationally
  urgent post-H-G3 (per-shard ceiling is higher) but still a real
  distribution phenomenon. **F11 implicit-drain catch-up — now
  structurally lossless** within the tested shape (4 spokes / 10K/sec
  3s burst / 13s drain → 4/4 spokes 100%, spread=0). Operational
  guidance: monitor `DroppedDeliveries()` counter; periodic
  `ReconcileSinceLastSeen` is no longer urgent for healthy mesh
  deployments — reserve it for actual queue-overflow or restart
  scenarios.
- **F10 urgency upgrade (post 5-seed measurement).** FNV
  shard collision is **routinely observable at production rates**,
  not an edge case. 5-seed sweep at 4 spokes / K=8 default: 3/5 seeds
  at 2K/sec hub rate hit a 2:1 collision (60% match P-theory ~67%);
  5/5 at 5K/sec hit collision. Operationally: for deployments with
  N ≥ 4 subscribers per hub, raise core-go's worker pool to **K ≥ 2N**
  via the substrate's K-tuning knob, or expect ~50% throughput spread
  between spokes. Doc-only fix preferred; arch decides whether v3.15
  §5.2 amendment is warranted given collisions are P-predictable.
- **Topology validation post-H-G3.** Full mesh (N=3, N=4)
  symmetric writes: 100% delivery all pairs — WB-28 reentrant-deadlock
  surface stays closed. Fan-in M-to-1 (M=2/4/8 writers, 1 subscriber):
  100% delivery. Slow-consumer fan-out: per-subscription backpressure
  isolation — slow spoke drops at own ceiling (~33%), other spokes
  unaffected (91-100%). Mid-burst connection drop: only the dropped
  spoke loses events (~11% loss across 500ms drop window); other
  spokes wholly unaffected. **Operational policy:** substrate
  degrades cleanly under stress; slow / dropped consumers don't drag
  the mesh down.
- **Idempotent-write dedup (H-G3 second-order effect).** Writes that
  produce the same content hash at the same path do NOT re-fire
  subscription deliveries — `NotifyingContentStore.Put` short-circuits
  on `inner.Has(computed)`. Apps that emit idempotent state
  refreshes (heartbeat-style "still here" pings; periodic re-publish
  of state machine snapshots) benefit automatically. Note: distinct
  *data* under the same path WILL fire deliveries; only true content
  bit-equality is deduped. **Empirical leverage:**
  bimodal realism probe (`TestBimodal_HeartbeatPlusBursts`)
  measured **99% downstream-delivery suppression** under heartbeat-
  style same-content rewrites (400 published, 4 delivered — first-
  instance writes only). Burst regime in the same probe delivered
  100% (3931/3931) across 4 alternating cycles; no inter-cycle drift.
- **Stage 6 substrate envelope (post core-go OP-1 + F18 fix).**
  Two core-go fixes landed: **F18** (ctx-leak in
  `Peer.ListenReady` killing per-connection serve loops mid-burst;
  fixed at `a0d3ec6` via `Peer.serveCtx` decoupling) and **OP-1**
  (incremental trie rebuild at `core/tree/trie_incremental.go::TriePut`
  per EXTENSION-TREE v3.15 §3.4.2; commit `8636519`).
  - **Hierarchical workload envelope:** F18 reproducer @ N=2000
    burns in 44s (was 254s pre-OP-1; **5.7× speedup**), 100%
    delivery, recovery 646ms. The "≤3× target" articulated in
    HANDOFF-STAGE-6 is met for hierarchical workloads.
  - **Wide-flat workload residual:** `TestRevision_HubSpoke_Throughput_OnVsOff`
    at N=1000 measures **36.0× ± 7% per-Put cost ratio** (3 seeds)
    vs 109.7× pre-OP-1. Substantial gain but well above ≤3× target.
    The residual is wire-format-bound per core-go's analysis (Merkle
    node encoding cost is Θ(N) for wide-fanout nodes regardless of
    rebuild algorithm).
  - **Recovery envelope:** `ReconcileSinceLastSeen` scales linearly
    from N=500 (150ms) through N=2000 (567ms) — flat at ~148µs per
    entity. Helper is suitable for production restart-recovery to
    at least 2000 entities under one prefix.
  - **F18 ledger update:** OPEN → CLOSED. The original triangulation
    pointed at "autoversion handler load" — wrong categorization
    (active burn was a confounder, not cause).
  - **F22 correction:** earlier assumption (from
    `autoversion_hierarchical_test.go` docstring) that hierarchical
    paths escape the cliff is **empirically wrong**. Hierarchical
    at N=2000 took 237s vs flat 137s. `BuildTrieForPrefix` walks
    ALL bindings regardless of tree shape; post-OP-1, incremental
    descent works correctly on both shapes but wide-node encoding
    cost still dominates wide-flat. **Operators picking path shapes
    should pick for app ergonomics, not perf, except** when
    avoiding wide-fanout encoding cost (auto-version on, flat prefix,
    sustained writes — see node-shape amendment review for the
    structural fix candidates).
  - **Node-shape amendment routing.** Core-go has
    routed `STAGE-6-OP1-ROUTING-NODE-SHAPE-AMENDMENT` to arch with
    5 options (A=document, B=HAMT, C=first-byte, D=hybrid-threshold,
    E=batched-auto-version). Workbench-go's position is a **strong
    preference for B/C/D over A**; treat E as a complement, not
    substitute. If A lands, this DEPLOYMENT-DIRECTION will need a
    "path-shape advisory" subsection naming the regimes operators
    must avoid; if B/C/D land, the wide-flat cliff is structurally
    gone and this whole section's framing flips.
- **F10 K-tuning at large fan-in (Stage 6 stress sweep).**
  `TestStress_MassiveFanIn` extended Stage 5's M=8 limit to
  M=16/32. Per-spoke distribution: M=8 100% perfect; M=16 mean
  84.4% with min-spoke 876/1499 (58%); **M=32 mean 56.9% with
  min-spoke 468/1489 (31%)**. Aggregate throughput keeps growing
  (M=32: 9,030/sec total), but at default K=8 the K>2N rule is
  violated and FNV shard collision dominates per-spoke distribution.
  **Operational guidance reconfirmed:** for fan-in deployments at
  M ≥ 4 subscribers per hub, set K ≥ 2M. At default K=8, M=32 is
  fundamentally throughput-uneven — collision frequency is K-tuning
  failure, not substrate bug.
- **Re-bootstrap leak on identity-aware peer reload.** Surfaced
  by `TestStorage_SqliteIdentityBundle_RebootstrapGrowsBoundedly`
  (currently skipped; run on demand). Every restart of an identity-
  aware peer with SQLite + bundle leaks ~4 entities into the content
  store; 4 specific paths get rebound to fresh content each ceremony,
  orphaning the prior content. Path count stays stable. Likely cause
  is PI-9 / PI-10 cap entities embedding ceremony-time state that
  varies between deterministic re-runs. Bounded per-restart (4
  entities), but linear over the peer's lifetime. Decision needed:
  fix the determinism gap, or accept the growth and add a periodic
  orphan-sweep. Roadmap item 5a-followup.
- **Backup tooling.** Today: "tar the identity directory yourself."
  Direction: an `entity-shell backup identity <name>` subcommand
  that does it correctly (encryption prompt, integrity check,
  off-host destination guidance). Low priority but it's the kind
  of thing that gets done badly when it's done ad-hoc.

---

## 8. References

- `USAGE-SHELL.md §1.3` — Persistent peers walkthrough.
- `SHELL-DIRECTION.md` — the long-running CLI direction.
- `entity-core-go/core/store/sqlite.go` — the persisted store
  itself; `schemaVersion` constant lives at line 18.
