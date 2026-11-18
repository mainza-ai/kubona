'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import {
  buildManuscript,
  createManuscript,
  createManuscriptAnnotation,
  fetchManuscript,
  fetchManuscripts,
  manuscriptArtifactUrl,
  reviewManuscript,
  setManuscriptAnnotationStatus,
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
    case 'deposit_ready':
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
  const [annotationAnchor, setAnnotationAnchor] = useState('general');
  const [annotationBody, setAnnotationBody] = useState('');

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

  const reloadSelected = async (id: string) => {
    if (!sessionId) return;
    const opened = await fetchManuscript(sessionId, id);
    if (opened.ok) setSelected(opened.data);
    else setError(opened.error);
  };

  const addAnnotation = async () => {
    const body = annotationBody.trim();
    if (!body || !selected || !sessionId) return;
    setBusy('annotate');
    setError(null);
    setNotice(null);
    const created = await createManuscriptAnnotation(
      sessionId, selected.manuscript.id, annotationAnchor, body,
    );
    if (!created.ok) {
      setError(created.error);
    } else {
      setAnnotationBody('');
      setNotice('Review note added.');
      await reloadSelected(selected.manuscript.id);
    }
    setBusy(null);
  };

  const resolveAnnotation = async (annotationId: string, status: 'addressed' | 'dismissed') => {
    if (!selected || !sessionId) return;
    setBusy(`annot-${annotationId}`);
    setError(null);
    setNotice(null);
    const updated = await setManuscriptAnnotationStatus(
      sessionId, selected.manuscript.id, annotationId, status,
    );
    if (!updated.ok) {
      setError(updated.error);
    } else {
      setSelected(updated.data);
      setNotice(status === 'addressed' ? 'Note marked addressed.' : 'Note dismissed.');
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
            const created = await createManuscript(sessionId, {
              request_id: typeof crypto !== 'undefined' && 'randomUUID' in crypto
                ? crypto.randomUUID()
                : `ms-${Date.now()}`,
            });
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
              disabled={selected.open_annotations > 0}
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
                  data: {
                    manuscript: selected.manuscript,
                    reviews: selected.reviews,
                    annotations: selected.annotations,
                    open_annotations: selected.open_annotations,
                  },
                };
              }, 'Zenodo manifest staged for a human upload (not published).')}
            >
              Stage deposit
            </Button>
          </div>
          {selected.open_annotations > 0 && (
            <p className="text-caption text-muted">
              Signoff is blocked while {selected.open_annotations} review note(s) are open.
            </p>
          )}
          {(!selected.manuscript.signoff || recommendationOf(selected) !== 'ACCEPT') && (
            <p className="text-caption text-muted">
              Deposit staging needs an ACCEPT review and a valid signoff. The review engine
              requires a comparator-linked kernel proof; until one is recorded, an honest
              manuscript stays at REVISE.
            </p>
          )}

          <div className="space-y-2 border-t border-border pt-3">
            <div className="flex items-center justify-between gap-2">
              <h5 className="text-label font-semibold">Review notes</h5>
              <Badge tone={selected.open_annotations > 0 ? 'warning' : 'neutral'}>
                {selected.open_annotations} open
              </Badge>
            </div>
            {selected.annotations.length === 0 && (
              <p className="text-caption text-muted">
                No notes. A note anchors to a section and blocks signoff until resolved.
              </p>
            )}
            <ul className="space-y-2 text-label">
              {selected.annotations.map((a) => (
                <li key={a.id} className="border border-border rounded-control px-3 py-2 space-y-1">
                  <div className="flex items-center gap-2">
                    <Badge tone={a.status === 'open' ? 'warning' : 'success'}>{a.status}</Badge>
                    <span className="text-caption text-muted">{a.anchor} · {a.author}</span>
                  </div>
                  <p>{a.body}</p>
                  {a.status === 'open' && (
                    <div className="flex gap-2">
                      <Button
                        size="compact"
                        pending={busy === `annot-${a.id}`}
                        onClick={() => void resolveAnnotation(a.id, 'addressed')}
                      >
                        Mark addressed
                      </Button>
                      <Button
                        size="compact"
                        variant="ghost"
                        onClick={() => void resolveAnnotation(a.id, 'dismissed')}
                      >
                        Dismiss
                      </Button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
            <form
              className="space-y-2"
              onSubmit={(e) => {
                e.preventDefault();
                void addAnnotation();
              }}
            >
              <div className="flex gap-2">
                <select
                  aria-label="Annotation section"
                  value={annotationAnchor}
                  onChange={(e) => setAnnotationAnchor(e.target.value)}
                  className="rounded-control border border-border bg-surface px-2 py-1 text-label"
                >
                  <option value="general">general</option>
                  {selected.manuscript.sections.map((s) => (
                    <option key={s.slug} value={s.slug}>{s.title}</option>
                  ))}
                </select>
                <input
                  aria-label="Review note"
                  value={annotationBody}
                  onChange={(e) => setAnnotationBody(e.target.value)}
                  placeholder="What needs attention?"
                  className="flex-1 rounded-control border border-border bg-surface px-2 py-1 text-label"
                />
                <Button size="compact" type="submit" pending={busy === 'annotate'} disabled={!annotationBody.trim()}>
                  Add note
                </Button>
              </div>
            </form>
          </div>

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
