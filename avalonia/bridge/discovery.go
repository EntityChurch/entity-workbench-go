package main

// Discovery panel bridge surface. Wraps entitysdk.AppPeer's mDNS
// discovery substrate so a panel can show "Nearby peers" without the
// user typing addresses. State (candidates by peer-id-hint) lives in
// the substrate's tree under system/discovery/candidate/mdns/* — the
// canonical signal for "a new peer showed up on the LAN".
//
// Handle lifecycle parity with the other panels (TreeOpen, PeerInfo,
// PeerConnections, SiteOpen): Open → handle, RegisterWake →
// wake-fanout goroutine, Render → snapshot, Close → tear down. The
// substrate writes candidate entities into the store from its mDNS
// observe-callback path; the watcher we install fires the wake.
//
// **Render reads the store; a separate goroutine drives Scan.**
// Render MUST NOT trigger a live mDNS browse — that would block the
// caller for ~1s AND feed back into our own prefix subscription
// (every Scan writes candidates → our OnPrefixChange fires → another
// wake → another Scan; the UI thread parks permanently). Instead:
// (a) Render reads system/discovery/candidate/mdns/* from the store
// directly via AppPeer.ReadDiscoveredCandidates — cheap, no network.
// (b) A background goroutine (scanLoop) calls AppPeer.DiscoverPeers
// on a coalesced cadence to keep the store warm. Core-go's mDNS
// backend currently writes to the store only as a side-effect of
// Scan (no persistent watcher yet — see comments in
// ../entity-core-go/ext/discovery/mdns/mdns.go); when that gap lands
// upstream, scanLoop can be deleted.

