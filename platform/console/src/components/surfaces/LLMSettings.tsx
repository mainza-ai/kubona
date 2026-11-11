'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Cpu,
  RefreshCw,
  Sliders,
  Send,
  Zap,
  Lock,
  Globe,
  Database,
  CheckCircle2,
  AlertCircle,
  ShieldAlert,
} from 'lucide-react';
import {
  fetchLLMSettings,
  saveLLMSettings,
  fetchLLMModels,
  testLLM,
} from '@/lib/api';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

interface ProviderConfig {
  type: string;
  name: string;
  base_url: string;
  api_key?: string;
  default_model: string;
  enabled: boolean;
  is_local: boolean;
  timeout_sec: number;
  last_success_at?: string;
}

interface RoleModelAssignment {
  role: string;
  provider: string;
  model: string;
}

interface SettingsPayload {
  active_provider: string;
  active_model: string;
  air_gapped: boolean;
  temperature: number;
  max_tokens: number;
  disable_dsml_fallback?: boolean;
  providers: Record<string, ProviderConfig>;
  role_assignments: Record<string, RoleModelAssignment>;
}

interface ModelInfo {
  id: string;
  owned_by: string;
  provider: string;
  is_default: boolean;
}

interface TestResult {
  success: boolean;
  reply?: string;
  model?: string;
  duration_ms?: number;
  gen_tok_per_sec?: number;
  prompt_tok_per_sec?: number;
  error?: string;
}

// Best-effort save-refusal mapper (Wave-2): attribute a backend refusal to
// the offending section so it can render adjacent to the relevant provider
// group. Matching is substring-based on provider keys/display names and
// known section keywords; anything unrecognized stays a general error.
function locateSaveSection(
  reason: string,
  settings: SettingsPayload,
): { section: string; provider?: string; message: string } {
  const lower = reason.toLowerCase();
  for (const [pt, pc] of Object.entries(settings.providers)) {
    if (lower.includes(pt.toLowerCase()) || (pc.name && lower.includes(pc.name.toLowerCase()))) {
      return { section: `provider ${pc.name}`, provider: pt, message: reason };
    }
  }
  if (lower.includes('temperature') || lower.includes('max_token') || lower.includes('max token')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('active_model') || lower.includes('active model')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('active_provider') || lower.includes('active provider')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('role') || lower.includes('routing') || lower.includes('assign')) {
    return { section: 'agent routing', message: reason };
  }
  if (lower.includes('air_gap') || lower.includes('air-gap') || lower.includes('airgap')) {
    return { section: 'air-gap controls', message: reason };
  }
  if (lower.includes('dsml')) {
    return { section: 'air-gap controls', message: reason };
  }
  if (lower.includes('api key') || lower.includes('api_key') || lower.includes('apikey')) {
    return { section: 'provider credentials', message: reason };
  }
  return { section: 'general', message: reason };
}

