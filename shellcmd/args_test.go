package shellcmd

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace_only", "   \t  ", nil},
		{"single", "ls", []string{"ls"}},
		{"two", "ls /docs", []string{"ls", "/docs"}},
		{"double_quoted_glob", `mount /tmp docs -include "*.md"`, []string{"mount", "/tmp", "docs", "-include", "*.md"}},
		{"single_quoted_glob", `mount /tmp docs -include '*.md'`, []string{"mount", "/tmp", "docs", "-include", "*.md"}},
		{"quoted_with_spaces", `exec h op "a b c"`, []string{"exec", "h", "op", "a b c"}},
		{"quote_with_other_inside", `exec h op "it's fine"`, []string{"exec", "h", "op", "it's fine"}},
		{"comma_in_quotes", `mount /tmp docs -exclude "*.go,*.rs"`, []string{"mount", "/tmp", "docs", "-exclude", "*.go,*.rs"}},
		{"unbalanced_double", `oops "stuck`, []string{"oops", "stuck"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SplitArgs(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
