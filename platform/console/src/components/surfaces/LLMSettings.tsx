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
} from 'lucide-react';
import {
  fetchLLMSettings,
  saveLLMSettings,
  fetchLLMModels,
  testLLM,
} from '@/lib/api';

interface ProviderConfig {
  type: string;
  name: string;
  base_url: string;
  api_key?: string;
  default_model: string;
  enabled: boolean;
  is_local: boolean;
  timeout_sec: number;
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

export function LLMSettings() {
  const [settings, setSettings] = useState<SettingsPayload | null>(null);
  const [baseline, setBaseline] = useState<string | null>(null);
  const [omlxModels, setOmlxModels] = useState<ModelInfo[]>([]);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testPrompt, setTestPrompt] = useState('Confirm you are active and describe how you optimize a VQE ansatz.');
  const [saveStatus, setSaveStatus] = useState<string>('');
  const [loadError, setLoadError] = useState<string | null>(null);
  const [tokenError, setTokenError] = useState<string | null>(null);
  // Key edits live outside settings state: secrets are never rendered and
  // only sent when the operator types them. Empty provider keys in the save
  // payload mean "keep stored" (backend preserves non-empty secrets).
  const [keyEdits, setKeyEdits] = useState<Record<string, string>>({});
  // Unsaved marker: dirty when current JSON differs from last loaded/saved.
  const dirty = settings != null && baseline != null && JSON.stringify(settings) !== baseline;

  const loadModels = useCallback(async () => {
    try {
      const data = await fetchLLMModels('omlx');
      if (data && Array.isArray((data as { models?: ModelInfo[] }).models)) {
        setOmlxModels((data as { models: ModelInfo[] }).models);
      }
    } catch (err) {
      setLoadError(`Models unreachable: ${err instanceof Error ? err.message : String(err)}`);
    }
  }, []);

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

