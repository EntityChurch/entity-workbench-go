# The FEED joint fixture (`J-4`) — vendored, frozen

**Provenance:** `entity-browser-rust` `dev` @ `2e24636`, their
`tests/fixtures/feed-joint/`, built at their `dc96c2e` (2026-09-12). Delivered in
`ROUTING-2026-09-12-d-workbench-go-THE-FEED-JOINT-FIXTURE-IS-BUILT-AND-THE-PEER-ID-IS-IN-THE-BYTES.md`.
Their `README.md` is the protocol and is the thing to read; this file records what is different on
our side of it.

**`J-5` split the work: they produce the rig, WE produce the bytes.** `entitysdk/feed_crossimpl_test.go`
is our half. Steps 0–4 are one test each, in the protocol's order, because the one thing they asked
back for when they accepted the split was **a per-key report before any root comparison** — a bare
root mismatch costs a session to localize.

**Do not regenerate anything here.** `EXPECTED.json` is their computed half. If it moves, their
encoder moved, and the question is which side is right. **A divergence in steps 1–3 is ROUTED, not
corrected.**

## ⛔ The two things that do not carry over from the site fixture

Both failures are silent — plausible entities, different bytes, nothing anywhere saying why.

| | site | feed |
|---|---|---|
| peer id in the entity body | no | **yes.** `FEED-R1` makes an entry's `author` the namespace it is read under, and every index page is `pin(author, entry_hash)` |
| keys pinned by the convention | every one, relative to the site root | **the index only.** §4.2 pins `app/feed/index` and `app/feed/index/{page}`; §2 leaves an entry's path local |

So the fixture pins an **Ed25519 seed**, not a peer id — a literal peer id would leave `FEED-R2`'s
detached signature out of the comparison entirely, because you cannot sign as a peer whose key you
do not hold. And there are **two roots, of which only `index_root` is a comparand**: their
`feed_root_ours` additionally covers `app/feed/entries/{hex}`, which is their local placement.

`entitysdk/testdata/crossimpl-rust-share/` is the mirror image on that axis — **nothing in those
bytes carries a peer id at all**, because SHARE §4 makes the namespace the publisher. Two
conventions, opposite answers on one axis; know which one you are in.

## What our run established, 2026-09-13

**All five steps agree on the first run**, with no correction to either side:

| step | result |
|---|---|
| 0 — peer id from the pinned seed | `2K42FX8pASWDrXaAVsGXMNJbAkCVBuVwXf4RuwFNnmyYis`, exact |
| 1 — five entry content hashes | 5/5 byte-identical |
| 2 — five detached signatures + their invariant-pointer keys | 5/5 byte-identical |
| 3 — four index bindings, key by key | 4/4 byte-identical |
| 4 — the trie root over the §4.2-pinned keys | `001064708fe2ad99d0…`, identical |

**Three neuters, each falsified, each landing on a distinct row** — the within-page order reversed
(steps 3 and 4 red), **both** orderings reversed (steps 3 and 4 red, which is the case the README
warns round-trips perfectly against a self-read harness), and `signer` set to something other than
the identity entity's content hash (step 2 red).

**This is the first cross-implementation agreement on `app/feed/*` in the ecosystem**, and per
[ADR-0012] it is not cohort-consistent: two independent code bases, two languages, one authored
input.

## Four encodings we MATCHED rather than re-derived

They shipped FEED first, so where the specification is silent their choice is the baseline —
*whatever publishes first becomes the corpus*. Each is cited in `entitysdk/feed.go`'s header so the
coupling is greppable if arch moves one.

1. **`signer` is the identity entity's CONTENT HASH, not the peer-id string.** §1.1's shorthand
   reads *"signer = author"* and `system/signature` is the kernel's type, whose field is a hash.
2. **Pages fill oldest-first; entries within a page read newest-first** (§4.5).
3. **A page's `updated_at` is the max `created_at` of the entries it carries** — their reading of a
   field §4.2 gives no semantics to, routed by them as `A-41`. **We match it deliberately**, which
   is what they asked for: one of us becomes the baseline either way and it should be on purpose.
4. **The head omits `oldest` when nothing has been dropped** (§4.2 defaults it to 0).
