package entitysdk

import (
	"strings"
	"sync"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
)

// Scope is a handle that binds an AppPeer, a tree prefix, and (later)
// a capability context. All path-taking operations on the Scope are
// resolved relative to the prefix.
//
// Per GUIDE-SDK-PATTERNS §1 the SDK SHOULD provide scoped handles:
//
//   - Prefix isolation. Operations cannot accidentally touch paths
//     outside the scope.
//   - Lifetime management. Watches created through the handle are
//     cancelled when Close is called.
//   - Implicit peer binding. The handle knows which peer it targets.
//   - Capability context. The handle carries the grant chain that
//     authorizes operations (not yet enforced in this SDK).
//
// Scopes accept peer-relative paths only; use AppPeer directly when
// you need to address absolute paths or other peer namespaces.
type Scope struct {
	peer   *AppPeer
	prefix string // guaranteed to end in "/" or be empty

	mu      sync.Mutex
	watches []*StoreWatch
	closed  bool
}

// Scope returns a scoped handle rooted at prefix. A trailing "/" is
// added if missing. An empty prefix is permitted (no-op scope —
// behaves like calling AppPeer directly).
func (a *AppPeer) Scope(prefix string) *Scope {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &Scope{peer: a, prefix: prefix}
}

// Prefix returns the scope's tree prefix, always with a trailing "/"
// unless empty.
func (s *Scope) Prefix() string { return s.prefix }

// Peer returns the AppPeer this scope is bound to.
func (s *Scope) Peer() *AppPeer { return s.peer }

// Close cancels all watches that were created through this scope.
// Safe to call more than once. Does not close the underlying peer.
func (s *Scope) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	watches := s.watches
	s.watches = nil
	s.mu.Unlock()

	for _, w := range watches {
		w.Close()
	}
}

// resolve joins prefix + relPath. Relative paths only — if the caller
// passes an absolute path, they're stepping outside the scope, which
// is treated as a programming error.
func (s *Scope) resolve(relPath string) (string, *Error) {
	if strings.HasPrefix(relPath, "/") {
		return "", NewError(400, "absolute_path_in_scope",
			"scope paths must be relative, got absolute path: "+relPath)
	}
	return s.prefix + relPath, nil
}

// --- Level 0 store ops (scoped) ---

// Get retrieves the entity bound at prefix + relPath.
func (s *Scope) Get(relPath string) (entity.Entity, bool) {
	full, err := s.resolve(relPath)
	if err != nil {
		return entity.Entity{}, false
	}
	return s.peer.Store().Get(full)
}

// Put stores an entity at prefix + relPath. Returns its content hash.
func (s *Scope) Put(relPath, typeName string, data interface{}) (hash.Hash, error) {
	full, err := s.resolve(relPath)
	if err != nil {
		return hash.Hash{}, err
	}
	return s.peer.Store().Put(full, typeName, data)
}

// PutCAS is a compare-and-swap put at prefix + relPath. See
// Store.PutCAS for semantics.
func (s *Scope) PutCAS(relPath, typeName string, data interface{}, expected hash.Hash) (hash.Hash, error) {
	full, err := s.resolve(relPath)
	if err != nil {
		return hash.Hash{}, err
	}
	return s.peer.Store().PutCAS(full, typeName, data, expected)
}

// Has reports whether a binding exists at prefix + relPath.
func (s *Scope) Has(relPath string) bool {
	full, err := s.resolve(relPath)
	if err != nil {
		return false
	}
	return s.peer.Store().Has(full)
}

// Remove unbinds prefix + relPath. Returns true if a binding was
// removed.
func (s *Scope) Remove(relPath string) bool {
	full, err := s.resolve(relPath)
	if err != nil {
		return false
	}
	return s.peer.Store().Remove(full)
}

// List returns entries under prefix + relPath, sorted by path. An
// empty relPath lists everything under the scope.
func (s *Scope) List(relPath string) []store.LocationEntry {
	full, err := s.resolve(relPath)
	if err != nil {
		return nil
	}
	return s.peer.Store().List(full)
}

// --- Change notification (scoped) ---

// Watch registers for changes matching prefix + relPattern. The
// returned StoreWatch is tracked by the scope and closed automatically
// when Scope.Close is called.
//
// Scope today is a scoped L0 handle — Get/Put/List/Has/Remove all
// delegate to Store — so Scope.Watch delegates to Store.Watch, which
// carries the same L0 bypass warning: no dispatch, no capability
// check. See Store.Watch. When a scoped L1 surface appears, Scope
// gains a Store()-style split and this method's semantics become
// explicit.
func (s *Scope) Watch(relPattern string) (*StoreWatch, error) {
	full, err := s.resolve(relPattern)
	if err != nil {
		return nil, err
	}
	w, wErr := s.peer.Store().Watch(full)
	if wErr != nil {
		return nil, wErr
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		w.Close()
		return nil, NewError(500, "scope_closed", "scope is closed")
	}
	s.watches = append(s.watches, w)
	s.mu.Unlock()
	return w, nil
}
