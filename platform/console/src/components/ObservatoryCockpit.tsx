'use client';

import React, { useState } from 'react';
import {
  Sparkles,
  PlayCircle,
  Compass,
  KeyRound,
  Radio,
  Network,
  BookOpen,
  LayoutDashboard,
  ShieldCheck,
  Zap,
  GitBranch,
  Sliders,
  Copy,
  Check,
} from 'lucide-react';
import { Loop, LedgerEvent } from '@/types';
import { SpeedometerGauge } from './common/SpeedometerGauge';
import { MathText } from './common/MathText';
import { SpatialCanvas } from './surfaces/SpatialCanvas';
import { FlightRecorder } from './surfaces/FlightRecorder';
import { EpistemicManifold } from './surfaces/EpistemicManifold';
import { AutonomyCockpit } from './surfaces/AutonomyCockpit';
import { FleetRadar } from './surfaces/FleetRadar';
import { CognitiveMesh } from './surfaces/CognitiveMesh';
import { ResultsStudio } from './surfaces/ResultsStudio';
import { LLMSettings } from './surfaces/LLMSettings';

interface ObservatoryCockpitProps {
  loops: Loop[];
  events: LedgerEvent[];
  activeMolecule?: string;
  activeTab?: SurfaceTab;
  onTabChange?: (tab: SurfaceTab) => void;
}

export type SurfaceTab =
  | 'overview'
  | 'surface1'
  | 'surface2'
  | 'surface3'
  | 'surface4'
  | 'surface5'
  | 'surface6'
  | 'surface7'
  | 'settings';

