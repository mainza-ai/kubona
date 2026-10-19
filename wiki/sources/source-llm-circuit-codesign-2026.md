---
type: source
tags: [source, circuit-synthesis, llm-search, reversible-computing, kickmix, tq-cost]
date: 2026-10-18
updated: 2026-10-18
status: source
origin: "https://arxiv.org/abs/2610.22367v1 (Gu/Kemeny/Yelin/Lukin, Discovering state-of-the-art quantum circuits with large language models, 15 Oct 2026, 14pp + SI 29pp; unreproduced — all numbers are Anthropic-claimed)"
---

# Source: LLM-Guided Quantum Circuit Co-Design (Gu et al. 2026)

**Live record:** `https://arxiv.org/abs/2610.22367v1` — Gu, Kemeny, Yelin, Lukin (Harvard / Anthropic STEM Fellows / QuEra).
**Method note:** pasted full text + 17 page-images + 363-line video-transcript brief read line-by-line 2026-10-18. No PDF vendored into `raw/` (licence unverified for redistribution); this page is the durable summary with equation/figure anchors.

## 1. The loop (reusable part; circuits are the demo)

Task spec (hand-written, no implementation hint) + blank Python program → LLM proposes a circuit-generating program → **independent programmatic evaluator** verifies + scores → feedback → revise. Each iteration starts a NEW agent session (parent + feedback + limited archive context; never the full history); the agent may run drafts (`trial_eval`), query a classical oracle, and delegate to subagents. Paper's hedge: "without substantive human intervention" — the specs are hand-written, so not zero-human. `OpenEvolve [33]` + `Claude Mythos 5 [28]`.

- Proposer ≠ checker: convincing-wrong scores zero.
- Objective is a number (`TQ-cost`), so revisions are rankable.
- Domain admits cheap exact check: target `|x⟩|y⟩ → |x⟩|y⊕f(x)⟩`; linearity reduces coherent correctness to computational-basis trajectories. Kickmix (reversible + diagonal + destructive X-measure) tracks `(bits, phase)`, not `2^n` amplitudes.

## 2. Validity + statistics (Methods Eqs 10-21)

- `V|x⟩|0^a⟩ = (-1)^{g(x)}|π(x)⟩|0^a⟩` (Eq 10); temp-AND `2T → 1T+CZ` via MBU (Eqs 11-12); measurement transition Eq 13; `V_m|x⟩ = (-1)^{φ(x,m)}|π(x)⟩` (Eq 15); exactness `φ(x,m)=g(x)` (Eq 16). **Record may XOR/combine and control diagonal corrections only — never a basis-changing gate.**
- Defective mass `ε`, `||ΦV−ΦU|| ≤ 2√ε`, fidelity `≥(1−2ε)²` (Eqs 17-19); `k` passes → `ε ≤ 1−δ^{1/k}` (Eq 20); bounded-failure `P_{k,b}(ε)` (Eq 21). DQI `|⟨f⟩impl−⟨f⟩| ≤ 4ε⟨f⟩` (SI Eq 25); gate `≤100/100k → ε ≤ 2.434e-3 @97.5%`.

## 3. Cost + search

- `TQ = Toffoli-count × peak-qubits` (`T` counts Toffoli+CCZ equally). Paper's variant is real-valued `Q^κ·T, κ>1` (1–3 cited from factoring estimates); Kubona discretizes to integer `κ∈{1,2,3}` for ledger-exact integer scoring (see `tq.py`). Toffoli-depth under parallel factories, active-volume under cheap cold storage; "any computable objective" runs.
- Fixed spec, builder API (`alloc/free`, `ccx`, free Clifford gates, corrected MBU), ideation on spec alone (≤5 families: F24's were F1 Fermat `x^14` by free squarings + mults, F2 full QROM 1-of-16, F3 tower `F(2²)²`), blank start, families mature separately then share elites, archive `programs+results+failure-feedback+DISCOVERY notes` (≤3 `DISCOVERY:` lines travel with the program), complete-program submission, MAP-Elites `(T,Q)` min-TQ per cell (no cell geometry given), non-monotonic lineage allowed.
- Extended-Data Fig 1 (two SEPARATE cases, not one chain): valid Fermat sample `66T/20Q`; five-products candidate INVALID (`t not returned to |0⟩`, `TQ-cost = ∞`) → repaired with `hmr` + `cz(x[0],x[1],cond=m)` → VALID at unchanged `5T/10Q`, `TQ = 50`. Task interface: preserve 4q `x`, XOR into ARBITRARY `h`, exact ∀x,h, zero global phase, emission-time classical allowed. Fig 2: Gaussian lineage ring `1583T/139Q`.

## 4. Three principles + results

1. **Conditional routing** `Ub=Rb⁻¹KRb` (2× controlled-MADD → swaps + 1× MADD, `/2`).
2. **History decoding** (invariants of reachable states; BM `σ0δ=1`; divstep `b=bit_{j−1}(g'vq⁻¹ mod 2^j)`; Lanczos recompute).
3. **Failure budget** (rare-input cost localized to stack/iterations/lanes, then omitted within `ε`).

Results (Anthropic-claimed, unreproduced): RS 12×, Hermitian 9100× vs Feng-Rao, HHL 32×, Coulomb 9.9×, Gaussian 39×, Fe2S2 4.1×, gauge 7.7×, divsteps new (4.0M/935 frontier + 2.1M/843 specialized-Euclid). Supporting: MBU, in-place accumulation, offline integer-exact fitting, evaluation-interpolation `Au=Φᵀdiag(η)Φu`.

## 5. Boundaries + cautions

Fixed-basis interface excludes coset reps; distribution/representation/accuracy encode embedding; extension criterion = expressive interface + cheap check; end-to-end (algorithm+circuit+code+magic+arch) is direction not result. Baselines from paper's own tables (Schrottenloher'26, Luo'26, Khattar'25).

> ⚑ Attribution: `53h` = elliptic-curve run only, FeMoCo curve = multi-group effort, and Mythos-vetted-only availability come from the VIDEO transcript brief (363 lines, operator-supplied), NOT from the paper HTML — no confirming quote found there. Treat as transcript-sourced until the video description/links confirm.

## Kubona use

Blueprint for [[evaluator-guided-circuit-codesign]], [[reversible-co-design-patterns]], [[failure-budget-and-frontier]]. Re-implement, never vendor. OpenEvolve = blueprint/reference until topology ruling.
