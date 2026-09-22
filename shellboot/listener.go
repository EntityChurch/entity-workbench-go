package shellboot

// listener.go — bringing a peer's inbound listener up, once, for every
// frontend.
//
// # Why this file exists
//
// `Config.ListenAddr` was plumbed from every frontend's flag surface into
// `peer.WithListenAddr` and **nothing called Listen** except
// `PeerManager.Create`. `entity-shell` does not use the manager: it calls
// `shellboot.Bootstrap` directly. So `entity-shell -listen 0.0.0.0:9100`
// — a documented flag whose help text reads *"TCP listener for inbound
// peer connections"*, and step 1 of `USAGE-SHARE-A-FOLDER.md` — accepted
// the address, stored it on the peer, and **bound no socket at all**.
// core-go reads `listenAddr` in exactly one place (`Peer.Listen`), so the
// flag could not have worked; the e2e suites stand their own listeners up
// with a local helper, which is why every one of them passes.
//
// That is the same shape as the `BlobResolveHandler` finding and the
// `Engine.Load` finding: a capability wired everywhere except in the
// binary an operator runs. The fix is not another call site — it is one
// exported bring-up that every frontend uses, so the next frontend cannot
// forget it.
//
// # What bring-up means, beyond binding
//
// Three things have to happen together, and they were split across two
// files with only one of them reachable:
//
//  1. **Bind** — and, for a DEFAULT address the operator did not choose,
//     fall back to an ephemeral port rather than refusing to start. A
//     second instance on one machine is a normal thing to do.
//  2. **Advertise** (§6.5.1a D1) — publish a transport profile so a peer
//     that knows our peer-id can dial us without being told an address.
//     This is what makes reconnect-by-peer-id work, and it was skipped
//     entirely for a wildcard bind, which is the only bind a machine on a
//     LAN should be using.
//  3. **Announce** (mDNS) — so peers on the same LAN find us with no
//     configuration at all.
//
// Advertise and announce are both non-fatal: a peer that binds but cannot
// publish is degraded, not broken, and the caller renders the reason.

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"entity-workbench-go/entitysdk"
)

// ListenerInfo is what a successful bring-up produced. Every field is
// rendered somewhere: an operator asking "why can nobody reach me?" is
// answered by BoundAddr, AdvertisedURL and the two error fields together,
// and by no one of them alone.
type ListenerInfo struct {
	// Scheme is "tcp" or "ws".
	Scheme string

	// Requested is the address the caller asked for, verbatim.
	Requested string

	// BoundAddr is the address actually bound, read back from the
	// listener. Differs from Requested whenever the port was 0 or the
	// fallback fired — and that difference is exactly what an operator
	// needs to see.
	BoundAddr string

	// FellBack reports that Requested was unavailable and an ephemeral
	// port was used instead. Only possible with Config.ListenFallback.
	FellBack bool

	// AdvertisedURL is the dial URL published as this peer's transport
	// profile, or "" when advertisement failed.
	AdvertisedURL string

	// AdvertiseErr is why no profile was published. Non-fatal.
	AdvertiseErr error

	// AnnounceErr is why mDNS announcement failed. Non-fatal — the
	// common cause is a host with no multicast-capable interface.
	AnnounceErr error

	// Cancel stops the listener. Always non-nil on success.
	Cancel func()
}

// Summary is the one-line operator-facing form. Written to stderr at
// startup by the frontends, because a listener that came up differently
// from what was asked for is the single most useful thing to know before
// anything else is attempted.
func (li *ListenerInfo) Summary() string {
	if li == nil {
		return "not listening (outbound only)"
	}
	s := "listening on " + li.BoundAddr
	if li.FellBack {
		s += " (requested " + li.Requested + " — in use, took an ephemeral port)"
	}
	switch {
	case li.AdvertisedURL != "":
		s += "; dialable at " + li.AdvertisedURL
	case li.AdvertiseErr != nil:
		s += "; NOT dialable by peer-id: " + li.AdvertiseErr.Error()
	}
	if li.AnnounceErr != nil {
		s += "; not discoverable on the LAN: " + li.AnnounceErr.Error()
	}
	return s
}

