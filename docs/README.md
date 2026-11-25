# Kubona documentation

This folder holds **current operator and developer documentation** for the
application. It describes how the shipped system is configured, operated,
released, and verified — not the history of how it got there.

## Policy

- **Evergreen only.** A page here must describe behaviour that exists on the
  current `main`/`develop` tree. Dated audits, plans, research notes, and
  point-in-time status reports are not kept here.
- **History lives in git and the wiki.** Dated records removed in the
  2026-11-24 cleanup (production audits, accuracy/performance/memory
  investigations, publication-pipeline and agentic-mode records, PAINS/Live-QPU
  acceptance runs, stale TODO lists) remain in `git log -- docs/` and, where a
  durable claim matters, in [`wiki/`](../wiki/index.md).
- **Generated files are not hand-edited.** `repo-counts.md` is written by
  `scripts/repo-counts.py --write`; CI fails with `--check` when it is stale.
- **`dev-docs/` is not part of this folder.** It is gitignored local scratch and
  never ships.

## Operator guides

| Page | Covers |
|---|---|
| [`local-deployment.md`](local-deployment.md) | Supported local-private deployment: Compose topology, `.env`, loopback bindings, UDS bridge, migrations, preflight, go-live checklist |
| [`backup-restore.md`](backup-restore.md) | Ledger backup, verification, restore drills, scheduled profile, encryption posture |
| [`release.md`](release.md) | Signed-tag release flow, GHCR images, cosign/SBOM verification, dry run |
| [`lean-comparator-runbook-2026-11-18.md`](lean-comparator-runbook-2026-11-18.md) | Pinned Lean comparator sandbox: image pins, host requirements, live `live_kernel_proven` attestation |

## Runtime and contracts

| Page | Covers |
|---|---|
| [`rate-cards.md`](rate-cards.md) | Vendor-derived runtime rate card, stale-card refusal, `kubonad rates-audit`, reconciliation checklist |
| [`llm-proposer.md`](llm-proposer.md) | Circuit proposer providers (local Strata / BYOK NVIDIA), gate adapter, air-gap and billing posture |
| [`provenance-spool.md`](provenance-spool.md) | Receipt spool for irreversible side effects, replay semantics, operator backup notes |
| [`error-mitigation.md`](error-mitigation.md) | ZNE axis invariants, per-scale retention, adequacy/difference gates, offline verification |

## Engineering hygiene

| Page | Covers |
|---|---|
| [`repo-hygiene.md`](repo-hygiene.md) | Dead-export lint, intentional large assets, console design/bundle ratchets, coverage floors |
| [`repo-counts.md`](repo-counts.md) | Generated route/package/test/wiki/contract counts (source of truth for numbers in prose) |

## Where else to look

- `README.md` (repo root) — product overview and gates.
- [`wiki/overview.md`](../wiki/overview.md) — living blueprint, decisions, open questions.
- `SECURITY.md` / `CONTRIBUTING.md` — threat model and contribution rules.
- `platform/contracts/` — OpenAPI and JSON contracts (generated/validated in CI).

## Removed in the 2026-11-24 cleanup

These dated records were removed because they described superseded states. They
remain readable at `git show 51e7da24:docs/<file>` (the commit immediately before
the cleanup), and durable conclusions were folded into `wiki/`.

| Removed file | Was |
|---|---|
| `accuracy-gap-investigation-2026-11-18.md` | TFIM/QAOA/H₂ accuracy root-cause investigation |
| `active-implementation-todos.md` | local execution list at `fe31d1af` |
| `agentic-mode-toggle-2026-11-18.md` | agentic-mode design and AG-17…AG-24 plan |
| `agentic-science-frontier-2026-11-18.md` | 2026 agentic-science frontier survey |
| `core-scientific-capabilities-2026-11-09.md` | capability audit at `386f857` |
| `full-space-chemistry-2026-11-08.md` | ADAPT-VQE full-space record |
| `live-qpu-2026-11-09.md` | first live QPU acceptance run |
| `manual-accessibility-2026-11-01.md` | manual a11y record (not run) |
| `memory-management-research-2026-11-18.md` | MEM-* research program |
| `operator-evidence-2026-11-18.md` | external-evidence register |
| `paid-e2e-readiness-2026-10-31.md` | paid-E2E readiness report |
| `performance-research-2026-11-18.md` | PERF-* research program |
| `placeholder-audit-2026-11-02.md` | placeholder/fake-data audit |
| `production-audit-2026-11-09*.md`, `-11-18`, `-11-20` | dated production audits |
| `production-completion-plan-2026-11-08.md` | completion plan |
| `production-gap-audit-2026-11-09.md` | gap audit |
| `production-readiness-plan-2026-10-31.md` | readiness plan |
| `production-unblock-plan-2026-11-08.md` | unblock plan |
| `publication-followup-audit-2026-11-18.md` | publication follow-up audit |
| `publication-pipeline-2026-11-18.md` | publication contract as of that date |
| `real-experiment-production-audit-2026-11-08.md` | real-experiment audit |
| `remaining-todos.md` | remaining-work list |
| `results-presentation-audit-2026-11-08.md` | results-presentation audit |
| `sandbox-live-2026-11-09.md` | sandbox live run |
| `ui-gap-audit-2026-11-09.md`, `ux-access-audit-2026-11-02.md` | UI/UX audits |
| `vendor-evidence-2026-10-31.md` | vendor-evidence register |

> Historical `docs/<name>.md` citations inside `wiki/` (and older issues) refer
> to this table; a few of them (`docs/api.md`, `docs/storage.md`, …) never
> existed and are stale pointers, not files removed here.

