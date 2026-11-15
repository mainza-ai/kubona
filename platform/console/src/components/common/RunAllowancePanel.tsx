'use client';

import React, { useState } from 'react';
import { ShieldCheck, Trash2 } from 'lucide-react';
import {
  fetchRunAllowances,
  mintRunAllowance,
  revokeRunAllowance,
  type RunAllowanceView,
} from '@/lib/api';
import { signWithCustody } from '@/lib/custody';
import { useSharedResource } from '@/domain/shared-resource';
import { isMeteredBackend } from '@/lib/approvalPacket';
import { canonicalAllowanceScope, defaultAllowanceDraft, isoSeconds } from '@/lib/runAllowance';

const LAST_USED_KEY = 'kubona.run-allowance.last.v1';

function readLastUsed(): Parameters<typeof defaultAllowanceDraft>[1] {
  try {
    if (typeof window === 'undefined') return null;
    const raw = window.localStorage.getItem(LAST_USED_KEY);
    return raw ? (JSON.parse(raw) as Record<string, number>) : null;
  } catch {
    return null;
  }
}

function writeLastUsed(draft: { maxPerJobUsd: number; maxTotalUsd: number; maxJobs: number; shotsCap: number; ttlHours: number }) {
  try {
    if (typeof window !== 'undefined') window.localStorage.setItem(LAST_USED_KEY, JSON.stringify(draft));
  } catch {
    // Presentation preference only; losing it never hides work.
  }
}

/**
 * C11 — "allow for this run": mint a signed, bounded, expiring allowance for
 * the campaign's metered backend, select it for the launch, or revoke it.
 * The server verifies the canonical scope; this panel never decides admission.
 */
