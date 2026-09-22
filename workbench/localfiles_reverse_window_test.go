package workbench

// ext/localfiles' reverse-write loop: the echo guard is CONTENT IDENTITY, and
// a genuine second write inside a burst must land.
//
// ⭐ WHAT THIS FILE WAS, AND WHY IT CHANGED SHAPE. It began as the M1
// reproducer for **core-go tracker row 2**: the reverse-write loop's echo
// guard was a five-second clock (`reverseTracker` / `recentWriteWindow`), and
// the clock DISCARDED — not deferred — any tree change event on a path we had
// written inside the window. A file edited twice in quick succession upstream
// lost its second version, permanently, with no error on any channel. Every
// test here asserted that defect on purpose, and each failure message said
// what to do if it ever stopped reproducing.
//
// ✅ **It stopped reproducing on 2026-09-17.** core-go removed `reverseTracker`
// entirely: the content check (`currentDiskBlobHash == fileData.Content`,
// §5.5 Amdt 3) is now the sole echo authority, and `reverseDelete`'s
// `os.IsNotExist` tolerance covers an echoed delete. Three tests here went red
// with their own retirement instructions, exactly as designed.
//
// ⭐ **THE TRIPWIRES ARE REPLACED, NOT DELETED — and that is the step most
// likely to be skipped.** A tripwire that keeps passing after its blocker
// lifts is the one failure it must not have; a tripwire DELETED on the day it
// fires leaves the fix with no gate at all, in the one tree that noticed the
// defect. So each of the three now asserts the PROPERTY the fix delivers,
// against the same rig, driving the same real handler:
//
//   - a genuinely different second update inside the old window LANDS;
//   - it lands PROMPTLY — the old window is not merely survivable, it is gone;
//   - a delete inside a burst LANDS.
//
// ⚠ **The anti-vacuity arm for all three is `…ContentIdentityStopsTheEcho`**,
// and it is not optional: "a second write lands" is satisfied just as well by
// a build with the echo guard deleted outright, which would put the peer back
// in a write loop with itself. That test proves the guard is still real, with
// the same rig, so the three positives cannot pass by suppression having been
// removed rather than corrected.
//
// WHAT THIS FILE TESTS. Nothing in it is workbench code. It drives core-go's
// real handler, real root mapping, real reverse-write loop and real
// filesystem, with a hand-fed tree event channel standing in for the notifying
// store's fan-out — the same seam `ext/localfiles/handler_test.go`'s own
// `TestReverseWrite` uses. It is kept here because this tree is where the
// defect was found and is the only tree whose product depends on the fix; a
// regression would present here as files silently not arriving.
//
// The filename still says `reverse_window` for the mechanism that is gone. It
// is left alone deliberately — `AGENTS.md` cites this path, and renaming a
// file to tidy a name costs a working citation.
//
// Tier: integration (TESTING-STRATEGY) — real handler, real disk, real
// goroutine, no network.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// reverseWindowPeerID is the local namespace the reverse-write loop filters
// on. `reverseWriteLoop` skips any event whose peer-id is not the local one,
// so every event this file emits must carry it.
const reverseWindowPeerID = "TestPeer1234567890abcdefghijklmnopqrstuvwxyz01"

// reverseWindowPrefix is the tree prefix of the single root these tests mount.
const reverseWindowPrefix = "local/files/rw/"

// reverseRig is a live localfiles root with the reverse-write loop running.
type reverseRig struct {
	t      *testing.T
	dir    string
	cs     store.ContentStore
	li     store.LocationIndex
	h      *localfiles.Handler
	events chan store.TreeChangeEvent
	seq    int
}

func newReverseRig(t *testing.T) *reverseRig {
	t.Helper()
	dir := t.TempDir()
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	h := localfiles.NewHandler(nil)
	if err := h.AddRoot("rw", localfiles.RootConfigData{
		Prefix:         reverseWindowPrefix,
		FilesystemRoot: dir,
	}, cs, li); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = h.Close() })

	events := make(chan store.TreeChangeEvent, 64)
	h.StartReverseWrite(ctx, events, cs, li, reverseWindowPeerID)

	return &reverseRig{t: t, dir: dir, cs: cs, li: li, h: h, events: events}
}

