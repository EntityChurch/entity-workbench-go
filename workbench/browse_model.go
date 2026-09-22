package workbench

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/entitysdk/publishedroot"
	"entity-workbench-go/fetch"
)

var _ Model[BrowseOutput] = (*BrowseModel)(nil)

// browse_model.go — the renderer-neutral **browser**: a location, a
// history, a page, and the provenance of the exact bytes on screen.
//
// # What this is, against what we already had
//
// [ConsumeModel] answers *"is this origin lying?"* for one origin an
// operator already knows the URL of. It is an inspector and it renders a
// chain instead of a page. That was the right first surface and it is
// half a journey: it cannot answer *what is out there*, *take me
// there*, or *where am I*.
//
// This model is the other half. The pipeline it drives is fixed by the
// substrate, not invented here (arch `STATUS-2026-08-19-b` §2):
//
//	name → registry resolve → peer-id + transports
//	     → the target's signed root
//	     → walk the trie → THE KEY SET IS THE METADATA
//	     → interpret the first segment by convention → sites/{id}/manifest
//
// Every one of those arrows is a check, and **the interesting design
// question is what a user is shown while they happen** — because at the
// end of it the screen shows a page, and a page looks the same whether
// eleven checks passed or none did.
//
// # The contrast with `entity-browser-rust`, stated so it is testable
//
// Theirs renders the pages and puts trust in the chrome — the reader's
// browser, and the right shape for a reader. Ours keeps the chain
// **beside** the page as a first-class, navigable object: eleven steps,
// each with its verdict and what a green verdict on that step actually
// proves, for the bytes currently displayed and no others.
//
// Two rules fall out of that and both are enforced below rather than
// left to a renderer's judgement:
//
//  1. **A step that could not be established is shown failing, never
//     omitted.** A chain that renders six green rows and then stops
//     looks green at a glance.
//  2. **No green verdict is ever rendered as the bare word "verified".**
//     Every one of them is *verified as of* a moment — the registry's
//     `published_at`, the binding's `issued_at`, the target's
//     `published_at` — and a quiet publisher is indistinguishable from a
//     withholding origin at every one of them (§6.5.3.1, D6/D7).
//
// Threading matches [ConsumeModel]: Open blocks on network I/O and holds
// no lock while doing it; Render is cheap and safe from a UI thread at
// any time, including mid-navigation.

// Address is where the browser is pointed.
//
// It is a *location*, not a URL: the wire URLs are the publisher's to
// declare (that is the whole AP21 lesson) and are derived at navigation
// time from the transport profile the binding names. What a user types
// and what history stores is this.
type Address struct {
	// Name is the registry name, when the address was reached by name.
	// Empty means the user supplied a peer-id directly — which is a
	// materially weaker starting point and the chain says so.
	Name string
	// PeerID is the target publisher. Populated from the resolve when
	// the address was a name.
	PeerID string
	SiteID string
	Page   string
}

// Host is the addressing authority: the name if there is one, else the
// peer-id. It is what an address bar shows.
func (a Address) Host() string {
	if a.Name != "" {
		return a.Name
	}
	return a.PeerID
}

// String renders the canonical `entity://` form.
func (a Address) String() string {
	var b strings.Builder
	b.WriteString("entity://")
	b.WriteString(a.Host())
	if a.SiteID != "" {
		b.WriteString("/")
		b.WriteString(a.SiteID)
	}
	if a.Page != "" {
		b.WriteString("/")
		b.WriteString(a.Page)
	}
	return b.String()
}

// ParseAddress reads what a user typed.
//
// Accepted: `entity://host/site/page`, `host/site/page`, `host`. The
// host is a **peer-id when it parses as one** and a registry name
// otherwise — the same literal-or-parse-as-peer-id decision NETWORK
// §6.5.6 makes at the URL demux, made once, here, so no surface below
// has to guess.
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "entity://")
	s = strings.Trim(s, "/")
	if s == "" {
		return Address{}, errors.New("an address needs at least a name or a peer-id")
	}
	host, rest, _ := strings.Cut(s, "/")
	site, page, _ := strings.Cut(rest, "/")

	a := Address{SiteID: site, Page: page}
	if _, _, err := publishedroot.DeriveKey(host); err == nil {
		a.PeerID = host
	} else {
		if _, err := fetch.NormalizeName(host); err != nil {
			return Address{}, err
		}
		a.Name = host
	}
	return a, nil
}

// RegistryRow is one name in the pinned registry's browsable list.
type RegistryRow struct {
	Name string
	// Target is the peer-id the binding names, once resolved. Empty
	// until a row is resolved — enumeration yields names and binding
	// hashes, and resolving each one is a separate verification.
	Target string
	// BindingHash is what the signed root committed for this name.
	BindingHash string
	// Committed is true when the name is in the signed key set. False
	// means it appeared **only in the served listing** — the origin is
	// advertising a name the registry has not signed into this root, and
	// a row like that must never be presented as available.
	Committed bool
	// Listed is true when the served menu also carries it. A committed
	// name that is not listed is a name hidden from the menu; benign
	// alone, and the shape a targeted withholding takes.
	Listed bool
	// ExpiresAt is the binding's `issued_at + ttl`, once resolved.
	ExpiresAt string
	// Err is why this row would not resolve, if it was tried.
	Err string
}

