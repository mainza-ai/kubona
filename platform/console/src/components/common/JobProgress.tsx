import React, { useEffect, useRef, useState } from 'react';
import { fetchJob, isOutcomeUnknown, isTerminalJobStatus, jobStatusLabel } from '@/lib/api';
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
      shouldContinue: (next) => {
        if (!next) return true;
        return !isTerminalJobStatus(next.status) && !isOutcomeUnknown(next.status);
      },
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
      <div className="rounded-card border border-border bg-surface p-2.5 text-label" role="alert" aria-label={`Job ${jobId} unavailable`}>
        <span className="text-muted font-code">{error}</span>
        <button
          type="button"
          onClick={onRetry}
          className="ml-2 px-2 py-1 rounded-control-compact bg-fill hover:bg-fill-hover border border-border text-label-secondary text-[12px] font-semibold"
        >
          Retry
        </button>
      </div>
    );
  }

  const st = jobStatusLabel(status);
  const unknown = isOutcomeUnknown(status);
  const done = isTerminalJobStatus(status);
  const failed = st === 'FAILED' || st === 'CANCELLED' || st === 'INTERRUPTED';
  const activeIdx = done ? 2 : st === 'RUNNING' ? 1 : 0;
  const steps = unknown
    ? ['OUTCOME UNKNOWN']
    : ['QUEUED', 'RUNNING', done ? (failed ? st : 'COMPLETED') : 'SETTLING'];
  return (
    <div className="rounded-card border border-border bg-surface p-2.5 text-label" aria-label={`Job ${status}`}>
      {/* Wave-2 a11y: polite live region announces phase transitions.
          Text derives from the settled status, so it only changes on
          phase change. Visual output is identical (sr-only). */}
      <span aria-live="polite" role="status" className="sr-only">
        {`Job ${jobId} status ${st}`}
      </span>
      <div className="flex items-center gap-1.5">
        {steps.map((s, i) => (
          <React.Fragment key={s}>
            {i > 0 && <span className="text-faint">→</span>}
            <span
              className={`px-1.5 py-0.5 rounded-capsule font-code text-[12px] ${
                unknown
                  ? 'bg-warning-wash text-warning-ink'
                  : done
                  ? failed && i === 2
                    ? st === 'INTERRUPTED'
                      ? 'bg-warning-wash text-warning-ink'
                      : 'bg-danger-wash text-danger-ink'
                    : 'bg-success-wash text-success-ink'
                  : i < activeIdx
                    ? 'bg-success-wash text-success-ink'
                    : i === activeIdx
                      ? 'bg-info-wash text-info-ink animate-pulse'
                      : 'bg-fill text-faint'
              }`}
            >
              {s}
            </span>
          </React.Fragment>
        ))}
        <span className="text-faint font-code ml-auto">{elapsed(detail.created, detail.completed)}</span>
      </div>
      {detail.jobType && <div className="text-faint mt-1 font-code text-[12px]">{detail.jobType}</div>}
    </div>
  );
}
