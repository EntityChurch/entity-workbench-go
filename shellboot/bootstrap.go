// Package shellboot is the shared startup sequence for the three
// frontends that drive a shell-backed workspace: the standalone
// entity-shell binary, the canvas GUI, and the console TUI. Each
// frontend defines its own flag surface, then calls Bootstrap to
// build the AppPeer + ShellWorkspace; from there the frontends
// diverge (REPL loop vs window event loop vs tview event loop).
//
// The point is that there is exactly one place where peer
// construction, identity binding, workbench-handler registration,
// and Phase E mount reload live. Frontends that skip the shared
// path will silently miss extensions; that's the divergence
// PHASE-G-SHELL-CENTRIC-UI-PLAN.md set out to fix.
package shellboot

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// Config is the renderer-neutral knobs every frontend exposes
// through its own flag surface. Stage 1 covers the substrate; extra
// frontend-specific config (window title, layout, JSON output) stays
// on the frontend side.
//
// JSON tags match the field names the bridge + Avalonia frontend
// serialize (legacy "alias" name kept for compat with the existing
// avalonia/frontend/Program.cs BridgeConfig serializer).
// ExtraPeerOptions is JSON-ignored (not serializable).
type Config struct {
	// Identity is the optional identity name bound to the peer.
	// Non-empty resolves the on-disk identity bundle via
	// entitysdk.IdentityBindingConfig.
	//
	// Empty means an ephemeral keypair — EXCEPT under
	// StorageKind == "sqlite", where Bootstrap substitutes
	// DefaultIdentityName and creates it if absent. A persistent store
	// under a per-invocation keypair is not persistence: the tree is
	// peer-id-namespaced, so each run writes a namespace the next one
	// cannot see. See Bootstrap.
	Identity string `json:"identity"`

	// CreateIdentity makes a NAMED identity create-if-absent instead of
	// load-or-fail. Off by default, and it must stay that way.
	//
	// Until 2026-09-06 only DefaultIdentityName was ever created, so
	// `-identity alice` on a machine that had never run `identity create
	// alice` failed with a 404 — while the Avalonia frontend's own usage
	// text said a named identity was "created on first launch". In the
	// GUI that was a dead end rather than an inconvenience: the bridge
	// fails to init, so no surface exists from which to create the
	// identity that would let it start.
	//
	// The fix is a SECOND FLAG rather than making -identity permissive,
	// because the two operations are not variants of one another.
	// Loading names a peer that exists; creating BRINGS A NEW PEER INTO
	// BEING, and the tree is peer-id-namespaced, so a typo under
	// create-if-absent silently abandons every entity, mount, offer and
	// capability grant the intended peer owns — including the ones other
	// machines wrote naming it. Refusing an unknown name is the correct
	// behaviour; having no way to say "yes, a new one" was the defect.
	CreateIdentity bool `json:"create_identity"`

	// LocalAlias is the alias under which the in-process peer is
	// registered in the shell workspace. Empty means: derive from
	// Identity when set, otherwise fall back to "self". "local" is
	// reserved for the local/* extension namespace and is rejected
	// as a peer alias.
	LocalAlias string `json:"alias"`

	// StorageKind selects the backing store. "" or "memory" is the
	// in-process default; "sqlite" persists to disk.
	StorageKind string `json:"storage"`

	// StoragePath is the SQLite path when StorageKind == "sqlite".
	// Empty → derived as ~/.entity/peers/{Identity}/store.db
	// (GUIDE-PERSISTENCE §1.1), using DefaultIdentityName when Identity
	// is empty. Use ":memory:" for an in-process SQL DB.
	StoragePath string `json:"storage_path"`

	// ListenAddr is the inbound TCP listener address (e.g.
	// "127.0.0.1:9100"). Empty means outbound-only.
	//
	// **Setting this does not bind anything by itself.** The address is
	// carried into peer.WithListenAddr, and core-go reads it in exactly
	// one place — Peer.Listen — so a frontend must call
	// BringUpListener (or PeerManager.Create, which does). Until
	// 2026-09-03 entity-shell did neither, and its documented -listen
	// flag bound no socket at all.
	ListenAddr string `json:"listen"`

	// ReconcileOnStart runs one pass of the sharing reconciler once the
	// peer is up: re-establish every declared relationship, re-authorize
	// every declared folder, and report what could not be established.
	//
	// Opt-in rather than automatic, for two reasons that are not the same
	// reason. A pass DIALS, and a test process that quietly opens
	// connections to whatever a fixture left in its store is a test suite
	// with a network dependency it never declared. And a one-shot CLI
	// invocation (`entity-shell mounts`) has no business spending a dial
	// budget on a relationship it will not use before it exits.
	//
	// A frontend that stays running — the GUI, the REPL — sets it, and
	// that is where "why do I have to press connect again" is actually
	// answered.
	ReconcileOnStart bool `json:"reconcile_on_start"`

	// CatchUpInterval starts the periodic backfill supervisor
	// (shellcmd/catchup.go). Zero means "default off ReconcileOnStart";
	// negative means OFF explicitly.
	//
	// It exists because a delivery this peer was never sent cannot be
	// detected here: the subscription engine drops on shard saturation at
	// the PUBLISHER, before anything reaches the wire, so a receiver sees
	// no error, no retry and no counter move. Measured 2026-09-07 — a
	// 2000-file directory copy delivered 676 files and stopped, silently
	// and permanently, with `syncs` healthy throughout. The only way back
	// is to ask the sender what it actually has, which is what a pass does.
	//
	// Opt-in for ReconcileOnStart's FIRST reason and not its second: a
	// pass does not dial, so it costs no connection budget, but a
	// one-shot CLI still has no business starting a background loop it
	// will outlive by milliseconds. A frontend that stays running sets it.
	//
	// Stop it with ws.StopCatchUp(); a process-lifetime peer need not.
	CatchUpInterval time.Duration `json:"catch_up_interval"`

	// LongRunning says this process intends to stay open — a desktop app,
	// a REPL session, a console. It starts the catch-up supervisor
	// independently of ReconcileOnStart.
	//
	// **It exists because ReconcileOnStart answers the wrong question.**
	// That flag means *"I have durable declarations to re-establish"*, and
	// the supervisor needs *"am I going to be around to take another
	// pass?"* The two coincide for the default configurations and diverge
	// for in-memory ones — so a peer with an in-memory tree could accept a
	// share mid-session, take a burst, lose files permanently, and have no
	// next launch to recover them in. `--ephemeral` on the GUI was exactly
	// that peer.
	//
	// It is a SEPARATE flag rather than a widened meaning for the old one,
	// and additively so: the supervisor starts if EITHER is set. Making
	// the loop unconditional would put one behind the several hundred
	// peers the suites build through Bootstrap and change the load profile
	// of the whole sweep — and this tree already has load-dependent
	// failures. Additive means the existing configurations measure exactly
	// as they did, and the new flag is the control arm for its own change.
	LongRunning bool `json:"long_running"`

	// HistoryPathBudget is how many recorded versions of ONE path a peer
	// keeps before it stops recording that path and says so
	// (shellcmd/history_budget.go). Zero takes
	// shellcmd.DefaultHistoryPathBudget; a NEGATIVE value turns the guard
	// off, which is the only way to re-measure the unbounded growth it
	// exists to stop.
	//
	// **Unlike CatchUpInterval this does NOT hang off ReconcileOnStart**,
	// and the difference is the failure each one addresses. A catch-up
	// pass is work a peer does on a schedule, so it belongs to a peer that
	// is going to be around to do it. Unbounded recording is not work — it
	// is a consequence of a mount existing, it accrues at whatever rate
	// something else is writing, and a peer that runs for an hour with a
	// log file in a shared folder has written a gigabyte whether or not it
	// intended to stay up. The guard costs one prefix watch and a map
	// increment per recorded transition, so there is no configuration in
	// which paying for it is worse than not having it.
	HistoryPathBudget int64 `json:"history_path_budget"`

	// ListenFallback allows an ephemeral port when ListenAddr is already
	// in use.
	//
	// Off by default, and that is the point: an address an operator TYPED
	// must fail loudly when something else holds it, because silently
	// binding a different port hides a real conflict and produces a peer
	// nobody can reach at the address they wrote down. It is set by a
	// frontend that supplies a DEFAULT port the operator never chose —
	// where refusing to start because a second instance is already
	// running would be absurd.
	ListenFallback bool `json:"listen_fallback"`

	// AdvertiseURL is the dial address published as this peer's
	// transport profile (EXTENSION-NETWORK §6.5.1a D1 self-publication)
	// once the listener binds. Empty means "derive it from ListenAddr",
	// which works whenever the bind host is concrete and is skipped
	// with HostedPeer.AdvertiseErr when it is a wildcard — a listener
	// binds 0.0.0.0, but a profile carries what a peer DIALS.
	//
	// Set it explicitly whenever the routable address differs from the
	// bound one: a LAN IP behind a 0.0.0.0 bind, a hostname, a
	// reverse-proxied wss:// URL.
	AdvertiseURL string `json:"advertise"`

	// OpenAccess, when true, grants every connecting peer wildcard
	// capabilities. Development use only — production peers should
	// configure scoped grants via the role extension. Required for
	// the prototype multi-peer flows in USAGE-PROTOTYPE-FILESYSTEM-SYNC.md.
	OpenAccess bool `json:"open_access"`

	// DisableRegistry turns OFF the EXTENSION-REGISTRY name-resolution
	// substrate, which every shellboot-hosted frontend otherwise carries.
	//
	// The default is ON, and it is a reversal: shellboot never set this
	// field until 2026-08-19, so `entity-shell` and the Avalonia frontend
	// shipped with no registry handler at all. Every piece of the name arc
	// — ResolveName, BindLocalName, the resolver-config, the v1.13/1.14
	// conformance work — was reachable only from unit tests, because the
	// handler it dispatches to was never registered in a shipped binary.
	// EXTENSION-REGISTRY §11.2 lists "UI / CLI surface for local-name
	// bind / unbind / list" as a SHOULD; `name` is that surface, and it
	// needs this on to do anything.
	//
	// The SDK keeps its own default OFF (entitysdk.PeerConfig is a library
	// surface and should not spend a namespace its embedder did not ask
	// for). shellboot is the application tier and makes the opposite call
	// on a measurement: entitysdk/registry_bootstrap_cost_test.go prices
	// the extension at +8 paths / +8 entities once, with ZERO marginal
	// cost per restart. The linear-rebootstrap-leak claim that justified
	// the old default did not survive its control — a registry-less peer
	// accretes at exactly the same rate.
	DisableRegistry bool `json:"disable_registry"`

	// DeliveryQueueSize is the subscription engine's total async
	// delivery buffer, in notification slots. Zero takes
	// entitysdk.DefaultDeliveryQueueSize (4096).
	//
	// **This is the knob that decides how big a burst a share can absorb
	// before it starts losing files.** The queue DROPS when full — it
	// does not block and it does not fail the write — so exceeding it
	// costs data on a path where nothing reports an error. Measured on a
	// 2000-file copy into a shared folder: the sender drops 2327
	// notifications and the receiver ends up with 676 files, permanently,
	// with both peers reporting healthy. See
	// docs/architecture/SYNC-LIMITS-AND-FAILURE-MODES.md §3.
	//
	// It existed as a documented mitigation — entitysdk's own doc says
	// "raise it on a peer that mounts large trees" — reachable from **no
	// frontend and no config file**, which is D23's shape one layer below
	// where the reachability sweep looks. A knob nobody can turn is not a
	// mitigation.
	//
	// The cost is memory, allocated eagerly at StartDelivery and never
	// released (core-go's `subscription.Engine` has no Stop): core-go's
	// own 65536 default measures ~20.3 MB per peer. So this is a
	// per-deployment call, not a global one — right for a desktop peer
	// holding one identity, wrong for a process holding a hundred.
	//
	// Raising it buys TIME, not throughput. If writes outrun delivery for
	// long enough, any finite queue fills; the catch-up supervisor is
	// what makes that a delay rather than a loss.
	// Zero takes shellboot's DefaultDeliveryQueueSize; a NEGATIVE value
	// takes entitysdk's library default instead, which is the way to ask
	// for the small ring deliberately.
	DeliveryQueueSize int `json:"delivery_queue_size"`

	// ExtraPeerOptions forwards raw peer options to entitysdk for
	// frontend-specific tuning (e.g. additional handlers, sync hooks).
	// Use sparingly; most knobs belong in Config above.
	ExtraPeerOptions []peer.Option `json:"-"`
}

