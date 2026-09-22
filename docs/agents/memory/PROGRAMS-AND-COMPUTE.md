# Programs & compute

The programs/ track, the input-port contract, determinism, and the Axis-1 admission corpus that is not wired into any target.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **`programs/` is a sibling of `workbench`, not a layer of it.** It holds the generic
  compute host, program descriptors, and the Life/Snake/Asteroids/heavyfield programs —
  extracted from `workbench/program_*.go` on 2026-07-22 because a research track does not
  belong inside the app's renderer-neutral model layer. It depends on `entitysdk` **only**;
  it must never import `workbench` (that dependency was zero at extraction — keep it zero).
  `avalonia/bridge` imports it as `pg`. Tests: `make test-programs`.
- **`Host.Input` ENQUEUES; it does not write through** (AP36). A clock-driven program reads its
  input ports at tick time and only then, so a value superseded before the next tick was never
  observed by anything — at Life's 6 Hz that window is **167 ms** and a mouse click is ~25 ms.
  Measured: **1 of 6 d-pad clicks moved the cursor**; 6 of 6 when held past a tick. The contract
  now is *every value offered to a port is observed by exactly one tick, in order* — bounded at
  `inputQueueMax`, coalescing at the tail on overflow, deduping an identical consecutive value,
  and **shape-agnostic** (the host still never decodes a program's bytes). The known limit is
  stated in the doc comment: a bit released and re-pressed between two ticks reads as one
  continuous hold, because the queue carries port *values*, not an event stream.
  **A test that writes the input and then calls `tickOnce()` itself cannot see any of this** —
  that is what every pre-2026-08-21 program test does. The region test is
  `programs/host_input_queue_test.go`, which drives a **running clock** from outside.
- **Compute has no randomness, on purpose — so a program's "randomizer" is a hash, and a
  hash that is linear in its varying input is a TRANSLATION** (AP38). Interactive Life's
  Regen slid one fixed pattern for a month: under a power-of-two modulus, bit *k* of an LCG
  step depends only on bits 0..*k* of its input, so bumping the generation counter is
  arithmetically the same as shifting the cell index. Measured at 0.98–1.00 agreement under
  a cyclic shift, and the operator's report was *"it moves the same map one or two over."*
  **One nonlinear round** (square, then fold the high bits back down — squaring mod 2^k
  leaves the low bits weak) is the minimum. Two things to carry: `!equal(before, after)` is
  the wrong assertion — a translation is never equal, so an anti-vacuity clause passes on
  every one of these boards — and **population/variance is the cheap tell**, since a
  translation preserves the count (σ 0.76 where an independent draw gives 7.75). Gate:
  `TestLifeEdit_RegenIsNotATranslation`. Determinism itself is correct and load-bearing —
  reproducible state hashes are the whole point; the entropy is the tick counter at the
  moment of the press.
- `docs/architecture/` — canonical framework (charter, `MODEL-AVALONIA-RUNTIME.md`,
  `GUIDE-AVALONIA-PANEL-PATTERNS.md` recipes P0–P7, `TESTING-STRATEGY.md`,
  `LOGGING-CONVENTIONS.md`, `DEPLOYMENT-DIRECTION.md`, `SHELL-DIRECTION.md`,
  `CROSS-IMPL-HELPER-REFERENCE.md`, `PERFORMANCE-CHARACTERISTICS.md`). These are **undated /
  living** — edit in place.
  - **`docs/STATUS.md` — the rolling status log, and it lives OUTSIDE `docs/status/` on
    purpose** (moved there 2026-08-24, to match the rest of the ecosystem).
    `docs/status/` is stripped wholesale from the published tree under [ADR-0031], so a
    rolling log left inside it is a canonical document sitting in a directory whose whole
    meaning is "none of this publishes". The path *is* the declaration: out of that
    directory means canonical, in it means working memory. **`docs/STATUS.md` is published
    — write it for the next session, but know a stranger can read it.**
  - `docs/status/` — the ephemeral area: dated `STATUS-YYYY-MM-DD.md` snapshots (immutable
    once published) and `HANDOFF-*`. Never published, no scrub obligation.
  - `docs/architecture/reviews/` — dated cross-team exchanges (`reviews/{TOPIC}-{DATE}.md`);
    closed ones move to `reviews/archive/`.
- **A compute error is a VALUE, and what happens to it depends on the POSITION, not on how it
  arrived** (`entitysdk/axis1/contain.go`, AP43). Two representations exist at every result site —
  *minted* (the evaluator raised it) and *value-form* (a `compute/error` entity arrived as an
  ordinary result), and §2.4 forbids them taking different paths. Three positions, and each call
  site in `axis1/eval.go` names its own: **CONSUMED** — the result is READ (arith/compare/logic
  operand, `if` condition, cast value, field target, construct field, index and its array, any
  collection operand, a filter predicate) → both short-circuit, via `evaluator.operand`;
  **CONTAINED** — the result is PLACED without being read (`map`'s output element, `fold`'s
  accumulator and `initial`) → both become a value in that slot, except `budget_exhausted` /
  `cascade_limit`, whose counters are not restored on unwind (`depth` *is*, so `depth_exceeded`
  contains like anything else); **BOUNDARY** — a contained error element materializes
  **code-only**, because `message` is prose no spec pins and containing it forks the array's bytes
  cross-impl. Getting the split wrong is silent under any test whose closures cannot fail — the
  filter half of the defect **kept** elements whose predicate had failed rather than erroring.
  Gates: `TestAxis1Equivalence_ContainedErrorPositions` (one vector per position) and, the real
  one, **arch's differential corpus below.**
- **`TestAxis1Admission_*` SKIPS unless you set `AXIS1_ADMISSION_CORPUS`, so a green
  `make test-sdk` says NOTHING about AE-5** — and a skip counts as a failure (AGENTS-STANDARD).
  This is the gate that matters and it is not wired into any target. Run it:

  ```
  (cd ../entity-core-go && go run ./cmd/internal/compute-corpus generate --profile inproc --out /tmp/c.cbor \
     && go run ./cmd/internal/compute-corpus emit --corpus /tmp/c.cbor --out /tmp/ref.cbor)
  (cd entitysdk && AXIS1_ADMISSION_CORPUS=/tmp/c.cbor AXIS1_ADMISSION_OUT=/tmp/a1.cbor go test -run TestAxis1Admission .)
  (cd ../entity-core-go && go run ./cmd/internal/compute-corpus cross-bless --corpus /tmp/c.cbor \
     --emission /tmp/ref.cbor --emission /tmp/a1.cbor)
  ```

  **Measured 2026-08-25 (corpus `8d2f55c8`, 362 vectors, core-go `13a42ea`): 334 agree, 0 diverge,
  28 INCOMPLETE — NOT LOCKED.** Every vector Axis-1 *answers* is byte-identical to the reference;
  the 28 deopt to Stage-1, and under **AE-6 a deopted vector's evidence is void**, so **AE-5 is not
  green and has not been since the v3.24/v3.25 primitives landed.** Do not quote the 2026-07-23
  admission as current. The corpus names the divergence-prone classes in its vector IDs
  (`cv8a-map-contains-minted-error`, `cv8c-filter-predicate-error-shortcircuit`,
  `cv9a-map-depth-exceeded-contains`, `worked/value-error/*`) — **it catches the AP43 bug in five
  vectors**, measured by re-running it against the pre-fix engine. Our home-grown 300-case fuzz
  found three cases and no cause; this corpus names them. **Reach for it first.**
