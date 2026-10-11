'use client';

import React, { useState, useEffect } from 'react';
import { fetchJob, jobStatusLabel } from '@/lib/api';
import type { Job } from '@/types';

function asRecord(v: unknown): Record<string, unknown> {
  if (v && typeof v === 'object') return v as Record<string, unknown>;
  if (typeof v === 'string') {
    try {
      const parsed: unknown = JSON.parse(v);
      if (parsed && typeof parsed === 'object') return parsed as Record<string, unknown>;
    } catch {
      /* fall through to empty */
    }
  }
  return {};
}

function kv(label: string, value: string) {
  return (
    <div>
      <span className="text-white/40 block text-[10px]">{label}</span>
      <span className="font-code text-slate-200 break-all">{value}</span>
    </div>
  );
}

/**
 * Job detail: full params + result payloads for one job id, fetched live.
 * Nothing here is invented: absent result → "no result recorded yet";
 * unknown id → refusal notice. Counts render when the executor recorded them.
 */
export function JobDetailWidget({ jobId }: { jobId: string }) {
  const [job, setJob] = useState<Job | null>(null);
  const [missing, setMissing] = useState(false);

  useEffect(() => {
    let mounted = true;
    fetchJob(jobId).then((j) => {
      if (!mounted) return;
      if (j) setJob(j);
      else setMissing(true);
    });
    return () => {
      mounted = false;
    };
  }, [jobId]);

  if (missing) {
    return (
      <div className="rounded-2xl border border-white/10 bg-black/50 p-4 text-xs text-slate-400 font-code">
        Job not found — it may belong to another session or have been pruned.
      </div>
    );
  }

  if (!job) {
    return (
      <div className="rounded-2xl border border-white/10 bg-black/50 p-4 text-xs text-slate-400 font-code animate-pulse">
        Loading job {jobId.slice(0, 8)}…
      </div>
    );
  }

  const params = asRecord(job.params_json);
  const result = asRecord(job.result_json);
  const countsRaw = asRecord(result.counts);
  const counts: Record<string, number> | undefined = Object.keys(countsRaw).length > 0
    ? Object.fromEntries(Object.entries(countsRaw).map(([k, v]) => [k, Number(v) || 0]))
    : undefined;
  const errorText =
    typeof result.error === 'string'
      ? result.error
      : typeof result.error_message === 'string'
        ? result.error_message
        : null;

  return (
    <div className="rounded-2xl border border-white/10 bg-black/50 p-4 space-y-3 text-xs">
      <div className="flex items-center justify-between">
        <span className="font-code text-slate-400">job {job.id.slice(0, 8)}</span>
        <span className="font-code text-cyan-200">{jobStatusLabel(job.status)}</span>
      </div>
      <div className="grid grid-cols-2 gap-3">
        {kv('Type', String(job.job_type || '—'))}
        {kv('Backend', String(params.requested_backend ?? '—'))}
        {kv('Shots', String(params.shots ?? '—'))}
        {kv('Max cost', params.max_cost_usd != null ? `$${Number(params.max_cost_usd).toFixed(2)}` : '—')}
      </div>
      {typeof params.qasm === 'string' && (
        <div>
          <span className="text-white/40 block text-[10px] mb-1">QASM</span>
          <pre className="p-2 rounded-lg bg-black/60 border border-white/10 overflow-x-auto font-code text-[10px] text-slate-300 max-h-32 overflow-y-auto">
            {params.qasm}
          </pre>
        </div>
      )}
      {counts ? (
        <div>
          <span className="text-white/40 block text-[10px] mb-1">Counts (measured)</span>
          <div className="space-y-1">
            {Object.entries(counts).map(([bits, n]) => (
              <div key={bits} className="flex items-center gap-2 font-code text-[11px]">
                <span className="text-cyan-200">{bits}</span>
                <div className="flex-1 h-1.5 rounded bg-white/10 overflow-hidden">
                  <div
                    className="h-full bg-cyan-400/70"
                    style={{ width: `${Math.min(100, (n / Math.max(1, ...Object.values(counts))) * 100)}%` }}
                  />
                </div>
                <span className="text-slate-300">{n}</span>
              </div>
            ))}
          </div>
        </div>
      ) : errorText ? (
        <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
          {errorText.slice(0, 300)}
        </div>
      ) : (
        <p className="text-[10px] font-code text-slate-500">No result recorded yet — job still queued or running.</p>
      )}
      {job.completed_at && (
        <p className="text-[10px] font-code text-slate-500">
          Completed {new Date(job.completed_at).toLocaleString()}
        </p>
      )}
    </div>
  );
}