// BrowseOutput is what a renderer draws.
type BrowseOutput struct {
	// Address is the canonical location of what is displayed. Empty
	// before the first navigation.
	Address string
	// Host / Site / Page are the parts, for a renderer that wants to
	// draw them separately (an address bar with a name segment styled
	// differently from the path, say).
	Host string
	Site string
	Page string

	// Registry is the pinned registry's peer-id, or empty. **The pin is
	// the only thing a user supplies out-of-band** and the only thing
	// trusted a priori, so it is on every render, not tucked in a
	// settings pane.
	Registry string
	// RegistryOrigin is where its bytes are being served from — a
	// different fact from who signs them, and the §6a.1a fourth actor
	// lives in the gap between the two.
	RegistryOrigin string
	// RegistryDiscovered is false when the registry's layout was pinned
	// by hand rather than read from a `transport-profile`.
	RegistryDiscovered bool
	// RegistryRebasedFrom is the peer the ORIGIN features, set only when
	// the registry we pinned is a different peer co-hosted on it. A
	// third provenance state, and a surface that collapses it into
	// "discovered" tells the operator the origin advertised a layout it
	// did not.
	RegistryRebasedFrom string
	// RegistryPinFromOrigin is true when the operator supplied no pin and
	// we took one from the origin's `entity-deployment.json`.
	//
	// **This is trust-on-first-use and a surface MUST say so.** An
	// operator's pin is the one fact the origin did not choose; a pin
	// the origin nominated means the origin picked its own trust root.
	// Everything still verifies under that key — a hostile origin cannot
	// forge a binding for a key it does not hold — but it can hand you
	// one it does hold and be perfectly consistent underneath it.
	RegistryPinFromOrigin bool
	// RegistryFresh is the registry root's `published_at`, rendered.
	RegistryFresh string

	// Names is the registry browser's rows.
	Names []RegistryRow
	// NamesAuthority says where Names came from, in words a user reads:
	// the walk is authoritative, the listing is a menu.
	NamesAuthority string
	// NamesNotARegistry is true when the pinned peer publishes something
	// other than registry bindings — the co-hosting case. A renderer
	// should show NamesNote prominently rather than an empty list, and
	// should offer re-pinning rather than implying the registry is empty.
	NamesNotARegistry bool
	// NamesNote carries a reconciliation disagreement, when there is
	// one. Empty when the menu and the signed key set agree.
	NamesNote string

	// Sites is every site the current target's signed root commits to —
	// the authoritative "what is published here".
	Sites []string
	// SiteDefaulted is true when the address named no site and one was
	// chosen for the user.
	//
	// It is surfaced because there is **no landing-site field anywhere in
	// the tree** to consult — a signature covers no such thing. Since
	// 2026-08-30 we do consult the origin's `entity-deployment.json`
	// `home_site` first, which is the cohort's deployment-configuration
	// answer, and this flag is now true only when that was absent or
	// named a site the signed root does not commit, leaving
	// first-in-byte-order. An arbitrary choice presented as "the site"
	// is a small lie that compounds — a user reads a page believing it
	// is the publisher's front door.
	SiteDefaulted bool

	// Notice is a message about the LAST ACTION that did not change the
	// page — currently, a link that leaves the entity system (see
	// [BrowseModel.Follow]).
	//
	// Deliberately not Err. A renderer clears the page on Err, because a
	// refused navigation must never leave the previous page sitting under
	// a failed chain. Clicking an external link refuses nothing and
	// invalidates nothing, so routing it through Err would blank a page
	// the user is still reading. Cleared at the start of every Follow.
	Notice string

	// Site content, via [SiteModel] over a [RemoteSiteResolver].
	Content SiteRenderOutput

	// Body is Content's body PREPARED FOR DISPLAY: HTML lowered to text,
	// `::embed` directives lowered into markdown images, and a size cap
	// with a note saying what was dropped.
	//
	// It is separate from Content.BodyMarkdown, which stays the verified
	// bytes, because those two answer different questions and collapsing
	// them is how an 8.27 MB pre-rendered paper ended up going through
	// Markdig on the UI thread. See body_display.go.
	Body BodyView

	// Steps is the trust chain for THESE bytes. Never a summary, never
	// collapsed, and a step that could not run is Failed or Skipped with
	// a reason — never absent.
	Steps []ConsumeStep

	// Freshness is the honest one-line scope of the whole chain. It
	// names a moment and never says "verified" alone.
	Freshness string

	CanBack    bool
	CanForward bool
	Running    bool
	// Err is a navigation-level failure. The step list says which link
	// broke; this is the sentence.
	Err string

	// hostName / hostPeer are the parts Host was built from, kept so
	// history can be re-pushed without re-parsing the rendered form.
	hostName string
	hostPeer string
}

// BrowseModel is the browser.
type BrowseModel struct {
	client *http.Client

	mu sync.Mutex

	reg       *fetch.Registry
	regRoot   *fetch.VerifiedRoot
	regPinned bool
	nameSet   *fetch.NameSet

	// targetOrigin overrides where a resolved binding's origin-relative
	// URLs are rooted. Empty means "the registry's own origin", which is
	// correct for every single-host deployment and stated at the seam
	// rather than assumed here.
	targetOrigin string

	// cache is process-lifetime and shared by every consumer this model
	// builds, registry included. Keyed by content hash, so it is a
	// memoization of proofs already done and not a freshness claim — the
	// argument is in fetch/cache.go, along with the measurement that
	// made it necessary (61 requests to open a page, 60 to click a link
	// in it, 51 of them the same trie every time).
	cache *fetch.Cache
	// consumers holds one verifying reader per publisher, for the life
	// of the browser. Rebuilding one per navigation is what reset the
	// seq floor to nothing (fetch.ErrSeqRollback) and threw away every
	// verified byte between two clicks.
	//
	// A live road keys as `entity://{peer-id}` and a static one by
	// layout, so one publisher reachable both ways has two entries here
	// — deliberately, because they are two readers. What they must NOT
	// have is two floors; see `floors` below.
	consumers map[string]*fetch.Consumer
	// floors is the §3-RES.4 monotonicity memory, one per PUBLISHER and
	// shared by every consumer reading that publisher. See
	// browse_road.go: keyed by peer-id because the road that carried a
	// root is not part of the statement the floor makes.
	floors map[string]*fetch.SeqFloor

	// peer is what makes the live road available, and nil is the ordinary
	// case rather than a degraded one.
	//
	// `entity-fetch` and a browser opened before a peer exists are
	// genuinely peer-less Mode A2 consumers, and they read static origins
	// correctly and completely. What a peer adds is the ABILITY to ask a
	// publisher directly — one fewer party who could be withholding a
	// newer root — never a stronger check. See browse_road.go.
	peer *entitysdk.AppPeer

	history []Address
	hpos    int

	site *SiteModel
	// assets serves the current page's figures. Held separately from
	// `site` because it is read from a renderer thread, long after the
	// navigation that produced it returned.
	assets AssetResolver
	out    BrowseOutput

	running   bool
	listeners []func()

	// Now backs the binding-expiry check. Nil means time.Now.
	Now func() time.Time
}

