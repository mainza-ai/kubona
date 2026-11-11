'use client';

import React, { useState, useEffect, useRef, useMemo, useImperativeHandle } from 'react';
import {
  Sparkles,
  Atom,
  Scale,
  Cpu,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  ChevronDown,
  ChevronUp,
  Bot,
  Maximize2,
  Minimize2,
  PanelRightClose,
  Copy,
  Check,
  Paperclip,
  ArrowUp,
  Search,
  Download,
  X,
  Trash2,
  RotateCcw,
  ThumbsUp,
  ThumbsDown,
  Pencil,
  XCircle,
} from 'lucide-react';
import type { ChatMessage, CampaignState, RunPhase } from '@/types';
import { useAuthPhase } from '@/domain/auth-state';
import { useCopilotChat, type CopilotChatController } from '@/domain/useCopilotChat';
import { ContextReadout } from './common/ContextReadout';
import { OverflowCard } from './common/OverflowCard';
import { MathText } from './common/MathText';
import { Tip } from './common/Tip';
import { MemoryPanel } from './MemoryPanel';
import ConfirmDialog from './common/ConfirmDialog';
import type { CanvasWidgetKind } from './sovereign/WidgetStage';

interface CopilotDrawerProps {
  sessionId: string;
  campaign?: CampaignState | null;
  controller?: CopilotChatController;
  dockComposer?: boolean;
  isExpanded?: boolean;
  onToggleExpand?: () => void;
  onExperimentLaunched?: () => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
  onSelectSurface?: (surface: string) => void;
  onCreateCampaign?: () => void;
  requiresSignIn?: boolean;
  onMaterialize?: (kind: CanvasWidgetKind, title: string, jobId?: string, sessionId?: string) => void;
  onRunStateChange?: (phase: RunPhase, detail: string) => void;
  onSessionChange?: (newSessionId: string) => void;
  onClose?: () => void;
}

export type CopilotHandle = {
  submit: (text: string) => Promise<void>;
  attach: (name: string, text: string) => void;
  detach: (name: string) => void;
};

