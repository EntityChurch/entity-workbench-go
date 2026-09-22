package publish_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// a36_peer_root_probe_test.go — what arch's `A-36` ruling costs, measured
// on the road the ruling is about.
//
// # The ruling
//
// `ROUTING-2026-09-15-a-…-A-36-is-ruled-the-prefix-objection-does-not-hold`
// §1.3: our filed objection said *"there is no prefix that contains both
// except the universal tree"*. That is wrong and arch is right — the
// prefix containing `app/feed/…` and `system/signature/…` is **this
// peer's own namespace**, which is one peer's subtree and not the
// universal tree. §1.6 tells us to **move the publish scope to the peer
// root**, and says it is the one item another seat is blocked on.
//
// # Why this file exists rather than a commit doing that
//
// `A-36` is a defect **on the static road** — a live reader already gets
// the signature, because the grant names `system/signature/*` separately
// (measured in `feed_live_test.go`, both arms). So the fix has to be
// evaluated on the static road, and on the static road **there is no
// grant at all**: `Publish` emits every entity the root commits to into
// a directory that gets uploaded.
//
// Arch's §1.3 answers the objection we *filed* — which was about the
// GRANT — and the ruling's own sentence *"with no wildcard anywhere and
// no grant widened"* is true. The question it does not reach is what
// the trie commits to, and that question has a different answer on each
// road. Hence three arms:
//
//   - `TestA36_PeerRootPublishDoesFixAttribution` — the ruling works.
//     The signature lands in the committed key set. **This arm is the
//     reason the other two are not an argument for doing nothing.**
//   - `TestA36_PeerRootPublishAlsoCommitsToEveryPrivateDeclaration` —
//     what else lands in it.
//   - `TestA36_PeerRootStaticEmitPutsPrivateStateInTheUploadDirectory` —
//     the same fact as bytes on disk in the directory an operator
//     `rsync`s to a CDN, which is the form that is not arguable.
//
// ⭐ **The generalisation worth keeping:** a ruling that answers the
// objection as filed can still miss the defect, because *the filing seat
// chose which objection to raise*. We filed the grant argument because
// the grant is where AP90 and AP99 bit us, and the grant is the half
// arch could check. Nobody checked the other half, on either side.

// seedPrivateState writes the declarations a real workbench peer holds —
// the ones an operator would be startled to find on a CDN.
//
// Written through the same types the product writes, not hand-rolled
// maps: the question is what a publish of THIS peer emits, and a
// fixture that invents lighter-weight entities would understate it.
func seedPrivateState(t *testing.T, ap *entitysdk.AppPeer) []string {
	t.Helper()

	var keys []string
	put := func(path, typeName string, data any) {
		t.Helper()
		if _, err := ap.Store().Put(path, typeName, data); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
		keys = append(keys, path)
	}

	// A device declaration: another machine's peer-id and the address we
	// last reached it on. `workbench/desired_state.go`.
	put(workbench.DevicePrefix+"peer-the-operator-shares-with", workbench.DeviceType,
		workbench.DeviceData{
			PeerID:    "peer-the-operator-shares-with",
			Label:     "laptop",
			Addresses: []string{"192.168.1.44:9110"},
		})

	// A folder declaration: a local filesystem path.
	put(workbench.FolderPrefix+"owner.notes", workbench.FolderType,
		workbench.FolderData{
			Root:      "notes",
			LocalRoot: "/home/operator/private/notes",
		})

	// An ingested document from a mounted folder.
	put("doc/notes/salary-review.md", "doc/markdown-file",
		map[string]any{
			"title": "salary review 2026",
			"body":  "the contents of a file in a folder nobody shared",
		})

	// The capability policy table itself — who we have granted what.
	put("system/capability/policy/peer-the-operator-shares-with", types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: "peer-the-operator-shares-with",
			Notes:       "who this peer has granted access to, and to what",
		})

	return keys
}