// NewBrowseModel builds an unpinned browser. Nothing resolves until a
// registry is pinned or an address names a peer-id outright.
func NewBrowseModel(client *http.Client) *BrowseModel {
	if client == nil {
		client = fetch.NewHTTPClient(0)
	}
	return &BrowseModel{
		client:    client,
		hpos:      -1,
		cache:     fetch.NewCache(0),
		consumers: map[string]*fetch.Consumer{},
		floors:    map[string]*fetch.SeqFloor{},
	}
}

// SetPeer gives this browser the ability to take the live road.
//
// Optional, and a browser without one is not degraded — it is
// `entity-fetch`'s configuration, which reads static origins correctly
// and completely. What the peer adds is the option of asking a publisher
// directly when the publisher's binding says it is reachable; see
// browse_road.go for the choice and for what it is and is not worth.
//
// **Set it before the first navigation.** A consumer is cached per
// publisher for the life of the browser, so a peer arriving mid-session
// changes nothing about publishers already read — which is the honest
// behaviour (a road is chosen once and the chain on screen describes the
// one that answered) but would be surprising if it were discovered rather
// than stated.
func (m *BrowseModel) SetPeer(ap *entitysdk.AppPeer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peer = ap
}

// HasPeer reports whether the live road is available at all.
//
// Exists for surfaces and for the refusal text: *"no live transport was
// offered"* and *"a live transport was offered and this browser cannot
// dispatch"* send an operator to two different machines.
func (m *BrowseModel) HasPeer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peer != nil
}

// consumerFor returns the verifying reader for a layout, building it
// once and keeping it.
//
// Keyed by (peer-id, origin, manifest URL) rather than by peer-id alone:
// a re-based layout and a discovered one can name the same peer through
// different prefixes, and they are different readers of the same
// publisher.
//
// **The seq floor is NOT keyed that way and must not be.** This comment
// used to end *"two consumers for one peer would each hold their own seq
// floor, which is how a floor stops being one"*, and avoided the problem
// by keying defensively. That stopped being enough the moment a
// publisher could be read live AND statically, which is two consumers for
// one peer on purpose — so the floor moved out to `floorFor`, one per
// peer-id, and every consumer built here takes it.
func (m *BrowseModel) consumerFor(layout fetch.Layout) *fetch.Consumer {
	key := layout.PeerID + "|" + layout.Origin + "|" + layout.ManifestURL()
	floor := m.floorFor(layout.PeerID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.consumers[key]; ok {
		return c
	}
	c := fetch.NewConsumerWithCache(layout, m.client, m.cache)
	c.UseFloor(floor)
	m.consumers[key] = c
	return c
}

// CacheStats reports what the content cache is holding and how often it
// has answered. Exposed so a surface can say *why* a navigation was
// fast, rather than leaving the speed-up looking like a shortcut.
func (m *BrowseModel) CacheStats() fetch.CacheStats { return m.cache.Stats() }

// PinRegistry sets the trust root.
//
// `ep` nil means discover the layout from the origin's well-known
// `transport-profile`; non-nil pins it. Both are legitimate and the
// difference is reported on every render — NETWORK §6.5.4 makes profile
// distribution out-of-band in v1, so an origin serving no profile is
// conformant, and a wrong pin is byte-identical to a withholding origin
// from here.
func (m *BrowseModel) PinRegistry(ctx context.Context, origin, peerID string, ep *types.TransportEndpoint) error {
	var (
		layout        fetch.Layout
		err           error
		pinFromOrigin bool
	)
	if ep != nil {
		layout, err = fetch.PinnedLayout(origin, peerID, *ep)
	} else {
		// No pin supplied: ask the origin what registry it nominates.
		// This is TOFU and is labelled as such downstream — but refusing
		// to look would repeat AP44, protecting an invariant at the cost
		// of the feature when the honest move is to do it and say what
		// it rests on. An operator with a domain and nothing else is the
		// normal first experience, not an edge case.
		if peerID == "" {
			if d, derr := fetch.LoadDeployment(ctx, origin, m.client); derr == nil && d.RegistryPin != nil {
				peerID = d.RegistryPin.PeerID
				if d.RegistryPin.Origin != "" {
					origin = d.RegistryPin.Origin
				}
				pinFromOrigin = true
			}
		}
		layout, err = fetch.LoadLayout(ctx, origin, m.client)
		if err == nil && peerID != "" && layout.PeerID != peerID {
			// NOT a mismatch: one origin may host several peers, and the
			// well-known profile features exactly one of them. This
			// refusal made the live federation's registry unreachable —
			// entitychurchregistry.org features its SITE peer while the
			// registry peer sits beside it (measured 2026-08-30). Re-base
			// the origin's layout onto the pin; the pinned root's own
			// signature is what checks the substitution, one hop later.
			layout, err = layout.RebaseTo(peerID)
		}
	}
	if err != nil {
		return err
	}
	reg, err := fetch.NewRegistryWithCache(layout, m.client, m.cache)
	if err != nil {
		return err
	}
	if m.Now != nil {
		reg.Now = m.Now
	}

	m.mu.Lock()
	m.reg, m.regPinned, m.nameSet, m.regRoot = reg, ep != nil, nil, nil
	m.out.Registry = layout.PeerID
	m.out.RegistryOrigin = layout.Origin
	m.out.RegistryDiscovered = ep == nil
	m.out.RegistryRebasedFrom = layout.RebasedFrom
	m.out.RegistryPinFromOrigin = pinFromOrigin
	m.mu.Unlock()
	m.fire()
	return nil
}

// SetTargetOrigin overrides where resolved bindings' origin-relative
// URLs are rooted. See [BrowseModel.targetOrigin].
func (m *BrowseModel) SetTargetOrigin(origin string) {
	m.mu.Lock()
	m.targetOrigin = origin
	m.mu.Unlock()
}

// Registry reports the pinned registry, or "" .
func (m *BrowseModel) Registry() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reg == nil {
		return ""
	}
	return m.reg.PeerID()
}

