package main

// Share / sync bridge surface — the folder-sharing flow as a panel can
// drive it: discover → share → offers → accept → receiving.
//
// **Why this file exists.** Every operation below already existed in
// `shellcmd` (share_op.go, sync_op.go) and was reachable from exactly
// one place: a verb typed into a shell. `LocalFilesPanel` could make a
// mount and could not share it; `PeerConnectionsPanel` could dial a peer
// and could not do anything with one. That is AP57 — a read-only surface
// over a read-write model — twice, and `make reachability` cannot see it
// because both panels do have *a* surface. The tell named in AGENTS.md is
// "a panel with no verb in it", and the fix named there is to extract the
// operation so the verb and the panel share it. The operations were
// already extracted. Nothing but the envelope was missing.
//
// So: no flow logic lives here. Every export is a JSON wrapper over one
// `ShellWorkspace` method, which is the same method the verb calls.
// Reimplementing any of the five stages in the panel would put the
// mutual-authorization rules (a sync is mutual; the grant is fixed at
// handshake; the dispatcher must reconnect; a dial-by-address authorizes
// the dialer only) in two places, and those four facts are precisely the
// ones nobody rediscovers by reading the code.
//
// **These are SYNCHRONOUS exports, and two of them block on the
// network.** `ShareOffers` and `ShareAccept` dispatch to a remote peer
// and re-establish a connection first, so they can take seconds. They
// are synchronous anyway, for the reason local_files.go states: the
// async cgo shape carries AP31 (a `*C.char` belongs to the .NET
// marshaller and is freed when the P/Invoke returns, so reading it on a
// goroutine is a use-after-free that reads as the empty string). The
// caller keeps the UI responsive by making the call on a thread-pool
// worker, which is what `SharePanel` does and what `PeerConnectionsPanel`
// already did for its dial.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// --- DTOs -------------------------------------------------------------
//
// Flat and explicit, per AP49: an undeclared field is discarded by
// System.Text.Json in total silence, so the wire shape is written out
// once here and asserted to arrive on the C# side.

type sharePeerDTO struct {
	PeerID    string `json:"peerId"`
	Alias     string `json:"alias"`
	Address   string `json:"address"`
	Connected bool   `json:"connected"`
	// Source is "connected" or "discovery". Kept because they are
	// different claims — a connection is a fact, an mDNS announcement is
	// an advertisement — and a surface that merges them tells the
	// operator a peer is reachable when nobody has reached it.
	Source string `json:"source"`

	// DialAddress is an address we could actually dial, or "" when we
	// have none. Distinct from Address above, which may be an inbound
	// connection's ephemeral source port.
	DialAddress    string `json:"dialAddress"`
	DialAddrSource string `json:"dialAddressSource"`
}

type shareOfferDTO struct {
	Root            string   `json:"root"`
	Title           string   `json:"title"`
	TargetPrefix    string   `json:"targetPrefix"`
	Audience        []string `json:"audience"`
	CreatedAtMillis uint64   `json:"createdAtMillis"`

	// AudienceState is populated for OUR OWN shares and empty for a
	// remote peer's offers — we can say whether we can reach the people
	// we shared with, and nothing about a stranger's audience.
	//
	// It exists so the reciprocal dial is a state the panel can render
	// and act on. Without it the last step of a share is only knowable
	// by clicking and finding out, which is how it ended up being
	// printed as a shell command for the operator to run by hand.
	AudienceState []shareAudienceDTO `json:"audienceState"`
}

// shareAudienceDTO is one peer we shared with, plus whether we can
// complete the reciprocal dial to them without being told an address.
type shareAudienceDTO struct {
	PeerID    string `json:"peerId"`
	Alias     string `json:"alias"`
	Connected bool   `json:"connected"`
	// Address is empty when nothing is known — the case the panel turns
	// into an address field rather than an error.
	Address string `json:"address"`
	// AddressSource distinguishes an address we have dialled before from
	// one the peer announced about itself. Different strengths of claim,
	// so the surface does not present a guess as a fact.
	AddressSource string `json:"addressSource"`
}

