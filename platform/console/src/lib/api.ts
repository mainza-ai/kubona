import { Session, ChatMessage, Job, Loop, LedgerEvent } from '@/types';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

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

export async function createSession(title: string, spendLimitUSD: number = 100): Promise<Session | null> {
  try {
    const res = await fetch(`${API_BASE}/api/sessions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, spend_limit_usd: spendLimitUSD }),
    });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Failed to create session:', err);
    return null;
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

export async function fetchMolecularADMET(smiles: string): Promise<any | null> {
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
): Promise<any | null> {
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

