'use client';

import React, { useState, useCallback } from 'react';
import {
  Cpu,
  RefreshCw,
  Save,
  CheckCircle2,
  AlertCircle,
  ShieldAlert,
  KeyRound,
  FlaskConical,
  Activity,
} from 'lucide-react';
import {
  saveQuantumSettings,
  runQuantumCanary,
  type QuantumBackendInfo,
} from '@/lib/api';
import { useQuantumSettingsProjection } from '@/domain/useQuantumSettingsProjection';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

// Quantum provider selection — the human-surface counterpart to the
// agent execution rail (which picks per MCP call, default
// local-simulator). This surface stores ONLY selection: default backend
// + per-harness overrides. Secrets are never accepted here — credentials
// live in the daemon environment (KUBONA_* / Bao sidecar) and surface
// only as presence. Selecting a credentialless QPU is allowed (plan
// before the token arrives); submit still refuses fail-closed.
export function QuantumSettings() {
  const { data, error, refresh } = useQuantumSettingsProjection();
  if (!data) {
    return (
      <div className="rounded-sheet border border-border bg-page p-7 text-[12px] text-label-secondary">
        <p className="font-medium text-text">Quantum provider catalog unavailable</p>
        <p className="mt-1 text-muted">{error || 'Loading provider catalog and saved selection…'}</p>
        <button type="button" onClick={refresh} className="mt-3 rounded-control-compact border border-border px-3 py-1.5 text-text">
          Retry
        </button>
      </div>
    );
  }
  return <QuantumSettingsForm key={JSON.stringify(data.settings ?? {})} initialBackends={data.backends} initialSettings={data.settings} onRefresh={refresh} />;
}

