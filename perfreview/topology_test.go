//go:build perfreview

package perfreview

// Topology coverage gaps surfaced by the perfreview
// inventory audit. Pre-existing probes covered hub-and-spoke fan-out
// + restart + partition; this file adds:
//
//   - Full mesh (N↔N symmetric writes) — tests WB-28 territory
//     (reentrant-deadlock surface, closed by core-go Class G
//     multiplexing 5792cdc). Re-probe post-H-G3.
//   - Fan-in (M writers → 1 subscriber) — aggregator pattern.
//   - Slow consumer — artificial handler delay, tests backpressure /
//     queue overflow semantics. F11 drain-catchup is structurally
//     validated only for FAST consumers; slow consumer is the real
//     production case (mobile peer, network lag, GC pause).
//
// All probes write at modest rates (≤2K/sec) and short walltimes
// (≤3s) so each subtest stays under 30s wall.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// meshPeer wraps a peer + per-other-peer subscription state for a
// full-mesh probe. peer[i] subscribes to peer[j]'s `mesh/*` prefix
// for every j != i.
type meshPeer struct {
	name      string
	ap        *entitysdk.AppPeer
	subs      map[string]*entitysdk.Subscription // keyed by remote peer-id
	delivered map[string]*atomic.Int64           // keyed by remote peer-id
	doneCh    chan struct{}
}

// bringUpFullMesh creates N peers, brings each up as a listener,
// connects each pair bidirectionally, and has every peer subscribe
// to every other peer's `mesh/*` prefix.
//
// Drain goroutines: one per (peer, remote-peer) pair, total N*(N-1).
// Each increments the matching delivered counter.
func bringUpFullMesh(t *testing.T, ctx context.Context, dir string, n int) []*meshPeer {
	t.Helper()
	mesh := make([]*meshPeer, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%d", i)
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, name+".db")},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", name, err)
		}
		ready := make(chan struct{})
		listenErr := make(chan error, 1)
		go func() { listenErr <- ap.ListenReady(ctx, ready) }()
		select {
		case <-ready:
		case err := <-listenErr:
			t.Fatalf("%s listen: %v", name, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s listen timeout", name)
		}
		mesh = append(mesh, &meshPeer{
			name:      name,
			ap:        ap,
			subs:      make(map[string]*entitysdk.Subscription),
			delivered: make(map[string]*atomic.Int64),
			doneCh:    make(chan struct{}),
		})
	}

	// Bidirectional connect every pair.
	for i := range mesh {
		for j := range mesh {
			if i == j {
				continue
			}
			if _, err := mesh[i].ap.Connect(ctx, mesh[j].ap.Addr().String()); err != nil {
				t.Fatalf("%s→%s connect: %v", mesh[i].name, mesh[j].name, err)
			}
		}
	}

	// Each peer subscribes to every other peer's mesh/* prefix.
	for i := range mesh {
		for j := range mesh {
			if i == j {
				continue
			}
			remoteID := mesh[j].ap.PeerID()
			sub, err := mesh[i].ap.SubscribeAt(remoteID, "mesh/*", entitysdk.SubscribeOpts{})
			if err != nil {
				t.Fatalf("%s SubscribeAt %s: %v", mesh[i].name, mesh[j].name, err)
			}
			counter := &atomic.Int64{}
			mesh[i].subs[remoteID] = sub
			mesh[i].delivered[remoteID] = counter
			go func(sub *entitysdk.Subscription, counter *atomic.Int64) {
				for range sub.Events() {
					counter.Add(1)
				}
			}(sub, counter)
		}
	}

	return mesh
}

func cleanupFullMesh(mesh []*meshPeer) {
	for _, mp := range mesh {
		for _, sub := range mp.subs {
			_ = sub.Close()
		}
	}
	for _, mp := range mesh {
		mp.ap.Close()
	}
}

