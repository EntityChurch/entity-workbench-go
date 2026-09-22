package main

import (
	"encoding/json"
	"testing"

	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// feed_envelope_test.go — the ENTRY-level field names, gated where they can
// actually be reached.
//
// # Why this file exists rather than an assertion in the C# suite
//
// `FeedPanelTests.The_Feed_Envelopes_Do_Not_Drop_The_Fields_The_Panel_Reads`
// covers the envelope's top level and says, in its own comment, what it cannot
// cover: **per-ENTRY fields**. Reaching one needs a populated timeline, which
// needs a reachable publisher that has posted and published — and no suite in
// this repo may touch the public internet, while writing a follow onto the
// shared `BridgeFixture.DefaultPeer` writes real state every other test in the
// assembly then renders (AP70). `via` and `listed` were named there as a hole
// left open on exactly that ground.
//
// **The hole was never about the fixture, it was about the layer.** The
// question is *"does the Go side emit the key the C# side reads"*, and that is
// answerable with no peer, no network and no fixture at all: build the model
// struct, run it through the same projection the export runs, and read the
// JSON. This does that, and it covers `via` and `listed` as well as the body
// fields — so the gap that file names is closed rather than extended.
//
// ⚠ **What it still does not establish:** that the C# DTO declares a matching
// `JsonPropertyName`. A rename on the C# side alone is invisible here. The two
// halves are gated in two languages and neither can see the other's — which is
// the shape of AP49 itself and is why the names below are written as
// **literals** rather than derived from the struct tags. A literal is the only
// form that disagrees with a rename.

// The keys the Avalonia panel reads off a timeline entry. Spelled out, never
// reflected: a test that derives the expected name from the tag it is checking
// agrees with itself for any value of the tag.
var timelineEntryKeys = []string{
	"subject", "label", "hash", "page", "createdAtMillis",
	"text", "mediaType", "isReply",
	"bodyRung", "bodyNote", "isMarkdown",
	"listed", "attributed", "attribution", "rejected", "problem",
}

// TestTimelineEntryEnvelope_CarriesEveryFieldThePanelReads.
//
// The three body fields are the ones this gate was written for, and their
// failure mode is the confident direction: drop `bodyRung` and every entry
// renders as a plain paragraph at a rung nobody can see, so an image post
// shows its alt text as though it were the post. Drop `isMarkdown` and a
// markdown post shows its source — or, worse, a `text/plain` post gets parsed
// and the author's asterisks vanish.
func TestTimelineEntryEnvelope_CarriesEveryFieldThePanelReads(t *testing.T) {
	out := shellcmd.TimelineOutcome{
		Merged: []shellcmd.TimelineEntry{{
			Subject: "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS",
			Label:   "alice",
			Entry: wb.FeedEntryRead{
				Text:       "a photo of a cat",
				MediaType:  "image/png",
				BodyRung:   wb.FeedBodyFallback,
				BodyNote:   "showing the author's fallback text",
				IsMarkdown: true,
				Listed:     true,
			},
		}},
	}

	raw, err := json.Marshal(timelineToDTO(out))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var env struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Entries) != 1 {
		t.Fatalf("the projection emitted %d entries, want 1 — this gate is measuring nothing",
			len(env.Entries))
	}

	for _, k := range timelineEntryKeys {
		if _, ok := env.Entries[0][k]; !ok {
			t.Errorf("`%s` is not in the entry envelope. System.Text.Json drops an undeclared "+
				"field in total silence (AP49), so the panel reads the zero value and renders it "+
				"as a fact", k)
		}
	}
}

// TestTimelineEntryEnvelope_TheBodyRungSurvivesAsAValueAndNotJustAKey.
//
// The key being present is not the assertion that matters. `FeedBodyRung` is a
// named string type, and a projection that dropped the conversion — or reached
// for the wrong field — emits a present, well-formed, empty `bodyRung`, which
// the renderer reads as "not a fallback" and draws as the post.
//
// **An empty rung is the dangerous value, not a missing key**, because a
// missing key is at least the same failure for every entry while an empty one
// is correct-looking for the entries that happen to be rendered at step 1.
func TestTimelineEntryEnvelope_TheBodyRungSurvivesAsAValueAndNotJustAKey(t *testing.T) {
	for _, rung := range []wb.FeedBodyRung{
		wb.FeedBodyRendered, wb.FeedBodyFallback, wb.FeedBodyUnrenderable,
	} {
		out := shellcmd.TimelineOutcome{
			Merged: []shellcmd.TimelineEntry{{
				Entry: wb.FeedEntryRead{Text: "x", BodyRung: rung},
			}},
		}
		raw, err := json.Marshal(timelineToDTO(out))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var env struct {
			Entries []struct {
				BodyRung string `json:"bodyRung"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(env.Entries) != 1 || env.Entries[0].BodyRung != string(rung) {
			t.Errorf("rung %q did not survive the projection (got %q)", rung, env.Entries[0].BodyRung)
		}
	}
}
