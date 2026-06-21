package shellcmd

import (
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// cmdQuery implements `query <prefix> [-type T] [-field F=V] [-limit N]`
// — Tier E (QUERY-derived) per GUIDE-SHELL-FRAMING.md. Runs system/query
// find against the local peer and returns matching paths + types as a
// LinesResult.
//
// Companion to `find` (substring path search) and `count` (cardinality
// only). `find` filters client-side; `query` pushes the predicate
// server-side via system/query.
func cmdQuery(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: query <prefix> [-type T] [-field F=V] [-limit N]")
	}
	expr, err := parseQueryArgs(args)
	if err != nil {
		return Result{}, err
	}

	pc := sh.Local
	if pc == nil {
		return Result{}, fmt.Errorf("query: no local peer")
	}

	lines, total, hasMore, err := Query(pc.Peer, expr)
	if err != nil {
		return Result{}, err
	}
	if len(lines) == 0 {
		return MessageResult(fmt.Sprintf("(no matches under %s)", expr.PathPrefix)), nil
	}
	hdr := fmt.Sprintf("# %d match(es)", total)
	if hasMore {
		hdr += " (more available — pass -limit higher)"
	}
	return LinesResult(append([]string{hdr, ""}, lines...)), nil
}

// cmdCount implements `count <prefix> [-type T] [-field F=V]` — Tier E
// (QUERY-derived). Runs system/query count and returns the cardinality
// as a single message.
func cmdCount(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: count <prefix> [-type T] [-field F=V]")
	}
	expr, err := parseQueryArgs(args)
	if err != nil {
		return Result{}, err
	}

	pc := sh.Local
	if pc == nil {
		return Result{}, fmt.Errorf("count: no local peer")
	}

	n, err := Count(pc.Peer, expr)
	if err != nil {
		return Result{}, err
	}
	return MessageResult(fmt.Sprintf("%d", n)), nil
}

// parseQueryArgs parses the shared query/count argument shape:
// positional prefix + optional -type / -field / -limit flags. -field
// uses F=V syntax with `=` as the eq predicate (other operators TBD
// when cross-impl convention emerges).
func parseQueryArgs(args []string) (types.QueryExpressionData, error) {
	expr := types.QueryExpressionData{PathPrefix: args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-type":
			if i+1 >= len(args) {
				return expr, fmt.Errorf("query: -type requires a value")
			}
			expr.TypeFilter = args[i+1]
			i++
		case "-field":
			if i+1 >= len(args) {
				return expr, fmt.Errorf("query: -field requires F=V")
			}
			pred, err := parseFieldPredicate(args[i+1])
			if err != nil {
				return expr, err
			}
			expr.FieldFilters = append(expr.FieldFilters, pred)
			i++
		case "-limit":
			if i+1 >= len(args) {
				return expr, fmt.Errorf("query: -limit requires a value")
			}
			var limit uint64
			if _, err := fmt.Sscanf(args[i+1], "%d", &limit); err != nil || limit == 0 {
				return expr, fmt.Errorf("query: -limit must be a positive integer, got %q", args[i+1])
			}
			expr.Limit = &limit
			i++
		default:
			return expr, fmt.Errorf("query: unknown flag %q", args[i])
		}
	}
	return expr, nil
}

func parseFieldPredicate(s string) (types.QueryFieldPredicateData, error) {
	idx := strings.IndexByte(s, '=')
	if idx <= 0 || idx == len(s)-1 {
		return types.QueryFieldPredicateData{}, fmt.Errorf("query: -field expects F=V form, got %q", s)
	}
	field := s[:idx]
	valueStr := s[idx+1:]
	raw, err := cbor.Marshal(valueStr)
	if err != nil {
		return types.QueryFieldPredicateData{}, fmt.Errorf("query: encode -field value %q: %w", valueStr, err)
	}
	return types.QueryFieldPredicateData{
		Field:    field,
		Operator: "eq",
		Value:    raw,
	}, nil
}

// Query is the exported verb-op (GUIDE-SHELL-FRAMING.md §8.1). Runs the
// expression against the local peer's system/query handler and returns
// matches formatted as lines, the reported total, and whether more
// pages are available.
//
// Pure callable: takes a peer + expr, returns shaped output. Reusable
// from panels, palette forms, and library callers.
func Query(ap *entitysdk.AppPeer, expr types.QueryExpressionData) (lines []string, total uint64, hasMore bool, err error) {
	res, err := ap.Executor().Query(expr)
	if err != nil {
		return nil, 0, false, fmt.Errorf("query: %w", err)
	}
	lines = make([]string, 0, len(res.Matches))
	for _, m := range res.Matches {
		lines = append(lines, fmt.Sprintf("%-50s  %s", m.Path, m.Type))
	}
	return lines, res.Total, res.HasMore, nil
}

// Count is the exported verb-op companion to Query — same expression
// shape, returns just the cardinality.
func Count(ap *entitysdk.AppPeer, expr types.QueryExpressionData) (uint64, error) {
	n, err := ap.Executor().QueryCount(expr)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return n, nil
}
