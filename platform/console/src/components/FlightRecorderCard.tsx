'use client';

import React from 'react';
import { API_BASE, apiFetch, authHeaders } from '@/lib/api';
import { usePollingResource } from '@/domain/use-polling-resource';

// Documented budget, not an invented one: the surface title below names the
// "(60s Gate)" auditor window. The measured fetch latency renders against
// it honestly — over budget shows the measured overrun, never a pass.
export const FLIGHT_RECORDER_GATE_BUDGET_MS = 60_000;

export interface FlightRecorderData {  ok: boolean;
  session_id: string;
  tenant_id: string;
  terminal_verdict: string;
  spent_usd: number;
  spend_limit_usd: number;
  duration_seconds: number;
  step_count: number;
  stop_reason: string;
  last_claims: Array<{
    id: string;
    statement: string;
    verdict: string;
    evidence_json: string;
  }>;
  recent_events: Array<{
    id: number;
    event_type: string;
    timestamp: string;
    entry_hash: string;
  }>;
}

// Malformed-payload guards: backend numerics may arrive null/undefined
// on a degraded bridge. Finite numbers render; anything else renders
// '—', never a toFixed crash.
function fmtUsd(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? `$${v.toFixed(2)}` : '—';
}
function fmtSecs(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? `${v.toFixed(1)}s` : '—';
}

function FlightRecorderCardInner({ sessionId }: { sessionId: string }) {
  const { data: telemetry, status, error } = usePollingResource(
    async () => {
      const t0 = typeof performance !== 'undefined' ? performance.now() : 0;
      const res = await apiFetch(`${API_BASE}/api/campaigns/${sessionId}/flight-recorder`, {
        cache: 'no-store',
        headers: authHeaders(),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = (await res.json()) as FlightRecorderData;
      if (json == null || typeof json !== 'object') {
        throw new Error('flight recorder returned no telemetry (null body)');
      }
      return { json, latencyMs: typeof performance !== 'undefined' ? performance.now() - t0 : 0 };
    },
    { intervalMs: 15_000 },
  );
  const data = telemetry?.json ?? null;
  const latencyMs = telemetry?.latencyMs ?? null;
  const loading = status === 'loading';

  return (
    <div className="rounded-control-compact border border-border-strong bg-surface p-4 text-text shadow-[var(--shadow-float)]">
      <div className="flex items-center justify-between border-b border-border pb-3">
        <div className="flex items-center space-x-2">
          <span className="flex h-2.5 w-2.5 rounded-capsule bg-accent" />
          <h3 className="text-body-strong font-semibold tracking-wide uppercase text-label-secondary">
            Auditor Flight Recorder (60s Gate)
          </h3>
        </div>
        {data && (
          <span
            className={`rounded-chip px-2.5 py-0.5 text-label font-mono font-bold tracking-wider uppercase ${
              data.terminal_verdict === 'SURVIVES'
                ? 'bg-success-wash text-success-ink border border-success-edge'
                : data.terminal_verdict === 'FALSIFIED' || data.terminal_verdict === 'REFUTED'
                ? 'bg-danger-wash text-danger-ink border border-danger-edge'
                : data.terminal_verdict === 'BUDGET_EXHAUSTED'
                ? 'bg-warning-wash text-warning-ink border border-warning-edge'
                : 'bg-fill text-label-secondary'
            }`}
          >
            {data.terminal_verdict}
          </span>
        )}
      </div>

      {loading && <p className="mt-3 text-label text-muted">Loading flight recorder telemetry…</p>}
      {error && <p className="mt-3 text-label text-danger-ink">Error: {error}</p>}
      {/* Measured fetch latency with its documented gate budget. A missed
          budget renders as an overrun in rose — never a passing badge. */}
      {latencyMs !== null && (
        <p
          data-testid="flight-recorder-latency"
          className={`mt-3 text-label font-mono ${latencyMs <= FLIGHT_RECORDER_GATE_BUDGET_MS ? 'text-muted' : 'text-danger-ink'}`}
        >
          telemetry fetch: {Math.round(latencyMs)} ms (gate budget 60,000 ms)
          {latencyMs > FLIGHT_RECORDER_GATE_BUDGET_MS ? ' — over budget' : ' — within budget'}
        </p>
      )}

      {data && (
        <div className="mt-3 space-y-4 text-label">
          {/* Key Auditor Questions (Top summary) */}
          <div className="grid grid-cols-2 gap-3 rounded-chip bg-card p-3 border border-border">
            <div>
              <span className="text-[12px] font-semibold tracking-wider text-muted uppercase">
                Why Did Agent Stop?
              </span>
              <p className="mt-1 font-mono text-label text-text break-words">
                {data.stop_reason || 'In Progress'}
              </p>
            </div>
            <div>
              <span className="text-[12px] font-semibold tracking-wider text-muted uppercase">
                What Did It Spend?
              </span>
              <p className="mt-1 font-mono text-label text-success-ink">
                {fmtUsd(data.spent_usd)}{' '}
                <span className="text-faint font-sans">
                  / {fmtUsd(data.spend_limit_usd)} limit
                </span>
              </p>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2 text-center text-muted">
            <div className="rounded-chip bg-tile p-2">
              <div className="text-[12px] uppercase tracking-wider">Tenant</div>
              <div className="mt-0.5 font-mono text-text">{data.tenant_id}</div>
            </div>
            <div className="rounded-chip bg-tile p-2">
              <div className="text-[12px] uppercase tracking-wider">Duration</div>
              <div className="mt-0.5 font-mono text-text">
                {fmtSecs(data.duration_seconds)}
              </div>
            </div>
            <div className="rounded-chip bg-tile p-2">
              <div className="text-[12px] uppercase tracking-wider">Step Count</div>
              <div className="mt-0.5 font-mono text-text">{data.step_count}</div>
            </div>
          </div>

          {Array.isArray(data.last_claims) && data.last_claims.length > 0 && (
            <div>
              <span className="text-[12px] font-semibold tracking-wider text-muted uppercase">
                Primary Claim & Verification Verdict
              </span>
              <div className="mt-1 rounded-chip bg-card p-2 border border-border">
                <div className="flex items-center justify-between">
                  <span className="font-mono text-[12px] text-info-ink">
                    {data.last_claims[0].id}
                  </span>
                  <span className="font-bold text-warning-ink">
                    {data.last_claims[0].verdict}
                  </span>
                </div>
                <p className="mt-1 text-label-secondary">{data.last_claims[0].statement}</p>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// Key the inner component by session id so switching sessions resets state
// without a synchronous setState inside an effect (React 19 lint rule).
export function FlightRecorderCard({ sessionId }: { sessionId: string | null }) {
  if (!sessionId) return null;
  return <FlightRecorderCardInner key={sessionId} sessionId={sessionId} />;
}