// ingest chunks and stores body exactly the way the watcher and
// `local/files:write` do, and returns the blob hash. Same chunker, same
// chunk_size, so the circuit breaker's recompute must agree with it.
func (r *reverseRig) ingest(body []byte) hash.Hash {
	r.t.Helper()
	ranges := chunker.ChunkFastCDC(body, types.DefaultChunkSize)
	blobEnt, chunks, err := content.BuildBlob(body, ranges, types.ChunkingFastCDC, types.DefaultChunkSize)
	if err != nil {
		r.t.Fatalf("BuildBlob: %v", err)
	}
	for _, c := range chunks {
		if _, err := r.cs.Put(c); err != nil {
			r.t.Fatalf("put chunk: %v", err)
		}
	}
	bh, err := r.cs.Put(blobEnt)
	if err != nil {
		r.t.Fatalf("put blob: %v", err)
	}
	return bh
}

// bind writes a file entity for body at the tree path and returns its hash.
// It does not emit an event — the two are separate so a test can bind
// without notifying, which is what a real store does in the other order.
func (r *reverseRig) bind(name string, body []byte) hash.Hash {
	r.t.Helper()
	fd := localfiles.FileData{
		Path:    name,
		Size:    uint64(len(body)),
		Content: r.ingest(body),
	}
	ent, err := fd.ToEntity()
	if err != nil {
		r.t.Fatalf("FileData.ToEntity: %v", err)
	}
	fh, err := r.cs.Put(ent)
	if err != nil {
		r.t.Fatalf("put file entity: %v", err)
	}
	if err := r.li.Set(reverseWindowPrefix+name, fh); err != nil {
		r.t.Fatalf("li.Set: %v", err)
	}
	return fh
}

// emit sends the tree change event in the qualified form the store's fan-out
// delivers it in — `/{peerID}/{barePath}`.
func (r *reverseRig) emit(name string, fh hash.Hash, ct store.ChangeType) {
	r.events <- store.TreeChangeEvent{
		Path:       "/" + reverseWindowPeerID + "/" + reverseWindowPrefix + name,
		PeerID:     reverseWindowPeerID,
		Hash:       fh,
		ChangeType: ct,
	}
}

// barrier makes "the loop has finished deciding about the previous event" an
// observable fact instead of a sleep. `reverseWriteLoop` is a single
// goroutine draining one channel in order, so once a later event's side
// effect appears on disk, every earlier event has been processed to a
// decision. Without this every negative assertion here would be a race
// dressed up as a timeout.
func (r *reverseRig) barrier() {
	r.t.Helper()
	r.seq++
	name := fmt.Sprintf("barrier-%d.txt", r.seq)
	body := []byte(name)
	fh := r.bind(name, body)
	r.emit(name, fh, store.ChangeCreated)
	if !r.waitForDisk(name, body, 10*time.Second) {
		r.t.Fatalf("barrier %s never reached disk — the reverse-write loop is not running, so nothing else in this test means anything", name)
	}
}

func (r *reverseRig) diskBytes(name string) []byte {
	b, err := os.ReadFile(filepath.Join(r.dir, name))
	if err != nil {
		return nil
	}
	return b
}

