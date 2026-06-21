package shellcmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// cmdFind implements `find <prefix> <substring>` — case-insensitive
// substring path search across entities under prefix.
//
// Built on top of system/query: PathPrefix narrows server-side,
// then we filter matches client-side. The query handler doesn't
// support substring search natively; rather than wait for a core-go
// extension we just do the post-filter ourselves. Idiomatic Go in
// the shell beats waiting for spec coordination — and the workbench
// can iterate faster than the core team.
func cmdFind(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: find <prefix> <substring> [-limit N]")
	}
	prefix, needle := args[0], args[1]
	limit := uint64(0) // 0 = unlimited (client-side cap)
	for i := 2; i < len(args); i++ {
		if args[i] == "-limit" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &limit)
			i++
		}
	}

	pc := sh.Local
	// find/grep want the full prefix, not the default-paginated 100.
	// 100k is a soft ceiling; if a real prefix has more we want a
	// dedicated content-search extension at that point.
	bigLimit := uint64(100000)
	expr := types.QueryExpressionData{PathPrefix: prefix, Limit: &bigLimit}
	res, err := pc.Peer.Executor().Query(expr)
	if err != nil {
		return Result{}, fmt.Errorf("query: %w", err)
	}

	needleLower := strings.ToLower(needle)
	type hit struct {
		path string
		typ  string
	}
	var hits []hit
	for _, m := range res.Matches {
		if strings.Contains(strings.ToLower(m.Path), needleLower) {
			hits = append(hits, hit{m.Path, m.Type})
			if limit > 0 && uint64(len(hits)) >= limit {
				break
			}
		}
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].path < hits[j].path })

	if len(hits) == 0 {
		return MessageResult(fmt.Sprintf("(no paths under %s match %q)", prefix, needle)), nil
	}
	lines := make([]string, 0, len(hits)+1)
	lines = append(lines, fmt.Sprintf("%d match(es) of %d entities under %s:",
		len(hits), len(res.Matches), prefix))
	for _, h := range hits {
		lines = append(lines, fmt.Sprintf("  %s  [%s]", h.path, h.typ))
	}
	return LinesResult(lines), nil
}

// cmdGrep implements `grep <prefix> <regex>` — content search across
// `doc/markdown-file` entities (and any entity with a string `content`
// field) under prefix.
//
// Like find, this is shell-side: query for the prefix, fetch each
// entity, decode CBOR, regex-match the content field. For 1k entities
// at 42 MB this completes in well under a second; if it ever becomes
// a bottleneck, file a query-extension feature for native content
// search.
//
// Output: per match, the path + the matching line(s) with context.
func cmdGrep(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: grep <prefix> <regex> [-i] [-l] [-context N]")
	}
	prefix, pattern := args[0], args[1]

	caseInsensitive := false
	pathsOnly := false // -l: only print matching paths, no lines
	context := 0       // lines of context (currently 0 = just matching line)
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "-i":
			caseInsensitive = true
		case "-l":
			pathsOnly = true
		case "-context":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-context requires a value")
			}
			fmt.Sscanf(args[i+1], "%d", &context)
			i++
		default:
			return Result{}, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{}, fmt.Errorf("regex: %w", err)
	}

	pc := sh.Local
	bigLimit := uint64(100000)
	expr := types.QueryExpressionData{PathPrefix: prefix, Limit: &bigLimit}
	res, err := pc.Peer.Executor().Query(expr)
	if err != nil {
		return Result{}, fmt.Errorf("query: %w", err)
	}

	var lines []string
	matchedPaths := 0
	for _, m := range res.Matches {
		ent, ok := pc.Peer.Store().GetByHash(m.Hash)
		if !ok {
			continue
		}

		// Resolve the entity's textual content. doc/markdown-file
		// carries a blob hash ref (CONTENT v3.6 substrate); other types
		// with a string `content` field are handled inline. Entities
		// whose blob is missing locally (cross-peer partial sync) are
		// skipped rather than treated as content-empty.
		content := resolveSearchableContent(pc.Peer.Store(), ent)
		if content == "" {
			continue
		}

		matches := matchLines(re, content, context)
		if len(matches) == 0 {
			continue
		}
		matchedPaths++
		if pathsOnly {
			lines = append(lines, m.Path)
			continue
		}
		lines = append(lines, fmt.Sprintf("=== %s ===", m.Path))
		lines = append(lines, matches...)
		lines = append(lines, "")
	}

	if matchedPaths == 0 {
		return MessageResult(fmt.Sprintf(
			"(no content matches for /%s/ under %s; %d entities scanned)",
			pattern, prefix, len(res.Matches))), nil
	}
	header := fmt.Sprintf("matched %d/%d entities", matchedPaths, len(res.Matches))
	return LinesResult(append([]string{header, ""}, lines...)), nil
}

// extractContentField pulls a string content field out of CBOR-encoded
// entity data. Looks for a top-level "content" key holding a string.
// Returns "" if the entity has no string content (binary entities,
// non-content-bearing types, or blob-referencing entities like
// doc/markdown-file under the v2 hash-ref shape).
func extractContentField(data []byte) string {
	var decoded map[string]interface{}
	if err := ecf.Decode(data, &decoded); err != nil {
		return ""
	}
	if c, ok := decoded["content"].(string); ok {
		return c
	}
	return ""
}

// resolveSearchableContent returns the textual body of an entity for
// grep purposes. For doc/markdown-file the body lives in a blob in the
// content store; for legacy entities with a string `content` field
// (e.g. older hand-put types) the body is inline. Returns "" when the
// entity has no resolvable text (blob missing locally, binary entity,
// unsupported type).
func resolveSearchableContent(st *entitysdk.Store, ent entity.Entity) string {
	if ent.Type == workbench.MarkdownFileType {
		md, err := workbench.MarkdownFileDataFromEntity(ent)
		if err != nil {
			return ""
		}
		body, present, err := workbench.LoadMarkdownContent(st.ContentStore(), md)
		if err != nil || !present {
			return ""
		}
		return string(body)
	}
	return extractContentField(ent.Data)
}

// matchLines returns the matching lines from content, optionally with
// `context` lines of surrounding context per match. Output format
// mirrors grep -n: "  N: line text".
func matchLines(re *regexp.Regexp, content string, context int) []string {
	allLines := strings.Split(content, "\n")
	var out []string
	emitted := make(map[int]bool)

	for i, line := range allLines {
		if !re.MatchString(line) {
			continue
		}
		start := i - context
		if start < 0 {
			start = 0
		}
		end := i + context
		if end >= len(allLines) {
			end = len(allLines) - 1
		}
		for j := start; j <= end; j++ {
			if emitted[j] {
				continue
			}
			emitted[j] = true
			marker := "  "
			if j == i {
				marker = "> "
			}
			out = append(out, fmt.Sprintf("%s%4d: %s", marker, j+1, allLines[j]))
		}
	}
	return out
}
