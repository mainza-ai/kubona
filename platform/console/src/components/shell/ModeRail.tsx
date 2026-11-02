'use client';

import React from 'react';
import { BadgeCheck, LayoutDashboard, Lock, Network, Settings } from 'lucide-react';

export type ShellMode = 'workbench' | 'evidence' | 'operations' | 'settings';

const MODES: Array<{
  id: ShellMode;
  label: string;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
  testId: string;
}> = [
  { id: 'workbench', label: 'Workbench', description: 'Campaign canvas, runs, measurements', icon: LayoutDashboard, testId: 'dock-arena' },
  { id: 'evidence', label: 'Evidence', description: 'Proofs, manuscripts, tribunal, codesign', icon: BadgeCheck, testId: 'dock-proofs' },
  { id: 'operations', label: 'Operations', description: 'Fleet, mesh, guardrails, models, labs', icon: Network, testId: 'dock-mesh' },
  { id: 'settings', label: 'Settings', description: 'Model, quantum, vault, and system settings', icon: Settings, testId: 'dock-settings' },
];

export function ModeRail({
  active,
  onSelect,
  airGapSovereign,
  onOpenCampaigns,
}: {
  active: ShellMode;
  onSelect: (mode: ShellMode) => void;
  airGapSovereign: boolean;
  onOpenCampaigns: () => void;
}) {
  return (
    <aside aria-label="Primary modes" className="relative z-30 w-14 lg:w-16 h-full flex flex-col items-center justify-between py-5 border-r border-white/5 bg-[#0A0C10] shrink-0">
      <div className="flex flex-col items-center gap-6">
        <div title="Kubona — sovereign research console" className="flex items-center justify-center">
          <div className="w-10 h-10 rounded-xl bg-cyan-400/10 border border-cyan-400/40 flex items-center justify-center">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" className="text-cyan-400" aria-hidden="true">
              <polygon points="12,2 22,8.5 22,15.5 12,22 2,15.5 2,8.5" stroke="currentColor" strokeWidth="1.75" strokeLinejoin="round" />
              <circle cx="12" cy="12" r="3" fill="currentColor" />
            </svg>
          </div>
        </div>

        <div className="w-6 h-[1px] bg-white/10" />

        <nav aria-label="Primary modes" className="flex flex-col items-center gap-3">
          {MODES.map((mode) => {
            const Icon = mode.icon;
            const isActive = active === mode.id;
            return (
              <button
                key={mode.id}
                type="button"
                data-testid={mode.testId}
                aria-label={mode.label}
                aria-pressed={isActive}
                aria-current={isActive ? 'page' : undefined}
                onClick={() => onSelect(mode.id)}
                title={`${mode.label} — ${mode.description}`}
                className={`group relative w-11 h-11 rounded-xl flex items-center justify-center transition-all hover:scale-105 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                  isActive
                    ? 'bg-cyan-400/10 border border-cyan-400/40 text-cyan-400'
                    : 'text-slate-400 hover:text-white hover:bg-white/5 border border-transparent hover:border-white/10'
                }`}
              >
                <Icon className="w-[21px] h-[21px]" />
                <span className="sr-only">{mode.label}</span>
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
          className={`w-9 h-9 rounded-lg flex items-center justify-center border cursor-default focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
            airGapSovereign ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20' : 'text-amber-400 bg-amber-400/10 border-amber-400/20'
          }`}
        >
          <Lock className="w-[18px] h-[18px]" />
        </div>

        <button
          type="button"
          aria-label="Campaigns"
          title="Campaigns"
          onClick={onOpenCampaigns}
          className="w-8 h-8 rounded-full bg-[var(--color-border-strong)] p-[1px] cursor-pointer hover:scale-105 transition-transform focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
        >
          <span className="w-full h-full rounded-full bg-black flex items-center justify-center text-[12px] font-code font-bold text-white">KB</span>
        </button>
      </div>
    </aside>
  );
}
