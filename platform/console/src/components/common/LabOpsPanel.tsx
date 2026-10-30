'use client';

import React, { useEffect, useState } from 'react';
import {
  setTenantCeiling,
  setTenantStatus,
  refundTenantSpend,
  registerTenantAdminKey,
  setTenantAutonomy,
  revokeTenantAutonomy,
} from '@/lib/api';
import { signAdminActionWithCustody, type AdminAction } from '@/lib/adminAction';
import { getOrCreateSigner, peekSignerPubkeyHex } from '@/lib/custody';
import ConfirmDialog from '@/components/common/ConfirmDialog';
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
  const [autonomyDraft, setAutonomyDraft] = useState(lab.autonomy_level);
  const [revokeReason, setRevokeReason] = useState('');
  const [revokeDraft, setRevokeDraft] = useState<string | null>(null);
  const [refundAmount, setRefundAmount] = useState('');
  const [refundReason, setRefundReason] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // Wave-2 typed gate for the IRREVERSIBLE refund: native confirm() is
  // invisible to screen readers and carries no typed acknowledgement, so
  // the refund stages a ConfirmDialog requiring the exact "$X.XX" amount.
  // Backend refusals surface in the dialog error slot with no silent close.
  const [refundDraft, setRefundDraft] = useState<{ amount: number; reason: string } | null>(null);
  const [refundBusy, setRefundBusy] = useState(false);
  const [refundError, setRefundError] = useState<string | null>(null);
  // Wave-3: ceiling + suspend/resume stage a ConfirmDialog (no typed text —
  // reversible, so a single explicit confirm suffices per the ConfirmDialog
  // contract). The refund above keeps the typed "$X.XX" gate: irreversible.
  const [ceilingDraft, setCeilingDraft] = useState<number | null>(null);
  const [statusDraft, setStatusDraft] = useState<'active' | 'suspended' | null>(null);

  const [browserPub, setBrowserPub] = useState<string | null>(null);
  // First-run bootstrap is the one tenant write that cannot use a lab-admin
  // session (none exists yet). The backend still requires the operator
  // bearer on this route, and production must not bake that token into the
  // bundle. The operator pastes it once; it is held only in component state.
  const [bootstrapToken, setBootstrapToken] = useState('');
  useEffect(() => {
    let active = true;
    peekSignerPubkeyHex().then((pub) => { if (active) setBrowserPub(pub); });
    return () => { active = false; };
  }, []);
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

  const envelope = async (action: 'set_ceiling' | 'set_status' | 'refund_spend' | 'grant_autonomy' | 'revoke_autonomy',
                           params: Record<string, number | string>): Promise<AdminAction | undefined> => {
    // When this browser holds the lab admin key we send the signed envelope;
    // otherwise the server uses the signed-in lab-admin session cookie
    // (Q-56). The server always re-checks authority.
    if (!browserPub || !isAdmin) return undefined;
    return signAdminActionWithCustody({ tenantId: lab.id, action, params });
  };

  if (isBootstrap) {
    const createKeyAndRegister = async () => {
      setBusy('bootstrap');
      setNotice(null);
      try {
        const signer = await getOrCreateSigner();
        setBrowserPub(signer.publicKeyHex);
        const res = await registerTenantAdminKey(lab.id, signer.publicKeyHex, undefined, bootstrapToken);
        if (res === null) {
          setNotice('backend unreachable — nothing changed.');
        } else if ('error' in res) {
          setNotice(`refused: ${res.error}`);
        } else {
          setBootstrapToken('');
          onChanged();
        }
      } catch (e) {
        setNotice(`failed: ${e instanceof Error ? e.message : String(e)}`);
      } finally {
        setBusy(null);
      }
    };
    return (
      <div className="space-y-1.5">
        <p className="text-[11px] text-amber-200">
          No admin registered for this lab — first write wins (bootstrap, sealed in the ledger).
        </p>
        <label htmlFor={`lab-bootstrap-token-${lab.id}`} className="block text-[10px] text-slate-400">
          Operator token (KUBONA_API_TOKEN; used once, not stored)
        </label>
        <input
          id={`lab-bootstrap-token-${lab.id}`}
          type="password"
          value={bootstrapToken}
          onChange={(e) => setBootstrapToken(e.target.value)}
          aria-label="Operator token"
          autoComplete="off"
          className="w-full bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono"
        />
        {!browserPub && (
          <p className="text-[11px] text-slate-500">
            No browser key yet. Creating one is an explicit action; it mints the
            non-extractable Ed25519 key that will become this lab&apos;s admin.
          </p>
        )}
        <button
          type="button"
          disabled={busy !== null || !bootstrapToken.trim()}
          onClick={() => {
            if (browserPub) {
              void run('bootstrap', () =>
                registerTenantAdminKey(lab.id, browserPub, undefined, bootstrapToken).then((r) =>
                  r !== null && 'tenant' in r ? { ok: true as const } : r
                )
              );
            } else {
              void createKeyAndRegister();
            }
          }}
          className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 hover:bg-cyan-500/25 text-[11px] disabled:opacity-40"
        >
          {browserPub
            ? `Register this browser key (${browserPub.slice(0, 12)}…) as lab admin`
            : 'Create browser key & register as lab admin'}
        </button>
        {notice && <p className="text-[11px] text-amber-200" role="alert">{notice}</p>}
      </div>
    );
  }

  const refundAmountText = refundDraft != null ? `$${refundDraft.amount.toFixed(2)}` : null;

  const confirmRefund = async () => {
    if (!refundDraft || refundBusy) return;
    const { amount, reason } = refundDraft;
    setRefundBusy(true);
    setRefundError(null);
    try {
      const res = await refundTenantSpend(
        lab.id,
        amount,
        reason,
        await envelope('refund_spend', { tenant_id: lab.id, amount_usd: amount, reason })
      );
      if (res === null) {
        // Transport loss: stay open, surface in the dialog error slot.
        setRefundError('backend unreachable — nothing changed.');
        return;
      }
      if ('error' in res) {
        setRefundError(`refused: ${res.error}`);
        return;
      }
      setCeiling('');
      setRefundAmount('');
      setRefundReason('');
      setRefundDraft(null);
      setRefundError(null);
      onChanged();
    } finally {
      setRefundBusy(false);
    }
  };

  return (
    <div className="space-y-2 pt-1">
      <div className="flex items-center gap-1.5 text-[11px]">
        <span className="text-slate-400">admin</span>
        <span className="font-code text-emerald-300" title={registeredAdmin}>
          {registeredAdmin.slice(0, 12)}… {isAdmin ? '(this browser key)' : '(signed-in lab-admin session)'}
        </span>
      </div>
      {!isAdmin && (
        <p className="text-[11px] text-slate-500" title={`lab admin: ${registeredAdmin}\nbrowser key: ${browserPub ?? 'none'}`}>
          Browser key {browserPub ? `${browserPub.slice(0, 12)}… ` : ''}is not this lab&apos;s admin. Writes use the signed-in lab-admin session; the server enforces it.
        </p>
      )}
      <div className="flex flex-wrap items-center gap-1.5">
        <label htmlFor={`lab-autonomy-${lab.id}`} className="text-[11px] text-slate-400">Autonomy</label>
        <select
          id={`lab-autonomy-${lab.id}`}
          value={autonomyDraft}
          onChange={(e) => setAutonomyDraft(e.target.value)}
          disabled={busy !== null || suspended}
          className="px-2 py-1 rounded-lg bg-black/60 border border-white/10 text-[11px] font-code text-slate-200 disabled:opacity-40"
        >
          {['L0', 'L1', 'L2', 'L3'].map((level) => <option key={level} value={level}>{level}</option>)}
        </select>
        <button
          type="button"
          disabled={busy !== null || suspended || autonomyDraft === lab.autonomy_level}
          title={suspended ? 'suspended labs cannot change autonomy — resume first' : 'Persist the autonomy level (server enforces dispatch)'}
          onClick={() => void run('autonomy', async () => {
            const action = await envelope('grant_autonomy', { level: autonomyDraft });
            return setTenantAutonomy(lab.id, autonomyDraft as 'L0' | 'L1' | 'L2' | 'L3', action);
          })}
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-200 disabled:opacity-40"
        >
          Set autonomy
        </button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input
          value={revokeReason}
          onChange={(e) => setRevokeReason(e.target.value)}
          placeholder="revoke autonomy reason (required)"
          aria-label={`Revoke reason for ${lab.id}`}
          className="flex-1 min-w-40 px-2 py-1 rounded-lg bg-black/60 border border-white/10 text-[11px] text-slate-200"
        />
        <button
          type="button"
          disabled={busy !== null}
          onClick={() => {
            if (!revokeReason.trim()) { setNotice('revoke needs a reason.'); return; }
            setRevokeDraft(revokeReason.trim());
          }}
          className="px-2 py-1 rounded-lg bg-rose-500/15 border border-rose-400/30 text-rose-200 text-[11px] disabled:opacity-40"
        >
          Revoke autonomy
        </button>
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
              setCeilingDraft(v);
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
              setStatusDraft(next);
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
              // Stage the typed gate — no network until the exact "$X.XX"
              // acknowledgement lands in the dialog.
              setRefundError(null);
              setRefundDraft({ amount: v, reason: refundReason.trim() });
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
      {revokeDraft != null && (
        <ConfirmDialog
          open
          title={`Revoke autonomy for ${lab.id}?`}
          body={<span>This aborts every unsettled job in this lab and drops it to L0. Type the lab id to confirm.</span>}
          requireText={lab.id}
          confirmLabel="Revoke autonomy"
          danger
          busy={busy !== null}
          onConfirm={() => void (async () => {
            const reason = revokeDraft;
            setRevokeDraft(null);
            await run('revoke', async () => {
              const action = await envelope('revoke_autonomy', { reason });
              return revokeTenantAutonomy(lab.id, reason, action);
            });
            setRevokeReason('');
          })()}
          onCancel={() => { if (busy === null) setRevokeDraft(null); }}
        />
      )}
      {ceilingDraft != null && (
        <ConfirmDialog
          open
          title={`Set ${lab.id} ceiling to $${ceilingDraft.toFixed(2)}?`}
          body={
            <span>
              Set the spend ceiling for <strong className="font-semibold text-white">{lab.id}</strong> to{' '}
              <strong className="font-semibold text-white">${ceilingDraft.toFixed(2)}</strong>. Reversible — the
              ceiling can be raised or lowered again.
            </span>
          }
          confirmLabel="Set ceiling"
          busy={busy !== null}
          onConfirm={() =>
            void (async () => {
              const v = ceilingDraft;
              setCeilingDraft(null);
              await run('ceiling', async () =>
                setTenantCeiling(lab.id, v, await envelope('set_ceiling', { ceiling_usd: v }))
              );
            })()
          }
          onCancel={() => {
            if (busy !== null) return;
            setCeilingDraft(null);
          }}
        />
      )}
      {statusDraft != null && (
        <ConfirmDialog
          open
          title={statusDraft === 'active' ? `Resume lab ${lab.id}?` : `Suspend lab ${lab.id}?`}
          body={
            <span>
              {statusDraft === 'active'
                ? 'Spend gates re-open. Reversible — the lab can be suspended again.'
                : 'Spend-affecting actions freeze. Reversible — resume re-opens the gates.'}
            </span>
          }
          confirmLabel={statusDraft === 'active' ? 'Resume lab' : 'Suspend lab'}
          danger={statusDraft !== 'active'}
          busy={busy !== null}
          onConfirm={() =>
            void (async () => {
              const next = statusDraft;
              setStatusDraft(null);
              await run('status', async () =>
                setTenantStatus(lab.id, next, await envelope('set_status', { status: next }))
              );
            })()
          }
          onCancel={() => {
            if (busy !== null) return;
            setStatusDraft(null);
          }}
        />
      )}
      {refundDraft != null && refundAmountText != null && (
        <ConfirmDialog
          open
          title={`Refund ${refundAmountText} to ${lab.id}?`}
          body={
            <span>
              Return <strong className="font-semibold text-white">{refundAmountText}</strong> to {lab.id} (“
              {refundDraft.reason}”). Audited, irreversible — the ledger keeps this refund forever.
            </span>
          }
          requireText={refundAmountText}
          confirmLabel={`Refund ${refundAmountText}`}
          danger
          busy={refundBusy || busy !== null}
          error={refundError}
          onConfirm={() => void confirmRefund()}
          onCancel={() => {
            if (refundBusy || busy !== null) return;
            setRefundDraft(null);
            setRefundError(null);
          }}
        />
      )}
    </div>
  );
}
