'use client';

// Global Manual/Auto/Locked indicator for the agentic workflow (AG-18).
//
// The Autoresearch panel owns the detailed control; this chip keeps the mode
// visible at the composer and lets the user flip it without opening the panel.
// A locked policy (operator halt) is visible but not toggleable here.

import React, { useCallback, useEffect, useState } from 'react';
import { fetchSessionAgentic, setSessionAgentic } from '@/domain/oversight';
import { Button } from '@/components/ui/Button';

type AgenticState = { enabled: boolean; locked: boolean; stages: Record<string, string> };

export function AgenticModeChip({ sessionId }: { sessionId?: string | null }) {
  const [state, setState] = useState<AgenticState | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const apply = useCallback((out: Awaited<ReturnType<typeof fetchSessionAgentic>>) => {
    setState(out ? { enabled: out.agentic_enabled, locked: out.agentic_locked, stages: out.stages } : null);
  }, []);

  const load = useCallback(async () => {
    if (!sessionId) return;
    apply(await fetchSessionAgentic(sessionId));
  }, [sessionId, apply]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    // Initial fetch in a promise callback (not synchronously inside the effect).
    fetchSessionAgentic(sessionId).then((out) => {
      if (live) apply(out);
    });
    const onChange = () => {
      void load();
    };
    window.addEventListener('kubona:agentic-changed', onChange);
    return () => {
      live = false;
      window.removeEventListener('kubona:agentic-changed', onChange);
    };
  }, [sessionId, apply, load]);

  const toggle = async () => {
    if (!sessionId || !state || state.locked) return;
    setNotice(null);
    const out = await setSessionAgentic(sessionId, !state.enabled);
    if (out.ok) {
      setState({ enabled: out.state.agentic_enabled, locked: out.state.agentic_locked, stages: out.state.stages });
      window.dispatchEvent(new Event('kubona:agentic-changed'));
      return;
    }
    setNotice(out.error);
    if (out.code === 'agentic_locked') {
      setState((previous) => (previous ? { ...previous, locked: true } : previous));
    }
  };

  if (!sessionId) return null;
  const label = state === null ? 'Agentic —' : state.locked ? 'Locked' : state.enabled ? 'Auto' : 'Manual';
  const tone = state === null
    ? 'border-border bg-fill text-faint'
    : state.locked
      ? 'border-warning-edge bg-warning-wash text-warning-ink'
      : state.enabled
        ? 'border-success-edge bg-success-wash text-success-ink'
        : 'border-border bg-fill text-muted';
  const dot = state?.locked ? 'bg-warning-ink' : state?.enabled ? 'bg-success-ink' : 'bg-muted';
  const title = state === null
    ? 'Agentic mode unavailable'
    : state.locked
      ? 'Locked by an operator halt — only an operator can unlock'
      : state.enabled
        ? 'Auto agent is on — click to switch to manual'
        : 'Manual mode — click to enable agent loops';

  return (
    <span className="inline-flex min-w-0 items-center gap-1">
      <Button
        role="switch"
        aria-checked={state?.enabled === true}
        aria-label="Auto agent mode"
        disabled={state === null || state.locked}
        title={title}
        onClick={() => void toggle()}
        variant="ghost"
        size="compact"
        className={`mb-0.5 h-8 shrink-0 gap-1.5 rounded-row border px-2 text-[11px] font-semibold ${tone}`}
      >
        <span aria-hidden className={`h-1.5 w-1.5 shrink-0 rounded-full ${dot}`} />
        <span>{label}</span>
      </Button>
      {notice && (
        <span role="status" className="max-w-40 truncate text-[11px] text-warning" title={notice}>
          {notice}
        </span>
      )}
    </span>
  );
}
