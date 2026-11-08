'use client';

import React, { useEffect, useState } from 'react';
import { History, MessageSquare, RefreshCw } from 'lucide-react';
import { appendLedgerEvent, fetchClaimReviews, fetchClaims, reviewClaim, sendChatMessage, type ClaimRecord, type ClaimReview } from '@/lib/api';

function evidenceJob(record: ClaimRecord): string | null {
  try {
    const evidence = JSON.parse(record.evidence_json || '{}') as { job_id?: unknown };
    return typeof evidence.job_id === 'string' ? evidence.job_id : null;
  } catch {
    return null;
  }
}

export function ClaimHistory({ sessionId }: { sessionId: string | null }) {
  const [claims, setClaims] = useState<{ sessionId: string; rows: ClaimRecord[] } | null>(null);
  const [error, setError] = useState<{ sessionId: string; message: string } | null>(null);
  const [reviewing, setReviewing] = useState<string | null>(null);
  const [reviewed, setReviewed] = useState<Record<string, 'requested' | 'failed'>>({});
  const [reviews, setReviews] = useState<Record<string, ClaimReview[]>>({});
  const [reason, setReason] = useState('');
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    if (!sessionId) return () => { cancelled = true; };
    void fetchClaims(sessionId).then((rows) => {
      if (cancelled) return;
      if (rows === null) setError({ sessionId, message: 'claim history unavailable' });
      else {
        setError(null);
        setClaims({ sessionId, rows });
        void Promise.all(rows.map(async (claim) => [claim.id, (await fetchClaimReviews(sessionId, claim.id)) ?? []] as const))
          .then((pairs) => { if (!cancelled) setReviews(Object.fromEntries(pairs)); });
      }
    });
    return () => { cancelled = true; };
  }, [sessionId, nonce]);

  const rows = !sessionId ? [] : claims?.sessionId === sessionId ? claims.rows : null;
  const errorMessage = error?.sessionId === sessionId ? error.message : null;

  const requestReview = async (record: ClaimRecord) => {
    if (!sessionId) return;
    setReviewing(record.id);
    try {
      const sealed = await appendLedgerEvent(sessionId, 'peer_review', {
        claim_id: record.id,
        verdict: record.verdict,
        action: 'review_requested',
        statement: record.statement,
      });
      const result = await sendChatMessage(
        sessionId,
        `[Claim review request ${record.id}] Re-verify this recorded verdict against its evidence: ${record.statement}`,
      );
      const sealedOk = Boolean(sealed && (sealed as { ok?: boolean }).ok !== false);
      setReviewed((prev) => ({ ...prev, [record.id]: sealedOk && result ? 'requested' : 'failed' }));
    } catch {
      setReviewed((prev) => ({ ...prev, [record.id]: 'failed' }));
    } finally {
      setReviewing(null);
    }
  };

  const decide = async (record: ClaimRecord, decision: ClaimReview['decision']) => {
    if (!sessionId) return;
    if (!reason.trim()) { setError({ sessionId, message: 'a review reason is required' }); return; }
    setReviewing(record.id);
    try {
      const review = await reviewClaim(sessionId, record.id, decision, reason);
      if (!review) { setError({ sessionId, message: 'claim review was refused or failed' }); return; }
      setReviews((prev) => ({ ...prev, [record.id]: [review, ...(prev[record.id] ?? [])] }));
      setReason('');
      setError(null);
    } finally {
      setReviewing(null);
    }
  };

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 space-y-3" aria-label="Claim history">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <History className="w-4 h-4 text-[var(--color-pulse-cyan)]" />
          <h3 className="text-sm font-semibold text-[var(--color-text)]">Claim history</h3>
        </div>
        <button
          type="button"
          onClick={() => setNonce((n) => n + 1)}
          className="inline-flex items-center gap-1 rounded-lg border border-white/10 px-2 py-1 text-[11px] text-[var(--color-muted)] hover:text-[var(--color-text)]"
          aria-label="Refresh claim history"
        >
          <RefreshCw className="w-3 h-3" /> Refresh
        </button>
      </div>
      <label className="block text-[11px] text-[var(--color-muted)]">
        Review reason (required for a decision)
        <input
          aria-label="Claim review reason"
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          className="mt-1 w-full rounded-lg border border-white/10 bg-black/30 px-2 py-1 text-[12px] text-[var(--color-text)]"
          placeholder="Why this decision is justified"
        />
      </label>
      {errorMessage && <p role="alert" className="text-[12px] text-[var(--color-warning)]">{errorMessage}</p>}
      {rows === null && !errorMessage && <p className="text-[12px] text-[var(--color-muted)]">Loading claims…</p>}
      {rows !== null && rows.length === 0 && (
        <p className="text-[12px] text-[var(--color-muted)]">No evaluated claims recorded for this session.</p>
      )}
      {rows !== null && rows.length > 0 && (
        <ol className="space-y-2">
          {rows.map((claim) => {
            const job = evidenceJob(claim);
            const review = reviewed[claim.id];
            return (
              <li key={claim.id} className="rounded-xl border border-white/10 bg-black/20 p-3 space-y-1">
                <div className="flex flex-wrap items-center gap-2 text-[11px]">
                  <span className="font-code text-[var(--color-pulse-cyan)]">{claim.id}</span>
                  <span className="rounded-full border border-white/10 px-2 py-0.5 text-[var(--color-muted)]">{claim.turn_kind}</span>
                  <span className="rounded-full border border-white/10 px-2 py-0.5 text-[var(--color-text)]">{claim.verdict}</span>
                  {job && <span className="font-code text-[var(--color-muted)]">job {job.slice(0, 8)}</span>}
                  <span className="ml-auto text-[var(--color-faint)]">{claim.created_at}</span>
                </div>
                <p className="text-[12px] leading-relaxed text-[var(--color-text)]">{claim.statement}</p>
                <div className="flex items-center gap-3 text-[11px] text-[var(--color-muted)]">
                  <span>α {claim.alpha < 0 ? 'not evaluated' : claim.alpha.toFixed(2)}</span>
                  <span>β {claim.beta < 0 ? 'not evaluated' : claim.beta.toFixed(2)}</span>
                  <span>γ {claim.gamma < 0 ? 'not evaluated' : claim.gamma.toFixed(2)}</span>
                  {reviews[claim.id]?.[0] && (
                    <span className="rounded-full border border-white/10 px-2 py-0.5 text-[var(--color-text)]">
                      review: {reviews[claim.id][0].decision}
                    </span>
                  )}
                  {(['accepted', 'contested', 'reopened'] as const).map((decision) => (
                    <button
                      key={decision}
                      type="button"
                      disabled={reviewing === claim.id || !sessionId}
                      onClick={() => void decide(claim, decision)}
                      className="rounded-lg border border-white/10 px-2 py-1 text-[var(--color-muted)] disabled:opacity-40"
                    >
                      {decision}
                    </button>
                  ))}
                  <button
                    type="button"
                    disabled={reviewing === claim.id || !sessionId}
                    onClick={() => void requestReview(claim)}
                    className="ml-auto inline-flex items-center gap-1 rounded-lg border border-white/10 px-2 py-1 text-[var(--color-text)] disabled:opacity-40"
                  >
                    <MessageSquare className="w-3 h-3" />
                    {reviewing === claim.id ? 'Sending…' : review === 'requested' ? 'Review requested' : review === 'failed' ? 'Review failed' : 'Request review'}
                  </button>
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
