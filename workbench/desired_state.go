package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// desired_state.go — the two objects an operator actually declares, and
// the only two records in the sharing flow that anybody writes on purpose.
//
// # Why this exists
//
// Before it, the durable state behind "this peer and I share this folder"
// was FIVE artefacts written by five verbs and restored by four different
// startup paths — a roster entry, a capability policy row, an offer
// record, a mount binding, a sync binding — and **none of them was the
// relationship.** The relationship could only be inferred by joining
// them, every one of them was written at a different moment, and any one
// going missing produced a different silent failure. Three of the four
// restart defects this repo has shipped since August are that shape.
//
// So: two declared objects, held in the tree, written by the operator's
// two real gestures and by nothing else.
//
//	app/workbench/devices/{peer-id}   a peer we know          (Device)
//	app/workbench/folders/{folder-id} a folder, and with whom (Folder)
//
// Everything else — policy rows, mounts, subscriptions, sync bindings,
// connections, maintain sessions — becomes OUTPUT, produced by a
// reconciler from these two. See docs/architecture/SHARING-DIRECTION.md
// §3 for the whole argument; the short version is that the existing flow
// is a wizard (order-dependent, non-idempotent, dead after a restart) and
// the substrate wanted a controller.
//
// # Why the tree and not a file
//
// `~/.entity/gui-layout.json` and `~/.entity/browser.json` are on-disk
// config, and the layout file's stated reason — an ephemeral default peer
// makes a peer-id-keyed record useless — stopped being true when the
// default peer became persistent. The rule that replaces it: **a file
// holds what you need in order to find the tree; the tree holds
// everything else.** Which identity, which store path, which listen
// address are pre-peer facts and must be a file. These two are per-peer
// state, and in the tree they are addressable, watchable, revisioned, and
// they travel with the peer when it is backed up or moved.
//
// # One convention, stated once
//
// **A boolean on a persisted record defaults to the SAFE behaviour when
// the field is absent**, because an older record decodes to the zero
// value and nobody notices. Hence `Paused` rather than `AutoConnect`: a
// device written by a build that had never heard of pausing decodes as
// active, which is what its author meant. The inverse spelling would have
// silently disconnected every previously-added peer on upgrade.

// DevicePrefix is where known peers live. Under `app/workbench/` — the
// application namespace, which extends freely (anything `system/*` is
// spec-adjacent and needs cross-impl coordination first).
const DevicePrefix = "app/workbench/devices/"

// DeviceType is the entity type at DevicePrefix.
const DeviceType = "workbench/device"

// DeviceData is one known peer: the durable record of a pairing.
//
// Adding one is the single approval that establishes a relationship, and
// it is what makes reconnect possible without an operator retyping an
// address — the same model Syncthing uses, and for the same reason.
type DeviceData struct {
	// PeerID is the Base58 peer-id. Also the path segment.
	PeerID string `cbor:"peer_id"`

	// Label is what a human calls this machine. Free text; defaults to a
	// short form of the peer-id when the operator gave none.
	Label string `cbor:"label,omitempty"`

	// Addresses are last-known dial addresses, freshest first. Sourced
	// from mDNS discovery, from an address the operator typed, or from
	// the peer's own advertised transport profile.
	//
	// A LIST rather than one address, because a laptop is a different
	// address on every network it joins and the previous one stays
	// worth trying: an address that worked yesterday is a better guess
	// than no guess. Bounded by MaxDeviceAddresses so a machine that
	// roams does not accumulate history forever.
	Addresses []string `cbor:"addresses,omitempty"`

	// AddedAtMillis is when this device was paired. Unix MILLIseconds —
	// the unit is in the field name because it crosses cgo and JSON,
	// where a doc comment is invisible and a mistaken unit killed the
	// process once already (AP61).
	AddedAtMillis uint64 `cbor:"added_at_millis"`

	// LastSeenMillis is the last time we observed this peer connected.
	// Advisory: the authority on liveness is `system/peer/status`, read
	// through PeerLivenessModel. This is here so a device that has never
	// been reachable can be told from one that was reachable an hour ago.
	LastSeenMillis uint64 `cbor:"last_seen_millis,omitempty"`

	// Paused stops the reconciler from maintaining a connection to this
	// device without forgetting the relationship. Absent means active —
	// see the note about boolean defaults in this file's header.
	Paused bool `cbor:"paused,omitempty"`
}

