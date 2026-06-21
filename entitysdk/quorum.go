package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/quorum"
)

// QuorumClient wraps system/quorum EXECUTE operations behind typed
// Go methods. Per SDK-IDENTITY-INFRASTRUCTURE §5.2 and
// EXTENSION-QUORUM §6.
//
// Quorums are the K-of-N node primitive that identity composes on.
// Direct QuorumClient use is appropriate for app-defined K-of-N
// scenarios (governance, transaction signing) outside the identity
// stack; identity-context quorums should be created through
// IdentityClient.CreateQuorum so the identity layer can record the
// quorum-id in peer-config.
type QuorumClient struct {
	ap        *AppPeer
	target    string
	quorumURI string
}

// Quorum returns a QuorumClient targeting the local peer.
func (a *AppPeer) Quorum() *QuorumClient {
	return &QuorumClient{
		ap:        a,
		target:    a.PeerID(),
		quorumURI: "system/quorum",
	}
}

// QuorumAt returns a QuorumClient targeting a remote peer.
func (a *AppPeer) QuorumAt(peerID string) *QuorumClient {
	return &QuorumClient{
		ap:        a,
		target:    peerID,
		quorumURI: extPeerURI(a.PeerID(), peerID, "system/quorum"),
	}
}

// PeerID returns the peer-id this QuorumClient targets.
func (qc *QuorumClient) PeerID() string { return qc.target }

// Create instantiates a system/quorum entity at
// system/quorum/{quorum_id_hex}. signerResolution defaults to
// "concrete"; "identity-resolved" is registered by the identity
// extension at install time.
func (qc *QuorumClient) Create(
	ctx context.Context,
	signers []hash.Hash,
	threshold uint64,
	name string,
) (types.QuorumCreateResultData, error) {
	q := types.QuorumData{
		Signers:   signers,
		Threshold: threshold,
		Name:      name,
	}
	req := types.QuorumCreateRequestData{QuorumData: q}
	ent, err := req.ToEntity()
	if err != nil {
		return types.QuorumCreateResultData{}, WrapError(500, "encode_request",
			"encode QuorumCreateRequest", err)
	}
	qEnt, err := q.ToEntity()
	if err != nil {
		return types.QuorumCreateResultData{}, WrapError(500, "encode_quorum",
			"compute quorum hash", err)
	}
	resultEnt, err := extDispatch(qc.ap, qc.quorumURI, "create",
		quorum.QuorumPath(qEnt.ContentHash), ent)
	if err != nil {
		return types.QuorumCreateResultData{}, err
	}
	out, err := types.QuorumCreateResultDataFromEntity(resultEnt)
	if err != nil {
		return types.QuorumCreateResultData{}, WrapError(500, "decode_result",
			"decode QuorumCreateResult", err)
	}
	return out, nil
}

// Update produces a quorum-update attestation per §3.2 — a self-event
// signed by the quorum's current K signers. Signature gathering
// (collecting K-of-N signatures over the update entity) is the
// caller's responsibility.
func (qc *QuorumClient) Update(
	ctx context.Context,
	quorumID hash.Hash,
	newSigners []hash.Hash,
	newThreshold uint64,
	supersedes *hash.Hash,
) (types.QuorumUpdateResultData, error) {
	req := types.QuorumUpdateRequestData{
		QuorumID:     quorumID,
		NewSigners:   newSigners,
		NewThreshold: newThreshold,
		Supersedes:   supersedes,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.QuorumUpdateResultData{}, WrapError(500, "encode_request",
			"encode QuorumUpdateRequest", err)
	}
	resultEnt, err := extDispatch(qc.ap, qc.quorumURI, "update",
		quorum.QuorumPath(quorumID), ent)
	if err != nil {
		return types.QuorumUpdateResultData{}, err
	}
	out, err := types.QuorumUpdateResultDataFromEntity(resultEnt)
	if err != nil {
		return types.QuorumUpdateResultData{}, WrapError(500, "decode_result",
			"decode QuorumUpdateResult", err)
	}
	return out, nil
}

// Publish produces a quorum-publish attestation per §3.3 — a snapshot
// of the current signer set carrying an optional published-handle.
// publishedHandle is a generic consumer-extension hook (per §3.3
// v1.2 abstraction); identity uses it to publish the controller's
// current handle.
func (qc *QuorumClient) Publish(
	ctx context.Context,
	req types.QuorumPublishRequestData,
) (types.QuorumPublishResultData, error) {
	ent, err := req.ToEntity()
	if err != nil {
		return types.QuorumPublishResultData{}, WrapError(500, "encode_request",
			"encode QuorumPublishRequest", err)
	}
	resultEnt, err := extDispatch(qc.ap, qc.quorumURI, "publish",
		quorum.QuorumPath(req.QuorumID), ent)
	if err != nil {
		return types.QuorumPublishResultData{}, err
	}
	out, err := types.QuorumPublishResultDataFromEntity(resultEnt)
	if err != nil {
		return types.QuorumPublishResultData{}, WrapError(500, "decode_result",
			"decode QuorumPublishResult", err)
	}
	return out, nil
}

// Verify runs K-of-N signature validation against the resolved
// signer set for quorumID. Returns SignedBy enumerating which
// constituents the signature scan matched.
func (qc *QuorumClient) Verify(
	ctx context.Context,
	entityHash, quorumID hash.Hash,
) (types.QuorumVerifyResultData, error) {
	req := types.QuorumVerifyRequestData{
		EntityHash: entityHash,
		QuorumID:   quorumID,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.QuorumVerifyResultData{}, WrapError(500, "encode_request",
			"encode QuorumVerifyRequest", err)
	}
	resultEnt, err := extDispatch(qc.ap, qc.quorumURI, "verify", "", ent)
	if err != nil {
		return types.QuorumVerifyResultData{}, err
	}
	out, err := types.QuorumVerifyResultDataFromEntity(resultEnt)
	if err != nil {
		return types.QuorumVerifyResultData{}, WrapError(500, "decode_result",
			"decode QuorumVerifyResult", err)
	}
	return out, nil
}
