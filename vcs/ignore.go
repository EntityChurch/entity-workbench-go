package vcs

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Always-ignored top-level entries. .entity is the repo state; .git
// is the canonical case where vcs runs side-by-side with git (the
// dogfooding scenario where workbench-go's own tree is mirrored
// through entity-vcs while the canonical history stays in git).
var alwaysIgnore = []string{".entity", ".git"}

// LoadIgnore reads {repoDir}/.gitignore (.entityignore is deferred)
// and returns a matcher closing over repoDir.
//
// First-form semantics — intentionally lossy:
//   - blank lines and lines starting with # are skipped
//   - each non-blank line is treated as either an exact basename
//     match or a filepath.Match glob (whichever the pattern looks like)
//   - negation (!pattern), directory-only (pattern/), and the leading-
//     slash anchored forms of gitignore are NOT implemented
//
// "Read gitignore as a comparable" — per user direction. Real
// gitignore semantics land later if any test corpus surfaces a case
// that matters.
func LoadIgnore(repoDir string) func(relPath string) bool {
	patterns := append([]string(nil), alwaysIgnore...)
	patterns = append(patterns, readIgnoreFile(filepath.Join(repoDir, ".gitignore"))...)

	return func(rel string) bool {
		base := filepath.Base(rel)
		for _, p := range patterns {
			if p == base || p == rel {
				return true
			}
			if matched, _ := filepath.Match(p, base); matched {
				return true
			}
			// Prefix match: ".entity" should ignore ".entity/store.db".
			if strings.HasPrefix(rel, p+"/") {
				return true
			}
		}
		return false
	}
}

func readIgnoreFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip trailing slash; we don't distinguish file vs dir
		// patterns in first form.
		line = strings.TrimSuffix(line, "/")
		out = append(out, line)
	}
	return out
}
