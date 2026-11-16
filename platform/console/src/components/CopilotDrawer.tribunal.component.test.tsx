import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CopilotDrawer } from './CopilotDrawer';

// No @/lib/api mock: the drawer's transcript read is answered at the fetch
// boundary, so the stored message (and its tribunal block) travels the real
// request/contract path into real DOM.
const CONTEXT = {
  session_id: 's',
  turns: 0,
  turn_cap: 40,
  history_chars: 0,
  history_chars_cap: 48000,
  would_drop: 0,
  pressure_pct: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
};

function stubTranscript(messages: unknown[]) {
  const calls: string[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push(`${method} ${path}`);
    if (method === 'GET' && path === '/api/sessions/s/messages') {
      return { ok: true, status: 200, json: async () => ({ messages }) };
    }
    if (method === 'GET' && path === '/api/chat/feedback') {
      return { ok: true, status: 200, json: async () => ({ feedback: [] }) };
    }
    if (method === 'GET' && path === '/api/sessions/context') {
      return { ok: true, status: 200, json: async () => CONTEXT };
    }
    throw new Error(`unexpected fetch ${method} ${path}`);
  });
  return calls;
}

function tribunalMessage(consensus: boolean) {
  return {
    id: 'm-trib-1',
    session_id: 's',
    role: 'assistant',
    content: 'Tribunal complete.',
    created_at: '2026-10-12T00:00:00Z',
    tribunal: {
      target: 'H2',
      molecule: 'H2',
      consensus_passed: consensus,
      synthesis: 'done',
      turns: [
        { turn_index: 0, role: 'ansatz_scout', name: 'Ansatz Scout', content: 'looks fine', verdict: 'pass' },
        { turn_index: 1, role: 'disconfirmation_officer', name: 'Disconfirmation Officer', content: 'not so fast', verdict: 'contest' },
      ],
    },
  };
}

function noticeMessage() {
  return {
    id: 'm-note',
    session_id: 's',
    role: 'assistant',
    content: '⚠️ [opencode Notice] (deepseek-v4.1-flash): disabled. Check the provider API key and account balance.',
    created_at: '2026-10-17T00:00:00Z',
  };
}

function plainMessage() {
  return {
    id: 'm-plain',
    session_id: 's',
    role: 'assistant',
    content: 'Hello — how can I help?',
    created_at: '2026-10-17T00:00:00Z',
  };
}

afterEach(() => vi.unstubAllGlobals());

describe('CopilotDrawer tribunal badge in a real browser', () => {
  it('reads CONTESTED on a finished tribunal without consensus, never DELIBERATING', async () => {
    const calls = stubTranscript([tribunalMessage(false)]);
    const screen = await render(<CopilotDrawer sessionId="s" />);

    await expect.poll(() => screen.container.textContent).toContain('CONTESTED');
    expect(screen.container.textContent).not.toContain('DELIBERATING');
    // The badge is driven by the stored record, which really crossed the wire.
    expect(calls).toContain('GET /api/sessions/s/messages');
  });

  it('reads CONSENSUS VERIFIED when consensus passed', async () => {
    stubTranscript([tribunalMessage(true)]);
    const screen = await render(<CopilotDrawer sessionId="s" />);

    await expect.poll(() => screen.container.textContent).toContain('CONSENSUS VERIFIED');
    expect(screen.container.textContent).not.toContain('DELIBERATING');
  });
});

describe('CopilotDrawer no-proposal hint in a real browser', () => {
  it('names the exit paths when the engine notice carries no proposal', async () => {
    stubTranscript([noticeMessage()]);
    const screen = await render(<CopilotDrawer sessionId="s" />);

    await expect.poll(() => screen.container.textContent).toContain('NO PROPOSAL');
    expect(screen.container.textContent).toContain('Simulate Offline');
  });

  it('stays silent on ordinary assistant text without a proposal', async () => {
    stubTranscript([plainMessage()]);
    const screen = await render(<CopilotDrawer sessionId="s" />);

    await expect.poll(() => screen.container.textContent).toContain('Hello — how can I help?');
    expect(screen.container.textContent).not.toContain('NO PROPOSAL');
  });
});