// TestTopology_FullMesh_SymmetricWrites probes the N-peer mesh shape
// where every peer writes AND subscribes to every other peer.
// Re-probes WB-28 territory (reentrant-deadlock surface, closed by
// Class G multiplexing). Post-H-G3 we want to confirm no regression.
//
// What we measure:
//   - Aggregate delivery count = N*(N-1) * sent_per_peer (perfect)
//   - Per-peer per-remote delivery percentage
//   - Time-to-quiescence (does the mesh converge?)
//
// What we DON'T do here:
//   - Test conflict resolution on same path (that's stage 4 territory).
//     Each peer writes under its OWN prefix so there's no cross-peer
//     write conflict.
func TestTopology_FullMesh_SymmetricWrites(t *testing.T) {
	for _, n := range []int{3, 4} {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			mesh := bringUpFullMesh(t, ctx, dir, n)
			defer cleanupFullMesh(mesh)

			const writesPerPeer = 200
			const writeRate = 500 // /sec per peer; modest to avoid the engine ceiling
			interval := time.Second / time.Duration(writeRate)

			// Each peer writes writesPerPeer entities under its own `mesh/*` prefix.
			doneWriters := make(chan struct{}, n)
			for i, mp := range mesh {
				i, mp := i, mp
				go func() {
					tick := time.NewTicker(interval)
					defer tick.Stop()
					for k := 0; k < writesPerPeer; k++ {
						<-tick.C
						path := fmt.Sprintf("mesh/%d-%05d", i, k)
						if _, err := mp.ap.Store().Put(path, "perfreview/mesh",
							map[string]interface{}{"writer": i, "k": k}); err != nil {
							t.Errorf("%s Put: %v", mp.name, err)
							return
						}
					}
					doneWriters <- struct{}{}
				}()
			}
			for k := 0; k < n; k++ {
				select {
				case <-doneWriters:
				case <-time.After(15 * time.Second):
					t.Fatalf("writer %d did not finish in 15s", k)
				}
			}

			// 2s drain window.
			time.Sleep(2 * time.Second)

			// Perfect delivery = each peer receives writesPerPeer events
			// from each of the other (n-1) peers.
			expectedPerPair := int64(writesPerPeer)
			totalExpected := int64(n) * int64(n-1) * expectedPerPair
			var totalDelivered int64
			minPct := 100.0
			for i, mp := range mesh {
				peerSum := int64(0)
				for j, other := range mesh {
					if i == j {
						continue
					}
					got := mp.delivered[other.ap.PeerID()].Load()
					peerSum += got
					pct := 100.0 * float64(got) / float64(expectedPerPair)
					if pct < minPct {
						minPct = pct
					}
					t.Logf("%s ← %s: delivered=%d/%d (%.1f%%)",
						mp.name, other.name, got, expectedPerPair, pct)
				}
				totalDelivered += peerSum
			}
			meanPct := 100.0 * float64(totalDelivered) / float64(totalExpected)
			t.Logf("\nn=%d total expected=%d delivered=%d mean-pct=%.1f%% worst-pair-pct=%.1f%%",
				n, totalExpected, totalDelivered, meanPct, minPct)
		})
	}
}

// TestTopology_FanIn_MWritersOneSubscriber probes the aggregator
// pattern: M writer peers each publish on their own `agg/*` tree
// prefix; one central peer subscribes to ALL M trees simultaneously
// and aggregates the deliveries.
//
// Why this is different from hub-and-spoke: in hub-and-spoke ONE
// hub writes and N spokes each open a subscription to the hub.
// Here M writers publish independently, and ONE subscriber holds M
// concurrent cross-peer subscriptions.
//
// Interesting question: does the subscriber's inbox handler serve
// M concurrent deliveries efficiently? Or does it fall back to
// per-subscription serial processing?
//
// Real production case: a logging peer aggregating from M app
// peers; a dashboard peer collecting metrics from M source peers.
func TestTopology_FanIn_MWritersOneSubscriber(t *testing.T) {
	for _, m := range []int{2, 4, 8} {
		m := m
		t.Run(fmt.Sprintf("m=%d", m), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// M writer peers, each listening so the central subscriber
			// can establish cross-peer subscriptions to them.
			writers := make([]*entitysdk.AppPeer, 0, m)
			for k := 0; k < m; k++ {
				name := fmt.Sprintf("writer-%d", k)
				w, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
					ListenAddr: "127.0.0.1:0",
					Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, name+".db")},
					RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
				})
				if err != nil {
					t.Fatalf("CreatePeer %s: %v", name, err)
				}
				wReady := make(chan struct{})
				wErrCh := make(chan error, 1)
				go func() { wErrCh <- w.ListenReady(ctx, wReady) }()
				select {
				case <-wReady:
				case err := <-wErrCh:
					t.Fatalf("%s listen: %v", name, err)
				case <-time.After(5 * time.Second):
					t.Fatalf("%s listen timeout", name)
				}
				writers = append(writers, w)
				defer w.Close()
			}

			// Central subscriber.
			central, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
				ListenAddr: "127.0.0.1:0",
				Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "central.db")},
				RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
			})
			if err != nil {
				t.Fatalf("CreatePeer central: %v", err)
			}
			defer central.Close()
			ready := make(chan struct{})
			listenErr := make(chan error, 1)
			go func() { listenErr <- central.ListenReady(ctx, ready) }()
			select {
			case <-ready:
			case err := <-listenErr:
				t.Fatalf("central listen: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("central listen timeout")
			}

			// Central bidirectionally connects to each writer + opens
			// a cross-peer subscription to each writer's agg/* prefix.
			var delivered atomic.Int64
			subs := make([]*entitysdk.Subscription, 0, m)
			defer func() {
				for _, s := range subs {
					_ = s.Close()
				}
			}()
			for k, w := range writers {
				if _, err := central.Connect(ctx, w.Addr().String()); err != nil {
					t.Fatalf("central→writer-%d connect: %v", k, err)
				}
				if _, err := w.Connect(ctx, central.Addr().String()); err != nil {
					t.Fatalf("writer-%d→central connect: %v", k, err)
				}
				sub, err := central.SubscribeAt(w.PeerID(), "agg/*", entitysdk.SubscribeOpts{})
				if err != nil {
					t.Fatalf("central SubscribeAt writer-%d: %v", k, err)
				}
				subs = append(subs, sub)
				go func(s *entitysdk.Subscription) {
					for range s.Events() {
						delivered.Add(1)
					}
				}(sub)
			}

			// Each writer publishes 200 events on its OWN tree under
			// `agg/{i}`. All writers run concurrently.
			const writesPerWriter = 200
			const writeRate = 500
			interval := time.Second / time.Duration(writeRate)
			doneWriters := make(chan struct{}, m)
			for k, w := range writers {
				k, w := k, w
				go func() {
					tick := time.NewTicker(interval)
					defer tick.Stop()
					for i := 0; i < writesPerWriter; i++ {
						<-tick.C
						path := fmt.Sprintf("agg/%05d", i)
						if _, err := w.Store().Put(path, "perfreview/agg",
							map[string]interface{}{"writer": k, "i": i}); err != nil {
							t.Errorf("writer-%d Put: %v", k, err)
							return
						}
					}
					doneWriters <- struct{}{}
				}()
			}
			for k := 0; k < m; k++ {
				select {
				case <-doneWriters:
				case <-time.After(15 * time.Second):
					t.Fatalf("writer %d did not finish in 15s", k)
				}
			}

			time.Sleep(2 * time.Second)

			expected := int64(m * writesPerWriter)
			got := delivered.Load()
			pct := 100.0 * float64(got) / float64(expected)
			t.Logf("m=%d writers expected=%d delivered=%d (%.1f%%)", m, expected, got, pct)
		})
	}
}

