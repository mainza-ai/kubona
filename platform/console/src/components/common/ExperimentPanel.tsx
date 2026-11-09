'use client';

import React, { useState } from 'react';
import { Play } from 'lucide-react';
import { fetchJob, runExperiment } from '@/lib/api';
import type { Job } from '@/types';

type Kind = 'vqe' | 'pes' | 'qaoa';

function num(r: Job | null, key: string): number | null {
  const result = r?.result_json as Record<string, unknown> | undefined;
  const value = result?.[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

/** Human submission surface for the real experiment rail. */
export function ExperimentPanel({ sessionId }: { sessionId: string | null }) {
  const [kind, setKind] = useState<Kind>('vqe');
  const [molecule, setMolecule] = useState('H2');
  const [bond, setBond] = useState('0.7414');
  const [rMin, setRMin] = useState('0.6');
  const [rMax, setRMax] = useState('0.9');
  const [shots, setShots] = useState('16384');
  const [layers, setLayers] = useState('2');
  const [iterations, setIterations] = useState('80');
  const [fullSpace, setFullSpace] = useState(false);
  const [fullSpacePool, setFullSpacePool] = useState<'uccsd' | 'ceo'>('uccsd');
  const [busy, setBusy] = useState(false);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!sessionId) { setError('select a campaign first'); return; }
    setBusy(true); setError(null); setJob(null);
    const spec: Record<string, unknown> = { kind };
    if (kind === 'qaoa') {
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
    } else {
      spec.molecule = molecule;
      spec.bond_distance = Number(bond);
      spec.shots = Number(shots);
      spec.layers = Number(layers);
      spec.max_iterations = Number(iterations);
      spec.full_space = fullSpace;
      if (fullSpace) spec.full_space_pool = fullSpacePool;
      if (kind === 'pes') {
        spec.r_min = Number(rMin); spec.r_max = Number(rMax); spec.steps = 5;
      }
    }
    const out = await runExperiment(sessionId, spec, `ui-experiment-${Date.now()}`);
    if (!out.ok || !out.job) {
      setError(out.error || 'experiment refused');
      setBusy(false);
      return;
    }
    let current: Job | null = out.job;
    setJob(current);
    for (let i = 0; i < 180; i++) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      current = await fetchJob(current.id, sessionId);
      if (!current) break;
      setJob(current);
      if (current.status !== 'queued' && current.status !== 'running') break;
    }
    setBusy(false);
  };

  const energy = num(job, 'energy');
  const uncertainty = num(job, 'uncertainty');
  const delta = num(job, 'delta_mha');
  const cut = num(job, 'optimal_cut');
  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-3" aria-label="Experiment launcher">
      <div>
        <span className="text-xs font-semibold text-white">Real experiment launcher</span>
        <p className="text-[12px] text-[var(--color-muted)]">Submits a ledger-bound VQE/PES/QAOA job. The result below is read from the job record; no client-side simulation.</p>
      </div>
      <div className="grid grid-cols-2 gap-2 text-[12px]">
        <label className="space-y-1">Kind
          <select aria-label="Experiment kind" value={kind} onChange={(e) => setKind(e.target.value as Kind)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
            <option value="vqe">VQE</option><option value="pes">PES</option><option value="qaoa">QAOA MaxCut</option>
          </select>
        </label>
        <label className="space-y-1">Molecule
          <select aria-label="Experiment molecule" value={molecule} onChange={(e) => { const next = e.target.value; setMolecule(next); if (next === 'H2') setFullSpace(false); if (next === 'H2') { setRMin('0.6'); setRMax('0.9'); } else { setRMin('1.2'); setRMax('2.0'); } }} disabled={kind === 'qaoa'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40">
            <option value="H2">H2</option><option value="LiH">LiH</option><option value="BeH2">BeH2</option>
          </select>
        </label>
        <label className="space-y-1">Bond distance (Å)
          <input aria-label="Bond distance" value={bond} onChange={(e) => setBond(e.target.value)} disabled={kind === 'qaoa'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40" />
        </label>
        {kind === 'pes' && (
          <>
            <label className="space-y-1">PES r min (Å)
              <input aria-label="PES r min" value={rMin} onChange={(e) => setRMin(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">PES r max (Å)
              <input aria-label="PES r max" value={rMax} onChange={(e) => setRMax(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
          </>
        )}
        <label className="space-y-1">Shots
          <input aria-label="Shots" value={shots} onChange={(e) => setShots(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="space-y-1">Layers
          <input aria-label="Layers" value={layers} onChange={(e) => setLayers(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="space-y-1">Max iterations
          <input aria-label="Max iterations" value={iterations} onChange={(e) => setIterations(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
          <input type="checkbox" aria-label="Full orbital space" checked={fullSpace} disabled={kind === 'qaoa' || molecule === 'H2'} onChange={(e) => setFullSpace(e.target.checked)} />
          Full orbital space (LiH/BeH2; slower, reports full-FCI gap honestly)
        </label>
        {fullSpace && (
          <label className="col-span-2 space-y-1">Full-space operator pool
            <select aria-label="Full-space operator pool" value={fullSpacePool} onChange={(e) => setFullSpacePool(e.target.value as 'uccsd' | 'ceo')} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
              <option value="uccsd">ADAPT-VQE · UCCSD pool (validated)</option>
              <option value="ceo">CEO-ADAPT-VQE · coupled exchange pool (experimental)</option>
            </select>
          </label>
        )}
      </div>
      {error && <p role="alert" className="text-[12px] text-rose-300">{error}</p>}
      <button type="button" onClick={() => void submit()} disabled={busy || !sessionId} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
        <Play className="w-3.5 h-3.5" /> {busy ? 'Running…' : 'Run experiment'}
      </button>
      {job && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] font-code text-[var(--color-text)] space-y-0.5">
          <div>job {job.id.slice(0, 8)} · {job.status}</div>
          {job.status === 'completed' && energy != null && <div>{kind === 'qaoa' ? `MaxCut ${energy.toFixed(4)}` : `${energy.toFixed(6)} Ha${uncertainty != null ? ` ± ${(uncertainty * 1000).toFixed(3)} mHa` : ''}${delta != null ? ` · Δ ${delta.toFixed(3)} mHa` : ''}`}</div>}
          {job.status === 'completed' && cut != null && <div>classical optimum {cut}</div>}
          {job.status === 'failed' && <div className="text-rose-300">failed — see job record</div>}
        </div>
      )}
    </div>
  );
}
