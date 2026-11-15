'use client';

import React, { useEffect, useState } from 'react';
import { Fingerprint, ChevronDown, ChevronUp, ArrowDownToLine } from 'lucide-react';
import { CampaignState, Session } from '@/types';
import { ApprovalPacketCard } from '@/components/common/ApprovalPacketCard';
import { buildApprovalPacketFromCampaign } from '@/lib/approvalPacket';
import { executeMeteredAuthorization } from '@/components/workbench/ConfirmCard';
import { useConsequentialSettle } from '@/lib/consequential';

interface ActionGateProps {
  campaign: CampaignState | null;
  session: Session | null;
  onAuthorized: () => void;
  onDenied: (reason: string) => void;
  onDowngrade: () => void;
}

/**
 * Calibrated Action Gate: the single high-stakes friction point. Rendered
 * ONLY while a metered-hardware dispatch awaits human authorization (parent
 * gates visibility on backend tier — L1 simulator runs never raise it, per
 * the anti-fatigue rule). Approval signs a real AP2 Ed25519 SpendMandate and
 * verifies it against the Go backend — denial halts fail-closed with a
 * required reason. Downgrade routes the intent to the local simulator rail.
 */
export function ActionGate({ campaign, session, onAuthorized, onDenied, onDowngrade }: ActionGateProps) {
  // No silent money defaults: an absent estimate disables authorization
  // (previously `?? 0.12` invented a spend cap for metered backends).
  const estimatedCost = campaign?.quantum_factors?.estimated_spend_usd;
  const costKnown = typeof estimatedCost === 'number' && isFinite(estimatedCost);
  const sessionBound = (session?.id ?? '') !== '';
  // W0.4: the mandate binds the exact program about to run — no QASM, no
  // signature. The daemon recomputes this binding at admission and refuses
  // a receipt for any other program.
  const programQasm = campaign?.quantum_factors?.qasm ?? '';
  const programKnown = programQasm !== '';
  const canAuthorize = costKnown && sessionBound && programKnown;
  const recommendedBackend = campaign?.quantum_factors?.recommended_backend || 'Local simulator';
  const qubits = campaign?.quantum_factors?.qubit_count ?? 0;
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showDetail, setShowDetail] = useState(false);
  const [showDeny, setShowDeny] = useState(false);
  const [denyReason, setDenyReason] = useState('');
  const [message, setMessage] = useState<string | null>(null);
  const [rejected, setRejected] = useState(false);
  const [dispatchNote, setDispatchNote] = useState<string | null>(null);
  const [unresolved, setUnresolved] = useState(false);
  const settled = useConsequentialSettle(true);

  const packet = buildApprovalPacketFromCampaign(
    campaign ?? null,
    session?.id || '',
    estimatedCost ?? 0,
    session?.spend_limit_usd ?? 25.0,
    session?.current_spend_usd ?? 0
  );

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      if (showDeny) {
        setShowDeny(false);
        setDenyReason('');
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [showDeny]);

  const handleAuthorize = async () => {
    setIsSubmitting(true);
    setMessage(null);
    try {
      const outcome = await executeMeteredAuthorization(campaign, session);
      if (!outcome.ok) {
        if (outcome.unknown) {
          setUnresolved(true);
          setRejected(false);
          setMessage(outcome.message);
          return;
        }
        setRejected(true);
        setMessage(outcome.message);
        return;
      }
      setRejected(false);
      setDispatchNote(`Dispatched job ${outcome.jobId.slice(0, 8)} to ${outcome.backend}. Track it in Recorder / Measured outputs.`);
      onAuthorized();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      data-testid="action-gate"
      className="absolute bottom-6 left-6 z-20 w-80 bg-float border border-warning rounded-card p-4 shadow-[var(--shadow-float)] text-text"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-capsule bg-warning" aria-hidden="true" />
          <span className="text-[12px] font-bold text-warning font-code uppercase tracking-wider">
            Action Gate
          </span>
        </div>
        <span className="text-[12px] font-code text-muted">Needs you</span>
      </div>

      <div className="mt-2.5">
        <div className="text-label text-text font-medium">Authorize {recommendedBackend} Dispatch</div>
        <div className="text-[12px] text-muted mt-0.5">
          {recommendedBackend} · {qubits} Qubits ·{' '}
          {costKnown ? `$${(estimatedCost as number).toFixed(2)} max` : 'unpriced — authorization disabled'}
        </div>
      </div>

      {message && (
        <div role={rejected ? 'alert' : 'status'} className={`mt-2.5 p-2.5 rounded-row text-[12px] border ${rejected ? 'bg-[color-mix(in_oklab,var(--color-danger)_14%,var(--color-surface))] border-danger text-danger' : 'bg-[color-mix(in_oklab,var(--color-info)_14%,var(--color-surface))] border-info text-info'}`}>
          {message}
        </div>
      )}
      {dispatchNote && (
        <div role="status" className="mt-2.5 p-2.5 rounded-row bg-[color-mix(in_oklab,var(--color-success)_14%,var(--color-surface))] border border-success text-success text-[12px]">
          {dispatchNote}
        </div>
      )}

      {!showDeny ? (
        <div className="mt-3 space-y-2">
          <button
            data-testid="action-gate-authorize"
            onClick={handleAuthorize}
            disabled={isSubmitting || !canAuthorize || unresolved || !settled}
            data-settle={settled ? 'ready' : 'pending'}
            title={unresolved ? 'Disabled until the lost launch reply is reconciled' : !canAuthorize ? 'Disabled: needs a bound session, a proposed program, and a priced campaign' : undefined}
            className="w-full min-h-[34px] py-2 px-3 rounded-row bg-warning hover:opacity-90 text-on-lit font-semibold text-label flex items-center justify-center gap-2 transition-opacity disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-warning focus-visible:outline-none"
          >
            <Fingerprint className="w-4 h-4" />
            <span>{isSubmitting ? 'Signing mandate…' : 'Authorize (Ed25519)'}</span>
          </button>
          <button
            data-testid="action-gate-downgrade"
            onClick={onDowngrade}
            disabled={isSubmitting}
            title="Route this intent to the local simulator rail instead ($0.00, auto-allow)"
            className="w-full min-h-[34px] py-2 px-3 rounded-row bg-control hover:bg-control-hover border border-border-strong text-text font-medium text-label flex items-center justify-center gap-2 transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-info focus-visible:outline-none"
          >
            <ArrowDownToLine className="w-4 h-4 text-info" />
            <span>Downgrade to simulator</span>
          </button>
          <div className="flex items-center justify-between">
            <button
              onClick={() => setShowDetail((v) => !v)}
              className="text-[12px] font-code text-muted hover:text-text flex items-center gap-1 transition-colors"
            >
              {showDetail ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
              <span>burden packet</span>
            </button>
            <button
              onClick={() => setShowDeny(true)}
              autoFocus
              data-testid="action-gate-deny"
              disabled={isSubmitting}
              className="text-[12px] font-code text-danger hover:opacity-80 transition-opacity disabled:opacity-50"
            >
              Abort
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-3 space-y-2">
          <input
            value={denyReason}
            onChange={(e) => setDenyReason(e.target.value)}
            placeholder="Denial reason (required)…"
            maxLength={280}
            className="w-full px-3 py-2 rounded-row bg-field border border-danger text-label text-text placeholder:text-faint focus:outline-none"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={() => denyReason.trim() && onDenied(denyReason.trim())}
              disabled={isSubmitting || !denyReason.trim()}
              className="flex-1 min-h-[34px] py-2 px-4 rounded-row bg-danger hover:opacity-90 text-on-lit text-label font-semibold transition-opacity disabled:opacity-50"
            >
              Confirm denial
            </button>
            <button
              onClick={() => { setShowDeny(false); setDenyReason(''); }}
              className="min-h-[34px] py-2 px-4 rounded-row bg-control hover:bg-control-hover text-label-secondary text-label transition-colors"
            >
              Back
            </button>
          </div>
        </div>
      )}

      {showDetail && (
        <div className="mt-3 max-h-72 overflow-y-auto">
          <ApprovalPacketCard
            packet={packet}
            isSubmitting={isSubmitting}
            onApproveOnce={handleAuthorize}
            onApproveUpToMax={handleAuthorize}
            onDeny={(reason) => onDenied(reason)}
          />
        </div>
      )}
    </div>
  );
}
