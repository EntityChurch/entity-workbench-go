package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/identity"
)

// IdentityClient wraps system/identity EXECUTE operations behind typed
// Go methods. Per SDK-IDENTITY-INFRASTRUCTURE §6.
//
// Note on first :configure (§4.1 L0 bootstrap exemption): on a fresh
// peer with no local peer→controller cap, the first configure call
// MUST go through the L0 in-process Startup path, not through
// dispatched EXECUTE. Use BootstrapIdentity (Cut 2c — landing in a
// follow-up push) for that step. Configure on this client targets the
// post-startup dispatched path; it expects a peer→controller cap to
// already exist and will return 503 authority_not_ready otherwise.
type IdentityClient struct {
	ap          *AppPeer
	target      string
	identityURI string
}

// Identity returns an IdentityClient targeting the local peer.
func (a *AppPeer) Identity() *IdentityClient {
	return &IdentityClient{
		ap:          a,
		target:      a.PeerID(),
		identityURI: "system/identity",
	}
}

// IdentityAt returns an IdentityClient targeting a remote peer
// (must be reachable via Connect or RegisterRemote).
func (a *AppPeer) IdentityAt(peerID string) *IdentityClient {
	return &IdentityClient{
		ap:          a,
		target:      peerID,
		identityURI: extPeerURI(a.PeerID(), peerID, "system/identity"),
	}
}

// PeerID returns the peer-id this IdentityClient targets.
func (ic *IdentityClient) PeerID() string { return ic.target }

// Configure runs the §6.1 configure ceremony on the target peer:
// installs peer-config, locates live controller-cert attestations
// under the trusted quorum, verifies their K-of-N signatures, and
// issues local peer→controller caps.
//
// First-call constraint: on a peer that has never been configured,
// callers MUST use BootstrapIdentity instead — the dispatched path
// requires a peer→controller cap that doesn't exist yet. Subsequent
// :configure calls (re-configure, multi-controller addition) work
// through this dispatched form.
func (ic *IdentityClient) Configure(
	ctx context.Context,
	req types.IdentityConfigureRequestData,
) (types.IdentityConfigureResultData, error) {
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentityConfigureResultData{}, WrapError(500, "encode_request",
			"encode IdentityConfigureRequest", err)
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "configure",
		identity.PeerConfigPath, ent)
	if err != nil {
		return types.IdentityConfigureResultData{}, err
	}
	out, err := types.IdentityConfigureResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentityConfigureResultData{}, WrapError(500, "decode_result",
			"decode IdentityConfigureResult", err)
	}
	return out, nil
}

// CreateQuorum mints a system/quorum entity on the target peer.
// Delegates to system/quorum:create internally and additionally
// records the resulting quorum-id in the identity peer-config when
// the caller is configured for it.
func (ic *IdentityClient) CreateQuorum(
	ctx context.Context,
	signers []hash.Hash,
	threshold uint64,
	name string,
) (types.IdentityCreateQuorumResultData, error) {
	q := types.QuorumData{
		Signers:   signers,
		Threshold: threshold,
		Name:      name,
	}
	req := types.IdentityCreateQuorumRequestData{QuorumData: q}
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentityCreateQuorumResultData{}, WrapError(500, "encode_request",
			"encode IdentityCreateQuorumRequest", err)
	}
	qEnt, err := q.ToEntity()
	if err != nil {
		return types.IdentityCreateQuorumResultData{}, WrapError(500, "encode_quorum",
			"compute quorum hash", err)
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "create_quorum",
		identity.QuorumPathFor(qEnt.ContentHash), ent)
	if err != nil {
		return types.IdentityCreateQuorumResultData{}, err
	}
	out, err := types.IdentityCreateQuorumResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentityCreateQuorumResultData{}, WrapError(500, "decode_result",
			"decode IdentityCreateQuorumResult", err)
	}
	return out, nil
}

