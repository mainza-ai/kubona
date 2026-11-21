'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Field, Select } from '@/components/ui/Forms';
import {
  fetchAutonomyHistory,
  fetchAutonomyPolicy,
  fetchScienceTelemetry,
  fetchSessionReceipts,
  haltAutonomy,
  rollbackAutonomyPolicy,
  undoSessionReceipt,
  updateAutonomyPolicy,
  type ActionReceipt,
  type AutonomyHistoryEntry,
  type AutonomyPolicyRecord,
} from '@/domain/oversight';

const STAGE_KEYS = ['hypothesize', 'plan', 'execute', 'collapse'] as const;
const STAGE_MODES = [
  { value: 'suggest', label: 'suggest' },
  { value: 'approve', label: 'approve' },
  { value: 'notify', label: 'notify' },
  { value: 'act', label: 'act' },
];

export function OversightGovernancePanel({ sessionId }: { sessionId: string }) {
  const [policy, setPolicy] = useState<AutonomyPolicyRecord | null>(null);
  const [stages, setStages] = useState<Record<string, string>>({
    hypothesize: 'approve',
    plan: 'approve',
    execute: 'approve',
    collapse: 'approve',
  });
  const [maxActBudget, setMaxActBudget] = useState('1.0');
  const [haltReason, setHaltReason] = useState('operator halt');
  const [history, setHistory] = useState<AutonomyHistoryEntry[]>([]);
  const [receipts, setReceipts] = useState<ActionReceipt[]>([]);
  const [telemetry, setTelemetry] = useState<Record<string, unknown> | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!sessionId) return;
    const [pol, hist, recs, ops] = await Promise.all([
      fetchAutonomyPolicy(sessionId),
      fetchAutonomyHistory(sessionId),
      fetchSessionReceipts(sessionId),
      fetchScienceTelemetry(),
    ]);
    if (pol?.policy) {
      setPolicy(pol.policy);
      setStages(pol.policy.stages ?? {});
      setMaxActBudget(String(pol.policy.max_act_budget_usd ?? 1.0));
    }
    if (hist) setHistory(hist.entries ?? []);
    if (recs) setReceipts(recs.receipts ?? []);
    if (ops) setTelemetry(ops);
  }, [sessionId]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    void Promise.all([
      fetchAutonomyPolicy(sessionId),
      fetchAutonomyHistory(sessionId),
      fetchSessionReceipts(sessionId),
      fetchScienceTelemetry(),
    ]).then(([pol, hist, recs, ops]) => {
      if (!live) return;
      if (pol?.policy) {
        setPolicy(pol.policy);
        setStages(pol.policy.stages ?? {});
        setMaxActBudget(String(pol.policy.max_act_budget_usd ?? 1.0));
      }
      if (hist) setHistory(hist.entries ?? []);
      if (recs) setReceipts(recs.receipts ?? []);
      if (ops) setTelemetry(ops);
    });
    return () => { live = false; };
  }, [sessionId]);

  if (!sessionId) return null;

  const savePolicy = async (unlock = false) => {
    const budget = Number(maxActBudget);
    if (!Number.isFinite(budget) || budget < 0) {
      setError('Max act budget must be a finite number >= 0.');
      return;
    }
    setBusy(unlock ? 'unlock' : 'save');
    setError(null);
    setNotice(null);
    const res = await updateAutonomyPolicy(sessionId, {
      stages,
      max_act_budget_usd: budget,
      ...(unlock ? { agentic_locked: false, agentic_enabled: true } : {}),
    });
    if (!res.ok) setError(res.error);
    else {
      setNotice(unlock ? 'Operator lock cleared and policy updated.' : 'Autonomy policy saved.');
      await load();
    }
    setBusy(null);
  };

  const triggerHalt = async () => {
    setBusy('halt');
    setError(null);
    setNotice(null);
    const res = await haltAutonomy(sessionId, haltReason.trim() || 'operator halt');
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Operator halt applied (${res.data.cancelled_jobs ?? 0} jobs cancelled, ${res.data.paused_loops ?? 0} loops paused).`);
      await load();
    }
    setBusy(null);
  };

  const triggerRollback = async (eventId: number) => {
    setBusy(`rb-${eventId}`);
    setError(null);
    setNotice(null);
    const res = await rollbackAutonomyPolicy(sessionId, eventId);
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Rolled back autonomy policy to event #${eventId}.`);
      await load();
    }
    setBusy(null);
  };

  const triggerUndo = async (receiptId: string) => {
    setBusy(`undo-${receiptId}`);
    setError(null);
    setNotice(null);
    const res = await undoSessionReceipt(sessionId, receiptId);
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Undid receipt ${receiptId} (${res.data.action}).`);
      await load();
    }
    setBusy(null);
  };

  return (
    <div className="w-full rounded-card border border-border bg-surface p-4 space-y-4" aria-label="Autonomy governance">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <span className="text-label font-semibold text-text">Stage Autonomy, Receipts &amp; Science Telemetry</span>
          <p className="text-[12px] text-faint">Operator-governed stage ladder, halt/unlock, policy rollback, and action receipts.</p>
        </div>
        <div className="flex items-center gap-2">
          {policy?.agentic_locked && <Badge tone="warning">operator locked</Badge>}
          <Button size="compact" variant="ghost" onClick={() => void load()}>Refresh</Button>
        </div>
      </div>

      {error && <p role="alert" className="text-label text-danger-ink">{error}</p>}
      {notice && <p role="status" className="text-label text-muted">{notice}</p>}

      <div className="grid grid-cols-2 sm:grid-cols-5 gap-2 items-end">
        {STAGE_KEYS.map((stage) => (
          <Select
            key={stage}
            label={stage}
            value={stages[stage] ?? 'approve'}
            onChange={(e) => setStages((prev) => ({ ...prev, [stage]: e.target.value }))}
            options={STAGE_MODES}
          />
        ))}
        <Field
          label="Act cap ($)"
          type="number"
          step="any"
          min="0"
          value={maxActBudget}
          onChange={(e) => setMaxActBudget(e.target.value)}
        />
      </div>

      <div className="flex flex-wrap items-end gap-2">
        <Button size="compact" variant="primary" pending={busy === 'save'} onClick={() => void savePolicy(false)}>
          Save stage policy
        </Button>
        {policy?.agentic_locked && (
          <Button size="compact" variant="secondary" pending={busy === 'unlock'} onClick={() => void savePolicy(true)}>
            Clear operator lock
          </Button>
        )}
        <Field
          label="Halt reason"
          value={haltReason}
          onChange={(e) => setHaltReason(e.target.value)}
          className="flex-1 min-w-40"
        />
        <Button size="compact" variant="danger" pending={busy === 'halt'} onClick={() => void triggerHalt()}>
          Operator halt
        </Button>
      </div>

      {history.length > 0 && (
        <details className="border-t border-border pt-2">
          <summary className="cursor-pointer text-label font-medium text-text">Policy history ({history.length})</summary>
          <ul className="mt-2 space-y-1.5 text-caption">
            {history.slice(0, 8).map((entry) => (
              <li key={entry.event_id} className="flex items-center justify-between gap-2 rounded-control border border-border bg-card px-2.5 py-1.5">
                <span className="font-code text-muted">#{entry.event_id} · {entry.event_type} · {entry.timestamp}</span>
                <Button size="compact" variant="ghost" pending={busy === `rb-${entry.event_id}`} onClick={() => void triggerRollback(entry.event_id)}>
                  Rollback
                </Button>
              </li>
            ))}
          </ul>
        </details>
      )}

      <details className="border-t border-border pt-2">
        <summary className="cursor-pointer text-label font-medium text-text">Action receipts &amp; undo ({receipts.length})</summary>
        {receipts.length === 0 ? (
          <p className="mt-1 text-caption text-muted">No receipts recorded yet for this session.</p>
        ) : (
          <ul className="mt-2 space-y-1.5 text-caption">
            {receipts.slice(0, 10).map((r) => (
              <li key={r.id} className="flex items-center justify-between gap-2 rounded-control border border-border bg-card px-2.5 py-1.5">
                <span className="truncate font-code text-label-secondary">{r.event_type} · {r.summary || r.id}</span>
                {r.undoable && (
                  <Button size="compact" variant="ghost" pending={busy === `undo-${r.id}`} onClick={() => void triggerUndo(r.id)}>
                    Undo
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </details>

      {telemetry && (
        <details className="border-t border-border pt-2">
          <summary className="cursor-pointer text-label font-medium text-text">Science Ops Telemetry</summary>
          <pre className="mt-2 max-h-48 overflow-auto rounded-control border border-border bg-field p-2.5 font-code text-caption text-label-secondary">
            {JSON.stringify(telemetry, null, 2)}
          </pre>
        </details>
      )}
    </div>
  );
}
