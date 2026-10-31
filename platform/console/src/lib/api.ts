import { Session, ChatMessage, Job, Loop, LedgerEvent, Lab } from '@/types';
import { contract } from './responseContract';

// Build-time default (local dev). The runtime route /api/config can
// override this per deployment (see configureApiBase) so one image
// serves any backend without a rebuild.
export let API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

let apiBaseConfigured = false;
let apiBaseRetryAt = 0;
const API_BASE_RETRY_MS = 5000;

// Reads the runtime backend URL and switches all subsequent API calls to
// it. A definitive answer pins it: a valid apiUrl, or a 4xx (older bundle /
// static export with no /api/config). A transient network/5xx failure keeps
// the build-time default for this call but allows a bounded retry, so one
// blip at boot cannot pin localhost for the whole session.
export async function configureApiBase(): Promise<string> {
  if (apiBaseConfigured) return API_BASE;
  if (apiBaseRetryAt !== 0 && Date.now() < apiBaseRetryAt) return API_BASE;
  try {
    const res = await apiFetch('/api/config', { cache: 'no-store' });
    if (res.ok) {
      const data = (await res.json()) as { apiUrl?: unknown };
      if (typeof data.apiUrl === 'string' && /^https?:\/\//.test(data.apiUrl)) {
        API_BASE = data.apiUrl.replace(/\/$/, '');
      }
      apiBaseConfigured = true;
    } else if (res.status >= 400 && res.status < 500) {
      apiBaseConfigured = true;
    } else {
      apiBaseRetryAt = Date.now() + API_BASE_RETRY_MS;
    }
  } catch {
    apiBaseRetryAt = Date.now() + API_BASE_RETRY_MS;
  }
  return API_BASE;
}

// Operator bearer token (local dev only): the bridge requires `Authorization:
// Bearer <token>` on every route and fails closed without it. In production
// this variable is unset, so no header is sent and the bridge keeps refusing.
// Never bake a real operator token into a shipped bundle.
const CONSOLE_TOKEN = process.env.NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN || '';

// NEXT_PUBLIC_* values ship inside the client JS bundle by construction —
// a real operator token here is a published secret. Local dev (`dev-token`)
// is the only legitimate use; production must unset it and log in through
// AuthPanel (Ed25519 session cookie) instead. A production build with the
// token set fails closed (2026-10-26 audit UI-03); KUBONA_ALLOW_DEV_TOKEN=1
// is the explicit, auditable escape hatch for a throwaway local demo build.
if (CONSOLE_TOKEN && process.env.NODE_ENV === 'production'
    && process.env.KUBONA_ALLOW_DEV_TOKEN !== '1') {
  throw new Error(
    '[kubona] refusing a production build with NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN set: ' +
      'the bearer ships to every browser. Unset it and use AuthPanel login, or set ' +
      'KUBONA_ALLOW_DEV_TOKEN=1 for a throwaway local demo build.',
  );
}

function authHeaders(extra: Record<string, string> = {}): Record<string, string> {
  return CONSOLE_TOKEN ? { Authorization: `Bearer ${CONSOLE_TOKEN}`, ...extra } : { ...extra };
}

// All console fetches carry cookies (same-origin by default). Required
// for session auth cross-origin (:3000 → :8080): without credentials the
// browser drops the kubona_session cookie and every session-authed call
// 401s despite a live login. Bearer headers (above) still work — the
// server checks the cookie first, then the token.
function withCredentials(init: RequestInit): RequestInit {
  return { credentials: 'include' as RequestCredentials, ...init };
}

// Default per-request timeout (streams excluded — they carry their own
// caller signal and a stall detector instead). A hung backend must fail
// loudly, never hang the UI forever: every wrapper below funnels through
// apiFetch, so this one default covers all of them.
export const API_TIMEOUT_MS = 15000;

// Stall budget for server-sent streams: a frame (token|done|error) must
// arrive at least this often or the stream is declared dead. Without it a
// wedged backend holds an infinite loading cursor with no recovery.
export const STREAM_STALL_MS = 30000;

// Dispatched on window whenever the backend answers 401 (expired/invalid
// session or token). The shell listens and surfaces "session expired —
// log in again" instead of rendering empty states; flows that would
// otherwise auto-create fresh state on auth failure must check
// wasRecentAuthFailure() first (see CopilotDrawer getActiveSessionId).
export const UNAUTH_EVENT = 'kubona:unauthorized';
// Any non-2xx/transport failure notifies the shell, even when a legacy
// wrapper below collapses it to []/null. Surfaces can keep their local
// empty-state copy without hiding a refusal globally.
export const API_PROBLEM_EVENT = 'kubona:api-problem';
export interface ApiProblem {
  path: string;
  status: number | null;
  message: string;
  at: number;
}
let lastProblemKey = '';
let lastProblemAt = 0;
function notifyApiProblem(input: RequestInfo | URL, status: number | null, message: string): void {
  const raw = typeof input === 'string' ? input : input instanceof URL ? input.toString() : String(input.url || '');
  let path = raw;
  try { path = new URL(raw, typeof window !== 'undefined' ? window.location.origin : 'http://localhost').pathname; } catch { /* keep raw */ }
  if (path === '/api/config' || path === '/api/health' || path === '/api/ready') return;
  const key = `${path}|${status ?? 'transport'}|${message.slice(0, 80)}`;
  const now = Date.now();
  if (key === lastProblemKey && now - lastProblemAt < 5000) return;
  lastProblemKey = key;
  lastProblemAt = now;
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    window.dispatchEvent(new CustomEvent<ApiProblem>(API_PROBLEM_EVENT, {
      detail: { path, status, message: message.slice(0, 240), at: now },
    }));
  }
}
let lastAuthFailureAt = 0;
export function wasRecentAuthFailure(withinMs = 30000): boolean {
  return Date.now() - lastAuthFailureAt < withinMs;
}
function notifyUnauthorized(): void {
  lastAuthFailureAt = Date.now();
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    window.dispatchEvent(new CustomEvent(UNAUTH_EVENT));
  }
}

// Module fetch: every console call goes through here so the session
// cookie flows cross-origin. Callers pass auth headers as before;
// credentials are orthogonal (the server checks cookie first, then bearer).
export function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const withTimeout: RequestInit =
    init.signal != null ? init : { ...init, signal: AbortSignal.timeout(API_TIMEOUT_MS) };
  return fetch(input, withCredentials(withTimeout)).then((res) => {
    if (res.status === 401) notifyUnauthorized();
    else if (!res.ok) notifyApiProblem(input, res.status, res.statusText || `HTTP ${res.status}`);
    return res;
  }).catch((err) => {
    // A 401/abort is already carried by its own state; report transport
    // failures so an empty surface cannot be mistaken for a real empty.
    if (!(err instanceof DOMException && err.name === 'AbortError')) {
      notifyApiProblem(input, null, err instanceof Error ? err.message : String(err));
    }
    throw err;
  });
}

// Exported for ledger helpers and any surface that must go through the
// authenticated bridge path (no raw unauthed fetch to /api/*).
export { authHeaders };


export async function fetchSessions(): Promise<Session[]> {
  try {
    const data = await dedupedGet<{ sessions?: Session[] }>(`${API_BASE}/api/sessions`, { headers: authHeaders() });
    contract('sessions.list', data);
    return data.sessions || [];
  } catch (err) {
    console.error('Failed to fetch sessions:', err);
    return [];
  }
}

export async function fetchSession(sessionId: string): Promise<Session | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return contract('session', (await res.json()) as Session);
  } catch (err) {
    console.error('Failed to fetch session:', err);
    return null;
  }
}

export async function createSession(
  title: string,
  spendLimitUSD: number = 100,
  targetMolecule?: string,
  fresh?: boolean
): Promise<Session | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        title,
        spend_limit_usd: spendLimitUSD,
        target_molecule: targetMolecule,
        fresh: fresh ?? false,
      }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to create session:', err);
    return null;
  }
}

export async function deleteSession(sessionId: string): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}`, {
      headers: authHeaders(),
      method: 'DELETE',
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to delete session:', err);
    return false;
  }
}

export async function fetchMessages(sessionId: string): Promise<ChatMessage[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/${sessionId}/messages`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('messages.list', data);
    return data.messages || [];
  } catch (err) {
    console.error('Failed to fetch messages:', err);
    return [];
  }
}

export async function sendChatMessage(sessionId: string, content: string): Promise<{ user: ChatMessage; assistant: ChatMessage } | null> {  try {
    const res = await apiFetch(`${API_BASE}/api/chat/messages?session_id=${encodeURIComponent(sessionId)}`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, role: 'user', content }),
    });
      if (!res.ok) return null;
      const data = await res.json();
      const assistant = data.assistant || data.assistant_message;
      if (assistant && data.model && !assistant.model) assistant.model = data.model;
      if (assistant && data.duration_ms != null && assistant.duration_ms == null) {
        assistant.duration_ms = data.duration_ms;
      }
    return contract('chat.send', {
      user: data.user || data.user_message,
      assistant,
    });
  } catch (err) {
    console.error('Failed to send chat message:', err);
    return null;
  }
}

export interface StreamDone {
  user: ChatMessage;
  assistant: ChatMessage;
  proposal?: unknown;
  campaign?: import('@/types').CampaignState;
  model?: string;
  duration_ms?: number;
  usage?: Record<string, number>;
}

