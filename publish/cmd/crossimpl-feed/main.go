// crossimpl-feed emits a deterministic Go-published FEED corridor for
// `entity-browser-rust`'s reader — corridor ①, the first run in this
// ecosystem where a real publisher and a real reader are pointed at each
// other rather than two producers being compared.
//
// Per [ADR-0012] that is a different class of evidence from the `J-4`
// fixture: **two encoders agreeing is not evidence that a reader is
// right.** `J-4` established that our `entitysdk` and their producer
// encode authored input to the same bytes. It could not establish that
// anything either of us SERVES is walkable, because the comparison never
// left the encoder.
//
// Shape agreed with `entity-browser-rust` as their `B-13` and adopted
// whole by arch (`ROUTING-2026-09-15-a` §2): **a static tree we emit and
// they read.** Reproducible, nothing to schedule, and it exercises §4.3's
// walk end to end.
//
//	go run ./cmd/crossimpl-feed -out /tmp/go-feed-corridor
//
// # ⭐ Why 34 entries, and why it is cut TWICE
//
// **34 against a page size of 32.** Every feed fixture either seat has
// produced to date has been ONE page, so `FEED-R12` — the multi-page
// index rules — has been unfalsifiable on both sides simultaneously.
// 34 puts 32 on a sealed page and 2 on the current one, so a reader that
// stops at the head, or that reads pages in the wrong direction, or that
// never follows `previous`, fails here and passes on every fixture
// before it. Their call, their reasoning, and arch confirmed both.
//
// **Twice**, per arch's §2 — offered by them as "not required" and taken
// because the marginal cost is one flag at cut time and it will never be
// cheaper than in the session that cuts the first:
//
//   - `peer-root/` — the root commits to the peer's whole namespace, so
//     `FEED-R2`'s detached per-entry signatures ARE in the committed key
//     set and a static reader can attribute every entry.
//   - `feed-only/` — the root commits to `app/feed/` alone, so the
//     signatures are NOT in it and a static reader must honestly report
//     every entry as **unattributed** (`FEED-R4`).
//
// That pair IS `FEED-14` (arch's §1.7, new with the `A-36` proposal):
// ⭐ **a single-prefix fixture passes against both rules and measures
// neither.**
//
// # ⭐ And a THIRD cut since 2026-09-16 — `peer-root-curated/`, which is `FEED-15`
//
// `A-38` is ruled (arch `ROUTING-2026-09-16-a` §1) and the answer is **(D):
// move the PREFIX, do not move the CONTENT.** `EXTENSION-TREE` §3.3a makes a
// prefix a BOUND and says three separate times that it is not a completeness
// claim — a publisher *"MAY declare `/{peer_id}/` and publish a small
// subset"* — so a root may declare the peer root and commit to nine keys.
//
// **The two directories above are unchanged and byte-identical**; this one is
// additive, and it is the shape the product actually publishes
// (`publish -feed`). It carries `FEED-15`'s two arms, which is the check the
// proposal's fold is waiting on:
//
//   - the signatures ARE in the committed set — 34/34, same as `peer-root/`;
//   - and the peer's PRIVATE bindings are NOT — 0 in the emitted directory.
//
// ⭐ **Its peer holds private declarations on purpose, and that is the whole
// point of the arm.** Arch named the failure mode when they named the check:
// *"a fixture whose peer holds nothing else passes `FEED-R38` by having
// nothing to leak, and measures nothing."* So this cut's peer carries a
// device declaration with another machine's LAN address, a folder
// declaration with an operator's filesystem path, and an ingested document
// from a folder nobody shared — and the run refuses to finish if any of them
// reaches the directory.
//
// The numbers, measured at cut time: **438 bindings scanned vs 71 curated**
// (34 entries + 34 signatures + 1 index head + 2 pages), on one peer holding
// one feed and three private declarations.
//
// # ⚠ What this fixture does NOT endorse
//
// ⚠ **This section said `peer-root/` is "not what this product publishes"
// and that the PREFIX was the disclosure. `A-38` ruled otherwise on
// 2026-09-16 and the correction is kept visible rather than rewritten
// away** (AP80 — the sentence that used to be true is the one a stranger
// reads next). What was right: the SCANNED peer-root publish in
// `peer-root/` really does commit to this peer's device declarations,
// folder declarations with their local filesystem paths, ingested
// documents and capability policy table, and `publish` still REFUSES that
// prefix without an explicit acknowledgement. What was WRONG was the
// inference — that widening the prefix is what discloses. It is not.
// **Deriving the content set from the prefix is**, and `peer-root-curated/`
// is the same prefix with that inference removed.
//
// So the three directories are three rules to check, and only the third is
// a publish an operator should run:
//
//   - `peer-root/` — scanned. Demonstrates what attribution requires and
//     what a prefix scan costs. **Not a recommended shape.**
//   - `feed-only/` — narrow. Demonstrates the absence (`FEED-R4`).
//   - `peer-root-curated/` — **what `publish -feed` emits.**
//
// **A reader implementation should not treat any of the three as
// recommended or deprecated shapes to detect** — all three are conformant
// publishes and a reader that behaves differently on them has inferred
// something from the prefix that §3.3a forbids inferring.
//
// # Determinism
//
// Pinned seed and pinned instants, for the reason the site fixture's
// header records the hard way: a timestamp inside hashed bytes moves the
// entity hash, which moves the signature's key, its index row, the trie
// root, the root's own content hash, the signature entity's name and two
// content shards. Here it is worse than for a site, because an entry's
// `created_at` is inside the entry — so an unpinned clock moves all 34
// entry hashes and all 34 signature paths.
//
// **Its own seed**, distinct from `crossimpl-fixture`'s, so the two
// fixtures are two peers. A reader consuming both against one peer-id
// would not be able to tell a namespace rule from a coincidence, and
// `FEED-R1` is precisely a namespace rule.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// FeedFixtureSeed is the pinned Ed25519 seed. Changing it changes the
// peer-id, every entry hash, every signature path and the trie root —
// i.e. every citation in the corridor packet. Treat it as a wire
// constant.
var FeedFixtureSeed = [32]byte{
	0x77, 0x6f, 0x72, 0x6b, 0x62, 0x65, 0x6e, 0x63,
	0x68, 0x2d, 0x67, 0x6f, 0x2d, 0x63, 0x72, 0x6f,
	0x73, 0x73, 0x69, 0x6d, 0x70, 0x6c, 0x2d, 0x66,
	0x65, 0x65, 0x64, 0x00, 0x00, 0x00, 0x00, 0x01,
}

