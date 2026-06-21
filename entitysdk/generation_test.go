package entitysdk

import (
	"testing"
)

func TestGeneration_StartsNonZeroAfterBootstrap(t *testing.T) {
	// peer.New seeds a bunch of bootstrap entities (type definitions,
	// handler manifests/interfaces). Each of those is a tree write, so
	// the generation counter has already advanced by the time CreatePeer
	// returns. We don't pin the exact count — just assert it's > 0 as a
	// sanity check that the hook is wired.
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if ap.Generation() == 0 {
		t.Error("Generation() == 0 after CreatePeer; expected bootstrap writes to have advanced the counter")
	}
}

func TestGeneration_IncrementsOnPut(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	before := ap.Generation()
	if _, err := ap.Put("test/alpha", "test/type", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if after := ap.Generation(); after <= before {
		t.Errorf("Generation did not advance on Put: before=%d after=%d", before, after)
	}
}

func TestGeneration_IncrementsOnRemove(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if _, err := ap.Put("test/r", "test/type", 1); err != nil {
		t.Fatal(err)
	}
	before := ap.Generation()
	if err := ap.Remove("test/r"); err != nil {
		t.Fatal(err)
	}
	if after := ap.Generation(); after <= before {
		t.Errorf("Generation did not advance on Remove: before=%d after=%d", before, after)
	}
}

func TestGeneration_NoChangeOnRead(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if _, err := ap.Put("test/x", "test/type", 1); err != nil {
		t.Fatal(err)
	}
	before := ap.Generation()

	// Reads must not bump the counter.
	_, _, _ = ap.Get("test/x")
	_, _ = ap.Has("test/x")

	if after := ap.Generation(); after != before {
		t.Errorf("Generation changed on reads: before=%d after=%d", before, after)
	}
}
