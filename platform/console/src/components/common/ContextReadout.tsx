import React, { useEffect, useState } from 'react';
import { fetchSessionContext, type SessionContext } from '@/lib/api';

// S8a: composer context readout — measured transcript pressure against the
// server's real BudgetHistory caps (turns/40, chars/48K), plus summed
// provider-reported tokens. No vendor window anywhere: pressure_pct is how
// close the next turn is to the cut the system actually applies.
// Fails silent (renders nothing) when the backend is unreachable — the
// readout is advisory; data correctness is enforced server-side.
export function ContextReadout({ sessionId, refreshKey }: { sessionId: string; refreshKey: number }) {
  const [ctx, setCtx] = useState<SessionContext | null>(null);

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

  if (!ctx) return null;
  const pct = Math.max(0, Math.min(999, Math.round(ctx.pressure_pct)));
  const hot = pct >= 80;
  const totalTok = ctx.prompt_tokens + ctx.completion_tokens;
  const title = `${ctx.turns}/${ctx.turn_cap} turns · ${ctx.history_chars}/${ctx.history_chars_cap} chars` +
    (ctx.would_drop > 0 ? ` · ${ctx.would_drop} oldest would drop` : '') +
    ` · ${totalTok} tokens measured`;
  return (
    <span
      className={`text-[10px] font-code ml-1 ${hot ? 'text-amber-300' : 'text-slate-500'}`}
      title={title}
      aria-label={`Context pressure ${pct} percent`}
    >
      ctx {pct}%
      {ctx.would_drop > 0 && <span className="text-amber-300"> ↓{ctx.would_drop}</span>}
    </span>
  );
}
