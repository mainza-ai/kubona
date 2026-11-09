'use client';

import React, { useState } from 'react';
import { Play } from 'lucide-react';
import { fetchJob, runExperiment } from '@/lib/api';
import type { Job } from '@/types';

type Kind = 'vqe' | 'pes' | 'qaoa' | 'spin_chain' | 'conjecture';

function num(r: Job | null, key: string): number | null {
  const result = r?.result_json as Record<string, unknown> | undefined;
  const value = result?.[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function resultRecord(job: Job | null): Record<string, unknown> | null {
  const value = job?.result_json;
  return value && typeof value === 'object' ? value as Record<string, unknown> : null;
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
  const [fullSpacePool, setFullSpacePool] = useState<'uccsd' | 'ceo'>('ceo');
  const [model, setModel] = useState('tfim');
  const [nSites, setNSites] = useState('4');
  const [coupling, setCoupling] = useState('1');
  const [field, setField] = useState('0.8');
  const [periodic, setPeriodic] = useState(true);
  const [targetError, setTargetError] = useState('0.001');
  const [conjValues, setConjValues] = useState('1, 1, 2, 3, 5, 8, 13, 21, 34');
  const [holdout, setHoldout] = useState('2');
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
    } else if (kind === 'spin_chain') {
      spec.model = model;
      spec.n_sites = Number(nSites);
      spec.coupling = Number(coupling);
      spec.field = Number(field);
      spec.periodic = periodic;
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
      spec.target_error = Number(targetError);
    } else if (kind === 'conjecture') {
      spec.values = conjValues.split(/[,\s]+/).filter(Boolean);
      spec.holdout = Number(holdout);
      spec.max_order = 6;
      spec.max_degree = 6;
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
  const exact = num(job, 'energy_exact');
  const uncertainty = num(job, 'uncertainty');
  const delta = num(job, 'delta_mha');
  const gap = num(job, 'gap');
  const cut = num(job, 'optimal_cut');
  const candidateCount = num(job, 'candidate_count');
  const result = resultRecord(job);
  const candidates = Array.isArray(result?.candidates)
    ? result.candidates as Array<{ law?: string; status?: string; predictions?: string[] }>
    : [];
  const claimable = result?.claimable === true;
  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-3" aria-label="Experiment launcher">
      <div>
        <span className="text-xs font-semibold text-white">Real experiment launcher</span>
        <p className="text-[12px] text-[var(--color-muted)]">Submits a ledger-bound VQE/PES/QAOA/spin-chain/conjecture job. The result below is read from the job record; no client-side simulation.</p>
      </div>
      <div className="grid grid-cols-2 gap-2 text-[12px]">
        <label className="space-y-1">Kind
          <select aria-label="Experiment kind" value={kind} onChange={(e) => setKind(e.target.value as Kind)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
            <option value="vqe">VQE</option><option value="pes">PES</option><option value="qaoa">QAOA MaxCut</option>
            <option value="spin_chain">Spin chain</option><option value="conjecture">Math conjecture</option>
          </select>
        </label>
        {kind === 'spin_chain' ? (
          <>
            <label className="space-y-1">Model
              <select aria-label="Spin model" value={model} onChange={(e) => setModel(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
                <option value="tfim">Transverse-field Ising</option>
                <option value="heisenberg">Heisenberg</option>
                <option value="xxz">XXZ</option>
              </select>
            </label>
            <label className="space-y-1">Sites
              <input aria-label="Spin sites" value={nSites} onChange={(e) => setNSites(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Spin coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Field h / anisotropy
              <input aria-label="Spin field" value={field} onChange={(e) => setField(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
              <input type="checkbox" aria-label="Periodic boundary" checked={periodic} onChange={(e) => setPeriodic(e.target.checked)} />
              Periodic boundary
            </label>
            <label className="space-y-1">Target error
              <input aria-label="Spin target error" value={targetError} onChange={(e) => setTargetError(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
          </>
        ) : kind === 'conjecture' ? (
          <>
            <label className="col-span-2 space-y-1">Exact sequence terms
              <textarea aria-label="Sequence terms" value={conjValues} onChange={(e) => setConjValues(e.target.value)} rows={2} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Holdout terms
              <input aria-label="Holdout terms" value={holdout} onChange={(e) => setHoldout(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
          </>
        ) : (
          <>
            <label className="space-y-1">Molecule
              <select aria-label="Experiment molecule" value={molecule} onChange={(e) => { const next = e.target.value; setMolecule(next); if (next === 'H2') setFullSpace(false); if (next === 'H2') { setRMin('0.6'); setRMax('0.9'); } else if (next === 'H2O') { setRMin('0.8'); setRMax('1.2'); } else { setRMin('1.2'); setRMax('2.0'); } }} disabled={kind === 'qaoa'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40">
                <option value="H2">H2</option><option value="LiH">LiH</option><option value="BeH2">BeH2</option><option value="H2O">H2O</option>
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
            <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
              <input type="checkbox" aria-label="Full orbital space" checked={fullSpace} disabled={kind === 'qaoa' || molecule === 'H2'} onChange={(e) => setFullSpace(e.target.checked)} />
              Full orbital space (LiH/BeH2/H2O; slower, refuses above the CPU qubit cap)
            </label>
            {fullSpace && (
              <label className="col-span-2 space-y-1">Full-space operator pool
                <select aria-label="Full-space operator pool" value={fullSpacePool} onChange={(e) => setFullSpacePool(e.target.value as 'uccsd' | 'ceo')} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
                  <option value="ceo">CEO-ADAPT-VQE · coupled exchange pool (validated)</option>
                  <option value="uccsd">ADAPT-VQE · UCCSD pool (validated reference)</option>
                </select>
              </label>
            )}
          </>
        )}
        <label className="space-y-1">Shots
          <input aria-label="Shots" value={shots} onChange={(e) => setShots(e.target.value)} disabled={kind === 'conjecture'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40" />
        </label>
        <label className="space-y-1">Layers
          <input aria-label="Layers" value={layers} onChange={(e) => setLayers(e.target.value)} disabled={kind === 'conjecture'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40" />
        </label>
        <label className="space-y-1">Max iterations
          <input aria-label="Max iterations" value={iterations} onChange={(e) => setIterations(e.target.value)} disabled={kind === 'conjecture'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40" />
        </label>
      </div>
      {error && <p role="alert" className="text-[12px] text-rose-300">{error}</p>}
      <button type="button" onClick={() => void submit()} disabled={busy || !sessionId} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
        <Play className="w-3.5 h-3.5" /> {busy ? 'Running…' : 'Run experiment'}
      </button>
      {job && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] font-code text-[var(--color-text)] space-y-0.5">
          <div>job {job.id.slice(0, 8)} · {job.status}</div>
          {job.status === 'completed' && kind === 'spin_chain' && energy != null && exact != null && (
            <div>spin E {energy.toFixed(6)} vs exact {exact.toFixed(6)}{uncertainty != null ? ` ± ${uncertainty.toFixed(6)}` : ''}{gap != null ? ` · gap ${gap.toFixed(4)}` : ''} · {claimable ? 'claimable' : 'not claimable'}</div>
          )}
          {job.status === 'completed' && kind === 'conjecture' && candidateCount != null && (
            <div>
              {candidateCount} candidate(s); not a theorem/claim.
              {candidates.slice(0, 2).map((c, i) => <div key={i}>{c.law}{c.status ? ` [${c.status}]` : ''}</div>)}
            </div>
          )}
          {job.status === 'completed' && kind !== 'spin_chain' && kind !== 'conjecture' && energy != null && <div>{kind === 'qaoa' ? `MaxCut ${energy.toFixed(4)}` : `${energy.toFixed(6)} Ha${uncertainty != null ? ` ± ${(uncertainty * 1000).toFixed(3)} mHa` : ''}${delta != null ? ` · Δ ${delta.toFixed(3)} mHa` : ''}`}</div>}
          {job.status === 'completed' && cut != null && <div>classical optimum {cut}</div>}
          {job.status === 'failed' && <div className="text-rose-300">failed — see job record</div>}
        </div>
      )}
    </div>
  );
}
