package publish_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// live_and_static_test.go — W2's gate. One site, two byte sources, one
// verification stack.
//
// # Why it lives in `publish`
//
// Because **publishing is one act with two projections** and this test
// is the assertion of exactly that. `publish.Publish` mints the signed
// root *into the publisher's own tree* and emits the static corridor in
// the same call; the live arm then reads that same signed root by
// dispatch. Putting the test anywhere else would need a second way to
// mint a root, and two ways to produce the artifact the whole trust
// argument hangs off is the thing that must not exist.
//
// # What a green run claims
//
//   - A peer can serve its own published site by dispatch, and the
//     bytes arrive through the identical verification stack: recomputed
//     content hashes, the two-hop signature against the key in the
//     peer-id, the `seq` floor, a fail-closed CHAMP walk from the signed
//     root.
//   - The two modes produce **byte-identical page content** and the same
//     committed key set, differing only in where each step looked.
//   - The read works under a **scoped** per-peer grant — four resources
//     across two operations — with no `OpenAccess` anywhere on either
//     peer.
//
// # What it does NOT claim
//
//   - **Nothing about the `default` (public) grant.** This authorizes
//     one named reader. A public site is a `default` row in the V7 §8
//     policy table and it is W4's, along with the negative arm that says
//     a stranger gets the site and nothing else.
//   - **Nothing about transport RANKING or about CHOOSING a mode (W3).**
//     Both consumers are constructed by the test: no binding is consulted
//     and nothing picks a road. The freshness sentence *is* asserted here
//     as of 2026-09-12 — this is the only place in the tree where both
//     sentences come out of real verifications of one published act
//     rather than out of struct literals — but producing the right
//     sentence for a mode you were handed is a different claim from
//     picking the mode, and only the first one is made here.
//   - Nothing about scale. Two pages.
//   - It is one process. Both peers are in it, over a real TCP
//     connection on loopback.

// siteID is fixed so the manifest body — which carries it, and therefore
// reaches the trie root — is the same in every arm.
const liveSiteID = "demo"

// fillerPages pushes the trie past one CHAMP node; see seedLiveSite.
const fillerPages = 60

// seedLiveSite authors a two-page site under ap and returns the tree
// prefix that contains it, in the peer-relative form `publish.Opts`
// takes.
func seedLiveSite(t *testing.T, ap *entitysdk.AppPeer) string {
	t.Helper()

	m := entitysdk.NewSiteManifest(liveSiteID, "Live and Static", "index", []entitysdk.NavItem{
		entitysdk.NewNavLeaf("Home", "/index"),
		entitysdk.NewNavLeaf("About", "/about"),
	})
	if _, err := ap.PutSiteManifest(liveSiteID, m); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	for _, p := range []struct{ slug, title, body string }{
		{"index", "Welcome", "# Welcome\n\nRead this from a CDN or from the machine that wrote it.\n"},
		{"about", "About", "# About\n\nThe same bytes either way.\n"},
	} {
		if _, err := ap.PutSitePage(liveSiteID, p.slug, entitysdk.NewMarkdownPage(p.title, p.body)); err != nil {
			t.Fatalf("put page %s: %v", p.slug, err)
		}
	}
	// ENOUGH PAGES THAT THE TRIE IS NOT ONE NODE. A CHAMP node holds 32
	// slots, so three keys make a single-node trie and "the walk agrees"
	// is then a statement about one fetch — no interior structure, no
	// second level, and none of [fetch.WalkConcurrency]'s eight-in-flight
	// dispatches. The first version of this gate was green at one node,
	// which is the shape of a test that measures the setup.
	for i := 0; i < fillerPages; i++ {
		slug := fmt.Sprintf("notes/n%02d", i)
		if _, err := ap.PutSitePage(liveSiteID, slug,
			entitysdk.NewMarkdownPage(slug, fmt.Sprintf("# %s\n\nfiller %d\n", slug, i))); err != nil {
			t.Fatalf("put page %s: %v", slug, err)
		}
	}
	return entitysdk.SitesSubpath + "/"
}

