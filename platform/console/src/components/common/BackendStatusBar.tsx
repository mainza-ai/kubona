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
      tone: 'bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]',
      dot: 'bg-[var(--color-danger-ink)]',
    };
  }
  if (!s.ready) {
    return {
      text: `backend degraded — ${s.message || 'not ready'}`,
      tone: 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]',
      dot: 'bg-[var(--color-warning)]',
    };
  }
  const disk =
    s.diskLevel && s.diskLevel !== 'ok' && s.diskLevel !== 'OK'
      ? ` · disk ${s.diskLevel}`
      : '';
  return {
    text: `${s.service || 'kubonad'}${s.version ? ` ${s.version}` : ''} · ready${disk}`,
    tone: 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]',
    dot: 'bg-[var(--color-success)]',
  };
}

export function BackendStatusBar({ compact = false }: { compact?: boolean } = {}) {
  const { data: status } = useBackendProjection();
  const { text, tone, dot } = label(status);
  return (
    <div
      role="status"
      aria-label={`Backend status: ${text}`}
      className={`flex min-w-0 items-center gap-2 text-[12px] font-code ${compact ? 'py-0' : `border-b border-[var(--color-border)] px-3 py-1 ${tone}`}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${dot}`} aria-hidden="true" />
      <span className="truncate">{text}</span>
    </div>
  );
}
