package entitysdk

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"go.entitychurch.org/entity-core-go/core/store"
)

// checkSqliteSchemaVersion peeks at an existing SQLite DB file's
// `PRAGMA user_version` and returns an error if it's newer than
// what this binary supports. Layered defense: core-go's
// `SqliteStore.init` now refuses to open a too-new DB itself (per
// CORE-GO-FIXES §3), but the workbench pre-flight gives a cleaner
// 503 `storage_schema_too_new` surface before construction.
//
// Returns nil when:
//   - path is `:memory:` (nothing to check; in-memory DBs are always fresh)
//   - the file doesn't exist yet (a fresh DB will get initialized correctly)
//   - the file has `user_version <= store.SchemaVersion`
//
// Returns a wrapped 503 SDK error when:
//   - the file exists but isn't a valid SQLite DB
//   - `user_version` is greater than this binary's supported version
//     (a future-binary wrote this DB and the operator should upgrade
//     the workbench binary before re-opening)
//
// Read-only: opens with `mode=ro` so we can't accidentally write
// the file. WAL-related side effects (rollback journal creation,
// etc.) shouldn't happen in read-only mode.
func checkSqliteSchemaVersion(path string) error {
	if path == ":memory:" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// Fresh DB — `store.NewSqliteStore` will create it and set
		// `user_version` correctly. Nothing to check.
		return nil
	} else if err != nil {
		return WrapError(500, "storage_stat_failed",
			"stat sqlite path "+path, err)
	}

	// modernc.org/sqlite's URI form: file:<path>?mode=ro
	dsn := "file:" + path + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return WrapError(503, "storage_open_failed",
			"open sqlite read-only for version check at "+path, err)
	}
	defer db.Close()

	var version int64
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return WrapError(503, "storage_version_read_failed",
			"read PRAGMA user_version at "+path, err)
	}
	if version > store.SchemaVersion {
		return NewError(503, "storage_schema_too_new",
			fmt.Sprintf("sqlite DB at %s has user_version=%d but this binary supports up to %d; upgrade the workbench binary before re-opening",
				path, version, store.SchemaVersion))
	}
	return nil
}
