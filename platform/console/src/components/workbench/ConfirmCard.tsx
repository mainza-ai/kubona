'use client';

import React, { useMemo, useState } from 'react';
import { ArrowDownToLine, CheckCircle2, Fingerprint, ShieldAlert } from 'lucide-react';
import type { CampaignState, Session } from '@/types';
import { authorizeSpend } from '@/lib/authorizeSpend';
import { launchProposal, mintProposal, type LaunchProposalResult, type RunAllowanceView } from '@/lib/api';
import { RunAllowancePanel } from '@/components/common/RunAllowancePanel';
import { canonicalJobBinding } from '@/lib/mandates';
import { buildApprovalPacketFromCampaign } from '@/lib/approvalPacket';
import { launchJobId, reconcileOperationFull, UNCONFIRMED_LAUNCH } from '@/domain/write-settlement';

export type ConfirmGateConfig = {
  campaign: CampaignState;
  session: Session | null;
  onAuthorized: () => void;
  onDenied: (reason: string) => void;
  onDowngrade: () => void;
};

export type AskReceipt = {
  outcome: 'authorized' | 'downgraded' | 'denied' | 'closed' | 'working' | 'unknown';
  label: string;
  detail: string;
  at: string;
};

export type MeteredAuthorizationOutcome =
  | { ok: true; jobId: string; backend: string }
  | { ok: false; message: string; unknown?: boolean };

async function settleLostLaunch(
  sessionId: string,
  launched: Extract<LaunchProposalResult, { outcome: 'unknown' }>,
  onWorking?: () => void,
): Promise<MeteredAuthorizationOutcome> {
  if (!launched.requestId) {
    return { ok: false, unknown: true, message: UNCONFIRMED_LAUNCH };
  }
  onWorking?.();
  const settled = await reconcileOperationFull(launched.requestId, sessionId);
  if (settled.phase === 'refused') {
    return { ok: false, message: `Mandate verified, but dispatch was refused: ${settled.detail}` };
  }
  if (settled.phase === 'applied') {
    const jobId = launchJobId(settled.receipt);
    if (jobId) return { ok: true, jobId, backend: '' };
  }
  return { ok: false, unknown: true, message: UNCONFIRMED_LAUNCH };
}

/**
 * Shared metered hardware authorization + dispatch flow:
 * 1. Signs & verifies AP2 Ed25519 SpendMandate over (session + exact QASM).
 * 2. Launches proposal only after mandate verification succeeds.
 */
