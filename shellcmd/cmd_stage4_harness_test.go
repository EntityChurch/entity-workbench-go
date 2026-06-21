package shellcmd_test

// Stage 4 — multi-peer convergent topologies harness.
//
// Generalizes the Stage 3 case-2 bidirectional shape to N peers under
// configurable topologies (mesh, hub-and-spoke, cascade). Each peer runs
// the canonical Stage 3 substrate stack (blob-resolve handler + chain-error
// recorder + localfiles watcher) and wires subscriptions per topology.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// stage4Peer bundles the per-peer state used by the Stage 4 harness.
type stage4Peer struct {
	name   string
	ap     *entitysdk.AppPeer
	fsRoot string
	br     *workbench.BlobResolveHandler
}

// stage4Setup spins up n peers with the standard Stage 3 substrate handler
// set (blob-resolve + chain-errors), each listening on a random local port,
// each with its own temp dir. Uses OpenAccessGrants on every peer. Returns
// the populated peer slice. Caller is responsible for connect + watcher +
// subscribe wiring.
func stage4Setup(t *testing.T, ctx context.Context, n int, names []string, rootName, sourcePrefix string) []stage4Peer {
	return stage4SetupWithGrants(t, ctx, n, names, rootName, sourcePrefix, peer.OpenAccessGrants())
}

// stage4SetupWithGrants is the explicit-grants variant: every peer uses the
// supplied connectionGrants for incoming-connection cap discipline. For the
// canonical mesh use case, the same grant set on every peer is symmetric.
func stage4SetupWithGrants(t *testing.T, ctx context.Context, n int, names []string, rootName, sourcePrefix string, connectionGrants []types.GrantEntry) []stage4Peer {
	t.Helper()
	if len(names) != n {
		t.Fatalf("stage4Setup: names slice (%d) must match n (%d)", len(names), n)
	}
	peers := make([]stage4Peer, n)
	for i := 0; i < n; i++ {
		dir := t.TempDir()
		br := workbench.NewBlobResolveHandler()
		br.RegisterMount(sourcePrefix, sourcePrefix)
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			RawOptions: []peer.Option{peer.WithConnectionGrants(connectionGrants)},
			Handlers: []entitysdk.HandlerRegistration{
				{Pattern: workbench.BlobResolvePattern, Handler: br},
				{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
			},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", names[i], err)
		}
		t.Cleanup(func() { _ = ap.Close() })
		bringUpListener(t, ctx, ap, names[i])
		peers[i] = stage4Peer{name: names[i], ap: ap, fsRoot: dir, br: br}
	}
	return peers
}

// stage4ConnectAllToAll dials every (i,j) pair so any peer can route to any
// other. Idempotent on the underlying connection pool.
func stage4ConnectAllToAll(t *testing.T, ctx context.Context, peers []stage4Peer) {
	t.Helper()
	for i := range peers {
		for j := range peers {
			if i == j {
				continue
			}
			if _, err := peers[i].ap.Connect(ctx, peers[j].ap.Addr().String()); err != nil {
				t.Fatalf("%s→%s connect: %v", peers[i].name, peers[j].name, err)
			}
		}
	}
}

// stage4StartWatchers mounts the localfiles root + starts the watcher on
// each peer, so disk writes propagate into the tree at sourcePrefix.
func stage4StartWatchers(t *testing.T, ctx context.Context, peers []stage4Peer, rootName, sourcePrefix string) {
	t.Helper()
	for i := range peers {
		p := peers[i]
		lf := p.ap.LocalFilesHandler()
		if lf == nil {
			t.Fatalf("%s local/files handler not wired", p.name)
		}
		if err := lf.AddRoot(rootName, localfiles.RootConfigData{
			Prefix:         sourcePrefix,
			FilesystemRoot: p.fsRoot,
		}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
			t.Fatalf("%s AddRoot: %v", p.name, err)
		}
		if err := lf.StartWatching(ctx, rootName, p.ap.RawContentStore(),
			p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
			t.Fatalf("%s StartWatching: %v", p.name, err)
		}
	}
}

// stage4MintChainCap mints the receive-side chain cap on a peer, scoped to
// the blob-resolve handler. Caller chooses the bind path (typically per-root).
func stage4MintChainCap(t *testing.T, p stage4Peer, rootName string) {
	t.Helper()
	chainGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := p.ap.MintChainCapabilityBound(chainGrants,
		"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
		t.Fatalf("%s mint chain cap: %v", p.name, err)
	}
}

// stage4Subscribe wires subscriber as a subscriber to source's sourcePrefix.
// One call per (subscriber, source) directed edge in the topology graph.
func stage4Subscribe(t *testing.T, subscriber, source stage4Peer, sourcePrefix string) {
	t.Helper()
	deliverURI := fmt.Sprintf("entity://%s/%s", subscriber.ap.PeerID(), workbench.BlobResolvePattern)
	if _, err := subscriber.ap.SubscribeRawAt(source.ap.PeerID(), sourcePrefix+"*",
		deliverURI, "receive", entitysdk.SubscribeOpts{
			Events:         []string{"created", "updated"},
			IncludePayload: true,
		}); err != nil {
		t.Fatalf("%s subscribe to %s: %v", subscriber.name, source.name, err)
	}
}

