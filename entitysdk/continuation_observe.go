package entitysdk

import (
	"context"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// ContinuationKind discriminates the three continuation entity types
// the observability surface returns.
type ContinuationKind string

const (
	ContinuationKindForward   ContinuationKind = "forward"   // system/continuation
	ContinuationKindJoin      ContinuationKind = "join"      // system/continuation/join
	ContinuationKindSuspended ContinuationKind = "suspended" // system/continuation/suspended
)

// ContinuationView is a structured projection of a continuation
// entity for listing / inspection. Common fields are surfaced
// directly; kind-specific fields are populated only when applicable
// (Expected/Received for join; Reason/ChainID/etc. for suspended).
//
// The raw entity hash is included so callers can fetch the underlying
// payload via Store.GetByHash if they need the unparsed form.
type ContinuationView struct {
	Path                string                           // tree path the entity is bound at
	Hash                hash.Hash                        // content hash of the continuation entity
	Kind                ContinuationKind                 // forward / join / suspended
	Target              string                           // dispatch target URI
	Operation           string                           // dispatch operation
	Resource            *types.ResourceTarget            // dispatch resource (if any)
	ResultField         string                           // slot for advance result (forward / join)
	ResultTransform     *types.ContinuationTransformData // transform applied to advance result (forward only)
	DeliverTo           *types.DeliverySpec              // where to route the dispatch result
	OnError             *types.DeliverySpec              // where to route errors
	RemainingExecutions *uint64                          // nil = standing (background-daemon-like)
	DispatchCapability  hash.Hash                        // cap authorizing the deferred dispatch

	// Join-only.
	Expected []string
	Received map[string]struct{} // slots that have been filled so far

	// Suspended-only.
	Reason         string
	ChainID        string
	OriginalAuthor hash.Hash
	SuspendedAt    uint64
}

// ListSuspended returns all suspended continuations under the
// canonical `system/continuation/suspended/` prefix. Each view
// carries the suspension reason + chain_id so callers can decide
// which to resume vs abandon.
func (cc *ContinuationClient) ListSuspended(ctx context.Context) ([]ContinuationView, error) {
	return cc.ListAt(ctx, "system/continuation/suspended/")
}

// ListAt returns continuations bound directly under pathPrefix (one
// level deep). Non-continuation entities at the prefix are skipped.
// For SDK-installed chains, pair with the InboxPath convention to
// enumerate by purpose / instance.
//
// The listing is shallow — callers wanting recursion (e.g. walking
// `system/inbox/follow/*` to find all follow chains) iterate by
// invoking ListAt on each child path.
func (cc *ContinuationClient) ListAt(ctx context.Context, pathPrefix string) ([]ContinuationView, error) {
	if pathPrefix == "" {
		return nil, NewError(400, "invalid_prefix", "list prefix is empty")
	}
	entries, err := cc.ap.List(pathPrefix)
	if err != nil {
		return nil, err
	}
	views := make([]ContinuationView, 0, len(entries))
	for _, e := range entries {
		if e.HasChildren {
			continue // directory; ListAt is shallow
		}
		view, ok, err := cc.inspectPath(e.Path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue // not a continuation entity
		}
		views = append(views, view)
	}
	return views, nil
}

// Inspect returns a structured view of the continuation at path, or
// (ContinuationView{}, false, nil) if no continuation entity exists
// there. Non-continuation entities at the path return (_, false, nil)
// — callers needing to distinguish absent vs wrong-type should
// follow up with a typed AppPeer.Get.
func (cc *ContinuationClient) Inspect(ctx context.Context, path string) (ContinuationView, bool, error) {
	view, ok, err := cc.inspectPath(path)
	return view, ok, err
}

func (cc *ContinuationClient) inspectPath(path string) (ContinuationView, bool, error) {
	ent, ok, err := cc.ap.Get(path)
	if err != nil {
		return ContinuationView{}, false, err
	}
	if !ok {
		return ContinuationView{}, false, nil
	}
	switch ent.Type {
	case types.TypeContinuation:
		cont, err := types.ContinuationDataFromEntity(ent)
		if err != nil {
			return ContinuationView{}, false,
				WrapError(500, "decode_continuation",
					"decode forward continuation at "+path, err)
		}
		return ContinuationView{
			Path:                path,
			Hash:                ent.ContentHash,
			Kind:                ContinuationKindForward,
			Target:              cont.Target,
			Operation:           cont.Operation,
			Resource:            cont.Resource,
			ResultField:         cont.ResultField,
			ResultTransform:     cont.ResultTransform,
			DeliverTo:           cont.DeliverTo,
			OnError:             cont.OnError,
			RemainingExecutions: cont.RemainingExecutions,
			DispatchCapability:  cont.DispatchCapability,
		}, true, nil
	case types.TypeContinuationJoin:
		j, err := types.ContinuationJoinDataFromEntity(ent)
		if err != nil {
			return ContinuationView{}, false,
				WrapError(500, "decode_continuation",
					"decode join continuation at "+path, err)
		}
		recv := make(map[string]struct{}, len(j.Received))
		for k := range j.Received {
			recv[k] = struct{}{}
		}
		return ContinuationView{
			Path:                path,
			Hash:                ent.ContentHash,
			Kind:                ContinuationKindJoin,
			Target:              j.Target,
			Operation:           j.Operation,
			Resource:            j.Resource,
			ResultField:         j.ResultField,
			DeliverTo:           j.DeliverTo,
			OnError:             j.OnError,
			RemainingExecutions: j.RemainingExecutions,
			DispatchCapability:  j.DispatchCapability,
			Expected:            append([]string(nil), j.Expected...),
			Received:            recv,
		}, true, nil
	case types.TypeContinuationSuspended:
		s, err := types.ContinuationSuspendedDataFromEntity(ent)
		if err != nil {
			return ContinuationView{}, false,
				WrapError(500, "decode_continuation",
					"decode suspended continuation at "+path, err)
		}
		return ContinuationView{
			Path:           path,
			Hash:           ent.ContentHash,
			Kind:           ContinuationKindSuspended,
			Target:         s.Target,
			Operation:      s.Operation,
			Resource:       s.Resource,
			Reason:         s.Reason,
			ChainID:        s.ChainID,
			OriginalAuthor: s.OriginalAuthor,
			SuspendedAt:    s.SuspendedAt,
		}, true, nil
	default:
		return ContinuationView{}, false, nil
	}
}

// IsStanding reports whether the continuation runs indefinitely
// (RemainingExecutions nil = no cap, daemon-like) versus one-shot
// or counted (RemainingExecutions set).
func (v ContinuationView) IsStanding() bool {
	return v.Kind != ContinuationKindSuspended && v.RemainingExecutions == nil
}

// Summary returns a one-line human-readable representation of the
// view for shell rendering — "{kind} {path} → {target}:{op}".
func (v ContinuationView) Summary() string {
	var b strings.Builder
	b.WriteString(string(v.Kind))
	b.WriteString(" ")
	b.WriteString(v.Path)
	b.WriteString(" → ")
	b.WriteString(v.Target)
	b.WriteString(":")
	b.WriteString(v.Operation)
	if v.Kind == ContinuationKindSuspended && v.Reason != "" {
		b.WriteString(" (")
		b.WriteString(v.Reason)
		b.WriteString(")")
	}
	return b.String()
}
