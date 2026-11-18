'use client';

// Durable approval inbox (AG-19): proposed plans wait here until a human
// approves, rejects, or defers them. The list is a ledger projection, so it
// survives restarts; a decision is accepted once.

import React, { useCallback, useEffect, useState } from 'react';
import { decideApproval, fetchApprovals, type ApprovalPlan } from '@/lib/api';

function riskTone(risk?: string): string {
  if (risk === 'high') return 'border-warning-edge bg-warning-wash text-warning-ink';
  if (risk === 'medium') return 'border-border bg-fill text-muted';
  return 'border-success-edge bg-success-wash text-success-ink';
}

export function ApprovalInbox({ sessionId }: { sessionId: string }) {
  const [pending, setPending] = useState<ApprovalPlan[]>([]);
  const [deferred, setDeferred] = useState<ApprovalPlan[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!sessionId) return;
    const out = await fetchApprovals(sessionId);
    if (out) {
      setPending(out.pending);
      setDeferred(out.deferred);
    }
  }, [sessionId]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchApprovals(sessionId).then((out) => {
      if (!live || !out) return;
      setPending(out.pending);
      setDeferred(out.deferred);
    });
    return () => { live = false; };
  }, [sessionId]);

  const decide = async (plan: ApprovalPlan, decision: 'approve' | 'reject' | 'defer', deferredUntil?: string) => {
    setBusy(`${plan.id}:${decision}`);
    setNotice(null);
    const out = await decideApproval(sessionId, plan.id, decision, { deferredUntil });
    if (out.ok) {
      setNotice(decision === 'defer' ? 'Deferred.' : decision === 'approve' ? 'Approved.' : 'Rejected.');
      await load();
    } else {
      setNotice(out.error);
    }
    setBusy(null);
  };

  const deferFor = (plan: ApprovalPlan, ms: number) => {
    void decide(plan, 'defer', new Date(Date.now() + ms).toISOString());
  };

  const card = (plan: ApprovalPlan, isDeferred: boolean) => (
    <div key={plan.id} className="rounded-row border border-border bg-card px-2.5 py-2 text-[12px] space-y-1">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="font-semibold text-text">{plan.statement}</div>
          <div className="font-code text-faint">
            {plan.method || 'method'} · {plan.observable || '—'} vs {plan.acceptance_metric || '—'}
            {plan.created_at ? ` · ${plan.created_at.slice(0, 19)}Z` : ''}
          </div>
        </div>
        <span className={`shrink-0 rounded-chip border px-1.5 py-0.5 text-[11px] font-semibold ${riskTone(plan.risk)}`}>
          {plan.risk || 'unclassified'}
        </span>
      </div>
      {isDeferred && (
        <p className="text-[11px] text-warning">
          Deferred until {plan.deferred_until}{plan.defer_reason ? ` · ${plan.defer_reason}` : ''}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-1">
        <button type="button" disabled={busy !== null} onClick={() => void decide(plan, 'approve')}
          className="rounded-chip border border-success-edge bg-success-wash px-2 py-1 text-success-ink disabled:opacity-40">
          Approve
        </button>
        <button type="button" disabled={busy !== null} onClick={() => void decide(plan, 'reject')}
          className="rounded-chip border border-danger-edge bg-danger-wash px-2 py-1 text-danger-ink disabled:opacity-40">
          Reject
        </button>
        {!isDeferred && (
          <>
            <button type="button" disabled={busy !== null} onClick={() => deferFor(plan, 60 * 60 * 1000)}
              className="rounded-chip border border-border bg-fill px-2 py-1 text-muted disabled:opacity-40">
              Defer 1h
            </button>
            <button type="button" disabled={busy !== null} onClick={() => deferFor(plan, 24 * 60 * 60 * 1000)}
              className="rounded-chip border border-border bg-fill px-2 py-1 text-muted disabled:opacity-40">
              Defer 1d
            </button>
          </>
        )}
      </div>
    </div>
  );

  return (
    <div className="w-full rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Approval inbox">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-label font-semibold text-text">Approvals</span>
          <p className="text-[12px] text-faint">Proposed plans wait for a human; decisions are durable and accepted once.</p>
        </div>
        <span className="font-code text-[11px] text-muted">{pending.length} pending · {deferred.length} deferred</span>
      </div>
      {notice && <p role="status" className="text-[12px] text-label-secondary">{notice}</p>}
      {pending.length === 0 && deferred.length === 0 && (
        <p className="text-[12px] text-muted">No pending approvals.</p>
      )}
      {pending.map((plan) => card(plan, false))}
      {deferred.map((plan) => card(plan, true))}
    </div>
  );
}
