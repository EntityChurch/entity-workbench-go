package workbench

// M1 reproducer — ext/localfiles' reverse-write loop guard is a clock, and
// the clock drops writes the content check would have delivered.
//
// WHAT THIS FILE IS. It is a reproducer for a defect in a SIBLING repo
// (`entity-core-go`, `ext/localfiles/reverse.go`), kept here because
// `AGENTS.md` says a routed ask owes a failing probe and because per AP43 a
// probe that does not reach the mechanism refutes nothing. Nothing in this
// file tests workbench code. It drives core-go's real handler, real root
// mapping, real reverse-write loop and real filesystem, with a hand-fed tree
// event channel standing in for the notifying store's fan-out — which is the
// same seam `ext/localfiles/handler_test.go`'s own `TestReverseWrite` uses.
//
// THE MECHANISM, cited so a reader can check it rather than trust it
// (`entity-core-go` `ext/localfiles/reverse.go`, symbols not line numbers
// because their history is republished):
//
//   - `reverseTracker` records `path -> time.Now()` in `markWritten`.
//   - `reverseWriteLoop` drops any tree change event whose bare path is
//     `isRecentlyWritten` — i.e. was written by us inside
//     `recentWriteWindow = 5 * time.Second`. The event is DISCARDED, not
//     deferred: there is no queue, no retry, and no second look.
//   - `reverseWrite` separately refuses to write when the on-disk bytes
//     already hash to the incoming blob (`currentDiskBlobHash == fileData.Content`).
//     That is a genuine content-identity circuit breaker and it sits BEFORE
//     `markWritten` in the same function.
//
// The ordering in that last bullet is the whole finding, and TestReverseWindow_
// TheClockEngagesOnlyAfterARealWrite is what establishes it: the tracker can
// only be armed by a call that got PAST the content check, so on the echo path
// the content check has already fired and the clock is never consulted. The
// clock therefore suppresses events the content check would have suppressed
// anyway — plus the ones below, which it should not have.
//
// EACH TEST ASSERTS THE DEFECT. That is deliberate. If one of these fails
// because the substrate now does the right thing, the substrate has been
// fixed: close the matching row in
// `docs/architecture/reviews/CORE-GO-TRACKER.md` and delete the test. Every
// failure message says so.
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

// TestReverseWindow_SuppressesGenuinelyNewContent is the core reproducer.
//
// Two updates land on one path inside five seconds. The first is written to
// disk and arms the clock; the second carries DIFFERENT bytes and is dropped
// as an echo it is not. Tree and disk diverge, silently, with no error on any
// channel.
//
// This is not an exotic shape. It is what a file edited twice in quick
// succession upstream looks like at the consuming peer, which is exactly the
// traffic M2 (two machines, one folder) is made of.
func TestReverseWindow_SuppressesGenuinelyNewContent(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("version A — the reverse writer put this on disk\n")
	b := []byte("version B — a genuinely different update to the same path\n")

	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeCreated)
	if !rig.waitForDisk("notes.md", a, 10*time.Second) {
		t.Fatalf("setup: version A never reached disk; the rig is wrong, not the substrate")
	}
	// The clock is armed for notes.md as of this instant.

	rig.emit("notes.md", rig.bind("notes.md", b), store.ChangeModified)
	rig.barrier()

	got := string(rig.diskBytes("notes.md"))
	if got == string(b) {
		t.Fatalf("SUBSTRATE FIXED: version B reached disk inside the 5s window. " +
			"The reverse-write clock no longer drops genuine updates — close row 2 in " +
			"docs/architecture/reviews/CORE-GO-TRACKER.md and delete this file.")
	}
	if got != string(a) {
		t.Fatalf("disk holds neither A nor B (%q) — the rig has drifted; re-derive the mechanism before routing anything", got)
	}

	// Say the divergence out loud rather than only asserting it, so a failing
	// run in someone else's CI carries the finding and not just a diff.
	t.Logf("REPRODUCED: tree binding at %snotes.md commits version B; disk holds version A. "+
		"No error was returned, logged, or queued for retry.", reverseWindowPrefix)
}

// TestReverseWindow_SuppressionIsPermanent establishes the half that makes
// the drop a data-loss bug rather than a latency bug: nothing re-drives it.
//
// The event is consumed and discarded. There is no deferred queue, so waiting
// out the window changes nothing — the disk stays stale until some unrelated
// future write to that path happens to arrive outside a window. The second
// leg re-emits the identical event after expiry and it lands, which proves
// the drop was the CLOCK and not the content, the entity, or the path.
func TestReverseWindow_SuppressionIsPermanent(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 5s recentWriteWindow")
	}
	rig := newReverseRig(t)

	a := []byte("version A\n")
	b := []byte("version B\n")

	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeCreated)
	if !rig.waitForDisk("notes.md", a, 10*time.Second) {
		t.Fatalf("setup: version A never reached disk")
	}

	fhB := rig.bind("notes.md", b)
	rig.emit("notes.md", fhB, store.ChangeModified)
	rig.barrier()

	// Wait past the window with no further events. If a retry existed, this
	// is where it would fire.
	time.Sleep(6 * time.Second)
	rig.barrier()

	if got := string(rig.diskBytes("notes.md")); got != string(a) {
		t.Fatalf("SUBSTRATE CHANGED: after the window expired the disk holds %q, not the stale version A. "+
			"Something re-drives suppressed events now — re-read reverseWriteLoop and update the packet.", got)
	}
	t.Logf("REPRODUCED: 6s after the drop, with the window long expired, the disk is still stale. The event was discarded, not deferred.")

	// Same event, same bytes, same entity hash — now outside the window.
	rig.emit("notes.md", fhB, store.ChangeModified)
	if !rig.waitForDisk("notes.md", b, 10*time.Second) {
		t.Fatalf("the identical event was dropped OUTSIDE the window too — the cause is not the clock; re-derive before routing")
	}
	t.Logf("CONTROL: the identical event delivered once the window expired. The only difference between delivery and loss is wall-clock time.")
}

