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
  const [omlxModels, setOmlxModels] = useState<ModelInfo[]>([]);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testPrompt, setTestPrompt] = useState('Confirm you are active and describe how you optimize a VQE ansatz.');
  const [saveStatus, setSaveStatus] = useState<string>('');

  const loadModels = useCallback(async () => {
    try {
      const data = await fetchLLMModels('omlx');
      if (data && Array.isArray((data as { models?: ModelInfo[] }).models)) {
        setOmlxModels((data as { models: ModelInfo[] }).models);
      }
    } catch (err) {
      console.error('Failed to fetch oMLX models:', err);
    }
  }, []);

  useEffect(() => {
    let mounted = true;
    fetchLLMSettings()
      .then((data) => {
        if (mounted && data) setSettings(data as unknown as SettingsPayload);
      })
      .catch((err) => console.error('Failed to load LLM settings:', err));

    fetchLLMModels('omlx')
      .then((data) => {
        const models = (data as { models?: ModelInfo[] })?.models;
        if (mounted && models) setOmlxModels(models);
      })
      .catch((err) => console.error('Failed to fetch oMLX models:', err));

    return () => {
      mounted = false;
    };
  }, []);

  const handleSave = async () => {
    if (!settings) return;
    setSaving(true);
    setSaveStatus('');
    try {
      const ok = await saveLLMSettings(settings as unknown as Record<string, unknown>);
      if (ok) {
        setSaveStatus('✅ Settings persisted successfully to SQLite ledger.');
      } else {
        setSaveStatus('❌ Failed to persist settings.');
      }
    } catch (err) {
      setSaveStatus(`❌ Network error: ${err}`);
    } finally {
      setSaving(false);
    }
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
      <div className="p-12 text-center text-slate-400 text-xs flex items-center justify-center space-x-2">
        <RefreshCw className="w-4 h-4 animate-spin text-cyan-400" />
        <span>Loading Sovereign Intelligence Configuration...</span>
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

      {/* Main Grid: Local Engines vs Cloud Egress */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Local Inference Engines */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono flex items-center space-x-2">
              <Cpu className="w-4 h-4 text-cyan-400" />
              <span>Local Engines (Air-Gap Compliant)</span>
            </h3>
            <button
              onClick={loadModels}
              className="text-[11px] text-slate-400 hover:text-cyan-300 flex items-center space-x-1 transition-colors"
            >
              <RefreshCw className="w-3 h-3" />
              <span>Refresh Models</span>
            </button>
          </div>

          {/* oMLX Card */}
          <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3.5">
            <div className="flex items-center justify-between border-b border-white/[0.06] pb-3">
              <div className="flex items-center space-x-2">
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                <span className="font-semibold text-xs text-white">oMLX Local Engine (Apple Silicon)</span>
              </div>
              <span className="text-[10px] font-mono text-emerald-400 bg-emerald-950/60 px-2 py-0.5 rounded-full border border-emerald-800">
                ACTIVE :8787
              </span>
            </div>

            <div className="space-y-2.5 text-xs">
              <div className="flex justify-between items-center text-slate-400 font-mono text-[11px]">
                <span>Base URL:</span>
                <span className="text-slate-200">http://127.0.0.1:8787/v1</span>
              </div>

              <div>
                <label className="block text-slate-400 mb-1 text-[11px]">Active Model:</label>
                <select
                  value={settings.active_model}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      active_model: e.target.value,
                    })
                  }
                  className="w-full bg-black/60 border border-white/10 rounded-xl p-2.5 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                >
                  {omlxModels.length > 0 ? (
                    omlxModels.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.id} {m.id.includes('Qwen3.8-27B') ? '★ [Default Laboratory Model]' : ''}
                      </option>
                    ))
                  ) : (
                    <option value="Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit">
                      Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit (Default)
                    </option>
                  )}
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3 pt-1">
                <div>
                  <label className="block text-slate-400 text-[11px] mb-1">Temperature:</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0"
                    max="1"
                    value={settings.temperature}
                    onChange={(e) =>
                      setSettings({
                        ...settings,
                        temperature: parseFloat(e.target.value) || 0.2,
                      })
                    }
                    className="w-full bg-black/60 border border-white/10 rounded-xl p-2 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-slate-400 text-[11px] mb-1">Max Tokens:</label>
                  <input
                    type="number"
                    step="128"
                    min="256"
                    max="4096"
                    value={settings.max_tokens}
                    onChange={(e) =>
                      setSettings({
                        ...settings,
                        max_tokens: parseInt(e.target.value) || 1024,
                      })
                    }
                    className="w-full bg-black/60 border border-white/10 rounded-xl p-2 text-xs text-slate-200 font-mono focus:border-cyan-500 focus:outline-none"
                  />
                </div>
              </div>
            </div>
          </div>

          {/* Alternative Local Engines */}
          <div className="grid grid-cols-2 gap-3">
            <div className="p-3.5 rounded-2xl bg-black/40 border border-white/10 space-y-1.5 text-xs">
              <div className="font-semibold text-slate-200">Ollama Engine</div>
              <div className="text-xs text-slate-300 font-mono">127.0.0.1:11434</div>
              <div className="text-[11px] text-slate-300 font-medium">Model: qwen2.5:32b</div>
            </div>

            <div className="p-3.5 rounded-2xl bg-black/40 border border-white/10 space-y-1.5 text-xs">
              <div className="font-semibold text-slate-200">LM Studio Engine</div>
              <div className="text-xs text-slate-300 font-mono">127.0.0.1:1234</div>
              <div className="text-[11px] text-slate-300 font-medium">Model: local-model</div>
            </div>
          </div>
        </div>

        {/* Right Column: Cloud Providers & Role Routing */}
        <div className="space-y-4">
          <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono flex items-center space-x-2">
            <Globe className="w-4 h-4 text-indigo-400" />
            <span>Cloud Frontier Providers (Gated by Air-Gap)</span>
          </h3>

          <div
            className={`p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3.5 transition-opacity ${
              settings.air_gapped ? 'opacity-60 pointer-events-none' : ''
            }`}
          >
            {settings.air_gapped && (
              <div className="text-[11px] text-amber-300 bg-amber-950/40 border border-amber-800/40 p-2.5 rounded-xl flex items-center space-x-2">
                <AlertCircle className="w-3.5 h-3.5 shrink-0 text-amber-400" />
                <span>Cloud egress blocked while Sovereign Air-Gap is engaged.</span>
              </div>
            )}

            <div className="space-y-3 text-xs">
              <div>
                <label className="block text-slate-300 font-medium mb-1">Anthropic Claude API Key:</label>
                <input
                  type="password"
                  placeholder="sk-ant-..."
                  className="w-full bg-black/60 border border-white/10 rounded-xl p-2.5 text-xs text-slate-200 font-mono focus:border-indigo-500 focus:outline-none"
                  disabled={settings.air_gapped}
                />
              </div>

              <div>
                <label className="block text-slate-300 font-medium mb-1">OpenAI API Key:</label>
                <input
                  type="password"
                  placeholder="sk-..."
                  className="w-full bg-black/60 border border-white/10 rounded-xl p-2.5 text-xs text-slate-200 font-mono focus:border-indigo-500 focus:outline-none"
                  disabled={settings.air_gapped}
                />
              </div>
            </div>
          </div>

          {/* Role Routing Matrix */}
          <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
            <h4 className="text-xs font-bold text-slate-200 uppercase tracking-wide font-mono">
              Tribunal Specialist Agent Routing
            </h4>
            <div className="space-y-2 text-xs">
              {Object.entries(settings.role_assignments).map(([role, assign]) => (
                <div key={role} className="flex items-center justify-between p-2.5 bg-white/[0.03] rounded-xl border border-white/10">
                  <span className="font-mono text-cyan-300 font-semibold">{role}</span>
                  <span className="text-slate-300 text-xs font-mono truncate max-w-[200px]">
                    {assign.provider}:{assign.model}
                  </span>
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
                  <span>Latency: <strong className="text-white">{testResult.duration_ms} ms</strong></span>
                  {testResult.gen_tok_per_sec !== undefined && (
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