// Streaming twin of sendChatMessage: tokens arrive live via SSE
// (token events), then a done event carries the stored records.
// Throws on transport failure so the caller can fall back to unary.
export async function streamChatMessage(
  sessionId: string,
  content: string,
  onToken: (text: string) => void,
  signal?: AbortSignal
): Promise<StreamDone> {
  const res = await apiFetch(`${API_BASE}/api/chat/stream?session_id=${encodeURIComponent(sessionId)}`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ session_id: sessionId, role: 'user', content }),
    signal,
  });
  if (!res.ok || !res.body) throw new Error(`stream refused (HTTP ${res.status})`);
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let done: StreamDone | null = null;
  let lastFrameAt = Date.now();
  const stallAfter = (ms: number): Promise<never> =>
    new Promise((_, reject) => {
      const t = setTimeout(() => reject(new Error('stream stalled — retry')), ms);
      if (typeof (t as unknown as { unref?: unknown }).unref === 'function') {
        (t as unknown as { unref(): void }).unref();
      }
    });
  for (;;) {
    const remaining = STREAM_STALL_MS - (Date.now() - lastFrameAt);
    if (remaining <= 0) throw new Error('stream stalled — retry');
    const { value, done: readerDone } = await Promise.race([reader.read(), stallAfter(remaining)]);
    // Bytes arriving at all proves the transport is alive (server comment
    // heartbeats carry no data: frames); parsed frames refresh it too.
    // Only a fully silent socket trips the stall detector.
    if (value) {
      lastFrameAt = Date.now();
      buf += decoder.decode(value, { stream: true });
    }
    let idx: number;
    while ((idx = buf.indexOf('\n\n')) !== -1) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const event = /event:\s*(\S+)/.exec(frame)?.[1] || '';
      const dataRaw = frame
        .split('\n')
        .filter((l) => l.startsWith('data:'))
        .map((l) => l.slice(5).trim())
        .join('\n');
      if (!dataRaw) continue;
      if (event === 'token') {
        try {
          onToken((JSON.parse(dataRaw) as { text: string }).text || '');
        } catch {
          /* partial frame — next read completes it */
        }
      } else if (event === 'done') {
        const d = JSON.parse(dataRaw) as StreamDone;
        if (d.assistant && d.model && !d.assistant.model) d.assistant.model = d.model;
        done = d;
      } else if (event === 'error') {
        throw new Error((JSON.parse(dataRaw) as { error: string }).error || 'stream error');
      }
    }
    if (readerDone) break;
  }
  if (!done) throw new Error('stream ended without done event');
  return contract('chat.stream.done', done);
}

export async function runTribunal(
  sessionId: string,
  prompt: string
): Promise<{ tribunal: import('@/types').TribunalResult; assistant_message: ChatMessage; campaign?: import('@/types').CampaignState } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chat/tribunal`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, prompt }),
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      console.error(`Failed to run tribunal (${res.status}):`, text || res.statusText);
      return null;
    }
    return contract('tribunal.run', await res.json());
  } catch (err) {
    console.error('Failed to run tribunal:', err);
    return null;
  }
}

// T-16: resume a tribunal from an exported turn bundle (R4 flow). The
// route exists with no surface yet — this client is the future console
// flow's calling convention (session-scoped, bearer-or-cookie like
// runTribunal). Refusals surface verbatim, never null-swallowed.
export async function resumeTribunal(
  sessionId: string,
  prompt: string,
  bundle: unknown
): Promise<{ tribunal: import('@/types').TribunalResult; assistant_message: ChatMessage; campaign?: import('@/types').CampaignState } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/tribunal/resume`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, prompt, bundle }),
    });
    if (!res.ok) {
      const text = (await res.text()).trim().slice(0, 280);
      return { error: text || `HTTP ${res.status}` };
    }
    return await res.json();
  } catch (err) {
    console.error('Failed to resume tribunal:', err);
    return null;
  }
}

export interface LaunchProposalOptions {
  mandateRef?: string;
  requestId?: string;
  backend?: string;
  budgetUsd?: number;
  qasm?: string;
  grantChain?: string[];
  inputLabels?: string[];
  callerJurisdictions?: string[];
  callerExportClass?: string;
  exportReviewed?: boolean;
}

export async function launchProposal(
  sessionId: string,
  proposal: unknown,
  options: LaunchProposalOptions = {},
): Promise<{ job: Job; loop: Loop } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chat/launch`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: sessionId,
        proposal,
        ...(options.mandateRef !== undefined ? { mandate_ref: options.mandateRef } : {}),
        ...(options.requestId !== undefined ? { request_id: options.requestId } : {}),
        ...(options.backend !== undefined ? { backend: options.backend } : {}),
        ...(options.budgetUsd !== undefined ? { budget_usd: options.budgetUsd } : {}),
        ...(options.qasm !== undefined ? { qasm: options.qasm } : {}),
        ...(options.grantChain !== undefined ? { grant_chain: options.grantChain } : {}),
        ...(options.inputLabels !== undefined ? { input_labels: options.inputLabels } : {}),
        ...(options.callerJurisdictions !== undefined ? { caller_jurisdictions: options.callerJurisdictions } : {}),
        ...(options.callerExportClass !== undefined ? { caller_export_class: options.callerExportClass } : {}),
        ...(options.exportReviewed !== undefined ? { export_reviewed: options.exportReviewed } : {}),
      }),
    });
    // Refusals carry the backend reason as plain text — surface it, never
    // swallow it into a generic "refused" (2026-10-11: a tier-routing
    // refusal was invisible in the UI).
    if (!res.ok) {
      const text = (await res.text()).trim().slice(0, 280);
      return { error: text || `HTTP ${res.status}` };
    }
    return contract('chat.launch', await res.json());
  } catch (err) {
    console.error('Failed to launch proposal:', err);
    return null;
  }
}

export async function fetchLoops(sessionId?: string | null): Promise<Loop[]> {
  try {
    // K2: session-scoped when a session is active — the header counts
    // this session's loops, not the fleet's. Unset keeps the global view.
    const qs = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : '';
    const res = await apiFetch(`${API_BASE}/api/loops${qs}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('loops.list', data);
    return data.loops || [];
  } catch (err) {
    console.error('Failed to fetch loops:', err);
    return [];
  }
}

export async function fetchEvents(sessionId?: string): Promise<LedgerEvent[]> {
  if (!sessionId) return [];
  try {
    const url = `${API_BASE}/api/events?session_id=${encodeURIComponent(sessionId)}`;
    const res = await apiFetch(url, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('events.list', data);
    return data.events || [];
  } catch (err) {
    console.error('Failed to fetch events:', err);
    return [];
  }
}

export async function fetchMolecularADMET(smiles: string): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chemistry/admet?smiles=${encodeURIComponent(smiles)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch ADMET profile:', err);
    return null;
  }
}

export async function evaluateDiscoveryPipeline(
  target: string,
  candidates: Array<{ name: string; smiles: string }>
): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/discovery/pipeline`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ target, candidates }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to evaluate discovery pipeline:', err);
    return null;
  }
}

export async function fetchCampaignState(sessionId: string): Promise<import('@/types').CampaignState | null> {
  try {
    // Coalesce concurrent reads of the same session: the shell and the
    // copilot mount together and used to issue duplicate campaign GETs.
    return await dedupedGet<import('@/types').CampaignState>(
      `${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign`,
      { headers: authHeaders() },
    );
  } catch (err) {
    console.error('Failed to fetch campaign state:', err);
    return null;
  }
}

export async function synthesizeCampaign(sessionId: string, prompt: string): Promise<import('@/types').CampaignState | null> {
  // Refusal (HTTP !ok) returns null so the caller renders "refused".
  // Transport failure throws so the caller can distinguish it from refusal.
  const res = await apiFetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign/synthesize`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ prompt }),
  });
  if (!res.ok) return null;
  return await res.json();
}

export async function fetchAutoresearchHarnesses(): Promise<{ harnesses: import('@/types').AutoresearchHarness[]; docker_runner: Record<string, unknown> | null }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/harnesses`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { harnesses: [], docker_runner: null };
    return contract('autoresearch.harnesses', await res.json());
  } catch (err) {
    console.error('Failed to fetch autoresearch harnesses:', err);
    return { harnesses: [], docker_runner: null };
  }
}

export async function startAutoresearchLoop(
  harnessId: string,
  maxIterations: number = 20,
  initialArtifact?: string,
  sessionId?: string
): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/start`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: sessionId || '',
        harness_id: harnessId,
        max_iterations: maxIterations,
        initial_artifact: initialArtifact || '',
      }),
    });
    if (!res.ok) return null;
    return contract('autoresearch.loop', await res.json());
  } catch (err) {
    console.error('Failed to start autoresearch loop:', err);
    return null;
  }
}

export async function fetchAutoresearchLoops(): Promise<import('@/types').AutoresearchLoop[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/loops`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('autoresearch.loops', data);
    return data.loops || [];
  } catch (err) {
    console.error('Failed to fetch autoresearch loops:', err);
    return [];
  }
}

export async function stepAutoresearchLoop(loopId: string): Promise<{ step: import('@/types').IterationRecord; state: import('@/types').AutoresearchLoop } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/step`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return contract('autoresearch.step', await res.json());
  } catch (err) {
    console.error('Failed to step autoresearch loop:', err);
    return null;
  }
}

export async function pauseAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/pause`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return contract('autoresearch.loop', await res.json());
  } catch (err) {
    console.error('Failed to pause autoresearch loop:', err);
    return null;
  }
}

export async function resumeAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/resume`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return contract('autoresearch.loop', await res.json());
  } catch (err) {
    console.error('Failed to resume autoresearch loop:', err);
    return null;
  }
}

