'use client';

import React, { useState } from 'react';
import { createLab, registerTenantAdminKey } from '@/lib/api';
import { getOrCreateSigner } from '@/lib/custody';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

// Multi-lab bootstrap: create the lab row, then register the browser key
// as its first admin in the same operator gesture. Both writes require the
// operator token; the token is held in component state only.
export function CreateLabPanel({ onCreated }: { onCreated: () => void }) {
  const [id, setId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [created, setCreated] = useState<string | null>(null);
  const settlement = useWriteSettlement();

  const create = async () => {
    const labId = id.trim();
    if (!labId) { setNotice('lab id is required.'); return; }
    if (!token.trim()) { setNotice('operator token is required to register the first admin.'); return; }
    setBusy(true);
    setNotice(null);
    setCreated(null);
    const requestId = settlement.begin('create lab + first admin');
    try {
      const out = await createLab(labId, displayName.trim() || labId);
      if (!out.ok) {
        settlement[out.unknown ? 'unknown' : 'refused'](requestId, out.error);
        setNotice(`create refused: ${out.error}`);
        return;
      }
      const signer = await getOrCreateSigner();
      const admin = await registerTenantAdminKey(labId, signer.publicKeyHex, undefined, token);
      if (admin === null) { settlement.unknown(requestId, 'lab created, admin registration transport failed'); setNotice('lab created, but admin registration transport failed. Retry with the token.'); return; }
      if ('error' in admin) { settlement.refused(requestId, admin.error); setNotice(`lab created, but admin registration refused: ${admin.error}`); return; }
      settlement.applied(requestId, `lab ${labId} created with first admin`);
      setCreated(labId);
      setId('');
      setDisplayName('');
      setToken('');
      onCreated();
    } catch (err) {
      settlement.unknown(requestId, err instanceof Error ? err.message : String(err));
      setNotice(`create failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Create laboratory">
      <div className="text-xs font-semibold text-[var(--color-text)]">Create laboratory</div>
      <p className="text-[12px] text-[var(--color-faint)]">Creates the lab and registers this browser key as its first admin. The operator token is used once and not stored.</p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <input value={id} onChange={(e) => setId(e.target.value)} aria-label="New lab ID" placeholder="lab id (e.g. materials-lab)" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] placeholder:text-[var(--color-faint)] font-mono" />
        <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} aria-label="New lab display name" placeholder="display name" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] placeholder:text-[var(--color-faint)]" />
      </div>
      <input value={token} onChange={(e) => setToken(e.target.value)} type="password" aria-label="New lab operator token" placeholder="operator token (KUBONA_API_TOKEN)" className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] placeholder:text-[var(--color-faint)] font-mono" />
      <button type="button" onClick={() => void create()} disabled={busy || !id.trim() || !token.trim()} className="px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] disabled:opacity-40">
        {busy ? 'Creating…' : 'Create lab + register admin'}
      </button>
      {created && <p role="status" className="text-[12px] text-[var(--color-success-ink)]">Lab {created} created. Log in with this lab ID after switching lab.</p>}
      {notice && <p role="alert" className="text-[12px] text-[var(--color-warning-ink)]">{notice}</p>}
      {writeSettlementLabel(settlement.write) && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">{writeSettlementLabel(settlement.write)}</p>
      )}
    </div>
  );
}