// Bootstrap builds the AppPeer + ShellWorkspace from a Config. The
// caller owns the peer's lifecycle; defer (*entitysdk.AppPeer).Close
// before exiting.
//
// The bootstrap responsibilities are deliberately concentrated here:
//
//  1. Derive the SQLite path from Identity when StorageKind=sqlite and
//     StoragePath is empty.
//  2. Create the storage directory if missing.
//  3. Resolve Identity into the peer-config identity binding.
//  4. Register the workbench handlers (notification-ingest,
//     blob-resolve, chain-errors) so `mount` and `sync` work.
//     (revision-converge used to be here and retired when
//     `revision:pull` landed in core-go; the list said otherwise for
//     long enough that it is worth naming the correction.)
//  5. Construct the AppPeer via entitysdk.CreatePeer.
//  6. Call localfiles.Engine.Load to re-start any persisted Phase E
//     mounts (restart-equivalence).
//  7. Restore the workbench half of every mount and every sync — the
//     source-to-target routing the kernel does not hold.
//  8. Construct the ShellWorkspace and stash the handler refs on it.
//
// Skipping any of these on the frontend side leaves a measurable
// feature gap (Phase E mounts don't reload, `revision follow` is
// broken, etc.). That's why all three frontends share this path.
func Bootstrap(ctx context.Context, cfg Config) (*entitysdk.AppPeer, *shellcmd.ShellWorkspace, error) {
	if cfg.LocalAlias == "" {
		if cfg.Identity != "" {
			cfg.LocalAlias = cfg.Identity
		} else {
			cfg.LocalAlias = "self"
		}
	}

	// **A persistent store needs a persistent peer-id, so sqlite without
	// an identity gets the default one — created on first use.**
	//
	// This is a fix, not a convenience. Without an identity the peer
	// generates a FRESH KEYPAIR PER INVOCATION, and the whole tree is
	// peer-id-namespaced: every run wrote under a different namespace of
	// the same database, so nothing the last run stored was visible to
	// the next. It presented as "persistence is broken" and it affected
	// every persistent surface — names, mounts, aliases, revisions — not
	// one feature. Found driving the `name` verb end to end across
	// separate processes, which is the only way to see it: a single
	// in-process test never restarts, so the keypair never changes.
	//
	// The database was still accumulating a full bootstrap per run, so
	// the cost was not merely invisible state — it was unbounded growth
	// nobody could account for.
	//
	// Ephemeral storage keeps the ephemeral keypair. That is coherent:
	// nothing survives the process either way, so there is no state for
	// a stable id to be the key to.
	if cfg.StorageKind == "sqlite" && cfg.Identity == "" {
		if err := ensureIdentity(DefaultIdentityName); err != nil {
			return nil, nil, err
		}
		cfg.Identity = DefaultIdentityName
	} else if cfg.Identity != "" && cfg.CreateIdentity {
		// The operator said the dangerous thing out loud. See
		// Config.CreateIdentity for why it has to be said.
		if err := ensureIdentity(cfg.Identity); err != nil {
			return nil, nil, err
		}
	}

	// SQLite path derivation: when -storage=sqlite and -storage-path
	// is empty, derive ~/.entity/peers/{Identity}/store.db per
	// GUIDE-PERSISTENCE §1.1. Identity is always set by the block above
	// for sqlite, so this no longer has an empty-identity door.
	resolvedStoragePath := cfg.StoragePath
	if cfg.StorageKind == "sqlite" && resolvedStoragePath == "" {
		p, err := entitysdk.DefaultPeerStoragePath(cfg.Identity)
		if err != nil {
			return nil, nil, fmt.Errorf("shellboot: resolve storage path: %w", err)
		}
		resolvedStoragePath = p
	}
	if err := entitysdk.EnsurePeerStorageDir(resolvedStoragePath); err != nil {
		return nil, nil, fmt.Errorf("shellboot: prepare storage dir: %w", err)
	}

	peerCfg := entitysdk.PeerConfig{
		Storage:    entitysdk.StorageConfig{Kind: cfg.StorageKind, Path: resolvedStoragePath},
		ListenAddr: cfg.ListenAddr,
	}
	// The name-resolution substrate the `name` verb dispatches to. See
	// Config.DisableRegistry for why this is on by default and what it costs.
	if !cfg.DisableRegistry {
		peerCfg.Extensions.Registry = &entitysdk.RegistryConfig{}
	}
	if q := cfg.DeliveryQueueSize; q >= 0 {
		if q == 0 {
			q = DefaultDeliveryQueueSize
		}
		peerCfg.Extensions.Subscription = &entitysdk.SubscriptionConfig{
			DeliveryQueueSize: q,
		}
	}
	if cfg.Identity != "" {
		peerCfg.Identity = &entitysdk.IdentityBindingConfig{Name: cfg.Identity}
	}
	if cfg.OpenAccess {
		peerCfg.RawOptions = append(peerCfg.RawOptions,
			peer.WithConnectionGrants(peer.OpenAccessGrants()))
	}
	if len(cfg.ExtraPeerOptions) > 0 {
		peerCfg.RawOptions = append(peerCfg.RawOptions, cfg.ExtraPeerOptions...)
	}

	// Wire workbench handlers needed for Phase E mounts + chain-errors
	// observability. These must be registered at peer-construction
	// time because PeerConfig.Handlers is consumed inside CreatePeer's
	// option list. Revision-follow convergence used to live here too
	// as `workbench.RevisionConvergeHandler`; it retired
	// once `revision:pull` (REVISION §4.4.8) landed in core-go — the
	// follow chain `subscribe head → revision:pull` now expresses the
	// same orchestration declaratively, with no workbench-internal
	// handler.
	ingestHandler := workbench.NewNotificationIngestHandler(nil)

	// blob-resolve is the CROSS-PEER half: a subscription on another
	// peer's local/files/{root}/* prefix delivers here, and this handler
	// pulls the blob closure across and writes the file to our disk. It
	// is what makes `sync` work.
	//
	// It had twelve test files behind it and no registration outside
	// them. shellboot wired ingest and chain-errors and not this, and
	// `subscription` has no create verb, so the entire cross-peer file
	// pipeline was built, tested, and reachable from nothing a user
	// could run. That is D23 at the handler layer, where the
	// reachability sweep does not look because it asks whether a MODEL
	// has a surface.
	blobResolveHandler := workbench.NewBlobResolveHandler()

	peerCfg.Handlers = append(peerCfg.Handlers,
		entitysdk.HandlerRegistration{
			Pattern: workbench.NotificationIngestPattern,
			Handler: ingestHandler,
		},
		entitysdk.HandlerRegistration{
			Pattern: workbench.BlobResolvePattern,
			Handler: blobResolveHandler,
		},
		entitysdk.HandlerRegistration{
			Pattern: workbench.ChainErrorsPattern,
			Handler: workbench.NewChainErrorsHandler(),
		},
	)

	ap, err := entitysdk.CreatePeer(peerCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("shellboot: create peer: %w", err)
	}

	// Ship §4.1a's default resolver-config. Registering the registry
	// handler makes name resolution POSSIBLE; this makes it WORK — without
	// a resolver-config the chain is empty, so `name resolve` consults no
	// backend and every name reports chain_exhausted, which reads to a user
	// as "the feature is broken" rather than "you have not configured it".
	// EXTENSION-REGISTRY §4.1a: a distribution SHOULD ship this.
	//
	// EnsureResolverConfig is idempotent and never overwrites an operator's
	// config, so this is a first-boot write, not a per-start one.
	//
	// A stored config that FAILS the §4.1 step 2 privacy MUST is a
	// DIAGNOSTIC, not a boot failure — EXTENSION-REGISTRY §4.1
	// [MUST, v1.17]: "surface it, never normalize it, never refuse to
	// start". We shipped the refusal (against §11.1's since-withdrawn
	// "refused or normalized at load"), and it was wrong in the specific
	// way the ruling names: refusing to boot on a config an operator
	// deliberately wrote revokes the override the same paragraph grants
	// them. The stderr line below is the surfacing, and it is the only
	// place a `--` frontend gets one; `name config` prints the same
	// condition beside the config on every invocation.
	//
	// Real errors — a config that will not decode, a failed write — still
	// abort. See EnsureResolverConfig for the split.
	if !cfg.DisableRegistry {
		if _, err := ap.EnsureResolverConfig(); err != nil {
			_ = ap.Close()
			return nil, nil, fmt.Errorf("shellboot: resolver-config unreadable: %w", err)
		}
		if diag := ap.ResolverConfigDiagnostic(); diag != nil {
			fmt.Fprintf(os.Stderr,
				"warning: the stored resolver-config discloses names (EXTENSION-REGISTRY "+
					"§4.1 step 2): %v\n         running under it as written; `name config` "+
					"shows it, and the peer is NOT resolving through a repaired copy.\n", diag)
		}
	}

	// Restart-equivalence for Phase E mounts: walk the persisted
	// local-files config and re-start watchers. The localfiles
	// handler's own Load() handles this; we call it after
	// CreatePeer returns so the store/index/identity hash are
	// available.
	if lfh := ap.LocalFilesHandler(); lfh != nil {
		if err := lfh.Load(ctx, ap.RawContentStore(), ap.RawLocationIndex(), ap.IdentityHash()); err != nil {
			_ = ap.Close()
			return nil, nil, fmt.Errorf("shellboot: reload local-files mounts: %w", err)
		}
		// ...and then restore them again, because the call above restores
		// NOTHING. Its enumeration TrimPrefixes a relative prefix off a
		// peer-qualified index path and skips every row (AP58); the
		// symptom is a mount that works until the app is restarted and is
		// silently dead afterwards — writes 404 `no_root_mapping` and the
		// watcher never starts, while every surface still lists it as
		// healthy because they read the tree, which is fine.
		//
		// Reproduced with no network and no GUI in
		// shellboot/mount_restart_write_test.go. Routed to core-go; this
		// is ours until it lands there. See
		// workbench/localfiles_root_restore.go.
		restored, problems := workbench.RestoreLocalFilesRoots(
			ctx, ap.Store(), lfh, ap.RawContentStore(), ap.RawLocationIndex(), ap.IdentityHash())
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "warning: local-files root not restored — %s\n", p)
		}
		_ = restored
	}

	// The kernel's Load above restores the WATCHER half of every mount.
	// This restores OURS: the source→target mapping the ingest handler
	// routes on, which lived only in process memory until
	// workbench/mount_binding.go and therefore did not survive a
	// restart. Without it the watcher came back, wrote its file
	// entities, and every delivery answered 404 no_mount_for_uri — a
	// mount that listed as healthy and had quietly stopped producing
	// documents.
	//
	// A binding that cannot be restored is NAMED on stderr rather than
	// dropped. This is startup, the operator is watching, and the
	// failure it reports is one whose only other symptom is that new
	// files stop appearing days later.
	if restored, problems := workbench.RestoreMountBindings(ap.Store(), ingestHandler); restored > 0 || len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "warning: mount binding not restored — %s\n", p)
		}
	}

	// The same restoration for the cross-peer side. Only the handler's
	// source→target routing needs rebuilding here: the subscription
	// itself is persistent kernel state and the engine rebuilds its
	// runtime index at open (entitysdk/app.go — it did not until
	// 2026-09-02, and until then a restart left every mount and every
	// sync listed as healthy and producing nothing).
	if restored, problems := workbench.RestoreSyncBindings(ap.Store(), blobResolveHandler); restored > 0 || len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "warning: sync binding not restored — %s\n", p)
		}
	}

	// Pre-S6 folder records were keyed on the bare root name, so the same
	// folder had a different id on each peer and no field joined the two
	// — there was no object either side could point at. One idempotent
	// pass here rather than inside Reconcile: this rewrites the
	// operator's own declarations, and a control loop that mutates
	// declarations on every pass is a different and worse thing than a
	// loop that reconciles substrate to them.
	if moved, problems := workbench.MigrateFolderIDs(ap.Store(), ap.PeerID()); len(moved) > 0 || len(problems) > 0 {
		for _, m := range moved {
			fmt.Fprintf(os.Stderr, "migrated folder id %s\n", m)
		}
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "warning: folder id not migrated — %s\n", p)
		}
	}

	ws := shellcmd.NewShellWorkspace(ap, cfg.LocalAlias, cfg.Identity)
	ws.NotificationIngest = ingestHandler
	ws.BlobResolve = blobResolveHandler

	// Started HERE and not by each frontend, for BringUpListener's
	// reason (listener.go, AP67): a step every long-running frontend
	// needs, left to each of them to remember, is a step one of them
	// forgets — and the forgetting is invisible, because a share that
	// silently stops part way looks exactly like one that is idle.
	// ReconcileOnStart is already the "I am a long-running frontend"
	// signal, and a frontend that wants its relationships re-established
	// wants its folders to finish arriving. Defaulting off that flag
	// means the GUI, the console and the REPL each get this without a
	// per-frontend line to forget — which is the whole argument, and it
	// is AP67's: a step every frontend needs, left to each of them to
	// remember, is a step one of them forgets, and here the forgetting is
	// invisible because a share that stopped part way looks idle.
	//
	// A negative interval turns it off explicitly, so a caller that wants
	// reconcile-on-start WITHOUT the loop can say so.
	//
	// EITHER flag starts it, and LongRunning is the one that actually
	// names the property the loop needs — see Config.LongRunning for why
	// ReconcileOnStart answers a different question and why the two are
	// or'd rather than the old one being redefined.
	interval := cfg.CatchUpInterval
	if interval == 0 && (cfg.LongRunning || cfg.ReconcileOnStart) {
		interval = shellcmd.DefaultCatchUpInterval
	}
	if interval > 0 {
		ws.EnableCatchUp(interval)
	}

	// The recording growth guard, for EVERY peer — see
	// Config.HistoryPathBudget for why this one does not hang off
	// ReconcileOnStart the way the supervisor above does.
	if cfg.HistoryPathBudget >= 0 {
		ws.EnableHistoryBudget(uint64(cfg.HistoryPathBudget))
	}

	return ap, ws, nil
}

