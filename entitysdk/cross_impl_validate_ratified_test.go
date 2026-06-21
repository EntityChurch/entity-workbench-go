package entitysdk_test

// Cross-implementation conformance probes for the ratified features
// that the workbench POC drove through arch:
//   - EXTENSION-CONTINUATION v1.15  — collect_keys transform op
//   - EXTENSION-CONTINUATION v1.16  — result_merge param-assembly mode
//                                     + per-reason lost-error marker path
//   - EXTENSION-REVISION       v3.4 — revision:fetch-diff incremental op
//
// EXTENSION-TREE v3.14 `tree:extract.since` was WITHDRAWN at v3.15
// (`PROPOSAL-TREE-EXTRACT-SINCE.md` Amendment 1 / Option E — wrong
// extension); the capability re-homed to `revision:fetch-diff`. The
// prior v3.14 probes are gone from this file; the audit-trail history
// is preserved by `poc_incremental_sync_test.go`.
//
// Per the arch team's "land-now-coordinate-during-build" posture, the
// workbench (POC site that drove ratification) reports what cross-impls
// do and don't have. Rust + Python land `revision:fetch-diff` +
// `result_merge` during their build cycle; this probe is the re-
// validation harness once they do.
//
// The test is env-gated so it never runs in the default sweep: set
//   CROSS_IMPL_ADDR=host:port  CROSS_IMPL_LABEL=rust  to invoke.
//
// Example:
//   CROSS_IMPL_ADDR=127.0.0.1:40525 CROSS_IMPL_LABEL=rust \
//       go test ./entitysdk -run TestCrossImpl_RatifiedFeatures -v
//
// Each probe prints PASS / FAIL / SKIP with a one-line diagnostic.
// On FAIL the test continues to surface ALL non-conformances per peer
// rather than fast-failing on the first.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

func TestCrossImpl_RatifiedFeatures(t *testing.T) {
	addr := os.Getenv("CROSS_IMPL_ADDR")
	if addr == "" {
		t.Skip("CROSS_IMPL_ADDR not set — env-gated cross-impl conformance probe")
	}
	label := os.Getenv("CROSS_IMPL_LABEL")
	if label == "" {
		label = "peer"
	}
	t.Logf("=== cross-impl-validate-ratified  target=%s  addr=%s ===", label, addr)

	probe := newCrossImplProbe(t, addr)
	defer probe.close()

	checks := []namedCrossImplCheck{
		{"collect_keys/mutual_exclusion", probe.checkCollectKeysMutualExclusion},
		{"collect_keys/singular_install", probe.checkCollectKeysSingularInstall},
		{"collect_keys/plural_install", probe.checkCollectKeysPluralInstall},
		{"revision_fetch_diff/full_closure_zero_base", probe.checkFetchDiffZeroBase},
		{"revision_fetch_diff/incremental_bandwidth", probe.checkFetchDiffIncremental},
		{"revision_fetch_diff/base_not_found", probe.checkFetchDiffBaseNotFound},
		{"revision_fetch_diff/no_local_state", probe.checkFetchDiffNoLocalState},
		{"result_merge/install_accepted", probe.checkResultMergeInstall},
		{"result_merge/mutex_with_result_field", probe.checkResultMergeMutex},
		{"revision_pull/op_recognized", probe.checkPullOpRecognized},
		{"revision_pull/missing_remote_rejected", probe.checkPullMissingRemote},
	}

	pass, failCnt, skipCnt := 0, 0, 0
	for _, c := range checks {
		out := c.fn()
		switch out.status {
		case "PASS":
			pass++
		case "FAIL":
			failCnt++
		case "SKIP":
			skipCnt++
		}
		t.Logf("  [%s] %-46s %s", out.status, c.name, out.detail)
	}
	t.Logf("\n  %s summary  pass=%d fail=%d skip=%d total=%d",
		label, pass, failCnt, skipCnt, len(checks))
	if failCnt > 0 {
		t.Errorf("%s: %d cross-impl conformance check(s) failed (see log above)", label, failCnt)
	}
}

type namedCrossImplCheck struct {
	name string
	fn   func() crossImplResult
}

type crossImplResult struct {
	status string
	detail string
}

func ciPass(detail string) crossImplResult { return crossImplResult{"PASS", detail} }
func ciFail(detail string) crossImplResult { return crossImplResult{"FAIL", detail} }
func ciSkip(detail string) crossImplResult { return crossImplResult{"SKIP", detail} }