// readSiteGrants is the grant a publisher must write for a peer that
// will read its site live.
//
// **The two `system/` resources are the part that is not obvious and
// that a site-shaped grant gets wrong.** A published site's bytes live
// under `sites/`, but the thing that makes them *verifiable*
// does not: the published-root sits at `system/peer/published-root` and
// its signature at `system/signature/{hex}`, both outside the prefix
// they commit to. A grant scoped to the site alone produces a peer that
// serves every page and cannot be verified at all — which presents as
// "this publisher has never published", the most misleading of the three
// states it could present as.
//
// Measured, not reasoned: dropping either row is the anti-vacuity arm in
// TestLiveRead_NeedsTheSignedRootPathsNotJustTheSite.
func readSiteGrants() []types.GrantEntry {
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources: types.CapabilityScope{Include: []string{
				"sites/*",
				"system/peer/published-root",
				"system/signature/*",
			}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}
}

// livePair stands up a publisher that has published a site both ways and
// a reader connected to it, with no wildcard grant anywhere.
type livePair struct {
	publisher *entitysdk.AppPeer
	reader    *entitysdk.AppPeer
	originURL string
	prefix    string
}

func newLivePair(t *testing.T, grants []types.GrantEntry) livePair {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	publisher, err := entitysdk.CreatePeer(entitysdk.PeerConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("publisher CreatePeer: %v", err)
	}
	t.Cleanup(func() { publisher.Close() })

	reader, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("reader CreatePeer: %v", err)
	}
	t.Cleanup(func() { reader.Close() })

	prefix := seedLiveSite(t, publisher)

	// The origin is baked into the profile the static consumer reads, so
	// the server has to exist before the publish — over the still-empty
	// directory it will be publishing into.
	out := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(out)))
	t.Cleanup(srv.Close)

	if _, err := publish.Publish(ctx, publish.Opts{
		Peer:      publisher,
		Prefix:    prefix,
		OutputDir: out,
		OriginURL: srv.URL,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// AP63: grants are assembled AT HANDSHAKE, so the policy row has to
	// exist before the dial. Written after the publish only because the
	// publish is what creates the thing being authorized.
	if grants != nil {
		policyPath := "system/capability/policy/" + reader.PeerID()
		if _, err := publisher.Store().Put(policyPath, types.TypeCapPolicyEntry,
			types.CapabilityPolicyEntryData{
				PeerPattern: reader.PeerID(),
				Grants:      grants,
				Notes:       "W2 gate: this peer may read the published site and verify it",
			}); err != nil {
			t.Fatalf("write publisher policy row: %v", err)
		}
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- publisher.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("publisher listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publisher not listening after 5s")
	}

	conn, err := reader.Connect(ctx, publisher.Addr().String())
	if err != nil {
		t.Fatalf("reader Connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return livePair{publisher: publisher, reader: reader, originURL: srv.URL, prefix: prefix}
}

// chain is what one arm of the journey produced: the facts a surface
// would print, plus the page bytes.
type chain struct {
	// mode is the ARM'S OWN label, typed into the call below.
	mode string
	// declaredMode is what the verified root says answered, which came
	// from the Source rather than from this test. The two being compared
	// is the point: a surface reads `declaredMode`, and if it could
	// disagree with which consumer was built, every caption downstream is
	// a guess.
	declaredMode fetch.Mode
	authority    string
	freshness    string
	manifestAt   string
	signatureAt  string
	seq          uint64
	trieRoot     hash.Hash
	keys         []string
	nodes        int
	pageBody     string
	pageTitle    string
}

// run drives manifest → signature → walk → page over one consumer.
func run(t *testing.T, ctx context.Context, mode string, c *fetch.Consumer) chain {
	t.Helper()
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		t.Fatalf("[%s] VerifiedRoot: %v", mode, err)
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		t.Fatalf("[%s] Walk: %v", mode, err)
	}
	res := workbench.NewRemoteSiteResolver(ctx, c, root, walk)
	out := res.ResolvePage(workbench.SiteRoot(liveSiteID))
	if !out.Ready || out.Page == nil {
		t.Fatalf("[%s] ResolvePage: not resolved (err=%v)", mode, out.Err)
	}
	return chain{
		mode:         mode,
		declaredMode: root.Mode,
		authority:    root.Authority,
		freshness:    root.Freshness(),
		manifestAt:   root.ManifestURL,
		signatureAt:  root.SignatureURL,
		seq:          root.Data.Seq,
		trieRoot:     root.Data.RootHash,
		keys:         walk.Keys(),
		nodes:        walk.Nodes(),
		pageBody:     out.Page.Page.Body,
		pageTitle:    out.Page.Page.Title(),
	}
}

// TestLiveAndStatic_SameSiteSameBytesDifferentLocators is W2's primary
// result, and **the control arm is half the test**.
//
// Without the static arm the live path could quietly be doing something
// else — reading the publisher's tree directly, skipping the walk,
// trusting the connection — and still look green. Running both over one
// published act and requiring the page bytes and the committed key set
// to agree is what makes "the same verification stack" a measurement
// rather than a claim about the code.
func TestLiveAndStatic_SameSiteSameBytesDifferentLocators(t *testing.T) {
	pair := newLivePair(t, readSiteGrants())
	ctx := context.Background()

	live, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	liveChain := run(t, ctx, "live", live)

	layout, err := fetch.LoadLayout(ctx, pair.originURL, nil)
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	if layout.PeerID != pair.publisher.PeerID() {
		t.Fatalf("the origin advertises peer %s; the publisher is %s", layout.PeerID, pair.publisher.PeerID())
	}
	staticChain := run(t, ctx, "static", fetch.NewConsumer(layout, nil))

	// The bytes. This is the product claim.
	if liveChain.pageBody != staticChain.pageBody {
		t.Errorf("the two modes served different page bodies\n  live:   %q\n  static: %q",
			liveChain.pageBody, staticChain.pageBody)
	}
	if liveChain.pageTitle != staticChain.pageTitle {
		t.Errorf("page title: live %q, static %q", liveChain.pageTitle, staticChain.pageTitle)
	}
	if liveChain.pageBody == "" {
		t.Fatal("anti-vacuity: both modes resolved an empty body, so equality says nothing")
	}

	// The commitment. Same signed root, same key set, same interior.
	if liveChain.trieRoot != staticChain.trieRoot {
		t.Errorf("root_hash: live %s, static %s", liveChain.trieRoot, staticChain.trieRoot)
	}
	if liveChain.seq != staticChain.seq {
		t.Errorf("seq: live %d, static %d", liveChain.seq, staticChain.seq)
	}
	if strings.Join(liveChain.keys, "\n") != strings.Join(staticChain.keys, "\n") {
		t.Errorf("committed key sets differ\n  live:   %v\n  static: %v", liveChain.keys, staticChain.keys)
	}
	if liveChain.nodes != staticChain.nodes {
		t.Errorf("CHAMP nodes walked: live %d, static %d", liveChain.nodes, staticChain.nodes)
	}
	if len(liveChain.keys) < fillerPages {
		t.Fatalf("anti-vacuity: only %d committed keys — a manifest and %d pages were seeded",
			len(liveChain.keys), fillerPages+2)
	}
	// The walk has to WALK. At one node this whole comparison is a
	// statement about a single fetch: no interior structure to disagree
	// about, and none of the concurrency a live walk runs at.
	if liveChain.nodes < 2 {
		t.Fatalf("anti-vacuity: the trie is %d node(s), so nothing was traversed", liveChain.nodes)
	}

	// And the one thing that MUST differ: where each step looked. A
	// surface that cannot tell the two apart cannot caption them
	// differently, and the freshness sentence is the product difference
	// (W3).
	if liveChain.manifestAt == staticChain.manifestAt {
		t.Errorf("both modes report the same locator %q — a chain that cannot say which mode "+
			"answered cannot carry the freshness sentence", liveChain.manifestAt)
	}
	if !strings.HasPrefix(liveChain.manifestAt, "entity://") {
		t.Errorf("live manifest locator %q is not an entity address", liveChain.manifestAt)
	}
	if !strings.HasPrefix(staticChain.manifestAt, "http") {
		t.Errorf("static manifest locator %q is not a URL", staticChain.manifestAt)
	}
	if !strings.HasPrefix(liveChain.signatureAt, "entity://") {
		t.Errorf("live signature locator %q is not an entity address", liveChain.signatureAt)
	}

	// W3 — and this is the only place in the tree where BOTH sentences
	// come out of a real verification rather than a struct literal.
	//
	// The mode is read off the root, which got it from the Source. It is
	// asserted against the arm's own label deliberately: the failure this
	// prevents is a surface captioning a chain from its memory of which
	// constructor it called, and the only way to catch that is to have
	// two independent statements of the same fact and require them to
	// agree.
	if liveChain.declaredMode != fetch.ModeLivePeer {
		t.Errorf("the live root declares mode %q, want %q", liveChain.declaredMode, fetch.ModeLivePeer)
	}
	if staticChain.declaredMode != fetch.ModeStaticOrigin {
		t.Errorf("the static root declares mode %q, want %q", staticChain.declaredMode, fetch.ModeStaticOrigin)
	}
	if liveChain.authority != "entity://"+pair.publisher.PeerID() {
		t.Errorf("live authority = %q, want the publisher's entity address", liveChain.authority)
	}
	if staticChain.authority != strings.TrimRight(pair.originURL, "/") {
		t.Errorf("static authority = %q, want the origin %q", staticChain.authority, pair.originURL)
	}

	// The sentences themselves, and the cross-exclusion — asserted here
	// too rather than only in `fetch/freshness_test.go`, because that one
	// builds its roots by hand and so cannot fail if `Describe` is never
	// called on the way through a real read.
	if !strings.Contains(staticChain.freshness, "withholding origin") {
		t.Errorf("the static arm does not confess the withholding case:\n  %s", staticChain.freshness)
	}
	if strings.Contains(staticChain.freshness, "answered for itself") {
		t.Errorf("the static arm claims the publisher answered for itself:\n  %s", staticChain.freshness)
	}
	if !strings.Contains(liveChain.freshness, "answered for itself") {
		t.Errorf("the live arm does not say the publisher answered for itself:\n  %s", liveChain.freshness)
	}
	if strings.Contains(liveChain.freshness, "withholding origin") {
		t.Errorf("the live arm confesses an origin that was not in the exchange:\n  %s", liveChain.freshness)
	}
	// Both describe the SAME signed act, so the signed moment and the seq
	// must appear identically in both; only the scope-of-claim differs.
	// Without this the two sentences could be right about the wrong root.
	for _, want := range []string{fmt.Sprintf("seq %d", liveChain.seq)} {
		if !strings.Contains(liveChain.freshness, want) || !strings.Contains(staticChain.freshness, want) {
			t.Errorf("the two sentences do not agree on %q:\n  live:   %s\n  static: %s",
				want, liveChain.freshness, staticChain.freshness)
		}
	}

	t.Logf("one site, two projections:\n  live   %s\n         sig %s\n         %s\n"+
		"  static %s\n         sig %s\n         %s\n  seq=%d root=%s keys=%d nodes=%d",
		liveChain.manifestAt, liveChain.signatureAt, liveChain.freshness,
		staticChain.manifestAt, staticChain.signatureAt, staticChain.freshness,
		liveChain.seq, liveChain.trieRoot, len(liveChain.keys), liveChain.nodes)
}

// TestLiveRead_NeedsTheSignedRootPathsNotJustTheSite is the arm that
// turns readSiteGrants' doc comment from an argument into a measurement.
//
// A grant scoped to `sites/*` alone is the one a person writes
// when they think about what a site *is*. It authorizes every page and
// refuses the published-root — and the failure does not present as
// "permission denied on the manifest". It presents as a publisher with
// no signed entry point, i.e. as the publisher's fault, on the reader's
// machine, with the grant looking complete.
func TestLiveRead_NeedsTheSignedRootPathsNotJustTheSite(t *testing.T) {
	siteOnly := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"sites/*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}
	pair := newLivePair(t, siteOnly)

	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	_, err = c.VerifiedRoot(context.Background())
	if err == nil {
		t.Fatal("a reader granted the site subtree only verified a published root anyway — " +
			"either the grant is not being enforced or the root no longer comes from system/")
	}
	t.Logf("site-only grant cannot verify: %v", err)
}

// TestLiveRead_UnpublishedPeerIsItsOwnState decides Q3 and pins the
// decision where a surface can act on it.
//
// A peer that answers and has published nothing is not unreachable and
// is not withholding. v1 refuses to read one through this stack — every
// check hangs off the signed root, so admitting it would mean structure
// coming from what the far side says it has — but it refuses *by its own
// name*, because the three states send an operator to three different
// places and only one of them is the publisher's problem.
func TestLiveRead_UnpublishedPeerIsItsOwnState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// No publish call anywhere in this test. The peer is reachable, has
	// a tree, and has committed to nothing.
	quiet, err := entitysdk.CreatePeer(entitysdk.PeerConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer quiet.Close()
	seedLiveSite(t, quiet)

	reader, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("reader CreatePeer: %v", err)
	}
	defer reader.Close()

	policyPath := "system/capability/policy/" + reader.PeerID()
	if _, err := quiet.Store().Put(policyPath, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: reader.PeerID(),
			Grants:      readSiteGrants(),
			Notes:       "W2 gate: fully authorized, and there is still nothing to read",
		}); err != nil {
		t.Fatalf("write policy row: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- quiet.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("peer not listening after 5s")
	}
	conn, err := reader.Connect(ctx, quiet.Addr().String())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	c, err := workbench.NewPeerConsumer(reader, quiet.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	_, err = c.VerifiedRoot(ctx)
	if !errors.Is(err, fetch.ErrNoPublishedRoot) {
		t.Fatalf("VerifiedRoot err = %v; want ErrNoPublishedRoot — a reachable peer that has "+
			"published nothing must not read as a transport failure or as a withholding origin", err)
	}
	// The refusal survives the layer above unwrapped, for the same reason
	// errNoManifestPrefix does: wrapped in "fetch manifest:" it reads as a
	// network problem, and retrying cannot change it.
	if strings.Contains(err.Error(), "fetch manifest:") {
		t.Errorf("an unpublished peer was reported as a fetch failure:\n  %v", err)
	}
	t.Logf("third state, named: %v", err)
}

// TestBrowseModel_PeerIDAddressTakesTheLiveRoadWithNoOrigin is W3's
// chooser, end to end, through the model every shipped surface drives.
//
// # Why this arm and not another
//
// The unit gate (`workbench/browse_road_test.go`) asserts the ORDER of
// the roads and the declines. What it cannot assert is that taking the
// live road produces a page: `roadsFor` has no bytes in it. And the two
// arms above construct their consumers directly, so nothing in this file
// had ever *chosen* a road either.
//
// This one goes through `BrowseModel.Open`, which is what `entity-shell`'s
// `open` verb and the Avalonia Browser panel's address box both call. A
// green run means an operator typing a peer-id gets a verified page off a
// machine with **no origin at all** — which is the configuration a laptop
// is permanently in, and the one the static corridor structurally cannot
// serve.
//
// # The refusal it replaces
//
// Before the chooser this returned *"a peer-id address needs an origin to
// fetch from"*. That sentence was true about peer-ids (NETWORK §6.5.4)
// and false about this situation — the reader was already connected to
// the publisher — which is AP44's shape: a refusal asserting a fact about
// the world, correct in general and wrong here, leaving no wrong answer
// behind for anyone to catch.
func TestBrowseModel_PeerIDAddressTakesTheLiveRoadWithNoOrigin(t *testing.T) {
	pair := newLivePair(t, readSiteGrants())
	ctx := context.Background()

	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)
	// No registry pin and NO TARGET ORIGIN. The static corridor is
	// deliberately unreachable from this model, so a green run cannot be
	// the origin answering.
	addr := "entity://" + pair.publisher.PeerID() + "/" + liveSiteID + "/index"
	if err := m.Open(ctx, addr); err != nil {
		t.Fatalf("Open(%s): %v", addr, err)
	}

	out := m.Render()
	if out.Err != "" {
		t.Fatalf("navigation reported: %s", out.Err)
	}
	if !strings.Contains(out.Content.BodyMarkdown, "Read this from a CDN or from the machine that wrote it") {
		t.Fatalf("page body is not the published one:\n%s", out.Content.BodyMarkdown)
	}

	// The chain must say a LIVE peer answered, and must say it from the
	// Source rather than from the model's memory of what it built — that
	// is fetch/freshness.go's rule and the reason `Mode` rides on the
	// root.
	if !strings.Contains(out.Freshness, "the publisher answered for itself") {
		t.Errorf("freshness sentence is not the live one:\n  %s", out.Freshness)
	}
	if strings.Contains(out.Freshness, "withholding origin") {
		t.Errorf("a live read was captioned with the static sentence:\n  %s", out.Freshness)
	}
	var transport workbench.ConsumeStep
	for _, s := range out.Steps {
		if s.Name == "transport" {
			transport = s
		}
	}
	if transport.Status != workbench.StepOK || !strings.Contains(transport.Detail, "live peer") {
		t.Errorf("transport step does not name the live road: %+v", transport)
	}
	if !strings.Contains(transport.Detail, "no origin consulted") {
		t.Errorf("transport step does not say an origin was never consulted: %q", transport.Detail)
	}

	// Anti-vacuity, and it is the arm that matters: the SAME address on a
	// browser with NO PEER must refuse, naming the missing origin. Without
	// it, everything above is satisfied by a build in which the peer-id
	// branch quietly reads the local store.
	noPeer := workbench.NewBrowseModel(nil)
	if err := noPeer.Open(ctx, addr); err == nil {
		t.Fatal("a peer-less browser opened a peer-id address with no origin — the live road " +
			"is being taken by something other than the peer")
	} else if !strings.Contains(err.Error(), "needs an origin") {
		t.Errorf("the peer-less refusal changed shape: %v", err)
	}
}
