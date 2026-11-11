'use client';

import React, { useState } from 'react';
import {
  Code2,
  Atom,
  Cpu,
  ShieldCheck,
  Radio,
  Compass,
  KeyRound,
  Sparkles,
} from 'lucide-react';
import type { CampaignState } from '@/types';
import {
  MolecularActiveSpaceWidget,
  QuantumCircuitQIREditorWidget,
  Lean4GoalTreeInspectorWidget,
  ShadowTomographyRadarWidget,
  EpistemicManifoldSurfaceWidget,
  CryptographicMandateCardWidget,
} from '../widgets';

type WidgetTab = 'all' | 'molecule' | 'circuit' | 'lean4' | 'shadows' | 'manifold' | 'mandate';

export function GenerativeUIStudio({
  campaign,
  tenantId,
  onOpenObservatory,
}: {
  campaign?: CampaignState | null;
  tenantId?: string;
  onOpenObservatory?: () => void;
}) {
  const [activeWidgetTab, setActiveWidgetTab] = useState<WidgetTab>('all');
  const hasLive = campaign?.quantum_factors != null &&
    (campaign.notebook != null || campaign.epistemics != null || (campaign.trajectory?.length ?? 0) > 0);

  const tabs: { id: WidgetTab; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
    { id: 'all', label: 'All Widgets', icon: Sparkles },
    { id: 'molecule', label: '3D Active Space', icon: Atom },
    { id: 'circuit', label: 'Circuit QIR', icon: Cpu },
    { id: 'lean4', label: 'Lean 4 Goals', icon: ShieldCheck },
    { id: 'shadows', label: 'Shadow Tomography', icon: Radio },
    { id: 'manifold', label: 'Epistemic Manifold', icon: Compass },
    { id: 'mandate', label: 'AP2 Spend Card', icon: KeyRound },
  ];

  const qf = campaign?.quantum_factors;

  return (
    <div className="space-y-6">
      {/* Surface Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 p-6 rounded-3xl bg-[var(--color-page)] border border-[var(--color-border)]">
        <div className="space-y-1">
          <div className="flex items-center space-x-2">
            <span className="px-2.5 py-0.5 rounded-full text-[12px] font-mono tracking-wider uppercase bg-[var(--color-soft-lilac-wash)] text-[var(--color-soft-lilac-ink)] border border-[var(--color-soft-lilac-edge)]">
              Surface 9 · Scientific Widgets
            </span>
            <span className="text-[var(--color-faint)] text-xs">• Agent-to-User Generative Scientific Widgets</span>
          </div>
          <h2 className="text-2xl font-bold text-[var(--color-text)] tracking-tight flex items-center space-x-2">
            <Code2 className="w-6 h-6 text-[var(--color-soft-lilac-ink)]" />
            <span>Generative Scientific UI Studio</span>
          </h2>
          <p className="text-xs text-[var(--color-muted)] max-w-2xl">
            Interactive visual components backed by campaign proposals and measured receipts: molecular active spaces, post-QIR circuits, Lean 4 goals, classical shadows, and spend mandate cards.
          </p>
        </div>

        {/* Filter Pills */}
        <div className="flex items-center flex-wrap gap-1 p-1 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)]">
          {tabs.map((t) => {
            const Icon = t.icon;
            const isSelected = activeWidgetTab === t.id;
            return (
              <button
                key={t.id}
                onClick={() => setActiveWidgetTab(t.id)}
                className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs transition ${
                  isSelected
                    ? 'bg-[var(--color-soft-lilac-wash)] border border-[var(--color-soft-lilac-edge)] text-[var(--color-text)] font-medium shadow-[var(--shadow-card)]'
                    : 'text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)]'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{t.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Widget Showcase Grid */}
      {!hasLive ? (
        <div className="rounded-3xl border border-[var(--color-border)] bg-[var(--color-page)] p-8 text-center space-y-2">
          <p className="text-sm font-medium text-[var(--color-text)]">No measured campaign data for the widget gallery.</p>
          <p className="text-xs text-[var(--color-muted)] max-w-xl mx-auto">
            Widgets appear after a campaign has a measured notebook, epistemic claim, or trajectory. A proposal alone does not populate them with fabricated values.
          </p>
        </div>
      ) : (
      <div className="space-y-6">
        {(activeWidgetTab === 'all' || activeWidgetTab === 'molecule') && (
          <MolecularActiveSpaceWidget
            moleculeName={qf?.target_molecule || campaign?.target_system || undefined}
            numElectrons={qf?.active_electrons}
            numOrbitals={qf?.active_orbitals}
            onOpenObservatory={onOpenObservatory}
          />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'circuit') && (
          <QuantumCircuitQIREditorWidget
            qubits={qf?.qubit_count}
            circuitDepth={qf?.circuit_depth}
          />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'lean4') && (
          <Lean4GoalTreeInspectorWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'shadows') && (
          <ShadowTomographyRadarWidget numQubits={qf?.qubit_count} />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'manifold') && (
          <EpistemicManifoldSurfaceWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'mandate') && (
          <CryptographicMandateCardWidget
            actionDescription={campaign?.research_goal || (campaign?.target_system ? `Execute VQE on ${campaign.target_system}` : undefined)}
            estimatedCostUsd={typeof qf?.estimated_spend_usd === 'number' ? qf.estimated_spend_usd : undefined}
            targetBackend={qf?.recommended_backend || 'local_simulator'}
            tenantId={tenantId}
            sessionId={campaign?.session_id ?? undefined}
            programQasm={qf?.qasm ?? undefined}
          />
        )}
      </div>
      )}
    </div>
  );
}
