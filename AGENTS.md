# Kubona Wiki — Maintainer Instructions (Schema)

> **Project name:** Kubona (Tonga (Zambia) — *to see*). The repo/folder path `qagent/` is
> retained for stability.

This folder implements the "LLM Wiki" pattern. You are the permanent maintainer of
everything under `wiki/`. Read this file at the start of every session.

**What this wiki is for.** It is the **blueprint** for Kubona: the place where insight is
gathered, options compared, and decisions recorded *so that the platform can be shaped
correctly*. The code is not the point of the wiki and its state is not the state of the
design — `platform/` is a prototype scaffold written before most insights existed, and a
passing test there answers "is this mechanism feasible", never "is this the right design".
The work-list is `wiki/questions.md`, not a task tracker.

**Repo status (2026-09-29):** the repo lives at github.com/mainza-ai/kubona and is **private by
decision**: while the blueprint is still being designed, going public would invite contributions
the project has no settled direction to direct them into. Publication is gated on the blueprint
becoming **contributor-legible** — Q-01 answered (what `platform/` is for) plus enough of the
method layer to say what to build first — *not* on the Phase-0 gate, which passed weeks ago, and
not on a date. The old "public once Phase 0 passes" trigger is superseded: Q-25 is closed, and
the mechanical precondition (`dev-docs/` + `.obsidian/` residue in private-remote history — the repo has never been public) is Q-27.

## Three layers

- `dev-docs/` — local working copies of source documents. **Never committed to git, never pushed —
  ever** (see `.gitignore`; enforced by a local `.git/hooks/pre-commit` guard that refuses any staged
  `dev-docs/` or `.obsidian/` path). Only `raw/` is ingested and versioned. Treat every `dev-docs/…`
  path as unresolvable for anyone reading the public repo — cite the live URL instead.
- `raw/` — immutable source documents. **Read-only. Never edit.** You ingest from here.
- `wiki/` — LLM-owned markdown. You create, update, and cross-link everything here.
- `AGENTS.md` — this file. The operating rules. Co-evolve it with the human.

## Directory conventions

- `wiki/entities/` — named things: platforms, providers, tools, orgs, people.
- `wiki/concepts/` — mechanisms, ideas, and recurring patterns.
- `wiki/sources/` — one summary page per ingested source (linked as an entity).
- `wiki/overview.md` — the synthesis: "what Kubona is and where the design stands."
- `wiki/questions.md` — **the work-list**: ranked open questions and design forks. Every
  `status: open-question` page must have a row here. This is where a session starts.
- `wiki/roadmap.md` — phases read as **acceptance criteria**, mirrored to sources; a gate
  proven only by a prototype test is marked as such, not as a milestone.
- `wiki/index.md` — catalog of every page + the layer map + the status legend.
- `wiki/log.md` — append-only chronological record with parseable entries:
  `## [YYYY-MM-DD] ingest | <title>` (also `question |`, `verification |`, `decision |`,
  `cleanup |`)

## Page conventions

- YAML frontmatter on every page: `type`, `tags`, `date`, `updated`, `status`, `sources`.
- `status` describes **how decided** something is — never how finished. Controlled
  vocabulary (full definitions in `wiki/index.md` §Status legend):
  `settled` · `adopted` · `open-question` · `candidate` · `tracked` · `reference` ·
  `source` · `living`. Do not invent new values; if a page seems to need one, that is a
  sign the taxonomy is wrong — raise it here instead.
- `sources` lists **canonical source-page basenames only**
  (`source-architecture-paper`, `source-gap-analysis-plan`, `source-cloudflare-economy`,
  `source-frontier-repo-audit-2026`). Inline prose citations use the same keys, e.g.
  `(source-architecture-paper)`. `(repo-audit-2026)` is the accepted short form of
  `source-frontier-repo-audit-2026` and marks a weaker evidence class.
- **Exemptions:** `type: source` pages do not list `sources` (they *are* sources) — they carry
  `raw:` (the immutable document path they summarise) or `origin:` (why no `raw/` document
  exists). `index.md` and `log.md` are meta and carry no `sources`.
