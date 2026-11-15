'use client';

import React, { useState } from 'react';
import { approveWorkflow, planWorkflow, runWorkflow } from '@/lib/api';
import type { WorkflowDefinition } from '@/types';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';
import { publishWorkflowPlan } from '@/domain/workflow-plan-store';

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
      if (!out.ok) {
        // A refused plan never reaches the canvas.
        setError(out.error || 'workflow plan refused without a reason');
        return;
      }
      // C8: publish the dry-run draft so the watchable plan is on the canvas.
      publishWorkflowPlan({
        workflowId: out.workflow_id,
        name: workflow.name,
        steps: Array.isArray(out.steps) && out.steps.length > 0 ? out.steps : workflow.steps,
        unresolved: Array.isArray(out.unresolved_references) ? out.unresolved_references : [],
      });
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
    <div className="w-full rounded-card border border-border-strong bg-surface p-4 space-y-2" aria-label="Workflow planner">
      <div>
        <span className="text-label font-semibold text-text">Workflow planner / runner</span>
        <p className="text-[12px] text-muted">Plan validates against the tool registry. Running executes tools and reserves the declared budget — confirm with the workflow name.</p>
      </div>
      <textarea aria-label="Workflow JSON" value={raw} onChange={(e) => setRaw(e.target.value)} rows={6} spellCheck={false} className="w-full rounded-control-compact bg-field border border-border px-2 py-1 text-[12px] font-mono text-text" />
      <label className="block text-[12px] text-muted">Approval token (agent-created / approval-required workflows)
        <input aria-label="Workflow approval token" type="password" value={approvalToken} onChange={(e) => setApprovalToken(e.target.value)} className="mt-1 w-full rounded-control-compact bg-page border border-border px-2 py-1 text-[12px] font-mono text-text" />
      </label>
      <div className="flex flex-wrap gap-1.5">
        <button type="button" onClick={() => void onPlan()} disabled={busy !== null} className="px-2 py-1 rounded-control-compact bg-info-wash border border-info-edge text-info-ink text-[12px] disabled:opacity-40">
          {busy === 'plan' ? 'Planning…' : 'Plan (dry run)'}
        </button>
        <button type="button" onClick={() => void onApprove()} disabled={busy !== null || !sessionId} className="px-2 py-1 rounded-control-compact bg-soft-lilac-wash border border-soft-lilac-edge text-soft-lilac-ink text-[12px] disabled:opacity-40">
          {busy === 'approve' ? 'Approving…' : 'Approve current workflow'}
        </button>
        <button type="button" onClick={() => { const wf = parse(); if (wf) setConfirm(wf); }} disabled={busy !== null || !sessionId} className="px-2 py-1 rounded-control-compact bg-warning-wash border border-warning-edge text-warning-ink text-[12px] disabled:opacity-40">
          Run workflow…
        </button>
      </div>
      {error && <p role="alert" className="text-[12px] text-danger-ink">{error}</p>}
      {writeSettlementLabel(runWrite.write) && (
        <p role="status" className="text-[12px] font-mono text-muted">{writeSettlementLabel(runWrite.write)}</p>
      )}
      {plan && (
        <div className="rounded-row bg-card border border-border p-2 text-[12px] font-code text-text space-y-0.5">
          <div>plan {String(plan.workflow_id ?? '—')} · dry_run {String(plan.dry_run)} · steps {Array.isArray(plan.steps) ? plan.steps.length : 0}</div>
          {Array.isArray(plan.unresolved_references) && plan.unresolved_references.length > 0 && (
            <div className="text-warning-ink">unresolved: {plan.unresolved_references.join(', ')}</div>
          )}
          <div className="text-muted">Run remains disabled until you use the confirmation dialog.</div>
        </div>
      )}
      {result && (
        <div className="rounded-row bg-success-wash border border-success-edge p-2 text-[12px] font-code text-success-ink space-y-0.5">
          <div>workflow {String(result.workflow_id ?? '—')} · completed {String(result.steps_completed ?? '—')} · spent ${String(result.spent_usd ?? '—')}</div>
          <div className="text-success-ink opacity-85 break-all">{JSON.stringify(result.last_results ?? {}).slice(0, 400)}</div>
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
