import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CopilotDrawer } from './CopilotDrawer';
import * as api from '@/lib/api';
import type { ChatMessage } from '@/types';

// Partial mock: auth + session + streaming seams are controlled per test;
// everything else stays real (backed by the global fetch stub below).
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    fetchMessages: vi.fn(),
    sendChatMessage: vi.fn(),
    streamChatMessage: vi.fn(),
    createSession: vi.fn(),
    authWhoami: vi.fn(),
    wasRecentAuthFailure: vi.fn(() => false),
  };
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

function userMsg(content: string): ChatMessage {
  return {
    id: 'u-1',
    session_id: 's',
    role: 'user',
    content,
    created_at: '2026-10-26T00:00:00Z',
  };
}

function assistantMsg(content: string): ChatMessage {
  return {
    id: 'a-1',
    session_id: 's',
    role: 'assistant',
    content,
    created_at: '2026-10-26T00:00:01Z',
  };
}

async function sendPrompt(
  tree: TestRenderer.ReactTestRenderer,
  prompt: string,
  until?: () => boolean,
) {
  await act(async () => {
    tree.root
      .findByProps({ 'aria-label': 'Chat message' })
      .props.onChange({ target: { value: prompt } });
  });
  await act(async () => {
    tree.root.findByType('form').props.onSubmit({ preventDefault() {} });
    for (let i = 0; i < 100; i++) {
      await new Promise((r) => setTimeout(r, 5));
      if (!until || until()) break;
    }
  });
}

describe('CopilotDrawer spend-fork guard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
    vi.mocked(api.fetchMessages).mockResolvedValue([]);
    vi.stubGlobal(
      'fetch',
      async () =>
        ({ ok: false, status: 500, json: async () => ({}) }) as unknown as Response,
    );
  });

  it('never mints a session when logged out — no createSession, login affordance renders', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue(null);
    vi.mocked(api.createSession).mockResolvedValue({
      id: 'should-never-exist',
      title: 'x',
    } as never);

    const onSelectSurface = vi.fn();
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(
        <CopilotDrawer sessionId="" onSelectSurface={onSelectSurface} />,
      );
    });

    await sendPrompt(tree!, 'map the H2 surface', () =>
      JSON.stringify(tree!.toJSON()).includes('Session expired'),
    );

    // THE money assertion: auth unknown → no spend-bearing session created.
    expect(api.createSession).not.toHaveBeenCalled();
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Session expired — log in again');

    const jumpBtn = tree!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Open Labs AuthPanel'));
    expect(jumpBtn).toBeDefined();
    await act(async () => {
      jumpBtn!.props.onClick();
    });
    expect(onSelectSurface).toHaveBeenCalledWith('labs');
  });

  it('treats a recent 401 as logged out even when whoami still reads authenticated', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue({
      authenticated: true,
      tenant_id: 'lab-alpha',
    });
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(true);
    vi.mocked(api.createSession).mockResolvedValue({
      id: 'should-never-exist',
      title: 'x',
    } as never);

    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="" />);
    });
    await sendPrompt(tree!, 'probe fidelity', () =>
      JSON.stringify(tree!.toJSON()).includes('Session expired'),
    );

    expect(api.createSession).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain(
      'Session expired — log in again',
    );
  });
});

