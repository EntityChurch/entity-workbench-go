# CROSS-IMPL-HELPER-REFERENCE — workbench-go canonical helpers

**Status:** Long-lived; update on helper landings and cross-impl port
events.
**Authority:** This file publishes the canonical Go reference shapes
for the 9 helpers BLESSED in arch's promotion review.
Other-impl teams (egui-rust, Godot Rust SDK, core-Rust SDK,
core-Python SDK) consume this as the structural reference when
porting; each impl team decides language-idiomatic surface.

**Out of scope:** behavioral guarantees not stated in EXTENSION-*.md
or SDK-EXTENSION-OPERATIONS.md (the spec is authoritative). This doc
captures the *shapes* and *contracts* workbench-go shipped and that
arch promoted as cross-impl reference.

---

## §0 How to use this doc

For each helper:

- **What** — one-line role
- **Where** — Go file:line citation
- **Signature** — public Go signature
- **Contract** — preconditions, side effects, failure modes
- **Design notes** — what problem it solves; why this shape
- **Tests** — pointer to canonical test cases
- **Port notes** — guidance for language-idiomatic forms

When porting: read the Go source + tests against this doc. If
behavior diverges from this doc, treat *this doc* as the convergence
target (or raise a divergence as a cross-impl coordination item per
D8 cadence — file in your repo's `reviews/` as a feedback memo to
this file).

---

## §1 Scope — generic prefix-isolation handle

**What.** Idiomatic L0 tree-op handle bound to a path prefix.
Closes over a prefix so callers thread paths relative to it.

**Where.** `entitysdk/scope.go:28` (type), `entitysdk/scope.go:40`
(constructor).

**Signature.**

```go
type Scope struct { /* ... */ }

func (a *AppPeer) Scope(prefix string) *Scope
func (s *Scope) Prefix() string
func (s *Scope) Peer() *AppPeer
func (s *Scope) Close()
func (s *Scope) Get(relPath string) (entity.Entity, bool)
func (s *Scope) Put(relPath, typeName string, data interface{}) (hash.Hash, error)
func (s *Scope) PutCAS(relPath, typeName string, data interface{}, expected hash.Hash) (hash.Hash, error)
func (s *Scope) Has(relPath string) bool
func (s *Scope) Remove(relPath string) bool
func (s *Scope) List(relPath string) []store.LocationEntry
func (s *Scope) Watch(relPattern string) (*StoreWatch, error)
```

**Contract.** Empty `relPath` resolves to the prefix itself. Relative
paths join with `/`. No cap check (L0 — by-passes the dispatch
layer). Scope handles do not own resources beyond the prefix binding
itself; `Close()` is a no-op currently but reserved for future
prefix-scoped lifecycle (subscription cleanup, etc.).

**Design notes.** Lots of workbench code operates on sub-trees:
`app/{app-id}/workspace/...`, `system/inbox/{purpose}/{instance}/...`,
etc. Without Scope, every callsite passes the prefix string and the
relative path separately, or string-concatenates. Scope makes the
prefix a first-class handle. Pattern lifted from GUIDE-SDK-PATTERNS
§1.

**Tests.** `entitysdk/scope_test.go` (Put/Get/List/PutCAS round-trip;
Watch matches relative pattern).

**Port notes.** Highest-leverage promotion candidate: egui-rust has
flat-only history/query/inbox per their PARITY-MATRIX. Rust idiom is
a struct holding `Arc<Peer>` + prefix, methods take `&self` +
relative path. Closure handles for Watch should use the same
`<-chan ChangeEvent` shape (Rust: `mpsc::Receiver`).

---

## §2 RestorePriorSubscriptions — post-restart subscription recovery

**What.** Replays subscriptions registered before the peer process
restarted. Reads sidecar tracking entities the SDK persists at
registration time and re-binds the inbox handlers.

**Where.** `entitysdk/subscription_restore.go:109`.

**Signature.**

