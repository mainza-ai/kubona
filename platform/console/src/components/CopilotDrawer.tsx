'use client';

import React, { useState, useEffect, useRef } from 'react';
import {
  Send,
  Rocket,
  Bot,
  User,
  Sparkles,
  Scale,
  ShieldAlert,
  BookOpen,
  RefreshCw,
  Atom,
  AlertCircle,
  Clock,
  CheckCircle2,
} from 'lucide-react';
import { ChatMessage, Proposal, CampaignState } from '@/types';
import { fetchMessages, sendChatMessage, launchProposal, createSession, fetchCampaignState } from '@/lib/api';
import { MathText } from './common/MathText';

interface CopilotDrawerProps {
  sessionId: string;
  onExperimentLaunched: () => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
}

export function CopilotDrawer({ sessionId, onExperimentLaunched, onCampaignUpdated }: CopilotDrawerProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [runningTribunal, setRunningTribunal] = useState(false);
  const [tribunalStage, setTribunalStage] = useState<string>('');
  const [launching, setLaunching] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<NodeJS.Timeout | null>(null);
  const pollRef = useRef<NodeJS.Timeout | null>(null);

  // Auto-scroll helper
  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, loading, runningTribunal]);

  // Load messages on session change
  useEffect(() => {
    if (sessionId) {
      fetchMessages(sessionId).then(setMessages);
    }
  }, [sessionId]);

  // Handle elapsed timer during local LLM reasoning
  useEffect(() => {
    if (loading || runningTribunal) {
      setElapsedSeconds(0);
      timerRef.current = setInterval(() => {
        setElapsedSeconds((prev) => prev + 1);
      }, 1000);
    } else {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      setElapsedSeconds(0);
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [loading, runningTribunal]);

  // Ensure active session ID
  const getActiveSessionId = async (): Promise<string> => {
    if (sessionId) return sessionId;
    const newSess = await createSession('Quantum Research Campaign #1', 150.0);
    return newSess ? newSess.id : '';
  };

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || loading || runningTribunal) return;

    const userText = input.trim();
    setInput('');
    setErrorMessage(null);

    const activeSessionId = await getActiveSessionId();

    // 1. Optimistic dispatch: Render immediately in the UI
    const tempUserMsg: ChatMessage = {
      id: `temp-user-${Date.now()}`,
      session_id: activeSessionId,
      role: 'user',
      content: userText,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);
    setLoading(true);

    try {
      const res = await sendChatMessage(activeSessionId, userText);
      if (res) {
        setMessages((prev) => {
          // Replace temp msg with the verified persisted messages
          const filtered = prev.filter((m) => m.id !== tempUserMsg.id);
          return [...filtered, res.user, res.assistant];
        });
        if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (c) onCampaignUpdated(c);
          });
        }
      } else {
        setErrorMessage('Failed to receive response from oMLX engine. Please check port 8787.');
      }
    } catch (err: any) {
      console.error('Chat error:', err);
      setErrorMessage(err?.message || 'Connection error to backend orchestrator.');
    } finally {
      setLoading(false);
    }
  };

  const handleRunTribunal = async (customPrompt?: string) => {
    if (runningTribunal || loading) return;

    const promptText =
      customPrompt ||
      input.trim() ||
      'Investigate H2 molecular ground state energy at equilibrium bond distance 0.7414 A';
    setInput('');
    setErrorMessage(null);
    setRunningTribunal(true);
    setTribunalStage('Ansatz Scout: Proposing variational quantum architecture...');

    const activeSessionId = await getActiveSessionId();

    // Optimistically render initial tribunal announcement
    const tempSystemMsg: ChatMessage = {
      id: `temp-tribunal-${Date.now()}`,
      session_id: activeSessionId,
      role: 'system',
      content: `🚀 **Multi-Agent Tribunal Convened** for target: *${promptText}*\nSpecialist sequence: Ansatz Scout → Disconfirmation Officer → Device Auditor → Epistemic Scribe.`,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempSystemMsg]);

    // Live turn polling interval: poll every 2.0 seconds while debate runs
    pollRef.current = setInterval(async () => {
      try {
        const msgs = await fetchMessages(activeSessionId);
        if (msgs && msgs.length > 0) {
          setMessages(msgs);
        }
      } catch (e) {
        // Continue polling silently
      }
    }, 2000);

    try {
      const res = await fetch('http://localhost:8080/api/chat/tribunal', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: activeSessionId,
          prompt: promptText,
        }),
      });

      if (res.ok) {
        const updated = await fetchMessages(activeSessionId);
        setMessages(updated);
        if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (c) onCampaignUpdated(c);
          });
        }
      } else {
        const errText = await res.text();
        setErrorMessage(`Tribunal execution failed: ${errText}`);
      }
    } catch (err: any) {
      console.error('Failed to run tribunal:', err);
      setErrorMessage(`Tribunal error: ${err?.message || err}`);
    } finally {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
      setRunningTribunal(false);
      setTribunalStage('');
      if (onCampaignUpdated) {
        fetchCampaignState(activeSessionId).then((c) => {
          if (c) onCampaignUpdated(c);
        });
      }
    }
  };

  const handleLaunch = async (proposal: Proposal) => {
    setLaunching(true);
    setErrorMessage(null);
    const activeSessionId = await getActiveSessionId();
    const res = await launchProposal(activeSessionId, proposal);
    if (res) {
      onExperimentLaunched();
      fetchMessages(activeSessionId).then(setMessages);
      if (onCampaignUpdated) {
        fetchCampaignState(activeSessionId).then((c) => {
          if (c) onCampaignUpdated(c);
        });
      }
    } else {
      setErrorMessage('Failed to authorize and launch proposal.');
    }
    setLaunching(false);
  };

  const getRoleBadge = (role: string) => {
    switch (role) {
      case 'ansatz-scout':
        return { label: 'Ansatz Scout', color: 'bg-cyan-950 text-cyan-300 border-cyan-800', icon: Sparkles };
      case 'disconfirmation-officer':
        return { label: 'Disconfirmation Officer', color: 'bg-amber-950 text-amber-300 border-amber-800', icon: Scale };
      case 'device-auditor':
        return { label: 'Device Auditor', color: 'bg-purple-950 text-purple-300 border-purple-800', icon: ShieldAlert };
      case 'epistemic-scribe':
        return { label: 'Epistemic Scribe', color: 'bg-emerald-950 text-emerald-300 border-emerald-800', icon: BookOpen };
      default:
        return null;
    }
  };

  return (
    <div className="flex flex-col h-full bg-[#0A0D14] border-l border-slate-800 text-slate-100 w-full max-w-md shadow-2xl">
      {/* Header */}
      <div className="p-4 border-b border-slate-800 flex items-center justify-between bg-[#05070B]/80 backdrop-blur-md">
        <div className="flex items-center space-x-2.5">
          <div className="w-8 h-8 rounded-lg bg-indigo-950/80 border border-indigo-700/60 flex items-center justify-center">
            <Bot className="w-4 h-4 text-indigo-400" />
          </div>
          <div>
            <h2 className="font-bold text-sm tracking-wide text-white">Research Copilot</h2>
            <div className="text-[10px] text-cyan-400 font-mono flex items-center space-x-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span>oMLX: Qwen3.8-27B (Port 8787)</span>
            </div>
          </div>
        </div>
        <button
          onClick={() => handleRunTribunal()}
          disabled={runningTribunal || loading}
          className="px-2.5 py-1 bg-gradient-to-r from-indigo-600 to-cyan-600 hover:from-indigo-500 hover:to-cyan-500 text-white rounded-lg text-[11px] font-bold flex items-center space-x-1.5 shadow transition-all disabled:opacity-50"
        >
          {runningTribunal ? (
            <>
              <RefreshCw className="w-3 h-3 animate-spin" />
              <span>Tribunal Active...</span>
            </>
          ) : (
            <>
              <Atom className="w-3.5 h-3.5" />
              <span>Run 4-Turn Tribunal</span>
            </>
          )}
        </button>
      </div>

      {/* Quick Scientific Action Chips */}
      <div className="px-3 py-2 bg-[#05070B] border-b border-slate-800/80 flex items-center space-x-1.5 overflow-x-auto scrollbar-none text-[10px]">
        <span className="text-slate-500 uppercase font-bold shrink-0">Presets:</span>
        <button
          onClick={() => handleRunTribunal('Investigate H2 ground state VQE at R=0.7414 A')}
          disabled={runningTribunal || loading}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-cyan-300 border border-slate-700 shrink-0 font-mono transition-colors disabled:opacity-50"
        >
          H2 VQE
        </button>
        <button
          onClick={() => handleRunTribunal('Investigate LiH ground state dissociation VQE at R=1.595 A')}
          disabled={runningTribunal || loading}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 border border-slate-700 shrink-0 font-mono transition-colors disabled:opacity-50"
        >
          LiH Ground
        </button>
        <button
          onClick={() => handleRunTribunal('Investigate BeH2 linear molecule VQE at R=1.33 A')}
          disabled={runningTribunal || loading}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-amber-300 border border-slate-700 shrink-0 font-mono transition-colors disabled:opacity-50"
        >
          BeH2 PES
        </button>
      </div>

      {/* Error Notice Banner */}
      {errorMessage && (
        <div className="px-3 py-2 bg-red-950/60 border-b border-red-800/60 text-red-200 text-xs flex items-center justify-between">
          <div className="flex items-center space-x-2 truncate">
            <AlertCircle className="w-3.5 h-3.5 text-red-400 shrink-0" />
            <span className="truncate">{errorMessage}</span>
          </div>
          <button
            onClick={() => setErrorMessage(null)}
            className="text-[10px] text-red-400 hover:text-white uppercase font-bold ml-2 shrink-0"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Messages Stream */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 font-sans text-xs">
        {messages.length === 0 && !loading && !runningTribunal ? (
          <div className="text-center py-16 text-slate-500 space-y-3">
            <Atom className="w-10 h-10 text-indigo-400 mx-auto opacity-40 animate-pulse" />
            <div className="space-y-1">
              <p className="font-semibold text-slate-300">Ready for Exploration</p>
              <p className="text-[11px] text-slate-500 max-w-xs mx-auto">
                Type any quantum target (e.g. <em>&quot;LiH ground state VQE&quot;</em>) or run the <strong>4-Turn Multi-Agent Tribunal</strong> to simulate with PySCF classical anchors and generate cryptographic AP2 spend cards.
              </p>
            </div>
          </div>
        ) : (
          messages.map((m) => {
            const roleBadge = getRoleBadge(m.role);
            const isUser = m.role === 'user';
            const isSystem = m.role === 'system';

            return (
              <div key={m.id} className={`flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
                <div
                  className={`p-3.5 rounded-xl max-w-[92%] space-y-2 leading-relaxed transition-all shadow-md ${
                    isUser
                      ? 'bg-indigo-600 text-white'
                      : isSystem
                      ? 'bg-emerald-950/40 border border-emerald-800/40 text-emerald-300'
                      : 'bg-[#111726] text-slate-200 border border-slate-800'
                  }`}
                >
                  {/* Persona Role Tag */}
                  {roleBadge ? (
                    <div className="flex items-center space-x-1.5">
                      <span className={`px-2 py-0.5 rounded text-[10px] font-bold font-mono border ${roleBadge.color}`}>
                        {roleBadge.label}
                      </span>
                    </div>
                  ) : (
                    <div className="flex items-center space-x-1.5 opacity-60 text-[10px] font-mono">
                      {isUser ? <User className="w-3 h-3" /> : <Bot className="w-3 h-3" />}
                      <span className="capitalize">{m.role}</span>
                      <span className="text-[9px] text-slate-500 ml-1">
                        {new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                      </span>
                    </div>
                  )}

                  {/* Message Content with KaTeX Math Rendering */}
                  <div className="text-xs break-words">
                    <MathText text={m.content} />
                  </div>

                  {/* Render AP2 Proposal Mandate Card if present */}
                  {m.proposal_json && (
                    <div className="mt-3 p-3 bg-[#05070B] rounded-lg border border-indigo-500/40 space-y-2 font-mono">
                      <div className="flex items-center justify-between border-b border-slate-800 pb-2">
                        <span className="font-bold text-indigo-300 text-[11px] truncate">
                          {m.proposal_json.target}
                        </span>
                        <span className="text-amber-400 font-bold text-[10px]">
                          {m.proposal_json.spend_estimate}
                        </span>
                      </div>

                      <div className="text-[10px] text-slate-400 space-y-1">
                        <div>
                          <span className="text-slate-500">Tier:</span>{' '}
                          <strong className="text-slate-200">{m.proposal_json.recommended_tier}</strong>
                        </div>
                        <div>
                          <span className="text-slate-500">Classical Anchor:</span>{' '}
                          <strong className="text-emerald-400">{m.proposal_json.classical_anchor}</strong>
                        </div>
                      </div>

                      <button
                        onClick={() => handleLaunch(m.proposal_json!)}
                        disabled={launching}
                        className="w-full mt-2 py-2 px-3 bg-gradient-to-r from-indigo-600 to-cyan-600 hover:from-indigo-500 hover:to-cyan-500 text-white font-bold rounded-lg flex items-center justify-center space-x-2 transition-all shadow disabled:opacity-50 text-xs"
                      >
                        <Rocket className="w-3.5 h-3.5" />
                        <span>{launching ? 'Authorizing & Enqueuing...' : 'Authorize & Launch on Tier'}</span>
                      </button>
                    </div>
                  )}
                </div>
              </div>
            );
          })
        )}

        {/* Real-time Animated Thinking Indicator during local LLM reasoning */}
        {loading && (
          <div className="flex flex-col items-start animate-fade-in">
            <div className="p-3.5 rounded-xl max-w-[92%] bg-[#111726] border border-cyan-500/40 shadow-lg space-y-2 text-slate-300">
              <div className="flex items-center space-x-2 text-[10px] font-mono text-cyan-400">
                <span className="w-2 h-2 rounded-full bg-cyan-400 animate-ping" />
                <span className="font-bold">oMLX Local Engine (Qwen3.8-27B)</span>
                <span className="text-slate-500">|</span>
                <span className="text-amber-300 flex items-center space-x-1">
                  <Clock className="w-3 h-3" />
                  <span>{elapsedSeconds}s</span>
                </span>
              </div>
              <div className="flex items-center space-x-2 text-xs">
                <div className="flex space-x-1">
                  <div className="w-1.5 h-1.5 bg-cyan-400 rounded-full animate-bounce [animation-delay:-0.3s]" />
                  <div className="w-1.5 h-1.5 bg-cyan-400 rounded-full animate-bounce [animation-delay:-0.15s]" />
                  <div className="w-1.5 h-1.5 bg-cyan-400 rounded-full animate-bounce" />
                </div>
                <span className="text-slate-400 text-[11px]">
                  Reasoning ab-initio &amp; checking classical anchor...
                </span>
              </div>
            </div>
          </div>
        )}

        {/* Real-time Animated Tribunal Deliberation Banner */}
        {runningTribunal && (
          <div className="flex flex-col items-start animate-fade-in">
            <div className="p-3.5 rounded-xl w-full bg-gradient-to-r from-indigo-950/60 to-cyan-950/60 border border-indigo-500/50 shadow-xl space-y-2">
              <div className="flex items-center justify-between text-[11px] font-mono">
                <div className="flex items-center space-x-2 text-cyan-300 font-bold">
                  <RefreshCw className="w-3.5 h-3.5 animate-spin text-cyan-400" />
                  <span>4-Turn Tribunal Deliberating</span>
                </div>
                <span className="text-amber-400 font-bold">{elapsedSeconds}s elapsed</span>
              </div>
              <div className="text-[11px] text-slate-300 leading-relaxed font-sans">
                {tribunalStage || 'Synthesizing multi-agent consensus...'}
              </div>
              <div className="w-full bg-black/50 rounded-full h-1 overflow-hidden">
                <div className="bg-gradient-to-r from-indigo-500 to-cyan-400 h-1 rounded-full animate-pulse" style={{ width: '100%' }} />
              </div>
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Form */}
      <form onSubmit={handleSend} className="p-3 border-t border-slate-800 bg-[#05070B]/90 flex space-x-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          disabled={loading || runningTribunal}
          placeholder="Ask copilot or type target (e.g. 'LiH VQE')..."
          className="flex-1 bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-500 font-mono disabled:opacity-50"
        />
        <button
          type="submit"
          disabled={loading || runningTribunal || !input.trim()}
          className="bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white px-3.5 py-2 rounded-lg flex items-center justify-center transition-colors shadow"
          title="Send message"
        >
          {loading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
        </button>
      </form>
    </div>
  );
}
