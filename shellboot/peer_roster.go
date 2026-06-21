// peer_roster.go — tree-persisted peer roster.
//
// The system peer's tree mirrors the in-memory peer set at
// `app/{appID}/system/peers/{peer_id}`. PeerManager owns the read +
// write sides; this file holds the path scheme + entity-type
// constant + the serialization helpers.
//
// Convention matches godot's `app/godot-workbench/system/peers/{pid}`
// pattern — same shape, app-id is the only difference.
//
// Moved from avalonia/bridge/roster.go into shellboot as part of the
// Phase I §12 refactor (peer manager logic out of the renderer
// adapter and into the renderer-neutral shared package).

package shellboot

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
)

const (
	// RosterEntryType is the canonical entity type for roster
	// entries. Matches godot's `app/state/peer_roster_entry`
	// (no `_godot` suffix — that was preserved upstream for
	// cross-impl schema-agreement-pending reasons we don't share).
	RosterEntryType = "app/state/peer_roster_entry"
)

// RosterEntry is the on-tree shape for one peer in the roster.
// Stored CBOR-encoded under `app/{appID}/system/peers/{pid}`.
//
// Field shape mirrors godot's PEER_ROSTER_ENTITY_TYPE (per
// SIBLING-FRONTEND-SURVEY §3.5) — keeping the same field names + types
// pays off when cross-impl roster sharing becomes a real ask.
//
// StoragePath is workbench-go-specific (godot doesn't surface it
// because the godot wrapper auto-derives) — required at restore time
// for peers that used an explicit path. Persisted as empty for peers
// using derived paths so the restore call gets the same value back.
type RosterEntry struct {
	PeerID      string `json:"peer_id" cbor:"peer_id"`
	Label       string `json:"label" cbor:"label"`
	AddedAt     int64  `json:"added_at" cbor:"added_at"`
	IsFavorite  bool   `json:"is_favorite" cbor:"is_favorite"`
	Identity    string `json:"identity,omitempty" cbor:"identity,omitempty"`
	StorageKind string `json:"storage_kind,omitempty" cbor:"storage_kind,omitempty"`
	StoragePath string `json:"storage_path,omitempty" cbor:"storage_path,omitempty"`
	ListenAddr  string `json:"listen_addresses,omitempty" cbor:"listen_addresses,omitempty"`
}

// RosterPath returns the canonical tree path for a peer's roster entry
// on the system peer's tree, given the app-id namespace.
func RosterPath(appID, peerID string) string {
	return fmt.Sprintf("app/%s/system/peers/%s", appID, peerID)
}

// RosterPrefix returns the directory prefix that lists every roster
// entry — `app/{appID}/system/peers/`. Used to enumerate at boot.
func RosterPrefix(appID string) string {
	return fmt.Sprintf("app/%s/system/peers/", appID)
}

// WriteRosterEntry mirrors a peer's roster entry to the system peer's
// tree. systemPeer.Put encodes via the registered entity-type codec.
func WriteRosterEntry(systemPeer *entitysdk.AppPeer, appID string, e RosterEntry) error {
	if systemPeer == nil {
		return fmt.Errorf("WriteRosterEntry: nil system peer")
	}
	if e.PeerID == "" {
		return fmt.Errorf("WriteRosterEntry: empty peer_id")
	}
	_, err := systemPeer.Put(RosterPath(appID, e.PeerID), RosterEntryType, e)
	if err != nil {
		return fmt.Errorf("WriteRosterEntry: put %s: %w", RosterPath(appID, e.PeerID), err)
	}
	return nil
}

// RemoveRosterEntry deletes the on-tree entry for peerID. Called from
// PeerManager.Destroy. No-op if the entry doesn't exist.
func RemoveRosterEntry(systemPeer *entitysdk.AppPeer, appID string, peerID string) error {
	if systemPeer == nil || peerID == "" {
		return nil
	}
	return systemPeer.Remove(RosterPath(appID, peerID))
}

// HasRosterEntry checks whether the on-tree entry for peerID exists.
// Used by tests to assert cascade-on-destroy.
func HasRosterEntry(systemPeer *entitysdk.AppPeer, appID string, peerID string) (bool, error) {
	if systemPeer == nil {
		return false, fmt.Errorf("HasRosterEntry: nil system peer")
	}
	return systemPeer.Has(RosterPath(appID, peerID))
}

// ListRosterEntries reads every roster entry mirrored on the system
// peer's tree. Returned in arbitrary order (List doesn't promise
// stable ordering across implementations). Callers that want a stable
// order should sort by AddedAt.
func ListRosterEntries(systemPeer *entitysdk.AppPeer, appID string) ([]RosterEntry, error) {
	if systemPeer == nil {
		return nil, fmt.Errorf("ListRosterEntries: nil system peer")
	}
	entries, err := systemPeer.List(RosterPrefix(appID))
	if err != nil {
		return nil, fmt.Errorf("ListRosterEntries: list %s: %w", RosterPrefix(appID), err)
	}
	out := make([]RosterEntry, 0, len(entries))
	for _, e := range entries {
		ent, ok, gerr := systemPeer.Get(e.Path)
		if gerr != nil || !ok {
			// Skip silently — a missing entity at a listed path is a
			// transient inconsistency, not a hard error.
			continue
		}
		re := RosterEntry{}
		if err := decodeRosterEntry(ent, &re); err != nil {
			continue // skip undecodable
		}
		out = append(out, re)
	}
	return out, nil
}

// decodeRosterEntry decodes the entity's CBOR-encoded data back into
// a RosterEntry. Mirrors the encode side that AppPeer.Put performs
// via the ecf codec.
func decodeRosterEntry(ent entity.Entity, out *RosterEntry) error {
	return ecf.Decode(ent.Data, out)
}
