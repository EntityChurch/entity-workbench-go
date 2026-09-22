package shellcmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"
)

// cmdHistory dispatches the `history <subcommand> [args...]` command
// surface against the local peer's history extension. Per
// SHELL-DIRECTION.md (shell-first feature development) this mirrors
// the typed HistoryClient.
//
// Subcommands:
//
//	history config <pattern>            — install a recording config
//	                                      matching pattern (e.g. "workspace/*")
//	history query <path> [-limit N]     — show transitions newest-first
//	history rollback <path> <hash>      — rebind path to an earlier hash
//
// Recording is opt-in: the recorder requires a HistoryConfigData
// match before it tracks a path. Without a config, queries return
// empty and rollback returns 404.
func cmdHistory(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: history <config|query|rollback> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "config":
		return cmdHistoryConfig(sh, rest)
	case "query", "log":
		return cmdHistoryQuery(sh, rest)
	case "rollback":
		return cmdHistoryRollback(sh, rest)
	default:
		return Result{}, fmt.Errorf("unknown history subcommand: %s", sub)
	}
}

// cmdHistoryConfig installs a recording config at
// system/history/config/{name} matching pattern. The recorder's
// config cache hot-reloads on writes to that prefix, so subsequent
// mutations begin recording immediately.
func cmdHistoryConfig(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: history config <pattern> [name]")
	}
	pattern := args[0]
	name := "default"
	if len(args) >= 2 {
		name = args[1]
	}

	cfg := types.HistoryConfigData{Pattern: pattern, Enabled: true}
	cfgPath := "system/history/config/" + name
	if _, err := sh.Local.Peer.Store().Put(cfgPath, "system/history/config", cfg); err != nil {
		return Result{}, fmt.Errorf("history config: %w", err)
	}
	return MessageResult(fmt.Sprintf("recording enabled for %q (config %s)", pattern, name)), nil
}

// historyPath accepts the path forms an operator already types
// everywhere else and hands the handler one it can key on.
//
// A BARE relative path (`local/files/received/f.txt`) is passed through
// untouched, because the store canonicalizes it against the local peer
// and that works from any working directory. An `@alias/…` path is
// RESOLVED, because the handler keys on `/{peer-id}/…` and has no idea
// what an alias is — before this it matched nothing and the caller was
// told "(no transitions recorded — is a config installed?)", which is a
// confidently wrong diagnosis pointing at the one thing that was fine.
//
// Not `sh.Resolve` unconditionally: at the REPL root the working
// directory is `/`, so resolving a bare path yields `/local/files/…`
// with no peer in it, which matches nothing either. The two forms are
// handled differently because they genuinely are different, and the
// silent-empty result is the same shape either way.
func historyPath(sh *Shell, raw string) string {
	if strings.HasPrefix(raw, "@") {
		return sh.Resolve(raw).String()
	}
	return raw
}

// cmdHistoryQuery walks the recorded transition chain at path.
func cmdHistoryQuery(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: history query <path> [-limit N]")
	}
	path := historyPath(sh, args[0])
	params := types.HistoryQueryParamsData{Path: path}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-limit":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-limit requires a value")
			}
			n, err := strconv.ParseUint(args[i+1], 10, 64)
			if err != nil {
				return Result{}, fmt.Errorf("-limit: %w", err)
			}
			params.Limit = &n
			i++
		default:
			return Result{}, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	res, err := sh.Local.Peer.History().Query(context.Background(), params)
	if err != nil {
		return Result{}, fmt.Errorf("history query: %w", err)
	}
	if len(res.Transitions) == 0 {
		return MessageResult(fmt.Sprintf(
			"(no transitions recorded for %s — is a config installed? "+
				"`history config <pattern>` enables recording; a shared folder's "+
				"mount prefix is configured for you when it receives)", path)), nil
	}
	lines := make([]string, 0, len(res.Transitions)+1)
	for i, td := range res.Transitions {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		ts := time.UnixMilli(int64(td.Timestamp)).Format("2006-01-02 15:04:05")
		// WHO wrote it, not only when. On a shared folder this is the
		// difference between "you changed this" and "their copy arrived
		// and replaced yours": a local edit is ingested by the WATCHER
		// (`local/files:watch`), a delivered one is dispatched by
		// blob-resolve (`local/files:write`). The recorder has carried
		// both fields since it was written and this renderer dropped
		// them, so the one surface an operator has for recovering an
		// overwritten edit could not say which position was theirs.
		src := td.Handler
		if td.Operation != "" {
			src += ":" + td.Operation
		}
		lines = append(lines, fmt.Sprintf("%s %s  %-9s %s  %s",
			marker, ts, td.Event, shortHash(td.Hash), src))
	}
	if res.HasMore {
		lines = append(lines, "(more — pass -limit to extend)")
	}
	return LinesResult(lines), nil
}

// cmdHistoryRollback rebinds path to a target hash from its recorded
// history. The rollback itself is a recorded transition (event type
// "rollback") so the chain remains complete.
func cmdHistoryRollback(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: history rollback <path> <target-hash>")
	}
	target, err := parseHashHex(args[1])
	if err != nil {
		return Result{}, fmt.Errorf("target-hash: %w", err)
	}
	res, err := sh.Local.Peer.History().Rollback(context.Background(), historyPath(sh, args[0]), target)
	if err != nil {
		return Result{}, fmt.Errorf("history rollback: %w", err)
	}
	return MessageResult(fmt.Sprintf("rolled back %s → %s", res.Path, shortHash(res.Restored))), nil
}
