'use client';

import React, { useEffect, useRef } from 'react';
import { Inbox, X } from 'lucide-react';
import { needsEscalation, waitingLabel, type NeedItem } from '@/lib/needs-you';
import { useDismissiblePopover } from '@/lib/useDismissiblePopover';

/**
 * Shell-level "Needs you" tray (report §3.1). Anything that asks the human
 * for a decision lives here until it is decided: a passing notice may leave
 * on its own, a question may not.
 *
 * The tray is a projection over server-decided obligations — it never closes
 * anything itself. "Review" focuses the subject where it lives; the decision
 * still belongs to the surface that owns it (the Action Gate, the obligation
 * queue, the approval card).
 */
export type NeedsYouQueueStatus = 'loading' | 'ready' | 'unavailable';

export function NeedsYouTray({
  items,
  open,
  onClose,
  onFocus,
  onOpenQueue,
  nowMs,
  queueStatus = 'ready',
}: {
  items: NeedItem[];
  open: boolean;
  onClose: () => void;
  onFocus: (item: NeedItem) => void;
  onOpenQueue: () => void;
  nowMs?: number;
  /** Unknown is never rendered as "nothing waiting". */
  queueStatus?: NeedsYouQueueStatus;
}) {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const decidable = items.filter((item) => item.state === 'decidable').length;

  useDismissiblePopover(open, panelRef, onClose, {
    ignoreSelector: '[data-popover-trigger="needs-you"]',
  });

  useEffect(() => {
    if (!open) return;
    panelRef.current?.focus();
  }, [open]);

  if (!open) return null;

  return (
    <div
      ref={panelRef}
      tabIndex={-1}
      role="dialog"
      aria-label="Needs you"
      data-testid="needs-you-tray"
      id="needs-you-tray"
      className="fixed left-14 lg:left-20 bottom-20 z-50 w-[min(22rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] max-h-[min(28rem,calc(100vh-6rem))] overflow-hidden flex flex-col rounded-card border border-border-strong bg-surface-elev shadow-[var(--shadow-float)] outline-none"
    >
      <div className="flex items-start justify-between gap-2 px-3 pt-3 pb-2">
        <div className="min-w-0">
          <span className="flex items-center gap-1.5 text-[12px] font-semibold text-text">
            <Inbox className="w-3.5 h-3.5 text-warning-ink" />
            Needs you
            <span className="font-code text-muted">({items.length})</span>
          </span>
          <p className="mt-0.5 text-[11px] text-muted">
            {queueStatus === 'loading'
              ? 'Reading the decision queue…'
              : queueStatus === 'unavailable'
                ? 'Queue unavailable — sign in or set the operator token.'
                : items.length === 0
                  ? 'Nothing is waiting on a decision.'
                  : decidable > 0
                    ? `${decidable} decidable now · stays here until decided.`
                    : 'All rows are blocked — nothing is decidable yet.'}
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close needs-you tray"
          className="shrink-0 w-6 h-6 rounded-control-compact flex items-center justify-center text-muted hover:text-text hover:bg-fill focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-2">
        {items.length === 0 ? (
          <p className="px-1 py-3 text-[12px] text-faint">
            {queueStatus === 'loading'
              ? 'The queue has not answered yet.'
              : queueStatus === 'unavailable'
                ? 'Sign in (or present the operator token) to read this lab’s queue.'
                : 'Approvals and open obligations surface here the moment they exist.'}
          </p>
        ) : (
          <ul className="space-y-1.5" aria-label="Open obligations">
            {items.map((item) => {
              const subject = item.subjectKind || 'obligation';
              const shortId = item.subjectId ? item.subjectId.slice(0, 12) : item.id.slice(0, 12);
              return (
                <li
                  key={item.id}
                  data-testid="needs-you-row"
                  className="rounded-row border border-border bg-card px-2.5 py-2 space-y-1"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span
                      className="min-w-0 truncate font-code text-[11px] uppercase tracking-wide text-muted"
                      title={`${subject} · ${item.subjectId || item.id}`}
                    >
                      {subject} · {shortId}
                    </span>
                    <span
                      className={`shrink-0 rounded-capsule border px-1.5 py-0.5 font-code text-[11px] ${
                        item.state === 'decidable'
                          ? 'border-info-edge bg-info-wash text-info-ink'
                          : 'border-border bg-fill text-muted'
                      }`}
                    >
                      {item.state === 'decidable' ? 'decidable now' : 'blocked'}
                    </span>
                  </div>
                  <p className="text-[12px] text-label-secondary">
                    {item.reason ?? 'No blocking reason recorded by the server.'}
                    {item.reasons.length > 1 ? (
                      <span className="text-muted"> (+{item.reasons.length - 1} more)</span>
                    ) : null}
                  </p>
                  <div className="flex items-center justify-between gap-2">
                    <span className={`font-code text-[11px] ${needsEscalation(item, nowMs) ? 'text-warning-ink' : 'text-faint'}`}>
                      {waitingLabel(item, nowMs)}
                      {needsEscalation(item, nowMs) ? ' · escalated' : ''}
                    </span>
                    <button
                      type="button"
                      onClick={() => onFocus(item)}
                      aria-label={`Review ${subject} ${shortId}`}
                      className="shrink-0 rounded-control-compact border border-border bg-fill px-2 py-1 text-[11px] text-text hover:bg-fill-hover focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
                    >
                      Review
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="border-t border-border px-3 py-2">
        <button
          type="button"
          onClick={onOpenQueue}
          aria-label="Open Guardrails queue"
          className="w-full rounded-control-compact border border-border bg-surface px-2 py-1.5 text-[11px] text-label-secondary hover:bg-fill-hover hover:text-text focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
        >
          Open Guardrails queue
        </button>
      </div>
    </div>
  );
}
