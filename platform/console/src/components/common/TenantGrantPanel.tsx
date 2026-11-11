'use client';

import React, { useState } from 'react';
import { issueTenantGrant, revokeTenantGrant } from '@/lib/api';
import { signGrant, signGrantRevoke } from '@/lib/grants';
import type { Lab } from '@/types';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

// Cross-lab read grants. The browser key must be the SOURCE lab's current
// admin; the server verifies the canonical signature, so a session alone
// cannot mint one. No grant-list endpoint exists yet: issue returns the id,
// and revocation takes that id explicitly.
export function TenantGrantPanel({ labs }: { labs: Lab[] }) {
  const [source, setSource] = useState(labs[0]?.id ?? '');
  const [target, setTarget] = useState('');
  const [scope, setScope] = useState('read');
  const [hours, setHours] = useState('24');
  const [grantId, setGrantId] = useState('');
  const [reason, setReason] = useState('operator revoke');
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const settlement = useWriteSettlement();

  const issue = async () => {
    setBusy(true);
    setNotice(null);
    let requestId: string | null = null;
    try {
      const h = Number(hours);
      if (!source.trim() || !target.trim() || !scope.trim() || !Number.isFinite(h) || h <= 0) {
        setNotice('source, target, scope, and a positive expiry are required.'); return;
      }
      const expiry = new Date(Date.now() + h * 3600_000).toISOString().replace(/\.\d{3}Z$/, 'Z');
      requestId = settlement.begin('issue grant');
      const sig = await signGrant(source.trim(), target.trim(), [scope.trim()], expiry);
      const out = await issueTenantGrant({
        source_tenant: source.trim(), target_tenant: target.trim(),
        scopes: [scope.trim()], expiry, ...sig,
      });
      if (!out.ok) { settlement[out.unknown ? 'unknown' : 'refused'](requestId!, out.error || 'not confirmed'); setNotice(`grant refused: ${out.error}`); return; }
      const id = String(out.grant.grant_id ?? '');
      settlement.applied(requestId, `grant ${id.slice(0, 12)} issued`);
      setGrantId(id);
      setNotice(`Grant issued: ${id}`);
    } catch (err) {
      if (requestId) settlement.unknown(requestId, err instanceof Error ? err.message : String(err));
      setNotice(`grant failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    setBusy(true);
    setNotice(null);
    let requestId: string | null = null;
    try {
      if (!source.trim() || !grantId.trim() || !reason.trim()) {
        setNotice('source, grant id, and reason are required.'); return;
      }
      requestId = settlement.begin('revoke grant');
      const sig = await signGrantRevoke(grantId.trim(), reason.trim());
      const out = await revokeTenantGrant({
        source_tenant: source.trim(), grant_id: grantId.trim(), reason: reason.trim(), ...sig,
      });
      if (!out.ok) { settlement[out.unknown ? 'unknown' : 'refused'](requestId!, out.error || 'not confirmed'); setNotice(`revoke refused: ${out.error}`); return; }
      settlement.applied(requestId, `grant ${grantId.trim().slice(0, 12)} revoked`);
      setNotice(`Grant ${grantId.trim()} revoked.`);
    } catch (err) {
      if (requestId) settlement.unknown(requestId, err instanceof Error ? err.message : String(err));
      setNotice(`revoke failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Tenant grants">
      <div>
        <div className="text-xs font-semibold text-[var(--color-text)]">Cross-lab grants</div>
        <p className="text-[12px] text-[var(--color-muted)]">Signed by the source lab admin; grants are read-scope and expire. No wildcard scopes.</p>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-1.5">
        <select aria-label="Grant source lab" value={source} onChange={(e) => setSource(e.target.value)} className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono">
          {labs.length === 0 && <option value="">no labs</option>}
          {labs.map((l) => <option key={l.id} value={l.id}>{l.display_name || l.id}</option>)}
        </select>
        <input aria-label="Grant target lab" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="target lab id" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono" />
        <input aria-label="Grant expiry hours" value={hours} onChange={(e) => setHours(e.target.value)} inputMode="numeric" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono" />
      </div>
      <div className="flex gap-1.5">
        <input aria-label="Grant scope" value={scope} onChange={(e) => setScope(e.target.value)} className="flex-1 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono" />
        <button type="button" onClick={() => void issue()} disabled={busy} className="px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] disabled:opacity-40">Issue grant</button>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto] gap-1.5">
        <input aria-label="Grant ID" value={grantId} onChange={(e) => setGrantId(e.target.value)} placeholder="grant id" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono" />
        <input aria-label="Grant revoke reason" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="revoke reason" className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)]" />
        <button type="button" onClick={() => void revoke()} disabled={busy} className="px-2 py-1 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px] disabled:opacity-40">Revoke grant</button>
      </div>
      {notice && <p role="status" className="text-[12px] text-[var(--color-text)] break-all">{notice}</p>}
      {writeSettlementLabel(settlement.write) && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">{writeSettlementLabel(settlement.write)}</p>
      )}
    </div>
  );
}
