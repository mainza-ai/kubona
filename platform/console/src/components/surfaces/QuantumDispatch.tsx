'use client';

import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { acceptWrite, newWriteRequestId, settleWrite, writeSettlementLabel, IDLE_WRITE, type WriteSettlement } from '@/domain/write-settlement';
import { Zap, Play, Ban, RefreshCw, Receipt, FlaskConical } from 'lucide-react';
import {
  submitQuantumJob,
  fetchQuantumJob,
  cancelQuantumJob,
  fetchLedgerExport,
} from '@/lib/api';
import { CryptographicMandateCardWidget } from '@/components/widgets/CryptographicMandateCardWidget';
import { useQuantumSettingsProjection } from '@/domain/useQuantumSettingsProjection';

const BELL_QASM = `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
measure q -> c;`;

const DEFAULT_QUBO = `[
  [-1.0, 2.0],
  [0.0, -1.0]
]`;

// Human quantum dispatch (R-34 console surface). Flow: pick backend →
// estimate (dry-run: full validation, no spend, no enqueue) → mandate
// card when the estimate is non-zero → dispatch (idempotency key minted
// per human intent) → poll status → cancel if needed. Every refusal
// renders verbatim; nothing here can invent a result.

// Poll tuning: base cadence, backoff multiplier + ceiling (same shape as
// FleetRadar's 1.5×/15 s discipline), and the consecutive-transport-loss
// budget after which polling pauses with an explicit Resume.
const POLL_BASE_MS = 5000;
const POLL_BACKOFF_FACTOR = 1.5;
const POLL_MAX_DELAY_MS = 15000;
const POLL_MAX_TRANSPORT_FAILURES = 5;
export function QuantumDispatch({
  sessionId,
  tenantId,
}: {
  sessionId: string | null;
  tenantId?: string;
}) {
  const { data: quantumSettings } = useQuantumSettingsProjection();
  const backends = quantumSettings?.backends ?? [];
  const defaultProvider = typeof quantumSettings?.settings?.default_backend === 'string' ? quantumSettings.settings.default_backend : 'local-simulator';
  const [providerDraft, setProvider] = useState('');
  const provider = providerDraft || defaultProvider;
  const [qasm, setQasm] = useState(BELL_QASM);
  const [shots, setShots] = useState(1024);
  const [estimating, setEstimating] = useState(false);
  const [estimate, setEstimate] = useState<Record<string, unknown> | null>(null);
  const [fidelity, setFidelity] = useState<Record<string, unknown> | null>(null);
  const [estimateError, setEstimateError] = useState<string | null>(null);
  const [mandateRef, setMandateRef] = useState<string | null>(null);
  const [dispatching, setDispatching] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [jobStatus, setJobStatus] = useState<string | null>(null);
  const [jobError, setJobError] = useState<string | null>(null);
  const [dispatchWrite, setDispatchWrite] = useState<WriteSettlement>(IDLE_WRITE);
  const [cancelWrite, setCancelWrite] = useState<WriteSettlement>(IDLE_WRITE);
  const [cancelNote, setCancelNote] = useState<string | null>(null);
  const [refunds, setRefunds] = useState<Array<Record<string, unknown>>>([]);
  // Poll discipline (mirrors FleetRadar): a chained setTimeout (never a
  // fixed setInterval) with exponential backoff on transport loss, a
  // visibility gate, and an explicit pause-with-resume after repeated
  // transport failures — a dead backend must not spin the loop forever.
  const [pollPaused, setPollPaused] = useState(false);
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollFailsRef = useRef(0);
  const pollDelayRef = useRef(POLL_BASE_MS);
  const pollJobIdRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) clearTimeout(pollRef.current);
    };
  }, []);

  const stopPoll = useCallback(() => {
    if (pollRef.current) {
      clearTimeout(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  const pollJob = useCallback((id: string) => {
    stopPoll();
    pollJobIdRef.current = id;
    pollFailsRef.current = 0;
    pollDelayRef.current = POLL_BASE_MS;
    setPollPaused(false);
    const schedule = (ms: number) => {
      pollRef.current = setTimeout(() => void tick(), ms);
    };
    const tick = async () => {
      // Visibility gate: no polling while the tab is hidden — reschedule
      // without counting a failure (same as FleetRadar / HF hub).
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        if (pollJobIdRef.current === id) schedule(2000);
        return;
      }
      const st = await fetchQuantumJob(id);
      if (pollJobIdRef.current !== id) return;
      if (!st) {
        pollFailsRef.current += 1;
        if (pollFailsRef.current >= POLL_MAX_TRANSPORT_FAILURES) {
          stopPoll();
          setPollPaused(true);
          setJobError('backend unreachable — polling paused');
          return;
        }
        setJobError('Status fetch failed (transport).');
        pollDelayRef.current = Math.min(pollDelayRef.current * POLL_BACKOFF_FACTOR, POLL_MAX_DELAY_MS);
        schedule(pollDelayRef.current);
        return;
      }
      if (!st.ok) {
        setJobError(String(st.error ?? 'unknown status error'));
        stopPoll();
        return;
      }
      pollFailsRef.current = 0;
      pollDelayRef.current = POLL_BASE_MS;
      const status = String(st.status ?? 'unknown');
      setJobStatus(status);
      if (status !== 'queued' && status !== 'running') {
        stopPoll();
        return;
      }
      schedule(POLL_BASE_MS);
    };
    void tick();
  }, [stopPoll]);

  const [quoteFingerprint, setQuoteFingerprint] = useState<string | null>(null);
  const currentFingerprint = useMemo(
    () => JSON.stringify({ sessionId: sessionId ?? null, provider, qasm, shots }),
    [sessionId, provider, qasm, shots],
  );
  const quoteFresh = estimate != null && quoteFingerprint === currentFingerprint;

  const onEstimate = useCallback(async () => {
    setEstimating(true);
    setEstimateError(null);
    setEstimate(null);
    setFidelity(null);
    setMandateRef(null);
    setQuoteFingerprint(null);
    const fingerprint = JSON.stringify({ sessionId: sessionId ?? null, provider, qasm, shots });
    const res = await submitQuantumJob({
      session_id: sessionId ?? undefined,
      qasm,
      provider,
      shots,
      idempotency_key: `estimate-${Date.now()}`,
      dry_run: true,
      caller_jurisdictions: ['US', 'UK', 'EU', 'CA', 'DE'],
      caller_export_class: 'allied',
    });
    setEstimating(false);
    if (!res.ok) {
      setEstimateError(res.error ?? 'Estimate refused.');
      return;
    }
    setEstimate((res.estimate ?? {}) as Record<string, unknown>);
    setFidelity((res.fidelity ?? {}) as Record<string, unknown>);
    setQuoteFingerprint(fingerprint);
  }, [qasm, provider, shots, sessionId]);

  // A quote is only valid for the exact provider/QASM/shots/session it was
  // produced from (2026-10-26 audit UI-01). Editing any of them makes the
  // quote stale: the mandate and Dispatch disable until a fresh estimate.
  const estCost = quoteFresh && typeof estimate?.estimated_usd === 'number'
    ? estimate.estimated_usd
    : null;

  // Wave-2 money copy: the Dispatch button restates the ALREADY-COMPUTED
  // estimate at 2dp with the provider — no new fetching, no invented
  // figure (falls back to the generic label while unpriced/stale).
  // Exception: the dry-run quote line above stays 6dp by spec.
  const dispatchPriced = quoteFresh && estCost != null;
  const dispatchLabel = dispatching
    ? dispatchPriced
      ? `Dispatching — $${(estCost as number).toFixed(2)} on ${provider}…`
      : 'Dispatching…'
    : dispatchPriced
      ? `Dispatch — $${(estCost as number).toFixed(2)} on ${provider}`
      : 'Dispatch job';
  const onDispatch = useCallback(async () => {
    if (!quoteFresh || estCost == null) {
      setJobError('The estimate is stale or unpriced: re-run Estimate for the current provider/QASM/shots/session.');
      return;
    }
    setDispatching(true);
    setJobError(null);
    setCancelNote(null);
    const key = newWriteRequestId();
    setDispatchWrite(acceptWrite(key, 'submitting job'));
    const res = await submitQuantumJob({
      session_id: sessionId ?? undefined,
      qasm,
      provider,
      shots,
      idempotency_key: key,
      dry_run: false,
      mandate_ref: mandateRef ?? undefined,
      caller_jurisdictions: ['US', 'UK', 'EU', 'CA', 'DE'],
      caller_export_class: 'allied',
    });
    setDispatching(false);
    if (!res.ok || !res.job_id) {
      setDispatchWrite(settleWrite(key, res.unknown ? 'unknown' : 'refused', res.error ?? 'dispatch not confirmed'));
      setJobError(res.error ?? 'Dispatch refused.');
      return;
    }
    setDispatchWrite(settleWrite(key, 'applied', `job ${res.job_id.slice(0, 8)} queued`));
    setJobId(res.job_id);
    setJobStatus('queued');
    pollJob(res.job_id);
  }, [qasm, provider, shots, sessionId, mandateRef, pollJob, quoteFresh, estCost]);

  const onCancel = useCallback(async () => {
    if (!jobId) return;
    const requestId = newWriteRequestId();
    setCancelWrite(acceptWrite(requestId, 'cancel requested'));
    const res = await cancelQuantumJob(jobId, 'operator cancel from console');
    if (!res.ok) {
      setCancelWrite(settleWrite(requestId, res.unknown ? 'unknown' : 'refused', res.error ?? 'cancel not confirmed'));
      setCancelNote(`Cancel ${res.unknown ? 'unconfirmed' : 'refused'}: ${res.error ?? 'unknown'}`);
      return;
    }
    setCancelWrite(settleWrite(
      requestId,
      'applied',
      res.provider_recall_attempted ? 'fenced; vendor recall attempted' : 'fenced cooperatively',
    ));
    setCancelNote(
      res.provider_recall_attempted
        ? `Fenced + vendor recall attempted (${res.provider_recall_status ?? 'unknown'}). Re-check status — running work may complete underneath.`
        : 'Fenced cooperatively. Re-check status — running work may complete underneath.',
    );
    pollJob(jobId);
  }, [jobId, pollJob]);

  const loadRefunds = useCallback(() => {
    if (!sessionId) return;
    // setState lives in .then continuations only (lint-clean in effects).
    void fetchLedgerExport(sessionId).then((ex) => {
      if (!ex.ok || !ex.events) return;
      setRefunds(
        ex.events.filter((e) => String(e.event_type ?? e.type ?? '').includes('refund')),
      );
    });
  }, [sessionId]);

  useEffect(() => {
    void loadRefunds();
  }, [loadRefunds, jobStatus]);

  const executable = backends.filter((b) => b.dispatch_ready ?? b.executable_now);
  const selectedBackend = backends.find((b) => b.provider === provider);
  const isQuboModality =
    provider === 'dwave-annealing' ||
    selectedBackend?.input_format === 'qubo' ||
    selectedBackend?.modality === 'annealing';

  const handleProviderChange = (nextProvider: string) => {
    setProvider(nextProvider);
    const nextBackend = backends.find((b) => b.provider === nextProvider);
    const nextIsQubo =
      nextProvider === 'dwave-annealing' ||
      nextBackend?.input_format === 'qubo' ||
      nextBackend?.modality === 'annealing';
    if (nextIsQubo && qasm.trim() === BELL_QASM.trim()) {
      setQasm(DEFAULT_QUBO);
    } else if (!nextIsQubo && qasm.trim() === DEFAULT_QUBO.trim()) {
      setQasm(BELL_QASM);
    }
  };

  return (
    <div className="relative rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border)] shadow-xl p-7 space-y-5">
      <div className="flex items-center space-x-2.5">
        <Zap className="w-5 h-5 text-[var(--color-warning-ink)]" />
        <div>
          <h2 className="text-sm font-bold text-[var(--color-text)] font-mono tracking-tight">Human dispatch</h2>
          <p className="text-[12px] text-[var(--color-muted)] font-sans">
            Estimate first (no spend, no enqueue), mandate when non-zero, then dispatch. Async only — poll for results.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <label className="block text-xs font-mono text-[var(--color-label-secondary)] space-y-1.5">
          <span className="uppercase tracking-wider text-[12px]">Backend</span>
          <select
            value={provider}
            onChange={(e) => handleProviderChange(e.target.value)}
            className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3 py-2 text-[var(--color-text)]"
          >
            {backends.map((b) => {
              const ready = b.dispatch_ready ?? b.executable_now;
              return (
                <option key={b.provider} value={b.provider} disabled={!ready}>
                  {b.display_name}{ready ? '' : ' (not executable)'}
                </option>
              );
            })}
          </select>
        </label>
        <label className="block text-xs font-mono text-[var(--color-label-secondary)] space-y-1.5">
          <span className="uppercase tracking-wider text-[12px]">{isQuboModality ? 'Samples / Reads' : 'Shots'}</span>
          <input
            type="number"
            min={1}
            value={shots}
            onChange={(e) => setShots(Math.max(1, Number(e.target.value) || 1))}
            className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3 py-2 text-[var(--color-text)]"
          />
        </label>
      </div>

      <label className="block text-xs font-mono text-[var(--color-label-secondary)] space-y-1.5">
        <span className="uppercase tracking-wider text-[12px]">
          {isQuboModality ? 'QUBO matrix (JSON 2-D array)' : 'OpenQASM circuit'}
        </span>
        <textarea
          value={qasm}
          onChange={(e) => setQasm(e.target.value)}
          rows={7}
          spellCheck={false}
          className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3 py-2 text-[var(--color-text)] font-mono text-[12px]"
        />
      </label>

      <div className="flex items-center space-x-3">
        <button
          onClick={() => void onEstimate()}
          disabled={estimating || executable.length === 0}
          className="px-4 py-2 rounded-xl font-mono text-xs font-bold flex items-center space-x-2 bg-[var(--color-tile)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)] disabled:opacity-50"
        >
          <FlaskConical className="w-4 h-4" />
          <span>{estimating ? 'Estimating…' : 'Estimate (dry run)'}</span>
        </button>
        {!sessionId && (
          <span className="text-[12px] text-[var(--color-warning-ink)] font-mono">No session — estimates run on an implicit capped session.</span>
        )}
      </div>

      {estimateError && (
        <div className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px] font-mono">
          Estimate refused: {estimateError}
        </div>
      )}

      {estimate && (
        <div className="p-3 rounded-xl bg-[var(--color-surface)] border border-[var(--color-border)] text-[12px] font-mono text-[var(--color-label-secondary)] space-y-1">
          <div>Estimated cost: <span className="text-[var(--color-text)] font-bold">{estCost == null ? 'UNPRICED — re-estimate' : `$${estCost.toFixed(6)}`}</span></div>
          <div>Fidelity: <span className="text-[var(--color-text)]">{fidelity?.admitted === true ? 'admitted' : fidelity?.admitted === false ? `refused — ${String(fidelity?.rejection_reason ?? 'see receipt').slice(0, 160)}` : String(fidelity?.skipped ?? 'unknown')}</span></div>
          <div className="text-[var(--color-muted)]">Dry run only — nothing enqueued, nothing spent.</div>
        </div>
      )}

      {estimate && !quoteFresh && (
        <div className="p-3 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-[12px] font-mono">
          Estimate is stale: the provider, program, shots, or session changed. Re-estimate before dispatch.
        </div>
      )}

      {quoteFresh && estCost != null && estCost > 0 && !mandateRef && (
        <CryptographicMandateCardWidget
          actionDescription={`Quantum job on ${provider} (${shots} shots)`}
          estimatedCostUsd={estCost ?? 0}
          targetBackend={provider}
          sessionId={sessionId ?? undefined}
          tenantId={tenantId}
          programQasm={qasm}
          onAuthorize={(mandateJson) => setMandateRef(mandateJson)}
        />
      )}

      <div className="flex items-center space-x-3">
        <button
          onClick={() => void onDispatch()}
          disabled={dispatching || !quoteFresh || estCost == null || (estCost > 0 && !mandateRef)}
          title={!quoteFresh || estCost == null ? 'Re-estimate the current provider/QASM/shots/session' : estCost > 0 && !mandateRef ? 'Sign the mandate card first' : 'Dispatch'}
          className="px-4 py-2 rounded-xl font-mono text-xs font-bold flex items-center space-x-2 bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border border-[var(--color-success-edge)] hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Play className="w-4 h-4" />
          <span>{dispatchLabel}</span>
        </button>
        {quoteFresh && estCost != null && estCost > 0 && !mandateRef && (
          <span className="text-[12px] text-[var(--color-warning-ink)] font-mono">Non-zero estimate: sign the mandate card above first.</span>
        )}
      </div>

      {dispatchWrite.phase !== 'idle' && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-label-secondary)]">{writeSettlementLabel(dispatchWrite)}</p>
      )}
      {jobError && (
        <div className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px] font-mono">
          Dispatch refused: {jobError}
        </div>
      )}

      {jobId && (
        <div className="p-3 rounded-xl bg-[var(--color-surface)] border border-[var(--color-border)] text-[12px] font-mono text-[var(--color-label-secondary)] space-y-2">
          <div>Job <span className="text-[var(--color-text)]">{jobId}</span> — status: <span className="text-[var(--color-text)] font-bold">{jobStatus ?? '…'}</span></div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => void onCancel()}
              className="px-3 py-1.5 rounded-lg bg-[var(--color-tile)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)] flex items-center space-x-1.5"
            >
              <Ban className="w-3.5 h-3.5" />
              <span>Cancel job</span>
            </button>
            <button
              onClick={() => pollJob(jobId)}
              className="px-3 py-1.5 rounded-lg bg-[var(--color-tile)] border border-[var(--color-border)] text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] flex items-center space-x-1.5"
            >
              <RefreshCw className="w-3.5 h-3.5" />
              <span>Refresh</span>
            </button>
          </div>
          {pollPaused && (
            <div className="p-2.5 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-[12px] font-mono flex items-center justify-between gap-2">
              <span>backend unreachable — polling paused after repeated transport failures.</span>
              <button
                type="button"
                onClick={() => pollJob(jobId)}
                className="shrink-0 px-3 py-1.5 rounded-lg bg-[var(--color-surface)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] font-semibold"
              >
                Resume
              </button>
            </div>
          )}
          {writeSettlementLabel(cancelWrite) && (
            <p role="status" className="text-[12px] font-mono text-[var(--color-label-secondary)]">{writeSettlementLabel(cancelWrite)}</p>
          )}
          {cancelNote && <div className="text-[var(--color-warning-ink)]">{cancelNote}</div>}
        </div>
      )}

      <div className="space-y-2">
        <div className="text-[12px] font-mono uppercase tracking-wider text-[var(--color-muted)] flex items-center space-x-2">
          <Receipt className="w-4 h-4" />
          <span>Refunds on this session</span>
        </div>
        {refunds.length === 0 ? (
          <div className="text-[12px] text-[var(--color-muted)] font-mono">No refunds recorded. Refunds are requested out-of-band with an admin-signed envelope (see R-32) — they appear here once sealed.</div>
        ) : (
          refunds.map((r, i) => (
            <div key={i} className="p-2.5 rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[12px] font-mono text-[var(--color-success-ink)]">
              refunded ${String((r.payload as Record<string, unknown> | undefined)?.amount_usd ?? (r as Record<string, unknown>).amount_usd ?? '?')} — {String((r.payload as Record<string, unknown> | undefined)?.reason ?? (r as Record<string, unknown>).reason ?? '')}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
