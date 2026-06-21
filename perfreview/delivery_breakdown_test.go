//go:build perfreview

package perfreview

// Per-delivery cost breakdown — diagnostic for Stage 5 F1/F2 root cause.
//
// Stage 5 saturation probe measured ~900 cross-peer deliveries/sec total
// throughput with single deliveryLoop. This file pins WHERE the time
// goes per delivery so the fix path is concrete:
//
//   1. Raw ed25519 Sign/Verify cost (baseline).
//   2. Per-step time in the cross-peer delivery path:
//      - lookupHandlerGrant + findSignatureFor (delivery.go:81-99 +
//        102-120)
//      - CreateAuthenticatedExecute (helpers.go:70-142)
//      - DispatchLocalEnvelope → RemoteExecute wire path
//
// Categorization for each finding:
//   F1 (1/N degradation) — IMPL bug. Root cause: engine.go:483
//     deliveryLoop is single goroutine. Explicit TODO at
//     engine.go:80-81 ("parallelize deliveryLoop with N workers").
//   F2 (cross-peer 2x lower than local) — IMPL bug (partly) + SYSTEM
//     REALITY (partly). The cacheable findSignatureFor sign is impl;
//     wire serialization + receiver-side verify is reality.
//   F3 (no implicit catch-up) — DESIGN as documented. Substrate is
//     event-driven by spec. Workbench-go consumer gap, NOT arch bug.

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
)

// TestDelivery_Ed25519BaselineCost measures raw ed25519 Sign + Verify
// throughput. These are the per-op costs that the delivery path
// includes; pin them so we know how much of the observed ~1.1ms/delivery
// is crypto vs other work.
func TestDelivery_Ed25519BaselineCost(t *testing.T) {
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	// A typical message is a 33-byte content hash (1 byte algo + 32 byte digest).
	msg := make([]byte, 33)
	if _, err := rand.Read(msg); err != nil {
		t.Fatalf("rand: %v", err)
	}

	const iters = 50_000

	signStart := time.Now()
	var sigs [][]byte
	for i := 0; i < iters; i++ {
		sigs = append(sigs, kp.Sign(msg))
	}
	signDur := time.Since(signStart)

	pub := []byte(kp.PublicKey)
	verifyStart := time.Now()
	for i := 0; i < iters; i++ {
		_ = crypto.Verify(kp.KeyType, pub, msg, sigs[0])
	}
	verifyDur := time.Since(verifyStart)

	signPerOp := signDur / iters
	verifyPerOp := verifyDur / iters

	t.Logf("ed25519 sign:    %d iters in %s  →  %s/op  (%.0f ops/sec)",
		iters, signDur, signPerOp, float64(iters)/signDur.Seconds())
	t.Logf("ed25519 verify:  %d iters in %s  →  %s/op  (%.0f ops/sec)",
		iters, verifyDur, verifyPerOp, float64(iters)/verifyDur.Seconds())

	// Reference: ed25519 on a modern x86 typically does ~70µs sign,
	// ~180µs verify. If we see significantly higher, that's a
	// Go-stdlib-on-this-host data point worth recording.
}

// TestDelivery_FindSignatureForCost measures the cost of the
// findSignatureFor reconstruction pattern. The function deliberately
// re-signs (delivery.go:103) to compute the entity's content hash for
// a store lookup. This sign happens ONCE PER DELIVERY but the
// signature is invariant for the same (target, signer) pair — i.e.,
// it's cacheable.
//
// Counts the per-call cost so we know how much of the per-delivery
// budget is this redundant sign.
func TestDelivery_FindSignatureForCost(t *testing.T) {
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	// Simulate the inner-loop work without going through the
	// subscription package (we're measuring the steady-state cost,
	// not exercising the engine).
	msg := make([]byte, 33)
	if _, err := rand.Read(msg); err != nil {
		t.Fatalf("rand: %v", err)
	}

	const iters = 50_000
	start := time.Now()
	for i := 0; i < iters; i++ {
		// What findSignatureFor does (delivery.go:103):
		//   sig := kp.Sign(targetHash.Bytes())
		// Plus a SignatureData -> Entity round-trip (encoding cost).
		// We model the sign only — the entity construction is the
		// same as the executable path's other entity ops.
		_ = kp.Sign(msg)
	}
	perOp := time.Since(start) / iters

	t.Logf("findSignatureFor inner sign: %d iters → %s/op",
		iters, perOp)
	t.Logf("if cached, savings ≈ %s/delivery (every notif could avoid this)",
		perOp)
}

