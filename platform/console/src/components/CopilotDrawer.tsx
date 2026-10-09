'use client';

import React, { useState, useEffect, useRef,  useMemo } from 'react';
import {
  Send,
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
  
  
  
  
} from 'lucide-react';
import { ChatMessage, CampaignState } from '@/types';
import { fetchMessages, sendChatMessage, createSession, runTribunal, fetchCampaignState } from '@/lib/api';
import { MathText } from './common/MathText';

interface CopilotDrawerProps {
  sessionId: string;
  campaign?: CampaignState | null;
  isExpanded?: boolean;
  onToggleExpand?: () => void;
  onExperimentLaunched?: () => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
  onSelectSurface?: (surface: string) => void;
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

  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
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

    try {
      const res = await sendChatMessage(activeSessionId, textToSend);
      if (res) {
        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== tempUserMsg.id);
          return [...filtered, res.user, res.assistant];
        });
        if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (c) onCampaignUpdated(c);
          });
        }
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setErrorMessage('Local AI engine did not respond. Verify that oMLX is listening on port 8787.');
      }
    } catch (err: unknown) {
      setErrorMessage(err instanceof Error ? err.message : 'Connection error to backend.');
    } finally {
      setLoading(false);
    }
  };

  const handleRunTribunal = async (topic: string) => {
    if (runningTribunal || loading) return;
    setRunningTribunal(true);
    setTribunalStage('Ansatz Scout: Proposing low-depth quantum circuit parameterization...');
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
    }, 2000);
    const stageTimer2 = setTimeout(() => {
      setTribunalStage('Device Auditor: Testing Clifford RB canary on simulated Heron backend...');
    }, 4500);
    const stageTimer3 = setTimeout(() => {
      setTribunalStage('Epistemic Scribe: Collapsing credibility state & synthesizing consensus...');
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
    }
  };

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const toggleTurnExpand = (turnKey: string) => {
    setExpandedTurns((prev) => ({
      ...prev,
      [turnKey]: !prev[turnKey],
    }));
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
      className="flex flex-col h-full w-full bg-[#080B11]/95 text-slate-100 p-5 space-y-4 select-none font-sans overflow-hidden"
    >
      {/* Header: Meta Muse Style Calm Identity & Spatial Controls */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3 shrink-0">
        <div className="flex items-center space-x-3">
          <div className="relative">
            <div className="w-9 h-9 rounded-2xl bg-gradient-to-tr from-cyan-500 via-indigo-600 to-purple-600 flex items-center justify-center shadow-lg shadow-cyan-950/40">
              <Sparkles className="w-4 h-4 text-white" />
            </div>
            <span className="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-emerald-400 ring-2 ring-[#080B11] animate-pulse" />
          </div>

          <div>
            <div className="flex items-center space-x-2">
              <h3 className="text-xs font-bold text-white tracking-tight">Kubona Companion</h3>
              <span className="text-[9px] font-mono font-bold px-1.5 py-0.2 rounded-full bg-cyan-950/90 text-cyan-300 border border-cyan-800/60">
                MUSE AGENTIC
              </span>
            </div>
            <p className="text-[10px] text-slate-400 font-sans">
              Collaborative Scientific Research Salon
            </p>
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
              title="Close Companion (⌘J)"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              <PanelRightClose className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Active Scientific Focus Ribbon */}
      <div className="px-3 py-2 rounded-xl bg-black/40 border border-white/[0.06] backdrop-blur-xl flex items-center justify-between text-[11px] font-mono shrink-0">
        <div className="flex items-center space-x-2 text-slate-300">
          <span className="text-cyan-400 font-bold">TARGET:</span>
          <span>{targetDisplayName}</span>
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
        className="flex-1 overflow-y-auto space-y-4 pr-1 scrollbar-thin"
      >
        {messages.length === 0 && (
          <div className="p-5 rounded-3xl bg-gradient-to-b from-white/[0.04] to-transparent border border-white/05 space-y-4 text-center my-4">
            <div className="w-12 h-12 rounded-2xl bg-cyan-500/10 border border-cyan-500/20 mx-auto flex items-center justify-center">
              <Bot className="w-6 h-6 text-cyan-400 animate-float-gentle" />
            </div>

            <div className="space-y-1.5 max-w-xs mx-auto">
              <h4 className="text-xs font-bold text-white tracking-tight">What shall we discover today?</h4>
              <p className="text-xs text-slate-300 leading-relaxed font-sans">
                State your physical hypothesis or target system. The specialist tribunal will synthesize classical ground truths and quantum circuits collaboratively.
              </p>
            </div>

            {/* Quick Scientific Suggestion Cards */}
            <div className="pt-2 space-y-2 text-left">
              <button
                onClick={() => handleSend('Map H2 ground state potential energy surface across nuclear coordinates')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-cyan-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <Atom className="w-4 h-4 text-cyan-400" />
                  <span className="font-medium">H₂ Potential Energy Surface</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-cyan-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleRunTribunal('Lithium Hydride (LiH) ground state electronic structure')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-indigo-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <Sparkles className="w-4 h-4 text-indigo-400" />
                  <span className="font-medium">Convene Tribunal for LiH</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-indigo-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleSend('Verify our variational ansatz on the IBM Heron QPU noise model with Clifford canary')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-emerald-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <Cpu className="w-4 h-4 text-emerald-400" />
                  <span className="font-medium">Probe IBM Heron Noise Model</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-emerald-300 group-hover:translate-x-0.5 transition-all" />
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
                  className={`p-4 rounded-3xl text-xs leading-relaxed max-w-[92%] font-sans ${
                    isUser
                      ? 'bg-gradient-to-r from-cyan-600 to-indigo-600 text-white rounded-br-none shadow-md'
                      : 'bg-black/60 border border-white/10 text-slate-200 rounded-bl-none shadow-xl'
                  }`}
                >
                  <MathText text={m.content} />
                </div>
              )}

              {/* Multi-Agent Tribunal Salon Presentation */}
              {hasTribunal && m.tribunal && (
                <div className="w-full rounded-3xl bg-black/60 border border-white/10 p-5 space-y-4 shadow-2xl backdrop-blur-2xl">
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
                    <div className="p-4 rounded-2xl bg-gradient-to-r from-emerald-950/30 to-cyan-950/30 border border-emerald-500/30 space-y-2">
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
                    </div>
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
      <div className="shrink-0 space-y-2 pt-1 border-t border-white/[0.06]">
        {/* Quick Prompt Suggestion Strip */}
        <div className="flex items-center space-x-1.5 overflow-x-auto pb-1 scrollbar-none text-xs">
          <button
            onClick={() => handleSend('Calculate potential energy surface for H2')}
            className="px-2.5 py-1 rounded-full bg-white/[0.04] hover:bg-white/[0.1] border border-white/10 text-slate-300 hover:text-white whitespace-nowrap transition-all font-medium"
          >
            H₂ Bond PES
          </button>
          <button
            onClick={() => handleRunTribunal(`${targetDisplayName} equilibrium state`)}
            className="px-2.5 py-1 rounded-full bg-white/[0.04] hover:bg-white/[0.1] border border-white/10 text-slate-300 hover:text-white whitespace-nowrap transition-all font-medium"
          >
            {targetSystem} Tribunal
          </button>
          <button
            onClick={() => handleSend('Simulate Clifford RB canary on IBM Heron')}
            className="px-2.5 py-1 rounded-full bg-white/[0.04] hover:bg-white/[0.1] border border-white/10 text-slate-300 hover:text-white whitespace-nowrap transition-all font-medium"
          >
            Canary Fidelity
          </button>
          <button
            onClick={() => handleSend('Verify classical PySCF STO-3G ground truth')}
            className="px-2.5 py-1 rounded-full bg-white/[0.04] hover:bg-white/[0.1] border border-white/10 text-slate-300 hover:text-white whitespace-nowrap transition-all font-medium"
          >
            Classical FCI Anchor
          </button>
        </div>

        {/* Input Bar */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            handleSend();
          }}
          className="space-y-2"
        >
          <div className="relative">
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="State a discovery hypothesis or ask a question..."
              className="w-full py-3 pl-4 pr-11 rounded-2xl bg-black/60 border border-white/10 text-xs text-white placeholder-slate-400 focus:outline-none focus:border-cyan-400/60 focus:ring-1 focus:ring-cyan-400/40 transition-all font-sans"
            />
            <button
              type="submit"
              disabled={!input.trim() || loading || runningTribunal}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 p-1.5 rounded-xl bg-cyan-500/20 text-cyan-300 hover:bg-cyan-500/40 disabled:opacity-30 transition-all"
            >
              <Send className="w-3.5 h-3.5" />
            </button>
          </div>

          {/* Tribunal Action Button */}
          <button
            type="button"
            onClick={() => handleRunTribunal(input.trim() || `${targetSystem} ground state electronic structure`)}
            disabled={loading || runningTribunal}
            className="w-full py-2 px-3 rounded-2xl bg-gradient-to-r from-indigo-600/30 via-cyan-600/30 to-purple-600/30 hover:from-indigo-600/50 hover:via-cyan-600/50 hover:to-purple-600/50 border border-white/10 text-xs text-slate-200 font-semibold flex items-center justify-center space-x-2 transition-all disabled:opacity-40 shadow-md active:scale-[0.99]"
          >
            <Sparkles className="w-3.5 h-3.5 text-cyan-400" />
            <span>Convene Multi-Agent Research Tribunal</span>
          </button>
        </form>
      </div>
    </div>
  );
}
