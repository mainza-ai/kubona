# Kubona Wiki — Maintainer Instructions (Schema)

> **Project name:** Kubona — Chitonga (Tonga, Zambia), *ku-bona*, "to see"
> (infinitive *ku-* + *-bona* "see, observe, perceive"). The repo/folder path `qagent/` is
> retained for stability.

This folder implements the "LLM Wiki" pattern. You are the permanent maintainer of
everything under `wiki/`. Read this file at the start of every session.

**What this wiki is for.** It is the **blueprint** for Kubona: the place where insight is
gathered, options compared, and decisions recorded *so that the platform can be shaped
correctly*. The code is not the point of the wiki and its state is not the state of the
design — `platform/` is a prototype scaffold written before most insights existed, and a
passing test there answers "is this mechanism feasible", never "is this the right design".
The work-list is `wiki/questions.md`, not a task tracker.

**Repo status (2026-10-26):** the repo lives at github.com/mainza-ai/kubona and is **public**
(GitHub API `private:false`). The earlier "private by decision" posture (Q-25) is superseded by
the actual visibility flip; the mechanical precondition (`dev-docs/` + `.obsidian/` residue in
history) was excised via `git filter-repo` on 2026-10-26 (Q-27 closed). Going public did **not**
make the platform production-ready: the Round-2 audit
([[production-readiness-audit-2026-10-26]]) found ten P0 blockers, and all production-readiness decisions (Q-49..Q-57) were resolved 2026-10-27; the implementation residuals are tracked in [[production-readiness-audit-2026-10-26]]. Do not cite "private" or "production-ready" as current fact.

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
  `ci.yml`'s `python-tests` job. Honest history: the pipeline was green 2026-09-29..2026-10-17,
  then red at `da800ea` (Python catalog, console lint, Docker boot, and two thesis-scale slices);
  branch protection was off. The 2026-10-26 Round-2 pass repaired the code-level causes and moved
  the thesis-scale slices to nightly. **Do not claim "green since 2026-09-29"** — claim green only
  after a full protected run, and name the run id. The lint is text-only over `wiki/**`.

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

## Current state (2026-10-26)

**Purpose:** the wiki is Kubona's **blueprint** — insight gathered so the platform can be
shaped before it is implemented. Implementation is held strictly to the 2026-10-12 Production
Bar (real logic or fail-closed refusal, no theater, line audit before landing).

**Contents:** 10 source pages (see `wiki/log.md`), 153 pages — overview/roadmap/questions/
index/log, 47 entities, 91 concepts. A 2026-10-26 Round-2 audit read all 831 in-scope
source files across Go core, Rust crates, Python runtime, Bridge, Console, Contracts,
Docker, and CI. Its verdict is **not production-ready**: ten P0 blockers remain at trust
boundaries (bridge auth, import forgery, sandbox escape, spend settlement, release gate,
scientific anchors, verification theater). The audit and its phased plan live in
[[production-readiness-audit-2026-10-26]]; prior "production polish" claims are superseded.

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

**Hygiene (tracked in [[risks]]):** the `ghp_…` PAT in a scratch clone is verified dead (401);
`credential.helper=store` was dropped globally (macOS keychain / `gh auth git-credential` only);
historical `dev-docs/` and `.obsidian/` residue was completely excised via `git filter-repo` on 2026-10-26 (Q-27 closed). Citation provenance (URL + SHA + line) must stay honest. The audited own-repos
(`chickensoup`, `milimoquantum`, `Milimo-QDD`) are **insight material, not supply** — unfinished
personal WIP mined for method; there is no licence task, only a release-time check if a derived
artefact ever ships inside the Apache-2.0 core. Prototype note: the hosted `ci.yml` push pipeline had **never**
passed (all 17 recorded runs failed) while `scripts/ci.sh` was green locally — fixed 2026-09-29,
first green in the pipeline's history, by collapsing the duplicated gate rule into one function
(`release_gate()`) that both CI paths call; that fix is also what unblocked the wiki lint there.
The same investigation found CI triggering only on `main`, which this repo advances by
fast-forwarding develop onto it — so no commit was ever gated as it landed, and `develop` +
`workflow_dispatch` were added as triggers. See [[implementation-gaps]].
