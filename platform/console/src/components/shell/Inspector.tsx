'use client';

import React, { useEffect, useRef, useState, type ReactNode } from 'react';
import type { CampaignState, Job } from '@/types';
import type { WorkbenchObject } from '@/components/workbench/workbench-model';
import { jobStatusLabel } from '@/lib/api';
import { Button } from '@/components/ui/Button';

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
  return <div><dt>{label}</dt><dd>{value}</dd></div>;
}

export function Inspector({
  open, tab, onTab, onClose, campaign, selectedObject, jobs, copilot, expanded = false, onOpenObject, onFocusObject, onRemoveObject,
}: {
  open: boolean;
  tab: InspectorTab;
  onTab: (tab: InspectorTab) => void;
  onClose: () => void;
  campaign: CampaignState | null;
  selectedObject: WorkbenchObject | null;
  jobs: Job[];
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
        className="fixed inset-y-0 right-0 z-50 flex w-[min(24rem,calc(100vw-3rem))] shrink-0 flex-col overflow-hidden border-l border-white/[0.08] bg-[#0A0C10] shadow-2xl lg:static lg:z-auto lg:w-[var(--inspector-width)]"
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
          className="absolute inset-y-0 left-0 hidden w-1 cursor-col-resize bg-transparent hover:bg-cyan-400/40 focus-visible:bg-cyan-400/60 focus-visible:outline-none lg:block"
        />
        <header className="flex items-center justify-between border-b border-white/[0.08] px-3 py-2">
          <div role="tablist" aria-label="Inspector sections" className="flex gap-1">
            {TABS.map((item) => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={tab === item.id}
                onClick={() => onTab(item.id)}
                className={`rounded-lg px-2 py-1 text-[12px] font-medium ${tab === item.id ? 'bg-cyan-400/10 text-cyan-300' : 'text-slate-400 hover:bg-white/5 hover:text-white'}`}
              >
                {item.label}
              </button>
            ))}
          </div>
          <button type="button" onClick={onClose} aria-label="Close inspector" className="rounded-lg px-2 py-1 text-[12px] text-slate-400 hover:bg-white/5 hover:text-white">Close</button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto p-4 text-slate-200">
          {tab === 'details' && (
            <section role="tabpanel" aria-label="Details" className="space-y-4">
              {selectedObject ? (
                <>
                  <div>
                    <div className="text-[12px] uppercase tracking-wide text-slate-500">{selectedObject.label}</div>
                    <h2 className="mt-1 text-base font-semibold text-white">{selectedObject.title}</h2>
                    <p className="mt-1 text-[12px] leading-relaxed text-slate-400">{selectedObject.detail}</p>
                  </div>
                  <dl className="space-y-2 text-[12px]">{selectedObject.facts.map((fact) => <Fact key={fact.label} label={fact.label} value={fact.value} />)}</dl>
                  {selectedObject.ref && <p className="font-mono text-[12px] text-slate-500">{selectedObject.ref.domain}:{selectedObject.ref.id}</p>}
                  <div className="flex flex-wrap gap-2 pt-1">
                    {onOpenObject && <Button type="button" size="compact" onClick={() => onOpenObject(selectedObject)}>Open</Button>}
                    {onFocusObject && <Button type="button" size="compact" variant="ghost" onClick={() => onFocusObject(selectedObject)}>Focus</Button>}
                    {onRemoveObject && <Button type="button" size="compact" variant="ghost" onClick={() => onRemoveObject(selectedObject)}>Remove from canvas</Button>}
                  </div>
                </>
              ) : campaign ? (
                <>
                  <h2 className="text-base font-semibold text-white">{campaign.target_system || 'Campaign'}</h2>
                  <p className="text-[12px] leading-relaxed text-slate-400">{campaign.research_goal || 'No research goal recorded.'}</p>
                  <dl className="space-y-2 text-[12px]">
                    <Fact label="Epistemic state" value={epistemics?.status || 'UNVERIFIED'} />
                    <Fact label="Hypothesis" value={campaign.hypothesis_id || 'not recorded'} />
                    <Fact label="Backend" value={campaign.quantum_factors?.recommended_backend || 'not chosen'} />
                    <Fact label="Anchor" value={anchor?.anchor_computed ? anchor.method : 'not computed'} />
                  </dl>
                </>
              ) : <p className="text-[12px] text-slate-400">No campaign selected.</p>}
            </section>
          )}

          {tab === 'evidence' && (
            <section role="tabpanel" aria-label="Evidence" className="space-y-4 text-[12px]">
              {campaign ? (
                <>
                  <div>
                    <div className="text-[12px] uppercase tracking-wide text-slate-500">Epistemic state</div>
                    <p className="mt-1 leading-relaxed text-white">{epistemics?.statement || 'No evaluated claim yet.'}</p>
                    <p className="mt-1 text-slate-400">{epistemics?.status || 'UNVERIFIED'}</p>
                  </div>
                  <dl className="space-y-2">
                    <Fact label="Corroborated α" value={num(epistemics?.alpha_corroborated, 3)} />
                    <Fact label="Contested β" value={num(epistemics?.beta_contested, 3)} />
                    <Fact label="Unverified γ" value={num(epistemics?.gamma_unverified, 3)} />
                    <Fact label="FCI anchor" value={`${num(anchor?.fci_exact_energy)} Ha`} />
                    <Fact label="VQE" value={`${num(anchor?.vqe_expectation_energy)} Ha`} />
                    <Fact label="Δ" value={`${num(anchor?.discrepancy_mha, 2)} mHa`} />
                    <Fact label="Provenance" value={notebook?.provenance_hash || 'not recorded'} />
                  </dl>
                  {(epistemics?.sources?.length ?? 0) > 0 && (
                    <div>
                      <div className="text-[12px] uppercase tracking-wide text-slate-500">Sources</div>
                      <ul className="mt-1 space-y-1 font-mono text-slate-300">{epistemics!.sources.map((source) => <li key={source}>{source}</li>)}</ul>
                    </div>
                  )}
                </>
              ) : <p className="text-slate-400">No campaign evidence.</p>}
            </section>
          )}

          {tab === 'jobs' && (
            <section role="tabpanel" aria-label="Jobs" className="space-y-2">
              {jobs.length === 0 ? <p className="text-[12px] text-slate-400">No jobs recorded for this campaign.</p> : jobs.map((job) => (
                <div key={job.id} className="rounded-xl border border-white/[0.08] bg-white/[0.03] p-2.5 text-[12px]">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-slate-300">{job.job_type || 'job'}</span>
                    <span className="text-slate-400">{jobStatusLabel(job.status)}</span>
                  </div>
                  <div className="mt-1 font-mono text-slate-500">{job.id.slice(0, 12)}</div>
                  <div className="mt-1 text-slate-500">created {job.created_at || 'unknown'}{job.completed_at ? ` · completed ${job.completed_at}` : ''}</div>
                </div>
              ))}
            </section>
          )}

          {tab === 'copilot' && <section role="tabpanel" aria-label="Copilot" className="h-full">{copilot}</section>}
        </div>
      </aside>
    </>
  );
}
