package entitysdk_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestRevision_Status confirms system/revision:status is dispatchable
// on a default-config peer — i.e. revision wiring (root tracker +
// auto-versioner Load + handler registration) didn't break the
// dispatch path. A status call against a never-tracked prefix returns
// 200 with zero conflicts / zero pending; an empty prefix is a 400
// invalid_params, which still proves the handler responded.
func TestRevision_Status(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	params := types.RevisionStatusParamsData{Prefix: "workspace"}
	paramEnt, err := params.ToEntity()
	if err != nil {
		t.Fatalf("encode params: %v", err)
	}
	resp, err := ap.Executor().ExecuteWithParams("system/revision", "status", paramEnt)
	if err != nil {
		t.Fatalf("revision:status: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status code = %d, want 200; type=%s", resp.Status, resp.Type)
	}
}

// TestCompute_HandlerReachable confirms system/compute is dispatchable
// after CreatePeer wires the engine + RebuildDependencyIndex + the
// dispatcher's EvaluateExpression hook. Dispatching eval against a
// path with no installed expression is a domain error (4xx), not
// handler_not_found.
func TestCompute_HandlerReachable(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// eval against a path with no installed expression — handler is
	// hit; the error is from compute, not the registry.
	_, err = ap.Executor().Execute("system/compute", "eval")
	if err == nil {
		// 200 is also fine: handler reached. Test only fails on routing.
		return
	}
	if entitysdk.IsNotFound(err) && strings.Contains(err.Error(), "no handler") {
		t.Errorf("system/compute not registered; expected reachable handler, got %v", err)
	}
}

// TestRevision_DisabledRemovesHandler confirms RevisionConfig.Disabled
// removes the handler — opt-out works the same way it does for the
// other stable extensions.
func TestRevision_DisabledRemovesHandler(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			Revision: &entitysdk.RevisionConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	for _, h := range entitysdk.DiscoverHandlers(ap.PeerContext()) {
		if h.Pattern == "system/revision" {
			t.Errorf("system/revision present despite Revision disabled")
		}
	}
}

// TestCompute_DisabledRemovesHandler confirms the same opt-out works
// for compute.
func TestCompute_DisabledRemovesHandler(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			Compute: &entitysdk.ComputeConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	for _, h := range entitysdk.DiscoverHandlers(ap.PeerContext()) {
		if h.Pattern == "system/compute" {
			t.Errorf("system/compute present despite Compute disabled")
		}
	}
}
