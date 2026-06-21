package entitysdk

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/attestation"
	"go.entitychurch.org/entity-core-go/ext/identity"
	"go.entitychurch.org/entity-core-go/ext/quorum"
)

// BootstrapOpts configures the L0 identity bootstrap ceremony run by
// AppPeer.BootstrapIdentity. The defaults produce the smallest valid
// identity-aware peer per SDK-IDENTITY-INFRASTRUCTURE §3 — a 1-of-1
// quorum + 1 controller (local peer).
//
// "Three quorum, one controller" deployment shape: set
// QuorumMembers=3, QuorumThreshold=2 (2-of-3 majority); the local
// peer is the controller automatically.
type BootstrapOpts struct {
	// QuorumMembers is the number of quorum-constituent keypairs to
	// generate. Default 1 (the simplest valid shape — quorum of 1).
	QuorumMembers int
	// QuorumThreshold is K in the K-of-N. Defaults to QuorumMembers
	// (everyone signs, the simplest spec-conformant shape). Must be
	// in [1, QuorumMembers] when QuorumMembers > 1.
	QuorumThreshold int
	// ControllerGrants seed the local-peer→controller capability.
	// Default is wildcard on all four scope dimensions, matching
	// SDK-OPERATIONS §11.2A Level 0 open access.
	ControllerGrants []types.GrantEntry
	// QuorumName is the human-readable label attached to the quorum
	// entity. Empty value is fine; spec doesn't require a name.
	QuorumName string
	// BundleName, when non-empty, persists the resulting identity
	// material to ~/.entity/identities/{BundleName}/ as a directory
	// bundle (per SDK-IDENTITY-INFRASTRUCTURE §8.4). The bundle can
	// be re-loaded on subsequent peer starts via PeerConfig.Identity
	// to produce the same identity (same quorum-id, same controller-
	// cert hash, same peer→controller cap chain).
	//
	// Empty BundleName produces an in-memory-only ceremony; the
	// identity does not survive process restart.
	BundleName string
}

// BootstrapResult carries the artifacts produced by BootstrapIdentity.
//
// QuorumMembers contains the freshly-minted member keypairs. The
// caller is responsible for distributing the constituent private
// keys to separate custody per SDK-IDENTITY-INFRASTRUCTURE §8.2.
// When BundleName is set, the keypairs are also persisted in the
// bundle dir — convenient for development, the catastrophic-loss
// surface for production.
//
// BundleDir is the absolute on-disk path of the persisted bundle,
// or empty if no bundle was written.
type BootstrapResult struct {
	QuorumID              hash.Hash
	QuorumMembers         []crypto.Keypair
	ControllerCertHash    hash.Hash
	PeerConfigPath        string
	LocalToControllerCaps []hash.Hash
	BundleDir             string
}

// BootstrapIdentity runs the L0 identity ceremony on this AppPeer per
// SDK-IDENTITY-INFRASTRUCTURE §4.1 + EXTENSION-IDENTITY §6.5. After a
// successful return, the peer has:
//
//   - A K-of-N quorum entity bound at system/quorum/{quorum_id}
//   - One identity-cert attestation for the local peer as controller,
//     K-of-N signed by the quorum's members
//   - A peer-config entity at system/identity/peer-config
//   - A local-peer→controller capability at
//     system/capability/grants/identity/peer-to-controller/{local_id}
//
// When opts.BundleName is set, the identity material (controller
// keypair + member keypairs + manifest) is also persisted to disk
// at ~/.entity/identities/{BundleName}/ so it can be re-loaded on
// subsequent peer starts.
func (a *AppPeer) BootstrapIdentity(ctx context.Context, opts BootstrapOpts) (BootstrapResult, error) {
	if a.identityHandler == nil || a.attHandler == nil || a.quorumHandler == nil {
		return BootstrapResult{}, NewError(503, "identity_stack_not_wired",
			"BootstrapIdentity requires the identity stack — see ExtensionsConfig.IdentityStack")
	}
	opts, err := normalizeBootstrapOpts(opts)
	if err != nil {
		return BootstrapResult{}, err
	}

	// If the caller asked to persist, fail-fast on bundle-already-
	// exists rather than running the ceremony and discovering the
	// collision after work has been done.
	if opts.BundleName != "" {
		exists, err := IsIdentityBundleDir(opts.BundleName)
		if err != nil {
			return BootstrapResult{}, WrapError(500, "stat_bundle_failed",
				"check existing bundle", err)
		}
		if exists {
			return BootstrapResult{}, NewError(409, "bundle_exists",
				fmt.Sprintf("bundle %q already exists; remove it or choose a different name",
					opts.BundleName))
		}
	}

	// Generate fresh member keypairs.
	memberKps := make([]crypto.Keypair, opts.QuorumMembers)
	for i := range memberKps {
		kp, err := crypto.Generate()
		if err != nil {
			return BootstrapResult{}, WrapError(500, "keygen_failed",
				fmt.Sprintf("generate quorum member %d", i+1), err)
		}
		memberKps[i] = kp
	}

	res, err := runBootstrapCeremony(ctx, a, memberKps, opts)
	if err != nil {
		return BootstrapResult{}, err
	}

	// Optional persistence — happens after the ceremony succeeds so
	// we don't leave half-written bundles on the filesystem after a
	// ceremony failure.
	if opts.BundleName != "" {
		bundle := IdentityBundle{
			SchemaVersion:      "1",
			Name:               opts.BundleName,
			CreatedAt:          time.Now().Unix(),
			QuorumID:           hexHash(res.QuorumID.Bytes()),
			ControllerCertHash: hexHash(res.ControllerCertHash.Bytes()),
			Threshold:          opts.QuorumThreshold,
			QuorumName:         opts.QuorumName,
			ControllerKeypair:  a.peer.Keypair(),
			QuorumMembers:      memberKps,
		}
		if err := WriteIdentityBundle(bundle); err != nil {
			return BootstrapResult{}, err
		}
		dir, _ := IdentityBundleDir(opts.BundleName)
		res.BundleDir = dir
	}
	return res, nil
}

