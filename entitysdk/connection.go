package entitysdk

import (
	"context"
	"net"
	"strings"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/peer"
)

// Connection is a connection to a remote peer. This is the
// entity-core-go Connection type, re-exported for SDK consumers.
// After Connect returns, the handshake has completed: ConnState()
// carries RemotePeerID and Session() carries the exchanged
// capability.
type Connection = peer.Connection

// Connect dials addr and performs the HELLO → AUTHENTICATE handshake
// per SDK-OPERATIONS §7.1. The transport is selected by URL scheme:
//
//   - "ws://..." / "wss://..." → core-go ConnectWebSocket (V7 §6.5.2b).
//   - anything else (incl. bare host:port and "tcp://...") → TCP.
//
// The returned Connection is ready for dispatched operations. Errors
// follow the spec error model:
//
//   - 500  Transport error (dial failed).
//   - 403  Handshake failed (signature / protocol / rejected).
//
// The connection is pooled by the local peer and reused for
// subsequent operations targeting the same remote peer ID.
func (a *AppPeer) Connect(ctx context.Context, addr string) (*Connection, error) {
	if addr == "" {
		return nil, NewError(400, "invalid_address", "empty connect address")
	}

	var conn *Connection
	var err error
	if strings.HasPrefix(addr, "ws://") || strings.HasPrefix(addr, "wss://") {
		conn, err = a.peer.ConnectWebSocket(ctx, addr)
	} else {
		// core-go's TCP Connect does net.Dial("tcp", addr) — passing a
		// "tcp://host:port" URL straight through trips
		// net.SplitHostPort's "too many colons" check. Strip the scheme
		// here so callers can pass either bare host:port or the URL
		// form (symmetric with RegisterRemote below).
		conn, err = a.peer.Connect(ctx, strings.TrimPrefix(addr, "tcp://"))
	}
	if err != nil {
		return nil, WrapError(500, "transport_failed", "dial "+addr, err)
	}
	if err := conn.PerformConnect(ctx); err != nil {
		conn.Close()
		return nil, WrapError(403, "handshake_failed", "handshake with "+addr, err)
	}

	cs := conn.ConnState()
	if cs == nil || cs.RemotePeerID == "" {
		conn.Close()
		return nil, NewError(500, "handshake_incomplete",
			"handshake completed without remote peer-id")
	}
	peerID := crypto.PeerID(cs.RemotePeerID)

	// Seed the transport-profile entity so the dispatcher's remote
	// path can resolve {peerID} → addr if the pool entry is later
	// evicted. Core-go has one RegisterRemote* per transport; route
	// by the same scheme we dialed with. Strip a "tcp://" prefix the
	// caller may have included — RegisterRemote (TCP) expects bare
	// host:port.
	var regErr error
	switch {
	case strings.HasPrefix(addr, "ws://"), strings.HasPrefix(addr, "wss://"):
		regErr = a.peer.RegisterRemoteWS(peerID, addr)
	case strings.HasPrefix(addr, "tcp://"):
		regErr = a.peer.RegisterRemote(peerID, strings.TrimPrefix(addr, "tcp://"))
	default:
		regErr = a.peer.RegisterRemote(peerID, addr)
	}
	if regErr != nil {
		conn.Close()
		return nil, WrapError(500, "register_remote_failed",
			"register transport for "+string(peerID), regErr)
	}

	// Cache the connection in the remote pool so subsequent dispatched
	// ops reuse it instead of dialing fresh. If a race resolved in favor
	// of an existing pool entry, AddRemoteConnection closes our conn and
	// returns the pooled one — callers see whichever connection the pool
	// is using, never an orphan.
	pooled, err := a.peer.AddRemoteConnection(peerID, conn)
	if err != nil {
		conn.Close()
		return nil, WrapError(500, "pool_insert_failed",
			"add remote connection for "+string(peerID), err)
	}
	return pooled, nil
}

// Disconnect closes the cached pooled connection (if any) to the
// remote peer-id and removes its transport-address entry from the
// local peer's tree. After Disconnect, dispatching to URIs naming
// peerID will fail address resolution unless re-registered via
// Connect or RegisterRemote.
//
// No-op if the peer-id is not currently known. Safe to call after
// the connection has been closed by the remote side.
func (a *AppPeer) Disconnect(peerID string) {
	if peerID == "" {
		return
	}
	a.peer.RemoveRemote(crypto.PeerID(peerID))
}

// Listen binds the configured listen address and accepts incoming
// connections until ctx is cancelled. Requires PeerConfig.ListenAddr
// to be set at CreatePeer time.
//
// Per SDK-OPERATIONS §7.2, returns 400 if no address was configured
// and 500 on transport failure.
func (a *AppPeer) Listen(ctx context.Context) error {
	if err := a.peer.Listen(ctx); err != nil {
		// Distinguish the "no listen address" config error from
		// generic transport errors.
		if err.Error() == "no listen address configured" {
			return NewError(400, "no_listen_addr",
				"Listen called on peer built without ListenAddr")
		}
		return WrapError(500, "listen_failed", "listen", err)
	}
	return nil
}

// ListenReady is like Listen but closes `ready` once the listener is
// bound and accepting. Useful for tests that need to synchronize a
// client Connect against the server's accept loop.
func (a *AppPeer) ListenReady(ctx context.Context, ready chan struct{}) error {
	if err := a.peer.ListenReady(ctx, ready); err != nil {
		if err.Error() == "no listen address configured" {
			return NewError(400, "no_listen_addr",
				"ListenReady called on peer built without ListenAddr")
		}
		return WrapError(500, "listen_failed", "listen", err)
	}
	return nil
}

// ListenWebSocketReady binds an HTTP listener at addr that upgrades
// urlPath to WebSocket and serves accepted connections through the
// same handshake path TCP uses (V7 §6.5.2b). If urlPath is empty,
// core-go defaults to "/ws".
//
// ready, if non-nil, is closed once the underlying listener is
// bound — mirrors ListenReady's contract.
//
// Returns when ctx is cancelled or the listener exits.
func (a *AppPeer) ListenWebSocketReady(ctx context.Context, addr, urlPath string, ready chan struct{}) error {
	if err := a.peer.ListenWebSocketReady(ctx, addr, urlPath, ready); err != nil {
		return WrapError(500, "listen_failed", "websocket listen "+addr, err)
	}
	return nil
}

// Addr returns the listener's bound address, or nil if this peer is
// not listening.
func (a *AppPeer) Addr() net.Addr {
	return a.peer.Addr()
}
