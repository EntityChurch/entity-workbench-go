package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"github.com/fxamacker/cbor/v2"
)

// BlobResolvePattern is the URI pattern under which the workbench's
// cross-peer blob-resolve handler registers. A subscription on a
// remote peer's local/files/{root}/* prefix (with include_payload:
// true) delivers tree-change notifications here; this handler does
// the full notification → blob-fetch → materialize pipeline in a
// single dispatch.
const BlobResolvePattern = "workbench/blob-resolve"

// BlobResolveHandler is the cross-peer materialization step in the
// Stage 3 cross-peer file-sync chain. Per `STAGE-3-DESIGN-RESPONSE.md
// §3` and arch's L10 algorithm-reference framing, this handler:
//
//  1. Unwraps the subscription delivery → tree-change notification.
//  2. Extracts the changed file entity from hctx.Included (delivered
//     because the subscription opted into `include_payload: true`).
//  3. Decodes the FileData → identifies the blob hash + source peer.
//  4. Drives content.EnsureClosure against content.AtPeer(hctx,
//     sourcePeerID) — cap-checked sequencer over system/content:get
//     that drains until the blob's full closure is locally present.
//     §7.4 sender batching + 503 partial-sync retry live inside the
//     SDK helper; this step is one call (was the §7.2 reimpl).
//  5. Dispatches local/files:write content-mode locally to atomically
//     write the file to disk (no bytes traverse the wire — the blob
//     is already in the local content store from step 4).
//
// **Why a single-handler shape (Q2 collapse, same reason as
// notification_ingest):** the notification URI carries the source
// peer ID + relative path; deriving the target tree path from the
// source URI requires string manipulation that continuation
// transforms can't express. Per the G1 idiom + the
// L11 deferral, substrate-resolution-as-a-handler-step is the
// correct shape.
//
// **State:** holds a map of source-prefix → target-prefix mappings
// populated via RegisterMount at sync-setup time. Lookup at
// receive-time matches the notification URI against registered
// source prefixes (longest-prefix match). For the typical case
// where both peers mount the same prefix (e.g.,
// `local/files/sync/`), source and target coincide; the map allows
// asymmetric mount paths if a deployment needs them.
//
// **Capability surface:** the handler operates under its
// internal_scope (declared in Manifest below). The cross-peer
// system/content:get dispatch reuses the standard cross-peer cap
// (the caller's connection cap). The local local/files:write
// dispatch uses the handler's grant on local/files.
type BlobResolveHandler struct {
	mu     sync.RWMutex
	mounts map[string]string

	// conflicts rate-limits the conflict path and fails closed past its
	// limit — see conflict_detect.go. On the handler and not on a folder
	// because a detection bug fires across every folder at once, which is
	// exactly the case a per-folder limit would let through.
	conflicts conflictLimiter
	// conflictsDetected is this process's count, read through
	// ConflictHealth. Guarded by the limiter's mutex.
	conflictsDetected int
}

// NewBlobResolveHandler returns a new, mountless BlobResolveHandler.
// Use RegisterMount before subscribing to bind source/target prefixes.
func NewBlobResolveHandler() *BlobResolveHandler {
	return &BlobResolveHandler{
		mounts: make(map[string]string),
	}
}

// RegisterMount associates a source prefix on a remote peer with a
// target prefix on the local peer. Both prefixes are normalized to
// end with "/". For the symmetric case (peer A mounts
// local/files/sync/ → peer B mounts local/files/sync/), pass the
// same string for both.
func (h *BlobResolveHandler) RegisterMount(sourcePrefix, targetPrefix string) {
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	if !strings.HasSuffix(targetPrefix, "/") {
		targetPrefix += "/"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mounts[sourcePrefix] = targetPrefix
}

// UnregisterMount removes a source-prefix mapping.
func (h *BlobResolveHandler) UnregisterMount(sourcePrefix string) {
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.mounts, sourcePrefix)
}

// LookupMount returns the target prefix registered for a source prefix,
// or "" when none is. Exact match, not the longest-prefix walk Handle
// does — this exists so a restore can be verified to have produced a
// mapping the handler can ROUTE on, and a registration nobody can
// dispatch against is the same silence it replaced.
func (h *BlobResolveHandler) LookupMount(sourcePrefix string) string {
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mounts[sourcePrefix]
}

