package main

// Publish bridge surface — the produce side, which the GUI could not
// reach at all.
//
// **Why this file exists.** Before it, publishing anything from this tree
// meant running `entity-publish`, a separate binary that opens a peer's
// store off disk. The whole consume journey is built out in the Browser
// panel, the Origin Inspector and the Local Site panel; the act those
// three exist to read had no verb and no pixel. That is D23's shape, and
// the reason the shell verb and this file are one change.
//
// **Render and publish are separate exports, for `status.go`'s reason
// and a sharper version of it.** A reconcile pass dials, so wiring it to
// a wake makes a dialer out of an open panel. A publish MINTS — it signs
// a new root and increments `seq` — so wiring *it* to a wake would make
// an open panel announce a new release of the site every time the window
// was focused. `PublishRender` reads and mints nothing; the DTO carries
// `minted` so the surface says which it is looking at rather than
// implying the stronger one.
//
// **Synchronous exports, like the rest of the share surface** (AP31: a
// `*C.char` belongs to the .NET marshaller and is freed when the P/Invoke
// returns, so reading it on a goroutine is a use-after-free that reads as
// the empty string). `PublishNow` walks the tree and may re-handshake
// every declared peer, so the caller runs it on a thread-pool worker.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"context"
	"time"

	"entity-workbench-go/shellcmd"
)

// --- DTO --------------------------------------------------------------
//
// Flat and explicit, per AP49. Every field here is either a disclosure
// fact or a reason something is not finished, and an undeclared one is
// dropped by System.Text.Json in total silence — which on this surface
// renders as "your site is published and nothing is wrong".

type publishDTO struct {
	PeerID   string `json:"peerId"`
	Prefix   string `json:"prefix"`
	Seq      uint64 `json:"seq"`
	RootHash string `json:"rootHash"`
	Bindings int    `json:"bindings"`

	// ContentSet is how those keys were CHOSEN — "" for the prefix scan,
	// "feed" for `A-38` ruling (D)'s curated set.
	//
	// **`shellcmd.PublishOutcome.ContentSet` says a surface MUST render
	// this, and until 2026-09-16 it did not cross the boundary at all.**
	// After `A-38` the prefix no longer implies the set: a peer-root
	// publish is 9 keys or 399 depending on a choice the prefix does not
	// record, and the difference is whether an operator's folder paths and
	// their peers' addresses are in the artifact. `prefix` and `bindings`
	// together still do not say it — 9 under `/` is only legible if you
	// already know a curated set exists.
	ContentSet string `json:"contentSet"`

	// Minted separates the act from the read. See the file header.
	Minted bool `json:"minted"`

	// NarrowedFrom is what the PREVIOUS published root committed to, when
	// this publish stopped committing to part of it. A peer has exactly
	// one published root, so this is a site going dark under a valid
	// signature — the publisher is the only party who can see it coming.
	NarrowedFrom string `json:"narrowedFrom"`

	// PublicPresent / PublicOurs / PublicPrefix / PublicStale are the
	// `default` policy row.
	//
	// `publicOurs` is not pedantry: a hand-written `default` row is
	// somebody's deliberate act, and a surface that offered to "unpublish"
	// it would delete authorization the operator wrote on purpose.
	PublicPresent bool   `json:"publicPresent"`
	PublicOurs    bool   `json:"publicOurs"`
	PublicPrefix  string `json:"publicPrefix"`
	PublicStale   bool   `json:"publicStale"`
	PublicSummary string `json:"publicSummary"`

	// Listening / Advertised are whether a reader could arrive. Both, and
	// never one: a peer with a door and no address on it and a peer with
	// an address and no door are different faults that send an operator
	// to different places.
	Listening  string   `json:"listening"`
	Advertised []string `json:"advertised"`

	StaticDir    string `json:"staticDir"`
	StaticOrigin string `json:"staticOrigin"`
	StaticPaths  int    `json:"staticPaths"`

	Problems []string `json:"problems"`
	Er       string   `json:"error"`
}

