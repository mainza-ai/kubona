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

export function GenerativeUIStudio({ campaign, tenantId }: { campaign?: CampaignState | null; tenantId?: string }) {
  const [activeWidgetTab, setActiveWidgetTab] = useState<WidgetTab>('all');
  const hasLive = campaign != null;

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
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 p-6 rounded-3xl bg-gradient-to-r from-purple-950/40 via-black to-blue-950/40 border border-white/[0.08] backdrop-blur-2xl">
        <div className="space-y-1">
          <div className="flex items-center space-x-2">
            <span className="px-2.5 py-0.5 rounded-full text-[10px] font-mono tracking-wider uppercase bg-purple-500/20 text-purple-300 border border-purple-500/30">
              Surface 9 · Preview Gallery
            </span>
            <span className="text-white/40 text-xs">• Agent-to-User Generative Scientific Widgets</span>
          </div>
          <h2 className="text-2xl font-bold text-white tracking-tight flex items-center space-x-2">
            <Code2 className="w-6 h-6 text-purple-400" />
            <span>Generative Scientific UI Studio</span>
          </h2>
          <p className="text-xs text-white/60 max-w-2xl">
            Interactive visual components dynamically rendered by autonomous agent collectives: 3D molecular active spaces, post-QIR circuits, Lean 4 goal trees, classical shadow tomography, and high-friction spend mandate cards.
          </p>
          {!hasLive && (
            <p className="text-[11px] font-code text-amber-300 border border-amber-400/40 bg-amber-950/40 rounded-xl px-3 py-2 mt-2">
              synthetic demo — not session data. Connect a live campaign to populate these widgets with measured values.
            </p>
          )}
        </div>

        {/* Filter Pills */}
        <div className="flex items-center flex-wrap gap-1 p-1 rounded-2xl bg-black/60 border border-white/[0.08]">
          {tabs.map((t) => {
            const Icon = t.icon;
            const isSelected = activeWidgetTab === t.id;
            return (
              <button
                key={t.id}
                onClick={() => setActiveWidgetTab(t.id)}
                className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs transition ${
                  isSelected
                    ? 'bg-purple-600/30 border border-purple-500/50 text-white font-medium shadow'
                    : 'text-white/60 hover:text-white hover:bg-white/[0.04]'
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
      <div className="space-y-6">
        {(activeWidgetTab === 'all' || activeWidgetTab === 'molecule') && (
          <MolecularActiveSpaceWidget
            moleculeName={qf?.target_molecule || campaign?.target_system || 'LiH'}
            numElectrons={qf?.active_electrons ?? 4}
            numOrbitals={qf?.active_orbitals ?? 4}
            homoLumoGapHa={
              typeof campaign?.classical_anchor?.rhf_mean_field_energy === 'number' && typeof campaign?.classical_anchor?.fci_exact_energy === 'number'
                ? Math.abs(campaign.classical_anchor.rhf_mean_field_energy - campaign.classical_anchor.fci_exact_energy)
                : 0.042
            }
          />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'circuit') && (
          <QuantumCircuitQIREditorWidget
            qubits={qf?.qubit_count ?? 4}
            circuitDepth={qf?.circuit_depth ?? 8}
          />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'lean4') && (
          <Lean4GoalTreeInspectorWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'shadows') && (
          <ShadowTomographyRadarWidget numQubits={qf?.qubit_count ?? 4} />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'manifold') && (
          <EpistemicManifoldSurfaceWidget
            currentCurvatureScalar={typeof campaign?.epistemics?.literature_drift_mha === 'number' ? -Math.max(0.5, campaign.epistemics.literature_drift_mha * 2.5) : undefined}
            fisherInfoNorm={typeof qf?.gradient_variance === 'number' ? Math.max(1.0, 1.0 / (qf.gradient_variance * 100 + 0.1)) : undefined}
            entropyReduction={typeof campaign?.epistemics?.p_correlated === 'number' ? (campaign.epistemics.p_correlated / 100) * 0.85 : undefined}
          />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'mandate') && (
          <CryptographicMandateCardWidget
            actionDescription={campaign?.research_goal || (campaign?.target_system ? `Execute VQE on ${campaign.target_system}` : undefined)}
            estimatedCostUsd={typeof qf?.estimated_spend_usd === 'number' ? qf.estimated_spend_usd : (qf ? 0.0 : undefined)}
            targetBackend={qf?.recommended_backend || 'local_simulator'}
            tenantId={tenantId}
            sessionId={campaign?.session_id ?? undefined}
            programQasm={qf?.qasm ?? undefined}
          />
        )}
      </div>
    </div>
  );
}
