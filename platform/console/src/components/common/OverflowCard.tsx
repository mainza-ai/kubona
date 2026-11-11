import React, { useEffect, useState } from 'react';
import { fetchSessionContext, createSession, clearMessages, type SessionContext } from '@/lib/api';
import ConfirmDialog from '@/components/common/ConfirmDialog';

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
  // Wave-3: clear-history stages a typed ConfirmDialog (CLEAR) instead of
  // window.confirm(). Jobs, ledger and claims are kept either way.
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);

  // Same key-change reset as ContextReadout (shared data source): never
  // show the old session's pressure card while the new one loads.
  const [prevSessionId, setPrevSessionId] = useState(sessionId);
  if (prevSessionId !== sessionId) {
    setPrevSessionId(sessionId);
    setCtx(null);
  }

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
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
    setBusy(true);
    try {
      const n = await clearMessages(sessionId);
      setDone(n == null ? 'Clear failed.' : `Cleared ${n} messages. Jobs, ledger and claims kept.`);
    } finally {
      setBusy(false);
      setClearConfirmOpen(false);
    }
  };

  return (
    <div
      className="rounded-2xl border border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] p-2.5 text-xs"
      role="alert"
      aria-label={`Context pressure ${Math.round(ctx.pressure_pct)} percent`}
    >
      <div className="text-[var(--color-warning-ink)] font-semibold">
        Context at {Math.round(ctx.pressure_pct)}%{ctx.would_drop > 0 ? ` — ${ctx.would_drop} oldest turns would drop next` : ''}
      </div>
      <div className="text-[var(--color-muted)] mt-0.5">
        {ctx.turns}/{ctx.turn_cap} turns · {ctx.history_chars}/{ctx.history_chars_cap} chars. Oldest-first cuts are announced by the model, but a fresh session or cleared history reads cleaner.
      </div>
      <div className="flex gap-2 mt-2">
        <button
          type="button"
          onClick={fresh}
          disabled={busy}
          className="px-2.5 py-1 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] font-semibold disabled:opacity-40"
        >
          New session
        </button>
        <button
          type="button"
          onClick={() => setClearConfirmOpen(true)}
          disabled={busy}
          className="px-2.5 py-1 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-text)] disabled:opacity-40"
        >
          Clear history
        </button>
      </div>
      {done && <div className="text-[var(--color-text)] mt-1.5">{done}</div>}
      {clearConfirmOpen && (
        <ConfirmDialog
          open
          title="Clear chat history?"
          body={
            <span>
              This session’s chat history is deleted. Jobs, ledger and claims are kept. Type{' '}
              <code className="rounded bg-[var(--color-fill)] px-1 font-mono">CLEAR</code> to confirm.
            </span>
          }
          requireText="CLEAR"
          confirmLabel="Clear history"
          danger
          busy={busy}
          onConfirm={() => void clear()}
          onCancel={() => {
            if (busy) return;
            setClearConfirmOpen(false);
          }}
        />
      )}
    </div>
  );
}
