package shellcmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// publish_op.go — publishing as one act with two projections, extracted
// so the verb and the panel share it (AP57).
//
// # What "publish" means here, and the sentence that decides the shape
//
// **Publishing is minting a signed root. Everything else is a
// projection of it.** `publish.MintRoot` writes the
// `system/peer/published-root` and its signature into this peer's own
// tree; after that a reader can get the bytes over HTTP from a static
// directory, or by dispatch from this process, and the verification
// chain is byte-identical either way (`publish/live_and_static_test.go`).
//
// So this file does not have a "static publish" and a "live publish".
// It has one publish, and two questions about what the operator wants
// done with it: write a directory, and/or let a stranger ask.
//
// # Three facts a publish cannot supply for itself, and must therefore report
//
// The act of publishing does not make a peer reachable, does not make
// anybody authorized, and does not make a static directory reach a web
// server. All three are somebody else's step, and all three are
// invisible from inside the act — which is exactly the shape of AP84:
// *a diagnosis whose visibility depends on the operator's layout is not
// a surface*. So [PublishOutcome] carries the reach state, the grant
// state and the upload reminder, and every surface renders them beside
// the success line rather than instead of it.
//
// # Why the public grant is not a declaration
//
// The sharing flow is a control loop: `share` / `accept` write
// declarations and the reconciler derives the policy rows (AP68, one
// writer). The public row is deliberately outside that loop, and it is
// safe to be:
//
//   - `reconcilePolicies` only writes rows for peers in its desired map,
//     which is built from declared DEVICES. It never enumerates the
//     table and never removes a row it does not recognise, so a
//     `default` row is not clobbered by a pass. (Checked, not assumed —
//     this is the failure `unshare` had, and it was a real one.)
//   - `default` is not a peer-id, so no device declaration can ever
//     collide with it.
//
// The rule the loop encodes — *a verb that changes what the operator
// wants MUST write a declaration* — is about state a later pass would
// otherwise undo. Nothing undoes this one.

// PublishRequest is what an operator asked for.
type PublishRequest struct {
	// Prefix is the peer-relative tree prefix the signed root will
	// commit to. Empty means [workbench.SitesSubpath] + "/", i.e.
	// every site this peer holds.
	Prefix string

	// OutputDir, when non-empty, also emits the static corridor there.
	OutputDir string

	// OriginURL is the HTTP origin the static directory will be served
	// from. It is baked into the emitted transport profile, so it is
	// wanted at emit time and not at upload time.
	OriginURL string

	// Public, when true, writes the `default` policy row so any peer
	// that can dial this one may read the published prefix and verify
	// it. This is the only field here that changes who can see
	// something.
	Public bool

	// Unpublic, when true, removes the `default` row. Mutually
	// exclusive with Public; a request that sets both is refused rather
	// than resolved in some order, because an operator who typed both
	// does not know what they asked for.
	Unpublic bool

	// AllowWholePeer acknowledges that the chosen prefix spans the
	// system boundary — the root will commit to this peer's own device
	// declarations, folder declarations with their local filesystem
	// paths, ingested documents and capability policy table, and with
	// `-out` every one of those is written into a directory whose next
	// step is an upload.
	//
	// Refused without it (`publish.disclosureAcrossSystem`). It is a
	// separate field from Prefix rather than a magic prefix value
	// because **the disclosure is the thing being consented to, not the
	// prefix** — an operator widening a prefix for `A-36`'s reason is
	// solving an attribution problem and has no reason to be thinking
	// about their folder list.
	AllowWholePeer bool

	// At pins `published_at`. Zero means now.
	At time.Time
}

