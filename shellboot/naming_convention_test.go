package shellboot

// naming_convention_test.go — the gate for `STYLE-NAMING-CONVENTIONS`'s
// kebab rule, over SOURCE, which is the one place nobody was looking.
//
// # Why this exists at all
//
// The rule is landed and it is GATED: *"entity-type paths must be kebab
// (snake = violation)"*, §7. The gate that enforces it reads `specs/` and
// `guides/` — the specification corpus — and an implementation's entity-type
// names are string literals in `.go` and `.cs` files, which that gate cannot
// see and was never meant to. So a rule described as mechanically enforced was
// unenforced everywhere it is actually WRITTEN, in every implementation at
// once. Measured across the two application seats: 15 violations in one tree,
// 1 in this one (`A-44`), and neither seat could see its own.
//
// That is the transferable half and it is worth more than the rename: **a gate
// scoped to one corpus reads, to everybody downstream, as a gate on the rule.**
// Nobody re-asks whether the thing being scanned is the thing they are writing.
//
// # What it checks, and what it deliberately does not
//
// It scans every tracked `.go` / `.cs` source for string literals that are
// entity-namespace paths — a literal whose first segment is one of this
// cohort's namespace roots — and fails on `_` in any segment of one.
//
// It does NOT try to tell an entity TYPE from an instance PATH or a handler
// op. It does not have to: §3's split puts type paths, operation names and
// enum values all on the kebab axis, and instance paths in this tree follow
// the type names they address. The snake axis is data-structure KEYS, which
// are `cbor:"peer_id"` struct tags and JSON field names — neither of which can
// match this pattern, because neither contains a `/`. So the one-character
// discriminator that makes the whole naming split legible on sight is also
// what makes this scan precise: **a slash means you are on the namespace
// axis.**
//
// # Both anti-vacuity arms are load-bearing
//
// A source-walking test has two ways to pass while measuring nothing, and this
// tree has been bitten by the second one at least twice (AP96, AP108): it can
// walk the wrong directory and find no files, and its detector can be broken
// so that it flags nothing. Neither failure produces any output distinguishable
// from a clean tree. So `TestNamingGateCanFire` runs the detector over a
// synthetic source that contains a violation, and the sweep asserts a FLOOR on
// how many conformant literals it saw.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// namespaceRoots is the first-segment allowlist.
//
// Deliberately an allowlist and not "anything with a slash": the second form
// sweeps up file paths, URLs and doc references, and a gate with false
// positives gets waived rather than fixed.
var namespaceRoots = []string{"app", "system", "doc", "local", "compute", "archives", "workbench"}

// namespacePathLiteral matches a double-quoted string whose content is an
// entity-namespace path: a root from [namespaceRoots], then only characters
// legal in a path/op. A literal containing `%`, `.`, an uppercase letter or a
// space is not matched, which drops format strings, Go import paths, file
// paths and prose. Built from the allowlist so adding a root is one edit.
var namespacePathLiteral = regexp.MustCompile(
	`"(?:` + strings.Join(namespaceRoots, "|") + `)/[a-z0-9_/:-]+"`)

// selfExclusion is this file, and it is the one file the sweep does not read.
//
// [TestNamingGateCanFire]'s fixture has to CONTAIN a violation in order to
// prove the detector fires, so a gate that reads its own source fails on its
// own evidence — which is a real result and the wrong one. The cost is stated
// rather than hidden: a genuine violation in this file is invisible to the
// sweep. It is one file, it writes no entity, and the alternative — assembling
// the fixture by string concatenation so the literal never appears — buys the
// coverage back by making the fixture unreadable as the thing it is a fixture
// for.
const selfExclusion = "naming_convention_test.go"

// minLiteralsScanned is the vacuity floor. The tree held 474 distinct
// namespace-path literals when this was written; a walker that has lost the
// repo root, or an extension filter that stopped matching, returns 0 and would
// otherwise pass silently.
const minLiteralsScanned = 200

type namingViolation struct {
	file    string
	line    int
	literal string
	segment string
}

