'use client';

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowUp, Brain, ChevronUp, Cpu, Paperclip, Sparkles, X } from 'lucide-react';
import type { ChatAttachment, FollowUpSpec } from '@/domain/useCopilotChat';
import { subscribeInvalidation } from '@/domain/invalidation';
import { fetchLLMModels, fetchLLMSettings, saveLLMSettings } from '@/lib/api';
import { ContextReadout } from '../common/ContextReadout';
import { MemoryPanel } from '../MemoryPanel';
import { OverflowCard } from '../common/OverflowCard';
import {
  ModelPicker,
  type PickerModelItem,
  type PickerProviderConfig,
  type PickerRoleSampling,
} from './ModelPicker';

type MorphPanel = 'actions' | 'memory' | 'model' | null;

type LlmModelItem = PickerModelItem;

export interface CommandDockProps {
  onSubmit: (text: string) => Promise<void> | void;
  contextLabel?: string | null;
  onClearContext?: () => void;
  onAttach?: (name: string, text: string) => void;
  onDetach?: (name: string) => void;
  attachments?: ChatAttachment[];
  onOpenModel?: () => void;
  onOpenDownloadHub?: () => void;
  modelLabel?: string;
  sessionId?: string | null;
  messageCount?: number;
  streaming?: boolean;
  busy?: boolean;
  onStop?: () => void;
  followUps?: FollowUpSpec[];
  onFollowUp?: (key: string) => void;
  onTribunal?: (topic: string) => Promise<void> | void;
  targetDisplayName?: string;
  onSessionChange?: (newSessionId: string) => void;
  onOpenInspector?: () => void;
  inspectorShortcut?: string;
}

function toneClass(tone: FollowUpSpec['tone']): string {
  if (tone === 'success') return 'text-[var(--color-success)]';
  if (tone === 'warning') return 'text-[var(--color-warning)]';
  return 'text-[var(--color-accent-ink)]';
}

