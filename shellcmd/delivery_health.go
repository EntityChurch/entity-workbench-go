package shellcmd

import "fmt"

// delivery_health.go — the saturation counter that makes a silent stall
// audible.
//
// # The failure this exists for
//
// Measured 2026-09-07, `shellboot/bigcopy_load_test.go`: two peers, one
// shared folder, 2000 files × 4 KiB dropped into it at once. The sender's
// watcher ingested all 2000 and bound all 2000. **676 reached the
// receiver.** No error on either side, no chain error, `syncs` listing
// healthy throughout, and the copy simply stopped — the operator-reported
// shape of "I turned on my share, copied a big directory over, and it
// crapped out".
//
// The cause is not ours and is not a bug: the subscription engine's
// delivery shards are bounded and DROP on full, deliberately, to avoid a
// deadlock (`ext/subscription/engine.go`, OnTreeChange — `default:
// e.droppedDeliveries.Add(1)`). The kernel counts every drop and exposes
// it precisely so an operator can see saturation.
//
// **Nothing in this repo had ever read that counter.** Not in shipped
// code, not in a test. So the one number that explains the failure was
// being maintained, for us, and thrown away — D20 aimed at the kernel
// again.
//
// # Where the drops happen, which is the part that surprises
//
// On the **sender**. Measured: `sender dropped=2327, receiver dropped=0`.
// The publisher discards notifications it never put on the wire, so the
// receiver is not "behind" — it was never told. That is why neither side
// reports anything: there is no failed delivery to log, no retry to
// exhaust, and no chain error to collect. A receiver cannot detect this
// on its own; it can only be told, or re-derive it by asking what the
// sender actually has (which is what a catch-up pass does).
//
// # Why a counter and not a fix
//
// Raising the shard capacity moves the cliff without removing it — any
// bound can be exceeded by a fast enough directory copy, and the failure
// mode at the new bound is identical and equally silent. The durable
// answer is catch-up (`resync` recovers a stalled 2000-file copy in
// ~700 ms, and a pass with nothing to do costs ~470 ms / 0.24 ms per
// file). This type is what makes the condition observable in the
// meantime, and what a catch-up supervisor triggers on.

// DeliveryHealth is this peer's subscription-delivery saturation.
//
// Peer-level and not per-folder on purpose: the counter is a property of
// the engine, shared by every subscription on the peer, and the engine
// does not attribute a drop to a subscription. Reporting it per folder
// would invent an attribution the substrate does not have — and the
// invented one would be confidently wrong whenever two folders are busy
// at once.
type DeliveryHealth struct {
	// Dropped is the engine's lifetime count of notifications discarded
	// because a delivery shard was full. Monotonic within a process and
	// reset by a restart — it is a process counter, not a tree entity, so
	// it says nothing about drops in an earlier run.
	Dropped uint64

	// QueueDepth is the current total depth across all delivery shards.
	// Approaching capacity means drops are imminent; zero after a burst
	// means the queue drained and whatever was lost is already lost.
	QueueDepth int

	// Available is false when the peer exposes no subscription engine, so
	// a caller can tell "no drops" from "not measured". Rendering the
	// second as the first is the failure this whole file is about.
	Available bool
}

// Saturated reports whether this peer has ever dropped a delivery.
func (d DeliveryHealth) Saturated() bool { return d.Available && d.Dropped > 0 }

// Summary is one operator-facing line, or "" when there is nothing to
// say. Empty rather than "0 dropped" because a healthy peer should not
// spend a line of a status table on a counter that is doing nothing.
func (d DeliveryHealth) Summary() string {
	if !d.Available {
		return ""
	}
	if d.Dropped == 0 {
		if d.QueueDepth > 0 {
			return fmt.Sprintf("delivery queue depth %d", d.QueueDepth)
		}
		return ""
	}
	return fmt.Sprintf(
		"%d subscription notification(s) DROPPED on this peer since start — "+
			"a burst of changes outran delivery, so peers subscribed to this "+
			"peer's folders may be missing files. They recover with `resync`; "+
			"nothing is lost from disk", d.Dropped)
}

// DeliveryHealthOf reads the counters from a workspace's local peer.
func (ws *ShellWorkspace) DeliveryHealth() DeliveryHealth {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return DeliveryHealth{}
	}
	eng := ws.Local.Peer.SubscriptionEngine()
	if eng == nil {
		return DeliveryHealth{}
	}
	return DeliveryHealth{
		Dropped:    eng.DroppedDeliveries(),
		QueueDepth: eng.DeliveryQueueDepth(),
		Available:  true,
	}
}

// noteSaturation records saturation on an outcome — the ONE place either
// entry point learns about it.
//
// Shared for observeDevice/observeFolder's reason (reconcile.go): a read
// and a pass must not be able to describe the same condition, or the same
// fault, differently. A second copy of this sentence would drift the
// moment one of them was improved.
func (ws *ShellWorkspace) noteSaturation(out *ReconcileOutcome) {
	out.Delivery = ws.DeliveryHealth()
	if line := out.Delivery.Summary(); line != "" && out.Delivery.Saturated() {
		out.Problems = append(out.Problems, line)
	}

	// A DROP AND A REFUSAL ARE DIFFERENT FAILURES AND ONLY ONE OF THEM
	// HAD A SURFACE.
	//
	// Saturation above is the SENDER's queue overflowing — measured, real,
	// and not what an operator usually hits. A delivery that reaches us
	// and is then refused at the last hop (`403 capability_denied` on the
	// blob fetch) moves no counter here at all: it binds a chain-error
	// marker into our own tree and, until 2026-09-10, was read by nothing
	// in the product. That is the state an operator spent a morning in,
	// with every panel reporting healthy.
	//
	// Both, always, and never one: a peer with an empty queue and four
	// hundred refusals is exactly as broken as a saturated one, and the
	// old code could only see the second kind.
	if ws.Local != nil && ws.Local.Peer != nil {
		if lines := DeliveryFailures(ws.Local.Peer.Store()).Problems(); len(lines) > 0 {
			out.Problems = append(out.Problems, lines...)
		}
	}
}
