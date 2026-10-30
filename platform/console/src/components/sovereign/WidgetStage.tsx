'use client';

import React, { useState } from 'react';
import { X, CircuitBoard, Radio, Atom, KeyRound, Fingerprint, Minus, Plus } from 'lucide-react';
import { CampaignState } from '@/types';
import { isMeteredBackend } from '@/lib/approvalPacket';
import {
  QuantumCircuitQIREditorWidget,
  ShadowTomographyRadarWidget,
  MolecularActiveSpaceWidget,
} from '@/components/widgets';
import { JobDetailWidget } from './JobDetailWidget';

export type CanvasWidgetKind = 'circuit' | 'shadows' | 'molecule' | 'mandate' | 'job';

export interface CanvasWidgetSpec {
  kind: CanvasWidgetKind;
  title: string;
  /** Job id for kind === 'job' (fetched live; never embedded). */
  jobId?: string;
  /** Owning session for kind === 'job' (backend scopes reads per session). */
  sessionId?: string;
}

const KIND_META: Record<CanvasWidgetKind, { icon: React.ComponentType<{ className?: string }>; dot: string }> = {
  circuit: { icon: CircuitBoard, dot: 'bg-cyan-400' },
  shadows: { icon: Radio, dot: 'bg-cyan-400' },
  molecule: { icon: Atom, dot: 'bg-amber-400' },
  job: { icon: CircuitBoard, dot: 'bg-emerald-400' },
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
  const [minimized, setMinimized] = useState(false);
  if (!widget) return null;
  const meta = KIND_META[widget.kind];
  const Icon = meta.icon;
  const qf = campaign?.quantum_factors;
  // Feed honesty per kind: shadows renders static reference figures;
  // circuit/molecule mix live labels over illustrative bodies; job and
  // mandate surfaces read live session data end to end.
  const feed: 'LIVE' | 'MIX' | 'REF' =
    widget.kind === 'shadows'
      ? 'REF'
      : widget.kind === 'circuit' || widget.kind === 'molecule'
        ? 'MIX'
        : 'LIVE';
  const feedTitle =
    feed === 'LIVE'
      ? 'Fed by live session data'
      : feed === 'MIX'
        ? 'Labels live, body illustrative'
        : 'Static reference figures — illustrative';

  return (
    <div
      data-testid="widget-stage"
      data-minimized={minimized ? 'true' : undefined}
      className="absolute right-6 top-1/2 -translate-y-1/2 z-20 w-[min(440px,92%)] max-h-[76%] flex flex-col rounded-2xl bg-[#0E1117]/92 backdrop-blur-2xl border border-white/10 shadow-2xl overflow-hidden"
    >
      <div className="flex items-center justify-between gap-2 px-4 py-2.5 border-b border-white/[0.06] shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${meta.dot}`} />
          <Icon className="w-3.5 h-3.5 text-slate-300 shrink-0" />
          <span className="text-[11px] font-semibold text-white truncate">{widget.title}</span>
          <span
            title={feedTitle}
            className={`px-1.5 py-0.5 rounded font-code text-[9px] shrink-0 ${
              feed === 'LIVE'
                ? 'bg-emerald-400/10 text-emerald-400'
                : feed === 'MIX'
                  ? 'bg-amber-400/10 text-amber-300'
                  : 'bg-white/[0.06] text-slate-400'
            }`}
          >
            {feed}
          </span>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button
            aria-label={minimized ? 'Expand widget' : 'Minimize widget'}
            data-testid="widget-minimize"
            onClick={() => setMinimized((v) => !v)}
            className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            {minimized ? <Plus className="w-3.5 h-3.5" /> : <Minus className="w-3.5 h-3.5" />}
          </button>
          <button
            aria-label="Dismiss widget"
            data-testid="widget-dismiss"
            onClick={onDismiss}
            className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
      {!minimized && (
      <div className="overflow-y-auto overflow-x-hidden p-3">
        {widget.kind === 'circuit' && (
          <QuantumCircuitQIREditorWidget
            qubits={qf?.qubit_count}
            circuitDepth={qf?.circuit_depth}
          />
        )}
        {widget.kind === 'shadows' && (
          <ShadowTomographyRadarWidget numQubits={qf?.qubit_count} />
        )}
        {widget.kind === 'molecule' && (
          <MolecularActiveSpaceWidget
            moleculeName={qf?.target_molecule || campaign?.target_system || undefined}
            numElectrons={qf?.active_electrons}
            numOrbitals={qf?.active_orbitals}
            homoLumoGapHa={
              typeof campaign?.classical_anchor?.rhf_mean_field_energy === 'number' && typeof campaign?.classical_anchor?.fci_exact_energy === 'number'
                ? Math.abs(campaign.classical_anchor.rhf_mean_field_energy - campaign.classical_anchor.fci_exact_energy)
                : undefined
            }
          />
        )}
        {widget.kind === 'mandate' && (() => {
          const estRaw = qf?.estimated_spend_usd;
          const priced = typeof estRaw === 'number' && Number.isFinite(estRaw);
          const capFinite = typeof spendCapUsd === 'number' && Number.isFinite(spendCapUsd);
          const spendFinite = typeof spendUsd === 'number' && Number.isFinite(spendUsd);
          const remaining = capFinite && spendFinite ? Math.max(0, (spendCapUsd as number) - (spendUsd as number)) : null;
          const breached = capFinite && spendFinite && (spendUsd as number) > (spendCapUsd as number);
          return (
          <div className="rounded-2xl border border-amber-500/25 bg-black/50 p-4 space-y-3">
            <div className="text-xs text-white font-medium">
              Dispatch on {qf?.recommended_backend || 'selected backend'}
            </div>
            <div className="grid grid-cols-2 gap-3 text-xs">
              <div>
                <span className="text-white/40 block text-[10px]">Est. spend</span>
                {priced ? (
                  <span className="font-code text-amber-300 font-bold">
                    ${(estRaw as number).toFixed(2)}
                  </span>
                ) : (
                  <span className="font-code text-amber-300 font-bold">UNPRICED — authorization disabled</span>
                )}
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Budget left</span>
                <span className={`font-code ${breached ? 'text-rose-300 font-bold' : 'text-slate-200'}`}>
                  {remaining != null ? `$${remaining.toFixed(2)}` : '—'}{breached ? ' · OVER CAP' : ''}
                </span>
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Qubits</span>
                <span className="font-code text-slate-200">{qf?.qubit_count ?? '—'}</span>
              </div>
              <div>
                <span className="text-white/40 block text-[10px]">Canary fidelity</span>
                <span className="font-code text-slate-200">
                  {typeof qf?.clifford_fidelity === 'number' && Number.isFinite(qf.clifford_fidelity) ? qf.clifford_fidelity.toFixed(4) : '—'}
                </span>
              </div>
            </div>
            {gateOpen ? (
              <button
                onClick={onRequestGateFocus}
                disabled={!priced}
                title={!priced ? 'Disabled: unpriced — authorization disabled' : undefined}
                className="w-full py-2 px-3 rounded-xl bg-amber-500/15 hover:bg-amber-500/25 border border-amber-400/40 text-amber-200 font-semibold text-xs flex items-center justify-center gap-2 transition-all disabled:opacity-40"
              >
                <Fingerprint className="w-4 h-4" />
                <span>{priced ? 'Review at Action Gate' : 'UNPRICED — gate disabled'}</span>
              </button>
            ) : isMeteredBackend(qf?.recommended_backend) ? (
              <p className="text-[10px] font-code text-slate-500">
                Gate already decided this session — card is read-only.
              </p>
            ) : (
              <p className="text-[10px] font-code text-slate-500">
                Local simulator needs no authorization (L1 auto-allow).
              </p>
            )}
          </div>
          );
        })()}
        {widget.kind === 'job' && widget.jobId && widget.sessionId && (
          <JobDetailWidget jobId={widget.jobId} sessionId={widget.sessionId} />
        )}
        {widget.kind === 'job' && (!widget.jobId || !widget.sessionId) && (
          <div className="rounded-2xl border border-rose-500/30 bg-rose-950/30 p-4 text-xs text-rose-200">
            Job widget requires a job id and session — nothing to show. Select a completed job to inspect it.
          </div>
        )}
      </div>
      )}
    </div>
  );
}