// publishedOverPeerRoot mints a root over the whole peer namespace and
// returns the committed key set as a verifying reader sees it.
func publishedOverPeerRoot(t *testing.T, posts int) (pair liveFeedPair, committed map[string]bool, private []string) {
	t.Helper()

	// The harness seeds the feed and mints; the private state has to be in
	// the tree BEFORE the mint or the root does not commit to it, which
	// would make this probe measure nothing while passing.
	pair = newLiveFeedPairSeeded(t, posts, "", func(ap *entitysdk.AppPeer) {
		private = seedPrivateState(t, ap)
	})

	ctx := context.Background()
	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		t.Fatalf("VerifiedRoot: %v", err)
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	committed = map[string]bool{}
	for _, k := range walk.Keys() {
		committed[rejoin(root.Data.Prefix, k)] = true
	}
	return pair, committed, private
}

// rejoin performs §3.3a's `prefix + relative_key` reconstruction.
//
// ⚠ **The universal tree is the one prefix where the concatenation as
// written is wrong**, and this cost a debugging round. §3.3a spells the
// universal tree `"/"` and every other prefix without a leading slash,
// so literal concatenation yields `"/app/feed/index"` for a peer-root
// publish and `"app/feed/index"` for every other — the same key in two
// shapes, differing only for the case an implementation is least likely
// to have a fixture for.
//
// A reader that gets this wrong sees a root that commits to NOTHING it
// recognises, which presents as an empty publish under a valid
// signature. That is the identical failure this repo already routed to
// arch as a reader's-mistakes entry (`ROUTING-2026-09-15-a` §5.3, taken
// into `GUIDE-APPLICATION-DEVELOPMENT`) — **this is its second instance
// and a different trigger**: that one was forgetting to add the prefix,
// this one is adding it exactly as specified.
func rejoin(prefix, relative string) string {
	if prefix == "/" || prefix == "" {
		return relative
	}
	return prefix + relative
}

