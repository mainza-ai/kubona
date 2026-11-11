'use client';

import React, { useState } from 'react';
import { approveWorkflow, planWorkflow, runWorkflow } from '@/lib/api';
import type { WorkflowDefinition } from '@/types';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

const SAMPLE = `{
  "name": "provider-audit",
  "budget_ceiling_usd": 0,
  "steps": [
    { "id": "s1", "tool": "execution/list_provider_registry", "args": {} }
  ]
}`;

export function WorkflowPanel({ sessionId }: { sessionId: string }) {
  const [raw, setRaw] = useState(SAMPLE);
  const [plan, setPlan] = useState<Record<string, unknown> | null>(null);
  const [result, setResult] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<'plan' | 'run' | 'approve' | null>(null);
  const [approvalToken, setApprovalToken] = useState('');
  const [confirm, setConfirm] = useState<WorkflowDefinition | null>(null);
  const runWrite = useWriteSettlement();

  const parse = (): WorkflowDefinition | null => {
    setError(null);
    try {
      const parsed = JSON.parse(raw) as WorkflowDefinition;
      if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.steps)) {
        setError('workflow must be an object with a steps[] array.');
        return null;
      }
      return parsed;
    } catch {
      setError('workflow must be valid JSON.');
      return null;
    }
  };

  const onPlan = async () => {
    const workflow = parse();
    if (!workflow) return;
    setBusy('plan');
    setPlan(null);
    setResult(null);
    try {
      const out = await planWorkflow({ session_id: sessionId || undefined, workflow });
      setPlan(out as unknown as Record<string, unknown>);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const onApprove = async () => {
    if (!sessionId) return;
    const workflow = parse();
    if (!workflow) return;
    setBusy('approve');
    setError(null);
    try {
      const out = await approveWorkflow({ session_id: sessionId, workflow, ttl_seconds: 3600 });
      setApprovalToken(out.approval_token);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const onRun = async () => {
    if (!confirm) return;
    setBusy('run');
    setResult(null);
    const requestId = runWrite.begin('run workflow');
    try {
      const out = await runWorkflow({ session_id: sessionId, workflow: confirm, ...(approvalToken.trim() ? { approval_token: approvalToken.trim() } : {}) });
      runWrite.applied(requestId, 'workflow completed');
      setResult(out as unknown as Record<string, unknown>);
      setConfirm(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      runWrite[message.includes('refused (HTTP') ? 'refused' : 'unknown'](requestId, message);
      setError(message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Workflow planner">
      <div>
        <span className="text-xs font-semibold text-[var(--color-text)]">Workflow planner / runner</span>
        <p className="text-[12px] text-[var(--color-muted)]">Plan validates against the tool registry. Running executes tools and reserves the declared budget — confirm with the workflow name.</p>
      </div>
      <textarea aria-label="Workflow JSON" value={raw} onChange={(e) => setRaw(e.target.value)} rows={6} spellCheck={false} className="w-full rounded-lg bg-[var(--color-field)] border border-[var(--color-border)] px-2 py-1 text-[12px] font-mono text-[var(--color-text)]" />
      <label className="block text-[12px] text-[var(--color-muted)]">Approval token (agent-created / approval-required workflows)
        <input aria-label="Workflow approval token" type="password" value={approvalToken} onChange={(e) => setApprovalToken(e.target.value)} className="mt-1 w-full rounded-lg bg-[var(--color-page)] border border-[var(--color-border)] px-2 py-1 text-[12px] font-mono text-[var(--color-text)]" />
      </label>
      <div className="flex flex-wrap gap-1.5">
        <button type="button" onClick={() => void onPlan()} disabled={busy !== null} className="px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] disabled:opacity-40">
          {busy === 'plan' ? 'Planning…' : 'Plan (dry run)'}
        </button>
        <button type="button" onClick={() => void onApprove()} disabled={busy !== null || !sessionId} className="px-2 py-1 rounded-lg bg-[var(--color-soft-lilac-wash)] border border-[var(--color-soft-lilac-edge)] text-[var(--color-soft-lilac-ink)] text-[12px] disabled:opacity-40">
          {busy === 'approve' ? 'Approving…' : 'Approve current workflow'}
        </button>
        <button type="button" onClick={() => { const wf = parse(); if (wf) setConfirm(wf); }} disabled={busy !== null || !sessionId} className="px-2 py-1 rounded-lg bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-[12px] disabled:opacity-40">
          Run workflow…
        </button>
      </div>
      {error && <p role="alert" className="text-[12px] text-[var(--color-danger-ink)]">{error}</p>}
      {writeSettlementLabel(runWrite.write) && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">{writeSettlementLabel(runWrite.write)}</p>
      )}
      {plan && (
        <div className="rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] p-2 text-[12px] font-code text-[var(--color-text)] space-y-0.5">
          <div>plan {String(plan.workflow_id ?? '—')} · dry_run {String(plan.dry_run)} · steps {Array.isArray(plan.steps) ? plan.steps.length : 0}</div>
          {Array.isArray(plan.unresolved_references) && plan.unresolved_references.length > 0 && (
            <div className="text-[var(--color-warning-ink)]">unresolved: {plan.unresolved_references.join(', ')}</div>
          )}
          <div className="text-[var(--color-muted)]">Run remains disabled until you use the confirmation dialog.</div>
        </div>
      )}
      {result && (
        <div className="rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] p-2 text-[12px] font-code text-[var(--color-success-ink)] space-y-0.5">
          <div>workflow {String(result.workflow_id ?? '—')} · completed {String(result.steps_completed ?? '—')} · spent ${String(result.spent_usd ?? '—')}</div>
          <div className="text-[var(--color-success-ink)] opacity-85 break-all">{JSON.stringify(result.last_results ?? {}).slice(0, 400)}</div>
        </div>
      )}
      {confirm && (
        <ConfirmDialog
          open
          title={`Run workflow “${confirm.name}”?`}
          body={<span>This executes the listed tools and may reserve up to <strong>${Number(confirm.budget_ceiling_usd ?? 0).toFixed(2)}</strong>. Type the workflow name to confirm.</span>}
          requireText={confirm.name}
          confirmLabel="Run workflow"
          busy={busy === 'run'}
          error={error}
          onConfirm={() => void onRun()}
          onCancel={() => { if (busy !== 'run') setConfirm(null); }}
        />
      )}
    </div>
  );
}