// MaxDeviceAddresses bounds the remembered dial addresses per device.
// Small on purpose: this is a hint list for the reconciler, not a
// history, and an unbounded list on a roaming laptop is an entity that
// grows without anyone deciding it should.
const MaxDeviceAddresses = 4

// ShortLabel is the display name: the operator's label, or a truncated
// peer-id when they gave none. Never the empty string, because a blank
// row in a device list is indistinguishable from a broken one.
func (d DeviceData) ShortLabel() string {
	if strings.TrimSpace(d.Label) != "" {
		return d.Label
	}
	if len(d.PeerID) > 10 {
		return d.PeerID[:10] + "…"
	}
	if d.PeerID == "" {
		return "(unnamed peer)"
	}
	return d.PeerID
}

// PreferredAddress is the freshest known dial address, or "" when this
// device has only ever been reachable by peer-id (via a transport
// profile the kernel resolves for itself).
func (d DeviceData) PreferredAddress() string {
	for _, a := range d.Addresses {
		if strings.TrimSpace(a) != "" {
			return a
		}
	}
	return ""
}

// WithAddress returns a copy with addr promoted to the front, deduped and
// bounded. Returns the receiver unchanged for an empty address, so a
// discovery pass that learned nothing cannot erase what we knew.
func (d DeviceData) WithAddress(addr string) DeviceData {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return d
	}
	out := make([]string, 0, MaxDeviceAddresses)
	out = append(out, addr)
	for _, a := range d.Addresses {
		if a == addr || strings.TrimSpace(a) == "" {
			continue
		}
		if len(out) >= MaxDeviceAddresses {
			break
		}
		out = append(out, a)
	}
	d.Addresses = out
	return d
}

// FolderPrefix is where shared folders live.
const FolderPrefix = "app/workbench/folders/"

// FolderType is the entity type at FolderPrefix.
const FolderType = "workbench/folder"

// Folder share states. A share is a two-sided fact and these are the
// states of ONE side's view of it.
const (
	// FolderStateOffered — we have offered this folder to that peer, or
	// they have offered it to us, and nobody has accepted yet.
	FolderStateOffered = "offered"
	// FolderStateAccepted — the receiving side said yes and named a
	// local directory. This is the only state the reconciler acts on.
	FolderStateAccepted = "accepted"
	// FolderStateDeclined — the receiving side said no. Kept rather than
	// deleted so the offer does not reappear as new on the next scan.
	FolderStateDeclined = "declined"
	// FolderStateWithdrawn — the offering side revoked it. Kept for the
	// same reason, and because "it stopped working" needs an answer.
	FolderStateWithdrawn = "withdrawn"
)

// Folder modes — THIS PEER's direction on a shared folder.
//
// Syncthing's three folder types exactly (`sendreceive` / `sendonly` /
// `receiveonly`), and for its reason: direction is a per-device property
// **of one shared folder**, not a property of who created it.
//
// That distinction is the whole of S6. Until 2026-09-04 the reconciler
// branched on `IsLocal()` — i.e. on ORIGIN — everywhere it meant
// direction, and origin is immutable and binary. So "bidirectional" was
// inexpressible: the only way to get bytes flowing both ways was a
// second, unrelated share on the other machine, producing a second
// folder object with a different id pointed at a different directory.
// The operator's words for the result were *"bilateral transfer to
// different locations, but they don't have the same understanding."*
// They were describing the data model accurately.
//
// Mode is what the reconciler reads now. Origin still says who
// originated the folder — that is what names it and what owns the offer
// record — and it no longer decides which way anything flows.
const (
	FolderModeSend    = "send"    // we publish changes; we never write theirs
	FolderModeReceive = "receive" // we accept changes; we never publish ours
	FolderModeBoth    = "both"
)