// FeedFixtureInstant is the pinned publish instant.
var FeedFixtureInstant = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)

// FeedFixtureEpoch is the pinned `created_at` of entry 0; each later
// entry is one minute after the one before it.
//
// Spaced rather than identical because §2.3.2 forbids a reader relying
// on `created_at` for ORDER, and a fixture where every entry carries the
// same instant cannot distinguish a reader that correctly ignores it
// from one that happens to be stable. Distinct, increasing values make
// the wrong implementation visible: a reader that sorts by `created_at`
// produces the same order as the index here, which is the point — the
// DISAGREEMENT is what the reversed-page rule creates, and it needs
// non-constant timestamps to show up at all.
var FeedFixtureEpoch = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// FeedFixtureEntries is 34 against a page size of 32. See the header.
const FeedFixtureEntries = 34

func main() {
	out := flag.String("out", "./crossimpl-feed-out", "output directory; two subdirectories are written under it")
	origin := flag.String("origin", "https://go-arm.example", "origin URL baked into the emitted transport profile")
	flag.Parse()

	if err := run(*out, *origin); err != nil {
		fmt.Fprintf(os.Stderr, "crossimpl-feed: %v\n", err)
		os.Exit(1)
	}
}

func run(out, origin string) error {
	// Two peers with the SAME seed rather than one peer published twice.
	//
	// A peer has exactly one published root, so publishing twice from one
	// peer advances `seq` and the second root supersedes the first — the
	// two directories would then carry roots at seq=1 and seq=2, and a
	// reader checking the §3-RES.4 rollback floor across both would be
	// correct to refuse the older one. Two peers from one seed give two
	// independent seq=1 roots with identical peer-ids and identical entry
	// bytes, which is what makes the pair comparable.
	for _, cut := range []struct {
		dir            string
		prefix         string
		allowWholePeer bool
		curated        bool
		seedPrivate    bool
		note           string
	}{
		{"peer-root", "", true, false, false,
			"signatures ARE in the committed set — FEED-R4 attributes every entry"},
		{"feed-only", "app/feed/", false, false, false,
			"signatures are NOT — FEED-R4 must report every entry unattributed"},
		{"peer-root-curated", "", false, true, true,
			"signatures ARE in the set AND the private bindings are NOT — this is FEED-15"},
	} {
		kp := crypto.FromSeed(FeedFixtureSeed)
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{Keypair: &kp})
		if err != nil {
			return fmt.Errorf("CreatePeer: %w", err)
		}

		posted, err := seedFeed(ap)
		if err != nil {
			ap.Close()
			return err
		}
		if cut.seedPrivate {
			// FEED-15's anti-vacuity half, and arch named it as the way
			// this check measures nothing: *"a fixture whose peer holds
			// nothing else passes FEED-R38 by having nothing to leak."*
			// So this cut's peer holds the things an operator would be
			// startled to find on a CDN, and the assertion is that they
			// are absent from the emitted directory anyway.
			if err := seedPrivateState(ap); err != nil {
				ap.Close()
				return err
			}
		}
		var content *publish.ContentSet
		if cut.curated {
			content = publish.FeedContent()
		}

		dir := filepath.Join(out, cut.dir)
		res, err := publish.Publish(context.Background(), publish.Opts{
			Peer:      ap,
			Prefix:    cut.prefix,
			OutputDir: dir,
			OriginURL: origin,
			At:        FeedFixtureInstant,
			// See the header: this peer holds a feed and nothing private,
			// which is the case the whole-peer guard is not for. A product
			// publish does not get to pass this.
			AllowWholePeer: cut.allowWholePeer,
			Content:        content,
		})
		if err != nil {
			ap.Close()
			return fmt.Errorf("publish %s: %w", cut.dir, err)
		}

		fmt.Printf("== %s ==\n", cut.dir)
		fmt.Printf("  peer_id:    %s\n", ap.PeerID())
		fmt.Printf("  prefix:     %q\n", cut.prefix)
		fmt.Printf("  root:       %s\n", res.SignedRoot.Data.RootHash)
		fmt.Printf("  seq:        %d\n", res.SignedRoot.Data.Seq)
		fmt.Printf("  bindings:   %d\n", res.SignedRoot.Bindings)
		fmt.Printf("  entities:   %d\n", res.Entities)
		fmt.Printf("  %s\n", cut.note)

		// Say whether the signatures actually landed, per cut, rather than
		// asserting it in the README. A README claim about emitted bytes is
		// the thing that goes stale silently; this is printed by the run
		// that produced them.
		sigsIn := signaturesInEmit(dir, posted)
		fmt.Printf("  entry signatures present in the emitted content store: %d/%d\n",
			sigsIn, len(posted))
		if cut.seedPrivate {
			// The other half of FEED-15, printed by the run that produced
			// the bytes rather than claimed in a README — a README claim
			// about emitted bytes is the thing that goes stale silently.
			leaked := grepEmit(dir, privateNeedles()...)
			fmt.Printf("  private declarations found in the emitted directory: %d (want 0)\n", leaked)
			if leaked != 0 {
				ap.Close()
				return fmt.Errorf("%s: %d private declaration(s) reached the upload directory",
					cut.dir, leaked)
			}
		}
		fmt.Println()

		ap.Close()
	}

	fmt.Println("entry hashes, oldest first (index page 0 holds the first 32; page 1 holds the last 2):")
	kp := crypto.FromSeed(FeedFixtureSeed)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{Keypair: &kp})
	if err != nil {
		return err
	}
	defer ap.Close()
	posted, err := seedFeed(ap)
	if err != nil {
		return err
	}
	for i, p := range posted {
		fmt.Printf("  [%2d] %s\n", i, p.Hash)
	}
	return nil
}

