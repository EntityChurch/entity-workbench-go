//go:build perfreview

package perfreview

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestPayload_SizeSweep measures the cost of writing entities at
// payload sizes ranging from tiny (100B) to large (1MB). CBOR encoding
// + SQLite blob storage + content-hash compute should all scale linearly
// with payload size; this test verifies that empirically.
//
// What this answers: if a user mounts a directory of 1MB markdown files
// instead of tiny test payloads, does anything go quadratic?
//
// Investigation 8 of PRODUCTION-READINESS-REVIEW.
func TestPayload_SizeSweep(t *testing.T) {
	type spec struct {
		Name string
		Size int // bytes of filler content
		N    int // entity count
	}
	cases := []spec{
		{"100B", 100, 10_000},
		{"1KiB", 1024, 10_000},
		{"10KiB", 10 * 1024, 5_000},
		{"100KiB", 100 * 1024, 1_000},
		{"1MiB", 1024 * 1024, 100},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			filler := strings.Repeat("x", c.Size)
			storeAPI := h.Peer().Store()
			latencies := make([]time.Duration, 0, c.N)

			t0 := time.Now()
			for i := 0; i < c.N; i++ {
				path := fmt.Sprintf("payload/%07d", i)
				payload := map[string]interface{}{
					"tick":    i,
					"content": filler,
				}
				start := time.Now()
				if _, err := storeAPI.Put(path, "perfreview/entity", payload); err != nil {
					t.Fatalf("Put: %v", err)
				}
				latencies = append(latencies, time.Since(start))
			}
			totalDur := time.Since(t0)

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			p50 := latencies[len(latencies)*50/100]
			p99 := latencies[len(latencies)*99/100]

			m := h.Snapshot(c.Name, c.N, totalDur, p50, 0, p99)

			bytesPerSec := float64(c.N) * float64(c.Size) / totalDur.Seconds() / 1024 / 1024
			t.Logf("\npayload=%s N=%d dur=%s p50=%s p99=%s throughput=%.1fMiB/s db=%.1fMiB heap=%.1fMiB",
				c.Name, c.N, short(totalDur), short(p50), short(p99),
				bytesPerSec,
				float64(m.SQLiteBytes)/1024/1024,
				float64(m.HeapInUseBytes)/1024/1024)
		})
	}
}
