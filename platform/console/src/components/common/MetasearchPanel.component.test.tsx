import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { MetasearchPanel } from './MetasearchPanel';

afterEach(() => {
  vi.unstubAllGlobals();
});

function proposal(partial: Record<string, unknown> = {}) {
  return {
    id: 'prop-1',
    loop_id: 'loop-1',
    harness_id: 'h1',
    target_file: 'executor.py',
    status: 'pending_approval',
    current_instructions: 'old',
    proposed_diff: '--- a\n+++ b',
    rationale: 'faster convergence',
    approved_by_human: false,
    created_at: '',
    ...partial,
  };
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(String(url), init));
}

function buttonLabels(screen: Awaited<ReturnType<typeof render>>): Array<string | null> {
  return Array.from(screen.container.querySelectorAll('button')).map((button) => button.getAttribute('aria-label'));
}

describe('MetasearchPanel in a real browser (U1-d)', () => {
  it('lists proposals with approve/reject on pending ones', async () => {
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, status: 200, json: async () => ({ proposals: [proposal()] }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<MetasearchPanel />);
    await expect.element(screen.getByText('faster convergence')).toBeVisible();
    const labels = buttonLabels(screen);
    expect(labels).toContain('Approve proposal prop-1');
    expect(labels).toContain('Reject proposal prop-1');
    await expect.element(screen.getByRole('button', { name: 'Approve proposal prop-1' })).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Reject proposal prop-1' })).toBeVisible();
  });

  it('approve posts proposal_id and refetches the list', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    let approved = false;
    stubFetch((url, init) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, status: 200, json: async () => ({ proposals: approved ? [] : [proposal()] }) };
      }
      seen.push({ url, body: String(init?.body) });
      approved = true;
      return { ok: true, status: 200, json: async () => ({ success: true }) };
    });
    const screen = await render(<MetasearchPanel />);
    await screen.getByRole('button', { name: 'Approve proposal prop-1' }).click();
    await expect.poll(() => screen.container.textContent).toContain('No proposals pending');
    expect(seen.map((s) => s.url)).toEqual(['http://localhost:8080/api/metasearch/approve']);
    expect(JSON.parse(seen[0].body)).toEqual({ proposal_id: 'prop-1', signer: 'console-operator' });
  });

  it('reject without the typed confirmation sends nothing', async () => {
    let calls = 0;
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, status: 200, json: async () => ({ proposals: [proposal()] }) };
      }
      calls++;
      return { ok: true, status: 200, json: async () => ({ success: true }) };
    });
    const screen = await render(<MetasearchPanel />);
    await screen.getByRole('button', { name: 'Reject proposal prop-1' }).click();
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeVisible();
    const confirm = screen.getByRole('button', { name: 'Reject proposal' });
    // Wait out the consequential settle window, then prove the gate that
    // remains is the typed proposal short-id: confirm is still inert, no POST.
    await expect.poll(() => confirm.element().getAttribute('data-settle')).toBe('ready');
    expect((confirm.element() as HTMLButtonElement).disabled).toBe(true);
    expect(calls).toBe(0);
    await screen.getByRole('button', { name: 'Cancel' }).click();
    await expect.poll(() => document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(calls).toBe(0);
  });

  it('apply shows only on approved proposals and surfaces refusal reasons', async () => {
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ proposals: [proposal({ id: 'a1', status: 'approved' }), proposal({ id: 'p2', status: 'pending_approval' })] }),
        };
      }
      if (url.endsWith('/api/metasearch/apply')) {
        return { ok: false, status: 401, text: async () => 'missing or invalid bearer token' };
      }
      return { ok: true, status: 200, json: async () => ({ success: true }) };
    });
    const screen = await render(<MetasearchPanel />);
    await expect.element(screen.getByRole('button', { name: 'Apply proposal a1' })).toBeVisible();
    expect(buttonLabels(screen)).not.toContain('Apply proposal p2');
    await screen.getByRole('button', { name: 'Apply proposal a1' }).click();
    await expect.poll(() => screen.container.textContent).toContain('missing or invalid bearer token');
    await expect.element(screen.getByRole('status')).toHaveTextContent('refused · missing or invalid bearer token');
  });
});
