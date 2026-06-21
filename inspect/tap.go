package inspect

// Path tap — register a recording handler at a path. Every dispatch
// arriving at the path is captured with raw entity, decoded CBOR,
// and (when the params are a system/protocol/inbox/delivery) the
// unwrapped delivery + decoded result.
//
// The tap is a terminal observation: it returns 200 with an
// acknowledgement entity rather than forwarding the request. To keep
// the chain flowing while observing, install the tap at a sibling
// path and route deliveries to it via DeliverTo or OnError.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

// Tap is a recording inbox handler installed at a specific path.
type Tap struct {
	handle *entitysdk.HandlerHandle
	path   string
	seq    atomic.Int64

	mu       sync.Mutex
	captures []Capture
}

// Capture records one dispatch that arrived at a tap.
type Capture struct {
	Seq       int64
	Timestamp time.Time
	RequestID string
	Path      string
	Operation string

	// Raw params entity as received.
	ParamsType string
	ParamsHash hash.Hash
	ParamsData cbor.RawMessage

	// Decoded form of the params data. Best-effort; may be nil if
	// decode fails.
	ParamsDecoded interface{}

	// If the params is a system/protocol/inbox/delivery, the
	// unwrapped delivery + decoded result.
	IsDelivery       bool
	DeliveryStatus   uint
	DeliveryRequest  string
	DeliveryResultRM cbor.RawMessage
	DeliveryResult   interface{}
}

// InstallTap installs a recording handler at path on peer. The
// handler accepts the "receive" operation by default; pass extra
// operations via ops.
func InstallTap(peer *entitysdk.AppPeer, path string, ops ...string) (*Tap, error) {
	if len(ops) == 0 {
		ops = []string{"receive"}
	}
	opMap := map[string]coretypes.HandlerOperationSpec{}
	for _, op := range ops {
		opMap[op] = coretypes.HandlerOperationSpec{InputType: "primitive/any"}
	}

	t := &Tap{path: path}

	h, err := peer.RegisterHandler(entitysdk.HandlerSpec{
		Pattern:    path,
		Name:       "inspect-tap",
		Operations: opMap,
	}, func(_ context.Context, req *handler.Request) (*handler.Response, error) {
		n := t.seq.Add(1)
		c := Capture{
			Seq:        n,
			Timestamp:  time.Now(),
			Path:       req.Path,
			Operation:  req.Operation,
			ParamsType: req.Params.Type,
			ParamsHash: req.Params.ContentHash,
			ParamsData: req.Params.Data,
		}
		if req.Context != nil {
			c.RequestID = req.Context.RequestID
		}
		_ = cbor.Unmarshal(req.Params.Data, &c.ParamsDecoded)

		if req.Params.Type == coretypes.TypeInboxDelivery {
			c.IsDelivery = true
			if d, derr := coretypes.InboxDeliveryDataFromEntity(req.Params); derr == nil {
				c.DeliveryStatus = d.Status
				c.DeliveryRequest = d.OriginalRequestID
				c.DeliveryResultRM = d.Result
				_ = cbor.Unmarshal(d.Result, &c.DeliveryResult)
			}
		}

		t.mu.Lock()
		t.captures = append(t.captures, c)
		t.mu.Unlock()

		// Bind a debug copy at an observable path so an external
		// observer can browse captures without holding our channel.
		if req.Context != nil && req.Context.Store != nil {
			storedHash, perr := req.Context.Store.Put(req.Params)
			if perr == nil {
				debugPath := fmt.Sprintf("system/runtime/tap/%s/%05d", t.path, n)
				_, _ = req.Context.TreeSet(debugPath, storedHash, "receive")
			}
		}

		resultRaw, _ := cbor.Marshal(map[string]interface{}{
			"tapped": true,
			"seq":    n,
		})
		result, _ := entity.NewEntity("system/runtime/tap/ack", resultRaw)
		return &handler.Response{Status: 200, Result: result}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("register tap at %s: %w", path, err)
	}
	t.handle = h
	return t, nil
}

// Close unregisters the tap. Captures already recorded remain
// accessible via Captures().
func (t *Tap) Close() error {
	if t.handle == nil {
		return nil
	}
	return t.handle.Close()
}

// Captures returns a snapshot of recorded captures.
func (t *Tap) Captures() []Capture {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Capture, len(t.captures))
	copy(out, t.captures)
	return out
}

// Count returns how many captures the tap has recorded.
func (t *Tap) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.captures)
}
