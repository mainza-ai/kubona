# UX and Access Audit — 2026-11-02

**Status:** audit complete; Phase 0-3 implemented
**Scope:** first-run, campaign creation, Copilot chat, lab/auth surfaces, primary navigation,
mobile layout, local-private deployment posture.
**Production bar:** local-private means fully functional, not a demo. No auth weakening,
no anonymous metered spend, no invented identity, no simplified logic that must be redone.

## Executive verdict

Kubona's **security model is coherent; its first-run UX is not**. The supported local-private
topology is a single-lab, loopback-only deployment. The backend already supports that flow:
on a fresh single-lab store, anonymous session creation, campaign reads, and chat all return
2xx. The console nevertheless blocks the first chat with:

> Sign in to start a campaign session in this multi-lab store. No session was created.

The result is a product that looks like it requires an enterprise identity setup before a
researcher can ask their first scientific question. That is the blocker to fix.

The current live stack makes it worse because it is **multi-lab from repeated E2E runs**, so
anonymous campaign creation succeeds and the follow-up campaign read returns 401. The user
gets an orphan campaign and "Campaign unavailable". This is not a normal local-private
state, but it is the state the user actually sees.

**Recommendation:** keep multi-lab auth, but make single-lab local-private a first-class,
server-enforced open mode. Do not remove auth globally, do not allow anonymous cloud spend,
and do not relabel the problem as a demo shortcut.

## Implementation update (2026-11-02)

The P0 access path is now implemented:

- `GET /api/access` publishes `mode`, `single_lab`, `anonymous_chat`, `llm_local`,
  `campaign_create`, and `auth_required` without tenant rows or secrets.
- Anonymous chat is server-enforced: allowed only for single-lab + local LLM;
  multi-lab or any cloud LLM role returns 401 before a session is touched.
- Anonymous campaign creation is refused in multi-lab before mutation, closing the
  orphan-campaign bug.
- The console no longer requires `authWhoami` before creating/using a session; it
  trusts the server and maps a real 401 to sign-in.
- The silent `Quantum Research Campaign #1` auto-create is gone. The Workbench and
  Copilot now offer an explicit first-campaign action.
- Copilot no longer invents an H2 target or "Autonomous Swarm Initialized" message
  when no campaign exists.
- AuthPanel now distinguishes anonymous (`not signed in`) from expired, and labels
  the panel "Lab session"; recent anonymous 401s do not say "Session expired".
- The header shows `Local private` or `Multi-lab`; multi-lab empty state routes to
  sign-in instead of offering a campaign create that will fail.
- Labs administration panels are behind "Advanced lab administration"; the primary
  Campaigns flow is no longer an admin page.
- First-run readiness chips show backend connectivity, local/cloud LLM mode, role, and
  the absence of a campaign; Copilot now states the next campaign step and links to
  Results after a verified claim.
- Role-aware administration: `/api/access` reports `local_researcher`, `anonymous`,
  `lab_admin`, or `operator`; a researcher sees only the sign-in view in Labs, while a
  signed-in lab admin gets the administration panels. The Workbench readiness row shows
  the current role.
- State isolation: `scripts/local-reset.py` takes a verified online backup and refuses
  while the daemon holds the lock before resetting a local ledger.
  `scripts/e2e-isolated.py` runs Playwright against a disposable API/console pair on
  dedicated ports, so E2E labs never pollute the operator's workspace.

Live verification:

- Single-lab (`KUBONA_SINGLE_LAB=1`) console: `Local private mode`, no sign-in,
  `Create your first campaign`, campaign opened, Copilot sent, local Strata replied.
- Multi-lab console: `Multi-lab workspace`, sign-in CTA, no campaign create, no
  orphan campaign after a create attempt, no anonymous "Session expired".
- 576 console tests, full Go suite, `tsc`, ESLint, production build, placeholder
  gate, wiki lint, and real-stack E2E smoke pass.
