package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// AttestationClient wraps system/attestation EXECUTE operations
// behind typed Go methods. Per SDK-IDENTITY-INFRASTRUCTURE §5.1 and
// EXTENSION-ATTESTATION §6.
//
// Attestations are the signed-graph substrate that identity and
// quorum compose on. Callers SHOULD use the higher-level identity
// ops (CreateAttestation / SupersedeAttestation / etc.) for
// identity-context attestations rather than calling this client
// directly — the identity layer enforces the per-mode path
// dispatch and validity rules.
//
// Direct AttestationClient use is appropriate for app-defined
// attestation kinds (claims outside the identity stack) and for
// the substrate ops the identity layer itself composes on.
type AttestationClient struct {
	ap             *AppPeer
	target         string
	attestationURI string
}

// Attestation returns an AttestationClient targeting the local peer.
func (a *AppPeer) Attestation() *AttestationClient {
	return &AttestationClient{
		ap:             a,
		target:         a.PeerID(),
		attestationURI: "system/attestation",
	}
}

// AttestationAt returns an AttestationClient targeting a remote peer.
func (a *AppPeer) AttestationAt(peerID string) *AttestationClient {
	return &AttestationClient{
		ap:             a,
		target:         peerID,
		attestationURI: extPeerURI(a.PeerID(), peerID, "system/attestation"),
	}
}

// PeerID returns the peer-id this AttestationClient targets.
func (ac *AttestationClient) PeerID() string { return ac.target }

// Create writes a new attestation at path. Generic signed-claim
// creation; authorized via caller cap covering
// system/attestation:create. Path-as-resource MUST per V7 §3.2.
func (ac *AttestationClient) Create(
	ctx context.Context,
	path string,
	att types.AttestationData,
) (types.AttestationCreateResultData, error) {
	req := types.AttestationCreateRequestData{AttestationData: att}
	ent, err := req.ToEntity()
	if err != nil {
		return types.AttestationCreateResultData{}, WrapError(500, "encode_request",
			"encode AttestationCreateRequest", err)
	}
	resultEnt, err := extDispatch(ac.ap, ac.attestationURI, "create", path, ent)
	if err != nil {
		return types.AttestationCreateResultData{}, err
	}
	out, err := types.AttestationCreateResultDataFromEntity(resultEnt)
	if err != nil {
		return types.AttestationCreateResultData{}, WrapError(500, "decode_result",
			"decode AttestationCreateResult", err)
	}
	return out, nil
}

// Supersede creates a successor attestation under strict-by-design
// rules: the handler copies attesting/attested from the predecessor
// (per EXTENSION-ATTESTATION §6.1). For controller-rotation cases
// where attesting/attested legitimately change, use the identity-
// layer SupersedeAttestation instead — it routes via REBIND_KINDS to
// substrate :create with explicit Supersedes.
func (ac *AttestationClient) Supersede(
	ctx context.Context,
	path string,
	req types.AttestationSupersedeRequestData,
) (types.AttestationSupersedeResultData, error) {
	ent, err := req.ToEntity()
	if err != nil {
		return types.AttestationSupersedeResultData{}, WrapError(500, "encode_request",
			"encode AttestationSupersedeRequest", err)
	}
	resultEnt, err := extDispatch(ac.ap, ac.attestationURI, "supersede", path, ent)
	if err != nil {
		return types.AttestationSupersedeResultData{}, err
	}
	out, err := types.AttestationSupersedeResultDataFromEntity(resultEnt)
	if err != nil {
		return types.AttestationSupersedeResultData{}, WrapError(500, "decode_result",
			"decode AttestationSupersedeResult", err)
	}
	return out, nil
}

// Revoke produces a revocation attestation (kind="revocation"). The
// revocation is itself a system/attestation entity that targets the
// hash to be revoked. attesting carries the revoker's identity hash;
// reason is informational.
func (ac *AttestationClient) Revoke(
	ctx context.Context,
	path string,
	targetHash, attesting hash.Hash,
	reason string,
) (types.AttestationRevokeResultData, error) {
	req := types.AttestationRevokeRequestData{
		TargetHash: targetHash,
		Attesting:  attesting,
		Reason:     reason,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.AttestationRevokeResultData{}, WrapError(500, "encode_request",
			"encode AttestationRevokeRequest", err)
	}
	resultEnt, err := extDispatch(ac.ap, ac.attestationURI, "revoke", path, ent)
	if err != nil {
		return types.AttestationRevokeResultData{}, err
	}
	out, err := types.AttestationRevokeResultDataFromEntity(resultEnt)
	if err != nil {
		return types.AttestationRevokeResultData{}, WrapError(500, "decode_result",
			"decode AttestationRevokeResult", err)
	}
	return out, nil
}

// Verify runs single-signature validation per §4.1 plus a
// consumer-driven liveness check. asOf, when non-nil, enables
// time-traveling validation against historical state per §4.3.
func (ac *AttestationClient) Verify(
	ctx context.Context,
	attestationHash hash.Hash,
	asOf *uint64,
) (types.AttestationVerifyResultData, error) {
	req := types.AttestationVerifyRequestData{
		AttestationHash: attestationHash,
		AsOf:            asOf,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.AttestationVerifyResultData{}, WrapError(500, "encode_request",
			"encode AttestationVerifyRequest", err)
	}
	resultEnt, err := extDispatch(ac.ap, ac.attestationURI, "verify", "", ent)
	if err != nil {
		return types.AttestationVerifyResultData{}, err
	}
	out, err := types.AttestationVerifyResultDataFromEntity(resultEnt)
	if err != nil {
		return types.AttestationVerifyResultData{}, WrapError(500, "decode_result",
			"decode AttestationVerifyResult", err)
	}
	return out, nil
}
