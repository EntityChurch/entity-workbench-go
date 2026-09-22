package shellcmd

// Gates for the resumable backfill walk (C15).
//
// The control arm for all of this lives next door in
// `TestProbe_BoundedWalkMakesNoProgress`, which drives the CURSORLESS
// entry point and asserts it still converges to a fixed incomplete
// prefix. Without it these tests could pass against a build where the
// cap stopped biting, which would be the fix's most flattering failure.
//
// Tier: unit + integration against a real peer store (TESTING-STRATEGY
// §1/§2). The store is real on purpose: `List` returns peer-qualified
// paths and a `HasChildren` flag, and a hand-rolled fake of that is
// exactly the fixture-omits-the-wrapper trap AP58 is about.

import (
	"testing"

	"entity-workbench-go/entitysdk"
)

// seedPaths puts one entity at each given path and returns the peer.
func seedPaths(t *testing.T, paths ...string) *entitysdk.AppPeer {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	for _, p := range paths {
		if _, err := ap.Store().Put(p, "local/files/file",
			map[string]any{"name": p}); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	return ap
}

// TestWalkRemoteFilesAfter_EmissionIsAscendingPathOrder pins the property
// the cursor is built on, and it pins it at the place it is easy to get
// wrong.
//
// Leaves under a directory `d` all begin with `d + "/"`, so a directory
// has to sort under THAT key rather than its bare name. With `a` (a
// directory) and `a.txt` (a file) as siblings, sorting by bare name emits
// `a/x.txt` before `a.txt` — and `a.txt` < `a/x.txt`, because `.` (0x2E)
// sorts below `/` (0x2F). One transposed pair is enough to make a cursor
// skip a file permanently, which is the defect this whole change is
// about, so the fixture is built to contain that exact pair.
func TestWalkRemoteFilesAfter_EmissionIsAscendingPathOrder(t *testing.T) {
	const prefix = "local/files/t"
	ap := seedPaths(t,
		prefix+"/a.txt",
		prefix+"/a/x.txt",
		prefix+"/a/y.txt",
		prefix+"/b.txt",
		prefix+"/z/deep/q.txt",
	)
	ws := NewShellWorkspace(ap, "self", "")

	got, truncated, err := ws.walkRemoteFilesAfter(ap, ap.PeerID(), prefix, "")
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if truncated {
		t.Fatalf("a 5-entry folder truncated at a %d cap — the walk is not doing what it says",
			backfillWalkLimit)
	}
	if len(got) != 5 {
		t.Fatalf("walk returned %d paths, want 5: %v", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("emission is not ascending at %d: %q then %q\n"+
				"  a cursor compares with > against this order, so an out-of-order pair "+
				"is a file the next pass skips forever\n  full: %v",
				i, got[i-1], got[i], got)
		}
	}
	// Name the pair explicitly, so a regression says which rule broke
	// rather than only that the order moved.
	aTxt := "/" + ap.PeerID() + "/" + prefix + "/a.txt"
	aX := "/" + ap.PeerID() + "/" + prefix + "/a/x.txt"
	if indexOf(got, aTxt) > indexOf(got, aX) {
		t.Errorf("the directory sorted under its bare name: %q came after %q.\n"+
			"  A directory must sort as itself plus a separator — see walkRemoteFilesAfter.",
			aTxt, aX)
	}
}

// TestWalkRemoteFilesAfter_ResumeCoversExactlyTheTail is the resume
// property stated as a property rather than a case: for EVERY position in
// the folder, resuming after it returns exactly the rest of the folder.
//
// This is the assertion that catches an over-eager subtree prune. The
// prune skips a whole directory when the cursor is past it, which is what
// makes a resumed pass cheap in round trips — and an off-by-one there
// drops a subtree silently, with the walk reporting a clean, complete,
// shorter folder.
func TestWalkRemoteFilesAfter_ResumeCoversExactlyTheTail(t *testing.T) {
	const prefix = "local/files/t"
	ap := seedPaths(t,
		prefix+"/a.txt",
		prefix+"/a/x.txt",
		prefix+"/a/y.txt",
		prefix+"/b.txt",
		prefix+"/m/n/o/deep.txt",
		prefix+"/m/sibling.txt",
		prefix+"/z.txt",
	)
	ws := NewShellWorkspace(ap, "self", "")

	all, _, err := ws.walkRemoteFilesAfter(ap, ap.PeerID(), prefix, "")
	if err != nil {
		t.Fatalf("full walk: %v", err)
	}
	if len(all) != 7 {
		t.Fatalf("full walk returned %d paths, want 7: %v", len(all), all)
	}

	for i, cursor := range all {
		tail, _, err := ws.walkRemoteFilesAfter(ap, ap.PeerID(), prefix, cursor)
		if err != nil {
			t.Fatalf("resume after %q: %v", cursor, err)
		}
		want := all[i+1:]
		if len(tail) != len(want) {
			t.Fatalf("resume after %q returned %d paths, want %d\n  got:  %v\n  want: %v",
				cursor, len(tail), len(want), tail, want)
		}
		for j := range want {
			if tail[j] != want[j] {
				t.Fatalf("resume after %q differs at %d: got %q, want %q",
					cursor, j, tail[j], want[j])
			}
		}
	}
}

// TestBackfillWalk_TwoPassesCoverAnOversizedFolder is C15 itself: the
// measurement that reported 5,000 entries unreachable, re-run against the
// cursor.
//
// It composes the two pieces `backfill` composes — the walk and
// `nextBackfillCursor` — rather than re-deriving the advance rule, because
// a gate that carries its own copy of the rule passes against a build
// where the real one is wrong.
func TestBackfillWalk_TwoPassesCoverAnOversizedFolder(t *testing.T) {
	const over = backfillWalkLimit + 500
	const prefix = "local/files/big"

	ap := seedFlat(t, prefix, over)
	ws := NewShellWorkspace(ap, "self", "")

	seenPaths := make(map[string]bool, over)
	cursor := ""
	passes := 0
	const maxPasses = 8

	for {
		passes++
		if passes > maxPasses {
			t.Fatalf("still not finished after %d passes with %d/%d paths seen — "+
				"the cursor is not advancing", maxPasses, len(seenPaths), over)
		}
		paths, truncated, err := ws.walkRemoteFilesAfter(ap, ap.PeerID(), prefix, cursor)
		if err != nil {
			t.Fatalf("pass %d: %v", passes, err)
		}
		// Anti-vacuity: the cap must actually bite on the first pass, or
		// this is measuring an ordinary complete walk and says nothing
		// about the defect it is named after.
		if passes == 1 && !truncated {
			t.Fatalf("pass 1 over %d entries did not truncate at the %d cap — "+
				"the cap did not bite and this gate is about nothing", over, backfillWalkLimit)
		}
		fresh := 0
		for _, p := range paths {
			if !seenPaths[p] {
				seenPaths[p] = true
				fresh++
			}
		}
		if passes > 1 && fresh == 0 {
			t.Fatalf("pass %d returned %d paths and NONE were new — this is the C15 "+
				"failure the cursor exists to fix, reproduced after the fix",
				passes, len(paths))
		}
		resumedAfter := cursor
		cursor = nextBackfillCursor(paths, truncated)
		t.Logf("pass %d: resumed after %q · %d paths, %d new · truncated=%v · next cursor %q",
			passes, trimPeerQualified(resumedAfter), len(paths), fresh, truncated,
			trimPeerQualified(cursor))
		if cursor == "" {
			break
		}
	}

	if len(seenPaths) != over {
		t.Fatalf("covered %d of %d entries in %d passes — %d are still unreachable",
			len(seenPaths), over, passes, over-len(seenPaths))
	}
	if passes != 2 {
		t.Errorf("took %d passes to cover %d entries at a %d cap; 2 is the arithmetic. "+
			"Not wrong, but it means a pass is re-covering ground", passes, over, backfillWalkLimit)
	}
	t.Logf("CONFIRMED: %d entries covered in %d passes, none unreachable "+
		"(C15's 5,000-entry permanent gap is closed)", over, passes)
}

// TestNextBackfillCursor is the advance rule, including the case that
// makes the loop terminate.
func TestNextBackfillCursor(t *testing.T) {
	paths := []string{"/p/a", "/p/b", "/p/c"}

	if got := nextBackfillCursor(paths, true); got != "/p/c" {
		t.Errorf("truncated pass: cursor %q, want the last path covered", got)
	}
	if got := nextBackfillCursor(paths, false); got != "" {
		t.Errorf("complete pass: cursor %q, want empty — a folder that finished must "+
			"wrap, or `resync` never reports everything already-current and the flow "+
			"loses its only positive confirmation", got)
	}
	// A truncated pass that returned nothing cannot advance; wrapping is
	// the only safe move, because holding a cursor nothing reached would
	// stall the folder for good.
	if got := nextBackfillCursor(nil, true); got != "" {
		t.Errorf("truncated pass with no paths: cursor %q, want empty", got)
	}
}

func indexOf(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
	return -1
}
