'use client';

import React, { useEffect, useState } from 'react';
import { Volume2, VolumeX } from 'lucide-react';
import { AUTH_REQUIRED_EVENT, fetchCostSummary } from '@/lib/api';
import { usePollingResource } from '@/domain/use-polling-resource';
import { useAttention } from '@/lib/attention-context';
import type { Loop } from '@/types';

function statusOf(loop: Loop): string {
  return String(loop.status || '').toLowerCase();
}

// D3-lite: one-glance operational strip (loops by status + LLM spend).
// Honest subset of the vision §7 calm check-in mode: burn-rate needs
// server-side time series and anomaly detection needs an anomaly feed —
// neither exists, so this strip shows measured counts only, never rates
// or alerts. Failed/diverged loops are called out by text, not color alone.

export function CheckInStrip({ loops, compact = false }: { loops: Loop[]; compact?: boolean }) {
  const { data: spend } = usePollingResource(async () => {
    const out = await fetchCostSummary();
    if (!out.ok) throw new Error(out.error);
    return out.result;
  }, { intervalMs: 60_000 });
  const [readOnly, setReadOnly] = useState(false);

  // An anonymous 401 is not an expired session. Keep the sign-in affordance
  // local to this strip instead of raising the global destructive banner.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onRequired = () => setReadOnly(true);
    window.addEventListener(AUTH_REQUIRED_EVENT, onRequired);
    return () => window.removeEventListener(AUTH_REQUIRED_EVENT, onRequired);
  }, []);

  let running = 0;
  let paused = 0;
  let attention = 0;
  for (const l of loops) {
    const s = statusOf(l);
    if (s === 'running') running += 1;
    else if (s === 'paused') paused += 1;
    else if (s === 'diverged' || s === 'failed' || s === 'killed' || l.diverged) attention += 1;
  }

  // P4: director-routed alert from any surface (observatory surprise,
  // spend gates, deadlocks). Ambient stays text-only; glance and above
  // get a level-colored pill. Mute toggle rides alongside (audio engine).
  // R-10: the presence dial lives here — Hands/Eyes/Ear/Away gates what
  // the Director may surface, per plan §3.
  const { alert, muted, toggleMute, presence, setPresence } = useAttention();
  const showAlert = alert && alert.channel !== 'ambient';
  const PRESENCE: Array<{ id: 'hands' | 'eyes' | 'ear' | 'away'; label: string; hint: string }> = [
    { id: 'hands', label: 'Hands', hint: 'Direct authoring: non-critical summons suppressed' },
    { id: 'eyes', label: 'Eyes', hint: 'Spectator/director: glance and above surface' },
    { id: 'ear', label: 'Ear', hint: 'Ambient: nudge and summon chime, at reduced presence' },
    { id: 'away', label: 'Away', hint: 'Lighthouse: only Level-3 summons pass' },
  ];

  return (
    <div
      role="status"
      aria-label={`Check-in: ${loops.length} loops, ${running} running${attention > 0 ? `, ${attention} need attention` : ''}${spend ? `, LLM spend $${spend.spent_usd.toFixed(2)}` : ''}`}
      className={`flex min-w-0 items-center gap-3 text-[12px] font-code text-slate-300 ${compact ? 'py-0' : 'border-b border-white/[0.06] bg-black/30 px-3 py-1'}`}
    >
      <span className="truncate">
        {loops.length} loop{loops.length === 1 ? '' : 's'} · {running} running · {paused} paused
      </span>
      {readOnly && (
        <span className="truncate text-slate-500" title="Read-only access: sign in through Mesh → Labs for live loops and writes">
          · read-only · sign in for live loops
        </span>
      )}
      {attention > 0 && (
        <span className="text-rose-300 shrink-0">· {attention} need{attention === 1 ? 's' : ''} attention</span>
      )}
      {spend && (
        <span className="truncate text-slate-400">
          · LLM ${spend.spent_usd.toFixed(2)}
          {spend.track_only ? ' · track-only (no cap)' : ` / $${spend.cap_usd.toFixed(0)} cap`}
          {spend.local_bypass ? ' · local $0' : ''}
          {spend.unmetered_count > 0 ? ` · ${spend.unmetered_count} blind` : ''}
        </span>
      )}
      {showAlert && (
        <span
          className={`shrink-0 truncate font-medium ${
            alert.level === 'anomalous'
              ? 'text-rose-300 animate-pulse'
              : alert.level === 'notable'
                ? 'text-amber-300'
                : 'text-cyan-300'
          }`}
          title={`${alert.source} · ${alert.reason}`}
        >
          · {alert.channel} {alert.level}: {alert.message.slice(0, 80)}
          {alert.duplicates > 0 ? ` (×${alert.duplicates + 1})` : ''}
        </span>
      )}
      <span className="shrink-0 flex items-center gap-0.5" title="Presence dial: your current degree of presence gates what the Director may surface">
        {PRESENCE.map((p) => (
          <button
            key={p.id}
            onClick={() => setPresence(p.id)}
            title={p.hint}
            aria-pressed={presence === p.id}
            className={`px-1.5 py-0.5 rounded text-[12px] font-code transition-colors ${
              presence === p.id
                ? 'bg-cyan-500/20 text-cyan-200'
                : 'text-slate-500 hover:text-slate-300'
            }`}
          >
            {p.label}
          </button>
        ))}
      </span>
      <button
        onClick={toggleMute}
        title={muted ? 'Unmute ambient laboratory sound' : 'Mute ambient laboratory sound'}
        className="shrink-0 text-slate-500 hover:text-slate-300 transition-colors flex items-center"
      >
        {muted ? <VolumeX className="w-3.5 h-3.5" /> : <Volume2 className="w-3.5 h-3.5" />}
      </button>
    </div>
  );
}
