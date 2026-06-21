package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// SubscriptionClient observes subscription entities bound under the
// canonical `system/subscription/{id}` prefix. The subscription
// handler writes one entity per active subscription; this client
// reads them back for "what subscriptions does this peer have?"
// queries.
//
// To create / cancel subscriptions use AppPeer.Subscribe /
// SubscribeAt and the returned Subscription handle's Close — those
// take care of inbox handler lifecycle that listing alone can't see.
type SubscriptionClient struct {
	ap *AppPeer
}

// Subscriptions returns a SubscriptionClient targeting the local
// peer's subscription registry.
func (a *AppPeer) Subscriptions() *SubscriptionClient {
	return &SubscriptionClient{ap: a}
}

// SubscriptionView is a structured projection of a subscription
// entity for listing / inspection.
type SubscriptionView struct {
	Path               string // tree path the entity is bound at
	Hash               hash.Hash
	SubscriptionID     string
	Pattern            string
	Events             []string
	DeliverURI         string
	DeliverOperation   string
	SubscriberIdentity hash.Hash
	DeliverToken       hash.Hash
	CreatedAt          uint64
	Limits             *types.SubscriptionLimitsData
}

// List returns all subscriptions currently bound under
// `system/subscription/`.
func (sc *SubscriptionClient) List(ctx context.Context) ([]SubscriptionView, error) {
	entries, err := sc.ap.List("system/subscription/")
	if err != nil {
		return nil, err
	}
	views := make([]SubscriptionView, 0, len(entries))
	for _, e := range entries {
		if e.HasChildren {
			continue
		}
		view, ok, err := sc.inspectPath(e.Path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		views = append(views, view)
	}
	return views, nil
}

// Inspect returns the subscription bound at path (typically
// `system/subscription/{id}`). Returns (_, false, nil) when no
// subscription entity is found there.
func (sc *SubscriptionClient) Inspect(ctx context.Context, path string) (SubscriptionView, bool, error) {
	return sc.inspectPath(path)
}

func (sc *SubscriptionClient) inspectPath(path string) (SubscriptionView, bool, error) {
	ent, ok, err := sc.ap.Get(path)
	if err != nil {
		return SubscriptionView{}, false, err
	}
	if !ok {
		return SubscriptionView{}, false, nil
	}
	if ent.Type != types.TypeSubscription {
		return SubscriptionView{}, false, nil
	}
	sub, err := types.SubscriptionDataFromEntity(ent)
	if err != nil {
		return SubscriptionView{}, false,
			WrapError(500, "decode_subscription",
				"decode subscription at "+path, err)
	}
	return SubscriptionView{
		Path:               path,
		Hash:               ent.ContentHash,
		SubscriptionID:     sub.SubscriptionID,
		Pattern:            sub.Pattern,
		Events:             append([]string(nil), sub.Events...),
		DeliverURI:         sub.DeliverURI,
		DeliverOperation:   sub.DeliverOperation,
		SubscriberIdentity: sub.SubscriberIdentity,
		DeliverToken:       sub.DeliverToken,
		CreatedAt:          sub.CreatedAt,
		Limits:             sub.Limits,
	}, true, nil
}
