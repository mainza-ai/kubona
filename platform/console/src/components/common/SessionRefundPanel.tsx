'use client';

import React, { useState } from 'react';
import { refundSessionSpend } from '@/lib/api';
import { signAdminActionWithCustody } from '@/lib/adminAction';
import type { Lab, Session } from '@/types';

// Audited session refund. The tenant admin envelope binds tenant, session,
// amount, and reason; the server independently verifies the session's tenant.
export function SessionRefundPanel({ labs, sessions }: { labs: Lab[]; sessions: Session[] }) {
  const [sessionId, setSessionId] = useState(sessions[0]?.id ?? '');
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  const refund = async () => {
    setNotice(null);
    const s = sessions.find((x) => x.id === sessionId);
    if (!s) { setNotice('pick a session.'); return; }
    const v = Number(amount);
    if (!Number.isFinite(v) || v <= 0) { setNotice('amount must be positive.'); return; }
    if (!reason.trim()) { setNotice('reason is required.'); return; }
    const tenantId = s.tenant_id || labs[0]?.id || '';
    if (!tenantId) { setNotice('session has no tenant binding.'); return; }
    setBusy(true);
    try {
      const action = await signAdminActionWithCustody({
        tenantId,
        action: 'session_refund_spend',
        params: { tenant_id: tenantId, session_id: sessionId, amount_usd: v, reason: reason.trim() },
      });
      const out = await refundSessionSpend(sessionId, v, reason.trim(), action);
      if (!out.ok) { setNotice(`refund refused: ${out.error}`); return; }
      setNotice(`Refunded $${v.toFixed(2)} to ${sessionId.slice(0, 8)}.`);
      setAmount('');
      setReason('');
    } catch (err) {
      setNotice(`refund failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Session refunds">
      <div>
        <div className="text-xs font-semibold text-white">Session spend refund</div>
        <p className="text-[10px] text-slate-500">Admin-signed, tenant-bound, reason-required. Refunds return consumed spend; they do not alter vendor charges.</p>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
        <select aria-label="Refund session" value={sessionId} onChange={(e) => setSessionId(e.target.value)} className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono">
          {sessions.length === 0 && <option value="">no sessions</option>}
          {sessions.map((s) => <option key={s.id} value={s.id}>{s.title || s.id.slice(0, 12)}</option>)}
        </select>
        <input aria-label="Session refund amount" value={amount} onChange={(e) => setAmount(e.target.value)} inputMode="decimal" placeholder="amount USD" className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono" />
      </div>
      <div className="flex gap-1.5">
        <input aria-label="Session refund reason" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="reason (required)" className="flex-1 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200" />
        <button type="button" onClick={() => void refund()} disabled={busy} className="px-2 py-1 rounded-lg bg-rose-500/15 border border-rose-400/30 text-rose-200 text-[11px] disabled:opacity-40">Refund session</button>
      </div>
      {notice && <p role="status" className="text-[11px] text-slate-300">{notice}</p>}
    </div>
  );
}
