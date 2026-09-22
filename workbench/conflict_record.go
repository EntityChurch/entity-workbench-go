package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// conflict_record.go — the durable statement that a delivery landed on
// top of somebody's local edit, and where the version it replaced is.
//
// # What a conflict is here, and what it is not
//
// Two peers edit the same path between two syncs. `DOMAIN-LOCAL-FILES`
// §1.1a rules this is **not** a CRDT case: no automatic merge, the tree
// records both writes at distinct chain positions, and the filesystem
// takes the most recent arrival. It also says, in as many words, that an
// application wanting collaborative-edit semantics layers them on top —
// `local/files` does not silently provide them.
//
// So the substrate has always kept both versions and always converged.
// **What was missing is the sentence.** Nothing told the operator their
// edit had been replaced, and nothing could get it back: the bytes were
// on the chain, correctly, at a position no surface rendered and no verb
// reached. A recoverable loss nobody is told about is an unrecoverable
// one.
//
// This record is that sentence, and it is durable because the process
// that noticed is not the process the operator will be running when they
// go looking.
//
// # Why the default does NOT create a second file
//
// `EXTENSION-REVISION` §2.3's `keep-both` writes the losing version to a
// sibling at `{path}.keep-both-{hash8}`. That is the right shape for a
// two-way folder and the wrong default here, for a reason that only shows
// up in the topology this product actually ships:
//
//	one-way share, A owns and B receives.
//	A edits -> hash a. B edits -> hash b.
//	keep-both at B: f = b, f.keep-both-{a8} = a.
//	A still holds f = a, and B does not publish, so A is never told.
//	The two peers now DIVERGE at f, permanently and silently.
//
// Today they converge — B ends at `a`, and `b` is on B's chain. That is
// the better default for a receive-only folder, and it is what §1.1a
// calls the right default for filesystem-backed content. So the default
// keeps convergence and adds the record; keep-both is a per-folder
// declaration for folders that genuinely flow both ways
// ([FolderData.Conflict]).
//
// **Both versions are recoverable either way. The difference is whether
// both are PRESENT.** The words are kept apart deliberately: `keep-both`
// means the sibling file, always, because that is EXTENSION-REVISION's
// word for exactly that and blurring it would make a cross-impl term mean
// two things.
//
// # What recovery rests on
//
// The chain, and therefore on recording being enabled for the path —
// which the reconciler does for every folder that receives
// (`shellcmd/folder_history.go`). Two known holes, both stated rather
// than papered over: a path whose recording hit the growth budget
// (`shellcmd/history_budget.go`) has no new chain positions, so an
// overwrite there is detected but not recoverable; and a folder that
// predates the reconciler's config has no chain at all, so an overwrite
// there is not even detected. Both are reported on the record rather than
// left for a reader to infer from a missing field.

// ConflictPrefix is where conflict records live. Under `app/workbench/`,
// the application namespace that extends freely.
const ConflictPrefix = "app/workbench/conflicts/"

// ConflictType is the entity type at ConflictPrefix.
const ConflictType = "workbench/conflict"

// Conflict resolution outcomes, as recorded on a resolved record. A
// resolved conflict is KEPT rather than deleted — see [ConflictData.Kept].
const (
	// ConflictUnresolved is a conflict the operator has not acted on.
	ConflictUnresolved = ""
	// ConflictKeptTheirs means the delivered version stands, which is
	// also what is already on disk. Resolving this way changes no bytes;
	// it is the operator saying so.
	ConflictKeptTheirs = "theirs"
	// ConflictKeptMine means the overwritten local version was restored
	// from the chain and is on disk again.
	ConflictKeptMine = "mine"
	// ConflictKeptBoth means the replaced version was written to a
	// sibling at the EXTENSION-REVISION §2.3 path, so both are present.
	ConflictKeptBoth = "both"
)

