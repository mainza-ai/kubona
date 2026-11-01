import { Session, ChatMessage, Job, Loop, LedgerEvent, Lab } from '@/types';
import { contract, setContractDriftHandler, type ContractName } from './responseContract';
import { API_OPERATIONS, type ApiOperationName, type ApiOperationResponse } from './generated/openapi';

// Build-time default (local dev). The runtime route /api/config can
// override this per deployment (see configureApiBase) so one image
// serves any backend without a rebuild.
export let API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
// Runtime kill switch: defaults on, disabled only by an explicit config answer.
export let CONSOLE_WORKBENCH_ENABLED = true;

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
      const data = (await res.json()) as { apiUrl?: unknown; workbench?: unknown };
      if (typeof data.apiUrl === 'string' && /^https?:\/\//.test(data.apiUrl)) {
        API_BASE = data.apiUrl.replace(/\/$/, '');
      }
      if (typeof data.workbench === 'boolean') CONSOLE_WORKBENCH_ENABLED = data.workbench;
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
// A 401 while anonymous is a sign-in affordance, not an expired session.
// Surfaces listen to this locally; it must never render the global
// "Session expired" banner.
export const AUTH_REQUIRED_EVENT = 'kubona:auth-required';
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
// Console contract drift is an API problem: route it through the existing
// banner once per schema, and let NEXT_PUBLIC_KUBONA_STRICT_CONTRACTS=1 make
// the response fail instead of only logging.
setContractDriftHandler((name, issues) => {
  const detail = issues.map((i) => `${i.path}: ${i.message}`).join('; ');
  notifyApiProblem(`contract:${name}`, null, `response contract drifted: ${detail}`);
});

let lastAuthFailureAt = 0;
// True only after this document has observed an authenticated identity (login
// or whoami). A 401 before that is "sign in required", not "session expired".
let authenticatedSession = false;
export function isAuthenticatedSession(): boolean {
  return authenticatedSession;
}
export function wasRecentAuthFailure(withinMs = 30000): boolean {
  return Date.now() - lastAuthFailureAt < withinMs;
}
function notifyUnauthorized(): void {
  const expired = authenticatedSession;
  authenticatedSession = false;
  lastAuthFailureAt = Date.now();
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    window.dispatchEvent(new CustomEvent(expired ? UNAUTH_EVENT : AUTH_REQUIRED_EVENT));
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

export class OperationError extends Error {
  constructor(
    readonly operation: string,
    readonly status: number | null,
    message: string,
  ) {
    super(message);
    this.name = 'OperationError';
  }
}

export type OperationRequestOptions = {
  params?: Record<string, string>;
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
  headers?: Record<string, string>;
};

/**
 * Typed request for a modeled operation. The generated path/method/response
 * mapping is the contract; callers supply only parameters and body.
 */
export async function requestOperation<K extends ApiOperationName>(
  name: K,
  { params = {}, query = {}, body, headers }: OperationRequestOptions = {},
): Promise<ApiOperationResponse[K]> {
  const operation = API_OPERATIONS[name];
  const path = operation.path.replace(/\{([^}]+)\}/g, (_, key: string) => {
    const value = params[key];
    if (!value) throw new OperationError(name, null, `missing path parameter ${key}`);
    return encodeURIComponent(value);
  });
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) qs.set(key, String(value));
  }
  const url = `${API_BASE}${path}${qs.toString() ? `?${qs.toString()}` : ''}`;
  const res = await apiFetch(url, {
    method: operation.method.toUpperCase(),
    cache: 'no-store',
    headers: authHeaders(body === undefined ? headers : { 'Content-Type': 'application/json', ...headers }),
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!res.ok) {
    let detail = '';
    try {
      const text = (await res.text()).trim();
      try {
        const body = JSON.parse(text) as { message?: unknown; error?: unknown };
        detail = String(body.message || body.error || text);
      } catch {
        detail = text;
      }
    } catch {
      /* no body */
    }
    throw new OperationError(
      name,
      res.status,
      (detail || `${operation.method.toUpperCase()} ${operation.path} refused (HTTP ${res.status})`).slice(0, 280),
    );
  }
  return contract(operation.response as ContractName, await res.json()) as ApiOperationResponse[K];
}


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
    return (await requestOperation('fetchSession', { params: { id: sessionId } })) as Session;
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
    return (await requestOperation('createSession', {
      body: {
        title,
        spend_limit_usd: spendLimitUSD,
        target_molecule: targetMolecule,
        fresh: fresh ?? false,
      },
    })) as unknown as Session;
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
    const data = await requestOperation('fetchMessages', { params: { id: sessionId } });
    return (data.messages || []) as unknown as ChatMessage[];
  } catch (err) {
    console.error('Failed to fetch messages:', err);
    return [];
  }
}

