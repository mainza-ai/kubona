'use client';

import React, { useEffect, useState } from 'react';
import { Play, Search } from 'lucide-react';
import { fetchEventsResult, planDiscovery, runDiscoveryProtocol, type DiscoveryPlan, type DiscoveryProtocolResult } from '@/lib/api';

/** Human entry point for the measured discovery protocol. The planning
 * response is deterministic; the run executes real jobs through the Go rail. */
export function DiscoveryProtocolPanel({ sessionId }: { sessionId: string | null }) {
  const [goal, setGoal] = useState('transverse-field Ising spin chain');
  const [plan, setPlan] = useState<DiscoveryPlan | null>(null);
  const [result, setResult] = useState<DiscoveryProtocolResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [history, setHistory] = useState<Array<Record<string, unknown>>>([]);
  // Reset during render on session change; the effect only loads the new
  // session's ledger tail, so there is no synchronous setState in the effect.
  const [prevSessionId, setPrevSessionId] = useState<string | null>(sessionId);
  if (prevSessionId !== sessionId) {
    setPrevSessionId(sessionId);
    setHistory([]);
  }

  const parseHistory = (events: Awaited<ReturnType<typeof fetchEventsResult>>) => {
    if (!events.ok) return [];
    return events.data
      .filter((event) => event.event_type === 'discovery_protocol_completed')
      .map((event) => ({ id: event.id, timestamp: event.timestamp, ...event.payload }))
      .sort((a, b) => String(b.timestamp).localeCompare(String(a.timestamp)))
      .slice(0, 10);
  };

  const loadHistory = async () => {
    if (!sessionId) return;
    setHistory(parseHistory(await fetchEventsResult(sessionId)));
  };

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchEventsResult(sessionId).then((events) => {
      if (live) setHistory(parseHistory(events));
    });
    return () => { live = false; };
  }, [sessionId]);

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
      await loadHistory();
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
      {history.length > 0 && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] space-y-1">
          <div className="font-semibold text-white">Protocol history</div>
          {history.map((row) => {
            const steps = Array.isArray(row.steps) ? row.steps as Array<Record<string, unknown>> : [];
            return (
              <div key={String(row.id)} className="border-t border-white/[0.06] pt-1 first:border-0 first:pt-0">
                <div className="font-code text-[var(--color-muted)]">{String(row.protocol_id || 'unknown').slice(0, 12)} · {String(row.completed ?? 0)} completed · {String(row.failed ?? 0)} failed · {String(row.measured ?? 0)} measured · {String(row.claimable ?? 0)} claimable</div>
                <div className="text-[var(--color-muted)]">{String(row.goal || '')}</div>
                {steps.length > 0 && <div className="font-code">{steps.map((step) => `${String(step.kind)}=${String(step.status)}`).join(' · ')}</div>}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
