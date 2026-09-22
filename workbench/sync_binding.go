package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// sync_binding.go — the durable half of a cross-peer folder sync.
//
// # What a sync is made of
//
// A sync is the receiving side of one folder. Three records, in three
// places, and only the middle one was ever ours:
//
//	system/subscription/{id}     KERNEL. The subscription on the REMOTE
//	                             peer's local/files/{root}/* prefix, with
//	                             include_payload, delivering to our
//	                             workbench/blob-resolve handler. Persistent
//	                             extension state; the engine rebuilds its
//	                             runtime index from it at open (see
//	                             entitysdk/app.go — it did not until
//	                             2026-09-02, and every mount in this repo
//	                             stopped working after a restart because of
//	                             it).
//	app/workbench/syncs/{key}    OURS. What this file holds.
//	system/config/local/files/…  KERNEL. The local mount the materialized
//	                             bytes are written into.
//
// Our half is the source→target mapping [BlobResolveHandler] routes on
// when a notification arrives. It is the same fact, and the same failure
// mode, as [MountBindingData]: held only in a handler's memory, a restart
// brings the subscription back and every delivery answers
// `404 no_mount_for_uri`. Writing mount_binding.go taught us this once;
// this file is that lesson applied before the bug rather than after it.
//
// # Why the key is composite
//
// One peer may sync several roots, and one root may be synced from
// several peers — neither the root name nor the remote peer-id is unique
// on its own. The key is `{remotePeerID}.{root}`, a single path segment,
// which is unambiguous because a root name is produced by
// `sanitizeRootName` and contains only `[a-z0-9-]`, and a Base58 peer-id
// contains no `.` either. Splitting on the last `.` therefore always
// recovers both halves.

// SyncBindingPrefix is where the workbench-application half of each sync
// lives. Under `app/workbench/`, the application namespace that extends
// freely — as opposed to anything `system/*`, which is spec-adjacent and
// would need cross-impl coordination.
const SyncBindingPrefix = "app/workbench/syncs/"

// SyncBindingType is the entity type at SyncBindingPrefix.
const SyncBindingType = "workbench/sync-binding"

// SyncBindingData is the workbench-owned record of one inbound sync.
type SyncBindingData struct {
	// RemotePeerID is the peer whose prefix we subscribed to.
	RemotePeerID string `cbor:"remote_peer_id"`
	// Root is the mount root name, which is the same on both sides —
	// the source prefix is derived from it.
	Root string `cbor:"root"`
	// SourcePrefix is the remote prefix the subscription watches. Stored
	// rather than derived for MountBindingData's reason: it is the key
	// the handler routes on, and a record carrying only the target would
	// force every reader to re-derive what it applied to.
	SourcePrefix string `cbor:"source_prefix"`
	// TargetPrefix is the local prefix the materialized file is written
	// under. Equal to SourcePrefix in the symmetric case, which is the
	// common one, and distinct when the two peers mount the same folder
	// under different names.
	TargetPrefix string `cbor:"target_prefix"`
	// SubscriptionID is the kernel subscription this sync established, so
	// Unsync can cancel exactly the one it created rather than guessing
	// from a pattern match.
	SubscriptionID string `cbor:"subscription_id"`
}

// SyncBindingKey is the single path segment one sync is stored under.
func SyncBindingKey(remotePeerID, root string) string {
	return remotePeerID + "." + root
}

