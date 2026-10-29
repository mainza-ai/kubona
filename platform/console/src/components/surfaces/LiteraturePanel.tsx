'use client';

import React, { useState } from 'react';
import { BookOpen, Search, ExternalLink } from 'lucide-react';
import { LiteratureRecord } from '@/types';
import { API_BASE, apiFetch, authHeaders } from '@/lib/api';

export function LiteraturePanel({ initialQuery = '' }: { initialQuery?: string }) {
  const [query, setQuery] = useState(initialQuery);
  const [records, setRecords] = useState<LiteratureRecord[]>([]);
  const [cached, setCached] = useState(false);
  const [searched, setSearched] = useState(false);
  const [busy, setBusy] = useState(false);
  // Honest empty: the searchLiterature wrapper collapses transport
  // failure and genuine emptiness into the same {records: [], cached:
  // false} shape, so this panel reads res.ok itself. failed=true means
  // "search unavailable — retry"; failed=false with zero records means
  // a genuine "no matches".
  const [failed, setFailed] = useState(false);

  async function run(e?: React.FormEvent) {
    e?.preventDefault();
    if (!query.trim() || busy) return;
    setBusy(true);
    setFailed(false);
    try {
      const res = await apiFetch(
        `${API_BASE}/api/literature?query=${encodeURIComponent(query.trim())}&limit=5`,
        { cache: 'no-store', headers: authHeaders() }
      );
      if (!res.ok) {
        setRecords([]);
        setCached(false);
        setFailed(true);
      } else {
        const data = (await res.json()) as { records?: unknown; cached?: unknown };
        const recs = Array.isArray(data.records) ? (data.records as LiteratureRecord[]) : [];
        setRecords(recs);
        setCached(data.cached === true);
      }
    } catch {
      setRecords([]);
      setCached(false);
      setFailed(true);
    } finally {
      setSearched(true);
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <form onSubmit={run} className="flex items-center space-x-2">
        <div className="flex items-center gap-2 flex-1 px-3 py-2 rounded-xl bg-neutral-900 border border-neutral-700 focus-within:border-cyan-500">
          <Search className="w-3.5 h-3.5 text-slate-400" />
          <input
            type="text"
            aria-label="Search literature"
            placeholder="Ground this report: e.g. variational quantum eigensolver"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent text-xs text-white placeholder-neutral-500 focus:outline-none"
          />
        </div>
        <button
          type="submit"
          disabled={!query.trim() || busy}
          className="px-3 py-2 rounded-xl bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 disabled:opacity-50 text-xs font-semibold text-white transition"
        >
          {busy ? 'Searching…' : 'Search'}
        </button>
      </form>

      {searched && !failed && (
        <div className="text-[10px] font-mono text-slate-400">
          {records.length} record{records.length === 1 ? '' : 's'}
          {/* Label invariant: the bridge never fabricates records, so a
              non-empty uncached result genuinely arrived live this session;
              an ok-but-empty result is "no matches" — never "unavailable". */}
          {records.length === 0
            ? ` · no matches for "${query.trim().slice(0, 80)}"`
            : cached
              ? ' · served from local cache'
              : ' · live fetch'}
        </div>
      )}

      {searched && failed && (
        <div className="flex items-center justify-between gap-2 p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
          <span>search unavailable — the backend did not answer. Nothing returned, nothing assumed.</span>
          <button
            type="button"
            onClick={() => void run()}
            disabled={busy}
            className="shrink-0 px-2.5 py-1 rounded-lg bg-white/[0.06] hover:bg-white/[0.1] border border-white/10 text-slate-200 text-[10px] font-semibold disabled:opacity-50"
          >
            Retry
          </button>
        </div>
      )}

      <div className="space-y-2">
        {records.map((r, i) => (
          <div
            key={r.id || r.title || `record-${i}`}
            className="p-3 rounded-xl bg-neutral-900/60 border border-neutral-800 space-y-1"
          >
            <div className="flex items-start justify-between gap-2">
              <span className="text-xs font-semibold text-white leading-snug">{r.title || 'Untitled record'}</span>
              {r.url && (
                <a
                  href={r.url}
                  target="_blank"
                  rel="noreferrer"
                  className="shrink-0 text-cyan-400 hover:text-cyan-300"
                  aria-label={`Open ${r.title}`}
                >
                  <ExternalLink className="w-3.5 h-3.5" />
                </a>
              )}
            </div>
            <div className="text-[10px] font-mono text-slate-400 flex items-center gap-2">
              <BookOpen className="w-3 h-3" />
              <span className="truncate">{r.authors}</span>
              {r.publication_year > 0 && <span>· {r.publication_year}</span>}
              <span>· {typeof r.citation_count === 'number' && Number.isFinite(r.citation_count) ? r.citation_count : '—'} citations</span>
            </div>
            {r.abstract && (
              <p className="text-[11px] text-neutral-400 leading-relaxed">{r.abstract}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