type crossImplProbe struct {
	t      *testing.T
	local  *entitysdk.AppPeer
	remote string
	ctx    context.Context
	cancel context.CancelFunc
}

func newCrossImplProbe(t *testing.T, addr string) *crossImplProbe {
	t.Helper()
	local, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("create local peer: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- local.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		cancel()
		t.Fatalf("listen: %v", err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("listen timeout")
	}
	conn, err := local.Connect(ctx, addr)
	if err != nil {
		cancel()
		t.Fatalf("connect %s: %v", addr, err)
	}
	sess := conn.Session()
	if sess == nil {
		cancel()
		t.Fatal("connection session unavailable after Connect")
	}
	return &crossImplProbe{
		t:      t,
		local:  local,
		remote: string(sess.RemotePeerID),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *crossImplProbe) close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.local != nil {
		_ = p.local.Close()
	}
}

func (p *crossImplProbe) executeOnRemote(handlerURI, op string, params entity.Entity) (*entitysdk.Response, error) {
	uri := fmt.Sprintf("entity://%s/%s", p.remote, handlerURI)
	return p.local.Executor().ExecuteOnResource(uri, op, params, nil)
}

// outcome extracts (status, code) from either a non-nil response or a
// wrapped SDK error. The SDK wraps any non-2xx as *entitysdk.Error and
// returns it as the error result, so probes that expect 4xx/404 spec
// rejections must inspect the wrapped error rather than just `resp`.
type outcome struct {
	status uint
	code   string
	msg    string // human-readable; populated when known
	resp   *entitysdk.Response
}

func observe(resp *entitysdk.Response, err error) outcome {
	if err != nil {
		var sdkErr *entitysdk.Error
		if errors.As(err, &sdkErr) {
			return outcome{status: sdkErr.Status, code: sdkErr.Code, msg: sdkErr.Message}
		}
		// Genuine transport / non-SDK error.
		return outcome{status: 0, code: "transport_error", msg: err.Error()}
	}
	if resp == nil {
		return outcome{status: 0, code: "nil_response"}
	}
	if resp.Status >= 400 {
		// This path is rarely hit (SDK wraps to err first) but covered
		// for robustness.
		return outcome{status: resp.Status, code: errorCodeOf(resp), resp: resp}
	}
	return outcome{status: resp.Status, resp: resp}
}

// --- collect_keys probes ---

func (p *crossImplProbe) checkCollectKeysMutualExclusion() crossImplResult {
	contData := types.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/tree", p.remote),
		Operation: "extract",
		Resource:  &types.ResourceTarget{Targets: []string{"any/"}},
		ResultTransform: &types.ContinuationTransformData{
			TransformOps: []types.ContinuationTransformOpData{
				{Op: "collect_keys", Field: "added", Fields: []string{"changed"}, Into: "paths"},
			},
		},
	}
	contEnt, err := contData.ToEntity()
	if err != nil {
		return ciFail("encode continuation: " + err.Error())
	}
	resp, err := p.installContinuation("system/inbox/validate-collect-keys-mutual", contEnt)
	o := observe(resp, err)
	if o.status >= 200 && o.status < 300 {
		return ciFail(fmt.Sprintf("MUST be rejected; got success status=%d (op silently accepted — spec violation)", o.status))
	}
	switch {
	case o.status == 400 && o.code == "invalid_transform_args":
		return ciPass("rejected 400 invalid_transform_args (v1.15 §2.2 pinned)")
	case o.status == 400 && o.code == "unknown_transform_op":
		return ciFail("rejected fail-closed but with PRE-RATIFICATION code `unknown_transform_op`; v1.15 §2.2 pinned the rename to `invalid_transform_args`")
	case o.status == 400:
		return ciFail(fmt.Sprintf("rejected 400 but error code=%q; v1.15 §2.2 pins `invalid_transform_args` (msg=%q)", o.code, o.msg))
	default:
		return ciFail(fmt.Sprintf("expected 400, got status=%d code=%q (msg=%q)", o.status, o.code, o.msg))
	}
}

