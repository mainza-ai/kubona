import React, { useEffect, useRef, useState } from 'react';
import { fetchJob, isTerminalJobStatus, jobStatusLabel } from '@/lib/api';

// S8b: stepwise progress for long operations. Polls the job record every
// 2 s and renders queued → running → settled with elapsed time. Stops on
// terminal status. Shows only measured states from the record (status,
// created/completed timestamps) — no invented progress bars.
const POLL_MS = 2000;

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
  const [status, setStatus] = useState<string>('JOB_STATUS_QUEUED');
  const [detail, setDetail] = useState<{ created?: string; completed?: string; jobType?: string; backend?: string }>({});
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    let live = true;
    const poll = async () => {
      const job = await fetchJob(jobId, sessionId);
      if (!live || !job) return;
      setStatus(String(job.status));
      setDetail({ created: job.created_at, completed: job.completed_at, jobType: job.job_type });
      if (isTerminalJobStatus(job.status) && timer.current) {
        clearInterval(timer.current);
        timer.current = null;
      }
    };
    poll();
    timer.current = setInterval(poll, POLL_MS);
    return () => {
      live = false;
      if (timer.current) clearInterval(timer.current);
    };
  }, [jobId, sessionId]);

  const st = jobStatusLabel(status);
  const done = isTerminalJobStatus(status);
  const failed = st === 'FAILED' || st === 'CANCELLED';
  const activeIdx = done ? 2 : st === 'RUNNING' ? 1 : 0;
  const steps = ['QUEUED', 'RUNNING', done ? (failed ? st : 'COMPLETED') : 'SETTLING'];
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-2.5 text-xs" aria-label={`Job ${status}`}>
      <div className="flex items-center gap-1.5">
        {steps.map((s, i) => (
          <React.Fragment key={s}>
            {i > 0 && <span className="text-slate-600">→</span>}
            <span
              className={`px-1.5 py-0.5 rounded-full font-code text-[10px] ${
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
      {detail.jobType && <div className="text-slate-500 mt-1 font-code text-[10px]">{detail.jobType}</div>}
    </div>
  );
}
