# Agentic mode: user-controlled autonomy, research synthesis, and implementation plan

**Date:** 2026-11-18 · **Scope:** turn the auto AI agentic workflow into a user-controlled
mode, ground the design in current practice, and update the implementation plan for the
remaining accuracy and frontend work.

---

## 1. Executive summary

Kubona now has a **session-level agentic master switch**:

- **On** — agent loops may start/continue and plans may execute under the existing
  tenant autonomy (L0–L3) and the AG-7 stage ladder (`suggest` / `approve` / `act`).
- **Off** — agent loops and autonomous plan execution are blocked, running loops are
  paused, and the **manual experiment rail remains available**.
- **Locked** — an operator `POST /autonomy/halt` sets the switch off and locks it; a
  session caller cannot clear the lock. Only an operator policy write can.

The switch is session-scoped (`PUT /api/sessions/{sid}/agentic`), visible in the
Autoresearch panel with a role=`switch` control and an explicit status line, and every
change is sealed as an `agentic_mode_changed` event. It is enforced at loop
start/resume/wake/step, plan execution, and restart rehydration — not just in the UI.

The research consensus is unambiguous: **autonomy is a framework-level UX dimension,
not a model or feature setting**. Current products expose a named ladder, grade it by
risk/reversibility, enforce it at the gateway, show the current mode at all times, and
keep a real kill switch. Kubona now has those pieces; the remaining gaps are durable
approvals, a notify rung, per-tool permissions, undo receipts, and approval-fatigue
telemetry (AG-18…AG-24).

The accuracy and frontend follow-ups from the previous investigation remain as planned:
AG-11 (TFIM optimizer/convergence), AG-12 (noise-aware scoring), AG-13 (H2
depth/ansatz), AG-14 (QAOA depth), AG-15 (console panel layout audit), AG-16
(telemetry semantics). New research in §8 sharpens AG-11/AG-13 (HVA-SB, multi-start
restarts, feature-evaluation budgets) and AG-14 (QAOA p=2/p=3 guarantees).

---

## 2. What the research says

### 2.1 Autonomy is a product dimension

The Autonomy Toggle (Tian Pan, 2026-05-02) argues that autonomy is a UX dimension like
notification frequency or density, not a model property. Teams that hardcode one level
split their users into “stop interrupting me” and “stop doing things behind my back”.
The fix is a framework-level ladder with consistent semantics across features. Four
rungs are usually enough:

1. **Show me** — surface the intended action, take none.
2. **Ask me** — prepare the action and block for approval.
3. **Do it and show me** — execute, then surface a synchronous receipt.
4. **Do it and tell me later** — execute and batch results into an auditable digest.

The semantic guarantee matters: “ask me” must block everywhere, never “ask me
sometimes”. Source: <https://tianpan.co/blog/2026/05/02/autonomy-toggle-user-setting-not-model-setting>.

### 2.2 Visible permission modes and a status line

Claude Code ships six named modes — Manual (`default`), acceptEdits, Plan, Auto, Don't
ask, bypassPermissions — switchable mid-session (Shift+Tab), with the current mode
always visible in the status bar. Plan mode lets a user review/edit/approve the whole
strategy before execution and then switch to Auto; organizations can disable Auto
(`disableAutoMode`). Auto mode delegates approvals to a classifier that blocks
irreversible/destructive/out-of-scope actions. Sources:
<https://code.claude.com/docs/en/permission-modes>,
<https://code.claude.com/docs/en/auto-mode-config>,
<https://www.anthropic.com/engineering/claude-code-auto-mode>.

The lesson for Kubona: the user must always be able to see **which** mode is active,
change it in place, and have the mode mean the same thing on every surface.

### 2.3 Tool-level approvals and durable pause/resume

The OpenAI Agents SDK marks tools with `needs_approval`, pauses the run with
`interruptions`, and resumes from a serialized `RunState`; missing/malformed arguments
fail closed to manual approval, and guard ordering prevents swapping approved arguments
before execution. Sources:
<https://openai.github.io/openai-agents-python/human_in_the_loop>,
<https://www.codesprintconsulting.com/blog/managed-tool-approvals-openai-agents-sdk>.

The lesson: an approval must be **durable** across restarts and scoped to the exact
action; “approve once” is not a substitute for “approve this payload”.

### 2.4 Tiered oversight at the gateway

