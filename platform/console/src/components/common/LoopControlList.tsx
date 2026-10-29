'use client';

import React, { useState } from 'react';
import { loopAction, type LoopAction } from '@/lib/api';
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
      <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Live loops">
        <div className="text-xs font-semibold text-white">Live loops · override</div>
        <div className="text-[11px] text-slate-500">no live loops</div>
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Live loops">
      <div className="text-xs font-semibold text-white">Live loops · override</div>
      <div className="space-y-1.5">
        {loops.map((loop) => {
          const st = statusOf(loop);
          const isBusy = busy.has(loop.id);
          const canPause = LIVE.has(st);
          const canResume = PAUSED.has(st);
          const canKill = LIVE.has(st) || PAUSED.has(st);
          return (
            <div
              key={loop.id}
              className="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-xl bg-white/[0.03] text-[11px]"
            >
              <span className="font-code text-slate-300 truncate" title={loop.id}>
                {loop.id.slice(0, 8)} · {st || 'unknown'} · step {loop.step_count}/{loop.max_steps}
              </span>
              <span className="flex items-center gap-1.5 shrink-0">
                {canPause && (
                  <button
                    type="button"
                    disabled={isBusy}
                    onClick={() => run(loop, 'pause')}
                    aria-label={`Pause loop ${loop.id}`}
                    className="px-2 py-1 rounded-lg bg-amber-500/15 border border-amber-400/30 text-amber-200 hover:bg-amber-500/25 disabled:opacity-40"
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
                    className="px-2 py-1 rounded-lg bg-emerald-500/15 border border-emerald-400/30 text-emerald-200 hover:bg-emerald-500/25 disabled:opacity-40"
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
                    className="px-2 py-1 rounded-lg bg-rose-500/15 border border-rose-400/30 text-rose-200 hover:bg-rose-500/25 disabled:opacity-40"
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
        <div className="text-[11px] text-amber-200" role="alert">
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
              <code className="rounded bg-white/10 px-1 font-mono">{killDraft.id.slice(0, 8)}</code> to confirm.
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