export async function sendChatMessage(sessionId: string, content: string): Promise<{ user: ChatMessage; assistant: ChatMessage } | null> {
  try {
    const data = (await requestOperation('sendChatMessage', {
      query: { session_id: sessionId },
      body: { session_id: sessionId, role: 'user', content },
    })) as unknown as {
      user?: ChatMessage;
      user_message?: ChatMessage;
      assistant?: ChatMessage;
      assistant_message?: ChatMessage;
      model?: string;
      duration_ms?: number;
    };
    const assistant = data.assistant || data.assistant_message;
    if (assistant && data.model && !assistant.model) assistant.model = data.model;
    if (assistant && data.duration_ms != null && assistant.duration_ms == null) {
      assistant.duration_ms = data.duration_ms;
    }
    const user = data.user || data.user_message;
    return user && assistant ? { user, assistant } : null;
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
    return (await requestOperation('runTribunal', {
      body: { session_id: sessionId, prompt },
    })) as unknown as {
      tribunal: import('@/types').TribunalResult;
      assistant_message: ChatMessage;
      campaign?: import('@/types').CampaignState;
    };
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
    return (await requestOperation('resumeTribunal', {
      body: { session_id: sessionId, prompt, bundle },
    })) as unknown as {
      tribunal: import('@/types').TribunalResult;
      assistant_message: ChatMessage;
      campaign?: import('@/types').CampaignState;
    };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
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
    return (await requestOperation('launchProposal', {
      body: {
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
      },
    })) as unknown as { job: Job; loop: Loop };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
    console.error('Failed to launch proposal:', err);
    return null;
  }
}

export async function fetchLoops(sessionId?: string | null): Promise<Loop[]> {
  try {
    // K2: session-scoped when a session is active — the header counts
    // this session's loops, not the fleet's. Unset keeps the global view.
    const data = await requestOperation('fetchLoops', {
      query: { session_id: sessionId || undefined },
    });
    return (data.loops || []) as Loop[];
  } catch (err) {
    console.error('Failed to fetch loops:', err);
    return [];
  }
}

export async function fetchEvents(sessionId?: string): Promise<LedgerEvent[]> {
  if (!sessionId) return [];
  try {
    const data = await requestOperation('fetchEvents', { query: { session_id: sessionId } });
    return (data.events || []) as LedgerEvent[];
  } catch (err) {
    console.error('Failed to fetch events:', err);
    return [];
  }
}

export async function fetchMolecularADMET(smiles: string): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('fetchMolecularADMET', { query: { smiles } })) as Record<string, unknown>;
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
    return (await requestOperation('evaluateDiscoveryPipeline', {
      body: { target, candidates },
    })) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to evaluate discovery pipeline:', err);
    return null;
  }
}

export async function fetchCampaignState(sessionId: string): Promise<import('@/types').CampaignState | null> {
  try {
    // Coalesce concurrent reads of the same session: the shell and the
    // copilot mount together and used to issue duplicate campaign GETs.
    const data = await dedupedGet<import('@/types').CampaignState>(
      `${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign`,
      { headers: authHeaders() },
    );
    return contract('campaign', data);
  } catch (err) {
    console.error('Failed to fetch campaign state:', err);
    return null;
  }
}

export async function synthesizeCampaign(sessionId: string, prompt: string): Promise<import('@/types').CampaignState | null> {
  // Refusal (HTTP !ok) returns null so the caller renders "refused".
  // Transport failure throws so the caller can distinguish it from refusal.
  try {
    return (await requestOperation('synthesizeCampaign', {
      params: { id: sessionId },
      body: { prompt },
    })) as unknown as import('@/types').CampaignState;
  } catch (err) {
    if (err instanceof OperationError && err.status !== null) return null;
    throw err;
  }
}

export async function fetchAutoresearchHarnesses(): Promise<{ harnesses: import('@/types').AutoresearchHarness[]; docker_runner: Record<string, unknown> | null }> {
  try {
    return (await requestOperation('fetchAutoresearchHarnesses')) as unknown as {
      harnesses: import('@/types').AutoresearchHarness[];
      docker_runner: Record<string, unknown> | null;
    };
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
    return (await requestOperation('startAutoresearchLoop', {
      body: {
        session_id: sessionId || '',
        harness_id: harnessId,
        max_iterations: maxIterations,
        initial_artifact: initialArtifact || '',
      },
    })) as unknown as import('@/types').AutoresearchLoop;
  } catch (err) {
    console.error('Failed to start autoresearch loop:', err);
    return null;
  }
}

export async function fetchAutoresearchLoops(): Promise<import('@/types').AutoresearchLoop[]> {
  try {
    const data = await requestOperation('fetchAutoresearchLoops');
    return (data.loops || []) as unknown as import('@/types').AutoresearchLoop[];
  } catch (err) {
    console.error('Failed to fetch autoresearch loops:', err);
    return [];
  }
}

export async function stepAutoresearchLoop(loopId: string): Promise<{ step: import('@/types').IterationRecord; state: import('@/types').AutoresearchLoop } | null> {
  try {
    return (await requestOperation('stepAutoresearchLoop', {
      params: { id: loopId },
      body: {},
    })) as unknown as { step: import('@/types').IterationRecord; state: import('@/types').AutoresearchLoop };
  } catch (err) {
    console.error('Failed to step autoresearch loop:', err);
    return null;
  }
}

export async function pauseAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    return (await requestOperation('pauseAutoresearchLoop', { params: { id: loopId }, body: {} })) as unknown as import('@/types').AutoresearchLoop;
  } catch (err) {
    console.error('Failed to pause autoresearch loop:', err);
    return null;
  }
}

