import React, { useEffect, useState } from 'react';
import { fetchSessionContext, createSession, clearMessages, type SessionContext } from '@/lib/api';

// S8b: overflow card — appears only at real context pressure (>=80% of the
// BudgetHistory caps, same source as ContextReadout). Every action is a
// real endpoint: new session (createSession), clear history
// (clearMessages, confirmed). There is deliberately NO "compact" button:
// no compact endpoint exists, and a button that pretends otherwise would
// be theater (production bar).
export const OVERFLOW_PRESSURE_PCT = 80;

export function OverflowCard({
  sessionId,
  refreshKey,
  onSessionChanged,
}: {
  sessionId: string;
  refreshKey: number;
  onSessionChanged?: (newSessionId: string) => void;
}) {
  const [ctx, setCtx] = useState<SessionContext | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    if (!sessionId) {
      setCtx(null);
      return;
    }
    fetchSessionContext(sessionId).then((c) => {
      if (live) setCtx(c);
    });
    return () => {
      live = false;
    };
  }, [sessionId, refreshKey]);

  if (!ctx || ctx.pressure_pct < OVERFLOW_PRESSURE_PCT) return null;

  const fresh = async () => {
    setBusy(true);
    try {
      const s = await createSession('Quantum Research Campaign (overflow)', 150.0);
      if (s && onSessionChanged) {
        onSessionChanged(s.id);
        setDone('Fresh session started — history preserved in the old one.');
      } else {
        setDone('Could not create a session.');
      }
    } finally {
      setBusy(false);
    }
  };

  const clear = async () => {
    if (!window.confirm('Clear this session\u2019s chat history? Jobs, ledger and claims are kept.')) return;
    setBusy(true);
    try {
      const n = await clearMessages(sessionId);
      setDone(n == null ? 'Clear failed.' : `Cleared ${n} messages. Jobs, ledger and claims kept.`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="rounded-2xl border border-amber-400/40 bg-amber-500/5 p-2.5 text-xs"
      role="alert"
      aria-label={`Context pressure ${Math.round(ctx.pressure_pct)} percent`}
    >
      <div className="text-amber-200 font-semibold">
        Context at {Math.round(ctx.pressure_pct)}%{ctx.would_drop > 0 ? ` — ${ctx.would_drop} oldest turns would drop next` : ''}
      </div>
      <div className="text-slate-400 mt-0.5">
        {ctx.turns}/{ctx.turn_cap} turns · {ctx.history_chars}/{ctx.history_chars_cap} chars. Oldest-first cuts are announced by the model, but a fresh session or cleared history reads cleaner.
      </div>
      <div className="flex gap-2 mt-2">
        <button
          type="button"
          onClick={fresh}
          disabled={busy}
          className="px-2.5 py-1 rounded-xl bg-cyan-400/90 hover:bg-cyan-300 text-black font-semibold disabled:opacity-40"
        >
          New session
        </button>
        <button
          type="button"
          onClick={clear}
          disabled={busy}
          className="px-2.5 py-1 rounded-xl bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 disabled:opacity-40"
        >
          Clear history
        </button>
      </div>
      {done && <div className="text-slate-300 mt-1.5">{done}</div>}
    </div>
  );
}