type shareSyncDTO struct {
	RemotePeerID string `json:"remotePeerId"`
	RemoteAlias  string `json:"remoteAlias"`
	Root         string `json:"root"`
	SourcePrefix string `json:"sourcePrefix"`
	TargetPrefix string `json:"targetPrefix"`
	// Live means this process holds the subscription handle. False is
	// "restored from disk, routed by the kernel" and NOT "broken" —
	// SyncRow's own doc comment is explicit that a surface must not call
	// it dead, so the panel renders the distinction rather than a health
	// dot that would be wrong half the time after a restart.
	Live bool `json:"live"`
}

type shareMountDTO struct {
	Root         string `json:"root"`
	TargetPrefix string `json:"targetPrefix"`
	SharedWith   int    `json:"sharedWith"`
}

type shareRenderDTO struct {
	OK bool   `json:"ok"`
	Er string `json:"error"`

	// LocalPeerID and ListenAddr are what the OTHER side needs typed at
	// it. Carried in the render rather than composed in the panel so the
	// "they must dial you" instruction can name a real address instead of
	// a placeholder the operator has to go and look up.
	LocalPeerID string `json:"localPeerId"`
	LocalAlias  string `json:"localAlias"`
	ListenAddr  string `json:"listenAddr"`
	Advertised  string `json:"advertisedUrl"`

	Peers  []sharePeerDTO  `json:"peers"`
	Mounts []shareMountDTO `json:"mounts"`
	Shares []shareOfferDTO `json:"shares"`
	Syncs  []shareSyncDTO  `json:"syncs"`

	// Problems are per-record read failures from SharesOffered/Syncs.
	// Surfaced rather than dropped: a share record that will not decode
	// is exactly the thing an operator needs told, and silently omitting
	// it renders a broken share as no share at all.
	Problems []string `json:"problems"`
}

//export ShareRender
func ShareRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("ShareRender", &result)
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	dto := shareRenderDTO{
		OK:          true,
		LocalPeerID: hp.AppPeer.PeerID(),
		LocalAlias:  ws.Local.Alias,
		ListenAddr:  hp.Config.ListenAddr,
		Advertised:  hp.AdvertisedURL,
		Peers:       []sharePeerDTO{},
		Mounts:      []shareMountDTO{},
		Shares:      []shareOfferDTO{},
		Syncs:       []shareSyncDTO{},
		Problems:    []string{},
	}

	// Peers. An error here is reported, not swallowed — "no peers" and
	// "we could not look" are different claims and read identically once
	// the list is empty (AP45).
	peers, perr := ws.Peers()
	if perr != nil {
		dto.Problems = append(dto.Problems, "peers: "+perr.Error())
	}
	for _, p := range peers {
		// DialAddress is NOT p.Address. p.Address is what `Peers` merged
		// from the connection pool and mDNS, and for an inbound
		// connection the pool's entry is the dialer's ephemeral source
		// port — V7 §6.7.1 states it MUST NOT be treated as dialable.
		// This field is the one a dial can be built from, and empty is a
		// meaningful answer the share form turns into a question.
		dialAddr, dialSrc := ws.DialableAddressFor(p.PeerID)
		dto.Peers = append(dto.Peers, sharePeerDTO{
			PeerID:         p.PeerID,
			Alias:          p.Alias,
			Address:        p.Address,
			Connected:      p.Connected,
			Source:         p.Source,
			DialAddress:    dialAddr,
			DialAddrSource: dialSrc,
		})
	}

	offers, oproblems := ws.SharesOffered()
	dto.Problems = append(dto.Problems, oproblems...)
	// connected is a peer-id set built once from the pool, so the loop
	// below does not re-scan it per audience member.
	connected := map[string]bool{}
	for _, c := range peers {
		if c.Connected {
			connected[c.PeerID] = true
		}
	}
	// sharedWith counts the audience per root so a mount row can say
	// "shared with 2" without the panel re-deriving it from the offer
	// list and getting a different answer.
	sharedWith := map[string]int{}
	for _, o := range offers {
		aud := o.Audience
		if aud == nil {
			aud = []string{}
		}
		sharedWith[o.Root] = len(aud)
		state := make([]shareAudienceDTO, 0, len(aud))
		for _, who := range aud {
			addr, src := ws.DialableAddressFor(who)
			state = append(state, shareAudienceDTO{
				PeerID:        who,
				Alias:         ws.AliasFor(who),
				Connected:     connected[who],
				Address:       addr,
				AddressSource: src,
			})
		}
		dto.Shares = append(dto.Shares, shareOfferDTO{
			Root:            o.Root,
			Title:           o.Title,
			TargetPrefix:    o.TargetPrefix,
			Audience:        aud,
			CreatedAtMillis: o.CreatedAtMillis,
			AudienceState:   state,
		})
	}

	for _, m := range localMountRoots(hp) {
		dto.Mounts = append(dto.Mounts, shareMountDTO{
			Root:         m.root,
			TargetPrefix: m.prefix,
			SharedWith:   sharedWith[m.root],
		})
	}

	rows, sproblems := ws.Syncs()
	dto.Problems = append(dto.Problems, sproblems...)
	for _, r := range rows {
		dto.Syncs = append(dto.Syncs, shareSyncDTO{
			RemotePeerID: r.RemotePeerID,
			RemoteAlias:  r.RemoteAlias,
			Root:         r.Root,
			SourcePrefix: r.SourcePrefix,
			TargetPrefix: r.TargetPrefix,
			Live:         r.Live,
		})
	}

	return marshalReply(dto, "share render")
}