export async function stopAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/stop`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return contract('autoresearch.loop', await res.json());
  } catch (err) {
    console.error('Failed to stop autoresearch loop:', err);
    return null;
  }
}

export async function fetchMetasearchProposals(): Promise<import('@/types').MetaProposal[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/metasearch/proposals`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('metasearch.proposals', data);
    return data.proposals || [];
  } catch (err) {
    console.error('Failed to fetch metasearch proposals:', err);
    return [];
  }
}

export async function approveMetasearchProposal(
  proposalId: string,
  signer: string = 'console-operator'
): Promise<{ ok: true } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/metasearch/approve`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ proposal_id: proposalId, signer }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to approve proposal:', err);
    return null;
  }
}

export async function rejectMetasearchProposal(
  proposalId: string,
  signer: string = 'console-operator'
): Promise<{ ok: true } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/metasearch/reject`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ proposal_id: proposalId, signer }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to reject proposal:', err);
    return null;
  }
}

export async function applyMetasearchProposal(
  proposalId: string
): Promise<{ ok: true } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/metasearch/apply`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ proposal_id: proposalId }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to apply proposal:', err);
    return null;
  }
}

export async function fetchFleet(): Promise<Record<string, unknown>> {
  try {
    const res = await apiFetch(`${API_BASE}/api/fleet`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { backends: [], refused: true, refuse_reason: text || `HTTP ${res.status}` };
    }
    return contract('fleet', await res.json());
  } catch (err) {
    console.error('Failed to fetch fleet:', err);
    return { backends: [], refused: true, refuse_reason: String(err).slice(0, 280) };
  }
}

export async function fetchRadar(): Promise<Record<string, unknown>> {
  try {
    const res = await apiFetch(`${API_BASE}/api/radar`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { radar_endpoints: [], refused: true, refuse_reason: text || `HTTP ${res.status}` };
    }
    return contract('radar', await res.json());
  } catch (err) {
    console.error('Failed to fetch radar:', err);
    return { radar_endpoints: [], refused: true, refuse_reason: String(err).slice(0, 280) };
  }
}

export async function fetchNotebooks(sessionId: string): Promise<{ ok: true; notebooks: import('@/types').SystematicNotebook[] } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/results/notebooks?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || !Array.isArray(data.notebooks)) {
      return { ok: false, error: String(data?.error || data?.message || `HTTP ${res.status}`).slice(0, 280) };
    }
    contract('notebooks.list', data);
    return { ok: true, notebooks: data.notebooks as import('@/types').SystematicNotebook[] };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export interface CapabilitiesPayload {
  catalog_digest: string;
  tools?: unknown;
  disk?: Record<string, unknown>;
  harnesses?: string[];
  budget?: Record<string, number>;
  flags?: Record<string, boolean>;
}

export async function fetchCanvasLayout(sessionId: string): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/canvas/layout?session_id=${encodeURIComponent(sessionId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return contract('canvas.layout', (await res.json()) as Record<string, unknown>);
  } catch (err) {
    console.error('Failed to fetch canvas layout:', err);
    return null;
  }
}

export async function saveCanvasLayout(sessionId: string, layout: Record<string, unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/canvas/layout?session_id=${encodeURIComponent(sessionId)}`, {
      method: 'PUT',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(layout),
    });
    if (!res.ok) return { ok: false, error: `HTTP ${res.status}` };
    return { ok: true };
  } catch (err) {
    return { ok: false, error: String(err) };
  }
}

export async function fetchCapabilities(): Promise<CapabilitiesPayload | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/capabilities`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return contract('capabilities', (await res.json()) as CapabilitiesPayload);
  } catch (err) {
    console.error('Failed to fetch capabilities:', err);
    return null;
  }
}

export async function fetchFidelity(device: string, depth: number, twoQubitGates: number): Promise<Record<string, unknown> | null> {
  try {
    const qs = new URLSearchParams({ device, depth: String(depth), two_qubit_gates: String(twoQubitGates) });
    const res = await apiFetch(`${API_BASE}/api/fidelity?${qs.toString()}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return (await res.json()) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch fidelity:', err);
    return null;
  }
}

export async function fetchEpistemics(sessionId?: string | null): Promise<Record<string, unknown>> {
  try {
    const qs = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : '';
    const res = await apiFetch(`${API_BASE}/api/epistemics${qs}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return {};
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch epistemics:', err);
    return {};
  }
}

import type {
  CodesignBudgetEntry,
  CodesignBudgetResult,
  CodesignBudgetRow,
  CodesignFrontierEntry,
  CodesignFrontierResult,
  CodesignTQResult,
  CodesignVerifiedComponent,
  CodesignVerifiedResult,
  CodesignFamilyOffer,
  CodesignFamiliesResult,
  CodesignPack,
  CodesignPacksResult,
  CodesignRerunResult,
  CostSummary,
} from '@/types';

export type {
  CodesignBudgetEntry,
  CodesignBudgetResult,
  CodesignBudgetRow,
  CodesignFrontierEntry,
  CodesignFrontierResult,
  CodesignTQResult,
  CodesignVerifiedComponent,
  CodesignVerifiedResult,
  CodesignFamilyOffer,
  CodesignFamiliesResult,
  CodesignPack,
  CodesignPacksResult,
  CodesignRerunResult,
  CostSummary,
};


// Server-sealed Table-S4 union bound (replaces the cockpit's local estimate;
// single math source stays Python-owned behind the Go rail).
export async function scoreCodesignBudget(
  entries: CodesignBudgetEntry[]
): Promise<{ ok: true; result: CodesignBudgetResult } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/budget`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ entries }),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true) {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        (data && typeof data.error === 'string' && data.error) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    return { ok: true, result: data as CodesignBudgetResult };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

// Versioned verified-results document (G4): numbers served, never verdicts.
// Shapes live in @/types (single source); this module only fetches.
export async function fetchCodesignVerified(): Promise<
  | { ok: true; result: CodesignVerifiedResult }
  | { ok: false; error: string }
> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/verified`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || typeof data.total?.toffoli !== 'number') {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    contract('codesign.verified', data);
    return { ok: true, result: data as CodesignVerifiedResult };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function evaluateCodesignArtifact(
  body: Record<string, unknown>
): Promise<{ ok: true; spec_kind: string; result: Record<string, unknown> } | { ok: false; error: string }> {  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/evaluate`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true) {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        (data && typeof data.error === 'string' && data.error) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    return contract('codesign.evaluate', {
      ok: true,
      spec_kind: String(data.spec_kind ?? ''),
      result: (data.result ?? {}) as Record<string, unknown>,
    }) as { ok: true; spec_kind: string; result: Record<string, unknown> };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

// Read-only LLM spend summary (G10): server-sealed aggregates, heuristic.
// Pack registry (ux-plan slice 4): served live, never hand-typed.
// Promotion stays a privileged op; this rail is read-only.
export async function evaluateCodesignBatch(
  body: Record<string, unknown>
): Promise<{ ok: true; spec_kind: string; frontier: Record<string, unknown>; details: Array<Record<string, unknown>> } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/evaluate-batch`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true) {
      const msg = (data && (data.message || data.error)) || `refused (HTTP ${res.status})`;
      return { ok: false, error: String(msg).slice(0, 280) };
    }
    return contract('codesign.batch', {
      ok: true,
      spec_kind: String(data.spec_kind ?? ''),
      frontier: (data.frontier ?? {}) as Record<string, unknown>,
      details: Array.isArray(data.details) ? data.details as Array<Record<string, unknown>> : [],
    }) as { ok: true; spec_kind: string; frontier: Record<string, unknown>; details: Array<Record<string, unknown>> };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function fetchCodesignPacks(): Promise<
  { ok: true; result: CodesignPacksResult } | { ok: false; error: string }
> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/packs`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true || !Array.isArray(data.packs)) {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        (data && typeof data.error === 'string' && data.error) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    contract('codesign.packs', data);
    return { ok: true, result: data as CodesignPacksResult };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

// Evaluated family search (ux-plan slice 3): tracks mature separately,
// elites cross-pollinate, widen-nudge names the most stagnant track.
export async function runFamilySearch(
  body: Record<string, unknown>
): Promise<{ ok: true; result: CodesignFamiliesResult } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/families`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true) {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        (data && typeof data.error === 'string' && data.error) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    contract('codesign.families', data);
    return { ok: true, result: data as CodesignFamiliesResult };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

// C5b governed rerun: the Go rail owns the loop, BYOK estimate, and
// sealed receipt. `request_id` is required; reuse it to replay the same
// receipt after a transient failure (never mint a new id to "retry").
export async function rerunCodesignProposal(
  body: Record<string, unknown>
): Promise<{ ok: true; result: CodesignRerunResult } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/codesign/rerun`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.ok !== true) {
      const raw =
        (data && typeof data.message === 'string' && data.message) ||
        (data && typeof data.error === 'string' && data.error) ||
        `refused (HTTP ${res.status})`;
      const msg = res.status === 401 ? `login required — ${raw}` : raw;
      return { ok: false, error: msg.slice(0, 280) };
    }
    contract('codesign.rerun', data);
    return { ok: true, result: data as CodesignRerunResult };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function fetchCostSummary(): Promise<
  { ok: true; result: CostSummary } | { ok: false; error: string }
