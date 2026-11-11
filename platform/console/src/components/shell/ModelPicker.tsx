'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Cpu, Search, SlidersHorizontal, Star, X } from 'lucide-react';

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
  if (!provider.enabled) return 'bg-[var(--color-faint)]';
  if (provider.key_status === 'invalid' || (provider.last_fault && provider.last_fault !== 'none')) {
    return 'bg-[var(--color-danger)]';
  }
  if (!provider.is_local && provider.has_key === false) {
    return 'bg-[var(--color-faint)]';
  }
  if (provider.key_status === 'valid' || provider.is_local) {
    return 'bg-[var(--color-success)]';
  }
  return 'bg-[var(--color-warning)]';
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
        className={`flex items-center justify-between gap-2 rounded-xl border px-2.5 py-1.5 transition-colors ${
          selected
            ? 'border-[var(--color-accent-edge)] bg-[var(--color-raised)] shadow-[var(--row-rim)]'
            : highlighted
              ? 'border-[var(--color-border-strong)] bg-[var(--color-raised)]'
              : 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-fill-hover)]'
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
            <span className={`truncate font-code text-[12px] ${selected ? 'font-semibold text-[var(--color-accent-ink)]' : 'text-[var(--color-text)]'}`}>
              {item.id}
            </span>
            {badgeHint && (
              <span className="rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-1.5 py-0.5 font-code text-[11px] text-[var(--color-label-secondary)]">
                {badgeHint}
              </span>
            )}
            {selected && (
              <span className="rounded-full bg-[var(--color-accent)] px-1.5 py-0.5 font-code text-[11px] font-medium text-[var(--color-on-accent)]">
                Active
              </span>
            )}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1 font-code text-[11px] text-[var(--color-label-secondary)]">
            {ctxBadge && (
              <span className="rounded border border-[var(--color-border)] bg-[var(--color-surface)] px-1 py-0.5 text-[var(--color-label-secondary)]">
                {ctxBadge}
              </span>
            )}
            {caps?.tools && (
              <span className="rounded border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] px-1 py-0.5 text-[var(--color-info-ink)]">
                Tools
              </span>
            )}
            {caps?.vision && (
              <span className="rounded border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] px-1 py-0.5 text-[var(--color-info-ink)]">
                Vision
              </span>
            )}
            {caps?.reasoning && (
              <span className="rounded border border-[var(--color-soft-indigo-edge)] bg-[var(--color-soft-indigo-wash)] px-1 py-0.5 text-[var(--color-accent-ink)]">
                Reasoning
              </span>
            )}
            {caps?.structured_output && (
              <span className="rounded border border-[var(--color-border)] bg-[var(--color-surface)] px-1 py-0.5 text-[var(--color-label-secondary)]">
                JSON
              </span>
            )}
            {priceBadge && (
              <span
                title={priceBadge.title}
                className={`inline-flex items-center gap-1 rounded border px-1 py-0.5 ${
                  priceBadge.warn
                    ? 'border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)]'
                    : 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-label-secondary)]'
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
          className={`shrink-0 rounded-lg p-1 transition-colors ${
            fav
              ? 'text-[var(--color-warning-ink)] hover:bg-[var(--color-warning-wash)]'
              : 'text-[var(--color-faint)] hover:bg-[var(--color-fill)] hover:text-[var(--color-text)]'
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
      className="mb-2.5 rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-float)] p-3 shadow-[var(--shadow-float)] space-y-2.5"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Cpu className="h-3.5 w-3.5 text-[var(--color-accent-ink)]" />
          <span className="font-code text-[11px] uppercase tracking-wider text-[var(--color-muted)]">
            Copilot Model {activeProvider ? `· ${activeProvider}` : ''}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={onToggleSyncMatchingRoles}
            aria-pressed={syncMatchingRoles}
            className={`rounded-lg border px-2 py-0.5 text-[11px] font-code transition-colors ${
              syncMatchingRoles
                ? 'border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]'
                : 'border-[var(--color-border)] bg-[var(--color-tile)] text-[var(--color-muted)] hover:text-[var(--color-text)]'
            }`}
          >
            {syncMatchingRoles ? 'Copilot + default roles' : 'Copilot only'}
          </button>
          <button
            type="button"
            aria-expanded={showSampling}
            onClick={() => setShowSampling((v) => !v)}
            className={`inline-flex items-center gap-1 rounded-lg border px-2 py-0.5 text-[11px] font-code transition-colors ${
              showSampling
                ? 'border-[var(--color-info-edge)] bg-[var(--color-info-wash)] text-[var(--color-info-ink)]'
                : 'border-[var(--color-border)] bg-[var(--color-tile)] text-[var(--color-muted)] hover:text-[var(--color-text)]'
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
              className="text-[12px] text-[var(--color-accent-ink)] hover:underline"
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
              className="text-[12px] text-[var(--color-muted)] hover:text-[var(--color-text)] hover:underline"
            >
              Local Downloads →
            </button>
          )}
          <button
            type="button"
            onClick={handleClose}
            aria-label="Close model switcher"
            className="rounded-lg p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill)]"
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
                className={`flex shrink-0 items-center justify-between gap-2 rounded-xl border px-2.5 py-1.5 text-left text-[12px] transition-colors ${
                  isTabSelected
                    ? 'border-[var(--color-accent-edge)] bg-[var(--color-raised)] text-[var(--color-text)] font-medium shadow-[var(--row-rim)]'
                    : 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]'
                }`}
              >
                <span className="flex min-w-0 items-center gap-1.5">
                  <span className={`h-2 w-2 shrink-0 rounded-full ${standingDotClass(cfg)}`} />
                  <span className="truncate font-code">{pt}</span>
                </span>
                <span className={`font-code text-[11px] ${isTabSelected ? 'text-[var(--color-label-secondary)]' : 'text-[var(--color-muted)]'}`}>
                  {cfg.is_local ? 'local' : 'cloud'}
                </span>
              </button>
            );
          })}
        </div>

        {/* Searchable Models & Sampling */}
        <div className="min-w-0 space-y-2">
          <div className="flex items-center gap-1.5 rounded-xl border border-[var(--color-border)] bg-[var(--color-field)] px-2.5 py-1.5 focus-within:border-[var(--color-accent-edge)]">
            <Search className="h-3.5 w-3.5 shrink-0 text-[var(--color-muted)]" />
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
              className="min-w-0 flex-1 bg-transparent font-code text-[12px] text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none"
            />
            {query && (
              <button
                type="button"
                aria-label="Clear model search"
                onClick={() => setQuery('')}
                className="text-[var(--color-muted)] hover:text-[var(--color-text)]"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>

          {showSampling && (
            <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5 space-y-2">
              <div className="grid grid-cols-3 gap-2 text-[11px]">
                <div>
                  <label htmlFor="picker-copilot-reasoning" className="block font-code text-[var(--color-muted)] mb-1">
                    Reasoning
                  </label>
                  <select
                    id="picker-copilot-reasoning"
                    aria-label="Copilot reasoning effort"
                    value={reasoningEffort}
                    onChange={(e) => setReasoningEffort(e.target.value)}
                    className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-field)] px-2 py-1 font-code text-[12px] text-[var(--color-text)]"
                  >
                    <option value="">default</option>
                    <option value="none">none</option>
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                  </select>
                </div>
                <div>
                  <label htmlFor="picker-copilot-temp" className="block font-code text-[var(--color-muted)] mb-1">
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
                    className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-field)] px-2 py-1 font-code text-[12px] text-[var(--color-text)]"
                  />
                </div>
                <div>
                  <label htmlFor="picker-copilot-seed" className="block font-code text-[var(--color-muted)] mb-1">
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
                    className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-field)] px-2 py-1 font-code text-[12px] text-[var(--color-text)]"
                  />
                </div>
              </div>
              {onApplySampling && (
                <div className="flex justify-end">
                  <button
                    type="button"
                    disabled={saving}
                    onClick={() => void onApplySampling(parsedSampling())}
                    className="rounded-lg border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-2.5 py-1 font-code text-[11px] text-[var(--color-accent-ink)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] disabled:opacity-40"
                  >
                    Save Copilot sampling
                  </button>
                </div>
              )}
            </div>
          )}

          {providerAirGapBlocked && (
            <p className="rounded-xl border border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] px-2.5 py-1.5 text-[12px] text-[var(--color-warning-ink)]">
              Cloud provider {selectedProvider} is blocked while Sovereign Air-Gap is active.
            </p>
          )}

          {providerError && (
            <p className="rounded-xl border border-[var(--color-danger-edge)] bg-[var(--color-danger-wash)] px-2.5 py-1.5 text-[12px] text-[var(--color-danger-ink)]">
              Model list error ({selectedProvider}): {providerError}
            </p>
          )}

          <div className="max-h-56 space-y-1.5 overflow-y-auto pr-0.5">
            {loadingProvider === selectedProvider ? (
              <p className="py-3 text-xs font-code text-[var(--color-muted)]">
                Loading configured models for {selectedProvider}…
              </p>
            ) : (
              <>
                {pinnedForSelected.length > 0 && (
                  <div className="space-y-1">
                    <div className="font-code text-[11px] uppercase tracking-wider text-[var(--color-muted)]">
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
                      <div className="pt-1 font-code text-[11px] uppercase tracking-wider text-[var(--color-muted)]">
                        Catalog ({filteredModels.length})
                      </div>
                    )}
                    {filteredModels.map((item, idx) => renderModelRow(item, idx))}
                  </div>
                ) : (
                  <p className="py-1 text-xs text-[var(--color-muted)]">
                    No models listed for {selectedProvider}. {trimmedQuery ? 'Apply the typed model ID below or open LLM Settings.' : 'Open LLM Settings to verify endpoint or configure a default.'}
                  </p>
                )}

                {trimmedQuery !== '' && !hasExactMatch && !providerAirGapBlocked && (
                  <button
                    type="button"
                    data-model-id={trimmedQuery}
                    disabled={saving}
                    onClick={() => void pickModel(selectedProvider, trimmedQuery)}
                    className="w-full rounded-xl border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-2.5 py-1.5 text-left font-code text-[12px] text-[var(--color-accent-ink)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] disabled:opacity-50"
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
        <p className="text-[12px] font-code text-[var(--color-label-secondary)]">{statusMessage}</p>
      )}
    </div>
  );
}
