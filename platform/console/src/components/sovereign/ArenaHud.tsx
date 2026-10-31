'use client';

import React from 'react';
import { Orbit, GitBranch, Radar, Zap, Crosshair, AlertTriangle } from 'lucide-react';

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

function fmt(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(2) : '—';
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
  const spendFinite = typeof spendUsd === 'number' && Number.isFinite(spendUsd);
  const capFinite = typeof spendCapUsd === 'number' && Number.isFinite(spendCapUsd) && spendCapUsd > 0;
  const overCap = spendFinite && capFinite && (spendUsd as number) > (spendCapUsd as number);
  return (
    <header className="absolute top-4 left-6 right-6 z-20 flex flex-wrap items-center gap-2 justify-between pointer-events-none">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 min-w-0 max-w-full pointer-events-auto bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 rounded-2xl sm:rounded-full px-4 py-2 shadow-2xl">
        <span className={`w-2 h-2 rounded-full ${statusText ? 'bg-cyan-400 animate-ping' : 'bg-emerald-400 animate-pulse'}`} />
        <span className="font-code text-xs text-white font-medium tracking-wide">{targetLabel}</span>
        <span className="text-white/20">|</span>
        {statusText ? (
          <span data-testid="hud-status" aria-live="polite" aria-atomic="true" className="font-code text-[11px] text-cyan-300">{statusText}</span>
        ) : (
          <span className="font-code text-[11px] text-cyan-400">{swarmDetail}</span>
        )}
      </div>

      <div
        role="tablist"
        aria-label="Arena view mode"
        className="pointer-events-auto bg-[#0E1117]/85 backdrop-blur-xl border border-white/10 rounded-2xl sm:rounded-full p-1 flex items-center max-w-full overflow-x-auto shadow-2xl"
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
              className={`px-3.5 py-1.5 rounded-full text-xs font-medium flex shrink-0 items-center gap-1.5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                isActive ? 'bg-white/10 text-white' : 'text-slate-400 hover:text-white'
              }`}
            >
              <Icon className={`w-[15px] h-[15px] ${isActive ? 'text-cyan-400' : ''}`} />
              <span>{m.label}</span>
            </button>
          );
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2 min-w-0 max-w-full pointer-events-auto">
        <div
          className={`bg-[#0E1117]/80 backdrop-blur-xl border rounded-full px-3.5 py-2 flex flex-wrap items-center gap-3 max-w-full shadow-2xl ${overCap ? 'border-rose-500/60' : 'border-white/10'}`}
          data-testid="hud-spend"
          data-breach={overCap ? 'true' : undefined}
        >
          <div className="flex items-center gap-1.5" title={capFinite ? `$${fmt(spendUsd)} of $${fmt(spendCapUsd)} cap` : `$${fmt(spendUsd)} (no cap)`}>
            <Zap className={`w-4 h-4 ${overCap ? 'text-rose-400' : 'text-amber-400'}`} />
            <span className={`font-code text-xs font-medium ${overCap ? 'text-rose-300 font-bold' : 'text-amber-400'}`}>
              ${fmt(spendUsd)}
            </span>
            {overCap && (
              <span className="flex items-center gap-1 font-code text-[10px] text-rose-200 bg-rose-950/70 border border-rose-500/50 rounded-full px-1.5 py-0.5">
                <AlertTriangle className="w-3 h-3" />
                OVER CAP
              </span>
            )}
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
