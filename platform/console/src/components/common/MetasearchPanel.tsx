'use client';

import React, { useEffect, useState } from 'react';
import {
  approveMetasearchProposal,
  fetchMetasearchProposals,
  rejectMetasearchProposal,
  applyMetasearchProposal,
} from '@/lib/api';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import type { MetaProposal } from '@/types';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

// U1-d: human-approval surface for meta-search strategy proposals.
// pending_approval → Approve / Reject (reject confirms: it closes the
// proposal); approved → Apply (resumes loop work server-side).
// applied/rejected are terminal — no buttons, never a no-op call.
// Approve/reject/apply are bearer-gated: without an operator token the
// backend refuses and the reason renders (fail-closed, not hidden).
// After every accepted action the list refetches (server owns state).
export function MetasearchPanel() {
  const [proposals, setProposals] = useState<MetaProposal[] | null>(null);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [notice, setNotice] = useState<string | null>(null);
  const settlement = useWriteSettlement();
  // Wave-3: reject stages a typed ConfirmDialog (proposal short-id) instead
  // of a native confirm(). Approve/apply are unchanged.
  const [rejectDraft, setRejectDraft] = useState<string | null>(null);

  const load = async () => {
    const list = await fetchMetasearchProposals();
    setProposals(list);
  };

  useEffect(() => {
    let live = true;
    fetchMetasearchProposals().then((list) => {
      if (live) setProposals(list);
    });
    return () => {
      live = false;
    };
  }, []);

  const act = async (id: string, kind: 'approve' | 'reject' | 'apply') => {
    setBusy((prev) => new Set(prev).add(id));
    setNotice(null);
    const requestId = settlement.begin(`${kind} proposal ${id.slice(0, 8)}`);
    try {
      const res =
        kind === 'approve'
          ? await approveMetasearchProposal(id)
          : kind === 'reject'
            ? await rejectMetasearchProposal(id)
            : await applyMetasearchProposal(id);
      if (res === null) {
        settlement.unknown(requestId, `${kind} not confirmed — backend unreachable`);
      } else if ('error' in res) {
        settlement.refused(requestId, res.error);
      } else {
        settlement.applied(requestId, `${kind} proposal ${id.slice(0, 8)}`);
        await load();
      }
    } finally {
      setBusy((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  return (
    <div className="w-full rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-2" aria-label="Meta-search proposals">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-[var(--color-text)]">Strategy proposals · human approval</span>
        <button
          type="button"
          onClick={() => load()}
          className="px-2 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)]"
        >
          Refresh
        </button>
      </div>
      {writeSettlementLabel(settlement.write) && (
        <p role="status" className="text-[12px] font-code text-[var(--color-label-secondary)]">{writeSettlementLabel(settlement.write)}</p>
      )}
      {proposals === null && (
        <p className="text-[12px] font-code text-[var(--color-faint)]">Loading proposals…</p>
      )}
      {proposals !== null && proposals.length === 0 && (
        <p className="text-[12px] text-[var(--color-muted)]">No proposals pending — meta-search has proposed nothing.</p>
      )}
      {(proposals ?? []).map((p) => {
        const isBusy = busy.has(p.id);
        return (
          <div key={p.id} className="px-2.5 py-2 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-[12px] space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <span className="font-code text-[var(--color-label-secondary)] truncate" title={p.id}>
                {p.id.slice(0, 8)} · {p.status}
              </span>
              <span className="flex items-center gap-1.5 shrink-0">
                {p.status === 'pending_approval' && (
                  <>
                    <button
                      type="button"
                      disabled={isBusy}
                      onClick={() => act(p.id, 'approve')}
                      aria-label={`Approve proposal ${p.id}`}
                      className="px-2 py-1 rounded-lg bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[var(--color-success-ink)] hover:opacity-90 disabled:opacity-40"
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      disabled={isBusy}
                      onClick={() => setRejectDraft(p.id)}
                      aria-label={`Reject proposal ${p.id}`}
                      className="px-2 py-1 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] hover:opacity-90 disabled:opacity-40"
                    >
                      Reject
                    </button>
                  </>
                )}
                {p.status === 'approved' && (
                  <button
                    type="button"
                    disabled={isBusy}
                    onClick={() => act(p.id, 'apply')}
                    aria-label={`Apply proposal ${p.id}`}
                    className="px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] hover:opacity-90 disabled:opacity-40"
                  >
                    Apply
                  </button>
                )}
              </span>
            </div>
            <p className="text-[var(--color-muted)] truncate" title={p.rationale}>
              {p.rationale || p.target_file}
            </p>
            <details className="text-[var(--color-faint)]">
              <summary className="cursor-pointer hover:text-[var(--color-label-secondary)]">proposed diff</summary>
              <pre className="mt-1 p-2 rounded-lg bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] overflow-x-auto font-code text-[12px] whitespace-pre-wrap">
                {p.proposed_diff || '(no diff recorded)'}
              </pre>
            </details>
          </div>
        );
      })}
      {notice && (
        <div className="text-[12px] text-[var(--color-warning-ink)]" role="alert">
          {notice}
        </div>
      )}
      {rejectDraft != null && (
        <ConfirmDialog
          open
          title={`Reject proposal ${rejectDraft.slice(0, 8)}?`}
          body={
            <span>
              The strategy change is closed, loop work stays as-is. Type{' '}
              <code className="rounded bg-[var(--color-fill)] px-1 font-mono">{rejectDraft.slice(0, 8)}</code> to confirm.
            </span>
          }
          requireText={rejectDraft.slice(0, 8)}
          confirmLabel="Reject proposal"
          danger
          busy={busy.has(rejectDraft)}
          onConfirm={() => {
            const id = rejectDraft;
            setRejectDraft(null);
            void act(id, 'reject');
          }}
          onCancel={() => {
            if (rejectDraft != null && busy.has(rejectDraft)) return;
            setRejectDraft(null);
          }}
        />
      )}
    </div>
  );
}