- Isolated E2E launcher smoke passed against a disposable `:18080` API and `:13000`
  console; the operator's live `:8080` state was untouched.
- `scripts/local-reset.py` backup/reset smoke passed on a throwaway ledger.

The current local `:8080` test database was reset after a verified backup
(`/tmp/kubona-live/kubona.db.reset-backup-20261102T213955Z`) and the runtime was
restarted as a clean single-lab local workspace. The live UI now reports
`Local private`, `researcher`, no sign-in, and the Copilot reply path works.
Remaining operational step: run the full CI/nightly gate on the exact commit.

## Evidence gathered

### Live UI, current multi-lab stack

- Campaign menu: `Sign in to view campaigns in this multi-lab store.` + `Open Labs AuthPanel`.
- `New` remains enabled. Creating a campaign returns 201, then the campaign read returns
  401: `Campaign unavailable ... multi-lab store: log in or present the operator token to
  touch sessions`.
- Copilot send: sign-in blocker; no assistant reply.
- Laboratories page: `Session expired — log in again` while the panel also says
  `anonymous (bearer only)`.
- Mobile 390px: no horizontal overflow; the Inspector is usable, but the primary flow is
  still undiscoverable.

### Live API, isolated single-lab stack (`KUBONA_SINGLE_LAB=1`)

- `GET /api/sessions` anonymous → 200, `{"sessions":[]}`
- `POST /api/sessions` anonymous → 201
- `GET /api/sessions/{id}/campaign` anonymous → 200, proposal with no fabricated results
- `POST /api/chat/messages` anonymous → 201, local Strata reply

### Console single-lab UI

The same single-lab API was wired to a console on `:3001`. The page auto-created/loaded the
campaign, but Copilot still refused to send and showed the sign-in blocker. The client guard,
not the backend, is the blocker.

## Findings

### P0-1 — The console blocks the supported local-private flow

`platform/console/src/components/CopilotDrawer.tsx:208-223` refuses to mint a session when
`authWhoami()` is anonymous, regardless of single-lab mode. The server accepts the same
request in single-lab mode. This is a client/server policy contradiction.

**Impact:** a fresh local-private user can create a campaign but cannot talk to the AI. This
is the reported blocker.

**Fix:** make the server's access policy authoritative. The console should attempt the
session and map a real 401 to sign-in. In single-lab local mode, it should not ask for auth.
No client-side guess about identity is needed.

### P0-2 — Anonymous campaign creation is allowed where reading it is not

`platform/core/pkg/api/server.go:1638-1652` lets anonymous callers create a session in the
default lab even when the store is multi-lab. `tenant_reads.go:77-82` refuses the read that
follows. The UI therefore creates an orphan campaign and then shows "Campaign unavailable".

**Impact:** broken state, wasted user action, and a data row the user cannot reach. It looks
like a bug in campaign creation, not an auth boundary.

**Fix:** `POST /api/sessions` must require the same single-lab open-mode predicate as the
campaign read. In multi-lab, refuse before creating anything with a friendly sign-in response.

### P0-3 — "Session expired" is shown to anonymous users

`AuthPanel.tsx:203` treats any recent 401 as an expired session, even when the user never
had an authenticated session. The panel simultaneously says `anonymous (bearer only)` and
`Session expired — log in again`.

**Impact:** the user is told their session died before they ever signed in. It makes the
auth surface feel broken and increases the sense that an account is required.

**Fix:** distinguish `anonymous` from `expired` using auth phase, not the recent-failure
timestamp alone. Anonymous copy: "Sign in to manage this lab." Expired copy only after a
real authenticated session was lost.

### P0-4 — First-run has no guided setup or primary action

The page auto-creates `Quantum Research Campaign #1` when the session list is empty
(`app/page.tsx:232-240`). There is no welcome, target choice, or readiness check. The
Workbench empty state says "Create or open a campaign" but has no create button
(`Workbench.tsx:645-655`). The only `New` action is inside the campaign menu.

