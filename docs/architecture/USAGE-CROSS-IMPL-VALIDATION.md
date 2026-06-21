# Cross-impl validation — how to run it

**Audience:** anyone (including future Claude) who needs to re-validate
that ratified spec features land conformant across the three peer impls
(Go / Rust / Python). Existence proof: the
`revision:fetch-diff` + `result_merge` + `collect_keys` + `revision:pull`
arc cycled through this workflow several times during May 2026.

## When to use this doc

You're using this when one of these is true:

1. A new ratified spec feature lands and you need to add probes for it
   to the workbench cross-impl harness, then verify rust + python.
2. Rust or Python ships an impl update for a feature whose probe was
   already FAIL-state, and you need to flip the validation memo to PASS.
3. You want to confirm "where are we" across the three impls without
   inventing a new test rig.

If none of these — you don't need to be here.

## Where the probe lives

Single file:
`entitysdk/cross_impl_validate_ratified_test.go`. One test
(`TestCrossImpl_RatifiedFeatures`) that walks a list of `namedCrossImplCheck`
entries and reports PASS / FAIL / SKIP per probe.

- **Env-gated:** the test skips unless `CROSS_IMPL_ADDR` is set. It does
  NOT run in default `make test`.
- **Non-fast-failing:** runs every probe even if some fail, so a single
  invocation surfaces the complete impl gap rather than first-failure-only.
- **One peer per invocation:** point `CROSS_IMPL_ADDR` at the target
  peer (rust OR python), set `CROSS_IMPL_LABEL` to a string that appears
  in the test log so the output is grep-friendly.

Each probe is one function on the `crossImplProbe` struct, returning a
`crossImplResult` with PASS/FAIL/SKIP + a one-line diagnostic. Adding a
new probe = add the function + one entry to the `checks` slice in
`TestCrossImpl_RatifiedFeatures`.

## How to invoke

### 1. Start the rust peer

```
cd ~/projects/entity-systems/entity-core-rust
./target/debug/entity peer start rust-validate \
    --listen 127.0.0.1:9000 --debug-grants
```

Run in background or a separate terminal. The peer prints its peer_id
and lists registered handlers — keep the listen address handy. If
`rust-validate` doesn't exist as a peer, `./target/debug/entity peer
init rust-validate --admin <identity-name>` first (rust will tell you
which identity is needed).

### 2. Start the python peer

```
cd ~/projects/entity-systems/entity-core-py
.venv/bin/entity-core start --listen 127.0.0.1:9001 \
    --identity python-validate --open-access
```

Same as rust — leave running, capture the peer_id. If
`python-validate` identity doesn't exist:
`entity-core list-identities` then create one or use the default.

### 3. Run the workbench probe against each

```
cd ~/projects/entity-systems/entity-workbench-go

CROSS_IMPL_ADDR=127.0.0.1:9000 CROSS_IMPL_LABEL=rust \
    make test-sdk ARGS="-run TestCrossImpl_RatifiedFeatures -v -count=1"

CROSS_IMPL_ADDR=127.0.0.1:9001 CROSS_IMPL_LABEL=python \
    make test-sdk ARGS="-run TestCrossImpl_RatifiedFeatures -v -count=1"
```

Output shape:

```
=== cross-impl-validate-ratified  target=<label>  addr=<addr> ===
  [PASS] probe-name-1   short-explanation
  [PASS] probe-name-2   short-explanation
  [FAIL] probe-name-3   expected X; got Y
  ...
  <label> summary  pass=N fail=M skip=K total=N+M+K
```

Test exits non-zero (FAIL) if any probe is FAIL. SKIP doesn't
fail the test.

### 4. Kill the peers when done

```
pkill -f "entity peer start rust-validate"
pkill -f "entity-core start --listen 127.0.0.1:9001"
```

Or by pid if you captured it. They're memory-only by default (no
persistent state to clean up).

## Writing a new probe

Pattern (copy from existing probes in the test file):

```go
func (p *crossImplProbe) checkMyNewFeature() crossImplResult {
    // 1. Build the dispatch params.
    req := types.MyFeatureParamsData{ /* ... */ }
    ent, _ := req.ToEntity()

    // 2. Dispatch to the remote peer.
    o := observe(p.executeOnRemote("system/<handler>", "<op>", ent))

    // 3. Diagnose the outcome against the spec-pinned expectation.
    if o.status == 400 && o.code == "unknown_operation" {
        return ciFail("op not yet implemented")
    }
    if o.status >= 200 && o.status < 300 {
        return ciPass(fmt.Sprintf("status=%d (op recognized)", o.status))
    }
    return ciFail(fmt.Sprintf("expected ...; got status=%d code=%q", o.status, o.code))
}
```

Then add to the `checks` slice at the top of
`TestCrossImpl_RatifiedFeatures`:

```go
{"my_feature/specific_behavior", probe.checkMyNewFeature},
```

Naming convention: `<feature_area>/<specific_behavior>`. The slash is
a logical group separator that lets readers scan the output.

### When NOT to add a probe

- **Full end-to-end multi-peer flows that require the remote impl to
  dial back into us.** Cross-peer reverse-dial is sensitive to test
  topology and gives false negatives. Cover those with workbench-side
  direct-dispatch tests (one peer's handler dispatched directly via
  SDK Executor, like `entitysdk/revision_pull_op_test.go`).
- **Anything that needs a multi-step continuation chain to be installed
  on the remote peer.** Same reason. The probe's job is op-recognition
  + parameter-validation + spec-pinned error codes — install-shape
  validation is fine; install-then-fire-trigger is brittle.

## Where the results get written

Each validation pass produces a memo in `docs/architecture/reviews/`:

```
CROSS-IMPL-VALIDATION-<FEATURE>-<YYYY-MM-DD>.md
```

Memo structure:

1. **Headline table**: each impl, pass/fail count, status.
2. **Probe configuration**: what's being probed and why this run.
3. **Per-impl results**: literal probe output + interpretation. PASS
   probes can be terse; FAIL probes get a "Finding R-N" section with
   spec citation + recommended fix.
4. **What this validates**: what the run proves end-to-end.
5. **Recommended next steps**: routed by team (arch / core-X / us).
6. **Provenance + reproduction**: probe source path, peer impl source
   paths, link back to this how-to doc.

If only one impl needs fixes, surface the impl-side findings clearly
so the relevant core-X team has actionable items.

## Lifecycle

The cross-impl probe is **living**. As new ratified features land:

1. Add probes to `cross_impl_validate_ratified_test.go`.
2. Run against rust + python.
3. Write the validation memo.
4. If findings: route to the impl team, leave probes in place; flip
   FAIL → PASS in a follow-up memo when the impl ships.

Probes for already-ratified-and-validated features stay in the test
forever. They're regression guards — if rust or python ever regresses
on `result_merge` or `collect_keys`, the next probe run catches it.

## Validation history

Cross-impl validation runs are recorded as dated memos under
`docs/architecture/reviews/` (named `CROSS-IMPL-VALIDATION-<FEATURE>-<DATE>.md`).
The established baseline covers ratified features (9/9), fetch-diff +
`result_merge`, and revision-pull, each validated against the Rust and Python
implementations.

Reading them in order shows the cadence: ratify → workbench impl +
probes → cross-impl run → memo → impl-side fixes if needed → re-run.