// RefreshNames enumerates the pinned registry — §6a.3a, the walk.
//
// It is a separate operation from navigation on purpose: enumerating is
// O(the registry) and resolving one name is O(1), so a browser that
// enumerated on every navigation would make the cheap path pay for the
// expensive one. A user asks for the list; a user does not ask for it
// again on every click.
func (m *BrowseModel) RefreshNames(ctx context.Context) error {
	m.mu.Lock()
	reg := m.reg
	m.mu.Unlock()
	if reg == nil {
		return errors.New("no registry is pinned: there is nothing to enumerate, and a browser " +
			"cannot invent a name authority")
	}

	set, err := reg.Enumerate(ctx)
	m.mu.Lock()
	defer func() { m.mu.Unlock(); m.fire() }()
	if err != nil {
		m.out.Names = nil
		m.out.NamesAuthority = "enumeration failed"
		m.out.NamesNote = err.Error()
		return err
	}
	m.nameSet = &set
	m.regRoot = &set.Root
	m.out.RegistryFresh = freshnessOf(set.Root.Data.PublishedAt)
	m.out.Names = rowsFrom(set)
	if set.Diagnosis != "" {
		// An empty list is a confident wrong answer when the peer is not a
		// registry. Lead with WHY, not with the count.
		m.out.NamesAuthority = "no names — and here is why, because an empty list would be misleading"
		m.out.NamesNote = set.Diagnosis
		m.out.NamesNotARegistry = set.NotARegistry
		return nil
	}
	m.out.NamesAuthority = fmt.Sprintf("%d names, from the walk of signed root %s — "+
		"the origin cannot hide one of these without the walk failing",
		len(set.Names), shortHash(set.Root.Data.RootHash.String()))
	m.out.NamesNote = reconcileNote(set)
	m.out.NamesNotARegistry = false
	return nil
}

// rowsFrom merges the two provenances into one list without flattening
// the difference.
//
// A listed-only name is included — deliberately. Dropping it would hide
// the disagreement, and a browser whose list silently differs from the
// origin's own menu is harder to debug than one that shows the extra row
// marked as uncommitted.
func rowsFrom(set fetch.NameSet) []RegistryRow {
	listed := map[string]bool{}
	for _, l := range set.Listing {
		listed[l] = true
	}
	rows := make([]RegistryRow, 0, len(set.Names)+len(set.AdvertisedOnly))
	for _, e := range set.Names {
		rows = append(rows, RegistryRow{
			Name:        e.Name,
			BindingHash: shortHash(e.BindingHash.String()),
			Committed:   true,
			Listed:      listed[e.Name],
		})
	}
	for _, n := range set.AdvertisedOnly {
		rows = append(rows, RegistryRow{
			Name:      n,
			Committed: false,
			Listed:    true,
			Err: "advertised in the served listing, not committed by the signed root — " +
				"this name has no binding the registry has signed into this root",
		})
	}
	return rows
}

func reconcileNote(set fetch.NameSet) string {
	switch {
	case set.ListingErr != nil:
		return "the origin serves no by-name listing; the walk answered on its own (a listing is " +
			"a convenience, never the authority)"
	case len(set.AdvertisedOnly) > 0 && len(set.CommittedOnly) > 0:
		return fmt.Sprintf("the served menu invents %v and hides %v — it disagrees with the signature in both directions",
			set.AdvertisedOnly, set.CommittedOnly)
	case len(set.AdvertisedOnly) > 0:
		return fmt.Sprintf("the served menu advertises %v with no signed binding behind it", set.AdvertisedOnly)
	case len(set.CommittedOnly) > 0:
		return fmt.Sprintf("the signed root commits to %v, which the served menu omits — "+
			"an origin can hide a name from a menu undetectably, and cannot hide one from the walk", set.CommittedOnly)
	default:
		return ""
	}
}

// Open navigates to an address and pushes it onto the history.
func (m *BrowseModel) Open(ctx context.Context, address string) error {
	addr, err := ParseAddress(address)
	if err != nil {
		m.mu.Lock()
		m.out.Err = err.Error()
		m.mu.Unlock()
		m.fire()
		return err
	}
	if err := m.goTo(ctx, addr); err != nil {
		return err
	}
	m.mu.Lock()
	m.history = append(m.history[:m.hpos+1], m.resolvedAddr())
	m.hpos = len(m.history) - 1
	m.syncNavLocked()
	m.mu.Unlock()
	m.fire()
	return nil
}

