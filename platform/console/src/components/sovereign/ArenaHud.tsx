'use client';

import React, { useEffect, useState } from 'react';
import { Orbit, GitBranch, Radar, Zap, Crosshair, AlertTriangle, LayoutDashboard, Box, ChevronDown } from 'lucide-react';
import { formatElapsed } from '@/lib/time';

export type ArenaView = 'workbench' | 'orbital' | 'proof-dag' | 'hardware-qpu';

const VIEW_MODES: Array<{
  id: ArenaView;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}> = [
  { id: 'workbench', label: 'Workbench', icon: LayoutDashboard },
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
  sessionElapsed?: string;
  sessionCreatedAt?: string;
  statusText?: string | null;
  onRecenter: () => void;
  showWorkbench?: boolean;
  campaignTitle?: string;
  onOpenCampaigns?: () => void;
  experimentLabOpen?: boolean;
  onOpenExperimentLab?: () => void;
  onOpenMoleculeView?: () => void;
}

function fmt(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(2) : '—';
}

function ElapsedClockTick({ createdAt }: { createdAt: string }) {
  const [text, setText] = useState(() => formatElapsed(createdAt));

  useEffect(() => {
    const timer = setInterval(() => setText(formatElapsed(createdAt)), 1000);
    return () => clearInterval(timer);
  }, [createdAt]);

  return <div className="font-code text-[12px] text-[var(--color-label-secondary)]">T+ {text}</div>;
}

function ElapsedClock({ createdAt, fallback = '00:00:00' }: { createdAt?: string; fallback?: string }) {
  if (!createdAt) {
    return <div className="font-code text-[12px] text-[var(--color-label-secondary)]">T+ {fallback}</div>;
  }
  return <ElapsedClockTick key={createdAt} createdAt={createdAt} />;
}

