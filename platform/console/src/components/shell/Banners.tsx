'use client';

import React, { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import {
  UNAUTH_EVENT,
  type ApiProblem,
} from '@/lib/api';
import { useAuthPhase } from '@/domain/auth-state';
import { isContractDrift, problemChipLabel } from '@/lib/api-problem';
import { needSentence, remedyFor } from '@/lib/need';
import type { Session } from '@/types';

function SessionExpiredView({ onOpenLabs, onDismiss }: { onOpenLabs: () => void; onDismiss: () => void }) {
  return (
    <div
      data-testid="auth-expired-banner"
      role="alert"
      className="flex items-center justify-between gap-3 px-4 py-2 bg-warning-wash border-b border-warning-edge text-warning-ink text-label"
    >
      <span>Session expired — log in again.</span>
      <span className="flex items-center gap-2 shrink-0">
        <button
          data-testid="auth-banner-login"
          onClick={onOpenLabs}
          className="k-button k-button-compact"
        >
          Log in (Mesh → Labs)
        </button>
        <button
          data-testid="auth-banner-dismiss"
          onClick={onDismiss}
          aria-label="Dismiss session expired banner"
          className="k-button k-button-ghost k-button-compact"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </span>
    </div>
  );
}

/**
 * 3.9 — non-blocking API/contract signal. A refused request or drifted
 * contract is observable in the StatusStrip, not as a page-wide alert:
 * role="status" (polite), dismissible by problem signature.
 */
export function ApiProblemChip({ problem, onDismiss }: { problem: ApiProblem; onDismiss: () => void }) {
  const drift = isContractDrift(problem);
  const remedy = problem.need ? remedyFor(problem.need.kind) : null;
  const needText = problem.need
    ? `${needSentence(problem.need)}${remedy ? ` · ${remedy.label}` : ''}`
    : '';
  return (
    <span
      data-testid={drift ? 'contract-drift-chip' : 'api-problem-chip'}
      role="status"
      title={`${problem.message}${needText ? ` · ${needText}` : ''}${problem.at ? ` · ${new Date(problem.at).toLocaleTimeString()}` : ''}`}
      className={`inline-flex max-w-64 shrink-0 items-center gap-1 truncate rounded-chip border px-1.5 py-0.5 font-code text-[11px] ${
        drift
          ? 'border-warning-edge bg-warning-wash text-warning-ink'
          : 'border-danger-edge bg-danger-wash text-danger-ink'
      }`}
    >
      <span className="truncate">
        {problemChipLabel(problem)}
        {problem.need ? ` · ${problem.need.kind}` : ''}
      </span>
      <button
        type="button"
        onClick={onDismiss}
        aria-label={drift ? 'Dismiss contract drift' : 'Dismiss API refusal'}
        className="shrink-0 rounded-chip px-1 leading-none hover:bg-fill-hover focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent"
      >
        ×
      </button>
    </span>
  );
}

function useExpiredBannerState() {
  const [eventVisible, setEventVisible] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const authPhase = useAuthPhase();
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const show = () => {
      setDismissed(false);
      setEventVisible(true);
    };
    window.addEventListener(UNAUTH_EVENT, show);
    return () => window.removeEventListener(UNAUTH_EVENT, show);
  }, []);
  const effectiveVisible = authPhase === 'authenticated'
    ? false
    : !dismissed && (authPhase === 'expired' || eventVisible);
  return {
    visible: effectiveVisible,
    dismiss: () => { setDismissed(true); setEventVisible(false); },
  };
}

export function SessionExpiredBanner({ onOpenLabs }: { onOpenLabs: () => void }) {
  const state = useExpiredBannerState();
  if (!state.visible) return null;
  return <SessionExpiredView onOpenLabs={onOpenLabs} onDismiss={state.dismiss} />;
}

/**
 * One global banner slot, reserved for blocking states. Non-blocking API and
 * contract signals live in StatusStrip as chips (see ApiProblemChip), so a
 * page-wide alert always means "act now".
 */
export function BannerStack({ onOpenLabs }: { onOpenLabs: () => void }) {
  const expired = useExpiredBannerState();
  if (expired.visible) return <SessionExpiredView onOpenLabs={onOpenLabs} onDismiss={expired.dismiss} />;
  return null;
}

export function describeCreateFailure(result: Session | null, error: unknown): string | null {
  if (result) return null;
  if (error != null) {
    const detail = error instanceof Error ? error.message : String(error);
    return `Create campaign failed: ${detail}`;
  }
  return 'Create campaign failed: createSession returned null — nothing was created. Retry from Campaigns once the backend is reachable.';
}
