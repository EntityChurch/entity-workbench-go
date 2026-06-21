package shellcmd_test

// Tests for the `inspect` command set — snapshot queries against
// substrate state. Validates entity / dump / find / errors / chain
// against a live local peer.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestInspect_Entity: put an entity, then `inspect entity <path>` round-trips
// the decoded body.
func TestInspect_Entity(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	if _, err := ap.Put("inspect-test/marker", "test/note",
		map[string]interface{}{"label": "hello"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"entity", "inspect-test/marker"})
	if err != nil {
		t.Fatalf("inspect entity: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "type:  test/note") {
		t.Errorf("missing type line; got:\n%s", joined)
	}
	if !strings.Contains(joined, "hello") {
		t.Errorf("decoded data should contain 'hello'; got:\n%s", joined)
	}
}

// TestInspect_EntityNotFound: missing path returns KindMessage, not error.
func TestInspect_EntityNotFound(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"entity", "missing/path"})
	if err != nil {
		t.Fatalf("inspect entity: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("expected KindMessage for missing path, got %v (%v)", res.Kind, res)
	}
	if !strings.Contains(res.Message, "no binding") {
		t.Errorf("expected 'no binding' message, got %q", res.Message)
	}
}

// TestInspect_Find: bind several paths under a prefix, then verify
// `inspect find` returns them.
func TestInspect_Find(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	for i := 0; i < 3; i++ {
		p := fmt.Sprintf("find-test/item-%d", i)
		if _, err := ap.Put(p, "test/marker", map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"find", "find-test"})
	if err != nil {
		t.Fatalf("inspect find: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	matches := 0
	for _, l := range res.Lines {
		if strings.Contains(l, "find-test/item-") {
			matches++
		}
	}
	if matches != 3 {
		t.Errorf("expected 3 find-test/item-* matches, got %d (lines=%v)", matches, res.Lines)
	}
}

// TestInspect_Errors_Empty: no errors → KindMessage.
func TestInspect_Errors_Empty(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"errors"})
	if err != nil {
		t.Fatalf("inspect errors: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("expected KindMessage when no errors, got %v", res.Kind)
	}
}

// TestInspect_ChainLs_Empty: clean peer → KindMessage with honest
// scope note about installed-continuation invisibility.
func TestInspect_ChainLs_Empty(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"chain", "ls"})
	if err != nil {
		t.Fatalf("inspect chain ls: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("expected KindMessage on empty peer, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "continuation ls") {
		t.Errorf("empty-message should point at `continuation ls` for installed chains; got %q", res.Message)
	}
}