export const CopilotDrawer = React.forwardRef<CopilotHandle, CopilotDrawerProps>(function CopilotDrawer(
  {
    sessionId,
    campaign,
    controller: externalController,
    dockComposer = false,
    isExpanded: externalExpanded,
    onToggleExpand,
    onExperimentLaunched,
    onCampaignUpdated,
    onSelectSurface,
    onCreateCampaign,
    requiresSignIn = false,
    onMaterialize,
    onRunStateChange,
    onSessionChange,
    onClose,
  },
  ref,
) {
  const internalController = useCopilotChat({
    sessionId,
    campaign,
    requiresSignIn,
    disabled: Boolean(externalController),
    onSessionChange,
    onCampaignUpdated,
    onExperimentLaunched,
    onRunStateChange,
    onMaterialize,
  });
  const chat = externalController ?? internalController;

  const [input, setInput] = useState('');
  const [internalExpanded, setInternalExpanded] = useState(false);
  const isExpanded = externalExpanded !== undefined ? externalExpanded : internalExpanded;
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [expandedTurns, setExpandedTurns] = useState<Record<string, boolean>>({});
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchIndex, setSearchIndex] = useState(0);

  const authPhase = useAuthPhase();
  const messageRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const attachInputRef = useRef<HTMLInputElement>(null);
  const submitRef = useRef<CopilotHandle['submit'] | null>(null);

  const {
    messages,
    loading,
    streamingId,
    runningTribunal,
    tribunalStage,
    elapsedSeconds,
    errorMessage,
    infoMessage,
    needsLogin,
    authExpired,
    attachments,
    chatWriteLabel,
    feedbackWriteLabel,
    launchingId,
    launchedNote,
    launchedJobs,
    clearing,
    regenerating,
    rewriting,
    votes,
    canRegenerate,
    targetSystem,
    targetDisplayName,
    discrepancy,
    isChemicalAccuracyMet,
    followUps,
  } = chat;

  const handleToggleExpand = () => {
    if (onToggleExpand) onToggleExpand();
    else setInternalExpanded((prev) => !prev);
  };

  useEffect(() => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTo({
        top: messagesContainerRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  }, [messages, loading, runningTribunal]);

  useEffect(() => {
    submitRef.current = (text: string) => chat.send(text);
  });

  useImperativeHandle(
    ref,
    () => ({
      submit: (text: string) => submitRef.current?.(text) ?? Promise.resolve(),
      attach: chat.attach,
      detach: chat.detach,
    }),
    [chat.attach, chat.detach],
  );

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleAttachFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    try {
      const text = await file.text();
      const trimmed = text.length > 4000 ? `${text.slice(0, 4000)}\n…[truncated]` : text;
      chat.attach(file.name, trimmed);
    } catch {
      chat.setErrorMessage(`Could not read ${file.name}: binary or unreadable file.`);
    }
  };

  const matchedIds = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return [];
    return messages.filter((m) => String(m.content ?? '').toLowerCase().includes(q)).map((m) => m.id);
  }, [messages, searchQuery]);

  const jumpToMatch = (dir: 1 | -1) => {
    if (matchedIds.length === 0) return;
    const next = (searchIndex + dir + matchedIds.length) % matchedIds.length;
    setSearchIndex(next);
    requestAnimationFrame(() => {
      messageRefs.current[matchedIds[next]]?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });
  };

  const exportTranscript = () => {
    const lines = [
      '# Kubona co-pilot transcript',
      '',
      `- Session: ${sessionId || 'unsaved'}`,
      `- Target: ${targetDisplayName}`,
      `- Exported: ${new Date().toISOString()}`,
      '',
    ];
    for (const m of messages) {
      const when = new Date(m.created_at).toLocaleString();
      lines.push(`## ${m.role} · ${when}${m.model ? ` · ${m.model}` : ''}`);
      lines.push('');
      lines.push(m.content);
      lines.push('');
      if (m.proposal_json) {
        lines.push('```json');
        lines.push(JSON.stringify(m.proposal_json, null, 2));
        lines.push('```');
        lines.push('');
      }
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `kubona-transcript-${(sessionId || 'session').slice(0, 8)}.md`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const confirmClearChat = async () => {
    const closed = await chat.confirmClearChat();
    if (closed) setClearConfirmOpen(false);
  };

  const handleRewriteSave = async (messageId: string) => {
    const ok = await chat.rewriteSave(messageId, editDraft);
    if (ok) setEditingId(null);
  };

  const toggleTurnExpand = (turnKey: string) => {
    setExpandedTurns((prev) => ({
      ...prev,
      [turnKey]: !prev[turnKey],
    }));
  };

  const renderProposalCard = (m: ChatMessage) => {
    if (!m.proposal_json) {
      if (m.role !== 'user' && !m.tribunal && typeof m.content === 'string' && m.content.includes('Notice]')) {
        return (
          <div
            className="w-full max-w-[95%] rounded-2xl bg-[var(--color-surface)] border border-[var(--color-warning-edge)] p-3 space-y-1.5"
            aria-label="No proposal — model unavailable"
          >
            <div className="text-[12px] font-code text-[var(--color-warning)]">NO PROPOSAL — MODEL UNREACHABLE OR UNCONFIGURED</div>
            <p className="text-[12px] text-[var(--color-label-secondary)]">
              Nothing was dispatched. Classical paths still work: open Autonomy Cockpit → Simulate Offline (needs a campaign with QASM), or ask about H₂ / LiH / BeH₂ / VQE for server-side scaffolding.
            </p>
          </div>
        );
      }
      return null;
    }
    const p = m.proposal_json;
    const note = launchedNote[m.id];
    const jobId = launchedJobs[m.id];
    return (
      <div className="w-full max-w-[95%] rounded-2xl bg-[var(--color-surface)] border border-[var(--color-accent-edge)] p-3 space-y-2">
        <div className="flex items-center gap-1.5 text-[12px] font-code text-[var(--color-accent-ink)]">
          <span>✦</span>
          <span>LAUNCHABLE PROPOSAL</span>
          {p.designed_by === 'server-fallback' && (
            <span
              className="ml-auto px-1.5 py-0.5 rounded bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning)]"
              title="Circuit synthesized by the server as scaffolding, not designed by the model — review before launch"
            >
              SERVER FALLBACK
            </span>
          )}
        </div>
        <div className="text-xs text-[var(--color-text)] font-medium">{p.target}</div>
        <div className="grid grid-cols-2 gap-2 text-[12px] font-code text-[var(--color-muted)]">
          <div>
            <span className="text-[var(--color-faint)] block">Tier</span>
            <span className="text-[var(--color-accent-ink)]">{p.recommended_tier}</span>
          </div>
          <div>
            <span className="text-[var(--color-faint)] block">Spend</span>
            <span className="text-[var(--color-warning)]">{p.spend_estimate}</span>
          </div>
        </div>
        <div className="text-[12px] font-code text-[var(--color-faint)] truncate" title={p.classical_anchor}>
          anchor · {p.classical_anchor}
        </div>
        {note ? (
          jobId ? (
            <button
              onClick={() => chat.onMaterialize?.('job', `Job ${jobId.slice(0, 8)} · detail`, jobId, m.session_id)}
              title="Open full job detail (params, counts, result)"
              className="w-full text-left text-[12px] font-code text-[var(--color-success)] hover:bg-[var(--color-fill)] rounded-lg px-1.5 py-1 transition-all"
            >
              {note} →
            </button>
          ) : (
            <div className="text-[12px] font-code text-[var(--color-success)]">{note}</div>
          )
        ) : (
          <button
            onClick={() => void chat.launchProposal(m.id, p)}
            disabled={launchingId === m.id}
            className="w-full py-1.5 px-3 rounded-xl bg-[var(--color-accent-soft)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-[12px] font-semibold transition-all disabled:opacity-50"
          >
            {launchingId === m.id ? 'Dispatching…' : 'Launch on local simulator'}
          </button>
        )}
      </div>
    );
  };

  return (
    <div className="flex flex-col h-full w-full bg-[var(--color-chrome)] text-[var(--color-text)] font-sovereign overflow-hidden">
      {/* Header: Kubona Copilot identity */}
      <div className="px-3.5 sm:px-5 py-3 sm:py-4 border-b border-[var(--color-border)] flex flex-wrap items-center justify-between gap-2 shrink-0 min-w-0">
        <div className="flex items-center gap-2.5 min-w-0 flex-1 basis-[11rem]">
          <div className="w-2.5 h-2.5 rounded-full bg-[var(--color-accent-ink)] shrink-0" />
          <div className="min-w-0 flex-1">
            <div className="text-xs font-semibold text-[var(--color-text)] tracking-wide flex flex-wrap items-center gap-1.5">
              <span>KUBONA COPILOT</span>
              <span className="px-1.5 py-0.5 rounded bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)] font-code text-[12px]">
                {streamingId ? 'STREAMING' : runningTribunal ? 'DELIBERATING' : 'READY'}
              </span>
            </div>
            <div className="text-[12px] text-[var(--color-muted)] truncate" title={`Target: ${targetDisplayName} · Δ = ${discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}`}>
              Target: {targetDisplayName} · Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-1 sm:space-x-1.5 shrink-0">
          <Tip label="Search conversation" sub="Filter + jump between matches" position="bottom">
            <button
              onClick={() => setSearchOpen((v) => !v)}
              aria-label="Search this conversation"
              className={`p-1.5 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                searchOpen
                  ? 'text-[var(--color-accent-ink)] bg-[var(--color-fill)]'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)]'
              }`}
            >
              <Search className="w-3.5 h-3.5" />
            </button>
          </Tip>

          <Tip label="Export transcript" sub="Markdown download" position="bottom">
            <button
              onClick={exportTranscript}
              aria-label="Export transcript"
              disabled={messages.length === 0}
              className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
            >
              <Download className="w-3.5 h-3.5" />
            </button>
          </Tip>

          <Tip label="Regenerate answer" sub="Re-answers the last prompt" position="bottom">
            <button
              onClick={() => void chat.regenerate()}
              aria-label="Regenerate last answer"
              disabled={regenerating || loading || runningTribunal || !canRegenerate}
              className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
            >
              <RotateCcw className={`w-3.5 h-3.5 ${regenerating ? 'animate-spin' : ''}`} />
            </button>
          </Tip>

          <Tip label="Clear chat" sub="Deletes messages; session survives" position="bottom">
            <button
              onClick={() => {
                if (!clearing && !loading && !runningTribunal && messages.length > 0) setClearConfirmOpen(true);
              }}
              aria-label="Clear chat history"
              disabled={clearing || loading || runningTribunal || messages.length === 0}
              className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-danger)] hover:bg-[var(--color-soft-rose-wash)] transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-[var(--color-danger)] focus-visible:outline-none"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </Tip>

          <Tip label={isExpanded ? 'Dock co-pilot' : 'Expand co-pilot'} position="bottom">
            <button
              onClick={handleToggleExpand}
              aria-label={isExpanded ? 'Contract to Side Dock' : 'Expand to Spatial Salon'}
              className="hidden lg:inline-flex p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
            >
              {isExpanded ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5 text-[var(--color-accent-ink)]" />}
            </button>
          </Tip>

          {onClose && (
            <Tip label="Close co-pilot" sub="⌘J to reopen" position="bottom">
              <button
                onClick={onClose}
                aria-label="Close Copilot (⌘J)"
                className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
              >
                <PanelRightClose className="w-3.5 h-3.5" />
              </button>
            </Tip>
          )}
        </div>
      </div>

      {/* Active Scientific Focus Ribbon */}
      <div className="mx-5 mt-3 px-3 py-2 rounded-xl bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-wrap items-center justify-between gap-2 text-[12px] font-code shrink-0 min-w-0">
        <div className="flex items-center gap-2 text-[var(--color-label-secondary)] min-w-0 flex-1">
          <span className="text-[var(--color-accent-ink)] font-bold shrink-0">TARGET:</span>
          <span className="truncate" title={targetDisplayName}>{targetDisplayName}</span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span
            className="w-1.5 h-1.5 rounded-full"
            style={{ background: isChemicalAccuracyMet ? 'var(--color-success)' : 'var(--color-warning)' }}
          />
          <span className={isChemicalAccuracyMet ? 'text-[var(--color-success)]' : 'text-[var(--color-warning)]'}>
            Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
          </span>
        </div>
      </div>

      {/* In-chat search bar */}
      {searchOpen && (
        <div className="mx-5 mt-2 flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] shrink-0">
          <Search className="w-3.5 h-3.5 text-[var(--color-faint)] shrink-0" />
          <input
            autoFocus
            value={searchQuery}
            aria-label="Find in conversation"
            onChange={(e) => {
              setSearchQuery(e.target.value);
              setSearchIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') jumpToMatch(1);
              if (e.key === 'Escape') setSearchOpen(false);
            }}
            placeholder="Find in conversation…"
            className="flex-1 min-w-0 bg-transparent text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none"
          />
          <span className="text-[12px] font-code text-[var(--color-muted)] shrink-0">
            {matchedIds.length === 0 ? '0' : `${searchIndex + 1}/${matchedIds.length}`}
          </span>
          <Tip label="Previous match" position="top">
            <button onClick={() => jumpToMatch(-1)} aria-label="Previous match" className="p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)]">
              <ChevronUp className="w-3.5 h-3.5" />
            </button>
          </Tip>
          <Tip label="Next match" position="top">
            <button onClick={() => jumpToMatch(1)} aria-label="Next match" className="p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)]">
              <ChevronDown className="w-3.5 h-3.5" />
            </button>
          </Tip>
          <Tip label="Close search" sub="Esc" position="top">
            <button
              onClick={() => {
                setSearchOpen(false);
                setSearchQuery('');
              }}
              aria-label="Close search"
              className="p-0.5 text-[var(--color-muted)] hover:text-[var(--color-text)]"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </Tip>
        </div>
      )}

      {/* Messages Feed */}
      <div ref={messagesContainerRef} className="flex-1 overflow-y-auto space-y-4 p-4 scrollbar-thin">
        {messages.length === 0 && (
          <div className="p-3 rounded-xl bg-[var(--color-surface)] border border-[var(--color-border)] text-[12px] text-[var(--color-muted)] leading-relaxed">
            {campaign ? (
              <div className="space-y-1.5">
                <p>
                  <span className="text-[var(--color-text)] font-medium">Campaign context:</span> {targetDisplayName}.
                </p>
                <p>
                  {campaign.epistemics?.status === 'COLLAPSED_VERIFIED' && campaign.notebook
                    ? 'Next: open Results to inspect the citable manuscript.'
                    : campaign.notebook
                      ? 'Next: evaluate the recorded notebook claim in the Tribunal, or ask for analysis.'
                      : 'Next: ask Kubona to propose the first experiment; nothing has executed yet.'}
                </p>
                {campaign.epistemics?.status === 'COLLAPSED_VERIFIED' && campaign.notebook && onSelectSurface && (
                  <button
                    type="button"
                    onClick={() => onSelectSurface('surface7')}
                    className="px-2.5 py-1 rounded-lg bg-[var(--color-accent-soft)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-[12px] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] transition-colors"
                  >
                    Open Results
                  </button>
                )}
              </div>
            ) : (
              <div className="space-y-2">
                <p>No campaign selected. Create or open a campaign to start.</p>
                {onCreateCampaign && (
                  <button
                    type="button"
                    onClick={onCreateCampaign}
                    className="px-2.5 py-1 rounded-lg bg-[var(--color-accent-soft)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-[12px] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] transition-colors"
                  >
                    Create campaign
                  </button>
                )}
              </div>
            )}
          </div>
        )}

        {messages.length === 0 && campaign && (() => {
          const starterTarget = campaign.quantum_factors?.target_molecule || campaign.target_system || '';
          const starterSubject = starterTarget || 'target system';
          return (
          <div className="pt-1 space-y-2 text-left">
            <button
              onClick={() => void chat.send(`Map ${starterSubject} ground state potential energy surface across nuclear coordinates`)}
              className="w-full p-3 rounded-2xl bg-[var(--color-surface)] hover:bg-[var(--color-raised)] border border-[var(--color-border)] hover:border-[var(--color-accent-edge)] text-xs text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-all flex items-center justify-between group"
            >
              <div className="flex items-center space-x-2.5 min-w-0">
                <span className="text-[var(--color-warning)] text-[12px] shrink-0">✦</span>
                <span className="font-medium truncate">
                  {starterTarget ? `${starterTarget} Potential Energy Surface` : 'Potential Energy Surface'}
                </span>
              </div>
              <ChevronRight className="w-3.5 h-3.5 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-accent-ink)] group-hover:translate-x-0.5 transition-all" />
            </button>

            <button
              onClick={() => void chat.conveneTribunal(`${starterSubject} ground state electronic structure`)}
              className="w-full p-3 rounded-2xl bg-[var(--color-surface)] hover:bg-[var(--color-raised)] border border-[var(--color-border)] hover:border-[var(--color-success-edge)] text-xs text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-all flex items-center justify-between group"
            >
              <div className="flex items-center space-x-2.5 min-w-0">
                <span className="text-[var(--color-success)] text-[12px] shrink-0">✦</span>
                <span className="font-medium truncate">Convene Tribunal for {starterSubject}</span>
              </div>
              <ChevronRight className="w-3.5 h-3.5 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-success)] group-hover:translate-x-0.5 transition-all" />
            </button>

            <button
              onClick={() => void chat.send('Verify our variational ansatz on the configured QPU backend with Clifford canary')}
              className="w-full p-3 rounded-2xl bg-[var(--color-surface)] hover:bg-[var(--color-raised)] border border-[var(--color-border)] hover:border-[var(--color-accent-edge)] text-xs text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-all flex items-center justify-between group"
            >
              <div className="flex items-center space-x-2.5 min-w-0">
                <span className="text-[var(--color-accent-ink)] text-[12px] shrink-0">✦</span>
                <span className="font-medium truncate">Run Clifford Canary Check</span>
              </div>
              <ChevronRight className="w-3.5 h-3.5 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-accent-ink)] group-hover:translate-x-0.5 transition-all" />
            </button>
          </div>
          );
        })()}

        {messages.map((m) => {
          const isUser = m.role === 'user';
          const hasTribunal = Boolean(m.tribunal && m.tribunal.turns && m.tribunal.turns.length > 0);

          return (
            <div
              key={m.id}
              ref={(el) => {
                messageRefs.current[m.id] = el;
              }}
              className={`flex flex-col space-y-1.5 rounded-xl -mx-1 px-1 py-0.5 transition-shadow ${
                isUser ? 'items-end' : 'items-start'
              } ${matchedIds.length > 0 && matchedIds[searchIndex] === m.id ? 'ring-1 ring-[var(--color-accent-edge)]' : ''}`}
            >
              <div className="flex items-center space-x-2 text-[12px] text-[var(--color-muted)] font-mono px-1">
                <span>{isUser ? 'You' : 'Kubona Tribunal Salon'}</span>
                <span>•</span>
                <span>{new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                {!isUser && m.model && (
                  <>
                    <span>•</span>
                    <span className="text-[var(--color-faint)]">served by {m.model}</span>
                  </>
                )}
                {!isUser && m.duration_ms != null && (
                  <>
                    <span>•</span>
                    <span className="text-[var(--color-faint)]">{(m.duration_ms / 1000).toFixed(1)}s</span>
                  </>
                )}
                {!isUser && m.usage?.completion_tokens != null && (
                  <>
                    <span>•</span>
                    <span className="text-[var(--color-faint)]">{m.usage.completion_tokens} tok</span>
                  </>
                )}
                <Tip label="Copy message" position="top">
                  <button
                    onClick={() => copyToClipboard(m.content, `copy-${m.id}`)}
                    aria-label="Copy message"
                    className="p-0.5 rounded text-[var(--color-faint)] hover:text-[var(--color-text)] transition-colors focus-visible:ring-1 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
                  >
                    {copiedId === `copy-${m.id}` ? <Check className="w-3 h-3 text-[var(--color-success)]" /> : <Copy className="w-3 h-3" />}
                  </button>
                </Tip>
                {isUser && !streamingId && (
                  <Tip label="Edit and resend" sub="Branches the thread" position="top">
                    <button
                      onClick={() => {
                        setEditingId(m.id);
                        setEditDraft(m.content);
                      }}
                      aria-label="Edit and resend"
                      className="p-0.5 rounded text-[var(--color-faint)] hover:text-[var(--color-text)] transition-colors focus-visible:ring-1 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
                    >
                      <Pencil className="w-3 h-3" />
                    </button>
                  </Tip>
                )}
                {!isUser && (
                  <>
                    <Tip label="Helpful" sub="Feeds the credibility flywheel" position="top">
                      <button
                        onClick={() => void chat.vote(m.id, 'up')}
                        aria-label="Mark helpful"
                        aria-pressed={votes[m.id] === 'up'}
                        className={`p-0.5 rounded transition-colors focus-visible:ring-1 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                          votes[m.id] === 'up' ? 'text-[var(--color-success)]' : 'text-[var(--color-faint)] hover:text-[var(--color-text)]'
                        }`}
                      >
                        <ThumbsUp className="w-3 h-3" />
                      </button>
                    </Tip>
                    <Tip label="Not helpful" sub="Feeds the credibility flywheel" position="top">
                      <button
                        onClick={() => void chat.vote(m.id, 'down')}
                        aria-label="Mark not helpful"
                        aria-pressed={votes[m.id] === 'down'}
                        className={`p-0.5 rounded transition-colors focus-visible:ring-1 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                          votes[m.id] === 'down' ? 'text-[var(--color-danger)]' : 'text-[var(--color-faint)] hover:text-[var(--color-text)]'
                        }`}
                      >
                        <ThumbsDown className="w-3 h-3" />
                      </button>
                    </Tip>
                  </>
                )}
              </div>

              {editingId === m.id && isUser ? (
                <div className="w-full max-w-[92%] rounded-2xl bg-[var(--color-surface)] border border-[var(--color-accent-edge)] p-2.5 space-y-2 self-end">
                  <textarea
                    autoFocus
                    value={editDraft}
                    onChange={(e) => setEditDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) void handleRewriteSave(m.id);
                      if (e.key === 'Escape') setEditingId(null);
                    }}
                    rows={3}
                    maxLength={4000}
                    className="w-full bg-transparent text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none resize-y"
                  />
                  <div className="flex items-center justify-end gap-1.5">
                    <button
                      onClick={() => setEditingId(null)}
                      disabled={rewriting}
                      className="px-2.5 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] text-[var(--color-label-secondary)] text-[12px] transition-all disabled:opacity-40"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={() => void handleRewriteSave(m.id)}
                      disabled={rewriting || !editDraft.trim()}
                      className="px-2.5 py-1 rounded-lg bg-[var(--color-accent-soft)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-[12px] font-semibold transition-all disabled:opacity-40"
                    >
                      {rewriting ? 'Resending…' : 'Resend (branches thread)'}
                    </button>
                  </div>
                </div>
              ) : (
                !hasTribunal && (
                  <div
                    className={`px-3.5 py-2.5 text-xs leading-relaxed max-w-[92%] ${
                      isUser
                        ? 'rounded-2xl rounded-tr-sm bg-[var(--color-accent-soft)] border border-[var(--color-accent-edge)] text-[var(--color-text)]'
                        : 'rounded-2xl rounded-tl-sm bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-label-secondary)] shadow-[var(--shadow-raised)]'
                    }`}
                  >
                    <MathText text={m.content} />
                    {m.id === streamingId && <span className="text-[var(--color-accent-ink)] animate-pulse"> ▍</span>}
                  </div>
                )
              )}
              {!isUser && renderProposalCard(m)}

              {hasTribunal && m.tribunal && (
                <div className="w-full space-y-2">
                  <div className="flex items-center gap-1.5 text-[12px] font-code text-[var(--color-accent-ink)] px-1">
                    <Bot className="w-3 h-3" />
                    <span>TRIBUNAL SYNTHESIS</span>
                  </div>
                  <div className="w-full rounded-2xl rounded-tl-sm bg-[var(--color-surface)] border border-[var(--color-border)] p-3.5 space-y-4 shadow-[var(--shadow-raised)]">
                    <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
                      <div className="flex items-center space-x-2.5">
                        <div className="w-7 h-7 rounded-xl bg-[var(--color-soft-indigo-wash)] border border-[var(--color-soft-indigo-edge)] flex items-center justify-center">
                          <Scale className="w-3.5 h-3.5 text-[var(--color-info)]" />
                        </div>
                        <div>
                          <div className="font-bold text-[var(--color-text)] text-xs">Autonomous Tribunal Deliberation</div>
                          <div className="text-[12px] text-[var(--color-muted)] font-mono">
                            Target: {m.tribunal.target || m.tribunal.molecule}
                          </div>
                        </div>
                      </div>

                      <span
                        className={`px-2.5 py-0.5 rounded-full text-[12px] font-mono font-bold border flex items-center space-x-1.5 ${
                          m.tribunal.consensus_passed
                            ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success)]'
                            : 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning)]'
                        }`}
                      >
                        {m.tribunal.consensus_passed ? <CheckCircle2 className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
                        <span>{m.tribunal.consensus_passed ? 'CONSENSUS VERIFIED' : 'CONTESTED'}</span>
                      </span>
                    </div>

                    <div className="flex items-center justify-between">
                      <span className="text-[12px] text-[var(--color-muted)]">{m.tribunal.turns.length} turns</span>
                      <div className="flex gap-2">
                        <button
                          onClick={() =>
                            setExpandedTurns(
                              Object.fromEntries((m.tribunal?.turns ?? []).map((_, idx) => [`${m.id}-turn-${idx}`, true])),
                            )
                          }
                          className="text-[12px] text-[var(--color-accent-ink)] hover:underline"
                        >
                          Expand all
                        </button>
                        <button onClick={() => setExpandedTurns({})} className="text-[12px] text-[var(--color-muted)] hover:text-[var(--color-text)]">
                          Collapse all
                        </button>
                      </div>
                    </div>

                    <div className="space-y-2.5">
                      {m.tribunal.turns.map((turn, idx) => {
                        const turnKey = `${m.id}-turn-${idx}`;
                        const isTurnExpanded = expandedTurns[turnKey] === true;

                        let accentColor = 'border-[var(--color-accent-edge)] text-[var(--color-accent-ink)]';
                        let roleIcon = Atom;
                        let roleTitle = 'Ansatz Scout';

                        if (turn.role.includes('disconfirmation') || turn.name.includes('Disconfirmation')) {
                          accentColor = 'border-[var(--color-warning-edge)] text-[var(--color-warning)]';
                          roleIcon = Scale;
                          roleTitle = 'Disconfirmation Officer';
                        } else if (turn.role.includes('auditor') || turn.name.includes('Auditor')) {
                          accentColor = 'border-[var(--color-soft-indigo-edge)] text-[var(--color-info)]';
                          roleIcon = Cpu;
                          roleTitle = 'Device Auditor';
                        } else if (turn.role.includes('scribe') || turn.name.includes('Scribe')) {
                          accentColor = 'border-[var(--color-success-edge)] text-[var(--color-success)]';
                          roleIcon = BookOpen;
                          roleTitle = 'Epistemic Scribe';
                        }

                        const RoleIconComponent = roleIcon;

                        return (
                          <div
                            key={idx}
                            className="rounded-2xl bg-[var(--color-fill)] border border-[var(--color-border)] p-3.5 space-y-2 hover:border-[var(--color-border-strong)] transition-all"
                          >
                            <button
                              onClick={() => toggleTurnExpand(turnKey)}
                              aria-expanded={isTurnExpanded}
                              aria-label={`${isTurnExpanded ? 'Collapse' : 'Expand'} turn: ${turn.name || roleTitle}`}
                              className="flex items-center justify-between cursor-pointer w-full text-left"
                            >
                              <div className="flex items-center space-x-2">
                                <RoleIconComponent className={`w-3.5 h-3.5 ${accentColor}`} />
                                <span className="font-bold text-[var(--color-text)] text-xs tracking-tight">{turn.name || roleTitle}</span>
                                <span className={`text-[12px] font-mono uppercase px-1.5 py-0.2 rounded border bg-[var(--color-field)] ${accentColor}`}>
                                  {turn.verdict || 'ANALYZED'}
                                </span>
                              </div>

                              <Tip label={isTurnExpanded ? 'Collapse turn' : 'Expand turn'} position="left">
                                <span className="text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors inline-flex">
                                  {isTurnExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                                </span>
                              </Tip>
                            </button>

                            {isTurnExpanded && (
                              <div className="text-xs text-[var(--color-label-secondary)] leading-relaxed font-sans pt-1 border-t border-[var(--color-border)]">
                                <MathText text={turn.content} />
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>

                    {m.tribunal.synthesis && (
                      <div className="p-3 rounded-2xl bg-[var(--color-raised)] border border-[var(--color-border)] space-y-2">
                        <div className="flex items-center space-x-1.5 text-xs font-bold text-[var(--color-success)]">
                          <Sparkles className="w-3.5 h-3.5" />
                          <span>Tribunal Synthesis &amp; Proposed Action</span>
                        </div>
                        <div className="text-xs text-[var(--color-label-secondary)] leading-relaxed font-sans">
                          <MathText text={m.tribunal.synthesis} />
                        </div>

                        <div className="pt-2 flex flex-wrap items-center gap-2">
                          {onSelectSurface && (
                            <button
                              onClick={() => onSelectSurface('surface1')}
                              className="px-3 py-1.5 rounded-xl bg-[var(--color-accent-soft)] hover:bg-[var(--color-accent)] hover:text-[var(--color-on-accent)] border border-[var(--color-accent-edge)] text-[var(--color-accent-ink)] text-[12px] font-semibold flex items-center space-x-1.5 transition-all"
                            >
                              <Atom className="w-3.5 h-3.5" />
                              <span>View on Living Canvas</span>
                            </button>
                          )}
                          {onSelectSurface && (
                            <button
                              onClick={() => onSelectSurface('surface7')}
                              className="px-3 py-1.5 rounded-xl bg-[var(--color-soft-indigo-wash)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-soft-indigo-edge)] text-[var(--color-info)] text-[12px] font-semibold flex items-center space-x-1.5 transition-all"
                            >
                              <BookOpen className="w-3.5 h-3.5" />
                              <span>Pin to Living Paper</span>
                            </button>
                          )}
                          <button
                            onClick={() => copyToClipboard(m.tribunal?.synthesis || '', m.id)}
                            className="px-3 py-1.5 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-[12px] font-semibold flex items-center space-x-1.5 transition-all"
                          >
                            {copiedId === m.id ? <Check className="w-3.5 h-3.5 text-[var(--color-success)]" /> : <Copy className="w-3.5 h-3.5" />}
                            <span>{copiedId === m.id ? 'Copied' : 'Copy Synthesis'}</span>
                          </button>
                        </div>

                        {onMaterialize && (
                          <div className="pt-1 flex flex-wrap items-center gap-1.5">
                            <button
                              onClick={() => onMaterialize('circuit', 'Circuit · live')}
                              className="px-2.5 py-1 rounded-full bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] transition-all flex items-center gap-1"
                            >
                              <span className="text-[var(--color-accent-ink)]">✦</span> Inspect circuit
                            </button>
                            <button
                              onClick={() => onMaterialize('shadows', 'Shadow estimates')}
                              className="px-2.5 py-1 rounded-full bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] transition-all flex items-center gap-1"
                            >
                              <span className="text-[var(--color-accent-ink)]">✦</span> Shadow estimates
                            </button>
                            <button
                              onClick={() => onMaterialize('molecule', 'Active space')}
                              className="px-2.5 py-1 rounded-full bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] transition-all flex items-center gap-1"
                            >
                              <span className="text-[var(--color-warning)]">✦</span> Active space
                            </button>
                            <button
                              onClick={() => onMaterialize('mandate', 'Spend mandate')}
                              className="px-2.5 py-1 rounded-full bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] transition-all flex items-center gap-1"
                            >
                              <span className="text-[var(--color-warning)]">✦</span> Spend card
                            </button>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  {!isUser && renderProposalCard(m)}
                </div>
              )}
            </div>
          );
        })}

        {(loading || runningTribunal) && (
          <div className="p-4 rounded-3xl bg-[var(--color-surface)] border border-[var(--color-accent-edge)] space-y-2.5 shadow-[var(--shadow-raised)]">
            <div className="flex items-center justify-between text-xs font-mono text-[var(--color-accent-ink)]">
              <div className="flex items-center space-x-2">
                <Sparkles className="w-4 h-4 animate-spin text-[var(--color-accent-ink)]" />
                <span className="font-bold">Autonomous Scientific Reasoning</span>
              </div>
              <span>{elapsedSeconds}s</span>
            </div>
            <p className="text-xs text-[var(--color-label-secondary)] leading-relaxed font-sans">
              {runningTribunal ? tribunalStage : 'Reasoning with the configured model…'}
            </p>
          </div>
        )}

        {needsLogin && (
          <div
            className="p-3.5 rounded-2xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning)] text-xs font-sans flex items-center justify-between gap-2"
            role="alert"
          >
            <span>
              {authExpired
                ? 'Session expired — log in again to start a campaign session. No session was created.'
                : authPhase === 'forbidden'
                  ? 'This lab forbids the requested write — sign in with an authorized account. No session was created.'
                  : 'Sign in to start a campaign session. No session was created.'}
            </span>
            <button
              type="button"
              onClick={() => onSelectSurface?.('labs')}
              className="shrink-0 px-2.5 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-warning-edge)] text-[var(--color-text)] text-[12px] font-semibold transition-all"
            >
              Sign in
            </button>
          </div>
        )}

        {errorMessage && (
          <div
            className="p-3.5 rounded-2xl bg-[var(--color-soft-rose-wash)] border border-[var(--color-soft-rose-edge)] text-[var(--color-danger)] text-xs font-sans"
            role="alert"
          >
            <div className="flex items-start justify-between gap-2">
              <span>{errorMessage}</span>
              {/stream stalled/i.test(errorMessage) && (
                <button
                  type="button"
                  aria-label="Retry stalled stream"
                  onClick={() => chat.retryStalled()}
                  className="shrink-0 px-2.5 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-soft-rose-edge)] text-[var(--color-text)] text-[12px] font-semibold transition-all"
                >
                  Retry
                </button>
              )}
            </div>
          </div>
        )}

        {infoMessage && (
          <div className="p-3.5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs font-sans">
            {infoMessage}
          </div>
        )}

        {chatWriteLabel && (
          <p role="status" className="px-1 text-[12px] font-mono text-[var(--color-muted)]">
            {chatWriteLabel}
          </p>
        )}
        {feedbackWriteLabel && (
          <p role="status" className="px-1 text-[12px] font-mono text-[var(--color-muted)]">
            {feedbackWriteLabel}
          </p>
        )}

        <div ref={messagesEndRef} />
      </div>

      {!dockComposer && (
        <div className="shrink-0 p-4 border-t border-[var(--color-border)] bg-[var(--color-chrome)] space-y-2">
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            {followUps.map((f) => (
              <button
                key={f.label}
                onClick={() => chat.runFollowUp(f.key)}
                title={f.label}
                className="max-w-full min-w-0 px-2.5 py-1 rounded-full bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] transition-all flex items-center gap-1"
              >
                <span className={`shrink-0 ${f.tone === 'success' ? 'text-[var(--color-success)]' : f.tone === 'warning' ? 'text-[var(--color-warning)]' : 'text-[var(--color-accent-ink)]'}`}>✦</span>
                <span className="truncate">{f.label}</span>
              </button>
            ))}
          </div>

          <MemoryPanel sessionId={sessionId} />

          <form
            onSubmit={(e) => {
              e.preventDefault();
              const textToSend = input.trim();
              if (!textToSend) return;
              setInput('');
              void chat.send(textToSend);
            }}
            className="relative bg-[var(--color-surface)] border border-[var(--color-border)] rounded-2xl p-2 focus-within:border-[var(--color-accent-edge)] transition-all"
          >
            {attachments.length > 0 && (
              <div className="flex flex-wrap gap-1.5 px-2 pb-1.5">
                {attachments.map((a) => (
                  <span
                    key={a.name}
                    className="flex items-center gap-1 px-2 py-0.5 rounded-full bg-[var(--color-accent-soft)] border border-[var(--color-accent-edge)] text-[12px] font-code text-[var(--color-accent-ink)]"
                  >
                    <Paperclip className="w-3 h-3" />
                    {a.name}
                    <button type="button" onClick={() => chat.detach(a.name)} aria-label={`Remove ${a.name}`} className="hover:text-[var(--color-text)]">
                      <X className="w-3 h-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            {input.startsWith('/') && (
              <div className="px-2 pb-1.5 text-[12px] font-code text-[var(--color-muted)]">
                <span className="text-[var(--color-accent-ink)]">/tribunal</span> convene ·{' '}
                <span className="text-[var(--color-accent-ink)]">/launch</span> last proposal ·{' '}
                <span className="text-[var(--color-accent-ink)]">/help</span>
              </div>
            )}
            <textarea
              rows={2}
              value={input}
              aria-label="Chat message"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  if (e.nativeEvent?.isComposing || e.keyCode === 229) return;
                  const textToSend = input.trim();
                  if (!textToSend) return;
                  setInput('');
                  void chat.send(textToSend);
                }
              }}
              placeholder="Command swarm, query proofs, or adjust target..."
              className="w-full bg-transparent text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none resize-none px-2 py-1 font-sovereign"
            />
            <input
              ref={attachInputRef}
              type="file"
              accept=".pdb,.cif,.xyz,.mol,.sdf,.txt,.md"
              aria-label="Attach molecular structure or notes"
              className="hidden"
              onChange={handleAttachFile}
            />
            <div className="px-0 pt-1.5">
              <OverflowCard
                sessionId={sessionId}
                refreshKey={messages.length}
                onSessionChanged={(id) => onSessionChange?.(id)}
              />
            </div>
            <div className="flex items-center justify-between pt-1 border-t border-[var(--color-border)] px-1">
              <div className="flex items-center gap-1">
                <Tip label="Attach file" sub="PDB · CIF · XYZ · text" position="top">
                  <button
                    type="button"
                    onClick={() => attachInputRef.current?.click()}
                    aria-label="Attach molecular file (PDB/CIF/XYZ)"
                    className="p-1 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-all focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
                  >
                    <Paperclip className="w-4 h-4" />
                  </button>
                </Tip>
                <span className="text-[12px] font-code text-[var(--color-muted)] ml-1">⌘K</span>
                <ContextReadout sessionId={sessionId} refreshKey={messages.length} />
              </div>
              <Tip label={streamingId ? 'Stop generation' : 'Send directive'} position="top">
                <button
                  type={streamingId ? 'button' : 'submit'}
                  onClick={streamingId ? chat.stop : undefined}
                  disabled={!streamingId && (!input.trim() || loading || runningTribunal)}
                  aria-label={streamingId ? 'Stop generation' : 'Send directive'}
                  className={`w-7 h-7 rounded-xl flex items-center justify-center transition-all focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none ${
                    streamingId
                      ? 'bg-[var(--color-danger)] text-[var(--color-on-accent)]'
                      : 'bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] disabled:opacity-40'
                  }`}
                >
                  {streamingId ? <span className="w-3 h-3 rounded-[2px] bg-current" /> : <ArrowUp className="w-4 h-4 font-bold" />}
                </button>
              </Tip>
            </div>
          </form>

          <button
            type="button"
            onClick={() => void chat.conveneTribunal(input.trim() || `${targetSystem || targetDisplayName || 'Active campaign'} ground state electronic structure`)}
            disabled={loading || runningTribunal}
            className="w-full py-2 px-3 rounded-2xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-xs text-[var(--color-label-secondary)] font-semibold flex items-center justify-center space-x-2 transition-all disabled:opacity-40 active:scale-[0.99]"
          >
            <Sparkles className="w-3.5 h-3.5 text-[var(--color-accent-ink)]" />
            <span>Convene Multi-Agent Research Tribunal</span>
          </button>
          <div className="text-center">
            <span className="text-[12px] text-[var(--color-muted)] font-code">Autonomous AI Swarm · Local-first</span>
          </div>
        </div>
      )}

      {clearConfirmOpen && (
        <ConfirmDialog
          open
          title="Clear this conversation?"
          body={
            <span>
              Messages are deleted; the session, campaign, and ledger history survive. Type{' '}
              <code className="rounded bg-[var(--color-fill)] px-1 font-mono">CLEAR</code> to confirm.
            </span>
          }
          requireText="CLEAR"
          confirmLabel="Clear conversation"
          danger
          busy={clearing}
          onConfirm={() => void confirmClearChat()}
          onCancel={() => {
            if (!clearing) setClearConfirmOpen(false);
          }}
        />
      )}
    </div>
  );
});
