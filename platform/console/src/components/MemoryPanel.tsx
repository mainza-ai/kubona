'use client';

import React, { useState } from 'react';
import { fetchMemoryRecall, retainMemory, type MemorySnippet } from '@/lib/api';
import { Tip } from './common/Tip';

const BANKS = ['experiences', 'world-facts', 'mental-models'] as const;

export type RetainedNoteReceipt = {
  id: string;
  bank: string;
  importance: number;
  content: string;
  retracted?: boolean;
};

// Session memory inspector: what this lab tenant's agent actually retains.
// Empty query lists newest-first (inventory, similarity shown as —);
// a query returns similarity-ranked matches. Operator notes retain into
// the selected bank at the chosen weight. Everything shown passed the
// tenant IFC check server-side; other tenants' rows never arrive here.
export function MemoryPanel({
  sessionId,
  defaultOpen = false,
  onRetained,
  onRetract,
}: {
  sessionId: string | null;
  defaultOpen?: boolean;
  onRetained?: (receipt: RetainedNoteReceipt) => void;
  onRetract?: (id: string) => Promise<{ ok: boolean; error?: string }>;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [query, setQuery] = useState('');
  const [bank, setBank] = useState<string>('experiences');
  const [snippets, setSnippets] = useState<MemorySnippet[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [note, setNote] = useState('');
  const [weight, setWeight] = useState('0.5');
  const [status, setStatus] = useState<string | null>(null);
  const [retainedReceipts, setRetainedReceipts] = useState<RetainedNoteReceipt[]>([]);
  const [lastSavedContent, setLastSavedContent] = useState<string | null>(null);
  const [retractingId, setRetractingId] = useState<string | null>(null);

  if (!sessionId) return null;

  const runRecall = async () => {
    setLoading(true);
    setStatus(null);
    try {
      const res = await fetchMemoryRecall(sessionId, query, { bank, limit: 20 });
      if (!res) {
        setStatus('Recall unavailable — nothing returned.');
        setSnippets(null);
        return;
      }
      setSnippets(res.snippets);
      if (res.snippets.length === 0) {
        setStatus(query.trim() ? 'No matches in this bank.' : 'Nothing retained yet in this bank.');
      }
    } finally {
      setLoading(false);
    }
  };

  const runRetain = async () => {
    const content = note.trim();
    if (!content) return;
    setStatus(null);
    const weightNum = parseFloat(weight);
    if (!Number.isFinite(weightNum)) {
      setStatus(`Retain refused — weight "${weight}" is not a number. Nothing stored.`);
      return;
    }
    const res = await retainMemory(sessionId, content, { bank, importance: weightNum });
    if (!res) {
      setStatus('Retain failed — nothing stored.');
      return;
    }
    const idRaw = typeof res.id === 'string' && res.id ? res.id : '';
    const idShort = idRaw ? `${idRaw.slice(0, 13)}…` : 'UNVERIFIED (no id)';
    const bankLabel = typeof res.bank === 'string' && res.bank ? res.bank : bank;
    const impLabel = typeof res.importance === 'number' && Number.isFinite(res.importance) ? res.importance : weightNum;
    const receipt: RetainedNoteReceipt = {
      id: idRaw || `unverified-${Date.now()}`,
      bank: bankLabel,
      importance: impLabel,
      content,
    };
    setLastSavedContent(content);
    setRetainedReceipts((prev) => [receipt, ...prev].slice(0, 5));
    onRetained?.(receipt);
    setNote('');
    setStatus(`Retained ${idShort} in ${bankLabel} at weight ${impLabel}.`);
    void runRecall();
  };

  const runUndoReceipt = async (receipt: RetainedNoteReceipt) => {
    if (!onRetract || !receipt.id || receipt.retracted || retractingId) return;
    setRetractingId(receipt.id);
    try {
      const out = await onRetract(receipt.id);
      if (out.ok) {
        setRetainedReceipts((prev) => prev.map((item) => (item.id === receipt.id ? { ...item, retracted: true } : item)));
        setStatus(`Retracted ${receipt.id.slice(0, 13)}… and restored draft.`);
        setNote(receipt.content);
        void runRecall();
      } else {
        setStatus(`Undo refused: ${out.error || 'backend did not confirm retraction.'}`);
      }
    } finally {
      setRetractingId(null);
    }
  };

  return (
    <div className="border-t border-[var(--color-border)]">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="w-full flex items-center justify-between px-5 py-2.5 text-[12px] text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
      >
        <span className="flex items-center gap-1.5">
          <span className="text-[var(--color-accent-ink)]">◈</span> SESSION MEMORY
        </span>
        <span className="font-code">{open ? '−' : '+'}</span>
      </button>
      {open && (
        <div className="px-5 pb-4 space-y-2.5">
          <div className="flex items-center gap-1.5">
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  if (e.nativeEvent?.isComposing || e.keyCode === 229) return;
                  void runRecall();
                }
              }}
              placeholder="Recall query (empty lists newest)"
              aria-label="Memory recall query"
              className="flex-1 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-accent-edge)]"
            />
            <select
              value={bank}
              onChange={(e) => setBank(e.target.value)}
              aria-label="Memory bank"
              className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-1.5 py-1.5 text-xs text-[var(--color-label-secondary)] focus:outline-none"
            >
              {BANKS.map((b) => (
                <option key={b} value={b}>{b}</option>
              ))}
            </select>
            <Tip label="Recall memory" sub="Similarity-ranked, tenant-scoped" position="top">
              <button
                onClick={() => void runRecall()}
                disabled={loading}
                aria-label="Recall memory"
                className="px-2.5 py-1.5 rounded-lg bg-[var(--color-accent-soft)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-xs transition-all disabled:opacity-40"
              >
                {loading ? '…' : 'Recall'}
              </button>
            </Tip>
          </div>

          {status && <div className="text-[12px] font-code text-[var(--color-muted)]">{status}</div>}

          {snippets !== null && snippets.length > 0 && (
            <ul className="space-y-1.5 max-h-48 overflow-y-auto">
              {snippets.map((s, i) => {
                const sim = typeof s.similarity === 'number' && Number.isFinite(s.similarity) ? `sim ${s.similarity.toFixed(2)}` : 'inventory';
                const when = (() => {
                  if (!s.created_at) return 'UNVERIFIED (no date)';
                  const d = new Date(s.created_at);
                  return Number.isNaN(d.getTime()) ? 'UNVERIFIED (bad date)' : d.toLocaleString();
                })();
                return (
                <li key={`${s.created_at}-${i}`} className="rounded-lg bg-[var(--color-surface)] border border-[var(--color-border)] px-2.5 py-1.5">
                  <div className="text-xs text-[var(--color-label-secondary)] leading-relaxed">{s.content}</div>
                  <div className="mt-0.5 text-[12px] font-code text-[var(--color-faint)]">
                    {s.bank} · {sim} · {when}
                  </div>
                </li>
                );
              })}
            </ul>
          )}

          <div className="flex items-center gap-1.5 pt-1">
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Retain an operator note…"
              aria-label="Operator note to retain"
              maxLength={4000}
              className="flex-1 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-accent-edge)]"
            />
            <select
              value={weight}
              onChange={(e) => setWeight(e.target.value)}
              aria-label="Note importance weight"
              title="Retention weight: low 0.3 · normal 0.5 · high 0.8"
              className="bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-1.5 py-1.5 text-xs text-[var(--color-label-secondary)] focus:outline-none"
            >
              <option value="0.3">low</option>
              <option value="0.5">normal</option>
              <option value="0.8">high</option>
            </select>
            <Tip label="Retain note" sub="Stored tenant-scoped, recallable" position="top">
              <button
                onClick={() => void runRetain()}
                disabled={!note.trim()}
                aria-label="Retain operator note"
                className="px-2.5 py-1.5 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] text-xs transition-all disabled:opacity-40"
              >
                Retain
              </button>
            </Tip>
            {lastSavedContent && !note.trim() && (
              <button
                type="button"
                onClick={() => setNote(lastSavedContent)}
                className="px-2 py-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-fill)] text-[12px] text-[var(--color-label-secondary)] hover:text-[var(--color-text)]"
                title="Restore last retained text into the draft field to supersede or revise"
              >
                Restore draft
              </button>
            )}
          </div>

          {retainedReceipts.length > 0 && (
            <ul className="space-y-1.5 pt-1" aria-label="Remembered receipts">
              {retainedReceipts.map((r) => (
                <li
                  key={r.id}
                  data-testid="remembered-receipt"
                  className="flex items-center justify-between gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-fill)] px-2.5 py-1.5 text-[12px]"
                >
                  <div className="min-w-0">
                    <div className="truncate font-medium text-[var(--color-label-secondary)]">
                      {r.retracted ? 'Retracted' : 'Remembered'} · {r.bank} · {r.id.slice(0, 12)}
                    </div>
                    <div className="truncate text-[var(--color-muted)]">{r.content}</div>
                  </div>
                  <button
                    type="button"
                    disabled={!onRetract || r.retracted || retractingId === r.id}
                    aria-disabled={!onRetract || r.retracted ? 'true' : undefined}
                    title={
                      r.retracted
                        ? 'Already retracted'
                        : onRetract
                          ? 'Retract this retained memory entry'
                          : 'Append-only ledger: server delete/retract endpoint is not exposed'
                    }
                    onClick={() => void runUndoReceipt(r)}
                    className="shrink-0 rounded-lg border border-[var(--color-border-strong)] bg-[var(--color-control)] px-2 py-1 text-[11px] text-[var(--color-label-secondary)] hover:bg-[var(--color-control-hover)] disabled:opacity-45"
                  >
                    {r.retracted ? 'Undone' : onRetract ? (retractingId === r.id ? 'Undoing…' : 'Undo') : 'Undo (ledger append-only)'}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
