// feed_gather_signer_test.go — the CONSUMER half of `DX-C1`'s third hop,
// isolated from the transport-authorization half that blocks it.
//
// `feed_gather_reach_test.go` runs the whole hop and measures it stopping at
// leg 4. That run cannot say **which** of two independent walls stopped it,
// and they belong to different seats:
//
//  1. the consumer asking at the wrong address — §2.2 roots the invariant
//     pointer at the SIGNER and ours derived it under the peer it was reading
//     from. **Ours, and fixed 2026-09-17.**
//  2. a republisher being unable to authorize a read of its own copy of the
//     author's namespace at all — measured, not ours, routed as core-go row 22.
//
// A gate that only runs the full hop reports (1) as unfixed for as long as (2)
// is open, which is how a repair that landed gets re-derived by the next
// session. So this file removes (2) by construction — it serves the
// republisher's bytes out of the republisher's own store, which is a statement
// about the transport and about nothing else — and asserts (1) on its own.
//
// ⚠ **What it therefore does NOT establish:** that a third party can reach any
// of this over a real connection. That is leg 4, it is still red, and it is
// still the honest state of the tier. This file is scoped to *given the bytes,
// does our consumer attribute them to the right key at the right address* —
// and that question had no gate at all.
package publish_test

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/workbench"
)

// republisherSource answers as the REPUBLISHER while holding the author's
// bytes — the one configuration in which "the peer being read from" and "the
// peer that signed" are different, which is the whole proposition of a mirror
// and is unreachable with two peers.
//
// `Leaf` reads the republisher's store directly. That is deliberate and is the
// isolation this file is named for: the capability layer is wall (2), it is
// measured next door, and routing through it here would make this gate report
// somebody else's blocker as our defect.
type republisherSource struct {
	republisher *entitysdk.AppPeer
	asked       []string // every tree path this source was asked for
}

func (s *republisherSource) PeerID() string { return s.republisher.PeerID() }

func (s *republisherSource) Describe() fetch.Description {
	return fetch.Description{Mode: fetch.ModeLivePeer, Authority: "entity://" + s.PeerID()}
}

func (s *republisherSource) Root(context.Context) (entity.Entity, string, error) {
	return entity.Entity{}, "", fetch.ErrNoPublishedRoot
}

func (s *republisherSource) Leaf(_ context.Context, treePath string) (hash.Hash, string, error) {
	s.asked = append(s.asked, treePath)
	locator := "store://" + s.PeerID() + "/" + strings.TrimPrefix(treePath, "/")
	ent, ok := s.republisher.Store().Get(treePath)
	if !ok {
		return hash.Hash{}, locator, fetch.ErrNoPublishedRoot
	}
	return ent.ContentHash, locator, nil
}

func (s *republisherSource) Blob(_ context.Context, h hash.Hash) (entity.Entity, string, error) {
	locator := "content://" + s.PeerID() + "/" + h.String()
	res, err := s.republisher.Content().Get(context.Background(), []hash.Hash{h})
	if err != nil {
		return entity.Entity{}, locator, err
	}
	ent, ok := res.Entities[h]
	if !ok {
		return entity.Entity{}, locator, fetch.ErrNoPublishedRoot
	}
	return ent, locator, nil
}

// TestGatherSigner_AttributionFollowsTheSignerAndNotTheServingPeer is the
// assertion `DX-C1` leg 4 will become once core-go row 22 lifts.
//
// Three things are checked and each fails on the pre-2026-09-17 consumer:
// the ADDRESS asked for is signer-rooted; the KEY verified against is the
// signer's; and naming the wrong signer refuses. The third is the control arm
// and it is not decoration — without it the test passes against a consumer
// that verifies nothing, which is the failure mode a signature gate has.
func TestGatherSigner_AttributionFollowsTheSignerAndNotTheServingPeer(t *testing.T) {
	ctx := context.Background()
	trio := newMirrorTrio(t, 2)
	author, gatherer := trio.author.PeerID(), trio.gatherer.PeerID()
	if author == gatherer {
		t.Fatal("author and gatherer are the same peer, so this measures nothing")
	}

	src := &republisherSource{republisher: trio.gatherer}
	c := fetch.NewConsumerFromSource(src, nil)

	// An entry the gather carried, taken from the republisher — so these are
	// bytes A authored and B holds, which is the state a mirror exists to
	// create.
	var carried entity.Entity
	for _, ce := range trio.plan.Carried {
		if ce.Kind == workbench.CarriedEntry {
			e, err := c.Blob(ctx, ce.Entity.ContentHash)
			if err != nil {
				t.Fatalf("the republisher does not serve a carried entry it planned: %v", err)
			}
			carried = e
			break
		}
	}
	if carried.ContentHash.IsZero() {
		t.Fatal("the plan carried no entry, so there is nothing to attribute")
	}

	// ---- The signer's key, at the signer's address ----
	if _, where, err := c.SignatureOver(ctx, "mirrored feed entry", author, carried); err != nil {
		t.Fatalf("a mirrored entry is not attributable to its AUTHOR even with the bytes in hand "+
			"(looked at %s): %v — this is the consumer half of `DX-C1` leg 4 and it is ours", where, err)
	}

	wantPath := "/" + author + "/" + types.LocalSignaturePath(carried.ContentHash)
	var found bool
	for _, p := range src.asked {
		if p == wantPath {
			found = true
		}
		if strings.HasPrefix(p, "/"+gatherer+"/system/signature/") {
			t.Errorf("the consumer asked for the signature under the REPUBLISHER (%s). §2.2 roots "+
				"the invariant pointer at /{signer_peer_id}/ and `ENTITY-CORE-PROTOCOL` §1.4 makes "+
				"re-qualifying an already-absolute path a MUST NOT", p)
		}
	}
	if !found {
		t.Errorf("the consumer never asked for %s; it asked for %v. The address is the half of this "+
			"that a passing verification cannot vouch for, because a consumer that asked at the "+
			"wrong place and got lucky would look identical", wantPath, src.asked)
	}

	// ---- Control arm: the SIGNER is what does the work ----
	//
	// Same bytes, same source, same address space — only the claimed signer
	// changes, to the peer actually serving them. It must refuse. Without this
	// the assertion above is satisfied by a consumer that accepts any
	// signature it can find, which is the specific way this check fails
	// silently: the mirror still renders, every entry still says "attributed",
	// and the attribution names the wrong party.
	if _, _, err := c.SignatureOver(ctx, "mirrored feed entry", gatherer, carried); err == nil {
		t.Error("a mirrored entry verified against the REPUBLISHER's key. §2.2's whole proposition " +
			"is that authorship survives detachment, and a reader that attributes to whoever handed " +
			"it the bytes has inverted it — a republisher could claim every entry it carries")
	}

	// ---- And an unnamed signer is refused rather than defaulted ----
	//
	// The default a reader would reach for is "the peer I am reading from",
	// which is right on a direct read and wrong on exactly this one. Refusing
	// is what stops that default being re-introduced by someone tidying the
	// signature up.
	if _, _, err := c.SignatureOver(ctx, "mirrored feed entry", "", carried); err == nil {
		t.Error("a signature verified with no signer named — the parameter is defaulting somewhere")
	}
}
