//go:build loadtest

package shellboot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// THE SCENARIO AN OPERATOR ACTUALLY HITS: turn on a share, drop a big
// directory in, and see whether it finishes.
//
// Tier: real-session (TESTING-STRATEGY §4), outside `test-each` behind
// the `loadtest` tag because it is minutes and it is a MEASUREMENT
// rather than a pass/fail claim about the tree.
//
// Every sync test in this repo moves one to ten files. The load
// question — does it complete, how long, what does it cost in the
// store, and does it RECOVER if it is interrupted — had never been
// asked, and "it crapped out half way and did not resume" is the
// failure an operator reports rather than a stack trace.
//
// Run: make loadtest             (defaults to 2000 files × 4 KiB)
//
//	LOAD_FILES=10000 make loadtest
//	LOAD_BYTES=65536 make loadtest
//
// It asserts only the things that are genuinely required — every file
// arrives with the right bytes, and the run makes progress — and
// REPORTS the rest. A cost measurement dressed as a threshold turns
// into a flaky gate on somebody's laptop.
func TestLoad_BigDirectoryThroughAShare(t *testing.T) {
	nFiles := envInt("LOAD_FILES", 2000)
	fileBytes := envInt("LOAD_BYTES", 4096)

	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "bulk"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)
	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

	// Through the shipped flow, so the reconciler's history config is in
	// play — the cost of that is part of what this measures.
	if _, err := sender.ws.Share(shellcmd.ShareRequest{Root: folder, Peer: receiver.id}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := receiver.ws.Accept(shellcmd.AcceptRequest{
		Peer: sender.id, Root: folder, Directory: receiverMount, AllowNonEmpty: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := receiver.ws.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	before := snapshot(sender, receiver)
	t.Logf("before: %s", before)

	// Write the directory. Not through any verb — this is an operator
	// dropping files in with their file manager, which is the whole
	// point of a mounted folder.
	body := make([]byte, fileBytes)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	writeStart := time.Now()
	for i := 0; i < nFiles; i++ {
		p := filepath.Join(senderMount, fmt.Sprintf("f%05d.dat", i))
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	writeDur := time.Since(writeStart)
	t.Logf("wrote %d files × %d B in %s (%.0f files/s) on the sender's disk",
		nFiles, fileBytes, writeDur.Round(time.Millisecond),
		float64(nFiles)/writeDur.Seconds())

	// Wait for arrival, reporting progress — and, critically, reporting
	// whether progress is still being MADE. A run that stalls at 1400 of
	// 2000 and a run that is merely slow look identical at the end; the
	// difference is the whole question.
	arrived, stalled := awaitArrival(t, receiverMount, nFiles, fileBytes, 20*time.Minute)

	after := snapshot(sender, receiver)
	t.Logf("after:  %s", after)
	t.Logf("delta:  sender +%d entities / +%d paths · receiver +%d entities / +%d paths",
		after.sEnt-before.sEnt, after.sPath-before.sPath,
		after.rEnt-before.rEnt, after.rPath-before.rPath)
	if nFiles > 0 {
		t.Logf("PER FILE: sender %.1f entities, receiver %.1f entities",
			float64(after.sEnt-before.sEnt)/float64(nFiles),
			float64(after.rEnt-before.rEnt)/float64(nFiles))
	}

	// THE CAUSE, read from the kernel's own counter. The subscription
	// engine's delivery shards are bounded and DROP on full, by design
	// (ext/subscription/engine.go, OnTreeChange: `default:
	// e.droppedDeliveries.Add(1)`), counting the drops so an operator can
	// see saturation. Nothing in this repo has ever read that counter.
	t.Logf("KERNEL COUNTERS — sender: dropped=%d depth=%d · receiver: dropped=%d depth=%d",
		sender.ap.SubscriptionEngine().DroppedDeliveries(),
		sender.ap.SubscriptionEngine().DeliveryQueueDepth(),
		receiver.ap.SubscriptionEngine().DroppedDeliveries(),
		receiver.ap.SubscriptionEngine().DeliveryQueueDepth())

	// Where did they go? The sender's tree is the upstream truth; if it
	// is short too, the loss is on the WATCH side and not the wire.
	sBound := boundUnder(sender, "local/files/"+folder+"/")
	rBound := boundUnder(receiver, "local/files/"+folder+"/")
	t.Logf("bound file entities: sender %d, receiver %d (files on receiver's disk: %d)",
		sBound, rBound, arrived)

	if arrived == nFiles && !stalled {
		return
	}

	// --- THE OPERATOR'S REMEDY -----------------------------------------
	//
	// `resync` exists for exactly this: re-run the catch-up without
	// touching the subscription. If a stalled copy cannot be finished by
	// the one verb documented to finish it, the failure is not "slow",
	// it is "stuck with no way out", which is a different product.
	t.Logf("--- stalled at %d/%d; running resync, which is the documented remedy ---",
		arrived, nFiles)
	resyncStart := time.Now()
	res, err := receiver.ws.Resync(sender.id, folder)
	if err != nil {
		t.Fatalf("resync itself failed: %v", err)
	}
	t.Logf("resync returned in %s: %s", time.Since(resyncStart).Round(time.Millisecond), res.Summary())

	recovered, stalledAgain := awaitArrival(t, receiverMount, nFiles, fileBytes, 10*time.Minute)
	t.Logf("after resync: %d/%d files (stalled again: %v)", recovered, nFiles, stalledAgain)
	t.Logf("bound file entities after resync: sender %d, receiver %d",
		boundUnder(sender, "local/files/"+folder+"/"),
		boundUnder(receiver, "local/files/"+folder+"/"))

	// THE PRICE OF A SELF-HEALING LOOP. A catch-up that runs on a timer
	// is only affordable if a pass over a folder with nothing to do is
	// cheap. Measured here rather than assumed, because it is the one
	// number that decides whether periodic catch-up is a design or a
	// regression.
	noopStart := time.Now()
	noop, nerr := receiver.ws.Resync(sender.id, folder)
	if nerr == nil {
		t.Logf("NO-OP RESYNC over %d files: %s in %s (%.0f files/s)",
			nFiles, noop.Summary(), time.Since(noopStart).Round(time.Millisecond),
			float64(nFiles)/time.Since(noopStart).Seconds())
	}

	if recovered != nFiles {
		t.Errorf("STALLED at %d of %d and RESYNC DID NOT RECOVER IT (%d of %d) — "+
			"the copy stops part way, nothing says so, and the documented remedy "+
			"does not finish it.", arrived, nFiles, recovered, nFiles)
		return
	}
	t.Logf("MEASURED: the raw subscription path stalled at %d of %d; resync recovered "+
		"it in full. This is characterisation, not a defect claim — the shipped "+
		"product runs the catch-up supervisor, which is gated by "+
		"TestCatchUp_HealsASilentlyStalledShare.", arrived, nFiles)
}

// boundUnder counts file entities bound under a tree prefix on a peer,
// read from the location index rather than from any surface.
func boundUnder(p *syncTestPeer, relPrefix string) int {
	return len(p.ap.RawLocationIndex().List("/" + p.id + "/" + relPrefix))
}

// --- helpers -------------------------------------------------------

type counts struct {
	sEnt, sPath, rEnt, rPath int
}

func (c counts) String() string {
	return fmt.Sprintf("sender %d entities / %d paths · receiver %d entities / %d paths",
		c.sEnt, c.sPath, c.rEnt, c.rPath)
}

func snapshot(sender, receiver *syncTestPeer) counts {
	return counts{
		sEnt:  sender.ap.EntityCount(),
		sPath: sender.ap.PathCount(),
		rEnt:  receiver.ap.EntityCount(),
		rPath: receiver.ap.PathCount(),
	}
}

// awaitArrival polls the receiving directory and returns how many files
// are fully present, and whether it gave up because progress stopped.
//
// "Stopped" and not "timed out": a big copy is legitimately slow, so a
// wall-clock deadline alone cannot tell a stall from a slow run. The
// stall detector is what makes this test able to report the operator's
// actual complaint.
func awaitArrival(t *testing.T, dir string, want, size int, timeout time.Duration) (int, bool) {
	t.Helper()
	const stallWindow = 90 * time.Second
	deadline := time.Now().Add(timeout)
	lastCount, lastProgress := 0, time.Now()
	start := time.Now()
	nextReport := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		n := countComplete(dir, size)
		if n >= want {
			t.Logf("ALL %d files arrived in %s (%.0f files/s end to end)",
				want, time.Since(start).Round(time.Millisecond),
				float64(want)/time.Since(start).Seconds())
			return n, false
		}
		if n > lastCount {
			lastCount, lastProgress = n, time.Now()
		}
		if time.Now().After(nextReport) {
			t.Logf("  ... %d/%d after %s", n, want, time.Since(start).Round(time.Second))
			nextReport = time.Now().Add(15 * time.Second)
		}
		if time.Since(lastProgress) > stallWindow {
			t.Logf("no new file in %s — treating as stalled at %d/%d after %s",
				stallWindow, lastCount, want, time.Since(start).Round(time.Second))
			return lastCount, true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return countComplete(dir, size), false
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