// TestDelivery_HypotheticalCeilingIfParallelized estimates the theoretical
// throughput ceiling if engine.deliveryLoop ran N workers instead of 1.
//
// Method: drive K parallel goroutines doing the same per-delivery work
// (2 ed25519 signs + ed25519 verify + envelope construction stand-in)
// and measure aggregate throughput. K=1 reproduces the observed
// ~900-1000/s; K=4 should give ~4× if the bottleneck is purely CPU
// crypto (no contention on shared state).
func TestDelivery_HypotheticalCeilingIfParallelized(t *testing.T) {
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	msg := make([]byte, 33)
	if _, err := rand.Read(msg); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pub := []byte(kp.PublicKey)
	sig := kp.Sign(msg)

	// Per-delivery work model: 2 signs (findSignatureFor + envelope) +
	// 1 verify (receiver-side, modeled here on sender for simplicity).
	// Real delivery has more (CBOR encode, dispatch overhead) but
	// crypto dominates per measurement.
	doDelivery := func() {
		_ = kp.Sign(msg)                             // findSignatureFor sign (CACHEABLE)
		_ = kp.Sign(msg)                             // envelope sign (NECESSARY)
		_ = crypto.Verify(kp.KeyType, pub, msg, sig) // receiver verify (NECESSARY)
	}

	for _, k := range []int{1, 2, 4, 8} {
		k := k
		t.Run(fmt.Sprintf("workers=%d", k), func(t *testing.T) {
			const perWorker = 10_000
			start := time.Now()
			done := make(chan struct{}, k)
			for w := 0; w < k; w++ {
				go func() {
					for i := 0; i < perWorker; i++ {
						doDelivery()
					}
					done <- struct{}{}
				}()
			}
			for w := 0; w < k; w++ {
				<-done
			}
			dur := time.Since(start)
			total := perWorker * k
			rate := float64(total) / dur.Seconds()
			t.Logf("workers=%d total=%d dur=%s → %.0f deliveries/sec",
				k, total, dur, rate)
		})
	}
}

// TestDelivery_HypotheticalCeilingWithCachedSignature models what
// happens if findSignatureFor's sign is cached. Per-delivery work
// becomes 1 sign (envelope only) + 1 verify (receiver). Compare to
// the prior test (2 signs + 1 verify) to quantify the savings.
func TestDelivery_HypotheticalCeilingWithCachedSignature(t *testing.T) {
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	msg := make([]byte, 33)
	if _, err := rand.Read(msg); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pub := []byte(kp.PublicKey)
	sig := kp.Sign(msg)

	doDeliveryCached := func() {
		// findSignatureFor → cache hit (map lookup, ~ns)
		_ = kp.Sign(msg)                             // envelope sign (NECESSARY)
		_ = crypto.Verify(kp.KeyType, pub, msg, sig) // receiver verify (NECESSARY)
	}

	for _, k := range []int{1, 2, 4, 8} {
		k := k
		t.Run(fmt.Sprintf("cached_workers=%d", k), func(t *testing.T) {
			const perWorker = 10_000
			start := time.Now()
			done := make(chan struct{}, k)
			for w := 0; w < k; w++ {
				go func() {
					for i := 0; i < perWorker; i++ {
						doDeliveryCached()
					}
					done <- struct{}{}
				}()
			}
			for w := 0; w < k; w++ {
				<-done
			}
			dur := time.Since(start)
			total := perWorker * k
			rate := float64(total) / dur.Seconds()
			t.Logf("cached + workers=%d total=%d dur=%s → %.0f deliveries/sec",
				k, total, dur, rate)
		})
	}
}
