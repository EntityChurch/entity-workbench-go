package shellcmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entity-workbench-go/entitysdk"
)

// cmdContinuation dispatches `continuation <subcommand>` against
// the local peer's continuation handler + tree. Per
// SHELL-DIRECTION.md and GUIDE-CONTINUATIONS-WORKBENCH.md, this is
// the "ps for continuations" surface — observe what processes are
// installed, see their state, and tear them down.
//
// Subcommands:
//
//	continuation ls [path-prefix]   — list installed continuations under prefix
//	continuation suspended          — list suspended continuations
//	continuation inspect <path>     — show detailed view of a continuation
//	continuation abandon <path>     — drop a suspended continuation
//	continuation resume <path>      — re-dispatch a suspended continuation
//
// Default ls prefix is system/inbox/ (the SDK convention for
// installed chains). Pass an explicit prefix to scope.
func cmdContinuation(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: continuation <ls|suspended|inspect|abandon|resume> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls", "list":
		return cmdContinuationLs(sh, rest)
	case "suspended":
		return cmdContinuationSuspended(sh, rest)
	case "inspect", "show":
		return cmdContinuationInspect(sh, rest)
	case "abandon":
		return cmdContinuationAbandon(sh, rest)
	case "resume":
		return cmdContinuationResume(sh, rest)
	default:
		return Result{}, fmt.Errorf("unknown continuation subcommand: %s", sub)
	}
}

func cmdContinuationLs(sh *Shell, args []string) (Result, error) {
	prefix := "system/inbox/"
	if len(args) >= 1 {
		prefix = args[0]
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}
	views, err := sh.Local.Peer.Continuation().ListAt(context.Background(), prefix)
	if err != nil {
		return Result{}, fmt.Errorf("continuation ls: %w", err)
	}
	if len(views) == 0 {
		return MessageResult(fmt.Sprintf("(no continuations under %s)", prefix)), nil
	}
	return renderContinuationViews(views), nil
}

func cmdContinuationSuspended(sh *Shell, _ []string) (Result, error) {
	views, err := sh.Local.Peer.Continuation().ListSuspended(context.Background())
	if err != nil {
		return Result{}, fmt.Errorf("continuation suspended: %w", err)
	}
	if len(views) == 0 {
		return MessageResult("(no suspended continuations)"), nil
	}
	return renderContinuationViews(views), nil
}

func cmdContinuationInspect(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: continuation inspect <path>")
	}
	view, ok, err := sh.Local.Peer.Continuation().Inspect(context.Background(), args[0])
	if err != nil {
		return Result{}, fmt.Errorf("continuation inspect: %w", err)
	}
	if !ok {
		return MessageResult(fmt.Sprintf("(no continuation at %s)", args[0])), nil
	}
	return LinesResult(formatContinuationDetail(view)), nil
}

func cmdContinuationAbandon(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: continuation abandon <path>")
	}
	if err := sh.Local.Peer.Continuation().Abandon(context.Background(), args[0]); err != nil {
		return Result{}, fmt.Errorf("continuation abandon: %w", err)
	}
	return MessageResult(fmt.Sprintf("abandoned %s", args[0])), nil
}

func cmdContinuationResume(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: continuation resume <path>")
	}
	if err := sh.Local.Peer.Continuation().Resume(context.Background(), args[0]); err != nil {
		return Result{}, fmt.Errorf("continuation resume: %w", err)
	}
	return MessageResult(fmt.Sprintf("resumed %s", args[0])), nil
}

func renderContinuationViews(views []entitysdk.ContinuationView) Result {
	lines := make([]string, 0, len(views)+1)
	lines = append(lines, fmt.Sprintf("%-8s  %-50s  %s", "KIND", "PATH", "→ DISPATCH"))
	for _, v := range views {
		standing := ""
		if v.IsStanding() {
			standing = " [standing]"
		}
		dispatch := v.Target + ":" + v.Operation
		if v.Kind == entitysdk.ContinuationKindSuspended && v.Reason != "" {
			dispatch += " (" + v.Reason + ")"
		}
		lines = append(lines, fmt.Sprintf("%-8s  %-50s  %s%s",
			string(v.Kind), v.Path, dispatch, standing))
	}
	return LinesResult(lines)
}

func formatContinuationDetail(v entitysdk.ContinuationView) []string {
	out := []string{
		fmt.Sprintf("path:               %s", v.Path),
		fmt.Sprintf("hash:               %s", v.Hash),
		fmt.Sprintf("kind:               %s", v.Kind),
		fmt.Sprintf("target:             %s", v.Target),
		fmt.Sprintf("operation:          %s", v.Operation),
	}
	if v.Resource != nil && len(v.Resource.Targets) > 0 {
		out = append(out, fmt.Sprintf("resource:           %s", strings.Join(v.Resource.Targets, ", ")))
	}
	if v.ResultField != "" {
		out = append(out, fmt.Sprintf("result_field:       %s", v.ResultField))
	}
	if v.ResultTransform != nil {
		t := v.ResultTransform
		var parts []string
		if t.Extract != "" {
			parts = append(parts, "extract="+t.Extract)
		}
		if len(t.Select) > 0 {
			parts = append(parts, fmt.Sprintf("select=%v", t.Select))
		}
		if t.ResourceExtract != "" {
			parts = append(parts, "resource_extract="+t.ResourceExtract)
		}
		if t.TargetExtract != "" {
			parts = append(parts, "target_extract="+t.TargetExtract)
		}
		if t.OperationExtract != "" {
			parts = append(parts, "operation_extract="+t.OperationExtract)
		}
		out = append(out, fmt.Sprintf("result_transform:   %s", strings.Join(parts, ", ")))
	}
	if v.DeliverTo != nil {
		out = append(out, fmt.Sprintf("deliver_to:         %s:%s",
			v.DeliverTo.URI, v.DeliverTo.Operation))
	}
	if v.OnError != nil {
		out = append(out, fmt.Sprintf("on_error:           %s:%s",
			v.OnError.URI, v.OnError.Operation))
	}
	if v.RemainingExecutions != nil {
		out = append(out, fmt.Sprintf("remaining:          %d", *v.RemainingExecutions))
	} else if v.Kind != entitysdk.ContinuationKindSuspended {
		out = append(out, "remaining:          standing (no cap)")
	}
	if !v.DispatchCapability.IsZero() {
		out = append(out, fmt.Sprintf("dispatch_cap:       %s", v.DispatchCapability))
	}
	if v.Kind == entitysdk.ContinuationKindJoin {
		out = append(out, fmt.Sprintf("expected:           %s", strings.Join(v.Expected, ", ")))
		if len(v.Received) > 0 {
			received := make([]string, 0, len(v.Received))
			for k := range v.Received {
				received = append(received, k)
			}
			out = append(out, fmt.Sprintf("received:           %s", strings.Join(received, ", ")))
		}
	}
	if v.Kind == entitysdk.ContinuationKindSuspended {
		out = append(out, fmt.Sprintf("reason:             %s", v.Reason))
		out = append(out, fmt.Sprintf("chain_id:           %s", v.ChainID))
		if !v.OriginalAuthor.IsZero() {
			out = append(out, fmt.Sprintf("original_author:    %s", v.OriginalAuthor))
		}
		if v.SuspendedAt != 0 {
			when := time.UnixMilli(int64(v.SuspendedAt)).UTC().Format(time.RFC3339)
			out = append(out, fmt.Sprintf("suspended_at:       %s", when))
		}
	}
	return out
}