export const CommandDock = React.memo(function CommandDock({
  onSubmit,
  contextLabel,
  onClearContext,
  onAttach,
  onDetach,
  attachments: controlledAttachments,
  onOpenModel,
  onOpenDownloadHub,
  modelLabel = 'Model Hub',
  sessionId = null,
  messageCount = 0,
  streaming = false,
  busy = false,
  onStop,
  followUps = [],
  onFollowUp,
  onTribunal,
  targetDisplayName = 'Active target',
  onSessionChange,
  onOpenInspector,
  inspectorShortcut,
}: CommandDockProps) {
  const [text, setText] = useState('');
  const [localAttachments, setLocalAttachments] = useState<Array<{ name: string }>>([]);
  const [submitting, setSubmitting] = useState(false);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [morph, setMorph] = useState<MorphPanel>(null);
  const [tribunalTopic, setTribunalTopic] = useState('');
  const [activeModelName, setActiveModelName] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<string | null>(null);
  const [airGapped, setAirGapped] = useState(false);
  const [providersMap, setProvidersMap] = useState<Record<string, PickerProviderConfig>>({});
  const [modelsByProvider, setModelsByProvider] = useState<Record<string, LlmModelItem[]>>({});
  const [modelErrorsByProvider, setModelErrorsByProvider] = useState<Record<string, string | null>>({});
  const [copilotSampling, setCopilotSampling] = useState<PickerRoleSampling>({});
  const [modelStatus, setModelStatus] = useState<string | null>(null);
  const [syncMatchingRoles, setSyncMatchingRoles] = useState(false);
  const [loadingProvider, setLoadingProvider] = useState<string | null>(null);
  const [savingModel, setSavingModel] = useState(false);
  const [rememberedCount, setRememberedCount] = useState(0);
  const [prevSessionId, setPrevSessionId] = useState(sessionId);
  if (prevSessionId !== sessionId) {
    setPrevSessionId(sessionId);
    setRememberedCount(0);
  }

  const areaRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const quickModelRef = useRef<HTMLButtonElement>(null);

  const hydrateLLMState = useCallback((settings: Record<string, unknown> | null | undefined) => {
    if (!settings) return null;
    const roles = (settings.role_assignments as Record<string, { provider?: string; model?: string; temperature?: number; seed?: number; reasoning_effort?: string }> | undefined) ?? {};
    const copilot = roles.copilot;
    const provider =
      (typeof copilot?.provider === 'string' && copilot.provider) ||
      (typeof settings.active_provider === 'string' && settings.active_provider) ||
      null;
    const model =
      (typeof copilot?.model === 'string' && copilot.model) ||
      (typeof settings.active_model === 'string' && settings.active_model) ||
      null;
    if (provider) setActiveProvider(provider);
    if (model) setActiveModelName(model);
    if (typeof settings.air_gapped === 'boolean') setAirGapped(settings.air_gapped);
    if (settings.providers && typeof settings.providers === 'object') {
      setProvidersMap(settings.providers as Record<string, PickerProviderConfig>);
    }
    setCopilotSampling({
      temperature: typeof copilot?.temperature === 'number' ? copilot.temperature : undefined,
      seed: typeof copilot?.seed === 'number' ? copilot.seed : undefined,
      reasoning_effort: typeof copilot?.reasoning_effort === 'string' ? copilot.reasoning_effort : undefined,
    });
    return { provider: provider || 'omlx', model };
  }, []);

  const loadModelsForProvider = useCallback(async (provider: string) => {
    setLoadingProvider(provider);
    setModelErrorsByProvider((prev) => ({ ...prev, [provider]: null }));
    try {
      const resp = await fetchLLMModels(provider);
      const list = Array.isArray(resp?.models) ? (resp.models as LlmModelItem[]) : [];
      setModelsByProvider((prev) => ({ ...prev, [provider]: list }));
      const errMsg = typeof resp?.error === 'string' && resp.error.trim() ? resp.error : '';
      if (errMsg) {
        setModelErrorsByProvider((prev) => ({ ...prev, [provider]: errMsg }));
      }
    } catch (err) {
      setModelErrorsByProvider((prev) => ({
        ...prev,
        [provider]: err instanceof Error ? err.message : 'Could not inspect active models.',
      }));
    } finally {
      setLoadingProvider((cur) => (cur === provider ? null : cur));
    }
  }, []);

  const attachments = controlledAttachments
    ? controlledAttachments.map((item) => ({ name: item.name }))
    : localAttachments;
  const isBusy = busy || submitting;

  const grow = () => {
    const el = areaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(132, el.scrollHeight)}px`;
  };

  useEffect(() => {
    let alive = true;
    const refresh = () => {
      void fetchLLMSettings()
        .then((settings) => {
          if (alive) hydrateLLMState(settings as Record<string, unknown> | null);
        })
        .catch(() => {});
    };
    refresh();
    const unsub = subscribeInvalidation(['llm'], refresh);
    return () => {
      alive = false;
      unsub();
    };
  }, [hydrateLLMState]);

  useEffect(() => {
    if (morph !== 'model') return;
    let alive = true;
    void (async () => {
      try {
        const settings = await fetchLLMSettings();
        if (!alive) return;
        const hydrated = hydrateLLMState(settings as Record<string, unknown> | null);
        const targetProvider = hydrated?.provider || 'omlx';
        await loadModelsForProvider(targetProvider);
      } catch (err) {
        if (alive) {
          setModelErrorsByProvider((prev) => ({
            ...prev,
            omlx: err instanceof Error ? err.message : 'Could not inspect active models.',
          }));
        }
      }
    })();
    return () => {
      alive = false;
    };
  }, [morph, hydrateLLMState, loadModelsForProvider]);

  const send = async () => {
    const value = text.trim();
    if (!value || isBusy) return;
    setSubmitting(true);
    setMorph(null);
    try {
      await onSubmit(value);
      setText('');
      if (!controlledAttachments) setLocalAttachments([]);
      if (areaRef.current) areaRef.current.style.height = 'auto';
    } finally {
      setSubmitting(false);
    }
  };

  const attach = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setAttachError(null);
    try {
      const raw = await file.text();
      const textValue = raw.length > 4000 ? `${raw.slice(0, 4000)}\n…[truncated]` : raw;
      if (!controlledAttachments) {
        setLocalAttachments((previous) => [...previous.filter((item) => item.name !== file.name), { name: file.name }]);
      }
      onAttach?.(file.name, textValue);
    } catch {
      setAttachError(`Could not read ${file.name}: binary or unreadable file.`);
    }
  };

  const detach = (name: string) => {
    if (!controlledAttachments) {
      setLocalAttachments((previous) => previous.filter((item) => item.name !== name));
    }
    onDetach?.(name);
  };

  const switchQuickModel = async (modelId: string, providerOverride?: string, sampling?: PickerRoleSampling) => {
    if (savingModel) return;
    setSavingModel(true);
    setModelStatus(null);
    try {
      const current = await fetchLLMSettings();
      if (!current) {
        setModelStatus('Settings unreachable — nothing changed.');
        return;
      }
      const existingRoles =
        (current.role_assignments as Record<string, { role: string; provider: string; model: string; temperature?: number; seed?: number; reasoning_effort?: string }> | undefined) ?? {};
      const prevCopilot = existingRoles.copilot;
      const targetProvider =
        providerOverride ||
        activeProvider ||
        prevCopilot?.provider ||
        (typeof current.active_provider === 'string' && current.active_provider) ||
        'omlx';
      const prevProvider = prevCopilot?.provider || (typeof current.active_provider === 'string' ? current.active_provider : '');
      const prevModel = prevCopilot?.model || (typeof current.active_model === 'string' ? current.active_model : '');

      const mergedCopilot = {
        ...(prevCopilot ?? { role: 'copilot', provider: targetProvider, model: modelId }),
        role: 'copilot',
        provider: targetProvider,
        model: modelId,
      };
      if (sampling?.temperature !== undefined) mergedCopilot.temperature = sampling.temperature;
      if (sampling?.seed !== undefined) mergedCopilot.seed = sampling.seed;
      if (sampling?.reasoning_effort !== undefined && sampling.reasoning_effort !== '') {
        mergedCopilot.reasoning_effort = sampling.reasoning_effort;
      }

      const nextRoles: Record<string, typeof mergedCopilot> = {
        ...existingRoles,
        copilot: mergedCopilot,
      };
      if (syncMatchingRoles) {
        for (const [role, assign] of Object.entries(existingRoles)) {
          if (role === 'copilot') continue;
          const matchesPrev = assign.provider === prevProvider && assign.model === prevModel;
          const isUnassigned = !assign.model;
          if (matchesPrev || isUnassigned) {
            nextRoles[role] = { ...assign, provider: targetProvider, model: modelId };
          }
        }
      }

      const res = await saveLLMSettings({
        ...current,
        active_provider: targetProvider,
        active_model: modelId,
        role_assignments: nextRoles,
      });
      if (res.ok) {
        setActiveProvider(targetProvider);
        setActiveModelName(modelId);
        if (sampling) setCopilotSampling(sampling);
        setModelStatus(
          syncMatchingRoles
            ? `Switched copilot & matching default roles to ${targetProvider}:${modelId}.`
            : `Switched copilot & active model to ${targetProvider}:${modelId}.`,
        );
      } else {
        setModelStatus(`Refused: ${res.error || 'save not confirmed'}.`);
      }
    } finally {
      setSavingModel(false);
    }
  };

  const applyCopilotSampling = async (sampling: PickerRoleSampling) => {
    if (savingModel) return;
    setSavingModel(true);
    setModelStatus(null);
    try {
      const current = await fetchLLMSettings();
      if (!current) {
        setModelStatus('Settings unreachable — nothing changed.');
        return;
      }
      const existingRoles =
        (current.role_assignments as Record<string, { role: string; provider: string; model: string; temperature?: number; seed?: number; reasoning_effort?: string }> | undefined) ?? {};
      const prevCopilot = existingRoles.copilot ?? {
        role: 'copilot',
        provider: activeProvider || (typeof current.active_provider === 'string' ? current.active_provider : 'omlx'),
        model: activeModelName || (typeof current.active_model === 'string' ? current.active_model : ''),
      };
      const updatedCopilot: typeof prevCopilot = {
        role: 'copilot',
        provider: prevCopilot.provider,
        model: prevCopilot.model,
      };
      if (sampling.temperature !== undefined) updatedCopilot.temperature = sampling.temperature;
      if (sampling.seed !== undefined) updatedCopilot.seed = sampling.seed;
      if (sampling.reasoning_effort) updatedCopilot.reasoning_effort = sampling.reasoning_effort;

      const res = await saveLLMSettings({
        ...current,
        role_assignments: { ...existingRoles, copilot: updatedCopilot },
      });
      if (res.ok) {
        setCopilotSampling(sampling);
        setModelStatus('Saved Copilot sampling overrides.');
      } else {
        setModelStatus(`Refused: ${res.error || 'save not confirmed'}.`);
      }
    } finally {
      setSavingModel(false);
    }
  };

  const effectiveModelLabel = activeModelName || modelLabel;

  return (
    <div className="relative shrink-0 border-t border-[var(--color-border)] bg-[var(--color-chrome)] px-4 py-3 text-[var(--color-text)]">
      {morph === 'actions' && (
        <div
          role="region"
          aria-label="Quick actions and tribunal"
          className="mb-2.5 rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-float)] p-3 shadow-[var(--shadow-float)] space-y-2.5"
        >
          <div className="flex items-center justify-between">
            <span className="font-code text-[11px] uppercase tracking-wider text-[var(--color-muted)]">
              Scientific Follow-ups &amp; Tribunal
            </span>
            <button
              type="button"
              onClick={() => setMorph(null)}
              aria-label="Close quick actions"
              className="rounded-md p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill)]"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
          {followUps.length > 0 && onFollowUp && (
            <div className="flex flex-wrap gap-1.5">
              {followUps.map((f) => (
                <button
                  key={f.label}
                  type="button"
                  onClick={() => {
                    onFollowUp(f.key);
                    setMorph(null);
                  }}
                  className="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-fill)] px-2.5 py-1 text-[12px] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)] transition-colors"
                >
                  <span className={toneClass(f.tone)}>✦</span>
                  <span>{f.label}</span>
                </button>
              ))}
            </div>
          )}
          {onTribunal && (
            <div className="flex flex-wrap items-center gap-2 pt-1 border-t border-[var(--color-border)]">
              <input
                type="text"
                value={tribunalTopic}
                onChange={(e) => setTribunalTopic(e.target.value)}
                placeholder={`${targetDisplayName} ground state electronic structure`}
                aria-label="Tribunal deliberation topic"
                className="min-w-[180px] flex-1 rounded-xl border border-[var(--color-border)] bg-[var(--color-field)] px-2.5 py-1.5 text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-accent-edge)]"
              />
              <button
                type="button"
                disabled={isBusy}
                onClick={() => {
                  const topic = (tribunalTopic.trim() || `${targetDisplayName} ground state electronic structure`).trim();
                  setMorph(null);
                  void onTribunal(topic);
                }}
                className="inline-flex items-center gap-1.5 rounded-xl border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-3 py-1.5 text-xs font-medium text-[var(--color-accent-ink)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] disabled:opacity-40 transition-colors"
              >
                <Sparkles className="h-3.5 w-3.5" />
                <span>Convene Multi-Agent Research Tribunal</span>
              </button>
            </div>
          )}
        </div>
      )}

      {morph === 'memory' && sessionId && (
        <div
          role="region"
          aria-label="Session memory morph"
          className="mb-2.5 rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-float)] overflow-hidden shadow-[var(--shadow-float)]"
        >
          <div className="flex items-center justify-between px-4 py-1.5 border-b border-[var(--color-border)] bg-[var(--color-chrome)]">
            <span className="font-code text-[11px] uppercase tracking-wider text-[var(--color-muted)]">
              Session Memory
            </span>
            <button
              type="button"
              onClick={() => setMorph(null)}
              aria-label="Close session memory"
              className="rounded-md p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill)]"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
          <MemoryPanel
            sessionId={sessionId}
            defaultOpen
            onRetained={() => setRememberedCount((c) => c + 1)}
          />
        </div>
      )}

      {morph === 'model' && (
        <ModelPicker
          activeProvider={activeProvider || 'omlx'}
          activeModel={activeModelName}
          airGapped={airGapped}
          providers={providersMap}
          modelsByProvider={modelsByProvider}
          errorsByProvider={modelErrorsByProvider}
          loadingProvider={loadingProvider}
          saving={savingModel}
          statusMessage={modelStatus}
          syncMatchingRoles={syncMatchingRoles}
          copilotSampling={copilotSampling}
          onToggleSyncMatchingRoles={() => setSyncMatchingRoles((v) => !v)}
          onSelectProviderTab={(prov) => {
            if (!modelsByProvider[prov]) {
              void loadModelsForProvider(prov);
            }
          }}
          onSelectModel={(provider, modelId, sampling) =>
            switchQuickModel(modelId, provider, sampling)
          }
          onApplySampling={applyCopilotSampling}
          onClose={() => setMorph(null)}
          onOpenSettings={onOpenModel}
          onOpenDownloadHub={onOpenDownloadHub}
          restoreFocusEl={quickModelRef.current}
        />
      )}

      <form
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
        aria-label="Composer"
      >
        {(contextLabel || attachments.length > 0 || text.startsWith('/')) && (
          <div className="mb-2 flex flex-wrap items-center gap-1.5">
            {contextLabel && (
              <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-fill)] px-2 py-0.5 text-[12px] text-[var(--color-label-secondary)]">
                <span className="truncate">{contextLabel}</span>
                {onClearContext && (
                  <button type="button" aria-label="Clear context" onClick={onClearContext} className="text-[var(--color-muted)] hover:text-[var(--color-text)]">
                    <X className="h-3 w-3" />
                  </button>
                )}
              </span>
            )}
            {attachments.map((attachment) => (
              <span
                key={attachment.name}
                className="inline-flex max-w-full items-center gap-1 rounded-full border border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] px-2 py-0.5 text-[12px] text-[var(--color-accent-ink)]"
              >
                <Paperclip className="h-3 w-3 shrink-0" />
                <span className="truncate">{attachment.name}</span>
                <button
                  type="button"
                  aria-label={`Remove ${attachment.name}`}
                  onClick={() => detach(attachment.name)}
                  className="text-[var(--color-accent-ink)] hover:text-[var(--color-text)]"
                >
                  <X className="h-3 w-3" />
                </button>
              </span>
            ))}
            {text.startsWith('/') && (
              <span className="text-[12px] font-code text-[var(--color-muted)]">
                <span className="text-[var(--color-accent-ink)]">/tribunal</span> convene ·{' '}
                <span className="text-[var(--color-accent-ink)]">/launch</span> last proposal ·{' '}
                <span className="text-[var(--color-accent-ink)]">/help</span>
              </span>
            )}
          </div>
        )}

        {sessionId && (
          <div className="mb-1.5">
            <OverflowCard
              sessionId={sessionId}
              refreshKey={messageCount}
              onSessionChanged={(id) => onSessionChange?.(id)}
            />
          </div>
        )}

        <div className="flex flex-wrap sm:flex-nowrap items-end gap-2">
          <div className="flex min-w-[min(100%,240px)] flex-1 flex-wrap sm:flex-nowrap items-end gap-1.5 sm:gap-2 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-accent-edge)]">
          <input
            ref={fileRef}
            type="file"
            accept=".pdb,.cif,.xyz,.mol,.sdf,.txt,.md,.json"
            className="hidden"
            onChange={(event) => {
              void attach(event);
            }}
            aria-label="Attach a text file"
          />
          <button
            type="button"
            aria-label="Attach a text file"
            title="Attach structure or text file (PDB · CIF · XYZ · 4,000 chars)"
            onClick={() => fileRef.current?.click()}
            className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)]"
          >
            <Paperclip className="h-4 w-4" />
          </button>

          <textarea
            ref={areaRef}
            rows={1}
            value={text}
            onChange={(event) => {
              setText(event.target.value);
              grow();
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                if (event.nativeEvent?.isComposing || event.keyCode === 229) return;
                void send();
              }
            }}
            aria-label="Ask Kubona"
            placeholder="Ask Kubona about this campaign, or type /tribunal · /launch…"
            className="max-h-[132px] min-h-6 min-w-[140px] flex-1 resize-none bg-transparent py-1 text-[13px] text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none"
          />

          <div className="flex items-center gap-1 ml-auto sm:ml-0 shrink-0">
          {(onTribunal || followUps.length > 0) && (
            <button
              type="button"
              aria-label="Quick actions and tribunal"
              aria-expanded={morph === 'actions'}
              onClick={() => setMorph((prev) => (prev === 'actions' ? null : 'actions'))}
              className={`mb-0.5 inline-flex h-8 shrink-0 items-center gap-1 rounded-xl px-2 text-[12px] transition-colors ${
                morph === 'actions'
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]'
                  : 'text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)]'
              }`}
            >
              <Sparkles className="h-3.5 w-3.5 shrink-0" />
              <span className="hidden sm:inline">Tribunal</span>
            </button>
          )}

          {sessionId && (
            <button
              type="button"
              aria-label="Session memory"
              aria-expanded={morph === 'memory'}
              onClick={() => setMorph((prev) => (prev === 'memory' ? null : 'memory'))}
              className={`mb-0.5 inline-flex h-8 shrink-0 items-center gap-1 rounded-xl px-2 text-[12px] transition-colors ${
                morph === 'memory'
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]'
                  : 'text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)]'
              }`}
            >
              <Brain className="h-3.5 w-3.5 shrink-0" />
              <span className="hidden md:inline">{rememberedCount > 0 ? `Remembered (${rememberedCount})` : 'Memory'}</span>
            </button>
          )}

          <div className="mb-0.5 inline-flex items-center">
            <button
              type="button"
              aria-label="Open Model Hub"
              title="Switch active Copilot model or open LLM Settings"
              onClick={() => {
                if (morph === 'model') {
                  setMorph(null);
                  return;
                }
                setModelStatus(null);
                setMorph('model');
              }}
              className="inline-flex h-8 max-w-28 shrink-0 items-center gap-1 rounded-xl px-2 text-[12px] text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)]"
            >
              <Cpu className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">{effectiveModelLabel}</span>
            </button>
            <button
              ref={quickModelRef}
              type="button"
              aria-label="Quick switch model"
              aria-expanded={morph === 'model'}
              title="Switch active model inline"
              onClick={() => {
                if (morph === 'model') {
                  setMorph(null);
                  return;
                }
                setModelStatus(null);
                setMorph('model');
              }}
              className={`flex h-8 w-6 items-center justify-center rounded-lg text-[var(--color-muted)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)] ${
                morph === 'model' ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]' : ''
              }`}
            >
              <ChevronUp className="h-3.5 w-3.5" />
            </button>
          </div>

          {streaming && onStop ? (
            <button
              type="button"
              onClick={onStop}
              aria-label="Stop generation"
              title="Stop generation"
              className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-[var(--color-danger)] text-[var(--color-on-accent)] hover:opacity-90"
            >
              <span className="h-2.5 w-2.5 rounded-[2px] bg-current" />
            </button>
          ) : (
            <button
              type="submit"
              disabled={!text.trim() || isBusy}
              aria-label="Send to copilot"
              className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-[var(--color-accent)] text-[var(--color-on-accent)] disabled:opacity-35 hover:bg-[var(--color-lit)] transition-colors"
            >
              <ArrowUp className="h-4 w-4" />
            </button>
          )}
          </div>
          </div>
          {onOpenInspector && (
            <button
              type="button"
              onClick={onOpenInspector}
              title={inspectorShortcut ? `Open Inspector (${inspectorShortcut})` : 'Open Inspector'}
              className="ml-auto sm:ml-0 inline-flex h-9 sm:h-12 shrink-0 items-center gap-2 rounded-2xl border border-[var(--color-accent-edge)] bg-[var(--color-float)] px-3.5 text-xs font-semibold text-[var(--color-label-secondary)] shadow-[var(--shadow-card)] hover:bg-[var(--color-fill-hover)] hover:text-[var(--color-text)] transition-colors"
            >
              <span className="relative flex h-4 w-4 items-center justify-center">
                <Sparkles className="h-3.5 w-3.5 text-[var(--color-accent-ink)]" />
                <span className="absolute -top-0.5 -right-0.5 h-1.5 w-1.5 rounded-full bg-[var(--color-success)]" />
              </span>
              <span>Inspector</span>
              {inspectorShortcut && (
                <kbd className="hidden sm:inline rounded border border-[var(--color-border)] bg-[var(--color-fill)] px-1.5 py-0.5 font-code text-[10px] text-[var(--color-muted)]">
                  {inspectorShortcut}
                </kbd>
              )}
            </button>
          )}
        </div>

        {attachError && <p role="alert" className="mt-1 text-[11px] text-[var(--color-warning)]">{attachError}</p>}
        <div className="mt-1 flex flex-wrap items-center justify-between gap-2 text-[11px] text-[var(--color-muted)]">
          <span>Enter sends · Shift+Enter newline · ⌘K command · context stays attached</span>
          {sessionId && <ContextReadout sessionId={sessionId} refreshKey={messageCount} />}
        </div>
      </form>
    </div>
  );
});