> {
  try {
    const res = await apiFetch(`${API_BASE}/api/cost/summary`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || typeof data.spent_usd !== 'number') {
      const msg =
        (data && typeof data.message === 'string' && data.message) ||
        `refused (HTTP ${res.status})`;
      return { ok: false, error: msg.slice(0, 280) };
    }
    contract('cost.summary', data);
    return { ok: true, result: data as CostSummary };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function fetchLLMSettings(): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/llm/settings`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch LLM settings:', err);
    return null;
  }
}

// Quantum provider selection (human surface). Secrets are never accepted
// here — credentials live in the daemon environment (KUBONA_* / Bao) and
// surface only as credential_state. The backend validates provider IDs
// and refuses unknown ones; the UI mirrors the refusal verbatim.
export interface QuantumBackendInfo {
  provider: string;
  display_name: string;
  modality: string;
  device: string;
  credential_env: string;
  credential_state: 'ready' | 'credentialless' | 'unavailable' | string;
  telemetry: 'measured' | 'declared' | string;
  executable_now: boolean;
  dispatch_ready?: boolean;
  input_format?: 'qasm' | 'qubo' | string;
}

export async function fetchQuantumBackends(): Promise<QuantumBackendInfo[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/backends`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = (await res.json()) as { backends?: QuantumBackendInfo[] };
    contract('quantum.backends', data);
    return Array.isArray(data.backends) ? data.backends : [];
  } catch (err) {
    console.error('Failed to fetch quantum backends:', err);
    return [];
  }
}

export async function fetchQuantumSettings(): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/settings`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch quantum settings:', err);
    return null;
  }
}

export interface QuantumCanaryResult {
  ok: boolean;
  results?: Record<string, unknown>;
  cached?: boolean;
  error?: string;
}

export async function runQuantumCanary(providers?: string[], maxSeconds?: number): Promise<QuantumCanaryResult> {
  try {
    const params = new URLSearchParams();
    if (providers && providers.length > 0) params.set('providers', providers.join(','));
    if (maxSeconds) params.set('max_seconds', String(maxSeconds));
    const qs = params.toString();
    const res = await apiFetch(`${API_BASE}/api/quantum/canary${qs ? `?${qs}` : ''}`, { cache: 'no-store', headers: authHeaders() });
    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
    if (!res.ok) return { ok: false, error: (data.error as string) || `HTTP ${res.status}` };
    return { ok: true, results: (data.results ?? {}) as Record<string, unknown>, cached: data.cached as boolean | undefined };
  } catch (err) {
    console.error('Failed to run quantum canary:', err);
    return { ok: false, error: String(err) };
  }
}

export async function saveQuantumSettings(
  settings: Record<string, unknown>
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/settings`, {
      method: 'PUT',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(settings),
    });
    if (!res.ok) {
      // Surface the backend reason verbatim — never swallow into generic false.
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore body read failure */
      }
      return { ok: false, error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to save quantum settings:', err);
    return { ok: false, error: String(err) };
  }
}

export interface QuantumJobSubmit {
  session_id?: string;
  qasm: string;
  provider: string;
  shots?: number;
  idempotency_key: string;
  dry_run?: boolean;
  mandate_ref?: string;
  caller_jurisdictions?: string[];
  caller_export_class?: string;
  require_review?: boolean;
}

export async function submitQuantumJob(
  job: QuantumJobSubmit
): Promise<{ ok: boolean; job_id?: string; queued?: boolean; dry_run?: boolean; estimate?: Record<string, unknown>; fidelity?: Record<string, unknown>; session_id?: string; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/jobs`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(job),
    });
    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
    if (!res.ok) return { ok: false, error: (data.error as string) || `HTTP ${res.status}` };
    return data as { ok: boolean; job_id?: string };
  } catch (err) {
    console.error('Failed to submit quantum job:', err);
    return { ok: false, error: String(err) };
  }
}

export async function fetchQuantumJob(jobId: string): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/jobs/${encodeURIComponent(jobId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch quantum job:', err);
    return null;
  }
}

export async function cancelQuantumJob(jobId: string, reason?: string): Promise<{ ok: boolean; error?: string; provider_recall_attempted?: boolean; provider_recall_status?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/quantum/jobs/${encodeURIComponent(jobId)}/cancel`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ reason: reason ?? '' }),
    });
    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
    if (!res.ok) return { ok: false, error: (data.error as string) || `HTTP ${res.status}` };
    return data as { ok: boolean };
  } catch (err) {
    console.error('Failed to cancel quantum job:', err);
    return { ok: false, error: String(err) };
  }
}

export async function saveLLMSettings(
  settings: Record<string, unknown>
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/llm/settings`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(settings),
    });
    if (!res.ok) {
      // Surface the backend reason verbatim — never swallow into generic false.
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore body read failure */
      }
      return { ok: false, error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to save LLM settings:', err);
    return { ok: false, error: String(err) };
  }
}

export async function fetchLLMModels(provider: string = 'omlx'): Promise<Record<string, unknown>> {
  try {
    const res = await apiFetch(`${API_BASE}/api/llm/models?provider=${encodeURIComponent(provider)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { models: [] };
    return contract('llm.models', await res.json());
  } catch (err) {
    console.error('Failed to fetch LLM models:', err);
    return { models: [] };
  }
}

export async function testLLM(prompt: string, role: string = 'copilot'): Promise<Record<string, unknown> | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/llm/test`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ role, prompt }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to test LLM:', err);
    return null;
  }
}

// -----------------------------------------------------------------------------
// Hugging Face Hub & In-Platform Downloader API
// -----------------------------------------------------------------------------

export async function searchHFModels(
  query: string = '',
  library: string = 'gguf',
  limit: number = 15
): Promise<import('@/types').HFModelSummary[]> {
  try {
    const url = `${API_BASE}/api/hf/search?q=${encodeURIComponent(query)}&library=${encodeURIComponent(library)}&limit=${limit}`;
    const res = await apiFetch(url, { cache: 'no-store', headers: authHeaders() });
    // Audit 2026-10-21: a bare `return []` here turned every refusal
    // (401 without an operator token, 429, 5xx) into "No models found"
    // — a lie the hub surface rendered as an empty result. Refusals
    // throw so callers toast them; only a genuine empty hub reads empty.
    if (!res.ok) throw new Error(`hub refused search (HTTP ${res.status}): no operator token or hub unreachable`);
    const data = await res.json();
    contract('hf.search', data);
    return data.models || [];
  } catch (err) {
    if (err instanceof Error && err.message.startsWith('hub refused')) throw err;
    console.error('Failed to search Hugging Face models:', err);
    return [];
  }
}

export async function getHFModelDetails(
  repoId: string
): Promise<import('@/types').HFModelSummary | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/hf/models/${encodeURIComponent(repoId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    const data = await res.json();
    contract('hf.details', data);
    return data.model || null;
  } catch (err) {
    console.error('Failed to fetch HF model details:', err);
    return null;
  }
}

export async function startHFDownload(
  repoId: string,
  filename: string,
  // T-11: client-declared size + explicit confirmation. The server
  // enforces the same bound, so the confirm dialog is not bypassable
  // via raw POST; unknown sizes omit the field and pass ungated.
  sizeBytes?: number,
  confirmed?: boolean
): Promise<import('@/types').HFDownloadTask | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/hf/download`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        repo_id: repoId,
        filename,
        ...(typeof sizeBytes === 'number' && Number.isFinite(sizeBytes) ? { size_bytes: sizeBytes } : {}),
        ...(confirmed ? { confirmed: true } : {}),
      }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    contract('hf.download', data);
    return data.task || null;
  } catch (err) {
    console.error('Failed to start HF download:', err);
    return null;
  }
}

export async function listHFDownloads(): Promise<import('@/types').HFDownloadTask[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/hf/downloads`, { cache: 'no-store', headers: authHeaders() });
    // Same audit rule as search: a refused list is not an empty list.
    // Pollers already swallow rejections silently; manual callers toast.
    if (!res.ok) throw new Error(`hub refused download list (HTTP ${res.status}): no operator token or hub unreachable`);
    const data = await res.json();
    contract('hf.downloads', data);
    return data.downloads || [];
  } catch (err) {
    if (err instanceof Error && err.message.startsWith('hub refused')) throw err;
    console.error('Failed to list HF downloads:', err);
    return [];
  }
}

export async function cancelHFDownload(taskId: string): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/hf/downloads/${encodeURIComponent(taskId)}/cancel`, {
      headers: authHeaders(),
      method: 'POST',
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to cancel HF download:', err);
    return false;
  }
}

// -----------------------------------------------------------------------------
// Custom Agents API
// -----------------------------------------------------------------------------

export interface SystemAgentRole {
  role: string;
  description: string;
  requires_human_ap2?: boolean;
}

export async function fetchSystemAgents(): Promise<SystemAgentRole[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/agents`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = (await res.json()) as { roles?: SystemAgentRole[] };
    contract('agents.system', data);
    return Array.isArray(data.roles) ? data.roles : [];
  } catch (err) {
    console.error('Failed to fetch system agents:', err);
    return [];
  }
}

export async function fetchCustomAgents(): Promise<import('@/types').CustomAgentManifest[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/agents/custom`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    contract('agents.custom', data);
    return data.agents || [];
  } catch (err) {
    console.error('Failed to fetch custom agents:', err);
    return [];
  }
}

export async function createCustomAgent(
  manifest: import('@/types').CustomAgentManifest
): Promise<import('@/types').CustomAgentManifest> {
  // Audit 2026-10-21: returning null on refusal made the mesh modal
  // close in silent success theater (roster unchanged, no message).
  // Refusals throw — the caller's catch toasts them honestly.
  const res = await apiFetch(`${API_BASE}/api/agents/custom`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(manifest),
  });
  if (!res.ok) {
    const detail = await res.text().catch(() => '');
    throw new Error(`agent registration refused (HTTP ${res.status}): ${detail.slice(0, 160) || 'operator token required'}`);
  }
  const data = await res.json();
  if (!data.agent) throw new Error('agent registration returned no agent: nothing registered');
  return data.agent;
}