// ApplyIdentityBundle re-runs the bootstrap ceremony on this AppPeer
// using a previously-persisted bundle's keypairs. Used at peer start
// to re-mint the in-memory identity material (quorum entity, cert
// attestation, signatures, peer-config, controller cap) from the
// loaded bundle. Because all inputs (controller keypair, member
// keypairs, properties shape) are deterministic, the resulting
// content hashes match those recorded in the bundle manifest.
//
// Conformance check: returns an error if the re-minted QuorumID or
// ControllerCertHash diverge from the manifest values — that
// signals either a bundle-tampering case or a code-level encoding
// drift that must be resolved before the peer can claim to be the
// same identity.
func (a *AppPeer) ApplyIdentityBundle(
	ctx context.Context,
	bundle IdentityBundle,
	controllerGrants []types.GrantEntry,
) (BootstrapResult, error) {
	if a.identityHandler == nil || a.attHandler == nil || a.quorumHandler == nil {
		return BootstrapResult{}, NewError(503, "identity_stack_not_wired",
			"ApplyIdentityBundle requires the identity stack")
	}
	// Sanity: the local peer's keypair must match the bundle's
	// controller keypair, otherwise the controller-cert hash will
	// diverge — silently proceeding would produce a peer that
	// claims to be an identity it doesn't actually control.
	if !keypairsMatch(a.peer.Keypair(), bundle.ControllerKeypair) {
		return BootstrapResult{}, NewError(409, "controller_keypair_mismatch",
			"AppPeer keypair does not match bundle controller keypair; "+
				"set PeerConfig.Keypair to bundle.ControllerKeypair before CreatePeer")
	}
	opts := BootstrapOpts{
		QuorumMembers:    len(bundle.QuorumMembers),
		QuorumThreshold:  bundle.Threshold,
		ControllerGrants: controllerGrants,
		QuorumName:       bundle.QuorumName,
	}
	opts, err := normalizeBootstrapOpts(opts)
	if err != nil {
		return BootstrapResult{}, err
	}

	res, err := runBootstrapCeremony(ctx, a, bundle.QuorumMembers, opts)
	if err != nil {
		return BootstrapResult{}, err
	}

	// Verify the re-minted hashes match the manifest.
	if bundle.QuorumID != "" && hexHash(res.QuorumID.Bytes()) != bundle.QuorumID {
		return BootstrapResult{}, NewError(500, "quorum_hash_drift",
			fmt.Sprintf("re-minted quorum-id %s != bundle %s",
				hexHash(res.QuorumID.Bytes()), bundle.QuorumID))
	}
	if bundle.ControllerCertHash != "" &&
		hexHash(res.ControllerCertHash.Bytes()) != bundle.ControllerCertHash {
		return BootstrapResult{}, NewError(500, "cert_hash_drift",
			fmt.Sprintf("re-minted cert-hash %s != bundle %s",
				hexHash(res.ControllerCertHash.Bytes()), bundle.ControllerCertHash))
	}
	return res, nil
}

