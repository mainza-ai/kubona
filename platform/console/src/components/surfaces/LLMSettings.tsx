'use client';

import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import {
  Cpu,
  RefreshCw,
  Sliders,
  Send,
  Zap,
  Globe,
  Database,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ShieldAlert,
  ArrowUp,
  ArrowDown,
  Plus,
  Trash2,
  Radar,
} from 'lucide-react';
import {
  clearLLMProviderKey,
  discoverLocalLLMProviders,
  fetchLLMSettings,
  saveLLMSettings,
  fetchLLMModels,
  testLLM,
  verifyLLMProvider,
  type DiscoveredLocalServer,
  type VerifyProviderResult,
} from '@/lib/api';
import { subscribeInvalidation } from '@/domain/invalidation';
import { AirGapToggleButton } from '@/components/common/AirGapControl';
import { BudgetSummaryStrip } from '@/components/common/BudgetSummaryStrip';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

interface ProviderConfig {
  type: string;
  name: string;
  base_url: string;
  api_key?: string;
  default_model: string;
  wire?: string;
  enabled: boolean;
  is_local: boolean;
  timeout_sec: number;
  last_success_at?: string;
  has_key?: boolean;
  secret_source?: 'env' | 'sealed_db' | 'plaintext_db' | 'none' | string;
  key_status?: 'missing' | 'set' | 'valid' | 'invalid' | string;
  last_fault?: string;
  last_checked_at?: string;
  clear_key?: boolean;
}

interface ModelTarget {
  provider: string;
  model: string;
}

interface RoleModelAssignment {
  role: string;
  provider: string;
  model: string;
  fallback?: ModelTarget[];
  temperature?: number;
  top_p?: number;
  seed?: number;
  max_tokens?: number;
  reasoning_effort?: string;
}

interface RoleMeta {
  role: string;
  group: 'interactive' | 'tribunal' | 'autonomous' | string;
  label: string;
  description: string;
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
  role_catalog?: RoleMeta[];
}

interface ModelCapabilities {
  vision?: boolean;
  tools?: boolean;
  reasoning?: boolean;
  structured_output?: boolean;
  streaming?: boolean;
}

interface ModelInfo {
  id: string;
  owned_by?: string;
  max_model_len?: number;
  context_window?: number;
  max_output_tokens?: number;
  provider: string;
  is_default?: boolean;
  capabilities?: ModelCapabilities;
  priced?: boolean;
  price_source?: string;
  input_per_1k_usd?: number;
  output_per_1k_usd?: number;
}

interface TestResult {
  success: boolean;
  provider?: string;
  wire?: string;
  reply?: string;
  model?: string;
  duration_ms?: number;
  gen_tok_per_sec?: number;
  prompt_tok_per_sec?: number;
  attempt_count?: number;
  cost_usd?: number;
  error?: string;
}

const DEFAULT_ROLE_META: RoleMeta[] = [
  { role: 'copilot', group: 'interactive', label: 'Copilot', description: 'Primary interactive research assistant and command-dock chat' },
  { role: 'codesign', group: 'interactive', label: 'Co-Design', description: 'Bounded quantum-circuit co-design proposal and hardware tradeoffs' },
  { role: 'ansatz-scout', group: 'tribunal', label: 'Ansatz Scout', description: 'Proposes candidate Hamiltonians, variational circuits, and experiments' },
  { role: 'disconfirmation-officer', group: 'tribunal', label: 'Disconfirmation Officer', description: 'Adversarially critiques proposals and hunts counter-evidence' },
  { role: 'device-auditor', group: 'tribunal', label: 'Device Auditor', description: 'Verifies hardware topology, noise constraints, and calibration telemetry' },
  { role: 'epistemic-scribe', group: 'tribunal', label: 'Epistemic Scribe', description: 'Synthesizes tribunal evidence into consensus and claim records' },
  { role: 'coder', group: 'autonomous', label: 'Autoresearch Coder', description: 'Generates bounded code and circuit mutations in autoresearch loops' },
  { role: 'critic', group: 'autonomous', label: 'Autoresearch Critic', description: 'Evaluates candidate diffs and rejects speculative regressions' },
  { role: 'supervisor', group: 'autonomous', label: 'Meta-Search Supervisor', description: 'Guides outer meta-search evolution strategies across stalled frontiers' },
];

const ROLE_GROUP_HEADERS: Array<{ key: string; title: string }> = [
  { key: 'interactive', title: 'Interactive Assistant' },
  { key: 'tribunal', title: 'Research Tribunal' },
  { key: 'autonomous', title: 'Autonomous Loops' },
];

const DEFAULT_PROVIDER_URLS: Record<string, string> = {
  strata: 'http://127.0.0.1:8085/v1',
  omlx: 'http://127.0.0.1:8787/v1',
  ollama: 'http://127.0.0.1:11434',
  lmstudio: 'http://127.0.0.1:1234/v1',
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com',
  gemini: 'https://generativelanguage.googleapis.com/v1beta',
  opencode: 'https://opencode.ai/zen/v1',
  openrouter: 'https://openrouter.ai/api/v1',
  nvidia: 'https://integrate.api.nvidia.com/v1',
  custom: 'http://127.0.0.1:8000/v1',
};

