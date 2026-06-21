package entitysdk_test

import (
	"context"
	"database/sql"
	hexenc "encoding/hex"
	"path/filepath"
	"sort"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestDiag_IdentityRebootstrapLeakPaths is the path-level probe:
// snapshots the location index (path → hash) across a reload and
// reports which paths drift. Skipped on master; run on demand.
func TestDiag_IdentityRebootstrapLeakPaths(t *testing.T) {
	t.Skip("diagnostic — run on demand to investigate the +4-entities-per-reload leak")

	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, "peer.db")
	ctx := context.Background()

	snapshot := func() map[string]string {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Identity: &entitysdk.IdentityBindingConfig{Name: "diag"},
			Storage:  entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("CreatePeer: %v", err)
		}
		defer ap.Close()
		entries := ap.Store().List("")
		m := make(map[string]string, len(entries))
		for _, e := range entries {
			m[e.Path] = e.Hash.String()
		}
		return m
	}

	func() {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("initial CreatePeer: %v", err)
		}
		defer ap.Close()
		if _, err := ap.BootstrapIdentity(ctx, entitysdk.BootstrapOpts{
			QuorumMembers:   3,
			QuorumThreshold: 2,
			QuorumName:      "diag",
			BundleName:      "diag",
		}); err != nil {
			t.Fatalf("BootstrapIdentity: %v", err)
		}
	}()

	before := snapshot()
	after := snapshot()

	type change struct{ path, before, after string }
	var changed []change
	for path, beforeHash := range before {
		if afterHash, ok := after[path]; ok && afterHash != beforeHash {
			changed = append(changed, change{path, beforeHash, afterHash})
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed = append(changed, change{path, "(absent)", after[path]})
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i].path < changed[j].path })

	t.Logf("paths with content drift across reload: %d", len(changed))
	for _, c := range changed {
		t.Logf("  %s\n    before: %s\n    after:  %s", c.path, c.before, c.after)
	}
}

// TestDiag_IdentityRebootstrapLeakEntities goes deeper: snapshots
// the SQLite content store directly (not just the location index)
// across a reload, and reports every entity hash that's new in the
// second snapshot. This catches entities that exist in the content
// store but aren't bound to any tree path — i.e. envelope wrappers
// or other intermediate entities that the path-level diagnostic
// misses.
//
// Required to explain the 4-vs-2 discrepancy: the path diagnostic
// shows 2 paths drift per reload, but EntityCount grows by 4 per
// reload. This test names every new entity (including its type
// and a content excerpt) so we can identify the missing 2.
//
// Skipped on master; run on demand.
func TestDiag_IdentityRebootstrapLeakEntities(t *testing.T) {
	t.Skip("diagnostic — run on demand to enumerate the full set of new entities per reload")

	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, "peer.db")
	ctx := context.Background()

	// Initial bootstrap.
	func() {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("initial CreatePeer: %v", err)
		}
		defer ap.Close()
		if _, err := ap.BootstrapIdentity(ctx, entitysdk.BootstrapOpts{
			QuorumMembers:   3,
			QuorumThreshold: 2,
			QuorumName:      "diag",
			BundleName:      "diag",
		}); err != nil {
			t.Fatalf("BootstrapIdentity: %v", err)
		}
	}()

	type entitySnap struct {
		hash       string
		entityType string
		dataLen    int
		dataHex    string // first 64 bytes as hex for diff visibility
	}
	enumerate := func() map[string]entitySnap {
		// Open the DB directly. The "sqlite" driver was registered
		// by core-go's transitive import.
		db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
		if err != nil {
			t.Fatalf("sql.Open ro: %v", err)
		}
		defer db.Close()
		rows, err := db.Query(`SELECT hash, entity_type, data FROM entities`)
		if err != nil {
			t.Fatalf("SELECT entities: %v", err)
		}
		defer rows.Close()
		m := make(map[string]entitySnap)
		for rows.Next() {
			var h, dat []byte
			var typ string
			if err := rows.Scan(&h, &typ, &dat); err != nil {
				t.Fatalf("scan: %v", err)
			}
			hashHex := hexenc.EncodeToString(h)
			head := dat
			if len(head) > 64 {
				head = head[:64]
			}
			m[hashHex] = entitySnap{
				hash:       hashHex,
				entityType: typ,
				dataLen:    len(dat),
				dataHex:    hexenc.EncodeToString(head),
			}
		}
		return m
	}

	// Open + close once to trigger ApplyIdentityBundle on existing
	// state. This is the "reload that leaks 4 entities" cycle.
	before := enumerate()
	func() {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Identity: &entitysdk.IdentityBindingConfig{Name: "diag"},
			Storage:  entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("reload CreatePeer: %v", err)
		}
		_ = ap.Close()
	}()
	after := enumerate()

	t.Logf("content-store size: before=%d after=%d (Δ=%d)",
		len(before), len(after), len(after)-len(before))

	// Type breakdown before/after — surfaces whether the ceremony
	// over-issues caps vs whether reload is adding net-new entity
	// kinds.
	countByType := func(m map[string]entitySnap) map[string]int {
		c := make(map[string]int)
		for _, e := range m {
			c[e.entityType]++
		}
		return c
	}
	beforeTypes := countByType(before)
	afterTypes := countByType(after)
	allTypes := map[string]struct{}{}
	for k := range beforeTypes {
		allTypes[k] = struct{}{}
	}
	for k := range afterTypes {
		allTypes[k] = struct{}{}
	}
	var typeNames []string
	for k := range allTypes {
		typeNames = append(typeNames, k)
	}
	sort.Strings(typeNames)
	t.Logf("entity-type counts (only types where count changed):")
	for _, k := range typeNames {
		if beforeTypes[k] != afterTypes[k] {
			t.Logf("  %-40s before=%2d after=%2d Δ=%+d",
				k, beforeTypes[k], afterTypes[k], afterTypes[k]-beforeTypes[k])
		}
	}

	// Find entities present in `after` but not in `before` — the new
	// ones deposited by the reload.
	var newHashes []string
	for h := range after {
		if _, ok := before[h]; !ok {
			newHashes = append(newHashes, h)
		}
	}
	sort.Strings(newHashes)

	t.Logf("new entities in content store after reload: %d", len(newHashes))
	for _, h := range newHashes {
		e := after[h]
		t.Logf("  hash:     %s", e.hash)
		t.Logf("    type:   %s", e.entityType)
		t.Logf("    bytes:  %d", e.dataLen)
		t.Logf("    head:   %s", e.dataHex)
	}
}
