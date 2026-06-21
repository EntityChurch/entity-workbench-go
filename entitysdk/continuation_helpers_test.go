package entitysdk_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

func TestInboxPath_Convention(t *testing.T) {
	cases := []struct {
		purpose, instance, step string
		want                    string
	}{
		{"follow", "peer-abc/docs", "fetch", "system/inbox/follow/peer-abc/docs/fetch"},
		{"sync", "", "extract", "system/inbox/sync/extract"},
		{"", "", "", "system/inbox"},
		{"ingest", "kb-2026", "", "system/inbox/ingest/kb-2026"},
	}
	for _, tc := range cases {
		got := entitysdk.InboxPath(tc.purpose, tc.instance, tc.step)
		if got != tc.want {
			t.Errorf("InboxPath(%q,%q,%q) = %q, want %q",
				tc.purpose, tc.instance, tc.step, got, tc.want)
		}
	}
}

func TestValidateContinuation_StructuralChecks(t *testing.T) {
	dummyHash, _ := hash.Compute("test/dummy", cbor.RawMessage{0xa0})
	params, _ := cbor.Marshal(map[string]string{"k": "v"})

	cases := []struct {
		name    string
		cont    types.ContinuationData
		wantErr string // substring of message; empty = expect nil
	}{
		{
			"valid trigger continuation",
			types.ContinuationData{
				Target:             "system/tree",
				Operation:          "get",
				DispatchCapability: dummyHash,
			},
			"",
		},
		{
			"missing target",
			types.ContinuationData{
				Operation:          "get",
				DispatchCapability: dummyHash,
			},
			"target is empty",
		},
		{
			"missing operation",
			types.ContinuationData{
				Target:             "system/tree",
				DispatchCapability: dummyHash,
			},
			"operation is empty",
		},
		{
			"missing dispatch_capability",
			types.ContinuationData{
				Target:    "system/tree",
				Operation: "get",
			},
			"dispatch_capability",
		},
		{
			"result_field without params",
			types.ContinuationData{
				Target:             "system/tree",
				Operation:          "get",
				DispatchCapability: dummyHash,
				ResultField:        "slot",
			},
			"result_field specified without params",
		},
		{
			"result_field with params ok",
			types.ContinuationData{
				Target:             "system/tree",
				Operation:          "get",
				DispatchCapability: dummyHash,
				ResultField:        "slot",
				Params:             cbor.RawMessage(params),
			},
			"",
		},
		{
			"deliver_to with empty URI",
			types.ContinuationData{
				Target:             "system/tree",
				Operation:          "get",
				DispatchCapability: dummyHash,
				DeliverTo:          &types.DeliverySpec{URI: "", Operation: "receive"},
			},
			"deliver_to set but URI is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := entitysdk.ValidateContinuation(tc.cont)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateContinuationJoin_RequiresExpected(t *testing.T) {
	dummyHash, _ := hash.Compute("test/dummy", cbor.RawMessage{0xa0})
	err := entitysdk.ValidateContinuationJoin(types.ContinuationJoinData{
		Target:             "system/tree",
		Operation:          "merge",
		DispatchCapability: dummyHash,
	})
	if err == nil {
		t.Fatal("expected error for join with no expected slots")
	}
	if !strings.Contains(err.Error(), "expected slot") {
		t.Errorf("error %q does not mention expected slots", err.Error())
	}
}
