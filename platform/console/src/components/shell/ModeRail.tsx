'use client';

import React from 'react';
import Image from 'next/image';
import { BadgeCheck, Inbox, LayoutDashboard, Lock, Network, Settings , Bell} from 'lucide-react';

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
  needsYouCount = 0,
  needsYouOpen = false,
  needsYouStatus = 'ready',
  onOpenNeedsYou,
  notificationsCount = 0,
  notificationsOpen = false,
  onOpenNotifications,
}: {
  active: ShellMode;
  onSelect: (mode: ShellMode) => void;
  airGapSovereign: boolean;
  onOpenCampaigns: () => void;
  /** Unread obligations waiting on the reviewer (report §3.1). */
  needsYouCount?: number;
  needsYouOpen?: boolean;
  /** Unknown is never labelled "nothing waiting". */
  needsYouStatus?: 'loading' | 'ready' | 'unavailable';
  onOpenNeedsYou?: () => void;
  notificationsCount?: number;
  notificationsOpen?: boolean;
  onOpenNotifications?: () => void;
}) {
  return (
    <aside aria-label="Primary modes" className="relative z-30 w-14 lg:w-16 h-full flex flex-col items-center justify-between py-5 border-r border-border bg-chrome shrink-0">
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

        <div className="w-6 h-[1px] bg-border-strong" />

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
                className={`group relative w-11 h-11 rounded-row flex items-center justify-center transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] hover:scale-105 focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none ${
                  isActive
                    ? 'bg-lit text-on-lit border border-border-strong'
                    : 'text-muted hover:text-text hover:bg-fill border border-transparent hover:border-border'
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
        {onOpenNeedsYou && (
          // The reviewer's inbox lives in the shell, not three levels deep:
          // the badge counts what has not been looked at yet, and the tray
          // stays until each item is decided (never merely dismissed).
          <button
            type="button"
            data-testid="dock-needs-you"
            aria-label={
              needsYouCount > 0
                ? `Needs you: ${needsYouCount} waiting`
                : needsYouStatus === 'unavailable'
                  ? 'Needs you: queue unavailable'
                  : needsYouStatus === 'loading'
                    ? 'Needs you: reading queue'
                    : 'Needs you: nothing waiting'
            }
            aria-expanded={needsYouOpen}
            aria-controls="needs-you-tray"
            title={
              needsYouCount > 0
                ? `${needsYouCount} waiting on you`
                : needsYouStatus === 'unavailable'
                  ? 'Queue unavailable — sign in or set the operator token'
                  : needsYouStatus === 'loading'
                    ? 'Reading the decision queue…'
                    : 'Nothing waiting on you'
            }
            onClick={onOpenNeedsYou}
            className={`relative w-9 h-9 rounded-control-compact flex items-center justify-center border hover:bg-fill focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none ${
              needsYouOpen
                ? 'border-warning-edge bg-warning-wash text-warning-ink'
                : 'border-border text-muted hover:text-text'
            }`}
          >
            <Inbox className="w-[18px] h-[18px]" />
            {needsYouCount > 0 && (
              <span
                data-testid="needs-you-badge"
                className="absolute -top-1 -right-1 min-w-4 h-4 px-1 rounded-capsule bg-danger text-on-lit text-[11px] font-code font-bold leading-4 text-center"
              >
                {needsYouCount > 9 ? '9+' : needsYouCount}
              </span>
            )}
          </button>
        )}

        {onOpenNotifications && (
          <button
            type="button"
            data-testid="dock-notifications"
            aria-label={notificationsCount > 0 ? `Notifications: ${notificationsCount} unread` : 'Notifications: none unread'}
            aria-expanded={notificationsOpen}
            aria-controls="notification-center"
            title={notificationsCount > 0 ? `${notificationsCount} unread notification(s)` : 'No unread notifications'}
            onClick={onOpenNotifications}
            className={`relative w-9 h-9 rounded-control-compact flex items-center justify-center border hover:bg-fill focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none ${
              notificationsOpen ? 'border-info-edge bg-info-wash text-info-ink' : 'border-border text-muted hover:text-text'
            }`}
          >
            <Bell className="w-[18px] h-[18px]" />
            {notificationsCount > 0 && (
              <span className="absolute -top-1 -right-1 min-w-4 h-4 px-1 rounded-capsule bg-info text-on-lit text-[11px] font-code font-bold leading-4 text-center">
                {notificationsCount > 9 ? '9+' : notificationsCount}
              </span>
            )}
          </button>
        )}

        <div
          role="status"
          tabIndex={0}
          aria-label={airGapSovereign ? 'Air-gap: sovereign, zero outbound egress' : 'Air-gap: egress open, network path live'}
          title={airGapSovereign ? 'Air-gap enclave: sovereign, 0 outbound egress' : 'Air-gap enclave: egress open'}
          className="w-9 h-9 rounded-control-compact flex items-center justify-center border cursor-default focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
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
          className="w-8 h-8 rounded-capsule bg-border-strong p-[1px] cursor-pointer hover:scale-105 transition-transform focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
        >
          <span className="w-full h-full rounded-capsule bg-raised flex items-center justify-center text-[12px] font-code font-bold text-text">KB</span>
        </button>
      </div>
    </aside>
  );
}
