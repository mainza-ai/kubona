'use client';

import React, { useEffect, useRef } from 'react';
import { Bell, Check, X } from 'lucide-react';
import type { NotificationPreferences, NotificationRow } from '@/lib/api';

const KINDS = ['job', 'run', 'proposal', 'claim', 'workflow', 'memory', 'allowance', 'obligation'];

export function NotificationCenter({
  open,
  notifications,
  unread,
  onClose,
  onMarkRead,
  onResolve,
  onOpenAction,
  prefs,
  onUpdatePrefs,
}: {
  open: boolean;
  notifications: NotificationRow[];
  unread: number;
  onClose: () => void;
  onMarkRead: (keys: string[]) => void;
  onResolve: (key: string) => void;
  onOpenAction?: (action: string) => void;
  prefs: NotificationPreferences;
  onUpdatePrefs: (next: NotificationPreferences) => void;
}) {
  const marked = useRef(new Set<string>());
  useEffect(() => {
    if (!open) return;
    const keys = notifications
      .filter((n) => !n.read_at && !marked.current.has(n.key))
      .map((n) => n.key);
    if (keys.length === 0) return;
    for (const key of keys) marked.current.add(key);
    onMarkRead(keys);
  }, [open, notifications, onMarkRead]);

  if (!open) return null;
  return (
    <div
      role="dialog"
      aria-label="Notifications"
      data-testid="notification-center"
      className="fixed left-14 lg:left-20 bottom-20 z-50 flex max-h-[min(30rem,calc(100vh-6rem))] w-[min(24rem,calc(100vw-3rem))] flex-col overflow-hidden rounded-card border border-border-strong bg-surface-elev shadow-[var(--shadow-float)]"
    >
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="flex items-center gap-1.5 text-label font-semibold text-text">
          <Bell className="h-3.5 w-3.5 text-info" aria-hidden="true" />
          Notifications
          {unread > 0 && <span className="font-code text-muted">({unread} unread)</span>}
        </span>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close notifications"
          className="rounded-inset p-0.5 text-muted hover:bg-fill hover:text-text"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
      <details className="border-b border-border px-3 py-1.5 text-caption text-muted">
        <summary className="cursor-pointer select-none">Delivery preferences</summary>
        <label className="mt-1 flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={prefs.quiet}
            onChange={(event) => onUpdatePrefs({ ...prefs, quiet: event.target.checked })}
          />
          Quiet mode (keep inbox, suppress badge/announcement)
        </label>
        <div className="mt-1 flex flex-wrap gap-x-2 gap-y-0.5">
          {KINDS.map((kind) => (
            <label key={kind} className="inline-flex items-center gap-1">
              <input
                type="checkbox"
                checked={!prefs.muted_kinds.includes(kind)}
                onChange={(event) => {
                  const next = event.target.checked
                    ? prefs.muted_kinds.filter((item) => item !== kind)
                    : [...prefs.muted_kinds, kind];
                  onUpdatePrefs({ ...prefs, muted_kinds: next });
                }}
              />
              {kind}
            </label>
          ))}
        </div>
      </details>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {notifications.length === 0 ? (
          <p className="px-1 py-4 text-body text-muted">No notifications yet. Terminal runs, proposals, reviews, and decisions land here.</p>
        ) : (
          <ul className="space-y-1.5" aria-label="Notification list">
            {notifications.map((item) => (
              <li
                key={item.key}
                className={`rounded-row border px-2.5 py-2 ${
                  item.severity === 'warning' || item.severity === 'danger'
                    ? 'border-warning-edge bg-warning-wash'
                    : 'border-border bg-surface'
                }`}
              >
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5 text-caption text-muted">
                      <span className="rounded-chip border border-border px-1.5 py-0.5 font-code">{item.kind}</span>
                      <span>{item.created_at.slice(0, 19).replace('T', ' ')}</span>
                      {!item.read_at && <span className="text-info-ink">new</span>}
                    </div>
                    <p className="mt-1 text-body text-text">{item.message}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {item.action && onOpenAction && (
                      <button
                        type="button"
                        onClick={() => onOpenAction(item.action)}
                        className="rounded-control-compact border border-border px-1.5 py-0.5 text-caption text-muted hover:text-text"
                      >
                        Open
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => onResolve(item.key)}
                      aria-label={`Resolve notification ${item.key}`}
                      title="Resolve"
                      className="rounded-control-compact p-0.5 text-muted hover:text-success"
                    >
                      <Check className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