// CreateAttestation mints an identity-context attestation. The
// handler dispatches per kind/function/mode (§5.3) to derive the
// canonical storage path. For mode=embedded, no tree write happens —
// the result carries EmbeddedAttestation instead of AttestationHash.
//
// K-of-N signatures (when the attestation is anchored under a
// multi-sig quorum) must be written separately as system/signature
// entities at /{signer_peer_id}/system/signature/{att_hex} per
// EXTENSION-ATTESTATION v1.1 §4.0. The proto-SDK doesn't abstract
// signature orchestration; neither does this wrapper.
func (ic *IdentityClient) CreateAttestation(
	ctx context.Context,
	att types.AttestationData,
) (types.IdentityCreateAttestationResultData, error) {
	req := types.IdentityCreateAttestationRequestData{AttestationData: att}
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentityCreateAttestationResultData{}, WrapError(500, "encode_request",
			"encode IdentityCreateAttestationRequest", err)
	}
	// Compute the canonical path; embedded mode returns "" → no resource.
	path := ""
	if p, perr := identity.CanonicalCertPath(att); perr == nil {
		path = p
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "create_attestation", path, ent)
	if err != nil {
		return types.IdentityCreateAttestationResultData{}, err
	}
	out, err := types.IdentityCreateAttestationResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentityCreateAttestationResultData{}, WrapError(500, "decode_result",
			"decode IdentityCreateAttestationResult", err)
	}
	return out, nil
}

// SupersedeAttestation mints a successor identity-context attestation
// per §6. The new attestation MUST set Supersedes to the prior
// attestation's content hash; the handler validates the kind matches.
func (ic *IdentityClient) SupersedeAttestation(
	ctx context.Context,
	newAtt types.AttestationData,
) (types.IdentitySupersedeAttestationResultData, error) {
	req := types.IdentitySupersedeAttestationRequestData{AttestationData: newAtt}
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentitySupersedeAttestationResultData{}, WrapError(500, "encode_request",
			"encode IdentitySupersedeAttestationRequest", err)
	}
	path := ""
	if p, perr := identity.CanonicalCertPath(newAtt); perr == nil {
		path = p
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "supersede_attestation", path, ent)
	if err != nil {
		return types.IdentitySupersedeAttestationResultData{}, err
	}
	out, err := types.IdentitySupersedeAttestationResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentitySupersedeAttestationResultData{}, WrapError(500, "decode_result",
			"decode IdentitySupersedeAttestationResult", err)
	}
	return out, nil
}

// RevokeAttestation produces a revocation attestation targeting an
// identity-context attestation per §6. The revocation is itself a
// system/attestation entity with kind="revocation". Callers must
// K-of-N sign it under the quorum's threshold for it to take effect
// on liveness; an unsigned revocation has no effect.
func (ic *IdentityClient) RevokeAttestation(
	ctx context.Context,
	targetHash hash.Hash,
	reason string,
) (types.IdentityRevokeAttestationResultData, error) {
	req := types.IdentityRevokeAttestationRequestData{TargetHash: targetHash, Reason: reason}
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentityRevokeAttestationResultData{}, WrapError(500, "encode_request",
			"encode IdentityRevokeAttestationRequest", err)
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "revoke_attestation", "", ent)
	if err != nil {
		return types.IdentityRevokeAttestationResultData{}, err
	}
	out, err := types.IdentityRevokeAttestationResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentityRevokeAttestationResultData{}, WrapError(500, "decode_result",
			"decode IdentityRevokeAttestationResult", err)
	}
	return out, nil
}

// PublishAttestation promotes/demotes a function=agent identity-cert
// across publication modes (internal / public / per-relationship).
// ContactID is required when newMode == "per-relationship".
func (ic *IdentityClient) PublishAttestation(
	ctx context.Context,
	attHash hash.Hash,
	newMode string,
	contactID *hash.Hash,
) (types.IdentityPublishAttestationResultData, error) {
	req := types.IdentityPublishAttestationRequestData{
		AttestationHash: attHash,
		NewMode:         newMode,
		ContactID:       contactID,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.IdentityPublishAttestationResultData{}, WrapError(500, "encode_request",
			"encode IdentityPublishAttestationRequest", err)
	}
	path := ""
	switch newMode {
	case types.ModeInternal:
		path = identity.InternalCertPath(attHash)
	case types.ModePublic:
		path = identity.PublicCertPath(attHash)
	case types.ModePerRelationship:
		if contactID != nil && !contactID.IsZero() {
			path = identity.RelationshipCertPath(*contactID, attHash)
		}
	}
	resultEnt, err := extDispatch(ic.ap, ic.identityURI, "publish_attestation", path, ent)
	if err != nil {
		return types.IdentityPublishAttestationResultData{}, err
	}
	out, err := types.IdentityPublishAttestationResultDataFromEntity(resultEnt)
	if err != nil {
		return types.IdentityPublishAttestationResultData{}, WrapError(500, "decode_result",
			"decode IdentityPublishAttestationResult", err)
	}
	return out, nil
}
