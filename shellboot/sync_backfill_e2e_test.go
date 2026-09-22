package shellboot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// Backfill, end to end: the files that were ALREADY in the folder.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # The defect
//
// `Sync` subscribed with Events{"created","updated"} and nothing else,
// so a folder with files already in it transferred NOTHING — no file in
// it ever "changed" again. The operator gesture the product is named
// after (pick a directory, share it with a peer) therefore produced an
// empty folder and no error anywhere, and the shipped surfaces said so
// out loud: `sync` printed "files already in their mount arrive when
// they next change", and USAGE-SHARE-A-FOLDER.md listed history replay
// under what-this-does-not-cover.
//
// That the limitation was documented is why it lasted. It read as a
// known edge rather than as the feature being absent, and the sibling
// e2e test above encodes the same assumption in its comments — it
// writes its file AFTER the sync, deliberately, and so could never have
// failed on this.
//
// # Why the control arm is here and not implied
//
// `TestBackfill_ControlArm_WithoutItNothingArrives` runs the SAME
// scenario with SkipBackfill and asserts the folder stays empty. Per the
// discipline that a guard test must not be able to pass vacuously: if
// some other mechanism (a watcher rescan, a delivery on connect) also
// happened to carry these files, the positive test would pass while
// proving nothing about the code it is named after. The control fails
// if that ever becomes true, and it is the arm to read first when this
// pair starts disagreeing.
func TestBackfill_FilesAlreadyInTheFolderArrive(t *testing.T) {
	sender, receiver, senderMount, receiverMount, root := backfillFixture(t, false)

	syncOut, err := receiver.ws.Sync(shellcmd.SyncRequest{
		Remote: sender.id,
		Root:   root,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	if syncOut.BackfillSkipped {
		t.Fatal("Sync reported the backfill as skipped when it was not requested to skip")
	}
	bf := syncOut.Backfill
	if bf.Scanned != 4 {
		t.Errorf("backfill scanned %d remote files, want 4 — three at the top level and "+
			"one in sub/deeper/ (%s)", bf.Scanned, bf.Summary())
	}
	if bf.Materialized != 4 {
		t.Errorf("backfill materialized %d files, want 4 (%s); errors: %v",
			bf.Materialized, bf.Summary(), bf.Errors)
	}
	if bf.Failed != 0 {
		t.Errorf("backfill reported %d failures: %v", bf.Failed, bf.Errors)
	}
	if !bf.Complete() {
		t.Errorf("backfill reports incomplete: %s", bf.Summary())
	}

	// The claim that matters is bytes on disk, not a count in a struct.
	// The count could be right while the write path is broken, and that
	// failure would look like success in every surface we ship.
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("existing-%d.md", i)
		want := backfillBody(i)
		landed := filepath.Join(receiverMount, name)
		if !awaitFileContent(landed, want, 30*time.Second) {
			got, rerr := os.ReadFile(landed)
			t.Fatalf("%s never arrived at %s (read %q, err %v)\n  backfill: %s",
				name, landed, string(got), rerr, bf.Summary())
		}
	}

	// The nested one, at its nested path. Asserted separately from the
	// loop above because the interesting failure is not "missing" but
	// "arrived flattened into the mount root".
	nestedLanded := filepath.Join(receiverMount, "sub", "deeper", "nested.md")
	if !awaitFileContent(nestedLanded, nestedBody, 30*time.Second) {
		flat := filepath.Join(receiverMount, "nested.md")
		if _, err := os.Stat(flat); err == nil {
			t.Fatalf("the nested file arrived FLATTENED at %s — the relative path is "+
				"being lost between the remote walk and local/files:write", flat)
		}
		t.Fatalf("the file in sub/deeper/ never arrived at %s\n  backfill: %s",
			nestedLanded, bf.Summary())
	}

	// A second pass transfers nothing and says so. This is the operator's
	// only positive confirmation that a sync is current — "no errors" and
	// "nothing happened" are otherwise the same output.
	again, err := receiver.ws.Resync(sender.id, root)
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if again.Materialized != 0 {
		t.Errorf("resync re-transferred %d files; the F9 content-hash short-circuit "+
			"is not holding and every pass is re-pulling blobs", again.Materialized)
	}
	if again.AlreadyCurrent != 4 {
		t.Errorf("resync reported %d already-current, want 4 (%s)",
			again.AlreadyCurrent, again.Summary())
	}

	// The live half still works after a backfill — the two must not be
	// alternatives. A backfill that quietly consumed the subscription
	// would pass everything above.
	const later = "written after the sync\n"
	if err := os.WriteFile(filepath.Join(senderMount, "later.md"), []byte(later), 0o600); err != nil {
		t.Fatalf("write later file: %v", err)
	}
	if !awaitFileContent(filepath.Join(receiverMount, "later.md"), later, 30*time.Second) {
		t.Error("a file written after the backfill never arrived — the backfill " +
			"replaced the live subscription instead of complementing it")
	}
}

// TestBackfill_ControlArm_WithoutItNothingArrives is the arm that keeps
// the test above honest. Same fixture, backfill switched off, and the
// folder must stay empty — if it does not, something ELSE is carrying
// these files and the positive assertion above is measuring that
// instead.
func TestBackfill_ControlArm_WithoutItNothingArrives(t *testing.T) {
	sender, receiver, _, receiverMount, root := backfillFixture(t, false)
	_ = sender

	out, err := receiver.ws.Sync(shellcmd.SyncRequest{
		Remote:       sender.id,
		Root:         root,
		SkipBackfill: true,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !out.BackfillSkipped {
		t.Fatal("SkipBackfill was requested and the outcome does not report it")
	}
	if out.Backfill.Scanned != 0 {
		t.Fatalf("SkipBackfill still scanned %d files", out.Backfill.Scanned)
	}

	// Bounded negative assertion. Deliberately short: this arm is not
	// trying to prove nothing arrives ever, only that the pre-existing
	// files do not arrive by some route other than the backfill within
	// the window the positive test comfortably completes in.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(receiverMount, "existing-1.md")); err == nil {
			t.Fatal("a pre-existing file arrived with the backfill disabled — some " +
				"other mechanism is replaying history, and the backfill test above " +
				"is therefore not measuring the backfill")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

const nestedBody = "# nested\n\nIn sub/deeper/, before the sync.\n"

func backfillBody(i int) string {
	return fmt.Sprintf("# existing file %d\n\nThis was in the folder BEFORE the sync.\n", i)
}

// backfillFixture builds two bootstrapped peers with three files
// already present in the sender's folder at mount time.
//
// The order is the point: files land on disk FIRST, then the mount, so
// they enter the sender's tree through the watcher's initial scan and
// are never the subject of a change event afterwards. A fixture that
// wrote them after mounting would generate exactly the created events
// the subscription already handled, and the test would pass against the
// unfixed code.
func backfillFixture(t *testing.T, _ bool) (
	sender, receiver *syncTestPeer, senderMount, receiverMount, root string,
) {
	t.Helper()
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender = newSyncTestPeer(t, "sender")
	receiver = newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "shared"
	senderMount = filepath.Join(senderDir, folder)
	receiverMount = filepath.Join(receiverDir, folder)
	for _, d := range []string{senderMount, receiverMount} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	for i := 1; i <= 3; i++ {
		p := filepath.Join(senderMount, fmt.Sprintf("existing-%d.md", i))
		if err := os.WriteFile(p, []byte(backfillBody(i)), 0o600); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	// A file in a SUBDIRECTORY. The remote enumeration is a
	// breadth-first walk that has to descend into child nodes, and a
	// walk that silently returned only the top level would still pass
	// every other assertion in this file — an operator with one nested
	// folder would get a partial transfer reported as a complete one,
	// which is the original defect wearing a smaller hat.
	nested := filepath.Join(senderMount, "sub", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.md"),
		[]byte(nestedBody), 0o600); err != nil {
		t.Fatalf("seed nested: %v", err)
	}

	senderOut, err := sender.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: senderMount,
		TargetPrefix:  "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	receiverOut, err := receiver.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: receiverMount,
		TargetPrefix:  "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("receiver mount: %v", err)
	}
	if senderOut.RootName != receiverOut.RootName {
		t.Fatalf("mounts derived different roots (%q vs %q)",
			senderOut.RootName, receiverOut.RootName)
	}
	root = senderOut.RootName

	// Wait for the sender's own tree to carry the three files. Without
	// this the backfill can legitimately find nothing and the test would
	// be measuring the watcher's scan latency rather than the backfill.
	prefix := "local/files/" + root + "/"
	deadline := time.Now().Add(30 * time.Second)
	for {
		// >= 4 direct children is the wrong test now that one file is
		// nested: `List` returns DIRECT children, so the top level has
		// three files plus the `sub` node. Four is still the right
		// number, for a different reason — say so, or the next person
		// changes the seed count and cannot tell which four.
		entries, err := sender.ap.List(prefix)
		if err == nil && len(entries) >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sender's watcher never ingested the seeded files under %s "+
				"(last err: %v)", prefix, err)
		}
		time.Sleep(150 * time.Millisecond)
	}

	return sender, receiver, senderMount, receiverMount, root
}
