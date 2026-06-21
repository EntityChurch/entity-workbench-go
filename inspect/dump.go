package inspect

// Entity browser + location browse — given a path or hash, return a
// human-readable decode of the entity; given a substring, enumerate
// path bindings.

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

// EntityDump decodes an entity at a path or hash into a human-
// readable form.
type EntityDump struct {
	Path string
	Type string
	Hash string
	Len  int
	Data interface{}
}

// DumpEntityAt returns a decoded entity dump for the entity bound at
// path on peer, or nil if no binding exists. Accepts either fully-
// qualified paths (`/<peerid>/foo`) or local-peer-relative paths
// (`foo`) — local-relative is namespaced to the local peer before
// lookup. NamespacedIndex.Get panics on malformed paths; recovered
// and returned as nil for operator surfaces.
func DumpEntityAt(peer *entitysdk.AppPeer, path string) (out *EntityDump) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
		}
	}()
	// Try forms in order: (1) as-given (caller passed fully qualified
	// /<peerid>/foo), (2) local-namespaced (caller passed bare
	// "inspect-test/marker"), (3) abs-with-leading-slash that's not
	// actually peer-namespaced (caller passed "/inspect-test/marker"
	// from sh.Resolve against root WD). NamespacedIndex.Get panics on
	// malformed paths, hence the recover above; we fall through to
	// the next form.
	lookup := path
	if len(path) == 0 || path[0] != '/' {
		lookup = "/" + peer.PeerID() + "/" + path
	} else if !pathLooksNamespaced(path, peer.PeerID()) {
		lookup = "/" + peer.PeerID() + path
	}
	h, ok := peer.RawLocationIndex().Get(lookup)
	if !ok {
		return nil
	}
	ent, ok := peer.Store().GetByHash(h)
	if !ok {
		return &EntityDump{Path: lookup, Hash: h.String(), Type: "(no entity at hash)"}
	}
	d := &EntityDump{
		Path: lookup,
		Type: ent.Type,
		Hash: ent.ContentHash.String(),
		Len:  len(ent.Data),
	}
	_ = cbor.Unmarshal(ent.Data, &d.Data)
	return d
}

// DumpEntityByHash returns a decoded entity dump for the entity at
// hash on peer, or nil if not present.
func DumpEntityByHash(peer *entitysdk.AppPeer, h hash.Hash) *EntityDump {
	ent, ok := peer.Store().GetByHash(h)
	if !ok {
		return nil
	}
	d := &EntityDump{
		Type: ent.Type,
		Hash: ent.ContentHash.String(),
		Len:  len(ent.Data),
	}
	_ = cbor.Unmarshal(ent.Data, &d.Data)
	return d
}

// FindChainErrors enumerates the chain-error-lost markers under
// system/runtime/chain-errors/ for diagnostic surface. Returns
// LocationEntries sorted by path order from the store.
func FindChainErrors(peer *entitysdk.AppPeer) []store.LocationEntry {
	var out []store.LocationEntry
	for _, e := range peer.Store().List("") {
		if strings.Contains(e.Path, "system/runtime/chain-errors") {
			out = append(out, e)
		}
	}
	return out
}

// FindUnder returns all path bindings whose paths contain substr.
// Generic browse facility for diagnostic surface.
func FindUnder(peer *entitysdk.AppPeer, substr string) []store.LocationEntry {
	var out []store.LocationEntry
	for _, e := range peer.Store().List("") {
		if strings.Contains(e.Path, substr) {
			out = append(out, e)
		}
	}
	return out
}

// PrettyPrint returns a human-readable rendering of a Capture
// suitable for test log output. Decoded payloads are formatted as
// indented CBOR-map structures with byte fields hex-rendered.
func PrettyPrint(c Capture) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  capture #%d (seq=%d) at %s\n",
		c.Seq, c.Seq, c.Timestamp.Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "    path=%s op=%s request_id=%s\n",
		c.Path, c.Operation, c.RequestID)
	fmt.Fprintf(&b, "    params.type=%q  params.hash=%s  params.len=%d\n",
		c.ParamsType, c.ParamsHash.String(), len(c.ParamsData))
	if c.ParamsDecoded != nil {
		fmt.Fprintf(&b, "    params.decoded=%s\n", RenderCBOR(c.ParamsDecoded, 0))
	}
	if c.IsDelivery {
		fmt.Fprintf(&b, "    delivery.status=%d  delivery.request=%s  delivery.result.len=%d\n",
			c.DeliveryStatus, c.DeliveryRequest, len(c.DeliveryResultRM))
		if c.DeliveryResult != nil {
			fmt.Fprintf(&b, "    delivery.result.decoded=%s\n",
				RenderCBOR(c.DeliveryResult, 0))
		}
	}
	return b.String()
}

// RenderCBOR produces a compact human rendering of a CBOR-decoded
// value. Maps become {key: value, ...}; byte slices become
// truncated hex; nested values get indented.
func RenderCBOR(v interface{}, depth int) string {
	if depth > 6 {
		return "..."
	}
	indent := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case map[interface{}]interface{}:
		var b strings.Builder
		b.WriteString("{\n")
		for k, vv := range x {
			fmt.Fprintf(&b, "%s  %v: %s\n", indent, k, RenderCBOR(vv, depth+1))
		}
		fmt.Fprintf(&b, "%s}", indent)
		return b.String()
	case map[string]interface{}:
		var b strings.Builder
		b.WriteString("{\n")
		for k, vv := range x {
			fmt.Fprintf(&b, "%s  %s: %s\n", indent, k, RenderCBOR(vv, depth+1))
		}
		fmt.Fprintf(&b, "%s}", indent)
		return b.String()
	case []byte:
		if len(x) <= 8 {
			return fmt.Sprintf("0x%s", hex.EncodeToString(x))
		}
		return fmt.Sprintf("0x%s...(%d bytes)", hex.EncodeToString(x[:8]), len(x))
	case string:
		return fmt.Sprintf("%q", x)
	case []interface{}:
		if len(x) == 0 {
			return "[]"
		}
		var b strings.Builder
		b.WriteString("[\n")
		for _, vv := range x {
			fmt.Fprintf(&b, "%s  %s\n", indent, RenderCBOR(vv, depth+1))
		}
		fmt.Fprintf(&b, "%s]", indent)
		return b.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// pathLooksNamespaced reports whether path's first segment after the
// leading "/" matches the local peer's id. Used by DumpEntityAt to
// disambiguate "/foo/bar" (intended local-relative, sh.Resolve added
// the leading slash) from "/<peerid>/foo/bar" (intended fully
// qualified).
func pathLooksNamespaced(path, localPeerID string) bool {
	if len(path) == 0 || path[0] != '/' {
		return false
	}
	rest := path[1:]
	end := strings.IndexByte(rest, '/')
	if end < 0 {
		return rest == localPeerID
	}
	return rest[:end] == localPeerID
}
