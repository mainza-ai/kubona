---
type: entity
name: leanrea
date: 2026-10-17
updated: 2026-10-17
tags: [agents, prior-art, reverse-engineering, skill, benchmark, mcp]
sources: [source-leanrea-investigation-2026]
status: reference
---

# leanrea

One 8 KB `SKILL.md` that claims REA's results with no MCP server, no
207 tools, no setup — routing knowledge (fastest command per target),
ready-to-paste recon one-liners, and the traps REA's code handles.
Single maintainer, distribution via `npx skills add`, MIT licence:

- `aicodeking/leanrea` — HEAD `09ba4aa` (2026-10-17). → `https://github.com/aicodeking/leanrea`

> Method note: read as prior art for *mechanisms*, never as supply
> ([[integration-topology-and-licence-discipline]]: reference class). Claims
> below anchor to the live tree at the SHA above: `leanrea/SKILL.md`
> (Rules, First move, 17-row Router, Speed patterns), `README.md`,
> `bench/BENCHMARK.md` + `bench/{ctx.sh,tasks.json}`. Full evidence
> record: [[source-leanrea-investigation-2026]].

## Why it matters to Kubona

leanrea is the counter-thesis to the REA investigation: every MCP tool
definition is a per-turn token tax, justified only where the wrapper
provides what the raw path cannot. Its measured numbers reframe our
surface — kubona's full Python MCP surface is 29 tools / ~8–10 KB /
~2k tokens (measured 2026-10-17, see [[leanrea-adoption-plan]] §1),
roughly 1/200th of REA's all-loaded 400k. The transferable patterns,
each verified in its files:

- **Knowledge-first distribution** — one paste-able text file beats an
  installer where no trust property is at stake (`README.md` install,
  `SKILL.md` plain-text fallback).
- **Router + traps, not wrappers** — fastest-command-per-target table
  plus the gotchas REA's code handles (`SKILL.md` Router, Rules).
- **Turns-cost-more-than-bytes batching** — independent probes in ONE
  shell call with `echo '--- name'` markers; 2–4 tool calls per task
  (`SKILL.md` Rules, Speed patterns).
- **seen / inferred / unknown** — every claim tagged with citation or
  explicit unknown; absence is never asserted (`SKILL.md` Rules).
- **Benchmark template** — small task set, hand-made regex keys, N reps
  × M arms, published caveats (`bench/tasks.json`, `run.py`, `ctx.sh`,
  `BENCHMARK.md` Caveats).
