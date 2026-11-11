'use client';

import React, { useEffect, useState } from 'react';
import { fetchCostSummary, type CostSummary } from '@/lib/api';

const fmtFixed = (value: unknown, digits: number) =>
  typeof value === 'number' && Number.isFinite(value) ? value.toFixed(digits) : '—';

export interface BudgetSummaryStripProps {
  summary?: CostSummary | null;
  refused?: string | null;
  autoFetch?: boolean;
}

export function BudgetSummaryStrip({ summary: controlledSummary, refused: controlledRefused, autoFetch = false }: BudgetSummaryStripProps) {
  const [fetchedSummary, setFetchedSummary] = useState<CostSummary | null>(null);
  const [fetchedRefused, setFetchedRefused] = useState<string | null>(null);

  useEffect(() => {
    if (!autoFetch) return;
    let mounted = true;
    fetchCostSummary()
      .then((res) => {
        if (!mounted) return;
        if (res.ok) {
          if (typeof res.result?.spent_usd === 'number') {
            setFetchedSummary(res.result);
            setFetchedRefused(null);
          }
        } else {
          setFetchedSummary(null);
          setFetchedRefused(res.error);
        }
      })
      .catch((err) => {
        if (mounted) setFetchedRefused(err instanceof Error ? err.message : String(err));
      });
    return () => {
      mounted = false;
    };
  }, [autoFetch]);

  const summary = controlledSummary !== undefined ? controlledSummary : fetchedSummary;
  const refused = controlledRefused !== undefined ? controlledRefused : fetchedRefused;

  return (
    <div className="p-4 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)]">
      <div className="text-[12px] font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono flex flex-wrap items-center gap-2">
        <span>LLM spend — server-sealed</span>
        {summary?.local_bypass && (
          <span className="text-[var(--color-success-ink)] normal-case font-sans">
            {`local inference $0 · unmetered (active: ${summary.active_provider})`}
          </span>
        )}
      </div>
      {refused && (
        <div className="mt-2 p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
          Spend refused — {refused.slice(0, 200)}. Nothing spent, nothing assumed.
        </div>
      )}
      {!refused && !summary && (
        <div className="mt-2 text-[12px] text-[var(--color-muted)] font-code">
          {autoFetch ? 'Awaiting spend telemetry…' : 'Loading spend…'}
        </div>
      )}
      {summary && (
        <div className="mt-1 text-xs text-[var(--color-label-secondary)] font-mono">
          spent ${fmtFixed(summary.spent_usd, 4)}
          {summary.track_only ? (
            <span className="text-[var(--color-muted)]"> · no cap (track-only)</span>
          ) : (
            <span>
              {' '}
              / ${fmtFixed(summary.cap_usd, 2)} cap · ${fmtFixed(summary.remaining_usd ?? 0, 2)} left
            </span>
          )}
          {summary.unmetered_count > 0 && (
            <span className="text-[var(--color-warning-ink)]">
              {' '}
              · {summary.unmetered_count} blind-stream record{summary.unmetered_count === 1 ? '' : 's'} (gate-relevant)
            </span>
          )}
        </div>
      )}
    </div>
  );
}