// runBootstrapCeremony is the deterministic ceremony driver. Given
// pre-existing member keypairs (either freshly-generated by
// BootstrapIdentity or loaded from a bundle by ApplyIdentityBundle),
// it mints quorum + controller-cert + signatures and runs
// identity.Startup. All entity content hashes are derived from the
// inputs, so equivalent inputs produce equivalent outputs.
func runBootstrapCeremony(
	ctx context.Context,
	a *AppPeer,
	memberKps []crypto.Keypair,
	opts BootstrapOpts,
) (BootstrapResult, error) {
	cs := a.peer.Store()
	li := a.peer.LocationIndex()
	localKp := a.peer.Keypair()
	localIdentity := a.peer.Identity()

	// 1. Persist quorum member identity entities so signer-resolution
	//    can map signer-hash → identity at verify time.
	memberHashes := make([]hash.Hash, len(memberKps))
	for i, kp := range memberKps {
		idEnt, err := kp.IdentityEntity()
		if err != nil {
			return BootstrapResult{}, WrapError(500, "encode_member_identity",
				fmt.Sprintf("encode quorum member %d identity", i+1), err)
		}
		if _, err := cs.Put(idEnt); err != nil {
			return BootstrapResult{}, WrapError(500, "persist_member_identity",
				fmt.Sprintf("persist quorum member %d identity", i+1), err)
		}
		memberHashes[i] = idEnt.ContentHash
	}

	// 2. Mint and persist the quorum entity.
	qData := types.QuorumData{
		Signers:   memberHashes,
		Threshold: uint64(opts.QuorumThreshold),
		Name:      opts.QuorumName,
	}
	qEnt, err := qData.ToEntity()
	if err != nil {
		return BootstrapResult{}, WrapError(500, "encode_quorum",
			"encode quorum entity", err)
	}
	if _, err := cs.Put(qEnt); err != nil {
		return BootstrapResult{}, WrapError(500, "persist_quorum",
			"persist quorum entity", err)
	}
	li.Set("system/quorum/"+hexHash(qEnt.ContentHash.Bytes()), qEnt.ContentHash)
	a.quorumHandler.InvalidateCache(qEnt.ContentHash)

	// 3. Mint the controller-cert attestation entity (build only —
	//    binding happens in step 5, after the K-of-N signatures are
	//    in the tree, so identity's process-attestation sync hook
	//    finds the signatures already-bound when it fires on the
	//    cert's binding event).
	certProps, err := types.EncodeProperties(types.IdentityCertProperties{
		Kind:     types.KindIdentityCert,
		Function: types.FunctionController,
		Mode:     types.ModeInternal,
	})
	if err != nil {
		return BootstrapResult{}, WrapError(500, "encode_props",
			"encode identity-cert properties", err)
	}
	attData := types.AttestationData{
		Attesting:  qEnt.ContentHash,
		Attested:   localIdentity.ContentHash,
		Properties: certProps,
	}
	attEnt, err := attData.ToEntity()
	if err != nil {
		return BootstrapResult{}, WrapError(500, "encode_attestation",
			"encode controller cert", err)
	}
	if _, err := cs.Put(attEnt); err != nil {
		return BootstrapResult{}, WrapError(500, "persist_attestation",
			"persist controller cert", err)
	}

	// 4. K-of-N sign the controller cert and bind each signature
	//    BEFORE the cert itself. Signatures are content-hashed over
	//    the cert's content hash (not the cert's tree binding), so
	//    they can be built and bound without the cert being bound
	//    yet. This ordering is critical: identity's
	//    process-attestation sync hook validates the cert against
	//    its quorum on bind; if the signatures aren't already in the
	//    tree, validation fail-closes and the hook unbinds the cert.
	//
	//    Ed25519 is deterministic per RFC 8032 — same keypair + same
	//    message produces the same signature bytes — so re-running
	//    the ceremony from a persisted bundle yields identical
	//    signature entities and identical content hashes.
	for i := 0; i < opts.QuorumThreshold; i++ {
		s := memberKps[i]
		sigBytes := s.Sign(attEnt.ContentHash.Bytes())
		sigData := types.SignatureData{
			Target:    attEnt.ContentHash,
			Signer:    memberHashes[i],
			Algorithm: "ed25519",
			Signature: sigBytes,
		}
		sigEnt, err := sigData.ToEntity()
		if err != nil {
			return BootstrapResult{}, WrapError(500, "encode_signature",
				fmt.Sprintf("encode signature %d", i+1), err)
		}
		if _, err := cs.Put(sigEnt); err != nil {
			return BootstrapResult{}, WrapError(500, "persist_signature",
				fmt.Sprintf("persist signature %d", i+1), err)
		}
		signerPeerID := string(s.PeerID())
		sigPath := "/" + signerPeerID + "/system/signature/" +
			hexHash(attEnt.ContentHash.Bytes())
		li.Set(sigPath, sigEnt.ContentHash)
	}

	// 5. Now bind the cert. Identity's process-attestation hook
	//    fires here, finds the signatures already bound, validates
	//    K-of-N against the quorum, and leaves the cert in place.
	certPath := "system/identity/internal/cert/" + hexHash(attEnt.ContentHash.Bytes())
	li.Set(certPath, attEnt.ContentHash)
	// Refresh attestation's in-memory index — index() is updated
	// incrementally via OnTreeChange in the dispatched path; for
	// the L0 bootstrap path we ensure the index has it before
	// identity.Startup walks.
	a.attHandler.OnTreeChange(boostrapTreeEvent(certPath, attEnt.ContentHash))

	// 5. Run identity.Startup — the L0 entry point.
	cfgReq := types.IdentityConfigureRequestData{
		TrustsQuorum:     qEnt.ContentHash,
		ControllerGrants: opts.ControllerGrants,
		Bindings:         nil,
	}
	cfgRes, err := bootstrapInvokeStartup(
		ctx, cs, li,
		a.attHandler, a.quorumHandler,
		localKp, localIdentity, cfgReq,
	)
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{
		QuorumID:              qEnt.ContentHash,
		QuorumMembers:         memberKps,
		ControllerCertHash:    attEnt.ContentHash,
		PeerConfigPath:        cfgRes.PeerConfigPath,
		LocalToControllerCaps: cfgRes.LocalPeerToControllerCaps,
	}, nil
}

