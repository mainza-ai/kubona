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

import type { CanvasWidgetKind } from '@/types';

export type { CanvasWidgetKind } from '@/types';

export interface CanvasWidgetSpec {
  kind: CanvasWidgetKind;
  title: string;
  /** Job id for kind === 'job' (fetched live; never embedded). */
  jobId?: string;
  /** Owning session for kind === 'job' (backend scopes reads per session). */
  sessionId?: string;
}

const KIND_META: Record<CanvasWidgetKind, { icon: React.ComponentType<{ className?: string }>; dot: string }> = {
  circuit: { icon: CircuitBoard, dot: 'bg-accent' },
  shadows: { icon: Radio, dot: 'bg-accent' },
  molecule: { icon: Atom, dot: 'bg-warning' },
  job: { icon: CircuitBoard, dot: 'bg-success' },
  mandate: { icon: KeyRound, dot: 'bg-warning' },
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
  // Feed honesty per kind: job/mandate read live session data; molecule uses
  // proposal counts; shadows/circuit have no measured body and show an empty
  // state instead of reference figures or a schematic circuit.
  const feed: 'LIVE' | 'PROPOSAL' | 'EMPTY' =
    widget.kind === 'job' || widget.kind === 'mandate'
      ? 'LIVE'
      : widget.kind === 'molecule'
        ? 'PROPOSAL'
        : 'EMPTY';
  const feedTitle =
    feed === 'LIVE'
      ? 'Fed by live session data'
      : feed === 'PROPOSAL'
        ? 'Proposal fields only'
        : 'No measured data for this widget';

  return (
    <div
      data-testid="widget-stage"
      data-minimized={minimized ? 'true' : undefined}
      className="absolute right-6 top-1/2 -translate-y-1/2 z-20 w-[min(440px,92%)] max-h-[76%] flex flex-col rounded-card bg-float border border-border-strong shadow-[var(--shadow-float)] overflow-hidden"
    >
      <div className="flex items-center justify-between gap-2 px-4 py-2.5 border-b border-border shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`w-1.5 h-1.5 rounded-capsule shrink-0 ${meta.dot}`} />
          <Icon className="w-3.5 h-3.5 text-label-secondary shrink-0" />
          <span className="text-[12px] font-semibold text-text truncate">{widget.title}</span>
          <span
            title={feedTitle}
            className={`px-1.5 py-0.5 rounded-chip font-code text-[12px] shrink-0 ${
              feed === 'LIVE'
                ? 'bg-success-wash text-success-ink'
                : feed === 'PROPOSAL'
                  ? 'bg-warning-wash text-warning-ink'
                  : 'bg-fill text-muted'
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
            className="p-1 rounded-control-compact text-muted hover:text-text hover:bg-fill-hover transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
          >
            {minimized ? <Plus className="w-3.5 h-3.5" /> : <Minus className="w-3.5 h-3.5" />}
          </button>
          <button
            aria-label="Dismiss widget"
            data-testid="widget-dismiss"
            onClick={onDismiss}
            className="p-1 rounded-control-compact text-muted hover:text-text hover:bg-fill-hover transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
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
          <div className="rounded-card border border-warning-edge bg-surface p-4 space-y-3">
            <div className="text-label text-text font-medium">
              Dispatch on {qf?.recommended_backend || 'selected backend'}
            </div>
            <div className="grid grid-cols-2 gap-3 text-label">
              <div>
                <span className="text-muted block text-[12px]">Est. spend</span>
                {priced ? (
                  <span className="font-code text-warning-ink font-bold">
                    ${(estRaw as number).toFixed(2)}
                  </span>
                ) : (
                  <span className="font-code text-warning-ink font-bold">UNPRICED — authorization disabled</span>
                )}
              </div>
              <div>
                <span className="text-muted block text-[12px]">Budget left</span>
                <span className={`font-code ${breached ? 'text-danger-ink font-bold' : 'text-text'}`}>
                  {remaining != null ? `$${remaining.toFixed(2)}` : '—'}{breached ? ' · OVER CAP' : ''}
                </span>
              </div>
              <div>
                <span className="text-muted block text-[12px]">Qubits</span>
                <span className="font-code text-text">{qf?.qubit_count ?? '—'}</span>
              </div>
              <div>
                <span className="text-muted block text-[12px]">Canary fidelity</span>
                <span className="font-code text-text">
                  {typeof qf?.clifford_fidelity === 'number' && Number.isFinite(qf.clifford_fidelity) ? qf.clifford_fidelity.toFixed(4) : '—'}
                </span>
              </div>
            </div>
            {gateOpen ? (
              <button
                onClick={onRequestGateFocus}
                disabled={!priced}
                title={!priced ? 'Disabled: unpriced — authorization disabled' : undefined}
                className="w-full py-2 px-3 rounded-row bg-warning-wash hover:opacity-90 border border-warning-edge text-warning-ink font-semibold text-label flex items-center justify-center gap-2 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] disabled:opacity-40"
              >
                <Fingerprint className="w-4 h-4" />
                <span>{priced ? 'Review at Action Gate' : 'UNPRICED — gate disabled'}</span>
              </button>
            ) : isMeteredBackend(qf?.recommended_backend) ? (
              <p className="text-[12px] font-code text-faint">
                Gate already decided this session — card is read-only.
              </p>
            ) : (
              <p className="text-[12px] font-code text-faint">
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
          <div className="rounded-card border border-danger-edge bg-danger-wash p-4 text-label text-danger-ink">
            Job widget requires a job id and session — nothing to show. Select a completed job to inspect it.
          </div>
        )}
      </div>
      )}
    </div>
  );
}