// PublicGrantState is what the `default` policy row says right now.
//
// Three states and not two: **present**, **absent**, and *present but
// covering a different prefix than the current published root*. The
// third is the one worth the type — it is a grant pointing at what this
// peer used to publish, which is a live disclosure of a prefix the
// operator has stopped thinking about.
type PublicGrantState struct {
	// Present reports whether a `default` row exists at all.
	Present bool

	// Ours reports whether the row has the shape [workbench.PublicSiteGrants]
	// produces. A hand-written `default` row is somebody's deliberate
	// act and this code will read it, report it, and never rewrite or
	// remove it.
	Ours bool

	// Prefix is what the row authorizes, recovered from the row itself.
	Prefix string

	// Summary is the human rendering of the grants, as the access
	// surface shows them.
	Summary string

	// Stale reports Present && Ours && Prefix != the published root's
	// prefix.
	Stale bool
}

// PublishReach is whether a stranger could actually arrive.
//
// Named as an observation rather than a status: this peer can see that
// it is listening and what it advertised, and it cannot see whether
// anything out there can route to it. Two of the three fields are
// facts about this process and the third is the one nobody local can
// answer, which is why it is prose and not a boolean.
type PublishReach struct {
	// Listening is the bound address, or "" when this peer bound no
	// listener at all.
	//
	// A peer that is not listening has published a site that is live in
	// no sense: the bytes are signed and in the tree and there is no
	// door. The static projection is unaffected, which is why this is
	// reported rather than refused on.
	Listening string

	// Advertised is every dial URL this peer self-published as a
	// transport profile, in profile-id order.
	//
	// Without one a reader who knows only the peer-id cannot find an
	// address. With one, a reader that has any route to this peer's
	// tree can.
	//
	// **A LIST, AND DELIBERATELY NOT A CHOICE.** §6.5.1a D1 ranking is
	// the READER's — core-go implements it fully in
	// `collectProfileCandidates`, including the `primary`-defaults-to-0
	// clause, and it runs on the machine doing the dialling. A publisher
	// that picked one of its own addresses here would be simulating a
	// decision it does not make, and would be the third copy of D1 in
	// this cohort. What a publisher can honestly say is *these are the
	// addresses I published*.
	Advertised []string

	// Note is the sentence the operator needs, and is empty when there
	// is nothing to say.
	Note string
}

// PublishOutcome is everything a surface prints about one publish.
type PublishOutcome struct {
	PeerID   string
	Prefix   string
	Seq      uint64
	RootHash string

	// Bindings is how many tree keys the signed root commits to.
	Bindings int

	// NarrowedFrom is the prefix the PREVIOUS published root committed
	// to, set only when this publish stopped committing to part of it.
	//
	// A peer has exactly ONE published root, so publishing `sites/notes/`
	// over a root that committed to `sites/` takes every other site
	// dark — with a perfectly valid signature over the replacement, so
	// a reader sees "absent", which carries no information. The
	// publisher is the only party who can see this coming; saying it is
	// not optional.
	NarrowedFrom string

	// Minted is false for a status read and true for a publish. Carried
	// in the outcome rather than remembered by the caller, for
	// `status`'s reason: a surface that has to recall which function it
	// called in order to caption its output will eventually caption it
	// wrong, in the confident direction.
	Minted bool

	// Static describes the directory projection, zero when none was
	// asked for.
	StaticDir    string
	StaticOrigin string
	StaticBytes  int64
	StaticPaths  int

	Public PublicGrantState
	Reach  PublishReach

	// Problems are things that are wrong and that the operator can act
	// on. Never fatal on their own — a publish that succeeded and is
	// unreachable still succeeded.
	Problems []string
}

// DefaultPublishPrefix is what `publish` commits to when the operator
// names no prefix: every site this peer holds.
//
// Not `sites/{id}/`. A peer has one published root, so a per-site
// default would make publishing a second site silently unpublish the
// first — and the failure would present to a reader as the first site
// having been withdrawn on purpose. The site-scoped form stays
// available and says what it is replacing.
//
// It is the FIRST-PUBLISH default only; see [ShellWorkspace.currentPublishPrefix].
var DefaultPublishPrefix = workbench.SitesSubpath + "/"

