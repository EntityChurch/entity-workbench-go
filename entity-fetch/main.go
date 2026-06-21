// entity-fetch — minimal consumer half of the CDN release corridor.
//
// Given the URL of a published bundle (the directory entity-publish
// emitted), a peer-id, and a tree path, fetch the entity at that
// path through the two-step content-addressed indirection and verify
// the hash.
//
// First form: one path per invocation, no transitive closure, no
// substitute-source chain, no signed manifest verification (publish
// doesn't emit one yet). Just enough to prove the loop closes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"go.entitychurch.org/entity-core-go/core/ecf"

	"entity-workbench-go/fetch"
)

const usage = `Usage:
  entity-fetch -base URL -peer-id ID -path PATH [-decode]

Flags:
  -base URL       Base URL of the published bundle (e.g. http://localhost:8000)
  -peer-id ID     Publisher's peer-id (base58)
  -path PATH      Tree path to fetch (e.g. wt/docs/foo.md)
  -decode         Best-effort decode the entity's data as JSON-ish for display
`

func main() {
	base := flag.String("base", "", "base URL")
	peerID := flag.String("peer-id", "", "publisher peer-id")
	path := flag.String("path", "", "tree path")
	decode := flag.Bool("decode", false, "best-effort decode for display")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *base == "" || *peerID == "" || *path == "" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	res, err := fetch.Fetch(context.Background(), fetch.Opts{
		BaseURL: *base,
		PeerID:  *peerID,
		Path:    *path,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "entity-fetch: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("path:     %s\n", *path)
	fmt.Printf("tree-url: %s (%d bytes)\n", res.TreeURL, res.TreeSize)
	fmt.Printf("hash:     %s\n", res.Hash)
	fmt.Printf("blob-url: %s (%d bytes)\n", res.BlobURL, res.BlobSize)
	fmt.Printf("type:     %s\n", res.Entity.Type)
	fmt.Printf("verified: yes (entity.ContentHash matches tree binding)\n")

	if *decode {
		var v any
		if err := ecf.Decode(res.Entity.Data, &v); err != nil {
			fmt.Printf("decode:   <failed: %v>\n", err)
			return
		}
		b, err := json.MarshalIndent(stringifyKeys(v), "", "  ")
		if err != nil {
			fmt.Printf("decode:   <marshal failed: %v>\n", err)
			return
		}
		fmt.Printf("decode:\n%s\n", b)
	}
}

// stringifyKeys converts CBOR-decoded map[interface{}]interface{}
// values into map[string]any so encoding/json can marshal them. We
// don't need a faithful CBOR-to-JSON because this is display-only —
// callers that need byte fidelity use the entity bytes directly.
func stringifyKeys(v any) any {
	switch t := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprintf("%v", k)] = stringifyKeys(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = stringifyKeys(val)
		}
		return out
	default:
		return v
	}
}
