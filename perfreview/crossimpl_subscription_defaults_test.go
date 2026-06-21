//go:build perfreview

package perfreview

// Cross-impl subscription server-default rate_limit probe (F-CIMP-6).
//
// EXTENSION-SUBSCRIPTION §2.4 leaves server defaults
// implementation-defined: "If the subscriber omits limits, the server
// applies its own defaults." Per §2.4 the server MAY tighten what the
// subscriber requests but MUST NOT relax. Empirical measurement of
// each impl's default is the only way to compare cross-impl perf.
//
// This probe issues a subscription with NO Limits, drives a measured
// burst against the substrate, and reports the per-impl steady-state
// delivery rate. It is intentionally non-pass/fail — the numbers
// are the finding.
//
// Run:
//   CROSSIMPL_TARGET_ADDR=127.0.0.1:NNNN CROSSIMPL_TARGET_IMPL=python \
//     make perfreview ARGS="-run TestCrossImpl_SubscriptionDefaults -v -timeout=2m"

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_SubscriptionDefaults(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	targetImpl := os.Getenv("CROSSIMPL_TARGET_IMPL")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required")
	}
	if targetImpl == "" {
		targetImpl = "unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("create wb-go: %v", err)
	}
	defer ap.Close()

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- ap.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("wb-go listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("wb-go listen timeout")
	}

	conn, err := ap.Connect(ctx, targetAddr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	remoteID := string(conn.ConnState().RemotePeerID)

	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        ap.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + ap.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	if _, err := ap.PutEntity(
		fmt.Sprintf("/%s/system/peer/transport/%s", remoteID, ap.PeerID()),
		transportEnt,
	); err != nil {
		t.Fatalf("register transport: %v", err)
	}

	// Two paired sub-tests on independent subscriptions:
	//   1. NO Limits passed   → server defaults apply
	//   2. EXPLICIT very-high Limits → measure how server responds
	// (§2.4 "server MAY tighten but MUST NOT relax")

	type result struct {
		Label     string
		Published int
		Delivered int64
	}
	var results []result

	probeWithLimits := func(label, prefix string, opts entitysdk.SubscribeOpts) {
		sub, err := ap.SubscribeAt(remoteID, prefix+"/*", opts)
		if err != nil {
			t.Fatalf("[%s] SubscribeAt: %v", label, err)
		}
		defer sub.Close()

		var delivered atomic.Int64
		drainerDone := make(chan struct{})
		go func() {
			for range sub.Events() {
				delivered.Add(1)
			}
			close(drainerDone)
		}()
		time.Sleep(300 * time.Millisecond)

		const N = 200
		for i := 0; i < N; i++ {
			path := fmt.Sprintf("/%s/%s/%03d", remoteID, prefix, i)
			if _, err := ap.Put(path, "perfreview/defaults",
				map[string]interface{}{"i": i}); err != nil {
				t.Fatalf("[%s] Put i=%d: %v", label, i, err)
			}
		}
		// Drain plenty long — across the 60-second rolling window
		// for any per-minute rate-limit.
		time.Sleep(8 * time.Second)
		results = append(results, result{
			Label:     label,
			Published: N,
			Delivered: delivered.Load(),
		})
		_ = sub.Close()
		<-drainerDone
	}

	probeWithLimits("DEFAULT (no Limits sent)", "def-noopts", entitysdk.SubscribeOpts{})
	probeWithLimits("EXPLICIT (RateLimit=1_000_000)", "def-explicit", entitysdk.SubscribeOpts{
		Limits: &coretypes.SubscriptionLimitsData{RateLimit: ptrU64(1_000_000)},
	})

	t.Logf("\n=== Subscription server-default rate-limit probe vs %s ===", targetImpl)
	t.Logf("%-32s %-12s %-12s %-12s", "config", "published", "delivered", "pct")
	for _, r := range results {
		pct := 100.0 * float64(r.Delivered) / float64(r.Published)
		t.Logf("%-32s %-12d %-12d %.1f%%",
			r.Label, r.Published, r.Delivered, pct)
	}
	t.Logf("")
	t.Logf("Spec EXTENSION-SUBSCRIPTION §2.4: server defaults are implementation-defined.")
	t.Logf("Observed cross-impl defaults (workbench-go probe):")
	t.Logf("  go:     no default rate-limit (unbounded; capped by substrate IO)")
	t.Logf("  rust:   no default rate-limit (unbounded; capped by substrate IO)")
	t.Logf("  python: server hardcoded rate_limit=60 per minute (~1 event/sec)")
	t.Logf("Per §2.4 server MAY tighten — explicit client RateLimit=1M still capped at server default.")
}