AWS's Agentic AI Lens recommends three tiers — **autonomous** (low-risk, reversible),
**notify** (medium-risk, proceed with awareness), **approve** (high-risk/irreversible) —
encoded as policy at the gateway, with automated first-pass review and an audit of
reviewer identity, rationale, and timestamp. It warns that uniform oversight either
bottlenecks routine work or lets high-consequence actions through. Source:
<https://docs.aws.amazon.com/wellarchitected/latest/agentic-ai-lens/agentrel02-bp05.html>.

### 2.5 Authority levels and meaningful human control

CSA's autonomy levels (human performs → AI advises → AI acts with approval → AI acts
with human review → fully autonomous) and Gartner/FPF authority levels make the same
point: calibrate autonomy by **stakes and reversibility**, enforce limits by design,
and name a human accountable. Approval fatigue is a real failure mode: reviewers who
face hundreds of context-free requests rubber-stamp them. Sources:
<https://labs.cloudsecurityalliance.org/research/agentic-ai-autonomy-levels-control-framework-v2-csa-styled>,
<https://sloanreview.mit.edu/article/responsible-ai-means-knowing-the-limits-of-agent-autonomy/>,
<https://fpf.org/wp-content/uploads/2026/08/FPF-2026-Best-Practices-for-AI-and-WA-Tech-R3-1.pdf>,
<https://www.ncsc.gov.uk/blogs/managing-the-cyber-risk-of-agentic-ai>.

### 2.6 Human-in-the-loop patterns

Production guidance converges on pre-flight plan approval, step checkpoints for
irreversible actions, asynchronous review with reversal windows, and continuous
interruptibility for long runs. “In-the-loop” blocks; “on-the-loop” watches and can
interrupt; “out-of-the-loop” reviews afterwards. The decision rule: reversible +
low-stakes → proceed and log; irreversible + complex → pre-flight or step approval.
Sources:
<https://agentinterface.co/blog/human-in-the-loop-ux-patterns-production-ai-agents>,
<https://www.smashingmagazine.com/2026/02/designing-agentic-ai-practical-ux-patterns>,
<https://aiuxplayground.com/guides/how-to-design-human-in-the-loop/>.

### 2.7 Mapping the ladder onto Kubona

| Kubona mechanism | Research analogue | Semantics |
|---|---|---|
| Tenant autonomy L0–L3 | Org-level authority / `disableAutoMode` | Outer bound; L0 refuses dispatch |
| AG-7 `suggest` | Show me / Plan mode | Agent proposes; human executes |
| AG-7 `approve` | Ask me / approval gate | Plan must be approved |
| AG-7 `act` | Do it and show me / Auto (bounded) | Low/medium risk inside budget; receipt |
| `agentic_enabled=false` | Manual mode | No loops, no agent execution; manual rail |
| `agentic_locked=true` | Org kill switch | Session caller cannot re-enable |
| `/autonomy/halt` | Emergency stop | Stages→suggest, switch→off, jobs cancelled |
| Risk classifier | autonomous/notify/approve tiers | Hardware/dual-use/overspend → approval |

The missing rung is **“do it and tell me later”** (notify/digest), which is AG-20.
Kubona's `act` is synchronous (the plan result is returned), so it maps to “do it and
show me”, not silent batching.

---

## 3. Design decisions

### 3.1 One master switch above the stage ladder

`agentic_enabled` is stored in the same `session_autonomy` policy document as the stage
modes. It is the **master switch**; the stage ladder is the **gradient** beneath it.
Turning it off does not erase the stage policy: turning it back on restores the
configured modes.

Default is **true** for compatibility with existing sessions; a policy written before
the switch normalizes to true. The switch is a control, not a new default posture — the
safety posture is still tenant L0 and `execute=approve`.

### 3.2 Two writers, two powers

| Caller | Endpoint | Can flip switch | Can clear lock | Can edit stages/budget |
|---|---|---|---|---|
| Session caller (cookie/lab) | `PUT /api/sessions/{sid}/agentic` | yes | **no** | no |
| Operator bearer | `PUT /api/sessions/{sid}/autonomy` | yes | yes | yes |
| Operator bearer | `POST /api/sessions/{sid}/autonomy/halt` | sets off | sets lock | sets all suggest |

Why: the user asked for a switch they can turn on/off, but an operator halt must not be
undoable by the workflow it just stopped. This mirrors the org-level disable in Claude
Code and the “kill switch” requirement in the agentic-risk guidance.

### 3.3 Enforcement matrix

