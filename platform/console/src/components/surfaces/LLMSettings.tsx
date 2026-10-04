'use client';

import React, { useState, useEffect } from 'react';
import {
  Cpu,
  ShieldCheck,
  Power,
  RefreshCw,
  CheckCircle,
  AlertCircle,
  Sliders,
  Send,
  Zap,
  Lock,
  Globe,
  Database,
  Activity,
} from 'lucide-react';

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

  const loadSettings = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/llm/settings');
      if (res.ok) {
        const data = await res.json();
        setSettings(data);
      }
    } catch (err) {
      console.error('Failed to load LLM settings:', err);
    }
  };

  const loadModels = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/llm/models?provider=omlx');
      if (res.ok) {
        const data = await res.json();
        setOmlxModels(data.models || []);
      }
    } catch (err) {
      console.error('Failed to fetch oMLX models:', err);
    }
  };

  useEffect(() => {
    loadSettings();
    loadModels();
  }, []);

  const handleSave = async () => {
    if (!settings) return;
    setSaving(true);
    setSaveStatus('');
    try {
      const res = await fetch('http://localhost:8080/api/llm/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      });
      if (res.ok) {
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
      const res = await fetch('http://localhost:8080/api/llm/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          role: 'copilot',
          prompt: testPrompt,
        }),
      });
      const data = await res.json();
      setTestResult(data);
    } catch (err) {
      setTestResult({ success: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  };

  if (!settings) {
    return (
      <div className="p-8 text-center text-slate-400 text-sm flex items-center justify-center space-x-2">
        <RefreshCw className="w-4 h-4 animate-spin text-indigo-400" />
        <span>Loading Hybrid LLM Configuration Plane...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Top Banner & Sovereign Air-Gap Toggle */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 shadow-xl flex items-center justify-between">
        <div>
          <h2 className="text-base font-bold text-white flex items-center space-x-2">
            <Sliders className="w-5 h-5 text-indigo-400" />
            <span>Hybrid LLM Configuration Plane</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Manage local Apple Silicon engines (oMLX, Ollama, LM Studio), cloud models, role routing, and air-gap controls
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() =>
              setSettings({
                ...settings,
                air_gapped: !settings.air_gapped,
              })
            }
            className={`px-3 py-1.5 rounded-lg text-xs font-bold flex items-center space-x-1.5 border transition-all ${
              settings.air_gapped
                ? 'bg-emerald-950 border-emerald-800 text-emerald-300'
                : 'bg-red-950 border-red-800 text-red-300'
            }`}
          >
            {settings.air_gapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Sovereign Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-red-400" />
                <span>Air-Gap: OPEN (Cloud Egress Allowed)</span>
              </>
            )}
          </button>

          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-semibold rounded-lg flex items-center space-x-1.5 shadow disabled:opacity-50 transition-colors"
          >
            {saving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Database className="w-3.5 h-3.5" />}
            <span>Save Configuration</span>
          </button>
        </div>
      </div>

      {saveStatus && (
        <div className="p-3 bg-slate-900 border border-slate-800 text-xs font-medium rounded-lg text-slate-200">
          {saveStatus}
        </div>
      )}

      {/* Main Grid: Local Engines vs Cloud Engines */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Local Inference Engines */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider flex items-center space-x-1.5">
              <Cpu className="w-4 h-4 text-cyan-400" />
              <span>Local Zero-Config Engines (Air-Gap Compliant)</span>
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
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-3">
            <div className="flex items-center justify-between border-b border-slate-800 pb-2.5">
              <div className="flex items-center space-x-2">
                <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse"></span>
                <span className="font-semibold text-xs text-white">oMLX Local Engine (Apple Silicon)</span>
              </div>
              <span className="text-[10px] font-mono text-emerald-400 bg-emerald-950/60 px-2 py-0.5 rounded border border-emerald-800">
                ACTIVE :8787
              </span>
            </div>

            <div className="space-y-2 text-xs">
              <div className="flex justify-between items-center text-slate-400">
                <span>Base URL:</span>
                <span className="font-mono text-slate-200">http://127.0.0.1:8787/v1</span>
              </div>

              <div>
                <label className="block text-slate-400 mb-1">Active Model:</label>
                <select
                  value={settings.active_model}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      active_model: e.target.value,
                    })
                  }
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg p-2 text-xs text-slate-200 font-mono focus:border-indigo-500 focus:outline-none"
                >
                  {omlxModels.length > 0 ? (
                    omlxModels.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.id} {m.id.includes('Qwen3.8-27B') ? '★ [Default Test Model]' : ''}
                      </option>
                    ))
                  ) : (
                    <option value="Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit">
                      Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit (Default)
                    </option>
                  )}
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3 pt-2">
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
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg p-1.5 text-xs text-slate-200 font-mono"
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
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg p-1.5 text-xs text-slate-200 font-mono"
                  />
                </div>
              </div>
            </div>
          </div>

          {/* Ollama & LM Studio Cards */}
          <div className="grid grid-cols-2 gap-3">
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-3.5 space-y-1.5 text-xs">
              <div className="font-semibold text-slate-200">Ollama Engine</div>
              <div className="text-[11px] text-slate-500 font-mono">127.0.0.1:11434</div>
              <div className="text-[10px] text-slate-400">Model: qwen2.5:32b</div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-xl p-3.5 space-y-1.5 text-xs">
              <div className="font-semibold text-slate-200">LM Studio Engine</div>
              <div className="text-[11px] text-slate-500 font-mono">127.0.0.1:1234</div>
              <div className="text-[10px] text-slate-400">Model: local-model</div>
            </div>
          </div>
        </div>

        {/* Right Column: Cloud Providers & Role Routing */}
        <div className="space-y-4">
          <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider flex items-center space-x-1.5">
            <Globe className="w-4 h-4 text-indigo-400" />
            <span>Cloud Frontier Providers (Gated by Air-Gap)</span>
          </h3>

          <div
            className={`bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-3 transition-opacity ${
              settings.air_gapped ? 'opacity-50 pointer-events-none' : ''
            }`}
          >
            {settings.air_gapped && (
              <div className="text-[11px] text-amber-400 bg-amber-950/40 border border-amber-800/40 p-2 rounded flex items-center space-x-1.5">
                <AlertCircle className="w-3.5 h-3.5 shrink-0" />
                <span>Cloud egress is blocked while Sovereign Air-Gap is engaged.</span>
              </div>
            )}

            <div className="space-y-3 text-xs">
              <div>
                <label className="block text-slate-300 font-medium mb-1">Anthropic Claude API Key:</label>
                <input
                  type="password"
                  placeholder="sk-ant-..."
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg p-2 text-xs text-slate-200 font-mono"
                  disabled={settings.air_gapped}
                />
              </div>

              <div>
                <label className="block text-slate-300 font-medium mb-1">OpenAI API Key:</label>
                <input
                  type="password"
                  placeholder="sk-..."
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg p-2 text-xs text-slate-200 font-mono"
                  disabled={settings.air_gapped}
                />
              </div>
            </div>
          </div>

          {/* Role Routing Matrix */}
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-3">
            <h4 className="text-xs font-bold text-slate-200 uppercase tracking-wide">
              Specialist Agent Role Routing
            </h4>
            <div className="space-y-2 text-xs">
              {Object.entries(settings.role_assignments).map(([role, assign]) => (
                <div key={role} className="flex items-center justify-between p-2 bg-slate-950 rounded-lg border border-slate-800/80">
                  <span className="font-mono text-indigo-300">{role}</span>
                  <span className="text-slate-400 text-[11px] font-mono truncate max-w-[200px]">
                    {assign.provider}:{assign.model}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Live Inference & Latency Tester */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 space-y-4 shadow-xl">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center space-x-2">
            <Zap className="w-4 h-4 text-amber-400" />
            <h3 className="text-xs font-bold text-white uppercase tracking-wider">
              Live Model Verification &amp; Latency Benchmark
            </h3>
          </div>
          <span className="text-[11px] font-mono text-cyan-400">
            Target: {settings.active_model}
          </span>
        </div>

        <div className="space-y-3">
          <div className="flex space-x-2">
            <input
              type="text"
              value={testPrompt}
              onChange={(e) => setTestPrompt(e.target.value)}
              className="flex-1 bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 font-mono"
              placeholder="Enter benchmark prompt..."
            />
            <button
              onClick={handleRunTest}
              disabled={testing || !testPrompt.trim()}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-semibold rounded-lg flex items-center space-x-1.5 disabled:opacity-50 shadow transition-colors"
            >
              {testing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
              <span>{testing ? 'Benchmarking...' : 'Test Model'}</span>
            </button>
          </div>

          {testResult && (
            <div className="bg-slate-950 border border-slate-800 rounded-lg p-4 space-y-3">
              <div className="flex items-center justify-between text-xs font-mono border-b border-slate-800/80 pb-2">
                <span className={testResult.success ? 'text-emerald-400' : 'text-red-400'}>
                  {testResult.success ? '● VERIFICATION PASSED' : '● BENCHMARK FAILED'}
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
                <div className="text-xs text-slate-300 whitespace-pre-wrap leading-relaxed max-h-48 overflow-y-auto">
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
