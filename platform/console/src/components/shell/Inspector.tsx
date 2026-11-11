'use client';

import React, { useEffect, useRef, useState, type ReactNode } from 'react';
import type { CampaignState, Task, Job } from '@/types';
import type { WorkbenchObject } from '@/components/workbench/workbench-model';
import { jobStatusLabel } from '@/lib/api';
import { Button } from '@/components/ui/Button';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

export type InspectorTab = 'details' | 'evidence' | 'jobs' | 'copilot';

const TABS: Array<{ id: InspectorTab; label: string }> = [
  { id: 'details', label: 'Details' },
  { id: 'evidence', label: 'Evidence' },
  { id: 'jobs', label: 'Jobs' },
  { id: 'copilot', label: 'Copilot' },
];
const MIN_WIDTH = 320;
const MAX_WIDTH = 420;
const WIDTH_KEY = 'kubona.inspector.width';

function num(value: unknown, digits = 5): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(digits) : 'UNVERIFIED';
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[var(--color-muted)]">{label}</dt>
      <dd className="m-0 min-w-0 font-mono text-[var(--color-text)] break-all">{value}</dd>
    </div>
  );
}

export const Inspector = React.memo(function Inspector({
  open, tab, onTab, onClose, campaign, selectedObject, jobs, tasks = [], jobsStatus = 'ready', jobsError = null, onRefreshJobs, onCreateTask, onTaskStatus, copilot, expanded = false, onOpenObject, onFocusObject, onFocusJob, onRemoveObject,
}: {
  open: boolean;
  tab: InspectorTab;
  onTab: (tab: InspectorTab) => void;
  onClose: () => void;
  campaign: CampaignState | null;
  selectedObject: WorkbenchObject | null;
  jobs: Job[];
  tasks?: Task[];
  onCreateTask?: (object: WorkbenchObject) => void;
  onTaskStatus?: (taskId: string, status: 'open' | 'done') => void;
  jobsStatus?: 'idle' | 'loading' | 'ready' | 'stale' | 'refused';
  jobsError?: string | null;
  onRefreshJobs?: () => void;
  onFocusJob?: (job: Job) => void;
  copilot: ReactNode;
  expanded?: boolean;
  onOpenObject?: (object: WorkbenchObject) => void;
  onFocusObject?: (object: WorkbenchObject) => void;
  onRemoveObject?: (object: WorkbenchObject) => void;
}) {
  const [width, setWidth] = useState(() => {
    if (typeof window === 'undefined' || !window.localStorage) return 380;
    try {
      const stored = Number(window.localStorage.getItem(WIDTH_KEY));
      return Number.isFinite(stored) ? Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, stored)) : 380;
    } catch {
      return 380;
    }
  });
  const drag = useRef<{ x: number; width: number } | null>(null);

  useEffect(() => {
    if (typeof window === 'undefined' || !window.localStorage) return;
    try { window.localStorage.setItem(WIDTH_KEY, String(Math.round(width))); } catch { /* storage denied */ }
  }, [width]);

  if (!open) return null;

  const maxWidth = expanded ? 700 : MAX_WIDTH;
  const effectiveWidth = expanded ? Math.max(width, 640) : width;
  const anchor = campaign?.classical_anchor;
  const epistemics = campaign?.epistemics;
  const notebook = campaign?.notebook;

  return (
    <>
      <div aria-hidden onClick={onClose} className="fixed inset-0 z-40 bg-black/60 lg:hidden" />
      <aside
        aria-label="Inspector"
        style={{ '--inspector-width': `${effectiveWidth}px` } as React.CSSProperties}
        className="fixed inset-y-0 right-0 z-50 flex w-[min(24rem,calc(100vw-3rem))] shrink-0 flex-col overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-chrome)] shadow-[var(--shadow-float)] lg:static lg:z-auto lg:w-[var(--inspector-width)]"
      >
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize inspector"
          aria-valuemin={MIN_WIDTH}
          aria-valuemax={maxWidth}
          aria-valuenow={Math.round(effectiveWidth)}
          tabIndex={0}
          onPointerDown={(event) => {
            event.currentTarget.setPointerCapture(event.pointerId);
            drag.current = { x: event.clientX, width };
          }}
          onPointerMove={(event) => {
            if (!drag.current) return;
            setWidth(Math.min(maxWidth, Math.max(MIN_WIDTH, drag.current.width + (drag.current.x - event.clientX))));
          }}
          onPointerUp={() => { drag.current = null; }}
          onPointerCancel={() => { drag.current = null; }}
          onKeyDown={(event) => {
            if (event.key === 'ArrowLeft') setWidth((value) => Math.min(maxWidth, value + 16));
            if (event.key === 'ArrowRight') setWidth((value) => Math.max(MIN_WIDTH, value - 16));
          }}
          className="absolute inset-y-0 left-0 hidden w-1 cursor-col-resize bg-transparent hover:bg-[var(--color-accent)]/40 focus-visible:bg-[var(--color-accent)]/60 focus-visible:outline-none lg:block"
        />
        <header className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-3 py-2 min-w-0">
          <div role="tablist" aria-label="Inspector sections" className="flex flex-wrap sm:flex-nowrap min-w-0 gap-1">
            {TABS.map((item) => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={tab === item.id}
                onClick={() => onTab(item.id)}
                className={`rounded-lg px-2 py-1 text-[12px] font-medium whitespace-nowrap ${tab === item.id ? 'bg-[var(--color-lit-soft)] text-[var(--color-text)]' : 'text-[var(--color-muted)] hover:bg-[var(--color-fill)] hover:text-[var(--color-text)]'}`}
              >
                {item.label}
              </button>
            ))}
          </div>
          <button type="button" onClick={onClose} aria-label="Close inspector" className="shrink-0 rounded-lg px-2 py-1 text-[12px] text-[var(--color-muted)] hover:bg-[var(--color-fill)] hover:text-[var(--color-text)]">Close</button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 text-[var(--color-label-secondary)]">
          {tab === 'details' && (
            <section role="tabpanel" aria-label="Details" className="space-y-4">
              {selectedObject ? (
                <>
                  <div>
                    <div className="text-[12px] uppercase tracking-wide text-[var(--color-faint)]">{selectedObject.label}</div>
                    <h2 className="mt-1 text-base font-semibold text-[var(--color-text)]">{selectedObject.title}</h2>
                    <p className="mt-1 text-[12px] leading-relaxed text-[var(--color-muted)]">{selectedObject.detail}</p>
                  </div>
                  <dl className="space-y-2 text-[12px]">{selectedObject.facts.map((fact) => <Fact key={fact.label} label={fact.label} value={fact.value} />)}</dl>
                  {selectedObject.basis && <p className="k-inspector-basis">{selectedObject.basis}</p>}
                  {selectedObject.sources?.length ? (
                    <div>
                      <div className="k-inspector-section-label">Sources</div>
                      <ul className="k-inspector-sources">
                        {selectedObject.sources.map((source) => (
                          <li key={source.label}>
                            {source.url
                              ? <a href={source.url} target="_blank" rel="noreferrer">{source.label}</a>
                              : source.label}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ) : null}
                  {selectedObject.ref && <p className="font-mono text-[12px] text-[var(--color-faint)]">{selectedObject.ref.domain}:{selectedObject.ref.id}</p>}
                  <div className="flex flex-wrap gap-2 pt-1">
                    {onOpenObject && <Button type="button" size="compact" onClick={() => onOpenObject(selectedObject)}>Open</Button>}
                    {onFocusObject && <Button type="button" size="compact" variant="ghost" onClick={() => onFocusObject(selectedObject)}>Focus</Button>}
                    {onRemoveObject && <Button type="button" size="compact" variant="ghost" onClick={() => onRemoveObject(selectedObject)}>Remove from canvas</Button>}
                    {onCreateTask && selectedObject.kind !== 'task' && selectedObject.ref && (
                      <Button type="button" size="compact" variant="ghost" onClick={() => onCreateTask(selectedObject)}>Create task</Button>
                    )}
                    {onTaskStatus && selectedObject.kind === 'task' && selectedObject.ref && (() => {
                      const task = tasks.find((item) => item.id === selectedObject.ref!.id);
                      if (!task) return null;
                      const next = task.status === 'done' ? 'open' : 'done';
                      return <Button type="button" size="compact" variant="ghost" onClick={() => onTaskStatus(task.id, next)}>{next === 'done' ? 'Mark done' : 'Reopen task'}</Button>;
                    })()}
                  </div>
                </>
              ) : campaign ? (
                <>
                  <h2 className="text-base font-semibold text-[var(--color-text)]">{campaign.target_system || 'Campaign'}</h2>
                  <p className="text-[12px] leading-relaxed text-[var(--color-muted)]">{campaign.research_goal || 'No research goal recorded.'}</p>
                  <dl className="space-y-2 text-[12px]">
                    <Fact label="Epistemic state" value={epistemics?.status || 'UNVERIFIED'} />
                    <Fact label="Hypothesis" value={campaign.hypothesis_id || 'not recorded'} />
                    <Fact label="Backend" value={campaign.quantum_factors?.recommended_backend || 'not chosen'} />
                    <Fact label="Anchor" value={anchor?.anchor_computed ? anchor.method : 'not computed'} />
                  </dl>
                </>
              ) : <p className="text-[12px] text-[var(--color-muted)]">No campaign selected.</p>}
            </section>
          )}

          {tab === 'evidence' && (
            <section role="tabpanel" aria-label="Evidence" className="space-y-4 text-[12px]">
              {campaign ? (
                <>
                  <div data-provenance={epistemics ? 'measured' : undefined}>
                    <div className="flex items-center gap-2 text-[12px] uppercase tracking-wide text-[var(--color-faint)]">
                      <span>Epistemic state</span>
                      {epistemics && <ProvenanceBadge kind="measured" detail="Evaluated claim record." />}
                    </div>
                    <p className="mt-1 leading-relaxed text-[var(--color-text)]">{epistemics?.statement || 'No evaluated claim yet.'}</p>
                    <p className="mt-1 text-[var(--color-muted)]">{epistemics?.status || 'UNVERIFIED'}</p>
                  </div>
                  <dl data-provenance={epistemics || anchor?.anchor_computed ? 'measured' : undefined} className="space-y-2">
                    <Fact label="Corroborated α" value={num(epistemics?.alpha_corroborated, 3)} />
                    <Fact label="Contested β" value={num(epistemics?.beta_contested, 3)} />
                    <Fact label="Unverified γ" value={num(epistemics?.gamma_unverified, 3)} />
                    <Fact label="FCI anchor" value={`${num(anchor?.fci_exact_energy)} Ha`} />
                    <Fact label="VQE" value={`${num(anchor?.vqe_expectation_energy)} Ha`} />
                    <Fact label="Δ" value={`${num(anchor?.discrepancy_mha, 2)} mHa`} />
                    <Fact label="Provenance" value={notebook?.provenance_hash || 'not recorded'} />
                  </dl>
                  {(epistemics?.sources?.length ?? 0) > 0 && (
                    <div className="min-w-0">
                      <div className="text-[12px] uppercase tracking-wide text-[var(--color-faint)]">Sources</div>
                      <ul className="mt-1 space-y-1 font-mono text-[var(--color-label-secondary)] break-all">
                        {epistemics!.sources.map((source) => <li key={source} className="min-w-0">{source}</li>)}
                      </ul>
                    </div>
                  )}
                </>
              ) : <p className="text-[var(--color-muted)]">No campaign evidence.</p>}
            </section>
          )}

          {tab === 'jobs' && (
            <section role="tabpanel" aria-label="Jobs" className="space-y-2">
              {jobsError && (
                <div role="alert" className="rounded-xl border border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_12%,var(--color-surface))] p-2.5 text-[12px] text-[var(--color-warning)]">
                  <p>Jobs unavailable: {jobsError}</p>
                  {onRefreshJobs && <button type="button" onClick={onRefreshJobs} className="mt-1 underline underline-offset-2">Retry</button>}
                </div>
              )}
              {!jobsError && jobs.length === 0 ? <p className="text-[12px] text-[var(--color-muted)]">{jobsStatus === 'loading' ? 'Loading jobs…' : 'No jobs recorded for this campaign.'}</p> : jobs.map((job) => (
                onFocusJob ? (
                  <button
                    key={job.id}
                    type="button"
                    data-testid={`inspector-job-${job.id}`}
                    onClick={() => onFocusJob(job)}
                    title="Focus this job card on the workbench canvas"
                    className="w-full text-start rounded-xl border border-[var(--color-border)] bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] p-2.5 text-[12px] transition-colors"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-[var(--color-label-secondary)]">{job.job_type || 'job'}</span>
                      <span className="text-[var(--color-muted)]">{jobStatusLabel(job.status)}</span>
                    </div>
                    <div className="mt-1 font-mono text-[var(--color-faint)]">{job.id.slice(0, 12)}</div>
                    <div className="mt-1 text-[var(--color-faint)]">created {job.created_at || 'unknown'}{job.completed_at ? ` · completed ${job.completed_at}` : ''}</div>
                  </button>
                ) : (
                  <div key={job.id} className="rounded-xl border border-[var(--color-border)] bg-[var(--color-fill)] p-2.5 text-[12px]">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-[var(--color-label-secondary)]">{job.job_type || 'job'}</span>
                      <span className="text-[var(--color-muted)]">{jobStatusLabel(job.status)}</span>
                    </div>
                    <div className="mt-1 font-mono text-[var(--color-faint)]">{job.id.slice(0, 12)}</div>
                    <div className="mt-1 text-[var(--color-faint)]">created {job.created_at || 'unknown'}{job.completed_at ? ` · completed ${job.completed_at}` : ''}</div>
                  </div>
                )
              ))}
            </section>
          )}

          {tab === 'copilot' && <section role="tabpanel" aria-label="Copilot" className="h-full">{copilot}</section>}
        </div>
      </aside>
    </>
  );
});
