# Entity System — fixture overview

Top-level fixture document used by the shell ingest e2e tests. It exists so the
tests drive a stable, self-contained tree instead of the repo's live `docs/`
(whose contents change, and which the publish pipeline filters to a canonical
subset — see RELEASE-CONVENTIONS / the canonical-docs filter).