// Follow navigates a link as written in the page currently on screen.
//
// This is the entry point a renderer calls when a user clicks a link in
// the rendered markdown, and it exists so that **no renderer has to know
// how a link resolves**. Classification is Layer-2 algorithm contract
// (see [resolveInSitePage]) — a C# or tview reimplementation would be a
// second, drifting copy of a rule that must be byte-identical across
// implementations, and the first symptom of drift is a dead link, which
// nobody files as a correctness bug.
//
// Three outcomes, and only the first moves the browser:
//
//   - in-site / cross-site / cross-peer → resolve against the current
//     location and navigate, pushing history exactly like [Open].
//   - external (`http(s)://`, `mailto:`) → **not followed**, and the page
//     stays up. This browser's whole contract is that what is on screen
//     came with a verification chain; opening an unverifiable URL in it
//     would put bytes on that surface with nothing behind them. The URL
//     is reported through Notice so a renderer can offer it for copying.
//   - malformed `entity://` → reported the same way rather than guessed
//     at, matching the reference implementation's fallback.
//
// A same-host link keeps the NAME we arrived by, not the resolved
// peer-id, so the chain is re-run through the registry on every hop and
// the address bar keeps saying what the user typed. That costs a name
// resolution per click and it is the same trade [Back] makes: a cached
// page is a claim about a moment that has passed.
func (m *BrowseModel) Follow(ctx context.Context, target string) error {
	m.mu.Lock()
	cur := Location{PeerID: m.out.hostPeer, SiteID: m.out.Site, Page: m.out.Page}
	name := m.out.hostName
	m.out.Notice = ""
	m.mu.Unlock()

	loc, kind, ok := ClassifyTarget(target, cur)
	if !ok || kind == LinkExternal {
		m.mu.Lock()
		m.out.Notice = fmt.Sprintf(
			"%q leaves the entity system. It is not opened here: every page this browser shows "+
				"arrives with the chain beside it, and an ordinary web URL has none.", target)
		m.mu.Unlock()
		m.fire()
		return nil
	}

	addr := Address{SiteID: loc.SiteID, Page: loc.Page}
	if kind == LinkCrossPeer {
		// A cross-peer link names its own peer and no name vouches for
		// it — goTo will say so in the chain (the skipAll branch).
		addr.PeerID = loc.PeerID
	} else {
		addr.Name = name
		addr.PeerID = cur.PeerID
	}

	if err := m.goTo(ctx, addr); err != nil {
		return err
	}
	m.mu.Lock()
	m.history = append(m.history[:m.hpos+1], m.resolvedAddr())
	m.hpos = len(m.history) - 1
	m.syncNavLocked()
	m.mu.Unlock()
	m.fire()
	return nil
}

// Back / Forward move through history without re-pushing.
//
// They re-run the whole chain rather than replaying a cached page. That
// is a cost and it is the honest one: a cached page is a claim about a
// moment that has passed, and this model's entire contract is that
// nothing on screen outlives the verification that produced it.
func (m *BrowseModel) Back(ctx context.Context) error { return m.step(ctx, -1) }

// Forward is Back's mirror.
func (m *BrowseModel) Forward(ctx context.Context) error { return m.step(ctx, +1) }

func (m *BrowseModel) step(ctx context.Context, delta int) error {
	m.mu.Lock()
	next := m.hpos + delta
	if next < 0 || next >= len(m.history) {
		m.mu.Unlock()
		return nil
	}
	addr := m.history[next]
	m.hpos = next
	m.syncNavLocked()
	m.mu.Unlock()

	err := m.goTo(ctx, addr)
	m.fire()
	return err
}

func (m *BrowseModel) syncNavLocked() {
	m.out.CanBack = m.hpos > 0
	m.out.CanForward = m.hpos >= 0 && m.hpos < len(m.history)-1
}

func (m *BrowseModel) resolvedAddr() Address {
	return Address{
		Name:   m.out.hostName,
		PeerID: m.out.hostPeer,
		SiteID: m.out.Site,
		Page:   m.out.Page,
	}
}

