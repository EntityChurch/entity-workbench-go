//go:build loadtest

package shellboot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
)

// IS THE BURST CLIFF A TUNING PROBLEM OR A STRUCTURAL ONE?
//
// Tier: real-session measurement (TESTING-STRATEGY §4), behind the
// `loadtest` tag. It ASSERTS almost nothing and REPORTS a table; a cost
// curve dressed as a threshold is a flaky gate on somebody else's
// machine.
//
// # Why this needs measuring rather than reasoning about
//
// The subscription engine's delivery queue drops on full. Its size is a
// local knob — core-go defaults to 65536 slots, our SDK to 4096 (the
// difference is memory: ~20.3 MB per peer eagerly allocated, and
// `subscription.Engine` has no Stop, so a process holding many peers
// never gets it back).
//
// So there are two very different stories, and they call for different
// answers at different layers:
//
//	TUNING     — the queue is simply too small for a realistic burst.
//	             Raise it and a 2000-file copy completes. The ecosystem
//	             question is then "what is the required minimum, and who
//	             says so", which is a specification question.
//
//	STRUCTURAL — writes outrun delivery, so any finite queue fills and
//	             the cliff just moves. The ecosystem question is then
//	             "the producer must be able to slow down", which is a
//	             protocol/kernel question and no amount of local
//	             configuration answers it.
//
// Everything we have written to architecture so far assumes the second.
// This test is the evidence for or against that, and it is cheap: sweep
// the knob, hold everything else fixed, count what arrives.
//
// **The supervisor is off in every arm.** It exists precisely to make
// this invisible, so leaving it on would measure the mitigation instead
// of the mechanism.
//
// Run: make loadtest ARGS="-run TestLoad_QueueDepthSweep -v"
//
//	LOAD_FILES=4000 make loadtest ARGS="-run TestLoad_QueueDepthSweep -v"
func TestLoad_QueueDepthSweep(t *testing.T) {
	nFiles := envInt("LOAD_FILES", 2000)
	fileBytes := envInt("LOAD_BYTES", 4096)

	// 0 means "take the SDK default" — the arm that reproduces what we
	// actually ship, and the baseline every other arm is read against.
	sizes := []int{0, 16384, 65536, 262144}
	if v := os.Getenv("LOAD_QUEUES"); v != "" {
		sizes = nil
		for _, f := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				t.Fatalf("LOAD_QUEUES: %q is not a number", f)
			}
			sizes = append(sizes, n)
		}
	}

	type arm struct {
		size     int
		arrived  int
		dropped  uint64
		bound    int
		stalled  bool
		duration time.Duration
	}
	var results []arm

	for _, size := range sizes {
		size := size
		label := fmt.Sprintf("queue=%d", size)
		if size == 0 {
			label = fmt.Sprintf("queue=default(%d)", entitysdk.DefaultDeliveryQueueSize)
		}
		t.Run(label, func(t *testing.T) {
			a := runQueueDepthArm(t, size, nFiles, fileBytes)
			results = append(results, arm{
				size: size, arrived: a.arrived, dropped: a.dropped,
				bound: a.bound, stalled: a.stalled, duration: a.duration,
			})
		})
	}

	// The table is the artifact. Printed at the end and in one place so
	// it can be pasted into a review packet without reassembly.
	t.Logf("")
	t.Logf("=== DELIVERY QUEUE DEPTH vs. BURST SURVIVAL (%d files x %d B, no supervisor) ===", nFiles, fileBytes)
	t.Logf("%-22s %10s %10s %12s %10s", "queue slots", "arrived", "of", "sender drops", "wall")
	for _, r := range results {
		slots := fmt.Sprintf("%d", r.size)
		if r.size == 0 {
			slots = fmt.Sprintf("%d (SDK default)", entitysdk.DefaultDeliveryQueueSize)
		}
		t.Logf("%-22s %10d %10d %12d %10s",
			slots, r.arrived, nFiles, r.dropped, r.duration.Round(time.Second))
	}
	t.Logf("")

	// The one thing worth asserting: the baseline arm must actually
	// saturate. If it does not, every other row in the table is being
	// read against a run that had no cliff in it, and the whole
	// measurement says nothing. Skip rather than report — this is
	// TestCatchUp's control-arm rule applied to a characterisation.
	if len(results) == 0 {
		t.Fatal("no arms ran")
	}
	if results[0].arrived == nFiles {
		t.Skipf("the %d-file burst did not saturate the default queue on this "+
			"machine (%d/%d arrived), so the sweep has no cliff to locate — "+
			"raise LOAD_FILES until it does", nFiles, results[0].arrived, nFiles)
	}
}

type queueArmResult struct {
	arrived  int
	dropped  uint64
	bound    int
	stalled  bool
	duration time.Duration
}

func runQueueDepthArm(t *testing.T, queueSize, nFiles, fileBytes int) queueArmResult {
	t.Helper()

	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newQueueSizedPeer(t, "sender", queueSize)
	receiver := newQueueSizedPeer(t, "receiver", queueSize)
	connectPeers(t, sender, receiver)

	const folder = "bulk"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)
	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

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

	body := make([]byte, fileBytes)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	start := time.Now()
	for i := 0; i < nFiles; i++ {
		p := filepath.Join(senderMount, fmt.Sprintf("f%05d.dat", i))
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	arrived, stalled := awaitArrival(t, receiverMount, nFiles, fileBytes, 15*time.Minute)
	dur := time.Since(start)

	dropped := sender.ap.SubscriptionEngine().DroppedDeliveries()
	bound := boundUnder(sender, "local/files/"+folder+"/")

	t.Logf("arrived %d/%d (stalled=%v) in %s · sender bound %d, dropped %d · receiver dropped %d",
		arrived, nFiles, stalled, dur.Round(time.Second), bound, dropped,
		receiver.ap.SubscriptionEngine().DroppedDeliveries())

	return queueArmResult{
		arrived: arrived, dropped: dropped, bound: bound,
		stalled: stalled, duration: dur,
	}
}

// newQueueSizedPeer is newSyncTestPeer with the delivery ring sized, and
// with the catch-up supervisor explicitly OFF.
//
// Off via a negative interval rather than by leaving ReconcileOnStart
// false, so this stays correct if the supervisor's default ever
// decouples from that flag — the arms have to measure the raw delivery
// path or they measure nothing.
func newQueueSizedPeer(t *testing.T, name string, queueSize int) *syncTestPeer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias:        name,
		ListenAddr:        "127.0.0.1:0",
		OpenAccess:        true,
		DeliveryQueueSize: queueSize,
		CatchUpInterval:   -1,
	})
	if err != nil {
		t.Fatalf("Bootstrap %s: %v", name, err)
	}
	t.Cleanup(func() { ws.StopCatchUp(); _ = ap.Close() })

	if _, running := ws.CatchUpInterval(); running {
		t.Fatalf("%s: the catch-up supervisor is running — this arm would "+
			"measure the mitigation instead of the mechanism", name)
	}
	return &syncTestPeer{ap: ap, ws: ws, id: ap.PeerID()}
}