func (h *BlobResolveHandler) Name() string { return "workbench-blob-resolve" }

// Manifest declares the handler + its internal scope.
//
// # THE HANDLER GRANT IS THE OUTBOUND GATE (0.8.2.19 Delta E1, F67)
//
// The sentence this comment used to end with — *"cross-peer dispatches
// use the caller's connection cap"* — stopped being true on 2026-09-10.
// It was the confused deputy the kernel's E1 change closes: a caller-
// supplied credential authorized the sub-dispatch on its own four
// dimensions and **the executing handler's own grant was never
// consulted**, so anyone holding any target→us capability could steer any
// handler here past its declared scope.
//
// After E1 the shape is: the handler's grant answers WHAT, on all four
// dimensions; a target-minted credential relaxes Dimension 4 (peers) and
// nothing else. `core/protocol/outbound_authz.go` is the gate.
//
// So this manifest is now load-bearing in a way it has never been. It is
// minted into `system/capability/grants/{pattern}` at peer construction
// and it is the ceiling on every outbound dispatch this handler makes —
// including the cross-peer `system/content:get` that pulls the blob
// closure, which is the ONE dispatch that makes a share transfer bytes.
//
// # Why Peers is "*" here, deliberately, and what that does and does not
// widen
//
// §5.2 Dimension 4 defaults an ABSENT peers scope to
// `{include:[local_peer_id]}` and still checks it. So "no peers field"
// does not mean "unrestricted", it means **this peer only** — and a
// blob-resolve handler that can only reach its own peer cannot fetch a
// byte from the peer that published the folder. Measured, on the whole
// sharing flow, in both directions, with and without wildcard grants:
// every file failed with `403 capability_denied ... a handler with no
// peers scope covering the target cannot reach a foreign peer`.
//
// The handler cannot name the peers at manifest time. Which peer it
// fetches from is a property of a mount registered later, and a grant
// minted once at construction cannot know a folder an operator will
// accept next week. So the honest scope is "*", and it is narrow in
// every dimension that is not the one we cannot know:
//
//	handler   system/content   — one handler, not "*"
//	operation get              — a READ. It cannot write anywhere.
//	resource  the content store, and nothing else
//	peers     *                — whichever peer published what we accepted
//
// What "*" here does NOT grant: writing at a foreign peer, reaching any
// other handler at a foreign peer, or anything at all outside a content
// read. The local `local/files:write` entry below keeps its default —
// materialization lands on OUR disk, so its peers scope is correctly the
// local peer, and widening it would be a real escalation. Do not
// "complete" the fix by touching it.
//
// # The half a code change cannot reach
//
// Handler grants are Class I / install-once: `createHandlerGrants` skips
// a pattern whose grant already exists, so **this change reaches new
// peers only.** Every already-running peer keeps the grant it was
// constructed with and stays broken. `MigrateHandlerGrants` is that half;
// see handler_grant_migrate.go.
func (h *BlobResolveHandler) Manifest() types.HandlerManifestData {
	return types.HandlerManifestData{
		Pattern: BlobResolvePattern,
		Name:    "workbench-blob-resolve",
		Operations: map[string]types.HandlerOperationSpec{
			"receive": {InputType: "primitive/any"},
		},
		InternalScope: BlobResolveInternalScope(),
	}
}

// BlobResolveInternalScope is the handler's declared authority, exported
// because the migration in handler_grant_migrate.go has to compare an
// installed grant against it. One definition, two readers — a migration
// that carried its own copy would drift from the manifest silently, and
// the symptom of that drift is a peer that re-mints forever or never.
func BlobResolveInternalScope() []types.GrantEntry {
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"write", "delete"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			// Both spellings on purpose. A bare "system/content"
			// canonicalizes against the GRANTER (PR-8) — us — so it names
			// our own content store and not the one we are fetching from.
			// The peer-wildcard form is what covers a foreign peer's, and
			// it is the same distinction `defaultHandlerSelfGrant` spells
			// out in core/peer/peer.go: bare "*" is own-namespace-only and
			// "/*/*" is the cross-peer form.
			Resources: types.CapabilityScope{Include: []string{
				"system/content", "/*/system/content", "/*/system/content/*",
			}},
			// The dimension the whole fix is about. See the Manifest
			// comment above for why this is "*" and why that is narrow.
			Peers: &types.CapabilityScope{Include: []string{"*"}},
		},
	}
}

