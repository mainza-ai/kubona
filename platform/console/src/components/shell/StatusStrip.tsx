'use client';

import React from 'react';
import { BackendStatusBar } from '@/components/common/BackendStatusBar';
import { CheckInStrip } from '@/components/sovereign/CheckInStrip';
import type { Loop } from '@/types';
import { Separator } from '@/components/ui/Controls';

/** One operational line: backend readiness + loops/spend/attention. */
export function StatusStrip({ loops, error, streamLive, lastEventAt }: { loops: Loop[]; error?: string | null; streamLive?: boolean; lastEventAt?: string | null }) {
  return (
    <div className="flex min-h-8 min-w-0 items-center gap-2 overflow-hidden border-t border-[var(--color-border)] bg-[var(--color-chrome)] px-3 py-1">
      <BackendStatusBar compact />
      <Separator vertical />
      <div className="min-w-0 flex-1 overflow-hidden">
        <CheckInStrip loops={loops} compact />
      </div>
      <span className="shrink-0 font-mono text-[12px] text-[var(--color-muted)]">
        {streamLive ? 'stream live' : 'stream idle'}
      </span>
      <Separator vertical />
      <span className="max-w-40 shrink-0 truncate font-mono text-[12px] text-[var(--color-muted)]" title={lastEventAt ?? undefined}>
        {lastEventAt ? `last ${lastEventAt.slice(11, 19)}` : 'no events'}
      </span>
      {error && (
        <span role="status" title={error} className="max-w-48 shrink-0 truncate font-mono text-[12px] text-[var(--color-warning)]">
          data refused
        </span>
      )}
    </div>
  );
}
