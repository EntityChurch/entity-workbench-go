package entitysdk

// PeerContext bundles the Executor (Level 1 dispatched operations)
// and the Store (Level 0 direct access) for callers that want both
// references in one place.
//
// Historical note: PeerContext used to cache an entry-list snapshot
// (Entries / MarkDirty / RefreshIfDirty) for shared use by UI
// consumers. That cache was a structural anti-pattern — every UI
// refresh rebuilt the cache by listing the whole tree, blocking the
// renderer's main goroutine for ~22ms on a populated store. The cache
// was removed; panel models now own per-prefix
// subscriptions via Store.OnPrefixChange and maintain their own local
// view-state.
//
// PeerContext now exists only as a small bundle of references. Future
// cleanup may delete it entirely once renderers thread Executor +
// Store directly. See SDK-OPERATIONS §2.7 on why the two levels are
// never aliased.
type PeerContext struct {
	executor *Executor
	store    *Store
}

// NewPeerContext creates a PeerContext backed by an Executor (for
// dispatch) and a Store (for direct-store reads).
func NewPeerContext(executor *Executor, store *Store) *PeerContext {
	return &PeerContext{executor: executor, store: store}
}

// Executor returns the Level 1 dispatch layer.
func (pc *PeerContext) Executor() *Executor {
	return pc.executor
}

// Store returns the Level 0 direct-store accessor. Reads through
// this bypass handler dispatch; use AppPeer's dispatched methods for
// capability-enforced operations.
func (pc *PeerContext) Store() *Store {
	return pc.store
}

// Resolve looks up an entity by path through the executor.
func (pc *PeerContext) Resolve(path string) (ResolvedEntity, bool) {
	ent, ok := pc.store.Get(path)
	if !ok {
		return ResolvedEntity{}, false
	}

	var decoded interface{}
	_ = decodeEntityData(ent.Data, &decoded)

	return ResolvedEntity{
		Path:    path,
		Hash:    ent.ContentHash,
		Entity:  ent,
		Decoded: decoded,
	}, true
}

// EntityCount returns the number of entities in the peer's store.
func (pc *PeerContext) EntityCount() int {
	return pc.store.EntityCount()
}

// PathCount returns the number of paths in the peer's location index.
func (pc *PeerContext) PathCount() int {
	return pc.store.PathCount()
}
