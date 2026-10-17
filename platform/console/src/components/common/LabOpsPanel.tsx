'use client';

import React, { useState } from 'react';
import {
  setTenantCeiling,
  setTenantStatus,
  refundTenantSpend,
  registerTenantAdminKey,
} from '@/lib/api';
import { signAdminAction } from '@/lib/adminAction';
import { getOrCreateSessionKeyPair, peekSessionPubkeyHex } from '@/lib/mandates';
import type { Lab } from '@/types';

// U1-b: per-lab operator writes. Every mutation carries BOTH rails: the
// bearer token (operator possession) and a lab-admin Ed25519 envelope over
// the exact parameters (lab-admin authority). Gating, all client-side
// before any network:
//   - no browser key → explain + point at the keyring (no silent mint);
//   - lab has no registered admin → offer bootstrap (first write wins);
//   - key mismatch → show both fingerprints, disable (never sign with the
//     wrong key — a mismatched signature is a confusing 403);
//   - suspended lab → only Resume is offered (server refuses the rest).
// After every accepted mutation the parent refetches (server owns state).
// Refusals render the backend reason verbatim.
export function LabOpsPanel({ lab, onChanged }: { lab: Lab; onChanged: () => void }) {
  const [ceiling, setCeiling] = useState('');
  const [refundAmount, setRefundAmount] = useState('');
  const [refundReason, setRefundReason] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const browserPub = peekSessionPubkeyHex();
  const registeredAdmin = (lab.admin_pubkey_hex || '').trim();
  const isBootstrap = registeredAdmin === '';
  const isAdmin = !isBootstrap && !!browserPub && registeredAdmin.toLowerCase() === browserPub.toLowerCase();
  const suspended = lab.status !== 'active';

  const run = async (kind: string, fn: () => Promise<{ ok: true } | { error: string } | null>) => {
    setBusy(kind);
    setNotice(null);
    try {
      const res = await fn();
      if (res === null) {
        setNotice('backend unreachable — nothing changed.');
      } else if ('error' in res) {
        setNotice(`refused: ${res.error}`);
      } else {
        setCeiling('');
        setRefundAmount('');
        setRefundReason('');
        onChanged();
      }
    } finally {
      setBusy(null);
    }
  };

  const envelope = (action: 'set_ceiling' | 'set_status' | 'refund_spend', params: Record<string, number | string>) => {
    // Reachable only when isAdmin (buttons gated below), so a keypair
    // exists in memory — getOrCreate returns it without generating.
    const keyPair = getOrCreateSessionKeyPair();
    return signAdminAction({ tenantId: lab.id, action, params, keyPair });
  };

  if (!browserPub) {
    return (
      <p className="text-[11px] text-slate-500">
        Operator writes need a browser key — create one in the keyring first (sovereign dock), then return here.
      </p>
    );
  }

  if (isBootstrap) {
    return (
      <div className="space-y-1.5">
        <p className="text-[11px] text-amber-200">
          No admin registered for this lab — first write wins (bootstrap, sealed in the ledger).
        </p>
        <button
          type="button"
          disabled={busy !== null}
          onClick={() =>
            run('bootstrap', () =>
              registerTenantAdminKey(lab.id, browserPub).then((r) =>
                r !== null && 'tenant' in r ? { ok: true as const } : r
              )
            )
          }
          className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 hover:bg-cyan-500/25 text-[11px] disabled:opacity-40"
        >
          Register this browser key ({browserPub.slice(0, 12)}…) as lab admin
        </button>
        {notice && <p className="text-[11px] text-amber-200" role="alert">{notice}</p>}
      </div>
    );
  }

  if (!isAdmin) {
    return (
      <p className="text-[11px] text-slate-500" title={`lab admin: ${registeredAdmin}\nbrowser key: ${browserPub}`}>
        This browser key ({browserPub.slice(0, 12)}…) is not this lab&apos;s admin ({registeredAdmin.slice(0, 12)}…) — writes disabled. Rotate the admin key or switch keys.
      </p>
    );
  }

  const ask = (msg: string) => confirm(msg);

  return (
    <div className="space-y-2 pt-1">
      <div className="flex items-center gap-1.5 text-[11px]">
        <span className="text-slate-400">admin</span>
        <span className="font-code text-emerald-300" title={registeredAdmin}>
          {registeredAdmin.slice(0, 12)}… (this browser key)
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input
          value={ceiling}
          onChange={(e) => setCeiling(e.target.value)}
          placeholder="new ceiling USD"
          inputMode="decimal"
          aria-label={`New ceiling for ${lab.id}`}
          className="w-32 px-2 py-1 rounded-lg bg-black/60 border border-white/10 text-[11px] font-code text-slate-200"
        />
        <button
          type="button"
          disabled={busy !== null || suspended}
          title={suspended ? 'suspended labs refuse ceiling changes — resume first' : 'Set spend ceiling'}
          onClick={() => {
            const v = Number(ceiling);
            if (!Number.isFinite(v) || v <= 0) {
              setNotice('ceiling must be a positive number.');
              return;
            }
            if (!ask(`Set ${lab.id} ceiling to $${v}?`)) return;
            void run('ceiling', () => setTenantCeiling(lab.id, v, envelope('set_ceiling', { ceiling_usd: v })));
          }}
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-200 disabled:opacity-40"
        >
          Set ceiling
        </button>
        <button
          type="button"
          disabled={busy !== null}
          onClick={() => {
            const next = suspended ? 'active' : 'suspended';
            if (!ask(suspended ? `Resume lab ${lab.id}? Spend gates re-open.` : `Suspend lab ${lab.id}? Spend-affecting actions freeze.`)) return;
            void run('status', () =>
              setTenantStatus(lab.id, next, envelope('set_status', { status: next }))
            );
          }}
          aria-label={`${suspended ? 'Resume' : 'Suspend'} lab ${lab.id}`}
          className={`px-2 py-1 rounded-lg border text-[11px] disabled:opacity-40 ${
            suspended
              ? 'bg-emerald-500/15 border-emerald-400/30 text-emerald-200 hover:bg-emerald-500/25'
              : 'bg-amber-500/15 border-amber-400/30 text-amber-200 hover:bg-amber-500/25'
          }`}
        >
          {suspended ? 'Resume' : 'Suspend'}
        </button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input
          value={refundAmount}
          onChange={(e) => setRefundAmount(e.target.value)}
          placeholder="refund USD"
          inputMode="decimal"
          aria-label={`Refund amount for ${lab.id}`}
          className="w-24 px-2 py-1 rounded-lg bg-black/60 border border-white/10 text-[11px] font-code text-slate-200"
        />
        <input
          value={refundReason}
          onChange={(e) => setRefundReason(e.target.value)}
          placeholder="reason (required)"
          aria-label={`Refund reason for ${lab.id}`}
          className="flex-1 min-w-32 px-2 py-1 rounded-lg bg-black/60 border border-white/10 text-[11px] text-slate-200"
        />
        <button
          type="button"
          disabled={busy !== null || suspended}
          title={suspended ? 'suspended labs refuse refunds — resume first' : 'Return consumed spend to the lab books (audited)'}
          onClick={() => {
            const v = Number(refundAmount);
            if (!Number.isFinite(v) || v <= 0) {
              setNotice('refund amount must be a positive number.');
              return;
            }
            if (!refundReason.trim()) {
              setNotice('refund needs a reason — unexplained money movement is refused.');
              return;
            }
            if (!ask(`Refund $${v} to ${lab.id} (“${refundReason.trim()}”)? Audited, irreversible.`)) return;
            const reason = refundReason.trim();
            void run('refund', () =>
              refundTenantSpend(lab.id, v, reason, envelope('refund_spend', { tenant_id: lab.id, amount_usd: v, reason }))
            );
          }}
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-200 disabled:opacity-40"
        >
          Refund
        </button>
      </div>
      {notice && (
        <p className="text-[11px] text-amber-200" role="alert">
          {notice}
        </p>
      )}
    </div>
  );
}