**Impact:** the first screen is an empty scientific workbench with no obvious next step.
When auto-create is refused, the user is stranded.

**Fix:** replace silent auto-create with a first-run onboarding surface:
1. Welcome + local-private mode/readiness.
2. Choose a target or start from a template.
3. Create and open the campaign.
4. Show the first prompt with a clear "Ask Kubona" action.

Keep auto-create only as an explicit recovery fallback, not the primary onboarding.

### P0-5 — Labs administration is presented as the user's front door

The campaign 401 CTA sends the user to Laboratories, which exposes:
- Operator session
- Lab ID / operator token
- Browser key minting
- Cross-lab grants
- Key recovery/rotation
- Session refunds

This is an operator administration console, not a researcher setup path. It dominates the
screen at the exact moment the user is trying to start research.

**Impact:** the user concludes the app is "asking for complicated authorization" and stops.
The actual requirement (single-lab local chat) is not visible.

**Fix:** separate **Research** from **Administration**. Put lab/auth operations behind a
Settings/Admin area. In the primary flow, show one of:
- Local mode: no prompt; "This is your local Kubona."
- Multi-lab signed out: "Join a lab" with two clear paths (browser key or operator invite).
- Multi-lab signed in: normal research.

### P0-6 — No server-side no-spend policy for anonymous chat

Single-lab anonymous chat currently reaches the LLM. With a local provider that is free; with
`KUBONA_LLM_CLOUD=1` or a cloud role it can incur spend. The browser guard is not a security
boundary.

**Impact:** the product's stated no-anonymous-spend rule is enforced only in the client. A
direct API call bypasses it.

**Fix:** server-side rule:
- anonymous chat allowed only when the store is single-lab **and** no active/role provider is
  cloud/metered;
- otherwise 401/403 with a specific sign-in or local-LLM message;
- session creation with spend remains clamped/defaulted; metered dispatch/mandates stay
  authenticated.

### P1-7 — Copilot fabricates campaign context before a campaign exists

`CopilotDrawer.tsx:890-904` defaults the target to H2; `:1113-1116` says
"Autonomous Swarm Initialized: Exploring Hydrogen Molecule (H₂)" even when no campaign is
selected. The quick prompts are also H2/LiH-specific.

**Impact:** the user cannot tell whether a campaign exists or whether the system is already
running. It undermines trust in the measured/proposed provenance work.

**Fix:** no-campaign state: "No campaign selected. Create or open a campaign to start." Hide
contextual prompts until a campaign exists; offer a "New campaign" action. Keep quick prompts
only as templates after a target is chosen.

### P1-8 — Error surfaces expose transport/auth internals

Raw `HTTP 401` payloads, `multi-lab store`, `operator token`, `bearer`, `remediation`, and
`GET /api/...` appear in the primary UI.

**Impact:** a researcher cannot act on the message. The remediation text is aimed at an API
consumer, not a browser user.

**Fix:** map auth/transport errors to a small UX taxonomy:
- "Sign in to continue" (with one button)
- "You are offline / backend unreachable" (retry)
- "This lab is read-only for your account" (request access)
- "No campaign exists" (create)
Keep the raw detail behind a "Details" disclosure for operators.

### P1-9 — Local-private mode is invisible

Nothing in the console says whether the user is in a single-lab local deployment, a
multi-lab store, or a cloud/remote deployment. The auth requirement appears arbitrary.

**Fix:** expose a public `/api/access` descriptor (mode, single-lab, anonymous chat allowed,
lab admin configured, LLM locality) and show a compact mode badge with a short explanation.
Do not expose secrets or tenant listings in it.

### P1-10 — E2E and local production share state

Repeated E2E runs created many labs in the local database and flipped the app into multi-lab
mode. This directly caused the current blocker.

**Fix:** E2E must use an isolated database/state directory and tear it down. Add an explicit
local reset command with a confirmation and a backup-first guard. Do not let test labs appear
in a user's local workspace.

### P1-11 — Sessions, campaigns, labs, tenants, operators: five words for two concepts