func TestEntityTypeLiteralsAreKebabCase(t *testing.T) {
	root := repoRoot(t)

	var (
		violations []namingViolation
		distinct   = map[string]bool{}
		filesSeen  int
	)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".cs":
		default:
			return nil
		}
		if filepath.Base(path) == selfExclusion {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		filesSeen++
		rel, _ := filepath.Rel(root, path)
		for _, v := range scanNamespaceLiterals(string(src), rel, distinct) {
			violations = append(violations, v)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	// Anti-vacuity: the walk found the tree.
	if filesSeen == 0 {
		t.Fatalf("scanned 0 source files under %s — the walk is not reaching the tree, "+
			"so a clean result here means nothing", root)
	}
	if len(distinct) < minLiteralsScanned {
		t.Fatalf("scanned only %d distinct namespace-path literals (floor %d) across %d files — "+
			"either the tree shrank drastically or the matcher stopped matching; a pass at this "+
			"count is not evidence",
			len(distinct), minLiteralsScanned, filesSeen)
	}

	// Collect, then report — never stop at the first (AP15). A count taken
	// from a loop that bails on the first hit is a lower bound, and this one
	// would be quoted at another seat.
	if len(violations) > 0 {
		sort.Slice(violations, func(i, j int) bool {
			if violations[i].file != violations[j].file {
				return violations[i].file < violations[j].file
			}
			return violations[i].line < violations[j].line
		})
		var b strings.Builder
		b.WriteString("entity-namespace path segments must be kebab-case " +
			"(`STYLE-NAMING-CONVENTIONS` §3: \"snake = violation\", gated). " +
			"snake_case is the data-structure-KEY axis — a `cbor:` tag stays snake, a path does not.\n")
		for _, v := range violations {
			b.WriteString("  " + v.file + ":" + strconv.Itoa(v.line) + ": segment " + v.segment +
				" in " + v.literal + "\n")
		}
		b.WriteString("  (" + strconv.Itoa(len(violations)) + " violation(s) across " +
			strconv.Itoa(filesSeen) + " files; " + strconv.Itoa(len(distinct)) + " literals scanned)")
		t.Error(b.String())
	}
}

// TestNamingGateCanFire is the arm that keeps the one above honest.
//
// Without it a detector that never flags anything is indistinguishable from a
// conformant tree, and the detector is where the subtlety is: the segment
// split, the `:` handling for `<handler-path>:<op>`, and the fact that a
// snake FIELD KEY beside a path must NOT be flagged.
func TestNamingGateCanFire(t *testing.T) {
	const synthetic = `
package p
const (
	Bad  = "app/state/peer_roster_entry"
	Good = "app/entity-workbench/peer-roster-entry"
	Op   = "workbench/blob_resolve:receive"
	OpOK = "workbench/blob-resolve:receive"
)
type T struct {
	// A snake field key is CORRECT and must not be flagged.
	PeerID string ` + "`cbor:\"peer_id\" json:\"peer_id\"`" + `
}
`
	got := scanNamespaceLiterals(synthetic, "synthetic.go", map[string]bool{})
	var flagged []string
	for _, v := range got {
		flagged = append(flagged, v.literal)
	}
	sort.Strings(flagged)

	want := []string{`"app/state/peer_roster_entry"`, `"workbench/blob_resolve:receive"`}
	if len(flagged) != len(want) {
		t.Fatalf("detector flagged %v, want exactly %v — if it flags nothing the sweep above "+
			"passes on any tree, and if it flags the field key it will be waived instead of fixed",
			flagged, want)
	}
	for i := range want {
		if flagged[i] != want[i] {
			t.Errorf("flagged[%d] = %s, want %s", i, flagged[i], want[i])
		}
	}
}

// scanNamespaceLiterals finds every namespace-path literal in src, records it
// in distinct, and returns the ones carrying a snake segment.
//
// The `:` split is not cosmetic. `workbench/blob-resolve:receive` is a handler
// path and an operation name in one token, and both are on the kebab axis
// (§3's table: entity-type path segment, operation name). Splitting only on
// `/` would leave `blob_resolve:receive` as one "segment" and still catch it —
// but it would report the op as part of the path, and the next reader would
// fix the wrong half.
func scanNamespaceLiterals(src, file string, distinct map[string]bool) []namingViolation {
	var out []namingViolation
	for _, idx := range namespacePathLiteral.FindAllStringIndex(src, -1) {
		lit := src[idx[0]:idx[1]]
		distinct[lit] = true
		body := strings.Trim(lit, `"`)
		for _, seg := range strings.FieldsFunc(body, func(r rune) bool {
			return r == '/' || r == ':'
		}) {
			if strings.Contains(seg, "_") {
				out = append(out, namingViolation{
					file:    file,
					line:    1 + strings.Count(src[:idx[0]], "\n"),
					literal: lit,
					segment: seg,
				})
				break // one report per literal; the fix is the whole literal
			}
		}
	}
	return out
}

// skipDir names directories whose contents are not this repo's source.
//
// `testdata` is skipped because it holds other implementations' frozen
// emissions — vendored bytes we must never "correct" (`AP96`: a cross-impl
// fixture measures the half of the corridor that faces it, and rewriting it
// makes the corridor agree with itself).
func skipDir(name string) bool {
	switch name {
	case ".git", "bin", "obj", "dist-native", "node_modules", "testdata",
		".test-logs", "run-logs":
		return true
	}
	return false
}

// repoRoot walks up from the test's working directory to the checkout root,
// identified by `go.work` beside `AGENTS.md` — two files, because either alone
// would also match a parent of this checkout on some machine.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		_, we := os.Stat(filepath.Join(dir, "go.work"))
		_, ae := os.Stat(filepath.Join(dir, "AGENTS.md"))
		if we == nil && ae == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no checkout root (a directory holding both go.work and AGENTS.md) " +
				"above the test's working directory")
		}
		dir = parent
	}
}