// Handle is the receive op: subscription delivery → blob fetch →
// local write. Returns 200 on successful materialization; 503
// blob_pending_sync when the source peer doesn't have the blob yet
// (caller's chain-retry path applies on next subscription event).
func (h *BlobResolveHandler) Handle(ctx context.Context, req *handler.Request) (*handler.Response, error) {
	if req.Operation != "receive" {
		return handler.NewErrorResponse(501, "unsupported_operation",
			fmt.Sprintf("blob-resolve does not support operation %q", req.Operation))
	}
	hctx := req.Context
	if hctx == nil {
		return handler.NewErrorResponse(500, "internal_error",
			"blob-resolve requires handler context")
	}
	if hctx.Execute == nil {
		return handler.NewErrorResponse(500, "internal_error",
			"blob-resolve requires hctx.Execute (in-handler dispatch capability)")
	}

	// Unwrap inbox delivery → notification.
	notifEnt := req.Params
	if notifEnt.Type == types.TypeInboxDelivery {
		delivery, err := types.InboxDeliveryDataFromEntity(notifEnt)
		if err != nil {
			return handler.NewErrorResponse(400, "decode_delivery",
				"decode inbox delivery: "+err.Error())
		}
		notifEnt = entity.Entity{Type: types.TypeSubscriptionNotification, Data: delivery.Result}
	}
	if notifEnt.Type != types.TypeSubscriptionNotification {
		return handler.NewErrorResponse(400, "wrong_input_type",
			"expected "+types.TypeSubscriptionNotification+", got "+notifEnt.Type)
	}
	notif, err := types.SubscriptionNotificationDataFromEntity(notifEnt)
	if err != nil {
		return handler.NewErrorResponse(400, "decode_notification",
			"decode notification: "+err.Error())
	}

	// Identify source peer + relative path from the notification URI.
	// URI shape: /{sourcePeerID}/{relativePath}
	sourcePeerID, relativeURI := splitPeerIDFromURI(notif.URI)
	if sourcePeerID == "" {
		return handler.NewErrorResponse(400, "invalid_uri",
			"notification URI lacks a source peer-id prefix: "+notif.URI)
	}

	// Find the matching source-prefix mount (longest-match).
	h.mu.RLock()
	var sourcePrefix, targetPrefix string
	for sp, tp := range h.mounts {
		if strings.HasPrefix(relativeURI, sp) && len(sp) > len(sourcePrefix) {
			sourcePrefix, targetPrefix = sp, tp
		}
	}
	h.mu.RUnlock()
	if sourcePrefix == "" {
		return handler.NewErrorResponse(404, "no_mount_for_uri",
			"no registered mount matches "+notif.URI)
	}

	// Deletion branch. The cross-peer source FileData was removed —
	// propagate by dispatching local/files:delete at the matching
	// LOCAL source path. handleDelete (DOMAIN-LOCAL-FILES §4.4) does
	// the fs unlink + tree:remove synchronously.
	//
	// Why explicit dispatch vs. just TreeRemove (which would let
	// DOMAIN-LOCAL-FILES §5.4 reverse_delete unlink the fs file):
	// the localfiles reverseTracker.isRecentlyWritten check
	// (core-go ext/localfiles/reverse.go:100) suppresses reverse-
	// events for paths within a 5s window after a handler write.
	// In normal collaborative editing, deletes shortly after writes
	// fall inside that window and reverse_delete is skipped, leaving
	// the fs file stranded. Explicit dispatch sidesteps the tracker
	// — handleDelete unlinks the file directly. Filed as a core-go
	// finding (reverseTracker check should be inside the write
	// branch only); workaround stays here until that lands.
	//
	// notif.Event values follow EXTENSION-SUBSCRIPTION ("created" /
	// "updated" / "deleted").
	if notif.Event == "deleted" {
		relPath := strings.TrimPrefix(relativeURI, sourcePrefix)
		// TARGET, not source. This read `sourcePrefix + relPath` — which
		// is just `relativeURI` back again, i.e. the path the file has on
		// the SENDER — and the write branch twenty lines below has always
		// used `targetPrefix + relPath`. So a delete was dispatched at a
		// path that exists on the other machine.
		//
		// It is the two-root-names trap in a third place. `accept <peer>
		// <root> <directory>` names the receiving mount after the
		// directory the operator picked, so `local/files/photos/` on the
		// sender is `local/files/received/` here. Whenever the two roots
		// happen to MATCH, sourcePrefix == targetPrefix and this line is
		// correct by coincidence — which is why every test in the tree
		// passed: they all use one name on both peers.
		//
		// Measured by scripts/twopeer-sync.sh, two containers, real TCP:
		// create and modify propagated, delete did not, and the ack still
		// said `deleted: true`.
		localTargetPath := targetPrefix + relPath
		// Only dispatch if a binding actually exists locally — a
		// concurrent watcher Remove may already have cleaned up. The
		// unbound case is now REPORTED rather than acked as a deletion:
		// this guard is exactly what swallowed the defect above, because
		// a wrong path is indistinguishable from "already gone" and both
		// returned success.
		dispatched := false
		if hctx.LocationIndex != nil {
			if _, bound := hctx.LocationIndex.Get(localTargetPath); bound {
				_, err := hctx.Execute(ctx, "local/files", "delete", entity.Entity{},
					handler.WithResource(&types.ResourceTarget{Targets: []string{localTargetPath}}))
				if err != nil {
					return handler.NewErrorResponse(500, "delete_dispatch_failed",
						"local/files:delete dispatch: "+err.Error())
				}
				dispatched = true
			}
		}
		return ackEntity(200, map[string]interface{}{
			"deleted":     dispatched,
			"source_path": localTargetPath,
		})
	}

	// Pull the changed file entity. Per EXTENSION-SUBSCRIPTION v3.14
	// include_payload, the engine attached the entity at notif.Hash
	// to the delivery; it arrives via hctx.Included on this side.
	// Fall back to a local store lookup if include_payload wasn't
	// set (we won't get a payload then, only a hash — and for
	// cross-peer, the entity won't be in the local store yet).
	var fileEnt entity.Entity
	if !notif.Hash.IsZero() {
		if ent, ok := hctx.Included[notif.Hash]; ok {
			fileEnt = ent
		} else if hctx.Store != nil {
			if ent, ok := hctx.Store.Get(notif.Hash); ok {
				fileEnt = ent
			}
		}
	}
	if fileEnt.Type == "" {
		return handler.NewErrorResponse(503, "file_entity_unresolved",
			fmt.Sprintf("file entity for hash %s not in Included or local store — "+
				"subscription needs include_payload: true", notif.Hash.String()))
	}
	if fileEnt.Type != localfiles.TypeFile {
		// Ignore non-file entities under the prefix (e.g., directory
		// listings or future entity types). The mount pattern is
		// fundamentally file-shaped; we don't materialize other types.
		return ackEntity(200, map[string]interface{}{
			"skipped":     true,
			"reason":      "not_a_file_entity",
			"entity_type": fileEnt.Type,
		})
	}
	file, err := localfiles.FileDataFromEntity(fileEnt)
	if err != nil {
		return handler.NewErrorResponse(400, "decode_file_data",
			"decode FileData: "+err.Error())
	}

	// Compute target path now so we can short-circuit on already-current
	// before any cross-peer work.
	relPath := strings.TrimPrefix(relativeURI, sourcePrefix)
	targetTreePath := targetPrefix + relPath

	// F9 idempotency short-circuit (Round 6 workbench-side fix):
	// the bidirectional symmetric topology of case 2 creates a
	// subscription loop where each TreeSet from local/files:write
	// fires a tree change event that the OTHER peer's subscription
	// observes, dispatching ANOTHER blob-resolve that targets the
	// same path with the same content hash. Without this check, the
	// loop runs unbounded (~150 iterations/sec; saturates CPU).
	//
	// Check: does my local tree at targetTreePath already have a
	// file entity bound with the same blob hash? If yes, the
	// materialization is a no-op — skip cross-peer fetch + write
	// dispatch entirely. Subsequent notifications for unchanged
	// content terminate the loop within one round-trip.
	//
	// The deeper architectural fix lives in core-go: hctx.TreeSet
	// (handler.go) should not fire a tree change event when the new
	// entity hash equals the existing entity hash at the same path.
	// The F9 arms, and the DIFFERENT one is where a conflict lives.
	//
	// Same hash → already current, short-circuit as above. Different hash
	// → somebody changed something, and until M3 the handler assumed that
	// somebody was the sender and overwrote. It is right most of the time
	// and it is not always right: if the version on disk here was authored
	// by OUR watcher, the overwrite replaces an edit the sender has never
	// seen. The tree kept it — §1.1a guarantees that and the baseline test
	// measures it — at a chain position no surface rendered and no verb
	// reached, which made a recoverable loss an unrecoverable one.
	var conflict conflictOutcome
	var mineHash hash.Hash
	var conflictRecoverable bool
	if hctx.Store != nil {
		if existingHash, ok := tryGetLocalFileBlobHash(hctx, targetTreePath); ok {
			if existingHash == file.Content {
				return ackEntity(200, map[string]interface{}{
					"skipped":     true,
					"reason":      "already_current",
					"target_path": targetTreePath,
					"blob_hash":   file.Content.String(),
				})
			}
			mineHash = existingHash
			conflict, conflictRecoverable = h.classifyConflict(
				hctx, targetTreePath, targetPrefix, existingHash, file.Content)
		}
	}

	// A decision the operator already made, honoured. Checked before the
	// closure fetch because declining is the whole point — pulling the
	// bytes of a version we are not going to write is work with no
	// outcome, on every pass, forever.
	if conflict == conflictDeclined {
		return ackEntity(200, map[string]interface{}{
			"skipped":     true,
			"reason":      "declined_by_operator",
			"target_path": targetTreePath,
			"blob_hash":   file.Content.String(),
		})
	}

	// FAIL CLOSED past the burst limit (SYNC-LIMITS §5 rule 1). Nothing
	// is overwritten and nothing new is written: a storm is far more
	// likely to be our comparison bug than their editing, and carrying on
	// is the cheap wrong answer that is unrecoverable at scale. Recovery
	// is automatic — the catch-up supervisor re-derives the truth on its
	// next pass, by which time the window has expired — so this refusal
	// delays a delivery rather than dropping it.
	// The folder's owner declares its reconciliation rule and we have not
	// read it. Nothing is overwritten: a shared folder names ONE rule, and
	// applying ours to somebody else's folder is the divergence the rule
	// exists to prevent. Recovers by itself — the reconciler reads the
	// owner's declaration on its next pass and records it.
	if conflict == conflictRuleUnknown {
		return handler.NewErrorResponse(409, "conflict_rule_unknown",
			fmt.Sprintf("not resolving a conflict at %s: this folder belongs to another "+
				"peer, its reconciliation rule is theirs, and we have not been able to "+
				"read it. Nothing was overwritten and your version is untouched. This "+
				"clears on the next pass once that peer is reachable; run `status` to "+
				"force one", targetTreePath))
	}

	if conflict == conflictRefuse {
		return handler.NewErrorResponse(429, "conflict_storm",
			fmt.Sprintf("refusing to resolve a conflict at %s: more than %d "+
				"conflicts in %s on this peer. Nothing was overwritten. Run "+
				"`conflicts` to see what has been detected; delivery resumes by "+
				"itself once the burst subsides",
				targetTreePath, ConflictBurstLimit, ConflictBurstWindow))
	}

	// Fetch the blob closure cross-peer via system/content:get. Per
	// SDK-EXTENSION-OPERATIONS v0.8 §11 + PROPOSAL-CONTENT-MATERIALIZATION
	// v2 closure-think reframe: content.EnsureClosure is the cap-checked
	// sequencer; content.AtPeer aims dispatch at the source peer while
	// the cap-check still flows through the inner HandlerContext
	// dispatcher. 503 retry semantics + §7.4 sender batching are inside
	// EnsureClosure now (was previously workbench-side defense-in-depth).
	if !file.Content.IsZero() {
		disp := content.AtPeer(hctx, crypto.PeerID(sourcePeerID))
		if err := content.EnsureClosure(ctx, disp, file.Content, "system/content"); err != nil {
			// Map to chain-visible status. 503 stays 503 (caller's chain-
			// retry path on next subscription event); other failures
			// surface as 503 blob_pending_sync too — local-files chain
			// step treats any closure-fetch failure as retry-eligible.
			var se *content.StatusError
			status := uint(503)
			code := "blob_pending_sync"
			if errors.As(err, &se) {
				status = se.Status
				if se.Code != "" {
					code = se.Code
				}
			}
			return handler.NewErrorResponse(status, code,
				fmt.Sprintf("blob closure fetch from %s: %v", sourcePeerID, err))
		}
	}

	// Materialize via local local/files:write content-mode. Dispatcher
	// applies the handler's internal-scope grant on local/files:write.
	// No bytes traverse the wire — the blob is now in the local
	// content store from the fetch above. targetTreePath was computed
	// above for the F9 idempotency check.
	contentHash := file.Content
	writeReq := localfiles.WriteRequestData{
		Content:    &contentHash,
		CreateDirs: true,
	}
	writeReqRaw, err := ecf.Encode(writeReq)
	if err != nil {
		return handler.NewErrorResponse(500, "encode_write_request",
			"encode write request: "+err.Error())
	}
	writeReqEnt, err := entity.NewEntity(localfiles.TypeWriteRequest, cbor.RawMessage(writeReqRaw))
	if err != nil {
		return handler.NewErrorResponse(500, "build_write_request",
			"build write request entity: "+err.Error())
	}
	writeResp, err := hctx.Execute(ctx, "local/files", "write", writeReqEnt,
		handler.WithResource(&types.ResourceTarget{Targets: []string{targetTreePath}}))
	if err != nil {
		return handler.NewErrorResponse(500, "write_dispatch_failed",
			"dispatch local/files:write: "+err.Error())
	}
	if writeResp == nil || writeResp.Status >= 400 {
		status := uint(500)
		if writeResp != nil {
			status = writeResp.Status
		}
		return handler.NewErrorResponse(status, "write_failed",
			fmt.Sprintf("local/files:write returned status %d", status))
	}

	// The conflict is recorded AFTER the write, and that ordering is the
	// safe one in both directions. A record written first and a write
	// that then fails would tell an operator their edit had been replaced
	// when it had not — and the reverse, a lost record after a successful
	// write, is exactly the state we were already in before M3, so the
	// failure mode of this ordering is the status quo rather than a new
	// false statement.
	if conflict == conflictRecord || conflict == conflictKeepBoth {
		keepBothPath := ""
		if conflict == conflictKeepBoth {
			keepBothPath = h.writeKeepBothSibling(ctx, hctx, targetTreePath, mineHash)
		}
		h.recordConflict(hctx, ConflictData{
			Path:         qualify(string(hctx.LocalPeerID), targetTreePath),
			Root:         strings.TrimSuffix(strings.TrimPrefix(targetPrefix, LocalFilesSourcePrefix), "/"),
			RemotePeerID: sourcePeerID,
			MineHash:     mineHash,
			TheirsHash:   file.Content,
			Recoverable:  conflictRecoverable,
			KeepBothPath: keepBothPath,
			AtMillis:     uint64(time.Now().UnixMilli()),
		})
	}

	ack := map[string]interface{}{
		"target_path": targetTreePath,
		"blob_hash":   file.Content.String(),
		"source_peer": sourcePeerID,
		"size":        file.Size,
	}
	if conflict == conflictRecord || conflict == conflictKeepBoth {
		ack["conflict"] = true
		ack["replaced_blob"] = mineHash.String()
		ack["recoverable"] = conflictRecoverable
	}
	return ackEntity(200, ack)
}

