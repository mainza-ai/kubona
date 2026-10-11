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
}

// Streaming twin of sendChatMessage: tokens arrive live via SSE
// (token events), then a done event carries the stored records.
// Throws on transport failure so the caller can fall back to unary.
export async function streamChatMessage(
  sessionId: string,
  content: string,
  onToken: (text: string) => void
): Promise<StreamDone> {
  const res = await fetch(`${API_BASE}/api/chat/stream?session_id=${encodeURIComponent(sessionId)}`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ session_id: sessionId, role: 'user', content }),
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

export async function launchProposal(sessionId: string, proposal: unknown): Promise<{ job: Job; loop: Loop } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chat/launch`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ session_id: sessionId, proposal }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to launch proposal:', err);
    return null;
  }
}

export async function fetchLoops(): Promise<Loop[]> {
  try {
    const res = await fetch(`${API_BASE}/api/loops`, { cache: 'no-store', headers: authHeaders() });
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
  try {
    const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/campaign/synthesize`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ prompt }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to synthesize campaign:', err);
    return null;
  }
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

export async function approveMetasearchProposal(proposalId: string, signer: string = 'human-quantum-engineer'): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/metasearch/approve`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ proposal_id: proposalId, signer }),
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to approve proposal:', err);
    return false;
  }
}

export async function applyMetasearchProposal(proposalId: string): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/metasearch/apply`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ proposal_id: proposalId }),
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to apply proposal:', err);
    return false;
  }
}

export async function fetchFleet(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/fleet`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { backends: [] };
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch fleet:', err);
    return { backends: [] };
  }
}

export async function fetchRadar(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/radar`, { cache: 'no-store', headers: authHeaders() });
    if (!res.ok) return { radar_endpoints: [] };
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch radar:', err);
    return { radar_endpoints: [] };
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

export async function saveLLMSettings(settings: Record<string, unknown>): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/llm/settings`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(settings),
    });
    return res.ok;
  } catch (err) {
    console.error('Failed to save LLM settings:', err);
    return false;
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

export async function searchLiterature(query: string, limit = 5): Promise<{ records: import('@/types').LiteratureRecord[]; cached: boolean }> {
  try {
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
