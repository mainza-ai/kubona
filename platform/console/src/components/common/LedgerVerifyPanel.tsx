'use client';

import React, { useState } from 'react';
import { verifyLedger } from '@/lib/api';

export function LedgerVerifyPanel({ sessionId }: { sessionId: string | null }) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; fault_count?: number; faults?: unknown[]; error?: string } | null>(null);

  const run = async () => {
    setBusy(true);
    setResult(null);
    try {
      setResult(await verifyLedger(sessionId ?? undefined));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Ledger verification">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-xs font-semibold text-white">Ledger chain verification</div>
          <p className="text-[10px] text-slate-500">Server recomputes the hash chain; a clean result is not a proof of backup freshness.</p>
        </div>
        <button
          type="button"
          onClick={() => void run()}
          disabled={busy}
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-200 disabled:opacity-40"
        >
          {busy ? 'Verifying…' : 'Verify ledger'}
        </button>
      </div>
      {result && (
        <div role="status" className={`p-2 rounded-lg text-[11px] font-code border ${result.ok ? 'bg-emerald-950/40 border-emerald-500/30 text-emerald-200' : 'bg-rose-950/40 border-rose-500/30 text-rose-200'}`}>
          {result.error ? `verification refused: ${result.error}` : result.ok ? 'chain clean (0 faults)' : `${result.fault_count ?? 0} fault(s) — inspect the export`}
        </div>
      )}
    </div>
  );
}