describe('CopilotDrawer stream abort', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
    vi.mocked(api.fetchMessages).mockResolvedValue([]);
    vi.stubGlobal(
      'fetch',
      async () =>
        ({ ok: false, status: 500, json: async () => ({}) }) as unknown as Response,
    );
  });

  it('unmount mid-stream aborts the reader with no post-unmount writes', async () => {
    const abortSpy = vi.fn();
    vi.mocked(api.streamChatMessage).mockImplementation(
      (_sid, _content, _onToken, signal) =>
        new Promise((_res, rej) => {
          signal?.addEventListener('abort', () => {
            abortSpy();
            rej(new DOMException('Aborted', 'AbortError'));
          });
        }),
    );

    const errors: unknown[][] = [];
    const origError = console.error;
    console.error = (...args: unknown[]) => {
      errors.push(args);
    };
    try {
      let tree: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        tree = TestRenderer.create(<CopilotDrawer sessionId="s1" />);
      });
      await sendPrompt(tree!, 'long running sweep', () =>
        vi.mocked(api.streamChatMessage).mock.calls.length > 0,
      );
      expect(api.streamChatMessage).toHaveBeenCalledTimes(1);

      await act(async () => {
        tree!.unmount();
      });
      expect(abortSpy).toHaveBeenCalledTimes(1);

      // Let the abort rejection (and any stragglers) settle post-unmount.
      await act(async () => {
        await new Promise((r) => setTimeout(r, 30));
      });
      const actWarnings = errors.filter((args) =>
        args.some(
          (a) =>
            typeof a === 'string' &&
            (a.includes('not wrapped in act') || a.includes('unmounted')),
        ),
      );
      expect(actWarnings).toEqual([]);
    } finally {
      console.error = origError;
    }
  });

  it('switching session aborts the previous in-flight stream', async () => {
    const abortSpy = vi.fn();
    vi.mocked(api.streamChatMessage).mockImplementation(
      (_sid, _content, _onToken, signal) =>
        new Promise((_res, rej) => {
          signal?.addEventListener('abort', () => {
            abortSpy();
            rej(new DOMException('Aborted', 'AbortError'));
          });
        }),
    );

    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s1" />);
    });
    await sendPrompt(tree!, 'sweep one', () =>
      vi.mocked(api.streamChatMessage).mock.calls.length > 0,
    );

    await act(async () => {
      tree!.update(<CopilotDrawer sessionId="s2" />);
    });
    expect(abortSpy).toHaveBeenCalled();
  });
});

describe('CopilotDrawer malformed transcript', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
    vi.stubGlobal(
      'fetch',
      async () =>
        ({ ok: false, status: 500, json: async () => ({}) }) as unknown as Response,
    );
  });

  it('search over a null-content entry does not crash', async () => {
    vi.mocked(api.fetchMessages).mockResolvedValue([
      {
        id: 'm-null',
        session_id: 's',
        role: 'assistant',
        content: null,
        created_at: '2026-10-26T00:00:00Z',
      } as unknown as ChatMessage,
      assistantMsg('tribunal convened for H2'),
    ]);

    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    // Open in-chat search and type — the matching path coerces content.
    await act(async () => {
      tree!.root
        .findByProps({ 'aria-label': 'Search this conversation' })
        .props.onClick();
    });
    await act(async () => {
      tree!.root
        .findByProps({ 'aria-label': 'Find in conversation' })
        .props.onChange({ target: { value: 'tribunal' } });
    });

    expect(tree!.toJSON()).toBeTruthy();
    expect(JSON.stringify(tree!.toJSON())).toContain('tribunal convened');
  });
});

describe('CopilotDrawer stream stall UX', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
    vi.mocked(api.fetchMessages).mockResolvedValue([]);
    vi.mocked(api.sendChatMessage).mockResolvedValue(null);
    vi.stubGlobal(
      'fetch',
      async () =>
        ({ ok: false, status: 500, json: async () => ({}) }) as unknown as Response,
    );
  });

  it('stall rejection renders an inline error with a Retry that re-sends the same prompt', async () => {
    vi.mocked(api.streamChatMessage)
      .mockRejectedValueOnce(new Error('stream stalled — retry'))
      .mockImplementation(async (sid, content) => ({
        user: { ...userMsg(content), session_id: sid },
        assistant: { ...assistantMsg('recovered answer'), session_id: sid },
      }));

    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    await sendPrompt(tree!, 'canary on heron', () =>
      JSON.stringify(tree!.toJSON()).includes('stream stalled'),
    );

    // Stall surfaces inline — no silent unary fallback masking it.
    expect(api.sendChatMessage).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('stream stalled');

    const retryBtn = tree!.root.findByProps({
      'aria-label': 'Retry stalled stream',
    });
    await act(async () => {
      retryBtn.props.onClick();
      for (let i = 0; i < 100; i++) {
        await new Promise((r) => setTimeout(r, 5));
        if (vi.mocked(api.streamChatMessage).mock.calls.length >= 2) break;
      }
    });

    const calls = vi.mocked(api.streamChatMessage).mock.calls;
    expect(calls.length).toBeGreaterThanOrEqual(2);
    // Same session, same prompt — a true re-send, not a new compose.
    expect(calls[1][0]).toBe(calls[0][0]);
    expect(calls[1][1]).toBe(calls[0][1]);
    expect(calls[1][1]).toContain('canary on heron');
  });
});
