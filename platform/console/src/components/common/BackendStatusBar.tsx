'use client';

import React from 'react';
import type { BackendStatus } from '@/lib/api';
import { useBackendProjection } from '@/domain/useBackendProjection';

// U1-a: always-visible backend status strip. Three states, never merged:
// healthy (emerald), degraded (amber — ready 503 with the backend's
// reason), unreachable (rose — transport failed, so every surface below
// may be showing cached or empty data). Polls slowly (15s): readiness
// changes on disk/bridge timescales, not interaction timescales.
// The shared backend store owns polling and visibility gating.

function label(s: BackendStatus | null): { text: string; tone: string; dot: string } {
  if (!s || !s.reachable) {
    return {
      text: 'backend unreachable — data below may be stale or empty',
      tone: 'bg-rose-500/10 border-rose-500/30 text-rose-200',
      dot: 'bg-rose-400',
    };
  }
  if (!s.ready) {
    return {
      text: `backend degraded — ${s.message || 'not ready'}`,
      tone: 'bg-amber-500/10 border-amber-500/30 text-amber-200',
      dot: 'bg-amber-400',
    };
  }
  const disk =
    s.diskLevel && s.diskLevel !== 'ok' && s.diskLevel !== 'OK'
      ? ` · disk ${s.diskLevel}`
      : '';
  return {
    text: `${s.service || 'kubonad'}${s.version ? ` ${s.version}` : ''} · ready${disk}`,
    tone: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-200',
    dot: 'bg-emerald-400',
  };
}

export function BackendStatusBar({ compact = false }: { compact?: boolean } = {}) {
  const { data: status } = useBackendProjection();
  const { text, tone, dot } = label(status);
  return (
    <div
      role="status"
      aria-label={`Backend status: ${text}`}
      className={`flex min-w-0 items-center gap-2 text-[12px] font-code ${compact ? 'py-0' : `border-b border-white/[0.06] px-3 py-1 ${tone}`}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${dot}`} aria-hidden="true" />
      <span className="truncate">{text}</span>
    </div>
  );
}