export async function deleteCustomAgent(agentId: string): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/agents/custom/${encodeURIComponent(agentId)}`, {
      headers: authHeaders(),
      method: 'DELETE',
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to delete custom agent:', err);
    return false;
  }
}

export async function routeMultimodalPayload(
  payload: import('@/types').MultimodalPayload
): Promise<import('@/types').RoutingDecision | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/mesh/route`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(payload),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return data.decision || null;
  } catch (err) {
    console.error('Failed to route multimodal payload:', err);
    return null;
  }
}

export async function planWorkflow(payload: {
  session_id?: string;
  workflow: import('@/types').WorkflowDefinition;
  inputs?: Record<string, unknown>;
}): Promise<import('@/types').WorkflowPlanResponse> {
  const res = await apiFetch(`${API_BASE}/api/workflows/plan`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const detail = await res.text().catch(() => '');
    throw new Error(`workflow plan refused (HTTP ${res.status}): ${detail.slice(0, 160)}`);
  }
  return contract('workflow.plan', await res.json());
}

export async function runWorkflow(payload: {
  session_id: string;
  workflow: import('@/types').WorkflowDefinition;
  inputs?: Record<string, unknown>;
  force?: boolean;
}): Promise<import('@/types').WorkflowRunResponse> {
  const res = await apiFetch(`${API_BASE}/api/workflows/run`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const detail = await res.text().catch(() => '');
    throw new Error(`workflow run refused (HTTP ${res.status}): ${detail.slice(0, 160)}`);
  }
  return contract('workflow.run', await res.json());
}

// ==========================================
// AP2 SPEND MANDATE CLIENT METHODS
// ==========================================

export interface SpendMandateSignRequest {
  tenant_id: string;
  job_hash: string;
  max_budget_usd: number;
  ttl_seconds?: number;
}

export interface SpendMandateSignResponse {
  mandate: import('./mandates').SpendMandate;
  public_key: string;
  verified: boolean;
}

export interface SpendMandateVerifyResponse {
  valid: boolean;
  mandate?: import('./mandates').SpendMandate;
  error?: string;
  verified_at?: string;
}

export async function requestServerSignedMandate(
  req: SpendMandateSignRequest
): Promise<SpendMandateSignResponse | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/mandates/sign`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(req),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to request server signed mandate:', err);
    return null;
  }
}

export async function verifySpendMandate(
  mandate: import('./mandates').SpendMandate
): Promise<SpendMandateVerifyResponse> {
  try {
    const res = await apiFetch(`${API_BASE}/api/mandates/verify`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(mandate),
    });
    if (!res.ok) {
      return { valid: false, error: `HTTP ${res.status}: ${res.statusText}` };
    }
    return await res.json();
  } catch (err) {
    console.error('Failed to verify spend mandate:', err);
    return { valid: false, error: String(err) };
  }
}






export async function fetchLabs(): Promise<Lab[]> {
  try {
    const data = await dedupedGet<{ tenants?: Lab[] }>(`${API_BASE}/api/tenants`, { headers: authHeaders() });
    contract('labs.list', data);
    return data.tenants || [];
  } catch (err) {
    console.error('Failed to fetch labs:', err);
    return [];
  }
}

// Ring-0 reflex intercept: settles every unsettled job in the session as
// failed and seals the abort in the ledger. Returns count + receipt.
export async function emergencyAbort(
  sessionId: string,
  reason = 'emergency abort from console'
): Promise<{ aborted_count: number; abort_receipt: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/supervisor/abort`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, reason }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Abort failed and nothing was settled:', err);
    return null;
  }
}

export async function searchLiterature(query: string, limit = 5): Promise<{ records: import('@/types').LiteratureRecord[]; cached: boolean }> {  try {
    const res = await apiFetch(
      `${API_BASE}/api/literature?query=${encodeURIComponent(query)}&limit=${limit}`,
      { cache: 'no-store', headers: authHeaders() }
    );
    if (!res.ok) return { records: [], cached: false };
    const data = await res.json();
    return { records: data.records || [], cached: !!data.cached };
  } catch (err) {
    console.error('Failed to search literature:', err);
    return { records: [], cached: false };
  }
}

// Clear-history: deletes every chat message in the session. Session +
// campaign + ledger history survive; the erasure is sealed server-side.
export async function clearMessages(sessionId: string): Promise<number | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/messages`, {
      method: 'DELETE',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return typeof data.deleted === 'number' ? data.deleted : 0;
  } catch (err) {
    console.error('Failed to clear messages:', err);
    return null;
  }
}

// Regenerate: drops trailing assistant messages and re-answers the last
// user prompt (no duplicated user turn). Same envelope as chat POST.
export async function regenerateChat(
  sessionId: string
): Promise<{ assistant: ChatMessage; model?: string; duration_ms?: number } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chat/regenerate`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    const assistant = data.assistant || data.assistant_message;
    if (assistant && data.model && !assistant.model) assistant.model = data.model;
    if (assistant && data.duration_ms != null && assistant.duration_ms == null) {
      assistant.duration_ms = data.duration_ms;
    }
    return { assistant, model: data.model, duration_ms: data.duration_ms };
  } catch (err) {
    console.error('Failed to regenerate:', err);
    return null;
  }
}

// Feedback: records a vote on an assistant message as a ledger event.
export async function sendFeedback(
  sessionId: string,
  messageId: string,
  vote: 'up' | 'down'
): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chat/feedback`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, message_id: messageId, vote }),
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to send feedback:', err);
    return false;
  }
}

// Rewrite (edit-resend branching): replaces a user message and deletes every
// later message, restarting the thread from the edited turn.
export async function rewriteChat(
  sessionId: string,
  messageId: string,
  content: string
): Promise<ChatMessage | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/chat/rewrite`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, message_id: messageId, content }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return data.message || null;
  } catch (err) {
    console.error('Failed to rewrite message:', err);
    return null;
  }
}

// Job status wire format (audit 2026-10-11): the backend marshals the
// proto int32 enum (1/2/3/4…), not names. Map both numeric wire values and
// DB/string forms (JOB_STATUS_COMPLETED) to canonical labels. Unknown
// values pass through verbatim — never invented.
const JOB_STATUS_LABELS: Record<number, string> = {
  0: 'UNSPECIFIED',
  1: 'QUEUED',
  2: 'RUNNING',
  3: 'COMPLETED',
  4: 'FAILED',
  5: 'PAUSED',
  6: 'CANCELLED',
};

export function jobStatusLabel(status: unknown): string {
  if (typeof status === 'number') return JOB_STATUS_LABELS[status] ?? String(status);
  if (typeof status === 'string') {
    const upper = status.toUpperCase().replace(/^JOB_STATUS_/, '');
    if (['QUEUED', 'RUNNING', 'COMPLETED', 'FAILED', 'PAUSED', 'CANCELLED', 'UNSPECIFIED'].includes(upper)) {
      return upper;
    }
    return status;
  }
  return 'UNKNOWN';
}

export function isTerminalJobStatus(status: unknown): boolean {
  const label = jobStatusLabel(status);
  return label === 'COMPLETED' || label === 'FAILED' || label === 'CANCELLED';
}

