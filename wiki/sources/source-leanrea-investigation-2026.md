---
name: leanrea investigation (Oct 2026)
type: source
tags: [audit, prior-art, skill, benchmark, mcp, reverse-engineering]
date: 2026-10-17
updated: 2026-10-17
status: source
origin: first-party maintainer inspection — live-repo read, no raw/ document (see Evidence class below)
---

# leanrea investigation — Oct 2026 (`source-leanrea-investigation-2026`)

**Evidence class.** This is *not* a `raw/` document. It is a first-party
maintainer read of the live public repo `aicodeking/leanrea` at HEAD
`09ba4aa` (2026-10-17), MIT licence, via the GitHub API (tree, blobs,
commit metadata). Nothing was cloned, vendored, or changed; no code
from the repo enters this codebase ([[integration-topology-and-licence-discipline]]:
reference class). Benchmark numbers below are *taken as reported* in
`bench/BENCHMARK.md` (methodology + caveats there) — not independently
reproduced. The durable analysis lives in [[leanrea-adoption-plan]];
the repo facts live in [[leanrea]].

**What was read, in full:** `leanrea/SKILL.md` (8 KB: Rules, First move,
Router × 17 targets, Speed patterns, Install), `README.md` (claims,
benchmark summary, install, credit), `bench/BENCHMARK.md` (method, 4
tasks × 3 reps × 3 arms, per-task tables, context-overhead probe,
microbenchmarks, caveats), `bench/` tree listing (`BENCHMARK.md`,
`ctx.sh`, `mcp_list.py`, `run.py`, `run_all.sh`, `setup.sh`,
`tasks.json`, `results/`), `bench/ctx.sh`, `bench/tasks.json`
(4 prompts + hand-made regex keys). HEAD commit `09ba4aa`
("docs: simplify usage example", 2026-10-17).

**Headline numbers (reported):** leanrea 388 s / $2.17 / 55 turns /
2.40 M tokens / 74-75; no-skill 479 s / $2.21 / 81 / 3.11 M / 67-75
(one refusal); REA+MCP 627 s / $3.36 / 124 / 5.93 M / 75-75. Context
probe ("Reply OK"): nothing 31.4k, skill 34.5k (+3.1k), REA+MCP
deferred-search 34.9k (+3.5k), REA all-loaded 400.7k (+369k, $1.60).
Caveats: n=3/cell, one model (Sonnet), one machine; REA tested in its
best case (deferred tool search); Ghidra/Hopper/jadx absent both arms.
