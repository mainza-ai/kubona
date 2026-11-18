'use client';

// Agentic digest (AG-20b): the batched receipts of notify-mode execution. The
// server projects them from sealed plan_executed events, so this panel never
// invents "what happened" — it only renders the ledger's answer.

import React, { useCallback, useEffect, useState } from 'react';
import { fetchAgenticDigest, type AgenticDigest } from '@/domain/oversight';
import { Button } from '@/components/ui/Button';

function outcomeTone(accepted?: boolean, status?: string): string {
  if (accepted) return 'text-success-ink';
  if (status === 'failed') return 'text-danger-ink';
  return 'text-warning-ink';
}

export function AgenticDigestPanel({ sessionId, hours = 24 }: { sessionId: string; hours?: number }) {
  const [digest, setDigest] = useState<AgenticDigest | null>(null);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!sessionId) return;
    const out = await fetchAgenticDigest(sessionId, hours);
    setDigest(out);
    setLoaded(true);
  }, [sessionId, hours]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchAgenticDigest(sessionId, hours).then((out) => {
      if (!live) return;
      setDigest(out);
      setLoaded(true);
    });
    return () => { live = false; };
  }, [sessionId, hours]);

  if (!sessionId) return null;
  const counts = digest?.counts;

  return (
    <div className="w-full rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Agentic digest">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <span className="text-label font-semibold text-text">Agentic digest</span>
          <p className="text-[12px] text-faint">
            Notify-mode receipts from the last {hours} hours; every entry is a sealed plan execution.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="font-code text-[11px] text-muted">
            {counts ? `${counts.total} run · ${counts.accepted} accepted · ${counts.notify} notify` : '—'}
          </span>
          <Button variant="ghost" size="compact" onClick={() => void load()}>Refresh</Button>
        </div>
      </div>
      {loaded && !digest && <p className="text-[12px] text-warning">Digest unavailable.</p>}
      {digest && digest.entries.length === 0 && (
        <p className="text-[12px] text-muted">No executions in this window.</p>
      )}
      {digest && digest.entries.length > 0 && (
        <ul className="space-y-1" aria-label="Digest entries">
          {digest.entries.slice(0, 8).map((entry) => (
            <li key={`${entry.plan_id}-${entry.at ?? ''}`} className="flex flex-wrap items-center justify-between gap-2 rounded-row border border-border bg-card px-2.5 py-1.5 text-[12px]">
              <span className="min-w-0">
                <span className="font-code text-text">{entry.plan_id}</span>
                <span className="text-faint"> · {entry.risk || 'unclassified'}{entry.notify ? ' · notify' : ''}</span>
              </span>
              <span className={`font-semibold ${outcomeTone(entry.accepted, entry.status)}`}>
                {entry.accepted ? 'accepted' : (entry.status || 'recorded')}
                {entry.at ? ` · ${entry.at.slice(0, 19).replace('T', ' ')}Z` : ''}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
