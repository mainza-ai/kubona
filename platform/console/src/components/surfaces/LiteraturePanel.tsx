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
        <div className="flex items-center gap-2 flex-1 px-3 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] focus-within:border-[var(--color-info-edge)]">
          <Search className="w-3.5 h-3.5 text-[var(--color-muted)]" />
          <input
            type="text"
            aria-label="Search literature"
            placeholder="Ground this report: e.g. variational quantum eigensolver"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none"
          />
        </div>
        <button
          type="submit"
          disabled={!query.trim() || busy}
          className="px-3 py-2 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] disabled:opacity-50 text-xs font-semibold text-[var(--color-on-accent)] transition"
        >
          {busy ? 'Searching…' : 'Search'}
        </button>
      </form>

      {searched && !failed && (
        <div className="text-[12px] font-mono text-[var(--color-muted)]">
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
        <div className="flex items-center justify-between gap-2 p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
          <span>search unavailable — the backend did not answer. Nothing returned, nothing assumed.</span>
          <button
            type="button"
            onClick={() => void run()}
            disabled={busy}
            className="shrink-0 px-2.5 py-1 rounded-lg bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-text)] text-[12px] font-semibold disabled:opacity-50"
          >
            Retry
          </button>
        </div>
      )}

      <div className="space-y-2">
        {records.map((r, i) => (
          <div
            key={r.id || r.title || `record-${i}`}
            className="p-3 rounded-xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-1"
          >
            <div className="flex items-start justify-between gap-2">
              <span className="text-xs font-semibold text-[var(--color-text)] leading-snug">{r.title || 'Untitled record'}</span>
              {r.url && (
                <a
                  href={r.url}
                  target="_blank"
                  rel="noreferrer"
                  className="shrink-0 text-[var(--color-info-ink)] hover:opacity-80"
                  aria-label={`Open ${r.title}`}
                >
                  <ExternalLink className="w-3.5 h-3.5" />
                </a>
              )}
            </div>
            <div className="text-[12px] font-mono text-[var(--color-muted)] flex items-center gap-2">
              <BookOpen className="w-3 h-3" />
              <span className="truncate">{r.authors}</span>
              {r.publication_year > 0 && <span>· {r.publication_year}</span>}
              <span>· {typeof r.citation_count === 'number' && Number.isFinite(r.citation_count) ? r.citation_count : '—'} citations</span>
            </div>
            {r.abstract && (
              <p className="text-[12px] text-[var(--color-muted)] leading-relaxed">{r.abstract}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
