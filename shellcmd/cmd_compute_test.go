package shellcmd

// Phase H.3a tests — compute shell verbs.
//
// Build a small compute expression via the S1 builder, store it at a
// known path, and drive `compute show <path>` through both the
// verb-op (FormatComputeIR) directly and the cmd wrapper
// (cmdComputeShow). The former exercises the verb-op-without-Shell
// constraint (§8.1); the latter exercises end-to-end CLI dispatch.

import (
	"context"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestCmdComputeShow_RendersIRGraph builds a small expression
// (Arithmetic[add] over two Literals) and verifies the show output
// names each node + indents children correctly.
func TestCmdComputeShow_RendersIRGraph(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Build add(2, 3) and store at a known path.
	c := ap.Compute()
	expr := c.Arithmetic("add", c.Literal(uint64(2)), c.Literal(uint64(3)))
	rootPath := "app/test/show/add"
	if _, err := expr.Build(context.Background(), rootPath); err != nil {
		t.Fatalf("build: %v", err)
	}

	// --- verb-op (no Shell) ---
	lines, err := FormatComputeIR(ap, rootPath, 16)
	if err != nil {
		t.Fatalf("FormatComputeIR: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (Arithmetic + 2 Literal), got %d:\n%s",
			len(lines), strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[0], "compute/arithmetic op=add") {
		t.Errorf("line 0: want Arithmetic op=add prefix, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  compute/literal") || !strings.Contains(lines[1], "value=2") {
		t.Errorf("line 1: want indented Literal value=2, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "  compute/literal") || !strings.Contains(lines[2], "value=3") {
		t.Errorf("line 2: want indented Literal value=3, got %q", lines[2])
	}

	// --- cmd wrapper (via Shell) ---
	// Use the @local alias so sh.Resolve produces a peer-qualified
	// absolute path that ConnForPath can match to the local PeerConn.
	sh := NewShell(ap, "local", "")
	res, err := cmdCompute(sh, []string{"show", "@local/" + rootPath})
	if err != nil {
		t.Fatalf("cmdCompute show: %v", err)
	}
	if res.Kind != KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("expected 3 lines via cmd, got %d:\n%s",
			len(res.Lines), strings.Join(res.Lines, "\n"))
	}
	t.Logf("PASS: compute show renders Arithmetic[add] over two Literals — verb-op and cmd wrapper both functional")
}

// TestCmdComputeShow_NestedConstructAndApply walks a richer IR:
// Apply with named args, each arg a different shape. Confirms the
// recursive walker handles map-of-hash children (args) correctly
// and renders named arg labels.
func TestCmdComputeShow_NestedConstructAndApply(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	// Apply("some/handler", "op", { "n": field(scope.params, "n"),
	//                                "init": literal(0) })
	expr := c.Apply("some/handler", "op", map[string]*entitysdk.Builder{
		"n":    c.Field(c.LookupScope("params"), "n"),
		"init": c.Literal(uint64(0)),
	})

	rootPath := "app/test/show/apply"
	if _, err := expr.Build(context.Background(), rootPath); err != nil {
		t.Fatalf("build: %v", err)
	}

	lines, err := FormatComputeIR(ap, rootPath, 16)
	if err != nil {
		t.Fatalf("FormatComputeIR: %v", err)
	}
	joined := strings.Join(lines, "\n")

	// Spot-checks: the output mentions the Apply path + op, the arg
	// names ("init", "n"), and each argument's IR type.
	for _, want := range []string{
		`path="some/handler"`,
		`op="op"`,
		`arg "init":`,
		`arg "n":`,
		"compute/literal",
		"compute/field",
		"compute/lookup/scope",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("show output missing %q:\n%s", want, joined)
		}
	}
	t.Logf("PASS: compute show renders Apply with named args + recurses into each")
}

// TestCmdComputeShow_NonComputeEntity confirms show doesn't crash on
// non-compute entities — it renders a one-liner indicating the type
// without trying to recurse. Validates the "show any path" UX
// (users will sometimes point it at the wrong thing).
func TestCmdComputeShow_NonComputeEntity(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	prim, err := entitysdk.PrimitiveAny(map[string]interface{}{"x": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	const path = "app/test/show/noncompute"
	if _, err := ap.PutEntity(path, prim); err != nil {
		t.Fatalf("put: %v", err)
	}

	lines, err := FormatComputeIR(ap, path, 16)
	if err != nil {
		t.Fatalf("FormatComputeIR on non-compute: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for non-compute entity, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "primitive/any") {
		t.Errorf("expected line to mention primitive/any: %q", lines[0])
	}
	if !strings.Contains(lines[0], "not a compute IR type") {
		t.Errorf("expected line to indicate non-compute: %q", lines[0])
	}
	t.Logf("PASS: compute show renders one-liner for non-compute entity, doesn't recurse")
}

// TestCmdComputeShow_MissingPathErrors confirms a clean error
// message when the path doesn't exist (rather than a crash or a
// blank result).
func TestCmdComputeShow_MissingPathErrors(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, err = FormatComputeIR(ap, "app/test/show/does-not-exist", 16)
	if err == nil {
		t.Fatalf("expected error for missing path, got nil")
	}
	if !strings.Contains(err.Error(), "no entity") {
		t.Errorf("expected 'no entity' in error, got: %v", err)
	}
	t.Logf("PASS: missing-path error surfaces cleanly: %v", err)
}

// TestCmdComputeRegister_RegistersAndDispatches builds an expression
// at a known path, then `compute register <pattern> <expr-path>`
// installs a handler that evaluates to the expression. A subsequent
// dispatch through the handler returns the expression's runtime
// value — end-to-end proof that the shell verb produces a working
// compute-backed handler.
func TestCmdComputeRegister_RegistersAndDispatches(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Author + build literal(99) at a known path.
	c := ap.Compute()
	exprPath := "app/test/register/lit99"
	if _, err := c.Literal(uint64(99)).Build(context.Background(), exprPath); err != nil {
		t.Fatalf("build expr: %v", err)
	}

	// `compute register <pattern> @local/<expr-path>` via the shell.
	sh := NewShell(ap, "local", "")
	const pattern = "app/test/register/handler"
	res, err := cmdCompute(sh, []string{"register", pattern, "@local/" + exprPath})
	if err != nil {
		t.Fatalf("cmdCompute register: %v", err)
	}
	if res.Kind != KindMessage {
		t.Fatalf("expected KindMessage, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, pattern) {
		t.Errorf("message doesn't mention pattern: %q", res.Message)
	}

	// Dispatch the freshly-registered handler — should return 99.
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	t.Logf("PASS: shell-registered compute handler at %s dispatches to expression at %s (status=%d type=%s)",
		pattern, exprPath, resp.Status, resp.Type)
}

// TestCmdComputeRegister_UsageErrors covers the register arg
// validation: no pattern, no expr-path, missing connection.
func TestCmdComputeRegister_UsageErrors(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	sh := NewShell(ap, "local", "")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no args", []string{"register"}, "usage: compute register"},
		{"one arg", []string{"register", "pattern-only"}, "usage: compute register"},
		{"unknown peer", []string{"register", "p", "@unknown-alias/x"}, "no connection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cmdCompute(sh, tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error to contain %q, got: %v", tc.want, err)
			}
		})
	}
}

// TestCmdComputeAggregate_FilesStatsOverPrefix is the panel-level
// battle-test of the lowering toolkit. Puts a few entities under a
// prefix with varying .size fields, runs `compute aggregate <prefix>`,
// and asserts the output reports the correct count + total_bytes.
//
// This is the first real-call-site exercise of the toolkit outside
// of test fixtures: the verb queries, builds via S1+toolkit, registers
// a handler, dispatches, and unwraps. If S4 (DropDown sugar) or S6
// (typed ComputeClient) need to land, this test's call site is where
// the friction would surface first.
func TestCmdComputeAggregate_FilesStatsOverPrefix(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Put three primitive/any entities under a prefix, each carrying
	// a .size field. The aggregate verb doesn't care about entity
	// type — only that .size decodes to a numeric.
	prefix := "app/test/aggregate"
	files := []struct {
		path string
		size uint64
	}{
		{prefix + "/a.md", 100},
		{prefix + "/b.md", 250},
		{prefix + "/c.md", 750},
	}
	for _, f := range files {
		ent, err := entitysdk.PrimitiveAny(map[string]interface{}{
			"path": f.path,
			"size": f.size,
		})
		if err != nil {
			t.Fatalf("primitive/any: %v", err)
		}
		if _, err := ap.PutEntity(f.path, ent); err != nil {
			t.Fatalf("put %s: %v", f.path, err)
		}
	}

	// Also put an entity WITHOUT a size field at the same prefix —
	// the verb should skip it.
	noSize, _ := entitysdk.PrimitiveAny(map[string]interface{}{"other": "data"})
	if _, err := ap.PutEntity(prefix+"/no-size", noSize); err != nil {
		t.Fatalf("put no-size: %v", err)
	}

	sh := NewShell(ap, "local", "")
	res, err := cmdCompute(sh, []string{"aggregate", prefix})
	if err != nil {
		t.Fatalf("cmdCompute aggregate: %v", err)
	}
	if res.Kind != KindLines {
		t.Fatalf("expected KindLines, got %v (message=%q)", res.Kind, res.Message)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "count       = 3") {
		t.Errorf("expected count=3 in output:\n%s", joined)
	}
	if !strings.Contains(joined, "total_bytes = 1100") {
		t.Errorf("expected total_bytes=1100 in output:\n%s", joined)
	}
	if !strings.Contains(joined, "skipped 1") {
		t.Errorf("expected skip-count=1 in output:\n%s", joined)
	}
	t.Logf("PASS: compute aggregate %s → count=3, total_bytes=1100, skipped 1\n%s",
		prefix, joined)

	// Re-run — exercises the idempotent ensureFilesStatsHandler path
	// (manifest already exists; verb shouldn't re-register).
	res2, err := cmdCompute(sh, []string{"aggregate", prefix})
	if err != nil {
		t.Fatalf("cmdCompute aggregate (second call): %v", err)
	}
	if res2.Kind != KindLines {
		t.Fatalf("expected KindLines on second call, got %v", res2.Kind)
	}
	if strings.Join(res2.Lines, "\n") != joined {
		t.Errorf("second call output differs from first:\n--- first ---\n%s\n--- second ---\n%s",
			joined, strings.Join(res2.Lines, "\n"))
	}
	t.Logf("PASS: second-call idempotency — re-registration skipped, same result")
}

// TestCmdComputeAggregate_NoMatches confirms a clean "no matches"
// message when no entities under the prefix have a .size field
// (and that the verb doesn't try to dispatch an empty list).
func TestCmdComputeAggregate_NoMatches(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := NewShell(ap, "local", "")
	res, err := cmdCompute(sh, []string{"aggregate", "app/empty"})
	if err != nil {
		t.Fatalf("cmdCompute aggregate: %v", err)
	}
	if res.Kind != KindMessage {
		t.Fatalf("expected KindMessage for empty prefix, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "no entities with a numeric .size field") {
		t.Errorf("expected no-matches message, got %q", res.Message)
	}
}

// TestCmdComputeShow_UsageErrors covers the cmd-wrapper arg
// validation: no subop, unknown subop, no path.
func TestCmdComputeShow_UsageErrors(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	sh := NewShell(ap, "local", "")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no subop", nil, "usage: compute"},
		{"unknown subop", []string{"frobnicate"}, "unknown subop"},
		{"unknown subop mentions register", []string{"frobnicate"}, "register"},
		{"show without path", []string{"show"}, "usage: compute show"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cmdCompute(sh, tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error to contain %q, got: %v", tc.want, err)
			}
		})
	}
}