export function RunAllowancePanel({
  sessionId,
  tenantId,
  backend,
  perJobEstimateUsd,
  onSelect,
}: {
  sessionId: string;
  tenantId: string;
  backend: string;
  perJobEstimateUsd: number;
  onSelect: (allowance: RunAllowanceView | null) => void;
}) {
  const { data, error: listError, refresh } = useSharedResource<RunAllowanceView[]>(
    `run-allowances:${sessionId}`,
    async () => {
      if (!sessionId) return [];
      const out = await fetchRunAllowances(sessionId);
      if (!out.ok) throw new Error(out.error);
      return out.allowances;
    },
    { intervalMs: 15_000 },
  );
  const rows = data ?? [];
  const [selected, setSelected] = useState<string | null>(null);
  const [draft, setDraft] = useState(() => defaultAllowanceDraft(perJobEstimateUsd, readLastUsed()));
  const [busy, setBusy] = useState(false);
  const [receipt, setReceipt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const active = rows.filter((row) => row.status === 'active');
  const eligible = Boolean(sessionId) && isMeteredBackend(backend);

  const mint = async () => {
    if (!eligible || busy) return;
    setBusy(true);
    setError(null);
    setReceipt(null);
    try {
      const nonce = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `ra-${Date.now()}`;
      const expiresAt = isoSeconds(Date.now() + draft.ttlHours * 3600_000);
      const canonical = canonicalAllowanceScope({
        tenantId,
        sessionId,
        campaignId: sessionId,
        backend,
        jobTypes: ['circuit_run'],
        maxPerJobUsd: draft.maxPerJobUsd,
        maxTotalUsd: draft.maxTotalUsd,
        maxJobs: draft.maxJobs,
        shotsCap: draft.shotsCap,
        expiresAt,
        nonce,
      });
      const { publicKeyHex, signature } = await signWithCustody(new TextEncoder().encode(canonical));
      const signatureHex = Array.from(signature, (byte) => byte.toString(16).padStart(2, '0')).join('');
      const out = await mintRunAllowance({
        session_id: sessionId,
        campaign_id: sessionId,
        backend,
        job_types: ['circuit_run'],
        max_per_job_usd: draft.maxPerJobUsd,
        max_total_usd: draft.maxTotalUsd,
        max_jobs: draft.maxJobs,
        shots_cap: draft.shotsCap,
        expires_at: expiresAt,
        nonce,
        signer_pubkey_hex: publicKeyHex,
        signature_hex: signatureHex,
      });
      if (!out.ok) {
        setError(out.error);
        return;
      }
      writeLastUsed(draft);
      setReceipt(`Allowance active · $${out.allowance.max_total_usd.toFixed(2)} · ${out.allowance.max_jobs} jobs · until ${new Date(out.allowance.expires_at).toLocaleTimeString()}`);
      refresh();
      setSelected(out.allowance.id);
      onSelect(out.allowance);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (id: string) => {
    setBusy(true);
    setError(null);
    const out = await revokeRunAllowance(id, sessionId, 'revoked from the launch card');
    if (!out.ok) setError(out.error ?? 'revoke refused');
    if (selected === id) {
      setSelected(null);
      onSelect(null);
    }
    refresh();
    setBusy(false);
  };

  if (!eligible) {
    return (
      <p className="k-allowance-note" data-testid="run-allowance-ineligible">
        Run allowances cover metered hardware only; simulator runs need no allowance.
      </p>
    );
  }

  return (
    <section className="k-allowance" aria-label="Run allowance" data-testid="run-allowance">
      <div className="k-allowance-head">
        <ShieldCheck size={13} />
        <span>Allow for this run</span>
        <span className="k-allowance-sub">one signature · bounded · expiring</span>
      </div>
      <div className="k-allowance-grid">
        <label>
          <span>Per job $</span>
          <input
            type="number" min="0.01" step="0.01" value={draft.maxPerJobUsd}
            onChange={(e) => setDraft((d) => ({ ...d, maxPerJobUsd: Number(e.target.value) }))}
          />
        </label>
        <label>
          <span>Total $</span>
          <input
            type="number" min="0.01" step="0.01" value={draft.maxTotalUsd}
            onChange={(e) => setDraft((d) => ({ ...d, maxTotalUsd: Number(e.target.value) }))}
          />
        </label>
        <label>
          <span>Jobs</span>
          <input
            type="number" min="1" max="100" step="1" value={draft.maxJobs}
            onChange={(e) => setDraft((d) => ({ ...d, maxJobs: Number(e.target.value) }))}
          />
        </label>
        <label>
          <span>Shots cap</span>
          <input
            type="number" min="1" step="1" value={draft.shotsCap}
            onChange={(e) => setDraft((d) => ({ ...d, shotsCap: Number(e.target.value) }))}
          />
        </label>
        <label>
          <span>Hours</span>
          <input
            type="number" min="0.02" max="24" step="0.5" value={draft.ttlHours}
            onChange={(e) => setDraft((d) => ({ ...d, ttlHours: Number(e.target.value) }))}
          />
        </label>
      </div>
      <button
        type="button"
        className="k-confirm-btn-secondary"
        data-testid="run-allowance-mint"
        disabled={busy || !(draft.maxTotalUsd > 0) || !(draft.maxPerJobUsd > 0) || draft.maxPerJobUsd > draft.maxTotalUsd}
        onClick={() => void mint()}
      >
        {busy ? 'Signing allowance…' : 'Allow for this run'}
      </button>
      {receipt && <p role="status" data-testid="run-allowance-receipt">{receipt}</p>}
      {(error || listError) && <p role="status" data-state="error" data-testid="run-allowance-error">{error || listError}</p>}
      {active.length > 0 && (
        <ul className="k-allowance-list" aria-label="Active allowances">
          {active.map((row) => (
            <li key={row.id}>
              <label>
                <input
                  type="radio"
                  name="run-allowance"
                  checked={selected === row.id}
                  onChange={() => { setSelected(row.id); onSelect(row); }}
                  aria-label={`Use allowance ${row.id.slice(0, 8)}`}
                />
                <span>
                  ${row.remaining_usd.toFixed(2)} left · {row.max_jobs - row.jobs_used} jobs · {row.backend}
                </span>
              </label>
              <button type="button" aria-label={`Revoke allowance ${row.id.slice(0, 8)}`} onClick={() => void revoke(row.id)} disabled={busy}>
                <Trash2 size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
