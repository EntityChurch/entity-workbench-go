package publish

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/tree"

	"entity-workbench-go/entitysdk"
)

// empty_prefix_test.go — a publish of nothing is a publish of a lie, and
// the guard that was supposed to stop it could not.
//
// Found 2026-09-12 by a *failing* run of the W2 gate: the test seeded a
// site at one path and published another, and the output said
// `— 0 paths` followed by `signed root: seq=1 … (+3 entities)`. The
// publisher had signed and emitted an origin committing to zero keys,
// with an explicit refusal for that case sitting in its own source.

// TestEmptyPrefixTrieIsNotZero is the CONTROL ARM, and without it the
// test below passes against a build where nothing was ever wrong.
//
// The whole defect rests on one substrate fact: a trie over a prefix
// with no bindings is not a zero hash, it is the hash of the canonical
// empty CHAMP node. If that ever stops being true, `mintSignedRoot`'s
// own `IsZero` arm becomes a real guard and the refusal added to
// [Publish] becomes redundant — so this asserts the fact the fix is
// aimed at, rather than the fix.
func TestEmptyPrefixTrieIsNotZero(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	// Something bound SOMEWHERE, so the peer is not trivially empty and
	// the measurement is about the prefix rather than about the store.
	if _, err := ap.Put("somewhere/else", "test/scalar", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root, err := tree.BuildTrieForPrefix(ap.RawContentStore(), ap.RawLocationIndex(),
		crypto.PeerID(ap.PeerID()), "nothing-here/")
	if err != nil {
		t.Fatalf("BuildTrieForPrefix: %v", err)
	}
	if root.IsZero() {
		t.Fatal("an empty prefix now yields a ZERO root — mintSignedRoot's IsZero arm is live again, " +
			"and the refusal in Publish can be reconsidered")
	}
	t.Logf("empty prefix -> non-zero root %s, which is why the IsZero guard never fired", root)
}

// TestPublishRefusesAnEmptyPrefix is the fix.
//
// The refusal has to be readable, because the operator's actual mistake
// is nearly always a prefix that is absolute where the field is
// peer-relative, or a stale one.
func TestPublishRefusesAnEmptyPrefix(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()
	if _, err := ap.Put("somewhere/else", "test/scalar", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err = Publish(context.Background(), Opts{
		Peer:      ap,
		Prefix:    "nothing-here/",
		OutputDir: t.TempDir(),
		OriginURL: "https://example.test",
	})
	if err == nil {
		t.Fatal("published a signed root over a prefix with no bindings — the origin is valid, " +
			"correctly signed, and answers \"absent\" to every key")
	}
	for _, want := range []string{"no bindings", "absent"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}

	// Anti-vacuity: the same call over a prefix that DOES have bindings
	// gets past the refusal. Without this the test above is satisfied by
	// a Publish that refuses everything.
	if _, err := Publish(context.Background(), Opts{
		Peer:      ap,
		Prefix:    "somewhere/",
		OutputDir: t.TempDir(),
		OriginURL: "https://example.test",
	}); err != nil {
		t.Fatalf("a non-empty prefix was refused too: %v", err)
	}
}
