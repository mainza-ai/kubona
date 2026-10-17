import { Session, ChatMessage, Job, Loop, LedgerEvent, Lab } from '@/types';

export const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

// Operator bearer token (local dev only): the bridge requires `Authorization:
// Bearer <token>` on every route and fails closed without it. In production
// this variable is unset, so no header is sent and the bridge keeps refusing.
// Never bake a real operator token into a shipped bundle.
const CONSOLE_TOKEN = process.env.NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN || '';

function authHeaders(extra: Record<string, string> = {}): Record<string, string> {
  return CONSOLE_TOKEN ? { Authorization: `Bearer ${CONSOLE_TOKEN}`, ...extra } : { ...extra };
}

// Exported for ledger helpers and any surface that must go through the
// authenticated bridge path (no raw unauthed fetch to /api/*).
export { authHeaders };


export async function fetchSessions(): Promise<Session[]> {
  try {
    const res = await fetch(`${API_BASE}/api/sessions`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.sessions || [];
  } catch (err) {
    console.error('Failed to fetch sessions:', err);
    return [];
  }
}

export async function createSession(
  title: string,
  spendLimitUSD: number = 100,
  targetMolecule?: string,
  fresh?: boolean
): Promise<Session | null> {
  try {
    const res = await fetch(`${API_BASE}/api/sessions`, {
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
    const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}`, {
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
    const res = await fetch(`${API_BASE}/api/sessions/${sessionId}/messages`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.messages || [];
  } catch (err) {
    console.error('Failed to fetch messages:', err);
    return [];
  }
}

export async function sendChatMessage(sessionId: string, content: string): Promise<{ user: ChatMessage; assistant: ChatMessage } | null> {  try {
    const res = await fetch(`${API_BASE}/api/chat/messages?session_id=${encodeURIComponent(sessionId)}`, {
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
    return {
      user: data.user || data.user_message,
      assistant,
    };
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
  const res = await fetch(`${API_BASE}/api/chat/stream?session_id=${encodeURIComponent(sessionId)}`, {
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
  for (;;) {
    const { value, done: readerDone } = await reader.read();
    if (value) buf += decoder.decode(value, { stream: true });
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
  return done;
}

export async function runTribunal(
  sessionId: string,
  prompt: string
): Promise<{ tribunal: import('@/types').TribunalResult; assistant_message: ChatMessage; campaign?: import('@/types').CampaignState } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chat/tribunal`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, prompt }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to run tribunal:', err);
    return null;
  }
}

export async function launchProposal(sessionId: string, proposal: unknown): Promise<{ job: Job; loop: Loop } | { error: string } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chat/launch`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, proposal }),
    });
    // Refusals carry the backend reason as plain text — surface it, never
    // swallow it into a generic "refused" (2026-10-11: a tier-routing
    // refusal was invisible in the UI).
    if (!res.ok) {
      const text = (await res.text()).trim().slice(0, 280);
      return { error: text || `HTTP ${res.status}` };
    }
    return await res.json();
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
    const res = await fetch(`${API_BASE}/api/loops${qs}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.loops || [];
  } catch (err) {
    console.error('Failed to fetch loops:', err);
    return [];
  }
}

export async function fetchEvents(sessionId?: string): Promise<LedgerEvent[]> {
  try {
    const url = sessionId ? `${API_BASE}/api/events?session_id=${sessionId}` : `${API_BASE}/api/events`;
    const res = await fetch(url, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.events || [];
  } catch (err) {
    console.error('Failed to fetch events:', err);
    return [];
  }
}

export async function fetchMolecularADMET(smiles: string): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chemistry/admet?smiles=${encodeURIComponent(smiles)}`, {
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
    const res = await fetch(`${API_BASE}/api/discovery/pipeline`, {
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
    const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch campaign state:', err);
    return null;
  }
}

export async function synthesizeCampaign(sessionId: string, prompt: string): Promise<import('@/types').CampaignState | null> {
  // Refusal (HTTP !ok) returns null so the caller renders "refused".
  // Transport failure throws so the caller can distinguish it from refusal.
  const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign/synthesize`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ prompt }),
  });
  if (!res.ok) return null;
  return await res.json();
}

