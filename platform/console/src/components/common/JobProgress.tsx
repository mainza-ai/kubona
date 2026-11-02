import React, { useEffect, useRef, useState } from 'react';
import { fetchJob, isTerminalJobStatus, jobStatusLabel } from '@/lib/api';
import { useSharedResource } from '@/domain/shared-resource';
import { subscribeInvalidation } from '@/domain/invalidation';
import { useSessionStreamLive } from '@/domain/stream-state';
import { StopPolling } from '@/domain/resource-store';
import type { Job } from '@/types';

// S8b: stepwise progress for long operations. Polls the job record every
// 2 s and renders queued → running → settled with elapsed time. Stops on
// terminal status. Shows only measured states from the record (status,
// created/completed timestamps) — no invented progress bars.
//
// Null-job path: fetchJob returns null on transport loss or unknown id.
// A single null is a transient blip (keep polling); N consecutive nulls
// mean the job is gone or the backend is down — stop the loop and render
// an explicit "job unavailable — retry" state instead of an infinite
// QUEUED pulse (same discipline as QuantumDispatch's paused polling).
const POLL_MS = 2000;
const MAX_CONSECUTIVE_NULLS = 3;

function elapsed(created?: string, completed?: string): string {
  if (!created) return '';
  const start = Date.parse(created);
  if (Number.isNaN(start)) return '';
  const end = completed && !Number.isNaN(Date.parse(completed)) ? Date.parse(completed as string) : Date.now();
  const s = Math.max(0, Math.round((end - start) / 1000));
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m ${s % 60}s`;
}

export function JobProgress({ jobId, sessionId }: { jobId: string; sessionId: string }) {
  const [retryKey, setRetryKey] = useState(0);
  return (
    <JobProgressInner
      key={`${sessionId}:${jobId}:${retryKey}`}
      jobId={jobId}
      sessionId={sessionId}
      onRetry={() => setRetryKey((key) => key + 1)}
    />
  );
}

function JobProgressInner({ jobId, sessionId, onRetry }: { jobId: string; sessionId: string; onRetry: () => void }) {
  const consecutiveNulls = useRef(0);
  const live = useSessionStreamLive();
  const { data: job, status: resourceStatus, error, setPollMs, refresh } = useSharedResource<Job | null>(`job:${sessionId}:${jobId}`,
    async () => {
      const next = await fetchJob(jobId, sessionId);
      if (!next) {
        consecutiveNulls.current += 1;
        if (consecutiveNulls.current >= MAX_CONSECUTIVE_NULLS) {
          throw new StopPolling('job unavailable — backend returned nothing (transport or unknown id).');
        }
      } else {
        consecutiveNulls.current = 0;
      }
      return next;
    },
    {
      intervalMs: POLL_MS,
      shouldContinue: (next) => !next || !isTerminalJobStatus(next.status),
    },
  );
  useEffect(() => { setPollMs(live ? 0 : POLL_MS); }, [live, setPollMs]);
  useEffect(() => subscribeInvalidation(['jobs'], refresh), [refresh]);
  const status = job?.status ? String(job.status) : 'JOB_STATUS_QUEUED';
  const detail = job
    ? { created: job.created_at, completed: job.completed_at, jobType: job.job_type }
    : {};
  const gone = resourceStatus === 'refused' && Boolean(error);

  if (gone) {
    return (
      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-2.5 text-xs" role="alert" aria-label={`Job ${jobId} unavailable`}>
        <span className="text-slate-400 font-code">{error}</span>
        <button
          type="button"
          onClick={onRetry}
          className="ml-2 px-2 py-1 rounded-lg bg-white/[0.06] hover:bg-white/[0.1] border border-white/10 text-slate-200 text-[12px] font-semibold"
        >
          Retry
        </button>
      </div>
    );
  }

  const st = jobStatusLabel(status);
  const done = isTerminalJobStatus(status);
  const failed = st === 'FAILED' || st === 'CANCELLED';
  const activeIdx = done ? 2 : st === 'RUNNING' ? 1 : 0;
  const steps = ['QUEUED', 'RUNNING', done ? (failed ? st : 'COMPLETED') : 'SETTLING'];
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-2.5 text-xs" aria-label={`Job ${status}`}>
      {/* Wave-2 a11y: polite live region announces phase transitions.
          Text derives from the settled status, so it only changes on
          phase change. Visual output is identical (sr-only). */}
      <span aria-live="polite" role="status" className="sr-only">
        {`Job ${jobId} status ${st}`}
      </span>
      <div className="flex items-center gap-1.5">
        {steps.map((s, i) => (
          <React.Fragment key={s}>
            {i > 0 && <span className="text-slate-600">→</span>}
            <span
              className={`px-1.5 py-0.5 rounded-full font-code text-[12px] ${
                done
                  ? failed && i === 2
                    ? 'bg-rose-500/20 text-rose-200'
                    : 'bg-emerald-500/15 text-emerald-200'
                  : i < activeIdx
                    ? 'bg-emerald-500/15 text-emerald-200'
                    : i === activeIdx
                      ? 'bg-cyan-500/15 text-cyan-200 animate-pulse'
                      : 'bg-white/5 text-slate-500'
              }`}
            >
              {s}
            </span>
          </React.Fragment>
        ))}
        <span className="text-slate-500 font-code ml-auto">{elapsed(detail.created, detail.completed)}</span>
      </div>
      {detail.jobType && <div className="text-slate-500 mt-1 font-code text-[12px]">{detail.jobType}</div>}
    </div>
  );
}
