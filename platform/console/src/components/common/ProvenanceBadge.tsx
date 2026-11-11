'use client';

import React from 'react';

export type ProvenanceKind = 'measured' | 'proposed' | 'catalog' | 'model';

const META: Record<ProvenanceKind, { label: string; title: string; className: string }> = {
  measured: {
    label: 'MEASURED',
    title: 'Folded from a recorded job, computed anchor, or evaluated claim.',
    className: 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]',
  },
  proposed: {
    label: 'PROPOSED',
    title: 'Campaign proposal or plan; not a measured result.',
    className: 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] text-[var(--color-info-ink)]',
  },
  catalog: {
    label: 'CATALOG',
    title: 'Reference catalog data, not a session measurement.',
    className: 'bg-[var(--color-fill)] border-[var(--color-border-strong)] text-[var(--color-label-secondary)]',
  },
  model: {
    title: 'Scientific model or visualization derived from recorded inputs.',
    label: 'MODEL',
    className: 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]',
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
