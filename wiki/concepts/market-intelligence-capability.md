---
type: concept
name: Market Intelligence Capability
date: 2026-10-11
updated: 2026-10-11
tags: [intelligence, data-pipeline, curation, taxonomy, entity-resolution]
sources: [source-gap-analysis-plan]
status: candidate
---

# Market Intelligence Capability (gather → curate → store)

**Report, 2026-10-11.** How Kubona could gather, curate, and store ecosystem
intelligence (companies, investors, funding, papers, patents, programs) on the
model of The Quantum Insider (https://thequantuminsider.com/about/), and what
to build, borrow, and refuse. Live pages studied: `/about/`, `/editorial-standards/`,
`/data/`, `/datasets/*`. This page proposes; nothing here is decided.

## 1. What TQI actually is (findings)

Four functionally-separated pillars under Resonance Alliance Inc. (Toronto;
vertical-intelligence parent also running AI and space hubs):

1. **News & Editorial** — daily reporting (computing/networking/sensing;
   capital markets, policy, business, breakthroughs), thousands of original
   articles. Editorial team is **functionally separate** from client-facing
   advisory/marketing; AI may assist but a **named human editor reviews
   everything**, with AI-use disclosure.
2. **Intelligence Platform** — proprietary database: 2,100+ companies,
   2,000+ investors, 1,300+ funding rounds, 600+ research groups, patents,
   programs, end users; queryable web app + **graph explorer** + API, daily updates.
3. **Packaged datasets** — single verified tables (sales deals since 2013;
   government funding, 657 program lines / 63 nations / $40.9B confirmed;
   market sizing with scenarios) as Excel workbook + PDF briefing, fixed
   price, versioned with **update logs and methodology sheets**, per-row sources.
4. **Advisory** — ecosystem assessments for governments (UK, Canada, Denmark,
   Saudi Arabia, US states) built *on top of* the same data.

The pipeline (`/data/` "How the data is built") is the transferable core:

1. **Source ingestion** — tens of thousands of sources monitored continuously:
   company announcements, regulatory filings, patent databases, peer-reviewed
   research, procurement portals, direct company disclosures.
2. **AI processing** — the Resonance Intelligence Engine extracts, deduplicates,
   and structures *candidate* records. **It publishes nothing on its own.**
3. **Analyst validation** — specialists check every material record against its
   primary source; ambiguous claims flagged or excluded; financials
   cross-referenced against ≥1 independent source.
4. **Taxonomy layer** — records classified against a house taxonomy refined
   since 2019, enabling segmentation and comparison.
5. **Structured output** — what survives becomes tables with **provenance
   intact (a source on every row)**.

Trust machinery: public editorial standards, corrections/updates/
clarifications/retractions policy (original URLs preserved, no silent edits),
schema.org `NewsMediaOrganization` + `Dataset` structured data, named editors,
primary-source linking, anonymous-source corroboration rule.

## 2. What Kubona already has (no new code needed)

| TQI stage | Kubona equivalent (prototype or adopted design) |
|---|---|
| Ingestion connectors | `integrations/sources.py` (arXiv/OpenAlex, rate-limited, license-flagged) + [[science-data-integrations]] Tier 1–2 catalog |
| AI extraction that publishes nothing | Admission gate + EvidenceTier T1/T2/T3 in `claim_grammar.py`: machine output is a *candidate* until verified |
| Analyst validation | Human approval gate ([[cost-governance]]), reviewer key (`KUBONA_REVIEWER_PRIVKEY_HEX`), tribunal audit in [[claim-credibility-wavefunctions]] |
| Provenance on every row | Ledger (`provenance.py`): every fetch writes `{source, query, license, fetched_at, freshness}`; [[provenance-and-reproducibility]] extends the record to hypothesis→citations |
| Taxonomy | `provider-capability-schema` (providers) + QIR taxonomy (circuits); no ecosystem taxonomy yet — the gap |
| Structured outputs | Results Studio / Observatory surfaces; conformance tables; no versioned dataset product — the gap |
| Correction discipline | `> ⚑ Correction:` blocks in wiki; no equivalent in served data yet — the gap |

## 3. Proposed design: the intelligence loop as a Kubona subsystem

Treat ecosystem intelligence as a **governed data pipeline behind the same
rail as everything else** — five stages mirroring §1, each mapped to existing
machinery:

- **Gather.** New `integrations/` connectors per source class, each with the
  Tier-1 treatment: permission table row (fetch/cache/quote/redistribute),
  rate limits in code, TOU flags into the ledger. Source classes in priority
  order: (a) scholarly graph (OpenAlex/CORE — done/adjacent), (b) patent
  full-text (Lens/EPO OPS terms review), (c) procurement portals + regulatory
  filings (jurisdiction-by-jurisdiction permission rows), (d) company
  announcements via feeds — **never by scraping TQI or any outlet itself**
  (licence gate Q-26; primary sources only).
- **Curate.** Two-step exactly as TQI: machine extraction → candidate records
  (EvidenceTier T3-by-default, quarantined corpus pattern from `provenance`),
  then human/analyst validation promoting T3→T2→T1 with reviewer signature.
  Entity resolution on the dedup literature: blocking→match→cluster with
  verified-merge discipline (arXiv:2508.03767 MERAI-scale pipeline;
  arXiv:2607.26298 lessons: domain knowledge in SOPs not weights, verified
  merges, sparsity-aware thresholds); OpenCTI-style deterministic-ID upsert
  (name-or-alias + class) is the simpler starting point already compatible
  with the ledger's content-addressing.
- **Store.** Entities (Company, Investor, Round, Group, Patent, Program,
  Person) + relationships (invested-in, partnered, published, cited) in the
  existing store (SQLite WAL → per-tenant sharding decision pending), every
  row carrying `source_url + fetched_at + license + validator + tier`.
  Versioned snapshots with **update logs + methodology notes** (the TQI
  dataset discipline) so any claim cites *a version*, never "the database".
- **Serve.** MCP resources first (agents query intelligence inside the
  discovery loop — funder landscape for [[science-discovery]] proposals,
  prior-art for [[autonomous-manuscript-synthesis-and-peer-review]]), console
  surface second (graph-explorer analog over the entity graph; the ledger
  already stores relationships). API/dataset packaging is a business-model
  decision, not an engineering one (see §5).
- **Correct.** Corrections policy ported from TQI: no silent edits —
  superseding records link predecessors, retractions preserve the ID with a
  notice (same shape as wiki `⚑ Correction:` blocks, enforced in data).

## 4. Phased build (simulator-buildable first)

- **Now (no accounts, no spend):** entity schema + deterministic-ID upsert +
  tier/quarantine promotion path, stub-tested; taxonomy v0 for quantum
  modalities reusing the provider-capability vocabulary; snapshot + update-log
  mechanism on the existing store. **Built 2026-10-11:** `intelligence/entities.py`
  (7 kinds, 5 relationship kinds, T3-default upsert, T2/T1 promotion ladder with
  independent-source rule, alias-conflict refusal, chained corrections,
  id-preserving retractions, citable snapshots; 8 tests green) — the prototype
  demonstrates the mechanism on stub data, per the register rule.
- **Built 2026-10-11 (taxonomy v0):** `intelligence/taxonomy.py` — technology
  axis reuses the provider `Modality` enum verbatim (tags and routing vocabulary
  cannot drift), domain/segment/stage controlled vocabularies, deterministic
  keyword `suggest()` that returns UNCLASSIFIED on ties/misses instead of
  guessing, validator-named `tag_entity()`; 4 tests green.
- **Next (keys/approvals):** one new connector at a time with permission rows
  (patents first — highest claim value for prior art); ER pipeline on the
  company/paper corpus behind the static gate. **Built 2026-10-11 (patents
  first):** `intelligence/patents.py` — EPO OPS seam with terms verified
  against epo.org (permission row on [[science-data-integrations]]),
  OAuth-from-env, 6 s rate floor, 4 GB/week quota refuse, unparseable-refuse,
  licence flag into ledger; 4 stub tests green. Live use gated on operator
  registration.
- **Gated (operator/business-model):** any dataset *product*, any API serving
  third parties, any paid source — needs the business-model sign-off already
  listed as blocked, plus per-source licence review.

## 5. What to refuse

- **Scraping intelligence outlets** (TQI or others) as sources — primary
  sources only; Q-26 licence gate applies to data exactly as to code.
- **Machine-published records** — the engine proposes, a validator disposes;
  TQI's "publishes nothing on its own" is the rule to copy most literally.
- **A marketing/advisory function inside the same trust boundary** — TQI's
  functional separation exists because intelligence that also sells loses
  credibility; Kubona's analyst/validator role must stay independent of any
  future commercial packaging.
- **Claiming coverage** ("2,000+ companies") without the validation receipts —
  [[quantum-advantage-realism]] applies to data claims too: record the source,
  always keep the per-row citation, never claim completeness.

Related: [[science-data-integrations]] · [[provenance-and-reproducibility]] ·
[[claim-credibility-wavefunctions]] · [[integration-topology-and-licence-discipline]] ·
[[provider-capability-schema]] · [[science-discovery]] · [[cost-governance]]