// --- Offers (REMOTE read) ---------------------------------------------

type shareOffersReplyDTO struct {
	OK     bool            `json:"ok"`
	Er     string          `json:"error"`
	Peer   string          `json:"peer"`
	Offers []shareOfferDTO `json:"offers"`
}

//export ShareOffers
func ShareOffers(peerHandle C.int64_t, peer *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareOffers", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	who := strings.TrimSpace(C.GoString(peer))
	if who == "" {
		return C.CString(`{"ok":false,"error":"no peer given"}`)
	}

	offers, err := ws.Offers(who)
	if err != nil {
		return marshalReply(shareOffersReplyDTO{
			Er: err.Error(), Peer: who, Offers: []shareOfferDTO{},
		}, "share offers")
	}

	dto := shareOffersReplyDTO{OK: true, Peer: who, Offers: []shareOfferDTO{}}
	for _, o := range offers {
		aud := o.Audience
		if aud == nil {
			aud = []string{}
		}
		dto.Offers = append(dto.Offers, shareOfferDTO{
			Root:            o.Root,
			Title:           o.Title,
			TargetPrefix:    o.TargetPrefix,
			Audience:        aud,
			CreatedAtMillis: o.CreatedAtMillis,
			// Empty, not nil: a remote peer's offer carries an audience
			// we can say nothing about, and `[]` deserializes to an
			// empty list where `null` deserializes to one that throws on
			// first use.
			AudienceState: []shareAudienceDTO{},
		})
	}
	return marshalReply(dto, "share offers")
}

// --- Complete (the reciprocal dial) ------------------------------------

type shareCompleteReplyDTO struct {
	OK            bool   `json:"ok"`
	Er            string `json:"error"`
	PeerID        string `json:"peerId"`
	PeerAlias     string `json:"peerAlias"`
	Address       string `json:"address"`
	AddressSource string `json:"addressSource"`
	Connected     bool   `json:"connected"`
	// NeedsAddress is not an error and a surface must not render it as
	// one: nothing is broken, we simply do not know where this peer
	// listens. The remedy is a field to type an address into.
	NeedsAddress bool   `json:"needsAddress"`
	Note         string `json:"note"`
}

//export ShareComplete
func ShareComplete(peerHandle C.int64_t, peer *C.char, address *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareComplete", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	out, err := ws.CompleteShare(
		strings.TrimSpace(C.GoString(peer)),
		strings.TrimSpace(C.GoString(address)))
	if err != nil {
		return marshalReply(shareCompleteReplyDTO{Er: err.Error()}, "share complete")
	}
	return marshalReply(shareCompleteReplyDTO{
		OK:            true,
		PeerID:        out.PeerID,
		PeerAlias:     out.PeerAlias,
		Address:       out.Address,
		AddressSource: out.AddressSource,
		Connected:     out.Connected,
		NeedsAddress:  out.NeedsAddress,
		Note:          out.Note,
	}, "share complete")
}

// --- Share / Unshare ---------------------------------------------------

