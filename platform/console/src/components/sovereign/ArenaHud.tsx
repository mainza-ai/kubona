'use client';

import React from 'react';
import { Orbit, GitBranch, Radar, Zap, Crosshair } from 'lucide-react';

export type ArenaView = 'orbital' | 'proof-dag' | 'hardware-qpu';

const VIEW_MODES: Array<{
  id: ArenaView;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}> = [
  { id: 'orbital', label: 'Orbital Space', icon: Orbit },
  { id: 'proof-dag', label: 'Proof DAG', icon: GitBranch },
  { id: 'hardware-qpu', label: 'Hardware QPU', icon: Radar },
];

interface ArenaHudProps {
  targetLabel: string;
  swarmDetail: string;
  view: ArenaView;
  onViewChange: (view: ArenaView) => void;
  spendUsd: number;
  spendCapUsd: number;
  sessionElapsed: string;
  statusText?: string | null;
  onRecenter: () => void;
}

export function ArenaHud({
  targetLabel,
  swarmDetail,
  view,
  onViewChange,
  spendUsd,
  spendCapUsd,
  sessionElapsed,
  statusText,
  onRecenter,
}: ArenaHudProps) {
  return (
    <header className="absolute top-4 left-6 right-6 z-20 flex items-center justify-between pointer-events-none">
      <div className="flex items-center gap-3 pointer-events-auto bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 rounded-full px-4 py-2 shadow-2xl">
        <span className={`w-2 h-2 rounded-full ${statusText ? 'bg-cyan-400 animate-ping' : 'bg-emerald-400 animate-pulse'}`} />
        <span className="font-code text-xs text-white font-medium tracking-wide">{targetLabel}</span>
        <span className="text-white/20">|</span>
        {statusText ? (
          <span data-testid="hud-status" className="font-code text-[11px] text-cyan-300">{statusText}</span>
        ) : (
          <span className="font-code text-[11px] text-cyan-400">{swarmDetail}</span>
        )}
      </div>

      <div
        role="tablist"
        aria-label="Arena view mode"
        className="pointer-events-auto bg-[#0E1117]/85 backdrop-blur-xl border border-white/10 rounded-full p-1 flex items-center shadow-2xl"
      >
        {VIEW_MODES.map((m) => {
          const Icon = m.icon;
          const isActive = view === m.id;
          return (
            <button
              key={m.id}
              role="tab"
              aria-selected={isActive}
              data-testid={`arena-view-${m.id}`}
              onClick={() => onViewChange(m.id)}
              className={`px-3.5 py-1.5 rounded-full text-xs font-medium flex items-center gap-1.5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                isActive ? 'bg-white/10 text-white' : 'text-slate-400 hover:text-white'
              }`}
            >
              <Icon className={`w-[15px] h-[15px] ${isActive ? 'text-cyan-400' : ''}`} />
              <span>{m.label}</span>
            </button>
          );
        })}
      </div>

      <div className="flex items-center gap-2 pointer-events-auto">
        <div className="bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 rounded-full px-3.5 py-2 flex items-center gap-3 shadow-2xl">
          <div className="flex items-center gap-1.5" title={`$${spendUsd.toFixed(2)} of $${spendCapUsd.toFixed(2)} cap`}>
            <Zap className="w-4 h-4 text-amber-400" />
            <span className="font-code text-xs text-amber-400 font-medium">
              ${spendUsd.toFixed(2)}
            </span>
          </div>
          <span className="text-white/20">|</span>
          <div className="font-code text-[11px] text-slate-300">T+ {sessionElapsed}</div>
        </div>

        <button
          aria-label="Recenter viewport"
          onClick={onRecenter}
          className="w-9 h-9 rounded-full bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 text-slate-300 hover:text-white flex items-center justify-center hover:scale-105 transition-all shadow-2xl focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <Crosshair className="w-[18px] h-[18px]" />
        </button>
      </div>
    </header>
  );
}
