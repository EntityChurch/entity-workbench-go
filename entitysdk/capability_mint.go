package entitysdk

import (
	"fmt"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
)

// MintChainCapability mints a single-sig self-capability scoped to
// the supplied grants and persists the resulting cap entity + its
// signature in the local content store. The returned entity's
// content hash is suitable as the `dispatch_capability` on
// continuations the local peer installs.
//
// **Why not OwnerCapability?** The peer's owner cap is wildcard
// across all four grant dimensions (handlers / operations /
// resources / peers). Using it as `dispatch_capability` on a chain
// means any step in the chain — and any peer participating in
// cross-peer delivery — can act with the full peer's authority.
// That's drastic overreach for a chain that only needs to read
// specific paths and write into a specific prefix. Owner-cap is
// fine for development; chains in real deployments need scoped
// caps so the capability surface is exercised the way production
// peers depend on it.
//
// **Granter / grantee shape.** This helper mints a self-cap:
// granter and grantee are both the local peer's identity hash.
// The chain installer's identity (also local) appears in the cap's
// authority chain by construction, so the R1 creator-authorization
// check at continuation install time (EXTENSION-CONTINUATION §3.2
// step 4) passes.
//
// **Persistence and revocation.** The cap entity is content-
// addressed and persisted in the content store; its signature
// is persisted as a sibling entity. The helper does NOT bind the
// cap at a tree path — callers that want revocation support
// (V7 §5.1) should write the cap at a stable tree path so the
// `is_revoked` walk can find the root. Recommended path
// convention: `system/capability/grants/chain/{chain-id}`. For
// short-lived chains where the cap is delivered alongside each
// EXECUTE and never persisted at a tree path, the cap's content-
// store row is sufficient and revocation is by entity-store
// removal (out of scope for v1).
//
// **CreatedAt non-determinism.** The cap embeds `CreatedAt` per
// CapabilityTokenData's required fields. Each mint produces a
// distinct cap. This is a known issue tracked in
// PROPOSAL-RESTART-EQUIVALENCE §5.4 (static-grant / runtime-token
// split). For chain caps it's bounded: the cap is minted ONCE per
// chain install, persisted to the content store, and referenced
// thereafter by hash. Peer restart does not re-mint — the same
// cap entity is read back. So this helper doesn't contribute to
// the per-restart leak described in §5.4.
func (a *AppPeer) MintChainCapability(grants []types.GrantEntry) (entity.Entity, error) {
	if len(grants) == 0 {
		return entity.Entity{}, NewError(400, "invalid_grants",
			"MintChainCapability requires at least one grant")
	}
	id := a.peer.Identity()
	tok := types.CapabilityTokenData{
		Grants:    grants,
		Granter:   types.SingleSigGranter(id.ContentHash),
		Grantee:   id.ContentHash,
		CreatedAt: uint64(time.Now().UnixMilli()),
	}
	if err := tok.ValidateStructure(); err != nil {
		return entity.Entity{}, WrapError(400, "invalid_token", "validate cap structure", err)
	}
	capEnt, err := tok.ToEntity()
	if err != nil {
		return entity.Entity{}, WrapError(500, "encode_cap", "encode capability token", err)
	}
	cs := a.peer.Store()
	if _, err := cs.Put(capEnt); err != nil {
		return entity.Entity{}, WrapError(500, "persist_cap", "persist capability token", err)
	}
	// Sign the cap with the local peer's keypair so chain-walk
	// validators can resolve a sibling signature reference.
	kp := a.peer.Keypair()
	sig := kp.Sign(capEnt.ContentHash.Bytes())
	sigData := types.SignatureData{
		Target:    capEnt.ContentHash,
		Signer:    id.ContentHash,
		Algorithm: "ed25519",
		Signature: sig,
	}
	sigEnt, err := sigData.ToEntity()
	if err != nil {
		return entity.Entity{}, WrapError(500, "encode_signature",
			"encode capability signature", err)
	}
	if _, err := cs.Put(sigEnt); err != nil {
		return entity.Entity{}, WrapError(500, "persist_signature",
			"persist capability signature", err)
	}
	return capEnt, nil
}

// MintChainCapabilityBound is the bound variant: mints the cap and
// also writes it at the given tree path so revocation walks
// (V7 §5.1) can find the root. Use this for chains whose dispatch
// caps are long-lived (Phase E mount caps, Phase C follow caps).
//
// The convention `system/capability/grants/chain/{chain-id}` keeps
// chain caps under a discoverable subtree; the path is opaque to
// the capability system itself (V7 §5.1 calls it "implementation-
// defined" for non-handler-grant capabilities).
func (a *AppPeer) MintChainCapabilityBound(grants []types.GrantEntry, treePath string) (entity.Entity, error) {
	if treePath == "" {
		return entity.Entity{}, NewError(400, "invalid_path",
			"MintChainCapabilityBound requires a non-empty tree path")
	}
	capEnt, err := a.MintChainCapability(grants)
	if err != nil {
		return entity.Entity{}, err
	}
	// Bind via PutEntity so the location index is updated and
	// downstream observers see the binding event.
	if _, err := a.PutEntity(treePath, capEnt); err != nil {
		return entity.Entity{}, WrapError(500, "bind_cap",
			fmt.Sprintf("bind capability at %s", treePath), err)
	}
	return capEnt, nil
}
