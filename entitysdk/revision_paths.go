package entitysdk

import (
	"encoding/hex"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"

	"github.com/fxamacker/cbor/v2"
)

// RevisionHeadPath returns the tree path where the revision handler
// stores the head pointer for prefix on peerID. The path is
// `system/revision/{prefix-hash}/head` per EXTENSION-REVISION v2.1
// + handler.go:155. Use it to build subscription patterns that
// trigger on head advances (e.g. for revision-follow chains).
//
// prefix may be passed in peer-relative form ("docs/") or
// absolute form ("/{peerID}/docs/"); RevisionHeadPath normalizes to
// absolute before hashing so the result matches the handler-side
// computation.
func RevisionHeadPath(peerID, prefix string) string {
	absolute := prefix
	if !strings.HasPrefix(prefix, "/") {
		absolute = "/" + peerID + "/" + prefix
	}
	data, _ := ecf.Encode(absolute)
	h, _ := hash.Compute("system/tree/path", cbor.RawMessage(data))
	return "system/revision/" + hex.EncodeToString(h.Bytes()) + "/head"
}
