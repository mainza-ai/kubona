'use client';

import React, { useState } from 'react';
import { History } from 'lucide-react';
import { fetchProposals, type ProposalRecord } from '@/lib/api';

export function ProposalHistory({
  sessionId,
  load = fetchProposals,
}: {
  sessionId: string | null;
  load?: (sessionId: string) => Promise<ProposalRecord[]>;
}) {
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<ProposalRecord[] | null>(null);

  if (!sessionId) return null;

  const toggle = async () => {
    const next = !open;
    setOpen(next);
    if (next && rows === null) {
      setRows(await load(sessionId));
    }
  };

  return (
    <details
      open={open}
      onToggle={(event) => {
        if ((event.currentTarget as HTMLDetailsElement).open !== open) void toggle();
      }}
      className="rounded-control border border-border bg-surface px-2.5 py-1.5 text-[12px]"
    >
      <summary className="flex cursor-pointer select-none items-center gap-1.5 text-label-secondary">
        <History className="h-3.5 w-3.5 text-accent" aria-hidden="true" />
        Proposal history
      </summary>
      {rows === null ? (
        <p className="mt-1 text-faint">Loading proposals…</p>
      ) : rows.length === 0 ? (
        <p className="mt-1 text-muted">No proposals recorded for this session.</p>
      ) : (
        <ul className="mt-1 space-y-1" aria-label="Proposal history">
          {rows.map((row) => (
            <li key={row.id} className="rounded-row border border-border bg-card px-2 py-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-code text-accent">{row.id.slice(0, 8)}</span>
                <span className="rounded-capsule border border-border px-1.5 py-0.5 text-muted">{row.status}</span>
                <span className="truncate text-text">{row.body?.target ?? '—'}</span>
                <span className="ml-auto text-faint">{row.created_at.slice(0, 19).replace('T', ' ')}</span>
              </div>
              <div className="mt-0.5 flex flex-wrap gap-2 font-code text-faint">
                <span>{row.body?.recommended_tier ?? '—'}</span>
                <span>{row.body?.spend_estimate ?? '—'}</span>
                {row.launched_job_id && <span>{`job ${row.launched_job_id.slice(0, 8)}`}</span>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </details>
  );
}
