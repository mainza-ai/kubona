'use client';

import React, { useCallback, useEffect, useState } from 'react';
import {
  fetchAutoresearchHarnesses,
  fetchAutoresearchLoops,
  startAutoresearchLoop,
  stepAutoresearchLoop,
  pauseAutoresearchLoop,
  resumeAutoresearchLoop,
  stopAutoresearchLoop,
} from '@/lib/api';
import type { AutoresearchHarness, AutoresearchLoop } from '@/types';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

export function AutoresearchPanel({ sessionId }: { sessionId: string }) {
  const [harnesses, setHarnesses] = useState<AutoresearchHarness[]>([]);
  const [loops, setLoops] = useState<AutoresearchLoop[] | null>(null);
  const [harnessId, setHarnessId] = useState('');
  const [runner, setRunner] = useState<Record<string, unknown> | null>(null);
  const [maxIterations, setMaxIterations] = useState('10');
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const settlement = useWriteSettlement();

  const load = useCallback(async () => {
    const [h, l] = await Promise.all([fetchAutoresearchHarnesses(), fetchAutoresearchLoops()]);
    setHarnesses(h.harnesses);
    setLoops(l);
    setHarnessId((cur) => cur || h.harnesses[0]?.id || '');
  }, []);

  useEffect(() => {
    let live = true;
    Promise.all([fetchAutoresearchHarnesses(), fetchAutoresearchLoops()]).then(([h, l]) => {
      if (!live) return;
      setHarnesses(h.harnesses);
      setRunner(h.docker_runner);
      setLoops(l);
      setHarnessId((cur) => cur || h.harnesses[0]?.id || '');
    });
    return () => { live = false; };
  }, []);

  const run = async (kind: string, fn: () => Promise<unknown>, success: string) => {
    setBusy(kind);
    setNotice(null);
    const requestId = settlement.begin(`${kind} loop`);
    try {
      const out = await fn();
      if (out) {
        settlement.applied(requestId, `${kind} loop · ${success.replace(/\.$/, '')}`);
      } else {
        settlement.unknown(requestId, `${kind} not confirmed — backend refused or transport failed`);
      }
      await load();
    } catch (err) {
      settlement.unknown(requestId, `${kind} not confirmed — ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Autoresearch loops">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-label font-semibold text-text">Autoresearch loops</span>
          <p className="text-[12px] text-faint">Sandboxed harness stepping; each transition re-reads server state.</p>
        </div>
        <button type="button" onClick={() => void load()} className="px-2 py-1 rounded-control-compact bg-fill border border-border text-[12px] text-label-secondary">Refresh</button>
      </div>
      {runner && runner.available !== true && (
        <p role="status" className="text-[12px] text-warning">
          Sandbox unavailable — autoresearch is fail-closed by default. {String(runner.reason || '')}
        </p>
      )}
      {runner && runner.available === true && (
        <p className="text-[12px] text-muted">Sandbox ready: {String(runner.image || 'runner')} (read-only, cap-drop, no network).</p>
      )}
      <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
        <select aria-label="Autoresearch harness" value={harnessId} onChange={(e) => setHarnessId(e.target.value)} className="bg-field border border-border rounded-control-compact px-2 py-1 text-text font-mono">
          {harnesses.length === 0 && <option value="">no harnesses</option>}
          {harnesses.map((h) => <option key={h.id} value={h.id}>{h.id}</option>)}
        </select>
        <input aria-label="Max iterations" value={maxIterations} onChange={(e) => setMaxIterations(e.target.value)} inputMode="numeric" className="w-16 bg-field border border-border rounded-control-compact px-2 py-1 text-text font-mono" />
        <button type="button" disabled={busy !== null || !harnessId || !sessionId} onClick={() => {
          const n = Number(maxIterations);
          if (!Number.isInteger(n) || n < 1 || n > 1000) { setNotice('max iterations must be an integer 1..1000.'); return; }
          void run('start', () => startAutoresearchLoop(harnessId, n, undefined, sessionId), 'Loop started.');
        }} className="px-2 py-1 rounded-control-compact bg-soft-lilac-wash border border-soft-lilac-edge text-soft-lilac-ink disabled:opacity-40">
          Start loop
        </button>
      </div>
      {(notice || writeSettlementLabel(settlement.write)) && (
        <p role="status" className="text-[12px] text-label-secondary">{writeSettlementLabel(settlement.write) || notice}</p>
      )}
      {loops === null && <p className="text-[12px] text-faint">Loading loops…</p>}
      {loops !== null && loops.length === 0 && <p className="text-[12px] text-muted">No autoresearch loops.</p>}
      {(loops ?? []).map((loop) => (
        <div key={loop.id} className="px-2.5 py-2 rounded-row bg-card border border-border text-[12px] space-y-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="font-code text-label-secondary">{loop.id.slice(0, 8)} · {loop.harness_id} · {loop.status} · {loop.current_iteration}/{loop.max_iterations}</span>
            <span className="flex items-center gap-1">
              <button type="button" disabled={busy !== null} onClick={() => void run('step', () => stepAutoresearchLoop(loop.id), 'Stepped.')} className="px-2 py-1 rounded-chip bg-info-wash border border-info-edge text-info-ink disabled:opacity-40">Step</button>
              {loop.status === 'running' ? (
                <button type="button" disabled={busy !== null} onClick={() => void run('pause', () => pauseAutoresearchLoop(loop.id), 'Paused.')} className="px-2 py-1 rounded-chip bg-warning-wash border border-warning-edge text-warning-ink disabled:opacity-40">Pause</button>
              ) : (
                <button type="button" disabled={busy !== null} onClick={() => void run('resume', () => resumeAutoresearchLoop(loop.id), 'Resumed.')} className="px-2 py-1 rounded-chip bg-success-wash border border-success-edge text-success-ink disabled:opacity-40">Resume</button>
              )}
              <button type="button" disabled={busy !== null} onClick={() => void run('stop', () => stopAutoresearchLoop(loop.id), 'Stopped.')} className="px-2 py-1 rounded-chip bg-danger-wash border border-danger-edge text-danger-ink disabled:opacity-40">Stop</button>
            </span>
          </div>
          <div className="text-faint font-code">champion {loop.champion_metric?.toFixed(6) ?? '—'} · depth {loop.champion_depth ?? '—'} · 2Q {loop.champion_2q_gates ?? '—'}{loop.stagnation_reason ? ` · ${loop.stagnation_reason}` : ''}</div>
        </div>
      ))}
    </div>
  );
}
