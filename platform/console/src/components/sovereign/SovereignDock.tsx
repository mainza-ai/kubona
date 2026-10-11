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

function DockTooltip({ title, sub }: { title: string; sub?: string }) {
  return (
    <div className="absolute left-16 px-3 py-1.5 rounded-lg bg-[#141720] border border-white/10 text-xs text-white whitespace-nowrap opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity duration-200 z-50 shadow-2xl">
      <div className="font-medium">{title}</div>
      {sub && <div className="text-[10px] text-slate-400">{sub}</div>}
    </div>
  );
}

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
      className="relative z-30 w-16 h-full flex flex-col items-center justify-between py-5 border-r border-white/5 bg-[#0A0C10]/90 backdrop-blur-2xl shrink-0"
    >
      <div className="flex flex-col items-center gap-6">
        {/* Brand sigil */}
        <div className="group relative flex items-center justify-center cursor-pointer">
          <div className="w-10 h-10 rounded-xl bg-cyan-400/10 border border-cyan-400/40 flex items-center justify-center transition-all duration-300 hover:scale-105 shadow-[0_0_20px_rgba(0,229,255,0.15)]">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" className="text-cyan-400">
              <polygon points="12,2 22,8.5 22,15.5 12,22 2,15.5 2,8.5" stroke="currentColor" strokeWidth="1.75" strokeLinejoin="round" />
              <circle cx="12" cy="12" r="3" fill="currentColor" />
            </svg>
          </div>
          <div className="absolute left-16 px-3 py-1.5 rounded-lg bg-[#141720] border border-white/10 text-xs text-white whitespace-nowrap opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity duration-200 z-50 shadow-2xl flex items-center gap-2">
            <span className="font-semibold tracking-wide">KUBONA</span>
            <span className="text-[10px] text-cyan-400 font-code">sovereign</span>
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
                onClick={() => onSelect(s.id)}
                className={`group relative w-11 h-11 rounded-xl flex items-center justify-center transition-all hover:scale-105 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                  isActive
                    ? 'bg-cyan-400/10 border border-cyan-400/40 text-cyan-400'
                    : 'text-slate-400 hover:text-white hover:bg-white/5 border border-transparent hover:border-white/10'
                }`}
              >
                <Icon className="w-[21px] h-[21px]" />
                <DockTooltip title={s.label} sub={s.description} />
              </button>
            );
          })}
        </nav>
      </div>

      <div className="flex flex-col items-center gap-3">
        <div
          className={`group relative w-9 h-9 rounded-lg flex items-center justify-center border cursor-default ${
            airGapSovereign
              ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
              : 'text-amber-400 bg-amber-400/10 border-amber-400/20'
          }`}
        >
          <Lock className="w-[18px] h-[18px]" />
          <div className="absolute left-16 px-3 py-1.5 rounded-lg bg-[#141720] border border-white/10 text-xs text-white whitespace-nowrap opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity duration-200 z-50 shadow-2xl">
            <span className={`font-medium ${airGapSovereign ? 'text-emerald-400' : 'text-amber-400'}`}>
              Air-Gap Enclave: {airGapSovereign ? 'Sovereign' : 'Egress Open'}
            </span>
            <span className="block text-[10px] text-slate-400 font-code">
              {airGapSovereign ? '0 Outbound Egress' : 'network path live'}
            </span>
          </div>
        </div>

        <button
          aria-label="Swarm parameters"
          onClick={onOpenSettings}
          className="group relative w-9 h-9 rounded-lg flex items-center justify-center text-slate-400 hover:text-white hover:bg-white/5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <SlidersHorizontal className="w-[19px] h-[19px]" />
          <DockTooltip title="Swarm Parameters" />
        </button>

        <button
          aria-label="Campaigns"
          onClick={onOpenCampaigns}
          className="w-8 h-8 rounded-full bg-gradient-to-tr from-cyan-500 to-amber-500 p-[1.5px] cursor-pointer hover:scale-105 transition-transform focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <div className="w-full h-full rounded-full bg-black flex items-center justify-center text-[11px] font-code font-bold text-white">
            KB
          </div>
        </button>
      </div>
    </aside>
  );
}