// normalizeBootstrapOpts applies defaults and validates threshold.
func normalizeBootstrapOpts(opts BootstrapOpts) (BootstrapOpts, error) {
	if opts.QuorumMembers < 1 {
		opts.QuorumMembers = 1
	}
	if opts.QuorumThreshold < 1 {
		opts.QuorumThreshold = opts.QuorumMembers
	}
	if opts.QuorumThreshold > opts.QuorumMembers {
		return opts, NewError(400, "invalid_threshold",
			fmt.Sprintf("threshold %d exceeds members %d",
				opts.QuorumThreshold, opts.QuorumMembers))
	}
	if opts.ControllerGrants == nil {
		opts.ControllerGrants = []types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
		}}
	}
	return opts, nil
}

// bootstrapInvokeStartup runs identity.Startup and wraps its error
// into the SDK error model.
func bootstrapInvokeStartup(
	ctx context.Context,
	cs store.ContentStore,
	li store.LocationIndex,
	att *attestation.Handler,
	q *quorum.Handler,
	kp crypto.Keypair,
	localIdentity entity.Entity,
	req types.IdentityConfigureRequestData,
) (types.IdentityConfigureResultData, error) {
	res, err := identity.Startup(ctx, cs, li, att, q, kp, localIdentity, req)
	if err != nil {
		return types.IdentityConfigureResultData{}, WrapError(500, "identity_startup_failed",
			"identity.Startup", err)
	}
	return res, nil
}

// boostrapTreeEvent synthesizes a TreeChangeEvent for the L0
// bootstrap path. Mirrors the kernel emit shape so attestation's
// OnTreeChange index-maintainer sees the new entity.
//
// Field name carries an extra 'o' for historical reasons; the
// helper is unexported so renames don't ripple.
func boostrapTreeEvent(path string, h hash.Hash) store.TreeChangeEvent {
	_ = entity.Entity{} // keep entity import alive — used elsewhere in the file
	return store.TreeChangeEvent{
		Path:       path,
		Hash:       h,
		ChangeType: store.ChangeCreated,
	}
}

// keypairsMatch reports whether two keypairs share the same public
// key (and therefore the same derived peer-id and identity-entity
// content hash).
func keypairsMatch(a, b crypto.Keypair) bool {
	ab := a.PublicKeyBytes()
	bb := b.PublicKeyBytes()
	if len(ab) != len(bb) {
		return false
	}
	for i := range ab {
		if ab[i] != bb[i] {
			return false
		}
	}
	return true
}

// hex helper used in earlier revisions; retained as
// hex.EncodeToString import via "encoding/hex" — kept here to make
// the function discoverable. The package-level hexHash in
// identity_bundle.go is the canonical one.
var _ = hex.EncodeToString