// ConflictData records one delivery that replaced a local edit.
type ConflictData struct {
	// Path is the qualified tree path the collision happened at.
	Path string `cbor:"path"`
	// Root is the mount root, so a surface can group by folder without
	// re-parsing the path.
	Root string `cbor:"root"`
	// RemotePeerID is the peer whose delivery won.
	RemotePeerID string `cbor:"remote_peer_id"`

	// MineHash is the blob the LOCAL edit produced — the version that was
	// replaced. This is the whole point of the record: it is what
	// `resolve --keep-mine` writes back, and without it recovery means an
	// operator walking a chain by hand.
	MineHash hash.Hash `cbor:"mine_hash"`
	// TheirsHash is the delivered blob, which is what is on disk.
	TheirsHash hash.Hash `cbor:"theirs_hash"`

	// Recoverable is whether MineHash is expected to still be reachable.
	// False when the path's recording had been stopped by the growth
	// guard, or was never enabled — in which case this record is a
	// NOTIFICATION and not a recovery handle, and a surface offering
	// "restore mine" against it would be offering something that fails.
	Recoverable bool `cbor:"recoverable"`

	// KeepBothPath is the sibling the replaced version was written to,
	// when the folder declared the keep-both policy. Empty under the
	// default, where nothing new is written to the folder.
	KeepBothPath string `cbor:"keep_both_path"`

	// AtMillis is when the collision was detected, Unix milliseconds.
	AtMillis uint64 `cbor:"at_millis"`

	// Kept is the resolution, one of the ConflictKept* constants, or
	// ConflictUnresolved.
	//
	// A resolved record is kept rather than removed, because "this file
	// was in conflict on Tuesday and you chose theirs" is the answer to
	// the question an operator asks a week later, and a record that
	// deletes itself on resolution cannot answer it. `conflicts -clear`
	// removes them when the operator wants them gone.
	Kept string `cbor:"kept"`
	// ResolvedAtMillis is when Kept was decided.
	ResolvedAtMillis uint64 `cbor:"resolved_at_millis"`
}

// Unresolved reports whether the operator has not yet acted.
func (c ConflictData) Unresolved() bool { return c.Kept == ConflictUnresolved }

// ConflictKey is the single path segment one record is stored under.
//
// Keyed on the PATH and the two hashes together, so a second collision at
// the same path is a second record. Keying on the path alone would make
// the newest collision silently replace the record for the previous one —
// and the previous one is what carries the hash needed to recover an
// edit two overwrites ago.
func ConflictKey(path string, mine, theirs hash.Hash) string {
	sum := sha256.Sum256([]byte(path + "\x00" + mine.String() + "\x00" + theirs.String()))
	tail := path
	if i := strings.LastIndexByte(tail, '/'); i >= 0 {
		tail = tail[i+1:]
	}
	tail = sanitizeKeySegment(tail)
	if tail == "" {
		tail = "path"
	}
	if len(tail) > 40 {
		tail = tail[:40]
	}
	return tail + "-" + hex.EncodeToString(sum[:6])
}

// Key is the record's own storage key.
func (c ConflictData) Key() string { return ConflictKey(c.Path, c.MineHash, c.TheirsHash) }

// KeepBothSuffix builds EXTENSION-REVISION §2.3's sibling path for a
// version.
//
// Byte-identical to `ext/revision/strategy.go`'s keep-both binding —
// `path + ".keep-both-" + hex(hash.Digest[:4])` — on purpose and not by
// coincidence. A sibling name is bytes that land in the tree, so it is a
// Layer-2 algorithm contract (AGENTS.md): two implementations that name
// the same conflict differently produce two files where there should be
// one. Restated rather than imported because the kernel's function is
// unexported; asserted against the kernel's own vectors in
// `workbench/conflict_naming_test.go`.
func KeepBothSuffix(h hash.Hash) string {
	return ".keep-both-" + hex.EncodeToString(h.Digest[:4])
}

// KeepBothPath is the sibling path for a replaced version at path.
func KeepBothPath(path string, h hash.Hash) string {
	return path + KeepBothSuffix(h)
}

