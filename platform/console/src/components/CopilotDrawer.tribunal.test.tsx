import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CopilotDrawer } from './CopilotDrawer';

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

describe('CopilotDrawer tribunal badge', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('reads CONTESTED on a finished tribunal without consensus, never DELIBERATING', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      json: async () => ({ messages: [tribunalMessage(false)] }),
    }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('CONTESTED');
    expect(text).not.toContain('DELIBERATING');
  });

  it('reads CONSENSUS VERIFIED when consensus passed', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      json: async () => ({ messages: [tribunalMessage(true)] }),
    }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('CONSENSUS VERIFIED');
  });
});

describe('CopilotDrawer no-proposal hint', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('names the exit paths when the engine notice carries no proposal', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      json: async () => ({
        messages: [{
          id: 'm-note',
          session_id: 's',
          role: 'assistant',
          content: '⚠️ [opencode Notice] (deepseek-v4.1-flash): disabled. Check the provider API key and account balance.',
          created_at: '2026-10-17T00:00:00Z',
        }],
      }),
    }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('NO PROPOSAL');
    expect(text).toContain('Simulate Offline');
  });

  it('stays silent on ordinary assistant text without a proposal', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      json: async () => ({
        messages: [{
          id: 'm-plain',
          session_id: 's',
          role: 'assistant',
          content: 'Hello — how can I help?',
          created_at: '2026-10-17T00:00:00Z',
        }],
      }),
    }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    expect(JSON.stringify(tree!.toJSON())).not.toContain('NO PROPOSAL');
  });
});
