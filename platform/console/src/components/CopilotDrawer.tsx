'use client';

import React, { useState, useEffect, useRef,  useMemo } from 'react';
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
} from 'lucide-react';
import { ChatMessage, CampaignState, RunPhase, Proposal } from '@/types';
import { fetchMessages, sendChatMessage, streamChatMessage, createSession, runTribunal, fetchCampaignState, launchProposal } from '@/lib/api';
import { MathText } from './common/MathText';
import type { CanvasWidgetKind } from './sovereign/WidgetStage';

interface CopilotDrawerProps {
  sessionId: string;
  campaign?: CampaignState | null;
  isExpanded?: boolean;
  onToggleExpand?: () => void;
  onExperimentLaunched?: () => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
  onSelectSurface?: (surface: string) => void;
  onMaterialize?: (kind: CanvasWidgetKind, title: string) => void;
  onRunStateChange?: (phase: RunPhase, detail: string) => void;
  onClose?: () => void;
}

export function CopilotDrawer({
  sessionId,
  campaign,
  isExpanded: externalExpanded,
  onToggleExpand,
  onExperimentLaunched,
  onCampaignUpdated,
  onSelectSurface,
  onMaterialize,
  onRunStateChange,
  onClose,
}: CopilotDrawerProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [runningTribunal, setRunningTribunal] = useState(false);
  const [tribunalStage, setTribunalStage] = useState<string>('Convening multi-agent scientific tribunal...');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [internalExpanded, setInternalExpanded] = useState(false);
  const isExpanded = externalExpanded !== undefined ? externalExpanded : internalExpanded;
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [expandedTurns, setExpandedTurns] = useState<Record<string, boolean>>({});
  const [launchingId, setLaunchingId] = useState<string | null>(null);
  const [launchedNote, setLaunchedNote] = useState<Record<string, string>>({});
  const [streamingId, setStreamingId] = useState<string | null>(null);

  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const attachInputRef = useRef<HTMLInputElement>(null);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  const handleToggleExpand = () => {
    if (onToggleExpand) {
      onToggleExpand();
    } else {
      setInternalExpanded((prev) => !prev);
    }
  };

  const scrollToBottom = () => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTo({
        top: messagesContainerRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, loading, runningTribunal]);

  useEffect(() => {
    if (sessionId) {
      fetchMessages(sessionId).then((msgs) => {
        if (msgs && msgs.length > 0) {
          setMessages(msgs);
        }
      });
    }
  }, [sessionId]);

  useEffect(() => {
    if (loading || runningTribunal) {
      const initTimer = setTimeout(() => setElapsedSeconds(0), 0);
      timerRef.current = setInterval(() => {
        setElapsedSeconds((prev) => prev + 1);
      }, 1000);
      return () => {
        clearTimeout(initTimer);
        if (timerRef.current) clearInterval(timerRef.current);
      };
    }
  }, [loading, runningTribunal]);

  const getActiveSessionId = async (): Promise<string> => {
    if (sessionId) return sessionId;
    const newSess = await createSession('Quantum Research Campaign #1', 150.0);
    return newSess ? newSess.id : '';
  };

  const handleSend = async (customPrompt?: string) => {
    const textToSend = customPrompt || input.trim();
    if (!textToSend || loading || runningTribunal) return;

    if (!customPrompt) setInput('');
    setErrorMessage(null);

    const activeSessionId = await getActiveSessionId();

    const tempUserMsg: ChatMessage = {
      id: `temp-user-${Date.now()}`,
      session_id: activeSessionId,
      role: 'user',
      content: textToSend,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);
    setLoading(true);
    onRunStateChange?.('thinking', textToSend);

    // Live bubble: tokens stream into it; the done event replaces it with
    // the stored (sanitized) records. Unary fallback on stream failure.
    const liveId = `live-assistant-${Date.now()}`;
    setMessages((prev) => [
      ...prev,
      {
        id: liveId,
        session_id: activeSessionId,
        role: 'assistant',
        content: '',
        created_at: new Date().toISOString(),
      },
    ]);
    setStreamingId(liveId);

    const settleStream = (done: { user: ChatMessage; assistant: ChatMessage; proposal?: unknown; campaign?: CampaignState }) => {
      setMessages((prev) => {
        const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
        const assistant: ChatMessage = { ...done.assistant };
        if (!assistant.proposal_json && done.proposal) {
          assistant.proposal_json = done.proposal as Proposal;
        }
        return [...filtered, done.user, assistant];
      });
      if (done.campaign && onCampaignUpdated) {
        onCampaignUpdated(done.campaign);
      } else if (onCampaignUpdated) {
        fetchCampaignState(activeSessionId).then((c) => {
          if (c) onCampaignUpdated(c);
        });
      }
      if (onExperimentLaunched) onExperimentLaunched();
    };

    try {
      const done = await streamChatMessage(activeSessionId, textToSend, (tok) => {
        setMessages((prev) =>
          prev.map((m) => (m.id === liveId ? { ...m, content: m.content + tok } : m))
        );
      });
      settleStream(done);
    } catch (streamErr) {
      console.warn('Stream failed, falling back to unary chat:', streamErr);
      try {
        const res = await sendChatMessage(activeSessionId, textToSend);
        if (res) {
          setMessages((prev) => {
            const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
            return [...filtered, res.user, res.assistant];
          });
          if (onCampaignUpdated) {
            fetchCampaignState(activeSessionId).then((c) => {
              if (c) onCampaignUpdated(c);
            });
          }
          if (onExperimentLaunched) onExperimentLaunched();
        } else {
          setMessages((prev) => prev.filter((m) => m.id !== liveId));
          setErrorMessage('Local AI engine did not respond. Verify that oMLX is listening on port 8787.');
        }
      } catch (err: unknown) {
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        setErrorMessage(err instanceof Error ? err.message : 'Connection error to backend.');
      }
    } finally {
      setLoading(false);
      setStreamingId(null);
      onRunStateChange?.('idle', '');
    }
  };

  const handleRunTribunal = async (topic: string) => {
    if (runningTribunal || loading) return;
    setRunningTribunal(true);
    setTribunalStage('Ansatz Scout: Proposing low-depth quantum circuit parameterization...');
    onRunStateChange?.('tribunal', 'Ansatz Scout: proposing parameterization…');
    setErrorMessage(null);

    const activeSessionId = await getActiveSessionId();

    const tempUserMsg: ChatMessage = {
      id: `temp-tribunal-${Date.now()}`,
      session_id: activeSessionId,
      role: 'user',
      content: `Convene Research Tribunal for: ${topic}`,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);

    // Simulated animated stage progression while waiting for local LLM
    const stageTimer1 = setTimeout(() => {
      setTribunalStage('Disconfirmation Officer: Benchmarking against PySCF FCI classical anchor...');
      onRunStateChange?.('tribunal', 'Disconfirmation Officer: benchmarking anchor…');
    }, 2000);
    const stageTimer2 = setTimeout(() => {
      setTribunalStage('Device Auditor: Testing Clifford RB canary on simulated Heron backend...');
      onRunStateChange?.('tribunal', 'Device Auditor: canary on simulator…');
    }, 4500);
    const stageTimer3 = setTimeout(() => {
      setTribunalStage('Epistemic Scribe: Collapsing credibility state & synthesizing consensus...');
      onRunStateChange?.('tribunal', 'Epistemic Scribe: synthesizing…');
    }, 7000);

    try {
      const res = await runTribunal(activeSessionId, topic);
      if (res && res.assistant_message) {
        const assistantWithTribunal: ChatMessage = {
          ...res.assistant_message,
          tribunal: res.tribunal,
        };

        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== tempUserMsg.id);
          return [...filtered, tempUserMsg, assistantWithTribunal];
        });

        if (res.campaign && onCampaignUpdated) {
          onCampaignUpdated(res.campaign);
        } else if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (c) onCampaignUpdated(c);
          });
        }
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setErrorMessage('Failed to convene tribunal. Verify backend gRPC bridge and local oMLX daemon.');
      }
    } catch (err: unknown) {
      setErrorMessage(err instanceof Error ? err.message : 'Tribunal connection error.');
    } finally {
      clearTimeout(stageTimer1);
      clearTimeout(stageTimer2);
      clearTimeout(stageTimer3);
      setRunningTribunal(false);
      onRunStateChange?.('idle', '');
    }
  };

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
      const trimmed = text.length > 4000 ? text.slice(0, 4000) + '\n…[truncated]' : text;
      setInput((prev) => `${prev}${prev ? '\n' : ''}[attached ${file.name}]\n${trimmed}\n`);
    } catch {
      setErrorMessage(`Could not read ${file.name}: binary or unreadable file.`);
    }
  };

  const toggleTurnExpand = (turnKey: string) => {
    setExpandedTurns((prev) => ({
      ...prev,
      [turnKey]: !prev[turnKey],
    }));
  };

  const handleLaunchProposal = async (messageId: string, proposal: Proposal) => {
    if (launchingId) return;
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    setLaunchingId(messageId);
    try {
      const res = await launchProposal(activeSessionId, proposal);
      if (res?.job) {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launched: job ${String(res.job.id).slice(0, 8)}` }));
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch refused by backend — nothing dispatched.' }));
      }
    } catch {
      setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch failed — nothing dispatched.' }));
    } finally {
      setLaunchingId(null);
    }
  };

  const renderProposalCard = (m: ChatMessage) => {
    if (!m.proposal_json) return null;
    const p = m.proposal_json;
    const note = launchedNote[m.id];
    return (
      <div className="w-full max-w-[95%] rounded-2xl bg-black/40 border border-cyan-400/25 p-3 space-y-2">
        <div className="flex items-center gap-1.5 text-[10px] font-code text-cyan-400">
          <span>✦</span>
          <span>LAUNCHABLE PROPOSAL</span>
        </div>
        <div className="text-xs text-white font-medium">{p.target}</div>
        <div className="grid grid-cols-2 gap-2 text-[10px] font-code text-slate-400">
          <div><span className="text-white/40 block">Tier</span><span className="text-cyan-200">{p.recommended_tier}</span></div>
          <div><span className="text-white/40 block">Spend</span><span className="text-amber-300">{p.spend_estimate}</span></div>
        </div>
        <div className="text-[10px] font-code text-slate-500 truncate" title={p.classical_anchor}>
          anchor · {p.classical_anchor}
        </div>
        {note ? (
          <div className="text-[11px] font-code text-emerald-300">{note}</div>
        ) : (
          <button
            onClick={() => handleLaunchProposal(m.id, p)}
            disabled={launchingId === m.id}
            className="w-full py-1.5 px-3 rounded-xl bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-400/30 text-cyan-200 text-[11px] font-semibold transition-all disabled:opacity-50"
          >
            {launchingId === m.id ? 'Dispatching…' : 'Launch on local simulator'}
          </button>
        )}
      </div>
    );
  };

  const targetSystem = campaign?.target_system || 'H2';
  const targetDisplayName = useMemo(() => {
    const raw = campaign?.target_system || 'H2';
    if (raw.toUpperCase() === 'H2' || raw === 'H₂') return 'Hydrogen Molecule (H₂)';
    if (raw.toUpperCase().includes('LIH')) return 'Lithium Hydride (LiH)';
    if (raw.toUpperCase().includes('BEH2')) return 'Beryllium Hydride (BeH₂)';
    if (raw.toUpperCase().includes('H2O')) return 'Water Molecule (H₂O)';
    if (raw.toUpperCase().includes('N2')) return 'Dinitrogen (N₂)';
    if (raw.toUpperCase().includes('CO')) return 'Carbon Monoxide (CO)';
    return raw;
  }, [campaign?.target_system]);
  const discrepancy = campaign?.classical_anchor?.discrepancy_mha;
  const isChemicalAccuracyMet = discrepancy != null && discrepancy <= 1.6;

  return (
    <div
      className="flex flex-col h-full w-full bg-[#0A0C10]/95 backdrop-blur-2xl text-slate-100 select-none font-sovereign overflow-hidden"
    >
      {/* Header: Kubona Copilot identity */}
      <div className="px-5 py-4 border-b border-white/5 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="w-2.5 h-2.5 rounded-full bg-cyan-400 animate-pulse" />
          <div>
            <div className="text-xs font-semibold text-white tracking-wide flex items-center gap-1.5">
              <span>KUBONA COPILOT</span>
              <span className="px-1.5 py-0.5 rounded bg-cyan-400/10 text-cyan-400 font-code text-[9px]">ACTIVE</span>
            </div>
            <div className="text-[10px] text-slate-400">
              Target: {targetDisplayName} · Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-1.5">
          {/* Spatial Expand/Contract Button */}
          <button
            onClick={handleToggleExpand}
            title={isExpanded ? 'Contract to Side Dock' : 'Expand to Spatial Salon'}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            {isExpanded ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5 text-cyan-400" />}
          </button>

          {/* Close Companion Button */}
          {onClose && (
            <button
              onClick={onClose}
              title="Close Copilot (⌘J)"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              <PanelRightClose className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Active Scientific Focus Ribbon */}
      <div className="mx-5 mt-3 px-3 py-2 rounded-xl bg-black/40 border border-white/[0.06] backdrop-blur-xl flex items-center justify-between text-[11px] font-code shrink-0">
        <div className="flex items-center space-x-2 text-slate-300">
          <span className="text-cyan-400 font-bold">TARGET:</span>
          <span className="truncate max-w-[180px]">{targetDisplayName}</span>
        </div>
        <div className="flex items-center space-x-1.5">
          <span className={`w-1.5 h-1.5 rounded-full ${isChemicalAccuracyMet ? 'bg-emerald-400' : 'bg-amber-400'}`} />
          <span className={isChemicalAccuracyMet ? 'text-emerald-300' : 'text-amber-300'}>
            Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
          </span>
        </div>
      </div>

      {/* Messages Feed */}
      <div
        ref={messagesContainerRef}
        className="flex-1 overflow-y-auto space-y-4 p-4 scrollbar-thin"
      >
        {messages.length === 0 && (
          <div className="p-3 rounded-xl bg-white/[0.02] border border-white/5 text-[11px] text-slate-400 leading-relaxed">
            <span className="text-white font-medium">Autonomous Swarm Initialized:</span> Exploring {targetDisplayName}. Type your intent or ask for telemetry breakdown on demand.
          </div>
        )}
        {messages.length === 0 && (
          <div className="space-y-2">

            {/* Quick Scientific Suggestion Cards */}
            <div className="pt-1 space-y-2 text-left">
              <button
                onClick={() => handleSend('Map H2 ground state potential energy surface across nuclear coordinates')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-cyan-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-amber-400 text-[10px]">✦</span>
                  <span className="font-medium">H₂ Potential Energy Surface</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-cyan-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleRunTribunal('Lithium Hydride (LiH) ground state electronic structure')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-emerald-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-emerald-400 text-[10px]">✦</span>
                  <span className="font-medium">Convene Tribunal for LiH</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-emerald-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleSend('Verify our variational ansatz on the IBM Heron QPU noise model with Clifford canary')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-cyan-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-cyan-400 text-[10px]">✦</span>
                  <span className="font-medium">Probe IBM Heron Noise Model</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-cyan-300 group-hover:translate-x-0.5 transition-all" />
              </button>
            </div>
          </div>
        )}

        {/* Render Conversation Stream */}
        {messages.map((m) => {
          const isUser = m.role === 'user';
          const hasTribunal = Boolean(m.tribunal && m.tribunal.turns && m.tribunal.turns.length > 0);

          return (
            <div
              key={m.id}
              className={`flex flex-col space-y-1.5 ${isUser ? 'items-end' : 'items-start'}`}
            >
              {/* Message Meta Info */}
              <div className="flex items-center space-x-2 text-[10px] text-slate-400 font-mono px-1">
                <span>{isUser ? 'You' : 'Kubona Tribunal Salon'}</span>
                <span>•</span>
                <span>{new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
              </div>

              {/* Standard Message Bubble */}
              {!hasTribunal && (
                <div
                  className={`px-3.5 py-2.5 text-xs leading-relaxed max-w-[92%] ${
                    isUser
                      ? 'rounded-2xl rounded-tr-sm bg-cyan-500/15 border border-cyan-400/30 text-white'
                      : 'rounded-2xl rounded-tl-sm bg-[#13161F] border border-white/10 text-slate-200 shadow-lg'
                  }`}
                >
                  <MathText text={m.content} />
                  {m.id === streamingId && <span className="text-cyan-400 animate-pulse"> ▍</span>}
                </div>
              )}
              {!isUser && renderProposalCard(m)}

              {/* Multi-Agent Tribunal Salon Presentation */}
              {hasTribunal && m.tribunal && (
                <div className="w-full space-y-2">
                  <div className="flex items-center gap-1.5 text-[10px] font-code text-cyan-400 px-1">
                    <Bot className="w-3 h-3" />
                    <span>TRIBUNAL SYNTHESIS</span>
                  </div>
                  <div className="w-full rounded-2xl rounded-tl-sm bg-[#13161F] border border-white/10 p-3.5 space-y-4 shadow-lg">
                  {/* Tribunal Header Banner */}
                  <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
                    <div className="flex items-center space-x-2.5">
                      <div className="w-7 h-7 rounded-xl bg-indigo-500/20 border border-indigo-500/30 flex items-center justify-center">
                        <Scale className="w-3.5 h-3.5 text-indigo-400" />
                      </div>
                      <div>
                        <div className="font-bold text-white text-xs">Autonomous Tribunal Deliberation</div>
                        <div className="text-[10px] text-slate-400 font-mono">Target: {m.tribunal.target || m.tribunal.molecule}</div>
                      </div>
                    </div>

                    <span
                      className={`px-2.5 py-0.5 rounded-full text-[10px] font-mono font-bold border flex items-center space-x-1.5 ${
                        m.tribunal.consensus_passed
                          ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300'
                          : 'bg-amber-950/70 border-amber-500/40 text-amber-300'
                      }`}
                    >
                      <CheckCircle2 className="w-3 h-3" />
                      <span>{m.tribunal.consensus_passed ? 'CONSENSUS VERIFIED' : 'DELIBERATING'}</span>
                    </span>
                  </div>

                  {/* 4 Colleague Discourse Cards */}
                  <div className="space-y-2.5">
                    {m.tribunal.turns.map((turn, idx) => {
                      const turnKey = `${m.id}-turn-${idx}`;
                      const isTurnExpanded = expandedTurns[turnKey] !== false; // Default expanded

                      let accentColor = 'border-cyan-500/40 text-cyan-300';
                      let roleIcon = Atom;
                      let roleTitle = 'Ansatz Scout';

                      if (turn.role.includes('disconfirmation') || turn.name.includes('Disconfirmation')) {
                        accentColor = 'border-amber-500/40 text-amber-300';
                        roleIcon = Scale;
                        roleTitle = 'Disconfirmation Officer';
                      } else if (turn.role.includes('auditor') || turn.name.includes('Auditor')) {
                        accentColor = 'border-indigo-500/40 text-indigo-300';
                        roleIcon = Cpu;
                        roleTitle = 'Device Auditor';
                      } else if (turn.role.includes('scribe') || turn.name.includes('Scribe')) {
                        accentColor = 'border-emerald-500/40 text-emerald-300';
                        roleIcon = BookOpen;
                        roleTitle = 'Epistemic Scribe';
                      }

                      const RoleIconComponent = roleIcon;

                      return (
                        <div
                          key={idx}
                          className="rounded-2xl bg-white/[0.02] border border-white/[0.06] p-3.5 space-y-2 hover:border-white/15 transition-all"
                        >
                          {/* Colleague Header */}
                          <div
                            onClick={() => toggleTurnExpand(turnKey)}
                            className="flex items-center justify-between cursor-pointer"
                          >
                            <div className="flex items-center space-x-2">
                              <RoleIconComponent className={`w-3.5 h-3.5 ${accentColor}`} />
                              <span className="font-bold text-white text-xs tracking-tight">{turn.name || roleTitle}</span>
                              <span className={`text-[9px] font-mono uppercase px-1.5 py-0.2 rounded border bg-black/40 ${accentColor}`}>
                                {turn.verdict || 'ANALYZED'}
                              </span>
                            </div>

                            <button className="text-slate-400 hover:text-white transition-colors">
                              {isTurnExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                            </button>
                          </div>

                          {/* Colleague Content */}
                          {isTurnExpanded && (
                            <div className="text-xs text-slate-300 leading-relaxed font-sans pt-1 border-t border-white/[0.04]">
                              <MathText text={turn.content} />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>

                  {/* Consensus Synthesis */}
                  {m.tribunal.synthesis && (
                    <div className="p-3 rounded-2xl bg-[#13161F] border border-white/10 space-y-2">
                      <div className="flex items-center space-x-1.5 text-xs font-bold text-emerald-300">
                        <Sparkles className="w-3.5 h-3.5" />
                        <span>Tribunal Synthesis &amp; Proposed Action</span>
                      </div>
                      <div className="text-xs text-slate-200 leading-relaxed font-sans">
                        <MathText text={m.tribunal.synthesis} />
                      </div>

                      {/* Interactive Canvas & Notebook Artifact Buttons */}
                      <div className="pt-2 flex flex-wrap items-center gap-2">
                        {onSelectSurface && (
                          <button
                            onClick={() => onSelectSurface('surface1')}
                            className="px-3 py-1.5 rounded-xl bg-cyan-600/30 hover:bg-cyan-600/50 border border-cyan-500/40 text-cyan-200 text-[11px] font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
                          >
                            <Atom className="w-3.5 h-3.5" />
                            <span>View on Living Canvas</span>
                          </button>
                        )}
                        {onSelectSurface && (
                          <button
                            onClick={() => onSelectSurface('surface7')}
                            className="px-3 py-1.5 rounded-xl bg-indigo-600/30 hover:bg-indigo-600/50 border border-indigo-500/40 text-indigo-200 text-[11px] font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
                          >
                            <BookOpen className="w-3.5 h-3.5" />
                            <span>Pin to Living Paper</span>
                          </button>
                        )}
                        <button
                          onClick={() => copyToClipboard(m.tribunal?.synthesis || '', m.id)}
                          className="px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/10 border border-white/10 text-slate-300 text-[11px] font-semibold flex items-center space-x-1.5 transition-all"
                        >
                          {copiedId === m.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                          <span>{copiedId === m.id ? 'Copied' : 'Copy Synthesis'}</span>
                        </button>
                      </div>

                      {/* On-demand canvas dispatch: agents show, don't tell */}
                      {onMaterialize && (
                        <div className="pt-1 flex flex-wrap items-center gap-1.5">
                          <button
                            onClick={() => onMaterialize('circuit', 'Circuit · live')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-cyan-400">✦</span> Inspect circuit
                          </button>
                          <button
                            onClick={() => onMaterialize('shadows', 'Shadow estimates')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-cyan-400">✦</span> Shadow estimates
                          </button>
                          <button
                            onClick={() => onMaterialize('molecule', 'Active space')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-amber-400">✦</span> Active space
                          </button>
                          <button
                            onClick={() => onMaterialize('mandate', 'Spend mandate')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-amber-400">✦</span> Spend card
                          </button>
                        </div>
                      )}
                    </div>
                  )}
                  </div>
              {!isUser && renderProposalCard(m)}
              {!isUser && m.model && (
                <span className="text-[9px] font-code text-slate-600 px-1">served by {m.model}</span>
              )}
                </div>
              )}
            </div>
          );
        })}

        {/* Live Loading & Multi-Stage Reasoning Aura */}
        {(loading || runningTribunal) && (
          <div className="p-4 rounded-3xl bg-black/60 border border-cyan-500/30 space-y-2.5 animate-pulse shadow-xl shadow-cyan-950/30">
            <div className="flex items-center justify-between text-xs font-mono text-cyan-300">
              <div className="flex items-center space-x-2">
                <Sparkles className="w-4 h-4 animate-spin text-cyan-400" />
                <span className="font-bold">Autonomous Scientific Reasoning</span>
              </div>
              <span>{elapsedSeconds}s</span>
            </div>

            <p className="text-xs text-slate-300 leading-relaxed font-sans">
              {runningTribunal ? tribunalStage : `Reasoning with local oMLX model on port 8787...`}
            </p>
          </div>
        )}

        {errorMessage && (
          <div className="p-3.5 rounded-2xl bg-red-950/40 border border-red-800/40 text-red-300 text-xs font-sans">
            {errorMessage}
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Stage with Suggestion Pills */}
      <div className="shrink-0 p-4 border-t border-white/5 bg-[#0A0C10] space-y-2">
        {/* Quick Prompt Suggestion Strip */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 scrollbar-none text-xs">
          <button
            onClick={() => handleSend('Calculate potential energy surface for H2')}
            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1 whitespace-nowrap"
          >
            <span className="text-amber-400">✦</span> H₂ Bond PES
          </button>
          <button
            onClick={() => handleRunTribunal(`${targetDisplayName} equilibrium state`)}
            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1 whitespace-nowrap"
          >
            <span className="text-emerald-400">✦</span> {targetSystem} Tribunal
          </button>
          <button
            onClick={() => handleSend('Simulate Clifford RB canary on IBM Heron')}
            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1 whitespace-nowrap"
          >
            <span className="text-cyan-400">✦</span> Canary Fidelity
          </button>
          <button
            onClick={() => handleSend('Verify classical PySCF STO-3G ground truth')}
            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1 whitespace-nowrap"
          >
            <span className="text-amber-400">✦</span> Classical FCI Anchor
          </button>
        </div>

        {/* Composer Card */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            handleSend();
          }}
          className="relative bg-[#13161F] border border-white/10 rounded-2xl p-2 focus-within:border-cyan-400/60 focus-within:ring-1 focus-within:ring-cyan-400/40 transition-all shadow-inner"
        >
          <textarea
            rows={2}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Command swarm, query proofs, or adjust target..."
            className="w-full bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none resize-none px-2 py-1 font-sovereign"
          />
          <input
            ref={attachInputRef}
            type="file"
            accept=".pdb,.cif,.xyz,.mol,.sdf,.txt,.md"
            className="hidden"
            onChange={handleAttachFile}
          />
          <div className="flex items-center justify-between pt-1 border-t border-white/5 px-1">
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => attachInputRef.current?.click()}
                title="Attach molecular file (PDB/CIF/XYZ)"
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
              >
                <Paperclip className="w-4 h-4" />
              </button>
              <span className="text-[10px] font-code text-slate-600 ml-1">⌘K</span>
            </div>
            <button
              type="submit"
              disabled={!input.trim() || loading || runningTribunal}
              aria-label="Send directive"
              className="w-7 h-7 rounded-xl bg-cyan-400 hover:bg-cyan-300 text-black flex items-center justify-center transition-all shadow-[0_0_15px_rgba(0,229,255,0.3)] disabled:opacity-40 focus-visible:ring-2 focus-visible:ring-cyan-200 focus-visible:outline-none"
            >
              <ArrowUp className="w-4 h-4 font-bold" />
            </button>
          </div>
        </form>

        {/* Tribunal Action Button */}
        <button
          type="button"
          onClick={() => handleRunTribunal(input.trim() || `${targetSystem} ground state electronic structure`)}
          disabled={loading || runningTribunal}
          className="w-full py-2 px-3 rounded-2xl bg-white/5 hover:bg-white/10 border border-white/10 text-xs text-slate-200 font-semibold flex items-center justify-center space-x-2 transition-all disabled:opacity-40 active:scale-[0.99]"
        >
          <Sparkles className="w-3.5 h-3.5 text-cyan-400" />
          <span>Convene Multi-Agent Research Tribunal</span>
        </button>
        <div className="text-center">
          <span className="text-[9px] text-slate-600 font-code">Autonomous AI Swarm · Local-first</span>
        </div>
      </div>
    </div>
  );
}