// Jobs: session-scoped run list for inline launch tracking.
export async function fetchJobs(sessionId: string): Promise<import('@/types').Job[]> {
  try {
    const res = await apiFetch(`${API_BASE}/api/jobs?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return [];
    const data = await res.json();
    contract('jobs.list', data);
    return data.jobs || [];
  } catch (err) {
    console.error('Failed to fetch jobs:', err);
    return [];
  }
}

// Job detail: full params + result payloads for the click-through view.
// Unknown ids 404 → null (never an empty shell). sessionId is REQUIRED:
// the backend scopes jobs per tenant session and refuses unscopend reads
// (400) — the client never makes a call it knows will fail.
export async function fetchJob(jobId: string, sessionId: string): Promise<import('@/types').Job | null> {
  if (!sessionId) return null;
  try {
    const res = await apiFetch(
      `${API_BASE}/api/jobs/${encodeURIComponent(jobId)}?session_id=${encodeURIComponent(sessionId)}`,
      {
        cache: 'no-store',
        headers: authHeaders(),
      }
    );
    if (!res.ok) return null;
    const data = await res.json();
    contract('job', data);
    return data.job || null;
  } catch (err) {
    console.error('Failed to fetch job:', err);
    return null;
  }
}

export interface MemorySnippet {
  content: string;
  bank: string;
  similarity: number | null;
  created_at: string;
}

export async function fetchMemoryRecall(
  sessionId: string,
  query: string,
  opts?: { bank?: string; limit?: number }
): Promise<{ snippets: MemorySnippet[]; tenant: string } | null> {
  try {
    const params = new URLSearchParams({ session_id: sessionId, q: query });
    if (opts?.bank) params.set('bank', opts.bank);
    if (opts?.limit) params.set('limit', String(opts.limit));
    const res = await apiFetch(`${API_BASE}/api/memory/recall?${params.toString()}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    return contract('memory.recall', await res.json());
  } catch (err) {
    console.error('Failed to recall memory:', err);
    return null;
  }
}

export async function retainMemory(
  sessionId: string,
  content: string,
  opts?: { bank?: string; importance?: number }
): Promise<{ id: string; bank: string; importance: number } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/memory/retain`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: sessionId,
        content,
        bank: opts?.bank || 'experiences',
        ...(opts?.importance !== undefined ? { importance: opts.importance } : {}),
      }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to retain memory:', err);
    return null;
  }
}

export interface QirCompileReceipt {
  num_qubits: number;
  depth: number;
  gate_count: number;
  t_count: number;
  qir_profile: string;
  llvm_ir?: string;
  compiler_used: string;
  fallback_estimate: boolean;
}

export async function fetchQirCompile(qasm: string): Promise<QirCompileReceipt | null> {
  // Refusal (HTTP !ok) returns null so the caller renders "refused".
  // Transport failure throws so the caller can distinguish it from refusal.
  const res = await apiFetch(`${API_BASE}/api/qir/compile`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ qasm }),
  });
  if (!res.ok) return null;
  return contract('qir.compile', await res.json());
}

export interface SessionContext {
  session_id: string;
  turns: number;
  turn_cap: number;
  history_chars: number;
  history_chars_cap: number;
  would_drop: number;
  pressure_pct: number;
  prompt_tokens: number;
  completion_tokens: number;
}

// S8a: measured transcript pressure against the server's real truncation
// rule (BudgetHistory caps) — never a vendor window. Null when the
// backend is unreachable (the readout hides; data failures stay loud in
// the Go tests, not in the composer).
export async function fetchSessionContext(sessionId: string): Promise<SessionContext | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/context?session_id=${encodeURIComponent(sessionId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return contract('session.context', await res.json());
  } catch (err) {
    console.error('Failed to fetch session context:', err);
    return null;
  }
}

// Ledger append via the authenticated bridge path (no raw fetch). Surfaces
// the backend refusal body via res.text() on !ok; throws on transport
// failure so callers can separate transport-null from refusal.
export async function appendLedgerEvent(
  sessionId: string,
  eventType: string,
  payload: Record<string, unknown>
): Promise<{ ok: boolean; record_hash?: string; event_id?: string | number; error?: string }> {
  const res = await apiFetch(`${API_BASE}/api/ledger/append`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ session_id: sessionId, event_type: eventType, payload }),
  });
  if (!res.ok) {
    let text = '';
    try {
      text = (await res.text()).trim().slice(0, 280);
    } catch {
      /* ignore */
    }
    return { ok: false, error: text || `ledger refused the commit (HTTP ${res.status})` };
  }
  const data = await res.json();
  if (!data?.ok || !data?.record_hash) {
    return { ok: false, error: (data?.error as string) || 'ledger returned no record hash' };
  }
  return { ok: true, record_hash: data.record_hash, event_id: data.event_id };
}

export interface BackendStatus {
  reachable: boolean;
  ready: boolean;
  service?: string;
  version?: string;
  diskLevel?: string;
  diskReason?: string;
  message?: string;
}

// Backend liveness for the status banner (U1-a). Three states, never
// conflated: unreachable (transport failed — surfaces may be showing
// cached/empty data), degraded (health 200 but ready 503 — the reason
// rides along), healthy. Distinguishes "empty" from "backend down".
export async function fetchBackendStatus(): Promise<BackendStatus> {
  let health: Record<string, unknown>;
  try {
    const res = await apiFetch(`${API_BASE}/api/health`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { reachable: false, ready: false };
    health = (await res.json()) as Record<string, unknown>;
  } catch {
    return { reachable: false, ready: false };
  }
  let ready = true;
  let message: string | undefined;
  try {
    const res = await apiFetch(`${API_BASE}/api/ready`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) {
      ready = false;
      try {
        const body = (await res.json()) as { message?: unknown; code?: unknown };
        message =
          typeof body.message === 'string'
            ? body.message
            : typeof body.code === 'string'
              ? body.code
              : `HTTP ${res.status}`;
      } catch {
        message = `HTTP ${res.status}`;
      }
    }
  } catch {
    return { reachable: false, ready: false };
  }
  const str = (v: unknown): string | undefined => (typeof v === 'string' ? v : undefined);
  return {
    reachable: true,
    ready,
    service: str(health.service),
    version: str(health.version),
    diskLevel: str(health.disk_level),
    diskReason: str(health.disk_reason),
    message,
  };
}

export type LoopAction = 'pause' | 'resume' | 'kill';

// U1-c: loop override (watch-and-interrupt). POSTs the supervisor action;
// refusals carry the backend reason as text (unknown id, illegal
// transition), transport failure returns null so the caller can tell
// "refused" from "unreachable".
export async function loopAction(
  loopId: string,
  action: LoopAction
): Promise<{ ok: true } | { error: string } | null> {
  try {
    const res = await apiFetch(
      `${API_BASE}/api/loops/${encodeURIComponent(loopId)}/${action}`,
      { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json' }) }
    );
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore body read failure */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error(`Failed loop ${action}:`, err);
    return null;
  }
}

export interface TenantWriteResult {
  ok: true;
  tenant: import('@/types').Lab;
}

export interface TenantWriteError {
  error: string;
}

// Shared POST helper for the bearer-gated, admin-enveloped tenant
// mutations (U1-b). Refusals carry the backend reason verbatim
// (not-admin, expired, suspended, rate-limited…); transport failure
// returns null so the UI can tell "refused" from "unreachable".
async function postTenantMutation(
  path: string,
  body: Record<string, unknown>,
  operatorToken?: string,
): Promise<TenantWriteResult | TenantWriteError | null> {
  try {
    const token = (operatorToken ?? '').trim();
    const res = await apiFetch(`${API_BASE}${path}`, {
      method: 'POST',
      headers: authHeaders({
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      }),
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore body read failure */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    const data = await res.json();
    return { ok: true, tenant: data as import('@/types').Lab };
  } catch (err) {
    console.error(`Tenant mutation ${path} failed:`, err);
    return null;
  }
}

// U1-b: tenant ceiling / suspend-resume / refund / admin-key bootstrap.
// Each takes a minted AdminAction envelope (see lib/adminAction) — the
// bearer token proves operator possession, the envelope proves lab-admin
// authority over the exact parameters. Bootstrap (set_admin_key on a lab
// with no registered admin) takes no envelope: first write wins.
export function setTenantCeiling(
  tenantId: string,
  ceilingUsd: number,
  action?: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/ceiling', {
    tenant_id: tenantId,
    ceiling_usd: ceilingUsd,
    ...(action ? { action } : {}),
  });
}

export function setTenantStatus(
  tenantId: string,
  status: 'active' | 'suspended',
  action?: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/status', {
    tenant_id: tenantId,
    status,
    ...(action ? { action } : {}),
  });
}

export function refundTenantSpend(
  tenantId: string,
  amountUsd: number,
  reason: string,
  action?: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/refund', {
    tenant_id: tenantId,
    amount_usd: amountUsd,
    reason,
    ...(action ? { action } : {}),
  });
}

export function registerTenantAdminKey(
  tenantId: string,
  newAdminPubkeyHex: string,
  action?: import('./adminAction').AdminAction,
  operatorToken?: string,
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/admin-key', {
    tenant_id: tenantId,
    new_admin_pubkey_hex: newAdminPubkeyHex,
    ...(action ? { action } : {}),
  }, operatorToken);
}

export async function createLab(id: string, displayName: string): Promise<{ ok: true; tenant: Lab } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/tenants`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ id, display_name: displayName }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true, tenant: data as Lab };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function refundSessionSpend(
  sessionId: string,
  amountUsd: number,
  reason: string,
  action?: import('./adminAction').AdminAction,
): Promise<{ ok: true } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/refund`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: sessionId, amount_usd: amountUsd, reason,
        ...(action ? { action } : {}),
      }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export function setTenantAutonomy(
  tenantId: string,
  level: 'L0' | 'L1' | 'L2' | 'L3',
  action?: import('./adminAction').AdminAction,
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/autonomy', {
    tenant_id: tenantId,
    level,
    ...(action ? { action } : {}),
  });
}

export function revokeTenantAutonomy(
  tenantId: string,
  reason: string,
  action?: import('./adminAction').AdminAction,
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/revoke', {
    tenant_id: tenantId,
    reason,
    ...(action ? { action } : {}),
  });
}

export async function issueTenantGrant(body: Record<string, unknown>): Promise<{ ok: true; grant: Record<string, unknown> } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/tenant-grants/issue`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true, grant: data as Record<string, unknown> };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function revokeTenantGrant(body: Record<string, unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/tenant-grants/revoke`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export interface ObligationRow {
  id: string;
  subject_kind: string;
  subject_id: string;
  owner: string;
  disposition: string;
  required_cases: string[];
  required_authorities: string[];
  close_ready: boolean;
  close_reasons: string[];
}

export async function fetchOpenObligations(): Promise<{ ok: true; obligations: ObligationRow[] } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/obligations/open`, { cache: 'no-store', headers: authHeaders() });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || !Array.isArray(data.obligations)) {
      return { ok: false, error: (data?.error || data?.message || `HTTP ${res.status}`).slice(0, 280) };
    }
    contract('obligations.open', data);
    return { ok: true, obligations: data.obligations as ObligationRow[] };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function closeObligation(id: string): Promise<{ ok: boolean; error?: string; reasons?: string[]; closed?: boolean }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/obligations/${encodeURIComponent(id)}`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({}),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true, closed: data.closed === true, reasons: data.reasons };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export interface VaultLease {
  lease_id: string;
  scope: string;
  ttl_seconds: number;
  lease_signature: string;
  issued_at: string;
  expires_at: string;
  session_id?: string;
  tenant_id?: string;
}

export async function issueVaultLease(scope: string, ttlSeconds: number, sessionId?: string): Promise<{ ok: true; lease: VaultLease } | { ok: false; error: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/leases`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ scope, ttl_seconds: ttlSeconds, ...(sessionId ? { session_id: sessionId } : {}) }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true, lease: data as VaultLease };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function revokeVaultLease(leaseId: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/leases/revoke`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ lease_id: leaseId }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: true };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export async function verifyLedger(sessionId?: string): Promise<{ ok: boolean; fault_count?: number; faults?: unknown[]; error?: string }> {
  try {
    const qs = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : '';
    const res = await apiFetch(`${API_BASE}/api/ledger/verify${qs}`, { cache: 'no-store', headers: authHeaders() });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: String(data.error || data.message || `HTTP ${res.status}`).slice(0, 280) };
    return { ok: data.ok === true, fault_count: data.fault_count, faults: data.faults };
  } catch (err) {
    return { ok: false, error: String(err).slice(0, 280) };
  }
}

export interface SessionExport {
  ok: true;
  manifest: unknown;
}

export interface SessionImportResult {
  ok: true;
  session_id: string;
  replayed: boolean;
}

// U2-a: session export/import + ledger export (FlightRecorder home).
// Export returns the manifest for client-side download; import posts a
// parsed manifest (unknown ids 404, invalid manifests 400 — reasons
// verbatim); ledger export returns JSON events or a markdown rendering
// (cross-tenant selections refuse whole, never redacted).
export async function exportSession(sessionId: string): Promise<SessionExport | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/export`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true, manifest: (await res.json()) as unknown };
  } catch (err) {
    console.error('Failed to export session:', err);
    return null;
  }
}

