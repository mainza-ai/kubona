'use client';

import React, { useState, useEffect } from 'react';
import { fetchJob, jobStatusLabel } from '@/lib/api';
import { asRecord, fmtFixed } from '@/lib/guards';
import type { Job } from '@/types';

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
 * unknown id → refusal notice. Counts render measured numbers only —
 * non-numeric entries render raw/UNVERIFIED, never coerced 0 bars.
 */
export function JobDetailWidget({ jobId }: { jobId: string }) {
  const [job, setJob] = useState<Job | null>(null);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset during render on job change (same key-change pattern as the
  // context readouts): stale job/error/missing flags must not bleed
  // across job selections while the refetch is in flight.
  const [prevJobId, setPrevJobId] = useState(jobId);
  if (prevJobId !== jobId) {
    setPrevJobId(jobId);
    setError(null);
    setMissing(false);
    setJob(null);
  }

  useEffect(() => {
    let mounted = true;
    fetchJob(jobId).then(
      (j) => {
        if (!mounted) return;
        if (j) setJob(j);
        else setMissing(true);
      },
      (e) => {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : String(e));
      }
    );
    return () => {
      mounted = false;
    };
  }, [jobId]);

  if (error) {
    return (
      <div className="rounded-2xl border border-rose-500/40 bg-rose-950/40 p-4 text-xs text-rose-200 font-code">
        Job load failed — nothing shown ({error.slice(0, 200)}).
      </div>
    );
  }

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
  const hasCounts = Object.keys(countsRaw).length > 0;
  const countsEntries: Array<{ bits: string; raw: unknown; n: number | null }> = hasCounts
    ? Object.entries(countsRaw).map(([k, v]) => ({
        bits: k,
        raw: v,
        n: typeof v === 'number' && Number.isFinite(v) ? v : null,
      }))
    : [];
  const numericCounts = countsEntries.filter((e) => e.n != null) as Array<{ bits: string; raw: unknown; n: number }>;
  const maxCount = numericCounts.length > 0 ? Math.max(...numericCounts.map((e) => e.n)) : 1;
  const maxCostRaw = params.max_cost_usd;
  const maxCostText =
    typeof maxCostRaw === 'number' && Number.isFinite(maxCostRaw)
      ? `$${maxCostRaw.toFixed(2)}`
      : maxCostRaw != null
        ? `UNVERIFIED (raw: ${String(maxCostRaw).slice(0, 40)})`
        : '—';
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
        {kv('Max cost', maxCostText)}
      </div>
      {typeof params.qasm === 'string' && (
        <div>
          <span className="text-white/40 block text-[10px] mb-1">QASM</span>
          <pre className="p-2 rounded-lg bg-black/60 border border-white/10 overflow-x-auto font-code text-[10px] text-slate-300 max-h-32 overflow-y-auto">
            {params.qasm}
          </pre>
        </div>
      )}
      {hasCounts ? (
        <div>
          <span className="text-white/40 block text-[10px] mb-1">Counts (measured)</span>
          <div className="space-y-1">
            {countsEntries.map(({ bits, raw, n }) => (
              <div key={bits} className="flex items-center gap-2 font-code text-[11px]">
                <span className="text-cyan-200">{bits}</span>
                {n != null ? (
                  <>
                    <div className="flex-1 h-1.5 rounded bg-white/10 overflow-hidden">
                      <div
                        className="h-full bg-cyan-400/70"
                        style={{ width: `${Math.min(100, (n / Math.max(1, maxCount)) * 100)}%` }}
                      />
                    </div>
                    <span className="text-slate-300">{n}</span>
                  </>
                ) : (
                  <span className="text-amber-300">UNVERIFIED (raw: {String(raw).slice(0, 40)})</span>
                )}
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
      {(() => {
        const c = job.completed_at;
        if (!c) return null;
        const d = new Date(c);
        return (
          <p className="text-[10px] font-code text-slate-500">
            Completed {Number.isNaN(d.getTime()) ? 'UNVERIFIED (bad timestamp)' : d.toLocaleString()}
          </p>
        );
      })()}
    </div>
  );
}

// Re-export for callers that still import the local helper.
export { asRecord };
export { fmtFixed };
