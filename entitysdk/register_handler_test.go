package entitysdk

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/types"
)

// okSpec returns a minimal valid HandlerSpec at the given pattern.
func okSpec(pattern string) HandlerSpec {
	return HandlerSpec{
		Pattern: pattern,
		Name:    "test-handler",
		Operations: map[string]types.HandlerOperationSpec{
			"ping": {InputType: "primitive/any"},
		},
	}
}

// okBody returns a 200 response regardless of input. Tests that need
// to observe invocation can capture their own state in a closure.
func okBody(ctx context.Context, req *handler.Request) (*handler.Response, error) {
	ent, err := entity.NewEntity("primitive/any", nil)
	if err != nil {
		return nil, err
	}
	return &handler.Response{Status: 200, Result: ent}, nil
}

func TestRegisterHandlerWritesThreeTreePathsWithoutGrant(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	h, err := ap.RegisterHandler(okSpec("app/test/greeter"), okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	defer h.Close()

	li := ap.peer.LocationIndex()
	if !li.Has("app/test/greeter") {
		t.Error("handler entity missing at pattern path")
	}
	if !li.Has("system/handler/app/test/greeter") {
		t.Error("interface entity missing at system/handler/{pattern}")
	}
	if li.Has("system/capability/grants/app/test/greeter") {
		t.Error("grant entity should NOT be written when InternalScope is nil")
	}
}

func TestRegisterHandlerWritesGrantWhenInternalScopeSet(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	spec := okSpec("app/test/with-grant")
	spec.InternalScope = []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/inbox/*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		},
	}

	h, err := ap.RegisterHandler(spec, okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	defer h.Close()

	li := ap.peer.LocationIndex()
	if !li.Has("system/capability/grants/app/test/with-grant") {
		t.Error("grant entity missing when InternalScope is set")
	}
}

func TestHandlerHandleCloseRemovesAllTreeEntries(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	spec := okSpec("app/test/closing")
	spec.InternalScope = []types.GrantEntry{
		{Operations: types.CapabilityScope{Include: []string{"*"}}},
	}

	h, err := ap.RegisterHandler(spec, okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	li := ap.peer.LocationIndex()
	if li.Has("app/test/closing") {
		t.Error("handler entity not removed on Close")
	}
	if li.Has("system/handler/app/test/closing") {
		t.Error("interface entity not removed on Close")
	}
	if li.Has("system/capability/grants/app/test/closing") {
		t.Error("grant entity not removed on Close")
	}
	if _, ok := ap.peer.Registry().Handlers()["app/test/closing"]; ok {
		t.Error("dispatch index entry not removed on Close")
	}
}

func TestHandlerHandleCloseIsIdempotent(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	h, err := ap.RegisterHandler(okSpec("app/test/idempotent"), okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Errorf("second Close: %v (expected nil)", err)
	}
	if err := h.Close(); err != nil {
		t.Errorf("third Close: %v (expected nil)", err)
	}
}

func TestRegisterHandlerCollisionReturns409(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	first, err := ap.RegisterHandler(okSpec("app/test/collide"), okBody)
	if err != nil {
		t.Fatalf("first RegisterHandler: %v", err)
	}
	defer first.Close()

	_, err = ap.RegisterHandler(okSpec("app/test/collide"), okBody)
	if err == nil {
		t.Fatal("second RegisterHandler should have failed")
	}
	sdkErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if sdkErr.Status != 409 {
		t.Errorf("status: want 409, got %d", sdkErr.Status)
	}
	if sdkErr.Code != "pattern_collision" {
		t.Errorf("code: want pattern_collision, got %q", sdkErr.Code)
	}
}

func TestRegisterHandlerCollisionAfterCloseSucceeds(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	h1, err := ap.RegisterHandler(okSpec("app/test/reuse"), okBody)
	if err != nil {
		t.Fatalf("first RegisterHandler: %v", err)
	}
	if err := h1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h2, err := ap.RegisterHandler(okSpec("app/test/reuse"), okBody)
	if err != nil {
		t.Fatalf("second RegisterHandler after close: %v", err)
	}
	defer h2.Close()
}

func TestRegisterHandlerInvalidSpecReturns400(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	cases := []struct {
		label string
		spec  HandlerSpec
	}{
		{"empty pattern", HandlerSpec{Pattern: "", Name: "n", Operations: map[string]types.HandlerOperationSpec{"x": {}}}},
		{"leading slash", HandlerSpec{Pattern: "/foo", Name: "n", Operations: map[string]types.HandlerOperationSpec{"x": {}}}},
		{"empty name", HandlerSpec{Pattern: "app/x", Operations: map[string]types.HandlerOperationSpec{"x": {}}}},
		{"empty operations", HandlerSpec{Pattern: "app/x", Name: "n"}},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			_, err := ap.RegisterHandler(c.spec, okBody)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			sdkErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("expected *Error, got %T", err)
			}
			if sdkErr.Status != 400 {
				t.Errorf("status: want 400, got %d", sdkErr.Status)
			}
			if sdkErr.Code != "invalid_handler_spec" {
				t.Errorf("code: want invalid_handler_spec, got %q", sdkErr.Code)
			}
		})
	}
}

func TestRegisterHandlerNilBodyReturns400(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	_, err = ap.RegisterHandler(okSpec("app/test/nilbody"), nil)
	if err == nil {
		t.Fatal("expected error on nil body")
	}
	sdkErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if sdkErr.Status != 400 || sdkErr.Code != "invalid_handler_spec" {
		t.Errorf("want 400 invalid_handler_spec, got %d %q", sdkErr.Status, sdkErr.Code)
	}
}

func TestRegisteredHandlerDispatches(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	called := 0
	spec := okSpec("app/test/dispatch")
	body := func(ctx context.Context, req *handler.Request) (*handler.Response, error) {
		called++
		ent, _ := entity.NewEntity("primitive/any", nil)
		return &handler.Response{Status: 200, Result: ent}, nil
	}

	h, err := ap.RegisterHandler(spec, body)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	defer h.Close()

	got, pattern, ok := ap.peer.Registry().Resolve("app/test/dispatch")
	if !ok {
		t.Fatal("dispatch index: handler not resolvable at registered pattern")
	}
	if pattern != "app/test/dispatch" {
		t.Errorf("pattern: want app/test/dispatch, got %q", pattern)
	}
	resp, err := got.Handle(context.Background(), &handler.Request{
		Path:      "app/test/dispatch",
		Operation: "ping",
	})
	if err != nil {
		t.Fatalf("handler dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status: want 200, got %d", resp.Status)
	}
	if called != 1 {
		t.Errorf("body invocations: want 1, got %d", called)
	}
}

func TestRegisterHandlerInterfaceEntityIsPublicContract(t *testing.T) {
	// The interface entity written at system/handler/{pattern} MUST
	// carry only the public contract — pattern, name, operations — and
	// NOT any security config (max_scope, internal_scope). §11.5.1.
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	spec := okSpec("app/test/iface")
	spec.InternalScope = []types.GrantEntry{
		{Operations: types.CapabilityScope{Include: []string{"*"}}},
	}

	h, err := ap.RegisterHandler(spec, okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	defer h.Close()

	li := ap.peer.LocationIndex()
	cs := ap.peer.Store()
	ifaceHash, ok := li.Get("system/handler/app/test/iface")
	if !ok {
		t.Fatal("interface entity not in location index")
	}
	ifaceEnt, ok := cs.Get(ifaceHash)
	if !ok {
		t.Fatal("interface entity missing from content store")
	}
	iface, err := types.HandlerInterfaceDataFromEntity(ifaceEnt)
	if err != nil {
		t.Fatalf("decode interface: %v", err)
	}
	if iface.Pattern != "app/test/iface" {
		t.Errorf("iface pattern: want app/test/iface, got %q", iface.Pattern)
	}
	if iface.Name != "test-handler" {
		t.Errorf("iface name: want test-handler, got %q", iface.Name)
	}
	if _, ok := iface.Operations["ping"]; !ok {
		t.Error("iface operations missing ping")
	}
	// HandlerInterfaceData has no scope fields at the type level — the
	// spec requirement is structural. If a future change adds them,
	// this test should be expanded to assert they stay empty.
}

func TestRegisterHandlerHandlerEntityReferencesInterfaceByPath(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	h, err := ap.RegisterHandler(okSpec("app/test/pathref"), okBody)
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	defer h.Close()

	li := ap.peer.LocationIndex()
	cs := ap.peer.Store()
	handlerHash, ok := li.Get("app/test/pathref")
	if !ok {
		t.Fatal("handler entity not in location index")
	}
	handlerEnt, ok := cs.Get(handlerHash)
	if !ok {
		t.Fatal("handler entity missing from content store")
	}
	hd, err := types.HandlerDataFromEntity(handlerEnt)
	if err != nil {
		t.Fatalf("decode handler: %v", err)
	}
	if hd.Interface != "system/handler/app/test/pathref" {
		t.Errorf("handler interface ref: want system/handler/app/test/pathref, got %q", hd.Interface)
	}
}

func TestValidateHandlerSpecMessages(t *testing.T) {
	// Quick guard against the error messages drifting — they're
	// visible in API consumers.
	err := validateHandlerSpec(HandlerSpec{})
	if err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Errorf("expected pattern error, got %v", err)
	}
}
