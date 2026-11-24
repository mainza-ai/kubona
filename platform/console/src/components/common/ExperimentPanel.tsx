'use client';

import React, { useEffect, useRef, useState } from 'react';
import { Play } from 'lucide-react';
import { fetchJob, fetchJobs, jobStatusLabel, runExperiment } from '@/lib/api';
import { publishInvalidation } from '@/domain/invalidation';
import { Field, Select } from '@/components/ui/Forms';
import { Badge } from '@/components/ui/Badge';
import type { Job } from '@/types';
import { HardwareLaunchPanel } from './HardwareLaunchPanel';

const PROMOTABLE_HARDWARE_JOB_TYPES = ['spin_chain', 'vqe_run', 'qaoa_run', 'vqe_pes'];

type Kind = 'vqe' | 'geometry_optimization' | 'vqe_repeats' | 'pes' | 'qaoa' | 'excited_state' | 'reaction' | 'spin_chain' | 'thermodynamic_limit' | 'lattice_dynamics' | 'conjecture' | 'qec_code_eval' | 'qec_distance' | 'qec_memory_sim';

const QEC_HERON_R2_CAL = {
  backend: 'ibm_kingston',
  calibration_snapshot_id: 'cal_8f65e57d6f49',
  two_qubit_error: 0.0018203,
  single_qubit_error: 0.00024423,
  readout_error: 0.011475,
  t1_us: 267.172889,
  t2_us: 142.351605,
};