// classifyConflict decides what to do about a delivery whose hash differs
// from what is on disk, and reports whether the replaced version will
// still be recoverable.
//
// The burst limiter is consulted ONLY once a conflict has been
// identified, so an ordinary catch-up over ten thousand files never
// touches it. Consuming a slot on every differing hash would make the
// limit a limit on syncing.
func (h *BlobResolveHandler) classifyConflict(hctx *handler.HandlerContext,
	targetTreePath, targetPrefix string, mine, theirs hash.Hash) (conflictOutcome, bool) {

	// Never conflict a keep-both sibling with itself. A sibling is a real
	// file that syncs like any other, and treating one as a conflict
	// candidate would let a single collision breed on every pass.
	if IsKeepBothPath(targetTreePath) {
		return conflictNone, false
	}

	// An operator's standing answer outranks everything below, INCLUDING
	// the burst limiter: declining costs nothing and refusing a delivery
	// the operator has already declined would report a storm made of
	// their own decisions.
	if operatorDeclinedDelivery(hctx, qualify(string(hctx.LocalPeerID), targetTreePath), mine, theirs) {
		return conflictDeclined, false
	}

	local, known := localEditAwaitsDelivery(hctx, targetTreePath)
	if !known {
		// No chain to read. NOT a conflict — see conflict_detect.go: the
		// alternative conflicts an entire folder on the first pass after
		// an upgrade, which is §5's storm arriving by way of the caution
		// that was meant to prevent it.
		return conflictNone, false
	}
	if !local {
		// The last thing that happened here was a delivery, so we are
		// simply behind. This is the overwhelmingly common case and it is
		// the one that must stay free.
		return conflictNone, false
	}

	policy, _, ruleKnown := folderConflictPolicy(hctx, targetPrefix)
	if !ruleKnown {
		// The rule belongs to the folder's owner and we have not read it,
		// so this delivery is HELD — see folderConflictPolicy.
		//
		// Decided BEFORE the burst limiter, and that ordering is the
		// point rather than an accident. A held delivery writes nothing,
		// overwrites nothing and records nothing: it is already the
		// safest outcome this function has, so spending storm budget on
		// it is not a stricter check, it is a worse diagnosis. The
		// limiter exists to catch OUR comparison bug firing on every
		// file; letting holds fill its window means an unreachable peer
		// trips it and the operator is told `conflict_storm` — a fault on
		// this machine that clears by itself — when the actual cause is
		// another machine they need to go and switch on.
		//
		// It is also why the detected counter is not incremented here:
		// ConflictHealth.Detected is documented as conflicts this process
		// ACTED ON, and this is the one branch that acts on nothing.
		return conflictRuleUnknown, false
	}

	if !h.conflicts.admit() {
		return conflictRefuse, false
	}
	h.conflicts.mu.Lock()
	h.conflictsDetected++
	h.conflicts.mu.Unlock()

	if policy == ConflictPolicyKeepBoth {
		return conflictKeepBoth, true
	}
	// Recoverable because the chain was readable — which is the same fact
	// that made this a conflict at all. The two cannot disagree, and
	// deriving one from the other is what keeps them that way.
	return conflictRecord, true
}

