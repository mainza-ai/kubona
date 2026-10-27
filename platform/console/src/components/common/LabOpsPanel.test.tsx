import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LabOpsPanel } from './LabOpsPanel';
import { getOrCreateSessionKeyPair, peekSessionPubkeyHex } from '@/lib/mandates';
import { bytesToHex } from '@noble/hashes/utils.js';
import type { Lab } from '@/types';

afterEach(() => {
  vi.unstubAllGlobals();
});

function lab(partial: Partial<Lab> & { id: string }): Lab {
  return {
    display_name: 'lab',
    autonomy_level: 'L1',
    status: 'active',
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

const kp = getOrCreateSessionKeyPair();
const PUB = bytesToHex(kp.publicKey);
if (!peekSessionPubkeyHex()) throw new Error('test setup: keypair must exist');

describe('LabOpsPanel (U1-b)', () => {
  it('ceilings carry the golden params_hash end-to-end (UI → envelope → Go)', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, json: async () => ({ id: 'lab-a' }) };
    });
    vi.stubGlobal('confirm', () => true);
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => {}} />
    );
    const input = tree.root.findByProps({ 'aria-label': 'New ceiling for lab-a' });
    await act(async () => {
      input.props.onChange({ target: { value: '150' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Set ceiling')!.props.onClick();
    });
    expect(seen).toHaveLength(1);
    expect(seen[0].url).toBe('http://localhost:8080/api/tenants/ceiling');
    const body = JSON.parse(seen[0].body) as Record<string, unknown>;
    expect(body.tenant_id).toBe('lab-a');
    expect(body.ceiling_usd).toBe(150);
    const action = body.action as Record<string, string>;
    expect(action.action).toBe('set_ceiling');
    // Golden: the exact Go ParamsHashOf for {"ceiling_usd":150}.
    expect(action.params_hash).toBe('95169ffc05abd4ab865019f68df37924f29d682856c4c4e9b929b74069e0e891');
    expect(action.pubkey_hex).toBe(PUB);
  });

  it('mismatched keys fall back to the lab-admin session without an envelope', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, json: async () => ({ id: 'lab-a' }) };
    });
    vi.stubGlobal('confirm', () => true);
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: 'ab'.repeat(32) })} onChanged={() => {}} />
    );
    const text = JSON.stringify(tree.toJSON());
    expect(text).toContain('is not this lab');
    const input = tree.root.findByProps({ 'aria-label': 'New ceiling for lab-a' });
    await act(async () => {
      input.props.onChange({ target: { value: '150' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Set ceiling')!.props.onClick();
    });
    expect(seen).toHaveLength(1);
    const body = JSON.parse(seen[0].body) as Record<string, unknown>;
    expect(body.ceiling_usd).toBe(150);
    expect('action' in body).toBe(false);
  });

  it('bootstraps an admin-less lab with no envelope (first write wins)', async () => {
    const seen: string[] = [];
    stubFetch((url) => {
      seen.push(url);
      return { ok: true, json: async () => ({ id: 'lab-new' }) };
    });
    const tree = await render(<LabOpsPanel lab={lab({ id: 'lab-new' })} onChanged={() => {}} />);
    await act(async () => {
      tree.root.findByType('button').props.onClick();
    });
    // run() is async — flush.
    await act(async () => {});
    expect(seen).toEqual(['http://localhost:8080/api/tenants/admin-key']);
  });

  it('refund without a reason refuses client-side (no network)', async () => {
    let calls = 0;
    stubFetch(() => {
      calls++;
      return { ok: true, json: async () => ({}) };
    });
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => {}} />
    );
    const amount = tree.root.findByProps({ 'aria-label': 'Refund amount for lab-a' });
    await act(async () => {
      amount.props.onChange({ target: { value: '10' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Refund')!.props.onClick();
    });
    expect(calls).toBe(0);
    expect(JSON.stringify(tree.toJSON())).toContain('needs a reason');
  });

  it('suspend without confirm sends nothing', async () => {
    let calls = 0;
    stubFetch(() => {
      calls++;
      return { ok: true, json: async () => ({}) };
    });
    vi.stubGlobal('confirm', () => false);
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => {}} />
    );
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Suspend lab lab-a' }).props.onClick();
    });
    expect(calls).toBe(0);
  });
});
