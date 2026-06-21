package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.entitychurch.org/entity-core-go/core/store"

	"entity-workbench-go/shellboot"
	wb "entity-workbench-go/workbench"
)

const usage = `Usage:
  entity-console [flags]

Flags:
  -identity NAME      Use named identity from ~/.entity/identities/
                      (default: ephemeral keypair, lost on exit)
  -alias NAME         Alias for the in-process peer in the shell
                      (default: -identity name, or "self")
  -storage KIND       Storage backend: "memory" (default) or "sqlite"
  -storage-path PATH  SQLite DB path. When -storage=sqlite and
                      -identity NAME is set, defaults to
                      ~/.entity/peers/NAME/store.db.
  -listen ADDR        TCP listener for inbound peer connections.
                      Empty (default) = no inbound listener.
  -open-access        DEV: grant wildcard caps to connecting peers.
`

func main() {
	identity := flag.String("identity", "", "identity name (default: ephemeral)")
	alias := flag.String("alias", "", "alias for the in-process peer")
	storage := flag.String("storage", "", "storage backend (memory, sqlite)")
	storagePath := flag.String("storage-path", "", "SQLite DB path")
	listenAddr := flag.String("listen", "", "TCP listener address for inbound connections")
	openAccess := flag.Bool("open-access", false, "DEV: grant wildcard caps to all connecting peers")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *openAccess {
		fmt.Fprintln(os.Stderr, "entity-console: WARNING — running with -open-access; all connecting peers receive wildcard capabilities (dev only)")
	}

	wb.Init()

	// Phase I §13: console now drives peers through shellboot.PeerManager
	// (the same renderer-neutral type the Avalonia bridge consumes per
	// §12). Single-peer behavior is preserved — we Create one peer from
	// the argv flags here; future multi-peer UI work would Create more
	// peers + add a picker for switching active. Manager.ShutdownAll on
	// process exit cascades through every registered peer's cleanup
	// (the AppPeer.Close + console's own dropManagedPeer hook).
	app := newApplication()
	defer app.manager.ShutdownAll()

	hA, err := app.manager.Create(shellboot.Config{
		Identity:    *identity,
		LocalAlias:  *alias,
		StorageKind: *storage,
		StoragePath: *storagePath,
		ListenAddr:  *listenAddr,
		OpenAccess:  *openAccess,
	})
	if err != nil {
		log.Fatalf("entity-console: %v", err)
	}
	hp := app.manager.Get(hA)
	if hp == nil {
		log.Fatalf("entity-console: peer manager lost handle %d immediately after Create", hA)
	}
	mp := app.addManagedPeer(hp)
	ap := hp.AppPeer

	// Seed test entity (L0 direct store — we are the peer owner).
	st := mp.peerCtx.Store()
	if _, err := st.Put("test/hello", "test/hello", map[string]string{
		"message": "entity-workbench-console",
	}); err != nil {
		log.Fatalf("seed: %v", err)
	}

	// Global refresh driver. Uses the raw §6.3 event stream because
	// we want every mutation, unfiltered — there's nothing pattern-
	// specific to filter for. Store.Watch would work (add per-pattern
	// matching overhead); AppPeer.Subscribe is the dispatched form
	// reserved for cross-peer / token-gated use. This app owns its
	// peer and lives in-process, so the raw L0 stream is the right
	// layer. See SDK-ALIGNMENT §7.1.
	//
	// Started BEFORE layout setup so startup events (writes from
	// window init) are captured.
	// Coalesce refresh requests. The drainer must keep up with tree-
	// event production — every event spent in this goroutine is one
	// the upstream emit can't deliver, and after the core-go switch to
	// blocking-fanout a slow drainer stalls every write
	// on the peer. Calling app.queueRefresh() per event hit tview's
	// update queue limit during 1000-file mount bursts; coalescing
	// keeps the drainer O(1) fast and the UI still refreshes within
	// one tview tick of the latest event.
	// Refresh driver. Most panel models (markdown_files, handler_model,
	// detail via OnSelectionChange) now subscribe to their own prefix
	// and update their local state from events directly — they don't
	// need queueRefresh to drive their data updates.
	//
	// Two panels still rely on this loop: tree-browser and peer-info,
	// both of which display "everything in the tree" and re-query the
	// store on each refresh. Their migration to event-driven
	// incremental updates is followup work.
	//
	// We still must drain TreeEvents (core-go's blocking-fanout fix
	// means a slow drainer stalls every write). The
	// drainer just signals refresh — no cache markdirty.
	refreshCh := make(chan struct{}, 1)
	go func() {
		for range refreshCh {
			app.queueRefresh()
		}
	}()
	go func() {
		for ev := range ap.TreeEvents() {
			change := "modified"
			switch ev.ChangeType {
			case store.ChangeCreated:
				change = "created"
			case store.ChangeDeleted:
				change = "deleted"
			}
			app.eventLog.Verbosef("tree.event %s %s", change, ev.Path)
			select {
			case refreshCh <- struct{}{}:
			default:
				// Refresh already pending; coalesce.
			}
		}
	}()

	// Build all screens from shared default config
	app.setupDefaultScreens()
	app.eventLog.Append("layout initialized")

	// Write all initial workspace state to tree. Selection slots seed
	// themselves via SelectionState.Select once user navigation begins.
	app.saveAllWindowStates()
	app.saveScreenState()

	app.eventLog.Append("console started")

	// Refresh-loop regression heartbeat: a slow background writer that
	// keeps the tree-event loop alive even when nothing else is
	// touching the store. Surfaces bugs where a panel rebuilds its
	// renderer state on every queueRefresh and stomps user navigation
	// — the kind of issue you wouldn't notice in a quiet repo and
	// would only catch the day a real subscription started writing.
	// (See markdown-files incident: tree rebuild on every
	// tree event was destroying tview's currentNode.) Keep it on so
	// the next "innocent panel rebuild" bug is caught immediately
	// instead of weeks later in a real workload. Writes are scoped to
	// the test/heartbeat/ prefix so they're easy to ignore in mounts
	// + filters.
	go func() {
		tick := 0
		for {
			time.Sleep(2 * time.Second)
			tick++
			path := fmt.Sprintf("test/heartbeat/%03d", tick)
			if _, err := st.Put(path, "test/heartbeat", map[string]interface{}{
				"tick": tick,
				"time": time.Now().Format(time.RFC3339),
			}); err != nil {
				app.eventLog.Appendf("heartbeat write error: %s", err)
			}
		}
	}()

	if err := app.ws.run(); err != nil {
		log.Fatalf("app: %v", err)
	}
}