func (p *crossImplProbe) checkCollectKeysSingularInstall() crossImplResult {
	contData := types.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/tree", p.remote),
		Operation: "extract",
		Resource:  &types.ResourceTarget{Targets: []string{"any/"}},
		ResultTransform: &types.ContinuationTransformData{
			TransformOps: []types.ContinuationTransformOpData{
				{Op: "collect_keys", Field: "added", Into: "paths"},
			},
		},
	}
	contEnt, err := contData.ToEntity()
	if err != nil {
		return ciFail("encode continuation: " + err.Error())
	}
	resp, err := p.installContinuation("system/inbox/validate-collect-keys-singular", contEnt)
	o := observe(resp, err)
	if o.status >= 200 && o.status < 300 {
		return ciPass(fmt.Sprintf("install accepted (status=%d) — collect_keys singular form recognized", o.status))
	}
	if o.status == 400 && o.code == "unknown_transform_op" {
		return ciFail("install rejected with `unknown_transform_op` — peer does NOT yet recognize `collect_keys` (v1.15 §2.2 unimplemented)")
	}
	return ciFail(fmt.Sprintf("install rejected: status=%d code=%q (msg=%q)", o.status, o.code, o.msg))
}

func (p *crossImplProbe) checkCollectKeysPluralInstall() crossImplResult {
	contData := types.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/tree", p.remote),
		Operation: "extract",
		Resource:  &types.ResourceTarget{Targets: []string{"any/"}},
		ResultTransform: &types.ContinuationTransformData{
			TransformOps: []types.ContinuationTransformOpData{
				{Op: "collect_keys", Fields: []string{"added", "changed"}, Into: "paths"},
			},
		},
	}
	contEnt, err := contData.ToEntity()
	if err != nil {
		return ciFail("encode continuation: " + err.Error())
	}
	resp, err := p.installContinuation("system/inbox/validate-collect-keys-plural", contEnt)
	o := observe(resp, err)
	if o.status >= 200 && o.status < 300 {
		return ciPass(fmt.Sprintf("install accepted (status=%d) — collect_keys plural form recognized", o.status))
	}
	if o.status == 400 && o.code == "unknown_transform_op" {
		return ciFail("install rejected with `unknown_transform_op` — peer does NOT yet recognize `collect_keys` (v1.15 §2.2 unimplemented)")
	}
	return ciFail(fmt.Sprintf("install rejected: status=%d code=%q (msg=%q)", o.status, o.code, o.msg))
}

// --- revision:fetch-diff probes (REVISION v3.4 §4.4.19) ---
//
// Shape B: params {prefix, base}; target = executing peer's local head
// (implicit). Single-dynamic-field shape is the one chain-expressible
// variant. Errors:
//   - 400 invalid_params      — prefix missing/undecodable
//   - 403 capability_denied
//   - 404 no_local_state      — no revision head bound for prefix
//   - 404 base_not_found      — base hash not in local content store
//   - 400 base_not_a_version  — base resolves but isn't a version entry

func (p *crossImplProbe) checkFetchDiffZeroBase() crossImplResult {
	// Stand up a revision-tracked prefix with one committed head, then
	// fetch-diff with zero base — should return the full closure as a
	// snapshot envelope. Proves the op is registered & dispatchable.
	if _, err := p.remoteSetupPrefix("fd-zero-base/", "v1"); err != nil {
		return ciSkip("setup prefix failed: " + err.Error())
	}
	req := types.RevisionFetchDiffParamsData{
		Prefix: "fd-zero-base/",
		// Base intentionally zero.
	}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "fetch-diff", ent))
	if o.status < 200 || o.status >= 300 {
		if o.status == 400 && (o.code == "invalid_params" || o.code == "unknown_operation") {
			return ciFail(fmt.Sprintf("peer does NOT recognize `revision:fetch-diff` (v3.4 §4.4.19 unimplemented): status=%d code=%q msg=%q",
				o.status, o.code, o.msg))
		}
		return ciFail(fmt.Sprintf("fetch-diff(base=zero) failed: status=%d code=%q msg=%q",
			o.status, o.code, o.msg))
	}
	respEnt := o.resp.Entity()
	if !isEnvelopeType(respEnt.Type) {
		return ciFail(fmt.Sprintf("expected envelope, got type=%q", respEnt.Type))
	}
	var env entity.Envelope
	if err := cbor.Unmarshal(respEnt.Data, &env); err != nil {
		return ciFail("decode envelope: " + err.Error())
	}
	if env.Root.Type != types.TypeTreeSnapshot {
		return ciFail(fmt.Sprintf("envelope root not snapshot; got %q", env.Root.Type))
	}
	return ciPass(fmt.Sprintf("envelope OK with %d included entities — op recognized & returning full closure",
		len(env.Included)))
}

