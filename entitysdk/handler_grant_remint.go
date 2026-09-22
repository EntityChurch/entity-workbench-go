package entitysdk

import (
	"bytes"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// handler_grant_remint.go — A HANDLER GRANT IS INSTALL-ONCE, SO CHANGING A
// MANIFEST DOES NOTHING TO A PEER THAT ALREADY EXISTS.
//
// # Why this is here
//
// `createHandlerGrants` (core/peer/peer.go) mints one self-capability per
// registered handler from its manifest's `internal_scope`, binds it at
// `system/capability/grants/{pattern}`, and **skips any pattern whose
// grant is already bound.** Its own comment says the consequence out
// loud: *"a change to defaultHandlerSelfGrant reaches NEW peers only. A
// persistent store minted before the change keeps the grant it was
// installed with; re-shaping those is a migration, not a code fix."*
//
// That is correct kernel behaviour — a handler grant is Class I, canonical
// for the peer's lifetime under an identity, and re-minting it on every
// launch would churn content hashes for nothing. It is also a trap with a
// very specific shape: **every test in this repo runs on a memory store,
// which has no grant at the path and therefore always mints fresh.** So a
// manifest fix is green everywhere in CI and inert on every machine an
// operator actually runs. The gap is not in the code, it is between the
// code and the installed base, and only a migration crosses it.
//
// This became load-bearing on 2026-09-10, when the kernel made the
// executing handler's grant the gate on outbound sub-dispatch (0.8.2.19
// Delta E1). Before that, a handler's internal scope was never consulted
// for a cross-peer dispatch at all, so nobody had ever needed to change
// one on a live peer.
//
// # Why re-minting is safe, and where the line is
//
// A handler grant is SELF-granted: granter and grantee are both this
// peer's identity. Re-minting it therefore asks nobody for anything and
// escalates nothing that the manifest does not already declare — the
// manifest is the authority, and this only makes the installed grant equal
// to it. What it must never become is a way to widen a grant from outside
// that declaration, which is why the API takes the grants and the caller
// is expected to pass the handler's own manifest scope rather than an
// ad-hoc set. `workbench.MigrateBlobResolveGrant` is the worked example:
// one exported scope function, read by both the manifest and the
// migration, so the two cannot drift.

// RemintHandlerGrant makes the installed handler grant at
// `system/capability/grants/{pattern}` equal to `grants`, and reports
// whether it had to change anything.
//
// IDEMPOTENT BY CONTENT, not by a flag or a version marker. It encodes
// both grant sets canonically and compares the bytes; equal means return
// (false, nil) having touched nothing. That matters more than it looks:
// the mint embeds a `CreatedAt`, so a migration that re-mints
// unconditionally produces a new content hash at every launch, and a peer
// whose handler-grant hash changes on every start is a peer whose
// capability chain roots move under anything that recorded one.
//
// CreatedAt is pinned to 0, which is what `createHandlerGrants` uses and
// for its reason: handler grants have no TTL and are never consulted for
// time-based validation, so a real timestamp would buy nothing and cost
// determinism.
func (a *AppPeer) RemintHandlerGrant(pattern string, grants []types.GrantEntry) (bool, error) {
	if a == nil || a.peer == nil {
		return false, NewError(500, "no_peer", "RemintHandlerGrant needs a peer")
	}
	if pattern == "" {
		return false, NewError(400, "invalid_pattern",
			"RemintHandlerGrant requires a handler pattern")
	}
	if len(grants) == 0 {
		// A REFUSAL. An empty scope is not "no change wanted", it is a
		// grant that authorizes nothing — and installing one would
		// silently disable the handler rather than fix it.
		return false, NewError(400, "invalid_grants",
			"RemintHandlerGrant requires at least one grant entry — an empty "+
				"scope would disable the handler, which is never what a migration means")
	}

	grantPath := "system/capability/grants/" + pattern

	want, err := ecf.Encode(grants)
	if err != nil {
		return false, WrapError(500, "encode_grants", "encode desired handler scope", err)
	}

	// Read what is installed. A peer with nothing at the path is one the
	// kernel just minted for (a memory store, or a first run), and there
	// is nothing to migrate — but we do not assume that: if the path is
	// unbound we mint, because a handler with no grant cannot dispatch at
	// all and leaving it unbound is strictly worse.
	if installed, ok := a.installedHandlerGrants(grantPath); ok {
		have, herr := ecf.Encode(installed)
		if herr == nil && bytes.Equal(have, want) {
			return false, nil
		}
	}

	tok := types.CapabilityTokenData{
		Grants:    grants,
		Granter:   types.SingleSigGranter(a.peer.Identity().ContentHash),
		Grantee:   a.peer.Identity().ContentHash,
		CreatedAt: 0,
	}
	if verr := tok.ValidateStructure(); verr != nil {
		return false, WrapError(400, "invalid_token", "validate handler grant", verr)
	}
	capEnt, cerr := tok.ToEntity()
	if cerr != nil {
		return false, WrapError(500, "encode_cap", "encode handler grant", cerr)
	}

	// The signature has to be BOUND, not merely stored. Dispatch-time
	// validation resolves it from the grant's content hash through
	// types.LocalSignaturePath (v7.74 §3.4), so a grant whose signature
	// lives only in the content store fails validation and the handler
	// loses every dispatch — a strictly worse state than the one being
	// migrated from.
	kp := a.peer.Keypair()
	sigEnt, serr := types.SignatureData{
		Target:    capEnt.ContentHash,
		Signer:    a.peer.Identity().ContentHash,
		Algorithm: "ed25519",
		Signature: kp.Sign(capEnt.ContentHash.Bytes()),
	}.ToEntity()
	if serr != nil {
		return false, WrapError(500, "encode_signature", "encode handler grant signature", serr)
	}

	// Signature first, then the grant. If the process dies between the
	// two, the peer keeps the OLD grant (still bound, still valid) plus an
	// orphan signature entity — recoverable and harmless. The other order
	// leaves a bound grant whose signature is not yet resolvable, which
	// takes the handler down completely.
	if _, err := a.PutEntity(types.LocalSignaturePath(capEnt.ContentHash), sigEnt); err != nil {
		return false, WrapError(500, "bind_signature",
			"bind handler grant signature for "+pattern, err)
	}
	if _, err := a.PutEntity(grantPath, capEnt); err != nil {
		return false, WrapError(500, "bind_grant", "bind handler grant at "+grantPath, err)
	}
	return true, nil
}

// installedHandlerGrants decodes the grant entries currently bound at a
// handler-grant path. Reports ok=false when nothing is bound there or the
// binding does not decode as a capability token — both of which mean "we
// cannot claim the installed scope matches", which is the safe answer
// because it leads to a re-mint rather than to a silent skip.
func (a *AppPeer) installedHandlerGrants(grantPath string) ([]types.GrantEntry, bool) {
	st := a.peer.Store()
	if st == nil {
		return nil, false
	}
	li := a.peer.LocationIndex()
	if li == nil {
		return nil, false
	}
	h, ok := li.Get(grantPath)
	if !ok {
		return nil, false
	}
	ent, ok := st.Get(h)
	if !ok {
		return nil, false
	}
	tok, err := types.CapabilityTokenDataFromEntity(ent)
	if err != nil {
		return nil, false
	}
	return tok.Grants, true
}

// HandlerGrantScope reports the grant entries currently installed for a
// handler pattern, for a surface or a test that needs to say what a peer's
// authority actually IS rather than what its manifest declares.
//
// The two answer different questions and the distance between them is the
// whole reason this file exists.
func (a *AppPeer) HandlerGrantScope(pattern string) ([]types.GrantEntry, error) {
	if a == nil || a.peer == nil {
		return nil, NewError(500, "no_peer", "HandlerGrantScope needs a peer")
	}
	grants, ok := a.installedHandlerGrants("system/capability/grants/" + pattern)
	if !ok {
		return nil, NewError(404, "not_found",
			fmt.Sprintf("no handler grant installed at system/capability/grants/%s", pattern))
	}
	return grants, nil
}
