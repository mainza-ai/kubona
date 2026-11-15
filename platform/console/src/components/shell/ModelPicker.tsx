'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Cpu, Search, SlidersHorizontal, Star, X } from 'lucide-react';
import { fetchLLMEvalStatus, type LLMEvalStatus } from '@/lib/api';

export interface PickerModelCapabilities {
  vision?: boolean;
  tools?: boolean;
  reasoning?: boolean;
  structured_output?: boolean;
  streaming?: boolean;
}

export interface PickerModelItem {
  id: string;
  owned_by?: string;
  max_model_len?: number;
  context_window?: number;
  max_output_tokens?: number;
  provider?: string;
  is_default?: boolean;
  capabilities?: PickerModelCapabilities;
  priced?: boolean;
  price_source?: string;
  input_per_1k_usd?: number;
  output_per_1k_usd?: number;
  cached_input_per_1k_usd?: number;
  cache_write_per_1k_usd?: number;
}

export interface PickerProviderConfig {
  type: string;
  name?: string;
  base_url?: string;
  default_model?: string;
  wire?: string;
  enabled: boolean;
  is_local: boolean;
  has_key?: boolean;
  secret_source?: string;
  key_status?: string;
  last_fault?: string;
}

export interface PickerRoleSampling {
  temperature?: number;
  seed?: number;
  reasoning_effort?: string;
}

export interface ModelPickerProps {
  activeProvider: string;
  activeModel: string | null;
  airGapped?: boolean;
  providers: Record<string, PickerProviderConfig>;
  modelsByProvider: Record<string, PickerModelItem[]>;
  errorsByProvider?: Record<string, string | null>;
  loadingProvider?: string | null;
  saving?: boolean;
  statusMessage?: string | null;
  syncMatchingRoles: boolean;
  copilotSampling?: PickerRoleSampling;
  onToggleSyncMatchingRoles: () => void;
  onSelectProviderTab: (provider: string) => void;
  onSelectModel: (provider: string, modelId: string, sampling?: PickerRoleSampling) => Promise<void> | void;
  onApplySampling?: (sampling: PickerRoleSampling) => Promise<void> | void;
  onClose: () => void;
  onOpenSettings?: () => void;
  onOpenDownloadHub?: () => void;
  restoreFocusEl?: HTMLElement | null;
}

const FAVORITES_KEY = 'kubona.model_picker.favorites.v1';
const RECENT_KEY = 'kubona.model_picker.recent.v1';

type ModelPin = { provider: string; model: string };

function readPins(storageKey: string): ModelPin[] {
  try {
    if (typeof window === 'undefined' || !window.localStorage) return [];
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (item): item is ModelPin =>
        Boolean(item) &&
        typeof item === 'object' &&
        typeof (item as ModelPin).provider === 'string' &&
        typeof (item as ModelPin).model === 'string',
    );
  } catch {
    return [];
  }
}

function writePins(storageKey: string, pins: ModelPin[]): void {
  try {
    if (typeof window === 'undefined' || !window.localStorage) return;
    window.localStorage.setItem(storageKey, JSON.stringify(pins.slice(0, 12)));
  } catch {
    // Ignore quota / sandbox restrictions cleanly.
  }
}

function standingDotClass(provider: PickerProviderConfig): string {
  if (!provider.enabled) return 'bg-faint';
  if (provider.key_status === 'invalid' || (provider.last_fault && provider.last_fault !== 'none')) {
    return 'bg-danger';
  }
  if (!provider.is_local && provider.has_key === false) {
    return 'bg-faint';
  }
  if (provider.key_status === 'valid' || provider.is_local) {
    return 'bg-success';
  }
  return 'bg-warning';
}

