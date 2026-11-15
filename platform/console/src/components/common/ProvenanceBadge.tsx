'use client';

import React from 'react';

export type ProvenanceKind = 'measured' | 'proposed' | 'catalog' | 'model';

const META: Record<ProvenanceKind, { label: string; title: string; className: string }> = {
  measured: {
    label: 'MEASURED',
    title: 'Folded from a recorded job, computed anchor, or evaluated claim.',
    className: 'bg-success-wash border-success-edge text-success-ink',
  },
  proposed: {
    label: 'PROPOSED',
    title: 'Campaign proposal or plan; not a measured result.',
    className: 'bg-info-wash border-info-edge text-info-ink',
  },
  catalog: {
    label: 'CATALOG',
    title: 'Reference catalog data, not a session measurement.',
    className: 'bg-fill border-border-strong text-label-secondary',
  },
  model: {
    title: 'Scientific model or visualization derived from recorded inputs.',
    label: 'MODEL',
    className: 'bg-warning-wash border-warning-edge text-warning-ink',
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
      className={`inline-flex items-center rounded-capsule border px-2 py-0.5 font-mono text-[12px] uppercase tracking-wide ${meta.className} ${className}`}
    >
      {meta.label}
    </span>
  );
}
