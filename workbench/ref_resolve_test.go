package workbench

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
)

// ref_resolve_test.go — the parts of a reference resolution that have no
// bytes in them.
//
// **The outcomes themselves are gated where a real published act exists**
// — `publish/ref_outcomes_live_test.go` drives all four of §2.2.2's rows
// plus the fifth state against a publisher that republishes between
// reads, because rows 2 and 3 cannot be produced by a fixture at all.
// What is under test here is the key arithmetic and the one refusal, both
// of which are wrong in ways a green end-to-end run would not show.
//
// Tier: unit.

// TestRefKeyUnderPrefix_IsSegmentExact is the trap that is right on every
// example anybody thinks of.
//
// §3.3a makes reconstruction pure concatenation, so the inverse is a
// string trim — and a publisher whose prefix does not end in a separator
// would then swallow a sibling directory. `app/feedback/` is not under
// `app/feed`, and a resolver that thought it was would report a path in
// somebody else's subtree as row 3 or row 4: a confident statement that
// a document is GONE, about a root that never mentioned it.
func TestRefKeyUnderPrefix_IsSegmentExact(t *testing.T) {
	const peer = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"
	cases := []struct {
		name      string
		abs       string
		absPrefix string
		wantKey   string
		wantIn    bool
	}{
		{"under a slash-terminated prefix",
			"/" + peer + "/app/feed/index", "/" + peer + "/app/feed/", "index", true},
		{"under a prefix with no separator, §3.3a concatenation kept",
			"/" + peer + "/app/feed/index", "/" + peer + "/app/feed", "/index", true},
		{"a sibling directory is NOT under it",
			"/" + peer + "/app/feedback/x", "/" + peer + "/app/feed", "", false},
		{"a sibling directory is not under the slash form either",
			"/" + peer + "/app/feedback/x", "/" + peer + "/app/feed/", "", false},
		{"another peer's path is never under this one's prefix",
			"/somebodyelse/app/feed/index", "/" + peer + "/app/feed/", "", false},
		{"the universal tree covers everything and keys stay qualified",
			"/" + peer + "/system/config/x", "", "/" + peer + "/system/config/x", true},
		{"the prefix itself",
			"/" + peer + "/app/feed", "/" + peer + "/app/feed", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, in := refKeyUnderPrefix(tc.abs, tc.absPrefix)
			if in != tc.wantIn || key != tc.wantKey {
				t.Errorf("refKeyUnderPrefix(%q, %q) = (%q, %v), want (%q, %v)",
					tc.abs, tc.absPrefix, key, in, tc.wantKey, tc.wantIn)
			}
		})
	}
}

// refuseSource is a [fetch.Source] that names a publisher and answers
// nothing. Enough to build a Consumer, which is all the refusal below
// needs — and deliberately not enough to resolve anything, so a test
// that got past the refusal would fail loudly rather than silently
// measuring something else.
type refuseSource struct{ peerID string }

func (s refuseSource) PeerID() string { return s.peerID }
func (s refuseSource) Describe() fetch.Description {
	return fetch.Description{Mode: fetch.ModeLivePeer, Authority: "entity://" + s.peerID}
}
func (s refuseSource) Root(context.Context) (entity.Entity, string, error) {
	return entity.Entity{}, "", fetch.ErrNoPublishedRoot
}
func (s refuseSource) Leaf(context.Context, string) (hash.Hash, string, error) {
	return hash.Hash{}, "", fetch.ErrNoPublishedRoot
}
func (s refuseSource) Blob(context.Context, hash.Hash) (entity.Entity, string, error) {
	return entity.Entity{}, "", fetch.ErrNoPublishedRoot
}

// TestResolveRef_RefusesAReaderBoundToAnotherPublisher.
//
// A reference is routable precisely BECAUSE it names its publisher (§1),
// so resolving one against a reader bound to a different peer checks the
// wrong key. The failure it prevents is the quiet kind: the other peer's
// root verifies perfectly, its walk does not carry the path, and the
// answer comes back as a confident row 4 about a document that is fine.
func TestResolveRef_RefusesAReaderBoundToAnotherPublisher(t *testing.T) {
	const them = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"
	const somebodyElse = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sT"

	c := fetch.NewConsumerFromSource(refuseSource{peerID: somebodyElse}, nil)
	_, err := ResolveRef(context.Background(), c, entitysdk.LiveRef(them, "/app/feed/index", hash.Hash{}),
		RefResolveOpts{})
	if err == nil {
		t.Fatal("resolved a reference to one publisher against a reader bound to another")
	}
	if !strings.Contains(err.Error(), them) || !strings.Contains(err.Error(), somebodyElse) {
		t.Errorf("the refusal names neither peer, so it cannot be acted on: %v", err)
	}
}

// TestResolveRef_PassesThroughTheNeverPublishedState.
//
// `fetch.ErrNoPublishedRoot` is a third state — not unreachable, not
// withholding — and it is exported so a surface can say so. A resolver
// that wrapped it in "could not resolve this reference" would send an
// operator to check a connection that is fine.
func TestResolveRef_PassesThroughTheNeverPublishedState(t *testing.T) {
	const them = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"
	c := fetch.NewConsumerFromSource(refuseSource{peerID: them}, nil)
	_, err := ResolveRef(context.Background(), c, entitysdk.LiveRef(them, "/app/feed/index", hash.Hash{}),
		RefResolveOpts{})
	if err == nil {
		t.Fatal("a publisher with no root resolved a live reference")
	}
	if !strings.Contains(err.Error(), fetch.ErrNoPublishedRoot.Error()) {
		t.Errorf("the never-published state did not survive to the caller: %v", err)
	}
}
