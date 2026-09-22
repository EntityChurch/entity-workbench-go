package workbench_test

// The two copies of the site types must encode one authored manifest to one
// content hash. Tier: cross-implementation conformance (TESTING-STRATEGY) —
// because the property it protects is `APP-CONVENTION-SEMANTIC-CONTENT-SITE`
// §9's byte-equality claim, and a claim we cannot keep with ourselves we
// certainly cannot keep with another implementation.
//
// WHY THIS EXISTS. `workbench` and `entitysdk` each declare `SiteManifest`,
// `SitePage` and a nav node. They are the same three shapes and nothing in the
// tree compared them. On 2026-09-16 they disagreed:
//
//	source (site_id + title, no nav)   25 B   ecf-sha256:42e061e3…
//	via workbench.SiteManifest          30 B   ecf-sha256:78650fa3…   `nav: []` ADDED
//	via entitysdk.SiteManifest          25 B   ecf-sha256:42e061e3…
//
// `workbench.SiteManifest.Nav` was tagged `cbor:"nav"` where SITE §4 declares
// `? nav` and calls title-only conformant. So an ordinary one-page site — no
// unknown field, nothing exotic — came out of this package at a different
// address than it went in, and `entity-browser-rust` measured the identical
// defect in their own tree the same morning (25 B -> 30 B, `nav:[]` added;
// their `25020d8f`). Two implementations, one mistake, independently.
//
// ⭐ WHY NOTHING CAUGHT IT, which is the transferable half.
// `fetch/site_entity_crossimpl_test.go` decodes another implementation's real
// site entities and re-encodes them byte-identically. It is green, it is a good
// gate, and it drives **entitysdk's** copy — while every site resolver in this
// package drives **workbench's**. The copy the product actually reads sites
// with was the unmeasured one. *A cross-impl fixture measures the half of a
// corridor that faces it* (AP96, recorded there about `SitesSubpath` and true
// here about the struct).
//
// HOW THIS GATE AVOIDS THE SAME TRAP. It does not compare the two copies to
// each other and stop — two copies that agree can both be wrong, which is the
// failure mode AP96 names. Each is asserted against **spelled-out literal
// bytes**, built from the CDDL by hand, so a change that moves both in step
// still fails.

import (
	"bytes"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// siteEntityFromMap builds the reference bytes the way the CDDL reads them: a
// map with exactly the declared keys present, and nothing else.
func siteEntityFromMap(t *testing.T, typeName string, data map[string]interface{}) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(data)
	if err != nil {
		t.Fatalf("encode reference %s: %v", typeName, err)
	}
	ent, err := entity.NewEntity(typeName, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("bind reference %s: %v", typeName, err)
	}
	return ent
}

// reencode runs the shape both copies are used in — decode obtained bytes into
// the declared type, re-encode, re-bind — and returns the resulting entity.
func reencode(t *testing.T, ent entity.Entity, into interface{}) entity.Entity {
	t.Helper()
	if err := ecf.Decode(ent.Data, into); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, err := ecf.Encode(into)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	out, err := entity.NewEntity(ent.Type, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("re-bind: %v", err)
	}
	return out
}

