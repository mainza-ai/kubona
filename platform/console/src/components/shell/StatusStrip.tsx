'use client';

import React from 'react';
import { BackendStatusBar } from '@/components/common/BackendStatusBar';
import { CheckInStrip } from '@/components/sovereign/CheckInStrip';
import type { Loop } from '@/types';

/** One operational line: backend readiness + loops/spend/attention. */
export function StatusStrip({ loops }: { loops: Loop[] }) {
  return (
    <div className="flex min-h-8 min-w-0 items-center gap-2 overflow-hidden border-b border-white/[0.06] bg-black/30 px-3 py-1">
      <BackendStatusBar compact />
      <span aria-hidden className="h-3 w-px shrink-0 bg-white/10" />
      <div className="min-w-0 flex-1 overflow-hidden">
        <CheckInStrip loops={loops} compact />
      </div>
    </div>
  );
}