- Use `[[wikilinks]]` for cross-references (Obsidian-compatible). Link to the page
  basename, e.g. `[[qiskit]]` resolves to `entities/qiskit.md`.
- Keep pages concise, precise, and factual. Every claim must trace to a source.
- **Separate the two registers.** A sentence about what the *design* says and a sentence
  about what the *prototype* does must be distinguishable without reading twice. Prefer
  "the plan specifies X" / "the prototype demonstrates X, on a simulator" over a bare
  "X is implemented".
- **Cite repositories by live URL, never by clone path.** Write `owner/repo`
  (→ `https://github.com/owner/repo`) plus the HEAD date/SHA and a `path:line` anchor.
  `dev-docs/` clones (including `dev-docs/submodules/`, which are read mirrors of live GitHub URLs)
  are scratch, gitignored and deleted when development ends — so a `dev-docs/…` path can never be
  the durable evidence for a claim.
- When re-verification contradicts an earlier claim on a page, keep the superseded claim visible in a
  `> ⚑ Correction:` block rather than silently deleting it, and link the source that settled it.
- On contradiction between sources: keep both claims, flag with a
  `> ⚑ Conflict:` warning block, and link both sources. Never silently pick one.
- Update `overview.md` whenever an ingest changes the overall picture.

## Workflows

### Ingest
1. Move the new document into `raw/` verbatim (never altered).
2. Read it fully.
3. Write a `wiki/sources/` summary page.
4. Create/update the entity + concept pages it touches (typically 10–15 pages). Assign each
   a `status` from the controlled vocabulary and place it on the `index.md` layer map.
5. **Update `questions.md`** — every insight narrows some question and opens others. An
   ingest that changes no question and closes none was probably not worth ingesting.
6. Refresh `overview.md` and, if the plan changed, `roadmap.md`.
7. Update `index.md`.
8. Append to `log.md`: `## [YYYY-MM-DD] ingest | <title>`.

### Question closed
1. Write the answer on the concept page it belongs to (not in `questions.md`).
2. Flip that page's `status` → `adopted`, or `settled` if it is now a constraint others obey.
3. Move the row in `questions.md` to `## Closed` with date + one-line answer + link.
4. Append `## [YYYY-MM-DD] question | Q-NN — <one line>` to `log.md`.

### Query
- Answer questions against wiki pages, citing sources. When the answer is a durable
  artifact (a comparison, an analysis, a connection), file it as a new page and index it
  so explorations compound instead of disappearing into chat history.

### Lint (periodic)
- Check for: contradictions between pages, stale claims superseded by newer sources,
  orphan pages with no inbound links, important concepts lacking a page, missing
  cross-references, and data gaps that a web search could fill. Report findings as a
  list; fix low-risk items directly.