// TestTopology_SlowConsumer probes hub-and-spoke fan-out where ONE
// spoke artificially slows its event processing (sleep per event).
// Tests backpressure semantics: does the slow spoke's queue back
// up, fill, and drop? Or does it block the hub? Or does it pull
// down delivery to the OTHER (fast) spokes?
//
// Post-H-G3 the substrate fully drains for fast consumers. This
// probe asks: what's the boundary when one consumer becomes slow?
func TestTopology_SlowConsumer(t *testing.T) {
	const numSpokes = 4
	const hubRate = 1000 // /sec
	const wallTime = 3 * time.Second
	const slowEventCost = 5 * time.Millisecond // 200 events/sec ceiling per slow spoke

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	hub, spokes := bringUpHubAndSpokes(t, ctx, dir, numSpokes)
	defer cleanupHubAndSpokes(hub, spokes)

	// Replace spoke 0's drain goroutine with a slow one. The
	// bringUpHubAndSpokes default drain reads as fast as possible;
	// we want spoke 0 to read with a per-event sleep.
	//
	// We can't tear down the existing drain goroutine cleanly without
	// touching the harness. Workaround: count the slow-consumer
	// pattern via a SEPARATE channel — we don't need to "be" spoke 0,
	// we just need to ensure spoke 0's drain is slower. The existing
	// drain just increments delivered.Add(1). To inject artificial
	// latency we override after subscription is established: we close
	// spoke 0's current sub, re-subscribe with a slow handler.
	if err := spokes[0].sub.Close(); err != nil {
		t.Fatalf("close spoke0 fast sub: %v", err)
	}
	// Wait for the original drain goroutine to exit (channel close).
	select {
	case <-spokes[0].doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("spoke0 fast drain did not exit")
	}
	// Re-subscribe — slow.
	slowSub, err := spokes[0].ap.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("re-subscribe slow spoke0: %v", err)
	}
	spokes[0].sub = slowSub
	spokes[0].doneCh = make(chan struct{})
	go func(r *spokeReceiver) {
		for range r.sub.Events() {
			time.Sleep(slowEventCost)
			r.delivered.Add(1)
		}
		close(r.doneCh)
	}(spokes[0])

	sent, _ := driveHubWrites(t, hub, hubRate, wallTime)

	time.Sleep(2 * time.Second)

	t.Logf("hub sent=%d at %d/s for %s", sent, hubRate, wallTime)
	for i, s := range spokes {
		d := s.delivered.Load()
		pct := 100.0 * float64(d) / float64(sent)
		tag := "fast"
		if i == 0 {
			tag = "SLOW"
		}
		t.Logf("spoke%d (%s): delivered=%d/%d (%.1f%%)", i+1, tag, d, sent, pct)
	}
}