type shareCreateReplyDTO struct {
	OK           bool     `json:"ok"`
	Er           string   `json:"error"`
	Root         string   `json:"root"`
	TargetPrefix string   `json:"targetPrefix"`
	PeerID       string   `json:"peerId"`
	PeerAlias    string   `json:"peerAlias"`
	PolicyPath   string   `json:"policyPath"`
	GrantSummary string   `json:"grantSummary"`
	Audience     []string `json:"audience"`

	// Reconnected is the difference between a share that is in force and
	// one that will be in force after some unrelated restart. The grant
	// set is assembled at handshake, so a policy written on a live
	// connection is inert until it is re-established — ShareOutcome
	// carries the field for exactly this reason and a surface that drops
	// it reports success for a share that does nothing.
	Reconnected   bool   `json:"reconnected"`
	ReconnectNote string `json:"reconnectNote"`
}

//export ShareCreate
func ShareCreate(peerHandle C.int64_t, root *C.char, peer *C.char, title *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareCreate", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	out, err := ws.Share(shellcmd.ShareRequest{
		Root:  strings.TrimSpace(C.GoString(root)),
		Peer:  strings.TrimSpace(C.GoString(peer)),
		Title: strings.TrimSpace(C.GoString(title)),
		// The clock is read here rather than in the operation so the
		// operation stays deterministic for its tests — ShareRequest
		// carries NowMillis for that reason.
		NowMillis: uint64(time.Now().UnixMilli()),
	})
	if err != nil {
		return marshalReply(shareCreateReplyDTO{Er: err.Error(), Audience: []string{}}, "share create")
	}

	aud := out.Audience
	if aud == nil {
		aud = []string{}
	}
	return marshalReply(shareCreateReplyDTO{
		OK:            true,
		Root:          out.Root,
		TargetPrefix:  out.TargetPrefix,
		PeerID:        out.PeerID,
		PeerAlias:     out.PeerAlias,
		PolicyPath:    out.PolicyPath,
		GrantSummary:  out.GrantSummary,
		Audience:      aud,
		Reconnected:   out.Reconnected,
		ReconnectNote: out.ReconnectNote,
	}, "share create")
}

type shareRevokeReplyDTO struct {
	OK            bool     `json:"ok"`
	Er            string   `json:"error"`
	Root          string   `json:"root"`
	PeerID        string   `json:"peerId"`
	PolicyRemoved bool     `json:"policyRemoved"`
	OfferRemoved  bool     `json:"offerRemoved"`
	StillOffered  []string `json:"stillOffered"`
	// Caveat is the sentence a surface MUST print: withdrawal stops the
	// NEXT handshake carrying the grant and does not reach into a live
	// connection that already holds one. UnshareOutcome carries it as a
	// field so it cannot be dropped by a renderer paraphrasing.
	Caveat string `json:"caveat"`
}

//export ShareRevoke
func ShareRevoke(peerHandle C.int64_t, root *C.char, peer *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareRevoke", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	out, err := ws.Unshare(strings.TrimSpace(C.GoString(root)), strings.TrimSpace(C.GoString(peer)))
	if err != nil {
		return marshalReply(shareRevokeReplyDTO{Er: err.Error(), StillOffered: []string{}}, "share revoke")
	}
	still := out.StillOffered
	if still == nil {
		still = []string{}
	}
	return marshalReply(shareRevokeReplyDTO{
		OK:            true,
		Root:          out.Root,
		PeerID:        out.PeerID,
		PolicyRemoved: out.PolicyRemoved,
		OfferRemoved:  out.OfferRemoved,
		StillOffered:  still,
		Caveat:        out.Caveat,
	}, "share revoke")
}

// --- Accept ------------------------------------------------------------