// TestSiteTypeCopies_AgreeByteForByte is the regression gate for the defect
// above, and the title-only arm is the one that fires.
func TestSiteTypeCopies_AgreeByteForByte(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		data map[string]interface{}
	}{
		{
			// ⭐ The arm the defect lived in. SITE §4: `? nav`, and the
			// convention states title-only is conformant. If `omitempty` is
			// ever dropped from either copy's nav, this is what goes red.
			name: "manifest, title-only — the conformant one-page site",
			typ:  workbench.SiteManifestType,
			data: map[string]interface{}{"site_id": "demo", "title": "Demo"},
		},
		{
			name: "manifest with a flat nav and params",
			typ:  workbench.SiteManifestType,
			data: map[string]interface{}{
				"site_id": "demo",
				"title":   "Demo",
				"nav": []interface{}{
					map[string]interface{}{"label": "Home", "target": "./index"},
				},
				"params": map[string]interface{}{"root": "index"},
			},
		},
		{
			// A section header: `? target` absent, per SITE's nav-node.
			name: "manifest with a section header carrying no target",
			typ:  workbench.SiteManifestType,
			data: map[string]interface{}{
				"site_id": "demo",
				"title":   "Demo",
				"nav": []interface{}{
					map[string]interface{}{
						"label": "Guides",
						"children": []interface{}{
							map[string]interface{}{"label": "Intro", "target": "./intro"},
						},
					},
				},
			},
		},
		{
			name: "page, body-only — no frontmatter key",
			typ:  workbench.SitePageType,
			data: map[string]interface{}{"format": "markdown", "body": "# Bare\n"},
		},
		{
			name: "page with frontmatter",
			typ:  workbench.SitePageType,
			data: map[string]interface{}{
				"format":      "markdown",
				"body":        "# Notes\n",
				"frontmatter": map[string]interface{}{"title": "Notes"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref := siteEntityFromMap(t, c.typ, c.data)

			var wb, sdk entity.Entity
			switch c.typ {
			case workbench.SiteManifestType:
				var w workbench.SiteManifest
				var s entitysdk.SiteManifest
				wb, sdk = reencode(t, ref, &w), reencode(t, ref, &s)
			case workbench.SitePageType:
				var w workbench.SitePage
				var s entitysdk.SitePage
				wb, sdk = reencode(t, ref, &w), reencode(t, ref, &s)
			default:
				t.Fatalf("unhandled type %q", c.typ)
			}

			// Against the LITERAL, each independently — not against each
			// other. Two copies that agree can both be wrong.
			if !bytes.Equal(ref.Data, wb.Data) {
				t.Errorf("workbench copy moved the bytes: %d B -> %d B\n  ref %s\n  got %s",
					len(ref.Data), len(wb.Data), ref.ContentHash.String(), wb.ContentHash.String())
			}
			if !bytes.Equal(ref.Data, sdk.Data) {
				t.Errorf("entitysdk copy moved the bytes: %d B -> %d B\n  ref %s\n  got %s",
					len(ref.Data), len(sdk.Data), ref.ContentHash.String(), sdk.ContentHash.String())
			}
			// Stated separately so the failure names the right thing when the
			// two disagree but one is still right.
			if wb.ContentHash.String() != sdk.ContentHash.String() {
				t.Errorf("the two copies of this type disagree with each other:\n  workbench %s\n  entitysdk %s",
					wb.ContentHash.String(), sdk.ContentHash.String())
			}
		})
	}
}

// TestSiteTypeCopies_TheGateCanFail is the anti-vacuity arm.
//
// Every assertion above is an equality, and a gate made only of equalities over
// input it constructed itself is one refactor away from comparing a value to
// itself. This arm proves the measurement can move: a field neither copy
// declares is dropped, the bytes shrink, and the address changes — which is
// also `SYSTEM-DATA-EXCHANGE` §2.1's hazard restated at the type these
// resolvers use (see fetch/dx_byte_preservation_test.go for that reading).
func TestSiteTypeCopies_TheGateCanFail(t *testing.T) {
	ref := siteEntityFromMap(t, workbench.SiteManifestType, map[string]interface{}{
		"site_id":    "demo",
		"title":      "Demo",
		"theme_hint": "dark", // declared by neither copy, and by no convention
	})

	var w workbench.SiteManifest
	got := reencode(t, ref, &w)

	if bytes.Equal(ref.Data, got.Data) {
		t.Fatal("an undeclared key survived the round trip — the equality assertions above " +
			"are therefore not measuring encoding fidelity, and this whole file is vacuous")
	}
	t.Logf("control: an undeclared key is dropped, %d B -> %d B", len(ref.Data), len(got.Data))
}