// writeKeepBothSibling materializes the replaced version at
// EXTENSION-REVISION §2.3's sibling path, and returns the path it wrote
// or "" if it could not.
//
// No closure fetch: the replaced version is OUR OWN previous content, so
// its blob is already local by construction. That is why keep-both costs
// nothing on the wire.
func (h *BlobResolveHandler) writeKeepBothSibling(ctx context.Context,
	hctx *handler.HandlerContext, targetTreePath string, mine hash.Hash) string {

	if mine.IsZero() {
		return ""
	}
	sibling := KeepBothPath(targetTreePath, mine)

	// Idempotent: a re-delivery of the same pair must not rewrite the
	// sibling, and must not report a second conflict for it either.
	if existing, ok := tryGetLocalFileBlobHash(hctx, sibling); ok && existing == mine {
		return sibling
	}

	content := mine
	req := localfiles.WriteRequestData{Content: &content, CreateDirs: true}
	raw, err := ecf.Encode(req)
	if err != nil {
		return ""
	}
	ent, err := entity.NewEntity(localfiles.TypeWriteRequest, cbor.RawMessage(raw))
	if err != nil {
		return ""
	}
	resp, err := hctx.Execute(ctx, "local/files", "write", ent,
		handler.WithResource(&types.ResourceTarget{Targets: []string{sibling}}))
	if err != nil || resp == nil || resp.Status >= 400 {
		return ""
	}
	return sibling
}