// stage4WireMesh: every peer subscribes to every other peer. Mints chain caps
// on each peer first.
func stage4WireMesh(t *testing.T, peers []stage4Peer, rootName, sourcePrefix string) {
	t.Helper()
	for i := range peers {
		stage4MintChainCap(t, peers[i], rootName)
	}
	for i := range peers {
		for j := range peers {
			if i == j {
				continue
			}
			stage4Subscribe(t, peers[i], peers[j], sourcePrefix)
		}
	}
}

// stage4WireHubSpoke: spokes subscribe to hub. Hub does not subscribe to
// anyone (it's a publisher). Pure fan-out shape.
func stage4WireHubSpoke(t *testing.T, peers []stage4Peer, hubIdx int, rootName, sourcePrefix string) {
	t.Helper()
	for i := range peers {
		if i == hubIdx {
			continue
		}
		stage4MintChainCap(t, peers[i], rootName)
		stage4Subscribe(t, peers[i], peers[hubIdx], sourcePrefix)
	}
}

// stage4WireCascade: peer[i+1] subscribes to peer[i]. Forms a chain A→B→C…
// Mints chain caps on all but the head (head doesn't receive).
func stage4WireCascade(t *testing.T, peers []stage4Peer, rootName, sourcePrefix string) {
	t.Helper()
	for i := 1; i < len(peers); i++ {
		stage4MintChainCap(t, peers[i], rootName)
		stage4Subscribe(t, peers[i], peers[i-1], sourcePrefix)
	}
}

// stage4WireRing: peer[i] subscribes to peer[(i-1+N) mod N]. Closes the
// cascade into a ring; every peer is both subscriber and source. Mints
// chain caps on every peer.
func stage4WireRing(t *testing.T, peers []stage4Peer, rootName, sourcePrefix string) {
	t.Helper()
	n := len(peers)
	for i := 0; i < n; i++ {
		stage4MintChainCap(t, peers[i], rootName)
	}
	for i := 0; i < n; i++ {
		predecessor := peers[(i-1+n)%n]
		stage4Subscribe(t, peers[i], predecessor, sourcePrefix)
	}
}

// stage4AwaitFile polls until name exists on the peer's filesystem at fsRoot,
// or the deadline elapses. Returns true on success.
func stage4AwaitFile(p stage4Peer, name string, deadline time.Time) bool {
	target := filepath.Join(p.fsRoot, name)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(target); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, err := os.Stat(target)
	return err == nil
}

// stage4DumpDiagnostics surfaces per-peer subscription + tree + chain-error
// state. Called when convergence check fails.
func stage4DumpDiagnostics(t *testing.T, peers []stage4Peer, sourcePrefix string) {
	t.Helper()
	for _, p := range peers {
		errPaths := listPrefix(p.ap, "system/runtime/chain-errors/")
		t.Logf("%s chain-error markers: %d", p.name, len(errPaths))
		for _, ep := range errPaths {
			t.Logf("  %s: %s", p.name, ep)
		}
		subs := listPrefix(p.ap, "system/subscription/")
		t.Logf("%s subscriptions: %d", p.name, len(subs))
		files := listPrefix(p.ap, sourcePrefix)
		t.Logf("%s tree entries under %s: %d", p.name, sourcePrefix, len(files))
		entries, _ := os.ReadDir(p.fsRoot)
		t.Logf("%s filesystem dir %s: %d entries", p.name, p.fsRoot, len(entries))
		for _, e := range entries {
			t.Logf("  %s: %s", p.name, e.Name())
		}
	}
}

// stage4AssertNoChainErrors fails the test if any peer has chain-error
// markers. Call after convergence.
func stage4AssertNoChainErrors(t *testing.T, peers []stage4Peer) {
	t.Helper()
	for _, p := range peers {
		errPaths := listPrefix(p.ap, "system/runtime/chain-errors/")
		if len(errPaths) > 0 {
			t.Errorf("%s has %d chain-error markers (want 0)", p.name, len(errPaths))
			for _, ep := range errPaths {
				t.Logf("  %s: %s", p.name, ep)
			}
		}
	}
}

// stage4AssertEntityCountBound checks that each peer has at most maxCount
// entities under sourcePrefix. Used to detect runaway re-binding under
// bidirectional/multi-peer setups (loop-prevention check).
func stage4AssertEntityCountBound(t *testing.T, peers []stage4Peer, sourcePrefix string, maxCount int) {
	t.Helper()
	for _, p := range peers {
		got := len(listPrefix(p.ap, sourcePrefix))
		if got > maxCount {
			t.Errorf("%s has %d entities under %s, want ≤ %d (possible runaway loop)",
				p.name, got, sourcePrefix, maxCount)
		}
	}
}