// Publishes reports whether this peer sends its changes for this folder.
func (f FolderData) Publishes() bool {
	m := f.EffectiveMode()
	return m == FolderModeSend || m == FolderModeBoth
}

// Receives reports whether this peer accepts the other side's changes.
func (f FolderData) Receives() bool {
	m := f.EffectiveMode()
	return m == FolderModeReceive || m == FolderModeBoth
}

// EffectiveMode is Mode, or — when it is absent — THE PRE-S6 BEHAVIOUR,
// which is derived from Origin: a folder we own published, a folder we
// received received.
//
// This is the whole migration story for direction, and getting it wrong
// is not symmetric. Defaulting an absent Mode to `both` (the first
// version of this) silently starts publishing a folder the operator only
// ever accepted — someone else's files, and their disk, going back out
// over a grant that already exists. Defaulting it the other way merely
// keeps doing what the record already did.
//
// So: absent means "what this record meant before the field was read by
// anything", every record written since carries an explicit value, and
// `both` is only ever reached by an operator asking for it.
func (f FolderData) EffectiveMode() string {
	switch f.Mode {
	case FolderModeSend, FolderModeReceive, FolderModeBoth:
		return f.Mode
	}
	if f.IsLocal() {
		return FolderModeSend
	}
	return FolderModeReceive
}

// NormalizeMode maps free text onto a mode constant, refusing anything
// else. A REFUSAL and not a fallback: a mode that quietly became `both`
// because it was misspelled would publish an operator's disk when they
// asked for receive-only, which is the one direction of this mistake
// that cannot be undone by fixing it afterwards (AP33).
func NormalizeMode(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case FolderModeSend, "sendonly", "send-only", "out":
		return FolderModeSend, nil
	case FolderModeReceive, "receiveonly", "receive-only", "in":
		return FolderModeReceive, nil
	case FolderModeBoth, "sendreceive", "send-receive", "bidirectional", "two-way":
		return FolderModeBoth, nil
	}
	return "", fmt.Errorf("mode %q is not one of send, receive, both", s)
}

// FolderData is one folder and the set of peers it is shared with.
//
// ONE type covers both directions. A folder we own has Origin "local";
// a folder we received has Origin set to the peer we got it from, Root
// naming THEIR mount root (which is the subscription key) and Path naming
// OUR local directory. Two types would duplicate every field and force
// every reader to handle both — and the operator thinks of them as one
// list, which is what the panel has to render.
type FolderData struct {
	// ID is the stable identifier and the path segment. For a local
	// folder it is the mount root name; for a received one it is
	// "{peer-id}.{their-root}", which is also the sync binding key.
	ID string `cbor:"id"`

	// Label is what a human calls it. Defaults to the directory basename.
	Label string `cbor:"label,omitempty"`

	// Kind selects the adapter that decides what an arriving change MEANS
	// — "files" today. See SHARING-DIRECTION.md §4: everything above this
	// field is identical for a shared revision project or document, and
	// this is the seam that lets the second kind reuse it.
	Kind string `cbor:"kind"`

	// Path is the local filesystem directory. Always ours, in both
	// directions.
	Path string `cbor:"path"`

	// Root is the local-files mount root this folder is bound to. For a
	// received folder it is the ORIGINATING peer's root name, because
	// that is what the subscription and the sync binding are keyed on.
	Root string `cbor:"root"`

	// LocalRoot is the mount root on THIS peer that a received folder's
	// bytes are written into. Empty means "the same as Root", which is
	// the symmetric case and stays the common one.
	//
	// It exists because accepting a folder now takes a DIRECTORY, and a
	// mount root is derived from that directory's basename — so the
	// moment an operator receives `downloads` into `~/from-alice`, the
	// two names differ. Before that they could not: the receiver had to
	// create a directory named exactly what the sender called theirs, an
	// unstated coupling that made the mount step feel arbitrary.
	//
	// Read it through ReceivingRoot(), never directly. Every consumer
	// wants "which local mount receives this", and a reader that reaches
	// for Root gets the answer that is right in the common case and
	// silently wrong in the one this field was added for — which is the
	// worst available failure mode, because it survives every test whose
	// two peers happen to agree on a name.
	LocalRoot string `cbor:"local_root,omitempty"`

	// Origin is "local" for a folder we own, or the peer-id we received
	// it from.
	Origin string `cbor:"origin"`

	// Mode is one of the FolderMode constants.
	Mode string `cbor:"mode"`

	// Conflict is what happens when a delivery lands on a path this peer
	// has edited since it last agreed with the sender. One of the
	// ConflictPolicy constants; empty means ConflictPolicyRecord.
	//
	// A DECLARATION and not a setting, like everything else in this
	// record: the reconciler reads it, the handler acts on it, and no
	// verb writes the behaviour directly. Set it with
	// ShellWorkspace.SetFolderConflictPolicy.
	//
	// The default is deliberately the one that does not change what is in
	// the folder — see conflict_record.go for why keep-both is wrong as a
	// default in a one-way share, which is the topology this product
	// ships most of.
	Conflict string `cbor:"conflict,omitempty"`

	// SharedWith is one entry per peer this folder is shared with, in
	// either direction. For a received folder it holds exactly one entry
	// — the originating peer — carrying OUR accept/decline state.
	SharedWith []FolderPeerData `cbor:"shared_with,omitempty"`
}

