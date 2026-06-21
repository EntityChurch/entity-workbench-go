package shellcmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// cmdSubscription dispatches `subscription <subcommand>` against
// the local peer's subscription registry. Companion to
// `continuation` — what the peer is listening to vs what's been
// installed to react.
//
// Subcommands:
//
//	subscription ls                  — list active subscriptions
//	subscription inspect <path|id>   — show detailed view
//	subscription rm <id>             — cancel by subscription_id
func cmdSubscription(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: subscription <ls|inspect|rm> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls", "list":
		return cmdSubscriptionLs(sh, rest)
	case "inspect", "show":
		return cmdSubscriptionInspect(sh, rest)
	case "rm", "cancel", "unsubscribe":
		return cmdSubscriptionRm(sh, rest)
	default:
		return Result{}, fmt.Errorf("unknown subscription subcommand: %s", sub)
	}
}

func cmdSubscriptionLs(sh *Shell, _ []string) (Result, error) {
	views, err := sh.Local.Peer.Subscriptions().List(context.Background())
	if err != nil {
		return Result{}, fmt.Errorf("subscription ls: %w", err)
	}
	if len(views) == 0 {
		return MessageResult("(no active subscriptions)"), nil
	}
	lines := make([]string, 0, len(views)+1)
	lines = append(lines, fmt.Sprintf("%-12s  %-40s  %s", "ID", "PATTERN", "→ DELIVER"))
	for _, v := range views {
		id := v.SubscriptionID
		if len(id) > 12 {
			id = id[:12]
		}
		lines = append(lines, fmt.Sprintf("%-12s  %-40s  %s:%s",
			id, v.Pattern, v.DeliverURI, v.DeliverOperation))
	}
	return LinesResult(lines), nil
}

func cmdSubscriptionInspect(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: subscription inspect <path|id>")
	}
	target := args[0]
	// If the arg looks like a path (contains '/'), inspect directly.
	// Otherwise treat it as a subscription_id and find the entity in
	// the listing.
	var view entitysdk.SubscriptionView
	var ok bool
	if strings.Contains(target, "/") {
		var err error
		view, ok, err = sh.Local.Peer.Subscriptions().Inspect(context.Background(), target)
		if err != nil {
			return Result{}, fmt.Errorf("subscription inspect: %w", err)
		}
	} else {
		views, err := sh.Local.Peer.Subscriptions().List(context.Background())
		if err != nil {
			return Result{}, fmt.Errorf("subscription inspect: %w", err)
		}
		for _, v := range views {
			if v.SubscriptionID == target || strings.HasPrefix(v.SubscriptionID, target) {
				view = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return MessageResult(fmt.Sprintf("(no subscription matching %q)", target)), nil
	}
	return LinesResult(formatSubscriptionDetail(view)), nil
}

func cmdSubscriptionRm(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: subscription rm <id>")
	}
	id := args[0]
	cancelReq := types.SubscriptionCancelData{SubscriptionID: id}
	paramsEnt, err := cancelReq.ToEntity()
	if err != nil {
		return Result{}, fmt.Errorf("encode cancel request: %w", err)
	}
	if _, err := sh.Local.Peer.Executor().ExecuteWithParams(
		"system/subscription", "unsubscribe", paramsEnt,
	); err != nil {
		return Result{}, fmt.Errorf("subscription rm: %w", err)
	}
	return MessageResult(fmt.Sprintf("unsubscribed %s", id)), nil
}

func formatSubscriptionDetail(v entitysdk.SubscriptionView) []string {
	out := []string{
		fmt.Sprintf("path:                %s", v.Path),
		fmt.Sprintf("subscription_id:     %s", v.SubscriptionID),
		fmt.Sprintf("pattern:             %s", v.Pattern),
		fmt.Sprintf("events:              %s", strings.Join(v.Events, ", ")),
		fmt.Sprintf("deliver_to:          %s:%s", v.DeliverURI, v.DeliverOperation),
	}
	if !v.SubscriberIdentity.IsZero() {
		out = append(out, fmt.Sprintf("subscriber:          %s", v.SubscriberIdentity))
	}
	if !v.DeliverToken.IsZero() {
		out = append(out, fmt.Sprintf("deliver_token:       %s", v.DeliverToken))
	}
	if v.CreatedAt != 0 {
		when := time.UnixMilli(int64(v.CreatedAt)).UTC().Format(time.RFC3339)
		out = append(out, fmt.Sprintf("created_at:          %s", when))
	}
	if v.Limits != nil {
		out = append(out, fmt.Sprintf("limits:              %+v", *v.Limits))
	}
	return out
}
