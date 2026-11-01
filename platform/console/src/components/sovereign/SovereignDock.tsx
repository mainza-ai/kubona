'use client';

import React from 'react';
import {
  Orbit,
  History,
  Activity,
  Network,
  BadgeCheck,
  Cpu,
  Lock,
  SlidersHorizontal,
} from 'lucide-react';

export type DockSurface =
  | 'arena'
  | 'recorder'
  | 'manifold'
  | 'fleet'
  | 'proofs'
  | 'mesh';

export const DOCK_SURFACES: Array<{
  id: DockSurface;
  label: string;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
}> = [
  { id: 'arena', label: 'Active Research Arena', description: 'Living Swarm Universe', icon: Orbit },
  { id: 'recorder', label: '4D Flight Recorder', description: 'Trajectory scrubbing & forks', icon: History },
  { id: 'manifold', label: 'Epistemic Manifold', description: 'Curvature & belief topology', icon: Activity },
  { id: 'fleet', label: 'Quantum Fleet Radar', description: 'QuEra, IBM & Slurm HPC', icon: Network },
  { id: 'proofs', label: 'Merkle Proofs & Papers', description: 'Lean 4 ASTs & LaTeX', icon: BadgeCheck },
  { id: 'mesh', label: 'Local Sovereign Mesh', description: 'MLX · Ollama · Air-Gap', icon: Cpu },
];

interface SovereignDockProps {
  active: DockSurface;
  onSelect: (surface: DockSurface) => void;
  airGapSovereign: boolean;
  onOpenSettings: () => void;
  onOpenCampaigns: () => void;
}

export function SovereignDock({
  active,
  onSelect,
  airGapSovereign,
  onOpenSettings,
  onOpenCampaigns,
}: SovereignDockProps) {
  return (
    <aside
      aria-label="Sovereign dock"
      className="relative z-30 w-12 lg:w-16 h-full flex flex-col items-center justify-between py-5 border-r border-white/5 bg-[#0A0C10] shrink-0"
    >
      <div className="flex flex-col items-center gap-6">
        {/* Brand sigil */}
        <div
          title="Kubona — sovereign research console"
          className="flex items-center justify-center"
        >
          <div className="w-10 h-10 rounded-xl bg-cyan-400/10 border border-cyan-400/40 flex items-center justify-center">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" className="text-cyan-400">
              <polygon points="12,2 22,8.5 22,15.5 12,22 2,15.5 2,8.5" stroke="currentColor" strokeWidth="1.75" strokeLinejoin="round" />
              <circle cx="12" cy="12" r="3" fill="currentColor" />
            </svg>
          </div>
        </div>

        <div className="w-6 h-[1px] bg-white/10" />

        <nav aria-label="Surfaces" className="flex flex-col items-center gap-3">
          {DOCK_SURFACES.map((s) => {
            const Icon = s.icon;
            const isActive = active === s.id;
            return (
              <button
                key={s.id}
                data-testid={`dock-${s.id}`}
                aria-label={s.label}
                aria-pressed={isActive}
                aria-current={isActive ? 'page' : undefined}
                onClick={() => onSelect(s.id)}
                title={`${s.label} — ${s.description}`}
                className={`group relative w-11 h-11 rounded-xl flex items-center justify-center transition-all hover:scale-105 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                  isActive
                    ? 'bg-cyan-400/10 border border-cyan-400/40 text-cyan-400'
                    : 'text-slate-400 hover:text-white hover:bg-white/5 border border-transparent hover:border-white/10'
                }`}
              >
                <Icon className="w-[21px] h-[21px]" />
              </button>
            );
          })}
        </nav>
      </div>

      <div className="flex flex-col items-center gap-3">
        <div
          role="status"
          tabIndex={0}
          aria-label={airGapSovereign ? 'Air-gap: sovereign, zero outbound egress' : 'Air-gap: egress open, network path live'}
          title={airGapSovereign ? 'Air-gap enclave: sovereign, 0 outbound egress' : 'Air-gap enclave: egress open'}
          className={`group relative w-9 h-9 rounded-lg flex items-center justify-center border cursor-default focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
            airGapSovereign
              ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
              : 'text-amber-400 bg-amber-400/10 border-amber-400/20'
          }`}
        >
          <Lock className="w-[18px] h-[18px]" />
        </div>

        <button
          aria-label="Swarm parameters"
          title="Swarm parameters"
          onClick={onOpenSettings}
          className="group relative w-9 h-9 rounded-lg flex items-center justify-center text-slate-400 hover:text-white hover:bg-white/5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <SlidersHorizontal className="w-[19px] h-[19px]" />
        </button>

        <button
          aria-label="Campaigns"
          title="Campaigns"
          onClick={onOpenCampaigns}
          className="w-8 h-8 rounded-full bg-gradient-to-tr from-cyan-500 to-amber-500 p-[1.5px] cursor-pointer hover:scale-105 transition-transform focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <div className="w-full h-full rounded-full bg-black flex items-center justify-center text-[12px] font-code font-bold text-white">
            KB
          </div>
        </button>
      </div>
    </aside>
  );
}
