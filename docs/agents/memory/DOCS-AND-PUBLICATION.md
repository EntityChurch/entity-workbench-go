# Docs & publication

CANONICAL-DOCS.toml as a published artifact and a keep-list, and where project state actually lives.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **`CANONICAL-DOCS.toml` is a published artifact, not configuration** (AP42). It is the whole
  interface to the release pipeline — we declare, it publishes ([ADR-0031]) — and its `blurb` is the prose a
  public reader gets **instead of** the document. Two rules, both earned on 2026-08-24: **(1) a
  blurb describes, it does not count** — every numeric range in it had gone false (D1–D23 for
  D1–D24, AP1–AP27 for AP1–AP40, "six-boundary" for seven, P0–P6 for P0–P7) plus a `github =`
  org that does not exist, because a `.toml` gets read as config and skipped by review; **(2)
  declaring a doc is part of adding it** — `AGENTS.md` is public and told the reader to open
  `DOCTRINE-CRASH-FORENSICS.md`, which was undeclared, so the instruction shipped broken. If a
  diff changes a count a blurb restates, or adds a doc a published doc cites, the manifest is
  part of that diff. **(3) An omission in a keep-list is an act of DELETION** — undeclared means
  dropped, so a root doc that is already on public `master` and missing from this file gets
  *withdrawn* at the next release. That was live on 2026-08-24 for seven files including
  `SECURITY.md`. Review the manifest against **what is currently published**, not just against
  the tree — the two move independently, so the answer changes without this repo changing.
  Compare a filtered export of `dev` against the published remote (**never a local `master`** —
  it drifts from what is actually published and answers this question wrong). The procedure and
  the local tool paths belong in your git-ignored `AGENTS.local.md` / `.agents/`, not here.

  **(4) Declaring a doc changes what "internal" means about it.** `AGENTS.md` is written for us
  and is *published*, and internal infrastructure names are exactly what an internal-audience
  document is made of — several had to be rewritten out of this file on the day it was declared,
  **and more had to come out on 2026-08-25**, which is the part worth learning from. What
  survived the first pass was everything that read as *engineering*: another team's incident
  history, their process, packet identifiers, who miscounted what. It was all true and all
  useless to the reader it was being shipped to. **A machine-local or internal path belongs in
  the git-ignored `AGENTS.local.md` / `.agents/`** ([ADR-0020]), never here. Assume you
  **cannot** self-check this category in one pass — the second reading is the one that finds it,
  and the test is not "is this true" but "is this the reader's business."

  Note rule (5) below already said this and was written to be applied to `docs/STATUS.md`. It
  binds every declared file, this one included. **A rule stated in a document does not exempt
  that document.**

  **(5) The rolling log is about this project.** `docs/STATUS.md` publishes; it carries our tree
  state, our defects, our decisions. Coordination with other teams — their processes, their
  tooling, their internal state — is **theirs**, and goes in `docs/status/` or a `reviews/`
  packet, neither of which publishes. "It explains why we changed a file" is not an exception;
  say what we changed and why it is right for this repo.
- **Don't synthesize project state from `git log` or top-down code reading** — use the
  framework + latest status snapshot + the newest `docs/status/HANDOFF-*`. **There is no
  roadmap doc and there has never been one** — this file named a
  `REPOSITORY-WORKSPACE-ROADMAP.md` and `PHASE-*-PLAN.md` for months and neither exists, which
  sent every new session looking for a file to orient from. "What's next" lives in the newest
  handoff's recommended-order section and in `STATUS.md`'s "Waiting on"; one direction doc per
  topic covers the rest. Corrected 2026-08-20, by audit.
