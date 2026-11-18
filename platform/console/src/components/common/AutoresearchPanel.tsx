'use client';

import React, { useCallback, useEffect, useState } from 'react';
import {
  fetchAutoresearchHarnesses,
  fetchAutoresearchLoops,
  startAutoresearchLoop,
  stepAutoresearchLoop,
  fetchSessionAgentic,
  setSessionAgentic,
  pauseAutoresearchLoop,
  resumeAutoresearchLoop,
  wakeAutoresearchLoop,
  stopAutoresearchLoop,
  steerAutoresearchLoop,
} from '@/domain/autoresearch';
import type { AutoresearchHarness, AutoresearchLoop } from '@/types';
import { needSentence, remedyFor } from '@/lib/need';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

export function AutoresearchPanel({ sessionId }: { sessionId: string }) {
  const [harnesses, setHarnesses] = useState<AutoresearchHarness[]>([]);
  const [loops, setLoops] = useState<AutoresearchLoop[] | null>(null);
  const [harnessId, setHarnessId] = useState('');
  const [runner, setRunner] = useState<Record<string, unknown> | null>(null);
  const [maxIterations, setMaxIterations] = useState('10');
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // Session master switch: null until the policy read resolves. The switch is
  // session-scoped; an operator halt locks it (session callers cannot clear it).
  const [agentic, setAgentic] = useState<{ enabled: boolean; locked: boolean; stages: Record<string, string> } | null>(null);
  const [steerDrafts, setSteerDrafts] = useState<Record<string, string>>({});
  const settlement = useWriteSettlement();

  const load = useCallback(async () => {
    const [h, l, a] = await Promise.all([
      fetchAutoresearchHarnesses(),
      fetchAutoresearchLoops(),
      sessionId ? fetchSessionAgentic(sessionId) : Promise.resolve(null),
    ]);
    setHarnesses(h.harnesses);
    setLoops(l);
    setHarnessId((cur) => cur || h.harnesses[0]?.id || '');
    if (a) setAgentic({ enabled: a.agentic_enabled, locked: a.agentic_locked, stages: a.stages });
  }, [sessionId]);

  useEffect(() => {
    let live = true;
    Promise.all([
      fetchAutoresearchHarnesses(),
      fetchAutoresearchLoops(),
      sessionId ? fetchSessionAgentic(sessionId) : Promise.resolve(null),
    ]).then(([h, l, a]) => {
      if (!live) return;
      setHarnesses(h.harnesses);
      setRunner(h.docker_runner);
      setLoops(l);
      setHarnessId((cur) => cur || h.harnesses[0]?.id || '');
      if (a) setAgentic({ enabled: a.agentic_enabled, locked: a.agentic_locked, stages: a.stages });
    });
    return () => { live = false; };
  }, [sessionId]);

  const toggleAgentic = async () => {
    if (!sessionId || !agentic || agentic.locked) return;
    setBusy('agentic');
    setNotice(null);
    const next = !agentic.enabled;
    const out = await setSessionAgentic(sessionId, next);
    if (out.ok) {
      setAgentic({ enabled: out.state.agentic_enabled, locked: out.state.agentic_locked, stages: out.state.stages });
      setNotice(next
        ? 'Auto agent on — loops may run under the session policy.'
        : `Manual mode — agent loops and plan execution are blocked${out.paused_loops ? `; ${out.paused_loops} loop(s) paused` : ''}.`);
      await load();
    } else {
      setNotice(out.error);
      await load();
    }
    setBusy(null);
  };

  const run = async (kind: string, fn: () => Promise<unknown>, success: string) => {
    setBusy(kind);
    setNotice(null);
    const requestId = settlement.begin(`${kind} loop`);
    try {
      const out = await fn();
      if (out) {
        settlement.applied(requestId, `${kind} loop · ${success.replace(/\.$/, '')}`);
      } else {
        settlement.unknown(requestId, `${kind} not confirmed — backend refused or transport failed`);
      }
      await load();
    } catch (err) {
      settlement.unknown(requestId, `${kind} not confirmed — ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-full rounded-card border border-border bg-surface p-4 space-y-2" aria-label="Autoresearch loops">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <span className="text-label font-semibold text-text">Autoresearch loops</span>
          <p className="text-[12px] text-faint">Sandboxed harness stepping; each transition re-reads server state.</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-[11px] uppercase tracking-wide text-faint">Auto agent</span>
          <button
            type="button"
            role="switch"
            aria-checked={agentic?.enabled === true}
            aria-label="Auto agent mode"
            disabled={agentic === null || agentic.locked || busy !== null}
            title={agentic === null ? 'Agentic mode unavailable' : agentic.locked ? 'Locked by an operator halt' : agentic.enabled ? 'Turn agentic workflow off' : 'Turn agentic workflow on'}
            onClick={() => void toggleAgentic()}
            className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-[background-color,border-color,opacity] ${agentic?.enabled ? 'bg-success-wash border-success-edge' : 'bg-fill border-border'} disabled:opacity-40`}
          >
            <span aria-hidden className={`inline-block h-3.5 w-3.5 rounded-full bg-text transition-transform ${agentic?.enabled ? 'translate-x-[18px]' : 'translate-x-0.5'}`} />
          </button>
          <span className="text-[11px] font-semibold text-label-secondary">
            {agentic === null ? '—' : agentic.locked ? 'Locked' : agentic.enabled ? 'On' : 'Off'}
          </span>
          <button type="button" onClick={() => void load()} className="px-2 py-1 rounded-control-compact bg-fill border border-border text-[12px] text-label-secondary">Refresh</button>
        </div>
      </div>
      {agentic && (
        <p role="status" className={`text-[12px] ${agentic.locked || !agentic.enabled ? 'text-warning' : 'text-muted'}`}>
          {agentic.locked
            ? 'Locked by an operator halt — only an operator can unlock agentic work.'
            : agentic.enabled
              ? `Agentic workflow is on — execute stage: ${agentic.stages.execute ?? '—'}.`
              : 'Manual mode — agent loops and plan execution are blocked; manual experiments remain available.'}
        </p>
      )}
      {runner && runner.available !== true && (
        <p role="status" className="text-[12px] text-warning">
          Sandbox unavailable — autoresearch is fail-closed by default. {String(runner.reason || '')}
        </p>
      )}
      {runner && runner.available === true && (
        <p className="text-[12px] text-muted">Sandbox ready: {String(runner.image || 'runner')} (read-only, cap-drop, no network).</p>
      )}
      <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
        <select aria-label="Autoresearch harness" value={harnessId} onChange={(e) => setHarnessId(e.target.value)} className="bg-field border border-border rounded-control-compact px-2 py-1 text-text font-mono">
          {harnesses.length === 0 && <option value="">no harnesses</option>}
          {harnesses.map((h) => <option key={h.id} value={h.id}>{h.id}</option>)}
        </select>
        <input aria-label="Max iterations" value={maxIterations} onChange={(e) => setMaxIterations(e.target.value)} inputMode="numeric" className="w-16 bg-field border border-border rounded-control-compact px-2 py-1 text-text font-mono" />
        <button type="button" disabled={busy !== null || !harnessId || !sessionId || !agentic?.enabled || agentic.locked} title={!agentic?.enabled ? 'Turn auto agent on to start loops' : agentic?.locked ? 'Locked by an operator halt' : 'Start loop'} onClick={() => {
          const n = Number(maxIterations);
          if (!Number.isInteger(n) || n < 1 || n > 1000) { setNotice('max iterations must be an integer 1..1000.'); return; }
          void run('start', () => startAutoresearchLoop(harnessId, n, undefined, sessionId), 'Loop started.');
        }} className="px-2 py-1 rounded-control-compact bg-soft-lilac-wash border border-soft-lilac-edge text-soft-lilac-ink disabled:opacity-40">
          Start loop
        </button>
      </div>
      {(notice || writeSettlementLabel(settlement.write)) && (
        <p role="status" className="text-[12px] text-label-secondary">{writeSettlementLabel(settlement.write) || notice}</p>
      )}
      {loops === null && <p className="text-[12px] text-faint">Loading loops…</p>}
      {loops !== null && loops.length === 0 && <p className="text-[12px] text-muted">No autoresearch loops.</p>}
      {(loops ?? []).map((loop) => (
        <div key={loop.id} className="px-2.5 py-2 rounded-row bg-card border border-border text-[12px] space-y-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="font-code text-label-secondary">{loop.id.slice(0, 8)} · {loop.harness_id} · {loop.status} · {loop.current_iteration}/{loop.max_iterations}</span>
            <span className="flex items-center gap-1">
              <button type="button" disabled={busy !== null} onClick={() => void run('step', () => stepAutoresearchLoop(loop.id), 'Stepped.')} className="px-2 py-1 rounded-chip bg-info-wash border border-info-edge text-info-ink disabled:opacity-40">Step</button>
              {loop.status === 'running' ? (
                <button type="button" disabled={busy !== null} onClick={() => void run('pause', () => pauseAutoresearchLoop(loop.id), 'Paused.')} className="px-2 py-1 rounded-chip bg-warning-wash border border-warning-edge text-warning-ink disabled:opacity-40">Pause</button>
              ) : loop.status === 'sleeping' ? (
                <button type="button" disabled={busy !== null} onClick={() => void run('wake', () => wakeAutoresearchLoop(loop.id), 'Woken.')} className="px-2 py-1 rounded-chip bg-success-wash border border-success-edge text-success-ink disabled:opacity-40">Wake</button>
              ) : (
                <button type="button" disabled={busy !== null} onClick={() => void run('resume', () => resumeAutoresearchLoop(loop.id), 'Resumed.')} className="px-2 py-1 rounded-chip bg-success-wash border border-success-edge text-success-ink disabled:opacity-40">Resume</button>
              )}
              <button type="button" disabled={busy !== null} onClick={() => void run('stop', () => stopAutoresearchLoop(loop.id), 'Stopped.')} className="px-2 py-1 rounded-chip bg-danger-wash border border-danger-edge text-danger-ink disabled:opacity-40">Stop</button>
            </span>
          </div>
          <div className="text-faint font-code">champion {loop.champion_metric?.toFixed(6) ?? '—'} · depth {loop.champion_depth ?? '—'} · 2Q {loop.champion_2q_gates ?? '—'}{loop.stagnation_reason ? ` · ${loop.stagnation_reason}` : ''}</div>
          {loop.need && (
            <p className="rounded-control-compact border border-warning-edge bg-warning-wash px-2 py-1 text-[11px] text-warning-ink" data-need={loop.need.kind}>
              {needSentence(loop.need)}
              {remedyFor(loop.need.kind) ? ` · ${remedyFor(loop.need.kind)!.label}` : ''}
            </p>
          )}
          {loop.status === 'running' && (
            <div className="flex items-center gap-1.5 pt-1">
              <input
                aria-label={`Steer autoresearch loop ${loop.id.slice(0, 8)}`}
                value={steerDrafts[loop.id] ?? ''}
                maxLength={500}
                placeholder="Instruction for the next model turn…"
                onChange={(event) => setSteerDrafts((prev) => ({ ...prev, [loop.id]: event.target.value }))}
                className="flex-1 min-w-0 bg-field border border-border rounded-control-compact px-2 py-1 text-text font-mono"
              />
              <button
                type="button"
                disabled={busy !== null || !(steerDrafts[loop.id] ?? '').trim()}
                onClick={async () => {
                  const text = (steerDrafts[loop.id] ?? '').trim();
                  if (!text) return;
                  setBusy('steer');
                  setNotice(null);
                  const out = await steerAutoresearchLoop(loop.id, text);
                  if (out.ok) {
                    setNotice(`Queued (${out.steer.id.slice(0, 8)}) — consumed by the next model turn.`);
                    setSteerDrafts((prev) => ({ ...prev, [loop.id]: '' }));
                    await load();
                  } else {
                    setNotice(out.error);
                  }
                  setBusy(null);
                }}
                className="px-2 py-1 rounded-chip bg-info-wash border border-info-edge text-info-ink disabled:opacity-40"
              >
                Steer
              </button>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
