package workbench

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// Unit tier (TESTING-STRATEGY §1).

// A keep-both sibling's NAME is bytes that land in the tree, so it is a
// Layer-2 algorithm contract (AGENTS.md): two implementations that name
// the same conflict differently produce two files where there should be
// one, and neither is wrong on its own terms.
//
// `ext/revision/strategy.go` builds it as
//
//	path + ".keep-both-" + hex.EncodeToString(remote.Digest[:4])
//
// Restated here because that function is unexported. This test IS the
// restatement's justification: it derives the expected name the kernel's
// way, from the kernel's own inputs, and compares.
func TestKeepBothPath_MatchesTheKernelsNaming(t *testing.T) {
	var h hash.Hash
	for i := range h.Digest {
		h.Digest[i] = byte(i * 7)
	}

	// The kernel's expression, written out rather than called.
	want := "docs/readme.md" + ".keep-both-" + hex.EncodeToString(h.Digest[:4])
	got := KeepBothPath("docs/readme.md", h)
	if got != want {
		t.Fatalf("keep-both path = %q, want %q — a sibling name is bytes in the "+
			"tree, so a divergence here is two files where there should be one",
			got, want)
	}

	// The kernel's own test pins the LENGTH of the suffix at 8 hex
	// characters (ext/revision/handler_test.go asserts
	// len(path) == len(prefix)+8). A five-byte digest slice would pass a
	// prefix check and fail interop.
	suffix := KeepBothSuffix(h)
	if n := len(suffix) - len(".keep-both-"); n != 8 {
		t.Errorf("suffix carries %d hex characters, want 8", n)
	}
}

// A sibling is a real file that syncs like any other, so it must be
// recognisable — and never treated as a conflict candidate itself, which
// is how one collision breeds on every pass.
func TestIsKeepBothPath(t *testing.T) {
	var h hash.Hash
	h.Digest[0], h.Digest[1], h.Digest[2], h.Digest[3] = 0xde, 0xad, 0xbe, 0xef

	yes := []string{
		KeepBothPath("local/files/shared/notes.md", h),
		KeepBothPath("a", h),
	}
	for _, p := range yes {
		if !IsKeepBothPath(p) {
			t.Errorf("%q not recognised as a keep-both sibling", p)
		}
	}

	// The negatives matter more than the positives: a false positive here
	// silently excludes a real file from conflict detection, and the file
	// it excludes is one somebody named unluckily.
	no := []string{
		"local/files/shared/notes.md",
		"local/files/shared/notes.md.keep-both-",
		"local/files/shared/notes.md.keep-both-xyz",
		"local/files/shared/notes.md.keep-both-deadbee",   // 7 chars
		"local/files/shared/notes.md.keep-both-deadbeeff", // 9 chars
		"local/files/shared/keep-both-deadbeef",           // no dot
		"local/files/shared/notes.keep-both-DEADBEEF",     // upper case
	}
	for _, p := range no {
		if IsKeepBothPath(p) {
			t.Errorf("%q wrongly recognised as a keep-both sibling", p)
		}
	}
}

// A conflict is identified by the PATH AND BOTH HASHES, so a second
// collision at the same path is a second record.
//
// Keying on the path alone would let the newest collision silently
// replace the record for the previous one — and the previous record is
// what carries the hash needed to recover an edit two overwrites ago.
func TestConflictKey_IsPerCollisionNotPerPath(t *testing.T) {
	var a, b, c hash.Hash
	a.Digest[0], b.Digest[0], c.Digest[0] = 1, 2, 3
	const p = "/peer/local/files/shared/notes.md"

	k1 := ConflictKey(p, a, b)
	k2 := ConflictKey(p, b, c)
	if k1 == k2 {
		t.Error("two different collisions at one path share a key — the first " +
			"record, and the hash that recovers it, is silently replaced")
	}
	if ConflictKey(p, a, b) != k1 {
		t.Error("the key is not deterministic — the same collision would record twice")
	}
	// Order matters: mine/theirs swapped is a different collision.
	if ConflictKey(p, b, a) == k1 {
		t.Error("the key ignores which side is which")
	}
	if strings.Contains(k1, "/") {
		t.Errorf("key %q is not a single path segment", k1)
	}
	if !strings.HasPrefix(k1, "notes-md-") {
		t.Errorf("key %q does not lead with a readable tail", k1)
	}
}