- **Register drift:** any page that describes prototype code in design language ("X is
  implemented", "we have X") without saying it is the prototype. This is the failure mode
  most likely to make the blueprint lie.
- **Machine-verifiable checks:** `python3 scripts/wiki-lint.py --strict` (repo root). Covers
  frontmatter fields, `status` inside the controlled vocabulary, canonical `sources` keys,
  dangling wikilinks, pages absent from `index.md`/`questions.md`, `type: source` pages
  missing `raw:`/`origin:`, and `path:line` anchors that resolve into `dev-docs/` scratch
  instead of a live URL. Clean (exit 0) as of 2026-09-27; checks were negative-tested against
  a deliberately broken page. **Wired into CI on 2026-09-27** as a *Wiki structural lint* step in
  `ci.yml`'s `python-tests` job, green in CI since 2026-09-29 (run 36508507064) — added only after
  the release-gate bug was fixed, because a step in a pipeline that already failed 17/17 adds no
  signal. It is text-only over `wiki/**`, so it needs neither `dev-docs/` nor the toolchain.

## Production bar (settled 2026-10-12)

Kubona is built for production, not demo. Simplification now means redo
later, which is not productive — so every implementation is held to this
bar before it lands. "Works" is necessary and insufficient.

- **Real logic or fail-closed refusal.** No invented `success:true`, no
  arithmetic standing in for measurement, no verification without a
  measured result. Thresholds are v1 heuristics and say so.
- **No scope-dodging.** A fix that leaves the same fiction class alive in
  an adjacent branch is incomplete (the notebooks template success path
  kept claiming an un-run VQE while its fallback branch was fixed — the
  line audit caught it). Fix the class, not the instance.
- **Threat analysis over consistency.** Every auth/gating change carries a
  threat argument: what breaks, for whom, reader-vs-writer
  classification. "Consistency" is never the argument (gating a pure
  read-only check broke the console display flow for zero gain — reverted
  in the audit). Reads stay public behind limiters; writes get edge auth.
- **No weakened tests.** A test updated for new behavior must assert the
  corrected behavior with equal or greater strength. Red going green by
  asserting less is falsification, not verification.
- **Drift-proof shared values.** Any literal a second file depends on
  (match/strip string pairs, weight sets, source labels) lives in one
  shared const — never two copies that can rot apart.
- **Line audit before landing.** Re-read the full diff line by line before
  every slice lands: check edge cases, error paths, float-bit and
  ordering subtleties, and every caller that could break. Then prove it
  live in both directions wherever behavior branches, and record
  reversals in the wiki so nobody re-applies them.

## Current state (2026-09-29)

**Purpose:** the wiki is Kubona's **blueprint** — insight gathered so the platform can be
shaped before it is implemented. Implementation is deliberately *not* the current activity;
`platform/` is a prototype scaffold that pre-dates most of the insights here and is treated
as evidence about mechanisms, not as a decided design.

**Contents:** 4 sources ingested (see `wiki/log.md`), 82 pages — overview/roadmap/questions/
index/log, 4 source pages, 35 entities, 38 concepts. Decidedness: `settled` 1 · `adopted` 24 ·
`open-question` 13 · `candidate` 6 · `tracked` 24 · `reference` 5 · `source` 4 · `living` 5.
One `settled` page against 24 `adopted` is the honest signal: the blueprint is largely
*understood*, not yet *chosen*.

**Domain:** an open-source, MCP-native quantum-computing **and AI-enhanced science-discovery
platform** for autonomous AI agents. The fourth source, [[source-frontier-repo-audit-2026]],
audited 25 repositories (remote · HEAD · licence · `path:line`) and produced the two
cross-cutting policies now wired through the wiki:
[[integration-topology-and-licence-discipline]] (submodule / sidecar / blueprint / reference —
"submodule" is a shipping decision, not a compliment) and [[quantum-advantage-realism]]
(record the backend that ran, always compute a classical anchor, never claim advantage).

**Where the next layers should go:** [[questions]] sections A–B. The method/epistemics layer
(claim vocabulary, credibility, the loop model, meta-loop bounds) is the most differentiating
and the least grounded, and Q-01 — what the prototype is *for* — gates how every other
question may be answered.

**Hygiene (tracked in [[risks]]):** the `ghp_…` PAT in a scratch clone is verified dead (401),
but `credential.helper=store` leaves a **live** `gho_…` token in plaintext in
`~/.git-credentials` (revoke the stale grant, drop the `store` helper). `dev-docs/` and
`.obsidian/` sit in *private-remote history (never public)* although untracked today (no secrets found); the choice
between publishing as-is, rewriting with `filter-repo`, or starting a clean public history is
[[questions]] Q-27. Citation provenance (URL + SHA + line) must stay honest. The audited own-repos
(`chickensoup`, `milimoquantum`, `Milimo-QDD`) are **insight material, not supply** — unfinished
personal WIP mined for method; there is no licence task, only a release-time check if a derived
artefact ever ships inside the Apache-2.0 core. Prototype note: the hosted `ci.yml` push pipeline had **never**
passed (all 17 recorded runs failed) while `scripts/ci.sh` was green locally — fixed 2026-09-29,
first green in the pipeline's history, by collapsing the duplicated gate rule into one function
(`release_gate()`) that both CI paths call; that fix is also what unblocked the wiki lint there.
The same investigation found CI triggering only on `main`, which this repo advances by
fast-forwarding develop onto it — so no commit was ever gated as it landed, and `develop` +
`workflow_dispatch` were added as triggers. See [[implementation-gaps]].