```go
type RestoredSubscription struct {
    InboxID    string
    RemotePeer string
    Pattern    string
    Opts       SubscribeOpts
}

func (a *AppPeer) RestorePriorSubscriptions() ([]RestoredSubscription, []error)
```

**Contract.** Reads tracking entities at
`system/sdk/subscription-tracking/{inbox-id}`. For each, replays
`SubscribeAt(remotePeer, pattern, opts)`. Returns the list of
restored bindings + per-binding errors. **Subscriptions that did NOT
persist a tracking entity at registration are NOT recovered** —
silent omission. Callers that need durability must explicitly opt in
when subscribing.

**Design notes.** Closes F6 (HIGH) from Stage 5 round-1 — the
substrate does not auto-rehydrate subscriptions after restart. Per
arch's response, this is by-design at the substrate layer; recovery
is a consumer responsibility, codified in EXTENSION-SUBSCRIPTION
v3.15 §5.7. The sidecar-tracking pattern is the workbench-go
contribution: cheap (one entity per sub), declarative (subscriptions
are now first-class entities in the tree), and survives any restart
shape (process restart, host reboot, full identity-bundle restore).

**Tests.** `entitysdk/subscription_restore_test.go` —
post-restart delivery (25/25 vs F6-baseline 0/25); error fan-out for
broken bindings.

**Port notes.** Rust idiom: `pub async fn
restore_prior_subscriptions(&self) -> (Vec<RestoredSubscription>,
Vec<Error>)`. Per-binding errors should be reported per-error, not
collapsed into a single fatal — partial recovery is better than no
recovery.

---

## §3 ReconcileSinceLastSeen — incremental cross-peer catch-up

**What.** Pulls writes a remote peer committed under a prefix after
a last-seen revision hash. Composes `revision:fetch-diff` +
`tree:merge`.

**Where.** `entitysdk/reconcile.go:62`.

**Signature.**

```go
type ReconcileResult struct {
    Entities    int    // number of entities pulled
    LastSeen    hash.Hash
    NoChange    bool
}

func (a *AppPeer) ReconcileSinceLastSeen(
    ctx context.Context,
    remotePeerID, prefix string,
    lastSeen hash.Hash,
) (ReconcileResult, error)
```

**Contract.** Pass `hash.Hash{}` (zero) for `lastSeen` on bootstrap
— pulls the full closure under prefix. Pass the prior call's
returned `LastSeen` for incremental delta. Idempotent: re-calling
with the same `lastSeen` is a no-op (`NoChange: true`).

**Design notes.** Closes F3+F7 from Stage 5 round-1 — missed
writes during downtime are recoverable but only via explicit pull;
the substrate surfaces nothing about what changed. This helper
threads the last-seen hash so callers don't re-pull the world on
every boot. Uses Revision FetchDiff (typed wrapper at
`entitysdk/revision.go:471`) + `tree:merge`. The composition is the
contribution; FetchDiff itself is spec-shipped (REVISION v3.4).

**Tests.** `entitysdk/reconcile_test.go` — bootstrap (41 entities
for 20 leaves) + incremental (31 for 15 new leaves) + idempotent
re-call.

**Port notes.** Rust idiom: `pub async fn reconcile_since_last_seen
(...)`. `NoChange` flag is non-load-bearing — entity count == 0 +
unchanged last_seen also signals it. Keep the explicit flag for
read-clarity.

---

## §4 IdentityBundle — on-disk identity manifest

**What.** Directory-on-disk shape for the identity-extension's
identity material (keypair + quorum + role assignments + certs).
Implements SDK-IDENTITY-INFRASTRUCTURE §8.4 Revision 6 Amendment 1.

**Where.** `entitysdk/identity_bundle.go:42` (type),
`entitysdk/identity_bundle.go:127` (Write),
`entitysdk/identity_bundle.go:163` (Load).

**Signature.**