// -----------------------------------------------------------------------
// R2 — a delete inside the window is discarded, and nothing else catches it.
// -----------------------------------------------------------------------

// TestReverseWindow_SuppressesDelete is the sharper half of the same defect.
//
// A write inside the window is at least covered on paper by the content check
// three lines below the clock. A DELETE is not: `reverseWriteLoop` routes
// `ChangeDeleted` to `reverseDelete` before any content is fetched, and
// `reverseDelete` has no circuit breaker of its own. So the clock is the only
// thing standing between a tree deletion and the filesystem, and inside the
// window it stops it permanently.
//
// Result: the tree says the file is gone, the disk says it is there, and the
// next watcher scan is entitled to ingest it back — a deletion that undoes
// itself.
func TestReverseWindow_SuppressesDelete(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("this file is about to be deleted upstream\n")
	rig.emit("doomed.md", rig.bind("doomed.md", a), store.ChangeCreated)
	if !rig.waitForDisk("doomed.md", a, 10*time.Second) {
		t.Fatalf("setup: the file never reached disk")
	}

	// Delete it from the tree, inside the window.
	if _, ok := rig.li.Remove(reverseWindowPrefix + "doomed.md"); !ok {
		t.Fatalf("setup: nothing bound at %sdoomed.md to remove", reverseWindowPrefix)
	}
	rig.emit("doomed.md", hash.Hash{}, store.ChangeDeleted)
	rig.barrier()

	if _, err := os.Stat(filepath.Join(rig.dir, "doomed.md")); err != nil {
		t.Fatalf("SUBSTRATE FIXED: the delete reached disk inside the window (%v). "+
			"Close row 2 in docs/architecture/reviews/CORE-GO-TRACKER.md and delete this test.", err)
	}
	t.Logf("REPRODUCED: the tree no longer binds %sdoomed.md and the file is still on disk. "+
		"reverseDelete has no content check, so nothing downstream of the clock can catch this one.", reverseWindowPrefix)
}

// -----------------------------------------------------------------------
// R3 — the control arm: what the clock is actually buying.
// -----------------------------------------------------------------------

// TestReverseWindow_ContentIdentityAlreadyStopsTheEcho is the control that
// makes the ask actionable rather than a complaint.
//
// The echo the clock exists to stop — our own disk write coming back around
// as a tree event — is already stopped by content identity. Here the file is
// put on disk directly, so `markWritten` is never called for it (the only two
// call sites are `reverseWrite`, past its content check, and `handleWrite`,
// which this test does not invoke). The clock is therefore provably not
// involved, and the event is still refused, by `currentDiskBlobHash ==
// fileData.Content`.
//
// If the echo is caught with the clock disarmed, the clock is not the guard.
func TestReverseWindow_ContentIdentityAlreadyStopsTheEcho(t *testing.T) {
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
		t.Fatalf("the echo was rewritten (mtime %v -> %v) with the clock disarmed — "+
			"the content circuit breaker did NOT catch it, which invalidates the packet's central claim. Re-derive before routing.",
			before.ModTime(), after.ModTime())
	}
	t.Logf("CONTROL: the echo was refused with the clock provably disarmed for this path. " +
		"The guard is currentDiskBlobHash == fileData.Content, not recentWriteWindow.")
}

// TestReverseWindow_TheClockEngagesOnlyAfterARealWrite pins the ordering the
// whole argument rests on: inside `reverseWrite`, the content check returns
// BEFORE `markWritten` is reached.
//
// Consequence — the clock cannot arm on an echo, because an echo never gets
// past the content check. It arms only after a real tree-to-disk write, which
// is precisely the moment when the NEXT event on that path is most likely to
// be a genuine follow-up update rather than an echo.
//
// The differential against TestReverseWindow_SuppressesGenuinelyNewContent is
// the evidence: identical second event, opposite outcome, and the only thing
// that differs is whether the first event caused a write.
func TestReverseWindow_TheClockEngagesOnlyAfterARealWrite(t *testing.T) {
	rig := newReverseRig(t)

	a := []byte("version A\n")
	b := []byte("version B\n")

	// Disk already holds A, so event 1 hits the content check and returns
	// without arming the clock.
	if err := os.WriteFile(filepath.Join(rig.dir, "notes.md"), a, 0o600); err != nil {
		t.Fatalf("seed disk: %v", err)
	}
	rig.emit("notes.md", rig.bind("notes.md", a), store.ChangeModified)
	rig.barrier()

	// Event 2 is the same event that was lost in the other test, at the same
	// distance from event 1 — well inside five seconds.
	rig.emit("notes.md", rig.bind("notes.md", b), store.ChangeModified)
	if !rig.waitForDisk("notes.md", b, 10*time.Second) {
		t.Fatalf("SUBSTRATE CHANGED: version B was dropped even though event 1 did not write. "+
			"markWritten is reachable without a write now — re-read reverseWrite and update the packet. Disk holds %q.",
			string(rig.diskBytes("notes.md")))
	}
	t.Logf("ESTABLISHED: markWritten sits downstream of the content check. An echo cannot arm the clock; " +
		"only a real write can, so the clock's five seconds always start at the exact moment a genuine follow-up is most likely.")
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
