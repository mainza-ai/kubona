import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { MemoryPanel } from './MemoryPanel';

afterEach(() => vi.unstubAllGlobals());

// B1-2: the panel renders nothing without a session, lists inventory with
// null-similarity labeling, and posts retains with explicit weight.
describe('MemoryPanel in a real browser', () => {
  it('renders nothing without a session', async () => {
    const screen = await render(<MemoryPanel sessionId={null} />);
    expect(screen.container.textContent).toBe('');
    expect(screen.container.querySelector('button')).toBeNull();
  });

  it('recalls tenant-scoped inventory and retains with an explicit weight', async () => {
    const calls: Array<{ url: string; method: string; body?: unknown }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? 'GET', body: init?.body });
      if (String(url).includes('/api/memory/recall')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            snippets: [
              {
                content: 'prior anchor note',
                bank: 'experiences',
                similarity: null,
                created_at: '2026-10-11T00:00:00Z',
              },
            ],
            tenant: 'default',
          }),
        };
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ id: 'mem-1', bank: 'world-facts', importance: 0.8 }),
      };
    });

    const screen = await render(<MemoryPanel sessionId="sess-1" />);
    await screen.getByRole('button', { name: /SESSION MEMORY/ }).click();

    // Recall with an empty query (inventory mode).
    await screen.getByRole('button', { name: 'Recall memory' }).click();
    await expect.poll(() => screen.container.textContent).toContain('prior anchor note');
    expect(screen.container.textContent).toContain('inventory');
    expect(screen.container.textContent).toContain('experiences · inventory');

    const recallCall = calls.find((call) => call.url.includes('/api/memory/recall'));
    expect(recallCall?.url).toContain('session_id=sess-1');
    expect(recallCall?.url).toContain('bank=experiences');

    // Retain posts content + bank + explicit weight and shows the receipt.
    await screen.getByLabelText('Operator note to retain').fill('operator note');
    await screen.getByLabelText('Note importance weight').selectOptions('0.8');
    await screen.getByRole('button', { name: 'Retain operator note' }).click();

    await expect.poll(() => calls.find((call) => call.url.includes('/api/memory/retain'))).toBeTruthy();
    const retainCall = calls.find((call) => call.url.includes('/api/memory/retain'))!;
    expect(retainCall.method).toBe('POST');
    const sentBody = JSON.parse(String(retainCall.body)) as {
      session_id: string;
      content: string;
      bank: string;
      importance: number;
    };
    expect(sentBody.session_id).toBe('sess-1');
    expect(sentBody.content).toBe('operator note');
    expect(sentBody.bank).toBe('experiences');
    expect(sentBody.importance).toBe(0.8);

    // The recall refresh clears the transient status line, so the durable
    // evidence is the receipt row plus the restored draft affordance.
    await expect.poll(() => screen.container.textContent).toContain('Remembered · world-facts · mem-1');
    expect(screen.container.textContent).toContain('operator note');
    await expect.element(screen.getByRole('button', { name: 'Restore draft' })).toBeVisible();
  });

  it('reports an empty bank instead of an empty list', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      status: 200,
      json: async () => ({ snippets: [], tenant: 'default' }),
    }));

    const screen = await render(<MemoryPanel sessionId="sess-1" />);
    await screen.getByRole('button', { name: /SESSION MEMORY/ }).click();
    await screen.getByRole('button', { name: 'Recall memory' }).click();
    await expect.poll(() => screen.container.textContent).toContain('Nothing retained yet in this bank.');
  });
});