```go
type IdentityBundle struct {
    Name        string
    Manifest    BundleManifest // ID, Version, CreatedAt, members
    PrimaryKey  crypto.Keypair
    Members     []MemberKey
    Quorum      *QuorumPart
    Attestation *AttestationPart
}

func WriteIdentityBundle(bundle IdentityBundle) error
func LoadIdentityBundle(name string) (IdentityBundle, error)
func IsIdentityBundleDir(name string) (bool, error)
func IdentityBundleDir(name string) (string, error)
```

**Contract.** Writes a directory `{IdentityBundleDir(name)}/`
containing: `manifest.txt` (plain-text header), `keypair.bin`
(primary key), `members/{id}/keypair.bin` (member keys),
`quorum.cbor` (optional), `attestation.cbor` (optional). Load is
the symmetric reader. `IsIdentityBundleDir(name)` detects the
shape without opening keys.

**Design notes.** Spec'd shape; workbench-go shipped the first
reference impl. Other impls (Rust SDK, Python SDK) currently carry
only the flat-keypair form (`crypto.LoadIdentity/SaveIdentity`,
which is the *misnamed* upstream form — see DEPLOYMENT-DIRECTION
§3 terminology note). Bundle includes the full identity stack
context so a peer can re-bootstrap from disk without re-running the
quorum ceremony.

**Tests.**
`entitysdk/identity_bundle_test.go` — round-trip + edge cases
(missing manifest, partial bundle).

**Port notes.** Rust idiom: `struct IdentityBundle` mirroring fields;
`fn write_identity_bundle(&self) -> Result<()>`,
`fn load_identity_bundle(name: &str) -> Result<IdentityBundle>`.
Filesystem layout is the cross-impl convention — implementations
MUST emit the same directory structure or the bundles aren't
exchangeable across hosts.

---

## §5 BootstrapIdentity — E2E identity setup workflow

**What.** End-to-end identity bootstrap: generate primary keypair,
run K-of-N quorum ceremony, issue controller cert, optionally write
the resulting bundle to disk.

**Where.** `entitysdk/identity_bootstrap.go:89`.

**Signature.**

```go
type BootstrapOpts struct {
    Name        string
    QuorumK     int
    QuorumN     int
    MemberKeys  []crypto.Keypair // optional; generates fresh if nil
    WriteBundle bool             // write to IdentityBundleDir on success
}

type BootstrapResult struct {
    PrimaryKey  crypto.Keypair
    Bundle      IdentityBundle
}

func (a *AppPeer) BootstrapIdentity(ctx context.Context, opts BootstrapOpts) (BootstrapResult, error)
func (a *AppPeer) ApplyIdentityBundle(...) // re-apply a prior-loaded bundle
```

**Contract.** Single-call workflow that produces a fully-configured
identity-bundled peer. Caller passes K, N; helper generates keys (or
uses caller-supplied), runs the quorum ceremony in-memory, issues
controller cert, writes bundle if requested. `ApplyIdentityBundle`
re-applies a loaded bundle to a peer that already exists (useful for
bundle-restore-from-backup scenarios).

**Design notes.** The substrate layer exposes ceremony primitives
(CreateQuorum, CreateAttestation, …) but composing them is
boilerplate every consumer would otherwise repeat. This helper
absorbs the composition. Reference for the rest of the ecosystem.

**Tests.** `entitysdk/identity_bootstrap_test.go` — full
ceremony + member-key sourcing variants.

**Port notes.** Rust idiom: builder pattern (`BootstrapOpts::new()
.k(2).n(3).build_async(&peer)`). Async first-class given quorum
involves multiple round-trips.

---

## §6 Compute.Builder + S1 sub-expression ops

**What.** Typed expression-builder for compute expressions (the
"S1 builder" — first standard IR floor authoring surface). Covers
Literal, LookupScope, LookupTree, Field, Index, Length, NumericCast,
Arithmetic, Compare, Logic, Construct, Apply, ApplyClosure,
BuiltinsCall, If, Let, Lambda.

**Where.** `entitysdk/compute_builder.go:49` (ComputeBuilder),
`entitysdk/compute_builder.go:57` (Builder),
`entitysdk/compute_builder.go:107` (entry).

