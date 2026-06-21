package shellcmd

import (
	"context"
	"fmt"
	"strconv"
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

// cmdHistoryQuery walks the recorded transition chain at path.
func cmdHistoryQuery(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: history query <path> [-limit N]")
	}
	params := types.HistoryQueryParamsData{Path: args[0]}
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
		return MessageResult(fmt.Sprintf("(no transitions recorded for %s — is a config installed?)", args[0])), nil
	}
	lines := make([]string, 0, len(res.Transitions)+1)
	for i, td := range res.Transitions {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		ts := time.UnixMilli(int64(td.Timestamp)).Format("2006-01-02 15:04:05")
		lines = append(lines, fmt.Sprintf("%s %s  %-9s %s", marker, ts, td.Event, shortHash(td.Hash)))
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
	res, err := sh.Local.Peer.History().Rollback(context.Background(), args[0], target)
	if err != nil {
		return Result{}, fmt.Errorf("history rollback: %w", err)
	}
	return MessageResult(fmt.Sprintf("rolled back %s → %s", res.Path, shortHash(res.Restored))), nil
}
