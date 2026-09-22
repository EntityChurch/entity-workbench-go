package shellboot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// TWO-WAY: both peers mount a folder, both share it, both sync it, and
// each side's pre-existing files end up on the other.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why this test exists
//
// The operator's model of the product is Dropbox: "this is the shared
// folder", on two machines, and the contents converge. Everything the
// repo had gated was ONE direction — `sync_e2e_test.go` establishes
// A→B and asserts a file written on A lands on B. Two-way was
// *supported* (the F9 idempotency short-circuit in
// `workbench/blob_resolve.go` exists specifically to stop the
// notification loop the "bidirectional symmetric topology" creates) and
// exercised only by `shellcmd/cmd_local_files_bidirectional_test.go`,
// which hand-assembles the chain rather than going through the verbs.
//
// So the thing an operator actually wants was the composition of two
// tested halves, and nothing measured the composition. This does.
//
// # What it deliberately does NOT establish
//
// `newSyncTestPeer` bootstraps with `OpenAccess: true`. Per AP63 that is
// a wildcard grant, so this file says **nothing** about the permission
// stage — it measures the data path only. The permission stage is gated
// without wildcards in `shellboot/flow_e2e_test.go`, and the two must
// not be confused: a green run here does not mean an operator's share
// will authorize.
func TestTwoWay_BothFoldersConverge(t *testing.T) {
	alice, bob, aliceDir, bobDir, root := twoWayFixture(t)

	// Each side receives the other's folder. This is the whole of
	// "two-way": there is no separate bidirectional verb, it is Sync
	// run once in each direction, and each call backfills what the
	// other side already had.
	aOut, err := alice.ws.Sync(shellcmd.SyncRequest{Remote: bob.id, Root: root})
	if err != nil {
		t.Fatalf("alice sync from bob: %v", err)
	}
	bOut, err := bob.ws.Sync(shellcmd.SyncRequest{Remote: alice.id, Root: root})
	if err != nil {
		t.Fatalf("bob sync from alice: %v", err)
	}

	// The scanned COUNT is not deterministic here, and the first
	// version of this test asserted that it was — measured, bob's
	// backfill scanned 4 where the obvious expectation is 2.
	//
	// That is correct behaviour and worth writing down, because it is
	// the two-way case's one genuinely counter-intuitive property. By
	// the time bob syncs, alice's own backfill has already pulled bob's
	// two files into alice's `local/files/{root}/`. Bob then enumerates
	// alice's folder and legitimately sees four files — two of alice's
	// and two of his own, round-tripped. The reported outcome was
	// "4 file(s) found, 2 transferred, 2 already current": the F9
	// content-hash short-circuit recognised his own bytes and did not
	// pull them back. That is the loop guard doing its job, visible in
	// a count.
	//
	// So the assertion is on what must hold rather than on a number
	// that depends on which sync won a race: each side sees at least
	// the other's two files, and nothing failed.
	if aOut.Backfill.Scanned < 2 || aOut.Backfill.Failed != 0 {
		t.Errorf("alice's backfill: %s (errors %v)",
			aOut.Backfill.Summary(), aOut.Backfill.Errors)
	}
	if bOut.Backfill.Scanned < 2 || bOut.Backfill.Failed != 0 {
		t.Errorf("bob's backfill: %s (errors %v)",
			bOut.Backfill.Summary(), bOut.Backfill.Errors)
	}
	// Whatever the race, no file may be pulled twice: the total
	// materialized across both directions is bounded by the four
	// distinct files that exist. A regression in the F9 short-circuit
	// shows up here as 5+ before it shows up as churn.
	if total := aOut.Backfill.Materialized + bOut.Backfill.Materialized; total > 4 {
		t.Errorf("the two backfills materialized %d files between them for 4 distinct "+
			"files — the content-hash short-circuit is not holding", total)
	}

	// Convergence: every seeded file is on both machines.
	for _, f := range []struct{ name, body string }{
		{"alice-1.md", twoWayBody("alice", 1)},
		{"alice-2.md", twoWayBody("alice", 2)},
		{"bob-1.md", twoWayBody("bob", 1)},
		{"bob-2.md", twoWayBody("bob", 2)},
	} {
		for who, dir := range map[string]string{"alice": aliceDir, "bob": bobDir} {
			p := filepath.Join(dir, f.name)
			if !awaitFileContent(p, f.body, 30*time.Second) {
				got, rerr := os.ReadFile(p)
				t.Fatalf("%s is missing %s on %s's disk (read %q, err %v)\n"+
					"  alice backfill: %s\n  bob backfill:   %s",
					who, f.name, who, string(got), rerr,
					aOut.Backfill.Summary(), bOut.Backfill.Summary())
			}
		}
	}

	// Live, both directions, after the backfill. A catch-up that
	// consumed the subscription would pass everything above.
	if err := os.WriteFile(filepath.Join(aliceDir, "live-from-alice.md"),
		[]byte("live a\n"), 0o600); err != nil {
		t.Fatalf("write live file on alice: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bobDir, "live-from-bob.md"),
		[]byte("live b\n"), 0o600); err != nil {
		t.Fatalf("write live file on bob: %v", err)
	}
	if !awaitFileContent(filepath.Join(bobDir, "live-from-alice.md"), "live a\n", 30*time.Second) {
		t.Error("alice→bob live delivery stopped working after the backfill")
	}
	if !awaitFileContent(filepath.Join(aliceDir, "live-from-bob.md"), "live b\n", 30*time.Second) {
		t.Error("bob→alice live delivery stopped working after the backfill")
	}

	// The loop guard. In a symmetric topology each write fires a tree
	// event the other peer's subscription observes, which dispatches a
	// blob-resolve that writes the same path with the same hash — and
	// without the F9 content-hash short-circuit that runs unbounded at
	// ~150 iterations/sec. Settle, then assert the file count is stable
	// rather than growing: a live loop shows up as churn, not as a
	// wrong byte, so no content assertion above can see it.
	time.Sleep(3 * time.Second)
	countA, countB := countFiles(t, aliceDir), countFiles(t, bobDir)
	time.Sleep(3 * time.Second)
	if a2 := countFiles(t, aliceDir); a2 != countA {
		t.Errorf("alice's folder is still churning: %d then %d", countA, a2)
	}
	if b2 := countFiles(t, bobDir); b2 != countB {
		t.Errorf("bob's folder is still churning: %d then %d", countB, b2)
	}
	if countA != 6 || countB != 6 {
		t.Errorf("converged folders differ: alice=%d bob=%d, want 6 each", countA, countB)
	}

	// A resync on a converged pair must transfer nothing. This is the
	// operator's "did it work" check, and on a two-way pair it is also
	// the check that the two directions are not re-pushing each other's
	// files back and forth beneath a stable file count.
	again, err := alice.ws.Resync(bob.id, root)
	if err != nil {
		t.Fatalf("alice resync: %v", err)
	}
	if again.Materialized != 0 {
		t.Errorf("resync on a converged folder transferred %d files, want 0 (%s)",
			again.Materialized, again.Summary())
	}
}

func twoWayBody(who string, i int) string {
	return fmt.Sprintf("# %s file %d\n\nSeeded before either sync existed.\n", who, i)
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

// twoWayFixture seeds BOTH folders before either mount, so neither
// side's files are ever the subject of a change event and both
// directions depend on the backfill.
func twoWayFixture(t *testing.T) (alice, bob *syncTestPeer, aliceDir, bobDir, root string) {
	t.Helper()
	aliceHome, bobHome := t.TempDir(), t.TempDir()

	alice = newSyncTestPeer(t, "alice")
	bob = newSyncTestPeer(t, "bob")
	connectPeers(t, alice, bob)

	const folder = "shared"
	aliceDir = filepath.Join(aliceHome, folder)
	bobDir = filepath.Join(bobHome, folder)
	for _, d := range []string{aliceDir, bobDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	for i := 1; i <= 2; i++ {
		if err := os.WriteFile(filepath.Join(aliceDir, fmt.Sprintf("alice-%d.md", i)),
			[]byte(twoWayBody("alice", i)), 0o600); err != nil {
			t.Fatalf("seed alice: %v", err)
		}
		if err := os.WriteFile(filepath.Join(bobDir, fmt.Sprintf("bob-%d.md", i)),
			[]byte(twoWayBody("bob", i)), 0o600); err != nil {
			t.Fatalf("seed bob: %v", err)
		}
	}

	aOut, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	bOut, err := bob.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: bobDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("bob mount: %v", err)
	}
	if aOut.RootName != bOut.RootName {
		t.Fatalf("mounts derived different roots (%q vs %q)", aOut.RootName, bOut.RootName)
	}
	root = aOut.RootName

	// Both watchers must have ingested their own seeds before either
	// sync, or a backfill legitimately finds nothing and this measures
	// scan latency instead of convergence.
	prefix := "local/files/" + root + "/"
	deadline := time.Now().Add(30 * time.Second)
	for {
		ae, aerr := alice.ap.List(prefix)
		be, berr := bob.ap.List(prefix)
		if aerr == nil && berr == nil && len(ae) >= 2 && len(be) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watchers never ingested seeds (alice err %v, bob err %v)", aerr, berr)
		}
		time.Sleep(150 * time.Millisecond)
	}
	return alice, bob, aliceDir, bobDir, root
}