// TestInspect_ChainLs_ListsFailedChain: after §4.7 marker fires,
// `inspect chain ls` should list it with status=failed.
func TestInspect_ChainLs_ListsFailedChain(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	rateLimit := uint64(60)
	sub, err := ap.SubscribeAt(ap.PeerID(), "ls-test/*", entitysdk.SubscribeOpts{
		Limits: &coretypes.SubscriptionLimitsData{RateLimit: &rateLimit},
	})
	if err != nil {
		t.Fatalf("SubscribeAt: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	for i := 0; i < 3; i++ {
		if _, err := ap.Put(fmt.Sprintf("ls-test/note-%d", i), "test/note",
			map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// Poll for marker — engine binds async.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		res, _ := reg.Dispatch(sh, "inspect", []string{"chain", "ls"})
		if res.Kind == shellcmd.KindLines {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	res, err := reg.Dispatch(sh, "inspect", []string{"chain", "ls"})
	if err != nil {
		t.Fatalf("inspect chain ls: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines after marker bound, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "failed") {
		t.Errorf("expected status=failed row, got:\n%s", joined)
	}
	if !strings.Contains(joined, "rate_limited") {
		t.Errorf("expected rate_limited last_reason, got:\n%s", joined)
	}
	if !strings.Contains(joined, "none") {
		t.Errorf("expected chain_id 'none' (fallback) row, got:\n%s", joined)
	}
}

// TestInspect_ChainShow_BackwardCompat: `inspect chain <id>` (id
// not matching a subverb) should still route to the show handler
// per backward-compat shim.
func TestInspect_ChainShow_BackwardCompat(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// Empty store: `inspect chain nonexistent` should produce the
	// no-artifacts §9 #8 honesty surface.
	res, err := reg.Dispatch(sh, "inspect", []string{"chain", "nonexistent-chain-id"})
	if err != nil {
		t.Fatalf("inspect chain <id>: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "NO ARTIFACTS FOUND") {
		t.Errorf("expected v1.1 §9 #8 honesty surface, got:\n%s", joined)
	}

	// Explicit `inspect chain show <id>` should produce the same result.
	resShow, err := reg.Dispatch(sh, "inspect", []string{"chain", "show", "nonexistent-chain-id"})
	if err != nil {
		t.Fatalf("inspect chain show: %v", err)
	}
	if resShow.Kind != res.Kind {
		t.Errorf("inspect chain show kind mismatch: %v vs %v", resShow.Kind, res.Kind)
	}
}

// TestInspect_ChainFor_Marker: put a synthetic chain-error marker
// at the v1.20 path layout; `inspect chain for <marker_hash>` returns
// the encoded chain_id.
func TestInspect_ChainFor_Marker(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	const chainID = "shell-chain-for-001"
	body := coretypes.ChainErrorLostData{
		Reason:    "not_found",
		ChainID:   chainID,
		StepIndex: "7",
		Timestamp: uint64(time.Now().UnixMicro()),
	}
	ent, err := body.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	markerPath := "system/runtime/chain-errors/lost/" + chainID + "/7/not_found/" + ent.ContentHash.String()
	if _, err := ap.PutEntity(markerPath, ent); err != nil {
		t.Fatalf("PutEntity: %v", err)
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{"chain", "for", ent.ContentHash.String()})
	if err != nil {
		t.Fatalf("inspect chain for: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, chainID) {
		t.Errorf("expected chain_id %q in output; got:\n%s", chainID, joined)
	}
}

// TestInspect_ChainFor_NotFound: unknown hash returns the
// "no chain attribution" message, not an error.
func TestInspect_ChainFor_NotFound(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// 66-char hex matching ECF format: 00 + 64 zero-bytes
	zeroHash := "00" + strings.Repeat("0", 64)
	res, err := reg.Dispatch(sh, "inspect", []string{"chain", "for", zeroHash})
	if err != nil {
		t.Fatalf("inspect chain for: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("expected KindMessage for unknown hash, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "no chain attribution") {
		t.Errorf("expected 'no chain attribution' message, got %q", res.Message)
	}
}

// TestInspect_Watch_Errors_DecodesMarker: trigger a chain-error
// marker bind during a short watch window and verify the verb
// decodes reason/chain_id from the marker body in its output.
func TestInspect_Watch_Errors_DecodesMarker(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	const chainID = "watch-marker-001"
	body := coretypes.ChainErrorLostData{
		Reason:    "rate_limited",
		ChainID:   chainID,
		StepIndex: "4",
		Timestamp: uint64(time.Now().UnixMicro()),
	}
	ent, err := body.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	markerPath := "system/runtime/chain-errors/lost/" + chainID + "/4/rate_limited/" + ent.ContentHash.String()

	// Bind the marker concurrently with the watch — sleep briefly so
	// the watch subscription is registered before the bind, then put.
	done := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = ap.PutEntity(markerPath, ent)
		close(done)
	}()

	res, err := reg.Dispatch(sh, "inspect", []string{
		"watch", "errors", "-n", "1", "-timeout", "5s",
	})
	if err != nil {
		t.Fatalf("inspect watch errors: %v", err)
	}
	<-done
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "chain="+chainID) {
		t.Errorf("expected decoded chain=%s in output; got:\n%s", chainID, joined)
	}
	if !strings.Contains(joined, "reason=rate_limited") {
		t.Errorf("expected decoded reason=rate_limited; got:\n%s", joined)
	}
}

// TestInspect_Watch_TimeoutQuiet: no activity → message with timeout,
// not error.
func TestInspect_Watch_TimeoutQuiet(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "inspect", []string{
		"watch", "errors", "-n", "5", "-timeout", "200ms",
	})
	if err != nil {
		t.Fatalf("inspect watch: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("expected KindMessage on quiet timeout, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "no events") {
		t.Errorf("expected 'no events' message, got %q", res.Message)
	}
}

// TestInspect_ChainErrors_Alias: `inspect chain errors` should
// equal `inspect errors`.
func TestInspect_ChainErrors_Alias(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	resTopLevel, err := reg.Dispatch(sh, "inspect", []string{"errors"})
	if err != nil {
		t.Fatalf("inspect errors: %v", err)
	}
	resNested, err := reg.Dispatch(sh, "inspect", []string{"chain", "errors"})
	if err != nil {
		t.Fatalf("inspect chain errors: %v", err)
	}
	if resTopLevel.Kind != resNested.Kind {
		t.Errorf("alias kind mismatch: %v vs %v", resTopLevel.Kind, resNested.Kind)
	}
}

// TestInspect_ChainDispatch_NoOp: dispatch through a verb that
// doesn't trigger any chain. Should report 'NO new chain_ids'
// cleanly and surface either a dispatch summary line or an error
// line — the verb composes gracefully either way.
func TestInspect_ChainDispatch_NoOp(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()
	sh.SetWD(shellcmd.Path("/" + ap.PeerID() + "/"))

	// tree:put with no subscriptions watching → no chain emission.
	res, err := reg.Dispatch(sh, "inspect", []string{
		"chain", "dispatch",
		"system/tree", "put",
		"quiet/x",
		`{"v":1}`,
		"-wait", "100ms",
	})
	if err != nil {
		t.Fatalf("inspect chain dispatch: %v", err)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "NO new chain_ids") {
		t.Errorf("expected NO new chain_ids for no-op dispatch, got:\n%s", joined)
	}
}

// TestInspect_Chain_RateLimitedMarker: trigger a §4.7 rate_limited
// marker on a self-subscribed peer, then verify `inspect chain` and
// `inspect errors` both surface it.
func TestInspect_Chain_RateLimitedMarker(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	rateLimit := uint64(60)
	sub, err := ap.SubscribeAt(ap.PeerID(), "shell-rl/*", entitysdk.SubscribeOpts{
		Limits: &coretypes.SubscriptionLimitsData{RateLimit: &rateLimit},
	})
	if err != nil {
		t.Fatalf("SubscribeAt: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	for i := 0; i < 3; i++ {
		if _, err := ap.Put(fmt.Sprintf("shell-rl/note-%d", i), "test/note",
			map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Allow engine to bind the marker.
	deadline := time.Now().Add(2 * time.Second)
	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()
	for time.Now().Before(deadline) {
		res, _ := reg.Dispatch(sh, "inspect", []string{"errors"})
		if res.Kind == shellcmd.KindLines {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Errors surface.
	res, err := reg.Dispatch(sh, "inspect", []string{"errors"})
	if err != nil {
		t.Fatalf("inspect errors: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines once marker bound, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "rate_limited") {
		t.Errorf("inspect errors did not surface rate_limited reason; got:\n%s", joined)
	}

	// `inspect chain none` walks the fallback chain_id used by §4.7
	// when no upstream causality.
	res, err = reg.Dispatch(sh, "inspect", []string{"chain", "none"})
	if err != nil {
		t.Fatalf("inspect chain none: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined = strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "rate_limited") {
		t.Errorf("inspect chain none did not surface rate_limited error; got:\n%s", joined)
	}
	if !strings.Contains(joined, "errors=") {
		t.Errorf("chain trace summary missing errors count; got:\n%s", joined)
	}
}
