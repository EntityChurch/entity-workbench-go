package shellcmd

import (
	"strings"
	"testing"

	"entity-workbench-go/workbench"
)

// Unit tier (TESTING-STRATEGY §1) — the path arithmetic the guard turns
// on, none of which is obvious from either end.

// A head pointer path carries the peer-id TWICE, and getting that wrong
// silently guards nothing: an unparsed event is dropped, so the guard
// runs, counts zero, and reports itself healthy.
func TestHistoryBudget_TrackedPathFromHead(t *testing.T) {
	const peer = "12D3KooWExamplePeerIdBase58"
	cases := []struct {
		name        string
		head        string
		wantTracked string
		wantRoot    string
		wantOK      bool
	}{
		{
			name:        "a file under a mount root",
			head:        "/" + peer + "/system/history/head/" + peer + "/local/files/photos/a.jpg",
			wantTracked: "/" + peer + "/local/files/photos/a.jpg",
			wantRoot:    "photos",
			wantOK:      true,
		},
		{
			name:        "a nested file keeps its whole relative path",
			head:        "/" + peer + "/system/history/head/" + peer + "/local/files/photos/2026/a.jpg",
			wantTracked: "/" + peer + "/local/files/photos/2026/a.jpg",
			wantRoot:    "photos",
			wantOK:      true,
		},
		{
			// A config an operator installed over their own prefix is
			// theirs. Applying a budget to it would be this loop deciding
			// something nobody declared.
			name:   "a path outside local/files is not ours to guard",
			head:   "/" + peer + "/system/history/head/" + peer + "/docs/report",
			wantOK: false,
		},
		{
			name:   "the mount root's own entity is not a file in it",
			head:   "/" + peer + "/system/history/head/" + peer + "/local/files/photos",
			wantOK: false,
		},
		{
			name:   "an unrelated prefix",
			head:   "/" + peer + "/app/workbench/folders/x",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracked, root, ok := trackedPathFromHead(tc.head)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (tracked %q root %q)", ok, tc.wantOK, tracked, root)
			}
			if !ok {
				return
			}
			if tracked != tc.wantTracked {
				t.Errorf("tracked = %q, want %q", tracked, tc.wantTracked)
			}
			if root != tc.wantRoot {
				t.Errorf("root = %q, want %q", root, tc.wantRoot)
			}
		})
	}
}

// The exclusion pattern must be SHORT form. An absolute pattern would
// still match — canonicalizePattern passes it through — so this cannot
// fail end to end in a single-peer test, and it would quietly encode our
// peer-id into a record that then means nothing after an identity change.
func TestHistoryBudget_ExclusionPatternIsShortForm(t *testing.T) {
	const peer = "12D3KooWExamplePeerIdBase58"
	got, ok := historyExclusionPattern("/" + peer + "/local/files/photos/a.jpg")
	if !ok {
		t.Fatal("no pattern for a path under local/files")
	}
	if got != "local/files/photos/a.jpg" {
		t.Errorf("pattern = %q, want the short form", got)
	}
	if strings.Contains(got, peer) {
		t.Errorf("pattern %q carries the peer-id — the recorder canonicalizes "+
			"an unrooted pattern against the local peer, so this is both "+
			"redundant and a claim about a namespace", got)
	}

	// The exclusion is MORE SPECIFIC than the folder config it has to
	// outrank. EXTENSION-HISTORY §6.2 orders on literal segment count
	// first, and configCache.find returns nil when the most specific
	// match is disabled — which is the whole mechanism.
	folder := FolderHistoryPattern("photos")
	if literalSegments(got) <= literalSegments(folder) {
		t.Errorf("exclusion %q has %d literal segments and the folder config %q "+
			"has %d — the exclusion does not outrank it, so it stops nothing",
			got, literalSegments(got), folder, literalSegments(folder))
	}
}

// literalSegments mirrors ext/history/config.go's key-1 specificity
// count. Restated rather than imported because that function is
// unexported; the end-to-end behaviour is gated in
// shellboot/history_budget_e2e_test.go, which runs the real kernel.
func literalSegments(pattern string) int {
	n := 0
	for _, seg := range strings.Split(strings.Trim(pattern, "/"), "/") {
		if seg != "*" && seg != "" {
			n++
		}
	}
	return n
}

