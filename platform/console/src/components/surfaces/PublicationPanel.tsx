'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import {
  buildManuscript,
  createManuscript,
  fetchManuscript,
  fetchManuscripts,
  manuscriptArtifactUrl,
  reviewManuscript,
  signoffManuscript,
  stageManuscriptDeposit,
  type ManuscriptDetail,
  type ManuscriptSummary,
} from '@/lib/api';

interface PublicationPanelProps {
  sessionId: string | null;
}

function recommendationOf(detail: ManuscriptDetail): string {
  const rec = (detail.manuscript.review as { recommendation?: unknown })?.recommendation;
  return typeof rec === 'string' ? rec : 'REVISE';
}

function toneFor(state: string): 'neutral' | 'info' | 'success' | 'warning' {
  switch (state) {
    case 'signed':
      return 'success';
    case 'built':
      return 'info';
    case 'reviewed':
      return 'warning';
    default:
      return 'neutral';
  }
}

/**
 * Publication workflow: server-owned drafts, evidence-gated review, build,
 * operator signoff, and deposit staging. The panel never authors numbers and
 * never claims a paper was published — a staged deposit is a manifest for a
 * human upload.
 */
export function PublicationPanel({ sessionId }: PublicationPanelProps) {
  const [items, setItems] = useState<ManuscriptSummary[] | null>(null);
  const [selected, setSelected] = useState<ManuscriptDetail | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async (preferId?: string) => {
    if (!sessionId) {
      setItems([]);
      setSelected(null);
      return;
    }
    const listed = await fetchManuscripts(sessionId);
    if (!listed.ok) {
      setError(listed.error);
      setItems([]);
      return;
    }
    setItems(listed.data.manuscripts);
    const id = preferId ?? selected?.manuscript.id ?? listed.data.manuscripts[0]?.id;
    if (!id) {
      setSelected(null);
      return;
    }
    const detail = await fetchManuscript(sessionId, id);
    if (detail.ok) setSelected(detail.data);
    else setError(detail.error);
  }, [sessionId, selected?.manuscript.id]);

  useEffect(() => {
    let cancelled = false;
    async function initial() {
      if (!sessionId) return;
      const listed = await fetchManuscripts(sessionId);
      if (cancelled) return;
      if (!listed.ok) {
        setError(listed.error);
        setItems([]);
        return;
      }
      setItems(listed.data.manuscripts);
      const first = listed.data.manuscripts[0]?.id;
      if (!first) {
        setSelected(null);
        return;
      }
      const opened = await fetchManuscript(sessionId, first);
      if (!cancelled) {
        if (opened.ok) setSelected(opened.data);
        else setError(opened.error);
      }
    }
    void initial();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  const refreshList = useCallback(async () => {
    if (!sessionId) return;
    const listed = await fetchManuscripts(sessionId);
    if (listed.ok) setItems(listed.data.manuscripts);
  }, [sessionId]);

  const run = async (label: string, op: () => Promise<{ ok: true; data: ManuscriptDetail } | { ok: false; error: string }>, done: string) => {
    setBusy(label);
    setError(null);
    setNotice(null);
    const result = await op();
    if (!result.ok) {
      setError(result.error);
    } else {
      // The action response is authoritative for this manuscript; re-listing
      // updates the sidebar without racing a second detail read.
      setSelected(result.data);
      setNotice(done);
      await refreshList();
    }
    setBusy(null);
  };

  if (!sessionId) {
    return <EmptyState title="No session">Select a session to draft a manuscript.</EmptyState>;
  }

  return (
    <section className="space-y-4" aria-labelledby="publication-heading">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 id="publication-heading" className="text-title font-semibold">Publication</h3>
          <p className="text-label text-muted">
            Drafts are composed from the stored campaign projection and reviewed over ledger-resolved evidence.
          </p>
        </div>
        <Button
          size="compact"
          pending={busy === 'create'}
          onClick={() => void run('create', async () => {
            if (!sessionId) return { ok: false as const, error: 'no session' };
            const created = await createManuscript(sessionId, {});
            return created.ok ? created : { ok: false as const, error: created.error };
          }, 'Draft created from the stored campaign.')}
        >
          New draft
        </Button>
      </div>

      {error && (
        <p role="alert" className="text-label text-danger border border-danger rounded-control px-3 py-2">
          {error}
        </p>
      )}
      {notice && <p role="status" className="text-label text-muted">{notice}</p>}

      {items && items.length === 0 && (
        <EmptyState title="No manuscript yet">
          Create a draft after the campaign has measured results. The draft will contain only
          values the ledger recorded.
        </EmptyState>
      )}

      {items && items.length > 0 && (
        <ul className="space-y-1" aria-label="Manuscripts">
          {items.map((m) => (
            <li key={m.id}>
              <button
                type="button"
                aria-current={selected?.manuscript.id === m.id ? 'true' : undefined}
                onClick={() => void load(m.id)}
                className={`w-full text-left px-3 py-2 rounded-control border ${
                  selected?.manuscript.id === m.id ? 'border-accent bg-surface-elev' : 'border-border hover:bg-surface-elev'
                }`}
              >
                <span className="flex items-center gap-2">
                  <span className="text-label font-medium">{m.title}</span>
                  <Badge tone={toneFor(m.state)}>{m.state}</Badge>
                </span>
                <span className="block text-caption text-muted">
                  {m.content_hash.slice(0, 12)}… · updated {m.updated_at}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {selected && (
        <article className="space-y-4 border border-border rounded-card p-4">
          <header className="flex flex-wrap items-center gap-2">
            <h4 className="text-subtitle font-semibold">{selected.manuscript.title}</h4>
            <Badge tone={toneFor(selected.manuscript.state)}>{selected.manuscript.state}</Badge>
            <Badge tone={recommendationOf(selected) === 'ACCEPT' ? 'success' : 'warning'}>
              review: {recommendationOf(selected)}
            </Badge>
            {selected.manuscript.signoff && <Badge tone="success">signed</Badge>}
          </header>

          <dl className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-label">
            <div><dt className="text-muted">Content hash</dt><dd className="font-mono break-all">{selected.manuscript.content_hash}</dd></div>
            <div><dt className="text-muted">Build hash</dt><dd className="font-mono break-all">{selected.manuscript.build_hash || 'not built'}</dd></div>
            <div><dt className="text-muted">Sections</dt><dd>{selected.manuscript.sections.length}</dd></div>
            <div><dt className="text-muted">Artifacts</dt><dd>{selected.manuscript.artifacts.length}</dd></div>
          </dl>

          <div className="flex flex-wrap gap-2">
            <Button
              size="compact"
              pending={busy === 'review'}
              onClick={() => void run('review', async () => {
                const res = await reviewManuscript(sessionId, selected.manuscript.id);
                return res.ok ? res : { ok: false as const, error: res.error };
              }, 'Review recomputed over the stored source.')}
            >
              Re-review
            </Button>
            <Button
              size="compact"
              variant="primary"
              pending={busy === 'build'}
              onClick={() => void run('build', async () => {
                const res = await buildManuscript(sessionId, selected.manuscript.id);
                return res.ok ? res : { ok: false as const, error: res.error };
              }, 'Built the PDF from the stored source.')}
            >
              Build PDF
            </Button>
            <Button
              size="compact"
              pending={busy === 'sign'}
              onClick={() => void run('sign', async () => {
                const res = await signoffManuscript(sessionId, selected.manuscript.id, selected.manuscript.content_hash);
                return res.ok ? res : { ok: false as const, error: res.error };
              }, 'Signed off by the operator over the content hash.')}
            >
              Sign off (operator)
            </Button>
            <Button
              size="compact"
              variant="secondary"
              pending={busy === 'deposit'}
              disabled={!selected.manuscript.signoff || recommendationOf(selected) !== 'ACCEPT'}
              onClick={() => void run('deposit', async () => {
                const res = await stageManuscriptDeposit(sessionId, selected.manuscript.id);
                if (!res.ok) return { ok: false as const, error: res.error };
                return {
                  ok: true as const,
                  data: { manuscript: selected.manuscript, reviews: selected.reviews },
                };
              }, 'Zenodo manifest staged for a human upload (not published).')}
            >
              Stage deposit
            </Button>
          </div>
          {(!selected.manuscript.signoff || recommendationOf(selected) !== 'ACCEPT') && (
            <p className="text-caption text-muted">
              Deposit staging needs an ACCEPT review and a valid signoff. The review engine
              requires a comparator-linked kernel proof; until one is recorded, an honest
              manuscript stays at REVISE.
            </p>
          )}

          {selected.manuscript.artifacts.length > 0 && (
            <div>
              <h5 className="text-label font-semibold mb-1">Artifacts</h5>
              <ul className="space-y-1 text-label">
                {selected.manuscript.artifacts.map((a) => (
                  <li key={a.name} className="flex items-center justify-between gap-2">
                    <span className="font-mono break-all">{a.name}</span>
                    <a
                      className="underline text-accent"
                      href={manuscriptArtifactUrl(sessionId, selected.manuscript.id, a.name)}
                      download={a.name}
                    >
                      download
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          )}

          <details>
            <summary className="text-label cursor-pointer">Sections</summary>
            <ol className="mt-2 space-y-2 text-label">
              {selected.manuscript.sections.map((s) => (
                <li key={s.slug}>
                  <span className="font-medium">{s.title}</span>
                  <span className="block text-caption text-muted font-mono">{s.slug}</span>
                </li>
              ))}
            </ol>
          </details>
        </article>
      )}
    </section>
  );
}
