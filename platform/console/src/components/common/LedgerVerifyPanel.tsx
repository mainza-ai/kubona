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
    <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Ledger verification">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-xs font-semibold text-[var(--color-text)]">Ledger chain verification</div>
          <p className="text-[12px] text-[var(--color-faint)]">Server recomputes the hash chain; a clean result is not a proof of backup freshness.</p>
        </div>
        <button
          type="button"
          onClick={() => void run()}
          disabled={busy}
          className="px-2 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] disabled:opacity-40"
        >
          {busy ? 'Verifying…' : 'Verify ledger'}
        </button>
      </div>
      {result && (
        <div role="status" className={`p-2 rounded-lg text-[12px] font-code border ${result.ok ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]' : 'bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]'}`}>
          {result.error ? `verification refused: ${result.error}` : result.ok ? 'chain clean (0 faults)' : `${result.fault_count ?? 0} fault(s) — inspect the export`}
        </div>
      )}
    </div>
  );
}
