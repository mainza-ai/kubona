import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, type RenderResult } from 'vitest-browser-react';
import React from 'react';
import { CopilotDrawer } from './CopilotDrawer';

// No @/lib/api mock: every seam this suite used to mock (session minting,
// transcript read, stream, regenerate) is answered at the transport boundary
// by URL + method, so the real request/contract/settlement path runs and the
// assertions observe real DOM after real clicks and typing.
//
// Ordering note: the final describe returns a real HTTP 401. '@/lib/api'
// remembers that for 30s (wasRecentAuthFailure) in module state that no test
// hook resets, and the stream error path checks it before the stall branch.
// The 401 case therefore runs last; every other test above it must never see
// a stale auth failure.

type Call = {
  method: string;
  path: string;
  query: URLSearchParams;
  body: Record<string, unknown> | undefined;
  signal: AbortSignal | null;
};

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

function json(data: unknown, status = 200) {
  return { ok: status < 400, status, json: async () => data, text: async () => JSON.stringify(data) };
}

/** A real streaming response body: api.ts reads it with getReader(). */
function sse(frames: Array<{ event: string; data: unknown }>) {
  const body = frames.map((frame) => `event: ${frame.event}\ndata: ${JSON.stringify(frame.data)}\n\n`).join('');
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
}

type Handler = (call: Call) => unknown;

/** Stub the daemon by "METHOD /path" and record every call it receives. */
function stubBackend(routes: Record<string, Handler>) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost:3000');
      const call: Call = {
        method: (init?.method ?? 'GET').toUpperCase(),
        path: url.pathname,
        query: url.searchParams,
        body: typeof init?.body === 'string' ? (JSON.parse(init.body) as Record<string, unknown>) : undefined,
        signal: init?.signal ?? null,
      };
      calls.push(call);
      const handler = routes[`${call.method} ${call.path}`];
      if (!handler) throw new Error(`unexpected fetch ${call.method} ${call.path}`);
      return handler(call);
    }),
  );
  return calls;
}

function transcriptRoutes(sessionId: string, messages: unknown[] = []) {
  return {
    [`GET /api/sessions/${sessionId}/messages`]: () => json({ messages }),
    'GET /api/chat/feedback': () => json({ feedback: [] }),
    'GET /api/sessions/context': () => json({ ...CONTEXT, session_id: sessionId }),
  } satisfies Record<string, Handler>;
}

function userMsg(content: string, sessionId = 's') {
  return {
    id: 'u-1',
    session_id: sessionId,
    role: 'user',
    content,
    created_at: '2026-10-26T00:00:00Z',
  };
}

function assistantMsg(content: string, sessionId = 's') {
  return {
    id: 'a-1',
    session_id: sessionId,
    role: 'assistant',
    content,
    created_at: '2026-10-26T00:00:01Z',
  };
}

async function sendPrompt(screen: RenderResult, prompt: string) {
  await screen.getByLabelText('Chat message').fill(prompt);
  const send = screen.getByRole('button', { name: 'Send directive', exact: true });
  await expect.poll(() => (send.element() as HTMLButtonElement).disabled).toBe(false);
  await send.click();
}