func (r *reverseRig) waitForDisk(name string, want []byte, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if string(r.diskBytes(name)) == string(want) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// -----------------------------------------------------------------------
// R1 — a genuinely new tree write inside the window is discarded.
// -----------------------------------------------------------------------

// TestReverseWrite_GenuineSecondUpdateLands is the property that replaces the
// core reproducer (core-go tracker row 2, fixed 2026-09-17).
//
// Two updates land on one path within milliseconds. The first is written to
// disk; the second carries DIFFERENT bytes and must reach disk too. Under the
// old clock the second was discarded as an echo it was not, and tree and disk
// diverged silently with no error on any channel.
//
// This is not an exotic shape. It is what a file edited twice in quick
// succession upstream looks like at the consuming peer, which is exactly the
// traffic M2 (two machines, one folder) is made of.
//
// Anti-vacuity: `…ContentIdentityStopsTheEcho` is the arm that stops this
// passing against a build with the echo guard simply removed. Read them
// together or neither means anything.
func TestReverseWrite_GenuineSecondUpdateLands(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("version A — the reverse writer put this on disk\n")
	b := []byte("version B — a genuinely different update to the same path\n")

	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeCreated)
	if !rig.waitForDisk("notes.md", a, 10*time.Second) {
		t.Fatalf("setup: version A never reached disk; the rig is wrong, not the substrate")
	}

	rig.emit("notes.md", rig.bind("notes.md", b), store.ChangeModified)
	if !rig.waitForDisk("notes.md", b, 10*time.Second) {
		got := string(rig.diskBytes("notes.md"))
		if got == string(a) {
			t.Fatalf("REGRESSION (core-go tracker row 2): version B was dropped on the heels of version A. "+
				"Disk holds the stale version A and the tree binding at %snotes.md commits version B. "+
				"An echo guard is discarding a genuine update again — re-read ext/localfiles/reverse.go "+
				"and check whether a time-based suppression has come back.", reverseWindowPrefix)
		}
		t.Fatalf("disk holds neither A nor B (%q) — the rig has drifted; re-derive the mechanism before routing anything", got)
	}
}

// TestReverseWrite_ASecondUpdateIsNotDelayedByAWindow replaces the half of the
// old reproducer that established the drop was DATA LOSS rather than latency:
// the event was consumed and discarded, with no deferred queue, so waiting out
// the window changed nothing.
//
// ⭐ The property that replaces it is deliberately about TIME, not just about
// arrival, because the two are different regressions and only one of them is
// caught by the test above. A build that re-introduced suppression *with* a
// retry queue would deliver version B — five or more seconds late — and
// `…GenuineSecondUpdateLands` would pass, since it waits ten. That is a
// latency defect a sync product feels directly, so it gets its own assertion:
// B must land well inside the window that used to swallow it.
//
// The bound is deliberately loose (2s against the old 5s window). This is a
// regression gate, not a performance budget — a real delivery is sub-millisecond
// here, so anything approaching seconds means a timer has come back, while a
// tight bound would just make the suite flaky under load.
func TestReverseWrite_ASecondUpdateIsNotDelayedByAWindow(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("version A\n")
	b := []byte("version B\n")

	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeCreated)
	if !rig.waitForDisk("notes.md", a, 10*time.Second) {
		t.Fatalf("setup: version A never reached disk")
	}

	const oldWindow = 5 * time.Second
	started := time.Now()
	rig.emit("notes.md", rig.bind("notes.md", b), store.ChangeModified)
	if !rig.waitForDisk("notes.md", b, 10*time.Second) {
		t.Fatalf("REGRESSION: version B never landed at all (disk holds %q) — "+
			"see TestReverseWrite_GenuineSecondUpdateLands, which is the same defect without the timing arm",
			string(rig.diskBytes("notes.md")))
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("REGRESSION: version B took %v to reach disk, against an old suppression window of %v. "+
			"It arrived, so the drop is gone, but something is now DEFERRING a genuine update on a timer "+
			"— re-read ext/localfiles/reverse.go for a reintroduced delay.", elapsed, oldWindow)
	}
}

// -----------------------------------------------------------------------
// R2 — a delete inside the window is discarded, and nothing else catches it.
// -----------------------------------------------------------------------

