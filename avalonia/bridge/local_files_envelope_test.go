package main

import (
	"encoding/json"
	"testing"
)

// local_files_envelope_test.go — the mount-row field names, gated in the one
// place that can see what Go actually emits.
//
// Same argument as `feed_envelope_test.go`: the question *"does the Go side
// emit the key the C# side reads"* needs no peer, no network and no fixture.
// `LocalFilesPanelTests` drives seeded JSON, so it establishes that the C#
// DTO can decode a shape **the C# test author wrote** — which is precisely the
// population that cannot contain a field Go renamed.
//
// This matters most for the watcher fields, because their failure is silent
// and reassuring: `watcherStatus` dropped by `System.Text.Json` leaves the
// empty string, the panel's `== "error"` branch never fires, and a mount whose
// watcher has died renders exactly like a healthy one (AP49 — a dropped field
// renders as "everything is fine", the worst available failure for a surface
// whose only job is to say otherwise).
//
// ⚠ Still not established here: that the C# DTO declares a matching
// `JsonPropertyName`. A rename on the C# side alone is invisible to this file.
// The two halves are gated in two languages and neither sees the other's,
// which is AP49's own shape — hence literals below, never values derived from
// the struct tags, because a derived expectation agrees with any rename.

// The keys `LocalFilesPanel.MountRow` reads. Spelled out, never reflected.
var mountRowKeys = []string{
	"root",
	"filesystemRoot",
	"prefix",
	"readOnly",
	"include",
	"exclude",
	"publishDescriptors",
	"configPath",
	"fileCount",
	"watcherObservable",
	"watcherStatus",
	"watcherError",
	"err",
}

func TestLocalFilesEnvelope_DoesNotDropTheFieldsThePanelReads(t *testing.T) {
	raw, err := json.Marshal(localFilesMountDTO{
		Root:              "notes",
		WatcherObservable: true,
		WatcherStatus:     "error",
		WatcherError:      "inotify watch limit reached",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, k := range mountRowKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("mount row envelope is missing %q — the panel's DTO declares it and "+
				"System.Text.Json will decode the absence to a zero value in silence", k)
		}
	}
}

// ⭐ The arm that matters: an `error` status must survive as an error, with its
// message. Without this the test above is satisfied by a field that is present
// and always empty.
func TestLocalFilesEnvelope_CarriesTheWatcherFaultRatherThanItsShape(t *testing.T) {
	raw, err := json.Marshal(localFilesMountDTO{
		Root:              "notes",
		WatcherObservable: true,
		WatcherStatus:     "error",
		WatcherError:      "inotify watch limit reached",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got struct {
		WatcherObservable bool   `json:"watcherObservable"`
		WatcherStatus     string `json:"watcherStatus"`
		WatcherError      string `json:"watcherError"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.WatcherObservable || got.WatcherStatus != "error" {
		t.Fatalf("watcher fault did not survive the envelope: observable=%v status=%q",
			got.WatcherObservable, got.WatcherStatus)
	}
	if got.WatcherError == "" {
		t.Error("watcherError is empty: an error status with no message tells an operator " +
			"a watcher failed and nothing about what to do")
	}
}

// And the third state stays distinguishable across the wire: no record is not
// `stopped`, so an absent record must not arrive as a status value.
func TestLocalFilesEnvelope_AbsentWatchRecordIsNotAStatus(t *testing.T) {
	raw, err := json.Marshal(localFilesMountDTO{Root: "notes"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		WatcherObservable bool   `json:"watcherObservable"`
		WatcherStatus     string `json:"watcherStatus"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WatcherObservable || got.WatcherStatus != "" {
		t.Errorf("an absent watch record crossed as observable=%v status=%q; the panel "+
			"renders three states and this one is \"no record\"", got.WatcherObservable, got.WatcherStatus)
	}
}
