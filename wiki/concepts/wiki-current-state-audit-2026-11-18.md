---
type: concept
name: Wiki Current-State Audit 2026-11-18
date: 2026-11-18
updated: 2026-11-18
tags: [audit, wiki, maintenance, status, verification]
sources: [source-gap-analysis-plan, source-architecture-paper, source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026, source-openai-math-corpus, source-cloudflare-economy]
status: living
---

# Wiki Current-State Audit (2026-11-18)

**Scope.** Reconcile the blueprint with the implementation state after the
agentic-science and Lean-comparator work. All **166 pages** were read in seven
parallel batches, then every checkable claim was verified against code,
contracts, generated counts, git history, and live evidence; the pages that
assert current state were corrected. Structural lint plus targeted sweeps ran
over the whole tree.

## Method

- `python3 scripts/wiki-lint.py --strict` (frontmatter, vocabulary, links,
  index/questions coverage, `raw:`/`origin:`, `dev-docs` anchors).
- `python3 scripts/repo-counts.py --check` as the numeric source of truth
  (routes, packages, tests, contracts, pages).
- Sweeps for stale numerals (`811`, `142/144 modeled`, `141 routes`, `121`
  event rows, `14` palette occurrences, `54/55`, `46/202`, `schema 10–14`) and
  stale status phrases (`still open`, `not implemented`, `no live kernel`,
  `operator-built`, `private by decision`, `remains a follow-up`).
- Code spot-checks for claims that had changed: steer consumption, feedback
  reason/dismiss UI, notification preferences, Attention Director bridge,
  memory retraction, claim provenance, and the repo visibility API.

## Fixed drift

| Page | Was | Now |
|---|---|---|
| [[overview]] | latest snapshot 2026-11-14 (154 routes/41 pkgs/173 modeled) | 2026-11-18 snapshot: 156/48/1142, 191/16 contracts, schema 15, 11/13 benchmark, live Lean proof |
| [[roadmap]] | "private by decision"; 2026-11-10 gate counts | Public since 2026-10-26; 2026-11-18 gate update with current counts and the live proof |
| [[kubona]] (entity) | 2026-11-11 prototype block | 2026-11-18 prototype block (current counts/contracts/schema) above the historical blocks |
| [[remaining-work]] | "Still open" A-series; Lean toolchain; 54/55; 121 event rows; palette 14 | A-series closures; 50/51; 188 event types; palette 0; 2026-11-18 state update |
| [[open-backlog-2026-11-15]] | most P0/P1 rows open; 162/14 contracts | Verified closures (steer, reason UI, prefs, Attention Director, eval-live, skills, typed needs, provenance, proposals, hardware rails, plan drafts, obligation events, chart refs, runtime gates, motion); 50/51, 44/195, 0 palette |
| [[feedback-loops-and-notifications]] | "remaining: reason UIs, prefs, Attention Director" | Those landed; only the P2 learning/metrics consumer remains |
| [[runtime-honesty-and-commit-policy]] | "1 h escalation not implemented" | Implemented and marked in the tray (`needs-you.ts`, tested) |
| [[agentic-science-frontier-2026]] | single-start QAOA 0.891/0.932; HEA 0.244 | Multi-start 0.899/0.948; HEA 0.246; HVA-6 0.0027; schema 15 |
| [[implementation-gaps]] | 142/14 contracts; 121 event rows | 191/16 (2026-11-18); 188 shared event types |
| [[ui-ux-vision]], [[frontend-backend-ui-ux-audit]], [[platform-architecture-diagram]], [[audit-topology-map]], [[agent-ux-control-plane]] | 142/144 contracts, 141/151 routes, palette 14; Mermaid showed a legacy read mirror and F-001 "503 in prod" | 191/16 contracts, 156 routes, palette 0; Mermaid adds oversight/notification and Lean nodes, marks the shim unserved, and records F-001 resolved |
| [[production-readiness-audit-2026-11-18]] | sixth-pass "build/warm the Lean image" | Seventh pass: live kernel proof, attestation event 780, reviewer key and required-backup preflight confirmed |
| [[comparator]], [[lean4]], [[formal-proof-verification-lean-comparator]] | no official Hub image / no live proof | Tarball-built image, comparator/landrun pins, live `live_kernel_proven` run |

