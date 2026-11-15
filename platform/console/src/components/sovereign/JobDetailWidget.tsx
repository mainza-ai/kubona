'use client';

import React, { useState, useEffect } from 'react';
import { fetchJob, jobStatusLabel } from '@/lib/api';
import { asRecord, fmtFixed } from '@/lib/guards';
import type { Job } from '@/types';

function kv(label: string, value: string) {
  return (
    <div>
      <span className="text-muted block text-[12px]">{label}</span>
      <span className="font-code text-text break-all">{value}</span>
    </div>
  );
}

/**
 * Job detail: full params + result payloads for one job id, fetched live.
 * Nothing here is invented: absent result → "no result recorded yet";
 * unknown id → refusal notice. Counts render measured numbers only —
 * non-numeric entries render raw/UNVERIFIED, never coerced 0 bars.
 */
export function JobDetailWidget({ jobId, sessionId }: { jobId: string; sessionId: string }) {
  const [job, setJob] = useState<Job | null>(null);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset during render on job/session change (same key-change pattern as
  // the context readouts): stale job/error/missing flags must not bleed
  // across selections while the refetch is in flight.
  const prevKey = `${jobId} ${sessionId}`;
  const [prevSeenKey, setPrevSeenKey] = useState(prevKey);
  if (prevSeenKey !== prevKey) {
    setPrevSeenKey(prevKey);
    setError(null);
    setMissing(false);
    setJob(null);
  }

  useEffect(() => {
    let mounted = true;
    fetchJob(jobId, sessionId).then(
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
  }, [jobId, sessionId]);

  if (error) {
    return (
      <div className="rounded-card border border-danger-edge bg-danger-wash p-4 text-label text-danger-ink font-code">
        Job load failed — nothing shown ({error.slice(0, 200)}).
      </div>
    );
  }

  if (missing) {
    return (
      <div className="rounded-card border border-border bg-surface p-4 text-label text-muted font-code">
        Job not found — it may belong to another session or have been pruned.
      </div>
    );
  }

  if (!job) {
    return (
      <div className="rounded-card border border-border bg-surface p-4 text-label text-muted font-code animate-pulse">
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
    <div className="rounded-card border border-border bg-surface p-4 space-y-3 text-label">
      <div className="flex items-center justify-between">
        <span className="font-code text-muted">job {job.id.slice(0, 8)}</span>
        <span className="font-code text-info-ink">{jobStatusLabel(job.status)}</span>
      </div>
      <div className="grid grid-cols-2 gap-3">
        {kv('Type', String(job.job_type || '—'))}
        {kv('Backend', String(params.requested_backend ?? '—'))}
        {kv('Shots', String(params.shots ?? '—'))}
        {kv('Max cost', maxCostText)}
      </div>
      {typeof params.qasm === 'string' && (
        <div>
          <span className="text-muted block text-[12px] mb-1">QASM</span>
          <pre className="p-2 rounded-control-compact bg-field border border-border overflow-x-auto font-code text-[12px] text-label-secondary max-h-32 overflow-y-auto">
            {params.qasm}
          </pre>
        </div>
      )}
      {hasCounts ? (
        <div>
          <span className="text-muted block text-[12px] mb-1">Counts (measured)</span>
          <div className="space-y-1">
            {countsEntries.map(({ bits, raw, n }) => (
              <div key={bits} className="flex items-center gap-2 font-code text-[12px]">
                <span className="text-info-ink">{bits}</span>
                {n != null ? (
                  <>
                    <div className="flex-1 h-1.5 rounded-chip bg-fill overflow-hidden">
                      <div
                        className="h-full bg-accent"
                        style={{ width: `${Math.min(100, (n / Math.max(1, maxCount)) * 100)}%` }}
                      />
                    </div>
                    <span className="text-label-secondary">{n}</span>
                  </>
                ) : (
                  <span className="text-warning-ink">UNVERIFIED (raw: {String(raw).slice(0, 40)})</span>
                )}
              </div>
            ))}
          </div>
        </div>
      ) : errorText ? (
        <div className="p-2.5 rounded-row bg-danger-wash border border-danger-edge text-danger-ink text-[12px]">
          {errorText.slice(0, 300)}
        </div>
      ) : (
        <p className="text-[12px] font-code text-faint">No result recorded yet — job still queued or running.</p>
      )}
      {(() => {
        const c = job.completed_at;
        if (!c) return null;
        const d = new Date(c);
        return (
          <p className="text-[12px] font-code text-faint">
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
