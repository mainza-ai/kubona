'use client';

// Durable approval inbox (AG-19/AG-21b): proposed plans wait here until a
// human approves, rejects, or defers them, and tool calls marked `ask` wait for
// a one-time approval. Both are ledger projections, so they survive restarts;
// each decision is accepted once.

import React, { useCallback, useEffect, useState } from 'react';
import {
  decideApproval,
  decideToolApproval,
  fetchApprovals,
  fetchToolApprovals,
  type ApprovalPlan,
  type PlanEdits,
  type ToolApproval,
} from '@/domain/oversight';
import { Button } from '@/components/ui/Button';
import { Field, TextArea } from '@/components/ui/Forms';

function riskTone(risk?: string): string {
  if (risk === 'high') return 'text-warning-ink';
  if (risk === 'medium') return 'text-muted';
  return 'text-success-ink';
}

type Draft = {
  statement: string;
  tolerance: string;
  stop_rule: string;
  budget: string;
  spec: string;
  feedback: string;
};

function draftFor(plan: ApprovalPlan): Draft {
  return {
    statement: plan.statement ?? '',
    tolerance: plan.tolerance === undefined ? '' : String(plan.tolerance),
    stop_rule: plan.stop_rule ?? '',
    budget: plan.budget_usd === undefined ? '' : String(plan.budget_usd),
    spec: plan.spec ?? '{}',
    feedback: plan.review_feedback ?? '',
  };
}