// currentPublishPrefix is what an unqualified `publish` commits to: the
// prefix this peer's published root ALREADY commits to, or
// [DefaultPublishPrefix] when it has never published.
//
// **The sticky default is the fix for a surprise measured by running the
// flow** (AP71). With a constant default, an operator who deliberately
// published `sites/notes/` and later ran `publish -private` — an access
// instruction, in their reading — silently re-signed a root over
// `sites/`. Nobody asked for that. It happens to be a widening, which is
// the harmless direction, but the same constant would have narrowed just
// as quietly for anyone whose deliberate prefix was the broader one.
//
// The rule underneath: **a default is what to do when nothing is known,
// and after the first publish something IS known.** Re-publishing is
// then idempotent in prefix, which is what makes `publish` safe to run
// after adding a page — the common case, and the one a constant default
// gets wrong in a way no error message appears for.
func (ws *ShellWorkspace) currentPublishPrefix(ctx context.Context) string {
	pr, err := ws.Local.Peer.ReadPublishedRoot(ctx, ws.Local.Peer.PeerID())
	if err != nil || pr.Data.Prefix == "" {
		return DefaultPublishPrefix
	}
	// §3.3a spells the universal tree "/" inside the root; the
	// peer-relative form `prepareMint` lists on is "". Every other prefix
	// is already peer-relative and passes through.
	if pr.Data.Prefix == "/" {
		return ""
	}
	return pr.Data.Prefix
}

// Publish mints this peer's signed published root over a prefix and
// applies whichever projections the request asked for.
//
// Order is load-bearing and is the same order the two-peer flow taught:
// **check, then mint, then authorize.**
//
// The check first is not tidiness. A publish is two durable acts — a
// signature and a disclosure — and a run that mints and then refuses the
// grant leaves a peer publishing something it never meant to publish and
// authorizing nobody, with an error message about the grant. Deriving
// the grant is pure and free, so it is done before anything is written
// and the whole request either happens or does not.
//
// Mint before authorize, because the grant is derived from the prefix
// the root actually committed to: there is no window in which `default`
// authorizes a prefix no root commits to.
func (ws *ShellWorkspace) Publish(ctx context.Context, req PublishRequest) (PublishOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return PublishOutcome{}, fmt.Errorf("publish: no local peer")
	}
	if req.Public && req.Unpublic {
		return PublishOutcome{}, fmt.Errorf(
			"publish: -public and -private were both given; they are opposite instructions " +
				"about who may read this site and there is no sensible order to apply them in")
	}
	ap := ws.Local.Peer
	prefix := strings.TrimSpace(req.Prefix)
	if prefix == "" {
		prefix = ws.currentPublishPrefix(ctx)
	}
	// Both grant refusals, before anything durable happens. See the doc
	// comment: a run that mints and then refuses leaves a peer publishing
	// something it did not mean to publish, under an error message about
	// authorization.
	if req.Public {
		if _, err := workbench.PublicSiteGrants(prefix); err != nil {
			return PublishOutcome{}, fmt.Errorf("publish: %w", err)
		}
	}
	if req.Public || req.Unpublic {
		if err := ws.checkPublicRowIsOurs(); err != nil {
			return PublishOutcome{}, err
		}
	}

	var (
		signed publish.SignedRoot
		static publish.Result
		err    error
	)
	// ONE MINT PER PUBLISH, whichever projections were asked for.
	// `publish.Publish` is the whole static corridor and mints its own
	// root deliberately — there is no way to hand it one somebody else
	// made, so the directory's manifest is always the root the
	// directory's content was collected against. Calling both would sign
	// two roots and immediately supersede the first, which shows up as
	// `seq` advancing by two for one operator action.
	if req.OutputDir != "" {
		static, err = publish.Publish(ctx, publish.Opts{
			Peer:           ap,
			Prefix:         prefix,
			OutputDir:      req.OutputDir,
			OriginURL:      req.OriginURL,
			At:             req.At,
			AllowWholePeer: req.AllowWholePeer,
		})
		signed = static.SignedRoot
	} else {
		signed, err = publish.MintRoot(ctx, publish.MintOpts{
			Peer: ap, Prefix: prefix, At: req.At, AllowWholePeer: req.AllowWholePeer,
		})
	}
	if err != nil {
		return PublishOutcome{}, err
	}

	out := PublishOutcome{
		PeerID:   ap.PeerID(),
		Prefix:   signed.Data.Prefix,
		Seq:      signed.Data.Seq,
		RootHash: signed.Data.RootHash.String(),
		Bindings: signed.Bindings,
		Minted:   true,
	}
	if narrowedFrom(signed.PriorPrefix, signed.Data.Prefix) {
		out.NarrowedFrom = signed.PriorPrefix
	}

	if req.OutputDir != "" {
		res := static
		out.StaticDir = res.OutputDir
		out.StaticOrigin = res.OriginURL
		out.StaticBytes = res.Bytes
		out.StaticPaths = res.Paths
		if res.OriginURL == "" {
			out.Problems = append(out.Problems,
				"the static directory was emitted with no origin URL, so its transport profile "+
					"carries empty URL prefixes and no consumer can build a request from it — "+
					"re-run with -origin once you know where it will be served")
		}
	}

	switch {
	case req.Public:
		if err := ws.grantPublicRead(prefix); err != nil {
			return out, err
		}
		out.Problems = append(out.Problems, ws.propagatePublicGrant("publish -public")...)
	case req.Unpublic:
		if err := ws.revokePublicRead(); err != nil {
			return out, err
		}
		out.Problems = append(out.Problems, ws.propagatePublicGrant("publish -private")...)
	}

	out.Public = ws.publicGrantState(out.Prefix)
	out.Reach = ws.publishReach()
	out.Problems = append(out.Problems, publishProblems(out)...)
	return out, nil
}