// goTo runs the chain. It holds no lock across network I/O.
func (m *BrowseModel) goTo(ctx context.Context, addr Address) error {
	m.mu.Lock()
	reg, set, targetOrigin := m.reg, m.nameSet, m.targetOrigin
	m.running = true
	m.out.Running = true
	m.out.Err = ""
	m.out.Steps = nil
	m.mu.Unlock()
	m.fire()

	nav := newChain()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.out.Running = false
		m.out.Steps = nav.steps
		m.out.Freshness = nav.freshness
		m.mu.Unlock()
	}()

	// ---------------------------------------------------------------
	// hop 1 — the name
	// ---------------------------------------------------------------
	var (
		res fetch.NameResolution
		err error
	)
	if addr.Name == "" {
		nav.skipAll(
			"You supplied a peer-id directly, so no name authority was consulted and none " +
				"vouched for this target. The peer-id IS the key (V7 §1.5), so the content " +
				"signature below still means everything it normally means — what is missing is " +
				"any statement that this peer is the one a human name refers to.")
	} else {
		if reg == nil {
			nav.fail("registry pin", "no registry pinned",
				"A name cannot resolve without a name authority, and a browser must not invent one.")
			return m.failed(errors.New("no registry pinned, so a name cannot be resolved"))
		}
		nav.ok("registry pin", fmt.Sprintf("%s at %s", reg.PeerID(), reg.Layout.Origin),
			"The registry's peer-id carries its public key (§6a.5), so this pin needs no key "+
				"distribution and the host serving the bytes is trusted for nothing.")

		if set == nil {
			// Resolve through the served pointer — §6a.4's floor.
			res, err = reg.Resolve(ctx, addr.Name)
			nav.record("name lookup", err, fmt.Sprintf("by-name pointer for %q (no enumeration loaded)", addr.Name),
				"The by-name pointer is served by the origin, so WHICH binding answers this name "+
					"is the host's choice. That is what the association step below exists to catch.")
		} else {
			res, err = reg.ResolveIn(ctx, *set, addr.Name)
			nav.record("name lookup", err, fmt.Sprintf("%q found in the signed key set of root %s",
				addr.Name, shortHash(set.Root.Data.RootHash.String())),
				"The binding hash came out of the walk, so the origin had no say in which binding "+
					"answers this name — a substitution is not merely detected here, it is impossible.")
		}
		if err != nil {
			return m.failedChain(nav, err, res)
		}

		nav.ok("binding signature", fmt.Sprintf("ed25519, signed by the registry (%s)", shortHash(res.BindingHash.String())),
			"The registry issued this binding. It does NOT prove what the binding was issued for — "+
				"that is the next step, and a verifier that stops here accepts a valid binding for "+
				"one name in answer to a query for another.")
		nav.ok("association", fmt.Sprintf("binding names %q, which is what was asked", res.Binding.Name),
			"§6a.4's MUST. The signed body carries `name`, so the pairing was always committed; "+
				"the defect this catches is a verifier discarding it.")
		// The detail carries the caveat rather than a bare tick: this
		// step is green in the sense that nothing revoked the binding,
		// and Bound() says exactly how little that is worth when the
		// walk did not cover the revocation prefix.
		nav.ok("revocation", res.Revocation.Bound(),
			"Presence proves revocation; absence proves nothing unless the walk covered it. "+
				"The bound on a revocation a hostile origin withholds is the binding's TTL.")
		nav.ok("binding freshness", fmt.Sprintf("issued %s, expires %s",
			res.IssuedAt.Format(time.RFC3339), res.ExpiresAt.Format(time.RFC3339)),
			"Checked against our own clock, from inside the signed body — the one check on this "+
				"hop a hostile byte-server cannot influence at all.")

		addr.PeerID = res.PeerID()
	}

	// ---------------------------------------------------------------
	// hop 2 — the target
	// ---------------------------------------------------------------
	var (
		layout   fetch.Layout
		consumer *fetch.Consumer
		root     fetch.VerifiedRoot
	)
	// liveDecline is what the live road said when it was tried and did not
	// answer. Kept so the fall-through to an origin still REPORTS it: a
	// decline that vanishes because a second road worked is the one fact
	// an operator needs in order to know their own peer is half-connected.
	liveDecline := ""
	if addr.Name == "" {
		// A peer-id address with a peer that can already reach that peer
		// needs no origin at all — we ask them. This branch used to refuse
		// outright whenever `targetOrigin` was empty, on the true premise
		// that a peer-id says nothing about where its bytes are served
		// (NETWORK §6.5.4). Once this browser holds a peer that has a
		// connection to the target, that refusal is AP44's shape: a
		// sentence asserting a fact about the world, made false by a
		// capability on this side of it. A LAN peer's site has no origin
		// and never will.
		if why, ok := m.canReachLive(addr.PeerID); ok {
			consumer, err = m.consumerForRoad(ctx, browseRoad{
				Class:     fetch.ClassLive,
				Candidate: fetch.TransportCandidate{Class: fetch.ClassLive, PeerID: addr.PeerID},
			})
			if err == nil {
				root, err = consumer.VerifiedRoot(ctx)
			}
			if err == nil {
				layout = fetch.Layout{PeerID: addr.PeerID}
				nav.ok("transport", "live peer at entity://"+addr.PeerID+" (no origin consulted)",
					"No name authority and no byte-server: this peer was asked directly over a "+
						"connection we already hold. That removes every party except the publisher "+
						"— and removes nobody's ability to be quiet, which is why the Freshness "+
						"line below still does not say \"current\".")
			}
			// A live road that fails still falls through to an origin when
			// one was supplied; see browse_road.go for why the ladder stops
			// at the first VERIFIED root and not one step later.
			if err != nil {
				liveDecline = "live read at entity://" + addr.PeerID + " declined (" + err.Error() + ")"
				if targetOrigin == "" {
					nav.fail("transport", liveDecline+" and no origin was supplied",
						"There was one road and it did not answer. An origin would be a second one.")
					return m.failedChain(nav, err, res)
				}
			}
		} else if targetOrigin == "" {
			return m.failedChain(nav, fmt.Errorf(
				"a peer-id address needs an origin to fetch from: nothing in a peer-id says where "+
					"its bytes are served, and NETWORK §6.5.4 makes profile distribution "+
					"out-of-band in v1 (%s)", why), res)
		}
	}
	switch {
	case consumer != nil && err == nil:
		// The live road above answered. Nothing further to choose.

	case addr.Name == "":
		// Drop the failed live consumer rather than carrying it: the rest
		// of this navigation reads from whatever answered, and a stale
		// handle here would make the walk and the page fetch describe a
		// road the chain says was declined.
		consumer = nil
		layout, err = fetch.LoadLayout(ctx, targetOrigin, m.client)
		detail := fmt.Sprintf("discovered profile at %s", targetOrigin)
		if liveDecline != "" {
			detail = liveDecline + "; fell through to " + detail
		}
		// The origin's well-known profile features ONE peer, and the
		// address named a peer. When they differ the origin is hosting
		// several peers — re-base onto the one that was asked for, or we
		// would walk the featured peer's root and answer a question
		// nobody asked, silently. (Measured 2026-08-30.)
		if err == nil && addr.PeerID != "" && layout.PeerID != addr.PeerID {
			featured := layout.PeerID
			layout, err = layout.RebaseTo(addr.PeerID)
			detail = fmt.Sprintf("%s features %s; layout RE-BASED onto the %s you addressed",
				targetOrigin, featured, addr.PeerID)
		}
		nav.record("transport", err, detail,
			"The layout came from the origin's own well-known profile, so no URL here was derived "+
				"by convention (AP21). Where a second peer is co-hosted, only the peer-id segment "+
				"moves — and the target root's signature is what checks that it moved correctly.")
		if err != nil {
			return m.failedChain(nav, err, res)
		}
		consumer = m.consumerFor(layout)
		root, err = consumer.VerifiedRoot(ctx)

	default:
		// The binding's `transports` is a RANKED LIST, not an array whose
		// first usable entry wins (§6.5.1a D1 via REGISTRY §4.1.1), and
		// which CLASS to prefer is this consumer's call rather than the
		// specification's — see browse_road.go for both halves.
		opts := reg.TransportsFor(ctx, res, targetOrigin)
		roads, declined := roadsFor(opts, m.HasPeer())
		if len(roads) == 0 {
			nav.fail("transport", "no usable transport: "+offeredSummary(nil, declined),
				"The registry told us how to reach this peer and we can drive none of it. That is "+
					"a statement about THIS browser as often as about the publisher, so the roads "+
					"are named rather than summarised as \"unreachable\".")
			return m.failedChain(nav, fmt.Errorf(
				"this binding offers no transport this browser can use: %s",
				offeredSummary(nil, declined)), res)
		}

		var (
			used     browseRoad
			attempts []string
		)
		consumer, root, used, attempts, err = m.travel(ctx, roads)
		detail := fmt.Sprintf("%d road(s) offered, in §6.5.1a D1 order (priority asc); %s",
			len(roads), strings.Join(attempts, "; "))
		if err == nil {
			layout = used.Candidate.Layout
			if used.Class == fetch.ClassLive {
				// A live road has no Layout — there is no origin and no URL
				// prefix. The publisher's peer-id is the whole address, and
				// everything downstream that reads `layout.PeerID` (the site
				// Location, the rendered host) needs it filled in.
				layout = fetch.Layout{PeerID: used.Candidate.PeerID}
			}
		}
		if len(declined) > 0 {
			detail += ". Not tried: " + strings.Join(declined, "; ")
		}
		nav.record("transport", err, detail,
			"The registry told us how to reach this peer and we followed it verbatim, in the "+
				"order §6.5.1a D1 gives — never the array's. Nothing here was derived from the "+
				"peer-id. Which ROAD answered changes what may be claimed about freshness and "+
				"nothing about what was verified; the two are separate lines below on purpose.")
		if err != nil {
			return m.failedChain(nav, err, res)
		}
	}

	rootDetail := fmt.Sprintf("%s seq=%d prefix=%q",
		shortHash(root.Data.RootHash.String()), root.Data.Seq, root.Data.Prefix)
	if root.Authority != "" {
		rootDetail += " from " + root.Authority
	}
	nav.record("target root", err, rootDetail,
		"A DIFFERENT key from the registry's: this peer signs its own root, and the registry's "+
			"signature has no standing over its content. What this verification is worth in TIME "+
			"depends on which kind of party answered, which is the Freshness line and not this "+
			"one — the two sentences are not interchangeable and neither may be typed here.")
	if err != nil {
		return m.failedChain(nav, err, res)
	}
	// The scope of this chain comes from the ROOT, which got it from the
	// source that answered — never from this function's memory of which
	// consumer it built. See fetch/freshness.go: there is one composition
	// and it switches on the mode, so a live read cannot be captioned
	// with the static claim or the other way round.
	nav.freshness = root.Freshness()

	walk, err := consumer.Walk(ctx, root.Data.RootHash)
	nav.record("target walk", err, fmt.Sprintf("%d CHAMP nodes, %d committed keys",
		walk.Nodes(), len(walk.Bindings)),
		"The only step in this chain a withholding origin cannot pass. Every other one is "+
			"satisfiable by an origin serving a correctly-signed root that commits to nothing.")
	if err != nil {
		return m.failedChain(nav, err, res)
	}

	resolver := NewRemoteSiteResolver(ctx, consumer, root, walk)
	sites := resolver.Sites()
	defaulted := false
	if addr.SiteID == "" && len(sites) > 0 {
		// Ask the origin which site is its front door before falling back
		// to byte order. `entity-deployment.json`'s `home_site` is the
		// cohort's answer and we were ignoring it: billslab.com declares
		// `billslab-main` and we opened `billslab-entity-system`, purely
		// because it sorts first. That is an arbitrary choice presented
		// as the publisher's front page, which is the small lie
		// SiteDefaulted was added to confess. Reading the declaration
		// removes the need to confess anything.
		//
		// Unsigned and origin-supplied, so it is a HINT: it may only
		// select among sites the signed root already commits to. An
		// origin naming a site that is not in the walk is ignored, not
		// followed — otherwise it could point the reader at a page the
		// publisher never signed.
		home := ""
		if d, derr := fetch.LoadDeployment(ctx, layout.Origin, m.client); derr == nil {
			for _, s := range sites {
				if s == d.HomeSite.Site {
					home = s
					break
				}
			}
		}
		if home != "" {
			addr.SiteID = home
		} else {
			addr.SiteID, defaulted = sites[0], true
		}
	}

	site := NewSiteModel(resolver, Location{PeerID: layout.PeerID, SiteID: addr.SiteID, Page: addr.Page})
	content := site.Render()
	if content.Error != "" {
		nav.fail("page", content.Error,
			"The bytes were committed and verified; this is the SITE convention failing to find "+
				"what it expects at the paths it expects.")
	} else {
		nav.ok("page", fmt.Sprintf("%s/%s (%d bytes of %s)",
			addr.SiteID, content.CurrentPage, len(content.BodyMarkdown), content.BodyFormat),
			"These exact bytes hash to what the signed root committed for this path. That is the "+
				"whole of what a green chain claims.")
	}

	m.mu.Lock()
	m.site = site
	m.out.hostName, m.out.hostPeer = addr.Name, layout.PeerID
	m.out.Host = addr.Host()
	if m.out.Host == "" {
		m.out.Host = layout.PeerID
	}
	m.out.Site = addr.SiteID
	m.out.Page = content.CurrentPage
	m.out.Address = Address{Name: addr.Name, PeerID: layout.PeerID,
		SiteID: addr.SiteID, Page: content.CurrentPage}.String()
	m.out.Sites = sites
	m.out.SiteDefaulted = defaulted && len(sites) > 1
	m.out.Content = content
	m.out.Body = NewBodyView(content.BodyFormat, content.BodyMarkdown)
	m.assets = resolver
	m.mu.Unlock()
	return nil
}

