import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { AutoresearchPanel } from './AutoresearchPanel';

afterEach(() => { vi.unstubAllGlobals(); });

function loop() {
  return {
    id: 'loop-1',
    harness_id: 'h1',
    status: 'running',
    current_iteration: 0,
    max_iterations: 10,
    champion_metric: null,
    champion_depth: null,
    champion_2q_gates: null,
    stagnation_reason: null,
    created_at: '',
    updated_at: '',
  };
}

function stub(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

async function render() {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => { tree = TestRenderer.create(<AutoresearchPanel sessionId="s1" />); });
  return tree!;
}

describe('AutoresearchPanel settlement', () => {
  it('shows applied after a confirmed start', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, json: async () => ({ harnesses: [{ id: 'h1' }] }) };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, json: async () => ({ loops: [loop()] }) };
      if (url.endsWith('/api/autoresearch/start')) return { ok: true, json: async () => loop() };
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Max iterations' }).props.onChange({ target: { value: '10' } });
    });
    await act(async () => {
      await tree.root.findAllByType('button').find((button) => button.props.children === 'Start loop')!.props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('applied · start loop');
  });

  it('does not render an unconfirmed transition as success', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, json: async () => ({ harnesses: [{ id: 'h1' }] }) };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, json: async () => ({ loops: [] }) };
      if (url.endsWith('/api/autoresearch/start')) return { ok: false, status: 500, json: async () => ({ error: 'no runner' }) };
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    await act(async () => {
      await tree.root.findAllByType('button').find((button) => button.props.children === 'Start loop')!.props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('unknown · start not confirmed');
  });
});