/*
#include <stdlib.h>
#include <stdint.h>

// Local copy of invoke_tree_wake — see peer_connections.go for the same pattern.
static inline void invoke_tree_wake_discovery(void* cb, int64_t handle) {
    if (cb != NULL) {
        ((void(*)(int64_t))cb)(handle);
    }
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	wb "entity-workbench-go/workbench"
)

type discoveryHandle struct {
	peerHandleID int64
	hp           *shellboot.HostedPeer

	wakeCh     chan struct{}
	doneCh     chan struct{}
	wakeDoneCh chan struct{}
	scanDoneCh chan struct{}
	cancelEv   func()
}

// scanInterval is the cadence at which scanLoop drives a fresh mDNS
// browse to keep the substrate's candidate prefix warm. 5s balances
// responsiveness for "peer just powered on" against announce noise +
// LAN multicast budget. When core-go ships the persistent watcher,
// scanLoop goes away and this constant with it.
const scanInterval = 5 * time.Second

var (
	discoCounter int64
	discoMu      sync.Mutex
	discos       = map[int64]*discoveryHandle{}
)

//export DiscoveryOpen
func DiscoveryOpen(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("DiscoveryOpen", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	if !hp.AppPeer.DiscoveryEnabled() {
		return C.CString(`{"ok":false,"error":"discovery not configured (peer constructed without ListenAddr)"}`)
	}
	ch := &discoveryHandle{
		peerHandleID: hp.Handle,
		hp:           hp,
		wakeCh:       make(chan struct{}, 1),
		doneCh:       make(chan struct{}),
	}
	// Subscribe to the candidate prefix — that's where the substrate
	// writes new mDNS observations. Any change here means a new peer
	// showed up (or an existing one's hint changed).
	ch.cancelEv = hp.AppPeer.OnDiscoveredPeerChange(func(_ entitysdk.ChangeEvent) {
		select {
		case ch.wakeCh <- struct{}{}:
		default:
		}
	})
	h := atomic.AddInt64(&discoCounter, 1)
	discoMu.Lock()
	discos[h] = ch
	discoMu.Unlock()
	// Spin up scanLoop — drives DiscoverPeers on a coalesced cadence
	// so the substrate's candidate prefix stays warm. The first scan
	// fires immediately so the panel paints something on mount; each
	// successful scan writes candidates to the store, which fires our
	// OnPrefixChange subscription, which queues the wake (no inline
	// Render → Scan loop).
	ch.scanDoneCh = make(chan struct{})
	go scanLoop(ch)
	// Seed initial wake so the panel paints whatever the store already
	// holds while the first scan is still in flight (empty on first
	// mount, populated on remount).
	select {
	case ch.wakeCh <- struct{}{}:
	default:
	}
	return C.CString(fmt.Sprintf(`{"ok":true,"handle":%d}`, h))
}

// scanLoop drives DiscoverPeers (live mDNS browse) on a fixed cadence
// while the discovery handle is open. Each Scan side-effects the store
// via the substrate's observe callback; that store write fires our
// OnPrefixChange subscription, which queues a wake the C# panel reads
// out via DiscoveryRender. scanLoop OWNS Scan; nothing else calls it.
//
// Decoupling Scan from Render is the architectural fix for B-5: prior
// to this loop, Render itself called Scan, which (a) blocked the UI
// thread for ~1s and (b) generated the wake that triggered the next
// Render → Scan → wake cycle.
func scanLoop(ch *discoveryHandle) {
	defer close(ch.scanDoneCh)
	t := time.NewTimer(0) // fire immediately
	defer t.Stop()
	for {
		select {
		case <-ch.doneCh:
			return
		case <-t.C:
			scanCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			_, _ = ch.hp.AppPeer.DiscoverPeers(scanCtx)
			cancel()
			// Reap entries the substrate hasn't refreshed within
			// DiscoveryCandidateMaxAge — keeps the candidate prefix
			// bounded (the entity hash embeds ObservedAt, so every
			// scan creates new paths) and prevents departed peers
			// from lingering in the Nearby list.
			_ = ch.hp.AppPeer.ReapStaleDiscoveredCandidates()
			t.Reset(scanInterval)
		}
	}
}

//export DiscoveryRegisterWake
func DiscoveryRegisterWake(h C.int64_t, cb unsafe.Pointer) *C.char {
	handle := int64(h)
	discoMu.Lock()
	ch, ok := discos[handle]
	discoMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown discovery handle"}`)
	}
	ch.wakeDoneCh = make(chan struct{})
	go func() {
		defer close(ch.wakeDoneCh)
		for {
			select {
			case <-ch.doneCh:
				return
			case <-ch.wakeCh:
				C.invoke_tree_wake_discovery(cb, C.int64_t(handle))
			}
		}
	}()
	return C.CString(`{"ok":true}`)
}

// DiscoveryRender returns the current snapshot of nearby peers. The
// snapshot is built by reading the substrate's candidate prefix from
// the store (NO live mDNS browse — that's scanLoop's job) and
// post-filtering against the workspace's connections map so
// already-connected peers are flagged distinctly. Entries are sorted
// by peer_id_hint for stable rendering.
//
//export DiscoveryRender
func DiscoveryRender(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("DiscoveryRender", &result)
	handle := int64(h)
	discoMu.Lock()
	ch, ok := discos[handle]
	discoMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown discovery handle"}`)
	}
	render := buildDiscoveryRender(ch.hp)
	b, err := json.Marshal(render)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(fmt.Sprintf(`{"ok":true,"result":%s}`, string(b)))
}

func buildDiscoveryRender(hp *shellboot.HostedPeer) wb.NearbyPeersRender {
	if hp == nil || hp.AppPeer == nil {
		return wb.NearbyPeersRender{}
	}
	cands := hp.AppPeer.ReadDiscoveredCandidates()
	// Build a peer-id → alias lookup over current connections so we can
	// mark already-connected entries.
	connectedPIDs := map[string]string{}
	if hp.Workspace != nil {
		for alias, pc := range hp.Workspace.Conns {
			connectedPIDs[pc.PeerID] = alias
		}
	}
	localPID := hp.AppPeer.PeerID()

	out := make([]wb.NearbyPeerEntry, 0, len(cands))
	for _, c := range cands {
		hint, decErr := entitysdk.DecodeMDNSEndpointHint(c.EndpointHint)
		if decErr != nil {
			continue
		}
		txt := parseTXTPairs(hint.Text)
		pidHint := txt["peer_id_hint"]
		if pidHint == "" {
			continue
		}
		if pidHint == localPID {
			continue
		}
		dialAddr := chooseDialAddr(hint, txt)
		_, alreadyConnected := connectedPIDs[pidHint]
		out = append(out, wb.NearbyPeerEntry{
			PeerID:    pidHint,
			DialURL:   dialAddr,
			Backend:   c.Backend,
			IsLocal:   false,
			Connected: alreadyConnected,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PeerID < out[j].PeerID
	})
	return wb.NearbyPeersRender{Entries: out}
}

// chooseDialAddr picks the URL form for AppPeer.Connect. The mDNS hint
// carries a proto TXT (key "proto") with comma-separated names per
// EXTENSION-DISCOVERY §3.2 — workbench v1 ships profile_refs "tcp" and
// "ws", which are also the protocol names. Prefer ws for browser
// interop; fall back to TCP otherwise.
//
// Host preference: a routable IPv4 from the hint beats the announced
// mDNS HostName, because the HostName is the canonical `.local.` form
// (`peer-host.lan.local.`) and resolving that requires nss-mdns /
// avahi-daemon on the dialing host. Falling back to HostName when no
// IPv4 was announced keeps loopback / IPv6-only LANs working. (IPv6
// fallback is third — many home routers fail IPv6 LAN reachability.)
func chooseDialAddr(hint entitysdk.MDNSEndpointHint, txt map[string]string) string {
	host := pickDialHost(hint)
	if host == "" || hint.Port == 0 {
		return ""
	}
	proto := txt["proto"]
	if proto == "ws" || proto == "wss" {
		return fmt.Sprintf("ws://%s:%d/ws", host, hint.Port)
	}
	return fmt.Sprintf("tcp://%s:%d", host, hint.Port)
}

// pickDialHost selects the most-dial-friendly host string from the
// hint: first non-empty IPv4 → first non-empty IPv6 → HostName.
func pickDialHost(hint entitysdk.MDNSEndpointHint) string {
	for _, ip := range hint.IPv4 {
		if ip != "" {
			return ip
		}
	}
	for _, ip := range hint.IPv6 {
		if ip != "" {
			// Bracket per net.Dial's host:port grammar.
			return "[" + ip + "]"
		}
	}
	return hint.HostName
}

// parseTXTPairs splits "key=value" entries (RFC 6763 §6 / §3.2). Used
// because we now decode the endpoint_hint into the full struct
// ourselves; the TXT lookup table that core-go's DecodeEndpointHint
// returned is no longer part of our decode path.
func parseTXTPairs(txt []string) map[string]string {
	out := make(map[string]string, len(txt))
	for _, t := range txt {
		i := -1
		for k, c := range t {
			if c == '=' {
				i = k
				break
			}
		}
		if i < 0 {
			continue
		}
		out[t[:i]] = t[i+1:]
	}
	return out
}

//export DiscoveryClose
func DiscoveryClose(h C.int64_t) {
	handle := int64(h)
	discoMu.Lock()
	ch, ok := discos[handle]
	if ok {
		delete(discos, handle)
	}
	discoMu.Unlock()
	if !ok {
		return
	}
	if ch.cancelEv != nil {
		ch.cancelEv()
	}
	close(ch.doneCh)
	if ch.wakeDoneCh != nil {
		<-ch.wakeDoneCh
	}
	if ch.scanDoneCh != nil {
		<-ch.scanDoneCh
	}
}

// cascadeDiscoveries tears down every discovery handle tagged with
// peer h. Registered as an OnPeerDestroyed hook in BridgeInit.
func cascadeDiscoveries(h int64) {
	discoMu.Lock()
	victims := []*discoveryHandle{}
	for id, ch := range discos {
		if ch.peerHandleID == h {
			victims = append(victims, ch)
			delete(discos, id)
		}
	}
	discoMu.Unlock()
	for _, ch := range victims {
		if ch.cancelEv != nil {
			ch.cancelEv()
		}
		close(ch.doneCh)
		if ch.wakeDoneCh != nil {
			<-ch.wakeDoneCh
		}
		if ch.scanDoneCh != nil {
			<-ch.scanDoneCh
		}
	}
}