export async function importSession(manifest: unknown): Promise<SessionImportResult | { error: string } | null> {
  let raw = '';
  try {
    const res = await apiFetch(`${API_BASE}/api/sessions/import`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ manifest }),
    });
    try {
      raw = (await res.text()).trim();
    } catch {
      raw = '';
    }
    if (!res.ok) {
      return { error: raw.slice(0, 280) || `HTTP ${res.status}` };
    }
    let data: { session_id?: unknown; replayed?: unknown } | null = null;
    try {
      data = JSON.parse(raw) as { session_id?: unknown; replayed?: unknown };
    } catch {
      data = null;
    }
    if (!data || typeof data.session_id !== 'string') {
      return { error: 'import returned no session id' };
    }
    return { ok: true, session_id: data.session_id, replayed: data.replayed === true };
  } catch (err) {
    console.error('Failed to import session:', err);
    return null;
  }
}

export type LedgerExportFormat = 'json' | 'markdown';

export async function exportLedger(
  sessionId: string,
  format: LedgerExportFormat
): Promise<{ ok: true; events?: unknown[]; markdown?: string } | { error: string } | null> {
  let raw = '';
  let status = 0;
  try {
    const res = await apiFetch(`${API_BASE}/api/ledger/export`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, format }),
    });
    status = res.status;
    try {
      raw = (await res.text()).trim();
    } catch {
      raw = '';
    }
    if (!res.ok) {
      return { error: raw.slice(0, 280) || `HTTP ${status}` };
    }
    let data: { events?: unknown[]; markdown?: string; ok?: boolean; error?: string } | null = null;
    try {
      data = JSON.parse(raw) as { events?: unknown[]; markdown?: string; ok?: boolean; error?: string };
    } catch {
      data = null;
    }
    if (!data || data.ok === false) {
      return { error: (data && data.error) || 'ledger refused the export' };
    }
    return { ok: true, events: data.events, markdown: data.markdown };
  } catch (err) {
    console.error('Failed to export ledger:', err);
    return null;
  }
}

// Client-side download for exports (U2-a). DOM-guarded: returns false
// when there is no document (non-browser runtimes) so callers can fall
// back to inline display instead of crashing.
export function downloadFile(filename: string, text: string, mime: string): boolean {
  if (typeof document === 'undefined') return false;
  try {
    const blob = new Blob([text], { type: mime });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    return true;
  } catch {
    return false;
  }
}

export interface AuthChallenge {
  nonce: string;
  message: string;
  expires_at: string;
}

export interface AuthSession {
  authenticated: boolean;
  tenant_id?: string;
  pubkey_fp?: string;
}

export interface AuthServerSession {
  id: string;
  pubkey_fp: string;
  created_at: string;
  expires_at: string;
}

// Session auth (A3/A4): challenge-response login bound to a lab admin key.
// Transport failures return null (unreachable); refusals carry the backend
// reason. The session cookie is httpOnly — JS never sees the token;
// whoami is how the UI learns who it is.
export async function authChallenge(): Promise<AuthChallenge | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/challenge`, { method: 'POST' });
    if (!res.ok) return { error: `HTTP ${res.status}` };
    return contract('auth.challenge', (await res.json()) as AuthChallenge);
  } catch (err) {
    console.error('Failed to fetch auth challenge:', err);
    return null;
  }
}

export async function authLogin(
  tenantId: string,
  pubkeyHex: string,
  nonce: string,
  signatureHex: string
): Promise<{ ok: true; tenant_id: string; pubkey_fp: string } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/login`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ tenant_id: tenantId, pubkey_hex: pubkeyHex, nonce, signature_hex: signatureHex }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return (await res.json()) as { ok: true; tenant_id: string; pubkey_fp: string };
  } catch (err) {
    console.error('Failed to log in:', err);
    return null;
  }
}

export async function authLogout(): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/logout`, { method: 'POST' });
    return res.ok;
  } catch (err) {
    console.error('Failed to log out:', err);
    return false;
  }
}

export async function authWhoami(): Promise<AuthSession | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/whoami`, { cache: 'no-store' });
    if (!res.ok) return null;
    return contract('auth.whoami', (await res.json()) as AuthSession);
  } catch (err) {
    console.error('Failed to check session:', err);
    return null;
  }
}

export async function authSessions(): Promise<AuthServerSession[] | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/sessions`, { cache: 'no-store' });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    const data = (await res.json()) as { sessions?: AuthServerSession[] };
    return data.sessions || [];
  } catch (err) {
    console.error('Failed to list sessions:', err);
    return null;
  }
}

export async function authRevoke(id: string): Promise<{ ok: true } | { error: string } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/revoke`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ id }),
    });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { error: text || `HTTP ${res.status}` };
    }
    return { ok: true };
  } catch (err) {
    console.error('Failed to revoke session:', err);
    return null;
  }
}

// Same-origin rendering for Atlas lens bytes. The browser CSP stays
// img-src 'self'; the Next route forwards the session cookie to Go.
export function lensImageProxyUrl(url: string | null | undefined): string | null {
  if (!url) return null;
  if (!/^https?:\/\//i.test(url)) return url; // data:/blob: stays inline
  return `/api/lens-image?url=${encodeURIComponent(url)}`;
}

export async function requestObservatoryLens(params: {
  sessionId: string;
  targetSystem: string;
  prompt?: string;
  taskType?: string;
  imageUrl?: string;
  depthAssetId?: string;
  camera?: { position: number[]; orientation: number[] };
  voxelSize?: number;
  ephemeralPurge?: boolean;
}): Promise<{
  ok: boolean;
  operation_id?: string;
  watermark?: string;
  cost_usd?: number;
  idempotency_key?: string;
  error?: string;
}> {
  try {
    // One idempotency key per logical request: transport retries replay the
    // same Atlas operation instead of minting duplicate $0.50 dispatches.
    const idemKey =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `lens-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const res = await apiFetch(`${API_BASE}/api/observatory/lens`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json', 'Idempotency-Key': idemKey }),
      body: JSON.stringify({
        session_id: params.sessionId,
        target_system: params.targetSystem,
        prompt: params.prompt,
        task_type: params.taskType || 'images2PosedRGBD',
        ...(params.imageUrl ? { image_url: params.imageUrl } : {}),
        ...(params.depthAssetId ? { depth_asset_id: params.depthAssetId } : {}),
        ...(params.camera ? { camera: params.camera } : {}),
        ...(params.voxelSize ? { voxel_size: params.voxelSize } : {}),
        ...(params.ephemeralPurge ? { ephemeral_purge: true } : {}),
      }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string; code?: string };
      return { ok: false, error: data.message || `Request failed (${res.status})` };
    }
    return (await res.json()) as {
      ok: boolean;
      operation_id?: string;
      watermark?: string;
      cost_usd?: number;
      idempotency_key?: string;
    };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

/** Upload inline base64 media as an Atlas project asset (P1 rail). */
export async function uploadObservatoryAsset(params: {
  sessionId: string;
  dataBase64: string;
  displayName?: string;
}): Promise<{ ok: boolean; asset_id?: string; idempotency_key?: string; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/observatory/assets`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: params.sessionId,
        data_base64: params.dataBase64,
        display_name: params.displayName,
      }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Upload failed (${res.status})` };
    }
    return (await res.json()) as { ok: boolean; asset_id?: string; idempotency_key?: string };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

/** Resolve a lens operation to its terminal record (persists promptUsed). */
export async function resolveObservatoryLens(params: {
  sessionId: string;
  operationId: string;
}): Promise<{
  ok: boolean;
  done?: boolean;
  prompt_used?: string;
  asset_urls?: string[];
  local_paths?: string[];
  error?: unknown;
}> {
  try {
    const res = await apiFetch(`${API_BASE}/api/observatory/lens/resolve`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: params.sessionId, operation_id: params.operationId }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Resolve failed (${res.status})` };
    }
    return (await res.json()) as {
      ok: boolean;
      done?: boolean;
      prompt_used?: string;
      asset_urls?: string[];
      local_paths?: string[];
      error?: unknown;
    };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