function formatContextBadge(item: PickerModelItem): string | null {
  const ctx = item.context_window || item.max_model_len || 0;
  if (ctx <= 0) return null;
  if (ctx >= 1000000) return `${(ctx / 1000000).toFixed(1).replace(/\.0$/, '')}M ctx`;
  if (ctx >= 1000) return `${Math.round(ctx / 1000)}K ctx`;
  return `${ctx} ctx`;
}

function formatPriceBadge(item: PickerModelItem, isLocal: boolean): { label: string; warn: boolean; title?: string } | null {
  if (isLocal) {
    return { label: 'local $0', warn: false, title: 'Zero metered API spend on local runtime' };
  }
  if (item.priced === true) {
    const in1M = (item.input_per_1k_usd ?? 0) * 1000;
    const out1M = (item.output_per_1k_usd ?? 0) * 1000;
    return {
      label: `$${in1M < 1 ? in1M.toFixed(2) : in1M.toFixed(1)}/$${out1M < 1 ? out1M.toFixed(2) : out1M.toFixed(1)} /1M`,
      warn: false,
      title: `Source: ${item.price_source || 'rate-card'} ($${(item.input_per_1k_usd ?? 0).toFixed(5)} in / $${(item.output_per_1k_usd ?? 0).toFixed(5)} out per 1K tok)`,
    };
  }
  if (item.priced === false) {
    return {
      label: 'unpriced · loops blocked',
      warn: true,
      title: 'Missing from rate-card and provider pricing metadata; metered autonomous loops refuse until priced',
    };
  }
  return null;
}