Also corrected: the pinned checkout has **405** challenge JSON files, not the
source document's 811 (retained with a correction in the source/entity pages);
the README now states 19 `experiment-result-v1` kinds (13 matrix capabilities)
and the current wiki/contract counts.

## Second pass (page-by-page, all 166 pages)

The batch read found additional current-state defects that the numeric sweep
could not see; all were verified in code before editing:

| Area | Was | Now |
|---|---|---|
| Paid-run label (3 pages) | "Paid `ibm_marrakesh` … `$3.20`" | Open Plan `pricing_type=free`, actual `$0.00`; `$3.20` was a pre-fix list-price estimate; paid evidence remains operator-owned |
| Publication pipeline | "No comparator producer, no ACCEPT path"; "annotations not persisted" | Live attestation accepted (event 780); annotations persist and gate signoff |
| Admin-key rotation (3 pages) | `RotateAdminKey` "immediately revokes all sessions" | `SetAdminKey` invalidates old-key admin actions; cookie sessions persist until logout/expiry/revoke (Q-56) |
| Epistemic curvature | "pure functions, golden-tested, emitted as ledger records" | Stateful engine with manifold tests; no golden corpus and no curvature event — ledger emission is planned |
| Audit remediation (2 pages) | "Nothing below is implemented/started" | Landed 2026-10-26; retained as the pre-fix deep dive |
| Deep implementation audit | Lean unbuilt; 5 servers; 37 console_api routes; no CI secret scan | Lean live proof; 6 servers; 22 unserved handlers; secret-scan job landed |
| Providers (2 pages) | "11+ providers" incl. Azure/Google | 10-provider closed vocabulary; Azure/Google are paper targets |
| HF hub | Python `mesh/hf_hub.py` as the live rail | Go `pkg/hf/hub.go` + console routes; Python module is legacy; runtime registration not wired |
| IBM probe | Wrong vendor job id + "cancelled while queued" | Vendor job `dbha28pq260s73busu70` `DONE/2 s`; local row stays cancelled/$0.00; $0 is the free-plan charge |
| World Labs Atlas | `[ILLUSTRATIVE_SKIN · ATLAS CHISEL]` | Shipped watermark `SYNTHETIC LENS — WORLD LABS ATLAS`; `dev_inspect` has no code path |
| Misc | 9 sources, 14 client-only, 48 questions A–E, 120 routes, 7 RPCs, UID 10001, `TailOverflowTripwire`, `useTelemetry` | 11 sources, 16 client-only, Q-01…Q-61 A–F, 156 routes, 19 RPCs, UID 1000, boolean `tail_overflow_tripped`, `useSessionStream` |

## Verified clean

- All six question sections are closed; no page carries
  `status: open-question` in frontmatter.
- Every page has valid frontmatter, a controlled `status`, canonical source
  keys, and resolves through `index.md`/`questions.md`.
- The status census is mechanical and matches the directory: **166 pages**
  (48 entities · 102 concepts · 11 sources · 5 meta).
- Historical audit pages and `log.md` were left as dated records; where they
  assert current state, a correction block points at the current page.

## Residual risks

- Dated snapshot blocks (2026-11-10/11/14) remain verbatim by design; they are
  superseded by the 2026-11-18 blocks above them.
- Operator-only facts (paid-QPU invoice, off-host key copy, fresh-volume reset,
  manual screen-reader pass, runtime rate card) are still not claimed.
- Re-run the full gate on the exact pushed commit before citing "green":
  generated counts and CI status are commit-bound.
