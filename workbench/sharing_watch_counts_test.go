package workbench

// sharing_watch_counts_test.go — the gate for the defect an operator hit
// on 2026-09-04: two sharing panels showing different file counts, both
// labelled "on disk", and a Refresh button as the only way to move either.
//
// The cause was not the counts and not the ingest chain — both are
// correct, and a probe that mounted seven files saw all seven reach the
// source layer and all seven become documents. The cause is that NOTHING
// WOKE THE PANEL. `DeclarationPrefixes` covers `app/workbench/*` and
// `app/share/*`; the numbers on screen are counts over `local/files/` and
// a mount's target prefix, which are in neither set. So the rows updated
// themselves, the numbers beside them froze at whatever was true when the
// panel opened, and two panels opened at different moments disagreed.
//
// The assertion below is on the PREFIX SET rather than on a live wake,
// deliberately. A wake test needs a store with a watch hub and a mount,
// and it would pass for the wrong reason the moment someone adds a
// timer-driven refresh. What must hold is that the layers a surface
// counts are in the set it subscribes to — and that is checkable exactly,
// against the same accessors the production path uses.

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
)

// TestSharingWatchPrefixes_CoverTheLayersTheSurfacesCount is the
// regression gate. FilesPresent counts LocalFilesSourcePrefix and
// FilesIngested counts a mount binding's TargetPrefix; both must be
// watched or the numbers go stale with nothing saying so.
func TestSharingWatchPrefixes_CoverTheLayersTheSurfacesCount(t *testing.T) {
	st := NewStore(store.NewMemoryContentStore(), store.NewMemoryLocationIndex())
	if err := SaveMountBinding(st, MountBindingData{
		Root:         "photos",
		SourcePrefix: "local/files/photos/",
		TargetPrefix: "archives/photos/",
	}); err != nil {
		t.Fatalf("SaveMountBinding: %v", err)
	}

	got := SharingWatchPrefixes(st)
	has := func(p string) bool {
		for _, g := range got {
			if g == p {
				return true
			}
		}
		return false
	}

	// The source layer, which FilesPresent counts.
	if !has(LocalFilesSourcePrefix) {
		t.Errorf("watch set omits %q — FilesPresent counts it and would freeze\ngot: %v",
			LocalFilesSourcePrefix, got)
	}
	// The target layer, which FilesIngested counts. Operator-chosen, so
	// it can only come from the store.
	if !has("archives/photos/") {
		t.Errorf("watch set omits the declared mount target %q — FilesIngested counts it\ngot: %v",
			"archives/photos/", got)
	}
	// Creating or removing a mount must itself be a wake, because it
	// changes which target prefixes exist.
	if !has(MountConfigPrefix) || !has(MountBindingPrefix) {
		t.Errorf("watch set omits a mount namespace, so a new mount is invisible\ngot: %v", got)
	}
	// And the declarations are still covered — this is a superset, not a
	// replacement.
	for _, d := range DeclarationPrefixes() {
		if !has(d) {
			t.Errorf("watch set dropped declaration prefix %q\ngot: %v", d, got)
		}
	}
}

// TestObservedPrefixes_SurvivesAStoreWithNoMounts — a peer with nothing
// mounted still watches the fixed layers, so the FIRST mount wakes the
// panel that was already open. Without this the very first share of a
// session renders stale, which is the only case that matters on day one.
func TestObservedPrefixes_SurvivesAStoreWithNoMounts(t *testing.T) {
	st := NewStore(store.NewMemoryContentStore(), store.NewMemoryLocationIndex())
	got := ObservedPrefixes(st)
	if len(got) < 3 {
		t.Fatalf("an empty peer watches %v — the first mount would not wake an open panel", got)
	}
	if ObservedPrefixes(nil) == nil {
		t.Errorf("ObservedPrefixes(nil) returned nil; a bridge with no store must still get the fixed set")
	}
}
