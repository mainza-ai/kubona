# Kubona Wiki — Maintainer Instructions (Schema)

> **Project name:** Kubona (Tonga (Zambia) — *to see*). The repo/folder path `qagent/` is
> retained for stability.

This folder implements the "LLM Wiki" pattern. You are the permanent maintainer of
everything under `wiki/`. Read this file at the start of every session.

**Repo status (2026-08-16):** initial repo lives at github.com/mainza-ai/kubona and stays
**private** until a working core exists (Phase-0 gate: simulator job end-to-end via MCP with
a provenance record). It will be made public, Apache-2.0, once that gate passes.

## Three layers

- `raw/` — immutable source documents. **Read-only. Never edit.** You ingest from here.
- `wiki/` — LLM-owned markdown. You create, update, and cross-link everything here.
- `AGENTS.md` — this file. The operating rules. Co-evolve it with the human.

## Directory conventions

- `wiki/entities/` — named things: platforms, providers, tools, orgs, people.
- `wiki/concepts/` — mechanisms, ideas, and recurring patterns.
- `wiki/sources/` — one summary page per ingested source (linked as an entity).
- `wiki/overview.md` — the synthesis: "what Kubona is and where it stands now."
- `wiki/roadmap.md` — the phased implementation plan, kept mirrored to sources.
- `wiki/index.md` — catalog of every page; updated on every ingest.
- `wiki/log.md` — append-only chronological record with parseable entries:
  `## [YYYY-MM-DD] ingest | <title>`

## Page conventions

- YAML frontmatter on every page: `type`, `tags`, `date`, `status`, `sources`.
- Use `[[wikilinks]]` for cross-references (Obsidian-compatible). Link to the page
  basename, e.g. `[[qiskit]]` resolves to `entities/qiskit.md`.
- Keep pages concise, precise, and factual. Every claim must trace to a source;
  cite inline as `(architecture-paper)`.
- On contradiction between sources: keep both claims, flag with a
  `> ⚑ Conflict:` warning block, and link both sources. Never silently pick one.
- Update `overview.md` whenever an ingest changes the overall picture.

## Workflows

### Ingest
1. Move the new document into `raw/` verbatim (never altered).
2. Read it fully.
3. Write a `wiki/sources/` summary page.
4. Create/update the entity + concept pages it touches (typically 10–15 pages).
5. Refresh `overview.md` and, if the plan changed, `roadmap.md`.
6. Update `index.md`.
7. Append to `log.md`: `## [YYYY-MM-DD] ingest | <title>`.

### Query
- Answer questions against wiki pages, citing sources. When the answer is a durable
  artifact (a comparison, an analysis, a connection), file it as a new page and index it
  so explorations compound instead of disappearing into chat history.

### Lint (periodic)
- Check for: contradictions between pages, stale claims superseded by newer sources,
  orphan pages with no inbound links, important concepts lacking a page, missing
  cross-references, and data gaps that a web search could fill. Report findings as a
  list; fix low-risk items directly.

## Current state (2026-08-16)

3 sources ingested (see `wiki/log.md`), 55 pages: overview/roadmap/index/log, 3 source
pages, 26 entities, 22 concepts. Domain: Kubona — an open-source, MCP-native
quantum-computing platform for autonomous AI agents; expanding toward an AI-enhanced
science-discovery platform (user/agent-created workflows & agents, autonomous, systematic
experiments — see wiki/concepts/science-discovery.md).