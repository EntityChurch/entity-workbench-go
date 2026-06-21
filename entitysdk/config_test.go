package entitysdk

import (
	"context"
	"errors"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
)

func TestCreatePeerZeroConfig(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer(zero) failed: %v", err)
	}
	defer ap.Close()
	if ap.PeerID() == "" {
		t.Error("peer has empty PeerID")
	}
	// Basic round-trip — the peer is usable.
	if _, err := ap.Store().Put("p", "test/v", 1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !ap.Store().Has("p") {
		t.Error("Put/Has round-trip failed")
	}
}

func TestCreatePeerMemoryStorageExplicit(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{Storage: StorageConfig{Kind: "memory"}})
	if err != nil {
		t.Fatalf("CreatePeer(memory) failed: %v", err)
	}
	defer ap.Close()
}

func TestCreatePeerUnsupportedStorage(t *testing.T) {
	_, err := CreatePeer(PeerConfig{Storage: StorageConfig{Kind: "filebacked-someday"}})
	if err == nil {
		t.Fatal("expected error for unsupported storage")
	}
	if !IsNotSupported(err) {
		t.Errorf("expected 501 NotSupported, got %v", err)
	}
}

func TestCreatePeerSqliteRequiresPath(t *testing.T) {
	_, err := CreatePeer(PeerConfig{Storage: StorageConfig{Kind: "sqlite"}})
	if err == nil {
		t.Fatal("expected error when sqlite Path is empty")
	}
	if !IsClientError(err) {
		t.Errorf("expected 400 client error, got %v", err)
	}
}

type echoHandler struct{}

func (echoHandler) Name() string { return "test/echo" }

func (echoHandler) Handle(ctx context.Context, req *handler.Request) (*handler.Response, error) {
	return &handler.Response{Status: 200, Result: entity.Entity{Type: "test/echo"}}, nil
}

func TestCreatePeerCustomHandler(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{
		Handlers: []HandlerRegistration{
			{Pattern: "test/echo", Handler: echoHandler{}},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	resp, err := ap.Executor().Execute("test/echo", "whatever")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 || resp.Type != "test/echo" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestCreatePeerRejectsBadHandlerRegistration(t *testing.T) {
	cases := []HandlerRegistration{
		{Pattern: "", Handler: echoHandler{}},
		{Pattern: "test/x", Handler: nil},
	}
	for _, c := range cases {
		_, err := CreatePeer(PeerConfig{Handlers: []HandlerRegistration{c}})
		if err == nil {
			t.Errorf("accepted invalid registration %+v", c)
			continue
		}
		if !IsClientError(err) {
			t.Errorf("expected 400 client error for %+v, got %v", c, err)
		}
	}
}

func TestWildcardGrantScope(t *testing.T) {
	s := WildcardGrantScope()
	for name, dim := range map[string]ScopeDimension{
		"Handlers":   s.Handlers,
		"Operations": s.Operations,
		"Resources":  s.Resources,
		"Peers":      s.Peers,
	} {
		if len(dim.Include) != 1 || dim.Include[0] != "*" {
			t.Errorf("%s.Include = %v, want [*]", name, dim.Include)
		}
		if len(dim.Exclude) != 0 {
			t.Errorf("%s.Exclude = %v, want empty", name, dim.Exclude)
		}
	}
	if WildcardGrant().Scope.Handlers.Include[0] != "*" {
		t.Error("WildcardGrant scope is not wildcard")
	}
}

func TestCloseReturnsError(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ap.Close(); err != nil {
		t.Errorf("Close: unexpected error %v", err)
	}
	// Double-close: core Peer.Close is idempotent and returns nil on
	// second call — we should pass that through without wrapping as
	// an error.
	if err := ap.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	// Sanity check: errors.As still works with other SDK errors.
	e := NewError(404, "x", "y")
	var tgt *Error
	if !errors.As(e, &tgt) {
		t.Error("errors.As did not unwrap SDK Error")
	}
}