// recordConflict writes the durable record.
//
// Best-effort and deliberately not fatal: the delivery has already
// landed, and turning a failed record into a failed delivery would make
// the reporting mechanism able to break the thing it reports on. A record
// that does not get written leaves the peer in the pre-M3 state, which is
// the state every peer was in until this shipped.
func (h *BlobResolveHandler) recordConflict(hctx *handler.HandlerContext, c ConflictData) {
	if hctx == nil || hctx.Store == nil || hctx.LocationIndex == nil {
		return
	}
	raw, err := ecf.Encode(c)
	if err != nil {
		return
	}
	ent, err := entity.NewEntity(ConflictType, cbor.RawMessage(raw))
	if err != nil {
		return
	}
	stored, err := hctx.Store.Put(ent)
	if err != nil {
		return
	}
	// Qualified, because TreeSet writes through the location index and a
	// bare path there is a different path (AP58's shape at the write end).
	path := qualify(string(hctx.LocalPeerID), ConflictPrefix+c.Key())
	_, _ = hctx.TreeSet(path, stored, "record-conflict")
}

// qualify prefixes a bare tree path with a peer-id. A path that already
// carries one is returned unchanged, so applying it is never wrong.
func qualify(peerID, path string) string {
	if strings.HasPrefix(path, "/") || peerID == "" {
		return path
	}
	return "/" + peerID + "/" + path
}

