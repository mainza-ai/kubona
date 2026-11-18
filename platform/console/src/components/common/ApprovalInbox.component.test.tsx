import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ApprovalInbox } from './ApprovalInbox';

afterEach(() => { vi.unstubAllGlobals(); });

type Post = { url: string; body: Record<string, unknown> };

const plan = {
  id: 'p1',
  statement: 'Run the H2 expressivity probe',
  risk: 'low',
  method: 'vqe',
  observable: 'energy',
  acceptance_metric: 'energy_exact',
  tolerance: 0.05,
  stop_rule: 'one run',
  budget_usd: 1,
  spec: '{"kind":"vqe","molecule":"H2"}',
  created_at: '2026-11-18T10:00:00Z',
};

function json(payload: unknown) {
  return { ok: true, status: 200, text: async () => JSON.stringify(payload), json: async () => payload };
}

function stubInbox(opts: { plans?: unknown[]; tools?: unknown[]; posts?: Post[] } = {}) {
  let plans = [...(opts.plans ?? [])];
  let tools = [...(opts.tools ?? [])];
  const posts = opts.posts ?? [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : null;
    if (init?.method === 'POST' && url.includes('/tool-approvals/')) {
      posts.push({ url, body: body ?? {} });
      tools = [];
      return json({ approval: { id: 'ta1', status: 'approved' } });
    }
    if (init?.method === 'POST' && url.includes('/approvals/')) {
      posts.push({ url, body: body ?? {} });
      plans = [];
      return json({ decision: body?.decision ?? '', plan: {} });
    }
    if (url.endsWith('/tool-approvals')) {
      return json({
        session_id: 's1', pending: tools, recent: [],
        counts: { pending: tools.length, approved: 0, denied: 0, used: 0, expired: 0 },
      });
    }
    if (url.endsWith('/approvals')) {
      return json({ session_id: 's1', pending: plans, deferred: [], counts: { pending: plans.length, deferred: 0 } });
    }
    throw new Error(`unexpected ${init?.method ?? 'GET'} ${url}`);
  });
}

describe('ApprovalInbox in a real browser', () => {
  it('approves a pending plan once and refreshes the inbox', async () => {
    const posts: Post[] = [];
    stubInbox({ plans: [plan], posts });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Approve' }).click();
    await expect.poll(() => posts.map((p) => p.body)).toEqual([{ decision: 'approve' }]);
    await expect.poll(() => screen.container.textContent).toContain('No pending approvals.');
    await expect.poll(() => screen.container.textContent).toContain('Approved.');
  });

  it('defers with a future timestamp', async () => {
    const posts: Post[] = [];
    stubInbox({ plans: [plan], posts });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Defer 1h' }).click();
    await expect.poll(() => posts.length).toBe(1);
    expect(posts[0].body.decision).toBe('defer');
    const until = new Date(String(posts[0].body.deferred_until)).getTime();
    expect(until).toBeGreaterThan(Date.now() + 55 * 60 * 1000);
    await expect.poll(() => screen.container.textContent).toContain('Deferred.');
  });

  it('sends reviewer edits with the approval', async () => {
    const posts: Post[] = [];
    stubInbox({ plans: [plan], posts });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Adjust' }).click();

    const statement = screen.getByRole('textbox', { name: 'Statement' });
    await statement.fill('Reviewed expressivity probe');
    const tolerance = screen.getByRole('spinbutton', { name: 'Tolerance' });
    await tolerance.fill('0.02');

    await screen.getByRole('button', { name: 'Approve' }).click();
    await expect.poll(() => posts.length).toBe(1);
    expect(posts[0].body.decision).toBe('approve');
    expect(posts[0].body.edits).toEqual({
      statement: 'Reviewed expressivity probe',
      tolerance: 0.02,
    });
    await expect.poll(() => screen.container.textContent).toContain('Approved with revisions.');
  });

  it('refuses an invalid edited spec before posting', async () => {
    const posts: Post[] = [];
    stubInbox({ plans: [plan], posts });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Adjust' }).click();
    await screen.getByRole('textbox', { name: 'Spec (JSON)' }).fill('{not json');
    await screen.getByRole('button', { name: 'Approve' }).click();
    await expect.poll(() => screen.container.textContent).toContain('Spec is not valid JSON.');
    expect(posts.length).toBe(0);
  });

  it('rejects with durable feedback', async () => {
    const posts: Post[] = [];
    stubInbox({ plans: [plan], posts });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Adjust' }).click();
    await screen.getByRole('textbox', { name: 'Feedback (attached on reject)' }).fill('Tighten the tolerance first.');
    await screen.getByRole('button', { name: 'Reject' }).click();
    await expect.poll(() => posts.length).toBe(1);
    expect(posts[0].body).toEqual({ decision: 'reject', feedback: 'Tighten the tolerance first.' });
    await expect.poll(() => screen.container.textContent).toContain('Rejected with feedback.');
  });

  it('approves a held tool call once', async () => {
    const posts: Post[] = [];
    stubInbox({
      tools: [{
        id: 'ta1', tool: 'run_vqe', args: '{"molecule":"H2"}', status: 'pending',
        requested_at: '2026-11-18T10:00:00Z',
      }],
      posts,
    });
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('run_vqe');
    await expect.poll(() => screen.container.textContent).toContain('molecule');
    await screen.getByRole('button', { name: 'Approve once' }).click();
    await expect.poll(() => posts.map((p) => p.body)).toEqual([{ decision: 'approve' }]);
    expect(posts[0].url).toContain('/tool-approvals/ta1');
    await expect.poll(() => screen.container.textContent).toContain('Tool call approved once.');
  });
});
