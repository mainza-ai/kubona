'use client';

import React, { useState } from 'react';
import { Radio } from 'lucide-react';

interface ShadowTomographyRadarWidgetProps {
  numQubits?: number;
  initialSnapshots?: number;
}

export function ShadowTomographyRadarWidget({
  numQubits = 2,
  initialSnapshots = 600,
}: ShadowTomographyRadarWidgetProps) {
  const [snapshotCount, setSnapshotCount] = useState<number>(initialSnapshots);

  // Reference observable estimates (static illustrative figures — see the
  // Illustrative badge; the pool-size slider below explores what-if pool
  // sizes without recomputing statistics).
  const observables = [
    { pauli: 'ZZ', exact: 1.000, shadowEst: 0.984, stdErr: 0.021 },
    { pauli: 'XX', exact: 1.000, shadowEst: 0.991, stdErr: 0.019 },
    { pauli: 'YY', exact: -1.000, shadowEst: -0.978, stdErr: 0.024 },
    { pauli: 'ZI', exact: 0.000, shadowEst: 0.012, stdErr: 0.015 },
    { pauli: 'IZ', exact: 0.000, shadowEst: -0.008, stdErr: 0.014 },
  ];

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-blue-500/10 border border-blue-500/20 text-blue-400">
            <Radio className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Classical Shadow Tomography Radar (Preview)</h3>
            <p className="text-xs text-white/50">Huang-Kueng-Preskill Protocol • {numQubits} Qubits • O(log M / ε²)</p>
          </div>
        </div>
        <span className="text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">
          Illustrative
        </span>
      </div>

      <p className="text-[10px] font-mono text-slate-500">
        REF — reference figures for the protocol, not measured from this session. Estimates below are static.
      </p>

      {/* Snapshots Controls & Efficiency Badge */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs">
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-1">
          <span className="text-white/40 block text-[10px]">Shadow Pool Size (REF)</span>
          <div className="flex justify-between items-center">
            <span className="font-mono text-base font-semibold text-blue-300">{snapshotCount} snapshots</span>
          </div>
          <input
            type="range"
            min="100"
            max="2000"
            step="100"
            value={snapshotCount}
            onChange={(e) => setSnapshotCount(parseInt(e.target.value, 10))}
            className="w-full accent-blue-500 bg-white/10 rounded-lg h-1.5 cursor-pointer mt-1"
          />
        </div>
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] flex flex-col justify-between">
          <span className="text-white/40 text-[10px]">Measurement Speedup vs Pauli Grouping (REF)</span>
          <div className="flex items-baseline space-x-1">
            <span className="font-mono text-xl font-bold text-emerald-400">14.8x</span>
            <span className="text-[10px] text-white/40">shot reduction</span>
          </div>
          <span className="text-[10px] text-emerald-300/70 font-mono">Decoupled from term count</span>
        </div>
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] flex flex-col justify-between">
          <span className="text-white/40 text-[10px]">State Representation Fidelity (REF)</span>
          <div className="flex items-baseline space-x-1">
            <span className="font-mono text-xl font-bold text-cyan-400">99.2%</span>
            <span className="text-[10px] text-white/40">F_shadow</span>
          </div>
          <span className="text-[10px] text-cyan-300/70 font-mono">Bell State |Φ+⟩</span>
        </div>
      </div>

      {/* Observables Table */}
      <div className="p-4 rounded-xl border border-white/[0.06] bg-black/40 overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-white/[0.06] text-white/40 font-mono text-[11px]">
              <th className="pb-2">Observable O_i</th>
              <th className="pb-2">Exact ⟨O_i⟩</th>
              <th className="pb-2">Shadow Est. (Median of Means)</th>
              <th className="pb-2">Std Error</th>
              <th className="pb-2">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/[0.04] font-mono">
            {observables.map((obs) => {
              const diff = Math.abs(obs.shadowEst - obs.exact);
              const exactOk = Number.isFinite(obs.exact);
              const estOk = Number.isFinite(obs.shadowEst);
              const errOk = Number.isFinite(obs.stdErr);
              return (
                <tr key={obs.pauli} className="hover:bg-white/[0.02]">
                  <td className="py-2.5 font-bold text-blue-300">σ_{obs.pauli} <span className="text-[9px] text-slate-500">REF</span></td>
                  <td className="py-2.5 text-white/60">{exactOk ? obs.exact.toFixed(3) : '—'} (ref)</td>
                  <td className="py-2.5 text-white/90">{estOk ? obs.shadowEst.toFixed(3) : '—'} (ref)</td>
                  <td className="py-2.5 text-white/40">±{errOk ? obs.stdErr.toFixed(3) : '—'} (ref)</td>
                  <td className="py-2.5">
                    <span className="px-2 py-0.5 rounded-full text-[10px] bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                      Converged (Δ={Number.isFinite(diff) ? diff.toFixed(3) : '—'}) (ref)
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