// DefaultDeliveryQueueSize is the subscription delivery ring shellboot
// gives a peer when the caller names no size, and it deliberately
// differs from entitysdk's.
//
// **This is the same call shellboot already makes about the registry
// extension, for the same reason** (see Config.DisableRegistry):
// entitysdk is a *library* surface and must be frugal because it does
// not know how many peers its embedder will hold; shellboot is the
// *application* tier and knows the answer is "one, or a handful". So
// where the SDK keeps 4096 slots (~1.2 MB/peer), a shellboot-hosted
// frontend takes core-go's own 65536 (~20.3 MB/peer, allocated eagerly
// at StartDelivery and not released on close).
//
// # Why 65536 and not more, given that more is measurably better
//
// Measured, catch-up supervisor off, `make loadtest ARGS="-run
// TestLoad_QueueDepthSweep"`:
//
//	files    4096      16384       65536      262144
//	 2000   675 ✗   2000 ✓ 3.0s   2000 ✓     2000 ✓
//	10000     —     2949 ✗       9100 ✗    10000 ✓ 5.9s
//
// Two things fall out. **The 4096 we shipped was our own bad
// arithmetic**: it was justified as core-go's stated "sized for
// 1000+-file mount bursts" plus 4× margin, but that counted FILES and
// the queue counts NOTIFICATIONS — a mounted file produces the
// watcher's file entity, the ingest chain's document and the blob
// bindings. Bracketing the two rows above puts peak demand at **6.5 to
// 8.2 slots per file**, so the "4× margin" was short by most of an
// order of magnitude.
//
// **And no finite value fixes it.** core-go's own 65536 still loses 900
// of 10,000. The cliff moves with the ring and never goes away, because
// the producer is a person with a file manager and nothing here can slow
// them down. So this is not sized to win an arms race — it is sized so
// that the bursts an operator produces by hand mostly complete at live
// speed (~3 s for 2000 files) instead of waiting on a catch-up pass, and
// the supervisor covers the rest. Raising it further trades memory for a
// cliff nobody reaches by hand; the honest fix is upstream and is routed
// (reviews/SUBSCRIPTION-SATURATION-AND-THE-LAYER-BOUNDARY-2026-09-07.md).
const DefaultDeliveryQueueSize = 65536

