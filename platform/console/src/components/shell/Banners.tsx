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

export function SessionExpiredBanner({ onOpenLabs }: { onOpenLabs: () => void }) {
  const [eventVisible, setEventVisible] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const authPhase = useAuthPhase();
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const show = () => setEventVisible(true);
    window.addEventListener(UNAUTH_EVENT, show);
    return () => window.removeEventListener(UNAUTH_EVENT, show);
  }, []);
  const visible = !dismissed && (authPhase === 'expired' || eventVisible);
  if (!visible) return null;
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
          className="px-2.5 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 hover:bg-cyan-500/25 transition-all"
        >
          Log in (Mesh → Labs)
        </button>
        <button
          data-testid="auth-banner-dismiss"
          onClick={() => { setDismissed(true); setEventVisible(false); }}
          aria-label="Dismiss session expired banner"
          className="p-1 rounded-lg text-amber-200/70 hover:text-amber-100 hover:bg-white/10 transition-all"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </span>
    </div>
  );
}

export function ApiProblemBanner() {
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
  if (!problem) return null;
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
        onClick={() => setProblem(null)}
        aria-label="Dismiss API refusal"
        className="shrink-0 px-2 py-0.5 rounded border border-rose-400/30 text-rose-200 hover:bg-rose-500/20"
      >
        Dismiss
      </button>
    </div>
  );
}

export function describeCreateFailure(result: Session | null, error: unknown): string | null {
  if (result) return null;
  if (error !== undefined && error !== null) {
    const detail = error instanceof Error ? error.message : String(error);
    return `Create campaign failed: ${detail}`;
  }
  return 'Create campaign failed: backend refused (createSession returned null) — nothing was created.';
}
