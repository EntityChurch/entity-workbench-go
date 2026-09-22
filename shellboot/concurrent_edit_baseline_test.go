package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// M3 BASELINE: what a concurrent same-path edit does TODAY.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # The question
//
// `DOMAIN-LOCAL-FILES` §1.1a — which this repo's own WB-25 closure
// produced — rules that a concurrent same-path write is NOT a CRDT case:
// last arrival wins at the filesystem surface, no automatic merge, and
// **both writes are recorded in the tree at distinct chain positions**.
//
// Nobody had checked whether the shipped code already does that. The
// milestone plan in `FILE-REPLICATION-LANDSCAPE.md` §6 assumes M3 has to
// build it. This test measures the baseline first, because the cost of
// M3 depends entirely on whether the tree-side guarantee is already met
// and only the SURFACE is missing.
//
// # What it does
//
//  1. Receiver takes delivery of a file (so both peers agree on it).
//  2. Receiver edits it locally — its own watcher ingests the edit.
//  3. Sender edits the same path — the delivery arrives and, per
//     blob_resolve.go's F9 branch, the hashes differ, so it overwrites.
//
// It then reads the receiver's own history at that path and reports the
// chain. It ASSERTS the two properties the spec names and nothing more;
// the rest is recorded with t.Log, because a measurement dressed as a
// check is how an undesigned behaviour gets frozen as a requirement.
func TestM3Baseline_ConcurrentEditLeavesBothWritesOnTheChain(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "shared"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)

	senderOut := mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")
	root := senderOut.RootName

	if _, err := receiver.ws.Sync(shellcmd.SyncRequest{Remote: sender.id, Root: root}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Recording is OPT-IN. The recorder tracks a path only when a
	// system/history/config matches it (ext/history/config.go, find()),
	// and NOTHING in the mount / sync / share path installs one — so the
	// chain §1.1a relies on is not being written for the one namespace
	// where it is the entire point. Measured: 0 transitions at a path
	// whose binding demonstrably exists.
	//
	// This test enables it explicitly, which is what makes the question
	// answerable at all; whether the product should enable it for a mount
	// is the finding, not a premise.
	enableHistoryFor(t, receiver, "local/files/*")

	const name = "contested.md"
	senderPath := filepath.Join(senderMount, name)
	receiverPath := filepath.Join(receiverMount, name)

	// 1 — both sides agree.
	const agreed = "shared starting point\n"
	if err := os.WriteFile(senderPath, []byte(agreed), 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if !awaitFileContent(receiverPath, agreed, 30*time.Second) {
		t.Fatal("precondition: the seed never reached the receiver")
	}

	seedChain := historyAt(t, receiver, "/"+receiver.id+"/local/files/"+root+"/"+name, 50)
	t.Logf("after the seed delivery, the receiver's chain has %d transitions", len(seedChain))
	for i, tr := range seedChain {
		t.Logf("  seed[%d] event=%s handler=%s:%s", i, tr.Event, tr.Handler, tr.Operation)
	}

	// 2 — the RECEIVER edits. Its own watcher ingests this, so the
	// receiver's tree gets a second chain position authored locally.
	const mine = "the receiver's own edit\n"
	if err := os.WriteFile(receiverPath, []byte(mine), 0o600); err != nil {
		t.Fatalf("receiver edit: %v", err)
	}
	if !awaitChainDepth(t, receiver, root, name, len(seedChain)+1, 15*time.Second) {
		after := historyAt(t, receiver, "/"+receiver.id+"/local/files/"+root+"/"+name, 50)
		t.Fatalf("the receiver's own edit never reached its tree: chain still %d "+
			"transitions (was %d after the seed). The watcher did not ingest a "+
			"local edit to a RECEIVED file.", len(after), len(seedChain))
	}

	// 3 — the SENDER edits the same path. Different blob hash, so F9
	// does not short-circuit and the write lands.
	const theirs = "the sender's conflicting edit\n"
	if err := os.WriteFile(senderPath, []byte(theirs), 0o600); err != nil {
		t.Fatalf("sender edit: %v", err)
	}
	if !awaitFileContent(receiverPath, theirs, 30*time.Second) {
		got, _ := os.ReadFile(receiverPath)
		t.Fatalf("the sender's edit never overwrote the receiver's (on disk: %q)", got)
	}

	// --- what the tree kept --------------------------------------------
	treePath := "/" + receiver.id + "/local/files/" + root + "/" + name
	trans := historyAt(t, receiver, treePath, 50)

	t.Logf("receiver chain at %s — %d transitions (most recent first):", treePath, len(trans))
	for i, tr := range trans {
		t.Logf("  [%d] event=%-8s hash=%s prev=%s handler=%s:%s",
			i, tr.Event, short(tr.Hash.String()), short(tr.PreviousHash.String()),
			tr.Handler, tr.Operation)
	}

	// ASSERTION 1 — last arrival wins at the filesystem surface. Already
	// established above by awaitFileContent; restated here because it is
	// half of what §1.1a rules and a reader of this test should see both.
	got, err := os.ReadFile(receiverPath)
	if err != nil || string(got) != theirs {
		t.Fatalf("last-arrival-wins violated: on disk %q, want %q (err %v)", got, theirs, err)
	}

	// ASSERTION 2 — both writes are on the chain at distinct positions.
	// Three positions minimum: the delivered seed, the receiver's own
	// edit, and the sender's conflicting edit.
	if len(trans) < 3 {
		t.Fatalf("the receiver's chain has %d transitions; §1.1a requires both writes "+
			"to be recorded at DISTINCT chain positions, so the seed, the local "+
			"edit and the incoming edit should all be present", len(trans))
	}

	// The receiver's own edit must still be reachable — that is what
	// makes the loss recoverable and what a keep-both or a three-way
	// merge would later be built on. It is present iff some transition
	// binds the hash the local edit produced.
	if !chainCarriesBody(t, receiver, trans, mine) {
		t.Errorf("the receiver's own edit is not recoverable from its chain — the "+
			"bytes %q appear at no chain position, so the overwrite was "+
			"destructive in the tree as well as on disk", mine)
	}

	// ASSERTION 3 — the chain says WHICH SIDE authored each position.
	//
	// This is the property M3's conflict detection is built on, and it
	// costs nothing because the recorder already writes it: a local edit
	// arrives through the WATCHER (`local/files:watch`), a delivered one
	// through blob_resolve's dispatch (`local/files:write`). Without a
	// discriminator, "the receiver edited this" and "the receiver is
	// behind" are the same observation, which is the distinction the
	// whole milestone turns on.
	//
	// Known limit, stated because it is invisible from the field name: a
	// LOCAL caller dispatching local/files:write directly is recorded as
	// a delivery. Nothing in the shipped flow does that today.
	if trans[0].Operation != "write" {
		t.Errorf("the winning position was authored by %s:%s, want local/files:write "+
			"(the delivered edit)", trans[0].Handler, trans[0].Operation)
	}
	if trans[1].Operation != "watch" {
		t.Errorf("the overwritten position was authored by %s:%s, want "+
			"local/files:watch (the receiver's own edit) — without this the two "+
			"sides of a conflict are indistinguishable on the chain",
			trans[1].Handler, trans[1].Operation)
	}

	// Everything below is MEASURED, not required. Whether an operator can
	// reach any of it is M3's actual question, and the answer today is no:
	// no surface reports that a delivery overwrote a local edit, and
	// nothing installs the history config this test had to install itself.
	t.Logf("MEASURED: the overwritten bytes are%s recoverable from the receiver's chain",
		map[bool]string{true: "", false: " NOT"}[chainCarriesBody(t, receiver, trans, mine)])
}

// --- helpers -------------------------------------------------------

func historyAt(t *testing.T, p *syncTestPeer, treePath string, limit uint64) []types.TransitionData {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := p.ap.History().Query(ctx, types.HistoryQueryParamsData{
		Path:  treePath,
		Limit: &limit,
	})
	if err != nil {
		t.Fatalf("history query at %s: %v", treePath, err)
	}
	return res.Transitions
}

// awaitChainDepth waits for the peer's own chain at a path to reach a
// depth. Settling on "the file changed on disk" would return before the
// watcher had ingested it, which is the window AP32 names.
func awaitChainDepth(t *testing.T, p *syncTestPeer, root, name string, want int, timeout time.Duration) bool {
	t.Helper()
	treePath := "/" + p.id + "/local/files/" + root + "/" + name
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(historyAt(t, p, treePath, 50)) >= want {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// chainCarriesBody reports whether any chain position binds a file
// entity whose blob reassembles to want.
func chainCarriesBody(t *testing.T, p *syncTestPeer, trans []types.TransitionData, want string) bool {
	t.Helper()
	for _, tr := range trans {
		if tr.Hash.IsZero() {
			continue
		}
		ent, ok := p.ap.RawContentStore().Get(tr.Hash)
		if !ok {
			continue
		}
		body, ok := readFileEntityBody(p, ent)
		if ok && body == want {
			return true
		}
	}
	return false
}

// readFileEntityBody reassembles the bytes a local-files file entity
// points at, from the peer's own content store.
func readFileEntityBody(p *syncTestPeer, ent entity.Entity) (string, bool) {
	if ent.Type != localfiles.TypeFile {
		return "", false
	}
	fd, err := localfiles.FileDataFromEntity(ent)
	if err != nil || fd.Content.IsZero() {
		return "", false
	}
	b, err := content.Reassemble(p.ap.RawContentStore(), fd.Content)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// enableHistoryFor installs a recording config through the same
// Store.Put the `history config` verb uses.
func enableHistoryFor(t *testing.T, p *syncTestPeer, pattern string) {
	t.Helper()
	cfg := types.HistoryConfigData{Pattern: pattern, Enabled: true}
	if _, err := p.ap.Store().Put("system/history/config/test", "system/history/config", cfg); err != nil {
		t.Fatalf("enable history for %q: %v", pattern, err)
	}
}

func short(s string) string {
	if len(s) > 16 {
		return s[:16] + "…"
	}
	return s
}
