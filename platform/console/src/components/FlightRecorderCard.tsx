'use client';

import React, { useEffect, useState } from 'react';
import { API_BASE, apiFetch, authHeaders } from '@/lib/api';

export interface FlightRecorderData {
  ok: boolean;
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

function FlightRecorderCardInner({ sessionId }: { sessionId: string }) {
  const [data, setData] = useState<FlightRecorderData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetchTelemetry = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await apiFetch(`${API_BASE}/api/campaigns/${sessionId}/flight-recorder`, {
          cache: 'no-store',
          headers: authHeaders(),
        });
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        const json = (await res.json()) as FlightRecorderData;
        if (!cancelled) {
          setData(json);
        }
      } catch (err: unknown) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load flight recorder');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    fetchTelemetry();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  return (
    <div className="rounded-lg border border-slate-700 bg-slate-900/95 p-4 text-slate-100 shadow-xl backdrop-blur">
      <div className="flex items-center justify-between border-b border-slate-800 pb-3">
        <div className="flex items-center space-x-2">
          <span className="flex h-2.5 w-2.5 rounded-full bg-cyan-400 animate-pulse" />
          <h3 className="text-sm font-semibold tracking-wide uppercase text-slate-200">
            Auditor Flight Recorder (60s Gate)
          </h3>
        </div>
        {data && (
          <span
            className={`rounded px-2.5 py-0.5 text-xs font-mono font-bold tracking-wider uppercase ${
              data.terminal_verdict === 'SURVIVES'
                ? 'bg-emerald-950 text-emerald-400 border border-emerald-800'
                : data.terminal_verdict === 'FALSIFIED' || data.terminal_verdict === 'REFUTED'
                ? 'bg-rose-950 text-rose-400 border border-rose-800'
                : data.terminal_verdict === 'BUDGET_EXHAUSTED'
                ? 'bg-amber-950 text-amber-400 border border-amber-800'
                : 'bg-slate-800 text-slate-300'
            }`}
          >
            {data.terminal_verdict}
          </span>
        )}
      </div>

      {loading && <p className="mt-3 text-xs text-slate-400">Loading flight recorder telemetry…</p>}
      {error && <p className="mt-3 text-xs text-rose-400">Error: {error}</p>}

      {data && (
        <div className="mt-3 space-y-4 text-xs">
          {/* Key Auditor Questions (Top summary) */}
          <div className="grid grid-cols-2 gap-3 rounded bg-slate-950/80 p-3 border border-slate-800">
            <div>
              <span className="text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                Why Did Agent Stop?
              </span>
              <p className="mt-1 font-mono text-xs text-slate-200 break-words">
                {data.stop_reason || 'In Progress'}
              </p>
            </div>
            <div>
              <span className="text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                What Did It Spend?
              </span>
              <p className="mt-1 font-mono text-xs text-emerald-400">
                ${data.spent_usd.toFixed(2)}{' '}
                <span className="text-slate-500 font-sans">
                  / ${data.spend_limit_usd.toFixed(2)} limit
                </span>
              </p>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2 text-center text-slate-400">
            <div className="rounded bg-slate-800/40 p-2">
              <div className="text-[10px] uppercase tracking-wider">Tenant</div>
              <div className="mt-0.5 font-mono text-slate-200">{data.tenant_id}</div>
            </div>
            <div className="rounded bg-slate-800/40 p-2">
              <div className="text-[10px] uppercase tracking-wider">Duration</div>
              <div className="mt-0.5 font-mono text-slate-200">
                {data.duration_seconds.toFixed(1)}s
              </div>
            </div>
            <div className="rounded bg-slate-800/40 p-2">
              <div className="text-[10px] uppercase tracking-wider">Step Count</div>
              <div className="mt-0.5 font-mono text-slate-200">{data.step_count}</div>
            </div>
          </div>

          {data.last_claims && data.last_claims.length > 0 && (
            <div>
              <span className="text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                Primary Claim & Verification Verdict
              </span>
              <div className="mt-1 rounded bg-slate-950/50 p-2 border border-slate-800/80">
                <div className="flex items-center justify-between">
                  <span className="font-mono text-[11px] text-cyan-300">
                    {data.last_claims[0].id}
                  </span>
                  <span className="font-bold text-amber-300">
                    {data.last_claims[0].verdict}
                  </span>
                </div>
                <p className="mt-1 text-slate-300">{data.last_claims[0].statement}</p>
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