export async function resumeAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    return (await requestOperation('resumeAutoresearchLoop', { params: { id: loopId }, body: {} })) as unknown as import('@/types').AutoresearchLoop;
  } catch (err) {
    console.error('Failed to resume autoresearch loop:', err);
    return null;
  }
}

export async function stopAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    return (await requestOperation('stopAutoresearchLoop', { params: { id: loopId }, body: {} })) as unknown as import('@/types').AutoresearchLoop;
  } catch (err) {
    console.error('Failed to stop autoresearch loop:', err);
    return null;
  }
}

export async function fetchMetasearchProposals(): Promise<import('@/types').MetaProposal[]> {
  try {
    const data = await requestOperation('fetchMetasearchProposals');
    return (data.proposals || []) as unknown as import('@/types').MetaProposal[];
  } catch (err) {
    console.error('Failed to fetch metasearch proposals:', err);
    return [];
  }
}

export async function approveMetasearchProposal(
  proposalId: string,
  signer: string = 'console-operator'
): Promise<{ ok: true; success?: boolean; proposal?: unknown; message?: string } | { error: string } | null> {
  try {
    const data = await requestOperation('approveMetasearchProposal', { body: { proposal_id: proposalId, signer } });
    return { ok: true, ...data };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
    console.error('Failed to approve proposal:', err);
    return null;
  }
}

export async function rejectMetasearchProposal(
  proposalId: string,
  signer: string = 'console-operator'
): Promise<{ ok: true; success?: boolean; proposal?: unknown; message?: string } | { error: string } | null> {
  try {
    const data = await requestOperation('rejectMetasearchProposal', { body: { proposal_id: proposalId, signer } });
    return { ok: true, ...data };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
    console.error('Failed to reject proposal:', err);
    return null;
  }
}

export async function applyMetasearchProposal(
  proposalId: string
): Promise<{ ok: true; success?: boolean; proposal?: unknown; message?: string } | { error: string } | null> {
  try {
    const data = await requestOperation('applyMetasearchProposal', { body: { proposal_id: proposalId } });
    return { ok: true, ...data };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
    console.error('Failed to apply proposal:', err);
    return null;
  }
}

export async function fetchFleet(): Promise<Record<string, unknown>> {
  try {
    return (await requestOperation('fetchFleet')) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch fleet:', err);
    return {
      backends: [],
      refused: true,
      refuse_reason: String(err instanceof Error ? err.message : err).slice(0, 280),
    };
  }
}

