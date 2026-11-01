'use client';

import React from 'react';
import { BackendStatusBar } from '@/components/common/BackendStatusBar';
import { CheckInStrip } from '@/components/sovereign/CheckInStrip';
import type { Loop } from '@/types';

/** One operational line: backend readiness + loops/spend/attention. */
export function StatusStrip({ loops, error, streamLive, lastEventAt }: { loops: Loop[]; error?: string | null; streamLive?: boolean; lastEventAt?: string | null }) {
  return (
    <div className="flex min-h-8 min-w-0 items-center gap-2 overflow-hidden border-b border-white/[0.06] bg-black/30 px-3 py-1">
      <BackendStatusBar compact />
      <span aria-hidden className="h-3 w-px shrink-0 bg-white/10" />
      <div className="min-w-0 flex-1 overflow-hidden">
        <CheckInStrip loops={loops} compact />
      </div>
      <span className="shrink-0 font-mono text-[11px] text-slate-400">
        {streamLive ? 'stream live' : 'stream idle'}
      </span>
      <span aria-hidden className="h-3 w-px shrink-0 bg-white/10" />
      <span className="max-w-40 shrink-0 truncate font-mono text-[11px] text-slate-500" title={lastEventAt ?? undefined}>
        {lastEventAt ? `last ${lastEventAt.slice(11, 19)}` : 'no events'}
      </span>
      {error && (
        <span role="status" title={error} className="max-w-48 shrink-0 truncate font-mono text-[11px] text-amber-300">
          data refused
        </span>
      )}
    </div>
  );
}