function QuantumSettingsForm({
  initialBackends,
  initialSettings,
  onRefresh,
}: {
  initialBackends: QuantumBackendInfo[];
  initialSettings: Record<string, unknown> | null;
  onRefresh: () => void;
}) {
  const initialDefault = typeof initialSettings?.default_backend === 'string' ? initialSettings.default_backend : 'local-simulator';
  const initialHarness = (initialSettings?.harness_backends ?? {}) as Record<string, string>;
  const backends = initialBackends;
  const [defaultBackend, setDefaultBackend] = useState<string>(initialDefault);
  const [harnessMap, setHarnessMap] = useState<Record<string, string>>(initialHarness);
  const [newHarness, setNewHarness] = useState('');
  const [newBackend, setNewBackend] = useState(initialDefault);
  const [baseline, setBaseline] = useState(JSON.stringify({ default_backend: initialDefault, harness_backends: initialHarness }));
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);
  const settlement = useWriteSettlement();
  const [canarying, setCanarying] = useState(false);
  const [canaryOut, setCanaryOut] = useState<string | null>(null);
  const [canaryResults, setCanaryResults] = useState<Record<string, unknown> | null>(null);

  const snapshot = useCallback(
    () => JSON.stringify({ default_backend: defaultBackend, harness_backends: harnessMap }),
    [defaultBackend, harnessMap],
  );
  const dirty = baseline !== '' && baseline !== snapshot();

  const onSave = useCallback(async () => {
    setSaving(true);
    setMessage(null);
    const requestId = settlement.begin('quantum provider selection');
    const res = await saveQuantumSettings({ default_backend: defaultBackend, harness_backends: harnessMap });
    setSaving(false);
    if (res.ok) {
      settlement.applied(requestId, 'quantum provider selection saved');
      setBaseline(snapshot());
      setMessage({ kind: 'ok', text: 'Quantum provider selection saved.' });
      onRefresh();
    } else {
      settlement[res.unknown ? 'unknown' : 'refused'](requestId, res.error || 'save not confirmed');
      setMessage({ kind: 'err', text: res.error || 'Save refused.' });
    }
  }, [defaultBackend, harnessMap, onRefresh, snapshot, settlement]);

  const credBadge = (b: QuantumBackendInfo) => {
    if (b.credential_state === 'ready') {
      return (
        <span className="px-2 py-0.5 rounded-capsule text-[12px] font-mono font-semibold bg-success-wash text-success-ink border border-success-edge">
          READY
        </span>
      );
    }
    if (b.credential_state === 'credentialless') {
      return (
        <span
          className="px-2 py-0.5 rounded-capsule text-[12px] font-mono font-semibold bg-info-wash text-info-ink border border-info-edge"
          title={`This daemon holds no credentials — set ${b.credential_env} to enable it.`}
        >
          NO CREDENTIALS
        </span>
      );
    }
    return (
      <span className="px-2 py-0.5 rounded-capsule text-[12px] font-mono font-semibold bg-tile text-muted border border-border">
        UNAVAILABLE
      </span>
    );
  };

  return (
    <div className="relative rounded-sheet overflow-hidden bg-page border border-border shadow-xl p-7 space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2.5">
          <Cpu className="w-5 h-5 text-info-ink" />
          <div>
            <h2 className="text-body-strong font-bold text-text font-mono tracking-tight">Quantum Providers</h2>
            <p className="text-[12px] text-muted font-sans">
              Human default backend + per-harness overrides. Agents keep their own per-call selection (default local-simulator).
            </p>
          </div>
        </div>
        <button
          onClick={onRefresh}
          aria-label="Reload backends and saved selection"
          className="p-2 rounded-row bg-tile border border-border text-label-secondary hover:bg-fill-hover"
          title="Reload backends and saved selection"
        >
          <RefreshCw className="w-4 h-4" />
        </button>
        <button
          onClick={() => {
            setCanarying(true);
            setCanaryOut(null);
            setCanaryResults(null);
            void runQuantumCanary().then((r) => {
              setCanarying(false);
              if (!r.ok) {
                setCanaryOut(`canary failed (declared catalog stays): ${r.error ?? 'unknown'}`);
                return;
              }
              const results = r.results ?? {};
              setCanaryResults(results);
              const names = Object.keys(results);
              setCanaryOut(
                names.length === 0
                  ? 'canary ran: no provider results (all skipped or credentialless)'
                  : `canary ran${r.cached ? ' (cached)' : ''}: ${names.join(', ')}`,
              );
            });
          }}
          disabled={canarying}
          aria-label="Run live canary check"
          className="p-2 rounded-row bg-tile border border-border text-label-secondary hover:bg-fill-hover disabled:opacity-50"
          title="Run the live canary (calibration reads only, no QPU spend). Credentialless providers skip; declared catalog stays the fallback."
        >
          <Activity className={canarying ? 'w-4 h-4 animate-pulse' : 'w-4 h-4'} />
        </button>
      </div>
      {canaryOut && (
        <div className="text-[12px] text-label-secondary font-mono bg-surface border border-border rounded-row px-3 py-2">
          {canaryOut}
        </div>
      )}
      {canaryResults && Object.keys(canaryResults).length > 0 && (
        <details data-testid="canary-results" className="text-[12px] text-label-secondary font-mono bg-surface border border-border rounded-row px-3 py-2">
          <summary className="cursor-pointer text-info-ink">Measured canary results (live, no QPU spend)</summary>
          <ul className="mt-2 space-y-1">
            {Object.entries(canaryResults).map(([provider, result]) => (
              <li key={provider} className="break-all">
                <span className="text-text">{provider}</span>: {JSON.stringify(result).slice(0, 500)}
              </li>
            ))}
          </ul>
        </details>
      )}

      {/* Backend list */}
      <div className="space-y-2">
        {backends.map((b) => (
          <label
            key={b.provider}
            className={`flex items-center justify-between gap-3 p-3 rounded-row border cursor-pointer transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] ${
              defaultBackend === b.provider
                ? 'border-info-edge bg-info-wash'
                : 'border-border hover:border-border-strong bg-surface'
            }`}
          >
            <div className="flex items-center space-x-3">
              <input
                type="radio"
                name="quantum-default-backend"
                checked={defaultBackend === b.provider}
                onChange={() => setDefaultBackend(b.provider)}
                className="accent-accent"
              />
              <div>
                <div className="text-label font-bold text-text font-mono">{b.display_name}</div>
                <div className="text-[12px] text-muted font-mono">
                  {b.provider} · {b.modality} · {b.device}
                  {b.telemetry === 'declared' && (
                    <span
                      className="ml-1.5 px-1.5 py-px rounded-chip bg-tile text-label-secondary border border-border text-[12px] font-bold"
                      title="Vendor catalog, not a live measurement."
                    >
                      CATALOG
                    </span>
                  )}
                </div>
                {b.credential_env !== '' && b.credential_state !== 'ready' && (
                  <div className="text-[12px] text-muted font-mono flex items-center space-x-1 mt-0.5">
                    <KeyRound className="w-3 h-3" />
                    <span>set {b.credential_env} on the daemon to enable</span>
                  </div>
                )}
              </div>
            </div>
            <div className="flex items-center space-x-2">
              {credBadge(b)}
              {!b.executable_now && b.credential_state === 'ready' && (
                <span
                  className="px-2 py-0.5 rounded-capsule text-[12px] font-mono font-semibold bg-warning-wash text-warning-ink border border-warning-edge"
                  title="Local solving works; the cloud path for this modality is not wired yet."
                >
                  LOCAL ONLY
                </span>
              )}
            </div>
          </label>
        ))}
        {backends.length === 0 && (
          <div className="text-[12px] text-muted font-mono p-3">No backends reported — is the daemon reachable?</div>
        )}
      </div>

      {/* Per-harness overrides */}
      <div className="space-y-2">
        <div className="text-label font-semibold text-label-secondary font-mono uppercase tracking-wider flex items-center space-x-2">
          <FlaskConical className="w-4 h-4 text-muted" />
          <span>Per-harness overrides (optional)</span>
        </div>
        {Object.entries(harnessMap).map(([harness, backend]) => (
          <div key={harness} className="flex items-center space-x-2 text-label font-mono">
            <span className="text-text bg-tile border border-border rounded-control-compact px-3 py-2 flex-1">{harness}</span>
            <span className="text-muted">→</span>
            <label htmlFor={`quantum-harness-backend-${harness}`} className="sr-only">Backend for harness {harness}</label>
            <select
              id={`quantum-harness-backend-${harness}`}
              value={backend}
              aria-label={`Backend for harness ${harness}`}
              onChange={(e) => setHarnessMap((m) => ({ ...m, [harness]: e.target.value }))}
              className="bg-field border border-border rounded-control-compact px-3 py-2 text-text"
            >
              {backends.map((b) => (
                <option key={b.provider} value={b.provider}>
                  {b.provider}
                </option>
              ))}
            </select>
            <button
              onClick={() =>
                setHarnessMap((m) => {
                  const next = { ...m };
                  delete next[harness];
                  return next;
                })
              }
              aria-label={`Remove harness ${harness}`}
              className="px-3 py-2 rounded-control-compact bg-tile border border-border text-muted hover:bg-fill-hover"
            >
              remove
            </button>
          </div>
        ))}
        <div className="flex items-center space-x-2 text-label font-mono">
          <label htmlFor="quantum-new-harness" className="sr-only">New harness name</label>
          <input
            id="quantum-new-harness"
            value={newHarness}
            aria-label="New harness name"
            onChange={(e) => setNewHarness(e.target.value)}
            placeholder="harness name (e.g. vqe-ansatz)"
            className="bg-field border border-border rounded-control-compact px-3 py-2 text-text flex-1 placeholder:text-faint"
          />
          <span className="text-muted">→</span>
          <label htmlFor="quantum-new-harness-backend" className="sr-only">Backend for new harness</label>
          <select
            id="quantum-new-harness-backend"
            value={newBackend}
            aria-label="Backend for new harness"
            onChange={(e) => setNewBackend(e.target.value)}
            className="bg-field border border-border rounded-control-compact px-3 py-2 text-text"
          >
            {backends.map((b) => (
              <option key={b.provider} value={b.provider}>
                {b.provider}
              </option>
            ))}
          </select>
          <button
            onClick={() => {
              const h = newHarness.trim();
              if (h === '') return;
              setHarnessMap((m) => ({ ...m, [h]: newBackend }));
              setNewHarness('');
            }}
            className="px-3 py-2 rounded-control-compact bg-tile border border-border text-label-secondary hover:bg-fill-hover"
          >
            add
          </button>
        </div>
      </div>

      {/* Save */}
      <div className="flex items-center space-x-3">
        <button
          onClick={() => void onSave()}
          disabled={!dirty || saving}
          className={`px-4 py-2 rounded-row font-mono text-label font-bold flex items-center space-x-2 border ${
            dirty && !saving
              ? 'bg-info-wash text-info-ink border-info-edge hover:opacity-90'
              : 'bg-tile text-faint border-border cursor-not-allowed'
          }`}
        >
          <Save className="w-4 h-4" />
          <span>{saving ? 'Saving…' : dirty ? 'Save selection' : 'Saved'}</span>
        </button>
        {message?.kind === 'ok' && (
          <span className="text-[12px] text-success-ink font-mono flex items-center space-x-1.5">
            <CheckCircle2 className="w-4 h-4" />
            <span>{message.text}</span>
          </span>
        )}
        {message?.kind === 'err' && (
          <span className="text-[12px] text-danger-ink font-mono flex items-center space-x-1.5">
            <AlertCircle className="w-4 h-4" />
            <span>{message.text}</span>
          </span>
        )}
        {writeSettlementLabel(settlement.write) && (
          <span role="status" className="text-[12px] text-muted font-mono">
            {writeSettlementLabel(settlement.write)}
          </span>
        )}
        <span className="text-[12px] text-muted font-mono flex items-center space-x-1.5">
          <ShieldAlert className="w-3.5 h-3.5" />
          <span>Selection only — keys stay in daemon env/Bao and are never accepted here.</span>
        </span>
      </div>
    </div>
  );
}
