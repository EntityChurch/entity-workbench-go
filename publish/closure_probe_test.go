package publish_test

// THE CLOSURE PROPERTY, RUN — a peer republishes a walk it obtained, and a
// third peer consumes the republication through the identical code path.
//
// # Why this one and not another
//
// The specification seat's draft makes this the load-bearing `MUST`:
//
//	"What a peer obtains through this mechanism, it MAY publish. A published
//	 result is the SAME KIND OF OBJECT as the sources it was built from —
//	 signed entries in the publishing peer's own namespace — and MUST be
//	 consumable by the identical code path."
//
// Their argument is that every system which centralized did so because its
// aggregator's output was a different TYPE from its input, so there could
// only be one of them. Their own note to us: *"what I'd want built first is
// the closure path, because if that doesn't hold, nothing else here
// matters."* Nobody had run it.
//
// # What this establishes, and the arm that makes it worth running
//
// A → publishes → B consumes → **B republishes into its own namespace** →
// B publishes → C consumes, with the same `fetch.Consumer`, no branch.
//
//   - the A→B→C chain carries byte-identical entities, and the entity
//     HASHES survive the hop. That is what keeps a detached signature
//     valid: a signature is bound at `system/signature/{hex(entry_hash)}`,
//     so a republication that moves the hash silently converts every
//     entry to UNATTRIBUTED.
//   - the CONTROL arm republishes the obvious way instead — decode the
//     body to a map and `Put` it back — through a struct that does not
//     declare one of the publisher's fields. **That is the realistic
//     gatherer**: it aggregates types it does not understand.
//
// # Measured 2026-09-11
//
// Byte-preserving republication holds: hashes identical across both hops,
// the third peer's consumer is the same code path with no branch.
// **The naive republication does NOT**: an undeclared field is dropped in
// silence and the hash moves, which is attribution loss with no error
// anywhere.
//
// Run: go test ./publish -run TestClosure -v -count=1

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/publish"
)

// originFor publishes ap's `feed/` prefix into a served directory.
func originFor(t *testing.T, ap *entitysdk.AppPeer) string {
	t.Helper()
	dir := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	if _, err := publish.Publish(context.Background(), publish.Opts{
		Peer: ap, Prefix: "feed/", OutputDir: dir, OriginURL: srv.URL,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return srv.URL
}

// consume walks an origin's signed root and returns key → entity, using the
// ONE consumer code path. Nothing in here knows whether the publisher
// authored the entries or republished somebody else's, which is the claim
// under test.
func consume(t *testing.T, origin string) (map[string]entityAt, *fetch.Consumer) {
	t.Helper()
	ctx := context.Background()
	layout, err := fetch.LoadLayout(ctx, origin, nil)
	if err != nil {
		t.Fatalf("LoadLayout(%s): %v", origin, err)
	}
	c := fetch.NewConsumer(layout, nil)
	rep, err := c.Consume(ctx, fetch.ConsumeOpts{})
	if err != nil {
		t.Fatalf("Consume(%s): %v", origin, err)
	}
	out := make(map[string]entityAt, len(rep.Walk.Bindings))
	for _, b := range rep.Walk.Bindings {
		ent, err := c.Blob(ctx, b.Hash)
		if err != nil {
			t.Fatalf("Blob(%s): %v", b.Key, err)
		}
		out[b.Key] = entityAt{Entity: ent, Hash: b.Hash}
	}
	return out, c
}

type entityAt struct {
	Entity entity.Entity
	Hash   hash.Hash
}

func TestClosure_RepublishedWalkIsTheSameKindOfObject(t *testing.T) {
	// ---- A authors three entries with a field no consumer struct declares.
	a, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer A: %v", err)
	}
	defer a.Close()
	for i := 0; i < 3; i++ {
		if _, err := a.Store().Put(fmt.Sprintf("feed/entry-%d", i), "app/feed/entry",
			map[string]any{
				"author": a.PeerID(),
				"body":   fmt.Sprintf("post %d", i),
				// A field a gatherer's struct would not declare. This is the
				// realistic case: an aggregator republishes types it has
				// never heard of.
				"content_warning": "spoilers",
			}); err != nil {
			t.Fatalf("A seed: %v", err)
		}
	}
	originA := originFor(t, a)

	// ---- B consumes A.
	fromA, _ := consume(t, originA)
	if len(fromA) != 3 {
		t.Fatalf("B saw %d keys from A, want 3", len(fromA))
	}

	// ---- B REPUBLISHES, byte-preserving, into its own namespace.
	b, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer B: %v", err)
	}
	defer b.Close()
	for key, raw := range fromA {
		h, err := b.PutEntity("feed/"+key, raw.Entity)
		if err != nil {
			t.Fatalf("B PutEntity %s: %v", key, err)
		}
		if h != raw.Hash {
			t.Errorf("⛔ CLOSURE BROKEN at the republish: %s went in as %s and bound as %s — "+
				"the detached signature is keyed on the original hash, so every republished "+
				"entry is now unattributed", key, raw.Hash, h)
		}
	}
	originB := originFor(t, b)

	// ---- C consumes B, SAME code path, no branch, no knowledge that B is a
	// republisher rather than an author.
	fromB, _ := consume(t, originB)
	if len(fromB) != 3 {
		t.Fatalf("C saw %d keys from B, want 3", len(fromB))
	}
	for key, orig := range fromA {
		got, ok := fromB[key]
		if !ok {
			t.Errorf("C did not receive %q through B", key)
			continue
		}
		if got.Hash != orig.Hash {
			t.Errorf("⛔ %q: A published %s, C received %s through B — not the same object",
				key, orig.Hash, got.Hash)
		}
	}
	t.Logf("CLOSURE HOLDS: 3 entries A→B→C, byte-identical hashes at both hops, "+
		"one consumer code path (peer A %s, peer B %s)", short(a.PeerID()), short(b.PeerID()))

	// ---- CONTROL ARM. Republish the obvious way: decode the body through a
	// struct that does not declare `content_warning`, then Put it back.
	// Without this arm the assertions above could be satisfied by an
	// encoding that cannot lose anything, and the MUST would be unearned.
	type gathererView struct {
		Author string `cbor:"author"`
		Body   string `cbor:"body"`
		// content_warning deliberately absent — this is the gatherer that
		// does not know the type.
	}
	d, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer D: %v", err)
	}
	defer d.Close()
	var moved, same int
	for key, orig := range fromA {
		var view gathererView
		if err := ecf.Decode(orig.Entity.Data, &view); err != nil {
			t.Fatalf("decode %s: %v", key, err)
		}
		h, err := d.Store().Put("feed/"+key, "app/feed/entry", view)
		if err != nil {
			t.Fatalf("D Put %s: %v", key, err)
		}
		if h == orig.Hash {
			same++
		} else {
			moved++
		}
	}
	t.Logf("CONTROL (naive republish through an undeclaring struct): %d of %d hashes MOVED",
		moved, moved+same)
	if moved == 0 {
		t.Errorf("the naive republish preserved every hash — then byte-preservation is not "+
			"load-bearing and the assertions above are vacuous (same=%d)", same)
	}
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8] + "…"
	}
	return id
}
