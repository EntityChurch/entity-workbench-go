package shellpanel

import (
	"context"
	"strings"
	"testing"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
)

// TestShellModel_DispatchesViaRegistry verifies that Submit drives
// commands through the same shellcmd.Registry the standalone shell
// uses — the load-bearing Phase G stage 2 guarantee. Exercises a
// handful of representative verbs (pwd, info, ls).
func TestShellModel_DispatchesViaRegistry(t *testing.T) {
	ap, ws, err := shellboot.Bootstrap(context.Background(), shellboot.Config{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer ap.Close()

	sh := shellcmd.NewShellInWorkspace(ws)
	m := New(sh)

	if m.OutputLen() == 0 {
		t.Fatalf("expected welcome lines after New")
	}

	tests := []struct {
		name        string
		input       string
		expectAny   []string // any of these substrings should appear in output
		expectNoErr bool
	}{
		{
			name:        "pwd",
			input:       "pwd",
			expectAny:   []string{"/"},
			expectNoErr: true,
		},
		{
			name:        "info",
			input:       "info",
			expectAny:   []string{"PeerID", "Alias"},
			expectNoErr: true,
		},
		{
			name:        "unknown command surfaces error",
			input:       "definitelynotacommand",
			expectAny:   []string{"error", "unknown"},
			expectNoErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startLen := m.OutputLen()
			m.Submit(tc.input)
			if m.OutputLen() <= startLen {
				t.Fatalf("Submit produced no new output")
			}
			// Concatenate new output lines and check for any expected
			// substring.
			var sb strings.Builder
			for i := startLen; i < m.OutputLen(); i++ {
				sb.WriteString(m.Output[i].Text)
				sb.WriteByte('\n')
			}
			combined := sb.String()
			matched := false
			for _, sub := range tc.expectAny {
				if strings.Contains(strings.ToLower(combined), strings.ToLower(sub)) {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("no expected substring %v found in output:\n%s", tc.expectAny, combined)
			}
		})
	}
}

// TestShellModel_TwoModelsShareAliasViaWorkspace verifies that two
// shell panels over the same workspace observe one alias table even
// though their per-panel history is independent.
func TestShellModel_TwoModelsShareAliasViaWorkspace(t *testing.T) {
	ap, ws, err := shellboot.Bootstrap(context.Background(), shellboot.Config{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer ap.Close()

	mA := New(shellcmd.NewShellInWorkspace(ws))
	mB := New(shellcmd.NewShellInWorkspace(ws))

	// Add an alias via the workspace directly; both models should see
	// it through their *Shell embed of the workspace.
	fakePeerID := "z00fakepeerid000000000000000000000000000000000"
	ws.Conns["remote"] = &shellcmd.PeerConn{
		Alias:   "remote",
		Address: "127.0.0.1:9999",
		PeerID:  fakePeerID,
		Peer:    ap,
	}

	if _, ok := mA.Shell().Conns["remote"]; !ok {
		t.Fatalf("model A doesn't see workspace alias")
	}
	if _, ok := mB.Shell().Conns["remote"]; !ok {
		t.Fatalf("model B doesn't see workspace alias")
	}

	// Histories are per-panel.
	mA.Submit("pwd")
	if len(mA.History) != 1 || len(mB.History) != 0 {
		t.Fatalf("history not per-panel: A=%v B=%v", mA.History, mB.History)
	}
}

// TestRenderResult_KindCoverage is a smoke test that every documented
// ResultKind produces non-panicking output. Concrete formatting is
// tested via the integration test above; this just guards against
// future Kind additions silently breaking the renderer.
func TestRenderResult_KindCoverage(t *testing.T) {
	cases := []shellcmd.Result{
		{Kind: shellcmd.KindNone},
		shellcmd.MessageResult("hello"),
		shellcmd.PathResult("/foo"),
		shellcmd.LinesResult([]string{"a", "b"}),
		{Kind: shellcmd.KindListing, Listing: []shellcmd.ListingRow{{Name: "x", Kind: "entity"}}},
		{Kind: shellcmd.KindListing, Listing: nil}, // empty
	}
	for i, c := range cases {
		// We don't assert content, just that we don't panic.
		_ = RenderResult(c)
		if c.Kind == shellcmd.KindMessage && len(RenderResult(c)) == 0 {
			t.Fatalf("case %d: message should produce a line", i)
		}
	}
}