// listenReadyTimeout bounds the wait for a bind. Well above a kernel
// ephemeral-port bind, so exceeding it means something is wrong rather
// than slow.
const listenReadyTimeout = 5 * time.Second

// BringUpListener binds cfg.ListenAddr, publishes the transport profile,
// and announces on mDNS. A nil ListenerInfo with a nil error means
// cfg.ListenAddr was empty — outbound-only, which is a legitimate
// configuration and not a failure.
//
// The returned Cancel must be called to stop the listener; AppPeer.Close
// does not stop one this function started, because the goroutine is
// parented on the context passed here.
func BringUpListener(ctx context.Context, ap *entitysdk.AppPeer, cfg Config) (*ListenerInfo, error) {
	if ap == nil {
		return nil, fmt.Errorf("shellboot: BringUpListener: nil peer")
	}
	if cfg.ListenAddr == "" {
		return nil, nil
	}
	scheme, bindAddr, wsPath, err := parseListenAddr(cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("shellboot: parse listen addr %q: %w", cfg.ListenAddr, err)
	}

	li := &ListenerInfo{Scheme: scheme, Requested: cfg.ListenAddr}

	cancel, bindErr := startListener(ctx, ap, scheme, bindAddr, wsPath)
	if bindErr != nil {
		// The fallback is deliberately NOT automatic for an address the
		// operator typed: if they asked for :9100 and something else has
		// it, silently binding :51423 hides a real conflict. It IS right
		// for a default nobody chose, which is why the decision belongs
		// to the caller that chose the default.
		fallbackAddr, ok := ephemeralFallback(scheme, bindAddr)
		if !cfg.ListenFallback || !ok {
			return nil, fmt.Errorf("shellboot: listen on %q: %w", cfg.ListenAddr, bindErr)
		}
		cancel, err = startListener(ctx, ap, scheme, fallbackAddr, wsPath)
		if err != nil {
			return nil, fmt.Errorf("shellboot: listen on %q (and on the ephemeral fallback %q): %w",
				cfg.ListenAddr, fallbackAddr, err)
		}
		li.FellBack = true
		bindAddr = fallbackAddr
	}
	li.Cancel = cancel

	// Read the address back rather than reporting the one we asked for.
	// With port 0 — which the fallback always uses and which a caller may
	// ask for directly — the requested address names no port at all, and
	// every downstream consumer (advertise, mDNS, the panel, the operator)
	// needs the real one.
	li.BoundAddr = bindAddr
	if addr := ap.Addr(); addr != nil {
		li.BoundAddr = addr.String()
	}

	li.AdvertisedURL, li.AdvertiseErr = advertiseListener(ap, cfg.AdvertiseURL, scheme, li.BoundAddr, wsPath)

	// mDNS. The backend resolves the port from the bound listener itself
	// (entitysdk/app.go's mdnsResolver reads Peer.Addr), so an ephemeral
	// port announces correctly with no help from here.
	announceCtx, announceCancel := context.WithTimeout(context.Background(), 2*time.Second)
	li.AnnounceErr = ap.Announce(announceCtx, scheme)
	announceCancel()

	return li, nil
}

// startListener runs the scheme-appropriate listener and waits for the
// bind. Returns the cancel func for the listener goroutine.
func startListener(ctx context.Context, ap *entitysdk.AppPeer, scheme, bindAddr, wsPath string) (func(), error) {
	listenCtx, cancel := context.WithCancel(ctx)
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if scheme == "ws" {
			errCh <- ap.ListenWebSocketReady(listenCtx, bindAddr, wsPath, ready)
		} else {
			errCh <- ap.ListenReady(listenCtx, ready)
		}
	}()
	select {
	case <-ready:
		return cancel, nil
	case err := <-errCh:
		cancel()
		return nil, err
	case <-time.After(listenReadyTimeout):
		cancel()
		return nil, fmt.Errorf("bind timed out after %s", listenReadyTimeout)
	}
}