**Signature.** See file. Entry:

```go
func (a *AppPeer) Compute() *ComputeBuilder

func (cb *ComputeBuilder) Literal(v interface{}) *Builder
func (cb *ComputeBuilder) LookupTree(path string, relative bool) *Builder
func (cb *ComputeBuilder) Field(target *Builder, name string) *Builder
func (cb *ComputeBuilder) Arithmetic(op string, left, right *Builder) *Builder
// ... full surface in source
func (b *Builder) Build(ctx context.Context, rootPath string) (hash.Hash, error)
```

**Contract.** Each builder method produces a `*Builder` node which
can be composed. Final `.Build(ctx, rootPath)` installs the
expression tree under `rootPath` and returns the root hash.
Canonical sort applied at Apply args (Rule 11 + EXTENSION-COMPUTE
§ pinned).

**Design notes.** Reference impl per arch's E7 appendix in
SDK-EXTENSION-OPERATIONS §8. Workbench-go is the **only typed
ComputeOps in any impl** today — Godot + egui-rust + Rust SDK +
Python SDK all lack typed builders. Per arch's promotion decision,
this is the canonical reference shape.

**Tests.** `entitysdk/compute_builder_test.go`,
`compute_translation_test.go` (9 tests covering CBOR indexing, Rule
11 NumericCast, Construct composition); `compute_scenarios_test.go`
(filter+sum, Let-workaround, aggregation, two-stage pipeline).

**Port notes.** Rust idiom: trait `ComputeBuilder` with associated
type `Builder`. Sub1 pitfalls baked-in (Rule 11 cast doesn't flow
through Let; canonical sort on Apply args; lambda vs closure hashes)
— see `feedback_compute_s1_pitfalls` in workbench-go's auto-memory
for the design constraint rationale. Don't re-discover at port time.

---

## §7 MintChainCapability + BundleCrossPeerChain

**What.** Cap-token minting with authority-chain validation. Local
form (`MintChainCapability`) and cross-peer envelope form
(`MintCrossPeerChainCapability` + `BundleCrossPeerChain`).

**Where.** `entitysdk/capability_mint.go:56` (local),
`entitysdk/capability_mint.go:110` (bound),
`entitysdk/cross_peer_cap_mint.go:61` (cross-peer mint),
`entitysdk/cross_peer_cap_mint.go:162` (bundle).

**Signature.**

```go
func (a *AppPeer) MintChainCapability(grants []types.GrantEntry) (entity.Entity, error)
func (a *AppPeer) MintChainCapabilityBound(grants []types.GrantEntry, treePath string) (entity.Entity, error)

func (a *AppPeer) MintCrossPeerChainCapability(
    remotePeerID string,
    grants []types.GrantEntry,
    expiresAt *uint64,
) (entity.Entity, error)

func (a *AppPeer) BundleCrossPeerChain(leafCap entity.Entity) (map[hash.Hash]entity.Entity, error)
```

**Contract.** `MintChainCapability` produces a local cap chained
under the peer's owner self-cap. `Bound` form pins the cap to a
specific tree path. Cross-peer forms produce a cap rooted at the
remote's authority and the bundle that needs to travel with it
(parent caps in the chain).

**Design notes.** Cap-chain semantics are spec-defined (V7 §5.x,
EXTENSION-CONTINUATION §3.2), but minting them in idiomatic code is
boilerplate-heavy. These helpers absorb the composition. Validated
by Stage 3 cap-delegation tests
(`shellcmd/cmd_stage3_cap_delegation_test.go`).

**Tests.** `entitysdk/capability_mint_test.go`,
`entitysdk/cross_peer_cap_mint_test.go` (structural contract).

**Port notes.** Rust idiom: same signatures, but cap entity should
be returned as a wrapping typed `ChainCap` newtype so the type
system surfaces "this is a cap chain, not a generic entity."

---

## §8 InstallRevisionFollowChain — V-1 cross-peer follow pattern