const callsTo = (calls: Call[], method: string, path: string) =>
  calls.filter((call) => call.method === method && call.path === path);

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('CopilotDrawer spend-fork guard in a real browser', () => {
  it('pre-empts a multi-lab refusal before minting any session', async () => {
    const calls = stubBackend({});
    const onSelectSurface = vi.fn();
    const screen = await render(
      <CopilotDrawer sessionId="" onSelectSurface={onSelectSurface} requiresSignIn />,
    );

    await sendPrompt(screen, 'map the H2 surface');
    await expect.poll(() => screen.container.textContent).toContain('Sign in to start a campaign session');

    // The page already knows the store requires auth: no POST /api/sessions,
    // so no spend-bearing work was ever minted.
    expect(callsTo(calls, 'POST', '/api/sessions')).toHaveLength(0);

    const signIn = screen.getByRole('button', { name: 'Sign in', exact: true });
    await expect.element(signIn).toBeVisible();
    await signIn.click();
    expect(onSelectSurface).toHaveBeenCalledWith('labs');
  });

  it('allows a single-lab local send without a whoami prerequisite', async () => {
    const calls = stubBackend({
      'POST /api/sessions': () =>
        json({ id: 'local-session', title: 'x', status: 'active', created_at: '', updated_at: '' }),
      'POST /api/chat/stream': () =>
        sse([
          { event: 'started', data: {} },
          {
            event: 'done',
            data: {
              user: userMsg('hello', 'local-session'),
              assistant: assistantMsg('local reply', 'local-session'),
            },
          },
        ]),
    });
    const screen = await render(<CopilotDrawer sessionId="" />);

    await sendPrompt(screen, 'hello');
    await expect.poll(() => screen.container.textContent).toContain('local reply');

    expect(callsTo(calls, 'POST', '/api/sessions')).toHaveLength(1);
    const streams = callsTo(calls, 'POST', '/api/chat/stream');
    expect(streams).toHaveLength(1);
    // The minted session is the one the stream actually writes to.
    expect(streams[0].query.get('session_id')).toBe('local-session');
    expect(streams[0].body).toMatchObject({ content: 'hello', role: 'user' });
  });
});

describe('CopilotDrawer stream abort in a real browser', () => {
  function abortableStream(aborted: string[]) {
    return (call: Call) =>
      new Promise((_resolve, reject) => {
        call.signal?.addEventListener('abort', () => {
          aborted.push('abort');
          reject(new DOMException('Aborted', 'AbortError'));
        });
      });
  }

  it('unmount mid-stream aborts the reader with no post-unmount writes', async () => {
    const aborted: string[] = [];
    const calls = stubBackend({
      ...transcriptRoutes('s1'),
      'POST /api/chat/stream': abortableStream(aborted),
    });

    const errors: unknown[][] = [];
    const origError = console.error;
    console.error = (...args: unknown[]) => {
      errors.push(args);
    };
    try {
      const screen = await render(<CopilotDrawer sessionId="s1" />);
      await sendPrompt(screen, 'long running sweep');
      // The stream is open and nothing has aborted it yet.
      await expect.poll(() => callsTo(calls, 'POST', '/api/chat/stream').length).toBe(1);
      expect(aborted).toHaveLength(0);

      await screen.unmount();
      expect(aborted).toHaveLength(1);

      // Let the abort rejection (and any stragglers) settle post-unmount.
      await new Promise((resolve) => setTimeout(resolve, 30));
      const actWarnings = errors.filter((args) =>
        args.some(
          (arg) =>
            typeof arg === 'string' &&
            (arg.includes('not wrapped in act') || arg.includes('unmounted')),
        ),
      );
      expect(actWarnings).toEqual([]);
    } finally {
      console.error = origError;
    }
  });

  it('switching session aborts the previous in-flight stream', async () => {
    const aborted: string[] = [];
    const calls = stubBackend({
      ...transcriptRoutes('s1'),
      ...transcriptRoutes('s2'),
      'POST /api/chat/stream': abortableStream(aborted),
    });

    const screen = await render(<CopilotDrawer sessionId="s1" />);
    await sendPrompt(screen, 'sweep one');
    // The stream is open and the drawer is still mounted when the session flips.
    await expect.poll(() => callsTo(calls, 'POST', '/api/chat/stream').length).toBe(1);
    expect(aborted).toHaveLength(0);

    await screen.rerender(<CopilotDrawer sessionId="s2" />);
    await expect.poll(() => aborted.length).toBeGreaterThanOrEqual(1);
  });
});

describe('CopilotDrawer malformed transcript in a real browser', () => {
  it('search over a null-content entry does not crash', async () => {
    stubBackend(
      transcriptRoutes('s', [
        {
          id: 'm-null',
          session_id: 's',
          role: 'assistant',
          content: null,
          created_at: '2026-10-26T00:00:00Z',
        },
        assistantMsg('tribunal convened for H2'),
      ]),
    );

    const screen = await render(<CopilotDrawer sessionId="s" />);
    await expect.poll(() => screen.container.textContent).toContain('tribunal convened');

    // Open in-chat search and type — the matching path coerces content.
    await screen.getByRole('button', { name: 'Search this conversation' }).click();
    await screen.getByLabelText('Find in conversation').fill('tribunal');

    await expect.poll(() => screen.container.textContent).toContain('1/1');
    expect(screen.container.textContent).toContain('tribunal convened');
  });
});

