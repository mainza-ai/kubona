import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { WorkflowPanel } from './WorkflowPanel';

const PLAN = {
  ok: true, dry_run: true, workflow_id: 'wf-1', steps: [], unresolved_references: [], compensations_planned: [],
};

function runBody(workflowId: string) {
  return {
    ok: true, workflow_id: workflowId, steps_completed: 1, step_records: [], spent_usd: 0, last_results: {}, compensations: [],
  };
}

type Call = { path: string; body: Record<string, unknown> };

function stubWorkflowFetch(routes: Record<string, unknown>, calls: Call[]) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    if (init?.method !== 'POST') throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`);
    calls.push({ path, body: JSON.parse(String(init.body)) as Record<string, unknown> });
    if (!(path in routes)) throw new Error(`unexpected fetch POST ${url}`);
    return { ok: true, status: 200, json: async () => routes[path] };
  });
}

type Screen = Awaited<ReturnType<typeof render>>;

async function openConfirmDialog(screen: Screen) {
  await vi.waitFor(() => {
    if (!document.querySelector('[role="alertdialog"]')) throw new Error('confirm dialog not open');
  });
  return screen.getByRole('alertdialog');
}

async function confirmDialog(screen: Screen, workflowName: string) {
  const dialog = await openConfirmDialog(screen);
  await dialog.getByRole('textbox').fill(workflowName);
  const confirm = screen.getByRole('button', { name: 'Run workflow' });
  // The dialog holds confirm inert for the consequential settle window.
  await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(false);
  await confirm.click();
}

afterEach(() => vi.unstubAllGlobals());

describe('WorkflowPanel in a real browser', () => {
  it('plans, confirms by name, and runs through the backend', async () => {
    const calls: Call[] = [];
    stubWorkflowFetch(
      { '/api/workflows/plan': PLAN, '/api/workflows/run': runBody('wf-1') },
      calls,
    );

    const screen = await render(<WorkflowPanel sessionId="s1" />);
    await screen.getByRole('button', { name: /^Plan/ }).click();
    await expect.poll(() => document.body.textContent).toContain('plan wf-1 · dry_run true · steps 0');
    expect(calls[0].path).toBe('/api/workflows/plan');
    expect((calls[0].body.workflow as Record<string, unknown>).name).toBe('provider-audit');

    await screen.getByRole('button', { name: 'Run workflow…' }).click();
    const dialog = await openConfirmDialog(screen);
    // Confirm stays inert until the workflow name is typed exactly.
    const confirm = screen.getByRole('button', { name: 'Run workflow' });
    await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(true);
    await dialog.getByRole('textbox').fill('provider-audit');
    await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(false);
    await confirm.click();

    await expect.poll(() => document.body.textContent).toContain('completed 1');
    expect(calls[1].path).toBe('/api/workflows/run');
    expect((calls[1].body.workflow as Record<string, unknown>).name).toBe('provider-audit');
    expect(calls[1].body.approval_token).toBeUndefined();
  });

  it('mints and passes an approval token for agent workflows', async () => {
    const calls: Call[] = [];
    stubWorkflowFetch(
      {
        '/api/workflows/approve': {
          ok: true, approval_token: 'signed-token', workflow_sha256: 'abc', expires_at: '2099-01-01T00:00:00Z',
        },
        '/api/workflows/run': runBody('wf-2'),
      },
      calls,
    );

    const screen = await render(<WorkflowPanel sessionId="s1" />);
    await screen.getByRole('button', { name: 'Approve current workflow' }).click();
    await expect.poll(() => (screen.getByLabelText('Workflow approval token').element() as HTMLInputElement).value)
      .toBe('signed-token');
    expect(calls[0].path).toBe('/api/workflows/approve');

    await screen.getByRole('button', { name: 'Run workflow…' }).click();
    await confirmDialog(screen, 'provider-audit');

    await expect.poll(() => document.body.textContent).toContain('workflow wf-2');
    expect(calls[1].path).toBe('/api/workflows/run');
    expect(calls[1].body.approval_token).toBe('signed-token');
  });
});
