'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { FlaskConical, ChevronDown, ChevronUp, AlertTriangle } from 'lucide-react';
import { Lab, Session } from '@/types';
import { fetchLabs, fetchSessions } from '@/lib/api';
import { fmtMoney } from '@/lib/guards';
import { LabOpsPanel } from '@/components/common/LabOpsPanel';
import { AuthPanel } from '@/components/common/AuthPanel';

function pct(spent: number, ceiling: number): number | null {
  if (!Number.isFinite(spent) || !Number.isFinite(ceiling) || !(ceiling > 0)) return null;
  return Math.min(100, Math.round((spent / ceiling) * 100));
}

export function LabsView() {
  const [labs, setLabs] = useState<Lab[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Refetch labs (U1-b: server owns tenant state — every accepted
  // ceiling/status/refund/admin-key mutation re-reads it).
  const reload = async () => {
    const l = await fetchLabs();
    setLabs(l);
  };

  useEffect(() => {
    let live = true;
    Promise.all([fetchLabs(), fetchSessions()])
      .then(([l, s]) => {
        if (!live) return;
        setLabs(l);
        setSessions(s);
        setLoaded(true);
      })
      .catch((e) => {
        if (!live) return;
        setError(e instanceof Error ? e.message : String(e));
        setLoaded(true);
      });
    return () => {
      live = false;
    };
  }, []);

  const byLab = useMemo(() => {
    const m: Record<string, Session[]> = {};
    for (const s of sessions) {
      const key = s.tenant_id || 'default';
      (m[key] ??= []).push(s);
    }
    return m;
  }, [sessions]);

  if (!loaded) {
    return <div className="text-slate-400 text-sm">Loading laboratories…</div>;
  }
  if (error) {
    return (
      <div className="p-3 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-xs">
        Laboratories unavailable — nothing loaded ({error.slice(0, 200)}).
      </div>
    );
  }
  if (labs.length === 0) {
    return <div className="text-slate-400 text-sm">No laboratories registered.</div>;
  }

  return (
    <div className="space-y-4">
      <AuthPanel labs={labs} />
      {labs.map((lab) => {
        const spent = typeof lab.spent_usd === 'number' && Number.isFinite(lab.spent_usd) ? lab.spent_usd : null;
        const ceiling = typeof lab.spend_ceiling_usd === 'number' && Number.isFinite(lab.spend_ceiling_usd) ? lab.spend_ceiling_usd : null;
        const unbounded = ceiling == null || !(ceiling > 0);
        const used = !unbounded && spent != null && ceiling != null ? pct(spent, ceiling) : null;
        const hot = used != null && used >= 90;
        const labSessions = byLab[lab.id] || [];
        const expanded = !!open[lab.id];
        return (
          <div
            key={lab.id}
            className="rounded-2xl bg-black/50 border border-white/[0.08] p-4 backdrop-blur-2xl"
          >
            <button
              className="w-full flex items-center justify-between text-left"
              onClick={() => setOpen((o) => ({ ...o, [lab.id]: !o[lab.id] }))}
              aria-expanded={expanded}
            >
              <span className="flex items-center space-x-2">
                <FlaskConical className="w-4 h-4 text-cyan-300" />
                <span className="text-white font-medium text-sm">
                  {lab.display_name || lab.id}
                </span>
                <span className="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-white/[0.06] text-slate-300 border-white/10">
                  {lab.autonomy_level}
                </span>
                {lab.status !== 'active' && (
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-amber-500/20 text-amber-200 border-amber-400/30">
                    {lab.status}
                  </span>
                )}
                {unbounded && (
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-rose-500/20 text-rose-200 border-rose-400/40">
                    NO-CEILING · UNBOUNDED
                  </span>
                )}
              </span>
              <span className="flex items-center space-x-2 text-xs text-slate-300">
                {unbounded ? (
                  <span>{spent != null ? fmtMoney(spent) : '—'} / UNBOUNDED (no ceiling)</span>
                ) : (
                  <span>{`${spent != null ? fmtMoney(spent) : '—'} / ${ceiling != null ? fmtMoney(ceiling) : '—'} (${used ?? '—'}%)`}</span>
                )}
                {hot && <AlertTriangle className="w-3.5 h-3.5 text-amber-300" />}
                {expanded ? (
                  <ChevronUp className="w-4 h-4" />
                ) : (
                  <ChevronDown className="w-4 h-4" />
                )}
              </span>
            </button>
            <div className="mt-2 h-1.5 rounded-full bg-white/[0.06]">
              {unbounded ? (
                <div className="h-1.5 rounded-full bg-rose-400/70" style={{ width: '100%' }} title="No ceiling — unbounded spend alarm" />
              ) : (
                <div
                  className={`h-1.5 rounded-full ${hot ? 'bg-amber-400' : 'bg-cyan-400'}`}
                  style={{ width: `${used ?? 0}%` }}
                />
              )}
            </div>
            {expanded && (
              <div className="mt-3 space-y-1.5">
                {/* U1-b: operator writes (ceiling/suspend/refund/admin) —
                    bearer + admin envelope, refusals verbatim. */}
                <LabOpsPanel lab={lab} onChanged={reload} />
                {labSessions.length === 0 && (
                  <div className="text-xs text-slate-500">No sessions in this lab.</div>
                )}
                {labSessions.map((s) => (
                  <div
                    key={s.id}
                    className="flex items-center justify-between text-xs px-2 py-1.5 rounded-lg bg-white/[0.04] border border-white/[0.06]"
                  >
                    <span className="text-slate-200">{s.title || s.id}</span>
                    <span className="text-slate-400 font-mono">
                      {`${typeof s.current_spend_usd === 'number' && Number.isFinite(s.current_spend_usd) ? fmtMoney(s.current_spend_usd) : '—'} / ${typeof s.spend_limit_usd === 'number' && Number.isFinite(s.spend_limit_usd) ? fmtMoney(s.spend_limit_usd) : '—'} · ${s.status}`}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