func (p *crossImplProbe) checkFetchDiffIncremental() crossImplResult {
	// V1: commit one leaf, capture V1 head VERSION hash.
	if _, err := p.remoteSetupPrefix("fd-incremental/", "v1-a"); err != nil {
		return ciSkip("setup V1 failed: " + err.Error())
	}
	v1Head, err := p.remoteHeadVersion("fd-incremental/")
	if err != nil {
		return ciSkip("read V1 head failed: " + err.Error())
	}
	// V2: add another leaf, commit, advance the head.
	if err := p.remotePut("fd-incremental/leaf-b", "v1-b"); err != nil {
		return ciSkip("setup put leaf-b: " + err.Error())
	}
	if err := p.remoteCommit("fd-incremental/"); err != nil {
		return ciSkip("setup commit-V2 failed: " + err.Error())
	}
	v2Head, err := p.remoteHeadVersion("fd-incremental/")
	if err != nil {
		return ciSkip("read V2 head failed: " + err.Error())
	}
	if v1Head == v2Head {
		return ciSkip("V1 head == V2 head; second commit did not advance the head")
	}

	// Incremental fetch (base=V1) vs full closure (base=zero).
	incReq := types.RevisionFetchDiffParamsData{Prefix: "fd-incremental/", Base: v1Head}
	incEnt, _ := incReq.ToEntity()
	incO := observe(p.executeOnRemote("system/revision", "fetch-diff", incEnt))
	if incO.status < 200 || incO.status >= 300 {
		return ciFail(fmt.Sprintf("incremental fetch-diff failed: status=%d code=%q msg=%q",
			incO.status, incO.code, incO.msg))
	}
	fullReq := types.RevisionFetchDiffParamsData{Prefix: "fd-incremental/"}
	fullEnt, _ := fullReq.ToEntity()
	fullO := observe(p.executeOnRemote("system/revision", "fetch-diff", fullEnt))
	if fullO.status < 200 || fullO.status >= 300 {
		return ciPass(fmt.Sprintf("incremental envelope OK (couldn't measure full closure: status=%d code=%q)",
			fullO.status, fullO.code))
	}
	var incEnv, fullEnv entity.Envelope
	if err := cbor.Unmarshal(incO.resp.Entity().Data, &incEnv); err != nil {
		return ciFail("decode incremental envelope: " + err.Error())
	}
	if err := cbor.Unmarshal(fullO.resp.Entity().Data, &fullEnv); err != nil {
		return ciFail("decode full envelope: " + err.Error())
	}
	if len(incEnv.Included) >= len(fullEnv.Included) {
		return ciFail(fmt.Sprintf("incremental envelope (%d entities) NOT smaller than full (%d) — peer may be ignoring `base`",
			len(incEnv.Included), len(fullEnv.Included)))
	}
	return ciPass(fmt.Sprintf("incremental envelope smaller (%d vs full %d) — diff-closure transport observed",
		len(incEnv.Included), len(fullEnv.Included)))
}

func (p *crossImplProbe) checkFetchDiffBaseNotFound() crossImplResult {
	if _, err := p.remoteSetupPrefix("fd-base-nf/", "v1"); err != nil {
		return ciSkip("setup failed: " + err.Error())
	}
	req := types.RevisionFetchDiffParamsData{
		Prefix: "fd-base-nf/",
		Base:   bogusHash(),
	}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "fetch-diff", ent))
	if o.status == 404 && o.code == "base_not_found" {
		return ciPass("rejected 404 base_not_found (v3.4 §4.4.19)")
	}
	if o.status >= 200 && o.status < 300 {
		return ciFail(fmt.Sprintf("bogus base MUST be 404 base_not_found; got success status=%d — peer ignores `base`?",
			o.status))
	}
	return ciFail(fmt.Sprintf("expected 404 base_not_found; got status=%d code=%q msg=%q",
		o.status, o.code, o.msg))
}