/** Per-session lens history from our own reservation ledger (rated
 * invoices stay on the billing dashboard — present as reservations). */
export interface LensHistoryOp {
  operation_id: string;
  task_type?: string;
  cost_usd?: number;
  prompt_used?: string;
  resolved?: boolean;
  ephemeral_purge?: boolean;
  local_paths?: string[];
}

export async function fetchLensHistory(sessionId: string): Promise<{
  ok: boolean;
  operations?: LensHistoryOp[];
  error?: string;
}> {
  try {
    const res = await apiFetch(`${API_BASE}/api/observatory/lens/history?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `History failed (${res.status})` };
    }
    return (await res.json()) as { ok: boolean; operations?: LensHistoryOp[] };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export async function fetchObservatoryLensStatus(operationId: string): Promise<Record<string, unknown>> {
  try {
    const res = await apiFetch(`${API_BASE}/api/observatory/lens?operation_id=${encodeURIComponent(operationId)}`, {
      headers: authHeaders(),
    });
    if (!res.ok) {
      return { error: `Fetch failed (${res.status})` };
    }
    return (await res.json()) as Record<string, unknown>;
  } catch (err) {
    return { error: (err as Error).message || 'Network error' };
  }
}

// -----------------------------------------------------------------------------
// Vault pre-registration (P3): seal expectations before compute, evaluate
// measurements against them. All math lives server-side; the console only
// renders ledger-bound values (never invented).
// -----------------------------------------------------------------------------

export interface SealedPrediction {
  prediction_hash: string;
  target_system: string;
  method: string;
  quantity: string;
  expected_mean: number;
  expected_std: number;
  budget_cap_usd: number;
  author: string;
  p_correlated: number;
}

export interface PredictionEvaluation {
  prediction_hash: string;
  measured_value: number;
  surprise_z: number;
  surprise_ratio: number;
  brier_score: number;
  within_3sigma: boolean;
  level?: string;
}

export async function sealPrediction(params: {
  sessionId: string;
  targetSystem: string;
  method: string;
  quantity: string;
  expectedMean: number;
  expectedStd: number;
  budgetCapUsd?: number;
  author?: string;
  pCorrelated?: number;
}): Promise<{ ok: boolean; prediction_hash?: string; expected_std_used?: number; std_floor_applied?: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/predictions/seal`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: params.sessionId,
        target_system: params.targetSystem,
        method: params.method,
        quantity: params.quantity,
        expected_mean: params.expectedMean,
        expected_std: params.expectedStd,
        budget_cap_usd: params.budgetCapUsd ?? 0,
        author: params.author ?? 'console-operator',
        p_correlated: params.pCorrelated ?? 0.88,
      }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Seal failed (${res.status})` };
    }
    return (await res.json()) as { ok: boolean; prediction_hash?: string; expected_std_used?: number; std_floor_applied?: boolean };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export async function evaluatePrediction(params: {
  sessionId: string;
  predictionHash: string;
  measuredValue: number;
}): Promise<{
  ok: boolean;
  surprise_z?: number;
  surprise_ratio?: number;
  brier_score?: number;
  within_3sigma?: boolean;
  level?: string;
  error?: string;
}> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/predictions/evaluate`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: params.sessionId,
        prediction_hash: params.predictionHash,
        measured_value: params.measuredValue,
      }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Evaluate failed (${res.status})` };
    }
    return (await res.json()) as {
      ok: boolean;
      surprise_z?: number;
      surprise_ratio?: number;
      brier_score?: number;
      within_3sigma?: boolean;
      level?: string;
    };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export interface PredictionAssertion {
  prediction_hash: string;
  rho: number;
  evidence_refs: string[];
}

export async function fetchLatestPrediction(sessionId: string): Promise<{
  ok: boolean;
  sealed?: SealedPrediction | null;
  evaluation?: PredictionEvaluation | null;
  assertion?: PredictionAssertion | null;
  error?: string;
}> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/predictions/latest?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Fetch failed (${res.status})` };
    }
    return (await res.json()) as {
      ok: boolean;
      sealed?: SealedPrediction | null;
      evaluation?: PredictionEvaluation | null;
      assertion?: PredictionAssertion | null;
    };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export async function assertPrediction(params: {
  sessionId: string;
  predictionHash: string;
  rho: number;
  evidenceRefs: string[];
}): Promise<{ ok: boolean; prediction_hash?: string; rho?: number; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/predictions/assert`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: params.sessionId,
        prediction_hash: params.predictionHash,
        rho: params.rho,
        evidence_refs: params.evidenceRefs,
      }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Assert failed (${res.status})` };
    }
    return (await res.json()) as { ok: boolean; prediction_hash?: string; rho?: number };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export interface AuthorCalibration {
  sealed: number;
  evaluated: number;
  mean_brier_score?: number;
  mean_abs_z?: number;
}

export async function fetchCalibration(sessionId: string): Promise<{
  ok: boolean;
  authors?: Record<string, AuthorCalibration>;
  evaluations?: number;
  mean_brier_score?: number;
  mean_abs_z?: number;
  error?: string;
}> {
  try {
    const res = await apiFetch(`${API_BASE}/api/vault/predictions/calibration?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Calibration failed (${res.status})` };
    }
    return (await res.json()) as {
      ok: boolean;
      authors?: Record<string, AuthorCalibration>;
      evaluations?: number;
      mean_brier_score?: number;
      mean_abs_z?: number;
    };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

export interface HistoryPrediction {
  event_type?: string;
  prediction_hash?: string;
  expected_mean?: number;
  expected_std?: number;
  surprise_z?: number;
  level?: string;
  rho?: number;
  [key: string]: unknown;
}

export async function fetchPredictionHistory(
  sessionId: string,
  limit = 50,
): Promise<{ ok: boolean; count?: number; predictions?: HistoryPrediction[]; error?: string }> {
  try {
    const res = await apiFetch(
      `${API_BASE}/api/vault/predictions/history?session_id=${encodeURIComponent(sessionId)}&limit=${limit}`,
      { cache: 'no-store', headers: authHeaders() },
    );
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `History failed (${res.status})` };
    }
    return (await res.json()) as { ok: boolean; count?: number; predictions?: HistoryPrediction[] };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

// Ledger export for agent-player overlays (gaze trails) and replay.
// Returns newest-first records; callers filter by event_type / payload.
export async function fetchLedgerExport(
  sessionId: string,
): Promise<{ ok: boolean; events?: Array<Record<string, unknown>>; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/ledger/export`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, format: 'json' }),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { message?: string };
      return { ok: false, error: data.message || `Export failed (${res.status})` };
    }
    const data = (await res.json()) as { events?: Array<Record<string, unknown>> };
    return { ok: true, events: Array.isArray(data.events) ? data.events : [] };
  } catch (err) {
    return { ok: false, error: (err as Error).message || 'Network error' };
  }
}

// ---------------------------------------------------------------------------
// Wave-3 data layer (append-only block; every line above is untouched by
// design — another workstream depends on this file's exact shape above).
// ---------------------------------------------------------------------------

// Single chokepoint for client-side error reporting. Today this logs via
// console.error only; it is the future Sentry hook — wire Sentry here once
// and every surface (incl. ErrorBoundary) reports through it, never via
// ad-hoc console.error at call sites.
// Future Sentry wiring (sketch — not a dependency, do not uncomment
// without adding @sentry/nextjs):
//   // Sentry.init({ dsn: process.env.NEXT_PUBLIC_SENTRY_DSN });
//   // Sentry.withScope((scope) => {
//   //   scope.setTag("context", context);
//   //   Sentry.captureException(err);
//   // });
export function reportError(context: string, err: unknown): void {
  console.error(`[kubona:${context}]`, err);
}

// Inflight GET coalescing keyed by URL + request headers. Concurrent
// identical GETs share one fetch promise; the entry is deleted on settle
// (fulfilled or rejected) so later calls re-fetch. Non-GET methods
// (POST/PUT/DELETE, incl. an explicit method in init) never coalesce —
// they always issue their own fetch.
const dedupInflight = new Map<string, Promise<unknown>>();

function dedupHeaderKey(init?: RequestInit): string {
  const h = init?.headers;
  if (!h) return '';
  if (h instanceof Headers) {
    return JSON.stringify([...h.entries()].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
  }
  if (Array.isArray(h)) {
    return JSON.stringify([...h].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
  }
  return JSON.stringify(
    Object.entries(h as Record<string, string>).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
  );
}

function dedupKeyFor(url: string, init?: RequestInit): string {
  return `${url}|${dedupHeaderKey(init)}`;
}

export function dedupedGet<T>(url: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase();
  if (method !== 'GET') {
    return apiFetch(url, init ?? {}).then(async (res) => ((await res.json()) as T));
  }
  const key = dedupKeyFor(url, init);
  const existing = dedupInflight.get(key);
  if (existing) return existing as Promise<T>;
  const pending: Promise<T> = apiFetch(url, { cache: 'no-store', ...init }).then(
    async (res) => {
      // A refused GET is not a payload. Parsing the error body as T was the
      // Wave-3 bug: a 401/404 JSON object could masquerade as data.
      if (!res.ok) throw new Error(`GET ${url} refused (HTTP ${res.status})`);
      return (await res.json()) as T;
    },
  );
  dedupInflight.set(key, pending);
  const cleanup = (): void => {
    if (dedupInflight.get(key) === pending) dedupInflight.delete(key);
  };
  pending.then(cleanup, cleanup);
  return pending;
}

// Test-only: clears the inflight map between cases (module state must not
// leak across suites).
export function resetDedupForTests(): void {
  dedupInflight.clear();
}

