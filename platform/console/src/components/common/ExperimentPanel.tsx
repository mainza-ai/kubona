'use client';

import React, { useState } from 'react';
import { Play } from 'lucide-react';
import { fetchJob, jobStatusLabel, runExperiment } from '@/lib/api';
import type { Job } from '@/types';
import { HardwareLaunchPanel } from './HardwareLaunchPanel';

type Kind = 'vqe' | 'geometry_optimization' | 'vqe_repeats' | 'pes' | 'qaoa' | 'excited_state' | 'reaction' | 'spin_chain' | 'thermodynamic_limit' | 'lattice_dynamics' | 'conjecture';

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
export function ExperimentPanel({ sessionId, tenantId }: { sessionId: string | null; tenantId?: string | null }) {
  const [kind, setKind] = useState<Kind>('vqe');
  const [molecule, setMolecule] = useState('H2');
  const [customAtom, setCustomAtom] = useState('H 0 0 0; H 0 0 0.7414');
  const [ncas, setNcas] = useState('2');
  const [nelecas, setNelecas] = useState('2');
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
  const [anisotropy, setAnisotropy] = useState('1');
  const [periodic, setPeriodic] = useState(true);
  const [thermoSizes, setThermoSizes] = useState('8, 12, 16');
  const [bondDim, setBondDim] = useState('32');
  const [sweeps, setSweeps] = useState('8');
  const [targetError, setTargetError] = useState('0.001');
  const [stateMode, setStateMode] = useState<'vqe' | 'exact'>('vqe');
  const [penalty, setPenalty] = useState('10');
  const [productDistance, setProductDistance] = useState('10');
  const [conjValues, setConjValues] = useState('1, 1, 2, 3, 5, 8, 13, 21, 34');
  const [holdout, setHoldout] = useState('2');
  const [repeats, setRepeats] = useState('3');
  const [latticeRows, setLatticeRows] = useState('2');
  const [latticeCols, setLatticeCols] = useState('2');
  const [initialField, setInitialField] = useState('1');
  const [quenchField, setQuenchField] = useState('0.2');
  const [latticeTimes, setLatticeTimes] = useState('0, 0.5, 1');
  const [busy, setBusy] = useState(false);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!sessionId) { setError('select a campaign first'); return; }
    setBusy(true); setError(null); setJob(null);
    const spec: Record<string, unknown> = { kind };
    if (kind === 'reaction') {
      spec.molecule = molecule;
      spec.reactant_distance = Number(bond);
      spec.product_distance = Number(productDistance);
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
    } else if (kind === 'geometry_optimization') {
      spec.molecule = molecule;
      if (molecule === 'custom') {
        spec.atom = customAtom.trim();
        spec.ncas = Number(ncas);
        spec.nelecas = Number(nelecas);
        spec.basis = 'sto-3g';
        spec.charge = 0;
        spec.spin = 0;
      } else {
        spec.initial_bond_distance = Number(bond);
      }
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
    } else if (kind === 'vqe_repeats') {
      spec.molecule = molecule;
      spec.bond_distance = Number(bond);
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
      spec.repeats = Number(repeats);
    } else if (kind === 'lattice_dynamics') {
      spec.rows = Number(latticeRows);
      spec.cols = Number(latticeCols);
      spec.coupling = Number(coupling);
      spec.initial_field = Number(initialField);
      spec.quench_field = Number(quenchField);
      spec.times = latticeTimes.split(/[,\s]+/).filter(Boolean).map(Number);
    } else if (kind === 'excited_state') {
      spec.molecule = molecule;
      spec.bond_distance = Number(bond);
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
      spec.penalty = Number(penalty);
    } else if (kind === 'qaoa') {
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
    } else if (kind === 'spin_chain') {
      spec.model = model;
      spec.n_sites = Number(nSites);
      spec.coupling = Number(coupling);
      spec.field = Number(field);
      spec.anisotropy = Number(anisotropy);
      spec.periodic = periodic;
      spec.layers = Number(layers);
      spec.shots = Number(shots);
      spec.max_iterations = Number(iterations);
      spec.target_error = Number(targetError);
      spec.state_mode = stateMode;
    } else if (kind === 'thermodynamic_limit') {
      spec.model = model;
      spec.sizes = thermoSizes.split(/[,\s]+/).filter(Boolean).map(Number);
      spec.coupling = Number(coupling);
      spec.field = Number(field);
      spec.anisotropy = Number(anisotropy);
      spec.bond_dim = Number(bondDim);
      spec.sweeps = Number(sweeps);
    } else if (kind === 'conjecture') {
      spec.values = conjValues.split(/[,\s]+/).filter(Boolean);
      spec.holdout = Number(holdout);
      spec.max_order = 6;
      spec.max_degree = 6;
    } else if (molecule === 'custom') {
      if (kind !== 'vqe') {
        setError('custom geometry is supported for VQE and geometry optimization only');
        setBusy(false);
        return;
      }
      spec.molecule = 'custom';
      spec.atom = customAtom.trim();
      spec.ncas = Number(ncas);
      spec.nelecas = Number(nelecas);
      spec.basis = 'sto-3g';
      spec.charge = 0;
      spec.spin = 0;
      spec.shots = Number(shots);
      spec.layers = Number(layers);
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
  const statusLabel = jobStatusLabel(job?.status);
  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-3" aria-label="Experiment launcher">
      <div>
        <span className="text-xs font-semibold text-white">Real experiment launcher</span>
        <p className="text-[12px] text-[var(--color-muted)]">Submits a ledger-bound VQE/PES/QAOA/spin-chain/conjecture job. The result below is read from the job record; no client-side simulation.</p>
      </div>
      <div className="grid grid-cols-2 gap-2 text-[12px]">
        <label className="space-y-1">Kind
          <select aria-label="Experiment kind" value={kind} onChange={(e) => setKind(e.target.value as Kind)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
            <option value="vqe">VQE</option><option value="geometry_optimization">Geometry optimization</option>
            <option value="vqe_repeats">Repeated VQE (variance)</option>
            <option value="pes">PES</option><option value="qaoa">QAOA MaxCut</option>
            <option value="lattice_dynamics">2D TFIM quench dynamics</option>
            <option value="excited_state">Excited state (H2/LiH/BeH2/H2O)</option>
            <option value="reaction">Reaction energy (H2/LiH/BeH2/H2O)</option>
            <option value="spin_chain">Spin chain</option><option value="thermodynamic_limit">DMRG / thermodynamic limit</option><option value="conjecture">Math conjecture</option>
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
            <label className="space-y-1">State mode
              <select aria-label="Spin state mode" value={stateMode} onChange={(e) => setStateMode(e.target.value as 'vqe' | 'exact')} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
                <option value="vqe">VQE (variational, may not reach ground)</option>
                <option value="exact">Exact state preparation + shot readout</option>
              </select>
            </label>
          </>
        ) : kind === 'lattice_dynamics' ? (
          <>
            <label className="space-y-1">Rows
              <input aria-label="Lattice rows" value={latticeRows} onChange={(e) => setLatticeRows(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Columns
              <input aria-label="Lattice columns" value={latticeCols} onChange={(e) => setLatticeCols(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Lattice coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Initial field
              <input aria-label="Initial field" value={initialField} onChange={(e) => setInitialField(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Quench field
              <input aria-label="Quench field" value={quenchField} onChange={(e) => setQuenchField(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="col-span-2 space-y-1">Times
              <input aria-label="Lattice times" value={latticeTimes} onChange={(e) => setLatticeTimes(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
          </>
        ) : kind === 'thermodynamic_limit' ? (
          <>
            <label className="space-y-1">Model
              <select aria-label="Thermodynamic model" value={model} onChange={(e) => setModel(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1">
                <option value="tfim">Transverse-field Ising</option>
                <option value="heisenberg">Heisenberg</option>
                <option value="xxz">XXZ (finite-size only)</option>
              </select>
            </label>
            <label className="space-y-1">Sizes
              <input aria-label="Thermodynamic sizes" value={thermoSizes} onChange={(e) => setThermoSizes(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Thermodynamic coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Field h
              <input aria-label="Thermodynamic field" value={field} onChange={(e) => setField(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            {model === 'xxz' && (
              <label className="space-y-1">Anisotropy Δ
                <input aria-label="Thermodynamic anisotropy" value={anisotropy} onChange={(e) => setAnisotropy(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
              </label>
            )}
            <label className="space-y-1">Bond dimension
              <input aria-label="DMRG bond dimension" value={bondDim} onChange={(e) => setBondDim(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <label className="space-y-1">Sweeps
              <input aria-label="DMRG sweeps" value={sweeps} onChange={(e) => setSweeps(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
            </label>
            <p className="col-span-2 text-[11px] text-[var(--color-muted)]">Open boundary; exact thermodynamic values only for TFIM and isotropic Heisenberg. No finite-size extrapolation is promoted to a limit.</p>
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
                <option value="custom">Custom PySCF geometry</option>
              </select>
            </label>
            {molecule !== 'custom' && (
              <label className="space-y-1">Bond distance (Å)
                <input aria-label="Bond distance" value={bond} onChange={(e) => setBond(e.target.value)} disabled={kind === 'qaoa'} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1 disabled:opacity-40" />
              </label>
            )}
            {molecule === 'custom' && (kind === 'vqe' || kind === 'geometry_optimization') && (
              <>
                <label className="col-span-2 space-y-1">PySCF atom geometry
                  <textarea aria-label="Custom atom geometry" value={customAtom} onChange={(e) => setCustomAtom(e.target.value)} rows={2} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
                </label>
                <label className="space-y-1">Active orbitals (ncas)
                  <input aria-label="Active orbitals" value={ncas} onChange={(e) => setNcas(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
                </label>
                <label className="space-y-1">Active electrons (nelecas)
                  <input aria-label="Active electrons" value={nelecas} onChange={(e) => setNelecas(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
                </label>
              </>
            )}
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
            {(kind === 'vqe' || kind === 'pes') && molecule !== 'custom' && (
              <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
                <input type="checkbox" aria-label="Full orbital space" checked={fullSpace} disabled={molecule === 'H2'} onChange={(e) => setFullSpace(e.target.checked)} />
                Full orbital space (LiH/BeH2/H2O; slower, refuses above the CPU qubit cap)
              </label>
            )}
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
        {kind === 'vqe_repeats' && (
          <label className="space-y-1">Repeats
            <input aria-label="VQE repeats" value={repeats} onChange={(e) => setRepeats(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
          </label>
        )}
        {kind === 'excited_state' && (
          <label className="space-y-1">Deflation penalty
            <input aria-label="Deflation penalty" value={penalty} onChange={(e) => setPenalty(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
          </label>
        )}
        {kind === 'reaction' && molecule === 'H2' && (
          <label className="space-y-1">Product distance (Å)
            <input aria-label="Product distance" value={productDistance} onChange={(e) => setProductDistance(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
          </label>
        )}
      </div>
      {error && <p role="alert" className="text-[12px] text-rose-300">{error}</p>}
      <button type="button" onClick={() => void submit()} disabled={busy || !sessionId} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
        <Play className="w-3.5 h-3.5" /> {busy ? 'Running…' : 'Run experiment'}
      </button>
      {job && statusLabel === 'COMPLETED' && ['spin_chain', 'vqe_run', 'qaoa_run', 'vqe_pes'].includes(job.job_type) && (
        <HardwareLaunchPanel sessionId={sessionId} tenantId={tenantId} sourceJob={job} />
      )}
      {job && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] font-code text-[var(--color-text)] space-y-0.5">
          <div>job {job.id.slice(0, 8)} · {job.status}</div>
          {statusLabel === 'COMPLETED' && kind === 'reaction' && result?.reaction_energy != null && (() => {
            const rxn = typeof result.reaction_energy === 'number' ? result.reaction_energy : null;
            const exactRxn = typeof result.reaction_energy_exact === 'number' ? result.reaction_energy_exact : null;
            return <div>reaction ΔE {rxn?.toFixed(6)} Ha vs exact {exactRxn?.toFixed(6)} · {result.claimable === true ? 'claimable' : 'not claimable'}</div>;
          })()}
          {statusLabel === 'COMPLETED' && kind === 'excited_state' && result?.ground_energy != null && (() => {
            const g = typeof result.ground_energy === 'number' ? result.ground_energy : null;
            const e = typeof result.excited_energy === 'number' ? result.excited_energy : null;
            const gap = typeof result.gap === 'number' ? result.gap : null;
            return <div>excited state: ground {g?.toFixed(6)} · excited {e?.toFixed(6)}{gap != null ? ` · gap ${gap.toFixed(5)}` : ''} · {result.claimable === true ? 'claimable' : 'not claimable'}</div>;
          })()}
          {statusLabel === 'COMPLETED' && kind === 'spin_chain' && energy != null && exact != null && (
            <div>spin E {energy.toFixed(6)} vs exact {exact.toFixed(6)}{uncertainty != null ? ` ± ${uncertainty.toFixed(6)}` : ''}{gap != null ? ` · gap ${gap.toFixed(4)}` : ''} · {claimable ? 'claimable' : 'not claimable'}{result?.state_mode === 'exact' ? ' · exact-state readout' : result?.ground_state_reached === false ? ' · VQE state not ground' : ''}</div>
          )}
          {statusLabel === 'COMPLETED' && kind === 'thermodynamic_limit' && (() => {
            const finite = Array.isArray(result?.finite_size) ? result.finite_size as Array<Record<string, unknown>> : [];
            const limit = result?.thermodynamic_limit && typeof result.thermodynamic_limit === 'object' ? result.thermodynamic_limit as Record<string, unknown> : null;
            const limitEnergy = typeof limit?.energy_per_site === 'number' ? limit.energy_per_site : null;
            return <div>{finite.length} sizes · DMRG bond dim {String(result?.bond_dim ?? '?')} · {limitEnergy != null ? `exact limit ${limitEnergy.toFixed(6)} / site` : 'no exact limit'} · exact classical baseline, not a measurement</div>;
          })()}
          {statusLabel === 'COMPLETED' && kind === 'conjecture' && candidateCount != null && (
            <div>
              {candidateCount} candidate(s) · {String(result?.novelty_status || 'unclassified')}; not a theorem/claim.
              {candidates.slice(0, 2).map((c, i) => <div key={i}>{c.law}{c.status ? ` [${c.status}]` : ''}</div>)}
            </div>
          )}
          {statusLabel === 'COMPLETED' && kind === 'geometry_optimization' && (result?.optimized_bond_distance != null || Array.isArray(result?.optimized_geometry)) && (() => {
            const geometry = Array.isArray(result?.optimized_geometry) ? result.optimized_geometry : [];
            const relaxation = result?.relaxation && typeof result.relaxation === 'object' ? result.relaxation as Record<string, unknown> : null;
            const where = result.optimized_bond_distance != null
              ? `optimized R ${Number(result.optimized_bond_distance).toFixed(6)} Å`
              : `${geometry.length} atoms relaxed`;
            const steps = typeof relaxation?.iterations === 'number' ? ` · BFGS ${relaxation.iterations} steps` : '';
            return <div>{where}{steps} · VQE {typeof result.energy === 'number' ? result.energy.toFixed(6) : '—'} Ha vs FCI {typeof result.fci_exact_energy === 'number' ? result.fci_exact_energy.toFixed(6) : '—'} · {result.claimable === true ? 'claimable' : 'not claimable'}</div>;
          })()}
          {statusLabel === 'COMPLETED' && kind === 'vqe_repeats' && result?.repeats != null && (
            <div>{String(result.repeats)} repeats · mean {typeof result.energy === 'number' ? result.energy.toFixed(6) : '—'} Ha · between-run SE {typeof result.between_run_standard_error === 'number' ? (result.between_run_standard_error * 1000).toFixed(3) : '—'} mHa · {result.claimable === true ? 'claimable' : 'not claimable'}</div>
          )}
          {statusLabel === 'COMPLETED' && kind === 'lattice_dynamics' && Array.isArray(result?.times) && result.times.length > 0 && (() => {
            const last = result.times[result.times.length - 1] as Record<string, unknown>;
            return <div>2D quench · {String(result.rows)}×{String(result.cols)} · final total magnetization {typeof last.total_magnetization === 'number' ? last.total_magnetization.toFixed(4) : '—'} · exact, not claimable</div>;
          })()}
          {statusLabel === 'COMPLETED' && kind !== 'spin_chain' && kind !== 'excited_state' && kind !== 'reaction' && kind !== 'conjecture' && kind !== 'geometry_optimization' && kind !== 'vqe_repeats' && kind !== 'lattice_dynamics' && energy != null && <div>{kind === 'qaoa' ? `MaxCut ${energy.toFixed(4)}` : `${energy.toFixed(6)} Ha${uncertainty != null ? ` ± ${(uncertainty * 1000).toFixed(3)} mHa` : ''}${delta != null ? ` · Δ ${delta.toFixed(3)} mHa` : ''}`}</div>}
          {statusLabel === 'COMPLETED' && cut != null && <div>classical optimum {cut}</div>}
          {statusLabel === 'FAILED' && <div className="text-rose-300">failed — see job record</div>}
        </div>
      )}
    </div>
  );
}
