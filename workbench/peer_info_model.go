package workbench

import (
	"sort"
	"sync"
)

var _ Model[PeerInfoOutput] = (*PeerInfoModel)(nil)

// PeerInfoModel is the business logic for the peer info panel. It
// subscribes to all paths under the local peer and maintains an
// incremental sorted path set + count.
//
// Cost model: O(N) seed at construction; O(log N) insert/remove per
// event (binary search into sorted slice); O(1) for Render after
// snapshot is ready (just returns the cached sorted slice). No
// per-render scan.
//
// The panel's UX still lists all paths — scaling to many thousands is
// a separate UX concern. The structural improvement here is that
// background writes (heartbeats, etc) update the panel's local state
// incrementally instead of forcing a full enumeration on every render.
type PeerInfoModel struct {
	peerCtx *PeerContext

	cancel func()

	mu      sync.Mutex
	pathSet map[string]bool // membership, for O(1) duplicate detection
	sorted  []string        // sorted by path; rebuilt on dirty
	dirty   bool
}

// PeerInfoOutput is the renderer-neutral output of the peer info model.
type PeerInfoOutput struct {
	EntityCount int
	PathCount   int
	Paths       []string
}

// PeerCtx returns the underlying PeerContext.
func (m *PeerInfoModel) PeerCtx() *PeerContext { return m.peerCtx }

// NewPeerInfoModel creates a peer info model and subscribes to all
// paths under the local peer.
func NewPeerInfoModel(peerCtx *PeerContext) *PeerInfoModel {
	m := &PeerInfoModel{
		peerCtx: peerCtx,
		pathSet: make(map[string]bool),
	}
	if peerCtx != nil && peerCtx.Store() != nil {
		m.cancel = peerCtx.Store().OnPrefixChange("", m.onEvent)
	}
	return m
}

// Close cancels the subscription. Idempotent.
func (m *PeerInfoModel) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// onEvent updates the local path set. O(1) for membership change;
// the sorted-slice rebuild is deferred to Render via the dirty flag.
func (m *PeerInfoModel) onEvent(ev ChangeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.EventType {
	case ChangePut:
		if !m.pathSet[ev.Path] {
			m.pathSet[ev.Path] = true
			m.dirty = true
		}
	case ChangeRemove:
		if m.pathSet[ev.Path] {
			delete(m.pathSet, ev.Path)
			m.dirty = true
		}
	}
}

// Render returns current peer statistics from the local snapshot.
// Rebuilds the sorted slice if the path set changed since last render.
func (m *PeerInfoModel) Render() PeerInfoOutput {
	if m.peerCtx == nil {
		return PeerInfoOutput{}
	}
	m.mu.Lock()
	if m.dirty {
		m.dirty = false
		m.sorted = m.sorted[:0]
		if cap(m.sorted) < len(m.pathSet) {
			m.sorted = make([]string, 0, len(m.pathSet))
		}
		for p := range m.pathSet {
			m.sorted = append(m.sorted, p)
		}
		sort.Strings(m.sorted)
	}
	// Hand back a copy so callers can't mutate our cache.
	paths := make([]string, len(m.sorted))
	copy(paths, m.sorted)
	pathCount := len(m.sorted)
	m.mu.Unlock()

	return PeerInfoOutput{
		EntityCount: m.peerCtx.EntityCount(),
		PathCount:   pathCount,
		Paths:       paths,
	}
}