export const ArenaHud = React.memo(function ArenaHud({
  targetLabel,
  swarmDetail,
  view,
  onViewChange,
  spendUsd,
  spendCapUsd,
  sessionElapsed = '00:00:00',
  sessionCreatedAt,
  statusText,
  onRecenter,
  showWorkbench = true,
  campaignTitle,
  onOpenCampaigns,
  experimentLabOpen = false,
  onOpenExperimentLab,
  onOpenMoleculeView,
}: ArenaHudProps) {
  const spendFinite = typeof spendUsd === 'number' && Number.isFinite(spendUsd);
  const capFinite = typeof spendCapUsd === 'number' && Number.isFinite(spendCapUsd) && spendCapUsd > 0;
  const overCap = spendFinite && capFinite && (spendUsd as number) > (spendCapUsd as number);
  return (
    <header className="k-arena-hud relative z-20 shrink-0 px-4 py-2.5 flex flex-wrap items-center gap-2 justify-between border-b border-[var(--color-border)] bg-[var(--color-chrome)]/95">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 min-w-0 max-w-full bg-[var(--color-surface)] border border-[var(--color-border)] rounded-2xl sm:rounded-full px-3.5 py-1.5 shadow-[var(--shadow-raised)]">
        <span className="w-2 h-2 rounded-full" style={{ background: statusText ? 'var(--color-info)' : 'var(--color-success)' }} />
        <span className="font-code text-xs text-[var(--color-text)] font-medium tracking-wide truncate max-w-[14rem]">{targetLabel}</span>
        {onOpenCampaigns && (
          <>
            <span className="text-[var(--color-border-strong)]">|</span>
            <button
              type="button"
              onClick={onOpenCampaigns}
              aria-label="Switch campaign"
              title={campaignTitle ? `Campaign: ${campaignTitle}` : 'Switch campaign'}
              className="flex items-center gap-1 min-h-[24px] px-1.5 rounded-[var(--radius-inset)] text-xs font-medium text-[var(--color-label-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none max-w-[10rem]"
            >
              <span className="truncate">{campaignTitle || 'Campaigns'}</span>
              <ChevronDown className="w-3.5 h-3.5 shrink-0" />
            </button>
          </>
        )}
        <span className="text-[var(--color-border-strong)] hidden sm:inline">|</span>
        {statusText ? (
          <span data-testid="hud-status" aria-live="polite" aria-atomic="true" className="font-code text-[12px] text-[var(--color-info)]">{statusText}</span>
        ) : (
          <span className="font-code text-[12px] text-[var(--color-muted)] hidden sm:inline">{swarmDetail}</span>
        )}
      </div>

      <div
        role="tablist"
        aria-label="Arena view mode"
        className="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-2xl sm:rounded-full p-1 flex items-center max-w-full overflow-x-auto shadow-[var(--shadow-raised)]"
      >
        {VIEW_MODES.filter((m) => showWorkbench || m.id !== 'workbench').map((m) => {
          const Icon = m.icon;
          const isActive = view === m.id;
          return (
            <button
              key={m.id}
              role="tab"
              aria-selected={isActive}
              data-testid={`arena-view-${m.id}`}
              onClick={() => onViewChange(m.id)}
              className={`px-3 py-1 rounded-full text-xs font-medium flex shrink-0 items-center gap-1.5 transition-all focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                isActive ? 'bg-[var(--color-lit)] text-[var(--color-on-lit)]' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              <Icon className="w-[15px] h-[15px]" />
              <span>{m.label}</span>
            </button>
          );
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2 min-w-0 max-w-full">
        <div
          className={`bg-[var(--color-surface)] border rounded-full px-3.5 py-1.5 flex flex-wrap items-center gap-3 max-w-full shadow-[var(--shadow-raised)] ${overCap ? 'border-[var(--color-danger)]' : 'border-[var(--color-border)]'}`}
          data-testid="hud-spend"
          data-breach={overCap ? 'true' : undefined}
        >
          <div className="flex items-center gap-1.5" title={capFinite ? `$${fmt(spendUsd)} of $${fmt(spendCapUsd)} cap` : `$${fmt(spendUsd)} (no cap)`}>
            <Zap className="w-4 h-4" style={{ color: overCap ? 'var(--color-danger)' : 'var(--color-warning)' }} />
            <span className="font-code text-xs font-medium" style={{ color: overCap ? 'var(--color-danger)' : 'var(--color-warning)', fontWeight: overCap ? 700 : 500 }}>
              ${fmt(spendUsd)}
            </span>
            {overCap && (
              <span className="flex items-center gap-1 font-code text-[12px] text-[var(--color-danger)] bg-[var(--color-soft-rose-wash)] border border-[var(--color-soft-rose-edge)] rounded-full px-1.5 py-0.5">
                <AlertTriangle className="w-3 h-3" />
                OVER CAP
              </span>
            )}
          </div>
          <span className="text-[var(--color-border-strong)]">|</span>
          <ElapsedClock createdAt={sessionCreatedAt} fallback={sessionElapsed} />
        </div>

        {onOpenExperimentLab && (
          <button
            type="button"
            onClick={onOpenExperimentLab}
            aria-expanded={experimentLabOpen}
            title="Open Experiment & Discovery Lab (⌘E)"
            aria-label="Open Experiment & Discovery Lab"
            className={`h-8 px-3 rounded-full border text-xs font-medium transition-all shadow-[var(--shadow-raised)] flex items-center gap-1.5 focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
              experimentLabOpen
                ? 'bg-[var(--color-lit)] text-[var(--color-on-lit)] border-[var(--color-lit)]'
                : 'bg-[var(--color-surface)] border-[var(--color-border)] hover:border-[var(--color-accent-edge)] hover:bg-[var(--color-fill-hover)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)]'
            }`}
          >
            <Zap className={`w-3.5 h-3.5 ${experimentLabOpen ? '' : 'text-[var(--color-accent-ink)]'}`} />
            <span>Experiment Lab</span>
          </button>
        )}
        {onOpenMoleculeView && (
          <button
            type="button"
            onClick={onOpenMoleculeView}
            title="Open molecule object view"
            aria-label="Open molecule object view"
            className="w-8 h-8 rounded-full bg-[var(--color-surface)] border border-[var(--color-border)] hover:border-[var(--color-accent-edge)] hover:bg-[var(--color-fill-hover)] text-[var(--color-accent-ink)] hover:text-[var(--color-text)] transition-all shadow-[var(--shadow-raised)] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
          >
            <Box className="w-4 h-4" />
          </button>
        )}
        <button
          type="button"
          aria-label="Recenter viewport"
          title="Recenter viewport (0: fit · 1: 100%)"
          onClick={onRecenter}
          className="w-8 h-8 rounded-full bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] flex items-center justify-center transition-all shadow-[var(--shadow-raised)] focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
        >
          <Crosshair className="w-[17px] h-[17px]" />
        </button>
      </div>
    </header>
  );
});