// Asset resolves one embed reference against the site currently on
// screen, returning the verified bytes.
//
// **Bound to the CURRENT page's location, never to a caller-supplied
// one.** A renderer asks for a reference it read out of the body it is
// drawing; letting it name the site as well would let a stale or
// mistaken caller pull bytes from a site the chain on screen does not
// cover, and the whole contract of this panel is that the chain
// describes the bytes displayed and no others.
//
// Returns ok=false for a ref [ClassifyAssetRef] rejects, for an asset
// the signed root does not commit, and for a fetch that fails — three
// different facts that are one answer here, because a renderer's move is
// the same in all three: draw the fallback text, not a broken image.
//
// Collapsing them is defensible only while the first of the three is
// *rare and correct*. It was neither until 2026-09-11: three of §3.4's
// four reference forms landed in it, so a publisher writing
// `/assets/figures/x.png` — the form §3.4 SHOULDs for generated links —
// got a fallback caption and no way to find out why.
func (m *BrowseModel) Asset(ref string) (SiteAsset, bool) {
	m.mu.Lock()
	res, loc := m.assets, Location{PeerID: m.out.hostPeer, SiteID: m.out.Site}
	m.mu.Unlock()
	if res == nil {
		return SiteAsset{}, false
	}
	return res.ResolveAsset(loc, ref)
}