func (p *crossImplProbe) checkFetchDiffNoLocalState() crossImplResult {
	// Prefix that has never been committed → no head pointer.
	req := types.RevisionFetchDiffParamsData{Prefix: "fd-no-state/"}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "fetch-diff", ent))
	if o.status == 404 && o.code == "no_local_state" {
		return ciPass("rejected 404 no_local_state (v3.4 §4.4.19)")
	}
	if o.status >= 200 && o.status < 300 {
		return ciFail(fmt.Sprintf("no-head prefix MUST be 404 no_local_state; got success status=%d",
			o.status))
	}
	return ciFail(fmt.Sprintf("expected 404 no_local_state; got status=%d code=%q msg=%q",
		o.status, o.code, o.msg))
}

// --- revision:pull probes (REVISION §4.4.8) ---
//
// `pull` is the spec'd convenience composition: fetch + incremental
// fetch-entities + merge folded into one handler op so the DAG-mirror
// recipe is chain-expressible. Input type is `system/revision/fetch-params`
// per the manifest (spec line 558), with `remote` identifying the peer
// to pull from. Output is `system/revision/merge-result`.
//
// The remote impl is conformant when:
//   - `system/revision:pull` is in its handler manifest (not 400
//     unknown_operation).
//   - Missing `remote` is rejected 400 invalid_params.
//
// Full end-to-end DAG-advancement validation (the remote peer actually
// fetches from us, walks our trie, and merges into its own DAG) is
// out of scope for these probes; it requires the remote impl to dial
// back into our peer, which makes the test brittle across topologies.
// The two install-style probes here are sufficient to detect
// op-registered conformance.

func (p *crossImplProbe) checkPullOpRecognized() crossImplResult {
	// Dispatch revision:pull at the remote. Pass remote=p.local.PeerID
	// so the remote's pull handler tries to pull FROM us. If the op
	// isn't implemented, we expect 400 unknown_operation. If it IS
	// implemented, we expect some non-unknown-op response (either
	// success if the remote could actually pull back, or a transport-
	// shaped error if it tries — both confirm op recognition).
	req := types.RevisionFetchParamsData{
		Prefix: "pull-recognize/",
		Remote: string(p.local.PeerID()),
	}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "pull", ent))
	if o.status == 400 && o.code == "unknown_operation" {
		return ciFail(fmt.Sprintf("peer does NOT recognize `revision:pull` (§4.4.8 unimplemented): status=%d code=%q msg=%q",
			o.status, o.code, o.msg))
	}
	// Any other outcome — success, 4xx capability/state error, 5xx
	// transport-shaped error from the reverse-dial attempt — confirms
	// the op is registered. Detailed conformance is covered by the
	// workbench's own direct-dispatch test
	// (`entitysdk/revision_pull_op_test.go`).
	return ciPass(fmt.Sprintf("op recognized — status=%d code=%q (full DAG flow not validated cross-impl; see workbench direct-dispatch test)",
		o.status, o.code))
}

func (p *crossImplProbe) checkPullMissingRemote() crossImplResult {
	// Pull with empty Remote — MUST be rejected 400 invalid_params.
	req := types.RevisionFetchParamsData{Prefix: "pull-no-remote/"}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "pull", ent))
	if o.status >= 200 && o.status < 300 {
		return ciFail(fmt.Sprintf("missing `remote` MUST be rejected; got success status=%d", o.status))
	}
	if o.status == 400 && o.code == "invalid_params" {
		return ciPass("rejected 400 invalid_params for missing remote (workbench Go impl convention)")
	}
	if o.status == 400 && o.code == "unknown_operation" {
		return ciFail("peer does NOT recognize `revision:pull` (§4.4.8 unimplemented)")
	}
	return ciFail(fmt.Sprintf("expected 400 invalid_params; got status=%d code=%q msg=%q",
		o.status, o.code, o.msg))
}

// --- result_merge probes (CONTINUATION v1.16 §2.1 + §3.2) ---

func (p *crossImplProbe) checkResultMergeInstall() crossImplResult {
	contData := types.ContinuationData{
		Target:      fmt.Sprintf("entity://%s/system/tree", p.remote),
		Operation:   "extract",
		Resource:    &types.ResourceTarget{Targets: []string{"any/"}},
		ResultMerge: true,
	}
	contEnt, err := contData.ToEntity()
	if err != nil {
		return ciFail("encode continuation: " + err.Error())
	}
	resp, err := p.installContinuation("system/inbox/validate-result-merge-install", contEnt)
	o := observe(resp, err)
	if o.status >= 200 && o.status < 300 {
		return ciPass(fmt.Sprintf("install accepted (status=%d) — result_merge field recognized", o.status))
	}
	if o.status == 400 && o.code == "invalid_continuation" {
		// Some impls may reject result_merge until they ship support — flag clearly.
		return ciFail(fmt.Sprintf("install rejected 400 invalid_continuation; peer may not yet recognize `result_merge` (v1.16 §2.1 unimplemented): msg=%q",
			o.msg))
	}
	return ciFail(fmt.Sprintf("install rejected: status=%d code=%q msg=%q",
		o.status, o.code, o.msg))
}

