'use client';

import React from 'react';
import { Radio } from 'lucide-react';

interface ShadowTomographyRadarWidgetProps {
  numQubits?: number;
}

/**
 * Shadow tomography is a measurement surface. Until a real snapshot pool and
 * observable estimates exist, this widget states that absence instead of
 * showing reference estimates as if they came from the session.
 */
export function ShadowTomographyRadarWidget({ numQubits }: ShadowTomographyRadarWidgetProps) {
  return (
    <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-soft-sky-wash)] border border-[var(--color-soft-sky-edge)] text-[var(--color-soft-sky-ink)]">
            <Radio className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-[var(--color-text)]">Classical Shadow Tomography</h3>
            <p className="text-xs text-[var(--color-muted)]">
              Huang-Kueng-Preskill protocol{numQubits != null ? ` • ${numQubits} qubits` : ''}
            </p>
          </div>
        </div>
      </div>
      <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-5 text-center space-y-1.5">
        <p className="text-xs font-medium text-[var(--color-text)]">No shadow-tomography execution is recorded for this campaign.</p>
        <p className="text-[12px] text-[var(--color-muted)]">
          Snapshot counts, observable estimates, standard errors, and speedups appear only after a measured snapshot job folds.
        </p>
      </div>
    </div>
  );
}
