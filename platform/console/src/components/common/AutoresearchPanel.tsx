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

export function AutoresearchPanel({ sessionId }: { sessionId: string }) {
  const [harnesses, setHarnesses] = useState<AutoresearchHarness[]>([]);
  const [loops, setLoops] = useState<AutoresearchLoop[] | null>(null);
  const [harnessId, setHarnessId] = useState('');
  const [maxIterations, setMaxIterations] = useState('10');
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

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
      setLoops(l);
      setHarnessId((cur) => cur || h.harnesses[0]?.id || '');
    });
    return () => { live = false; };
  }, []);

  const run = async (kind: string, fn: () => Promise<unknown>, success: string) => {
    setBusy(kind);
    setNotice(null);
    try {
      const out = await fn();
      setNotice(out ? success : `${kind} refused or transport failed — nothing changed.`);
      await load();
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Autoresearch loops">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-xs font-semibold text-white">Autoresearch loops</span>
          <p className="text-[10px] text-slate-500">Sandboxed harness stepping; each transition re-reads server state.</p>
        </div>
        <button type="button" onClick={() => void load()} className="px-2 py-1 rounded-lg bg-white/5 border border-white/10 text-[11px] text-slate-300">Refresh</button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
        <select aria-label="Autoresearch harness" value={harnessId} onChange={(e) => setHarnessId(e.target.value)} className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-slate-200 font-mono">
          {harnesses.length === 0 && <option value="">no harnesses</option>}
          {harnesses.map((h) => <option key={h.id} value={h.id}>{h.id}</option>)}
        </select>
        <input aria-label="Max iterations" value={maxIterations} onChange={(e) => setMaxIterations(e.target.value)} inputMode="numeric" className="w-16 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-slate-200 font-mono" />
        <button type="button" disabled={busy !== null || !harnessId || !sessionId} onClick={() => {
          const n = Number(maxIterations);
          if (!Number.isInteger(n) || n < 1 || n > 1000) { setNotice('max iterations must be an integer 1..1000.'); return; }
          void run('start', () => startAutoresearchLoop(harnessId, n, undefined, sessionId), 'Loop started.');
        }} className="px-2 py-1 rounded-lg bg-indigo-500/15 border border-indigo-400/30 text-indigo-200 disabled:opacity-40">
          Start loop
        </button>
      </div>
      {notice && <p role="status" className="text-[11px] text-slate-300">{notice}</p>}
      {loops === null && <p className="text-[11px] text-slate-500">Loading loops…</p>}
      {loops !== null && loops.length === 0 && <p className="text-[11px] text-slate-400">No autoresearch loops.</p>}
      {(loops ?? []).map((loop) => (
        <div key={loop.id} className="px-2.5 py-2 rounded-xl bg-white/[0.03] border border-white/[0.06] text-[11px] space-y-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="font-code text-slate-300">{loop.id.slice(0, 8)} · {loop.harness_id} · {loop.status} · {loop.current_iteration}/{loop.max_iterations}</span>
            <span className="flex items-center gap-1">
              <button type="button" disabled={busy !== null} onClick={() => void run('step', () => stepAutoresearchLoop(loop.id), 'Stepped.')} className="px-2 py-1 rounded bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 disabled:opacity-40">Step</button>
              {loop.status === 'running' ? (
                <button type="button" disabled={busy !== null} onClick={() => void run('pause', () => pauseAutoresearchLoop(loop.id), 'Paused.')} className="px-2 py-1 rounded bg-amber-500/15 border border-amber-400/30 text-amber-200 disabled:opacity-40">Pause</button>
              ) : (
                <button type="button" disabled={busy !== null} onClick={() => void run('resume', () => resumeAutoresearchLoop(loop.id), 'Resumed.')} className="px-2 py-1 rounded bg-emerald-500/15 border border-emerald-400/30 text-emerald-200 disabled:opacity-40">Resume</button>
              )}
              <button type="button" disabled={busy !== null} onClick={() => void run('stop', () => stopAutoresearchLoop(loop.id), 'Stopped.')} className="px-2 py-1 rounded bg-rose-500/15 border border-rose-400/30 text-rose-200 disabled:opacity-40">Stop</button>
            </span>
          </div>
          <div className="text-slate-500 font-code">champion {loop.champion_metric?.toFixed(6) ?? '—'} · depth {loop.champion_depth ?? '—'} · 2Q {loop.champion_2q_gates ?? '—'}{loop.stagnation_reason ? ` · ${loop.stagnation_reason}` : ''}</div>
        </div>
      ))}
    </div>
  );
}
