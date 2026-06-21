package workbench

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/handler"
)

// ChainErrorsHandler is the on-error sink for continuation chains. The
// full bind path (persist payload + TreeSet at {target}/{request-id})
// is exercised by the continuation/mount integration tests; here we
// pin the deterministic guard surface.

func TestChainErrors_Guards(t *testing.T) {
	h := NewChainErrorsHandler()

	// Unknown operation.
	resp, err := h.Handle(context.Background(), &handler.Request{Operation: "frob"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 400 || errCode(t, resp) != "unknown_operation" {
		t.Fatalf("unknown op: status=%d code=%q", resp.Status, errCode(t, resp))
	}

	// Missing handler context.
	resp, err = h.Handle(context.Background(), &handler.Request{Operation: "receive"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 500 || errCode(t, resp) != "internal_error" {
		t.Fatalf("nil ctx: status=%d code=%q", resp.Status, errCode(t, resp))
	}

	// Context present but no resource target → missing_resource.
	_, s, li := testPeerContext(t)
	resp, err = h.Handle(context.Background(), &handler.Request{
		Operation: "receive",
		Context:   &handler.HandlerContext{Store: s, LocationIndex: li},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 400 || errCode(t, resp) != "missing_resource" {
		t.Fatalf("no resource: status=%d code=%q", resp.Status, errCode(t, resp))
	}
}

func TestChainErrors_Name(t *testing.T) {
	if got := NewChainErrorsHandler().Name(); got != "workbench-chain-errors" {
		t.Fatalf("Name() = %q", got)
	}
}
