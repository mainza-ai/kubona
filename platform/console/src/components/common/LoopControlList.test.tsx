import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
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

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

async function render(el: React.ReactElement): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(el);
  });
  return tree!;
}

describe('LoopControlList (U1-c)', () => {
  it('pauses a running loop and refetches on acceptance', async () => {
    const seen: string[] = [];
    let changed = 0;
    stubFetch((url, init) => {
      seen.push(`${init?.method} ${url}`);
      return { ok: true, json: async () => ({ loop_id: 'l1', action: 'pause', status: 'ok' }) };
    });
    const tree = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => { changed++; }} />);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Pause loop l1' }).props.onClick();
    });
    expect(seen).toEqual(['POST http://localhost:8080/api/loops/l1/pause']);
    expect(changed).toBe(1);
  });

  it('shows resume only on paused loops and kill only on live loops', async () => {
    stubFetch(() => ({ ok: true, json: async () => ({}) }));
    const tree = await render(
      <LoopControlList
        loops={[loop({ id: 'run' }), loop({ id: 'pau', status: 'paused' }), loop({ id: 'dead', status: 'killed' })]}
        onChanged={() => {}}
      />
    );
    const labels = tree.root.findAllByType('button').map((b) => b.props['aria-label'] as string);
    expect(labels).toContain('Pause loop run');
    expect(labels).toContain('Resume loop pau');
    expect(labels).toContain('Kill loop run');
    expect(labels).toContain('Kill loop pau');
    expect(labels).not.toContain('Pause loop pau');
    expect(labels).not.toContain('Resume loop run');
    expect(labels).not.toContain('Kill loop dead');
  });

  it('kill without confirm sends nothing', async () => {
    let calls = 0;
    stubFetch(() => {
      calls++;
      return { ok: true, json: async () => ({}) };
    });
    vi.stubGlobal('confirm', () => false);
    const tree = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => {}} />);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Kill loop l1' }).props.onClick();
    });
    expect(calls).toBe(0);
  });

  it('surfaces the backend refusal reason verbatim', async () => {
    stubFetch(() => ({ ok: false, status: 500, text: async () => 'loop l1 not found' }));
    const tree = await render(<LoopControlList loops={[loop({ id: 'l1' })]} onChanged={() => {}} />);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Pause loop l1' }).props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('loop l1 not found');
  });

  it('renders an honest "no live loops" line when there are no loops', async () => {
    stubFetch(() => {
      throw new Error('must not fetch');
    });
    const tree = await render(<LoopControlList loops={[]} onChanged={() => {}} />);
    expect(tree.toJSON()).not.toBeNull();
    expect(JSON.stringify(tree.toJSON())).toContain('no live loops');
  });
});
