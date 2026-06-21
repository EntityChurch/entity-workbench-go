//go:build perfreview

package perfreview

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestDisabledExtensions_Matrix turns each toggleable extension off
// one at a time, runs the baseline workload, and reports the cost
// delta. Establishes "what does each extension cost when wired but
// idle?" — the dispatch-loop overhead an operator pays even when the
// feature isn't actively used.
//
// Each toggle takes the same workload as the scale baseline at a
// modest N (50K) and reports write latency stats + bootstrap entity
// count + heap.
//
// Investigation 15 of PRODUCTION-READINESS-REVIEW.
func TestDisabledExtensions_Matrix(t *testing.T) {
	type tweak struct {
		name string
		mod  func(c *entitysdk.ExtensionsConfig)
	}
	cases := []tweak{
		{"all-default", func(c *entitysdk.ExtensionsConfig) {}},
		{"no-history", func(c *entitysdk.ExtensionsConfig) {
			c.History = &entitysdk.HistoryConfig{Disabled: true}
		}},
		{"no-clock", func(c *entitysdk.ExtensionsConfig) {
			c.Clock = &entitysdk.ClockConfig{Disabled: true}
		}},
		{"no-continuation", func(c *entitysdk.ExtensionsConfig) {
			c.Continuation = &entitysdk.ContinuationConfig{Disabled: true}
		}},
		{"no-subscription", func(c *entitysdk.ExtensionsConfig) {
			c.Subscription = &entitysdk.SubscriptionConfig{Disabled: true}
		}},
		{"no-revision", func(c *entitysdk.ExtensionsConfig) {
			c.Revision = &entitysdk.RevisionConfig{Disabled: true}
		}},
		{"no-compute", func(c *entitysdk.ExtensionsConfig) {
			c.Compute = &entitysdk.ComputeConfig{Disabled: true}
		}},
		{"no-identity-stack", func(c *entitysdk.ExtensionsConfig) {
			c.IdentityStack = &entitysdk.IdentityStackConfig{Disabled: true}
		}},
		{"no-role", func(c *entitysdk.ExtensionsConfig) {
			c.Role = &entitysdk.RoleConfig{Disabled: true}
		}},
		{"minimal", func(c *entitysdk.ExtensionsConfig) {
			// Strip every toggleable extension.
			c.History = &entitysdk.HistoryConfig{Disabled: true}
			c.Clock = &entitysdk.ClockConfig{Disabled: true}
			c.Continuation = &entitysdk.ContinuationConfig{Disabled: true}
			c.Subscription = &entitysdk.SubscriptionConfig{Disabled: true}
			c.Revision = &entitysdk.RevisionConfig{Disabled: true}
			c.Compute = &entitysdk.ComputeConfig{Disabled: true}
			c.IdentityStack = &entitysdk.IdentityStackConfig{Disabled: true}
			c.Role = &entitysdk.RoleConfig{Disabled: true}
		}},
	}

	type result struct {
		Name             string
		Bootstrap        int
		BootstrapHeapMiB float64
		AfterEntities    int
		AfterHeapMiB     float64
		WriteDur         time.Duration
		P50, P95, P99    time.Duration
		Goroutines       int
	}
	rows := make([]result, 0, len(cases))

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "peer.db")
			t.Setenv("HOME", filepath.Join(dir, "home"))
			os.MkdirAll(filepath.Join(dir, "home"), 0o755)

			cfg := entitysdk.PeerConfig{
				Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
			}
			c.mod(&cfg.Extensions)

			ap, err := entitysdk.CreatePeer(cfg)
			if err != nil {
				t.Fatalf("CreatePeer (%s): %v", c.name, err)
			}
			defer ap.Close()

			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			bootstrapHeap := float64(ms.HeapInuse) / 1024 / 1024

			// Bootstrap entity count via direct sqlite query — same as
			// the harness uses.
			bootstrapEntities := countEntities(t, dbPath)

			const N = 50_000
			storeAPI := ap.Store()
			latencies := make([]time.Duration, 0, N)
			t0 := time.Now()
			for i := 0; i < N; i++ {
				path := fmt.Sprintf("bench/%07d", i)
				start := time.Now()
				if _, err := storeAPI.Put(path, "perfreview/entity",
					map[string]interface{}{"tick": i, "time": "x"}); err != nil {
					t.Fatalf("Put: %v", err)
				}
				latencies = append(latencies, time.Since(start))
			}
			writeDur := time.Since(t0)

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

			runtime.GC()
			runtime.ReadMemStats(&ms)

			rows = append(rows, result{
				Name:             c.name,
				Bootstrap:        bootstrapEntities,
				BootstrapHeapMiB: bootstrapHeap,
				AfterEntities:    countEntities(t, dbPath),
				AfterHeapMiB:     float64(ms.HeapInuse) / 1024 / 1024,
				WriteDur:         writeDur,
				P50:              latencies[len(latencies)*50/100],
				P95:              latencies[len(latencies)*95/100],
				P99:              latencies[len(latencies)*99/100],
				Goroutines:       runtime.NumGoroutine(),
			})
		})
	}

	t.Logf("\n%-22s %12s %12s %12s %12s %10s %10s %10s %10s %5s",
		"config", "boot-ent", "boot-MiB", "after-ent", "after-MiB", "wr-dur", "p50", "p95", "p99", "gor")
	for _, r := range rows {
		t.Logf("%-22s %12d %12.1f %12d %12.1f %10s %10s %10s %10s %5d",
			r.Name, r.Bootstrap, r.BootstrapHeapMiB, r.AfterEntities, r.AfterHeapMiB,
			short(r.WriteDur), short(r.P50), short(r.P95), short(r.P99), r.Goroutines)
	}
}
