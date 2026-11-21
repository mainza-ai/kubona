---
name: kubona-research
description: Orchestrate Kubona's agentic science lifecycle — hypotheses, Elo tournaments, research graphs, lab negative-result knowledge, persistent memory, manuscript verification, RO-Crate/PROV exports, and Lean 4 proofs.
---

# kubona-research: Hypotheses, Memory, Manuscripts & Proofs

Version 2026-11-20. Use this skill for multi-turn scientific campaigns that
propose hypotheses, accumulate evidence, verify manuscripts, and export
reproducible publication bundles.

## 1. Hypotheses & Elo Tournaments
- List existing hypotheses: `list_hypotheses` (`GET /api/sessions/{sessionId}/hypotheses`).
- Propose a candidate: `propose_hypothesis` (`POST /api/sessions/{sessionId}/hypotheses`)
  with a falsifiable `statement` and `rationale`.
- Rank candidates: `POST /api/sessions/{sessionId}/hypotheses/tournament`.
- **Evidence-gated collapse:** Collapsing a hypothesis
  (`POST /api/sessions/{sessionId}/hypotheses/{id}/collapse`) requires a
  completed accepted plan or verified claim — otherwise it refuses with `409`
  `hypothesis_not_evidenced`.
- **Shortlist before planning:** Planning from an unshortlisted hypothesis
  refuses with `409` `hypothesis_not_shortlisted`.

## 2. Resumable Research Graph & Lab Knowledge
- Call `get_research_graph` (`GET /api/sessions/{sessionId}/research-graph`)
  with `since_seq=0` initially, then pass the returned cursor to resume
  incrementally within the byte budget.
- Check tenant negative results before repeating failed setups via
  `GET /api/sessions/{sessionId}/lab-knowledge{qs}`. Repeating a known
  negative result fails closed with `409` `prior_negative_exists` unless
  explicitly acknowledged or revised.

## 3. Persistent Scientific Memory
- **Recall before proposing:** Call `recall_memory` (`GET /api/memory/recall`)
  with `session_id`, `query`, and optional `bank` (`world-facts`, etc.).
- **Retain only measured facts:** Call `retain_memory` (`POST /api/memory/retain`)
  only for verified observations (cite job/record IDs in `content`).
- **Retract superseded claims:** Operators retract stale memories via
  `POST /api/memory/retract`.

## 4. Provenance Ledger (`provenance` server)
- `write_record` appends a content-addressed record to the hash-chained ledger.
- `query_records` reads tenant-scoped records. Absence of rows under
  `requester=None` only reflects public rows — always pass the session/tenant.

## 5. Manuscripts, Numeric/Citation Verification & RO-Crate Export
1. Draft from a session with a campaign projection (`POST /api/sessions/{sessionId}/manuscripts`;
   fails with `409` `campaign_missing` if no experiment ran).
2. **Verify before signoff:** Call `verify_manuscript`
   (`POST /api/sessions/{sessionId}/manuscripts/{id}/verify`). Fabricated
   numbers or dead citations block signoff with `409`
   `evidence_verification_failed`.
3. Resolve all open annotations (`409` `annotations_open`), obtain an `ACCEPT`
   review (`409` `review_not_accepted`), build (`POST .../build`), and record
   operator Ed25519 signoff (`POST .../signoff`) before deposit staging.
4. Export reproducible RO-Crate 1.1 + W3C PROV-JSON + CRediT bundles via
   `GET /api/sessions/{sessionId}/manuscripts/{id}/export` or
   `GET /api/sessions/{sessionId}/experiment-export`.

## 6. Machine-Checked Proofs (Lean 4 Comparator)
- Only kernel-accepted Lean 4 solutions (0 `sorryAx`, signed by the pinned
  comparator key) are attested as `live_kernel_proven`. Unconfigured hosts
  refuse with `503` `comparator_unconfigured`.
