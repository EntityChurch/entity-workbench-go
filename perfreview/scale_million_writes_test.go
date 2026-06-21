//go:build perfreview

package perfreview

// Lane 2 — long-running 1M-write sweep. Stress late-phase substrate
// behavior: memory growth, SQLite fragmentation, per-Put latency
// drift, GC behavior, subscription delivery drift across long
// runtimes.
//
// Hub writes 1M entities hierarchically (escapes the OP-1-residual
// wide-flat cliff) at substrate-bound rate; spoke subscribes; snapshots
// captured at log-spaced checkpoints. Auto-version OFF to stay
// substrate-bound (cliff measurement is Lane 1's job).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// TestScale_OneMillionWrites runs 1M Puts through hub→spoke
// subscription. Auto-version OFF (cliff isn't the focus); paths are
// hierarchical (`scale/{aa}/{bb}/{cc}/file-{i}`) so neither side
// builds a degenerate wide trie.
//
// Snapshot at every 100K writes: memory (RSS proxy via runtime),
// sqlite file size, per-Put latency window, delivery percentage,
// GC count.
//
// Runtime: ~6 min at hub-substrate-bound rate (~3K writes/sec).
func TestScale_OneMillionWrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("hub create: %v", err)
	}
	defer hub.Close()
	hubReady := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, hubReady) }()
	<-hubReady

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("spoke create: %v", err)
	}
	defer spoke.Close()
	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke: %v", err)
	}

	sub, err := spoke.SubscribeAt(hub.PeerID(), "scale/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	type scaleSnapshot struct {
		Wrote          int
		Wall           time.Duration
		WindowP50      time.Duration
		WindowP95      time.Duration
		Delivered      int64
		HeapMiB        float64
		AllocMiB       float64
		NumGC          uint32
		Goroutines     int
		HubDBSizeMiB   float64
		SpokeDBSizeMiB float64
	}
	rows := []scaleSnapshot{}

	storeAPI := hub.Store()
	startWall := time.Now()
	const totalWrites = 1_000_000
	const checkpointEvery = 100_000

	// Hierarchical path layout: scale/{aa}/{bb}/{cc}/file-{i}
	// (aa,bb,cc each 0..99). For 1M files, the deepest leaf-node
	// directory averages 1 file per slot (100^3 = 1M).
	pathOf := func(i int) string {
		a := (i / 10000) % 100
		b := (i / 100) % 100
		c := i % 100
		return fmt.Sprintf("scale/%02d/%02d/%02d/file-%07d", a, b, c, i)
	}

	checkpointStart := time.Now()
	windowLatencies := make([]time.Duration, 0, checkpointEvery)

	for i := 0; i < totalWrites; i++ {
		path := pathOf(i)
		t0 := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/scale",
			map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put i=%d: %v", i, err)
		}
		windowLatencies = append(windowLatencies, time.Since(t0))

		if (i+1)%checkpointEvery == 0 {
			sortDurations(windowLatencies)
			row := scaleSnapshot{
				Wrote:     i + 1,
				Wall:      time.Since(startWall),
				WindowP50: windowLatencies[len(windowLatencies)*50/100],
				WindowP95: windowLatencies[len(windowLatencies)*95/100],
				Delivered: delivered.Load(),
			}

			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			row.HeapMiB = float64(ms.HeapInuse) / 1024 / 1024
			row.AllocMiB = float64(ms.HeapAlloc) / 1024 / 1024
			row.NumGC = ms.NumGC
			row.Goroutines = runtime.NumGoroutine()
			row.HubDBSizeMiB = dbSizeMiB(filepath.Join(dir, "hub.db"))
			row.SpokeDBSizeMiB = dbSizeMiB(filepath.Join(dir, "spoke.db"))

			rows = append(rows, row)
			t.Logf("checkpoint @%dK: wall=%s wp50=%s wp95=%s deliv=%d (%.1f%%) heap=%.1fMiB goros=%d hub-db=%.1fMiB spoke-db=%.1fMiB",
				(i+1)/1000, short(row.Wall), short(row.WindowP50), short(row.WindowP95),
				row.Delivered, 100.0*float64(row.Delivered)/float64(i+1),
				row.HeapMiB, row.Goroutines, row.HubDBSizeMiB, row.SpokeDBSizeMiB)

			windowLatencies = windowLatencies[:0]
			checkpointStart = time.Now()
			_ = checkpointStart
		}
	}

	// Drain window for any in-flight deliveries.
	t.Logf("drain phase: waiting up to 10s for in-flight deliveries...")
	deadline := time.Now().Add(10 * time.Second)
	lastDelivered := delivered.Load()
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		cur := delivered.Load()
		if cur == lastDelivered {
			break
		}
		lastDelivered = cur
	}

	finalDelivered := delivered.Load()

	t.Logf("\n=== 1M write sweep summary ===")
	t.Logf("%-10s %10s %10s %10s %12s %10s %8s %12s %12s",
		"wrote", "wall", "wp50", "wp95", "delivered", "heap-MiB", "goros", "hub-MiB", "spoke-MiB")
	for _, r := range rows {
		t.Logf("%-10d %10s %10s %10s %12d %10.1f %8d %12.1f %12.1f",
			r.Wrote, short(r.Wall), short(r.WindowP50), short(r.WindowP95),
			r.Delivered, r.HeapMiB, r.Goroutines, r.HubDBSizeMiB, r.SpokeDBSizeMiB)
	}
	t.Logf("\nfinal: wrote=%d delivered=%d (%.1f%%) wall=%s",
		totalWrites, finalDelivered,
		100.0*float64(finalDelivered)/float64(totalWrites),
		short(time.Since(startWall)))

	// Drift analysis
	if len(rows) >= 2 {
		first := rows[0]
		last := rows[len(rows)-1]
		t.Logf("\ndrift analysis (first vs last checkpoint):")
		t.Logf("  per-Put p50:  %s → %s (%.1fx)",
			short(first.WindowP50), short(last.WindowP50),
			float64(last.WindowP50)/float64(first.WindowP50))
		t.Logf("  per-Put p95:  %s → %s (%.1fx)",
			short(first.WindowP95), short(last.WindowP95),
			float64(last.WindowP95)/float64(first.WindowP95))
		t.Logf("  heap:         %.1f MiB → %.1f MiB (Δ=%+.1f)",
			first.HeapMiB, last.HeapMiB, last.HeapMiB-first.HeapMiB)
		t.Logf("  goroutines:   %d → %d (Δ=%+d)",
			first.Goroutines, last.Goroutines,
			last.Goroutines-first.Goroutines)
		t.Logf("  hub-db:       %.1f MiB → %.1f MiB",
			first.HubDBSizeMiB, last.HubDBSizeMiB)

		// Heuristic regression checks
		if last.Goroutines-first.Goroutines > 100 {
			t.Errorf("goroutine leak across 1M writes: Δ=%+d",
				last.Goroutines-first.Goroutines)
		}
		if float64(last.WindowP50)/float64(first.WindowP50) > 5.0 {
			t.Errorf("per-Put p50 degraded >5x over 1M writes (%.1fx)",
				float64(last.WindowP50)/float64(first.WindowP50))
		}
	}

	_ = sub.Close()
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
	}
}