// DefaultIdentityName is the identity a persistent peer uses when the
// operator named none.
//
// A plain name rather than a derived or hidden one, because it appears
// in `identity ls`, in `~/.entity/identities/`, and in the derived
// store path `~/.entity/peers/default/store.db`. An operator who never
// asked for an identity should still be able to see the one they got,
// name it in a later `-identity default`, and delete it.
const DefaultIdentityName = "default"

// ensureIdentity creates the named identity if it is absent, and is a
// no-op when it exists.
//
// Named rather than default-only since 2026-09-06: the logic was always
// general and was reachable for exactly one value, which is why
// `-identity alice` on a fresh machine failed with a 404 that the GUI's
// own usage text said could not happen. WHICH names reach it is the
// caller's decision (Config.CreateIdentity) and deliberately not this
// function's.
//
// **Create-if-absent, never overwrite.** A keypair is the peer's
// identity: regenerating one over an existing file would orphan every
// entity written under the old peer-id — the same silent re-namespacing
// this whole change exists to fix, made permanent. So an existing
// identity is loaded, and only a genuine absence is filled.
//
// The 409-exists race is treated as success on purpose. Two shells
// starting at once both see "absent" and both create; one wins, and the
// loser must use the winner's keypair rather than fail. Returning an
// error there would make concurrent startup a coin flip.
func ensureIdentity(name string) error {
	if _, err := entitysdk.LoadIdentity(name); err == nil {
		return nil
	} else if !entitysdk.IsNotFound(err) {
		// A bundle directory, a permissions problem, a corrupt file —
		// anything that is not "absent" is a real failure, and creating
		// over it is exactly what must not happen.
		return fmt.Errorf("shellboot: load identity %q: %w", name, err)
	}
	if _, err := entitysdk.CreateIdentity(name); err != nil {
		if entitysdk.IsConflict(err) {
			return nil // lost the race; the winner's keypair is the one to use
		}
		return fmt.Errorf("shellboot: create identity %q: %w", name, err)
	}
	return nil
}