| Action | Switch on | Switch off | Locked |
|---|---|---|---|
| Start autoresearch loop | allowed (tenant/autonomy permitting) | **403 `agentic_disabled`** | **409 `agentic_locked`** |
| Resume / wake / step loop | allowed | 403 | 409 |
| Pause / stop / steer loop | allowed | allowed | allowed |
| Execute a plan | AG-7 decision | **403 `agentic_disabled`** | 409 |
| Create/approve a plan | allowed | allowed | allowed |
| Manual `POST /api/experiments` | allowed | **allowed** | allowed |
| Restart rehydration | resumes as persisted | **paused** | **paused** |

The manual rail is deliberately outside the switch: “off” means “no agent acting”, not
“no science”.

### 3.4 Turning off pauses, never destroys

`pauseSessionLoops` pauses running loops and persists the paused state. Spend
reservations are preserved; the user can resume after re-enabling. The operator halt is
stronger: it also cancels live jobs (`CancelSessionJobs`) and sets all stages to
`suggest`.

### 3.5 Restart safety

`rehydrateAutoloops` previously resumed every persisted running loop at boot. It now
loads the session policy, constructs the engine for visibility, forces `Pause()` when
the switch is off or locked, and persists the paused row. A restart cannot silently
resume work the user turned off.

### 3.6 Audit

Every flip writes `agentic_mode_changed` with `enabled`, `locked`, `actor`
(`session`/`operator`), and `paused_loops`; the operator full-policy write also keeps
`autonomy_policy_updated`. The signed event chain remains the audit trail.

### 3.7 Threat analysis

- **Session user re-enables after an operator halt** → blocked by `agentic_locked`; only
  the operator policy write clears it.
- **Session user turns the switch on** → still bounded by tenant autonomy (L0 refuses),
  the stage ladder (execute may be `suggest`/`approve`), the AG-7 risk classifier, and
  the act budget.
- **Restart silently resumes** → fixed by paused rehydration.
- **Switch off but agent still spends** → loops pause; plan execution refuses before
  submission; manual experiments are human-initiated and still subject to tenant/session
  budgets.
- **Lock bypass via the full policy endpoint** → operator bearer only.
- **UI-only switch** → every enforcement point is server-side; the console is a view.
- **Lost switch state** → stored in the ledger; a malformed/missing policy normalizes to
  enabled + unlocked (fail-open for availability, bounded by the other rails). A future
  strict mode could fail closed on unreadable policy (AG-24).

---

## 4. Implementation

### Backend

- `pkg/autonomy.Policy` gains `agentic_enabled` (pointer; nil → true) and
  `agentic_locked` (bool). `Normalize` fills defaults; `Agentic()` reads the effective
  value.
- `pkg/api/agentic.go`: `GET/PUT /api/sessions/{sid}/agentic`, `requireAgentic`,
  `pauseSessionLoops`.
- `handleAutoresearchStart`: gate after session auth, before harness/spend.
- `handleAutoresearchLoopAction`: gate `step`/`resume`/`wake`; allow pause/stop/steer.
- `handlePlanExecute`: gate before the AG-7 decision.
- `rehydrateAutoloops`: pause when off/locked and persist.
- `autonomy` PUT merges optional switch fields so a stage-only edit cannot change them;
  pause loops when disabled; emit the event on change.
- `autonomy/halt`: switch off + lock + pause loops + cancel jobs.
- Errors: `agentic_disabled` (403), `agentic_locked` (409). Event:
  `agentic_mode_changed`.

### Console

- `lib/api.ts`: `fetchSessionAgentic` / `setSessionAgentic`.
- `AutoresearchPanel`: master switch (role=`switch`, `aria-checked`), status line,
  Locked/On/Off label, Start disabled when off/locked, pause/stop still available.
- Component tests (real Chromium): default on, toggle off sends
  `{"enabled":false}` and blocks Start, locked disables the switch and shows the
  operator message.

### Verification

```
GET /agentic                          → enabled=true, locked=false
PUT  /agentic {enabled:false}          → 200
POST /autoresearch/start               → 403 agentic_disabled
POST /plans/{id}/execute               → 403 agentic_disabled
POST /api/experiments                  → 201 (manual rail open)
POST /autonomy/halt                    → enabled=false, locked=true
PUT  /agentic {enabled:true}           → 409 agentic_locked
PUT  /autonomy (operator unlock)       → 200, enabled=true, locked=false
PUT  /agentic {enabled:true}           → 200
Inspector/Autoresearch browser tests   → 7/7 + 5/5
Go suite / ESLint                      → green
```

---

## 5. Gaps and missing functionality