// Absent means the DEFAULT, never keep-both.
//
// Same rule and same reason as EffectiveMode's absent case: defaulting to
// the stronger action starts changing what is inside folders an operator
// set up under the weaker one, and that mistake is not symmetric — the
// weak default loses nothing, the strong one leaves files nobody asked
// for and stops a folder converging.
func TestConflictPolicy_AbsentMeansRecord(t *testing.T) {
	if got := (FolderData{}).ConflictPolicy(); got != ConflictPolicyRecord {
		t.Errorf("an absent policy is %q, want %q", got, ConflictPolicyRecord)
	}
	if got := (FolderData{Conflict: "nonsense"}).ConflictPolicy(); got != ConflictPolicyRecord {
		t.Errorf("an unrecognised policy is %q, want %q — a value a newer build "+
			"wrote must not read as the destructive option", got, ConflictPolicyRecord)
	}
	if got := (FolderData{Conflict: ConflictPolicyKeepBoth}).ConflictPolicy(); got != ConflictPolicyKeepBoth {
		t.Errorf("a declared keep-both reads as %q", got)
	}

	for _, s := range []string{"keep-both", "KEEP-BOTH", "both", " keep_both "} {
		if p, err := ParseConflictPolicy(s); err != nil || p != ConflictPolicyKeepBoth {
			t.Errorf("ParseConflictPolicy(%q) = (%q, %v)", s, p, err)
		}
	}
	for _, s := range []string{"record", "converge", "lww"} {
		if p, err := ParseConflictPolicy(s); err != nil || p != ConflictPolicyRecord {
			t.Errorf("ParseConflictPolicy(%q) = (%q, %v)", s, p, err)
		}
	}
	if _, err := ParseConflictPolicy("three-way"); err == nil {
		t.Error("an unsupported strategy was accepted — a policy this product " +
			"does not implement must refuse rather than degrade to a default " +
			"the operator did not choose (AP33)")
	}
}

// The burst limiter is SYNC-LIMITS §5 rule 1, and it is the part that has
// to work when everything else in the feature is wrong.
//
// Driven with an injected clock rather than by sleeping: a window this
// long is untestable in real time, and a test that shortens the constant
// is testing a constant nobody ships.
func TestConflictLimiter_FailsClosedAndRecovers(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	l := &conflictLimiter{now: func() time.Time { return now }}

	for i := 0; i < ConflictBurstLimit; i++ {
		if !l.admit() {
			t.Fatalf("refused conflict %d of %d, before the limit", i+1, ConflictBurstLimit)
		}
	}
	if l.admit() {
		t.Fatal("admitted a conflict past the limit — the storm carries on, " +
			"which is the cheap wrong answer §5 rule 1 exists to forbid")
	}
	if l.refused != 1 {
		t.Errorf("refused count is %d, want 1", l.refused)
	}

	// A SLIDING window, not a resetting counter. Half a window later, the
	// events are still in scope and the limit still bites — a counter
	// that reset on a boundary would let 2×limit through across two
	// adjacent windows, which is twice as many files touched by the bug
	// the limit exists to stop.
	now = base.Add(ConflictBurstWindow / 2)
	if l.admit() {
		t.Error("the limit lapsed half a window in — this is a resetting " +
			"counter, not a sliding window")
	}

	// Past the window it recovers by itself, with no operator action.
	// That is what makes a refusal a DELAY: the catch-up supervisor
	// re-derives the truth on its next pass.
	now = base.Add(ConflictBurstWindow + time.Second)
	if !l.admit() {
		t.Error("the limiter never recovers — a single burst would stop " +
			"delivery for the life of the process")
	}
}

// A resolved conflict is KEPT with its decision.
//
// "This file was in conflict on Tuesday and you chose theirs" is the
// question an operator asks a week later, and a record that deletes
// itself on resolution cannot answer it.
func TestConflictData_UnresolvedAndSummary(t *testing.T) {
	c := ConflictData{Path: "/p/local/files/s/n.md", RemotePeerID: "12D3KooWSomePeer"}
	if !c.Unresolved() {
		t.Error("a fresh record reads as resolved")
	}
	if !strings.Contains(c.Summary(), "NOT recoverable") {
		t.Errorf("a record with Recoverable false does not say so: %q", c.Summary())
	}

	c.Recoverable = true
	if !strings.Contains(c.Summary(), "resolve ") {
		t.Errorf("a recoverable record does not name the verb that recovers it: %q",
			c.Summary())
	}

	c.Kept = ConflictKeptMine
	if c.Unresolved() {
		t.Error("a resolved record still reads as unresolved")
	}
	if strings.Contains(c.Summary(), "resolve ") {
		t.Errorf("a resolved record still tells the operator to resolve it: %q",
			c.Summary())
	}
}
