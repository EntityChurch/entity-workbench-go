package workbench

// observed_state.go — WHAT ANOTHER PEER SAYS, KEPT WHERE IT CANNOT BE
// MISTAKEN FOR WHAT WE DECLARED.
//
// # The rule this exists to satisfy
//
// A folder shared between two peers is ONE subject (S6), and a subject
// whose authority class is `shared` must name ONE reconciliation rule —
// otherwise the two peers can hold different rules for the same object
// and diverge with nothing noticing. We shipped exactly that:
// `FolderData.Conflict` is stored per peer, is carried by no wire field,
// and is read by each side from its own record. A can declare `record`
// while B declares `keep-both`.
//
// The specification seat ruled the shape on our own proposal: *the
// authority-class declaration names a party, and that party's copy of the
// rule is the subject's; a reader that cannot read it MUST refuse to
// reconcile rather than guess.* `FolderID(owner, root)` already designates
// an owner, on every peer, even under `Mode: both` — so the party is
// already named and nothing has to be invented.
//
// # Why the owner's rule is NOT stored in our FolderData
//
// Because that is the mistake this repository has already made once and
// written down: *"a RemoteFolderView exists so that stays visible at every
// call site — a reader who has one in hand cannot mistake it for local
// state, which is precisely the confusion that put another peer's
// acceptance decision inside our own declaration record and then had the
// reconciler branch on it."*
//
// `FolderData` is a DECLARATION: what this operator wants. Everything here
// is an OBSERVATION: what another machine said, at a time, and which may
// be wrong by now. Merging them would make `Conflict` mean two things
// depending on which side of the folder you read it from, which is the
// defect one layer down rather than the fix.
//
// # Why it is in the tree rather than in process memory
//
// The consumer is `folderConflictPolicy`, which runs inside the
// blob-resolve handler at delivery time — in `workbench`, which cannot
// import `shellcmd`, where the observation is made. The tree is the
// channel between them, and it is the same channel the declaration itself
// travels on. It also survives a restart, which matters because the
// alternative is a peer that refuses to resolve every conflict until its
// first reconcile pass lands.
//
// # What an ABSENT record means, and why that is the whole point
//
// Unknown. Not "the default" — the refusal is the ruling, and a default
// here would be the guess it forbids. See `folderConflictPolicy`.

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// ObservedFolderPrefix is where another peer's statements about a folder
// we both participate in are kept.
//
// Deliberately NOT under FolderPrefix: a prefix watch on declarations
// must not fire for hearsay, and a reader listing folders must not have
// to filter observations out of the result.
const ObservedFolderPrefix = "app/workbench/observed/folders/"

// ObservedFolderType is the entity type at ObservedFolderPrefix.
const ObservedFolderType = "workbench/observed-folder"

// ObservedFolderData is what the OWNER of a shared folder says about it,
// read from their tree and recorded here.
//
// Every field is hearsay in the strict sense — a fact about another
// machine, observed rather than computed. The type name says so, the
// prefix says so, and no field on it is ever merged into a FolderData.
type ObservedFolderData struct {
	// FolderID is the shared identifier — the same string on both peers.
	FolderID string `cbor:"folder_id"`

	// OwnerPeerID is whose record this came from. Recorded rather than
	// re-derived at read time so a stale observation cannot be silently
	// attributed to a new owner if a folder id is ever reused.
	OwnerPeerID string `cbor:"owner_peer_id"`

	// Conflict is the owner's declared reconciliation rule, VERBATIM —
	// including empty, which is a real declaration meaning the default.
	//
	// Defaulting it here would destroy the distinction the whole file
	// exists for: an empty string means "they declared nothing, which
	// means record", and an ABSENT RECORD means "we do not know". Those
	// are different answers and only one of them is allowed to produce a
	// delivery.
	Conflict string `cbor:"conflict,omitempty"`

	// ObservedAtMillis is when this VALUE was first seen — not when we
	// last looked.
	//
	// The distinction is forced and worth stating: the writer only saves
	// when the rule has CHANGED, because a reconcile pass runs on a timer
	// and an unconditional write would put one tree mutation per folder
	// per pass on a watched prefix, forever, to record that nothing
	// happened. So "we checked five seconds ago" is not recoverable from
	// this field, and a surface must not caption it that way.
	//
	// Nothing expires on it. An old observation is still the last thing
	// that peer actually said.
	ObservedAtMillis uint64 `cbor:"observed_at_millis"`
}

// OwnerConflictPolicy is the owner's rule, defaulted the same way
// FolderData.ConflictPolicy defaults theirs.
//
// The two must agree by construction, because they are the same field
// read from two machines — so this calls the same helper rather than
// repeating the comparison.
func (o ObservedFolderData) OwnerConflictPolicy() string {
	return FolderData{Conflict: o.Conflict}.ConflictPolicy()
}

// SaveObservedFolder records what an owner says about a folder.
func SaveObservedFolder(st *Store, o ObservedFolderData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if err := validateSegment(o.FolderID, "folder id"); err != nil {
		return err
	}
	if o.OwnerPeerID == "" {
		return fmt.Errorf("an observation with no owner names nobody")
	}
	if _, err := st.Put(ObservedFolderPrefix+o.FolderID, ObservedFolderType, o); err != nil {
		return fmt.Errorf("persist observation of folder %s: %w", o.FolderID, err)
	}
	return nil
}

// LoadObservedFolder reads what we last heard from a folder's owner.
//
// `false` means UNKNOWN — never a default. Every caller has to decide
// what to do about not knowing, which is the point.
func LoadObservedFolder(st *Store, id string) (ObservedFolderData, bool) {
	if st == nil || id == "" {
		return ObservedFolderData{}, false
	}
	ent, ok := st.Get(ObservedFolderPrefix + id)
	if !ok || ent.Type != ObservedFolderType {
		return ObservedFolderData{}, false
	}
	var o ObservedFolderData
	if err := ecf.Decode(ent.Data, &o); err != nil {
		return ObservedFolderData{}, false
	}
	if o.FolderID == "" {
		o.FolderID = id
	}
	return o, true
}

// RemoveObservedFolder drops an observation. Used when the folder itself
// goes away; an observation outliving its subject is a fact about nothing.
func RemoveObservedFolder(st *Store, id string) bool {
	if st == nil || id == "" {
		return false
	}
	if _, ok := st.Get(ObservedFolderPrefix + id); !ok {
		return false
	}
	return st.Remove(ObservedFolderPrefix + id)
}

// LoadObservedFolders reads every observation, sorted by folder id.
func LoadObservedFolders(st *Store) []ObservedFolderData {
	if st == nil {
		return nil
	}
	var out []ObservedFolderData
	for _, e := range st.List(ObservedFolderPrefix) {
		seg, under := RelativeUnder(e.Path, ObservedFolderPrefix)
		if !under || seg == "" || strings.Contains(seg, "/") {
			continue
		}
		if o, ok := LoadObservedFolder(st, seg); ok {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FolderID < out[j].FolderID })
	return out
}