**What.** Installs the canonical 2-step content-mirror chain
(`subscribe head → revision:fetch-diff → tree:merge`) that
auto-mirrors a remote peer's subtree state when its revision head
advances. The V-1 closed pattern.

**Where.** `shellcmd/cmd_revision_follow.go:158`.

**Signature.**

```go
func InstallRevisionFollowChain(
    ctx context.Context,
    local *entitysdk.AppPeer,
    remoteID, prefix string,
) (*entitysdk.RawSubscription, error)
```

**Contract.** Installs:
1. A continuation chain at `system/inbox/follow/{remoteID}/{prefix-slug}/`
2. A subscription on `system/revision/head/{prefix}` on `remoteID`
   that dispatches into step 1

The chain runs forever until torn down; cross-peer; chain-error
markers bound under the local peer's runtime namespace.

**Design notes.** Per arch's GUIDE-REVISION-AUTO-VERSION §4 "Form
1." REVISION v3.4 ratified fetch-diff to make this chain-expressible
in 2 steps instead of N. Cross-impl-validated 9/9 on rust + python.
This is the reference shape for "mirror this prefix into our tree."

**Tests.** `shellcmd/cmd_revision_follow_test.go`, plus the Stage 3
cycle test suite.

**Port notes.** Rust idiom: `pub async fn install_revision_follow
_chain(...)`. Chain-error vocab MUST be the unified v1.19 form
(merged in core-go `5792cdc`).

---

## §9 perfreview/ probe shapes — D10 reference

**What.** Build-tagged probe pattern for saturation, restart-
equivalence, partition-tolerance, and per-component cost breakdown
measurements.

**Where.** `perfreview/saturation_multipeer_test.go`,
`restart_mesh_test.go`, `partition_test.go`,
`delivery_breakdown_test.go`, `harness.go`.

**Pattern.** Build-tag separation (`//go:build perfreview`) keeps
the probes out of the default `make test` sweep; opt-in via
`make perfreview`. Each probe family:

- Multi-subtest with log-spaced parameters
- Harness abstraction (`perfreview/harness.go`) for peer-setup
  boilerplate
- Asserts measured ceilings against known thresholds; logs raw
  numbers regardless

**Design notes.** Promoted as the workbench-go form of D10 (Real-
Session Coverage). The pattern is what travels; each impl ports its
own measurements. Cross-impl convergence on saturation thresholds
(F1/F2 fix shape applies uniformly) is the eventual ask once core-
rust + core-python land their H-G1/H-G2 equivalents.

**Tests.** N/A — these *are* tests.

**Port notes.** Rust idiom: `#[cfg(feature = "perfreview")]`
gating on the test functions. Python: separate test module with a
pytest marker. Harness shape: per-impl idiomatic; the *measurements*
are the cross-impl artifact.

---

## §10 Open promotion candidates (not yet BLESSED)

Helpers worth promoting but not in the current cycle's BLESSED set:

- **CrossPeerFollower aggregation form** — V-1 close was the
  per-prefix shape; aggregate "follow N prefixes from M peers" is a
  panel-level battle-test surface still open.
- **AppPeer.OnPrefixChange / Watch fluent forms** — `Subscribe` +
  `*Subscription.Events()` is the substrate primitive; ergonomic
  `OnPrefixChange(prefix, handler)` is a wrapper we'd offer once
  enough cross-impl forms emerge.

These remain workbench-internal until arch promotes them in a
future cycle.

---

## §11 References

The 9 helpers cataloged here are the BLESSED set from arch's
cross-impl promotion review. Behavioral guarantees live in the
SDK extension operations spec (`SDK-EXTENSION-OPERATIONS.md` v0.9)
and the identity infrastructure spec
(`SDK-IDENTITY-INFRASTRUCTURE.md`), which remain authoritative for
contracts; this doc captures the Go reference shapes.

---

## §12 Changelog

- Initial publication: 9 helpers cataloged from arch's BLESSED
  promotion list, with cross-impl port hints per helper.