// Conflict policies — what a delivery does when it lands on a locally
// edited path.
const (
	// ConflictPolicyRecord converges (the delivered version wins on disk,
	// which is `DOMAIN-LOCAL-FILES` §1.1a's last-arrival-wins) and writes
	// a durable record naming the version it replaced, which stays
	// recoverable from the chain. The default, and empty means this.
	ConflictPolicyRecord = "record"
	// ConflictPolicyKeepBoth additionally writes the replaced version to
	// `{path}.keep-both-{hash8}`, EXTENSION-REVISION §2.3's sibling, so
	// both versions are PRESENT rather than merely recoverable. The
	// folder stops converging until an operator acts, which is why it is
	// opt-in.
	ConflictPolicyKeepBoth = "keep-both"
)

// ConflictPolicy is Conflict, defaulted.
//
// Absent means ConflictPolicyRecord and never keep-both, for the same
// reason EffectiveMode's absent case is the pre-S6 behaviour: defaulting
// to the stronger action starts changing what is in folders an operator
// set up under the weaker one, and that mistake is not symmetric.
func (f FolderData) ConflictPolicy() string {
	if f.Conflict == ConflictPolicyKeepBoth {
		return ConflictPolicyKeepBoth
	}
	return ConflictPolicyRecord
}

// ParseConflictPolicy accepts the spellings an operator types.
func ParseConflictPolicy(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ConflictPolicyRecord, "record-only", "converge", "last-arrival-wins", "lww":
		return ConflictPolicyRecord, nil
	case ConflictPolicyKeepBoth, "keepboth", "keep_both", "both":
		return ConflictPolicyKeepBoth, nil
	}
	return "", fmt.Errorf("conflict policy must be %q or %q, got %q",
		ConflictPolicyRecord, ConflictPolicyKeepBoth, s)
}

// FolderPeerData is one peer's participation in a folder.
type FolderPeerData struct {
	PeerID string `cbor:"peer_id"`
	State  string `cbor:"state"`
	// AtMillis is when this state was last set. Unix milliseconds.
	AtMillis uint64 `cbor:"at_millis"`
	// Note carries WHY, for the states where a bare word is not an
	// answer — a decline reason, a withdrawal reason. Rendered beside
	// the state, never instead of it.
	Note string `cbor:"note,omitempty"`
}

// IsLocal reports whether this peer owns the folder (as opposed to
// having received it from someone).
func (f FolderData) IsLocal() bool { return f.Origin == "" || f.Origin == "local" }

