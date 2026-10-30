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
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Vault leases">
      <div>
        <div className="text-xs font-semibold text-white">Vault secret leases</div>
        <p className="text-[10px] text-slate-500">Short-lived lease IDs for the governed secret rail. No value is stored or shown here.</p>
      </div>
      <div className="flex flex-wrap gap-1.5">
        <input aria-label="Lease scope" value={scope} onChange={(e) => setScope(e.target.value)} className="flex-1 min-w-40 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] font-mono text-slate-200" />
        <input aria-label="Lease TTL seconds" value={ttl} onChange={(e) => setTtl(e.target.value)} inputMode="numeric" className="w-20 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] font-mono text-slate-200" />
        <button type="button" onClick={() => void issue()} disabled={busy} className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] disabled:opacity-40">Issue lease</button>
      </div>
      {lease && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[11px] font-mono text-slate-300 space-y-0.5">
          <div>id {lease.lease_id}</div>
          <div>scope {lease.scope} · ttl {lease.ttl_seconds}s · expires {lease.expires_at}</div>
          <div className="text-slate-500 break-all">sig {lease.lease_signature.slice(0, 24)}…</div>
          <button type="button" onClick={() => void revoke()} disabled={busy} className="mt-1 px-2 py-1 rounded bg-rose-500/15 border border-rose-400/30 text-rose-200 disabled:opacity-40">Revoke lease</button>
        </div>
      )}
      {notice && <p role="status" className="text-[11px] text-slate-300">{notice}</p>}
    </div>
  );
}
