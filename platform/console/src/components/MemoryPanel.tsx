'use client';

import React, { useState } from 'react';
import { fetchMemoryRecall, retainMemory, type MemorySnippet } from '@/lib/api';
import { Tip } from './common/Tip';

const BANKS = ['experiences', 'world-facts', 'mental-models'] as const;

// Session memory inspector: what this lab tenant's agent actually retains.
// Empty query lists newest-first (inventory, similarity shown as —);
// a query returns similarity-ranked matches. Operator notes retain into
// the selected bank at the chosen weight. Everything shown passed the
// tenant IFC check server-side; other tenants' rows never arrive here.
export function MemoryPanel({ sessionId }: { sessionId: string | null }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [bank, setBank] = useState<string>('experiences');
  const [snippets, setSnippets] = useState<MemorySnippet[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [note, setNote] = useState('');
  const [weight, setWeight] = useState('0.5');
  const [status, setStatus] = useState<string | null>(null);

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
    const idShort = typeof res.id === 'string' && res.id ? `${res.id.slice(0, 13)}…` : 'UNVERIFIED (no id)';
    const bankLabel = typeof res.bank === 'string' && res.bank ? res.bank : bank;
    const impLabel = typeof res.importance === 'number' && Number.isFinite(res.importance) ? res.importance : weightNum;
    setNote('');
    setStatus(`Retained ${idShort} in ${bankLabel} at weight ${impLabel}.`);
    void runRecall();
  };

  return (
    <div className="border-t border-white/5">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="w-full flex items-center justify-between px-5 py-2.5 text-[11px] text-slate-400 hover:text-white transition-colors"
      >
        <span className="flex items-center gap-1.5">
          <span className="text-cyan-400">◈</span> SESSION MEMORY
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
                if (e.key === 'Enter') void runRecall();
              }}
              placeholder="Recall query (empty lists newest)"
              aria-label="Memory recall query"
              className="flex-1 bg-black/40 border border-white/10 rounded-lg px-2.5 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-cyan-400/40"
            />
            <select
              value={bank}
              onChange={(e) => setBank(e.target.value)}
              aria-label="Memory bank"
              className="bg-black/40 border border-white/10 rounded-lg px-1.5 py-1.5 text-xs text-slate-300 focus:outline-none"
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
                className="px-2.5 py-1.5 rounded-lg bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-400/30 text-cyan-200 text-xs transition-all disabled:opacity-40"
              >
                {loading ? '…' : 'Recall'}
              </button>
            </Tip>
          </div>

          {status && <div className="text-[11px] font-code text-slate-400">{status}</div>}

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
                <li key={`${s.created_at}-${i}`} className="rounded-lg bg-black/40 border border-white/10 px-2.5 py-1.5">
                  <div className="text-xs text-slate-200 leading-relaxed">{s.content}</div>
                  <div className="mt-0.5 text-[10px] font-code text-slate-500">
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
              className="flex-1 bg-black/40 border border-white/10 rounded-lg px-2.5 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-cyan-400/40"
            />
            <select
              value={weight}
              onChange={(e) => setWeight(e.target.value)}
              aria-label="Note importance weight"
              title="Retention weight: low 0.3 · normal 0.5 · high 0.8"
              className="bg-black/40 border border-white/10 rounded-lg px-1.5 py-1.5 text-xs text-slate-300 focus:outline-none"
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
                className="px-2.5 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 text-xs transition-all disabled:opacity-40"
              >
                Retain
              </button>
            </Tip>
          </div>
        </div>
      )}
    </div>
  );
}
