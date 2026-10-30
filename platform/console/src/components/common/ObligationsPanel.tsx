'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { closeObligation, fetchOpenObligations, type ObligationRow } from '@/lib/api';

// Mandate-review queue (steady-state ops). A closed obligation is decided
// by the server's AttemptClose rules; this panel only lists open rows and
// lets an authorized session trigger closure. Refusals stay visible.
export function ObligationsPanel() {
  const [rows, setRows] = useState<ObligationRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    const out = await fetchOpenObligations();
    if (out.ok) {
      setRows(out.obligations);
      setError(null);
    } else {
      setRows([]);
      setError(out.error);
    }
  }, []);

  useEffect(() => {
    let live = true;
    fetchOpenObligations().then((out) => {
      if (!live) return;
      if (out.ok) {
        setRows(out.obligations);
        setError(null);
      } else {
        setRows([]);
        setError(out.error);
      }
    });
    return () => { live = false; };
  }, []);

  const close = async (id: string) => {
    setBusy(id);
    setNotice(null);
    try {
      const out = await closeObligation(id);
      if (!out.ok) {
        setNotice(`close refused: ${out.error ?? 'unknown'}`);
      } else if (out.closed) {
        setNotice(`obligation ${id.slice(0, 8)} closed.`);
      } else {
        setNotice(`obligation ${id.slice(0, 8)} is not close-ready: ${(out.reasons ?? []).join('; ') || 'server gave no reasons'}`);
      }
      await load();
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Open obligations">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-xs font-semibold text-white">Open obligations · mandate review</span>
          <p className="text-[10px] text-slate-500">Closure only triggers the server rule; this panel cannot force-close.</p>
        </div>
        <button type="button" onClick={() => void load()} className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-300">Refresh</button>
      </div>
      {rows === null && <p className="text-[11px] font-code text-slate-500">Loading obligations…</p>}
      {error && <p role="alert" className="text-[11px] text-rose-300">Queue unavailable — {error}</p>}
      {rows !== null && !error && rows.length === 0 && <p className="text-[11px] text-slate-400">No open obligations.</p>}
      {(rows ?? []).map((ob) => (
        <div key={ob.id} className="px-2.5 py-2 rounded-xl bg-white/[0.03] border border-white/[0.06] text-[11px] space-y-1">
          <div className="flex items-center justify-between gap-2">
            <span className="font-code text-slate-300 truncate" title={ob.id}>{ob.id.slice(0, 12)} · {ob.subject_kind}/{ob.subject_id.slice(0, 12)}</span>
            <button
              type="button"
              disabled={busy !== null || !ob.close_ready}
              title={ob.close_ready ? 'Trigger server closure evaluation' : (ob.close_reasons.join('; ') || 'not close-ready')}
              onClick={() => void close(ob.id)}
              className="shrink-0 px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 disabled:opacity-40"
            >
              {busy === ob.id ? 'Closing…' : ob.close_ready ? 'Evaluate close' : 'Not close-ready'}
            </button>
          </div>
          <div className="text-slate-500 font-code">owner {ob.owner.slice(0, 24)} · disposition {ob.disposition}</div>
          {ob.close_reasons.length > 0 && <div className="text-amber-300/80">{ob.close_reasons.join(' · ')}</div>}
        </div>
      ))}
      {notice && <p role="status" className="text-[11px] text-slate-300">{notice}</p>}
    </div>
  );
}