| Area | Gap | Impact | Plan |
|---|---|---|---|
| Global visibility | Mode is visible only in the Autoresearch panel | Users can forget the mode | AG-18: status bar / command palette indicator |
| Approvals | Plan status is the approval, no inbox with edit/reject-feedback/defer/remember | Approval fatigue; no durable queue | AG-19 |
| Notify rung | No batched “do it and tell me later” | Long runs still require a person at the end | AG-20 |
| Tool permissions | No per-tool allow/ask/block for agent tools | Future write tools would be coarse | AG-21 |
| Fatigue metrics | Plan latency exists; no queue depth/rubber-stamp signal | Cannot detect oversight decay | AG-22 |
| Undo | Jobs/claims immutable; no reversal receipt | Higher-stakes actions lack recovery | AG-23 |
| Org default | No tenant-level default switch or policy revision/rollback | New sessions inherit a hardcoded default | AG-24 |
| Strict policy read | Unreadable policy normalizes to enabled | Availability bias | AG-24 (strict mode) |
| Risk classifier | v1 lexical list; no protocol/domain taxonomy | High-risk direction may be missed | AG-24 (extend) |

### Accuracy/frontend follow-ups (from the previous investigation)

| Slice | Status | Research input |
|---|---|---|
| AG-11 TFIM multi-start + convergence | **Open** | Multi-start/restart literature; plateaus vs traps diagnostic; FE-matched MS-BFGS |
| AG-12 noise-aware scoring | **Open** | Shot-uncertainty vs tolerance; perfect-state control |
| AG-13 H2 chemical accuracy | **Open** | HVA/UCCSD, 20k shots; HVA-SB |
| AG-14 QAOA depth | **Open** | p=2 worst-case 0.7559, average ~0.85; p=3 0.7924 |
| AG-15 console layout audit | **Open** | Container queries; panel-width-aware controls |
| AG-16 telemetry semantics | **Open** | Gap-based `converged`, optimizer status |

---

## 6. Research-backed recommendations for the open accuracy work

### TFIM (AG-11 / AG-13)

- **HVA instead of, or alongside, the hardware-efficient ring.** The TFIM literature
  uses `RZZ` coupling layers and `Rx` field layers, with an optional symmetry-breaking
  `Rz` layer (HVA-SB) to reach parity-violating states. HVA has fewer parameters and a
  smoother structure; HEA is more expressive per layer but harder to train.
  Sources: <https://arxiv.org/html/2602.17662v1>,
  <https://arxiv.org/html/2604.20961v2>.
- **Multi-start is the first fix, not more layers.** At n=8 our 2-layer gap fell from
  1.202 to 0.244 Ha with four L-BFGS-B restarts; 4 layers 0.062, 6 layers 0.030. The
  plateau-vs-trap diagnostic is the spread of final costs across starts; use it to
  decide whether to add restarts or change the ansatz.
  Sources: <https://hbar.university/pdfs/vqa/module4-barren-plateaus/node4-7-plateaus-vs-traps.pdf>,
  <https://arxiv.org/html/2609.00235v1>.
- **Budget feature evaluations, not just iterations.** Report evals, wall time, and the
  best-of-restarts distribution; FE-matched comparisons are the honest way to claim an
  optimizer improvement.
- **Add a 6-layer, multi-start TFIM task** after AG-11; keep the 2-layer task as a
  legitimate depth failure.

### QAOA (AG-14)

- Wurtz–Love give worst-case p=2 **0.7559** and p=3 **0.7924** on 3-regular graphs
  (fixed angles), with average optimal-angle values around **0.85** at p=2. Our measured
  0.891 (p=2) and 0.932 (p=3) are consistent. The 0.85 p=2 bar and 0.90 p=3 bar are
  defensible; 0.90 at p=2 was not.
  Sources: <https://arxiv.org/abs/2010.11209>,
  <https://link.aps.org/accepted/10.1103/PhysRevA.103.042612>,
  <https://link.aps.org/accepted/10.1103/PhysRevA.104.052419>.
- Multi-start or fixed-angle transfer can recover a few tenths of a percent; p=4 does
  not beat p=3 on this graph in our runs, so don’t add a depth task without evidence.

### H2 (AG-13)

- The statevector optimizer already reaches FCI; the remaining task is shot budget and
  a depth/ansatz that stays under 1.6 mHa at a declared shot count. HVA/UCCSD is the
  natural next comparison; report both statevector and shot errors.

---

## 7. Implementation plan update

**AG-17 (done): agentic master switch**
- Session `PUT /agentic`, operator policy merge, lock, halt lock, loop/plan/rehydrate
  gates, console switch, tests, live verification.

