package fetch

import (
	"net/http"
	"time"
)

// client.go — the transport this package actually wants, and the default
// that quietly undoes a concurrent walk.
//
// Go's `http.DefaultTransport` keeps **two** idle connections per host
// (`MaxIdleConnsPerHost: 2`). A trie walk running [WalkConcurrency] = 8
// fetches at a time against one origin therefore opens eight
// connections, returns six of them to a pool that will not hold them,
// and **re-does a TLS handshake for six of the next eight**. The
// concurrency is real and most of what it buys is spent again on
// handshakes.
//
// Measured on the live federation opening billslab.com (51 CHAMP nodes,
// 61 requests): serial walk 7.3 s → concurrent walk on the stock
// transport 4.2 s → concurrent walk with the pool sized to match 1.4 s.
// The last step is one struct field and it is worth more than the
// goroutines are.
//
// Nothing here changes what is fetched or what is verified. It is a
// connection pool.

// NewHTTPClient builds the client this package's concurrency assumes.
//
// `timeout` <= 0 means 30 s. The timeout is per request, not per
// navigation — a walk of a large trie is many requests and bounding the
// whole traversal from here would turn a big site into a transport
// error.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Sized to the walk's fan-out, with headroom for the page fetches
	// that follow it on the same origin.
	tr.MaxIdleConnsPerHost = WalkConcurrency * 2
	tr.MaxConnsPerHost = WalkConcurrency * 2
	tr.IdleConnTimeout = 90 * time.Second
	return &http.Client{Transport: tr, Timeout: timeout}
}
