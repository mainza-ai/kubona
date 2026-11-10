'use client';

import React, { useEffect, useState } from 'react';
import { fetchJobs, jobStatusLabel } from '@/lib/api';
import { asRecord } from '@/lib/guards';
import { JobProgress } from './common/JobProgress';
import { ProvenanceBadge } from './common/ProvenanceBadge';
import type { Job } from '@/types';

function num(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

// One measured row per job type: latest COMPLETED job only. Every figure
// names the job it came from; energies never float unattributed.
function summarize(job: Job): string | null {
  const r = asRecord(job.result_json);
  if (r.success === false) return null;
  const short = String(job.id).slice(0, 8);
  switch (job.job_type) {
    case 'circuit_run': {
      const counts = asRecord(r.counts);
      const cut = num(r.measured_cut);
      if (r.qaoa_meta && cut !== null) {
        const ratio = num(r.approximation_ratio);
        return `QAOA hardware cut ${cut.toFixed(4)}${ratio !== null ? ` · ratio ${ratio.toFixed(4)}` : ''} · ${str(r.resolved_backend) || 'hardware'} · job ${short}`;
      }
      const shots = Object.values(counts).reduce<number>(
        (a, v) => a + (typeof v === 'number' ? v : 0),
        0
      );
      const backend = str(r.resolved_backend) || 'unknown backend';
      const compiler = job.compiler_used
        ? ` · compiled ${job.compiler_used}${job.fallback_estimate ? ' (estimate)' : ''}`
        : '';
      return `counts on ${backend} · ${shots} shots · job ${short}${compiler}`;
    }
    case 'hardware_experiment': {
      const e = num(r.energy);
      if (e === null) return null;
      const unc = num(r.uncertainty);
      const delta = num(r.delta_mha);
      const fci = num(r.fci_exact_energy);
      return `hardware VQE ${e.toFixed(6)} Ha${unc !== null ? ` ± ${(unc * 1000).toFixed(3)} mHa` : ''}${fci !== null ? ` vs FCI ${fci.toFixed(6)}` : ''}${delta !== null ? ` · Δ ${delta.toFixed(3)} mHa` : ''} · ${r.simulated_optimizer === false ? 'hardware-in-the-loop optimizer' : 'local optimizer'}${r.calibration_fresh === false ? ' · calibration STALE' : ''} · job ${short}`;
    }
    case 'qaoa_hardware': {
      const cut = num(r.measured_cut) ?? num(r.energy);
      if (cut === null) return null;
      const ratio = num(r.approximation_ratio);
      return `hardware QAOA cut ${cut.toFixed(4)}${ratio !== null ? ` · ratio ${ratio.toFixed(4)}` : ''} · ${r.simulated_optimizer === false ? 'hardware-in-the-loop optimizer' : 'local optimizer'} · job ${short}`;
    }
    case 'geometry_optimization': {
      const e = num(r.energy);
      const exact = num(r.fci_exact_energy);
      const rOpt = num(r.optimized_bond_distance);
      if (e === null || exact === null) return null;
      return `geometry optimized${rOpt !== null ? ` at R=${rOpt.toFixed(5)} Å` : ''} · VQE ${e.toFixed(6)} Ha vs FCI ${exact.toFixed(6)} · ${r.claimable === true ? 'claimable' : 'not claimable'} · job ${short}`;
    }
    case 'vqe_repeats': {
      const e = num(r.energy);
      const se = num(r.between_run_standard_error);
      if (e === null) return null;
      return `repeated VQE ${String(r.repeats ?? '?')} runs · mean ${e.toFixed(6)} Ha${se !== null ? ` · between-run SE ${(se * 1000).toFixed(3)} mHa` : ''} · ${r.claimable === true ? 'claimable' : 'not claimable'} · job ${short}`;
    }
    case 'thermodynamic_limit': {
      const finite = Array.isArray(r.finite_size) ? r.finite_size : [];
      const limit = r.thermodynamic_limit && typeof r.thermodynamic_limit === 'object' ? r.thermodynamic_limit as Record<string, unknown> : null;
      const limitEnergy = num(limit?.energy_per_site);
      if (finite.length === 0) return null;
      return `DMRG ${finite.length} sizes${limitEnergy !== null ? ` · exact limit ${limitEnergy.toFixed(6)} / site` : ' · no exact limit'} · classical baseline · job ${short}`;
    }
    case 'lattice_dynamics': {
      const times = Array.isArray(r.times) ? r.times : [];
      if (times.length === 0) return null;
      const last = asRecord(times[times.length - 1]);
      const mag = num(last.total_magnetization);
      return `2D TFIM quench ${String(r.rows)}×${String(r.cols)} · ${times.length} times${mag !== null ? ` · final total Mz ${mag.toFixed(4)}` : ''} · exact, not claimable · job ${short}`;
    }
    case 'spin_chain': {
      const e = num(r.energy);
      const exact = num(r.energy_exact);
      if (e === null || exact === null) return null;
      const unc = num(r.uncertainty);
      const gap = num(r.gap);
      return `spin chain ${str(r.model)} n=${num(r.n_sites) ?? '?'} E ${e.toFixed(6)} vs exact ${exact.toFixed(6)}${unc !== null ? ` ± ${unc.toFixed(6)}` : ''}${gap !== null ? ` · gap ${gap.toFixed(4)}` : ''} · ${r.claimable === true ? 'claimable' : 'not claimable'}${r.state_mode === 'exact' ? ' · exact-state readout' : r.ground_state_reached === false ? ' · VQE state not ground' : ''} · job ${short}`;
    }
    case 'reaction': {
      const e = num(r.reaction_energy);
      const exact = num(r.reaction_energy_exact);
      if (e === null || exact === null) return null;
      const unc = num(r.uncertainty);
      return `reaction ${str(r.reaction) || 'H2 -> 2 H'} ΔE ${e.toFixed(6)} Ha vs exact ${exact.toFixed(6)}${unc !== null ? ` ± ${unc.toFixed(6)}` : ''} · ${r.claimable === true ? 'claimable' : 'not claimable'} · job ${short}`;
    }
    case 'excited_state': {
      const g = num(r.ground_energy);
      const e = num(r.excited_energy);
      const gap = num(r.gap);
      if (g === null || e === null) return null;
      const gapExact = num(r.gap_exact);
      return `excited state ground ${g.toFixed(6)} · excited ${e.toFixed(6)}${gap !== null ? ` · gap ${gap.toFixed(5)}` : ''}${gapExact !== null ? ` vs exact ${gapExact.toFixed(5)}` : ''} · ${r.claimable === true ? 'claimable' : 'not claimable'} · job ${short}`;
    }
    case 'spin_chain_hardware': {
      const e = num(r.energy);
      const exact = num(r.energy_exact);
      if (e === null || exact === null) return null;
      const unc = num(r.uncertainty);
      return `hardware spin chain ${str(r.model)} n=${num(r.n_sites) ?? '?'} E ${e.toFixed(6)} vs exact ${exact.toFixed(6)}${unc !== null ? ` ± ${unc.toFixed(6)}` : ''} · ${r.claimable === true ? 'claimable' : 'not claimable'}${r.calibration_fresh === false ? ' · calibration STALE' : ''} · ${str(r.backend) || 'hardware'} · job ${short}`;
    }
    case 'conjecture_run': {
      const count = num(r.candidate_count);
      if (count === null) return null;
      const candidates = Array.isArray(r.candidates) ? r.candidates : [];
      const first = asRecord(candidates[0]);
      const law = str(first.law);
      return `math conjecture ${count} candidate(s)${law ? ` · ${law}` : ''} · ${str(r.novelty_status) || 'unclassified'} · finite-scope, not a theorem · job ${short}`;
    }
    case 'qaoa_run':
    case 'qaoa_hardware': {
      const ratio = num(r.approximation_ratio);
      const cut = num(r.energy) ?? num(r.measured_cut);
      const optimal = num(r.optimal_cut);
      if (cut === null) return null;
      return `QAOA MaxCut ${cut.toFixed(3)}${optimal !== null ? ` vs classical ${optimal}` : ''}${ratio !== null ? ` · ratio ${ratio.toFixed(4)}` : ''} · job ${short}`;
    }
    case 'vqe_run': {
      const e = num(r.energy);
      const fci = num(r.fci_exact_energy);
      const delta = num(r.delta_mha);
      if (e === null) return null;
      const unc = num(r.uncertainty);
      const fullSpace = str(r.full_space_status);
      return `VQE ${e.toFixed(6)} Ha${unc !== null ? ` ± ${(unc * 1000).toFixed(3)} mHa` : ''}${fci !== null ? ` vs FCI ${fci.toFixed(6)} Ha` : ''}${delta !== null ? ` · Δ ${delta.toFixed(3)} mHa` : ''}${fullSpace && fullSpace !== 'within_target' ? ` · full-space ${fullSpace.replace(/_/g, ' ')}` : ''} · ${str(r.backend) || 'simulator'} · job ${short}`;
    }
    case 'vqe_pes':
    case 'hardware_pes': {
      const pts = Array.isArray(r.points) ? r.points : [];
      if (pts.length === 0) return null;
      let min = asRecord(pts[0]);
      let closest = asRecord(pts[0]);
      for (const p of pts) {
        const rec = asRecord(p);
        if ((num(rec.energy) ?? Infinity) < (num(min.energy) ?? Infinity)) min = rec;
        if ((num(rec.delta_mha) ?? Infinity) < (num(closest.delta_mha) ?? Infinity)) closest = rec;
      }
      const minR = num(min.bond_distance);
      const closeR = num(closest.bond_distance);
      return `VQE PES ${pts.length} points${minR !== null ? ` · min E R=${minR.toFixed(4)} Å` : ''}${closeR !== null ? ` · closest R=${closeR.toFixed(4)} Å` : ''} · job ${short}`;
    }
    case 'krylov_solve':
    case 'krylov_json': {
      const e = num(r.ground_state_energy);
      if (e === null) return null;
      return `ground state ${e.toFixed(5)} Ha (classical emulation) · job ${short}`;
    }
    case 'classical_anchor': {
      const fci = num(r.fci_exact_energy);
      const mol = str(r.molecule) || 'molecule';
      if (fci === null) return null;
      return `${mol} FCI ${fci.toFixed(5)} Ha · job ${short}`;
    }
    case 'claim_eval': {
      const pass = r.passes_chemical_accuracy === true;
      const delta = num(r.delta_mha);
      return `claim ${pass ? 'PASSED' : 'FAILED'}${delta !== null ? ` · Δ ${delta.toFixed(2)} mHa` : ''} · job ${short}`;
    }
    case 'mitigate':
    case 'mitigate_json': {
      const mit = num(r.mitigated_value);
      const method = str(r.method_used) || 'mitigation';
      if (mit === null) return null;
      return `${method}: mitigated ${mit.toFixed(6)} · job ${short}`;
    }
    case 'circuit_cut': {
      const frags = Array.isArray(r.fragments) ? r.fragments.length : 0;
      return `cut into ${frags} fragments · job ${short}`;
    }
    case 'pes_sweep': {
      const pts = Array.isArray(r.points) ? r.points : [];
      const measured = pts.filter((p) => num(asRecord(p).fci_energy) !== null);
      if (measured.length === 0) return null;
      let best = asRecord(measured[0]);
      for (const p of measured) {
        const rec_p = asRecord(p);
        if ((num(rec_p.fci_energy) ?? Infinity) < (num(best.fci_energy) ?? Infinity)) best = rec_p;
      }
      const bestR = num(best.r);
      return `${measured.length}/${pts.length} geometries · coarse min${bestR !== null ? ` near R=${bestR.toFixed(4)} Å` : ''} · job ${short}`;
    }
    default:
      return null;
  }
}

// MeasuredOutputs: the job-result wiring. Notebook energies describe the
// plan; this table describes what actually ran, latest completed job per
// type, each figure bound to its job id. Empty sessions say so.
export function MeasuredOutputs({ sessionId }: { sessionId: string | null }) {
  const [rows, setRows] = useState<Array<{ type: string; text: string; at: string }> | null>(null);
  // S8b/c: in-flight jobs render live progress (JobProgress polls the
  // record); completed jobs keep the measured table below. Active IDs
  // derive from the same fetch — one source, no second listing.
  const [activeIds, setActiveIds] = useState<string[] | null>(null);

  // Reset during render on session change (same key-change pattern):
  // measured rows belong to exactly one session — stale rows must not
  // render while the new session's jobs load.
  const [prevSessionId, setPrevSessionId] = useState<string | null>(sessionId);
  if (prevSessionId !== sessionId) {
    setPrevSessionId(sessionId);
    setRows(null);
    setActiveIds(null);
  }

  useEffect(() => {
    if (!sessionId) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const load = async () => {
      if (cancelled) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        timer = setTimeout(load, 15000);
        return;
      }
      const jobs = await fetchJobs(sessionId);
      if (cancelled) return;
      setActiveIds(
        jobs
          .filter((j) => {
            const st = jobStatusLabel(j.status);
            return st === 'QUEUED' || st === 'RUNNING';
          })
          .map((j) => j.id)
      );
      const latest = new Map<string, Job>();
      for (const j of jobs) {
        // R7: status is the gate, not timestamp presence — FAILED jobs
        // carry completed_at too and must never render as measured.
        if (jobStatusLabel(j.status) !== 'COMPLETED' || !j.result_json) continue;
        const prev = latest.get(j.job_type);
        if (!prev || (prev.completed_at ?? '') < (j.completed_at ?? '')) latest.set(j.job_type, j);
      }
      const out: Array<{ type: string; text: string; at: string }> = [];
      for (const [type, job] of latest) {
        const text = summarize(job);
        if (text) out.push({ type, text, at: job.completed_at ?? '' });
      }
      out.sort((a, b) => a.type.localeCompare(b.type));
      setRows(out);
      timer = setTimeout(load, 5000);
    };
    void load();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [sessionId]);

  if (!sessionId) return null;

  return (
    <div data-provenance="measured" className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2">
      <div className="flex items-center gap-2">
        <div className="text-xs font-semibold text-white">Measured outputs</div>
        <ProvenanceBadge kind="measured" detail="Latest completed job per type." />
      </div>
      <p className="text-[12px] text-slate-500">
        Latest completed job per type — what ran, not what was planned. Notebook energies describe the plan.
      </p>
      {rows === null && <p className="text-[12px] font-code text-slate-500">Loading job outputs…</p>}
      {rows !== null && rows.length === 0 && (
        <p className="text-[12px] font-code text-slate-500">No completed jobs yet — measured outputs appear here.</p>
      )}
      {activeIds !== null && activeIds.length > 0 && (
        <div className="space-y-1.5">
          {activeIds.map((id) => (
            <JobProgress key={id} jobId={id} sessionId={sessionId ?? ''} />
          ))}
        </div>
      )}
      {rows !== null && rows.length > 0 && (
        <ul className="space-y-1.5">
          {rows.map((r) => (
            <li
              key={r.type}
              className="flex items-baseline justify-between gap-3 rounded-lg bg-white/[0.03] border border-white/[0.06] px-2.5 py-1.5"
            >
              <span className="text-[12px] font-code text-cyan-300 shrink-0">{r.type}</span>
              <span data-metric className="text-[12px] font-code text-slate-200 text-right">{r.text}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
