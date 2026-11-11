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
    <div data-provenance={hasGeometry ? 'measured' : undefined} className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]">
            <Compass className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-[var(--color-text)]">Riemannian Epistemic Manifold Surface</h3>
              {hasGeometry && <ProvenanceBadge kind="measured" detail="Geometry supplied by the campaign computation." />}
            </div>
            <p className="text-xs text-[var(--color-muted)]">Fisher-Rao information metric g_μν</p>
          </div>
        </div>
        {hasCurv && (
          <span className="px-2 py-0.5 rounded-full bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[12px] font-mono text-[var(--color-warning-ink)]">
            R = {currentCurvatureScalar.toFixed(3)}
          </span>
        )}
      </div>

      {!hasGeometry ? (
        <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-[var(--color-text)]">No epistemic manifold geometry is recorded for this campaign.</p>
          <p className="text-[12px] text-[var(--color-muted)]">
            Curvature, Fisher information, and expected entropy reduction appear only when a computation supplies them; no template values are shown.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs">
          <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-1">
            <span className="text-[var(--color-muted)] text-[12px]">Fisher Information Norm ||g_μν||</span>
            <div data-metric className="font-mono text-lg font-bold text-[var(--color-warning-ink)]">{hasFisher ? fisherInfoNorm.toFixed(3) : '—'}</div>
            <p className="text-[12px] text-[var(--color-faint)]">Local parameter sensitivity</p>
          </div>
          <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-1">
            <span className="text-[var(--color-muted)] text-[12px]">Curvature Scalar R</span>
            <div data-metric className="font-mono text-lg font-bold text-[var(--color-danger-ink)]">{hasCurv ? currentCurvatureScalar.toFixed(3) : '—'}</div>
            <p className="text-[12px] text-[var(--color-faint)]">Recorded geometry only</p>
          </div>
          <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-1">
            <span className="text-[var(--color-muted)] text-[12px]">Expected Entropy Reduction</span>
            <div data-metric className="font-mono text-lg font-bold text-[var(--color-success-ink)]">{hasEntropy ? `ΔS = -${entropyReduction.toFixed(2)} nats` : '—'}</div>
            <p className="text-[12px] text-[var(--color-faint)]">Recorded geometry only</p>
          </div>
        </div>
      )}
    </div>
  );
}
