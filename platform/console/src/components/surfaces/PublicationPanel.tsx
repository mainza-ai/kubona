'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Field, Select, TextArea } from '@/components/ui/Forms';
import { EmptyState } from '@/components/ui/EmptyState';
import {
  buildManuscript,
  createManuscript,
  createManuscriptAnnotation,
  deleteManuscript,
  downloadManuscriptArtifact,
  exportManuscriptBundle,
  fetchFormalVerifications,
  fetchManuscript,
  fetchManuscripts,
  replaceManuscriptCitations,
  reviewManuscript,
  searchLiterature,
  setManuscriptAnnotationStatus,
  signoffManuscript,
  stageManuscriptDeposit,
  updateManuscriptDraft,
  verifyManuscript,
  type FormalVerificationEntry,
  type ManuscriptDetail,
  type ManuscriptSummary,
  type ManuscriptVerificationReport,
} from '@/domain/publication';
import type { LiteratureRecord } from '@/types';

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
  const [litQuery, setLitQuery] = useState('');
  const [litResults, setLitResults] = useState<LiteratureRecord[]>([]);
  const [litBusy, setLitBusy] = useState(false);
  const [verification, setVerification] = useState<ManuscriptVerificationReport | null>(null);
  const [depositManifest, setDepositManifest] = useState<Record<string, unknown> | null>(null);
  const [editingDraft, setEditingDraft] = useState(false);
  const [draftTitle, setDraftTitle] = useState('');
  const [draftAbstract, setDraftAbstract] = useState('');
  const [formalProofs, setFormalProofs] = useState<FormalVerificationEntry[] | null>(null);

  const downloadJson = (filename: string, payload: unknown) => {
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

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

  const saveCitations = async (next: Array<Record<string, unknown>>) => {
    if (!sessionId || !selected) return;
    setBusy('citations');
    setError(null);
    setNotice(null);
    const res = await replaceManuscriptCitations(sessionId, selected.manuscript.id, next as never);
    if (!res.ok) setError(res.error);
    else {
      setSelected(res.data);
      setNotice('References updated; review reset for the draft.');
    }
    setBusy(null);
  };

  const addReference = async (record: LiteratureRecord) => {
    if (!selected) return;
    const existing = selected.manuscript.citations.map((c) => ({
      key: c.key, title: c.title, authors: c.authors, year: c.year, doi: c.doi,
      ...(c.claim_id ? { claim_id: c.claim_id } : {}),
      ...(c.source_id ? { source_id: c.source_id } : {}),
    }));
    const next = [...existing, {
      key: record.id,
      title: record.title,
      authors: record.authors,
      year: String(record.publication_year ?? ''),
      source_id: record.id,
    }];
    setLitResults([]);
    setLitQuery('');
    await saveCitations(next);
  };

  const removeReference = async (key: string) => {
    if (!selected) return;
    const next = selected.manuscript.citations
      .filter((c) => c.key !== key)
      .map((c) => ({
        key: c.key, title: c.title, authors: c.authors, year: c.year, doi: c.doi,
        ...(c.claim_id ? { claim_id: c.claim_id } : {}),
        ...(c.source_id ? { source_id: c.source_id } : {}),
      }));
    await saveCitations(next);
  };

  const searchReferences = async () => {
    if (!litQuery.trim()) return;
    setLitBusy(true);
    setError(null);
    const res = await searchLiterature(litQuery.trim(), 5);
    setLitResults(res.records);
    if (res.records.length === 0) setError('No literature records found (the bridge may be offline).');
    setLitBusy(false);
  };

  const downloadArtifact = async (name: string) => {
    if (!sessionId || !selected) return;
    setBusy(`download-${name}`);
    setError(null);
    const result = await downloadManuscriptArtifact(sessionId, selected.manuscript.id, name);
    if (!result.ok) setError(result.error);
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
              <Button
                variant="ghost"
                aria-current={selected?.manuscript.id === m.id ? 'true' : undefined}
                onClick={() => void load(m.id)}
                className={`w-full justify-start text-left px-3 py-2 rounded-control border ${
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
              </Button>
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
              pending={busy === 'verify'}
              onClick={() => void (async () => {
                setBusy('verify');
                setError(null);
                setNotice(null);
                const res = await verifyManuscript(sessionId, selected.manuscript.id);
                if (!res.ok) {
                  setError(res.error);
                } else {
                  setVerification(res.data);
                  setNotice(`Verification finished: ${res.data.report?.summary?.status ?? 'checked'}.`);
                }
                setBusy(null);
              })()}
            >
              Verify numbers &amp; citations
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
              pending={busy === 'export-rocrate'}
              onClick={() => void (async () => {
                setBusy('export-rocrate');
                setError(null);
                setNotice(null);
                const res = await exportManuscriptBundle(sessionId, selected.manuscript.id);
                if (!res.ok) {
                  setError(res.error);
                } else {
                  downloadJson(`kubona-rocrate-${selected.manuscript.id}.json`, res.data);
                  setNotice('Exported RO-Crate 1.1 + W3C PROV-JSON + CRediT bundle.');
                }
                setBusy(null);
              })()}
            >
              Export RO-Crate / PROV
            </Button>
            {selected.manuscript.state === 'draft' && (
              <>
                <Button
                  size="compact"
                  variant="ghost"
                  onClick={() => {
                    setDraftTitle(selected.manuscript.title);
                    setDraftAbstract(selected.manuscript.abstract);
                    setEditingDraft((v) => !v);
                  }}
                >
                  {editingDraft ? 'Cancel edit' : 'Edit draft'}
                </Button>
                <Button
                  size="compact"
                  variant="ghost"
                  pending={busy === 'delete'}
                  onClick={() => void (async () => {
                    setBusy('delete');
                    setError(null);
                    setNotice(null);
                    const res = await deleteManuscript(sessionId, selected.manuscript.id);
                    if (!res.ok) {
                      setError(res.error);
                    } else {
                      setSelected(null);
                      setVerification(null);
                      setDepositManifest(null);
                      setNotice('Draft deleted.');
                      await load();
                    }
                    setBusy(null);
                  })()}
                >
                  Delete draft
                </Button>
              </>
            )}
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
                setDepositManifest(res.data.manifest);
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

          {editingDraft && selected.manuscript.state === 'draft' && (
            <div className="space-y-2 border border-border rounded-control p-3 bg-surface">
              <Field
                label="Draft title"
                value={draftTitle}
                onChange={(e) => setDraftTitle(e.target.value)}
              />
              <TextArea
                label="Draft abstract"
                value={draftAbstract}
                onChange={(e) => setDraftAbstract(e.target.value)}
                rows={4}
              />
              <div className="flex gap-2">
                <Button
                  size="compact"
                  variant="primary"
                  pending={busy === 'save-draft'}
                  onClick={() => void run('save-draft', async () => {
                    const res = await updateManuscriptDraft(sessionId, selected.manuscript.id, {
                      title: draftTitle,
                      abstract: draftAbstract,
                    });
                    if (res.ok) setEditingDraft(false);
                    return res.ok ? res : { ok: false as const, error: res.error };
                  }, 'Draft updated; review reset.')}
                >
                  Save draft
                </Button>
                <Button size="compact" variant="ghost" onClick={() => setEditingDraft(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          )}

          {verification?.report && (
            <div className="border border-border rounded-control p-3 space-y-2 text-label bg-surface">
              <div className="flex items-center justify-between gap-2">
                <span className="font-semibold">Numeric &amp; citation verification</span>
                <Badge tone={verification.report.summary?.status === 'pass' ? 'success' : 'warning'}>
                  {verification.report.summary?.status ?? 'checked'}
                </Badge>
              </div>
              <p className="text-caption text-muted">
                Numbers checked: {verification.report.summary?.numbers_checked ?? verification.report.numbers?.length ?? 0}
                {' · '}Citations checked: {verification.report.summary?.citations_checked ?? verification.report.citations?.length ?? 0}
                {' · '}Blocking: {verification.report.summary?.blocking ?? 0}
              </p>
              {(verification.report.numbers ?? []).filter((n) => n.status !== 'verified').length > 0 && (
                <ul className="list-disc pl-5 text-caption text-warning-ink">
                  {(verification.report.numbers ?? [])
                    .filter((n) => n.status !== 'verified')
                    .map((issue, idx) => (
                      <li key={`num-${idx}`}>
                        {issue.text} ({issue.status})
                      </li>
                    ))}
                </ul>
              )}
              {(verification.report.citations ?? []).filter((c) => c.status !== 'verified').length > 0 && (
                <ul className="list-disc pl-5 text-caption text-warning-ink">
                  {(verification.report.citations ?? [])
                    .filter((c) => c.status !== 'verified')
                    .map((issue, idx) => (
                      <li key={`cit-${idx}`}>
                        {issue.key}: {issue.status}{issue.error ? ` — ${issue.error}` : issue.note ? ` — ${issue.note}` : ''}
                      </li>
                    ))}
                </ul>
              )}
            </div>
          )}

          {depositManifest && (
            <div className="border border-border rounded-control p-3 space-y-2 text-label bg-surface">
              <div className="flex items-center justify-between gap-2">
                <span className="font-semibold">Staged Zenodo deposit manifest</span>
                <Button
                  size="compact"
                  variant="secondary"
                  onClick={() => downloadJson(`kubona-zenodo-manifest-${selected.manuscript.id}.json`, depositManifest)}
                >
                  Download manifest JSON
                </Button>
              </div>
              <pre className="text-caption font-mono overflow-x-auto max-h-40 p-2 rounded-control bg-page border border-border">
                {JSON.stringify(depositManifest, null, 2)}
              </pre>
            </div>
          )}
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
            <h5 className="text-label font-semibold">References</h5>
            {selected.manuscript.citations.length === 0 && (
              <p className="text-caption text-muted">No references. Search OpenAlex and add one; the draft is recomposed.</p>
            )}
            <ul className="space-y-1 text-label">
              {selected.manuscript.citations.map((c) => (
                <li key={c.key} className="flex items-center justify-between gap-2">
                  <span>
                    {c.title}
                    <span className="text-caption text-muted"> · {c.authors} {c.year}</span>
                    {c.verified && <Badge tone="success">claim</Badge>}
                  </span>
                  {selected.manuscript.state === 'draft' && (
                    <Button size="compact" variant="ghost" onClick={() => void removeReference(c.key)}>
                      remove
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            {selected.manuscript.state === 'draft' && (
              <div className="space-y-1">
                <div className="flex gap-2">
                  <Field
                    label="Literature search"
                    value={litQuery}
                    onChange={(e) => setLitQuery(e.target.value)}
                    placeholder="Search literature (OpenAlex)"
                    className="flex-1"
                  />
                  <Button size="compact" pending={litBusy} onClick={() => void searchReferences()}>
                    Search
                  </Button>
                </div>
                {litResults.length > 0 && (
                  <ul className="space-y-1 text-label border border-border rounded-control p-2">
                    {litResults.map((record) => (
                      <li key={record.id} className="flex items-center justify-between gap-2">
                        <span>{record.title} <span className="text-caption text-muted">· {record.publication_year}</span></span>
                        <Button size="compact" pending={busy === 'citations'} onClick={() => void addReference(record)}>
                          Add
                        </Button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>

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
                <Select
                  label="Annotation section"
                  value={annotationAnchor}
                  onChange={(e) => setAnnotationAnchor(e.target.value)}
                  options={[
                    { value: 'general', label: 'general' },
                    ...selected.manuscript.sections.map((s) => ({ value: s.slug, label: s.title })),
                  ]}
                />
                <Field
                  label="Review note"
                  value={annotationBody}
                  onChange={(e) => setAnnotationBody(e.target.value)}
                  placeholder="What needs attention?"
                  className="flex-1"
                />
                <Button size="compact" type="submit" pending={busy === 'annotate'} disabled={!annotationBody.trim()}>
                  Add note
                </Button>
              </div>
            </form>
          </div>

          <details className="border-t border-border pt-3">
            <summary className="text-label cursor-pointer">Review dimensions</summary>
            <div className="mt-2 space-y-2 text-label">
              <p>{String((selected.manuscript.review as { summary?: unknown }).summary ?? 'No review summary.')}</p>
              <dl className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                {[
                  ['Rigor', 'rigor'],
                  ['Novelty', 'novelty'],
                  ['Grounding', 'grounding'],
                  ['Hygiene', 'hygiene'],
                  ['Reproducibility', 'reproducibility'],
                  ['Mean', 'mean_score'],
                ].map(([label, key]) => (
                  <div key={key}>
                    <dt className="text-muted">{label}</dt>
                    <dd>{typeof (selected.manuscript.review as Record<string, unknown>)[key] === 'number'
                      ? ((selected.manuscript.review as Record<string, number>)[key]).toFixed(2)
                      : 'unmeasured'}</dd>
                  </div>
                ))}
              </dl>
              {Array.isArray((selected.manuscript.review as { critiques?: unknown }).critiques) && (
                <ul className="list-disc pl-5">
                  {((selected.manuscript.review as { critiques: string[] }).critiques).map((c) => (
                    <li key={c}>{c}</li>
                  ))}
                </ul>
              )}
            </div>
          </details>

          {selected.manuscript.artifacts.length > 0 && (
            <div>
              <h5 className="text-label font-semibold mb-1">Artifacts</h5>
              <ul className="space-y-1 text-label">
                {selected.manuscript.artifacts.map((a) => (
                  <li key={a.name} className="flex items-center justify-between gap-2">
                    <span className="font-mono break-all">{a.name}</span>
                    <Button
                      size="compact"
                      variant="ghost"
                      pending={busy === `download-${a.name}`}
                      onClick={() => void downloadArtifact(a.name)}
                    >
                      download
                    </Button>
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

      <details
        className="border-t border-border pt-3"
        onToggle={(e) => {
          if (e.currentTarget.open && formalProofs === null && sessionId) {
            void fetchFormalVerifications(sessionId).then((res) => {
              setFormalProofs(res?.verifications ?? []);
            });
          }
        }}
      >
        <summary className="text-label cursor-pointer font-medium">
          Formal verifications (Lean 4 comparator attestations)
        </summary>
        <div className="mt-2 space-y-2 text-label">
          <div className="flex justify-end">
            <Button
              size="compact"
              variant="ghost"
              pending={busy === 'formal-proofs'}
              onClick={() => void (async () => {
                setBusy('formal-proofs');
                const res = await fetchFormalVerifications(sessionId);
                setFormalProofs(res?.verifications ?? []);
                setBusy(null);
              })()}
            >
              Refresh proofs
            </Button>
          </div>
          {formalProofs && formalProofs.length === 0 && (
            <p className="text-caption text-muted">No Lean 4 formal verifications recorded for this session yet.</p>
          )}
          {formalProofs && formalProofs.length > 0 && (
            <ul className="space-y-1.5">
              {formalProofs.map((fv, idx) => (
                <li key={fv.event_id ?? fv.proof_hash ?? idx} className="border border-border rounded-control px-3 py-2 flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <span className="font-medium">
                      {fv.theorem_names?.length ? fv.theorem_names.join(', ') : `Proof #${fv.event_id ?? idx + 1}`}
                    </span>
                    <span className="block text-caption font-mono text-muted">
                      {fv.proof_hash ? `${fv.proof_hash.slice(0, 16)}…` : 'no hash'}
                      {fv.timestamp ? ` · ${fv.timestamp}` : ''}
                    </span>
                  </div>
                  <Badge tone={fv.verdict === 'live_kernel_proven' || fv.verdict === 'pass' ? 'success' : 'neutral'}>
                    {fv.verdict || 'recorded'}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </div>
      </details>
    </section>
  );
}