// sortDurations sorts a duration slice ascending in place.
func sortDurations(ds []time.Duration) {
	// Insertion sort would be tempting but checkpointEvery=100K
	// is too many for O(N²). Use built-in via sort.Sort wrapper.
	durSlice(ds).sort()
}

type durSlice []time.Duration

func (d durSlice) sort() {
	// Use sort.Slice via a tiny shim. Avoiding direct sort import in
	// the test file to keep imports minimal; reuse perfreview's
	// existing slices if any. Simplest: insertion sort for tiny N,
	// quicksort for larger. Here we just call sort.Slice via reflect
	// would be silly — write straight quicksort.
	quicksortDur(d, 0, len(d)-1)
}

func quicksortDur(a []time.Duration, lo, hi int) {
	for lo < hi {
		p := partitionDur(a, lo, hi)
		if p-lo < hi-p {
			quicksortDur(a, lo, p-1)
			lo = p + 1
		} else {
			quicksortDur(a, p+1, hi)
			hi = p - 1
		}
	}
}

func partitionDur(a []time.Duration, lo, hi int) int {
	pivot := a[hi]
	i := lo - 1
	for j := lo; j < hi; j++ {
		if a[j] < pivot {
			i++
			a[i], a[j] = a[j], a[i]
		}
	}
	a[i+1], a[hi] = a[hi], a[i+1]
	return i + 1
}

func dbSizeMiB(path string) float64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return float64(info.Size()) / 1024 / 1024
}