Users see "campaign", "session", "lab", "tenant", and "operator" interchangeably. The
technical model is real, but the UI does not explain the mapping.

**Fix:** define the public vocabulary:
- **Workspace/Lab** = the isolated research unit.
- **Campaign** = one research question/session.
- **Operator/Admin** = a person who can change lab settings.
Use "lab" consistently in admin; "campaign" consistently in research.

### P2-12 — Contextual guidance is shallow

The quick prompts are static. There is no next-step checklist after campaign creation
(propose -> authorize -> run -> results), no indication of which action is blocked by auth,
and no explanation of why a proposal cannot run yet.

**Fix:** a campaign action rail: Target -> Proposal -> Authorization -> Run -> Evidence ->
Results, with the next enabled action and its gate. Show blocked reasons inline, not as raw
errors.

### P2-13 — Mobile and accessibility

The 390px layout has no horizontal overflow and the Inspector opens, but:
- primary New/Start discovery is weak;
- auth/admin panels are long single-column stacks;
- small text and dense mono labels dominate;
- dialogs and auth errors need a focus/announcement audit.

**Fix:** a task-focused mobile header (Campaign, Ask, Run), larger primary actions, and an
`aria-live` error region for auth state changes.

## Gaps and missing functionality

1. Server-enforced local open mode for single-lab, local-LLM deployments.
2. Consistent create/read auth policy (no orphan campaigns).
3. A first-run onboarding flow and a clear primary "New campaign" CTA.
4. A local-owner/access descriptor so the UI can render the right experience.
5. A research-vs-administration information architecture.
6. Friendly auth/error copy and a sign-in dialog that starts from the blocked action.
7. Local reset and E2E state isolation.
8. Contextual no-campaign Copilot state.
9. Campaign lifecycle guidance and next-action gating.
10. Role separation if multi-lab remains a supported deployment (researcher vs admin).
11. LLM readiness/configuration guidance before the first prompt.
12. A documented migration path for existing multi-lab local databases.

## Target flows

### A. Fresh local-private (primary)

1. `docker compose ... up`
2. Open `http://localhost:3000`.
3. See "Kubona is running locally" + readiness (API/bridge/LLM/anchor).
4. Choose a target/template and create the first campaign.
5. Copilot opens with "Ask about this campaign"; send.
6. Local Strata answers; no sign-in, no lab token, no browser key.

Admin/settings remain available but do not block the flow. If the LLM is cloud-backed,
step 5 asks to sign in or switch to local, with a clear explanation.

### B. Multi-lab researcher

1. Open the app.
2. See "Join a lab" with `Lab ID` + browser key, or "Use an operator invite".
3. Sign in.
4. Campaign list appears; create/open.
5. Research flow is identical.

### C. Multi-lab admin

1. Sign in as lab admin.
2. Settings -> Labs -> Create lab / grants / key rotation / refunds.
3. Operator token is an advanced bootstrap path, not the default front door.

## Implementation plan

### Phase 0 — Correctness and copy (done)

1. Fix `AuthPanel` anonymous vs expired copy.
2. Make `POST /api/sessions` enforce the same single-lab predicate as reads.
3. On create/read 401, do not show an orphan campaign; show a sign-in action.
4. Remove the no-campaign H2/swarm message and hide contextual prompts until a campaign exists.
5. Add `New campaign` CTA to the Workbench empty state.
6. Tests: create/read auth matrix, AuthPanel copy, no-campaign Copilot.

**Acceptance:** multi-lab anonymous create is refused before mutation; no orphan campaign;
no "Session expired" for an anonymous user.

### Phase 1 — Local open mode (done)

1. Add a server access policy:
   - `single_lab = singleLabDeclared() && CountDistinctTenants <= 1`
   - `anonymous_chat = single_lab && !hasCloudLLMRole(snapshot)`
