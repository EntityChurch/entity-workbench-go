package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
)

// share_offer.go — the OFFER half of a share: what a peer is publishing,
// stated in the tree so the other side can read it instead of being told.
//
// The authorization lives in `access_policy.go`. This file is the label,
// and APP-CONVENTION-SHARE §2.2 is explicit that the two are different
// things: *"The record does NOT restate the grant's scope: the grant is
// the authority and the record is the label. A consumer MUST NOT infer
// authorization from the record."* So nothing here grants anything, and
// a receiver that finds an offer still gets a 403 unless the policy entry
// exists. That is the correct failure and not a bug.
//
// Without this file the receiving operator has to be told a peer-id and a
// root name out of band — which is exactly the step that makes a "share"
// feel like a configuration exercise rather than a share.
//
// `entitysdk.ShareRecordData` is the convention's type and we use it
// unchanged. What we add is the storage location and the reverse lookup,
// neither of which the convention pins.

// ShareOfferPrefix is where this peer's outgoing offers live.
//
// Under `app/share/` — the type tags in `entitysdk.share.go` are
// `app/share/*`, and the convention notes the tag is the index key for
// cross-peer aggregation, so a tag under our own `app/workbench/` prefix
// would break browser-to-go interop. The PATH is ours to choose; the
// TYPE is not.
const ShareOfferPrefix = "app/share/records/"

// ShareOffer is one published offer as a surface should show it.
type ShareOffer struct {
	// Root is the mount root name the receiver passes to `sync`.
	Root string
	// Title is the human label.
	Title string
	// TargetPrefix is the tree prefix the documents land under on the
	// publishing side. Informational: a receiver mounts wherever they
	// like, and the two need not match.
	TargetPrefix string
	// Audience is the peer-ids this offer names. Empty means **self-only**
	// — an authored share with no members yet — and never "public"; see
	// Public.
	Audience []string
	// Public is true when this offer came from an `app/share/publication`
	// (§2.5): no audience, no minted token, pull-only.
	//
	// **A separate field and not `len(Audience) == 0`.** §2.2 gives an empty
	// audience the meaning *"authored, no members yet"*, so the two states
	// are opposites — one is reachable by anybody, the other by nobody — and
	// `SHARE-7` is the conformance vector that makes reading one as the other
	// fail loudly. A surface MUST render them differently.
	Public bool
	// CreatedAt is the authoring timestamp in Unix milliseconds, carried
	// as the convention stores it.
	CreatedAtMillis uint64
}

// OfferedTo reports whether this offer names a peer.
//
// **A publication is offered to everyone**, so it answers true for any
// non-empty peer-id — the alternative is an audience check that reads a
// pull-only share as addressed to nobody, which is the same conflation
// ShareOffer.Public exists to prevent.
func (o ShareOffer) OfferedTo(peerID string) bool {
	if o.Public {
		return peerID != ""
	}
	for _, a := range o.Audience {
		if a == peerID {
			return true
		}
	}
	return false
}

// SaveShareOffer publishes an offer for one mount root.
//
// Keyed by root, so re-sharing the same folder to a second peer replaces
// one record with a wider audience rather than accumulating records that
// each describe the same folder. The audience is the caller's whole
// intent for that root.
func SaveShareOffer(st *Store, o ShareOffer) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if strings.TrimSpace(o.Root) == "" {
		return fmt.Errorf("a share offer needs a root name")
	}
	if strings.Contains(o.Root, "/") {
		return fmt.Errorf("root %q contains a path separator; it is one path segment", o.Root)
	}
	title := o.Title
	if title == "" {
		title = o.Root
	}
	audience := make([]entitysdk.AudienceEntryData, 0, len(o.Audience))
	for _, a := range o.Audience {
		audience = append(audience, entitysdk.AudienceEntryData{
			Grantee: a,
			Via:     entitysdk.AudienceOriginDirect,
			AddedAt: o.CreatedAtMillis,
		})
	}
	rec := entitysdk.ShareRecordData{
		Title:     title,
		Target:    entitysdk.PrefixTarget(o.TargetPrefix),
		Audience:  audience,
		Note:      "local/files root: " + o.Root,
		CreatedAt: o.CreatedAtMillis,
	}
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("share record for %s: %w", o.Root, err)
	}
	if _, err := st.Put(ShareOfferPrefix+o.Root, entitysdk.TypeShareRecord, rec); err != nil {
		return fmt.Errorf("persist share offer %s: %w", o.Root, err)
	}
	return nil
}