export function ApprovalInbox({ sessionId }: { sessionId: string }) {
  const [pending, setPending] = useState<ApprovalPlan[]>([]);
  const [deferred, setDeferred] = useState<ApprovalPlan[]>([]);
  const [toolPending, setToolPending] = useState<ToolApproval[]>([]);
  const [toolCounts, setToolCounts] = useState<{ denied: number; used: number; expired: number }>({ denied: 0, used: 0, expired: 0 });
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [openEditors, setOpenEditors] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const applyPlans = (out: Awaited<ReturnType<typeof fetchApprovals>>) => {
    if (!out) return;
    setPending(out.pending);
    setDeferred(out.deferred);
  };
  const applyTools = (out: Awaited<ReturnType<typeof fetchToolApprovals>>) => {
    if (!out) return;
    setToolPending(out.pending);
    const recent = out.recent ?? [];
    setToolCounts({
      denied: recent.filter((item) => item.status === 'denied').length,
      used: recent.filter((item) => item.status === 'used').length,
      expired: recent.filter((item) => item.expired).length,
    });
  };

  const load = useCallback(async () => {
    if (!sessionId) return;
    applyPlans(await fetchApprovals(sessionId));
    applyTools(await fetchToolApprovals(sessionId));
  }, [sessionId]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchApprovals(sessionId).then((out) => {
      if (live) applyPlans(out);
    });
    fetchToolApprovals(sessionId).then((out) => {
      if (live) applyTools(out);
    });
    return () => { live = false; };
  }, [sessionId]);

  const editDraft = (plan: ApprovalPlan, patch: Partial<Draft>) => {
    setDrafts((prev) => ({ ...prev, [plan.id]: { ...(prev[plan.id] ?? draftFor(plan)), ...patch } }));
  };

  const toggleEditor = (plan: ApprovalPlan) => {
    setOpenEditors((prev) => {
      const next = !prev[plan.id];
      if (next) setDrafts((draftsPrev) => (draftsPrev[plan.id] ? draftsPrev : { ...draftsPrev, [plan.id]: draftFor(plan) }));
      return { ...prev, [plan.id]: next };
    });
  };

  // collectEdits validates the reviewer's draft client-side. The server
  // re-validates; this only avoids a round trip for obvious mistakes.
  const collectEdits = (plan: ApprovalPlan): PlanEdits | null => {
    const draft = drafts[plan.id];
    if (!draft) return {};
    const edits: PlanEdits = {};
    if (draft.statement.trim() !== (plan.statement ?? '')) {
      if (!draft.statement.trim()) {
        setNotice('Statement cannot be empty.');
        return null;
      }
      edits.statement = draft.statement.trim();
    }
    if (draft.tolerance !== '' && Number(draft.tolerance) !== plan.tolerance) {
      const tolerance = Number(draft.tolerance);
      if (!Number.isFinite(tolerance) || tolerance < 0) {
        setNotice('Tolerance must be a finite number >= 0.');
        return null;
      }
      edits.tolerance = tolerance;
    }
    if (draft.stop_rule.trim() !== (plan.stop_rule ?? '') && draft.stop_rule.trim()) {
      edits.stop_rule = draft.stop_rule.trim();
    }
    if (draft.budget !== '' && Number(draft.budget) !== plan.budget_usd) {
      const budget = Number(draft.budget);
      if (!Number.isFinite(budget) || budget <= 0) {
        setNotice('Budget must be a finite number > 0.');
        return null;
      }
      edits.budget_usd = budget;
    }
    if (draft.spec.trim() !== (plan.spec ?? '{}')) {
      try {
        const parsed = JSON.parse(draft.spec) as unknown;
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          setNotice('Spec must be a JSON object.');
          return null;
        }
        edits.spec = parsed as Record<string, unknown>;
      } catch {
        setNotice('Spec is not valid JSON.');
        return null;
      }
    }
    return edits;
  };

  const decide = async (plan: ApprovalPlan, decision: 'approve' | 'reject' | 'defer', deferredUntil?: string) => {
    setBusy(`${plan.id}:${decision}`);
    setNotice(null);
    const feedback = drafts[plan.id]?.feedback?.trim() || undefined;
    let edits: PlanEdits | undefined;
    if (decision === 'approve') {
      const collected = collectEdits(plan);
      if (collected === null) {
        setBusy(null);
        return;
      }
      edits = collected;
    }
    const out = await decideApproval(sessionId, plan.id, decision, { deferredUntil, feedback, edits });
    if (out.ok) {
      setNotice(
        decision === 'defer'
          ? 'Deferred.'
          : decision === 'approve'
            ? (edits && Object.keys(edits).length > 0 ? 'Approved with revisions.' : 'Approved.')
            : 'Rejected with feedback.',
      );
      setOpenEditors((prev) => ({ ...prev, [plan.id]: false }));
      await load();
    } else {
      setNotice(out.error);
    }
    setBusy(null);
  };

  const decideTool = async (item: ToolApproval, decision: 'approve' | 'deny') => {
    setBusy(`${item.id}:${decision}`);
    setNotice(null);
    const out = await decideToolApproval(sessionId, item.id, decision);
    if (out.ok) {
      setNotice(decision === 'approve' ? 'Tool call approved once.' : 'Tool call denied.');
      await load();
    } else {
      setNotice(out.error);
    }
    setBusy(null);
  };

  const card = (plan: ApprovalPlan, isDeferred: boolean) => {
    const draft = drafts[plan.id] ?? draftFor(plan);
    const open = !!openEditors[plan.id];
    return (
      <div key={plan.id} className="rounded-row border border-border bg-card px-2.5 py-2 text-[12px] space-y-1">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="font-semibold text-text">{plan.statement}</div>
            <div className="font-code text-faint">
              {plan.method || 'method'} · {plan.observable || '—'} vs {plan.acceptance_metric || '—'}
              {plan.created_at ? ` · ${plan.created_at.slice(0, 19)}Z` : ''}
            </div>
          </div>
          <span
            className={`shrink-0 rounded-chip border border-border px-1.5 py-0.5 text-[11px] font-semibold ${riskTone(plan.risk)}`}
            title={plan.risk_categories?.length ? `risk categories: ${plan.risk_categories.join(', ')}` : undefined}
          >
            {plan.risk || 'unclassified'}
            {plan.risk_categories && plan.risk_categories.length > 0 ? ` · ${plan.risk_categories.join(' · ')}` : ''}
          </span>
        </div>
        {isDeferred && (
          <p className="text-[11px] text-warning">
            Deferred until {plan.deferred_until}{plan.defer_reason ? ` · ${plan.defer_reason}` : ''}
          </p>
        )}
        {plan.review_feedback && (
          <p className="text-[11px] text-label-secondary">Feedback: {plan.review_feedback}</p>
        )}
        {open && (
          <div className="space-y-1 rounded-row border border-border bg-surface p-2" aria-label="Review adjustments">
            <Field
              label="Statement"
              value={draft.statement}
              onChange={(event) => editDraft(plan, { statement: event.target.value })}
            />
            <div className="grid grid-cols-2 gap-1">
              <Field
                label="Tolerance"
                type="number"
                min="0"
                step="any"
                value={draft.tolerance}
                onChange={(event) => editDraft(plan, { tolerance: event.target.value })}
              />
              <Field
                label="Budget (USD)"
                type="number"
                min="0"
                step="any"
                value={draft.budget}
                onChange={(event) => editDraft(plan, { budget: event.target.value })}
              />
            </div>
            <Field
              label="Stop rule"
              value={draft.stop_rule}
              onChange={(event) => editDraft(plan, { stop_rule: event.target.value })}
            />
            <TextArea
              label="Spec (JSON)"
              rows={3}
              className="font-code"
              value={draft.spec}
              onChange={(event) => editDraft(plan, { spec: event.target.value })}
            />
            <Field
              label="Feedback (attached on reject)"
              value={draft.feedback}
              onChange={(event) => editDraft(plan, { feedback: event.target.value })}
            />
          </div>
        )}
        <div className="flex flex-wrap items-center gap-1">
          <Button variant="primary" size="compact" disabled={busy !== null} onClick={() => void decide(plan, 'approve')}>
            Approve
          </Button>
          <Button variant="danger" size="compact" disabled={busy !== null} onClick={() => void decide(plan, 'reject')}>
            Reject
          </Button>
          {!isDeferred && (
            <>
              <Button variant="ghost" size="compact" disabled={busy !== null} onClick={() => void decide(plan, 'defer', new Date(Date.now() + 60 * 60 * 1000).toISOString())}>
                Defer 1h
              </Button>
              <Button variant="ghost" size="compact" disabled={busy !== null} onClick={() => void decide(plan, 'defer', new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString())}>
                Defer 1d
              </Button>
            </>
          )}
          <Button variant="ghost" size="compact" aria-expanded={open} onClick={() => toggleEditor(plan)}>
            {open ? 'Close review' : 'Adjust'}
          </Button>
        </div>
      </div>
    );
  };

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

      <div className="space-y-2 border-t border-border pt-2">
        <div className="flex items-center justify-between">
          <div>
            <span className="text-label font-semibold text-text">Tool calls</span>
            <p className="text-[12px] text-faint">A tool marked <span className="font-code">ask</span> waits for one approval of these exact arguments.</p>
          </div>
          <span className="font-code text-[11px] text-muted">
            {toolPending.length} pending · {toolCounts.denied} denied · {toolCounts.used} spent{toolCounts.expired > 0 ? ` · ${toolCounts.expired} expired` : ''}
          </span>
        </div>
        {toolPending.length === 0 && <p className="text-[12px] text-muted">No tool calls waiting.</p>}
        {toolPending.map((item) => (
          <div key={item.id} className="rounded-row border border-border bg-card px-2.5 py-2 text-[12px] space-y-1">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="font-semibold text-text">{item.tool}</div>
                <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-all font-code text-[11px] text-faint">{item.args}</pre>
              </div>
              <span className="shrink-0 rounded-chip border border-border px-1.5 py-0.5 text-[11px] text-muted">
                {item.requested_at.slice(0, 19).replace('T', ' ')}Z
              </span>
            </div>
            <div className="flex flex-wrap items-center gap-1">
              <Button variant="primary" size="compact" disabled={busy !== null} onClick={() => void decideTool(item, 'approve')}>
                Approve once
              </Button>
              <Button variant="danger" size="compact" disabled={busy !== null} onClick={() => void decideTool(item, 'deny')}>
                Deny
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
