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
    <div className="rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Ledger verification">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-label font-semibold text-text">Ledger chain verification</div>
          <p className="text-[12px] text-faint">Server recomputes the hash chain; a clean result is not a proof of backup freshness.</p>
        </div>
        <button
          type="button"
          onClick={() => void run()}
          disabled={busy}
          className="px-2 py-1 rounded-control-compact bg-fill hover:bg-fill-hover border border-border text-[12px] text-label-secondary disabled:opacity-40"
        >
          {busy ? 'Verifying…' : 'Verify ledger'}
        </button>
      </div>
      {result && (
        <div role="status" className={`p-2 rounded-control-compact text-[12px] font-code border ${result.ok ? 'bg-success-wash border-success-edge text-success-ink' : 'bg-danger-wash border-danger-edge text-danger-ink'}`}>
          {result.error ? `verification refused: ${result.error}` : result.ok ? 'chain clean (0 faults)' : `${result.fault_count ?? 0} fault(s) — inspect the export`}
        </div>
      )}
    </div>
  );
}
