'use client';

import React from 'react';
import { X, CircuitBoard, Radio, Atom, KeyRound, Fingerprint } from 'lucide-react';
import { CampaignState } from '@/types';
import {
  QuantumCircuitQIREditorWidget,
  ShadowTomographyRadarWidget,
  MolecularActiveSpaceWidget,
} from '@/components/widgets';

export type CanvasWidgetKind = 'circuit' | 'shadows' | 'molecule' | 'mandate';

export interface CanvasWidgetSpec {
  kind: CanvasWidgetKind;
  title: string;
}

const KIND_META: Record<CanvasWidgetKind, { icon: React.ComponentType<{ className?: string }>; dot: string }> = {
  circuit: { icon: CircuitBoard, dot: 'bg-cyan-400' },
  shadows: { icon: Radio, dot: 'bg-cyan-400' },
  molecule: { icon: Atom, dot: 'bg-amber-400' },
  mandate: { icon: KeyRound, dot: 'bg-amber-400' },
};

interface WidgetStageProps {
  widget: CanvasWidgetSpec | null;
  campaign?: CampaignState | null;
  gateOpen: boolean;
  spendUsd?: number;
  spendCapUsd?: number;
  onDismiss: () => void;
  onRequestGateFocus: () => void;
}

/**
 * On-demand canvas backdrop: the co-pilot materializes exactly one widget at
 * a time onto the arena, fed with live campaign props. Empty by default —
 * zero clutter until an agent has something worth showing. Every widget is
 * dismissible; the mandate card never signs — authorization happens only at
 * the Action Gate (single friction point).
 */
export function WidgetStage({
  widget,
  campaign,
  gateOpen,
  spendUsd = 0,
  spendCapUsd = 0,
  onDismiss,
  onRequestGateFocus,
}: WidgetStageProps) {
  if (!widget) return null;
  const meta = KIND_META[widget.kind];
  const Icon = meta.icon;
  const qf = campaign?.quantum_factors;

  return (
    <div
      data-testid="widget-stage"
      className="absolute right-6 top-1/2 -translate-y-1/2 z-20 w-[min(380px,92%)] max-h-[76%] flex flex-col rounded-2xl bg-[#0E1117]/92 backdrop-blur-2xl border border-white/10 shadow-2xl overflow-hidden"
    >
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06] shrink-0">
        <div className="flex items-center gap-2">
          <span className={`w-1.5 h-1.5 rounded-full ${meta.dot}`} />
          <Icon className="w-3.5 h-3.5 text-slate-300" />
          <span className="text-[11px] font-semibold text-white">{widget.title}</span>
          <span className="px-1.5 py-0.5 rounded bg-emerald-400/10 text-emerald-400 font-code text-[9px]">LIVE</span>
        </div>
        <button
          aria-label="Dismiss widget"
          data-testid="widget-dismiss"
          onClick={onDismiss}
          className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className="overflow-y-auto p-3">
        {widget.kind === 'circuit' && (
          <QuantumCircuitQIREditorWidget
            qubits={qf?.qubit_count ?? 2}
            circuitDepth={qf?.circuit_depth ?? 0}
            qirCode={qf?.qasm || undefined}
          />
        )}
        {widget.kind === 'shadows' && (
          <ShadowTomographyRadarWidget numQubits={qf?.qubit_count ?? 2} />
        )}
        {widget.kind === 'molecule' && (
          <MolecularActiveSpaceWidget
            moleculeName={qf?.target_molecule || campaign?.target_system || 'H2'}
            numElectrons={qf?.active_electrons}
            numOrbitals={qf?.active_orbitals}
          />
        )}
        {widget.kind === 'mandate' && (
          <div className="rounded-2xl border border-amber-500/25 bg-black/50 p-4 space-y-3">
            <div className="text-xs text-white font-medium">
              Deploy pulse on {qf?.recommended_backend || 'selected backend'}
            </div>
            <div className="grid grid-cols-2 gap-3 text-xs">
              <div>
                <span className="text-white/40 block text-[10px]">Est. spend</span>
                <span className="font-code text-amber-300 font-bold">
                  ${(qf?.estimated_spend_usd ?? 0).toFixed(2)}
                </span>
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Budget left</span>
                <span className="font-code text-slate-200">
                  ${(spendCapUsd - spendUsd).toFixed(2)}
                </span>
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Qubits</span>
                <span className="font-code text-slate-200">{qf?.qubit_count ?? '—'}</span>
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Canary fidelity</span>
                <span className="font-code text-slate-200">
                  {qf?.clifford_fidelity != null ? qf.clifford_fidelity.toFixed(4) : '—'}
                </span>
              </div>
            </div>
            {gateOpen ? (
              <button
                onClick={onRequestGateFocus}
                className="w-full py-2 px-3 rounded-xl bg-amber-500/15 hover:bg-amber-500/25 border border-amber-400/40 text-amber-200 font-semibold text-xs flex items-center justify-center gap-2 transition-all"
              >
                <Fingerprint className="w-4 h-4" />
                <span>Review at Action Gate</span>
              </button>
            ) : (
              <p className="text-[10px] font-code text-slate-500">
                Gate already decided this session — card is read-only.
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