export async function executeMeteredAuthorization(
  campaign: CampaignState | null | undefined,
  session: Session | null | undefined,
  onWorking?: () => void,
  allowanceId?: string,
): Promise<MeteredAuthorizationOutcome> {
  const estimatedCost = campaign?.quantum_factors?.estimated_spend_usd;
  const costKnown = typeof estimatedCost === 'number' && Number.isFinite(estimatedCost);
  const sessionBound = (session?.id ?? '') !== '';
  const programQasm = campaign?.quantum_factors?.qasm ?? '';
  const programKnown = programQasm !== '';
  const recommendedBackend = campaign?.quantum_factors?.recommended_backend || 'Local simulator';

  if (!sessionBound) return { ok: false, message: 'No session bound — authorization disabled.' };
  if (!programKnown) {
    return { ok: false, message: 'No program in this campaign — authorization disabled until a circuit is proposed.' };
  }
  if (!costKnown) {
    return { ok: false, message: 'No cost estimate on this campaign — authorization disabled until priced.' };
  }

  try {
    // C11: a run allowance replaces the per-job mandate. Admission still
    // enforces every scope/budget/taint rule; the client only names it.
    let mandateRef: string | undefined;
    if (!allowanceId) {
      const res = await authorizeSpend({ session, campaign });
      if (!res.ok) {
        return { ok: false, message: `${res.reason} Halted fail-closed — nothing dispatched, nothing spent.` };
      }
      mandateRef = JSON.stringify(res.mandate);
    }
    if (!session?.id) {
      return { ok: false, message: 'No session bound — authorization was verified but nothing could be dispatched.' };
    }
    const requestId = typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID()
      : `launch-${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
    // B1: the server prices and owns the plan. If it cannot be minted honestly
    // (unknown backend, unpriced metered hardware, no stored circuit), nothing
    // is dispatched — the refusal is shown, never papered over with a default
    // budget or a client-built proposal.
    const minted = await mintProposal(session.id, 'campaign');
    if (!minted.ok) {
      return { ok: false, message: `No launchable plan: ${minted.error}` };
    }
    const backend = minted.proposal.body.backend || recommendedBackend;
    const launched = await launchProposal(
      session.id,
      { proposalId: minted.proposal.id, digest: minted.proposal.digest },
      {
        ...(mandateRef ? { mandateRef } : {}),
        ...(allowanceId ? { runAllowanceId: allowanceId } : {}),
        requestId,
      },
    );
    if ('outcome' in launched) {
      const settled = await settleLostLaunch(session.id, launched, onWorking);
      if (settled.ok) return { ...settled, backend };
      return settled;
    }
    if ('error' in launched) {
      return { ok: false, message: `Mandate verified, but dispatch was refused: ${launched.error}` };
    }
    return { ok: true, jobId: String(launched.job.id), backend };
  } catch (err) {
    console.error('Failed to authorize pulse:', err);
    return { ok: false, message: 'Failed to sign mandate envelope. Halted fail-closed.' };
  }
}

export function AskReceiptBanner({ receipt }: { receipt: AskReceipt }) {
  return (
    <div
      className="k-ask-receipt"
      data-testid="ask-receipt"
      data-outcome={receipt.outcome}
    >
      {receipt.outcome === 'unknown' || receipt.outcome === 'working'
        ? <ShieldAlert size={14} />
        : <CheckCircle2 size={14} />}
      <div>
        <strong>{receipt.label}</strong>
        <span>{receipt.detail}</span>
      </div>
    </div>
  );
}

export function ConfirmCard({
  gate,
  receipt,
  onSettled,
}: {
  gate?: ConfirmGateConfig | null;
  receipt?: AskReceipt | null;
  onSettled?: (receipt: AskReceipt) => void;
}) {
  const campaign = gate?.campaign;
  const session = gate?.session ?? null;
  const estimatedCost = campaign?.quantum_factors?.estimated_spend_usd;
  const costKnown = typeof estimatedCost === 'number' && Number.isFinite(estimatedCost);
  const sessionBound = (session?.id ?? '') !== '';
  const programQasm = campaign?.quantum_factors?.qasm ?? '';
  const programKnown = programQasm !== '';
  const canAuthorize = Boolean(gate) && costKnown && sessionBound && programKnown;
  const packet = useMemo(() => buildApprovalPacketFromCampaign(
    campaign,
    session?.id || '',
    estimatedCost ?? 0,
    session?.spend_limit_usd ?? 25.0,
    session?.current_spend_usd ?? 0,
  ), [campaign, session?.id, session?.spend_limit_usd, session?.current_spend_usd, estimatedCost]);

  const qasmDigest = useMemo(() => {
    if (!sessionBound || !programKnown || !session) return packet.programHash;
    try {
      return canonicalJobBinding(session.id, programQasm);
    } catch {
      return packet.programHash;
    }
  }, [sessionBound, programKnown, session, programQasm, packet.programHash]);

  const [busy, setBusy] = useState(false);
  const [allowance, setAllowance] = useState<RunAllowanceView | null>(null);
  const [unresolved, setUnresolved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showDeny, setShowDeny] = useState(false);
  const [denyReason, setDenyReason] = useState('');

  if (receipt) {
    return <AskReceiptBanner receipt={receipt} />;
  }
  if (!gate || !campaign) {
    return null;
  }

  const { onAuthorized, onDenied, onDowngrade } = gate;

  const handleApprove = async () => {
    setBusy(true);
    setError(null);
    try {
      const outcome = await executeMeteredAuthorization(campaign, session, () => {
        onSettled?.({
          outcome: 'working',
          label: 'Checking the launch receipt',
          detail: 'Retry stays disabled until this request is reconciled.',
          at: new Date().toISOString(),
        });
      }, allowance?.id);
      if (!outcome.ok) {
        if (outcome.unknown) {
          setUnresolved(true);
          setError(outcome.message);
          onSettled?.({
            outcome: 'unknown',
            label: 'Launch outcome unknown',
            detail: outcome.message,
            at: new Date().toISOString(),
          });
          return;
        }
        setError(outcome.message);
        return;
      }
      onSettled?.({
        outcome: 'authorized',
        label: `Authorized on ${outcome.backend}`,
        detail: `Job ${outcome.jobId.slice(0, 8)} · ${costKnown ? `$${(estimatedCost as number).toFixed(2)} max` : 'bound'}`,
        at: new Date().toISOString(),
      });
      onAuthorized();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="k-confirm-card"
      data-testid="confirm-card"
      data-tone="needs-you"
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
    >
      <div className="k-confirm-head">
        <span>Needs you · Metered hardware gate</span>
        <strong>
          {costKnown ? `≤ $${(estimatedCost as number).toFixed(2)}` : 'Unpriced'}
        </strong>
      </div>
      <dl className="k-confirm-meta">
        <div>
          <dt>Backend</dt>
          <dd>{packet.backend} · {packet.qubits}q</dd>
        </div>
        <div>
          <dt>Budget left</dt>
          <dd>${packet.budgetRemainingUsd.toFixed(2)}{packet.budgetBreached ? ' · OVERSPENT' : ''}</dd>
        </div>
        <div>
          <dt>QASM digest</dt>
          <dd title={qasmDigest}>{qasmDigest.slice(0, 18)}</dd>
        </div>
      </dl>
      {packet.budgetBreached && (
        <div role="alert" className="k-confirm-error">
          Budget breached — ceiling exceeded. Authorization refuses until funded.
        </div>
      )}
      <RunAllowancePanel
        sessionId={session?.id ?? ''}
        tenantId={session?.tenant_id ?? ''}
        backend={packet.backend}
        perJobEstimateUsd={costKnown ? (estimatedCost as number) : 1}
        onSelect={setAllowance}
      />
      <p className="k-confirm-trust">
        <ShieldAlert size={13} />
        <span>Untrusted circuit/prompt input: Ed25519 binds exact QASM digest at Go admission.</span>
      </p>
      {error && (
        <div role="alert" className="k-confirm-error">
          {error}
        </div>
      )}
      {!showDeny ? (
        <div className="k-confirm-actions">
          <button
            type="button"
            className="k-confirm-btn-primary"
            data-testid="confirm-card-authorize"
            disabled={busy || unresolved || !canAuthorize}
            title={!canAuthorize ? 'Disabled: needs bound session, proposed QASM, and cost estimate' : undefined}
            onClick={() => void handleApprove()}
          >
            <Fingerprint size={14} />
            <span>{busy ? 'Signing mandate…' : 'Authorize (Ed25519)'}</span>
          </button>
          <button
            type="button"
            className="k-confirm-btn-secondary"
            data-testid="confirm-card-downgrade"
            disabled={busy}
            onClick={() => {
              onSettled?.({
                outcome: 'downgraded',
                label: 'Downgraded to local simulator',
                detail: 'L1 auto-allow rail · $0.00 spend',
                at: new Date().toISOString(),
              });
              onDowngrade();
            }}
          >
            <ArrowDownToLine size={14} />
            <span>Downgrade to simulator</span>
          </button>
          <button
            type="button"
            className="k-confirm-btn-ghost"
            disabled={busy}
            onClick={() => setShowDeny(true)}
          >
            Deny dispatch…
          </button>
        </div>
      ) : (
        <div className="k-confirm-deny">
          <input
            value={denyReason}
            onChange={(e) => setDenyReason(e.target.value)}
            placeholder="Denial reason (required)…"
            aria-label="Denial reason"
            maxLength={280}
          />
          <div className="k-confirm-deny-row">
            <button
              type="button"
              className="k-confirm-btn-danger"
              disabled={busy || !denyReason.trim()}
              onClick={() => {
                const reason = denyReason.trim();
                if (!reason) return;
                onSettled?.({
                  outcome: 'denied',
                  label: 'Dispatch denied fail-closed',
                  detail: reason,
                  at: new Date().toISOString(),
                });
                onDenied(reason);
              }}
            >
              Confirm denial
            </button>
            <button
              type="button"
              className="k-confirm-btn-ghost"
              onClick={() => { setShowDeny(false); setDenyReason(''); }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
