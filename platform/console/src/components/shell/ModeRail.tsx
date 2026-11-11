'use client';

import React from 'react';
import Image from 'next/image';
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
    <aside aria-label="Primary modes" className="relative z-30 w-14 lg:w-16 h-full flex flex-col items-center justify-between py-5 border-r border-[var(--color-border)] bg-[var(--color-chrome)] shrink-0">
      <div className="flex flex-col items-center gap-6">
        <div title="Kubona — sovereign research console" className="flex items-center justify-center">
          <Image
            src="/assets/milimo_kubona_logo.png"
            alt="Kubona"
            width={40}
            height={34}
            priority
            className="h-9 w-auto object-contain"
          />
        </div>

        <div className="w-6 h-[1px] bg-[var(--color-border-strong)]" />

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
                className={`group relative w-11 h-11 rounded-xl flex items-center justify-center transition-all hover:scale-105 focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                  isActive
                    ? 'bg-[var(--color-lit)] text-[var(--color-on-lit)] border border-[var(--color-border-strong)]'
                    : 'text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill)] border border-transparent hover:border-[var(--color-border)]'
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
          className="w-9 h-9 rounded-lg flex items-center justify-center border cursor-default focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
          style={
            airGapSovereign
              ? { color: 'var(--color-success)', background: 'color-mix(in oklab, var(--color-success) 14%, transparent)', borderColor: 'color-mix(in oklab, var(--color-success) 30%, transparent)' }
              : { color: 'var(--color-warning)', background: 'color-mix(in oklab, var(--color-warning) 14%, transparent)', borderColor: 'color-mix(in oklab, var(--color-warning) 30%, transparent)' }
          }
        >
          <Lock className="w-[18px] h-[18px]" />
        </div>

        <button
          type="button"
          aria-label="Campaigns"
          title="Campaigns"
          onClick={onOpenCampaigns}
          className="w-8 h-8 rounded-full bg-[var(--color-border-strong)] p-[1px] cursor-pointer hover:scale-105 transition-transform focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
        >
          <span className="w-full h-full rounded-full bg-[var(--color-raised)] flex items-center justify-center text-[12px] font-code font-bold text-[var(--color-text)]">KB</span>
        </button>
      </div>
    </aside>
  );
}
