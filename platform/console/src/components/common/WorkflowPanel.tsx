'use client';

import React, { useState } from 'react';
import { planWorkflow, runWorkflow } from '@/lib/api';
import type { WorkflowDefinition } from '@/types';
import ConfirmDialog from '@/components/common/ConfirmDialog';

const SAMPLE = `{
  "name": "campaign-audit",
  "budget_ceiling_usd": 0,
  "steps": [
    { "id": "s1", "tool": "get_campaign", "args": {} }
  ]
}`;

export function WorkflowPanel({ sessionId }: { sessionId: string }) {
  const [raw, setRaw] = useState(SAMPLE);
  const [plan, setPlan] = useState<Record<string, unknown> | null>(null);
  const [result, setResult] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<'plan' | 'run' | null>(null);
  const [confirm, setConfirm] = useState<WorkflowDefinition | null>(null);

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

  const onRun = async () => {
    if (!confirm) return;
    setBusy('run');
    setResult(null);
    try {
      const out = await runWorkflow({ session_id: sessionId, workflow: confirm });
      setResult(out as unknown as Record<string, unknown>);
      setConfirm(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Workflow planner">
      <div>
        <span className="text-xs font-semibold text-white">Workflow planner / runner</span>
        <p className="text-[10px] text-slate-500">Plan validates against the tool registry. Running executes tools and reserves the declared budget — confirm with the workflow name.</p>
      </div>
      <textarea aria-label="Workflow JSON" value={raw} onChange={(e) => setRaw(e.target.value)} rows={6} spellCheck={false} className="w-full rounded-lg bg-black/60 border border-white/10 px-2 py-1 text-[11px] font-mono text-slate-200" />
      <div className="flex flex-wrap gap-1.5">
        <button type="button" onClick={() => void onPlan()} disabled={busy !== null} className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] disabled:opacity-40">
          {busy === 'plan' ? 'Planning…' : 'Plan (dry run)'}
        </button>
        <button type="button" onClick={() => { const wf = parse(); if (wf) setConfirm(wf); }} disabled={busy !== null || !sessionId} className="px-2 py-1 rounded-lg bg-amber-500/15 border border-amber-400/30 text-amber-200 text-[11px] disabled:opacity-40">
          Run workflow…
        </button>
      </div>
      {error && <p role="alert" className="text-[11px] text-rose-300">{error}</p>}
      {plan && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[11px] font-code text-slate-300 space-y-0.5">
          <div>plan {String(plan.workflow_id ?? '—')} · dry_run {String(plan.dry_run)} · steps {Array.isArray(plan.steps) ? plan.steps.length : 0}</div>
          {Array.isArray(plan.unresolved_references) && plan.unresolved_references.length > 0 && (
            <div className="text-amber-300">unresolved: {plan.unresolved_references.join(', ')}</div>
          )}
          <div className="text-slate-500">Run remains disabled until you use the confirmation dialog.</div>
        </div>
      )}
      {result && (
        <div className="rounded-xl bg-emerald-950/40 border border-emerald-500/30 p-2 text-[11px] font-code text-emerald-200 space-y-0.5">
          <div>workflow {String(result.workflow_id ?? '—')} · completed {String(result.steps_completed ?? '—')} · spent ${String(result.spent_usd ?? '—')}</div>
          <div className="text-emerald-300/70 break-all">{JSON.stringify(result.last_results ?? {}).slice(0, 400)}</div>
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