// PublishStatus reads what is published without publishing anything.
//
// The separation is `status`'s, for `status`'s reason: a read and an
// act are different operations and the reading must say which one made
// it. Here the stakes are higher than a stale caption — a status panel
// that refreshed by minting would bump `seq` on a timer, which is a
// publisher claiming a new release every time somebody looks at it.
// `Minted` is false on everything this returns.
func (ws *ShellWorkspace) PublishStatus(ctx context.Context) (PublishOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return PublishOutcome{}, fmt.Errorf("publish: no local peer")
	}
	ap := ws.Local.Peer
	out := PublishOutcome{PeerID: ap.PeerID()}

	pr, err := ap.ReadPublishedRoot(ctx, ap.PeerID())
	if err == nil {
		out.Prefix = pr.Data.Prefix
		out.Seq = pr.Data.Seq
		out.RootHash = pr.Data.RootHash.String()
		// Re-derived rather than remembered: the binding count is a fact
		// about the tree NOW, and the interesting case is precisely when
		// it has moved since the publish — that is an operator who has
		// added pages and not re-published.
		out.Bindings = len(entitysdk.ListEntriesSorted(
			ap.RawLocationIndex(), strings.TrimPrefix(pr.Data.Prefix, "/")))
	} else {
		// The headline already says nothing is published; what this adds
		// is WHY, which is the part that separates "never published" from
		// "published and the root will not load".
		out.Problems = append(out.Problems, "no published root could be read: "+err.Error())
	}

	out.Public = ws.publicGrantState(out.Prefix)
	out.Reach = ws.publishReach()
	out.Problems = append(out.Problems, publishProblems(out)...)
	return out, nil
}