export async function fetchRadar(): Promise<Record<string, unknown>> {
  try {
    return (await requestOperation('fetchRadar')) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch radar:', err);
    return { radar_endpoints: [], refused: true, refuse_reason: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function fetchNotebooks(sessionId: string): Promise<{ ok: true; notebooks: import('@/types').SystematicNotebook[] } | { ok: false; error: string }> {
  try {
    const data = await requestOperation('fetchNotebooks', { query: { session_id: sessionId } });
    return { ok: true, notebooks: data.notebooks as unknown as import('@/types').SystematicNotebook[] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('fetchCanvasLayout', {
      query: { session_id: sessionId },
    })) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch canvas layout:', err);
    return null;
  }
}

export async function fetchWorkbenchLayout(sessionId: string): Promise<{ revision: number; document: Record<string, unknown> } | null> {
  try {
    const res = await apiFetch(`${API_BASE}/api/workbench/layout?session_id=${encodeURIComponent(sessionId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    const data = (await res.json()) as { revision?: unknown; document?: unknown };
    if (typeof data.revision !== 'number' || !data.document || typeof data.document !== 'object') return null;
    contract('workbench.layout', data);
    return { revision: data.revision, document: data.document as Record<string, unknown> };
  } catch {
    return null;
  }
}

export async function saveWorkbenchLayout(
  sessionId: string,
  revision: number,
  document: Record<string, unknown>,
): Promise<{ ok: boolean; revision?: number; conflict?: boolean; currentRevision?: number; unknown?: boolean; error?: string }> {
  try {
    const res = await apiFetch(`${API_BASE}/api/workbench/layout?session_id=${encodeURIComponent(sessionId)}`, {
      method: 'PUT',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ revision, document }),
    });
    if (res.status === 409) {
      const data = (await res.json().catch(() => ({}))) as { current_revision?: unknown };
      return {
        ok: false,
        conflict: true,
        currentRevision: typeof data.current_revision === 'number' ? data.current_revision : undefined,
        error: 'Layout changed elsewhere — reload to keep both.',
      };
    }
    if (!res.ok) return { ok: false, error: `HTTP ${res.status}` };
    const data = (await res.json()) as { revision?: unknown };
    if (typeof data.revision === 'number') {
      contract('workbench.save', data);
      return { ok: true, revision: data.revision };
    }
    return { ok: true, revision: revision + 1 };
  } catch (err) {
    return { ok: false, unknown: true, error: String(err) };
  }
}

export async function saveCanvasLayout(sessionId: string, layout: Record<string, unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    await requestOperation('saveCanvasLayout', { query: { session_id: sessionId }, body: layout });
    return { ok: true };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err) };
  }
}
export async function fetchCapabilities(): Promise<CapabilitiesPayload | null> {
  try {
    return (await requestOperation('fetchCapabilities')) as CapabilitiesPayload;
  } catch (err) {
    console.error('Failed to fetch capabilities:', err);
    return null;
  }
}

export async function fetchFidelity(device: string, depth: number, twoQubitGates: number): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('fetchFidelity', {
      query: { device, depth, two_qubit_gates: twoQubitGates },
    })) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch fidelity:', err);
    return null;
  }
}

export async function fetchEpistemics(sessionId?: string | null): Promise<Record<string, unknown>> {
  try {
    return (await requestOperation('fetchEpistemics', {
      query: { session_id: sessionId || undefined },
    })) as Record<string, unknown>;
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
    return { ok: true, result: (await requestOperation('fetchCodesignVerified')) as unknown as CodesignVerifiedResult };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function evaluateCodesignArtifact(
  body: Record<string, unknown>
): Promise<{ ok: true; spec_kind: string; result: Record<string, unknown> } | { ok: false; error: string }> {
  try {
    const data = (await requestOperation('evaluateCodesignArtifact', { body })) as {
      ok?: boolean;
      spec_kind?: unknown;
      result?: unknown;
    };
    if (data.ok !== true) return { ok: false, error: 'codesign evaluator refused without a reason' };
    return {
      ok: true,
      spec_kind: String(data.spec_kind ?? ''),
      result: (data.result ?? {}) as Record<string, unknown>,
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

// Read-only LLM spend summary (G10): server-sealed aggregates, heuristic.
// Pack registry (ux-plan slice 4): served live, never hand-typed.
// Promotion stays a privileged op; this rail is read-only.
export async function evaluateCodesignBatch(
  body: Record<string, unknown>
): Promise<{ ok: true; spec_kind: string; frontier: Record<string, unknown>; details: Array<Record<string, unknown>> } | { ok: false; error: string }> {
  try {
    const data = (await requestOperation('evaluateCodesignBatch', { body })) as {
      ok?: boolean;
      spec_kind?: unknown;
      frontier?: unknown;
      details?: unknown;
    };
    if (data.ok !== true) return { ok: false, error: 'codesign batch refused without a reason' };
    return {
      ok: true,
      spec_kind: String(data.spec_kind ?? ''),
      frontier: (data.frontier ?? {}) as Record<string, unknown>,
      details: Array.isArray(data.details) ? (data.details as Array<Record<string, unknown>>) : [],
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function fetchCodesignPacks(): Promise<
  { ok: true; result: CodesignPacksResult } | { ok: false; error: string }
> {
  try {
    return { ok: true, result: (await requestOperation('fetchCodesignPacks')) as unknown as CodesignPacksResult };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

// Evaluated family search (ux-plan slice 3): tracks mature separately,
// elites cross-pollinate, widen-nudge names the most stagnant track.
export async function runFamilySearch(
  body: Record<string, unknown>
): Promise<{ ok: true; result: CodesignFamiliesResult } | { ok: false; error: string }> {
  try {
    const data = (await requestOperation('runFamilySearch', { body })) as CodesignFamiliesResult & { ok?: boolean };
    if (data.ok !== true) return { ok: false, error: 'family search refused without a reason' };
    return { ok: true, result: data };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

// C5b governed rerun: the Go rail owns the loop, BYOK estimate, and
// sealed receipt. `request_id` is required; reuse it to replay the same
// receipt after a transient failure (never mint a new id to "retry").
export async function rerunCodesignProposal(
  body: Record<string, unknown>
): Promise<{ ok: true; result: CodesignRerunResult } | { ok: false; error: string }> {
  try {
    const data = (await requestOperation('rerunCodesignProposal', { body })) as CodesignRerunResult & { ok?: boolean };
    if (data.ok !== true) return { ok: false, error: 'codesign rerun refused without a reason' };
    return { ok: true, result: data };
  } catch (err) {
    const message = String(err instanceof Error ? err.message : err).slice(0, 280);
    return { ok: false, error: err instanceof OperationError && err.status === 401 ? `login required — ${message}` : message };
  }
}

export async function fetchCostSummary(): Promise<
  { ok: true; result: CostSummary } | { ok: false; error: string }
> {
  try {
    return { ok: true, result: (await requestOperation('fetchCostSummary')) as CostSummary };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function fetchLLMSettings(): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('fetchLLMSettings')) as Record<string, unknown>;
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
    const data = await requestOperation('fetchQuantumBackends');
    return (Array.isArray(data.backends) ? data.backends : []) as unknown as QuantumBackendInfo[];
  } catch (err) {
    console.error('Failed to fetch quantum backends:', err);
    return [];
  }
}

export async function fetchQuantumSettings(): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('fetchQuantumSettings')) as Record<string, unknown>;
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
    const data = await requestOperation('runQuantumCanary', {
      query: {
        providers: providers && providers.length > 0 ? providers.join(',') : undefined,
        max_seconds: maxSeconds || undefined,
      },
    });
    return { ok: true, results: (data.results ?? {}) as Record<string, unknown>, cached: data.cached };
  } catch (err) {
    console.error('Failed to run quantum canary:', err);
    return { ok: false, error: String(err instanceof Error ? err.message : err) };
  }
}
export async function saveQuantumSettings(
  settings: Record<string, unknown>
): Promise<{ ok: boolean; error?: string }> {
  try {
    await requestOperation('saveQuantumSettings', { body: settings });
    return { ok: true };
  } catch (err) {
    console.error('Failed to save quantum settings:', err);
    return { ok: false, error: String(err instanceof Error ? err.message : err) };
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
): Promise<{ ok: boolean; job_id?: string; queued?: boolean; dry_run?: boolean; estimate?: Record<string, unknown>; fidelity?: Record<string, unknown>; session_id?: string; unknown?: boolean; error?: string }> {
  try {
    return (await requestOperation('submitQuantumJob', { body: job })) as {
      ok: boolean;
      job_id?: string;
      queued?: boolean;
      dry_run?: boolean;
      estimate?: Record<string, unknown>;
      fidelity?: Record<string, unknown>;
      session_id?: string;
    };
  } catch (err) {
    console.error('Failed to submit quantum job:', err);
    return { ok: false, unknown: !(err instanceof OperationError) || err.status === null, error: String(err instanceof Error ? err.message : err) };
  }
}

export async function fetchQuantumJob(jobId: string): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('fetchQuantumJob', { params: { id: jobId } })) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch quantum job:', err);
    return null;
  }
}

export async function cancelQuantumJob(jobId: string, reason?: string): Promise<{ ok: boolean; error?: string; provider_recall_attempted?: boolean; provider_recall_status?: string }> {
  try {
    const data = await requestOperation('cancelQuantumJob', { params: { id: jobId }, body: { reason: reason ?? '' } });
    return { ok: data.ok === true, provider_recall_attempted: data.provider_recall_attempted, provider_recall_status: data.provider_recall_status };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function saveLLMSettings(
  settings: Record<string, unknown>
): Promise<{ ok: boolean; error?: string }> {
  try {
    await requestOperation('saveLLMSettings', { body: settings });
    return { ok: true };
  } catch (err) {
    console.error('Failed to save LLM settings:', err);
    return { ok: false, error: String(err instanceof Error ? err.message : err) };
  }
}
export async function fetchLLMModels(provider: string = 'omlx'): Promise<Record<string, unknown>> {
  try {
    return (await requestOperation('fetchLLMModels', { query: { provider } })) as Record<string, unknown>;
  } catch (err) {
    console.error('Failed to fetch LLM models:', err);
    return { models: [] };
  }
}

export async function testLLM(prompt: string, role: string = 'copilot'): Promise<Record<string, unknown> | null> {
  try {
    return (await requestOperation('testLLM', { body: { role, prompt } })) as Record<string, unknown>;
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
    const data = (await requestOperation('searchHFModels', {
      query: { q: query, library, limit },
    })) as { models?: import('@/types').HFModelSummary[] };
    return data.models || [];
  } catch (err) {
    if (err instanceof OperationError && err.status !== null) {
      throw new Error(`hub refused search (HTTP ${err.status}): no operator token or hub unreachable`);
    }
    console.error('Failed to search Hugging Face models:', err);
    return [];
  }
}

export async function getHFModelDetails(
  repoId: string
): Promise<import('@/types').HFModelSummary | null> {
  try {
    const data = (await requestOperation('getHFModelDetails', { params: { id: repoId } })) as {
      model?: import('@/types').HFModelSummary | null;
    };
    return data.model || null;
  } catch (err) {
    console.error('Failed to fetch HF model details:', err);
    return null;
  }
}

export async function startHFDownload(
  repoId: string,
  filename: string,
  sizeBytes?: number,
  confirmed?: boolean
): Promise<import('@/types').HFDownloadTask | null> {
  try {
    const data = (await requestOperation('startHFDownload', {
      body: {
        repo_id: repoId,
        filename,
        ...(typeof sizeBytes === 'number' && Number.isFinite(sizeBytes) ? { size_bytes: sizeBytes } : {}),
        ...(confirmed ? { confirmed: true } : {}),
      },
    })) as { task?: import('@/types').HFDownloadTask | null };
    return data.task || null;
  } catch (err) {
    console.error('Failed to start HF download:', err);
    return null;
  }
}

export async function listHFDownloads(): Promise<import('@/types').HFDownloadTask[]> {
  try {
    const data = (await requestOperation('listHFDownloads')) as {
      downloads?: import('@/types').HFDownloadTask[];
    };
    return data.downloads || [];
  } catch (err) {
    if (err instanceof OperationError && err.status !== null) {
      throw new Error(`hub refused download list (HTTP ${err.status}): no operator token or hub unreachable`);
    }
    console.error('Failed to list HF downloads:', err);
    return [];
  }
}

export async function cancelHFDownload(taskId: string): Promise<boolean> {
  try {
    const data = await requestOperation('cancelHFDownload', { params: { id: taskId } });
    return data.ok === true;
  } catch (err) {
    console.error('Failed to cancel HF download:', err);
    return false;
  }
}
export interface SystemAgentRole {
  role: string;
  description: string;
  requires_human_ap2?: boolean;
}

export async function fetchSystemAgents(): Promise<SystemAgentRole[]> {
  try {
    const data = await requestOperation('fetchSystemAgents');
    return (Array.isArray(data.roles) ? data.roles : []) as unknown as SystemAgentRole[];
  } catch (err) {
    console.error('Failed to fetch system agents:', err);
    return [];
  }
}

export async function fetchCustomAgents(): Promise<import('@/types').CustomAgentManifest[]> {
  try {
    const data = await requestOperation('fetchCustomAgents');
    return (data.agents || []) as unknown as import('@/types').CustomAgentManifest[];
  } catch (err) {
    console.error('Failed to fetch custom agents:', err);
    return [];
  }
}

export async function createCustomAgent(
  manifest: import('@/types').CustomAgentManifest
): Promise<import('@/types').CustomAgentManifest> {
  try {
    const data = (await requestOperation('createCustomAgent', { body: manifest })) as {
      agent?: import('@/types').CustomAgentManifest;
    };
    if (!data.agent) throw new Error('agent registration returned no agent: nothing registered');
    return data.agent;
  } catch (err) {
    if (err instanceof OperationError) {
      throw new Error(`agent registration refused (HTTP ${err.status ?? 'transport'}): ${err.message.slice(0, 160)}`);
    }
    throw err;
  }
}

export async function deleteCustomAgent(agentId: string): Promise<boolean> {
  try {
    const data = await requestOperation('deleteCustomAgent', { params: { id: agentId } });
    return data.ok === true;
  } catch (err) {
    console.error('Failed to delete custom agent:', err);
    return false;
  }
}

export async function routeMultimodalPayload(
  payload: import('@/types').MultimodalPayload
): Promise<import('@/types').RoutingDecision | null> {
  try {
    const data = (await requestOperation('routeMultimodalPayload', { body: payload })) as {
      decision?: import('@/types').RoutingDecision;
    };
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
  try {
    return (await requestOperation('planWorkflow', { body: payload })) as unknown as import('@/types').WorkflowPlanResponse;
  } catch (err) {
    const status = err instanceof OperationError ? err.status : null;
    throw new Error(`workflow plan refused${status ? ` (HTTP ${status})` : ''}: ${String(err instanceof Error ? err.message : err).slice(0, 160)}`);
  }
}

export async function runWorkflow(payload: {
  session_id: string;
  workflow: import('@/types').WorkflowDefinition;
  inputs?: Record<string, unknown>;
  force?: boolean;
}): Promise<import('@/types').WorkflowRunResponse> {
  try {
    return (await requestOperation('runWorkflow', { body: payload })) as unknown as import('@/types').WorkflowRunResponse;
  } catch (err) {
    const status = err instanceof OperationError ? err.status : null;
    throw new Error(`workflow run refused${status ? ` (HTTP ${status})` : ''}: ${String(err instanceof Error ? err.message : err).slice(0, 160)}`);
  }
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
    return (await requestOperation('requestServerSignedMandate', { body: req })) as unknown as SpendMandateSignResponse;
  } catch (err) {
    console.error('Failed to request server signed mandate:', err);
    return null;
  }
}

export async function verifySpendMandate(
  mandate: import('./mandates').SpendMandate
): Promise<SpendMandateVerifyResponse> {
  try {
    return (await requestOperation('verifySpendMandate', { body: mandate })) as unknown as SpendMandateVerifyResponse;
  } catch (err) {
    console.error('Failed to verify spend mandate:', err);
    return { valid: false, error: String(err instanceof Error ? err.message : err) };
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
    return (await requestOperation('emergencyAbort', {
      body: { session_id: sessionId, reason },
    })) as { aborted_count: number; abort_receipt: string };
  } catch (err) {
    console.error('Abort failed and nothing was settled:', err);
    return null;
  }
}

export async function searchLiterature(query: string, limit = 5): Promise<{ records: import('@/types').LiteratureRecord[]; cached: boolean }> {
  try {
    const data = (await requestOperation('searchLiterature', {
      query: { query, limit },
    })) as { records?: import('@/types').LiteratureRecord[]; cached?: boolean };
    return { records: data.records || [], cached: Boolean(data.cached) };
  } catch (err) {
    console.error('Failed to search literature:', err);
    return { records: [], cached: false };
  }
}

// Clear-history: deletes every chat message in the session. Session +
// campaign + ledger history survive; the erasure is sealed server-side.
export async function clearMessages(sessionId: string): Promise<number | null> {
  try {
    const data = await requestOperation('clearMessages', { params: { id: sessionId } });
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
    const data = (await requestOperation('regenerateChat', {
      body: { session_id: sessionId },
    })) as unknown as { assistant?: ChatMessage; assistant_message?: ChatMessage; model?: string; duration_ms?: number };
    const assistant = data.assistant || data.assistant_message;
    if (assistant && data.model && !assistant.model) assistant.model = data.model;
    if (assistant && data.duration_ms != null && assistant.duration_ms == null) {
      assistant.duration_ms = data.duration_ms;
    }
    return assistant ? { assistant, model: data.model, duration_ms: data.duration_ms } : null;
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
    const data = await requestOperation('sendFeedback', {
      body: { session_id: sessionId, message_id: messageId, vote },
    });
    return data.recorded === true;
  } catch (err) {
    console.error('Failed to send feedback:', err);
    return false;
  }
}
export async function rewriteChat(
  sessionId: string,
  messageId: string,
  content: string
): Promise<ChatMessage | null> {
  try {
    const data = (await requestOperation('rewriteChat', {
      body: { session_id: sessionId, message_id: messageId, content },
    })) as { message?: ChatMessage | null };
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
export async function fetchJobs(sessionId: string): Promise<Job[]> {
  try {
    const data = await requestOperation('fetchJobs', { query: { session_id: sessionId } });
    return (data.jobs || []) as unknown as Job[];
  } catch (err) {
    console.error('Failed to fetch jobs:', err);
    return [];
  }
}

// Job detail: full params + result payloads for the click-through view.
// Unknown ids 404 → null (never an empty shell). sessionId is REQUIRED:
// the backend scopes jobs per tenant session and refuses unscopend reads
// (400) — the client never makes a call it knows will fail.
export async function fetchJob(jobId: string, sessionId: string): Promise<Job | null> {
  if (!sessionId) return null;
  try {
    const data = await requestOperation('fetchJob', {
      params: { id: jobId },
      query: { session_id: sessionId },
    });
    return (data.job || null) as unknown as Job | null;
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
    return (await requestOperation('fetchMemoryRecall', {
      query: { session_id: sessionId, q: query, bank: opts?.bank, limit: opts?.limit },
    })) as unknown as { snippets: MemorySnippet[]; tenant: string };
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
    return (await requestOperation('retainMemory', {
      body: {
        session_id: sessionId,
        content,
        bank: opts?.bank || 'experiences',
        ...(opts?.importance !== undefined ? { importance: opts.importance } : {}),
      },
    })) as unknown as { id: string; bank: string; importance: number };
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
  try {
    return (await requestOperation('fetchQirCompile', { body: { qasm } })) as unknown as QirCompileReceipt;
  } catch (err) {
    if (err instanceof OperationError && err.status !== null) return null;
    throw err;
  }
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
    return (await requestOperation('fetchSessionContext', {
      query: { session_id: sessionId },
    })) as SessionContext;
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
  try {
    const data = (await requestOperation('appendLedgerEvent', {
      body: { session_id: sessionId, event_type: eventType, payload },
    })) as { ok?: boolean; record_hash?: string; event_id?: string | number; error?: string };
    if (!data?.ok || !data?.record_hash) {
      return { ok: false, error: data?.error || 'ledger returned no record hash' };
    }
    return { ok: true, record_hash: data.record_hash, event_id: data.event_id };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
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
    await requestOperation('loopAction', { params: { id: loopId, action } });
    return { ok: true };
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
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
    const tenant = (await requestOperation('createLab', {
      body: { id, display_name: displayName },
    })) as unknown as Lab;
    return { ok: true, tenant };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    const data = await requestOperation('fetchOpenObligations');
    return { ok: true, obligations: data.obligations as unknown as ObligationRow[] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function closeObligation(id: string): Promise<{ ok: boolean; error?: string; reasons?: string[]; closed?: boolean }> {
  try {
    const data = await requestOperation('closeObligation', { params: { id }, body: {} });
    return { ok: true, closed: data.closed === true, reasons: data.reasons };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    const data = await requestOperation('issueVaultLease', {
      body: { scope, ttl_seconds: ttlSeconds, ...(sessionId ? { session_id: sessionId } : {}) },
    });
    return { ok: true, lease: data.lease as unknown as VaultLease };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function revokeVaultLease(leaseId: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const data = await requestOperation('revokeVaultLease', { body: { lease_id: leaseId } });
    return { ok: data.revoked === true };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}
export async function verifyLedger(sessionId?: string): Promise<{ ok: boolean; fault_count?: number; faults?: unknown[]; error?: string }> {
  try {
    return (await requestOperation('verifyLedger', {
      query: { session_id: sessionId || undefined },
    })) as { ok: boolean; fault_count?: number; faults?: unknown[] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('authChallenge', { body: {} })) as unknown as AuthChallenge;
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
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
    const data = (await requestOperation('authLogin', {
      body: { tenant_id: tenantId, pubkey_hex: pubkeyHex, nonce, signature_hex: signatureHex },
    })) as unknown as { ok: true; tenant_id: string; pubkey_fp: string };
    authenticatedSession = true;
    return data;
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
    console.error('Failed to log in:', err);
    return null;
  }
}

export async function authLogout(): Promise<boolean> {
  try {
    const res = await apiFetch(`${API_BASE}/api/auth/logout`, { method: 'POST' });
    if (res.ok) authenticatedSession = false;
    return res.ok;
  } catch (err) {
    console.error('Failed to log out:', err);
    return false;
  }
}

export async function authWhoami(): Promise<AuthSession | null> {
  try {
    const data = (await requestOperation('authWhoami')) as unknown as AuthSession;
    authenticatedSession = data.authenticated === true;
    return data;
  } catch (err) {
    console.error('Failed to check session:', err);
    return null;
  }
}

export async function authSessions(): Promise<AuthServerSession[] | { error: string } | null> {
  try {
    const data = (await requestOperation('authSessions')) as { sessions?: AuthServerSession[] };
    return data.sessions || [];
  } catch (err) {
    if (err instanceof OperationError) return { error: err.message };
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
    const idemKey =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `lens-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    return (await requestOperation('requestObservatoryLens', {
      headers: { 'Idempotency-Key': idemKey },
      body: {
        session_id: params.sessionId,
        target_system: params.targetSystem,
        prompt: params.prompt,
        task_type: params.taskType || 'images2PosedRGBD',
        ...(params.imageUrl ? { image_url: params.imageUrl } : {}),
        ...(params.depthAssetId ? { depth_asset_id: params.depthAssetId } : {}),
        ...(params.camera ? { camera: params.camera } : {}),
        ...(params.voxelSize ? { voxel_size: params.voxelSize } : {}),
        ...(params.ephemeralPurge ? { ephemeral_purge: true } : {}),
      },
    })) as {
      ok: boolean;
      operation_id?: string;
      watermark?: string;
      cost_usd?: number;
      idempotency_key?: string;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

/** Upload inline base64 media as an Atlas project asset (P1 rail). */
export async function uploadObservatoryAsset(params: {
  sessionId: string;
  dataBase64: string;
  displayName?: string;
}): Promise<{ ok: boolean; asset_id?: string; idempotency_key?: string; error?: string }> {
  try {
    return (await requestOperation('uploadObservatoryAsset', {
      body: {
        session_id: params.sessionId,
        data_base64: params.dataBase64,
        display_name: params.displayName,
      },
    })) as { ok: boolean; asset_id?: string; idempotency_key?: string };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('resolveObservatoryLens', {
      body: { session_id: params.sessionId, operation_id: params.operationId },
    })) as {
      ok: boolean;
      done?: boolean;
      prompt_used?: string;
      asset_urls?: string[];
      local_paths?: string[];
      error?: unknown;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('fetchLensHistory', {
      query: { session_id: sessionId },
    })) as unknown as { ok: boolean; operations?: LensHistoryOp[] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function fetchObservatoryLensStatus(operationId: string): Promise<Record<string, unknown>> {
  try {
    return (await requestOperation('fetchObservatoryLensStatus', {
      query: { operation_id: operationId },
    })) as Record<string, unknown>;
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
    return (await requestOperation('sealPrediction', {
      body: {
        session_id: params.sessionId,
        target_system: params.targetSystem,
        method: params.method,
        quantity: params.quantity,
        expected_mean: params.expectedMean,
        expected_std: params.expectedStd,
        budget_cap_usd: params.budgetCapUsd ?? 0,
        author: params.author ?? 'console-operator',
        p_correlated: params.pCorrelated ?? 0.88,
      },
    })) as {
      ok: boolean;
      prediction_hash?: string;
      expected_std_used?: number;
      std_floor_applied?: boolean;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('evaluatePrediction', {
      body: {
        session_id: params.sessionId,
        prediction_hash: params.predictionHash,
        measured_value: params.measuredValue,
      },
    })) as {
      ok: boolean;
      surprise_z?: number;
      surprise_ratio?: number;
      brier_score?: number;
      within_3sigma?: boolean;
      level?: string;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('fetchLatestPrediction', {
      query: { session_id: sessionId },
    })) as unknown as {
      ok: boolean;
      sealed?: SealedPrediction | null;
      evaluation?: PredictionEvaluation | null;
      assertion?: PredictionAssertion | null;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

export async function assertPrediction(params: {
  sessionId: string;
  predictionHash: string;
  rho: number;
  evidenceRefs: string[];
}): Promise<{ ok: boolean; prediction_hash?: string; rho?: number; error?: string }> {
  try {
    return (await requestOperation('assertPrediction', {
      body: {
        session_id: params.sessionId,
        prediction_hash: params.predictionHash,
        rho: params.rho,
        evidence_refs: params.evidenceRefs,
      },
    })) as { ok: boolean; prediction_hash?: string; rho?: number };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('fetchCalibration', {
      query: { session_id: sessionId },
    })) as unknown as {
      ok: boolean;
      authors?: Record<string, AuthorCalibration>;
      evaluations?: number;
      mean_brier_score?: number;
      mean_abs_z?: number;
    };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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
    return (await requestOperation('fetchPredictionHistory', {
      query: { session_id: sessionId, limit },
    })) as unknown as { ok: boolean; count?: number; predictions?: HistoryPrediction[] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
  }
}

// Ledger export for agent-player overlays (gaze trails) and replay.
// Returns newest-first records; callers filter by event_type / payload.
export async function fetchLedgerExport(
  sessionId: string,
): Promise<{ ok: boolean; events?: Array<Record<string, unknown>>; error?: string }> {
  try {
    const data = (await requestOperation('fetchLedgerExport', {
      body: { session_id: sessionId, format: 'json' },
    })) as unknown as { events?: Array<Record<string, unknown>> };
    return { ok: true, events: Array.isArray(data.events) ? data.events : [] };
  } catch (err) {
    return { ok: false, error: String(err instanceof Error ? err.message : err).slice(0, 280) };
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