// RemoveShareOffer withdraws an offer. Reports whether one was there.
func RemoveShareOffer(st *Store, root string) bool {
	if st == nil || root == "" {
		return false
	}
	return st.Remove(ShareOfferPrefix + root)
}

// LoadShareOffers reads this peer's own offers, sorted by root.
func LoadShareOffers(st *Store) (offers []ShareOffer, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(ShareOfferPrefix) {
		// RelativeUnder, never TrimPrefix (AP58).
		root, under := RelativeUnder(e.Path, ShareOfferPrefix)
		if !under || root == "" || strings.Contains(root, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, root+": listed but not resolvable")
			continue
		}
		offer, problem := decodeOfferEntity(root, ent)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		offers = append(offers, offer)
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].Root < offers[j].Root })
	sort.Strings(problems)
	return offers, problems
}

// decodeOfferEntity turns one offer entity into a ShareOffer, or returns a
// problem line naming the root. **The one decode point for both §2 share
// types**, shared by the local listing and by the remote read, because the two
// used to carry the type check separately and only one of them would have been
// taught about `app/share/publication`.
//
// The problem string is the empty string on success — a caller must test that
// rather than the zero ShareOffer, since a publication legitimately has an
// empty audience.
func decodeOfferEntity(root string, ent entity.Entity) (ShareOffer, string) {
	switch ent.Type {
	case entitysdk.TypeShareRecord:
		var rec entitysdk.ShareRecordData
		if err := ecf.Decode(ent.Data, &rec); err != nil {
			return ShareOffer{}, root + ": did not decode: " + err.Error()
		}
		return shareOfferFromRecord(root, rec), ""
	case entitysdk.TypeSharePublication:
		var pub entitysdk.SharePublicationData
		if err := ecf.Decode(ent.Data, &pub); err != nil {
			return ShareOffer{}, root + ": did not decode: " + err.Error()
		}
		return shareOfferFromPublication(root, pub), ""
	default:
		return ShareOffer{}, root + ": unexpected type " + ent.Type
	}
}

// DecodeRemoteShareOffer converts one record read from ANOTHER peer's
// tree. Separate from the local path because the root name arrives in the
// path there and has to be recovered from it here.
//
// Reads `app/share/publication` as well as `app/share/record` — the peer whose
// tree this is need not be us, and the other application tier on this protocol
// emits the publication type for its public shares.
func DecodeRemoteShareOffer(path string, ent entity.Entity) (ShareOffer, bool) {
	root := path
	if i := strings.LastIndex(root, "/"); i >= 0 {
		root = root[i+1:]
	}
	if root == "" {
		return ShareOffer{}, false
	}
	offer, problem := decodeOfferEntity(root, ent)
	if problem != "" {
		return ShareOffer{}, false
	}
	return offer, true
}

// shareOfferFromPublication reads §2.5's audience-less form. `Public` is set
// and `Audience` is left empty — which is why `Public` is a field and not
// derived from the audience length (see ShareOffer.Public).
func shareOfferFromPublication(root string, pub entitysdk.SharePublicationData) ShareOffer {
	return ShareOffer{
		Root:            root,
		Title:           pub.Title,
		TargetPrefix:    pub.Target.Path,
		Audience:        []string{},
		Public:          true,
		CreatedAtMillis: pub.CreatedAt,
	}
}

func shareOfferFromRecord(root string, rec entitysdk.ShareRecordData) ShareOffer {
	audience := make([]string, 0, len(rec.Audience))
	for _, a := range rec.Audience {
		audience = append(audience, a.Grantee)
	}
	sort.Strings(audience)
	return ShareOffer{
		Root:            root,
		Title:           rec.Title,
		TargetPrefix:    rec.Target.Path,
		Audience:        audience,
		CreatedAtMillis: rec.CreatedAt,
	}
}