type shareAcceptReplyDTO struct {
	OK           bool   `json:"ok"`
	Er           string `json:"error"`
	PeerID       string `json:"peerId"`
	PeerAlias    string `json:"peerAlias"`
	Root         string `json:"root"`
	PolicyPath   string `json:"policyPath"`
	GrantSummary string `json:"grantSummary"`

	SourcePrefix   string `json:"sourcePrefix"`
	TargetPrefix   string `json:"targetPrefix"`
	TargetRoot     string `json:"targetRoot"`
	SubscriptionID string `json:"subscriptionId"`

	Reconnected   bool   `json:"reconnected"`
	ReconnectNote string `json:"reconnectNote"`

	// PublisherMustDial is the one step in the whole flow that neither
	// side's verb can complete on its own: over a dial-by-address
	// connection the kernel's reciprocal grant is gated on
	// `EstablishedViaRendezvousKey()`, so accepting buys us the authority
	// to subscribe and fetch and gives the publisher nothing — while
	// DELIVERY runs publisher→us. If they never dial back, the folder
	// stays empty and there is no error on this side to see. The panel
	// renders this as an instruction, not a footnote.
	PublisherMustDial string `json:"publisherMustDial"`

	// Backfill is the catch-up over files that were ALREADY in the
	// remote folder. It is the operator's actual question — "did my
	// files come across" — and before it existed the honest answer on
	// this surface was "no, and nothing will tell you".
	Backfill backfillDTO `json:"backfill"`

	// LocalRoot / MountedDir / CreatedMount describe WHERE THE FILES
	// WENT, which is the operator's first question and had no answer on
	// this surface at all.
	//
	// CreatedMount distinguishes "this accept bridged a directory on
	// your disk" from "it used one you had already bridged". They are
	// different events and a panel that renders them identically is
	// hiding the one that needs confirming.
	LocalRoot    string `json:"localRoot"`
	MountedDir   string `json:"mountedDir"`
	CreatedMount bool   `json:"createdMount"`

	// NonEmpty is set when the accept was REFUSED because the chosen
	// directory already had files in it, with the count, so the panel
	// can say "this folder has 12 items" beside a button that proceeds.
	// Declared here because an undeclared field crosses this boundary
	// and is discarded in silence (AP49).
	NonEmpty        bool   `json:"nonEmpty"`
	NonEmptyPath    string `json:"nonEmptyPath"`
	NonEmptyEntries int    `json:"nonEmptyEntries"`
}