// checkPublicRowIsOurs refuses to touch a `default` policy row this code
// did not write.
//
// It is somebody's deliberate act, it may grant something entirely
// different, and `SaveAccessPolicy` REPLACES rather than merges — so on
// this one path the accommodating behaviour is the destructive one. The
// recogniser compares the grant's SHAPE rather than its note, because a
// hand-edited note does not make a row ours and a hand-edited row with
// our note is not either.
func (ws *ShellWorkspace) checkPublicRowIsOurs() error {
	existing, ok := workbench.LoadAccessPolicy(ws.Local.Peer.Store(), workbench.PublicPolicyPeer)
	if !ok || workbench.IsPublicSiteGrant(existing.Grants) {
		return nil
	}
	return fmt.Errorf(
		"publish: this peer already has a `default` policy row that was not written here — it "+
			"grants %q to every peer that can dial this one. Writing or removing the public site "+
			"grant would REPLACE it rather than add to it, so this verb will not touch it; "+
			"`access` shows it, and removing it is your call", existing.Summary)
}

// grantPublicRead writes the `default` row for one published prefix.
func (ws *ShellWorkspace) grantPublicRead(prefix string) error {
	grants, err := workbench.PublicSiteGrants(prefix)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	if err := ws.checkPublicRowIsOurs(); err != nil {
		return err
	}
	if err := workbench.SaveAccessPolicy(ws.Local.Peer.Store(), workbench.PublicPolicyPeer, grants,
		workbench.PublicSiteNote(prefix)); err != nil {
		return fmt.Errorf("publish: write the public grant: %w", err)
	}
	return nil
}

// propagatePublicGrant re-derives every declared peer's row and
// re-establishes the connections whose authority changed.
//
// # Why a public grant has to touch per-peer rows at all
//
// Because the kernel's policy resolution is **first match wins**:
// `readHandshakePolicyGrants` returns at the first of
// `hex(identityHash)` / Base58 peer-id / `default` that exists. A peer
// with a row of its own never reaches `default`, so without this the
// `-public` flag would publish a site readable by every peer on earth
// **except the ones this operator has shared a folder with** — i.e.
// except the only peer they have to test it with. See
// `desiredGrantsByPeer` for the derivation.
//
// # And why it reconnects
//
// Grants are assembled at HANDSHAKE (AP63). A row rewritten on a live
// connection is inert until that connection is replaced, so a verb that
// changes authorization and stops is a verb that appears to do nothing.
// `direction` shipped in exactly that state for four days with a doc
// comment claiming otherwise.
//
// Returns problems rather than an error: failing to re-derive one peer's
// row does not un-publish the site, and the operator's next `status`
// pass fixes it. Refusing the whole publish over it would be a worse
// trade.
func (ws *ShellWorkspace) propagatePublicGrant(note string) []string {
	var problems []string
	devices, _ := workbench.LoadDevices(ws.Local.Peer.Store())
	for _, d := range devices {
		if d.PeerID == "" || d.PeerID == ws.Local.Peer.PeerID() {
			continue
		}
		changed, err := ws.ApplyDeclaredPolicy(d.PeerID, note)
		if err != nil {
			problems = append(problems,
				fmt.Sprintf("could not re-derive authorization for %s (%v) — that peer keeps its "+
					"previous grants until the next status pass", d.PeerID, err))
			continue
		}
		if !changed {
			continue
		}
		if ok, why := ws.refreshGrantConnection(d.PeerID); !ok && why != "" {
			problems = append(problems, fmt.Sprintf(
				"%s: authorization changed and the connection was not re-established (%s) — "+
					"grants are read at handshake, so it is in force only from their next connect",
				d.PeerID, why))
		}
	}
	return problems
}

// revokePublicRead removes the `default` row, and only ours.
func (ws *ShellWorkspace) revokePublicRead() error {
	st := ws.Local.Peer.Store()
	existing, ok := workbench.LoadAccessPolicy(st, workbench.PublicPolicyPeer)
	if !ok {
		return nil
	}
	if !workbench.IsPublicSiteGrant(existing.Grants) {
		return ws.checkPublicRowIsOurs()
	}
	workbench.RemoveAccessPolicy(st, workbench.PublicPolicyPeer)
	return nil
}

