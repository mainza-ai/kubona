'use client';

import React, { useState } from 'react';
import { BookOpen, Search, ExternalLink } from 'lucide-react';
import { LiteratureRecord } from '@/types';
import { searchLiterature } from '@/lib/api';

export function LiteraturePanel({ initialQuery = '' }: { initialQuery?: string }) {
  const [query, setQuery] = useState(initialQuery);
  const [records, setRecords] = useState<LiteratureRecord[]>([]);
  const [cached, setCached] = useState(false);
  const [searched, setSearched] = useState(false);
  const [busy, setBusy] = useState(false);

  async function run(e?: React.FormEvent) {
    e?.preventDefault();
    if (!query.trim() || busy) return;
    setBusy(true);
    const out = await searchLiterature(query.trim());
    setRecords(out.records);
    setCached(out.cached);
    setSearched(true);
    setBusy(false);
  }

  return (
    <div className="space-y-3">
      <form onSubmit={run} className="flex items-center space-x-2">
        <div className="flex items-center gap-2 flex-1 px-3 py-2 rounded-xl bg-neutral-900 border border-neutral-700 focus-within:border-cyan-500">
          <Search className="w-3.5 h-3.5 text-slate-400" />
          <input
            type="text"
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

      {searched && (
        <div className="text-[10px] font-mono text-slate-400">
          {records.length} record{records.length === 1 ? '' : 's'}
          {cached ? ' · served from local cache' : ' · live fetch'}
        </div>
      )}

      <div className="space-y-2">
        {records.map((r) => (
          <div
            key={r.id || r.title}
            className="p-3 rounded-xl bg-neutral-900/60 border border-neutral-800 space-y-1"
          >
            <div className="flex items-start justify-between gap-2">
              <span className="text-xs font-semibold text-white leading-snug">{r.title}</span>
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
              <span>· {r.citation_count} citations</span>
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
