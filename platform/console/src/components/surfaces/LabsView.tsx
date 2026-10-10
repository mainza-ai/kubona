'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { FlaskConical, ChevronDown, ChevronUp, AlertTriangle } from 'lucide-react';
import { Lab, Session } from '@/types';
import { fetchLabs, fetchSessions } from '@/lib/api';

function pct(spent: number, ceiling: number): number {
  if (!(ceiling > 0)) return 0;
  return Math.min(100, Math.round((spent / ceiling) * 100));
}

export function LabsView() {
  const [labs, setLabs] = useState<Lab[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let live = true;
    Promise.all([fetchLabs(), fetchSessions()]).then(([l, s]) => {
      if (!live) return;
      setLabs(l);
      setSessions(s);
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
  if (labs.length === 0) {
    return <div className="text-slate-400 text-sm">No laboratories registered.</div>;
  }

  return (
    <div className="space-y-4">
      {labs.map((lab) => {
        const used = pct(lab.spent_usd ?? 0, lab.spend_ceiling_usd);
        const hot = used >= 90;
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
              </span>
              <span className="flex items-center space-x-2 text-xs text-slate-300">
                <span>{`$${lab.spent_usd ?? 0} / $${lab.spend_ceiling_usd} (${used}%)`}</span>
                {hot && <AlertTriangle className="w-3.5 h-3.5 text-amber-300" />}
                {expanded ? (
                  <ChevronUp className="w-4 h-4" />
                ) : (
                  <ChevronDown className="w-4 h-4" />
                )}
              </span>
            </button>
            <div className="mt-2 h-1.5 rounded-full bg-white/[0.06]">
              <div
                className={`h-1.5 rounded-full ${hot ? 'bg-amber-400' : 'bg-cyan-400'}`}
                style={{ width: `${used}%` }}
              />
            </div>
            {expanded && (
              <div className="mt-3 space-y-1.5">
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
                      {`$${s.current_spend_usd ?? 0} / $${s.spend_limit_usd} · ${s.status}`}
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
