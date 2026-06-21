package entitysdk_test

import (
	"context"
	"fmt"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// Test-local helpers for the incremental-sync probe.

// executeOnRemote dispatches an EXECUTE on the named remote peer's URI
// using the local peer's owner capability via the executor's
// URI-aware routing (entity://{peer-id}/handler).
func executeOnRemote(_ context.Context, ap *entitysdk.AppPeer, remotePeerID, handlerURI, op string, params entity.Entity) (entity.Entity, error) {
	remoteURI := fmt.Sprintf("entity://%s/%s", remotePeerID, handlerURI)
	resp, err := ap.Executor().ExecuteOnResource(remoteURI, op, params, nil)
	if err != nil {
		return entity.Entity{}, err
	}
	if resp == nil {
		return entity.Entity{}, fmt.Errorf("remote execute %s.%s returned no response", handlerURI, op)
	}
	if resp.Status >= 400 {
		return entity.Entity{}, fmt.Errorf("remote execute %s.%s returned status=%d", handlerURI, op, resp.Status)
	}
	return resp.Entity(), nil
}

// mustGet returns the entity at the named hash from the peer's content
// store; fatals if absent.
func mustGet(t *testing.T, ap *entitysdk.AppPeer, h hash.Hash) entity.Entity {
	t.Helper()
	ent, ok := ap.Store().GetByHash(h)
	if !ok {
		t.Fatalf("entity not in store: %s", h)
	}
	return ent
}

// decodeEntity unwraps an entity's Data into target.
func decodeEntity(ent entity.Entity, target interface{}) error {
	return ecf.Decode(ent.Data, target)
}

// countEnvelopeIncluded counts the entries in an envelope-wrapped result.
// The result entity's data is an entity.Envelope; its Included is the
// content bundle.
func countEnvelopeIncluded(t *testing.T, envEnt entity.Entity) int {
	t.Helper()
	if envEnt.Type != types.TypeEnvelope {
		t.Fatalf("expected %s, got %s", types.TypeEnvelope, envEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(envEnt.Data, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return len(env.Included)
}

// mapKeys returns the sorted keys of a string-keyed map (for diagnostics).
func mapKeys(m map[string]types.DiffChangeData) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
