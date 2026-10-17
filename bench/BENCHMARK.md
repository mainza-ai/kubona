# Kubona bench — baseline (2026-10-17)

Hermetic behavior floors + MCP surface measurement. No LLM calls, no
network, no keys — deterministic, CI-safe. Keyed-model comparisons run
elsewhere (operator periodic); this harness refuses to fake them.

## Method

`./bench/run.py [--reps N] [--tasks id,...]` executes `bench/tasks.json`
in-process (live tree, state sandboxed to `bench/work/`, wiped per run):
gateway tool calls with `dev-token`, mandate crypto round-trips, ledger
append+validate. Each key is a regex graded against the JSON output;
results append to `bench/results/<utc>.jsonl`; exit nonzero on any fail.
`./bench/ctx.sh` measures the served surface (same measurer as the CI
gate) and fails past 16 KB.

Tribunal structure rides the existing Go suites (`TestExecuteTribunal*`,
`TestFallbackRunDeclaresUnobserved`, rubric separation) rather than a
duplicated harness — the bench documents them as the tribunal floor.

## Baseline numbers (2026-10-17, darwin arm64, hermetic)

| task | reps | wall_s (each) | result |
|---|---|---|---|
| provider-inventory | 3 | 0.000–0.001 | PASS |
| session-lifecycle (create→burn→close) | 3 | 0.000 | PASS |
| mandate-shape (valid + over-budget refusal) | 3 | 0.001 | PASS |
| ledger-tail (append + validate) | 3 | 0.000 | PASS |

MCP surface: 29 tools, 8720 bytes (~2180 tokens), budget 16384 — within.
Tribunal floor (Go suites, recorded): 4 turns/run, 2 LLM trips/turn,
coverage facets on every sealed turn, rubric fixture delta 0.88.

## Rerun (L2-rerun, post-L3, 2026-10-17)

4/4 tasks ×3 reps green; surface still 8720 bytes; Go tribunal floor
still green including the new fallback-tag test. Delta vs baseline:
zero across every cell — expected and correct, since L3 alters scribe
wording only (prompts + fallback template), nothing this harness
measures (dispatch shapes, timings, surface bytes, structural floors).
The rerun's value is the negative result on record: the tagging change
moved no needle it shouldn't have.

## Caveats (read before citing)

- These are deterministic floors, not agent comparisons. There is no
  skill/no-skill arm here — that needs keyed models and is queued.
- Wall times measure in-process dispatch, not model latency; tribunal
  economics (cost/turn, p95, model deltas) remain unmeasured (G6).
- `bench/work/` is wiped per run; results files are gitignored scratch
  (this file carries the published numbers, not the jsonl).
- Known contract wart found while keying: `list_provider_registry`
  returns a bare payload with no `ok` envelope (graded via
  `require_ok: false`). Follow-up candidate, not bench scope.

## Reproduce

```sh
./bench/run.py --reps 3
./bench/ctx.sh
go test ./pkg/agents/ -run 'TestExecuteTribunal|TestRubric|TestFallbackRunDeclares' -count=1
```
