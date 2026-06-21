// entity-serve-cors — minimal static file server with CORS headers.
//
// Why this exists: `entity-publish -origin URL` emits an Amendment-5
// static dir intended to be fetched from a browser-running consumer
// (egui-rust, the DOM impl). Browsers enforce CORS on cross-origin
// fetch(); Python's `http.server` doesn't emit `Access-Control-Allow-
// Origin`, so the consumer's resolver fails on every request. This
// tiny binary serves a directory exactly like `http.server` but with
// `Access-Control-Allow-Origin: *` and `Access-Control-Allow-Methods:
// GET, HEAD, OPTIONS` on every response. It also handles preflight
// OPTIONS requests so a browser fetch with custom headers (none yet,
// but reserved) doesn't trip.
//
// Not a long-term answer: a real CDN deployment terminates CORS at
// the edge. This is the "make the cross-impl proof go" tool.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", "./publish-out", "directory to serve")
	addr := flag.String("addr", ":8080", "listen address (e.g. :8080 or 0.0.0.0:8080)")
	quiet := flag.Bool("quiet", false, "suppress per-request logging")
	flag.Parse()

	abs, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "entity-serve-cors: resolve dir: %v\n", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "entity-serve-cors: not a directory: %s\n", abs)
		os.Exit(1)
	}

	fileServer := http.FileServer(http.Dir(abs))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS — open egress for any browser-running consumer. The
		// published dir is static-immutable content; nothing here
		// warrants origin-pinning at the static layer.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		// Preflight — short-circuit before hitting the file server.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !*quiet {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		fileServer.ServeHTTP(w, r)
	})

	host, port, _ := net.SplitHostPort(*addr)
	if host == "" {
		host = "0.0.0.0"
	}
	log.Printf("serving %s at http://%s:%s (CORS: *)", abs, host, port)

	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "entity-serve-cors: listen: %v\n", err)
		os.Exit(1)
	}
}