export function ModelPicker({
  activeProvider,
  activeModel,
  airGapped = false,
  providers,
  modelsByProvider,
  errorsByProvider = {},
  loadingProvider = null,
  saving = false,
  statusMessage = null,
  syncMatchingRoles,
  copilotSampling,
  onToggleSyncMatchingRoles,
  onSelectProviderTab,
  onSelectModel,
  onApplySampling,
  onClose,
  onOpenSettings,
  onOpenDownloadHub,
  restoreFocusEl,
}: ModelPickerProps) {
  const defaultProvider = activeProvider || Object.keys(providers)[0] || 'omlx';
  const [tabOverride, setTabOverride] = useState<{ base: string; value: string } | null>(null);
  const selectedProvider =
    tabOverride && tabOverride.base === defaultProvider ? tabOverride.value : defaultProvider;
  const [query, setQuery] = useState('');
  const [favorites, setFavorites] = useState<ModelPin[]>(() => readPins(FAVORITES_KEY));
  const [recent, setRecent] = useState<ModelPin[]>(() => readPins(RECENT_KEY));
  const [highlightedIdx, setHighlightedIdx] = useState(0);
  const [showSampling, setShowSampling] = useState(false);
  const [temperatureInput, setTemperatureInput] = useState(
    typeof copilotSampling?.temperature === 'number' ? String(copilotSampling.temperature) : '',
  );
  const [seedInput, setSeedInput] = useState(
    typeof copilotSampling?.seed === 'number' ? String(copilotSampling.seed) : '',
  );
  const [reasoningEffort, setReasoningEffort] = useState(copilotSampling?.reasoning_effort || '');
  const [evalStatus, setEvalStatus] = useState<{ key: string; status: LLMEvalStatus } | null>(null);
  const evalKey = `${activeProvider ?? ''}|${activeModel ?? ''}`;
  const visibleEvalStatus = evalStatus?.key === evalKey ? evalStatus.status : null;

  useEffect(() => {
    if (!activeProvider || !activeModel || typeof window === 'undefined') return;
    let cancelled = false;
    fetchLLMEvalStatus('copilot', activeProvider, activeModel).then((status) => {
      if (!cancelled && status) setEvalStatus({ key: `${activeProvider}|${activeModel}`, status });
    });
    return () => {
      cancelled = true;
    };
  }, [activeProvider, activeModel]);

  const searchRef = useRef<HTMLInputElement>(null);
  const regionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  const providerEntries = useMemo<Array<[string, PickerProviderConfig]>>(() => {
    const entries = Object.entries(providers);
    if (entries.length > 0) return entries;
    return [
      [
        selectedProvider,
        {
          type: selectedProvider,
          name: selectedProvider,
          enabled: true,
          is_local: selectedProvider === 'omlx' || selectedProvider === 'strata' || selectedProvider === 'ollama' || selectedProvider === 'lmstudio',
        },
      ],
    ];
  }, [providers, selectedProvider]);

  const currentProviderCfg = providers[selectedProvider] ?? {
    type: selectedProvider,
    name: selectedProvider,
    enabled: true,
    is_local: selectedProvider === 'omlx' || selectedProvider === 'strata' || selectedProvider === 'ollama' || selectedProvider === 'lmstudio',
  };

  const rawModels = useMemo(
    () => modelsByProvider[selectedProvider] ?? [],
    [modelsByProvider, selectedProvider],
  );
  const filteredModels = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rawModels;
    return rawModels.filter(
      (m) => m.id.toLowerCase().includes(q) || (m.owned_by ?? '').toLowerCase().includes(q),
    );
  }, [query, rawModels]);

  const isPinned = (provider: string, model: string, list: ModelPin[]) =>
    list.some((p) => p.provider === provider && p.model === model);

  const pinnedForSelected = useMemo(() => {
    const seen = new Set<string>();
    const combined: Array<PickerModelItem & { pinKind: 'favorite' | 'recent' }> = [];
    const byId = new Map(rawModels.map((m) => [m.id, m]));
    const q = query.trim().toLowerCase();
    for (const fav of favorites) {
      if (fav.provider !== selectedProvider) continue;
      if (q && !fav.model.toLowerCase().includes(q)) continue;
      seen.add(fav.model);
      combined.push({ ...(byId.get(fav.model) ?? { id: fav.model }), pinKind: 'favorite' });
    }
    for (const rec of recent) {
      if (rec.provider !== selectedProvider || seen.has(rec.model)) continue;
      if (q && !rec.model.toLowerCase().includes(q)) continue;
      seen.add(rec.model);
      combined.push({ ...(byId.get(rec.model) ?? { id: rec.model }), pinKind: 'recent' });
    }
    return combined;
  }, [favorites, recent, rawModels, selectedProvider, query]);

  const parsedSampling = (): PickerRoleSampling => {
    const next: PickerRoleSampling = {};
    const tRaw = temperatureInput.trim();
    if (tRaw !== '') {
      const parsed = Number(tRaw);
      if (Number.isFinite(parsed) && parsed >= 0 && parsed <= 2) {
        next.temperature = parsed;
      }
    }
    const sRaw = seedInput.trim();
    if (sRaw !== '') {
      const parsed = Number(sRaw);
      if (Number.isInteger(parsed)) {
        next.seed = parsed;
      }
    }
    if (reasoningEffort) {
      next.reasoning_effort = reasoningEffort;
    }
    return next;
  };

  const toggleFavorite = (provider: string, model: string) => {
    const exists = isPinned(provider, model, favorites);
    const next = exists
      ? favorites.filter((p) => !(p.provider === provider && p.model === model))
      : [{ provider, model }, ...favorites];
    setFavorites(next);
    writePins(FAVORITES_KEY, next);
  };

  const pickModel = async (provider: string, modelId: string) => {
    const nextRecent = [
      { provider, model: modelId },
      ...recent.filter((p) => !(p.provider === provider && p.model === modelId)),
    ].slice(0, 6);
    setRecent(nextRecent);
    writePins(RECENT_KEY, nextRecent);
    await onSelectModel(provider, modelId, parsedSampling());
  };

  const handleClose = () => {
    onClose();
    if (restoreFocusEl && typeof restoreFocusEl.focus === 'function') {
      setTimeout(() => restoreFocusEl.focus(), 0);
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      handleClose();
      return;
    }
    if (filteredModels.length === 0) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setHighlightedIdx((idx) => (idx + 1) % filteredModels.length);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setHighlightedIdx((idx) => (idx - 1 + filteredModels.length) % filteredModels.length);
    } else if (event.key === 'Enter' && document.activeElement === searchRef.current) {
      const target = filteredModels[highlightedIdx] ?? (query.trim() ? { id: query.trim() } : null);
      if (target) {
        event.preventDefault();
        void pickModel(selectedProvider, target.id);
      }
    }
  };

  const providerError = errorsByProvider[selectedProvider];
  const providerAirGapBlocked = airGapped && !currentProviderCfg.is_local;
  const trimmedQuery = query.trim();
  const hasExactMatch = trimmedQuery !== '' && rawModels.some((m) => m.id === trimmedQuery);

  const renderModelRow = (item: PickerModelItem, idx: number, badgeHint?: string) => {
    const selected = selectedProvider === activeProvider && item.id === activeModel;
    const highlighted = idx >= 0 && idx === highlightedIdx;
    const fav = isPinned(selectedProvider, item.id, favorites);
    const ctxBadge = formatContextBadge(item);
    const priceBadge = formatPriceBadge(item, currentProviderCfg.is_local);
    const caps = item.capabilities;

    return (
      <div
        key={`${badgeHint ?? 'list'}-${item.id}`}
        className={`flex items-center justify-between gap-2 rounded-row border px-2.5 py-1.5 transition-colors ${
          selected
            ? 'border-accent-edge bg-raised shadow-[var(--row-rim)]'
            : highlighted
              ? 'border-border-strong bg-raised'
              : 'border-border bg-surface hover:bg-fill-hover'
        }`}
      >
        <button
          type="button"
          data-model-id={item.id}
          disabled={saving || providerAirGapBlocked}
          onClick={() => void pickModel(selectedProvider, item.id)}
          className="min-w-0 flex-1 text-left disabled:opacity-50"
        >
          <div className="flex flex-wrap items-center gap-1.5">
            <span className={`truncate font-code text-[12px] ${selected ? 'font-semibold text-accent-ink' : 'text-text'}`}>
              {item.id}
            </span>
            {badgeHint && (
              <span className="rounded-capsule border border-border bg-surface px-1.5 py-0.5 font-code text-[11px] text-label-secondary">
                {badgeHint}
              </span>
            )}
            {selected && (
              <span className="rounded-capsule bg-accent px-1.5 py-0.5 font-code text-[11px] font-medium text-on-accent">
                Active
              </span>
            )}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1 font-code text-[11px] text-label-secondary">
            {ctxBadge && (
              <span className="rounded-chip border border-border bg-surface px-1 py-0.5 text-label-secondary">
                {ctxBadge}
              </span>
            )}
            {caps?.tools && (
              <span className="rounded-chip border border-info-edge bg-info-wash px-1 py-0.5 text-info-ink">
                Tools
              </span>
            )}
            {caps?.vision && (
              <span className="rounded-chip border border-info-edge bg-info-wash px-1 py-0.5 text-info-ink">
                Vision
              </span>
            )}
            {caps?.reasoning && (
              <span className="rounded-chip border border-soft-indigo-edge bg-soft-indigo-wash px-1 py-0.5 text-accent-ink">
                Reasoning
              </span>
            )}
            {caps?.structured_output && (
              <span className="rounded-chip border border-border bg-surface px-1 py-0.5 text-label-secondary">
                JSON
              </span>
            )}
            {priceBadge && (
              <span
                title={priceBadge.title}
                className={`inline-flex items-center gap-1 rounded-chip border px-1 py-0.5 ${
                  priceBadge.warn
                    ? 'border-warning-edge bg-warning-wash text-warning-ink'
                    : 'border-border bg-surface text-label-secondary'
                }`}
              >
                {priceBadge.warn && <AlertTriangle className="h-3 w-3 shrink-0" />}
                <span>{priceBadge.label}</span>
              </span>
            )}
          </div>
        </button>
        <button
          type="button"
          aria-label={fav ? `Unfavorite ${item.id}` : `Favorite ${item.id}`}
          aria-pressed={fav}
          onClick={() => toggleFavorite(selectedProvider, item.id)}
          className={`shrink-0 rounded-control-compact p-1 transition-colors ${
            fav
              ? 'text-warning-ink hover:bg-warning-wash'
              : 'text-faint hover:bg-fill hover:text-text'
          }`}
        >
          <Star className={`h-3.5 w-3.5 ${fav ? 'fill-current' : ''}`} />
        </button>
      </div>
    );
  };

  return (
    <div
      ref={regionRef}
      role="region"
      aria-label="Quick model switcher"
      onKeyDown={handleKeyDown}
      className="mb-2.5 rounded-card border border-border-strong bg-float p-3 shadow-[var(--shadow-float)] space-y-2.5"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Cpu className="h-3.5 w-3.5 text-accent-ink" />
          <span className="font-code text-[11px] uppercase tracking-wider text-muted">
            Copilot Model {activeProvider ? `· ${activeProvider}` : ''}
          </span>
          {visibleEvalStatus && (
            <span
              data-testid="llm-eval-status"
              title={
                visibleEvalStatus.status === 'evaluated'
                  ? `Brier ${visibleEvalStatus.brier?.toFixed(3) ?? '—'}, ECE ${visibleEvalStatus.ece?.toFixed(3) ?? '—'} on the pinned copilot fixtures`
                  : visibleEvalStatus.status === 'stale'
                    ? 'The recorded evaluation predates the current fixtures'
                    : 'No recorded decision evaluation for this model'
              }
              className={`rounded-chip border px-1.5 py-0.5 font-code text-[11px] ${
                visibleEvalStatus.status === 'evaluated'
                  ? 'border-success-edge bg-success-wash text-success-ink'
                  : visibleEvalStatus.status === 'stale'
                    ? 'border-warning-edge bg-warning-wash text-warning-ink'
                    : 'border-border bg-tile text-muted'
              }`}
            >
              {visibleEvalStatus.status === 'evaluated'
                ? `evaluated ${visibleEvalStatus.evaluated}/${visibleEvalStatus.total} · ${
                    visibleEvalStatus.evaluated_at && !Number.isNaN(new Date(visibleEvalStatus.evaluated_at).getTime())
                      ? new Date(visibleEvalStatus.evaluated_at).toLocaleDateString()
                      : 'date unknown'
                  }`
                : visibleEvalStatus.status === 'stale'
                  ? 'evaluation stale'
                  : 'unevaluated'}
            </span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={onToggleSyncMatchingRoles}
            aria-pressed={syncMatchingRoles}
            className={`rounded-control-compact border px-2 py-0.5 text-[11px] font-code transition-colors ${
              syncMatchingRoles
                ? 'border-accent-edge bg-accent-soft text-accent-ink'
                : 'border-border bg-tile text-muted hover:text-text'
            }`}
          >
            {syncMatchingRoles ? 'Copilot + default roles' : 'Copilot only'}
          </button>
          <button
            type="button"
            aria-expanded={showSampling}
            onClick={() => setShowSampling((v) => !v)}
            className={`inline-flex items-center gap-1 rounded-control-compact border px-2 py-0.5 text-[11px] font-code transition-colors ${
              showSampling
                ? 'border-info-edge bg-info-wash text-info-ink'
                : 'border-border bg-tile text-muted hover:text-text'
            }`}
          >
            <SlidersHorizontal className="h-3 w-3" />
            <span>Sampling</span>
          </button>
          {onOpenSettings && (
            <button
              type="button"
              onClick={() => {
                onClose();
                onOpenSettings();
              }}
              className="text-[12px] text-accent-ink hover:underline"
            >
              Open LLM Settings →
            </button>
          )}
          {onOpenDownloadHub && (
            <button
              type="button"
              onClick={() => {
                onClose();
                onOpenDownloadHub();
              }}
              className="text-[12px] text-muted hover:text-text hover:underline"
            >
              Local Downloads →
            </button>
          )}
          <button
            type="button"
            onClick={handleClose}
            aria-label="Close model switcher"
            className="rounded-control-compact p-0.5 text-muted hover:text-text hover:bg-fill"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-[170px_1fr]">
        {/* Provider Rail */}
        <div
          role="tablist"
          aria-label="Providers"
          className="flex gap-1.5 overflow-x-auto sm:flex-col sm:overflow-y-auto sm:max-h-60 pr-0.5"
        >
          {providerEntries.map(([pt, cfg]) => {
            const isTabSelected = pt === selectedProvider;
            return (
              <button
                key={pt}
                type="button"
                role="tab"
                aria-selected={isTabSelected}
                onClick={() => {
                  setTabOverride({ base: defaultProvider, value: pt });
                  setHighlightedIdx(0);
                  onSelectProviderTab(pt);
                }}
                className={`flex shrink-0 items-center justify-between gap-2 rounded-row border px-2.5 py-1.5 text-left text-[12px] transition-colors ${
                  isTabSelected
                    ? 'border-accent-edge bg-raised text-text font-medium shadow-[var(--row-rim)]'
                    : 'border-border bg-surface text-label-secondary hover:bg-fill-hover'
                }`}
              >
                <span className="flex min-w-0 items-center gap-1.5">
                  <span className={`h-2 w-2 shrink-0 rounded-capsule ${standingDotClass(cfg)}`} />
                  <span className="truncate font-code">{pt}</span>
                </span>
                <span className={`font-code text-[11px] ${isTabSelected ? 'text-label-secondary' : 'text-muted'}`}>
                  {cfg.is_local ? 'local' : 'cloud'}
                </span>
              </button>
            );
          })}
        </div>

        {/* Searchable Models & Sampling */}
        <div className="min-w-0 space-y-2">
          <div className="flex items-center gap-1.5 rounded-row border border-border bg-field px-2.5 py-1.5 focus-within:border-accent-edge">
            <Search className="h-3.5 w-3.5 shrink-0 text-muted" />
            <input
              ref={searchRef}
              type="text"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setHighlightedIdx(0);
              }}
              aria-label="Search models"
              placeholder={`Filter ${selectedProvider} models or type custom slug…`}
              className="min-w-0 flex-1 bg-transparent font-code text-[12px] text-text placeholder:text-faint focus:outline-none"
            />
            {query && (
              <button
                type="button"
                aria-label="Clear model search"
                onClick={() => setQuery('')}
                className="text-muted hover:text-text"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>

          {showSampling && (
            <div className="rounded-row border border-border bg-surface p-2.5 space-y-2">
              <div className="grid grid-cols-3 gap-2 text-[11px]">
                <div>
                  <label htmlFor="picker-copilot-reasoning" className="block font-code text-muted mb-1">
                    Reasoning
                  </label>
                  <select
                    id="picker-copilot-reasoning"
                    aria-label="Copilot reasoning effort"
                    value={reasoningEffort}
                    onChange={(e) => setReasoningEffort(e.target.value)}
                    className="w-full rounded-control-compact border border-border bg-field px-2 py-1 font-code text-[12px] text-text"
                  >
                    <option value="">default</option>
                    <option value="none">none</option>
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                  </select>
                </div>
                <div>
                  <label htmlFor="picker-copilot-temp" className="block font-code text-muted mb-1">
                    Temp (0–2)
                  </label>
                  <input
                    id="picker-copilot-temp"
                    aria-label="Copilot temperature override"
                    type="number"
                    min="0"
                    max="2"
                    step="0.05"
                    placeholder="global"
                    value={temperatureInput}
                    onChange={(e) => setTemperatureInput(e.target.value)}
                    className="w-full rounded-control-compact border border-border bg-field px-2 py-1 font-code text-[12px] text-text"
                  />
                </div>
                <div>
                  <label htmlFor="picker-copilot-seed" className="block font-code text-muted mb-1">
                    Seed
                  </label>
                  <input
                    id="picker-copilot-seed"
                    aria-label="Copilot deterministic seed"
                    type="number"
                    step="1"
                    placeholder="unset"
                    value={seedInput}
                    onChange={(e) => setSeedInput(e.target.value)}
                    className="w-full rounded-control-compact border border-border bg-field px-2 py-1 font-code text-[12px] text-text"
                  />
                </div>
              </div>
              {onApplySampling && (
                <div className="flex justify-end">
                  <button
                    type="button"
                    disabled={saving}
                    onClick={() => void onApplySampling(parsedSampling())}
                    className="rounded-control-compact border border-accent-edge bg-accent-soft px-2.5 py-1 font-code text-[11px] text-accent-ink hover:bg-accent hover:text-on-accent disabled:opacity-40"
                  >
                    Save Copilot sampling
                  </button>
                </div>
              )}
            </div>
          )}

          {providerAirGapBlocked && (
            <p className="rounded-row border border-warning-edge bg-warning-wash px-2.5 py-1.5 text-[12px] text-warning-ink">
              Cloud provider {selectedProvider} is blocked while Sovereign Air-Gap is active.
            </p>
          )}

          {providerError && (
            <p className="rounded-row border border-danger-edge bg-danger-wash px-2.5 py-1.5 text-[12px] text-danger-ink">
              Model list error ({selectedProvider}): {providerError}
            </p>
          )}

          <div className="max-h-56 space-y-1.5 overflow-y-auto pr-0.5">
            {loadingProvider === selectedProvider ? (
              <p className="py-3 text-label font-code text-muted">
                Loading configured models for {selectedProvider}…
              </p>
            ) : (
              <>
                {pinnedForSelected.length > 0 && (
                  <div className="space-y-1">
                    <div className="font-code text-[11px] uppercase tracking-wider text-muted">
                      Pinned &amp; Recent
                    </div>
                    {pinnedForSelected.map((item) =>
                      renderModelRow(item, -1, item.pinKind === 'favorite' ? '★ Saved' : 'Recent'),
                    )}
                  </div>
                )}

                {filteredModels.length > 0 ? (
                  <div className="space-y-1">
                    {pinnedForSelected.length > 0 && (
                      <div className="pt-1 font-code text-[11px] uppercase tracking-wider text-muted">
                        Catalog ({filteredModels.length})
                      </div>
                    )}
                    {filteredModels.map((item, idx) => renderModelRow(item, idx))}
                  </div>
                ) : (
                  <p className="py-1 text-label text-muted">
                    No models listed for {selectedProvider}. {trimmedQuery ? 'Apply the typed model ID below or open LLM Settings.' : 'Open LLM Settings to verify endpoint or configure a default.'}
                  </p>
                )}

                {trimmedQuery !== '' && !hasExactMatch && !providerAirGapBlocked && (
                  <button
                    type="button"
                    data-model-id={trimmedQuery}
                    disabled={saving}
                    onClick={() => void pickModel(selectedProvider, trimmedQuery)}
                    className="w-full rounded-row border border-accent-edge bg-accent-soft px-2.5 py-1.5 text-left font-code text-[12px] text-accent-ink hover:bg-accent hover:text-on-accent disabled:opacity-50"
                  >
                    Use custom model &ldquo;{trimmedQuery}&rdquo; on {selectedProvider}
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      </div>

      {statusMessage && (
        <p className="text-[12px] font-code text-label-secondary">{statusMessage}</p>
      )}
    </div>
  );
}
