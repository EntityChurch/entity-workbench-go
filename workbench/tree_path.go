package workbench

import "strings"

// tree_path.go — the one place that turns a path the store HANDED BACK
// into a path you can do prefix arithmetic on.
//
// # The bug this exists to stop, which had already shipped
//
// `Store.List(prefix)` accepts a tree-relative prefix and returns
// **peer-qualified** paths. The prefix goes through
// `NamespacedIndex.canonicalize` on the way in; the entries come back out
// of the inner index untouched, carrying the `/{peer-id}/` they are
// stored under. So this, the obvious thing, is wrong:
//
//	for _, e := range st.List("system/config/local/files/") {
//	    root := strings.TrimPrefix(e.Path, "system/config/local/files/")   // WRONG
//	}
//
// TrimPrefix finds nothing to trim, `root` is the whole qualified path,
// and every downstream test on it — `== ""`, `strings.Contains(root,
// "/")`, a map key, a re-derived child path — silently gets the wrong
// answer.
//
// **This was live in shipped code.** `LocalFilesModel.Render` filtered
// every row out on `strings.Contains(root, "/")`, so the Local Files
// panel reported **"no filesystem mounts on this peer"** for a peer that
// had just mounted one — the operator's mount worked, wrote its config,
// started its watcher, and vanished from the only surface built to show
// it. The shell's `mounts` verb had the same defect one shade milder: it
// printed the qualified path as the root NAME, which is not a name you
// can pass to `unmount`.
//
// # Why every existing test missed it
//
// The cheap test scaffolding — `NewStore(NewMemoryContentStore(),
// NewMemoryLocationIndex())` — has **no NamespacedIndex**, so `List`
// returns bare relative paths and the arithmetic works. Every model test
// in this package uses that store. The namespacing only appears on a
// store from `CreatePeer`, which is what every real peer has and what no
// unit test here had.
//
// That is AP4's shape exactly: the pipeline under test was not the
// pipeline that runs. The lesson worth more than the fix — **a fixture
// that omits a wrapper the production object always has cannot fail on
// anything the wrapper changes**, and a namespacing index changes the
// *return value shape* of the most-used read in the codebase.
//
// # The rule
//
// Any path that came OUT of the store — a `LocationEntry.Path`, a
// `ChangeEvent.Path` — goes through [TreeRelative] before it is compared
// against, or trimmed by, a relative prefix. Paths you constructed
// yourself are already relative and pass through unchanged, so applying
// it is never wrong.

// TreeRelative drops a leading `/{peer-id}/` segment, returning a
// tree-relative path suitable for prefix arithmetic.
//
// Idempotent and safe on any input: a path that is already relative (no
// leading "/") is returned unchanged, so a caller does not have to know
// which form it holds. That property is the point — the alternative is
// every call site knowing whether its path came from a List, an event,
// or a local constant, which is the knowledge that was wrong five times.
//
// A single-segment absolute path ("/foo") has no second slash and is
// returned unchanged rather than emptied: it is not a namespaced path and
// guessing that it is would turn a real path into "".
func TreeRelative(path string) string {
	if !strings.HasPrefix(path, "/") {
		return path
	}
	rest := path[1:]
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return path
	}
	return rest[i+1:]
}

// RelativeUnder returns the portion of a store-returned path that sits
// under prefix, and whether it is under it at all.
//
// The two-step that every caller was open-coding — normalize, then trim —
// with the "is it actually under this prefix" check that several of them
// skipped. A bare TrimPrefix returns the whole string when the prefix
// does not match, which reads as a valid relative path and is not one.
func RelativeUnder(path, prefix string) (string, bool) {
	rel := TreeRelative(path)
	if prefix == "" {
		return rel, true
	}
	if !strings.HasPrefix(rel, prefix) {
		return "", false
	}
	return strings.TrimPrefix(rel, prefix), true
}
