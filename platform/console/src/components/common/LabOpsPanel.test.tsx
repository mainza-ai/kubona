import { describe, it, expect, vi, afterEach, beforeAll } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LabOpsPanel } from './LabOpsPanel';
import ConfirmDialog from './ConfirmDialog';
import { getOrCreateSigner } from '@/lib/custody';
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

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
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

let PUB = '';
beforeAll(async () => {
  PUB = (await getOrCreateSigner()).publicKeyHex;
});

describe('LabOpsPanel (U1-b)', () => {
  it('ceilings carry the golden params_hash end-to-end (UI → envelope → Go)', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, json: async () => ({ id: 'lab-a' }) };
    });
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => {}} />
    );
    await vi.waitFor(() => expect(textOf(tree.root)).toContain('this browser key'));
    const input = tree.root.findByProps({ 'aria-label': 'New ceiling for lab-a' });
    await act(async () => {
      input.props.onChange({ target: { value: '150' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Set ceiling')!.props.onClick();
    });
    // Wave-3: ceiling stages a ConfirmDialog (reversible, no typed text).
    await act(async () => {
      await tree.root.findByType(ConfirmDialog).props.onConfirm();
    });
    await vi.waitFor(() => expect(seen).toHaveLength(1));
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
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: 'ab'.repeat(32) })} onChanged={() => {}} />
    );
    await vi.waitFor(() => expect(textOf(tree.root)).toContain('is not this lab'));
    const input = tree.root.findByProps({ 'aria-label': 'New ceiling for lab-a' });
    await act(async () => {
      input.props.onChange({ target: { value: '150' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Set ceiling')!.props.onClick();
    });
    await act(async () => {
      await tree.root.findByType(ConfirmDialog).props.onConfirm();
    });
    await vi.waitFor(() => expect(seen).toHaveLength(1));
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
    await vi.waitFor(() => expect(textOf(tree.root)).toContain('No admin registered'));
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

describe('LabOpsPanel refund typed gate (Wave-2)', () => {
  async function openRefundGate(
    seen: Array<{ url: string; body: string }>,
    opts: { amount?: string; reason?: string } = {},
  ): Promise<TestRenderer.ReactTestRenderer> {
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => {}} />
    );
    await vi.waitFor(() => expect(textOf(tree.root)).toContain('this browser key'));
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Refund amount for lab-a' }).props.onChange({
        target: { value: opts.amount ?? '10' },
      });
    });
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Refund reason for lab-a' }).props.onChange({
        target: { value: opts.reason ?? 'overcharge correction' },
      });
    });
    expect(seen).toHaveLength(0);
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Refund')!.props.onClick();
    });
    return tree;
  }

  function gateOf(tree: TestRenderer.ReactTestRenderer) {
    return tree.root.findByType(ConfirmDialog);
  }

  function confirmBtn(gate: TestRenderer.ReactTestInstance) {
    const btns = gate.findAllByType('button');
    return btns.find((b) => typeof b.props.children === 'string' && b.props.children.startsWith('Refund $'))!;
  }

  it('requires the exact "$X.XX": wrong text keeps confirm disabled, exact text enables + fires', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, json: async () => ({ id: 'lab-a' }) };
    });
    let changed = 0;
    const tree = await render(
      <LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: PUB })} onChanged={() => { changed++; }} />
    );
    await vi.waitFor(() => expect(textOf(tree.root)).toContain('this browser key'));
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Refund amount for lab-a' }).props.onChange({ target: { value: '10' } });
    });
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Refund reason for lab-a' }).props.onChange({ target: { value: 'overcharge correction' } });
    });
    await act(async () => {
      tree.root.findAllByType('button').find((b) => b.props.children === 'Refund')!.props.onClick();
    });
    // No native confirm, no network until the typed acknowledgement.
    expect(seen).toHaveLength(0);
    const gate = gateOf(tree);
    expect(gate.props.requireText).toBe('$10.00');
    expect(gate.props.confirmLabel).toBe('Refund $10.00');
    expect(textOf(tree.root)).toContain('Audited, irreversible');
    const input = gate.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: '10.00' } });
    });
    expect(confirmBtn(gateOf(tree)).props.disabled).toBe(true);
    await act(async () => {
      gateOf(tree).findByType('input').props.onChange({ target: { value: '$10.00' } });
    });
    expect(confirmBtn(gateOf(tree)).props.disabled).not.toBe(true);
    await act(async () => {
      gateOf(tree).props.onConfirm();
    });
    await vi.waitFor(() => expect(seen).toHaveLength(1));
    expect(seen[0].url).toBe('http://localhost:8080/api/tenants/refund');
    const body = JSON.parse(seen[0].body) as Record<string, unknown>;
    expect(body.amount_usd).toBe(10);
    await vi.waitFor(() => expect(changed).toBe(1));
    // Success closes the gate.
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(0);
  });

  it('busy disables the gate; backend refusal surfaces in the dialog error slot with no silent close', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: false, status: 403, text: async () => 'not-admin' };
    });
    const tree = await openRefundGate(seen);
    const gate = gateOf(tree);
    await act(async () => {
      gate.findByType('input').props.onChange({ target: { value: '$10.00' } });
    });
    await act(async () => {
      gateOf(tree).props.onConfirm();
    });
    // Refused: the gate stays open with the backend reason verbatim.
    await vi.waitFor(() => expect(String(gateOf(tree).props.error)).toContain('not-admin'));
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(1);
    expect(seen).toHaveLength(1);
  });

  it('backdrop/cancel dismisses the gate with no network', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, json: async () => ({ id: 'lab-a' }) };
    });
    const tree = await openRefundGate(seen);
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(1);
    // Backdrop click funnels to onCancel.
    await act(async () => {
      gateOf(tree).findByProps({ role: 'presentation' }).props.onClick();
    });
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(0);
    expect(seen).toHaveLength(0);
    // Escape funnels to the same onCancel (trapped dialog contract).
    await openRefundGate(seen).then(async (tree2) => {
      expect(tree2.root.findAllByType(ConfirmDialog)).toHaveLength(1);
      await act(async () => {
        tree2.root.findByType(ConfirmDialog).props.onCancel();
      });
      expect(tree2.root.findAllByType(ConfirmDialog)).toHaveLength(0);
      expect(seen).toHaveLength(0);
    });
  });
});
