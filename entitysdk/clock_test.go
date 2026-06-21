package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// TestClockClient_NowDefaultMode verifies a default-config peer
// answers system/clock:now with mode=wall and a non-zero timestamp.
func TestClockClient_NowDefaultMode(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	state, err := ap.Clock().Now(context.Background())
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if state.Mode != types.DefaultClockMode {
		t.Errorf("mode = %q, want %q", state.Mode, types.DefaultClockMode)
	}
	if state.Timestamp == nil || state.Timestamp.Ms == 0 {
		t.Errorf("default mode should include a non-zero wall timestamp; got %+v", state.Timestamp)
	}
}

// TestClockClient_CompareTimestamps verifies the compare op orders two
// timestamp values correctly. A < B → "before"; A > B → "after"; equal → "equal".
func TestClockClient_CompareTimestamps(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	encode := func(d types.ClockTimestampData) cbor.RawMessage {
		raw, err := ecf.Encode(d)
		if err != nil {
			t.Fatalf("encode timestamp: %v", err)
		}
		return raw
	}

	ctx := context.Background()
	cc := ap.Clock()

	for _, tt := range []struct {
		name string
		a, b uint64
		want string
	}{
		{"a-before-b", 100, 200, "before"},
		{"a-after-b", 200, 100, "after"},
		{"equal", 150, 150, "equal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			order, err := cc.Compare(ctx,
				encode(types.ClockTimestampData{Ms: tt.a}),
				encode(types.ClockTimestampData{Ms: tt.b}),
			)
			if err != nil {
				t.Fatalf("Compare: %v", err)
			}
			if order != tt.want {
				t.Errorf("order = %q, want %q", order, tt.want)
			}
		})
	}
}
