package shellcmd

import (
	"context"
	"fmt"
	"time"
)

// cmdConnect implements `connect <alias> <host:port>`. Dials the
// remote peer through the shell's local AppPeer, which performs the
// HELLO → AUTHENTICATE handshake, registers the transport address,
// and caches the connection in the remote pool. Subsequent
// operations against this alias reuse the pooled connection — no
// re-dial — until the peer is disconnected.
func cmdConnect(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: connect <alias> <host:port>")
	}
	alias, err := NormalizeAlias(args[0])
	if err != nil {
		return Result{}, fmt.Errorf("connect: %w", err)
	}
	if IsReservedAlias(alias) {
		return Result{}, fmt.Errorf("connect: alias %q is reserved (built-in pronoun)", alias)
	}
	addr := args[1]

	if _, exists := sh.Conns[alias]; exists {
		return Result{}, fmt.Errorf("alias %q already in use (disconnect first)", alias)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := sh.Local.Peer.Connect(ctx, addr)
	if err != nil {
		return Result{}, fmt.Errorf("connect: %w", err)
	}

	state := conn.ConnState()
	if state == nil || state.RemotePeerID == "" {
		return Result{}, fmt.Errorf("handshake completed without remote peer-id")
	}
	peerID := string(state.RemotePeerID)

	if existing, ok := sh.peerMap[peerID]; ok {
		return Result{}, fmt.Errorf("peer %s already bound to alias %q", peerID, existing)
	}

	pc := &PeerConn{
		Alias:   alias,
		Address: addr,
		PeerID:  peerID,
		Peer:    sh.Local.Peer,
	}
	sh.addConn(pc)

	short := peerID
	if len(short) > 12 {
		short = short[:12] + "..."
	}
	return MessageResult(fmt.Sprintf("connected to %s (%s, peer-id %s)", alias, addr, short)), nil
}

// cmdDisconnect implements `disconnect <alias>`. Closes the cached
// pooled connection at the local peer level, removes the
// transport-address tree entry, and drops the alias from shell state.
func cmdDisconnect(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: disconnect <alias>")
	}
	alias := args[0]
	if alias == sh.Local.Alias {
		return Result{}, fmt.Errorf("cannot disconnect the local peer")
	}
	pc, ok := sh.Conns[alias]
	if !ok {
		return Result{}, fmt.Errorf("not connected: %s", alias)
	}

	// Evict pool + unregister transport address before dropping the
	// alias, so any in-flight dispatcher resolution sees a clean state.
	sh.Local.Peer.Disconnect(pc.PeerID)
	sh.removeConn(alias)

	// If the user was working inside the disconnected peer, drop them
	// back to the shell root. Comparing PeerIDs (not aliases) is the
	// stable form — WD always carries the peer-id segment.
	if sh.WD.PeerID() == pc.PeerID {
		sh.SetWD("/")
	}

	return MessageResult(fmt.Sprintf("disconnected from %s", alias)), nil
}