export async function fetchAutoresearchHarnesses(): Promise<{ harnesses: import('@/types').AutoresearchHarness[]; docker_runner: Record<string, unknown> | null }> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/harnesses`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { harnesses: [], docker_runner: null };
    return await res.json();
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
    const res = await fetch(`${API_BASE}/api/autoresearch/start`, {
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
    return await res.json();
  } catch (err) {
    console.error('Failed to start autoresearch loop:', err);
    return null;
  }
}

export async function fetchAutoresearchLoops(): Promise<import('@/types').AutoresearchLoop[]> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/loops`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.loops || [];
  } catch (err) {
    console.error('Failed to fetch autoresearch loops:', err);
    return [];
  }
}

export async function stepAutoresearchLoop(loopId: string): Promise<{ step: import('@/types').IterationRecord; state: import('@/types').AutoresearchLoop } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/step`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to step autoresearch loop:', err);
    return null;
  }
}

export async function pauseAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/pause`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to pause autoresearch loop:', err);
    return null;
  }
}

export async function resumeAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/resume`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to resume autoresearch loop:', err);
    return null;
  }
}

export async function stopAutoresearchLoop(loopId: string): Promise<import('@/types').AutoresearchLoop | null> {
  try {
    const res = await fetch(`${API_BASE}/api/autoresearch/loops/${encodeURIComponent(loopId)}/stop`, {
      headers: authHeaders(),
      method: 'POST',
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to stop autoresearch loop:', err);
    return null;
  }
}

export async function fetchMetasearchProposals(): Promise<import('@/types').MetaProposal[]> {
  try {
    const res = await fetch(`${API_BASE}/api/metasearch/proposals`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
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
    const res = await fetch(`${API_BASE}/api/metasearch/approve`, {
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
    const res = await fetch(`${API_BASE}/api/metasearch/reject`, {
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
    const res = await fetch(`${API_BASE}/api/metasearch/apply`, {
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
    const res = await fetch(`${API_BASE}/api/fleet`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { backends: [], refused: true, refuse_reason: text || `HTTP ${res.status}` };
    }
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch fleet:', err);
    return { backends: [], refused: true, refuse_reason: String(err).slice(0, 280) };
  }
}

export async function fetchRadar(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/radar`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) {
      let text = '';
      try {
        text = (await res.text()).trim().slice(0, 280);
      } catch {
        /* ignore */
      }
      return { radar_endpoints: [], refused: true, refuse_reason: text || `HTTP ${res.status}` };
    }
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch radar:', err);
    return { radar_endpoints: [], refused: true, refuse_reason: String(err).slice(0, 280) };
  }
}

