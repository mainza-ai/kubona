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
import {
  MolecularActiveSpaceWidget,
  QuantumCircuitQIREditorWidget,
  Lean4GoalTreeInspectorWidget,
  ShadowTomographyRadarWidget,
  EpistemicManifoldSurfaceWidget,
  CryptographicMandateCardWidget,
} from '../widgets';

export function GenerativeUIStudio() {
  const [activeWidgetTab, setActiveWidgetTab] = useState<
    'all' | 'molecule' | 'circuit' | 'lean4' | 'shadows' | 'manifold' | 'mandate'
  >('all');

  const tabs = [
    { id: 'all', label: 'All Widgets', icon: Sparkles },
    { id: 'molecule', label: '3D Active Space', icon: Atom },
    { id: 'circuit', label: 'Circuit QIR', icon: Cpu },
    { id: 'lean4', label: 'Lean 4 Goals', icon: ShieldCheck },
    { id: 'shadows', label: 'Shadow Tomography', icon: Radio },
    { id: 'manifold', label: 'Epistemic Manifold', icon: Compass },
    { id: 'mandate', label: 'AP2 Spend Card', icon: KeyRound },
  ];

  return (
    <div className="space-y-6">
      {/* Surface Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 p-6 rounded-3xl bg-gradient-to-r from-purple-950/40 via-black to-blue-950/40 border border-white/[0.08] backdrop-blur-2xl">
        <div className="space-y-1">
          <div className="flex items-center space-x-2">
            <span className="px-2.5 py-0.5 rounded-full text-[10px] font-mono tracking-wider uppercase bg-purple-500/20 text-purple-300 border border-purple-500/30">
              Surface 9 · A2UI
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
        </div>

        {/* Filter Pills */}
        <div className="flex items-center flex-wrap gap-1 p-1 rounded-2xl bg-black/60 border border-white/[0.08]">
          {tabs.map((t) => {
            const Icon = t.icon;
            const isSelected = activeWidgetTab === t.id;
            return (
              <button
                key={t.id}
                onClick={() => setActiveWidgetTab(t.id as any)}
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
          <MolecularActiveSpaceWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'circuit') && (
          <QuantumCircuitQIREditorWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'lean4') && (
          <Lean4GoalTreeInspectorWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'shadows') && (
          <ShadowTomographyRadarWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'manifold') && (
          <EpistemicManifoldSurfaceWidget />
        )}

        {(activeWidgetTab === 'all' || activeWidgetTab === 'mandate') && (
          <CryptographicMandateCardWidget />
        )}
      </div>
    </div>
  );
}