// TestReverseWrite_DeleteWithinBurstLands is the sharper half, and it is the
// one whose old failure could not be caught anywhere else.
//
// A suppressed WRITE was at least covered on paper by the content check:
// `reverseWriteLoop` routes `ChangeDeleted` to `reverseDelete` before any
// content is fetched, so under the old clock the clock was the *only* thing
// standing between a tree deletion and the filesystem — and it stopped it
// permanently, with no content check downstream to catch it.
//
// The failure that produced: the tree says the file is gone, the disk says it
// is there, and the next watcher scan is entitled to ingest it back. A
// deletion that undoes itself, which reads to an operator as the product
// refusing to delete their file.
func TestReverseWrite_DeleteWithinBurstLands(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("this file is about to be deleted upstream\n")
	rig.emit("doomed.md", rig.bind("doomed.md", a), store.ChangeCreated)
	if !rig.waitForDisk("doomed.md", a, 10*time.Second) {
		t.Fatalf("setup: the file never reached disk")
	}

	// Delete it from the tree immediately — inside what used to be the window.
	if _, ok := rig.li.Remove(reverseWindowPrefix + "doomed.md"); !ok {
		t.Fatalf("setup: nothing bound at %sdoomed.md to remove", reverseWindowPrefix)
	}
	rig.emit("doomed.md", hash.Hash{}, store.ChangeDeleted)
	rig.barrier()

	if _, err := os.Stat(filepath.Join(rig.dir, "doomed.md")); err == nil {
		t.Fatalf("REGRESSION (core-go tracker row 2): the tree no longer binds %sdoomed.md and the file is "+
			"STILL ON DISK. reverseDelete has no content check, so a suppression in front of it is "+
			"unrecoverable — the next watcher scan will ingest the file back and the deletion undoes itself.",
			reverseWindowPrefix)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat after delete: %v (expected the file to be gone)", err)
	}
}

// -----------------------------------------------------------------------
// R3 — the control arm: what the clock is actually buying.
// -----------------------------------------------------------------------

// TestReverseWindow_ContentIdentityStopsTheEcho is the ANTI-VACUITY ARM for
// the three property tests above, and it is the reason they mean anything.
//
// Each of those asserts that something LANDS. All three are satisfied just as
// well by a build with echo suppression removed outright — which would put the
// peer in a write loop with itself, a strictly worse failure than the one they
// were written for. This arm holds the other side: our own disk write coming
// back around as a tree event is still REFUSED, by content identity
// (`currentDiskBlobHash == fileData.Content`) and not by any clock.
//
// It was originally the control that made the row-2 ask actionable rather than
// a complaint — it established that the guard the clock was standing in front
// of already existed. core-go's fix was to delete the clock and keep exactly
// this guard, so the test needed no change at all; only its job did.
//
// The file is seeded on disk directly, so nothing in the write path ran for it
// first. If the echo is caught anyway, content identity is the guard.
func TestReverseWindow_ContentIdentityStopsTheEcho(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("bytes that are already on disk\n")
	fsPath := filepath.Join(rig.dir, "echo.md")
	if err := os.WriteFile(fsPath, a, 0o600); err != nil {
		t.Fatalf("seed disk: %v", err)
	}
	// Guarantee a distinguishable mtime if anything rewrites the file:
	// reverseWrite streams to a temp file and renames, so a real write always
	// produces a fresh mtime.
	time.Sleep(20 * time.Millisecond)
	before, err := os.Stat(fsPath)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}

	rig.emit("echo.md", rig.bind("echo.md", a), store.ChangeModified)
	rig.barrier()

	after, err := os.Stat(fsPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("ANTI-VACUITY ARM FAILED: the echo was rewritten (mtime %v -> %v). "+
			"Content identity is no longer refusing our own write coming back as a tree event, which means "+
			"the three property tests above are passing against a build with NO echo guard — a peer in a "+
			"write loop with itself. Fix this before trusting any of them.",
			before.ModTime(), after.ModTime())
	}
}