describe('CopilotDrawer stream stall UX in a real browser', () => {
  it('stall rejection renders an inline error with a Retry that re-sends the same prompt', async () => {
    const calls = stubBackend({
      ...transcriptRoutes('s'),
      'POST /api/chat/stream': () => {
        if (callsTo(calls, 'POST', '/api/chat/stream').length > 1) {
          return sse([
            { event: 'started', data: {} },
            {
              event: 'done',
              data: {
                user: userMsg('canary on heron'),
                assistant: assistantMsg('recovered answer'),
              },
            },
          ]);
        }
        // A frame-level failure before `started`: the prompt was not stored,
        // and the message is the stall wording the drawer recognises.
        return sse([{ event: 'error', data: { error: 'stream stalled — retry' } }]);
      },
    });

    const screen = await render(<CopilotDrawer sessionId="s" />);
    await sendPrompt(screen, 'canary on heron');
    await expect.poll(() => screen.container.textContent).toContain('stream stalled');

    // Stall surfaces inline — no silent unary fallback masking it.
    expect(callsTo(calls, 'POST', '/api/chat/messages')).toHaveLength(0);

    const retry = screen.getByRole('button', { name: 'Retry stalled stream' });
    await expect.element(retry).toBeVisible();
    await retry.click();
    await expect.poll(() => screen.container.textContent).toContain('recovered answer');

    const streams = callsTo(calls, 'POST', '/api/chat/stream');
    expect(streams.length).toBeGreaterThanOrEqual(2);
    // Same session, same prompt, same idempotency key — a true re-send, not a
    // new compose.
    expect(streams[1].query.get('session_id')).toBe(streams[0].query.get('session_id'));
    expect(streams[1].body?.content).toBe(streams[0].body?.content);
    expect(streams[1].body?.content).toContain('canary on heron');
    expect(streams[1].body?.request_id).toBe(streams[0].body?.request_id);
  });

  it('re-answers a prompt the stream already stored instead of posting it again', async () => {
    const calls = stubBackend({
      ...transcriptRoutes('s'),
      'POST /api/chat/stream': () =>
        sse([
          { event: 'started', data: {} },
          { event: 'error', data: { error: 'stream ended without done event' } },
        ]),
      'POST /api/chat/regenerate': () => json({ assistant: assistantMsg('recovered answer') }),
    });

    const screen = await render(<CopilotDrawer sessionId="s" />);
    await sendPrompt(screen, 'stored once');
    await expect.poll(() => screen.container.textContent).toContain('recovered answer');

    // The prompt row already exists server-side: no second user turn is posted.
    expect(callsTo(calls, 'POST', '/api/chat/messages')).toHaveLength(0);
    const regenerates = callsTo(calls, 'POST', '/api/chat/regenerate');
    expect(regenerates).toHaveLength(1);
    expect(regenerates[0].body?.session_id).toBe('s');
  });
});

describe('CopilotDrawer server create refusal in a real browser', () => {
  // Runs last on purpose: a real 401 plants '@/lib/api' module state that
  // outlives the per-test auth reset (see the file header note).
  it('maps a server create refusal to the sign-in affordance', async () => {
    const calls = stubBackend({
      'POST /api/sessions': () => json({ message: 'sign in required' }, 401),
    });
    const screen = await render(<CopilotDrawer sessionId="" />);

    await sendPrompt(screen, 'probe fidelity');
    await expect.poll(() => screen.container.textContent).toContain('Sign in to start a campaign session');

    // The server is the authority: the create was attempted and refused; no
    // stream or spend-bearing turn followed.
    expect(callsTo(calls, 'POST', '/api/sessions')).toHaveLength(1);
    expect(callsTo(calls, 'POST', '/api/chat/stream')).toHaveLength(0);
    expect(callsTo(calls, 'POST', '/api/chat/messages')).toHaveLength(0);
  });
});
