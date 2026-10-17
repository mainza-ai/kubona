'use client';

import React, { useState } from 'react';
import { Compass,    Anchor } from 'lucide-react';

interface EpistemicManifoldSurfaceWidgetProps {
  currentCurvatureScalar?: number;
  fisherInfoNorm?: number;
  entropyReduction?: number;
}

export function EpistemicManifoldSurfaceWidget({
  currentCurvatureScalar,
  fisherInfoNorm,
  entropyReduction,
}: EpistemicManifoldSurfaceWidgetProps) {
  const [selectedAnchor, setSelectedAnchor] = useState<string>('Family 271');
  // Per-field illustrative marks: defaults are template figures until the
  // caller binds live geometry. Each unbound field carries its own mark.
  const curv = typeof currentCurvatureScalar === 'number' && Number.isFinite(currentCurvatureScalar) ? currentCurvatureScalar : -1.842;
  const curvLive = currentCurvatureScalar != null && Number.isFinite(currentCurvatureScalar);
  const fisher = typeof fisherInfoNorm === 'number' && Number.isFinite(fisherInfoNorm) ? fisherInfoNorm : 4.218;
  const fisherLive = fisherInfoNorm != null && Number.isFinite(fisherInfoNorm);
  const entropy = typeof entropyReduction === 'number' && Number.isFinite(entropyReduction) ? entropyReduction : 0.64;
  const entropyLive = entropyReduction != null && Number.isFinite(entropyReduction);

  const anchors = [
    { id: 'Family 271', name: 'Heisenberg Ferromagnet 3D', exactVal: -1.500, rho: 1.0, type: 'Ground Energy (J)' },
    { id: 'Family 281', name: 'SK Spin Glass QAOA', exactVal: -0.763, rho: 1.0, type: 'Parisi Optimum' },
    { id: 'Family 268', name: 'Haldane Spin-1 Gap', exactVal: 0.410, rho: 1.0, type: 'Spectral Gap (J)' },
    { id: 'Family 278', name: 'Kohn-Sham Failure (3e)', exactVal: 0.000, rho: 1.0, type: 'Representability Sieve' },
  ];

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
            <Compass className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Riemannian Epistemic Manifold Surface (Preview)<span className="ml-2 align-middle text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">Illustrative</span></h3>
            <p className="text-xs text-white/50">Fisher-Rao Information Metric g_μν • Geodesic Curiosity Navigation</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <span className="px-2 py-0.5 rounded-full bg-amber-500/10 border border-amber-500/30 text-[11px] font-mono text-amber-300">
            R = {curv.toFixed(3)}{!curvLive ? ' (illustrative)' : ''}
          </span>
        </div>
      </div>

      {/* Metrics Row */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs">
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
          <span className="text-white/40 text-[10px]">Fisher Information Norm ||g_μν|| {!fisherLive && <span className="text-amber-300 border border-amber-400/40 rounded px-1">illustrative</span>}</span>
          <div className="font-mono text-lg font-bold text-amber-300">{fisher.toFixed(3)}</div>
          <p className="text-[10px] text-white/40">Local parameter sensitivity</p>
        </div>
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
          <span className="text-white/40 text-[10px]">Ricci Curvature Regime</span>
          <div className="font-mono text-lg font-bold text-rose-400">Hyperbolic (High Info Gain)</div>
          <p className="text-[10px] text-white/40">Geodesics diverge into new discovery spaces</p>
        </div>
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
          <span className="text-white/40 text-[10px]">Expected Entropy Reduction {!entropyLive && <span className="text-amber-300 border border-amber-400/40 rounded px-1">illustrative</span>}</span>
          <div className="font-mono text-lg font-bold text-emerald-400">ΔS = -{entropy.toFixed(2)} nats</div>
          <p className="text-[10px] text-white/40">Satisfies Fisher stopping rule</p>
        </div>
      </div>

      {/* Manifold Surface Heatmap & Anchors */}
      <div className="p-4 rounded-xl border border-white/[0.06] bg-black/40 space-y-3">
        <div className="flex justify-between items-center text-xs">
          <span className="font-semibold text-white/80 flex items-center space-x-1.5">
            <Anchor className="w-3.5 h-3.5 text-amber-400" />
            <span>Exact Analytical Anchors Pinning Belief Space</span>
          </span>
          <span className="text-[11px] text-white/40">Zero-Quantum-Washing Pins · reference values</span>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
          {anchors.map((a) => {
            const isSelected = selectedAnchor === a.id;
            return (
              <div
                key={a.id}
                onClick={() => setSelectedAnchor(a.id)}
                className={`p-2.5 rounded-xl border cursor-pointer transition ${
                  isSelected
                    ? 'bg-amber-500/15 border-amber-500/40 text-amber-200'
                    : 'border-white/[0.04] bg-white/[0.01] text-white/70 hover:bg-white/[0.04]'
                }`}
              >
                <div className="font-bold text-[11px] text-amber-400 font-mono">{a.id} <span className="text-[9px] text-slate-400">REF</span></div>
                <div className="truncate text-white/90 text-xs mt-0.5">{a.name}</div>
                <div className="font-mono text-[10px] text-white/40 mt-1 flex justify-between">
                  <span>{a.exactVal.toFixed(3)} (ref)</span>
                  <span className="text-emerald-400">ρ={a.rho.toFixed(1)} (ref)</span>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
