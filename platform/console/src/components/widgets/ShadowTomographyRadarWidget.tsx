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
    <div className="rounded-card border border-border bg-surface p-5 text-text space-y-4">
      <div className="flex items-center justify-between border-b border-border pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-row bg-soft-sky-wash border border-soft-sky-edge text-soft-sky-ink">
            <Radio className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-body-strong font-semibold text-text">Classical Shadow Tomography</h3>
            <p className="text-label text-muted">
              Huang-Kueng-Preskill protocol{numQubits != null ? ` • ${numQubits} qubits` : ''}
            </p>
          </div>
        </div>
      </div>
      <div className="rounded-row border border-border bg-card p-5 text-center space-y-1.5">
        <p className="text-label font-medium text-text">No shadow-tomography execution is recorded for this campaign.</p>
        <p className="text-[12px] text-muted">
          Snapshot counts, observable estimates, standard errors, and speedups appear only after a measured snapshot job folds.
        </p>
      </div>
    </div>
  );
}