// tryGetLocalFileBlobHash returns the blob hash of a file entity at
// the given local tree path, if one is bound. Used by the F9
// idempotency check to short-circuit redundant materialization on
// notification bounce-back. Returns (zero, false) on any miss or
// non-file entity; never errors out (idempotency check is best-effort).
func tryGetLocalFileBlobHash(hctx *handler.HandlerContext, treePath string) (hash.Hash, bool) {
	if hctx == nil || hctx.Store == nil {
		return hash.Hash{}, false
	}
	// LocationIndex lookup resolves the qualified path. blob-resolve
	// runs under the local peer's namespace; treePath is bare so we
	// qualify with the local peer-id via PeerContext if available.
	li := hctx.LocationIndex
	if li == nil {
		return hash.Hash{}, false
	}
	qualified := treePath
	if hctx.LocalPeerID != "" {
		qualified = "/" + string(hctx.LocalPeerID) + "/" + treePath
	}
	h, ok := li.Get(qualified)
	if !ok {
		return hash.Hash{}, false
	}
	ent, ok := hctx.Store.Get(h)
	if !ok {
		return hash.Hash{}, false
	}
	if ent.Type != localfiles.TypeFile {
		return hash.Hash{}, false
	}
	file, err := localfiles.FileDataFromEntity(ent)
	if err != nil {
		return hash.Hash{}, false
	}
	return file.Content, true
}

// splitPeerIDFromURI splits "/{peerID}/rest..." into (peerID, rest).
// Returns ("", uri) if the URI is not in qualified form.
func splitPeerIDFromURI(qualified string) (string, string) {
	if !strings.HasPrefix(qualified, "/") {
		return "", qualified
	}
	rest := qualified[1:]
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return "", qualified
	}
	return rest[:i], rest[i+1:]
}

// ackEntity returns a `workbench/blob-resolve/ack` result entity
// summarizing what happened. The subscription engine doesn't consume
// the result, but it shows up in dispatcher traces and is useful for
// tests asserting the chain ran.
func ackEntity(status uint, fields map[string]interface{}) (*handler.Response, error) {
	raw, err := ecf.Encode(fields)
	if err != nil {
		return handler.NewErrorResponse(500, "encode_ack", "encode ack: "+err.Error())
	}
	ent, err := entity.NewEntity("workbench/blob-resolve/ack", cbor.RawMessage(raw))
	if err != nil {
		return handler.NewErrorResponse(500, "build_ack", "build ack entity: "+err.Error())
	}
	return &handler.Response{Status: status, Result: ent}, nil
}
