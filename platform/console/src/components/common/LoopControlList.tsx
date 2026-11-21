'use client';

import React, { useState } from 'react';
import { loopAction, settleSessionLoops, type LoopAction } from '@/lib/api';
import { Button } from '@/components/ui/Button';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import type { Loop } from '@/types';

// U1-c: loop override roster (watch-and-interrupt). Pause shows only on
// running loops, resume only on paused ones, kill only on live loops
// (running/paused) — completed/killed loops get no buttons, never a
// no-op call. Kill confirms (destructive, irreversible server-side).
// After every accepted action the parent refetches (server is the source,
// never optimistic state). Refusals render the backend reason verbatim.
const LIVE = new Set(['running']);
const PAUSED = new Set(['paused']);

function statusOf(loop: Loop): string {
  return String(loop.status || '').toLowerCase();
}

export function LoopControlList({
  loops,
  onChanged,
}: {
  loops: Loop[];
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [notice, setNotice] = useState<string | null>(null);
  // Wave-3: kill stages a typed ConfirmDialog (loop short-id) instead of a
  // native confirm(). Pause/resume are reversible single clicks, unchanged.
  const [killDraft, setKillDraft] = useState<Loop | null>(null);

  const run = async (loop: Loop, action: LoopAction) => {
    setBusy((prev) => new Set(prev).add(loop.id));
    setNotice(null);
    try {
      const res = await loopAction(loop.id, action);
      if (res === null) {
        setNotice(`loop ${action} failed: backend unreachable — nothing changed.`);
      } else if ('error' in res) {
        setNotice(`loop ${action} refused: ${res.error}`);
      } else {
        onChanged();
      }
    } finally {
      setBusy((prev) => {
        const next = new Set(prev);
        next.delete(loop.id);
        return next;
      });
    }
  };

  // Honest empty: an empty roster states "no live loops" instead of
  // rendering nothing (a missing panel reads as a load failure).
  if (loops.length === 0) {
    return (
      <div className="rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Live loops">
        <div className="text-label font-semibold text-text">Live loops · override</div>
        <div className="text-[12px] text-faint">no live loops</div>
      </div>
    );
  }

  return (
    <div className="rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Live loops">
      <div className="text-label font-semibold text-text">Live loops · override</div>
      <div className="space-y-1.5">
        {loops.map((loop) => {
          const st = statusOf(loop);
          const isBusy = busy.has(loop.id);
          const canPause = LIVE.has(st);
          const canResume = PAUSED.has(st);
          const canKill = LIVE.has(st) || PAUSED.has(st);
          const canSettle = st === 'sleeping';
          return (
            <div
              key={loop.id}
              className="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-row bg-card border border-border text-[12px]"
            >
              <span className="font-code text-label-secondary truncate" title={loop.id}>
                {loop.id.slice(0, 8)} · {st || 'unknown'} · step {loop.step_count}/{loop.max_steps}
              </span>
              <span className="flex items-center gap-1.5 shrink-0">
                {canSettle && (
                  <Button
                    size="compact"
                    variant="secondary"
                    disabled={isBusy}
                    onClick={() => void (async () => {
                      setBusy((prev) => new Set(prev).add(loop.id));
                      setNotice(null);
                      try {
                        const res = await settleSessionLoops(loop.session_id);
                        if (!res.ok) {
                          setNotice(`loop settle refused: ${res.error}`);
                        } else {
                          onChanged();
                        }
                      } finally {
                        setBusy((prev) => {
                          const next = new Set(prev);
                          next.delete(loop.id);
                          return next;
                        });
                      }
                    })()}
                  >
                    Settle orphan
                  </Button>
                )}
                {canPause && (
                  <button
                    type="button"
                    disabled={isBusy}
                    onClick={() => run(loop, 'pause')}
                    aria-label={`Pause loop ${loop.id}`}
                    className="px-2 py-1 rounded-control-compact bg-warning-wash border border-warning-edge text-warning-ink hover:opacity-90 disabled:opacity-40"
                  >
                    Pause
                  </button>
                )}
                {canResume && (
                  <button
                    type="button"
                    disabled={isBusy}
                    onClick={() => run(loop, 'resume')}
                    aria-label={`Resume loop ${loop.id}`}
                    className="px-2 py-1 rounded-control-compact bg-success-wash border border-success-edge text-success-ink hover:opacity-90 disabled:opacity-40"
                  >
                    Resume
                  </button>
                )}
                {canKill && (
                  <button
                    type="button"
                    disabled={isBusy}
                    onClick={() => setKillDraft(loop)}
                    aria-label={`Kill loop ${loop.id}`}
                    className="px-2 py-1 rounded-control-compact bg-danger-wash border border-danger-edge text-danger-ink hover:opacity-90 disabled:opacity-40"
                  >
                    Kill
                  </button>
                )}
              </span>
            </div>
          );
        })}
      </div>
      {notice && (
        <div className="text-[12px] text-warning-ink" role="alert">
          {notice}
        </div>
      )}
      {killDraft != null && (
        <ConfirmDialog
          open
          title={`Kill loop ${killDraft.id.slice(0, 8)}?`}
          body={
            <span>
              Killed loops never resume — in-flight work is abandoned. Type{' '}
              <code className="rounded-chip bg-fill px-1 font-mono">{killDraft.id.slice(0, 8)}</code> to confirm.
            </span>
          }
          requireText={killDraft.id.slice(0, 8)}
          confirmLabel="Kill loop"
          danger
          busy={busy.has(killDraft.id)}
          onConfirm={() => {
            const target = killDraft;
            setKillDraft(null);
            void run(target, 'kill');
          }}
          onCancel={() => {
            if (killDraft != null && busy.has(killDraft.id)) return;
            setKillDraft(null);
          }}
        />
      )}
    </div>
  );
}