func (m *BrowseModel) failed(err error) error {
	m.mu.Lock()
	m.out.Err = err.Error()
	m.mu.Unlock()
	return err
}

// failedChain records a navigation failure with the resolution's own
// diagnostic attached, which is §6a.4's "surface which require failed to
// your own operator".
func (m *BrowseModel) failedChain(nav *chain, err error, res fetch.NameResolution) error {
	msg := err.Error()
	if errors.Is(err, fetch.ErrNameAssociation) && res.Binding.Name != "" {
		msg = fmt.Sprintf("%s — the origin served a binding the registry legitimately signed for "+
			"%q, in answer to a different question", msg, res.Binding.Name)
	}
	m.mu.Lock()
	m.out.Err = msg
	m.out.Content = SiteRenderOutput{}
	m.out.Sites = nil
	m.mu.Unlock()
	return err
}

// OnChange registers a render-invalidation listener; the returned func
// unsubscribes.
func (m *BrowseModel) OnChange(h func()) func() {
	m.mu.Lock()
	m.listeners = append(m.listeners, h)
	i := len(m.listeners) - 1
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		if i < len(m.listeners) {
			m.listeners[i] = nil
		}
		m.mu.Unlock()
	}
}

func (m *BrowseModel) fire() {
	m.mu.Lock()
	ls := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	for _, h := range ls {
		if h != nil {
			h()
		}
	}
}

// Render returns the current output. Cheap; safe from a UI thread.
func (m *BrowseModel) Render() BrowseOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.out
	out.Steps = append([]ConsumeStep{}, m.out.Steps...)
	out.Names = append([]RegistryRow{}, m.out.Names...)
	out.Sites = append([]string{}, m.out.Sites...)
	return out
}

// -------------------------------------------------------------------
// the chain
// -------------------------------------------------------------------

// chain accumulates the trust steps for one navigation.
//
// It exists so that "a step that could not be established is shown
// failing, never omitted" is a property of the type rather than a
// discipline every call site has to remember: [chain.record] takes the
// error and decides, and there is no way to add a step without one.
type chain struct {
	steps     []ConsumeStep
	freshness string
}

func newChain() *chain { return &chain{} }

func (c *chain) ok(name, detail, proves string) {
	c.steps = append(c.steps, ConsumeStep{Name: name, Status: StepOK, Detail: detail, Proves: proves})
}

func (c *chain) fail(name, reason, proves string) {
	c.steps = append(c.steps, ConsumeStep{
		Name: name, Status: StepFailed, Detail: reason, Proves: proves, Err: reason,
	})
}

func (c *chain) record(name string, err error, detail, proves string) {
	st := ConsumeStep{Name: name, Status: StepOK, Detail: detail, Proves: proves}
	if err != nil {
		st.Status, st.Err = StepFailed, err.Error()
	}
	c.steps = append(c.steps, st)
}

// skipAll marks the whole naming hop as not-run, with the reason.
//
// Skipped is not a quiet pass. Every one of these rows is drawn, because
// "no name authority was consulted" is a fact about what the user is
// looking at and a chain that simply started at the target would read as
// a shorter, cleaner, equally-green chain.
func (c *chain) skipAll(why string) {
	for _, n := range []string{"registry pin", "name lookup", "binding signature", "association",
		"revocation", "binding freshness"} {
		c.steps = append(c.steps, ConsumeStep{Name: n, Status: StepSkipped, Detail: "not run", Proves: why})
	}
}

func freshnessOf(publishedAt uint64) string {
	if publishedAt == 0 {
		return "no published_at"
	}
	return time.UnixMilli(int64(publishedAt)).UTC().Format(time.RFC3339)
}

func shortHash(s string) string {
	if i := strings.Index(s, ":"); i >= 0 && len(s) > i+13 {
		return s[:i+13] + "…"
	}
	return s
}
