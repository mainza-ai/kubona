'use client';

import React from 'react';

export type ProvenanceKind = 'measured' | 'proposed' | 'catalog' | 'model';

const META: Record<ProvenanceKind, { label: string; title: string; className: string }> = {
  measured: {
    label: 'MEASURED',
    title: 'Folded from a recorded job, computed anchor, or evaluated claim.',
    className: 'bg-emerald-400/10 border-emerald-400/40 text-emerald-300',
  },
  proposed: {
    label: 'PROPOSED',
    title: 'Campaign proposal or plan; not a measured result.',
    className: 'bg-cyan-400/10 border-cyan-400/40 text-cyan-300',
  },
  catalog: {
    label: 'CATALOG',
    title: 'Reference catalog data, not a session measurement.',
    className: 'bg-slate-400/10 border-slate-400/40 text-slate-300',
  },
  model: {
    title: 'Scientific model or visualization derived from recorded inputs.',
    label: 'MODEL',
    className: 'bg-amber-400/10 border-amber-400/40 text-amber-300',
  },
};

export function ProvenanceBadge({
  kind,
  detail,
  className = '',
}: {
  kind: ProvenanceKind;
  detail?: string;
  className?: string;
}) {
  const meta = META[kind];
  return (
    <span
      data-testid={`provenance-${kind}`}
      data-provenance={kind}
      title={detail ? `${meta.title} ${detail}` : meta.title}
      className={`inline-flex items-center rounded-full border px-2 py-0.5 font-mono text-[12px] uppercase tracking-wide ${meta.className} ${className}`}
    >
      {meta.label}
    </span>
  );
}