func (p *crossImplProbe) checkResultMergeMutex() crossImplResult {
	contData := types.ContinuationData{
		Target:      fmt.Sprintf("entity://%s/system/tree", p.remote),
		Operation:   "extract",
		Resource:    &types.ResourceTarget{Targets: []string{"any/"}},
		ResultMerge: true,
		ResultField: "paths",
	}
	contEnt, err := contData.ToEntity()
	if err != nil {
		return ciFail("encode continuation: " + err.Error())
	}
	resp, err := p.installContinuation("system/inbox/validate-result-merge-mutex", contEnt)
	o := observe(resp, err)
	if o.status >= 200 && o.status < 300 {
		return ciFail(fmt.Sprintf("result_merge + result_field MUST be rejected (v1.16 §3.2); got success status=%d", o.status))
	}
	if o.status == 400 && o.code == "invalid_continuation" {
		return ciPass("rejected 400 invalid_continuation (v1.16 §3.2 mutual exclusivity)")
	}
	return ciFail(fmt.Sprintf("expected 400 invalid_continuation; got status=%d code=%q msg=%q",
		o.status, o.code, o.msg))
}

// --- setup helpers ---
//
// All setup operations target the remote peer's own namespace. Reading
// trie roots goes via `tree:extract` (which returns an envelope with a
// snapshot root entity) — this avoids needing a cross-peer
// system/content:get path that not every impl exposes.

func (p *crossImplProbe) remotePut(path, body string) error {
	qualified := "/" + p.remote + "/" + path
	_, err := p.local.Put(qualified, "test/note", body)
	return err
}

// remoteCommit fires a revision:commit on the remote without trying to
// read back the trie root. Use remoteCurrentTrieRoot when the caller
// needs the root.
func (p *crossImplProbe) remoteCommit(prefix string) error {
	commitParams := types.RevisionCommitParamsData{
		Prefix: "/" + p.remote + "/" + prefix,
	}
	ent, _ := commitParams.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "commit", ent))
	if o.status < 200 || o.status >= 300 {
		return fmt.Errorf("commit status=%d code=%q msg=%q", o.status, o.code, o.msg)
	}
	return nil
}

