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
