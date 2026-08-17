---
type: concept
name: Science Data Integrations
date: 2026-08-16
tags: [data, science, literature, apis]
sources: [source-gap-analysis-plan]
status: living
---

# Science Data Integrations (literature + domain data)

Maintainer synthesis. A discovery platform needs grounding: literature, prior art, and
domain datasets as first-class inputs to the discovery loop ([[science-discovery]]). This
page shortlists candidates from the public-apis catalog (public-apis/public-apis) most
relevant to Kubona, ranked by leverage.

## Tier 1 — literature & scholarly grounding

| API | What it gives Kubona |
|---|---|
| **arXiv** (no auth) | Full-text preprints for physics/quantum/compsci — the ground-truth corpus for prior-art search and hypothesis grounding |
| **OpenAlex** (no auth) | Open scholarly graph (works, authors, institutions, concepts, citations) — citation/prior-art mapping for [[arxiv-mcp-quantum]]-style checks |
| **CORE** (key) | Open-access full text beyond arXiv (journals, repositories) |
| **NASA ADS** (OAuth) | Astrophysics literature + data links |
| **Open Science Framework** | Study designs, preregistration, research materials — the reproducibility/pre-registration substrate |
| **SHARE** | Research-activity dataset (grants, papers, datasets) |

## Tier 2 — domain data for experiments

| API | Domain |
|---|---|
| **Materials Platform for Data Science (MPDS)** | Curated experimental materials data — pairs directly with VQE/molecular-simulation workflows |
| **GBIF / iDigBio / ITIS** | Biodiversity & specimens — biology discovery workflows |
| **NASA / ISRO / Launch Library** | Space & astronomy observation data |
| **Purple Air** | Environmental/air-quality time series |

## Integration pattern

- Each integration is an **MCP tool or resource** behind the same governance rail:
  rate limits, TOU/license flags, cost metering where paid, and **provenance of fetched
  data into the ledger** (source URL, fetch time, license) so every claim in a report
  traces to a citation — consistent with [[provenance-and-reproducibility]].
- Freshness/trust scoring applies to data sources exactly as it does to
  [[provider-capability-schema|providers]] — "certify before you trust it".
- Phase 6 scope ([[roadmap]]); Tier-1 arXiv/OpenAlex are the first two candidates.

## Model & inference infrastructure (supporting)

**Hugging Face** (model hub + inference), **Jina AI** (embeddings/reranking for grounded
RAG), **Groq** (fast inference) — operational choices for grounding pipelines and
[[unsloth]]-served models, not dependencies.