// remoteCurrentTrieRoot reads the prefix's current trie root via a
// no-filter tree:extract — the envelope's snapshot root entity carries
// the trie root hash. Works regardless of whether the prefix is
// revision-tracked.
func (p *crossImplProbe) remoteCurrentTrieRoot(prefix string) (hash.Hash, error) {
	req := types.ExtractRequestData{Prefix: prefix}
	ent, _ := req.ToEntity()
	o := observe(p.executeOnRemote("system/tree", "extract", ent))
	if o.status < 200 || o.status >= 300 {
		return hash.Hash{}, fmt.Errorf("extract status=%d code=%q msg=%q", o.status, o.code, o.msg)
	}
	respEnt := o.resp.Entity()
	if !isEnvelopeType(respEnt.Type) {
		return hash.Hash{}, fmt.Errorf("unexpected response type %q", respEnt.Type)
	}
	var env entity.Envelope
	if err := cbor.Unmarshal(respEnt.Data, &env); err != nil {
		return hash.Hash{}, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Root.Type != types.TypeTreeSnapshot {
		return hash.Hash{}, fmt.Errorf("envelope root not snapshot; got %q", env.Root.Type)
	}
	snap, err := types.SnapshotDataFromEntity(env.Root)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return snap.Root, nil
}

// remoteSetupPrefix puts one leaf under prefix and commits to make the
// prefix revision-tracked (`revision:fetch-diff` requires a head
// pointer per §4.4.19). Returns the committed trie root for callers
// that care; ignore the return if you only need a tracked prefix.
func (p *crossImplProbe) remoteSetupPrefix(prefix, body string) (hash.Hash, error) {
	if err := p.remotePut(prefix+"leaf-a", body); err != nil {
		return hash.Hash{}, fmt.Errorf("put: %w", err)
	}
	if err := p.remoteCommit(prefix); err != nil {
		return hash.Hash{}, fmt.Errorf("commit: %w", err)
	}
	return p.remoteCurrentTrieRoot(prefix)
}

// remoteHeadVersion reads the prefix's current head VERSION hash via
// revision:status. fetch-diff's `base` parameter takes a version hash
// (not a trie root); callers chaining commits need this.
func (p *crossImplProbe) remoteHeadVersion(prefix string) (hash.Hash, error) {
	statusParams := types.RevisionStatusParamsData{
		Prefix: "/" + p.remote + "/" + prefix,
	}
	ent, _ := statusParams.ToEntity()
	o := observe(p.executeOnRemote("system/revision", "status", ent))
	if o.status < 200 || o.status >= 300 {
		return hash.Hash{}, fmt.Errorf("status status=%d code=%q msg=%q",
			o.status, o.code, o.msg)
	}
	statusData, err := types.RevisionStatusDataFromEntity(o.resp.Entity())
	if err != nil {
		return hash.Hash{}, fmt.Errorf("decode status: %w", err)
	}
	if statusData.Head.IsZero() {
		return hash.Hash{}, fmt.Errorf("status returned zero head for prefix %q", prefix)
	}
	return statusData.Head, nil
}

// installContinuation installs a continuation on the REMOTE peer at
// the given install path. The SDK's ContinuationClient handles the
// `entity://{remote}/system/continuation` routing + the "install" op.
// Returns a synthesized response so the surrounding probes can use
// observe() uniformly.
//
// Mints a dispatch_capability on the local peer scoped to the
// continuation's Target/Operation/Resource and attaches it before
// dispatch — required by EXTENSION-CONTINUATION §3.2 R1 install
// authorization. The cap is included in the install dispatch so the
// remote can resolve it without a separate content fetch.
func (p *crossImplProbe) installContinuation(path string, contEnt entity.Entity) (*entitysdk.Response, error) {
	// Decode the continuation to inject a dispatch cap if missing.
	contData, err := types.ContinuationDataFromEntity(contEnt)
	if err != nil {
		return nil, fmt.Errorf("decode continuation: %w", err)
	}
	if contData.DispatchCapability.IsZero() {
		// Mint a permissive cap (resource=*, operation=*) and attach.
		grants := []types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{"*"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		}}
		capEnt, mintErr := p.local.MintChainCapability(grants)
		if mintErr != nil {
			return nil, fmt.Errorf("mint dispatch cap: %w", mintErr)
		}
		contData.DispatchCapability = capEnt.ContentHash
		newEnt, encErr := contData.ToEntity()
		if encErr != nil {
			return nil, fmt.Errorf("re-encode continuation: %w", encErr)
		}
		contEnt = newEnt
	}
	// Bundle the cap + signature + identity so the remote can resolve
	// the dispatch_capability without a separate fetch.
	capEnt, ok := p.local.Store().GetByHash(contData.DispatchCapability)
	var included map[hash.Hash]entity.Entity
	if ok {
		if bundle, bErr := p.local.BundleCrossPeerChain(capEnt); bErr == nil {
			included = bundle
		}
	}
	_, err = p.local.ContinuationAt(p.remote).InstallWithIncluded(p.ctx, path, contEnt, included)
	if err != nil {
		return nil, err
	}
	return &entitysdk.Response{Status: 200}, nil
}

// --- diagnostic helpers ---

func errorCodeOf(resp *entitysdk.Response) string {
	if resp == nil {
		return ""
	}
	if resp.Type != types.TypeError {
		return ""
	}
	var ed types.ErrorData
	if err := ecf.Decode(resp.Data, &ed); err != nil {
		return ""
	}
	return ed.Code
}

// isEnvelopeType accepts either `system/envelope` (Go) or
// `system/protocol/envelope` (Rust) — both extend `core/envelope` per
// the core type registry; the cross-impl naming divergence is
// pre-existing and not the subject of this conformance probe.
func isEnvelopeType(t string) bool {
	return t == types.TypeEnvelope || t == "system/protocol/envelope"
}

func bogusHash() hash.Hash {
	h := hash.Hash{Algorithm: hash.AlgorithmSHA256}
	for i := range h.Digest {
		h.Digest[i] = 0xab
	}
	return h
}
