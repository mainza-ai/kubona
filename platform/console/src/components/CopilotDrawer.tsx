'use client';

import React, { useState, useEffect } from 'react';
import { Send, Rocket, Bot, User, Sparkles, Scale, ShieldAlert, BookOpen, RefreshCw, Atom } from 'lucide-react';
import { ChatMessage, Proposal } from '@/types';
import { fetchMessages, sendChatMessage, launchProposal } from '@/lib/api';
import { MathText } from './common/MathText';

interface CopilotDrawerProps {
  sessionId: string;
  onExperimentLaunched: () => void;
}

interface TribunalTurn {
  turn_index: number;
  role: string;
  name: string;
  content: string;
  verdict: string;
}

export function CopilotDrawer({ sessionId, onExperimentLaunched }: CopilotDrawerProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [runningTribunal, setRunningTribunal] = useState(false);
  const [launching, setLaunching] = useState(false);

  useEffect(() => {
    if (sessionId) {
      fetchMessages(sessionId).then(setMessages);
    }
  }, [sessionId]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || loading) return;

    const userText = input;
    setInput('');
    setLoading(true);

    const res = await sendChatMessage(sessionId, userText);
    if (res) {
      setMessages((prev) => [...prev, res.user, res.assistant]);
    }
    setLoading(false);
  };

  const handleRunTribunal = async (customPrompt?: string) => {
    const promptText = customPrompt || input || 'Investigate H2 molecular ground state energy at equilibrium bond distance 0.7414 A';
    setInput('');
    setRunningTribunal(true);

    try {
      const res = await fetch('http://localhost:8080/api/chat/tribunal', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: sessionId,
          prompt: promptText,
        }),
      });
      if (res.ok) {
        // Reload all messages for the session to display the full debate
        const updated = await fetchMessages(sessionId);
        setMessages(updated);
      }
    } catch (err) {
      console.error('Failed to run tribunal:', err);
    } finally {
      setRunningTribunal(false);
    }
  };

  const handleLaunch = async (proposal: Proposal) => {
    setLaunching(true);
    const res = await launchProposal(sessionId, proposal);
    if (res) {
      onExperimentLaunched();
      // Reload chat messages to show system confirmation
      fetchMessages(sessionId).then(setMessages);
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
      <div className="p-4 border-b border-slate-800 flex items-center justify-between bg-[#05070B]/60">
        <div className="flex items-center space-x-2.5">
          <Bot className="w-5 h-5 text-indigo-400" />
          <div>
            <h2 className="font-bold text-sm tracking-wide text-white">Research Copilot</h2>
            <div className="text-[10px] text-slate-400 font-mono">oMLX: Qwen3.8-27B-Uncensored</div>
          </div>
        </div>
        <button
          onClick={() => handleRunTribunal()}
          disabled={runningTribunal}
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
        <span className="text-slate-500 uppercase font-bold shrink-0">Preset:</span>
        <button
          onClick={() => handleRunTribunal('Investigate H2 ground state VQE at R=0.7414 A')}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-cyan-300 border border-slate-700 shrink-0 font-mono"
        >
          H2 VQE
        </button>
        <button
          onClick={() => handleRunTribunal('Investigate LiH ground state dissociation VQE at R=1.595 A')}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 border border-slate-700 shrink-0 font-mono"
        >
          LiH Ground
        </button>
        <button
          onClick={() => handleRunTribunal('Investigate BeH2 linear molecule VQE at R=1.33 A')}
          className="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-amber-300 border border-slate-700 shrink-0 font-mono"
        >
          BeH2 PES
        </button>
      </div>

      {/* Messages Stream */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 font-sans text-xs">
        {messages.length === 0 ? (
          <div className="text-center py-16 text-slate-500 space-y-2">
            <Atom className="w-8 h-8 text-indigo-400 mx-auto opacity-40 animate-pulse" />
            <p>
              Ask the copilot or launch a <strong>4-Turn Multi-Agent Tribunal</strong> to simulate molecular systems and generate cryptographic AP2 execution mandates.
            </p>
          </div>
        ) : (
          messages.map((m) => {
            const roleBadge = getRoleBadge(m.role);
            const isUser = m.role === 'user';
            const isSystem = m.role === 'system';

            return (
              <div key={m.id} className={`flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
                <div
                  className={`p-3.5 rounded-xl max-w-[92%] space-y-2 leading-relaxed ${
                    isUser
                      ? 'bg-indigo-600 text-white shadow-md'
                      : isSystem
                      ? 'bg-emerald-950/40 border border-emerald-800/40 text-emerald-300'
                      : 'bg-[#111726] text-slate-200 border border-slate-800 shadow-lg'
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
                    </div>
                  )}

                  {/* Message Content with KaTeX Math Rendering */}
                  <div className="text-xs">
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
      </div>

      {/* Input Form */}
      <form onSubmit={handleSend} className="p-3 border-t border-slate-800 bg-[#05070B]/80 flex space-x-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Ask copilot or type target (e.g. 'LiH VQE')..."
          className="flex-1 bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-500 font-mono"
        />
        <button
          type="submit"
          disabled={loading || !input.trim()}
          className="bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white p-2 rounded-lg flex items-center justify-center transition-colors"
        >
          <Send className="w-4 h-4" />
        </button>
      </form>
    </div>
  );
}