// ephemeralFallback rewrites a bind address to port 0 on the same host.
// Reports false when the address has no parseable host:port shape, in
// which case the original error is the honest one to return.
func ephemeralFallback(scheme, bindAddr string) (string, bool) {
	host, _, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "", false
	}
	return net.JoinHostPort(host, "0"), true
}

// advertiseListener self-publishes the peer's transport profile for the
// listener that just bound, returning the URL actually advertised.
//
// Explicit AdvertiseURL wins — it is the only thing that can be right
// behind a NAT or a reverse proxy.
//
// Otherwise the dial URL is derived from the BOUND address. Until
// 2026-09-03 a wildcard bind was refused here with "a listener binds
// 0.0.0.0, but a profile carries what a peer DIALS" — correct as far as
// it goes, and it meant the only sensible bind for a machine on a LAN
// published no profile at all. A peer with no profile cannot be
// reconnected to by peer-id: `EnsureConnected` resolves a transport
// profile, finds none, and the relationship can only be revived by
// somebody re-typing an address. That is the manual step the whole
// redesign exists to delete.
//
// So a wildcard now resolves to this host's LAN address (see lanDialHost).
// It is a GUESS, and it is labelled as one where it is rendered: a machine
// with several interfaces may be reachable on an address other than the
// one a default route implies. A guess that is usually right and always
// visible beats no profile at all — and an operator who knows better sets
// AdvertiseURL, which still wins.
func advertiseListener(ap *entitysdk.AppPeer, advertiseURL, scheme, boundAddr, wsPath string) (string, error) {
	dialURL := advertiseURL
	if dialURL == "" {
		dialHost := boundAddr
		if host, port, err := net.SplitHostPort(boundAddr); err == nil && isWildcardHost(host) {
			lan, lerr := lanDialHost()
			if lerr != nil {
				return "", fmt.Errorf("shellboot: bind %q is a wildcard and this host's LAN address could not be determined (%v) — pass -advertise to publish a dialable profile", boundAddr, lerr)
			}
			dialHost = net.JoinHostPort(lan, port)
		}
		switch scheme {
		case "ws":
			dialURL = "ws://" + dialHost + wsPath
		default:
			dialURL = "tcp://" + dialHost
		}
	}
	if err := ap.AdvertiseTransport(dialURL); err != nil {
		return "", fmt.Errorf("shellboot: advertise %q: %w", dialURL, err)
	}
	return dialURL, nil
}

// isWildcardHost reports whether a bind host names every interface rather
// than one address.
func isWildcardHost(host string) bool {
	if host == "" || host == "*" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsUnspecified()
}

// lanDialHost returns the IP another machine on this LAN would use to
// reach this one.
//
// The UDP "connect" trick: a connected UDP socket performs a route lookup
// and binds a local address, and **sends nothing**. It costs no packets,
// needs no DNS and works offline as long as a route exists. The target is
// TEST-NET-3 (RFC 5737) rather than a public resolver so that nothing here
// resembles a call home even to someone reading a packet capture.
//
// The interface walk is the fallback for a host with no default route —
// common on an isolated bridge, which is exactly where two peers might
// still want to find each other.
func lanDialHost() (string, error) {
	if c, err := net.Dial("udp4", "203.0.113.1:9"); err == nil {
		defer c.Close()
		if ua, ok := c.LocalAddr().(*net.UDPAddr); ok && ua.IP != nil && !ua.IP.IsUnspecified() {
			return ua.IP.String(), nil
		}
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("enumerate interfaces: %w", err)
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() || ipn.IP.To4() == nil {
			continue
		}
		return ipn.IP.String(), nil
	}
	// Loopback last: a same-host pair is a real configuration (it is what
	// every e2e test in this repo runs), and publishing 127.0.0.1 is more
	// useful than publishing nothing.
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no non-loopback IPv4 address on any interface")
}