**AG-18 (P1): global mode visibility**
- A persistent “Manual / Auto / Locked” chip in the CommandDock/status bar and the
  command palette, with one-click switch; the Autoresearch panel remains the detailed
  control. Acceptance: mode changes are visible without opening the panel; a browser
  test covers the chip.

**AG-19 (P1): durable approval inbox**
- Pending plan approvals as first-class rows with approve/edit-arguments/reject-with-
  feedback/defer/remember; survives restart; resume from the exact payload. Acceptance:
  a deferred approval survives a restart and cannot be double-executed.

**AG-20 (P2): notify rung**
- A `notify` stage mode: execute low/medium-risk work, batch receipts into a digest
  with an audit view; no silent high-risk actions. Acceptance: a nightly loop completes
  and produces a digest without a synchronous review.

**AG-21 (P2): per-tool permissions**
- Agent tools declare `allow`/`ask`/`block`; the execution path enforces it before the
  tool body. Acceptance: a blocked tool never runs; an ask tool pauses and resumes.

**AG-22 (P2): approval-fatigue telemetry**
- Queue depth, approval latency percentiles, rejection rate, and a “reviewed without
  opening evidence” proxy in `/api/ops/science`. Acceptance: metrics appear with real
  values and an explicit unmeasured status where unavailable.

**AG-23 (P2): undo/receipts**
- For reversible side effects, record a before/after receipt and an undo action; jobs
  and claims remain immutable. Acceptance: an undo restores the prior state and is
  audited.

**AG-24 (P2): org defaults and strict policy**
- Tenant-level default `agentic_enabled`; policy revision/rollback; fail-closed on an
  unreadable policy in strict deployments; extend the risk classifier with a protocol/
  domain taxonomy. Acceptance: a new session inherits the tenant default; a corrupted
  policy refuses rather than silently enabling.

**AG-11…AG-16** remain as previously planned, now with the research input above.

---

## 8. Sources

Autonomy and oversight:
- The Autonomy Toggle — <https://tianpan.co/blog/2026/05/02/autonomy-toggle-user-setting-not-model-setting>
- Claude Code permission modes — <https://code.claude.com/docs/en/permission-modes>
- Claude Code auto mode — <https://code.claude.com/docs/en/auto-mode-config>
- Claude Code auto-mode engineering — <https://www.anthropic.com/engineering/claude-code-auto-mode>
- Anthropic trustworthy agents — <https://www.anthropic.com/research/trustworthy-agents>
- OpenAI Agents SDK HITL — <https://openai.github.io/openai-agents-python/human_in_the_loop>
- OpenAI managed approvals — <https://www.codesprintconsulting.com/blog/managed-tool-approvals-openai-agents-sdk>
- AWS Agentic AI Lens tiered oversight — <https://docs.aws.amazon.com/wellarchitected/latest/agentic-ai-lens/agentrel02-bp05.html>
- CSA autonomy levels — <https://labs.cloudsecurityalliance.org/research/agentic-ai-autonomy-levels-control-framework-v2-csa-styled>
- FPF AI authority levels — <https://fpf.org/wp-content/uploads/2026/08/FPF-2026-Best-Practices-for-AI-and-WA-Tech-R3-1.pdf>
- MIT Sloan on limits of autonomy — <https://sloanreview.mit.edu/article/responsible-ai-means-knowing-the-limits-of-agent-autonomy/>
- NCSC agentic cyber risk — <https://www.ncsc.gov.uk/blogs/managing-the-cyber-risk-of-agentic-ai>
- HITL UX patterns — <https://agentinterface.co/blog/human-in-the-loop-ux-patterns-production-ai-agents>
- Smashing Magazine agentic UX — <https://www.smashingmagazine.com/2026/02/designing-agentic-ai-practical-ux-patterns>

TFIM / VQE:
- HVA / expressivity TFIM study — <https://arxiv.org/html/2602.17662v1>
- TFIM ansatz expressivity across sizes — <https://arxiv.org/html/2604.20961v2>
- Hamiltonian expressibility — <https://link.springer.com/article/10.1007/s42484-026-00407-3>
- Plateaus vs traps / multi-start — <https://hbar.university/pdfs/vqa/module4-barren-plateaus/node4-7-plateaus-vs-traps.pdf>
- FE-matched multistart BFGS — <https://arxiv.org/html/2609.00235v1>

QAOA:
- Wurtz–Love p>1 guarantees — <https://arxiv.org/abs/2010.11209>
- Fixed-angle conjectures — <https://link.aps.org/accepted/10.1103/PhysRevA.104.052419>