// ReceivingRoot is the local mount root this folder's bytes land in.
//
// The one way to ask that question. LocalRoot defaults to Root, so an
// older record — written before a received folder could land anywhere
// but a same-named mount — decodes to exactly what it used to mean.
func (f FolderData) ReceivingRoot() string {
	if strings.TrimSpace(f.LocalRoot) != "" {
		return f.LocalRoot
	}
	return f.Root
}

// DisplayLabel is never empty, for the same reason DeviceData.ShortLabel
// is not.
func (f FolderData) DisplayLabel() string {
	if strings.TrimSpace(f.Label) != "" {
		return f.Label
	}
	if f.Root != "" {
		return f.Root
	}
	return f.ID
}

// PeerState returns this folder's state for one peer.
func (f FolderData) PeerState(peerID string) (FolderPeerData, bool) {
	for _, p := range f.SharedWith {
		if p.PeerID == peerID {
			return p, true
		}
	}
	return FolderPeerData{}, false
}

// AcceptedPeers returns the peers whose state is accepted — the only ones
// the reconciler establishes anything for.
func (f FolderData) AcceptedPeers() []string {
	var out []string
	for _, p := range f.SharedWith {
		if p.State == FolderStateAccepted {
			out = append(out, p.PeerID)
		}
	}
	sort.Strings(out)
	return out
}

// WithPeerState returns a copy with peerID's entry set to state. Adds the
// entry when absent; replaces it when present. Never appends a duplicate,
// which a caller doing this by hand gets wrong the first time a peer
// accepts something they had previously declined.
func (f FolderData) WithPeerState(peerID, state string, atMillis uint64, note string) FolderData {
	next := make([]FolderPeerData, 0, len(f.SharedWith)+1)
	replaced := false
	for _, p := range f.SharedWith {
		if p.PeerID == peerID {
			next = append(next, FolderPeerData{PeerID: peerID, State: state, AtMillis: atMillis, Note: note})
			replaced = true
			continue
		}
		next = append(next, p)
	}
	if !replaced {
		next = append(next, FolderPeerData{PeerID: peerID, State: state, AtMillis: atMillis, Note: note})
	}
	sort.Slice(next, func(i, j int) bool { return next[i].PeerID < next[j].PeerID })
	f.SharedWith = next
	return f
}

// FolderID is the identifier of a shared folder, and it is THE SAME
// STRING ON EVERY PEER THAT PARTICIPATES IN IT.
//
// # The defect this closes (S6)
//
// A folder had no identity across peers. `share` wrote
// `folders/{root}`; `accept` wrote `folders/{owner}.{their-root}`.
// Different ids, different roots, no shared name, and nothing joining
// them — so there was no object either side could point at and say "that
// one". Every downstream symptom the operator reported came from this:
// the sources and destinations do not line up because there is nothing
// that says they are two views of one thing.
//
// # Why it is derived and not minted
//
// Syncthing mints a random folder ID and copies it between devices. We
// do not have to: the pair (owner peer-id, the owner's root name) is
// already what identifies the folder, both sides already know both
// halves at the moment they need the id, and deriving it means **no wire
// change and no new field in the offer record** — which matters because
// `app/share/*` is APP-CONVENTION-SHARE's namespace and adding a field
// there is a cross-impl coordination, not a local edit.
//
// It also makes the id impossible to get wrong by construction, which
// the two-typed-strings version was not.
//
// The cost is that renaming the owner's root renames the folder. That is
// already true of the subscription and the sync binding, which are keyed
// on the same name, so this adds no new fragility — it inherits the
// existing one. If we ever need rename-stability, mint at the owner and
// carry it in the offer; do not paper over it by re-deriving elsewhere.
//
// The owner is the peer whose disk the folder originates on: ourselves
// for a folder we share out, the sender for one we accepted.
func FolderID(ownerPeerID, root string) string {
	return SyncBindingKey(ownerPeerID, root)
}

