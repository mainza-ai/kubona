'use client';

import React, { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import {
  API_PROBLEM_EVENT,
  UNAUTH_EVENT,
  type ApiProblem,
} from '@/lib/api';
import { useAuthPhase } from '@/domain/auth-state';
import type { Session } from '@/types';

function SessionExpiredView({ onOpenLabs, onDismiss }: { onOpenLabs: () => void; onDismiss: () => void }) {
  return (
    <div
      data-testid="auth-expired-banner"
      role="alert"
      className="flex items-center justify-between gap-3 px-4 py-2 bg-amber-950/90 border-b border-amber-500/40 text-amber-100 text-xs"
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

function ApiProblemView({ problem, onDismiss }: { problem: ApiProblem; onDismiss: () => void }) {
  return (
    <div
      data-testid="api-problem-banner"
      role="alert"
      className="flex items-center justify-between gap-3 px-4 py-1.5 bg-rose-950/90 border-b border-rose-500/40 text-rose-100 text-[12px] font-code"
    >
      <span className="truncate">
        API refusal — {problem.path} · {problem.status ?? 'transport'} · {problem.message}. Some surfaces may show empty data until this is fixed.
      </span>
      <button
        type="button"
        onClick={onDismiss}
        aria-label="Dismiss API refusal"
        className="shrink-0 px-2 py-0.5 rounded border border-rose-400/30 text-rose-200 hover:bg-rose-500/20"
      >
        Dismiss
      </button>
    </div>
  );
}

function useExpiredBannerState() {
  const [eventVisible, setEventVisible] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const authPhase = useAuthPhase();
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const show = () => setEventVisible(true);
    window.addEventListener(UNAUTH_EVENT, show);
    return () => window.removeEventListener(UNAUTH_EVENT, show);
  }, []);
  return {
    visible: !dismissed && (authPhase === 'expired' || eventVisible),
    dismiss: () => { setDismissed(true); setEventVisible(false); },
  };
}

function useProblemState() {
  const [problem, setProblem] = useState<ApiProblem | null>(null);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onProblem = (event: Event) => {
      const detail = (event as CustomEvent<ApiProblem>).detail;
      if (detail) setProblem(detail);
    };
    window.addEventListener(API_PROBLEM_EVENT, onProblem);
    return () => window.removeEventListener(API_PROBLEM_EVENT, onProblem);
  }, []);
  return { problem, dismiss: () => setProblem(null) };
}

export function SessionExpiredBanner({ onOpenLabs }: { onOpenLabs: () => void }) {
  const state = useExpiredBannerState();
  if (!state.visible) return null;
  return <SessionExpiredView onOpenLabs={onOpenLabs} onDismiss={state.dismiss} />;
}

export function ApiProblemBanner() {
  const state = useProblemState();
  if (!state.problem) return null;
  return <ApiProblemView problem={state.problem} onDismiss={state.dismiss} />;
}

/** One global banner slot: expired/forbidden auth outranks API refusal. */
export function BannerStack({ onOpenLabs }: { onOpenLabs: () => void }) {
  const expired = useExpiredBannerState();
  const problem = useProblemState();
  if (expired.visible) return <SessionExpiredView onOpenLabs={onOpenLabs} onDismiss={expired.dismiss} />;
  if (problem.problem) return <ApiProblemView problem={problem.problem} onDismiss={problem.dismiss} />;
  return null;
}

export function describeCreateFailure(result: Session | null, error: unknown): string | null {
  if (result) return null;
  if (error !== undefined && error !== null) {
    const detail = error instanceof Error ? error.message : String(error);
    return `Create campaign failed: ${detail}`;
  }
  return 'Create campaign failed: backend refused (createSession returned null) — nothing was created.';
}