// SplitSyncBindingKey recovers (remotePeerID, root) from a key. Splits on
// the LAST dot: a root name cannot contain one, so anything before it
// belongs to the peer-id.
func SplitSyncBindingKey(key string) (remotePeerID, root string, ok bool) {
	i := strings.LastIndex(key, ".")
	if i <= 0 || i == len(key)-1 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

// SaveSyncBinding persists the workbench half of a sync.
func SaveSyncBinding(st *Store, b SyncBindingData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if b.RemotePeerID == "" || b.Root == "" {
		return fmt.Errorf("sync binding needs a remote peer id and a root name")
	}
	b.SourcePrefix = ensureSlash(b.SourcePrefix)
	b.TargetPrefix = ensureSlash(b.TargetPrefix)
	key := SyncBindingKey(b.RemotePeerID, b.Root)
	if _, err := st.Put(SyncBindingPrefix+key, SyncBindingType, b); err != nil {
		return fmt.Errorf("persist sync binding %s: %w", key, err)
	}
	return nil
}

// LoadSyncBinding reads one sync's binding.
func LoadSyncBinding(st *Store, remotePeerID, root string) (SyncBindingData, bool) {
	if st == nil || remotePeerID == "" || root == "" {
		return SyncBindingData{}, false
	}
	ent, ok := st.Get(SyncBindingPrefix + SyncBindingKey(remotePeerID, root))
	if !ok || ent.Type != SyncBindingType {
		return SyncBindingData{}, false
	}
	var b SyncBindingData
	if err := ecf.Decode(ent.Data, &b); err != nil {
		return SyncBindingData{}, false
	}
	return b, true
}

// RemoveSyncBinding drops one sync's binding. Reports whether something
// was there.
func RemoveSyncBinding(st *Store, remotePeerID, root string) bool {
	if st == nil || remotePeerID == "" || root == "" {
		return false
	}
	return st.Remove(SyncBindingPrefix + SyncBindingKey(remotePeerID, root))
}

// LoadSyncBindings reads every binding, sorted by key.
//
// Returns the rows it could decode and a separate list of the ones it
// could not, rather than failing the whole read on one bad row — a single
// corrupt binding must not take every other sync down with it, and must
// also not vanish (AP33).
func LoadSyncBindings(st *Store) (bindings []SyncBindingData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(SyncBindingPrefix) {
		// RelativeUnder, never TrimPrefix: Store.List returns
		// PEER-QUALIFIED paths and the prefix passed in is canonicalized
		// on the way, so a TrimPrefix with the relative prefix trims
		// nothing and every guard after it gets a confidently wrong
		// answer (AP58).
		key, under := RelativeUnder(e.Path, SyncBindingPrefix)
		if !under || key == "" || strings.Contains(key, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, key+": listed but not resolvable")
			continue
		}
		if ent.Type != SyncBindingType {
			problems = append(problems, key+": unexpected type "+ent.Type)
			continue
		}
		var b SyncBindingData
		if err := ecf.Decode(ent.Data, &b); err != nil {
			problems = append(problems, key+": did not decode: "+err.Error())
			continue
		}
		if b.RemotePeerID == "" || b.Root == "" {
			if peerID, root, ok := SplitSyncBindingKey(key); ok {
				if b.RemotePeerID == "" {
					b.RemotePeerID = peerID
				}
				if b.Root == "" {
					b.Root = root
				}
			}
		}
		bindings = append(bindings, b)
	}
	sort.Slice(bindings, func(i, j int) bool {
		return SyncBindingKey(bindings[i].RemotePeerID, bindings[i].Root) <
			SyncBindingKey(bindings[j].RemotePeerID, bindings[j].Root)
	})
	sort.Strings(problems)
	return bindings, problems
}

// RestoreSyncBindings re-registers every persisted sync with the
// blob-resolve handler. Call once at startup.
//
// Only the handler mapping needs restoring: the subscription itself is
// persistent kernel state and the engine rebuilds its runtime index at
// open. That was not true before 2026-09-02 — nothing called
// `subscription.Engine.Load` — and a sync established before that fix
// would come back with a live mapping and a dead subscription, which is
// the same "healthy and producing nothing" shape from the other side.
//
// Problems are RETURNED rather than logged, because the caller is startup
// code with a stderr an operator reads, and a sync that silently fails to
// come back has no other symptom than files that stop arriving.
func RestoreSyncBindings(st *Store, br *BlobResolveHandler) (restored int, problems []string) {
	if st == nil || br == nil {
		return 0, nil
	}
	bindings, problems := LoadSyncBindings(st)
	for _, b := range bindings {
		if b.SourcePrefix == "" || b.TargetPrefix == "" {
			problems = append(problems,
				SyncBindingKey(b.RemotePeerID, b.Root)+
					": binding has an empty prefix, cannot route (source="+
					b.SourcePrefix+" target="+b.TargetPrefix+")")
			continue
		}
		br.RegisterMount(b.SourcePrefix, b.TargetPrefix)
		restored++
	}
	return restored, problems
}