// ReceivedFolderID is FolderID for the receiving side, where the owner is
// the peer we got it from. Kept as its own name because the sync binding
// key is the same string by construction, and a reader at the call site
// wants to be told that rather than to re-derive it.
func ReceivedFolderID(remotePeerID, root string) string {
	return FolderID(remotePeerID, root)
}

// OwnerOf recovers the peer whose disk this folder lives on.
//
// Origin is authoritative when set; the id is the fallback for a record
// written before Origin was, and for one whose Origin says "local",
// where the owner is the reading peer and only they can supply it.
func (f FolderData) OwnerOf(selfPeerID string) string {
	if f.Origin != "" && f.Origin != "local" {
		return f.Origin
	}
	return selfPeerID
}

// MigrateFolderIDs rewrites pre-S6 folder records, whose id was the bare
// root name, to the shared FolderID form. Idempotent; reports what it
// moved so a startup path can say so rather than silently rewriting an
// operator's declarations.
//
// A MIGRATION and not a dual-read in LoadFolder. A dual-read leaves two
// records that can both exist and disagree, and every future reader has
// to know which wins — the ambiguity outlives the transition and is
// exactly the kind of thing this repo has shipped as a silent defect
// before. One pass, one record, and afterwards there is one shape.
func MigrateFolderIDs(st *Store, selfPeerID string) (moved []string, problems []string) {
	if st == nil || selfPeerID == "" {
		return nil, nil
	}
	folders, probs := LoadFolders(st)
	problems = append(problems, probs...)
	for _, f := range folders {
		owner := f.OwnerOf(selfPeerID)
		want := FolderID(owner, f.Root)
		if f.ID == want {
			continue
		}
		if _, clash := LoadFolder(st, want); clash {
			problems = append(problems,
				fmt.Sprintf("folder %q would migrate to %q, which already exists — left alone", f.ID, want))
			continue
		}
		old := f.ID
		f.ID = want
		if err := SaveFolder(st, f); err != nil {
			problems = append(problems, fmt.Sprintf("migrate folder %q: %v", old, err))
			continue
		}
		RemoveFolder(st, old)
		moved = append(moved, old+" -> "+want)
	}
	sort.Strings(moved)
	sort.Strings(problems)
	return moved, problems
}

// --- persistence -----------------------------------------------------
//
// Every List below goes through RelativeUnder rather than TrimPrefix.
// Store.List returns PEER-QUALIFIED paths, so trimming a relative prefix
// matches nothing and the guard after it gets a confidently wrong answer
// — AP58, which this repo shipped twice and the kernel still ships. The
// helper is idempotent, so applying it is never wrong.

// SaveDevice persists a device record.
func SaveDevice(st *Store, d DeviceData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if err := validateSegment(d.PeerID, "device peer-id"); err != nil {
		return err
	}
	if _, err := st.Put(DevicePrefix+d.PeerID, DeviceType, d); err != nil {
		return fmt.Errorf("persist device %s: %w", d.PeerID, err)
	}
	return nil
}

// LoadDevice reads one device record.
func LoadDevice(st *Store, peerID string) (DeviceData, bool) {
	if st == nil || peerID == "" {
		return DeviceData{}, false
	}
	ent, ok := st.Get(DevicePrefix + peerID)
	if !ok || ent.Type != DeviceType {
		return DeviceData{}, false
	}
	var d DeviceData
	if err := ecf.Decode(ent.Data, &d); err != nil {
		return DeviceData{}, false
	}
	if d.PeerID == "" {
		d.PeerID = peerID
	}
	return d, true
}

// RemoveDevice deletes a device record. Reports whether one was there.
//
// It removes the DECLARATION only. Folders shared with that peer, the
// files already received from it, and the capability policy row are
// separate records with separate owners — deleting a device must not
// delete an operator's bytes, and the reconciler is what withdraws the
// rest on its next pass.
func RemoveDevice(st *Store, peerID string) bool {
	if st == nil || peerID == "" {
		return false
	}
	if _, ok := st.Get(DevicePrefix + peerID); !ok {
		return false
	}
	return st.Remove(DevicePrefix + peerID)
}

