// Package fetch is the consumer half of the CDN release corridor.
//
// First form. Given a base URL + peer-id + path, fetch
// the published bundle's two-step indirection (tree binding → content
// blob), verify the content hash, decode the entity.
//
// What's deferred and tagged: signed manifest verification (publish
// doesn't emit one yet); substitute-source chain traversal (one
// origin only); subscriber notification on fetched content (this is
// a read-once tool, not a peer); transitive closure (only fetches
// the entity at the requested path, not its references).
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// Opts configures a Fetch call.
type Opts struct {
	BaseURL string // e.g. http://localhost:8000
	PeerID  string // the publisher's peer-id (base58)
	Path    string // tree path, no leading slash, no .bin suffix
	Client  *http.Client
}

// Result bundles the resolved hash and decoded entity.
type Result struct {
	Hash     hash.Hash
	Entity   entity.Entity
	TreeURL  string
	BlobURL  string
	TreeSize int
	BlobSize int
}

// Fetch performs the two-step indirection and verifies the hash.
//
//  1. GET {base}/{peer-id}/tree/{path}.bin → 33-byte hash wire form
//  2. Parse hash, shard the digest
//  3. GET {base}/content/{first-2-hex}/{rest} → ECF-encoded entity
//  4. ecf.Decode the entity
//  5. Validate hash.Compute(type, data) == hash from step 2
//
// Returns the decoded entity. The hash check guarantees byte
// fidelity — same as the SDK's content-addressed reads.
func Fetch(ctx context.Context, opts Opts) (Result, error) {
	if opts.BaseURL == "" || opts.PeerID == "" {
		return Result{}, fmt.Errorf("fetch: BaseURL and PeerID required")
	}
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	path := strings.TrimLeft(opts.Path, "/")

	treeURL := fmt.Sprintf("%s/%s/tree/%s.bin", base, opts.PeerID, path)
	treeBytes, err := httpGet(ctx, client, treeURL)
	if err != nil {
		return Result{}, fmt.Errorf("fetch tree: %w", err)
	}
	h, err := hash.FromBytes(treeBytes)
	if err != nil {
		return Result{}, fmt.Errorf("fetch: parse hash from tree binding: %w", err)
	}

	hex := hashHex(h)
	blobURL := fmt.Sprintf("%s/content/%s/%s", base, hex[:2], hex[2:])
	blobBytes, err := httpGet(ctx, client, blobURL)
	if err != nil {
		return Result{}, fmt.Errorf("fetch content: %w", err)
	}

	var ent entity.Entity
	if err := ecf.Decode(blobBytes, &ent); err != nil {
		return Result{}, fmt.Errorf("fetch: decode entity: %w", err)
	}
	if err := ent.Validate(); err != nil {
		return Result{}, fmt.Errorf("fetch: entity validate: %w", err)
	}
	if ent.ContentHash != h {
		return Result{}, fmt.Errorf("fetch: hash mismatch — tree says %s, content is %s",
			h, ent.ContentHash)
	}

	return Result{
		Hash:     h,
		Entity:   ent,
		TreeURL:  treeURL,
		BlobURL:  blobURL,
		TreeSize: len(treeBytes),
		BlobSize: len(blobBytes),
	}, nil
}

func httpGet(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func hashHex(h hash.Hash) string {
	const hexchars = "0123456789abcdef"
	digest := h.Digest[:]
	out := make([]byte, len(digest)*2)
	for i, b := range digest {
		out[i*2] = hexchars[b>>4]
		out[i*2+1] = hexchars[b&0x0f]
	}
	return string(out)
}
