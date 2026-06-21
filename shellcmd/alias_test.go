package shellcmd

import "testing"

func TestNormalizeAlias_AcceptsAscii(t *testing.T) {
	for _, name := range []string{"alice", "peer-a", "node_1", "x", "AB", "123"} {
		out, err := NormalizeAlias(name)
		if err != nil {
			t.Errorf("NormalizeAlias(%q) errored: %v", name, err)
			continue
		}
		if out != name {
			t.Errorf("NormalizeAlias(%q) = %q, want %q (ASCII passes through unchanged)", name, out, name)
		}
	}
}

func TestNormalizeAlias_AcceptsUnicode(t *testing.T) {
	// Per GUIDE-SHELL-FRAMING.md §6.1: any Unicode codepoint sequence
	// except structural-conflict chars is permitted.
	for _, name := range []string{"Алиса", "愛麗絲", "علي", "café", "naïve"} {
		out, err := NormalizeAlias(name)
		if err != nil {
			t.Errorf("NormalizeAlias(%q) errored: %v", name, err)
			continue
		}
		if out == "" {
			t.Errorf("NormalizeAlias(%q) returned empty string", name)
		}
	}
}

func TestNormalizeAlias_NFCEquivalence(t *testing.T) {
	// "café" with precomposed é (U+00E9) vs decomposed e (U+0065) +
	// combining acute (U+0301) should normalize to the same form.
	precomposed := "café"
	decomposed := "café"
	a, err := NormalizeAlias(precomposed)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeAlias(decomposed)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("NFC normalization failed: precomposed %q normalized to %q; decomposed %q normalized to %q",
			precomposed, a, decomposed, b)
	}
}

func TestNormalizeAlias_RejectsStructuralConflicts(t *testing.T) {
	cases := []struct {
		name string
		want string // substring expected in the error
	}{
		{"", "empty"},
		{"a@b", "@"},
		{"@alice", "@"},
		{"a/b", "/"},
		{"a:b", ":"},
		{"a b", "whitespace"},
		{"a\tb", "whitespace"},
		{"a\nb", "whitespace"},
		{"a\x00b", "control"},
		{"a\x7fb", "control"},
		{"a'b", "shell-escape"},
		{`a"b`, "shell-escape"},
		{`a\b`, "shell-escape"},
	}
	for _, tc := range cases {
		_, err := NormalizeAlias(tc.name)
		if err == nil {
			t.Errorf("NormalizeAlias(%q) = nil error, want error containing %q", tc.name, tc.want)
			continue
		}
		if !contains(err.Error(), tc.want) {
			t.Errorf("NormalizeAlias(%q) error = %q, want substring %q", tc.name, err.Error(), tc.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestIsReservedAlias(t *testing.T) {
	for _, name := range []string{"self", "primary", "system", "PRIMARY", "System"} {
		if !IsReservedAlias(name) {
			t.Errorf("IsReservedAlias(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"alice", "selfish", "peer-a", "primaries"} {
		if IsReservedAlias(name) {
			t.Errorf("IsReservedAlias(%q) = true, want false", name)
		}
	}
}
