# `app/share/*` bodies from entity-browser-rust's encoder — vendored, frozen

**Provenance:** `entity-browser-rust` `dev` @ `2e24636`, their
`tests/fixtures/share-crossimpl/`, built at their `bd84bcf` (2026-09-12) in answer to our ask
**`B-7`**. Delivered in
`ROUTING-2026-09-12-f-workbench-go-B-7-IS-BUILT-AND-A-PUBLISHED-FEED-NOW-REACHES-A-BROWSER.md`.

**Do not regenerate these bytes here.** They are the other seat's emission and the whole point is
that they did not come from our encoder. If they move, they moved *there*, and the question is which
side is right — see `entitysdk/share_crossimpl_test.go`'s header.

## Why we asked for it

`workbench/share_publication_crossimpl_test.go` builds its bodies **from the CDDL and from their
emitter's field spellings**, by hand. That catches a rename on *our* side and cannot catch one on
theirs — and, as it turned out, it could not catch one on ours either when our hand-built fixture
and our decoder were wrong in the *same* direction. *A test population you generated cannot contain
the shape you are missing.*

## What is here

```
bodies/{content_hash_hex}   the CANONICAL HASHABLE BODY of one share entity
EXPECTED.json               what each body is, and what a conformant reader decodes it to
```

A body file is byte-for-byte what a publisher writes to `content/{aa}/{bb}/{hex}` —
`ecf.EncodeHashable(type, data)`, the two-key `{data, type}` map. Re-hashing a body must give the
filename it is filed under, which makes the fixture self-verifying with no manifest.

## ⭐ Nothing in these bytes carries a peer id — the opposite of the FEED fixture

`APP-CONVENTION-SHARE` §4 makes the **namespace** the publisher, so a reader is handed it rather
than reading a `from` field out of the body. `EXPECTED.json` states a `publisher` only so the
`decodes_to.from` row has a value, and so a seat that *does* read a `from` out of the body has
something to disagree with.

`entitysdk/testdata/crossimpl-rust-feed/` is the mirror image: `FEED-R1` puts the author **in** the
bytes, so that fixture pins an Ed25519 seed. **Two conventions, opposite answers on one axis.** Know
which one you are in before designing anything against either.

## What it found on arrival

**Our `audience` encoding was wrong and had been since the type was written.** §2.2's
`audience: [* audience-entry]` and §2.3's `audience-entry = { type: "app/share/audience-entry",
data: {...} }` make each element a **full entity map**; we emitted the bare `data` map. The
convention distinguishes the two shapes one CDDL block apart — `share-target`'s arms are bare inline
maps with no `type` key, `audience-entry` is not — so this is our non-conformance rather than a
cohort disagreement, and it was corrected here rather than routed. See `share_crossimpl_test.go`.
