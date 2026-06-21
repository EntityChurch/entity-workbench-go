package shellcmd

import (
	"fmt"
	"strings"
	"time"

	"entity-workbench-go/entitysdk"
)

// cmdTail implements `tail <path> [-n N] [-timeout DUR]` — wait for
// the next N change events matching path (or pattern with `*` suffix
// for prefix) and return them as a Result.
//
// Tier E (SUBSCRIPTION-derived) per GUIDE-SHELL-FRAMING.md. Wraps the
// SDK SubscribeAt surface; both local and cross-peer paths work
// uniformly (cross-peer goes through the dispatch + inbox bridge).
//
// Bounded shape (no `-f` follow mode yet). Streaming output requires a
// Result variant we haven't added; the bounded shape covers single-
// shot wait + N-burst-collection cases and is testable. Adding `-f`
// is a follow-on once streaming Result emerges or once cross-impl
// converges on a streaming convention.
//
// Defaults: `-n 1`, `-timeout 30s`.
func cmdTail(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: tail <path> [-n N] [-timeout DUR]")
	}
	target := args[0]
	n := 1
	timeout := 30 * time.Second
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-n":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("tail: -n requires a value")
			}
			if _, err := fmt.Sscanf(args[i+1], "%d", &n); err != nil || n < 1 {
				return Result{}, fmt.Errorf("tail: -n must be a positive integer, got %q", args[i+1])
			}
			i++
		case "-timeout":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("tail: -timeout requires a value")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				return Result{}, fmt.Errorf("tail: -timeout %q: %w", args[i+1], err)
			}
			timeout = d
			i++
		default:
			return Result{}, fmt.Errorf("tail: unknown flag %q", args[i])
		}
	}

	pc := sh.Local
	if pc == nil {
		return Result{}, fmt.Errorf("tail: no local peer (run with a peer context)")
	}

	// Resolve target into (peerID, pattern within peer namespace). The
	// dispatcher-tier PathArgs(0) has already expanded any @alias →
	// peer-id, so target is now /{peer-id}/bare/path.
	peerID, pattern, err := splitTargetForSubscribe(sh, target)
	if err != nil {
		return Result{}, err
	}

	events, err := Tail(pc.Peer, peerID, pattern, n, timeout)
	if err != nil {
		return Result{}, err
	}
	if len(events) == 0 {
		return MessageResult(fmt.Sprintf("(no events on %s within %s)", target, timeout)), nil
	}
	return LinesResult(events), nil
}

// splitTargetForSubscribe takes a resolved shell path and returns the
// peer-id + the bare pattern within the peer's namespace suitable for
// SubscribeAt.
//
// Resolution rules:
//   - Path `/{known-peer-id}/bare/...` → split as peer-id + bare
//   - Path `/{unknown-segment}/...` → treat as local-peer-relative;
//     PathArgs(0) leaves unknown aliases as literal first segment, so
//     a bare path like "workspace/note" arrives here as
//     `/workspace/note` and we route to the local peer
//   - Bare path (no leading `/`) → local-peer-relative
//
// Trailing `*` is preserved as the prefix marker per SDK-OPERATIONS
// §6.1 pattern forms.
func splitTargetForSubscribe(sh *Shell, target string) (peerID, pattern string, err error) {
	if sh.Local == nil {
		return "", "", fmt.Errorf("tail: no local peer; cannot resolve %q", target)
	}
	p := Path(target)
	first := p.PeerID()
	if first == sh.Local.PeerID {
		peerID = first
		pattern = p.BarePath()
	} else if _, known := sh.Conns[first]; known {
		peerID = first
		pattern = p.BarePath()
	} else {
		peerID = sh.Local.PeerID
		pattern = strings.TrimPrefix(target, "/")
	}
	if pattern == "" {
		return "", "", fmt.Errorf("tail: empty pattern after resolving %q", target)
	}
	return peerID, pattern, nil
}

// Tail is the exported verb-op per GUIDE-SHELL-FRAMING.md §8.1: a
// pure callable over typed inputs (PeerCtx-equivalent + pattern +
// bounds), reusable from panels, palette forms, library callers, and
// future L3 SDK promotion.
//
// Subscribes via SubscribeAt(peerID, pattern, ...) and reads up to n
// events or until timeout. Returns the collected events as formatted
// lines.
//
// Returns nil events + nil error on timeout with zero matches — the
// caller decides how to surface that (the shell renders a message;
// programmatic callers can branch on len(events) == 0).
func Tail(ap *entitysdk.AppPeer, peerID, pattern string, n int, timeout time.Duration) ([]string, error) {
	if n < 1 {
		return nil, fmt.Errorf("tail: n must be >= 1, got %d", n)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("tail: timeout must be > 0, got %s", timeout)
	}
	sub, err := ap.SubscribeAt(peerID, pattern, entitysdk.SubscribeOpts{})
	if err != nil {
		return nil, fmt.Errorf("tail: subscribe %s: %w", pattern, err)
	}
	defer sub.Close()

	out := make([]string, 0, n)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for len(out) < n {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				return out, nil
			}
			out = append(out, formatTailEvent(ev))
		case <-deadline.C:
			return out, nil
		}
	}
	return out, nil
}

func formatTailEvent(ev entitysdk.ChangeEvent) string {
	switch ev.EventType {
	case entitysdk.ChangeRemove:
		return fmt.Sprintf("remove  %s", ev.Path)
	case entitysdk.ChangePut:
		return fmt.Sprintf("put     %s  %s", ev.Path, ev.NewHash.String()[:12])
	default:
		return fmt.Sprintf("%-6s  %s", ev.EventType, ev.Path)
	}
}