func publishOutcomeToDTO(out shellcmd.PublishOutcome) publishDTO {
	adv := out.Reach.Advertised
	if adv == nil {
		// Never null across the boundary: a C# `string[]?` that arrives
		// null and one that arrives empty take different code paths in
		// every renderer that touches it, and the difference here is not
		// meaningful.
		adv = []string{}
	}
	probs := out.Problems
	if probs == nil {
		probs = []string{}
	}
	return publishDTO{
		PeerID:        out.PeerID,
		Prefix:        out.Prefix,
		Seq:           out.Seq,
		RootHash:      out.RootHash,
		Bindings:      out.Bindings,
		ContentSet:    out.ContentSet,
		Minted:        out.Minted,
		NarrowedFrom:  out.NarrowedFrom,
		PublicPresent: out.Public.Present,
		PublicOurs:    out.Public.Ours,
		PublicPrefix:  out.Public.Prefix,
		PublicStale:   out.Public.Stale,
		PublicSummary: out.Public.Summary,
		Listening:     out.Reach.Listening,
		Advertised:    adv,
		StaticDir:     out.StaticDir,
		StaticOrigin:  out.StaticOrigin,
		StaticPaths:   out.StaticPaths,
		Problems:      probs,
	}
}

// panelPublishTimeout bounds one publish driven from the GUI.
//
// A publish builds a CHAMP trie over every binding under the prefix and,
// with `-public`, re-derives and re-handshakes every declared peer. The
// second half is what makes this longer than a status render: a declared
// peer that is switched off is the normal case, and the reconnect waits
// on it.
const panelPublishTimeout = 60 * time.Second

// PublishRender reads what is published and mints NOTHING.
//
// Safe on a wake and safe on a timer, which `PublishNow` is not.
//
//export PublishRender
func PublishRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("PublishRender", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelPublishTimeout)
	defer cancel()
	out, err := ws.PublishStatus(ctx)
	if err != nil {
		return marshalReply(publishDTO{Er: err.Error()}, "publish render")
	}
	return marshalReply(publishOutcomeToDTO(out), "publish render")
}

// PublishNow signs a new root over this peer's sites, and optionally
// changes who may read it.
//
// `public` is a tri-state on purpose and is not a bool: 1 writes the
// public grant, -1 removes it, 0 leaves it exactly as it is. A bool would
// force every re-publish to also restate a disclosure decision, so an
// operator pressing "Publish" to pick up a new page would silently
// un-publish their site — which is a change of who can read it, made by a
// button that does not say so.
//
// `feed` non-zero asks for `A-38` ruling (D)'s curated binding set: publish
// at the peer root — the only prefix containing both `app/feed/…` and the
// `system/signature/…` keys that attribute an entry — and commit to exactly
// the entries, the index head and pages, and each entry's signature.
//
// **It is a second parameter and not a mode enum**, because the two
// questions are independent: *what does this root commit to* and *who may
// read it*. Collapsing them would make "publish my feed" also restate a
// disclosure decision, which is the same mistake the `public` tri-state
// exists to avoid one field over.
//
// It does NOT imply the whole-peer disclosure acknowledgement, and that is
// the ruling rather than a shortcut: the acknowledgement exists for the
// keys a prefix SCAN sweeps along, and a curated set sweeps none. There is
// therefore deliberately no way to ask for a peer-root SCAN from this
// bridge — that is the `-whole-peer` flag on the shell verb, where an
// operator types the acknowledgement out.
//
//export PublishNow
func PublishNow(peerHandle C.int64_t, public C.int, feed C.int) (result *C.char) {
	defer recoverToErrorEnvelope("PublishNow", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelPublishTimeout)
	defer cancel()
	out, err := ws.Publish(ctx, shellcmd.PublishRequest{
		Public:   public > 0,
		Unpublic: public < 0,
		Feed:     feed != 0,
	})
	if err != nil {
		return marshalReply(publishDTO{Er: err.Error()}, "publish now")
	}
	return marshalReply(publishOutcomeToDTO(out), "publish now")
}