// publicGrantState reads the `default` row and compares it with the
// prefix the published root commits to.
func (ws *ShellWorkspace) publicGrantState(publishedPrefix string) PublicGrantState {
	st := ws.Local.Peer.Store()
	row, ok := workbench.LoadAccessPolicy(st, workbench.PublicPolicyPeer)
	if !ok {
		return PublicGrantState{}
	}
	s := PublicGrantState{
		Present: true,
		Ours:    workbench.IsPublicSiteGrant(row.Grants),
		Summary: row.Summary,
	}
	if s.Ours {
		s.Prefix = workbench.PublicSitePrefix(row.Grants)
		// Compared on the trimmed form because the grant pattern carries
		// no leading or trailing slash and §3.3a makes the root's prefix
		// carry a trailing one: `sites` and `sites/` are the same prefix
		// spelled two ways, and reporting them as a mismatch would raise
		// a stale-disclosure alarm on every correctly-published peer.
		s.Stale = publishedPrefix != "" &&
			strings.Trim(publishedPrefix, "/") != strings.Trim(s.Prefix, "/")
	}
	return s
}

// publishReach observes whether a stranger could arrive.
func (ws *ShellWorkspace) publishReach() PublishReach {
	ap := ws.Local.Peer
	var r PublishReach
	if addr := ap.Addr(); addr != nil {
		r.Listening = addr.String()
	}
	r.Advertised = advertisedDialURLs(ap)
	switch {
	case r.Listening == "":
		r.Note = "this peer is not listening, so nothing can dial it — a live read is impossible " +
			"until it is started with a listen address. The static projection is unaffected."
	case len(r.Advertised) == 0:
		r.Note = "this peer is listening but published no transport profile, so a reader that " +
			"knows only the peer-id cannot find an address for it."
	}
	return r
}

// advertisedDialURLs reads back this peer's own self-published transport
// profiles.
//
// Read from the TREE rather than from the listener, on purpose: the
// question is what a READER can learn, and a listener is not something a
// reader can see. A peer that bound a socket and failed to advertise is a
// peer with a door and no address on it, and those two failures send an
// operator to different places — which is why [PublishReach] carries both
// and neither one alone.
//
// The prefix is `system/peer/transport/{identity-hash-hex}/`, not the
// Base58 peer-id: `core/types/crypto.go` pins hex for non-root path
// positions and core-go's own `transportProfilePrefix` builds it that
// way. Getting this wrong yields an empty list, which reads as "this peer
// advertised nothing" — a confident, wrong, and entirely plausible
// answer.
func advertisedDialURLs(ap *entitysdk.AppPeer) []string {
	prefix := "system/peer/transport/" + types.PeerIdentityHashHex(ap.IdentityHash()) + "/"
	var ids []string
	byID := map[string]string{}
	for _, e := range ap.Store().List(prefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		id, under := workbench.RelativeUnder(e.Path, prefix)
		if !under || id == "" || strings.Contains(id, "/") {
			continue
		}
		ent, ok := ap.Store().Get(e.Path)
		if !ok {
			continue
		}
		url, ok := dialURLOf(ent)
		if !ok {
			continue
		}
		if _, seen := byID[id]; !seen {
			ids = append(ids, id)
		}
		byID[id] = url
	}
	sort.Strings(ids)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out
}

