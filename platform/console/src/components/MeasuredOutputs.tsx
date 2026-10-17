'use client';

import React, { useEffect, useState } from 'react';
import { fetchJobs } from '@/lib/api';
import { JobProgress } from './common/JobProgress';
import type { Job } from '@/types';

function rec(v: unknown): Record<string, unknown> {
  return (v && typeof v === 'object' ? v : {}) as Record<string, unknown>;
}

function num(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

// One measured row per job type: latest COMPLETED job only. Every figure
// names the job it came from; energies never float unattributed.
function summarize(job: Job): string | null {
  const r = rec(job.result_json);
  const short = String(job.id).slice(0, 8);
  switch (job.job_type) {
    case 'circuit_run': {
      const counts = rec(r.counts);
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
    case 'krylov_solve': {
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
    case 'mitigate': {
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
      const measured = pts.filter((p) => num(rec(p).fci_energy) !== null);
      if (measured.length === 0) return null;
      let best = measured[0] as Record<string, unknown>;
      for (const p of measured) {
        const rec_p = rec(p);
        if ((num(rec_p.fci_energy) ?? Infinity) < (num(rec(best).fci_energy) ?? Infinity)) best = rec_p;
      }
      const bestR = num(rec(best).r);
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
    fetchJobs(sessionId).then((jobs) => {
      if (cancelled) return;
      setActiveIds(
        jobs
          .filter((j) => j.status === 'JOB_STATUS_QUEUED' || j.status === 'JOB_STATUS_RUNNING')
          .map((j) => j.id)
      );
      const latest = new Map<string, Job>();
      for (const j of jobs) {
        // R7: status is the gate, not timestamp presence — FAILED jobs
        // carry completed_at too and must never render as measured.
        if (j.status !== 'JOB_STATUS_COMPLETED' || !j.result_json) continue;
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
    });
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  if (!sessionId) return null;

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2">
      <div className="text-xs font-semibold text-white">Measured outputs</div>
      <p className="text-[11px] text-slate-500">
        Latest completed job per type — what ran, not what was planned. Notebook energies describe the plan.
      </p>
      {rows === null && <p className="text-[11px] font-code text-slate-500">Loading job outputs…</p>}
      {rows !== null && rows.length === 0 && (
        <p className="text-[11px] font-code text-slate-500">No completed jobs yet — measured outputs appear here.</p>
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
              <span className="text-[10px] font-code text-cyan-300 shrink-0">{r.type}</span>
              <span className="text-[11px] font-code text-slate-200 text-right">{r.text}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