// TestA36_PeerRootPublishDoesFixAttribution is the arm that says the
// ruling works, and it is first on purpose.
//
// Publishing over the peer root puts `FEED-R2`'s detached signature
// inside the committed key set, which is exactly what `A-36` asked for
// and what a static reader has no other route to. The two arms below
// are the price of this one, not a refutation of it.
func TestA36_PeerRootPublishDoesFixAttribution(t *testing.T) {
	pair, committed, _ := publishedOverPeerRoot(t, 2)

	for i, p := range pair.posts {
		sigKey := types.LocalSignaturePath(p.Hash)
		if !committed[sigKey] {
			t.Fatalf("entry %d: publishing over the peer root still does not commit to %q — "+
				"if this fails, arch's A-36 fix does not work in this tree and the finding is "+
				"bigger than the one this file reports", i, sigKey)
		}
	}

	// Anti-vacuity: the same feed published over `app/feed/` must NOT
	// commit to them, or `committed` is not measuring the prefix at all.
	// This is `FEED-14`'s negative arm (arch §1.7) and it is the check
	// neither seat had.
	narrow := newLiveFeedPair(t, 2)
	ctx := context.Background()
	c, err := workbench.NewPeerConsumer(narrow.reader, narrow.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		t.Fatalf("VerifiedRoot: %v", err)
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, k := range walk.Keys() {
		if strings.HasPrefix(rejoin(root.Data.Prefix, k), "system/signature/") {
			t.Fatalf("the `app/feed/` publish committed to %q — then the positive arm above proves "+
				"nothing about the prefix", rejoin(root.Data.Prefix, k))
		}
	}
}

// TestA36_PeerRootPublishAlsoCommitsToEveryPrivateDeclaration measures
// what else a peer-root root commits to.
//
// ⛔ This is not a bug in the ruling's reasoning. It is the half of the
// question the ruling did not reach: `A-36` is a defect about the
// STATIC road, and moving the publish scope to satisfy it moves what the
// static corridor emits.
func TestA36_PeerRootPublishAlsoCommitsToEveryPrivateDeclaration(t *testing.T) {
	_, committed, private := publishedOverPeerRoot(t, 2)

	var leaked []string
	for _, k := range private {
		if committed[k] {
			leaked = append(leaked, k)
		}
	}
	sort.Strings(leaked)

	if len(leaked) != len(private) {
		// Fewer than all of them would mean the peer-root publish is
		// filtering something, which `Publish` explicitly refuses to do —
		// worth knowing about, and not what we expect.
		t.Logf("NOTE: %d of %d seeded private keys are in the committed set, not all of them — "+
			"something is scoping this publish and the refusal in publish.go says nothing does",
			len(leaked), len(private))
	}
	if len(leaked) == 0 {
		t.Fatalf("no private declaration is in the committed set — then this probe is measuring "+
			"the wrong thing and the report built on it is wrong.\nseeded: %v", private)
	}

	t.Logf("a peer-root publish commits to %d of %d seeded private keys:\n  %s",
		len(leaked), len(private), strings.Join(leaked, "\n  "))

	// The whole committed set, counted by top-level segment, so the report
	// carries a number rather than an adjective.
	byPrefix := map[string]int{}
	for k := range committed {
		seg := k
		if i := strings.Index(k, "/"); i >= 0 {
			seg = k[:i]
		}
		byPrefix[seg]++
	}
	segs := make([]string, 0, len(byPrefix))
	for s := range byPrefix {
		segs = append(segs, s)
	}
	sort.Strings(segs)
	var parts []string
	for _, s := range segs {
		parts = append(parts, fmt.Sprintf("%s=%d", s, byPrefix[s]))
	}
	t.Logf("committed key set by top-level segment: %s (total %d)",
		strings.Join(parts, " "), len(committed))
}

// TestA36_PeerRootStaticEmitPutsPrivateStateInTheUploadDirectory is the
// same fact in the form that is not arguable: bytes on disk, in the
// directory an operator uploads.
//
// The live road has a grant and `PublicSiteGrants` refuses the whole
// tree, so on that road the peer-root publish simply has no public grant
// available. The static road has **no grant at all** — `Publish` writes
// the closure of everything the root commits to into `OutputDir`, and
// the operator's next step is to copy that directory to a public origin.
func TestA36_PeerRootStaticEmitPutsPrivateStateInTheUploadDirectory(t *testing.T) {
	ctx := context.Background()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })

	seedLiveFeed(t, ap, 2)
	private := seedPrivateState(t, ap)

	narrowDir := t.TempDir()
	if _, err := publish.Publish(ctx, publish.Opts{
		Peer: ap, Prefix: "app/feed/", OutputDir: narrowDir,
		OriginURL: "https://example.invalid", At: time.UnixMilli(1_700_000_000_000),
	}); err != nil {
		t.Fatalf("narrow Publish: %v", err)
	}

	wideDir := t.TempDir()
	wide, err := publish.Publish(ctx, publish.Opts{
		Peer: ap, Prefix: "", OutputDir: wideDir,
		OriginURL: "https://example.invalid", At: time.UnixMilli(1_700_000_000_000),
		// Explicit, because the guard this measurement produced now
		// refuses it by default — see TestA36_TheGuard… below.
		AllowWholePeer: true,
	})
	if err != nil {
		t.Fatalf("peer-root Publish: %v", err)
	}

	narrowHits := grepDir(t, narrowDir, "salary review 2026", "/home/operator/private/notes", "192.168.1.44:9110")
	wideHits := grepDir(t, wideDir, "salary review 2026", "/home/operator/private/notes", "192.168.1.44:9110")

	t.Logf("static emit over `app/feed/`: %d entities, private strings found in %d files",
		countEntities(t, narrowDir), narrowHits)
	t.Logf("static emit over the peer root: %d entities (%d paths), private strings found in %d files",
		wide.Entities, wide.Paths, wideHits)

	if narrowHits != 0 {
		t.Errorf("the `app/feed/` emit already contains private strings in %d files — then the "+
			"scope is not what is protecting anything and this finding is larger", narrowHits)
	}
	if wideHits == 0 {
		t.Fatalf("the peer-root emit contains none of the seeded private strings — then the "+
			"report built on this probe is wrong.\nseeded keys: %v", private)
	}
}