export async function fetchEpistemics(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/epistemics`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return {};
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch epistemics:', err);
    return {};
  }
}

export async function fetchLLMSettings(): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetch(`${API_BASE}/api/llm/settings`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch LLM settings:', err);
    return null;
  }
}

export async function saveLLMSettings(
  settings: Record<string, unknown>
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/llm/settings`, {
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
    const res = await fetch(`${API_BASE}/api/llm/models?provider=${encodeURIComponent(provider)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { models: [] };
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch LLM models:', err);
    return { models: [] };
  }
}

export async function testLLM(prompt: string, role: string = 'copilot'): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetch(`${API_BASE}/api/llm/test`, {
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
    const res = await fetch(url, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.models || [];
  } catch (err) {
    console.error('Failed to search Hugging Face models:', err);
    return [];
  }
}

export async function getHFModelDetails(
  repoId: string
): Promise<import('@/types').HFModelSummary | null> {
  try {
    const res = await fetch(`${API_BASE}/api/hf/models/${encodeURIComponent(repoId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    const data = await res.json();
    return data.model || null;
  } catch (err) {
    console.error('Failed to fetch HF model details:', err);
    return null;
  }
}

export async function startHFDownload(
  repoId: string,
  filename: string
): Promise<import('@/types').HFDownloadTask | null> {
  try {
    const res = await fetch(`${API_BASE}/api/hf/download`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ repo_id: repoId, filename }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return data.task || null;
  } catch (err) {
    console.error('Failed to start HF download:', err);
    return null;
  }
}

export async function listHFDownloads(): Promise<import('@/types').HFDownloadTask[]> {
  try {
    const res = await fetch(`${API_BASE}/api/hf/downloads`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.downloads || [];
  } catch (err) {
    console.error('Failed to list HF downloads:', err);
    return [];
  }
}

export async function cancelHFDownload(taskId: string): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/hf/downloads/${encodeURIComponent(taskId)}/cancel`, {
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

export async function fetchCustomAgents(): Promise<import('@/types').CustomAgentManifest[]> {
  try {
    const res = await fetch(`${API_BASE}/api/agents/custom`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
    return data.agents || [];
  } catch (err) {
    console.error('Failed to fetch custom agents:', err);
    return [];
  }
}

export async function createCustomAgent(
  manifest: import('@/types').CustomAgentManifest
): Promise<import('@/types').CustomAgentManifest | null> {
  try {
    const res = await fetch(`${API_BASE}/api/agents/custom`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(manifest),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return data.agent || null;
  } catch (err) {
    console.error('Failed to create custom agent:', err);
    return null;
  }
}

export async function deleteCustomAgent(agentId: string): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/agents/custom/${encodeURIComponent(agentId)}`, {
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
    const res = await fetch(`${API_BASE}/api/mesh/route`, {
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
    const res = await fetch(`${API_BASE}/api/mandates/sign`, {
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
    const res = await fetch(`${API_BASE}/api/mandates/verify`, {
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
    const res = await fetch(`${API_BASE}/api/tenants`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return [];
    const data = await res.json();
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
    const res = await fetch(`${API_BASE}/api/supervisor/abort`, {
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
    const res = await fetch(
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
    const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/messages`, {
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
    const res = await fetch(`${API_BASE}/api/chat/regenerate`, {
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
    const res = await fetch(`${API_BASE}/api/chat/feedback`, {
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
    const res = await fetch(`${API_BASE}/api/chat/rewrite`, {
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
    const res = await fetch(`${API_BASE}/api/jobs?session_id=${encodeURIComponent(sessionId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return [];
    const data = await res.json();
    return data.jobs || [];
  } catch (err) {
    console.error('Failed to fetch jobs:', err);
    return [];
  }
}

// Job detail: full params + result payloads for the click-through view.
// Unknown ids 404 → null (never an empty shell).
export async function fetchJob(jobId: string): Promise<import('@/types').Job | null> {
  try {
    const res = await fetch(`${API_BASE}/api/jobs/${encodeURIComponent(jobId)}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    const data = await res.json();
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
    const res = await fetch(`${API_BASE}/api/memory/recall?${params.toString()}`, {
      cache: 'no-store',
      headers: authHeaders(),
    });
    if (!res.ok) return null;
    return await res.json();
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
    const res = await fetch(`${API_BASE}/api/memory/retain`, {
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
  const res = await fetch(`${API_BASE}/api/qir/compile`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ qasm }),
  });
  if (!res.ok) return null;
  return await res.json();
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
    const res = await fetch(`${API_BASE}/api/sessions/context?session_id=${encodeURIComponent(sessionId)}`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
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
  const res = await fetch(`${API_BASE}/api/ledger/append`, {
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
    const res = await fetch(`${API_BASE}/api/health`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { reachable: false, ready: false };
    health = (await res.json()) as Record<string, unknown>;
  } catch {
    return { reachable: false, ready: false };
  }
  let ready = true;
  let message: string | undefined;
  try {
    const res = await fetch(`${API_BASE}/api/ready`, { cache: 'no-store', headers: authHeaders() });
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
    const res = await fetch(
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
  body: Record<string, unknown>
): Promise<TenantWriteResult | TenantWriteError | null> {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
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
  action: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/ceiling', {
    tenant_id: tenantId,
    ceiling_usd: ceilingUsd,
    action,
  });
}

export function setTenantStatus(
  tenantId: string,
  status: 'active' | 'suspended',
  action: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/status', {
    tenant_id: tenantId,
    status,
    action,
  });
}

export function refundTenantSpend(
  tenantId: string,
  amountUsd: number,
  reason: string,
  action: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/refund', {
    tenant_id: tenantId,
    amount_usd: amountUsd,
    reason,
    action,
  });
}

export function registerTenantAdminKey(
  tenantId: string,
  newAdminPubkeyHex: string,
  action?: import('./adminAction').AdminAction
): Promise<TenantWriteResult | TenantWriteError | null> {
  return postTenantMutation('/api/tenants/admin-key', {
    tenant_id: tenantId,
    new_admin_pubkey_hex: newAdminPubkeyHex,
    ...(action ? { action } : {}),
  });
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
    const res = await fetch(`${API_BASE}/api/sessions/export`, {
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
    const res = await fetch(`${API_BASE}/api/sessions/import`, {
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
    const res = await fetch(`${API_BASE}/api/ledger/export`, {
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
