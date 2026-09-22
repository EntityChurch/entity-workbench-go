package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// history_limit.go — the durable record of a path whose change recording
// we STOPPED, and why.
//
// # What this is a record of
//
// A folder that receives has change recording switched on for its whole
// mount prefix (`shellcmd/folder_history.go`), because
// `DOMAIN-LOCAL-FILES` §1.1a makes an overwritten local edit recoverable
// only if the tree kept a chain position for it. Recording costs a flat
// ~1.2 KB and 2 entities per WRITE, and the latency does not grow — that
// part is measured and affordable (SYNC-LIMITS-AND-FAILURE-MODES §2).
//
// **What is not affordable is that nothing prunes it.** The cost is per
// write, not per file, so one continuously rewritten path — a log, a
// database file, an editor swap file, a build output — is ~1.2 GB/day,
// with no error and no ceiling, until the disk is full.
//
// # Why STOPPING is the only bound available to us
//
// The substrate appears to offer the cheap fix and does not.
// `types.HistoryConfigData.MaxDepth` is documented "Max transitions per
// path" and the recorder calls `prune` after every transition; measured
// (`shellboot/history_maxdepth_probe_test.go`), `max_depth = 3` and
// twelve writes leaves twelve transitions reachable. `prune` walks the
// chain and writes nothing, and it cannot easily do otherwise, because
// transitions are immutable and content-addressed. Nor can a later pass
// reclaim them: a transition is not in the location index at all — only
// the head pointer is — so there is nothing to remove and no garbage
// collector in the cohort to remove it. Routed as
// `reviews/CORE-GO-HISTORY-MAXDEPTH-PRUNES-NOTHING-2026-09-07.md`.
//
// So the only lever that actually bounds disk is to stop adding, and the
// honest way to say that is: **this guard trades the RECENT history of
// one runaway path for the disk the whole peer runs on.** That trade is
// backwards for recovery — you keep the oldest positions and lose the
// newest — and it is stated here rather than buried, because it is the
// reason a tripped limit has to be visible on a surface rather than
// simply happening.
//
// # Why per path and not per folder
//
// The failure is a property of one path's write rate, not of the folder.
// Disabling a whole folder's recording because one log file inside it is
// hot would take the chain away from the documents beside it — which are
// exactly the files a conflict is recovered from. The kernel's own
// specificity rule (EXTENSION-HISTORY §6.2, `ext/history/config.go`
// `moreSpecific`) makes the surgical form work: an exact-path config
// outranks the folder's `…/*` config on literal-segment count, and
// `configCache.find` returns nil when the most specific match is
// disabled. So one disabled exact-path config stops one path and leaves
// the rest of the folder recording.
//
// # Why a record of our own, beside the config that does the work
//
// The config is the mechanism; this is the statement. `HistoryConfigData`
// has nowhere to say when a limit tripped, at what depth, or against what
// budget, and a surface that renders "recording stopped" without those
// numbers is asking an operator to take it on faith. It is also the
// idempotency key: a limit that already exists is never re-derived and
// never re-applied, which is what lets an operator re-enable the config
// by hand without the guard immediately undoing them.

// HistoryLimitPrefix is where the records live. Under `app/workbench/`,
// the application namespace that extends freely — as opposed to anything
// `system/*`, which is spec-adjacent.
const HistoryLimitPrefix = "app/workbench/history-limits/"

// HistoryLimitType is the entity type at HistoryLimitPrefix.
const HistoryLimitType = "workbench/history-limit"

// HistoryLimitData records one path whose recording was stopped.
type HistoryLimitData struct {
	// Path is the qualified tree path recording stopped for, e.g.
	// `/{peer}/local/files/{root}/server.log`.
	Path string `cbor:"path"`
	// Root is the mount root the path belongs to, so a surface can group
	// by folder without re-parsing the path.
	Root string `cbor:"root"`
	// ConfigName is the `system/history/config/{name}` entity that does
	// the stopping. Named so an operator who wants recording back knows
	// exactly which entity to flip, and so a later pass can tell its own
	// config from one somebody wrote by hand.
	ConfigName string `cbor:"config_name"`
	// Transitions is the chain depth measured at the moment the budget
	// tripped — a floor on what was already stored, not an estimate.
	Transitions uint64 `cbor:"transitions"`
	// Budget is the limit that was in force. Recorded because the default
	// can change between builds, and "why did this stop" is unanswerable
	// against a number nobody wrote down.
	Budget uint64 `cbor:"budget"`
	// AtMillis is when the limit tripped, Unix milliseconds.
	AtMillis uint64 `cbor:"at_millis"`
}

