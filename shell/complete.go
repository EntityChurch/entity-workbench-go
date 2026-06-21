package shell

import (
	"sort"
	"strings"

	"entity-workbench-go/shellcmd"
)

// completer returns tab-completion candidates for an input line. It
// understands two cases:
//
//   - the user is still typing the command verb (no whitespace yet) →
//     candidates are command names from the registry.
//   - the user is typing a path argument → candidates are children of
//     the directory the path points into, fetched live via
//     AppPeer.List (which handles local and remote routing).
//
// Returned candidates are full replacement strings for the entire
// input line (that's what liner expects). Trailing "/" is added for
// directory-shaped entries so the user can keep tabbing.
//
// Path completion fetches one directory listing per TAB. For local
// peers that's an in-memory walk; for remote peers it's a single
// dispatched listing. No background tree-building; everything is
// incremental.
func (a *App) completer(input string) []string {
	verb, argStart, hasArg := splitVerb(input)
	if !hasArg {
		return a.completeVerb(input)
	}
	linePrefix := input[:argStart]
	token := input[argStart:]
	cands := a.completePath(token)
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, linePrefix+c)
	}
	// Stable order so repeated tabs don't shuffle.
	sort.Strings(out)
	_ = verb
	return out
}

// splitVerb finds the boundary between the command verb and its
// arguments. Returns (verb, argStartIndex, hasArg). If the input has
// no whitespace yet the user is still typing the verb itself.
func splitVerb(input string) (verb string, argStart int, hasArg bool) {
	idx := strings.IndexAny(input, " \t")
	if idx < 0 {
		return input, 0, false
	}
	verb = input[:idx]
	// Skip past run of spaces.
	argStart = idx
	for argStart < len(input) && (input[argStart] == ' ' || input[argStart] == '\t') {
		argStart++
	}
	return verb, argStart, true
}

func (a *App) completeVerb(token string) []string {
	var out []string
	for _, c := range a.reg.Commands() {
		if strings.HasPrefix(c.Name, token) {
			out = append(out, c.Name)
		}
	}
	sort.Strings(out)
	return out
}

// completePath returns candidates for the path token the user is
// typing. The candidates include the dir-portion of the token so they
// can be substituted whole (liner replaces the entire line, not just
// the suffix).
func (a *App) completePath(token string) []string {
	dirToken, leaf := splitPathToken(token)

	// User is typing an "@alias" prefix (no "/" yet) — complete the
	// alias name regardless of WD. Per GUIDE-SHELL-FRAMING.md §3.4
	// the `@alias` sigil is the canonical alias-substitution form.
	if dirToken == "" && strings.HasPrefix(token, "@") {
		return atAliasCandidates(a, token[1:])
	}

	// At-root-or-empty: candidates are alias prefixes of connected
	// peers. Mirrors the behavior of `ls /`.
	if dirToken == "" && a.sh.WD.IsRoot() {
		return aliasCandidates(a, leaf)
	}

	// Resolve dirToken into a Path. An empty dirToken means "the
	// current working directory."
	var target shellcmd.Path
	if dirToken == "" {
		target = a.sh.WD
	} else {
		target = a.sh.Resolve(dirToken)
	}

	// If we resolved to the shell root (e.g., user typed `/`),
	// suggest connected peers as `/alias/`.
	if target.IsRoot() {
		return rootSlashCandidates(a, leaf)
	}

	pc := a.sh.ConnForPath(target)
	if pc == nil {
		return nil
	}

	listPath := target.String()
	if !strings.HasSuffix(listPath, "/") {
		listPath += "/"
	}
	entries, err := pc.Peer.List(listPath)
	if err != nil {
		return nil
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, leaf) {
			continue
		}
		cand := dirToken + e.Name
		if e.HasChildren {
			cand += "/"
		}
		out = append(out, cand)
	}
	return out
}

// splitPathToken divides a partial path token into (dirToken, leaf):
// dirToken is everything up to and including the last separator
// (slash, or the colon in legacy `alias:`); leaf is what comes after,
// the part being filtered.
//
// Examples:
//
//	"system/han"    → ("system/",      "han")
//	"@local/sys"    → ("@local/",      "sys")
//	"local:sys"     → ("local:",       "sys")   (legacy form)
//	"/local/sys"    → ("/local/",      "sys")
//	"sys"           → ("",             "sys")
//	""              → ("",             "")
//
// Note: tokens starting with "@" but without a "/" yet (e.g.,
// "@lo") fall through to ("", "@lo") so the caller can route them
// to atAliasCandidates.
func splitPathToken(token string) (dirToken, leaf string) {
	if idx := strings.LastIndex(token, "/"); idx >= 0 {
		return token[:idx+1], token[idx+1:]
	}
	if idx := strings.IndexByte(token, ':'); idx > 0 {
		return token[:idx+1], token[idx+1:]
	}
	return "", token
}

// atAliasCandidates returns "@alias/" candidates filtered by the
// leaf (the partial alias name without the "@" prefix). This is the
// canonical form per GUIDE-SHELL-FRAMING.md §3.4 — the trailing "/"
// lets the user keep tabbing to extend into the peer's tree.
func atAliasCandidates(a *App, leaf string) []string {
	var out []string
	for alias := range a.sh.Conns {
		if strings.HasPrefix(alias, leaf) {
			out = append(out, "@"+alias+"/")
		}
	}
	sort.Strings(out)
	return out
}

// aliasCandidates returns root-level alias candidates in `@alias`
// form filtered by leaf. Emitted at the shell root when the user
// types `<TAB>` with no prefix; replaces the legacy `alias:` form
// per GUIDE-SHELL-FRAMING.md §3.4 (the parser still accepts
// `alias:` during the deprecation window, but completion prefers
// the canonical sigil form).
func aliasCandidates(a *App, leaf string) []string {
	var out []string
	for alias := range a.sh.Conns {
		if strings.HasPrefix(alias, leaf) {
			out = append(out, "@"+alias+"/")
		}
	}
	sort.Strings(out)
	return out
}

// rootSlashCandidates returns `/@alias/` candidates for completion
// against the canonical absolute form (e.g., `/<TAB>` → `/@local/`,
// `/@lo<TAB>` → `/@local/`). The leaf may include a leading "@"
// (when the user has typed `/@`); both `/loc` and `/@loc` complete
// to `/@local/`.
func rootSlashCandidates(a *App, leaf string) []string {
	// Strip optional leading "@" so the prefix filter matches the bare
	// alias name in either typing form.
	leaf = strings.TrimPrefix(leaf, "@")
	var out []string
	for alias := range a.sh.Conns {
		if strings.HasPrefix(alias, leaf) {
			out = append(out, "/@"+alias+"/")
		}
	}
	sort.Strings(out)
	return out
}