//export ShareAccept
func ShareAccept(peerHandle C.int64_t, peer *C.char, root *C.char, directory *C.char,
	allowNonEmpty C.int) (result *C.char) {
	defer recoverToErrorEnvelope("ShareAccept", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	out, err := ws.Accept(shellcmd.AcceptRequest{
		Peer:          strings.TrimSpace(C.GoString(peer)),
		Root:          strings.TrimSpace(C.GoString(root)),
		Directory:     strings.TrimSpace(C.GoString(directory)),
		AllowNonEmpty: allowNonEmpty != 0,
	})
	if err != nil {
		// A non-empty target is the one refusal the operator is expected
		// to be able to overrule, so it crosses as STRUCTURE rather than
		// as prose a panel would have to parse.
		if ne, ok := shellcmd.AsNonEmptyDirectory(err); ok {
			return marshalReply(shareAcceptReplyDTO{
				Er:              ne.Error(),
				NonEmpty:        true,
				NonEmptyPath:    ne.Path,
				NonEmptyEntries: ne.Entries,
			}, "share accept")
		}
		// Accept's error text is load-bearing — it deliberately leaves the
		// delivery grant in place and says so. Passed through verbatim
		// rather than summarized.
		return marshalReply(shareAcceptReplyDTO{Er: err.Error()}, "share accept")
	}

	mountedDir := ""
	if out.Mounted != nil {
		mountedDir = out.Mounted.FilesystemRoot
	}
	return marshalReply(shareAcceptReplyDTO{
		OK:                true,
		PeerID:            out.PeerID,
		PeerAlias:         out.PeerAlias,
		Root:              out.Root,
		PolicyPath:        out.PolicyPath,
		GrantSummary:      out.GrantSummary,
		SourcePrefix:      out.Sync.SourcePrefix,
		TargetPrefix:      out.Sync.TargetPrefix,
		TargetRoot:        out.Sync.TargetRoot,
		SubscriptionID:    out.Sync.SubscriptionID,
		Reconnected:       out.Reconnected,
		ReconnectNote:     out.ReconnectNote,
		PublisherMustDial: out.PublisherMustDial,
		Backfill:          backfillToDTO(out.Sync.Backfill, out.Sync.BackfillSkipped),
		LocalRoot:         out.LocalRoot,
		MountedDir:        mountedDir,
		CreatedMount:      out.Mounted != nil,
	}, "share accept")
}

// --- Unsync ------------------------------------------------------------

type shareUnsyncReplyDTO struct {
	OK       bool   `json:"ok"`
	Er       string `json:"error"`
	PeerID   string `json:"peerId"`
	Root     string `json:"root"`
	Found    bool   `json:"found"`
	CloseErr string `json:"subscriptionCloseError"`
	// Note states what unsync deliberately does NOT do: the mount and
	// everything already received stay. Silence there reads as "the
	// files are gone", which is the opposite of true.
	Note string `json:"note"`
}

//export ShareUnsync
func ShareUnsync(peerHandle C.int64_t, peer *C.char, root *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareUnsync", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	out, err := ws.Unsync(strings.TrimSpace(C.GoString(peer)), strings.TrimSpace(C.GoString(root)))
	if err != nil {
		return marshalReply(shareUnsyncReplyDTO{Er: err.Error()}, "share unsync")
	}
	closeErr := ""
	if out.SubscriptionCloseErr != nil {
		closeErr = out.SubscriptionCloseErr.Error()
	}
	return marshalReply(shareUnsyncReplyDTO{
		OK:       true,
		PeerID:   out.RemotePeerID,
		Root:     out.Root,
		Found:    out.Found,
		CloseErr: closeErr,
		Note: "the local mount and every document already received are left in place; " +
			"this stops further deliveries only.",
	}, "share unsync")
}

// --- Resync ------------------------------------------------------------

// backfillDTO mirrors shellcmd.BackfillResult across the boundary.
//
// Every count is carried separately rather than pre-rendered into one
// string, because the panel distinguishes them: "transferred" is
// progress, "already current" is the confirmation an operator re-runs
// the pull to obtain, and "failed" must never be summarized away. The
// Summary field is a convenience for a one-line slot, not the source of
// truth — a surface that renders only Summary loses the failure count's
// separability, which is the thing this whole area got wrong before.
type backfillDTO struct {
	Scanned        int      `json:"scanned"`
	Materialized   int      `json:"materialized"`
	AlreadyCurrent int      `json:"alreadyCurrent"`
	Skipped        int      `json:"skipped"`
	Failed         int      `json:"failed"`
	Truncated      bool     `json:"truncated"`
	Unreachable    bool     `json:"unreachable"`
	ListError      string   `json:"listError"`
	Errors         []string `json:"errors"`
	Summary        string   `json:"summary"`
	Ran            bool     `json:"ran"`
}

func backfillToDTO(res shellcmd.BackfillResult, skipped bool) backfillDTO {
	errs := res.Errors
	if errs == nil {
		errs = []string{}
	}
	return backfillDTO{
		Scanned:        res.Scanned,
		Materialized:   res.Materialized,
		AlreadyCurrent: res.AlreadyCurrent,
		Skipped:        res.Skipped,
		Failed:         res.Failed,
		Truncated:      res.Truncated,
		Unreachable:    res.Unreachable,
		ListError:      res.ListError,
		Errors:         errs,
		Summary:        res.Summary(),
		Ran:            !skipped,
	}
}

type shareResyncReplyDTO struct {
	OK       bool        `json:"ok"`
	Er       string      `json:"error"`
	PeerID   string      `json:"peerId"`
	Root     string      `json:"root"`
	Backfill backfillDTO `json:"backfill"`
}

// ShareResync re-pulls everything currently in a synced folder, without
// touching the subscription.
//
// The panel needs this as its own verb rather than as a re-Accept: a
// re-Accept rewrites policy and re-establishes a connection, which are
// side effects an operator asking "is my folder up to date?" did not ask
// for and cannot undo.
//
//export ShareResync
func ShareResync(peerHandle C.int64_t, peer *C.char, root *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareResync", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	p := strings.TrimSpace(C.GoString(peer))
	r := strings.TrimSpace(C.GoString(root))
	res, err := ws.Resync(p, r)
	if err != nil {
		return marshalReply(shareResyncReplyDTO{Er: err.Error()}, "share resync")
	}
	return marshalReply(shareResyncReplyDTO{
		OK:       true,
		PeerID:   p,
		Root:     r,
		Backfill: backfillToDTO(res, false),
	}, "share resync")
}

// --- Forget ------------------------------------------------------------

type shareForgetOneDTO struct {
	PeerID          string   `json:"peerId"`
	PeerAlias       string   `json:"peerAlias"`
	SyncsStopped    []string `json:"syncsStopped"`
	OffersWithdrawn []string `json:"offersWithdrawn"`
	PolicyRemoved   bool     `json:"policyRemoved"`
	Disconnected    bool     `json:"disconnected"`
	Problems        []string `json:"problems"`
	Summary         string   `json:"summary"`
}

type shareForgetReplyDTO struct {
	OK      bool                `json:"ok"`
	Er      string              `json:"error"`
	Peers   []shareForgetOneDTO `json:"peers"`
	Caveat  string              `json:"caveat"`
	AllMode bool                `json:"allMode"`
}

func forgetToDTO(o shellcmd.ForgetOutcome) shareForgetOneDTO {
	nz := func(in []string) []string {
		if in == nil {
			return []string{}
		}
		return in
	}
	return shareForgetOneDTO{
		PeerID:          o.PeerID,
		PeerAlias:       o.PeerAlias,
		SyncsStopped:    nz(o.SyncsStopped),
		OffersWithdrawn: nz(o.OffersWithdrawn),
		PolicyRemoved:   o.PolicyRemoved,
		Disconnected:    o.Disconnected,
		Problems:        nz(o.Problems),
		Summary:         o.Summary(),
	}
}

// forgetCaveatText is the sentence the panel MUST render. Both halves
// of it look like a bug to an operator who just asked for a clean slate:
// their files are still there (correct — they are the operator's files),
// and the OTHER machine still remembers this one (correct — a peer
// cannot reach into another peer's tree, which is the whole security
// model). Left unsaid, the second one makes the next "clean" test
// silently dirty on one side.
const forgetCaveatText = "Files already received are untouched. The other peer still remembers " +
	"YOU — run Forget on that machine too before treating the next run as a clean test."

// ShareForget drops what this peer remembers about another peer.
//
// Pass an empty peer to forget every remote peer.
//
//export ShareForget
func ShareForget(peerHandle C.int64_t, peer *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShareForget", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	target := strings.TrimSpace(C.GoString(peer))

	if target == "" {
		outs, err := ws.ForgetAll()
		if err != nil {
			return marshalReply(shareForgetReplyDTO{Er: err.Error()}, "share forget")
		}
		peers := make([]shareForgetOneDTO, 0, len(outs))
		for _, o := range outs {
			peers = append(peers, forgetToDTO(o))
		}
		return marshalReply(shareForgetReplyDTO{
			OK: true, Peers: peers, Caveat: forgetCaveatText, AllMode: true,
		}, "share forget")
	}

	out, err := ws.Forget(target)
	if err != nil {
		return marshalReply(shareForgetReplyDTO{Er: err.Error()}, "share forget")
	}
	return marshalReply(shareForgetReplyDTO{
		OK:     true,
		Peers:  []shareForgetOneDTO{forgetToDTO(out)},
		Caveat: forgetCaveatText,
	}, "share forget")
}

// --- internals ---------------------------------------------------------

// shareWorkspace resolves a peer handle to its workspace, returning the
// error envelope to send back when it cannot. Every export starts with
// this rather than repeating the three nil checks, and the Workspace nil
// check is not paranoia: a handle can outlive a Destroy.
func shareWorkspace(peerHandle C.int64_t) (*shellcmd.ShellWorkspace, *shellboot.HostedPeer, string) {
	if manager == nil {
		return nil, nil, errNotInit
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return nil, nil, errBadPeer
	}
	if hp.Workspace == nil || hp.Workspace.Local == nil || hp.Workspace.Local.Peer == nil {
		return nil, nil, `{"ok":false,"error":"peer has no workspace"}`
	}
	return hp.Workspace, hp, ""
}

type mountRootRef struct {
	root   string
	prefix string
}

// localMountRoots lists this peer's mount roots — the set a share can
// name. Read through the same LocalFilesModel the Local Files panel
// renders, so the two surfaces cannot disagree about what is mounted.
func localMountRoots(hp *shellboot.HostedPeer) []mountRootRef {
	out := []mountRootRef{}
	for _, m := range wb.NewLocalFilesModel(hp.AppPeer.Store()).Render().Mounts {
		if m.Err != "" {
			// A mount whose config will not decode still gets listed, by
			// root name, because it is a thing the operator has and a
			// share against it will fail with a real reason. Dropping it
			// would render a broken mount as no mount.
			out = append(out, mountRootRef{root: m.Root})
			continue
		}
		out = append(out, mountRootRef{root: m.Root, prefix: m.Prefix})
	}
	return out
}

func marshalReply(v any, what string) *C.char {
	b, err := json.Marshal(v)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, "marshal "+what))
	}
	return C.CString(string(b))
}