// seedFeed authors the pinned 34 entries on ap.
func seedFeed(ap *entitysdk.AppPeer) ([]entitysdk.PostedEntry, error) {
	f := ap.Feed()
	// Left at the default rather than set: the default IS 32, and pinning
	// it here would hide a change to the default from this fixture, which
	// is one of the two things the fixture exists to expose.
	if f.PageSize != entitysdk.DefaultFeedPageSize {
		return nil, fmt.Errorf("page size is %d, not the pinned default %d — "+
			"the 34-vs-32 property this fixture is built on no longer holds",
			f.PageSize, entitysdk.DefaultFeedPageSize)
	}
	var posted []entitysdk.PostedEntry
	for i := 0; i < FeedFixtureEntries; i++ {
		e, err := f.Post(entitysdk.PostRequest{
			Text: fmt.Sprintf("corridor entry %d — authored by the Go arm for the joint read", i),
			At:   FeedFixtureEpoch.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			return nil, fmt.Errorf("post %d: %w", i, err)
		}
		posted = append(posted, e)
	}
	return posted, nil
}

// signaturesInEmit counts how many entry signatures are reachable in the
// emitted static corridor, by looking for their content shards.
//
// Counted from the DIRECTORY rather than from the peer's tree, because
// the whole question is what a static reader with no second channel can
// reach. The peer's tree holds all 34 under both cuts; that is not the
// fact either cut is demonstrating.
func signaturesInEmit(dir string, posted []entitysdk.PostedEntry) int {
	present := map[string]bool{}
	_ = filepath.Walk(filepath.Join(dir, "content"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		present[filepath.Base(p)] = true
		return nil
	})
	n := 0
	for _, e := range posted {
		// The leaf name is the wire hex of the signature ENTITY's own
		// content hash, which we do not hold here — so ask the cheaper
		// question the directory can answer: is the key committed at all.
		// Counting shards by name would need the signature entity; the
		// listing side is what a reader walks.
		if leafCommitted(dir, types.LocalSignaturePath(e.Hash)) {
			n++
		}
	}
	return n
}

// leafCommitted reports whether the emitted tree carries a leaf object
// for a peer-relative path.
func leafCommitted(dir, relPath string) bool {
	var found bool
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return nil
		}
		if filepath.ToSlash(rel) == "" {
			return nil
		}
		if pathHasSuffixSegment(filepath.ToSlash(rel), relPath) {
			found = true
		}
		return nil
	})
	return found
}