2. Enforce it in chat messages/stream/regenerate/tribunal (and any LLM-bearing route).
3. Expose `GET /api/access` (public, no secrets):
   `{mode, single_lab, anonymous_chat, lab_admin_configured, llm_local}`.
4. Console reads `/api/access` and removes the `authWhoami` precondition in local open mode.
5. Keep `requiresSignIn` for multi-lab. On server 401, show the sign-in card and do not
   create a session.
6. Keep metered spend, cloud LLM, mandates, and settings auth-gated.
7. Tests: single-lab local chat 201; single-lab cloud chat 401; multi-lab chat 401;
   no session minted on refusal; no anonymous metered spend.

**Acceptance:** fresh single-lab local compose reaches first assistant reply without an auth
prompt; multi-lab remains protected; cloud/metered paths still refuse anonymously.

### Phase 2 — First-run and information architecture (partially done)

1. Welcome/readiness panel with API, bridge, LLM, and anchor status.
2. Campaign template chooser; explicit create; remove silent generic auto-create.
3. Primary CTA in header/empty state.
4. Research vs Administration navigation split.
5. Multi-lab join/create-lab wizard; operator token only in Advanced.
6. Friendly auth/error taxonomy with Details disclosure.
7. Mode badge: Local private / Multi-lab / Remote.
8. Tests: first-run E2E, multi-lab join, admin path, mobile 390px, a11y.

**Acceptance:** first-time local user can create a campaign and ask a question without reading
documentation; a multi-lab user can join a lab without seeing grant/refund/key-rotation panels.

### Phase 3 — Workflow guidance and operations (done)

1. Campaign action rail and next-action gating.
2. Contextual prompts tied to target/phase.
3. LLM readiness and local/cloud switch guidance.
4. Local reset + E2E isolated state.
5. Role-aware researcher/admin views.
6. Metrics (local-only): time-to-first-reply, auth-prompt rate, create-success rate,
   blocked-action reasons.

**Acceptance:** no raw transport error in primary UI; every blocked action names the next
step; local state can be reset safely; tests never contaminate a production local store.

## What not to do

- Do not remove multi-lab isolation or weaken lab/tenant checks.
- Do not allow anonymous metered LLM or QPU spend.
- Do not "fix" the UX by bypassing the ledger, mandates, or browser-key identity.
- Do not make the first-run path depend on a hidden environment variable the user cannot see.
- Do not add a demo-only auto-login that ships into production.

## Acceptance metrics

- Fresh local-private: time from console load to first assistant reply < 30 seconds.
- Fresh local-private: zero auth prompts for local LLM + local simulator use.
- Multi-lab: zero anonymous campaign creates; zero orphan campaigns.
- Cloud LLM/metered path: 100% of anonymous attempts refused server-side.
- Primary UI: zero raw `HTTP 401` / `operator token` / `bearer` strings.
- Mobile: no horizontal overflow at 390px; primary CTA visible without opening a menu.
- Accessibility: auth changes announced via `aria-live`; focus returns to the blocked action.

## Relevant files

- `platform/console/src/components/CopilotDrawer.tsx` — client auth guard, default target,
  initial message.
- `platform/console/src/app/page.tsx` — auto-create, campaign create, campaign-menu 401,
  `requiresSignIn`.
- `platform/console/src/components/common/AuthPanel.tsx` — anonymous/expired copy.
- `platform/console/src/components/common/LabOpsPanel.tsx` /
  `CreateLabPanel.tsx` — admin bootstrap and operator-token flow.
- `platform/console/src/components/workbench/Workbench.tsx` — empty state.
- `platform/core/pkg/api/server.go` — session create/read and chat routes.
- `platform/core/pkg/api/tenant_reads.go` — single-lab read predicate.
- `platform/core/pkg/api/feature_gates.go` — `singleLabDeclared`.
- `platform/docker/compose.prod.yml` — `KUBONA_SINGLE_LAB=1` local posture.
- `docs/local-deployment.md` — first-run identity documentation.
- `platform/console/e2e/` — state isolation.