function parseTermPairs(raw: string): number[][] {
  return raw
    .split(';')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => part.split(/[,\s]+/).filter(Boolean).map((n) => Number(n)))
    .filter((pair) => pair.length === 2 && Number.isFinite(pair[0]) && Number.isFinite(pair[1]));
}

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
  const [qecPreset, setQecPreset] = useState<'bb_72' | 'gross_144' | 'surface_d3' | 'surface_d5' | 'custom_bb'>('bb_72');
  const [qecEll, setQecEll] = useState('6');
  const [qecM, setQecM] = useState('6');
  const [qecATerms, setQecATerms] = useState('3,0; 0,1; 0,2');
  const [qecBTerms, setQecBTerms] = useState('0,3; 1,0; 2,0');
  const [qecProveDistance, setQecProveDistance] = useState<'proved' | 'upper_bound'>('proved');
  const [qecNoiseModel, setQecNoiseModel] = useState<'si1000' | 'uniform_depolarizing' | 'heron_calibration'>('heron_calibration');
  const [qecPhysicalError, setQecPhysicalError] = useState('0.003');
  const [qecRounds, setQecRounds] = useState('3');
  const [qecShots, setQecShots] = useState('1000');
  const [qecDecoder, setQecDecoder] = useState<'auto' | 'pymatching' | 'bposd' | 'ebposd'>('auto');
  const [busy, setBusy] = useState(false);
  const [job, setJob] = useState<Job | null>(null);
  const [existingJobs, setExistingJobs] = useState<Job[]>([]);
  const [selectedParentId, setSelectedParentId] = useState<string>('');
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  useEffect(() => {
    if (!sessionId) return;
    let cancelled = false;
    void fetchJobs(sessionId).then((list) => {
      if (cancelled || !mountedRef.current) return;
      const promotable = list.filter(
        (j) =>
          jobStatusLabel(j.status) === 'COMPLETED' &&
          PROMOTABLE_HARDWARE_JOB_TYPES.includes(j.job_type),
      );
      setExistingJobs(promotable);
    });
    return () => {
      cancelled = true;
    };
  }, [sessionId]);
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
    } else if (kind === 'qec_code_eval' || kind === 'qec_distance') {
      spec.ell = Number(qecEll);
      spec.m = Number(qecM);
      spec.a_terms = parseTermPairs(qecATerms);
      spec.b_terms = parseTermPairs(qecBTerms);
      spec.code_name = qecPreset === 'bb_72' ? 'bravyi2024_bb_72_12_6' : qecPreset === 'gross_144' ? 'bravyi2024_gross_144_12_12' : `BB(${qecEll},${qecM})`;
      spec.prove_distance = kind === 'qec_distance' || qecProveDistance === 'proved';
      spec.time_limit_s = 30;
    } else if (kind === 'qec_memory_sim') {
      spec.noise_model = qecNoiseModel;
      spec.physical_error_rate = Number(qecPhysicalError);
      if (qecNoiseModel === 'heron_calibration') {
        spec.calibration = QEC_HERON_R2_CAL;
      }
      spec.rounds = Number(qecRounds);
      spec.shots = Number(qecShots);
      spec.decoder = qecDecoder;
      if (qecPreset === 'surface_d3' || qecPreset === 'surface_d5') {
        spec.family = 'surface';
        spec.distance = qecPreset === 'surface_d5' ? 5 : 3;
        spec.code_name = qecPreset;
      } else {
        spec.ell = Number(qecEll);
        spec.m = Number(qecM);
        spec.a_terms = parseTermPairs(qecATerms);
        spec.b_terms = parseTermPairs(qecBTerms);
        spec.code_name = qecPreset === 'bb_72' ? 'bravyi2024_bb_72_12_6' : qecPreset === 'gross_144' ? 'bravyi2024_gross_144_12_12' : `BB(${qecEll},${qecM})`;
      }
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
      if (!mountedRef.current) return;
      current = await fetchJob(current.id, sessionId);
      if (!mountedRef.current || !current) break;
      setJob(current);
      const st = jobStatusLabel(current.status).toUpperCase();
      if (st !== 'QUEUED' && st !== 'RUNNING') break;
    }
    if (!mountedRef.current) return;
    publishInvalidation(new Set(['campaign', 'jobs', 'obligations', 'loops', 'cost']));
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
    <div className="rounded-card border border-border-strong bg-surface p-4 space-y-3 text-text" aria-label="Experiment launcher">
      <div>
        <span className="text-label font-semibold text-text">Real experiment launcher</span>
        <p className="text-[12px] text-muted">Submits a ledger-bound VQE/PES/QAOA/spin-chain/conjecture job. The result below is read from the job record; no client-side simulation.</p>
      </div>
      <div className="grid grid-cols-2 gap-2 text-[12px]">
        <label className="space-y-1">Kind
          <select
            aria-label="Experiment kind"
            value={kind}
            onChange={(e) => {
              const nextKind = e.target.value as Kind;
              setKind(nextKind);
              if (molecule === 'custom' && nextKind !== 'vqe' && nextKind !== 'geometry_optimization') {
                setMolecule('H2');
              }
            }}
            className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text"
          >
            <option value="vqe">VQE</option><option value="geometry_optimization">Geometry optimization</option>
            <option value="vqe_repeats">Repeated VQE (variance)</option>
            <option value="pes">PES</option><option value="qaoa">QAOA MaxCut</option>
            <option value="lattice_dynamics">2D TFIM quench dynamics</option>
            <option value="excited_state">Excited state (H2/LiH/BeH2/H2O)</option>
            <option value="reaction">Reaction energy (H2/LiH/BeH2/H2O)</option>
            <option value="spin_chain">Spin chain</option><option value="thermodynamic_limit">DMRG / thermodynamic limit</option><option value="conjecture">Math conjecture</option>
            <option value="qec_code_eval">QEC code eval (BB / CSS)</option>
            <option value="qec_distance">QEC exact distance (HiGHS MILP)</option>
            <option value="qec_memory_sim">QEC memory simulation (Stim / Heron-r2)</option>
          </select>
        </label>
        {kind === 'qec_code_eval' || kind === 'qec_distance' || kind === 'qec_memory_sim' ? (
          <>
            <Select
              label="QEC preset"
              value={qecPreset}
              onChange={(e) => {
                const v = e.target.value as typeof qecPreset;
                setQecPreset(v);
                if (v === 'bb_72') {
                  setQecEll('6'); setQecM('6'); setQecATerms('3,0; 0,1; 0,2'); setQecBTerms('0,3; 1,0; 2,0');
                } else if (v === 'gross_144') {
                  setQecEll('12'); setQecM('6'); setQecATerms('3,0; 0,1; 0,2'); setQecBTerms('0,3; 1,0; 2,0');
                }
              }}
              options={[
                { value: 'bb_72', label: 'Bravyi 2024 BB [[72,12,6]]' },
                { value: 'gross_144', label: 'Gross BB [[144,12,12]]' },
                ...(kind === 'qec_memory_sim'
                  ? [
                      { value: 'surface_d3', label: 'Rotated Surface d=3 (baseline)' },
                      { value: 'surface_d5', label: 'Rotated Surface d=5 (baseline)' },
                    ]
                  : []),
                { value: 'custom_bb', label: 'Custom BB(ell, m)' },
              ]}
            />
            {qecPreset !== 'surface_d3' && qecPreset !== 'surface_d5' && (
              <>
                <Field label="ell" value={qecEll} onChange={(e) => setQecEll(e.target.value)} />
                <Field label="m" value={qecM} onChange={(e) => setQecM(e.target.value)} />
                <Field label="A terms (a,b; ...)" value={qecATerms} onChange={(e) => setQecATerms(e.target.value)} />
                <Field label="B terms (a,b; ...)" value={qecBTerms} onChange={(e) => setQecBTerms(e.target.value)} />
              </>
            )}
            {kind === 'qec_code_eval' && (
              <Select
                label="Distance mode"
                value={qecProveDistance}
                onChange={(e) => setQecProveDistance(e.target.value as 'proved' | 'upper_bound')}
                options={[
                  { value: 'proved', label: 'Exact HiGHS MILP (proved)' },
                  { value: 'upper_bound', label: 'Fast BP-OSD (upper_bound)' },
                ]}
              />
            )}
            {kind === 'qec_memory_sim' && (
              <>
                <Select
                  label="Noise model"
                  value={qecNoiseModel}
                  onChange={(e) => setQecNoiseModel(e.target.value as typeof qecNoiseModel)}
                  options={[
                    { value: 'heron_calibration', label: 'IBM Heron r2 (cal_8f65e57d6f49 · Held-Out)' },
                    { value: 'si1000', label: 'SI1000 superconducting' },
                    { value: 'uniform_depolarizing', label: 'Uniform depolarizing' },
                  ]}
                />
                {qecNoiseModel !== 'heron_calibration' && (
                  <Field label="Physical error p" value={qecPhysicalError} onChange={(e) => setQecPhysicalError(e.target.value)} />
                )}
                <Field label="Rounds" value={qecRounds} onChange={(e) => setQecRounds(e.target.value)} />
                <Field label="QEC shots" value={qecShots} onChange={(e) => setQecShots(e.target.value)} />
                <Select
                  label="Decoder"
                  value={qecDecoder}
                  onChange={(e) => setQecDecoder(e.target.value as typeof qecDecoder)}
                  options={[
                    { value: 'auto', label: 'Auto (PyMatching / BP-OSD)' },
                    { value: 'pymatching', label: 'PyMatching (MWPM)' },
                    { value: 'bposd', label: 'BP-OSD (32 iter)' },
                    { value: 'ebposd', label: 'EBP+OSD (5 iter)' },
                  ]}
                />
              </>
            )}
          </>
        ) : kind === 'spin_chain' ? (
          <>
            <label className="space-y-1">Model
              <select aria-label="Spin model" value={model} onChange={(e) => setModel(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text">
                <option value="tfim">Transverse-field Ising</option>
                <option value="heisenberg">Heisenberg</option>
                <option value="xxz">XXZ</option>
              </select>
            </label>
            <label className="space-y-1">Sites
              <input aria-label="Spin sites" value={nSites} onChange={(e) => setNSites(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Spin coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Field h
              <input aria-label="Spin field" value={field} onChange={(e) => setField(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            {model === 'xxz' && (
              <label className="space-y-1">Anisotropy Δ
                <input aria-label="Spin anisotropy" value={anisotropy} onChange={(e) => setAnisotropy(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
              </label>
            )}
            <label className="col-span-2 flex items-center gap-2 text-[12px] text-muted">
              <input type="checkbox" aria-label="Periodic boundary" checked={periodic} onChange={(e) => setPeriodic(e.target.checked)} />
              Periodic boundary
            </label>
            <label className="space-y-1">Target error
              <input aria-label="Spin target error" value={targetError} onChange={(e) => setTargetError(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">State mode
              <select aria-label="Spin state mode" value={stateMode} onChange={(e) => setStateMode(e.target.value as 'vqe' | 'exact')} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text">
                <option value="vqe">VQE (variational, may not reach ground)</option>
                <option value="exact">Exact state preparation + shot readout</option>
              </select>
            </label>
          </>
        ) : kind === 'lattice_dynamics' ? (
          <>
            <label className="space-y-1">Rows
              <input aria-label="Lattice rows" value={latticeRows} onChange={(e) => setLatticeRows(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Columns
              <input aria-label="Lattice columns" value={latticeCols} onChange={(e) => setLatticeCols(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Lattice coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Initial field
              <input aria-label="Initial field" value={initialField} onChange={(e) => setInitialField(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Quench field
              <input aria-label="Quench field" value={quenchField} onChange={(e) => setQuenchField(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="col-span-2 space-y-1">Times
              <input aria-label="Lattice times" value={latticeTimes} onChange={(e) => setLatticeTimes(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
          </>
        ) : kind === 'thermodynamic_limit' ? (
          <>
            <label className="space-y-1">Model
              <select aria-label="Thermodynamic model" value={model} onChange={(e) => setModel(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text">
                <option value="tfim">Transverse-field Ising</option>
                <option value="heisenberg">Heisenberg</option>
                <option value="xxz">XXZ (finite-size only)</option>
              </select>
            </label>
            <label className="space-y-1">Sizes
              <input aria-label="Thermodynamic sizes" value={thermoSizes} onChange={(e) => setThermoSizes(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Coupling J
              <input aria-label="Thermodynamic coupling" value={coupling} onChange={(e) => setCoupling(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Field h
              <input aria-label="Thermodynamic field" value={field} onChange={(e) => setField(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            {model === 'xxz' && (
              <label className="space-y-1">Anisotropy Δ
                <input aria-label="Thermodynamic anisotropy" value={anisotropy} onChange={(e) => setAnisotropy(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
              </label>
            )}
            <label className="space-y-1">Bond dimension
              <input aria-label="DMRG bond dimension" value={bondDim} onChange={(e) => setBondDim(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Sweeps
              <input aria-label="DMRG sweeps" value={sweeps} onChange={(e) => setSweeps(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <p className="col-span-2 text-[11px] text-muted">Open boundary; exact thermodynamic values only for TFIM and isotropic Heisenberg. No finite-size extrapolation is promoted to a limit.</p>
          </>
        ) : kind === 'conjecture' ? (
          <>
            <label className="col-span-2 space-y-1">Exact sequence terms
              <textarea aria-label="Sequence terms" value={conjValues} onChange={(e) => setConjValues(e.target.value)} rows={2} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
            <label className="space-y-1">Holdout terms
              <input aria-label="Holdout terms" value={holdout} onChange={(e) => setHoldout(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
            </label>
          </>
        ) : (
          <>
            <label className="space-y-1">Molecule
              <select aria-label="Experiment molecule" value={molecule} onChange={(e) => { const next = e.target.value; setMolecule(next); if (next === 'H2') setFullSpace(false); if (next === 'H2') { setRMin('0.6'); setRMax('0.9'); } else if (next === 'H2O') { setRMin('0.8'); setRMax('1.2'); } else { setRMin('1.2'); setRMax('2.0'); } }} disabled={kind === 'qaoa'} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text disabled:opacity-40">
                <option value="H2">H2</option><option value="LiH">LiH</option><option value="BeH2">BeH2</option><option value="H2O">H2O</option>
                {(kind === 'vqe' || kind === 'geometry_optimization') && (
                  <option value="custom">Custom PySCF geometry</option>
                )}
              </select>
            </label>
            {molecule !== 'custom' && (
              <label className="space-y-1">Bond distance (Å)
                <input aria-label="Bond distance" value={bond} onChange={(e) => setBond(e.target.value)} disabled={kind === 'qaoa'} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text disabled:opacity-40" />
              </label>
            )}
            {molecule === 'custom' && (kind === 'vqe' || kind === 'geometry_optimization') && (
              <>
                <label className="col-span-2 space-y-1">PySCF atom geometry
                  <textarea aria-label="Custom atom geometry" value={customAtom} onChange={(e) => setCustomAtom(e.target.value)} rows={2} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
                </label>
                <label className="space-y-1">Active orbitals (ncas)
                  <input aria-label="Active orbitals" value={ncas} onChange={(e) => setNcas(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
                </label>
                <label className="space-y-1">Active electrons (nelecas)
                  <input aria-label="Active electrons" value={nelecas} onChange={(e) => setNelecas(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
                </label>
              </>
            )}
            {kind === 'pes' && (
              <>
                <label className="space-y-1">PES r min (Å)
                  <input aria-label="PES r min" value={rMin} onChange={(e) => setRMin(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
                </label>
                <label className="space-y-1">PES r max (Å)
                  <input aria-label="PES r max" value={rMax} onChange={(e) => setRMax(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
                </label>
              </>
            )}
            {(kind === 'vqe' || kind === 'pes') && molecule !== 'custom' && (
              <label className="col-span-2 flex items-center gap-2 text-[12px] text-muted">
                <input type="checkbox" aria-label="Full orbital space" checked={fullSpace} disabled={molecule === 'H2'} onChange={(e) => setFullSpace(e.target.checked)} />
                Full orbital space (LiH/BeH2/H2O; slower, refuses above the CPU qubit cap)
              </label>
            )}
            {fullSpace && (
              <label className="col-span-2 space-y-1">Full-space operator pool
                <select aria-label="Full-space operator pool" value={fullSpacePool} onChange={(e) => setFullSpacePool(e.target.value as 'uccsd' | 'ceo')} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text">
                  <option value="ceo">CEO-ADAPT-VQE · coupled exchange pool (validated)</option>
                  <option value="uccsd">ADAPT-VQE · UCCSD pool (validated reference)</option>
                </select>
              </label>
            )}
          </>
        )}
        <label className="space-y-1">Shots
          <input aria-label="Shots" value={shots} onChange={(e) => setShots(e.target.value)} disabled={kind === 'conjecture' || kind === 'thermodynamic_limit' || kind === 'lattice_dynamics' || kind === 'qec_code_eval' || kind === 'qec_distance' || kind === 'qec_memory_sim'} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text disabled:opacity-40" />
        </label>
        <label className="space-y-1">Layers
          <input aria-label="Layers" value={layers} onChange={(e) => setLayers(e.target.value)} disabled={kind === 'conjecture' || kind === 'thermodynamic_limit' || kind === 'lattice_dynamics' || kind === 'qec_code_eval' || kind === 'qec_distance' || kind === 'qec_memory_sim'} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text disabled:opacity-40" />
        </label>
        <label className="space-y-1">Max iterations
          <input aria-label="Max iterations" value={iterations} onChange={(e) => setIterations(e.target.value)} disabled={kind === 'conjecture' || kind === 'thermodynamic_limit' || kind === 'lattice_dynamics' || kind === 'qec_code_eval' || kind === 'qec_distance' || kind === 'qec_memory_sim'} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text disabled:opacity-40" />
        </label>
        {kind === 'vqe_repeats' && (
          <label className="space-y-1">Repeats
            <input aria-label="VQE repeats" value={repeats} onChange={(e) => setRepeats(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
          </label>
        )}
        {kind === 'excited_state' && (
          <label className="space-y-1">Deflation penalty
            <input aria-label="Deflation penalty" value={penalty} onChange={(e) => setPenalty(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
          </label>
        )}
        {kind === 'reaction' && molecule === 'H2' && (
          <label className="space-y-1">Product distance (Å)
            <input aria-label="Product distance" value={productDistance} onChange={(e) => setProductDistance(e.target.value)} className="w-full rounded-chip bg-field border border-border px-2 py-1 text-text" />
          </label>
        )}
      </div>
      {error && <p role="alert" className="text-[12px] text-danger-ink">{error}</p>}
      <button type="button" onClick={() => void submit()} disabled={busy || !sessionId} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-control-compact bg-info-wash hover:bg-fill-hover border border-info-edge text-info-ink text-[12px] disabled:opacity-40">
        <Play className="w-3.5 h-3.5" /> {busy ? 'Running…' : 'Run experiment'}
      </button>
      {(() => {
        const justCompletedPromotable =
          job && statusLabel === 'COMPLETED' && PROMOTABLE_HARDWARE_JOB_TYPES.includes(job.job_type)
            ? job
            : null;
        const campaignPromotable = sessionId ? existingJobs : [];
        const selectedExisting =
          campaignPromotable.find((j) => j.id === selectedParentId) || campaignPromotable[0] || null;
        const hardwareSourceJob = justCompletedPromotable || selectedExisting;
        if (!hardwareSourceJob) return null;
        return (
          <div className="space-y-2">
            {!justCompletedPromotable && campaignPromotable.length > 0 && (
              <Select
                label="Promote completed simulator job to QPU"
                aria-label="Parent simulator job for QPU promotion"
                value={hardwareSourceJob.id}
                onChange={(e) => setSelectedParentId(e.target.value)}
                options={campaignPromotable.map((pj) => ({
                  value: pj.id,
                  label: `${pj.job_type} · ${pj.id.slice(0, 8)} (COMPLETED)`,
                }))}
              />
            )}
            <HardwareLaunchPanel sessionId={sessionId} tenantId={tenantId} sourceJob={hardwareSourceJob} />
          </div>
        );
      })()}
      {job && (
        <div className="rounded-row bg-card border border-border p-2 text-[12px] font-code text-text space-y-0.5">
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
          {statusLabel === 'COMPLETED' && (kind === 'qec_code_eval' || kind === 'qec_distance') && result?.n != null && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span>[[{String(result.n)}, {String(result.k)}, {String(result.d ?? '?')}]]</span>
              <Badge tone={result.d_kind === 'proved' ? 'success' : 'warning'}>
                {result.d_kind === 'proved' ? 'PROVED' : 'UPPER BOUND'}
              </Badge>
              {typeof result.fom === 'number' && <span>· FOM {result.fom.toFixed(2)}</span>}
              <span>· {result.decomposable ? 'decomposable' : 'indecomposable'}</span>
              {result.catalog_known === true && <span>· catalog: {String(result.catalog_match ?? 'known')}</span>}
            </div>
          )}
          {statusLabel === 'COMPLETED' && kind === 'qec_memory_sim' && typeof result?.ler === 'number' && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span>LER {result.ler.toFixed(4)}</span>
              {typeof result.ci_low === 'number' && typeof result.ci_high === 'number' && (
                <span>[{result.ci_low.toFixed(4)}, {result.ci_high.toFixed(4)}]</span>
              )}
              {result.d_circ != null && <span>· d_circ {String(result.d_circ)}</span>}
              {typeof result.decoder_latency_us === 'number' && <span>· {result.decoder_latency_us.toFixed(1)} µs/shot</span>}
              <Badge tone="neutral">{String(result.decoder ?? 'auto')}</Badge>
            </div>
          )}
          {statusLabel === 'COMPLETED' && kind !== 'spin_chain' && kind !== 'excited_state' && kind !== 'reaction' && kind !== 'conjecture' && kind !== 'geometry_optimization' && kind !== 'vqe_repeats' && kind !== 'lattice_dynamics' && kind !== 'qec_code_eval' && kind !== 'qec_distance' && kind !== 'qec_memory_sim' && energy != null && <div>{kind === 'qaoa' ? `MaxCut ${energy.toFixed(4)}` : `${energy.toFixed(6)} Ha${uncertainty != null ? ` ± ${(uncertainty * 1000).toFixed(3)} mHa` : ''}${delta != null ? ` · Δ ${delta.toFixed(3)} mHa` : ''}`}</div>}
          {statusLabel === 'COMPLETED' && cut != null && <div>classical optimum {cut}</div>}
          {statusLabel === 'FAILED' && <div className="text-danger-ink">failed — see job record</div>}
        </div>
      )}
    </div>
  );
}