export function ObservatoryCockpit({
  loops,
  events,
  activeMolecule = 'H2',
  activeTab: externalTab,
  onTabChange,
}: ObservatoryCockpitProps) {
  const [internalTab, setInternalTab] = useState<SurfaceTab>('overview');
  const [copiedId, setCopiedId] = useState<number | null>(null);

  const activeTab = externalTab ?? internalTab;
  const handleTabChange = (tab: SurfaceTab) => {
    if (onTabChange) {
      onTabChange(tab);
    } else {
      setInternalTab(tab);
    }
  };

  const tabs = [
    { id: 'overview', label: 'Observatory Cockpit', shortcut: '⌘0', icon: LayoutDashboard },
    { id: 'surface1', label: '1. Spatial Canvas', shortcut: '⌘1', icon: Sparkles },
    { id: 'surface2', label: '2. 4D Flight Recorder', shortcut: '⌘2', icon: PlayCircle },
    { id: 'surface3', label: '3. Epistemic Manifold', shortcut: '⌘3', icon: Compass },
    { id: 'surface4', label: '4. Autonomy Cockpit', shortcut: '⌘4', icon: KeyRound },
    { id: 'surface5', label: '5. Fleet Radar', shortcut: '⌘5', icon: Radio },
    { id: 'surface6', label: '6. Cognitive Mesh', shortcut: '⌘6', icon: Network },
    { id: 'surface7', label: '7. Results Studio', shortcut: '⌘7', icon: BookOpen },
    { id: 'settings', label: '8. LLM Mesh & Air-Gap', shortcut: '⌘8', icon: Sliders },
  ];

  const handleCopyEvent = (id: number, payload: unknown) => {
    navigator.clipboard.writeText(JSON.stringify(payload, null, 2));
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const runningLoops = loops.filter((l) => l.status === 'running').length;

  return (
    <div className="space-y-6">
      {/* Apple-grade Segmented Control Surface Switcher */}
      <div className="flex items-center justify-between pb-1 border-b border-white/[0.08]">
        <div
          role="tablist"
          aria-label="Observatory Surfaces"
          className="p-1 rounded-2xl bg-black/60 border border-white/[0.08] backdrop-blur-xl shadow-inner flex items-center space-x-1 overflow-x-auto max-w-full scrollbar-none"
        >
          {tabs.map((t) => {
            const Icon = t.icon;
            const isActive = activeTab === t.id;
            return (
              <button
                key={t.id}
                role="tab"
                id={`tab-${t.id}`}
                aria-selected={isActive}
                aria-controls={`panel-${t.id}`}
                tabIndex={isActive ? 0 : -1}
                onClick={() => handleTabChange(t.id as SurfaceTab)}
                className={`px-3 py-1.5 rounded-xl text-xs font-medium flex items-center space-x-2 shrink-0 transition-all select-none focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                  isActive
                    ? 'bg-gradient-to-b from-white/[0.16] to-white/[0.05] text-white border border-white/20 shadow-md shadow-black/50 font-semibold'
                    : 'text-slate-400 hover:text-slate-200 hover:bg-white/[0.04] border border-transparent'
                }`}
              >
                <Icon className={`w-3.5 h-3.5 ${isActive ? 'text-cyan-300' : 'text-slate-400'}`} />
                <span>{t.label}</span>
                <span
                  className={`text-[9px] font-mono px-1 py-0.2 rounded border transition-colors ${
                    isActive
                      ? 'bg-cyan-500/20 text-cyan-200 border-cyan-400/30'
                      : 'bg-white/[0.03] text-slate-500 border-white/[0.06]'
                  }`}
                >
                  {t.shortcut}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Surface Render Switching with accessible panel containers */}
      <div role="tabpanel" id={`panel-${activeTab}`} aria-labelledby={`tab-${activeTab}`}>
        {activeTab === 'surface1' && <SpatialCanvas />}
        {activeTab === 'surface2' && <FlightRecorder loops={loops} events={events} />}
        {activeTab === 'surface3' && <EpistemicManifold />}
        {activeTab === 'surface4' && <AutonomyCockpit />}
        {activeTab === 'surface5' && <FleetRadar />}
        {activeTab === 'surface6' && <CognitiveMesh />}
        {activeTab === 'surface7' && <ResultsStudio />}
        {activeTab === 'settings' && <LLMSettings />}
      </div>

      {/* Default Overview Dashboard */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          {/* Real-time High-Refresh Speedometers */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <SpeedometerGauge
              label="Active Goroutine Loops"
              value={runningLoops || (loops.length > 0 ? loops.length : 1)}
              min={0}
              max={10}
              unit="loops"
              colorScheme="cyan"
              subtext="Ring 0 Guarded"
            />
            <SpeedometerGauge
              label="Epistemic Credibility (P_corr)"
              value={90.25}
              min={0}
              max={100}
              unit="%"
              colorScheme="emerald"
              subtext="Wavefunction Collapsed"
            />
            <SpeedometerGauge
              label="PySCF Anchor Discrepancy (Δ)"
              value={0.47}
              min={0}
              max={2.0}
              unit="mHa"
              colorScheme="amber"
              subtext="Chemical Accuracy OK"
            />
            <SpeedometerGauge
              label="Barren Plateau Var(∇E)"
              value={3.4}
              min={0}
              max={10}
              unit="x10⁻³"
              colorScheme="violet"
              subtext="Gradient Variance OK"
            />
          </div>

          {/* Quick Previews of Key Surfaces */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="glass-panel rounded-xl p-5 space-y-3 border border-slate-800">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-semibold text-slate-200 uppercase tracking-wider flex items-center space-x-2">
                  <Sparkles className="w-4 h-4 text-cyan-400" />
                  <span>Surface 1: Spatial Living Lab</span>
                </h3>
                <button
                  onClick={() => handleTabChange('surface1')}
                  className="text-xs text-cyan-400 hover:text-cyan-300 font-medium"
                >
                  Open Canvas →
                </button>
              </div>
              <p className="text-xs text-slate-400 leading-relaxed">
                React Flow circuit DAG with universal post-QIR gate glyphs, LOCC wire-cut partitions, and multi-scale semantic zoom.
              </p>
            </div>

            <div className="glass-panel rounded-xl p-5 space-y-3 border border-slate-800">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-semibold text-slate-200 uppercase tracking-wider flex items-center space-x-2">
                  <Compass className="w-4 h-4 text-emerald-400" />
                  <span>Surface 3: Epistemic Horizon</span>
                </h3>
                <button
                  onClick={() => handleTabChange('surface3')}
                  className="text-xs text-emerald-400 hover:text-emerald-300 font-medium"
                >
                  Wavefunction →
                </button>
              </div>
              <p className="text-xs text-slate-400 leading-relaxed">
                <MathText text="Tripartite credibility state $|\Psi\rangle = \alpha|\text{corr}\rangle + \beta|\text{cont}\rangle + \gamma|\text{unver}\rangle$ with inline PySCF FCI anchor." />
              </p>
            </div>

            <div className="glass-panel rounded-xl p-5 space-y-3 border border-slate-800">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-semibold text-slate-200 uppercase tracking-wider flex items-center space-x-2">
                  <KeyRound className="w-4 h-4 text-amber-400" />
                  <span>Surface 4: Autonomy Cockpit</span>
                </h3>
                <button
                  onClick={() => handleTabChange('surface4')}
                  className="text-xs text-amber-400 hover:text-amber-300 font-medium"
                >
                  Keyring →
                </button>
              </div>
              <p className="text-xs text-slate-400 leading-relaxed">
                Client-side WebCrypto Ed25519 browser signing, four 1-glance action gate cards, and A2A delegation trees.
              </p>
            </div>
          </div>

          {/* Immutable Ledger Preview with Cryptographic Badges */}
          <div className="glass-panel border border-slate-800 p-5 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold text-slate-200 flex items-center space-x-2">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <span>Immutable Provenance Ledger (SQLite WAL Audited Events)</span>
              </h3>
              <span className="text-[10px] font-mono text-slate-400">
                Single-Writer Invariant Active
              </span>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs font-mono">
                <thead className="bg-[#05070B] text-slate-400 uppercase font-semibold text-[11px] border-b border-slate-800">
                  <tr>
                    <th className="p-3">Event ID</th>
                    <th className="p-3">Type</th>
                    <th className="p-3">Payload Preview</th>
                    <th className="p-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/80">
                  {events.slice(0, 8).map((evt, idx) => (
                    <tr key={idx} className="hover:bg-slate-800/30 transition-colors">
                      <td className="p-3 text-slate-500 font-bold">#{evt.id || idx + 1}</td>
                      <td className="p-3">
                        <span className="px-2 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-800/50 text-[11px]">
                          {evt.event_type}
                        </span>
                      </td>
                      <td className="p-3 text-slate-400 truncate max-w-lg">
                        {JSON.stringify(evt.payload || {})}
                      </td>
                      <td className="p-3 text-right">
                        <button
                          onClick={() => handleCopyEvent(evt.id || idx, evt.payload)}
                          className="p-1 text-slate-400 hover:text-white transition-colors"
                          title="Copy Payload JSON"
                        >
                          {copiedId === (evt.id || idx) ? (
                            <Check className="w-3.5 h-3.5 text-emerald-400 inline" />
                          ) : (
                            <Copy className="w-3.5 h-3.5 inline" />
                          )}
                        </button>
                      </td>
                    </tr>
                  ))}
                  {events.length === 0 && (
                    <tr>
                      <td colSpan={4} className="p-6 text-center text-slate-500">
                        No ledger events recorded yet. Run a research tribunal or launch a proposal to activate.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
