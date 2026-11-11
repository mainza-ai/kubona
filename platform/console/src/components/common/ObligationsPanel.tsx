'use client';

import React, { useState } from 'react';
import { closeObligation } from '@/lib/api';
import { useObligationsProjection } from '@/domain/useObligationsProjection';
import { acceptWrite, newWriteRequestId, settleWrite, writeSettlementLabel, IDLE_WRITE, type WriteSettlement } from '@/domain/write-settlement';

// Mandate-review queue (steady-state ops). A closed obligation is decided
// by the server's AttemptClose rules; this panel only lists open rows and
// lets an authorized session trigger closure. Refusals stay visible.
export function ObligationsPanel() {
  const { obligations, status, error, refresh } = useObligationsProjection();
  const rows = status === 'idle' || status === 'loading' ? null : obligations;
  const [busy, setBusy] = useState<string | null>(null);
  const [write, setWrite] = useState<WriteSettlement>(IDLE_WRITE);

  const close = async (id: string) => {
    setBusy(id);
    const requestId = newWriteRequestId();
    setWrite(acceptWrite(requestId, `closing ${id.slice(0, 8)}`));
    try {
      const out = await closeObligation(id);
      if (!out.ok) {
        setWrite(settleWrite(requestId, 'refused', out.error ?? 'unknown'));
      } else if (out.closed) {
        setWrite(settleWrite(requestId, 'applied', `obligation ${id.slice(0, 8)} closed`));
      } else {
        setWrite(settleWrite(requestId, 'refused', `not close-ready: ${(out.reasons ?? []).join('; ') || 'server gave no reasons'}`));
      }
      refresh();
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Open obligations">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-xs font-semibold text-[var(--color-text)]">Open obligations · mandate review</span>
          <p className="text-[12px] text-[var(--color-muted)]">Closure only triggers the server rule; this panel cannot force-close.</p>
        </div>
        <button type="button" onClick={() => void refresh()} className="px-2 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-text)]">Refresh</button>
      </div>
      {rows === null && <p className="text-[12px] font-code text-[var(--color-muted)]">Loading obligations…</p>}
      {error && <p role="alert" className="text-[12px] text-[var(--color-danger-ink)]">Queue unavailable — {error}</p>}
      {rows !== null && !error && rows.length === 0 && <p className="text-[12px] text-[var(--color-muted)]">No open obligations.</p>}
      {(rows ?? []).map((ob) => (
        <div key={ob.id} className="px-2.5 py-2 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-[12px] space-y-1">
          <div className="flex items-center justify-between gap-2">
            <span className="font-code text-[var(--color-text)] truncate" title={ob.id}>{ob.id.slice(0, 12)} · {ob.subject_kind}/{ob.subject_id.slice(0, 12)}</span>
            <button
              type="button"
              disabled={busy !== null || !ob.close_ready}
              title={ob.close_ready ? 'Trigger server closure evaluation' : (ob.close_reasons.join('; ') || 'not close-ready')}
              onClick={() => void close(ob.id)}
              className="shrink-0 px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] disabled:opacity-40"
            >
              {busy === ob.id ? 'Closing…' : ob.close_ready ? 'Evaluate close' : 'Not close-ready'}
            </button>
          </div>
          <div className="text-[var(--color-muted)] font-code">owner {ob.owner.slice(0, 24)} · disposition {ob.disposition}</div>
          {ob.close_reasons.length > 0 && <div className="text-[var(--color-warning-ink)]">{ob.close_reasons.join(' · ')}</div>}
        </div>
      ))}
      {write.phase !== 'idle' && <p role="status" className="text-[12px] text-[var(--color-text)]">{writeSettlementLabel(write)}</p>}
    </div>
  );
}