export function LLMSettings() {
  const [settings, setSettings] = useState<SettingsPayload | null>(null);
  const [baseline, setBaseline] = useState<string | null>(null);
  const [omlxModels, setOmlxModels] = useState<ModelInfo[]>([]);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testPrompt, setTestPrompt] = useState('Confirm you are active and describe how you optimize a VQE ansatz.');
  const [saveStatus, setSaveStatus] = useState<string>('');
  const [saveSectionError, setSaveSectionError] = useState<{ section: string; provider?: string; message: string } | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const settlement = useWriteSettlement();
  const [tokenError, setTokenError] = useState<string | null>(null);
  // Key edits live outside settings state: secrets are never rendered and
  // only sent when the operator types them. Empty provider keys in the save
  // payload mean "keep stored" (backend preserves non-empty secrets).
  const [keyEdits, setKeyEdits] = useState<Record<string, string>>({});
  // Unsaved marker: dirty when current JSON differs from last loaded/saved.
  const dirty = settings != null && baseline != null && JSON.stringify(settings) !== baseline;

  // Model list: the active provider's live models plus oMLX's (local
  // discovery baseline) — never biased to a single hardcoded provider.
  // setState lives in .then continuations only (lint-clean in effects).
  const loadModels = useCallback(() => {
    const providers = [settings?.active_provider, 'omlx'].filter((p): p is string => !!p);
    const seen = new Set<string>();
    const lists: Array<Promise<ModelInfo[]>> = [];
    for (const p of providers) {
      if (seen.has(p)) continue;
      seen.add(p);
      lists.push(
        fetchLLMModels(p).then(
          (data) => (Array.isArray((data as { models?: ModelInfo[] })?.models) ? (data as { models: ModelInfo[] }).models : []),
          () => [] as ModelInfo[]
        )
      );
    }
    void Promise.all(lists).then((arrs) => {
      const all: ModelInfo[] = [];
      for (const arr of arrs) {
        for (const m of arr) {
          if (!all.some((x) => x.id === m.id)) all.push(m);
        }
      }
      setOmlxModels(all);
    });
  }, [settings?.active_provider]);

  // Mount + retry loader. The error reset lives with the retry callers
  // (event handlers), never synchronously in the mount effect below —
  // on mount the error is already null from initialization.
  const loadAll = useCallback(() => {
    fetchLLMSettings()
      .then((data) => {
        if (data) {
          setSettings(data as unknown as SettingsPayload);
          setBaseline(JSON.stringify(data));
        } else {
          setLoadError('Settings refused — backend returned nothing. Retry to reload.');
        }
      })
      .catch((err) => setLoadError(`Failed to load LLM settings: ${err instanceof Error ? err.message : String(err)}`));
  }, []);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  const handleSave = async () => {
    if (!settings) return;
    setSaving(true);
    setSaveStatus('');
    setSaveSectionError(null);
    const requestId = settlement.begin('LLM settings');
    try {
      // Merge key edits; untouched providers send "" (backend keeps stored).
      const providers: Record<string, ProviderConfig> = {};
      for (const [pt, pc] of Object.entries(settings.providers)) {
        providers[pt] = { ...pc, api_key: keyEdits[pt] ?? '' };
      }
      const res = await saveLLMSettings({ ...settings, providers } as unknown as Record<string, unknown>);
      if (res.ok) {
        settlement.applied(requestId, 'LLM settings persisted');
        setSaveStatus('✅ Settings persisted successfully to SQLite ledger.');
        setKeyEdits({});
        // Dirty reference is the live settings shape: key edits are never
        // part of settings state (empty payload keys mean "keep stored"),
        // so baselining the merged payload would pin the gate dirty forever.
        setBaseline(JSON.stringify(settings));
      } else {
        const reason = res.error || 'no reason';
        settlement[res.unknown ? 'unknown' : 'refused'](requestId, reason);
        const mapped = locateSaveSection(reason, settings);
        setSaveSectionError(mapped);
        setSaveStatus(`❌ Failed to persist settings — backend refused (${mapped.section}: ${reason}). Nothing changed.`);
      }
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      settlement.unknown(requestId, reason);
      const mapped = locateSaveSection(reason, settings);
      setSaveSectionError(mapped);
      setSaveStatus(`❌ Network error (${mapped.section}: ${reason}) — nothing changed.`);
    } finally {
      setSaving(false);
    }
  };

  const setProvider = (pt: string, patch: Partial<ProviderConfig>) => {
    setSettings((prev) => {
      if (!prev) return prev;
      return { ...prev, providers: { ...prev.providers, [pt]: { ...prev.providers[pt], ...patch } } };
    });
  };

  const setRole = (role: string, patch: Partial<RoleModelAssignment>) => {
    setSettings((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        role_assignments: { ...prev.role_assignments, [role]: { ...prev.role_assignments[role], ...patch } },
      };
    });
  };

  // Model options for the active-model + per-role selects: live oMLX list,
  // every enabled provider's default, plus the current value as fallback so
  // the control never renders blank on a cloud selection.
  const modelOptions = (current: string): string[] => {
    const opts = [...omlxModels.map((m) => m.id)];
    if (settings) {
      for (const pc of Object.values(settings.providers)) {
        if (pc.enabled && pc.default_model && !opts.includes(pc.default_model)) opts.push(pc.default_model);
      }
    }
    if (current && !opts.includes(current)) opts.unshift(current);
    return opts;
  };

  const handleRunTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const data = await testLLM(testPrompt, 'copilot');
      if (data) {
        setTestResult(data as unknown as TestResult);
      } else {
        setTestResult({ success: false, error: 'No response from intelligence orchestrator.' });
      }
    } catch (err) {
      setTestResult({ success: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  };

  if (!settings) {
    return (
      <div className="p-12 text-center space-y-3">
        {loadError ? (
          <>
            <div className="text-[var(--color-danger-ink)] text-xs">Settings load failed — {loadError} Nothing loaded.</div>
            <button
              onClick={() => { setLoadError(null); loadAll(); }}
              className="px-4 py-1.5 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] text-[var(--color-text)] border border-[var(--color-border)] text-xs font-semibold"
            >
              Retry load
            </button>
          </>
        ) : (
          <div className="space-y-3">
            <div className="text-[var(--color-muted)] text-xs flex items-center justify-center space-x-2">
              <RefreshCw className="w-4 h-4 animate-spin text-[var(--color-accent)]" />
              <span>Loading Sovereign Intelligence Configuration...</span>
            </div>
            {/* Wave-3: honest first-load skeleton for the settings form
                (gated on settings === null — hidden the moment settings or a
                refusal lands). */}
            <div data-testid="llm-settings-skeleton" className="space-y-2 max-w-xl mx-auto" aria-hidden="true">
              <div className="h-4 w-1/2 mx-auto rounded bg-[var(--color-fill)] animate-pulse" />
              <div className="h-24 rounded-2xl bg-[var(--color-card)] border border-[var(--color-border)] animate-pulse" />
              <div className="h-24 rounded-2xl bg-[var(--color-card)] border border-[var(--color-border)] animate-pulse" />
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="relative rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border)] shadow-xl p-7 space-y-6 select-none">
      {/* Serene Header & Save Button */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[var(--color-border)] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] flex items-center justify-center">
            <Sliders className="w-5 h-5 text-[var(--color-info-ink)]" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-[var(--color-text)] tracking-tight flex items-center space-x-2">
              <span>Sovereign Intelligence Configuration</span>
              <span className="w-2 h-2 rounded-full bg-[var(--color-success)]" />
            </h2>
            <p className="text-xs text-[var(--color-muted)] mt-0.5">
              Local Apple Silicon Neural Engine (oMLX, Ollama), cloud models, and air-gap controls
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          {dirty && (
            <span className="px-2.5 py-1 rounded-full text-[12px] font-mono bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)] border border-[var(--color-warning-edge)]">
              Unsaved changes
            </span>
          )}
          {loadError && (
            <button
              onClick={() => { setLoadError(null); loadAll(); }}
              className="px-3 py-1.5 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs font-semibold"
              title={loadError}
            >
              Retry load
            </button>
          )}
          {/* Sovereign Air-Gap Toggle */}
          <button
            onClick={() =>
              setSettings({
                ...settings,
                air_gapped: !settings.air_gapped,
              })
            }
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 ${
              settings.air_gapped
                ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
                : 'bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border-[var(--color-border)] text-[var(--color-label-secondary)]'
            }`}
          >
            {settings.air_gapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
                <span>Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-[var(--color-danger-ink)]" />
                <span>Air-Gap: OFF (Cloud Egress Allowed)</span>
              </>
            )}
          </button>

          {/* DSML tool-invocation fallback kill-switch (M1-5). KILLED =
              model prose never becomes tool calls; ON (default) keeps
              the dialect for gateways that need it. Persists to the
              daemon via Save — the toggle alone changes nothing
              server-side. */}
          <button
            onClick={() =>
              setSettings({
                ...settings,
                disable_dsml_fallback: !settings.disable_dsml_fallback,
              })
            }
            aria-label="Toggle DSML tool-invocation fallback"
            title="When on, model text is never parsed as tool calls. Save to apply."
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 ${
              settings.disable_dsml_fallback
                ? 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]'
                : 'bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border-[var(--color-border)] text-[var(--color-label-secondary)]'
            }`}
          >
            <ShieldAlert className="w-3.5 h-3.5 text-[var(--color-warning-ink)]" />
            <span>DSML fallback: {settings.disable_dsml_fallback ? 'KILLED' : 'ON'}</span>
          </button>

          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold rounded-xl flex items-center space-x-2 shadow-sm active:scale-95 disabled:opacity-50 transition-all"
          >
            {saving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Database className="w-3.5 h-3.5" />}
            <span>Save Settings</span>
          </button>
        </div>
      </div>

      {saveStatus && (
        <div role={saveSectionError ? 'alert' : undefined} data-testid="llm-save-status" className="p-3 bg-[var(--color-tile)] border border-[var(--color-border)] text-xs font-medium rounded-xl text-[var(--color-label-secondary)]">
          {saveStatus}
        </div>
      )}
      {writeSettlementLabel(settlement.write) && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">{writeSettlementLabel(settlement.write)}</p>
      )}

      {/* Main Grid: Providers vs Routing */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Providers (live) */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono flex items-center space-x-2">
              <Cpu className="w-4 h-4 text-[var(--color-info-ink)]" />
              <span>Providers</span>
            </h3>
            <button
              onClick={loadModels}
              className="text-[12px] text-[var(--color-muted)] hover:text-[var(--color-info-ink)] flex items-center space-x-1 transition-colors"
            >
              <RefreshCw className="w-3 h-3" />
              <span>Refresh Models</span>
            </button>
          </div>

          {Object.entries(settings.providers).map(([pt, pc]) => (
            <div key={pt} className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3.5">
              <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
                <div className="flex items-center space-x-2">
                  <span className={`w-2 h-2 rounded-full ${pc.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--color-faint)]'}`} />
                  <span className="font-semibold text-xs text-[var(--color-text)]">{pc.name}</span>
                  {pc.is_local && (
                    <span className="text-[12px] font-mono text-[var(--color-success-ink)] bg-[var(--color-success-wash)] px-1.5 py-0.5 rounded-full border border-[var(--color-success-edge)]">LOCAL</span>
                  )}
                  {!pc.is_local && (
                    <span className="text-[12px] font-mono text-[var(--color-accent-ink)] bg-[var(--color-soft-indigo-wash)] px-1.5 py-0.5 rounded-full border border-[var(--color-soft-indigo-edge)]">CLOUD</span>
                  )}
                </div>
                <button
                  onClick={() => setProvider(pt, { enabled: !pc.enabled })}
                  aria-label={`Toggle ${pc.name}`}
                  className={`text-[12px] font-mono px-2 py-0.5 rounded-full border transition-all ${
                    pc.enabled
                      ? 'text-[var(--color-success-ink)] bg-[var(--color-success-wash)] border-[var(--color-success-edge)]'
                      : 'text-[var(--color-muted)] bg-[var(--color-tile)] border-[var(--color-border)] hover:text-[var(--color-label-secondary)]'
                  }`}
                >
                  {pc.enabled ? (pt === 'omlx' ? 'ACTIVE :8787' : 'ENABLED') : 'OFF'}
                </button>
              </div>

              <div className="space-y-2.5 text-xs">
                <div className="flex justify-between items-center text-[var(--color-muted)] font-mono text-[12px]">
                  <span>Base URL:</span>
                  <span className="text-[var(--color-label-secondary)] truncate max-w-[220px]">{pc.base_url}</span>
                </div>
                <div className="flex justify-between items-center gap-2 text-[var(--color-muted)] font-mono text-[12px]">
                  <label htmlFor={`llm-default-model-${pt}`} className="shrink-0" title="Empty clears the default — calls then fail naming the variable to set">Default model:</label>
                  <input
                    id={`llm-default-model-${pt}`}
                    value={pc.default_model || ''}
                    onChange={(e) => setProvider(pt, { default_model: e.target.value.trim() })}
                    placeholder="(unset — configure per .env or here)"
                    aria-label={`${pc.name} default model`}
                    className="flex-1 min-w-0 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                  />
                </div>
                <div className="flex justify-between items-center text-[var(--color-muted)] font-mono text-[12px]">
                  <span>Last success:</span>
                  <span className="text-[var(--color-label-secondary)]" title="Process-lifetime signal: last completed call via this provider. Unknown until the next success after a restart.">
                    {pc.last_success_at ? new Date(pc.last_success_at).toLocaleString() : 'never here'}
                  </span>
                </div>
                {!pc.is_local && (
                  <div>
                    <label htmlFor={`llm-api-key-${pt}`} className="block text-[var(--color-muted)] mb-1 text-[12px]">
                      API key {pc.enabled ? <span className="text-[var(--color-success-ink)] font-mono">(stored ●●●)</span> : <span className="text-[var(--color-warning-ink)] font-mono">(not set)</span>}
                      <span className="block text-[12px] text-[var(--color-muted)]">Prefer the daemon env/Bao in production; UI-entered keys are persisted in the local ledger.</span>
                    </label>
                    <input
                      id={`llm-api-key-${pt}`}
                      type="password"
                      value={keyEdits[pt] ?? ''}
                      placeholder={pc.enabled ? 'Stored — type to rotate' : 'Paste key…'}
                      onChange={(e) => setKeyEdits((prev) => ({ ...prev, [pt]: e.target.value }))}
                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2.5 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-accent-edge)] focus:outline-none"
                    />
                  </div>
                )}
                {saveSectionError?.provider === pt && (
                  <div role="alert" data-testid={`llm-save-error-${pt}`} className="p-2 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
                    Save refused (provider {pc.name}): {saveSectionError.message} — nothing changed.
                  </div>
                )}
              </div>
            </div>
          ))}

          <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3.5">
            <div className="text-xs font-semibold text-[var(--color-text)]">Generation defaults</div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="llm-temperature" className="block text-[var(--color-muted)] text-[12px] mb-1">Temperature:</label>
                <input
                  id="llm-temperature"
                  type="number"
                  step="0.05"
                  min="0"
                  max="1"
                  value={settings.temperature}
                  onChange={(e) => {
                    const v = parseFloat(e.target.value);
                    if (!Number.isFinite(v)) {
                      setTokenError('Temperature must be a finite number — change rejected.');
                      return;
                    }
                    setTokenError(null);
                    setSettings({ ...settings, temperature: v });
                  }}
                  className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                />
              </div>
              <div>
                <label htmlFor="llm-max-tokens" className="block text-[var(--color-muted)] text-[12px] mb-1">Max tokens (0 = uncapped):</label>
                <input
                  id="llm-max-tokens"
                  type="number"
                  step="128"
                  min="0"
                  value={settings.max_tokens}
                  onChange={(e) => {
                    const raw = e.target.value.trim();
                    if (raw === '') {
                      setTokenError('Token cap empty — enter 0 for uncapped or a finite duration.');
                      return;
                    }
                    const v = parseInt(raw, 10);
                    // Reject NaN: empty/garbage input must not silently become 0.
                    if (!Number.isFinite(v)) {
                      setTokenError(`Token cap rejected: "${raw}" is not a number — nothing changed.`);
                      return;
                    }
                    setTokenError(null);
                    setSettings({ ...settings, max_tokens: Math.max(0, v) });
                  }}
                  className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                />
              </div>
            </div>
            {tokenError && (
              <div className="p-2 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
                {tokenError}
              </div>
            )}
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label htmlFor="llm-active-provider" className="block text-[var(--color-muted)] mb-1 text-[12px]">Active provider:</label>
                <select
                  id="llm-active-provider"
                  value={settings.active_provider}
                  onChange={(e) =>
                    setSettings({ ...settings, active_provider: e.target.value })
                  }
                  aria-label="Active provider"
                  className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2.5 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                >
                  {Object.entries(settings.providers).map(([pt, pc]) => (
                    <option key={pt} value={pt}>
                      {pt}{pc.enabled ? '' : ' (off)'}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="llm-active-model" className="block text-[var(--color-muted)] mb-1 text-[12px]">Active model (used when a role has no assignment):</label>
                <input
                  id="llm-active-model"
                  value={settings.active_model}
                  onChange={(e) =>
                    setSettings({ ...settings, active_model: e.target.value.trim() })
                  }
                  list="kubona-model-options"
                  placeholder="(unset — resolve fails naming the variable)"
                  aria-label="Active model"
                  className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2.5 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                />
              </div>
            </div>
            <datalist id="kubona-model-options">
              {modelOptions(settings.active_model).map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
          </div>
        </div>

        {/* Right Column: Agent routing */}
        <div className="space-y-4">
          <h3 className="text-xs font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono flex items-center space-x-2">
            <Globe className="w-4 h-4 text-[var(--color-accent-ink)]" />
            <span>Agent Routing {settings.air_gapped ? '(air-gapped: local only)' : ''}</span>
          </h3>

          {settings.air_gapped && (
            <div className="text-[12px] text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] p-2.5 rounded-xl flex items-center space-x-2">
              <AlertCircle className="w-3.5 h-3.5 shrink-0 text-[var(--color-warning-ink)]" />
              <span>Cloud egress blocked while Sovereign Air-Gap is engaged. Roles on cloud providers will refuse.</span>
            </div>
          )}
          {!settings.air_gapped && (
            <div className="text-[12px] text-[var(--color-muted)] bg-[var(--color-tile)] border border-[var(--color-border)] p-2.5 rounded-xl">
              Local providers (Strata, oMLX, Ollama, LM Studio) work without a cloud opt-in. In production,
              cloud providers additionally require the deployment flag <span className="font-mono text-[var(--color-text)]">KUBONA_LLM_CLOUD=1</span>.
            </div>
          )}

          {/* Role Routing Matrix (editable) */}
          <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3">
            <h4 className="text-xs font-bold text-[var(--color-label-secondary)] uppercase tracking-wide font-mono">
              Agent → Provider : Model
            </h4>
            <div className="space-y-2 text-xs">
              {Object.entries(settings.role_assignments).map(([role, assign]) => (
                <div key={role} className="p-2.5 bg-[var(--color-tile)] rounded-xl border border-[var(--color-border)] space-y-2">
                  <div className="font-mono text-[var(--color-info-ink)] font-semibold">{role}</div>
                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <label htmlFor={`llm-role-provider-${role}`} className="sr-only">{role} provider</label>
                      <select
                        id={`llm-role-provider-${role}`}
                        aria-label={`${role} provider`}
                        value={assign.provider}
                        onChange={(e) => setRole(role, { provider: e.target.value })}
                        className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg p-1.5 text-[12px] text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                      >
                        {Object.entries(settings.providers).map(([pt, pc]) => (
                          <option key={pt} value={pt}>
                            {pt} · {pc.is_local ? 'local' : 'cloud'}{pc.enabled ? '' : ' (off)'}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label htmlFor={`llm-role-model-${role}`} className="sr-only">{role} model</label>
                      <input
                        id={`llm-role-model-${role}`}
                        aria-label={`${role} model`}
                        value={assign.model}
                        list={`kubona-role-models-${role}`}
                        onChange={(e) => setRole(role, { model: e.target.value.trim() })}
                        placeholder="(unset — resolve fails naming the variable)"
                        className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg p-1.5 text-[12px] text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                      />
                    </div>
                    <datalist id={`kubona-role-models-${role}`}>
                      {modelOptions(assign.model).map((m) => (
                        <option key={m} value={m} />
                      ))}
                    </datalist>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Live Inference & Latency Benchmark */}
      <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
          <div className="flex items-center space-x-2">
            <Zap className="w-4 h-4 text-[var(--color-info-ink)]" />
            <h3 className="text-xs font-bold text-[var(--color-text)] uppercase tracking-wider font-mono">
              Live Model Verification &amp; Latency Benchmark
            </h3>
          </div>
          <span className="text-[12px] font-mono text-[var(--color-info-ink)]">
            Target: {settings.active_model}
          </span>
        </div>

        <div className="space-y-3">
          <div className="flex space-x-2">
            <label htmlFor="llm-benchmark-prompt" className="sr-only">Benchmark prompt</label>
            <input
              id="llm-benchmark-prompt"
              type="text"
              value={testPrompt}
              aria-label="Benchmark prompt"
              onChange={(e) => setTestPrompt(e.target.value)}
              className="flex-1 bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3.5 py-2 text-xs text-[var(--color-text)] focus:outline-none focus:border-[var(--color-info-edge)] font-mono"
              placeholder="Enter benchmark prompt..."
            />
            <button
              onClick={handleRunTest}
              disabled={testing || !testPrompt.trim()}
              className="px-4 py-2 bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold rounded-xl flex items-center space-x-2 disabled:opacity-50 shadow-sm active:scale-95 transition-all"
            >
              {testing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
              <span>{testing ? 'Benchmarking...' : 'Test Model'}</span>
            </button>
          </div>

          {testResult && (
            <div className="p-4 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] space-y-3 animate-fadeIn">
              <div className="flex items-center justify-between text-xs font-mono border-b border-[var(--color-border)] pb-2">
                <span className={testResult.success ? 'text-[var(--color-success-ink)] flex items-center space-x-1' : 'text-[var(--color-danger-ink)] flex items-center space-x-1'}>
                  <CheckCircle2 className="w-3.5 h-3.5 mr-1" />
                  <span>{testResult.success ? 'VERIFICATION PASSED' : 'BENCHMARK FAILED'}</span>
                </span>
                <div className="flex items-center space-x-4 text-[var(--color-muted)]">
                  <span>Latency: <strong className="text-[var(--color-text)]">{typeof testResult.duration_ms === 'number' && Number.isFinite(testResult.duration_ms) ? `${testResult.duration_ms} ms` : 'UNVERIFIED (no duration measured)'}</strong></span>
                  {typeof testResult.gen_tok_per_sec === 'number' && Number.isFinite(testResult.gen_tok_per_sec) && (
                    <span>Speed: <strong className="text-[var(--color-success-ink)]">{testResult.gen_tok_per_sec.toFixed(1)} tok/s</strong></span>
                  )}
                  <span>Model: <strong className="text-[var(--color-info-ink)]">{testResult.model}</strong></span>
                </div>
              </div>

              {testResult.reply && (
                <div className="text-xs text-[var(--color-label-secondary)] whitespace-pre-wrap leading-relaxed max-h-48 overflow-y-auto font-sans">
                  {testResult.reply}
                </div>
              )}

              {testResult.error && (
                <div className="text-xs text-[var(--color-danger-ink)] font-mono">
                  {testResult.error}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