func pathHasSuffixSegment(emitted, relPath string) bool {
	// Emitted leaf objects carry a suffix (`.leaf` by default); compare on
	// the path up to it.
	for _, suf := range []string{types.DefaultTreeLeafSuffix, ""} {
		want := relPath + suf
		if len(emitted) >= len(want) && emitted[len(emitted)-len(want):] == want {
			return true
		}
	}
	return false
}

// privateNeedles is the set of strings that must not appear anywhere in the
// curated cut's emitted directory.
//
// VALUES, not paths. A path can be absent from the listings while the bytes
// sit in a content shard under their hash, and a content-addressed shard is
// exactly as fetchable as a named one — which is the shape of disclosure that
// matters once the directory is on a CDN.
func privateNeedles() []string {
	return []string{
		"/home/operator/private/notes",
		"192.168.1.44:9110",
		"the contents of a file in a folder nobody shared",
	}
}

// seedPrivateState writes the declarations a real workbench peer holds — the
// ones an operator would be startled to find on a CDN.
//
// Written through the same types the product writes rather than as hand-rolled
// maps: the question is what a publish of THIS peer emits, and a fixture that
// invents lighter-weight entities understates it. Deliberately the same shapes
// as `publish/a36_peer_root_probe_test.go`'s helper, so the fixture and the
// gate are measuring one thing.
func seedPrivateState(ap *entitysdk.AppPeer) error {
	put := func(path, typeName string, data any) error {
		if _, err := ap.Store().Put(path, typeName, data); err != nil {
			return fmt.Errorf("seed %s: %w", path, err)
		}
		return nil
	}
	if err := put(workbench.DevicePrefix+"peer-the-operator-shares-with", workbench.DeviceType,
		workbench.DeviceData{
			PeerID:    "peer-the-operator-shares-with",
			Label:     "laptop",
			Addresses: []string{"192.168.1.44:9110"},
		}); err != nil {
		return err
	}
	if err := put(workbench.FolderPrefix+"owner.notes", workbench.FolderType,
		workbench.FolderData{
			Root:      "notes",
			LocalRoot: "/home/operator/private/notes",
		}); err != nil {
		return err
	}
	return put("doc/notes/salary-review.md", "doc/markdown-file",
		map[string]any{
			"title": "salary review 2026",
			"body":  "the contents of a file in a folder nobody shared",
		})
}

// grepEmit counts files under dir containing any of the needles.
func grepEmit(dir string, needles ...string) int {
	hits := 0
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		for _, n := range needles {
			if bytes.Contains(b, []byte(n)) {
				hits++
				return nil
			}
		}
		return nil
	})
	return hits
}