// dialURLOf pulls the dial address out of one transport profile, for the
// families `AppPeer.AdvertiseTransport` can publish.
//
// http-poll is absent on purpose: it is a STATIC origin profile — a
// directory on a web server — and nothing dials it. Including it here
// would report an origin URL as evidence that this peer is reachable,
// which is the one confusion this whole struct exists to prevent.
func dialURLOf(ent entity.Entity) (string, bool) {
	switch ent.Type {
	case types.TypePeerTransportTCP:
		d, err := types.TCPProfileDataFromEntity(ent)
		if err != nil || d.Endpoint.URL == "" {
			return "", false
		}
		return d.Endpoint.URL, true
	case types.TypePeerTransportWebSocket:
		d, err := types.WebSocketProfileDataFromEntity(ent)
		if err != nil || d.Endpoint.URL == "" {
			return "", false
		}
		return d.Endpoint.URL, true
	case types.TypePeerTransportHTTP:
		d, err := types.HTTPProfileDataFromEntity(ent)
		if err != nil || d.Endpoint.URL == "" {
			return "", false
		}
		return d.Endpoint.URL, true
	}
	return "", false
}

// narrowedFrom reports whether moving from prior to next stops
// committing to keys the old root committed to.
//
// A pure prefix comparison and not a set comparison: `sites/` → `sites/a/`
// narrows, `sites/a/` → `sites/` widens (and is fine), and two unrelated
// prefixes narrow in the sense that matters — everything under the old
// one went dark. The first publish has no prior and narrows nothing.
//
// **The leading slash has to come off first**, and this is not cosmetic.
// §3.3a spells the universal tree `"/"` and every other prefix without a
// leading slash (`publishedPrefix`), so `strings.HasPrefix("sites/", "/")`
// is false and publishing the WHOLE TREE over one site would be reported
// as taking that site dark — a warning that fires on the one move that
// cannot possibly lose a key. Stripped, the universal form is `""`, which
// is a prefix of everything, which is exactly what it means.
func narrowedFrom(prior, next string) bool {
	prior, next = strings.TrimPrefix(prior, "/"), strings.TrimPrefix(next, "/")
	if prior == "" || prior == next {
		return false
	}
	// next is a widening of prior: everything prior committed to is still
	// committed to.
	return !strings.HasPrefix(prior, next)
}

// publishProblems turns an outcome's state into the sentences an
// operator can act on, in one place so the act and the read cannot
// describe the same peer differently.
//
// Same discipline as `FolderStatus.problems()` — one writer, shared by
// `Publish` and `PublishStatus`, because the two surfaces are read side
// by side and a difference between them reads as a change.
func publishProblems(out PublishOutcome) []string {
	var probs []string
	if out.NarrowedFrom != "" {
		probs = append(probs, fmt.Sprintf(
			"this peer's previous published root committed to %q and this one commits to %q — "+
				"a peer has exactly one published root, so anything under %q and outside %q is no "+
				"longer committed to by this peer, and a reader asking for it gets a correctly-signed "+
				"\"absent\"",
			out.NarrowedFrom, out.Prefix, out.NarrowedFrom, out.Prefix))
	}
	if out.Public.Present && !out.Public.Ours {
		probs = append(probs, fmt.Sprintf(
			"this peer has a `default` policy row that is not the public site grant — it grants %q "+
				"to every peer that can dial this one. It was not written here and is not touched here",
			out.Public.Summary))
	}
	if out.Public.Stale {
		probs = append(probs, fmt.Sprintf(
			"the public grant authorizes %q and the published root commits to %q — the grant is "+
				"pointing at a prefix this peer no longer publishes, which is a disclosure without "+
				"a site behind it; re-run publish with -public to re-derive it",
			out.Public.Prefix, out.Prefix))
	}
	if out.Public.Present && out.Public.Ours && out.Reach.Note != "" {
		// Only worth saying when something is actually on offer. A peer
		// that has published nothing publicly is not failing to be
		// reachable; it is just not serving.
		probs = append(probs, out.Reach.Note)
	}
	if out.StaticDir != "" {
		probs = append(probs, fmt.Sprintf(
			"the static directory at %s is a set of files and nothing serves it — upload it to "+
				"the origin before a reader can fetch anything from there", out.StaticDir))
	}
	return probs
}
