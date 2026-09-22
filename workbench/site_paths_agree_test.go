package workbench

import (
	"testing"

	"entity-workbench-go/entitysdk"
)

// site_paths_agree_test.go — the check that was missing, at the only
// layer that can run it.
//
// `entitysdk` WRITES a site (`PutSiteManifest` / `PutSitePage`, and the
// path helpers `entity-seed-site` uses). `workbench` READS one (both
// resolvers, the demo seeder, the ref classifier). Until 2026-09-12 they
// named different placements — the SDK on the v0.4.2 `content/sites/`
// that SITE v0.5 §2 drops by name, this package on the correct `sites/`
// — so a site authored through the SDK was invisible to every surface in
// this repo that renders one.
//
// **Nothing failed, and the reason generalizes.** Each package's tests
// round-trip through its own constant, so both halves agreed with
// themselves; the remote resolver's only end-to-end exercise is
// `entity-browser-rust`'s frozen fixture, which uses the correct path
// and therefore proved the reader right while saying nothing about the
// writer. It took pointing our own writer at our own reader (W2) to see
// it — the same shape as the 2026-08-19 publish/fetch corridor, where
// two green halves were four ways apart.
//
// The literals below are spelled out on purpose. Both packages now share
// one `SitesSubpath`, so a test that COMPOSED the expected path from it
// could not fail on the segment however wrong the segment was — which is
// exactly the vacuity that hid this for four months. These come from the
// spec.

const (
	wantSitePrefix   = "/PEER1/sites/blog/"
	wantManifestPath = "/PEER1/sites/blog/manifest"
	wantPagesPrefix  = "/PEER1/sites/blog/pages/"
	wantPagePath     = "/PEER1/sites/blog/pages/docs/intro"
)

// TestSitePlacementIsOneConventionAcrossTheWriterAndTheReader asserts
// both packages against the spec's placement AND against each other.
//
// Both arms are needed. The literal arm catches a move away from v0.5;
// the agreement arm catches the two halves drifting apart again, which
// is a different failure and the one that is silent.
func TestSitePlacementIsOneConventionAcrossTheWriterAndTheReader(t *testing.T) {
	for _, c := range []struct {
		what   string
		writer string
		reader string
		want   string
	}{
		{"site prefix", entitysdk.SitePrefix("PEER1", "blog"), SitePrefix("PEER1", "blog"), wantSitePrefix},
		{"manifest", entitysdk.SiteManifestPath("PEER1", "blog"), ManifestPath("PEER1", "blog"), wantManifestPath},
		{"pages prefix", entitysdk.SitePagesPrefix("PEER1", "blog"), PagesPrefix("PEER1", "blog"), wantPagesPrefix},
		{"page", entitysdk.SitePagePath("PEER1", "blog", "docs/intro"), PagePath("PEER1", "blog", "docs/intro"), wantPagePath},
	} {
		if c.writer != c.want {
			t.Errorf("%s: entitysdk (the WRITER) says %q, SITE v0.5 §2 says %q — "+
				"a site authored through the SDK lands where nothing in this repo reads it",
				c.what, c.writer, c.want)
		}
		if c.reader != c.want {
			t.Errorf("%s: workbench (the READER) says %q, SITE v0.5 §2 says %q — "+
				"every resolver in this package is looking in the wrong place",
				c.what, c.reader, c.want)
		}
		if c.writer != c.reader {
			t.Errorf("%s: the writer and the reader disagree\n  entitysdk:  %q\n  workbench:  %q\n"+
				"this is silent in both suites: each round-trips through its own constant",
				c.what, c.writer, c.reader)
		}
	}
}
