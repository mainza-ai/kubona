---
type: entity
name: mainza-consciousness (mainza-ai)
date: 2026-09-27
updated: 2026-09-27
tags: [prior-art, owner-repo, memory, caution, incident-source]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# mainza-consciousness — good memory code, bad quantum code, one dead secret

**https://github.com/mainza-ai/mainza-consciousness** · maintainer's own repo, MIT ·
HEAD 2025-09-30. Integration class: **Blueprint — own WIP, mined for ideas only; caution flag stands** because half
of it is theatre ([[integration-topology-and-licence-discipline]]).

This repo is the audit's best illustration of why *file-level* reading matters: the headline feature
is theatre, while the unglamorous module next to it is genuinely reusable.

- **Theatre**: what the code calls a quantum-consciousness computation is
  `np.random.dirichlet(np.ones(len(states)))`
  (`backend/utils/quantum_consciousness_engine.py:331`) — sampling a Dirichlet distribution and
  calling the result a quantum state. Nothing else in the module implements quantum mechanics
  ([[quantum-advantage-realism]]).
- **Reusable**: a real memory lifecycle in `backend/utils/advanced_memory_architecture.py` —
  `MemoryType` (:18) and `MemoryConsolidationLevel` (:29), `decay_rate=0.95` (:51),
  `decay_threshold=0.3` (:73), `create_episodic_memory` (:76), `consolidate_memories` (:273),
  `_calculate_consolidation_strength` (:431) with time decay `max(0.1, 1-0.01t)` (:441). Types,
  thresholds, and a decay curve you can unit-test — the shape Kubona needs for
  [[agentic-memory-and-observability]], where a memory that never expires is a claim that never
  gets re-checked.
- **Hygiene failures**: 6 checked-in `.py.bak` files, and a **plaintext `ghp_…` GitHub PAT inside
  the remote URL of `.git/config`** — the classic `git clone https://user:token@…` mistake. The clone
  is gitignored scratch and the token never entered a commit. Re-checked against the API on
  2026-09-27: **401, already revoked/expired**, so nothing is exposed and nothing needs revoking —
  the residue is the string in the URL, deleted with the folder.
  > ⚑ An earlier version of this page called that token "live; revoke immediately". Superseded by
  > the 2026-09-27 API check. The *live* credential on this machine is unrelated to this repo: a
  > `gho_…` token left in plaintext by `credential.helper=store` ([[secrets-and-workload-isolation]],
  > [[risks]]).

Cite the URL + SHA + line numbers; never a `dev-docs/…` path.