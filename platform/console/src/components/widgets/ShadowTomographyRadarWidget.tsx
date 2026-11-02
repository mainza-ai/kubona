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
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 text-white space-y-4">
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-blue-500/10 border border-blue-500/20 text-blue-400">
            <Radio className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Classical Shadow Tomography</h3>
            <p className="text-xs text-white/50">
              Huang-Kueng-Preskill protocol{numQubits != null ? ` • ${numQubits} qubits` : ''}
            </p>
          </div>
        </div>
      </div>
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5 text-center space-y-1.5">
        <p className="text-xs font-medium text-white/80">No shadow-tomography execution is recorded for this campaign.</p>
        <p className="text-[12px] text-slate-400">
          Snapshot counts, observable estimates, standard errors, and speedups appear only after a measured snapshot job folds.
        </p>
      </div>
    </div>
  );
}