// TestA36_TheGuardRefusesAWholePeerPublishAndNamesWhatItWouldDisclose
// is the guard the measurement above produced.
//
// `A-38` is open — arch has not ruled which of the three shapes `A-36`
// ends up as — and this is right under all three: after the numbers in
// this file, a publish that commits an operator's private declarations
// to an upload directory must be something they said, not something an
// empty string did.
//
// The refusal is keyed on the FACT (this publish commits to `system/`
// keys and to application keys) rather than on the prefix's spelling,
// which is AP97's lesson: `mintSignedRoot`'s empty-prefix guard compared
// against a root hash that is never zero and therefore could never fire.
func TestA36_TheGuardRefusesAWholePeerPublishAndNamesWhatItWouldDisclose(t *testing.T) {
	ctx := context.Background()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	seedLiveFeed(t, ap, 2)
	seedPrivateState(t, ap)

	_, err = publish.MintRoot(ctx, publish.MintOpts{Peer: ap, Prefix: ""})
	if err == nil {
		t.Fatal("a whole-peer publish went through with no acknowledgement — " +
			"the operator's device list, folder paths and documents are now in a signed root " +
			"because somebody typed an empty string")
	}
	msg := err.Error()

	// It must NAME what it would disclose. A refusal that only says "no"
	// sends the operator to re-read the prefix, which is AP44's shape and
	// is what the sibling empty-prefix refusal had to be fixed for.
	for _, want := range []string{"app", "doc", "system", "-whole-peer", "A-36"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not mention %q — it has to say what is on offer, "+
				"name the way through, and name the reason somebody is here.\n  got: %s", want, msg)
		}
	}

	// Arm two: the acknowledgement works. A guard with no way past it is
	// a feature deletion wearing a safety argument.
	if _, err := publish.MintRoot(ctx, publish.MintOpts{
		Peer: ap, Prefix: "", AllowWholePeer: true,
	}); err != nil {
		t.Fatalf("the acknowledged whole-peer publish was still refused: %v", err)
	}

	// Arm three, the control: a narrow publish is untouched. Without this
	// the whole guard could be `return error` and every arm above passes.
	if _, err := publish.MintRoot(ctx, publish.MintOpts{Peer: ap, Prefix: "app/feed/"}); err != nil {
		t.Fatalf("a narrow publish was refused by the whole-peer guard: %v", err)
	}

	// Arm four: a prefix that names `system/` ALONE is not caught. The
	// operator named the one thing being published, so there is no second
	// category being swept along and nothing to disclose that the prefix
	// did not already say. This arm is what stops the guard growing into
	// "refuse anything that mentions system", which would break
	// `registry_roundtrip_test.go` and every future system-prefix publish.
	if _, err := publish.MintRoot(ctx, publish.MintOpts{Peer: ap, Prefix: "system/"}); err != nil {
		t.Fatalf("a deliberate `system/` publish was refused: %v", err)
	}
}

// grepDir counts files under root whose bytes contain any of the needles.
//
// Reads raw bytes rather than decoding: the question is what an operator
// uploads, and a CBOR entity body carries its strings in the clear
// whether or not anything in this tree can name the type.
func grepDir(t *testing.T, root string, needles ...string) int {
	t.Helper()
	hits := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, n := range needles {
			if bytes.Contains(b, []byte(n)) {
				hits++
				t.Logf("  %q found in %s", n, strings.TrimPrefix(p, root))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return hits
}

// countEntities counts the content-addressed shards in an emit directory.
func countEntities(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(filepath.Join(root, "content"), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("counting %s: %v", root, err)
	}
	return n
}
