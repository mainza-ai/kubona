'use client';

import React, { useState, useEffect, useCallback } from 'react';
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
  fetchQuantumBackends,
  fetchQuantumSettings,
  saveQuantumSettings,
  runQuantumCanary,
  type QuantumBackendInfo,
} from '@/lib/api';

// Quantum provider selection — the human-surface counterpart to the
// agent execution rail (which picks per MCP call, default
// local-simulator). This surface stores ONLY selection: default backend
// + per-harness overrides. Secrets are never accepted here — credentials
// live in the daemon environment (KUBONA_* / Bao sidecar) and surface
// only as presence. Selecting a credentialless QPU is allowed (plan
// before the token arrives); submit still refuses fail-closed.
export function QuantumSettings() {
  const [backends, setBackends] = useState<QuantumBackendInfo[]>([]);
  const [defaultBackend, setDefaultBackend] = useState<string>('local-simulator');
  const [harnessMap, setHarnessMap] = useState<Record<string, string>>({});
  const [newHarness, setNewHarness] = useState('');
  const [newBackend, setNewBackend] = useState('local-simulator');
  const [baseline, setBaseline] = useState('');
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);
  const [canarying, setCanarying] = useState(false);
  const [canaryOut, setCanaryOut] = useState<string | null>(null);
  const [canaryResults, setCanaryResults] = useState<Record<string, unknown> | null>(null);

  const snapshot = useCallback(
    () => JSON.stringify({ default_backend: defaultBackend, harness_backends: harnessMap }),
    [defaultBackend, harnessMap],
  );
  const dirty = baseline !== '' && baseline !== snapshot();

  const load = useCallback(() => {
    // setState lives in .then continuations only (lint-clean in effects):
    // the effect below merely kicks off the fetch.
    void Promise.all([fetchQuantumBackends(), fetchQuantumSettings()]).then(([b, s]) => {
      setMessage(null);
      setBackends(b);
      if (s) {
        const def = typeof s.default_backend === 'string' ? s.default_backend : 'local-simulator';
        const hm = (s.harness_backends ?? {}) as Record<string, string>;
        setDefaultBackend(def);
        setHarnessMap(hm);
        setNewBackend(def);
        setBaseline(JSON.stringify({ default_backend: def, harness_backends: hm }));
      }
    });
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const onSave = useCallback(async () => {
    setSaving(true);
    setMessage(null);
    const res = await saveQuantumSettings({ default_backend: defaultBackend, harness_backends: harnessMap });
    setSaving(false);
    if (res.ok) {
      setBaseline(snapshot());
      setMessage({ kind: 'ok', text: 'Quantum provider selection saved.' });
    } else {
      setMessage({ kind: 'err', text: res.error || 'Save refused.' });
    }
  }, [defaultBackend, harnessMap, snapshot]);

  const credBadge = (b: QuantumBackendInfo) => {
    if (b.credential_state === 'ready') {
      return (
        <span className="px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold bg-emerald-950/60 text-emerald-300 border border-emerald-800/60">
          READY
        </span>
      );
    }
    if (b.credential_state === 'credentialless') {
      return (
        <span
          className="px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold bg-sky-950/60 text-sky-300 border border-sky-800/60"
          title={`This daemon holds no credentials — set ${b.credential_env} to enable it.`}
        >
          NO CREDENTIALS
        </span>
      );
    }
    return (
      <span className="px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold bg-white/[0.06] text-slate-400 border border-white/10">
        UNAVAILABLE
      </span>
    );
  };

  return (
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2.5">
          <Cpu className="w-5 h-5 text-cyan-300" />
          <div>
            <h2 className="text-sm font-bold text-white font-mono tracking-tight">Quantum Providers</h2>
            <p className="text-[11px] text-slate-400 font-sans">
              Human default backend + per-harness overrides. Agents keep their own per-call selection (default local-simulator).
            </p>
          </div>
        </div>
        <button
          onClick={() => void load()}
          aria-label="Reload backends and saved selection"
          className="p-2 rounded-xl bg-white/[0.04] border border-white/10 text-slate-300 hover:bg-white/[0.08]"
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
          className="p-2 rounded-xl bg-white/[0.04] border border-white/10 text-slate-300 hover:bg-white/[0.08] disabled:opacity-50"
          title="Run the live canary (calibration reads only, no QPU spend). Credentialless providers skip; declared catalog stays the fallback."
        >
          <Activity className={canarying ? 'w-4 h-4 animate-pulse' : 'w-4 h-4'} />
        </button>
      </div>
      {canaryOut && (
        <div className="text-[11px] text-slate-300 font-mono bg-black/40 border border-white/[0.06] rounded-xl px-3 py-2">
          {canaryOut}
        </div>
      )}
      {canaryResults && Object.keys(canaryResults).length > 0 && (
        <details data-testid="canary-results" className="text-[11px] text-slate-300 font-mono bg-black/40 border border-white/[0.06] rounded-xl px-3 py-2">
          <summary className="cursor-pointer text-cyan-200">Measured canary results (live, no QPU spend)</summary>
          <ul className="mt-2 space-y-1">
            {Object.entries(canaryResults).map(([provider, result]) => (
              <li key={provider} className="break-all">
                <span className="text-white">{provider}</span>: {JSON.stringify(result).slice(0, 500)}
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
            className={`flex items-center justify-between gap-3 p-3 rounded-xl border cursor-pointer transition-all ${
              defaultBackend === b.provider
                ? 'border-cyan-500/50 bg-cyan-950/20'
                : 'border-white/[0.07] hover:border-white/15 bg-black/40'
            }`}
          >
            <div className="flex items-center space-x-3">
              <input
                type="radio"
                name="quantum-default-backend"
                checked={defaultBackend === b.provider}
                onChange={() => setDefaultBackend(b.provider)}
                className="accent-cyan-400"
              />
              <div>
                <div className="text-xs font-bold text-white font-mono">{b.display_name}</div>
                <div className="text-[11px] text-slate-400 font-mono">
                  {b.provider} · {b.modality} · {b.device}
                  {b.telemetry === 'declared' && (
                    <span
                      className="ml-1.5 px-1.5 py-px rounded bg-slate-800/80 text-slate-300 border border-white/10 text-[9px] font-bold"
                      title="Vendor catalog, not a live measurement."
                    >
                      CATALOG
                    </span>
                  )}
                </div>
                {b.credential_env !== '' && b.credential_state !== 'ready' && (
                  <div className="text-[11px] text-slate-500 font-mono flex items-center space-x-1 mt-0.5">
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
                  className="px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold bg-amber-950/60 text-amber-300 border border-amber-800/60"
                  title="Local solving works; the cloud path for this modality is not wired yet."
                >
                  LOCAL ONLY
                </span>
              )}
            </div>
          </label>
        ))}
        {backends.length === 0 && (
          <div className="text-[11px] text-slate-500 font-mono p-3">No backends reported — is the daemon reachable?</div>
        )}
      </div>

      {/* Per-harness overrides */}
      <div className="space-y-2">
        <div className="text-xs font-semibold text-slate-300 font-mono uppercase tracking-wider flex items-center space-x-2">
          <FlaskConical className="w-4 h-4 text-slate-400" />
          <span>Per-harness overrides (optional)</span>
        </div>
        {Object.entries(harnessMap).map(([harness, backend]) => (
          <div key={harness} className="flex items-center space-x-2 text-xs font-mono">
            <span className="text-white bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2 flex-1">{harness}</span>
            <span className="text-slate-500">→</span>
            <label htmlFor={`quantum-harness-backend-${harness}`} className="sr-only">Backend for harness {harness}</label>
            <select
              id={`quantum-harness-backend-${harness}`}
              value={backend}
              aria-label={`Backend for harness ${harness}`}
              onChange={(e) => setHarnessMap((m) => ({ ...m, [harness]: e.target.value }))}
              className="bg-black/40 border border-white/10 rounded-lg px-3 py-2 text-slate-200"
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
              className="px-3 py-2 rounded-lg bg-white/[0.04] border border-white/10 text-slate-400 hover:bg-white/[0.08]"
            >
              remove
            </button>
          </div>
        ))}
        <div className="flex items-center space-x-2 text-xs font-mono">
          <label htmlFor="quantum-new-harness" className="sr-only">New harness name</label>
          <input
            id="quantum-new-harness"
            value={newHarness}
            aria-label="New harness name"
            onChange={(e) => setNewHarness(e.target.value)}
            placeholder="harness name (e.g. vqe-ansatz)"
            className="bg-black/40 border border-white/10 rounded-lg px-3 py-2 text-slate-200 flex-1 placeholder:text-slate-600"
          />
          <span className="text-slate-500">→</span>
          <label htmlFor="quantum-new-harness-backend" className="sr-only">Backend for new harness</label>
          <select
            id="quantum-new-harness-backend"
            value={newBackend}
            aria-label="Backend for new harness"
            onChange={(e) => setNewBackend(e.target.value)}
            className="bg-black/40 border border-white/10 rounded-lg px-3 py-2 text-slate-200"
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
            className="px-3 py-2 rounded-lg bg-white/[0.04] border border-white/10 text-slate-300 hover:bg-white/[0.08]"
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
          className={`px-4 py-2 rounded-xl font-mono text-xs font-bold flex items-center space-x-2 border ${
            dirty && !saving
              ? 'bg-cyan-600/20 text-cyan-200 border-cyan-500/50 hover:bg-cyan-600/30'
              : 'bg-white/[0.03] text-slate-500 border-white/10 cursor-not-allowed'
          }`}
        >
          <Save className="w-4 h-4" />
          <span>{saving ? 'Saving…' : dirty ? 'Save selection' : 'Saved'}</span>
        </button>
        {message?.kind === 'ok' && (
          <span className="text-[11px] text-emerald-300 font-mono flex items-center space-x-1.5">
            <CheckCircle2 className="w-4 h-4" />
            <span>{message.text}</span>
          </span>
        )}
        {message?.kind === 'err' && (
          <span className="text-[11px] text-rose-300 font-mono flex items-center space-x-1.5">
            <AlertCircle className="w-4 h-4" />
            <span>{message.text}</span>
          </span>
        )}
        <span className="text-[11px] text-slate-500 font-mono flex items-center space-x-1.5">
          <ShieldAlert className="w-3.5 h-3.5" />
          <span>Selection only — keys stay in daemon env/Bao and are never accepted here.</span>
        </span>
      </div>
    </div>
  );
}
