import { Session, ChatMessage, Job, Loop, LedgerEvent } from '@/types';

export const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export async function fetchSessions(): Promise<Session[]> {
  try {
    const res = await fetch(`${API_BASE}/api/sessions`, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/sessions/${sessionId}/messages`, { cache: 'no-store' });
    if (!res.ok) return [];
    const data = await res.json();
    return data.messages || [];
  } catch (err) {
    console.error('Failed to fetch messages:', err);
    return [];
  }
}

export async function sendChatMessage(sessionId: string, content: string): Promise<{ user: ChatMessage; assistant: ChatMessage } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chat/messages?session_id=${encodeURIComponent(sessionId)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionId, role: 'user', content }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return {
      user: data.user || data.user_message,
      assistant: data.assistant || data.assistant_message,
    };
  } catch (err) {
    console.error('Failed to send chat message:', err);
    return null;
  }
}

export async function runTribunal(
  sessionId: string,
  prompt: string
): Promise<{ tribunal: import('@/types').TribunalResult; assistant_message: ChatMessage; campaign?: import('@/types').CampaignState } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chat/tribunal`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/loops`, { cache: 'no-store' });
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
    const res = await fetch(url, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/autoresearch/harnesses`, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/autoresearch/loops`, { cache: 'no-store' });
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
    const res = await fetch(`${API_BASE}/api/metasearch/proposals`, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/fleet`, { cache: 'no-store' });
    if (!res.ok) return { backends: [] };
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch fleet:', err);
    return { backends: [] };
  }
}

export async function fetchRadar(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/radar`, { cache: 'no-store' });
    if (!res.ok) return { radar_endpoints: [] };
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch radar:', err);
    return { radar_endpoints: [] };
  }
}

export async function fetchEpistemics(): Promise<Record<string, unknown>> {
  try {
    const res = await fetch(`${API_BASE}/api/epistemics`, { cache: 'no-store' });
    if (!res.ok) return {};
    return await res.json();
  } catch (err) {
    console.error('Failed to fetch epistemics:', err);
    return {};
  }
}

export async function fetchLLMSettings(): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetch(`${API_BASE}/api/llm/settings`, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
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
    const res = await fetch(`${API_BASE}/api/llm/models?provider=${encodeURIComponent(provider)}`, { cache: 'no-store' });
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
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ role, prompt }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to test LLM:', err);
    return null;
  }
}



