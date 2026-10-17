import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
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
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

async function render(): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<MetasearchPanel />);
  });
  return tree!;
}

describe('MetasearchPanel (U1-d)', () => {
  it('lists proposals with approve/reject on pending ones', async () => {
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, json: async () => ({ proposals: [proposal()] }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    const labels = tree.root.findAllByType('button').map((b) => b.props['aria-label'] as string | undefined);
    expect(labels).toContain('Approve proposal prop-1');
    expect(labels).toContain('Reject proposal prop-1');
    expect(JSON.stringify(tree.toJSON())).toContain('faster convergence');
  });

  it('approve posts proposal_id and refetches the list', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    let approved = false;
    stubFetch((url, init) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, json: async () => ({ proposals: approved ? [] : [proposal()] }) };
      }
      seen.push({ url, body: String(init?.body) });
      approved = true;
      return { ok: true, json: async () => ({ success: true }) };
    });
    const tree = await render();
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Approve proposal prop-1' }).props.onClick();
    });
    expect(seen.map((s) => s.url)).toEqual(['http://localhost:8080/api/metasearch/approve']);
    expect(JSON.parse(seen[0].body)).toEqual({ proposal_id: 'prop-1', signer: 'console-operator' });
    expect(JSON.stringify(tree.toJSON())).toContain('No proposals pending');
  });

  it('reject without confirm sends nothing', async () => {
    let calls = 0;
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, json: async () => ({ proposals: [proposal()] }) };
      }
      calls++;
      return { ok: true, json: async () => ({ success: true }) };
    });
    vi.stubGlobal('confirm', () => false);
    const tree = await render();
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Reject proposal prop-1' }).props.onClick();
    });
    expect(calls).toBe(0);
  });

  it('apply shows only on approved proposals and surfaces refusal reasons', async () => {
    stubFetch((url) => {
      if (url.endsWith('/api/metasearch/proposals')) {
        return { ok: true, json: async () => ({ proposals: [proposal({ id: 'a1', status: 'approved' }), proposal({ id: 'p2', status: 'pending_approval' })] }) };
      }
      if (url.endsWith('/api/metasearch/apply')) {
        return { ok: false, status: 401, text: async () => 'missing or invalid bearer token' };
      }
      return { ok: true, json: async () => ({ success: true }) };
    });
    const tree = await render();
    const labels = tree.root.findAllByType('button').map((b) => b.props['aria-label'] as string | undefined);
    expect(labels).toContain('Apply proposal a1');
    expect(labels).not.toContain('Apply proposal p2');
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Apply proposal a1' }).props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('missing or invalid bearer token');
  });
});