// LoadDevices reads every device record, sorted by peer-id.
func LoadDevices(st *Store) (devices []DeviceData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(DevicePrefix) {
		seg, under := RelativeUnder(e.Path, DevicePrefix)
		if !under || seg == "" || strings.Contains(seg, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, seg+": listed but not resolvable")
			continue
		}
		if ent.Type != DeviceType {
			problems = append(problems, seg+": unexpected type "+ent.Type)
			continue
		}
		var d DeviceData
		if err := ecf.Decode(ent.Data, &d); err != nil {
			problems = append(problems, seg+": did not decode: "+err.Error())
			continue
		}
		if d.PeerID == "" {
			d.PeerID = seg
		}
		devices = append(devices, d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].PeerID < devices[j].PeerID })
	sort.Strings(problems)
	return devices, problems
}

// SaveFolder persists a folder record.
func SaveFolder(st *Store, f FolderData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if err := validateSegment(f.ID, "folder id"); err != nil {
		return err
	}
	if f.Kind == "" {
		f.Kind = "files"
	}
	if f.Origin == "" {
		f.Origin = "local"
	}
	// EffectiveMode, never a flat `both`. Defaulting an absent mode to
	// `both` here started publishing every folder the operator had only
	// ever ACCEPTED — someone else's files going back out over a grant
	// that already existed — the moment the reconciler began reading the
	// field. Caught by TestReconcile_SaysNothingAboutDialingAPeerWeOnly-
	// RECEIVEFrom, which passes a record with no Mode precisely because
	// that is what every pre-S6 record on disk looks like.
	f.Mode = f.EffectiveMode()
	if _, err := st.Put(FolderPrefix+f.ID, FolderType, f); err != nil {
		return fmt.Errorf("persist folder %s: %w", f.ID, err)
	}
	return nil
}

// LoadFolder reads one folder record.
func LoadFolder(st *Store, id string) (FolderData, bool) {
	if st == nil || id == "" {
		return FolderData{}, false
	}
	ent, ok := st.Get(FolderPrefix + id)
	if !ok || ent.Type != FolderType {
		return FolderData{}, false
	}
	var f FolderData
	if err := ecf.Decode(ent.Data, &f); err != nil {
		return FolderData{}, false
	}
	if f.ID == "" {
		f.ID = id
	}
	return f, true
}

// RemoveFolder deletes a folder record. Like RemoveDevice it removes the
// declaration and nothing on disk.
func RemoveFolder(st *Store, id string) bool {
	if st == nil || id == "" {
		return false
	}
	if _, ok := st.Get(FolderPrefix + id); !ok {
		return false
	}
	return st.Remove(FolderPrefix + id)
}

// LoadFolders reads every folder record, sorted by id.
func LoadFolders(st *Store) (folders []FolderData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(FolderPrefix) {
		seg, under := RelativeUnder(e.Path, FolderPrefix)
		if !under || seg == "" || strings.Contains(seg, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, seg+": listed but not resolvable")
			continue
		}
		if ent.Type != FolderType {
			problems = append(problems, seg+": unexpected type "+ent.Type)
			continue
		}
		var f FolderData
		if err := ecf.Decode(ent.Data, &f); err != nil {
			problems = append(problems, seg+": did not decode: "+err.Error())
			continue
		}
		if f.ID == "" {
			f.ID = seg
		}
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].ID < folders[j].ID })
	sort.Strings(problems)
	return folders, problems
}

// validateSegment refuses an id that would not be a single path segment.
//
// A REFUSAL, not a sanitization. Silently rewriting an id produces a
// record at a path the caller did not ask for, which then cannot be found
// by the name they used — a tolerant fallback turning malformed input
// into a well-formed entity, which is AP33 exactly.
func validateSegment(s, what string) error {
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("%s is empty", what)
	case strings.Contains(s, "/"):
		return fmt.Errorf("%s %q contains a path separator", what, s)
	case s == "." || s == "..":
		return fmt.Errorf("%s %q is not a usable path segment", what, s)
	}
	return nil
}
