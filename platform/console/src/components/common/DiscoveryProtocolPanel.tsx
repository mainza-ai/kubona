'use client';

import React, { useState } from 'react';
import { Play, Search } from 'lucide-react';
import { planDiscovery, runDiscoveryProtocol, type DiscoveryPlan, type DiscoveryProtocolResult } from '@/lib/api';

/** Human entry point for the measured discovery protocol. The planning
 * response is deterministic; the run executes real jobs through the Go rail. */
export function DiscoveryProtocolPanel({ sessionId }: { sessionId: string | null }) {
  const [goal, setGoal] = useState('transverse-field Ising spin chain');
  const [plan, setPlan] = useState<DiscoveryPlan | null>(null);
  const [result, setResult] = useState<DiscoveryProtocolResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const makePlan = async () => {
    setBusy(true); setError(null); setResult(null);
    try {
      setPlan(await planDiscovery(goal));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const run = async () => {
    if (!sessionId) { setError('select a campaign first'); return; }
    setBusy(true); setError(null); setResult(null);
    try {
      setResult(await runDiscoveryProtocol(sessionId, goal, `ui-discovery-${Date.now()}`));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-3" aria-label="Measured discovery protocol">
      <div>
        <span className="text-xs font-semibold text-white">Measured discovery protocol</span>
        <p className="text-[12px] text-[var(--color-muted)]">Plan a bounded experiment from a goal, then execute 1–5 real jobs through the governed rail. The summary contains stored job results, not synthesized prose.</p>
      </div>
      <label className="block space-y-1 text-[12px]">Goal
        <input aria-label="Discovery goal" value={goal} onChange={(e) => setGoal(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
      </label>
      <div className="flex flex-wrap gap-2">
        <button type="button" onClick={() => void makePlan()} disabled={busy || !goal.trim()} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
          <Search className="w-3.5 h-3.5" /> Plan
        </button>
        <button type="button" onClick={() => void run()} disabled={busy || !sessionId || !goal.trim()} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
          <Play className="w-3.5 h-3.5" /> {busy ? 'Running…' : 'Run protocol'}
        </button>
      </div>
      {error && <p role="alert" className="text-[12px] text-rose-300">{error}</p>}
      {plan && !result && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] space-y-1">
          <div>planned by {plan.planned_by}</div>
          {plan.steps.map((step, i) => <div key={i}>{i + 1}. {step.kind}</div>)}
        </div>
      )}
      {result && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] space-y-1">
          <div>protocol {result.protocol_id.slice(0, 12)} · {result.steps_completed} completed · {result.steps_failed} failed · {result.measured_results} measured · {result.claimable_results} claimable</div>
          {result.results.map((r, i) => <div key={i}>{i + 1}. {r.kind} · {r.status}{r.job_id ? ` · job ${r.job_id.slice(0, 8)}` : ''}{r.error ? ` · ${r.error}` : ''}</div>)}
        </div>
      )}
    </div>
  );
}