// A fixed probe threshold beside a configurable budget is wrong the
// moment the budget is set below it: nothing is ever measured, so nothing
// ever trips, and the guard reports itself running while doing nothing.
// That reads as "no runaway paths", which is the failure mode this whole
// file exists to prevent.
func TestHistoryBudget_ProbeThresholdTracksTheBudget(t *testing.T) {
	for _, budget := range []uint64{1, 5, 9, 10, 100, DefaultHistoryPathBudget} {
		got := historyProbeThreshold(budget)
		if got == 0 {
			t.Errorf("budget %d yields probe threshold 0 — a path would be "+
				"measured on no event at all", budget)
		}
		if got > budget {
			t.Errorf("budget %d yields probe threshold %d, which is above the "+
				"budget: the depth is never derived, so a path carried over "+
				"from an earlier run can never trip", budget, got)
		}
	}
	if got := historyProbeThreshold(DefaultHistoryPathBudget); got != 200 {
		t.Errorf("default probe threshold = %d, want 200 — the doc comment "+
			"quotes this number", got)
	}
}

// The key has to survive two paths whose tails agree and two whose
// sanitized forms collide. Both shapes exist in any real folder.
func TestHistoryLimitKey_IsCollisionFreeAndReadable(t *testing.T) {
	keys := map[string]string{}
	paths := []string{
		"/p/local/files/photos/a/notes.txt",
		"/p/local/files/photos/b/notes.txt",
		"/p/local/files/photos/a-b",
		"/p/local/files/photos/a/b",
		"/p/local/files/photos/Notes.TXT",
	}
	for _, p := range paths {
		k := workbench.HistoryLimitKey(p)
		if prev, dup := keys[k]; dup {
			t.Errorf("key %q collides: %q and %q", k, prev, p)
		}
		keys[k] = p
		if strings.Contains(k, "/") {
			t.Errorf("key %q for %q is not a single path segment", k, p)
		}
	}
	if k := workbench.HistoryLimitKey("/p/local/files/photos/server.log"); !strings.HasPrefix(k, "server-log-") {
		t.Errorf("key %q does not lead with a readable tail — "+
			"`ls app/workbench/history-limits/` has to mean something", k)
	}
	// Deterministic: the same path twice is one entity, not two.
	a := workbench.HistoryLimitKey("/p/local/files/photos/server.log")
	b := workbench.HistoryLimitKey("/p/local/files/photos/server.log")
	if a != b {
		t.Errorf("key is not deterministic: %q then %q", a, b)
	}
}

// A guard config must be tellable from the folder's own and from one an
// operator wrote by hand, at a glance in `system/history/config/`.
func TestHistoryBudget_ConfigNameIsMarked(t *testing.T) {
	name := historyLimitConfigName("/p/local/files/photos/server.log")
	if !strings.HasPrefix(name, historyLimitConfigNamePrefix) {
		t.Errorf("config name %q is not marked as the guard's", name)
	}
	if strings.HasPrefix(name, "folder-") {
		t.Errorf("config name %q collides with the folder config's namespace", name)
	}
	if got := folderHistoryConfigName("photos"); strings.HasPrefix(got, historyLimitConfigNamePrefix) {
		t.Errorf("folder config name %q reads as a guard config", got)
	}
}

// The head prefix is a spec-adjacent path we restate because the
// kernel's constant is unexported. If the recorder ever moves it, the
// watch silently observes nothing — a guard that counts zero and reports
// itself healthy — so the restatement is asserted against the shape the
// recorder actually writes.
func TestHistoryBudget_HeadPrefixMatchesTheRecorder(t *testing.T) {
	const peer = "12D3KooWExamplePeerIdBase58"
	tracked := "/" + peer + "/local/files/photos/a.jpg"
	// ext/history/recorder.go: headPointerPath = headPrefix +
	// strings.TrimPrefix(trackedPath, "/").
	head := "/" + peer + "/" + historyHeadPrefix + strings.TrimPrefix(tracked, "/")
	got, root, ok := trackedPathFromHead(head)
	if !ok || got != tracked || root != "photos" {
		t.Fatalf("round trip failed: %q -> (%q, %q, %v), want %q",
			head, got, root, ok, tracked)
	}
}