// TestReverseWrite_ARefusedEchoDoesNotBlockTheNextUpdate keeps the property
// that survives the clock's removal.
//
// It used to pin the ORDERING the whole row-2 argument rested on — the content
// check returns before `markWritten`, so an echo can never arm the clock —
// and with `reverseTracker` gone there is no ordering left to pin. What is
// still worth asserting, and is not covered above, is the composition: an
// event REFUSED by the echo guard must not poison the path for the genuine
// update that follows it. A guard that latched on a refusal would pass every
// other test in this file and silently stall exactly the path a burst is
// hitting.
func TestReverseWrite_ARefusedEchoDoesNotBlockTheNextUpdate(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("version A\n")
	b := []byte("version B\n")

	// Disk already holds A, so event 1 is refused by the content check.
	if err := os.WriteFile(filepath.Join(rig.dir, "notes.md"), a, 0o600); err != nil {
		t.Fatalf("seed disk: %v", err)
	}
	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeModified)
	rig.barrier()

	// Event 2 is a genuine update on the path event 1 was refused on.
	rig.emit("notes.md", rig.bind("notes.md", b), store.ChangeModified)
	if !rig.waitForDisk("notes.md", b, 10*time.Second) {
		t.Fatalf("REGRESSION: version B was dropped after a REFUSED echo on the same path. "+
			"The echo guard is latching rather than deciding per event, so a path stalls for as long as "+
			"echoes keep arriving on it. Disk holds %q.", string(rig.diskBytes("notes.md")))
	}
}

// -----------------------------------------------------------------------
// R4 — what the clock does NOT do, against a claim we published.
// -----------------------------------------------------------------------

// TestReverseWindow_WatcherIngestIsNotSuppressed refutes this repo's own
// published description of the defect.
//
// `docs/STATUS.md` §0H said the window "drops any filesystem event for a path
// the reverse writer touched in the last five seconds", so that "save your own
// edit to that path inside the window and it is discarded as an echo — silent
// data loss". Read the two directions apart and that is wrong in both its
// mechanism and its consequence: `isRecentlyWritten` is consulted in exactly
// one place, `reverseWriteLoop`, which handles TREE change events. The
// watcher's `flush` — the disk-to-tree direction, where a user's edit lives —
// never consults the tracker at all.
//
// So the user's edit is ingested. What is lost is the other direction. Per
// AP43 a refutation owes a probe that reaches the mechanism, so this drives a
// real watcher over a real inotify with the clock genuinely armed, rather
// than reading the source and asserting the conclusion.
func TestReverseWindow_WatcherIngestIsNotSuppressed(t *testing.T) {
	if testing.Short() {
		t.Skip("real inotify plus the watcher's 2s debounce")
	}
	rig := newReverseRig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rig.h.StartWatching(ctx, "rw", rig.cs, rig.li, hash.Hash{}); err != nil {
		t.Fatalf("StartWatching: %v", err)
	}

	a := []byte("version A, written to disk by the reverse writer\n")
	c := []byte("version C, typed by the operator inside the window\n")

	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeCreated)
	if !rig.waitForDisk("notes.md", a, 10*time.Second) {
		t.Fatalf("setup: version A never reached disk")
	}
	armed := time.Now() // the clock is armed for notes.md from here

	// The operator saves their own edit, inside the window.
	if err := os.WriteFile(filepath.Join(rig.dir, "notes.md"), c, 0o600); err != nil {
		t.Fatalf("operator edit: %v", err)
	}
	if elapsed := time.Since(armed); elapsed > 4*time.Second {
		t.Fatalf("the edit landed %v after the write — outside the 5s window, so this run proves nothing", elapsed)
	}

	// The claim under test is about ingest, so assert on the TREE: does the
	// binding at notes.md come to commit the operator's bytes?
	wantBlob := rig.ingest(c)
	deadline := time.Now().Add(25 * time.Second)
	var lastSeen hash.Hash
	for time.Now().Before(deadline) {
		if fh, ok := rig.li.Get(reverseWindowPrefix + "notes.md"); ok {
			if ent, ok := rig.cs.Get(fh); ok {
				if fd, err := localfiles.FileDataFromEntity(ent); err == nil {
					lastSeen = fd.Content
					if fd.Content == wantBlob {
						t.Logf("REFUTED (as published): the operator's edit was ingested %v after the reverse write, "+
							"well inside the 5s window. The watcher does not consult the tracker; only the tree-to-disk "+
							"direction does. STATUS §0H described the wrong direction.", time.Since(armed))
						return
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the operator's edit did NOT reach the tree within 25s (binding commits blob %v, wanted %v). "+
		"If this is reproducible, STATUS §0H's original reading was right after all and the packet needs rewriting.",
		lastSeen, wantBlob)
}
