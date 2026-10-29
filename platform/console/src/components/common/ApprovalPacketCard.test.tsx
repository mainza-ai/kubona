import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act, type ReactTestInstance } from 'react-test-renderer';
import { ApprovalPacketCard, type ApprovalPacketView } from './ApprovalPacketCard';

const PACKET: ApprovalPacketView = {
  packetId: 'appr-test',
  agentId: 'ansatz-scout',
  backend: 'ibm_brisbane',
  programHash: 'prog:abc123',
  qubits: 20,
  shots: 4000,
  costMaxUsd: 48,
  budgetRemainingUsd: 212,
  costBasis: 'price sheet v2026-10-01',
  blastRadius: 'spend only; no data egress',
  reversibility: 'irreversible spend once dequeued',
  evidence: { dryRun: 'sim 1000 shots Hellinger 0.03', classicalAnchor: 'MPS n=40' },
  alternatives: { simFirstSavings: '$48.00' },
  expiresAtIso: '2026-10-09T15:00:00Z',
  noncePreview: 'a1b2c3d4',
};

function textOf(node: ReactTestInstance | string | number): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const children = (node as ReactTestInstance).children ?? [];
  return children.map((c) => textOf(c as string | number | ReactTestInstance)).join(' ');
}

function buttonByText(root: ReactTestInstance, t: string) {
  const found = root
    .findAllByType('button')
    .find((b) => textOf(b).includes(t));
  if (!found) throw new Error(`button not found: ${t}`);
  return found;
}

function renderCard(props: Partial<React.ComponentProps<typeof ApprovalPacketCard>> = {}) {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(
      <ApprovalPacketCard
        packet={PACKET}
        onApproveOnce={() => {}}
        onApproveUpToMax={() => {}}
        onDeny={() => {}}
        {...props}
      />
    );
  });
  return tree!;
}

describe('ApprovalPacketCard (Q-11 burden-scaled packet)', () => {
  it('renders every competent-approval field', () => {
    const tree = renderCard();
    const text = textOf(tree.root);
    for (const needle of [
      'ibm_brisbane',
      '48.00',
      '212.00',
      'no data egress',
      'irreversible spend once dequeued',
      'Hellinger 0.03',
      '2026-10-09T15:00:00Z',
      'prog:abc123',
    ]) {
      expect(text, needle).toContain(needle);
    }
  });

  it('approve-once and approve-up-to-max are distinct actions (up-to-max restates $X.XX)', () => {
    const once = vi.fn();
    const upToMax = vi.fn();
    const tree = renderCard({ onApproveOnce: once, onApproveUpToMax: upToMax });
    // Wave-2 money copy: the button restates the exact bound at 2dp.
    const upToMaxBtn = buttonByText(tree.root, 'Approve up to $48.00');
    expect(textOf(tree.root)).toContain('Approve up to $48.00');
    act(() => {
      buttonByText(tree.root, 'Approve once').props.onClick();
    });
    expect(once).toHaveBeenCalledTimes(1);
    act(() => {
      upToMaxBtn.props.onClick();
    });
    expect(upToMax).toHaveBeenCalledTimes(1);
  });

  it('denial requires a reason and returns it', () => {
    const onDeny = vi.fn();
    const tree = renderCard({ onDeny });
    act(() => {
      buttonByText(tree.root, 'Deny').props.onClick();
    });
    const confirm = buttonByText(tree.root, 'Confirm denial');
    act(() => {
      confirm.props.onClick();
    });
    expect(onDeny).not.toHaveBeenCalled();
    const input = tree.root.findByType('input');
    act(() => {
      input.props.onChange({ target: { value: 'too pricey' } });
    });
    act(() => {
      buttonByText(tree.root, 'Confirm denial').props.onClick();
    });
    expect(onDeny).toHaveBeenCalledWith('too pricey');
  });

  it('surfaces decision errors without dispatching', () => {
    const once = vi.fn();
    const tree = renderCard({ onApproveOnce: once, decisionError: 'mandate REJECTED: expired' });
    expect(textOf(tree.root)).toContain('mandate REJECTED');
  });

  it('deny-reason and budget-breach behavior are unchanged', () => {
    // Breach disables BOTH approve actions but leaves Deny enabled.
    const tree = renderCard({ packet: { ...PACKET, budgetBreached: true } });
    expect(textOf(tree.root)).toContain('OVERSPENT');
    const approveOnce = buttonByText(tree.root, 'Approve once');
    const upToMax = buttonByText(tree.root, 'Approve up to $48.00');
    expect(approveOnce.props.disabled).toBe(true);
    expect(upToMax.props.disabled).toBe(true);
    const deny = buttonByText(tree.root, 'Deny');
    expect(deny.props.disabled).not.toBe(true);
  });

  it('unpriced packet falls back to the generic up-to-max label', () => {
    const tree = renderCard({
      packet: { ...PACKET, costMaxUsd: NaN },
    });
    expect(textOf(tree.root)).toContain('Approve up to max');
    expect(textOf(tree.root)).not.toContain('Approve up to $');
  });
});
