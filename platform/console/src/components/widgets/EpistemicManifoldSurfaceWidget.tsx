'use client';

import React from 'react';
import { Compass } from 'lucide-react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

interface EpistemicManifoldSurfaceWidgetProps {
  currentCurvatureScalar?: number;
  fisherInfoNorm?: number;
  entropyReduction?: number;
}

function finite(v: number | undefined): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

/**
 * Displays only recorded manifold geometry. There is no default curvature,
 * Fisher norm, entropy, or analytical-anchor badge: absent geometry reads as
 * absent until a backend computation supplies it.
 */
export function EpistemicManifoldSurfaceWidget({
  currentCurvatureScalar,
  fisherInfoNorm,
  entropyReduction,
}: EpistemicManifoldSurfaceWidgetProps) {
  const hasCurv = finite(currentCurvatureScalar);
  const hasFisher = finite(fisherInfoNorm);
  const hasEntropy = finite(entropyReduction);
  const hasGeometry = hasCurv || hasFisher || hasEntropy;

  return (
    <div data-provenance={hasGeometry ? 'measured' : undefined} className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 text-white space-y-4">
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
            <Compass className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-white/90">Riemannian Epistemic Manifold Surface</h3>
              {hasGeometry && <ProvenanceBadge kind="measured" detail="Geometry supplied by the campaign computation." />}
            </div>
            <p className="text-xs text-white/50">Fisher-Rao information metric g_μν</p>
          </div>
        </div>
        {hasCurv && (
          <span className="px-2 py-0.5 rounded-full bg-amber-500/10 border border-amber-500/30 text-[12px] font-mono text-amber-300">
            R = {currentCurvatureScalar.toFixed(3)}
          </span>
        )}
      </div>

      {!hasGeometry ? (
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-white/80">No epistemic manifold geometry is recorded for this campaign.</p>
          <p className="text-[12px] text-slate-400">
            Curvature, Fisher information, and expected entropy reduction appear only when a computation supplies them; no template values are shown.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs">
          <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
            <span className="text-white/40 text-[12px]">Fisher Information Norm ||g_μν||</span>
            <div data-metric className="font-mono text-lg font-bold text-amber-300">{hasFisher ? fisherInfoNorm.toFixed(3) : '—'}</div>
            <p className="text-[12px] text-white/40">Local parameter sensitivity</p>
          </div>
          <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
            <span className="text-white/40 text-[12px]">Curvature Scalar R</span>
            <div data-metric className="font-mono text-lg font-bold text-rose-400">{hasCurv ? currentCurvatureScalar.toFixed(3) : '—'}</div>
            <p className="text-[12px] text-white/40">Recorded geometry only</p>
          </div>
          <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
            <span className="text-white/40 text-[12px]">Expected Entropy Reduction</span>
            <div data-metric className="font-mono text-lg font-bold text-emerald-400">{hasEntropy ? `ΔS = -${entropyReduction.toFixed(2)} nats` : '—'}</div>
            <p className="text-[12px] text-white/40">Recorded geometry only</p>
          </div>
        </div>
      )}
    </div>
  );
}
