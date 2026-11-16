import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LoopControlList } from './LoopControlList';
import type { Loop } from '@/types';

afterEach(() => {
  vi.unstubAllGlobals();
});

function loop(partial: Partial<Loop> & { id: string }): Loop {
  return {
    session_id: 's1',
    status: 'running',
    step_count: 3,
    max_steps: 50,
    diverged: false,
    created_at: '',
    updated_at: '',
    ...partial,
  };
}

/** Record every request the component makes, so a no-op can be proven. */
function stubFetch(handler: (url: string, init?: RequestInit) => unknown): string[] {
  const seen: string[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    seen.push(`${init?.method ?? 'GET'} ${String(url)}`);
    return handler(String(url), init);
  });
  return seen;
}

function buttonLabels(screen: Awaited<ReturnType<typeof render>>): Array<string | null> {
  return Array.from(screen.container.querySelectorAll('button')).map((button) => button.getAttribute('aria-label'));
}

describe('LoopControlList in a real browser (U1-c)', () => {
  it('pauses a running loop and refetches on acceptance', async () => {
    const seen = stubFetch(() => ({ ok: true, status: 200, json: async () => ({ loop_id: 'l1', action: 'pause', status: 'ok' }) }));
    let changed = 0;
    const screen = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => { changed++; }} />);
    await screen.getByRole('button', { name: 'Pause loop l1' }).click();
    await expect.poll(() => changed).toBe(1);
    expect(seen).toEqual(['POST http://localhost:8080/api/loops/l1/pause']);
  });

  it('shows resume only on paused loops and kill only on live loops', async () => {
    stubFetch(() => ({ ok: true, status: 200, json: async () => ({}) }));
    const screen = await render(
      <LoopControlList
        loops={[loop({ id: 'run' }), loop({ id: 'pau', status: 'paused' }), loop({ id: 'dead', status: 'killed' })]}
        onChanged={() => {}}
      />,
    );
    const labels = buttonLabels(screen);
    expect(labels).toContain('Pause loop run');
    expect(labels).toContain('Resume loop pau');
    expect(labels).toContain('Kill loop run');
    expect(labels).toContain('Kill loop pau');
    expect(labels).not.toContain('Pause loop pau');
    expect(labels).not.toContain('Resume loop run');
    expect(labels).not.toContain('Kill loop dead');
  });

  it('kill without the typed confirmation sends nothing', async () => {
    const seen = stubFetch(() => ({ ok: true, status: 200, json: async () => ({}) }));
    const screen = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => {}} />);
    await screen.getByRole('button', { name: 'Kill loop l1' }).click();
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeVisible();
    const confirm = screen.getByRole('button', { name: 'Kill loop' });
    // Wait out the consequential settle window, then prove the gate that
    // remains is the typed loop short-id: confirm is still inert, no request.
    await expect.poll(() => confirm.element().getAttribute('data-settle')).toBe('ready');
    expect((confirm.element() as HTMLButtonElement).disabled).toBe(true);
    expect(seen).toEqual([]);
    await screen.getByRole('button', { name: 'Cancel' }).click();
    await expect.poll(() => document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(seen).toEqual([]);
  });

  it('surfaces the backend refusal reason verbatim', async () => {
    stubFetch(() => ({ ok: false, status: 500, text: async () => 'loop l1 not found' }));
    const screen = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => {}} />);
    await screen.getByRole('button', { name: 'Pause loop l1' }).click();
    await expect.poll(() => screen.container.textContent).toContain('loop l1 not found');
    await expect.element(screen.getByRole('alert')).toHaveTextContent('loop pause refused: loop l1 not found');
  });

  it('renders an honest "no live loops" line when there are no loops', async () => {
    const seen = stubFetch(() => {
      throw new Error('must not fetch');
    });
    const screen = await render(<LoopControlList loops={[]} onChanged={() => {}} />);
    await expect.element(screen.getByText('no live loops')).toBeVisible();
    expect(screen.container.querySelectorAll('button')).toHaveLength(0);
    expect(seen).toEqual([]);
  });
});
