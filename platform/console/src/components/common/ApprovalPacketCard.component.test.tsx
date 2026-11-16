import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
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

type Props = React.ComponentProps<typeof ApprovalPacketCard>;

function renderCard(props: Partial<Props> = {}) {
  return render(
    <ApprovalPacketCard
      packet={PACKET}
      onApproveOnce={() => {}}
      onApproveUpToMax={() => {}}
      onDeny={() => {}}
      {...props}
    />,
  );
}

describe('ApprovalPacketCard in a real browser (Q-11 burden-scaled packet)', () => {
  it('renders every competent-approval field', async () => {
    const screen = await renderCard();
    const text = screen.container.textContent ?? '';

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

  it('approve-once and approve-up-to-max are distinct actions (up-to-max restates $X.XX)', async () => {
    const once = vi.fn();
    const upToMax = vi.fn();
    const screen = await renderCard({ onApproveOnce: once, onApproveUpToMax: upToMax });

    // Wave-2 money copy: the button restates the exact bound at 2dp.
    const upToMaxBtn = screen.getByRole('button', { name: 'Approve up to $48.00' });
    expect(screen.container.textContent).toContain('Approve up to $48.00');

    await screen.getByRole('button', { name: 'Approve once' }).click();
    expect(once).toHaveBeenCalledTimes(1);

    await upToMaxBtn.click();
    expect(upToMax).toHaveBeenCalledTimes(1);
  });

  it('denial requires a reason and returns it', async () => {
    const onDeny = vi.fn();
    const screen = await renderCard({ onDeny });

    await screen.getByRole('button', { name: 'Deny' }).click();
    const confirm = screen.getByRole('button', { name: 'Confirm denial' });
    expect((confirm.element() as HTMLButtonElement).disabled).toBe(true);
    // A disabled confirm cannot dispatch.
    (confirm.element() as HTMLButtonElement).click();
    expect(onDeny).not.toHaveBeenCalled();

    await screen.getByRole('textbox').fill('too pricey');
    await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(false);
    await confirm.click();
    expect(onDeny).toHaveBeenCalledWith('too pricey');
  });

  it('surfaces decision errors without dispatching', async () => {
    const once = vi.fn();
    const screen = await renderCard({ onApproveOnce: once, decisionError: 'mandate REJECTED: expired' });

    expect(screen.container.textContent).toContain('mandate REJECTED');
    expect(once).not.toHaveBeenCalled();
  });

  it('deny-reason and budget-breach behavior are unchanged', async () => {
    // Breach disables BOTH approve actions but leaves Deny enabled.
    const screen = await renderCard({ packet: { ...PACKET, budgetBreached: true } });

    expect(screen.container.textContent).toContain('OVERSPENT');
    const approveOnce = screen.getByRole('button', { name: 'Approve once' }).element() as HTMLButtonElement;
    const upToMax = screen.getByRole('button', { name: 'Approve up to $48.00' }).element() as HTMLButtonElement;
    expect(approveOnce.disabled).toBe(true);
    expect(upToMax.disabled).toBe(true);
    const deny = screen.getByRole('button', { name: 'Deny' }).element() as HTMLButtonElement;
    expect(deny.disabled).not.toBe(true);
  });

  it('unpriced packet falls back to the generic up-to-max label', async () => {
    const screen = await renderCard({ packet: { ...PACKET, costMaxUsd: NaN } });
    const text = screen.container.textContent ?? '';

    expect(text).toContain('Approve up to max');
    expect(text).not.toContain('Approve up to $');
  });
});