    fetchLLMModels('omlx')
      .then((data) => {
        const models = (data as { models?: ModelInfo[] })?.models;
        if (models) setOmlxModels(models);
      })
      .catch((err) => setLoadError(`Failed to fetch oMLX models: ${err instanceof Error ? err.message : String(err)}`));
  }, []);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  const handleSave = async () => {
    if (!settings) return;
    setSaving(true);
    setSaveStatus('');
    try {
      // Merge key edits; untouched providers send "" (backend keeps stored).
      const providers: Record<string, ProviderConfig> = {};
      for (const [pt, pc] of Object.entries(settings.providers)) {
        providers[pt] = { ...pc, api_key: keyEdits[pt] ?? '' };
      }
      const res = await saveLLMSettings({ ...settings, providers } as unknown as Record<string, unknown>);
      if (res.ok) {
        setSaveStatus('✅ Settings persisted successfully to SQLite ledger.');
        setKeyEdits({});
        setBaseline(JSON.stringify({ ...settings, providers }));
      } else {
        setSaveStatus(`❌ Failed to persist settings — backend refused (${res.error || 'no reason'}). Nothing changed.`);
      }
    } catch (err) {
      setSaveStatus(`❌ Network error: ${err instanceof Error ? err.message : String(err)} — nothing changed.`);
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
            <div className="text-rose-300 text-xs">Settings load failed — {loadError} Nothing loaded.</div>
            <button
              onClick={() => { setLoadError(null); loadAll(); }}
              className="px-4 py-1.5 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] text-white text-xs font-semibold"
            >
              Retry load
            </button>
          </>
        ) : (
          <div className="text-slate-400 text-xs flex items-center justify-center space-x-2">
            <RefreshCw className="w-4 h-4 animate-spin text-cyan-400" />
            <span>Loading Sovereign Intelligence Configuration...</span>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6 select-none">
      {/* Serene Header & Save Button */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-cyan-500/10 border border-cyan-500/20 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Sliders className="w-5 h-5 text-cyan-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center space-x-2">
              <span>Sovereign Intelligence Configuration</span>
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Local Apple Silicon Neural Engine (oMLX, Ollama), cloud models, and air-gap controls
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          {dirty && (
            <span className="px-2.5 py-1 rounded-full text-[10px] font-mono bg-amber-500/20 text-amber-300 border border-amber-500/40">
              Unsaved changes
            </span>
          )}
          {loadError && (
            <button
              onClick={() => { setLoadError(null); loadAll(); }}
              className="px-3 py-1.5 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] text-slate-300 text-xs font-semibold"
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
                ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300 ring-1 ring-emerald-500/20'
                : 'bg-white/[0.04] hover:bg-white/[0.08] border-white/10 text-slate-300'
            }`}
          >
            {settings.air_gapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-red-400" />
                <span>Air-Gap: OFF (Cloud Egress Allowed)</span>
              </>
            )}
          </button>

          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl flex items-center space-x-2 shadow-md active:scale-95 disabled:opacity-50 transition-all"
          >
            {saving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Database className="w-3.5 h-3.5" />}
            <span>Save Settings</span>
          </button>
        </div>
      </div>

      {saveStatus && (
        <div className="p-3 bg-white/[0.03] border border-white/10 text-xs font-medium rounded-xl text-slate-200">
          {saveStatus}
        </div>
      )}

      {/* Main Grid: Providers vs Routing */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Providers (live) */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono flex items-center space-x-2">
              <Cpu className="w-4 h-4 text-cyan-400" />
              <span>Providers</span>
            </h3>
            <button
              onClick={loadModels}
              className="text-[11px] text-slate-400 hover:text-cyan-300 flex items-center space-x-1 transition-colors"
            >
              <RefreshCw className="w-3 h-3" />
              <span>Refresh Models</span>
            </button>
          </div>

          {Object.entries(settings.providers).map(([pt, pc]) => (
            <div key={pt} className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3.5">
              <div className="flex items-center justify-between border-b border-white/[0.06] pb-3">
                <div className="flex items-center space-x-2">
                  <span className={`w-2 h-2 rounded-full ${pc.enabled ? 'bg-emerald-400 animate-pulse' : 'bg-slate-600'}`} />
                  <span className="font-semibold text-xs text-white">{pc.name}</span>
                  {pc.is_local && (
                    <span className="text-[9px] font-mono text-emerald-300 bg-emerald-950/60 px-1.5 py-0.5 rounded-full border border-emerald-800">LOCAL</span>
                  )}
                  {!pc.is_local && (
                    <span className="text-[9px] font-mono text-indigo-300 bg-indigo-950/60 px-1.5 py-0.5 rounded-full border border-indigo-800">CLOUD</span>
                  )}
                </div>
                <button
                  onClick={() => setProvider(pt, { enabled: !pc.enabled })}
                  aria-label={`Toggle ${pc.name}`}
                  className={`text-[10px] font-mono px-2 py-0.5 rounded-full border transition-all ${
                    pc.enabled
                      ? 'text-emerald-400 bg-emerald-950/60 border-emerald-800'
                      : 'text-slate-500 bg-white/[0.03] border-white/10 hover:text-slate-300'
                  }`}
                >
                  {pc.enabled ? (pt === 'omlx' ? 'ACTIVE :8787' : 'ENABLED') : 'OFF'}
                </button>
              </div>

              <div className="space-y-2.5 text-xs">
                <div className="flex justify-between items-center text-slate-400 font-mono text-[11px]">
                  <span>Base URL:</span>
                  <span className="text-slate-200 truncate max-w-[220px]">{pc.base_url}</span>
                </div>
                <div className="flex justify-between items-center text-slate-400 font-mono text-[11px]">
                  <span>Default model:</span>
                  <span className="text-slate-200 truncate max-w-[220px]">{pc.default_model}</span>
                </div>
                {!pc.is_local && (
                  <div>
                    <label className="block text-slate-400 mb-1 text-[11px]">
                      API key {pc.api_key ? <span className="text-emerald-400 font-mono">(stored ●●●)</span> : <span className="text-amber-400 font-mono">(not set)</span>}
                    </label>
                    <input
                      type="password"
                      value={keyEdits[pt] ?? ''}
                      placeholder={pc.api_key ? 'Stored — type to rotate' : 'Paste key…'}
                      onChange={(e) => setKeyEdits((prev) => ({ ...prev, [pt]: e.target.value }))}
                      className="w-full bg-black/60 border border-white/10 rounded-xl p-2.5 text-xs text-slate-200 font-mono focus:border-indigo-500 focus:outline-none"
                    />
                  </div>
                )}
              </div>
            </div>
          ))}

          <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3.5">
            <div className="text-xs font-semibold text-white">Generation defaults</div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-slate-400 text-[11px] mb-1">Temperature:</label>
                <input
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
                  className="w-full bg-black/60 border border-white/10 rounded-xl p-2 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-slate-400 text-[11px] mb-1">Max tokens (0 = uncapped):</label>
                <input
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
                  className="w-full bg-black/60 border border-white/10 rounded-xl p-2 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                />
              </div>
            </div>
            {tokenError && (
              <div className="p-2 rounded-lg bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
                {tokenError}
              </div>
            )}
            <div>
              <label className="block text-slate-400 mb-1 text-[11px]">Active model (used when a role has no assignment):</label>
              <select
                value={settings.active_model}
                onChange={(e) =>
                  setSettings({ ...settings, active_model: e.target.value })
                }
                className="w-full bg-black/60 border border-white/10 rounded-xl p-2.5 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
              >
                {modelOptions(settings.active_model).map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
            </div>
          </div>
        </div>

        {/* Right Column: Agent routing */}
        <div className="space-y-4">
          <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono flex items-center space-x-2">
            <Globe className="w-4 h-4 text-indigo-400" />
            <span>Agent Routing {settings.air_gapped ? '(air-gapped: local only)' : ''}</span>
          </h3>

          {settings.air_gapped && (
            <div className="text-[11px] text-amber-300 bg-amber-950/40 border border-amber-800/40 p-2.5 rounded-xl flex items-center space-x-2">
              <AlertCircle className="w-3.5 h-3.5 shrink-0 text-amber-400" />
              <span>Cloud egress blocked while Sovereign Air-Gap is engaged. Roles on cloud providers will refuse.</span>
            </div>
          )}

          {/* Role Routing Matrix (editable) */}
          <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
            <h4 className="text-xs font-bold text-slate-200 uppercase tracking-wide font-mono">
              Agent → Provider : Model
            </h4>
            <div className="space-y-2 text-xs">
              {Object.entries(settings.role_assignments).map(([role, assign]) => (
                <div key={role} className="p-2.5 bg-white/[0.03] rounded-xl border border-white/10 space-y-2">
                  <div className="font-mono text-cyan-300 font-semibold">{role}</div>
                  <div className="grid grid-cols-2 gap-2">
                    <select
                      aria-label={`${role} provider`}
                      value={assign.provider}
                      onChange={(e) => setRole(role, { provider: e.target.value })}
                      className="bg-black/60 border border-white/10 rounded-lg p-1.5 text-[11px] text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                    >
                      {Object.entries(settings.providers).map(([pt, pc]) => (
                        <option key={pt} value={pt}>
                          {pt}{pc.enabled ? '' : ' (off)'}
                        </option>
                      ))}
                    </select>
                    <select
                      aria-label={`${role} model`}
                      value={assign.model}
                      onChange={(e) => setRole(role, { model: e.target.value })}
                      className="bg-black/60 border border-white/10 rounded-lg p-1.5 text-[11px] text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                    >
                      {modelOptions(assign.model).map((m) => (
                        <option key={m} value={m}>{m}</option>
                      ))}
                    </select>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Live Inference & Latency Benchmark */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-4">
        <div className="flex items-center justify-between border-b border-white/[0.06] pb-3">
          <div className="flex items-center space-x-2">
            <Zap className="w-4 h-4 text-cyan-400" />
            <h3 className="text-xs font-bold text-white uppercase tracking-wider font-mono">
              Live Model Verification &amp; Latency Benchmark
            </h3>
          </div>
          <span className="text-[11px] font-mono text-cyan-300">
            Target: {settings.active_model}
          </span>
        </div>

        <div className="space-y-3">
          <div className="flex space-x-2">
            <input
              type="text"
              value={testPrompt}
              onChange={(e) => setTestPrompt(e.target.value)}
              className="flex-1 bg-black/60 border border-white/10 rounded-xl px-3.5 py-2 text-xs text-slate-200 focus:outline-none focus:border-cyan-500 font-mono"
              placeholder="Enter benchmark prompt..."
            />
            <button
              onClick={handleRunTest}
              disabled={testing || !testPrompt.trim()}
              className="px-4 py-2 bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl flex items-center space-x-2 disabled:opacity-50 shadow-md active:scale-95 transition-all"
            >
              {testing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
              <span>{testing ? 'Benchmarking...' : 'Test Model'}</span>
            </button>
          </div>

          {testResult && (
            <div className="p-4 rounded-xl bg-black/60 border border-white/[0.08] space-y-3 animate-fadeIn">
              <div className="flex items-center justify-between text-xs font-mono border-b border-white/[0.06] pb-2">
                <span className={testResult.success ? 'text-emerald-400 flex items-center space-x-1' : 'text-red-400'}>
                  <CheckCircle2 className="w-3.5 h-3.5 mr-1" />
                  <span>{testResult.success ? 'VERIFICATION PASSED' : 'BENCHMARK FAILED'}</span>
                </span>
                <div className="flex items-center space-x-4 text-slate-400">
                  <span>Latency: <strong className="text-white">{typeof testResult.duration_ms === 'number' && Number.isFinite(testResult.duration_ms) ? `${testResult.duration_ms} ms` : 'UNVERIFIED (no duration measured)'}</strong></span>
                  {typeof testResult.gen_tok_per_sec === 'number' && Number.isFinite(testResult.gen_tok_per_sec) && (
                    <span>Speed: <strong className="text-emerald-400">{testResult.gen_tok_per_sec.toFixed(1)} tok/s</strong></span>
                  )}
                  <span>Model: <strong className="text-cyan-300">{testResult.model}</strong></span>
                </div>
              </div>

              {testResult.reply && (
                <div className="text-xs text-slate-300 whitespace-pre-wrap leading-relaxed max-h-48 overflow-y-auto font-sans">
                  {testResult.reply}
                </div>
              )}

              {testResult.error && (
                <div className="text-xs text-red-400 font-mono">
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
