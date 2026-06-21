package shellcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestE2E_CollaborativeDeletion verifies that a fs-level delete on
// one peer propagates end-to-end:
//
//   - The deleting peer removes both source (local/files/shared/<f>)
//     and target (archives/notes/<f>) tree entries.
//   - The other peer's fs unlinks the file.
//   - The other peer's source + target tree entries are removed.
//
// Phase E v2 §7.2 — wire "deleted" events through notification-ingest
// (own-tree cascade to target prefix) and blob-resolve (cross-peer fs
// + tree cleanup).
//
// Before §7.2 wiring, this test demonstrates the gap: both peers
// would still have the stale doc/markdown-file entity at archives/
// notes/, and the receiving peer would still have the FileData +
// the file on disk. After §7.2, all four become absent.
func TestE2E_CollaborativeDeletion(t *testing.T) {
	const rootName = "shared"
	const sourcePrefix = "local/files/" + rootName + "/"
	const targetPrefix = "archives/notes/"

	aliceDir := t.TempDir()
	bobDir := t.TempDir()

	alice := newCollabPeer(t, "alice", aliceDir)
	bob := newCollabPeer(t, "bob", bobDir)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	bringUpListener(t, ctx, alice.ap, "alice")
	bringUpListener(t, ctx, bob.ap, "bob")
	if _, err := alice.ap.Connect(ctx, bob.ap.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}
	if _, err := bob.ap.Connect(ctx, alice.ap.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}

	alice.installMount(t, ctx, sourcePrefix, targetPrefix)
	bob.installMount(t, ctx, sourcePrefix, targetPrefix)
	alice.installCrossPeerMaterialize(t, bob.ap.PeerID(), sourcePrefix)
	bob.installCrossPeerMaterialize(t, alice.ap.PeerID(), sourcePrefix)

	// Seed two files on alice and wait for convergence.
	writeAndSettle(t, alice, bob, "intro.md", "# Intro\n\nFirst.\n", "seed intro")
	writeAndSettle(t, alice, bob, "keep.md", "# Keep\n\nThis one stays.\n", "seed keep")

	// Sanity: both files present on both peers' fs + both peers' trees.
	verifyConverged(t, "pre-delete intro", alice, bob,
		sourcePrefix+"intro.md", targetPrefix+"intro.md", "# Intro\n\nFirst.\n")
	verifyConverged(t, "pre-delete keep", alice, bob,
		sourcePrefix+"keep.md", targetPrefix+"keep.md", "# Keep\n\nThis one stays.\n")

	// === Delete intro.md on alice's fs ============================
	if err := os.Remove(filepath.Join(aliceDir, "intro.md")); err != nil {
		t.Fatalf("delete intro.md on alice's fs: %v", err)
	}
	t.Logf("deleted intro.md on alice's fs")

	// Wait for deletion to propagate end-to-end. Predicates:
	//   - alice's source binding for intro.md gone
	//   - alice's target binding for intro.md gone
	//   - bob's fs no longer has intro.md
	//   - bob's source binding gone
	//   - bob's target binding gone
	// keep.md is untouched throughout.
	allDeleted := waitFor(20*time.Second, func() bool {
		_, aliceSrcOK := alice.ap.Store().Get(sourcePrefix + "intro.md")
		_, aliceTgtOK := alice.ap.Store().Get(targetPrefix + "intro.md")
		_, bobSrcOK := bob.ap.Store().Get(sourcePrefix + "intro.md")
		_, bobTgtOK := bob.ap.Store().Get(targetPrefix + "intro.md")
		_, bobFsErr := os.Stat(filepath.Join(bobDir, "intro.md"))
		return !aliceSrcOK && !aliceTgtOK && !bobSrcOK && !bobTgtOK && os.IsNotExist(bobFsErr)
	})

	if !allDeleted {
		t.Logf("deletion convergence FAILED — diagnostic dump:")
		_, aliceSrcOK := alice.ap.Store().Get(sourcePrefix + "intro.md")
		_, aliceTgtOK := alice.ap.Store().Get(targetPrefix + "intro.md")
		_, bobSrcOK := bob.ap.Store().Get(sourcePrefix + "intro.md")
		_, bobTgtOK := bob.ap.Store().Get(targetPrefix + "intro.md")
		_, bobFsErr := os.Stat(filepath.Join(bobDir, "intro.md"))
		t.Logf("  alice source intro.md present=%v (want false)", aliceSrcOK)
		t.Logf("  alice target intro.md present=%v (want false)", aliceTgtOK)
		t.Logf("  bob   source intro.md present=%v (want false)", bobSrcOK)
		t.Logf("  bob   target intro.md present=%v (want false)", bobTgtOK)
		t.Logf("  bob   fs intro.md missing=%v (want true)", os.IsNotExist(bobFsErr))
		t.Fatalf("deletion did not propagate within 20s")
	}
	t.Logf("intro.md deletion propagated end-to-end")

	// keep.md should still be present everywhere — only intro.md was deleted.
	verifyConverged(t, "post-delete keep survives", alice, bob,
		sourcePrefix+"keep.md", targetPrefix+"keep.md", "# Keep\n\nThis one stays.\n")

	// No chain-error markers leaked.
	assertNoChainErrors(t, alice.ap, "alice")
	assertNoChainErrors(t, bob.ap, "bob")

	// Quick sanity: localfiles type constant import used.
	_ = localfiles.TypeFile
	_ = entitysdk.PeerConfig{}
	_ = workbench.MarkdownFileType
}
