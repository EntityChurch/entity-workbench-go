package entitysdk

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
)

// TestCheckSqliteSchemaVersion_AcceptsFreshOrCompatible covers the
// happy paths: ":memory:" is always OK; a non-existent file is OK
// (gets initialized on first open); an existing v1 DB is OK.
func TestCheckSqliteSchemaVersion_AcceptsFreshOrCompatible(t *testing.T) {
	dir := t.TempDir()

	// In-memory path is always fine.
	if err := checkSqliteSchemaVersion(":memory:"); err != nil {
		t.Errorf(":memory: should be OK, got %v", err)
	}

	// Non-existent file is fine (NewSqliteStore will create it).
	missing := filepath.Join(dir, "does-not-exist.db")
	if err := checkSqliteSchemaVersion(missing); err != nil {
		t.Errorf("missing file should be OK, got %v", err)
	}

	// A fresh DB at user_version=1 (what NewSqliteStore creates).
	v1Path := filepath.Join(dir, "v1.db")
	writeRawSqlite(t, v1Path, 1)
	if err := checkSqliteSchemaVersion(v1Path); err != nil {
		t.Errorf("v1 DB should be OK, got %v", err)
	}

	// Older versions (e.g. v0) are also fine — represents a DB that
	// pre-dates the user_version convention. Forward-compat check
	// only refuses *newer* versions.
	v0Path := filepath.Join(dir, "v0.db")
	writeRawSqlite(t, v0Path, 0)
	if err := checkSqliteSchemaVersion(v0Path); err != nil {
		t.Errorf("v0 DB should be OK, got %v", err)
	}
}

// TestCheckSqliteSchemaVersion_RejectsFutureVersion is the
// load-bearing assertion: if some future workbench binary wrote a
// DB at user_version=N where N > store.SchemaVersion, a
// current-binary attempt to open it must refuse rather than
// downgrade / misinterpret it.
//
// This is the case the user actually cares about: deploy a peer,
// upgrade the workbench, then need to roll back; the rolled-back
// binary must not silently clobber state written by the newer one.
func TestCheckSqliteSchemaVersion_RejectsFutureVersion(t *testing.T) {
	dir := t.TempDir()
	futurePath := filepath.Join(dir, "future.db")
	writeRawSqlite(t, futurePath, store.SchemaVersion+1)

	err := checkSqliteSchemaVersion(futurePath)
	if err == nil {
		t.Fatal("expected error for future-version DB, got nil")
	}
	if got := StatusOf(err); got != 503 {
		t.Errorf("expected 503 schema-too-new, got status=%d err=%v", got, err)
	}
}

// TestCheckSqliteSchemaVersion_WiredIntoCreatePeer verifies the
// pre-flight check actually runs in the CreatePeer code path.
// Hand-craft a v99 DB on disk, point CreatePeer at it, expect a
// 503 storage_schema_too_new at peer construction time — BEFORE any
// init() runs and clobbers the version marker.
func TestCheckSqliteSchemaVersion_WiredIntoCreatePeer(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future.db")
	writeRawSqlite(t, dbPath, 99)

	_, err := CreatePeer(PeerConfig{
		Storage: StorageConfig{Kind: "sqlite", Path: dbPath},
	})
	if err == nil {
		t.Fatal("CreatePeer should refuse to open a v99 DB")
	}
	if got := StatusOf(err); got != 503 {
		t.Errorf("expected 503 schema-too-new from CreatePeer, got status=%d err=%v", got, err)
	}

	// Confirm the bug-we-set-up-the-check-to-prevent didn't happen:
	// the on-disk user_version should still be 99 because we refused
	// before NewSqliteStore could run init() and clobber it.
	got := readRawSqliteUserVersion(t, dbPath)
	if got != 99 {
		t.Errorf("after refused-open, on-disk user_version = %d, want 99 (init() clobbered it — pre-flight ordering is wrong)",
			got)
	}
}

// writeRawSqlite creates an empty SQLite DB at path and sets
// user_version. Uses database/sql directly so we don't pull in
// core-go's full schema setup.
func writeRawSqlite(t *testing.T, path string, userVersion int) {
	t.Helper()
	// Ensure parent dir exists.
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	// Touch a trivial table so the file is a real SQLite DB, not an
	// empty file. PRAGMA user_version persists only after the
	// database has been written.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS marker (k INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = ` + itoa(userVersion)); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
}

// readRawSqliteUserVersion peeks at user_version through a raw
// open — used to verify pre-flight ordering didn't let init() run.
func readRawSqliteUserVersion(t *testing.T, path string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("sql.Open ro: %v", err)
	}
	defer db.Close()
	var v int64
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0
		}
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	return v
}

// itoa duplicated to keep this file standalone — see
// storage_sqlite_identity_test.go for the test-side version.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