// IsKeepBothPath reports whether a path is a keep-both sibling.
//
// Needed by anything that counts or lists a folder's files: a sibling is
// a real file and must not be mistaken for one the operator put there,
// and it must never be treated as a conflict candidate itself.
func IsKeepBothPath(path string) bool {
	i := strings.LastIndex(path, ".keep-both-")
	if i < 0 {
		return false
	}
	suffix := path[i+len(".keep-both-"):]
	if len(suffix) != 8 {
		return false
	}
	for _, r := range suffix {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// SaveConflict persists one record.
func SaveConflict(st *Store, c ConflictData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if c.Path == "" {
		return fmt.Errorf("conflict record needs a path")
	}
	if _, err := st.Put(ConflictPrefix+c.Key(), ConflictType, c); err != nil {
		return fmt.Errorf("persist conflict at %s: %w", c.Path, err)
	}
	return nil
}

// LoadConflict reads one record by key.
func LoadConflict(st *Store, key string) (ConflictData, bool) {
	if st == nil || key == "" {
		return ConflictData{}, false
	}
	ent, ok := st.Get(ConflictPrefix + key)
	if !ok || ent.Type != ConflictType {
		return ConflictData{}, false
	}
	var c ConflictData
	if err := ecf.Decode(ent.Data, &c); err != nil {
		return ConflictData{}, false
	}
	return c, true
}

// RemoveConflict drops one record.
func RemoveConflict(st *Store, key string) bool {
	if st == nil || key == "" {
		return false
	}
	return st.Remove(ConflictPrefix + key)
}

// LoadConflicts reads every record, newest first.
//
// Newest first because a conflict list is read to answer "what just
// happened", and the answer is at the top. Undecodable rows are reported
// rather than dropped (AP33) — a record that exists and does not parse
// means an overwritten edit nobody can find.
func LoadConflicts(st *Store) (conflicts []ConflictData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(ConflictPrefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		key, under := RelativeUnder(e.Path, ConflictPrefix)
		if !under || key == "" || strings.Contains(key, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, key+": listed but not resolvable")
			continue
		}
		if ent.Type != ConflictType {
			problems = append(problems, key+": unexpected type "+ent.Type)
			continue
		}
		var c ConflictData
		if err := ecf.Decode(ent.Data, &c); err != nil {
			problems = append(problems, key+": did not decode: "+err.Error())
			continue
		}
		conflicts = append(conflicts, c)
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].AtMillis != conflicts[j].AtMillis {
			return conflicts[i].AtMillis > conflicts[j].AtMillis
		}
		return conflicts[i].Path < conflicts[j].Path
	})
	sort.Strings(problems)
	return conflicts, problems
}

// UnresolvedConflicts is the subset an operator still has to decide.
func UnresolvedConflicts(st *Store) (conflicts []ConflictData, problems []string) {
	all, problems := LoadConflicts(st)
	for _, c := range all {
		if c.Unresolved() {
			conflicts = append(conflicts, c)
		}
	}
	return conflicts, problems
}

// ShortPeerID abbreviates a Base58 peer-id for an operator-facing line.
//
// Here rather than beside its shellcmd twin because a conflict summary is
// composed in the model layer, so both frontends print the same sentence
// — the rule the whole workbench/renderer split exists for.
func ShortPeerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}

// Summary is one operator-facing line.
func (c ConflictData) Summary() string {
	switch c.Kept {
	case ConflictKeptMine:
		return fmt.Sprintf("%s — your version was restored", c.Path)
	case ConflictKeptTheirs:
		return fmt.Sprintf("%s — you kept the version that arrived", c.Path)
	case ConflictKeptBoth:
		return fmt.Sprintf("%s — both kept; yours is at %s", c.Path, c.KeepBothPath)
	}
	if !c.Recoverable {
		return fmt.Sprintf(
			"%s — a change from %s replaced an edit you made here, and the "+
				"replaced version is NOT recoverable: this path had no change "+
				"recording at the time. Nothing else was lost",
			c.Path, ShortPeerID(c.RemotePeerID))
	}
	return fmt.Sprintf(
		"%s — a change from %s replaced an edit you made here. Your version is "+
			"still recoverable: `resolve %s -keep mine`",
		c.Path, ShortPeerID(c.RemotePeerID), c.Key())
}
