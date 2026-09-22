package shellboot

import (
	"context"
	"path/filepath"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestNamedIdentity_LoadOrCreate covers the split that shipped broken:
// only DefaultIdentityName was ever created, so a NAMED identity had to
// pre-exist, while the Avalonia frontend's own usage text advertised
// "created on first launch" for the flag that takes a name.
//
// Why it matters more in the GUI than in the shell: `entity-shell` has an
// `identity create` verb, so a 404 there is an inconvenience with a
// documented next step. In the GUI the bridge fails to initialise, so
// there is no surface from which to create the identity that would let
// the app start — a dead end reachable by following the app's own help.
//
// Found on 2026-09-06 by scripts/twopeer-gui.sh, on its first attempt to
// stand up two named peers. No unit test could have found it: every
// existing bootstrap test either passes no identity at all or creates one
// first, and both of those work.
//
// Three cases, because the interesting behaviour is the ASYMMETRY. A test
// that only checked the create path would pass equally well against a
// build where -identity itself became create-if-absent — which is the
// change that must NOT be made, since the tree is peer-id-namespaced and
// a typo would silently abandon everything the intended peer owns.
func TestNamedIdentity_LoadOrCreate(t *testing.T) {
	t.Run("a named identity that does not exist is REFUSED", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		_, _, err := Bootstrap(context.Background(), Config{
			Identity:    "never-created",
			StorageKind: "sqlite",
			StoragePath: filepath.Join(home, "store.db"),
		})
		if err == nil {
			t.Fatal("bootstrap accepted an identity that does not exist; a typo would " +
				"silently become a different peer with an empty tree")
		}
		if !entitysdk.IsNotFound(err) {
			t.Fatalf("want a typed 404 so a surface can offer to create it, got: %v", err)
		}
	})

	t.Run("CreateIdentity makes the same name succeed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		ap, _, err := Bootstrap(context.Background(), Config{
			Identity:       "brand-new",
			CreateIdentity: true,
			StorageKind:    "sqlite",
			StoragePath:    filepath.Join(home, "store.db"),
		})
		if err != nil {
			t.Fatalf("bootstrap with CreateIdentity: %v", err)
		}
		defer ap.Close()

		if _, err := entitysdk.LoadIdentity("brand-new"); err != nil {
			t.Fatalf("the identity was not written to disk: %v", err)
		}
	})

	// The half that keeps the feature honest. Creating is create-IF-ABSENT
	// and never overwrite: regenerating a keypair over an existing file
	// would orphan every entity written under the old peer-id, which is
	// the exact silent re-namespacing the persistent-identity work exists
	// to prevent — made permanent, and by the flag whose whole job is to
	// be the safe way to say "new peer".
	t.Run("CreateIdentity does not overwrite an existing identity", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		first, err := entitysdk.CreateIdentity("already-here")
		if err != nil {
			t.Fatalf("seed: %v", err)
		}

		ap, _, err := Bootstrap(context.Background(), Config{
			Identity:       "already-here",
			CreateIdentity: true,
			StorageKind:    "sqlite",
			StoragePath:    filepath.Join(home, "store.db"),
		})
		if err != nil {
			t.Fatalf("bootstrap: %v", err)
		}
		defer ap.Close()

		if got := ap.PeerID(); got != first.PeerID {
			t.Fatalf("the peer came up under a DIFFERENT peer-id: seeded %s, got %s — "+
				"every grant, mount and offer naming the old id is now orphaned",
				first.PeerID, got)
		}
	})
}
