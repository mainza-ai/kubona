'use client';

import React, { useState } from 'react';
import { issueVaultLease, revokeVaultLease, type VaultLease } from '@/lib/api';

// Short-lived provider-secret lease UI. No secret value is ever returned by
// the server; the lease is an ID + HMAC receipt used by the governed rail.
export function VaultLeasePanel({ sessionId }: { sessionId?: string }) {
  const [scope, setScope] = useState('provider:session_token');
  const [ttl, setTtl] = useState('60');
  const [lease, setLease] = useState<VaultLease | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const issue = async () => {
    setBusy(true);
    setNotice(null);
    try {
      const n = Number(ttl);
      if (!Number.isInteger(n) || n < 1) { setNotice('ttl must be a positive integer.'); return; }
      const out = await issueVaultLease(scope.trim(), n, sessionId);
      if (!out.ok) { setNotice(`lease refused: ${out.error}`); return; }
      setLease(out.lease);
      setNotice('Lease issued. The secret value is never returned by this rail.');
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    if (!lease) return;
    setBusy(true);
    setNotice(null);
    try {
      const out = await revokeVaultLease(lease.lease_id);
      if (!out.ok) { setNotice(`revoke refused: ${out.error}`); return; }
      setLease(null);
      setNotice('Lease revoked.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Vault leases">
      <div>
        <div className="text-xs font-semibold text-[var(--color-text)]">Vault secret leases</div>
        <p className="text-[12px] text-[var(--color-muted)]">Short-lived lease IDs for the governed secret rail. No value is stored or shown here.</p>
      </div>
      <div className="flex flex-wrap gap-1.5">
        <input aria-label="Lease scope" value={scope} onChange={(e) => setScope(e.target.value)} className="flex-1 min-w-40 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] font-mono text-[var(--color-text)]" />
        <input aria-label="Lease TTL seconds" value={ttl} onChange={(e) => setTtl(e.target.value)} inputMode="numeric" className="w-20 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] font-mono text-[var(--color-text)]" />
        <button type="button" onClick={() => void issue()} disabled={busy} className="px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] disabled:opacity-40">Issue lease</button>
      </div>
      {lease && (
        <div className="rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] p-2 text-[12px] font-mono text-[var(--color-text)] space-y-0.5">
          <div>id {lease.lease_id}</div>
          <div>scope {lease.scope} · ttl {lease.ttl_seconds}s · expires {lease.expires_at}</div>
          <div className="text-[var(--color-muted)] break-all">sig {lease.lease_signature.slice(0, 24)}…</div>
          <button type="button" onClick={() => void revoke()} disabled={busy} className="mt-1 px-2 py-1 rounded bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] disabled:opacity-40">Revoke lease</button>
        </div>
      )}
      {notice && <p role="status" className="text-[12px] text-[var(--color-text)]">{notice}</p>}
    </div>
  );
}