// HistoryLimitKey is the single path segment one record is stored under.
//
// A readable tail plus a hash of the whole path: the tail is what makes
// `ls app/workbench/history-limits/` mean something to a person, and the
// hash is what makes the key collision-free for two paths whose tails
// agree (`a/notes.txt` and `b/notes.txt`) or whose sanitized forms
// collide (`a/b` and `a-b`). Deterministic, so writing the same limit
// twice is one entity.
func HistoryLimitKey(path string) string {
	sum := sha256.Sum256([]byte(path))
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

// sanitizeKeySegment reduces a name to something safe in one path
// segment. Deliberately lossy — the exact path is a field on the record,
// so the key never has to be reversible.
func sanitizeKeySegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// SaveHistoryLimit persists one record.
func SaveHistoryLimit(st *Store, l HistoryLimitData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if l.Path == "" {
		return fmt.Errorf("history limit needs a path")
	}
	if _, err := st.Put(HistoryLimitPrefix+HistoryLimitKey(l.Path), HistoryLimitType, l); err != nil {
		return fmt.Errorf("persist history limit for %s: %w", l.Path, err)
	}
	return nil
}

// LoadHistoryLimit reads one path's record.
func LoadHistoryLimit(st *Store, path string) (HistoryLimitData, bool) {
	if st == nil || path == "" {
		return HistoryLimitData{}, false
	}
	ent, ok := st.Get(HistoryLimitPrefix + HistoryLimitKey(path))
	if !ok || ent.Type != HistoryLimitType {
		return HistoryLimitData{}, false
	}
	var l HistoryLimitData
	if err := ecf.Decode(ent.Data, &l); err != nil {
		return HistoryLimitData{}, false
	}
	return l, true
}

// RemoveHistoryLimit drops one record and reports whether one was there.
//
// Removing the record does NOT re-enable recording — the disabled config
// is what stops it, and the two are separate on purpose: an operator who
// wants the path recorded again flips the config, and dropping this
// record only means the guard will measure the path afresh.
func RemoveHistoryLimit(st *Store, path string) bool {
	if st == nil || path == "" {
		return false
	}
	return st.Remove(HistoryLimitPrefix + HistoryLimitKey(path))
}

// LoadHistoryLimits reads every record, sorted by path.
//
// Undecodable rows are reported rather than dropped (AP33): a record that
// exists and does not parse means a path is silently not being recorded
// for a reason nobody can see, which is the exact failure this whole file
// exists to make visible.
func LoadHistoryLimits(st *Store) (limits []HistoryLimitData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(HistoryLimitPrefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		key, under := RelativeUnder(e.Path, HistoryLimitPrefix)
		if !under || key == "" || strings.Contains(key, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, key+": listed but not resolvable")
			continue
		}
		if ent.Type != HistoryLimitType {
			problems = append(problems, key+": unexpected type "+ent.Type)
			continue
		}
		var l HistoryLimitData
		if err := ecf.Decode(ent.Data, &l); err != nil {
			problems = append(problems, key+": did not decode: "+err.Error())
			continue
		}
		limits = append(limits, l)
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].Path < limits[j].Path })
	sort.Strings(problems)
	return limits, problems
}

// Summary is one operator-facing line for a tripped limit.
//
// It names the remedy as well as the fact. "Recording stopped" on its own
// invites the reading that something broke; what actually happened is
// that a bound we chose was reached, and the two available answers —
// leave it, or take the path out of the shared folder — are both the
// operator's to make.
func (l HistoryLimitData) Summary() string {
	return fmt.Sprintf(
		"change recording STOPPED for %s after %d recorded versions (budget %d) — "+
			"something is rewriting this file continuously, and the chain was growing "+
			"without bound. Its earlier versions are still recoverable with "+
			"`history query`; changes from now on are not. Move the file out of the "+
			"shared folder, or re-enable recording by setting %s enabled",
		l.Path, l.Transitions, l.Budget, "system/history/config/"+l.ConfigName)
}