const WIRE_OPTIONS = [
  { value: '', label: 'auto (provider default)' },
  { value: 'openai-chat', label: 'openai-chat (/chat/completions)' },
  { value: 'openai-responses', label: 'openai-responses (/responses)' },
  { value: 'anthropic-messages', label: 'anthropic-messages (/v1/messages)' },
  { value: 'gemini-generate', label: 'gemini-generate (:generateContent)' },
  { value: 'ollama-chat', label: 'ollama-chat (/api/chat)' },
];

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
  if (lower.includes('temperature') || lower.includes('top_p') || lower.includes('max_token') || lower.includes('max token') || lower.includes('reasoning')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('active_model') || lower.includes('active model')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('active_provider') || lower.includes('active provider')) {
    return { section: 'generation defaults', message: reason };
  }
  if (lower.includes('role') || lower.includes('routing') || lower.includes('assign') || lower.includes('fallback')) {
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

function providerStandingBadge(pc: ProviderConfig): { label: string; toneClass: string } | null {
  if (pc.key_status === 'valid') {
    return {
      label: 'KEY VALID',
      toneClass: 'text-[var(--color-success-ink)] bg-[var(--color-success-wash)] border-[var(--color-success-edge)]',
    };
  }
  if (pc.key_status === 'invalid') {
    return {
      label: `KEY INVALID${pc.last_fault && pc.last_fault !== 'none' ? ` · ${pc.last_fault}` : ''}`,
      toneClass: 'text-[var(--color-danger-ink)] bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)]',
    };
  }
  if (!pc.is_local && pc.key_status === 'set') {
    return {
      label: 'KEY SET (UNVERIFIED)',
      toneClass: 'text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)]',
    };
  }
  if (!pc.is_local && (pc.key_status === 'missing' || pc.has_key === false)) {
    return {
      label: 'KEY MISSING',
      toneClass: 'text-[var(--color-muted)] bg-[var(--color-tile)] border-[var(--color-border)]',
    };
  }
  if (pc.is_local && pc.last_fault && pc.last_fault !== 'none') {
    return {
      label: pc.last_fault.toUpperCase(),
      toneClass: 'text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)]',
    };
  }
  return null;
}

export function LLMSettings() {
  const [settings, setSettings] = useState<SettingsPayload | null>(null);
  const [baseline, setBaseline] = useState<string | null>(null);
  const [omlxModels, setOmlxModels] = useState<ModelInfo[]>([]);
  const [modelErrors, setModelErrors] = useState<Record<string, string>>({});
  const [clearingKeyFor, setClearingKeyFor] = useState<string | null>(null);
  const [verifyingFor, setVerifyingFor] = useState<string | null>(null);
  const [verifyResults, setVerifyResults] = useState<Record<string, VerifyProviderResult>>({});
  const [verifyErrors, setVerifyErrors] = useState<Record<string, string>>({});
  const verifyGenRef = useRef<Record<string, number>>({});

  const [discovering, setDiscovering] = useState(false);
  const [discoveredServers, setDiscoveredServers] = useState<DiscoveredLocalServer[] | null>(null);
  const [discoveredPorts, setDiscoveredPorts] = useState<number[]>([]);
  const [discoverError, setDiscoverError] = useState<string | null>(null);

  const [expandedRoles, setExpandedRoles] = useState<Record<string, boolean>>({});
  const [profileStatus, setProfileStatus] = useState<string | null>(null);

  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testPrompt, setTestPrompt] = useState('Confirm you are active and describe how you optimize a VQE ansatz.');
  const [benchProvider, setBenchProvider] = useState('');
  const [benchModel, setBenchModel] = useState('');

  const [saveStatus, setSaveStatus] = useState<string>('');
  const [saveSectionError, setSaveSectionError] = useState<{ section: string; provider?: string; message: string } | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const settlement = useWriteSettlement();
  const [tokenError, setTokenError] = useState<string | null>(null);
  const [keyEdits, setKeyEdits] = useState<Record<string, string>>({});
  const dirty = settings != null && baseline != null && JSON.stringify(settings) !== baseline;

  const modelLookup = useMemo(() => {
    const map = new Map<string, ModelInfo>();
    for (const m of omlxModels) {
      map.set(`${m.provider}:${m.id}`, m);
    }
    return map;
  }, [omlxModels]);

  const activeProvider = settings?.active_provider;
  const settingsProviders = settings?.providers;
  const loadModels = useCallback(() => {
    const providerSet = new Set<string>(['omlx']);
    if (activeProvider) providerSet.add(activeProvider);
    if (settingsProviders) {
      for (const [pt, pc] of Object.entries(settingsProviders)) {
        if (pc.enabled) providerSet.add(pt);
      }
    }
    const providers = Array.from(providerSet);
    const lists = providers.map((p) =>
      fetchLLMModels(p).then(
        (data) => {
          const models = Array.isArray((data as { models?: ModelInfo[] })?.models)
            ? (data as { models: ModelInfo[] }).models.map((m) => ({ ...m, provider: m.provider || p }))
            : [];
          const err = typeof (data as { error?: unknown })?.error === 'string'
            ? String((data as { error: string }).error).trim()
            : '';
          return { provider: p, models, error: err };
        },
        (err) => ({ provider: p, models: [] as ModelInfo[], error: err instanceof Error ? err.message : String(err) }),
      ),
    );
    void Promise.all(lists).then((results) => {
      const all: ModelInfo[] = [];
      const errs: Record<string, string> = {};
      for (const r of results) {
        if (r.error) errs[r.provider] = r.error;
        for (const m of r.models) {
          if (!all.some((x) => x.id === m.id && x.provider === m.provider)) {
            all.push(m);
          }
        }
      }
      setModelErrors(errs);
      setOmlxModels(all);
    });
  }, [activeProvider, settingsProviders]);

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

  useEffect(() => {
    return subscribeInvalidation(['llm'], () => {
      if (!dirty) {
        loadAll();
      }
    });
  }, [dirty, loadAll]);

  const handleSave = async () => {
    if (!settings) return;
    setSaving(true);
    setSaveStatus('');
    setSaveSectionError(null);
    const requestId = settlement.begin('LLM settings');
    try {
      const providers: Record<string, ProviderConfig> = {};
      for (const [pt, pc] of Object.entries(settings.providers)) {
        providers[pt] = { ...pc, api_key: keyEdits[pt] ?? '' };
      }
      const res = await saveLLMSettings({ ...settings, providers } as unknown as Record<string, unknown>);
      if (res.ok) {
        settlement.applied(requestId, 'LLM settings persisted');
        setSaveStatus('✅ Settings persisted successfully to SQLite ledger.');
        setKeyEdits({});
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

  const handleClearKey = async (pt: string) => {
    setClearingKeyFor(pt);
    setSaveSectionError(null);
    const requestId = settlement.begin(`clear key (${pt})`);
    try {
      const res = await clearLLMProviderKey(pt);
      if (res.ok) {
        settlement.applied(requestId, `removed key for ${pt}`);
        setKeyEdits((prev) => {
          const next = { ...prev };
          delete next[pt];
          return next;
        });
        if (res.settings) {
          setSettings(res.settings as unknown as SettingsPayload);
          setBaseline(JSON.stringify(res.settings));
        } else {
          loadAll();
        }
        setSaveStatus(`Removed stored API key for ${pt}.`);
      } else {
        const reason = res.error || 'clear refused';
        settlement.refused(requestId, reason);
        setSaveSectionError({ section: `provider ${pt}`, provider: pt, message: reason });
        setSaveStatus(`❌ Failed to remove key for ${pt}: ${reason}`);
      }
    } finally {
      setClearingKeyFor(null);
    }
  };

  const handleVerifyProvider = async (pt: string) => {
    if (!settings) return;
    const pc = settings.providers[pt];
    if (!pc) return;
    const nextGen = (verifyGenRef.current[pt] ?? 0) + 1;
    verifyGenRef.current[pt] = nextGen;
    setVerifyingFor(pt);
    setVerifyErrors((prev) => ({ ...prev, [pt]: '' }));
    try {
      const transientKey = (keyEdits[pt] ?? '').trim();
      const res = await verifyLLMProvider(pt, {
        api_key: transientKey || undefined,
        base_url: pc.base_url || undefined,
        wire: pc.wire || undefined,
        check_generation: nextGen,
      });
      if (verifyGenRef.current[pt] !== nextGen) {
        return;
      }
      if (res.verification) {
        setVerifyResults((prev) => ({ ...prev, [pt]: res.verification! }));
        if (res.verification.models && res.verification.models.length > 0) {
          const discovered: ModelInfo[] = res.verification.models
            .map((raw) => {
              const m = raw as Partial<ModelInfo>;
              return {
                ...m,
                id: String(m.id ?? ''),
                provider: String(m.provider || pt),
              };
            })
            .filter((m) => m.id !== '');
          setOmlxModels((prev) => {
            const retained = prev.filter((x) => x.provider !== pt);
            return [...retained, ...discovered];
          });
        }
        if (res.settings) {
          const serverSnap = res.settings as unknown as SettingsPayload;
          if (!dirty) {
            setSettings(serverSnap);
            setBaseline(JSON.stringify(serverSnap));
          } else {
            const updatedProv = serverSnap.providers?.[pt];
            if (updatedProv) {
              setSettings((prev) =>
                prev
                  ? {
                      ...prev,
                      providers: {
                        ...prev.providers,
                        [pt]: {
                          ...prev.providers[pt],
                          key_status: updatedProv.key_status,
                          last_fault: updatedProv.last_fault,
                          last_checked_at: updatedProv.last_checked_at,
                        },
                      },
                    }
                  : prev,
              );
            }
          }
        }
      } else {
        setVerifyErrors((prev) => ({ ...prev, [pt]: res.error || 'Verification refused' }));
      }
    } finally {
      if (verifyGenRef.current[pt] === nextGen) {
        setVerifyingFor((cur) => (cur === pt ? null : cur));
      }
    }
  };

  const handleDiscoverLocal = async () => {
    setDiscovering(true);
    setDiscoverError(null);
    try {
      const res = await discoverLocalLLMProviders();
      if (res.error) {
        setDiscoverError(res.error);
      }
      setDiscoveredServers(res.servers ?? []);
      setDiscoveredPorts(res.ports_scanned ?? []);
    } catch (err) {
      setDiscoverError(err instanceof Error ? err.message : String(err));
    } finally {
      setDiscovering(false);
    }
  };

  const handleAttachDiscovered = (server: DiscoveredLocalServer) => {
    if (!settings) return;
    const candidateType =
      server.provider === 'ollama' || server.provider === 'lmstudio' || server.provider === 'omlx'
        ? server.provider
        : 'custom';
    const targetType = settings.providers[candidateType] ? candidateType : 'custom';
    const prevCfg = settings.providers[targetType];
    if (!prevCfg) return;
    const firstModel = server.models?.[0] || prevCfg.default_model;
    setSettings((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        providers: {
          ...prev.providers,
          [targetType]: {
            ...prev.providers[targetType],
            base_url: server.base_url,
            enabled: true,
            wire: server.wire || prev.providers[targetType].wire,
            default_model: firstModel || prev.providers[targetType].default_model,
          },
        },
      };
    });
    if (server.models && server.models.length > 0) {
      setOmlxModels((prev) => {
        const retained = prev.filter((x) => x.provider !== targetType);
        return [
          ...retained,
          ...server.models.map((id) => ({
            id,
            max_model_len: server.context_window,
            provider: targetType,
          })),
        ];
      });
    }
    setProfileStatus(
      `Staged ${server.provider} (${server.base_url}) onto provider slot "${targetType}". Click Save Settings to persist.`,
    );
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

  const moveFallbackHop = (role: string, index: number, delta: -1 | 1) => {
    const assign = settings?.role_assignments[role];
    if (!assign?.fallback) return;
    const targetIdx = index + delta;
    if (targetIdx < 0 || targetIdx >= assign.fallback.length) return;
    const copy = [...assign.fallback];
    const [item] = copy.splice(index, 1);
    copy.splice(targetIdx, 0, item);
    setRole(role, { fallback: copy });
  };

  const removeFallbackHop = (role: string, index: number) => {
    const assign = settings?.role_assignments[role];
    if (!assign?.fallback) return;
    const copy = assign.fallback.filter((_, idx) => idx !== index);
    setRole(role, { fallback: copy.length > 0 ? copy : undefined });
  };

  const addFallbackHop = (role: string) => {
    if (!settings) return;
    const assign = settings.role_assignments[role];
    const existing = assign?.fallback ?? [];
    const fallbackProv =
      Object.entries(settings.providers).find(([pt, pc]) => pc.enabled && pt !== assign?.provider)?.[0] ||
      settings.active_provider ||
      'omlx';
    const fallbackModel =
      settings.providers[fallbackProv]?.default_model || settings.active_model || '';
    setRole(role, {
      fallback: [...existing, { provider: fallbackProv, model: fallbackModel }],
    });
  };

  const fillUnassignedRoles = () => {
    if (!settings) return;
    const targetModel = settings.active_model || settings.providers[settings.active_provider]?.default_model || '';
    if (!targetModel) {
      setProfileStatus('Cannot fill unassigned roles: Active model is empty.');
      return;
    }
    let count = 0;
    const nextRoles: Record<string, RoleModelAssignment> = { ...settings.role_assignments };
    for (const [role, assign] of Object.entries(nextRoles)) {
      if (!assign.model) {
        nextRoles[role] = { ...assign, provider: settings.active_provider, model: targetModel };
        count += 1;
      }
    }
    setSettings({ ...settings, role_assignments: nextRoles });
    setProfileStatus(
      count > 0
        ? `Filled ${count} unassigned role(s) with ${settings.active_provider}:${targetModel}. Click Save Settings to persist.`
        : 'All roles already have explicit model assignments.',
    );
  };

  const applyRoutingProfile = (mode: 'sovereign' | 'hybrid' | 'frontier') => {
    if (!settings) return;
    const enabledLocals = ['omlx', 'ollama', 'lmstudio', 'strata', 'custom'].filter(
      (pt) => settings.providers[pt]?.enabled && settings.providers[pt]?.is_local,
    );
    const localProv = enabledLocals[0] || 'omlx';
    const localModel =
      settings.providers[localProv]?.default_model ||
      omlxModels.find((m) => m.provider === localProv)?.id ||
      (settings.providers[settings.active_provider]?.is_local ? settings.active_model : '') ||
      '';

    const cloudCandidates = ['anthropic', 'openai', 'openrouter', 'gemini', 'nvidia', 'opencode'].filter(
      (pt) => settings.providers[pt]?.enabled && !settings.providers[pt]?.is_local && settings.providers[pt]?.has_key !== false,
    );
    const cloudProv = cloudCandidates[0] || '';
    const cloudModel = cloudProv
      ? settings.providers[cloudProv]?.default_model || omlxModels.find((m) => m.provider === cloudProv)?.id || ''
      : '';

    const nextRoles: Record<string, RoleModelAssignment> = { ...settings.role_assignments };

    if (mode === 'sovereign') {
      if (!localModel) {
        setProfileStatus(`Staged Sovereign Local profile requires a model id on ${localProv}.`);
        return;
      }
      for (const [role, assign] of Object.entries(nextRoles)) {
        nextRoles[role] = {
          ...assign,
          provider: localProv,
          model: localModel,
        };
      }
      setSettings({
        ...settings,
        air_gapped: true,
        active_provider: localProv,
        active_model: localModel,
        role_assignments: nextRoles,
      });
      setProfileStatus(`Applied Sovereign Local profile (${localProv}:${localModel}, Air-Gap ON). Save Settings to persist.`);
      return;
    }

    if (!cloudProv || !cloudModel) {
      setProfileStatus('Hybrid / High-Rigor profile requires at least one enabled cloud provider with an API key and default model.');
      return;
    }

    if (mode === 'hybrid') {
      for (const [role, assign] of Object.entries(nextRoles)) {
        const useLocal = (role === 'epistemic-scribe' || role === 'critic') && Boolean(localModel);
        const primaryProv = useLocal ? localProv : cloudProv;
        const primaryModel = useLocal ? localModel : cloudModel;
        const fallback =
          !useLocal && localModel
            ? [{ provider: localProv, model: localModel }]
            : assign.fallback;
        nextRoles[role] = {
          ...assign,
          provider: primaryProv,
          model: primaryModel,
          fallback,
        };
      }
      setSettings({
        ...settings,
        active_provider: cloudProv,
        active_model: cloudModel,
        role_assignments: nextRoles,
      });
      setProfileStatus(`Applied Cost-Aware Hybrid profile (frontier ${cloudProv}, local scribe/critic ${localProv || 'none'}). Save Settings to persist.`);
      return;
    }

    // High-Rigor Frontier
    for (const [role, assign] of Object.entries(nextRoles)) {
      const highRigor = role === 'ansatz-scout' || role === 'disconfirmation-officer' || role === 'device-auditor' || role === 'supervisor';
      const deterministic = role === 'device-auditor' || role === 'critic';
      nextRoles[role] = {
        ...assign,
        provider: cloudProv,
        model: cloudModel,
        reasoning_effort: highRigor ? 'high' : assign.reasoning_effort,
        temperature: deterministic ? 0.2 : assign.temperature,
      };
    }
    setSettings({
      ...settings,
      active_provider: cloudProv,
      active_model: cloudModel,
      role_assignments: nextRoles,
    });
    setProfileStatus(`Applied High-Rigor Frontier profile (${cloudProv}:${cloudModel}). Save Settings to persist.`);
  };

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
      const data = await testLLM(testPrompt, 'copilot', {
        provider: benchProvider || undefined,
        model: benchModel.trim() || undefined,
      });
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

  const monoModelWarning = useMemo(() => {
    if (!settings) return null;
    const scout = settings.role_assignments['ansatz-scout'];
    const officer = settings.role_assignments['disconfirmation-officer'];
    const scoutProv = scout?.provider || settings.active_provider;
    const scoutModel = scout?.model || settings.active_model;
    const offProv = officer?.provider || settings.active_provider;
    const offModel = officer?.model || settings.active_model;
    if (scoutProv && scoutModel && scoutProv === offProv && scoutModel === offModel) {
      return `Mono-model tribunal advisory: Ansatz Scout and Disconfirmation Officer share ${scoutProv}:${scoutModel}. Consider assigning distinct models or providers to reduce correlated blind spots.`;
    }
    return null;
  }, [settings]);

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

  const effectiveBenchTarget = benchModel.trim() || settings.active_model || '(role default)';

  return (
    <div className="relative rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border)] shadow-xl p-7 space-y-6 select-none">
      {/* Header & Primary Controls */}
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
              Local Apple Silicon Neural Engine (oMLX, Ollama), cloud models, ordered role failover, and air-gap controls
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
          <AirGapToggleButton
            value={settings.air_gapped}
            offLabel="Air-Gap: OFF (Cloud Egress Allowed)"
            onToggle={() =>
              setSettings({
                ...settings,
                air_gapped: !settings.air_gapped,
              })
            }
          />

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

      {/* Server-Sealed Spend Telemetry */}
      <BudgetSummaryStrip autoFetch />

      {/* Local Runtime Discovery Strip */}
      <div className="p-4 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center space-x-2">
            <Radar className="w-4 h-4 text-[var(--color-info-ink)]" />
            <h3 className="text-xs font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono">
              Detected Local Servers
            </h3>
            {discoveredPorts.length > 0 && (
              <span className="text-[11px] font-mono text-[var(--color-muted)]">
                (ports {discoveredPorts.join(', ')})
              </span>
            )}
          </div>
          <button
            type="button"
            disabled={discovering}
            onClick={() => void handleDiscoverLocal()}
            className="inline-flex items-center gap-1.5 rounded-xl border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] px-3 py-1 text-[12px] font-mono text-[var(--color-info-ink)] hover:opacity-90 disabled:opacity-50"
          >
            <RefreshCw className={`w-3 h-3 ${discovering ? 'animate-spin' : ''}`} />
            <span>{discovering ? 'Scanning local ports…' : 'Scan local servers'}</span>
          </button>
        </div>

        {discoverError && (
          <div className="p-2 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs">
            Discovery error: {discoverError}
          </div>
        )}

        {discoveredServers !== null && discoveredServers.length === 0 && !discoverError && (
          <p className="text-xs text-[var(--color-muted)] font-mono">
            No local servers answered on ports {discoveredPorts.join(', ')}.
          </p>
        )}

        {discoveredServers && discoveredServers.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {discoveredServers.map((srv) => (
              <div
                key={`${srv.provider}-${srv.base_url}`}
                data-testid={`discovered-server-${srv.provider}`}
                className="p-3 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] space-y-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="h-2 w-2 rounded-full bg-[var(--color-success)]" />
                      <span className="font-mono text-xs font-semibold text-[var(--color-text)]">{srv.name || srv.provider}</span>
                      <span className="font-mono text-[11px] text-[var(--color-muted)]">:{srv.port} · {srv.wire}</span>
                      {srv.context_window ? (
                        <span className="font-mono text-[11px] text-[var(--color-info-ink)]">
                          {Math.round(srv.context_window / 1000)}K ctx
                        </span>
                      ) : null}
                    </div>
                    <div className="font-mono text-[11px] text-[var(--color-label-secondary)] truncate">{srv.base_url}</div>
                  </div>
                  <button
                    type="button"
                    onClick={() => handleAttachDiscovered(srv)}
                    className="shrink-0 rounded-lg border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-2.5 py-1 font-mono text-[11px] text-[var(--color-accent-ink)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)]"
                  >
                    Attach
                  </button>
                </div>
                {srv.models.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {srv.models.slice(0, 4).map((modelId) => (
                      <span
                        key={modelId}
                        className="rounded border border-[var(--color-border)] bg-[var(--color-surface)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-label-secondary)]"
                      >
                        {modelId}
                      </span>
                    ))}
                  </div>
                )}
                {srv.context_warning && (
                  <div
                    role="status"
                    className="flex items-start gap-1.5 rounded-lg border border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] px-2 py-1.5 text-[11px] text-[var(--color-warning-ink)]"
                  >
                    <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
                    <span>{srv.context_warning}</span>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Main Grid: Providers vs Routing */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Providers */}
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

          {Object.entries(settings.providers).map(([pt, pc]) => {
            const standing = providerStandingBadge(pc);
            const vres = verifyResults[pt];
            const verr = verifyErrors[pt];
            const defaultURL = DEFAULT_PROVIDER_URLS[pt];
            const urlChangedFromDefault = Boolean(defaultURL && pc.base_url !== defaultURL);

            return (
              <div key={pt} className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3.5">
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--color-border)] pb-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={`w-2 h-2 rounded-full ${pc.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--color-faint)]'}`} />
                    <span className="font-semibold text-xs text-[var(--color-text)]">{pc.name}</span>
                    {pc.is_local ? (
                      <span className="text-[12px] font-mono text-[var(--color-success-ink)] bg-[var(--color-success-wash)] px-1.5 py-0.5 rounded-full border border-[var(--color-success-edge)]">LOCAL</span>
                    ) : (
                      <span className="text-[12px] font-mono text-[var(--color-accent-ink)] bg-[var(--color-soft-indigo-wash)] px-1.5 py-0.5 rounded-full border border-[var(--color-soft-indigo-edge)]">CLOUD</span>
                    )}
                    {standing && (
                      <span className={`text-[11px] font-mono px-1.5 py-0.5 rounded-full border ${standing.toneClass}`}>
                        {standing.label}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      disabled={verifyingFor === pt}
                      aria-label={`Verify ${pc.name}`}
                      onClick={() => void handleVerifyProvider(pt)}
                      className="text-[11px] font-mono px-2 py-0.5 rounded-full border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] text-[var(--color-info-ink)] hover:opacity-90 disabled:opacity-50"
                    >
                      {verifyingFor === pt ? 'Checking…' : pc.is_local ? 'Test Server' : 'Test Key'}
                    </button>
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
                </div>

                <div className="space-y-2.5 text-xs">
                  <div className="flex justify-between items-center gap-2 text-[var(--color-muted)] font-mono text-[12px]">
                    <label htmlFor={`llm-base-url-${pt}`} className="shrink-0">Base URL:</label>
                    <div className="flex flex-1 min-w-0 items-center gap-1.5">
                      <input
                        id={`llm-base-url-${pt}`}
                        aria-label={`${pc.name} base URL`}
                        value={pc.base_url || ''}
                        onChange={(e) => setProvider(pt, { base_url: e.target.value })}
                        placeholder={defaultURL || 'https://...'}
                        className="flex-1 min-w-0 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                      />
                      {urlChangedFromDefault && (
                        <button
                          type="button"
                          onClick={() => setProvider(pt, { base_url: defaultURL })}
                          className="shrink-0 text-[11px] font-mono text-[var(--color-muted)] hover:text-[var(--color-text)] underline"
                        >
                          Reset
                        </button>
                      )}
                    </div>
                  </div>

                  <div className="flex justify-between items-center gap-2 text-[var(--color-muted)] font-mono text-[12px]">
                    <label htmlFor={`llm-wire-${pt}`} className="shrink-0">Wire:</label>
                    <select
                      id={`llm-wire-${pt}`}
                      aria-label={`${pc.name} wire protocol`}
                      value={pc.wire || ''}
                      onChange={(e) => setProvider(pt, { wire: e.target.value })}
                      className="flex-1 min-w-0 bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[12px] text-[var(--color-text)] font-mono focus:border-[var(--color-info-edge)] focus:outline-none"
                    >
                      {WIRE_OPTIONS.map((w) => (
                        <option key={w.value} value={w.value}>
                          {w.label}
                        </option>
                      ))}
                    </select>
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
                  {!pc.is_local && (() => {
                    const keyPresent = typeof pc.has_key === 'boolean' ? pc.has_key : undefined;
                    const src = pc.secret_source;
                    return (
                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between gap-2">
                          <label htmlFor={`llm-api-key-${pt}`} className="block text-[var(--color-muted)] text-[12px]">
                            API key{' '}
                            {keyPresent === true ? (
                              src === 'plaintext_db' ? (
                                <span className="text-[var(--color-warning-ink)] font-mono">(unsealed DB ●●●)</span>
                              ) : src === 'env' ? (
                                <span className="text-[var(--color-success-ink)] font-mono">(env ●●●)</span>
                              ) : (
                                <span className="text-[var(--color-success-ink)] font-mono">(sealed ●●●)</span>
                              )
                            ) : keyPresent === false ? (
                              <span className="text-[var(--color-warning-ink)] font-mono">(not set)</span>
                            ) : (
                              <span className="text-[var(--color-muted)] font-mono">(status unknown)</span>
                            )}
                          </label>
                          {keyPresent === true && src !== 'env' && (
                            <button
                              type="button"
                              disabled={clearingKeyFor === pt}
                              onClick={() => void handleClearKey(pt)}
                              className="text-[11px] font-mono text-[var(--color-danger-ink)] hover:underline disabled:opacity-50"
                            >
                              {clearingKeyFor === pt ? 'Removing…' : 'Remove key'}
                            </button>
                          )}
                        </div>
                        <span className="block text-[11px] text-[var(--color-muted)]">
                          {src === 'plaintext_db'
                            ? 'Warning: stored unsealed in SQLite — set KUBONA_SECRET_KEY on daemon to encrypt at rest.'
                            : src === 'env'
                              ? 'Managed via daemon environment variable; type below only to override in local ledger.'
                              : 'Prefer daemon env or KUBONA_SECRET_KEY sealed vault in production; empty input keeps stored key.'}
                        </span>
                        <input
                          id={`llm-api-key-${pt}`}
                          type="password"
                          value={keyEdits[pt] ?? ''}
                          placeholder={keyPresent ? 'Stored — type to rotate' : 'Paste key…'}
                          onChange={(e) => setKeyEdits((prev) => ({ ...prev, [pt]: e.target.value }))}
                          className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl p-2.5 text-xs text-[var(--color-text)] font-mono focus:border-[var(--color-accent-edge)] focus:outline-none"
                        />
                      </div>
                    );
                  })()}

                  {vres && (
                    <div
                      data-testid={`llm-verify-result-${pt}`}
                      role="status"
                      className={`p-2 rounded-lg border text-[12px] font-mono ${
                        vres.ok
                          ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
                          : 'bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]'
                      }`}
                    >
                      {vres.ok
                        ? `Verified (${vres.wire}, ${vres.latency_ms ?? 0} ms, ${vres.models?.length ?? 0} models)`
                        : `Verification failed [${vres.fault || 'error'}]: ${vres.error || 'refused'}`}
                    </div>
                  )}
                  {verr && (
                    <div role="alert" className="p-2 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
                      Verify request error ({pt}): {verr}
                    </div>
                  )}
                  {modelErrors[pt] && (
                    <div role="status" data-testid={`llm-model-error-${pt}`} className="p-2 rounded-lg bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-[12px]">
                      Model discovery failed ({pt}): {modelErrors[pt]}
                    </div>
                  )}
                  {saveSectionError?.provider === pt && (
                    <div role="alert" data-testid={`llm-save-error-${pt}`} className="p-2 rounded-lg bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
                      Save refused (provider {pc.name}): {saveSectionError.message} — nothing changed.
                    </div>
                  )}
                </div>
              </div>
            );
          })}

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

        {/* Right Column: Agent Routing & Profiles */}
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-xs font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono flex items-center space-x-2">
              <Globe className="w-4 h-4 text-[var(--color-accent-ink)]" />
              <span>Agent Routing {settings.air_gapped ? '(air-gapped: local only)' : ''}</span>
            </h3>
            <div className="flex flex-wrap gap-1.5">
              <button
                type="button"
                onClick={fillUnassignedRoles}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-tile)] px-2 py-1 text-[11px] font-mono text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]"
              >
                Fill unassigned roles
              </button>
              <button
                type="button"
                onClick={() => applyRoutingProfile('sovereign')}
                className="rounded-lg border border-[var(--color-success-edge)] bg-[var(--color-success-wash)] px-2 py-1 text-[11px] font-mono text-[var(--color-success-ink)] hover:opacity-90"
              >
                Sovereign Local
              </button>
              <button
                type="button"
                onClick={() => applyRoutingProfile('hybrid')}
                className="rounded-lg border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] px-2 py-1 text-[11px] font-mono text-[var(--color-info-ink)] hover:opacity-90"
              >
                Cost-Aware Hybrid
              </button>
              <button
                type="button"
                onClick={() => applyRoutingProfile('frontier')}
                className="rounded-lg border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-2 py-1 text-[11px] font-mono text-[var(--color-accent-ink)] hover:opacity-90"
              >
                High-Rigor Frontier
              </button>
            </div>
          </div>

          {profileStatus && (
            <div role="status" className="text-[12px] font-mono text-[var(--color-info-ink)] bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] p-2.5 rounded-xl">
              {profileStatus}
            </div>
          )}

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

          {monoModelWarning && (
            <div
              data-testid="mono-model-tribunal-warning"
              role="status"
              className="text-[12px] text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] p-2.5 rounded-xl flex items-start gap-2"
            >
              <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5 text-[var(--color-warning-ink)]" />
              <span>{monoModelWarning}</span>
            </div>
          )}

          {/* Role Routing Matrix */}
          <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
            <h4 className="text-xs font-bold text-[var(--color-label-secondary)] uppercase tracking-wide font-mono">
              Agent → Provider : Model (Fallbacks &amp; Sampling)
            </h4>
            {(() => {
              const catalog = settings.role_catalog && settings.role_catalog.length > 0
                ? settings.role_catalog
                : DEFAULT_ROLE_META;
              const metaByRole = new Map(catalog.map((m) => [m.role, m]));
              const assignedKeys = Object.keys(settings.role_assignments);
              const grouped = [
                ...ROLE_GROUP_HEADERS.map((g) => ({
                  ...g,
                  roles: catalog.filter((m) => m.group === g.key && assignedKeys.includes(m.role)).map((m) => m.role),
                })),
                {
                  key: 'other',
                  title: 'Additional Roles',
                  roles: assignedKeys.filter((r) => !metaByRole.has(r)),
                },
              ].filter((g) => g.roles.length > 0);

              return (
                <div className="space-y-4 text-xs">
                  {grouped.map((group) => (
                    <div key={group.key} className="space-y-2">
                      <div className="text-[11px] font-mono uppercase tracking-wider text-[var(--color-muted)]">
                        {group.title}
                      </div>
                      {group.roles.map((role) => {
                        const assign = settings.role_assignments[role];
                        const meta = metaByRole.get(role);
                        const isOpen = Boolean(expandedRoles[role]);
                        const resolvedProv = assign.provider || settings.active_provider;
                        const resolvedModel = assign.model || settings.active_model;
                        const provCfg = settings.providers[resolvedProv];
                        const knownModel = modelLookup.get(`${resolvedProv}:${resolvedModel}`);
                        const unpricedNonLocal = Boolean(
                          resolvedModel && provCfg && !provCfg.is_local && knownModel?.priced === false,
                        );
                        const fallbackCount = assign.fallback?.length ?? 0;

                        return (
                          <div key={role} className="p-2.5 bg-[var(--color-tile)] rounded-xl border border-[var(--color-border)] space-y-2">
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="flex flex-wrap items-center gap-2 min-w-0">
                                <span className="font-mono text-[var(--color-info-ink)] font-semibold">{role}</span>
                                {meta && (
                                  <span className="text-[11px] text-[var(--color-muted)] truncate" title={meta.description}>
                                    {meta.label}
                                  </span>
                                )}
                                {unpricedNonLocal && (
                                  <span className="inline-flex items-center gap-1 rounded-full border border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-warning-ink)]">
                                    <AlertTriangle className="h-3 w-3" />
                                    <span>Unpriced · loops blocked</span>
                                  </span>
                                )}
                              </div>
                              <button
                                type="button"
                                aria-expanded={isOpen}
                                aria-label={`Configure ${role} sampling and fallbacks`}
                                onClick={() => setExpandedRoles((prev) => ({ ...prev, [role]: !prev[role] }))}
                                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-0.5 font-mono text-[11px] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]"
                              >
                                {isOpen ? 'Hide options' : `Options${fallbackCount > 0 ? ` (${fallbackCount} fallback)` : ''}`}
                              </button>
                            </div>
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

                            {isOpen && (
                              <div className="pt-2 border-t border-[var(--color-border)] space-y-2.5">
                                <div className="grid grid-cols-2 sm:grid-cols-5 gap-2">
                                  <div>
                                    <label htmlFor={`llm-role-temp-${role}`} className="block font-mono text-[11px] text-[var(--color-muted)] mb-1">
                                      Temp (0–2)
                                    </label>
                                    <input
                                      id={`llm-role-temp-${role}`}
                                      aria-label={`${role} temperature`}
                                      type="number"
                                      min="0"
                                      max="2"
                                      step="0.05"
                                      placeholder="global"
                                      value={typeof assign.temperature === 'number' ? assign.temperature : ''}
                                      onChange={(e) => {
                                        const raw = e.target.value.trim();
                                        if (!raw) {
                                          setRole(role, { temperature: undefined });
                                          return;
                                        }
                                        const v = Number(raw);
                                        if (Number.isFinite(v)) setRole(role, { temperature: v });
                                      }}
                                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[11px] text-[var(--color-text)] font-mono"
                                    />
                                  </div>
                                  <div>
                                    <label htmlFor={`llm-role-topp-${role}`} className="block font-mono text-[11px] text-[var(--color-muted)] mb-1">
                                      Top P
                                    </label>
                                    <input
                                      id={`llm-role-topp-${role}`}
                                      aria-label={`${role} top_p`}
                                      type="number"
                                      min="0.05"
                                      max="1"
                                      step="0.05"
                                      placeholder="unset"
                                      value={typeof assign.top_p === 'number' ? assign.top_p : ''}
                                      onChange={(e) => {
                                        const raw = e.target.value.trim();
                                        if (!raw) {
                                          setRole(role, { top_p: undefined });
                                          return;
                                        }
                                        const v = Number(raw);
                                        if (Number.isFinite(v)) setRole(role, { top_p: v });
                                      }}
                                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[11px] text-[var(--color-text)] font-mono"
                                    />
                                  </div>
                                  <div>
                                    <label htmlFor={`llm-role-seed-${role}`} className="block font-mono text-[11px] text-[var(--color-muted)] mb-1">
                                      Seed
                                    </label>
                                    <input
                                      id={`llm-role-seed-${role}`}
                                      aria-label={`${role} seed`}
                                      type="number"
                                      step="1"
                                      placeholder="unset"
                                      value={typeof assign.seed === 'number' ? assign.seed : ''}
                                      onChange={(e) => {
                                        const raw = e.target.value.trim();
                                        if (!raw) {
                                          setRole(role, { seed: undefined });
                                          return;
                                        }
                                        const v = Number.parseInt(raw, 10);
                                        if (Number.isFinite(v)) setRole(role, { seed: v });
                                      }}
                                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[11px] text-[var(--color-text)] font-mono"
                                    />
                                  </div>
                                  <div>
                                    <label htmlFor={`llm-role-maxtok-${role}`} className="block font-mono text-[11px] text-[var(--color-muted)] mb-1">
                                      Max tokens
                                    </label>
                                    <input
                                      id={`llm-role-maxtok-${role}`}
                                      aria-label={`${role} max tokens`}
                                      type="number"
                                      min="0"
                                      step="128"
                                      placeholder="global"
                                      value={typeof assign.max_tokens === 'number' ? assign.max_tokens : ''}
                                      onChange={(e) => {
                                        const raw = e.target.value.trim();
                                        if (!raw) {
                                          setRole(role, { max_tokens: undefined });
                                          return;
                                        }
                                        const v = Number.parseInt(raw, 10);
                                        if (Number.isFinite(v) && v >= 0) setRole(role, { max_tokens: v });
                                      }}
                                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[11px] text-[var(--color-text)] font-mono"
                                    />
                                  </div>
                                  <div>
                                    <label htmlFor={`llm-role-effort-${role}`} className="block font-mono text-[11px] text-[var(--color-muted)] mb-1">
                                      Reasoning
                                    </label>
                                    <select
                                      id={`llm-role-effort-${role}`}
                                      aria-label={`${role} reasoning effort`}
                                      value={assign.reasoning_effort || ''}
                                      onChange={(e) => setRole(role, { reasoning_effort: e.target.value || undefined })}
                                      className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg px-2 py-1 text-[11px] text-[var(--color-text)] font-mono"
                                    >
                                      <option value="">default</option>
                                      <option value="none">none</option>
                                      <option value="low">low</option>
                                      <option value="medium">medium</option>
                                      <option value="high">high</option>
                                    </select>
                                  </div>
                                </div>

                                <div className="space-y-1.5">
                                  <div className="flex items-center justify-between">
                                    <span className="font-mono text-[11px] text-[var(--color-muted)]">
                                      Ordered Failover Chain
                                    </span>
                                    <button
                                      type="button"
                                      onClick={() => addFallbackHop(role)}
                                      className="inline-flex items-center gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-0.5 font-mono text-[11px] text-[var(--color-accent-ink)] hover:bg-[var(--color-fill-hover)]"
                                    >
                                      <Plus className="h-3 w-3" />
                                      <span>Add fallback</span>
                                    </button>
                                  </div>

                                  {(assign.fallback ?? []).map((hop, idx) => (
                                    <div key={`${role}-fb-${idx}`} className="grid grid-cols-[1fr_1fr_auto] items-center gap-1.5">
                                      <div>
                                        <label htmlFor={`llm-fb-provider-${role}-${idx}`} className="sr-only">
                                          {role} fallback {idx + 1} provider
                                        </label>
                                        <select
                                          id={`llm-fb-provider-${role}-${idx}`}
                                          aria-label={`${role} fallback ${idx + 1} provider`}
                                          value={hop.provider}
                                          onChange={(e) => {
                                            const copy = [...(assign.fallback ?? [])];
                                            copy[idx] = { ...copy[idx], provider: e.target.value };
                                            setRole(role, { fallback: copy });
                                          }}
                                          className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg p-1 text-[11px] text-[var(--color-text)] font-mono"
                                        >
                                          {Object.entries(settings.providers).map(([pt, pc]) => (
                                            <option key={pt} value={pt}>
                                              {pt}{pc.enabled ? '' : ' (off)'}
                                            </option>
                                          ))}
                                        </select>
                                      </div>
                                      <div>
                                        <label htmlFor={`llm-fb-model-${role}-${idx}`} className="sr-only">
                                          {role} fallback {idx + 1} model
                                        </label>
                                        <input
                                          id={`llm-fb-model-${role}-${idx}`}
                                          aria-label={`${role} fallback ${idx + 1} model`}
                                          value={hop.model}
                                          list={`kubona-role-models-${role}`}
                                          placeholder="fallback model id"
                                          onChange={(e) => {
                                            const copy = [...(assign.fallback ?? [])];
                                            copy[idx] = { ...copy[idx], model: e.target.value.trim() };
                                            setRole(role, { fallback: copy });
                                          }}
                                          className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-lg p-1 text-[11px] text-[var(--color-text)] font-mono"
                                        />
                                      </div>
                                      <div className="flex items-center gap-1">
                                        <button
                                          type="button"
                                          disabled={idx === 0}
                                          aria-label={`Move ${role} fallback ${idx + 1} up`}
                                          onClick={() => moveFallbackHop(role, idx, -1)}
                                          className="rounded border border-[var(--color-border)] p-1 text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30"
                                        >
                                          <ArrowUp className="h-3 w-3" />
                                        </button>
                                        <button
                                          type="button"
                                          disabled={idx === (assign.fallback?.length ?? 1) - 1}
                                          aria-label={`Move ${role} fallback ${idx + 1} down`}
                                          onClick={() => moveFallbackHop(role, idx, 1)}
                                          className="rounded border border-[var(--color-border)] p-1 text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30"
                                        >
                                          <ArrowDown className="h-3 w-3" />
                                        </button>
                                        <button
                                          type="button"
                                          aria-label={`Remove ${role} fallback ${idx + 1}`}
                                          onClick={() => removeFallbackHop(role, idx)}
                                          className="rounded border border-[var(--color-border)] p-1 text-[var(--color-danger-ink)] hover:bg-[var(--color-danger-wash)]"
                                        >
                                          <Trash2 className="h-3 w-3" />
                                        </button>
                                      </div>
                                    </div>
                                  ))}
                                </div>
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  ))}
                </div>
              );
            })()}
          </div>
        </div>
      </div>

      {/* Live Inference & Latency Benchmark */}
      <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--color-border)] pb-3">
          <div className="flex items-center space-x-2">
            <Zap className="w-4 h-4 text-[var(--color-info-ink)]" />
            <h3 className="text-xs font-bold text-[var(--color-text)] uppercase tracking-wider font-mono">
              Live Model Verification &amp; Latency Benchmark
            </h3>
          </div>
          <span className="text-[12px] font-mono text-[var(--color-info-ink)]">
            Target: {benchProvider ? `${benchProvider}:${effectiveBenchTarget}` : effectiveBenchTarget} · spends provider tokens
          </span>
        </div>

        <div className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-[180px_220px_1fr_auto] gap-2 items-center">
            <div>
              <label htmlFor="llm-benchmark-provider" className="sr-only">Benchmark provider override</label>
              <select
                id="llm-benchmark-provider"
                aria-label="Benchmark provider override"
                value={benchProvider}
                onChange={(e) => setBenchProvider(e.target.value)}
                className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3 py-2 text-xs text-[var(--color-text)] font-mono focus:outline-none focus:border-[var(--color-info-edge)]"
              >
                <option value="">Use copilot role default</option>
                {Object.entries(settings.providers).map(([pt, pc]) => (
                  <option key={pt} value={pt}>
                    {pt}{pc.enabled ? '' : ' (off)'}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="llm-benchmark-model" className="sr-only">Benchmark model override</label>
              <input
                id="llm-benchmark-model"
                type="text"
                aria-label="Benchmark model override"
                value={benchModel}
                onChange={(e) => setBenchModel(e.target.value)}
                list="kubona-model-options"
                placeholder="Model override (optional)"
                className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3 py-2 text-xs text-[var(--color-text)] font-mono focus:outline-none focus:border-[var(--color-info-edge)]"
              />
            </div>
            <div>
              <label htmlFor="llm-benchmark-prompt" className="sr-only">Benchmark prompt</label>
              <input
                id="llm-benchmark-prompt"
                type="text"
                value={testPrompt}
                aria-label="Benchmark prompt"
                onChange={(e) => setTestPrompt(e.target.value)}
                className="w-full bg-[var(--color-field)] border border-[var(--color-border)] rounded-xl px-3.5 py-2 text-xs text-[var(--color-text)] focus:outline-none focus:border-[var(--color-info-edge)] font-mono"
                placeholder="Enter benchmark prompt..."
              />
            </div>
            <button
              onClick={handleRunTest}
              disabled={testing || !testPrompt.trim()}
              className="px-4 py-2 bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold rounded-xl flex items-center justify-center space-x-2 disabled:opacity-50 shadow-sm active:scale-95 transition-all"
            >
              {testing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
              <span>{testing ? 'Benchmarking...' : 'Test Model'}</span>
            </button>
          </div>

          {testResult && (
            <div className="p-4 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] space-y-3 animate-fadeIn">
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs font-mono border-b border-[var(--color-border)] pb-2">
                <span className={testResult.success ? 'text-[var(--color-success-ink)] flex items-center space-x-1' : 'text-[var(--color-danger-ink)] flex items-center space-x-1'}>
                  <CheckCircle2 className="w-3.5 h-3.5 mr-1" />
                  <span>{testResult.success ? 'VERIFICATION PASSED' : 'BENCHMARK FAILED'}</span>
                </span>
                <div className="flex flex-wrap items-center gap-3 text-[var(--color-muted)]">
                  <span>Latency: <strong className="text-[var(--color-text)]">{typeof testResult.duration_ms === 'number' && Number.isFinite(testResult.duration_ms) ? `${testResult.duration_ms} ms` : 'UNVERIFIED (no duration measured)'}</strong></span>
                  {typeof testResult.gen_tok_per_sec === 'number' && Number.isFinite(testResult.gen_tok_per_sec) && (
                    <span>Speed: <strong className="text-[var(--color-success-ink)]">{testResult.gen_tok_per_sec.toFixed(1)} tok/s</strong></span>
                  )}
                  {testResult.provider && (
                    <span>Provider: <strong className="text-[var(--color-text)]">{testResult.provider}</strong></span>
                  )}
                  {testResult.wire && (
                    <span>Wire: <strong className="text-[var(--color-text)]">{testResult.wire}</strong></span>
                  )}
                  {typeof testResult.attempt_count === 'number' && (
                    <span>Attempts: <strong className="text-[var(--color-text)]">{testResult.attempt_count}</strong></span>
                  )}
                  {typeof testResult.cost_usd === 'number' && (
                    <span>Est: <strong className="text-[var(--color-text)]">{`$${testResult.cost_usd.toFixed(5)}`}</strong></span>
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
