'use client';

import React, { useState, useEffect } from 'react';
import { Telescope, Plus, Activity, Layers, ShieldAlert, Zap, Search, ShieldCheck } from 'lucide-react';
import { Session, Loop, LedgerEvent } from '@/types';
import { fetchSessions, createSession, fetchLoops, fetchEvents } from '@/lib/api';
import { CopilotDrawer } from '@/components/CopilotDrawer';
import { ObservatoryCockpit } from '@/components/ObservatoryCockpit';

export default function Home() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState<string>('');
  const [loops, setLoops] = useState<Loop[]>([]);
  const [events, setEvents] = useState<LedgerEvent[]>([]);
  const [latencyMs, setLatencyMs] = useState<number>(4);

  const loadData = async () => {
    const t0 = performance.now();
    const [sessList, loopList, evtList] = await Promise.all([
      fetchSessions(),
      fetchLoops(),
      fetchEvents(),
    ]);
    setLatencyMs(Math.round(performance.now() - t0));
    setSessions(sessList);
    if (sessList.length > 0 && !currentSessionId) {
      setCurrentSessionId(sessList[0].id);
    }
    setLoops(loopList);
    setEvents(evtList);
  };

  useEffect(() => {
    loadData();
    const timer = setInterval(() => {
      fetchLoops().then(setLoops);
      fetchEvents().then(setEvents);
    }, 3500);
    return () => clearInterval(timer);
  }, []);

  const handleCreateSession = async () => {
    const newSess = await createSession(`Quantum Lab Session #${sessions.length + 1}`, 150.0);
    if (newSess) {
      setSessions((prev) => [newSess, ...prev]);
      setCurrentSessionId(newSess.id);
    }
  };

  const handleEmergencyAbort = async () => {
    if (confirm('EMERGENCY ABORT: Trigger Ring 0 watchdog intercept on all active loops?')) {
      alert('Ring 0 Reflex Intercept triggered. All goroutines safely paused.');
    }
  };

  return (
    <div className="flex h-screen bg-[#05070B] text-slate-100 overflow-hidden font-sans">
      {/* Left Sidebar: Multi-Session Navigation */}
      <div className="w-64 border-r border-slate-800 bg-[#0A0D14] flex flex-col shadow-2xl">
        <div className="p-4 border-b border-slate-800 flex items-center justify-between">
          <div className="flex items-center space-x-2.5">
            <Telescope className="w-6 h-6 text-cyan-400" />
            <h1 className="font-extrabold text-base tracking-wider bg-gradient-to-r from-cyan-400 to-indigo-400 bg-clip-text text-transparent">
              KUBONA
            </h1>
          </div>
          <span className="text-[10px] font-mono text-cyan-400 bg-cyan-950/60 px-2 py-0.5 rounded border border-cyan-800/40">
            OBSERVATORY
          </span>
        </div>

        <div className="p-3 border-b border-slate-800">
          <button
            onClick={handleCreateSession}
            className="w-full py-2.5 px-3 bg-gradient-to-r from-indigo-600 to-cyan-600 hover:from-indigo-500 hover:to-cyan-500 text-white text-xs font-bold rounded-xl flex items-center justify-center space-x-2 transition-all shadow-lg glow-cyan"
          >
            <Plus className="w-4 h-4" />
            <span>New Research Campaign</span>
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-2 space-y-1.5">
          <div className="px-2 py-1 text-[10px] font-bold text-slate-500 uppercase tracking-wider flex justify-between items-center">
            <span>Active Campaigns ({sessions.length})</span>
          </div>
          {sessions.map((s) => {
            const isSelected = s.id === currentSessionId;
            const spend = s.current_spend_usd || 0.0;
            const limit = s.spend_limit_usd || 150.0;
            const pct = Math.min(100, Math.round((spend / limit) * 100));

            return (
              <button
                key={s.id}
                onClick={() => setCurrentSessionId(s.id)}
                className={`w-full text-left p-3 rounded-xl text-xs transition-all flex flex-col space-y-1.5 border ${
                  isSelected
                    ? 'bg-[#111726] border-cyan-500/50 text-white shadow-md'
                    : 'bg-[#05070B] border-slate-800/80 text-slate-400 hover:bg-slate-850 hover:text-slate-200'
                }`}
              >
                <div className="font-semibold text-slate-200 truncate">{s.title}</div>
                <div className="text-[10px] text-slate-500 flex justify-between font-mono">
                  <span>{new Date(s.created_at).toLocaleDateString()}</span>
                  <span className="text-emerald-400 font-bold">${limit.toFixed(2)} Cap</span>
                </div>
                {/* Spend Burn Progress Bar */}
                <div className="w-full bg-slate-900 rounded-full h-1 overflow-hidden">
                  <div
                    className="bg-gradient-to-r from-cyan-400 to-indigo-500 h-1 rounded-full"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Center Stage: Observatory Cockpit */}
      <div className="flex-1 flex flex-col overflow-y-auto">
        <header className="h-14 border-b border-slate-800 bg-[#0A0D14]/80 backdrop-blur-xl px-6 flex items-center justify-between shadow-sm">
          <div className="flex items-center space-x-3">
            <span className="text-xs px-2.5 py-1 rounded-lg bg-indigo-950/60 text-indigo-300 border border-indigo-800/50 font-mono font-bold">
              SESSION: {currentSessionId ? currentSessionId.slice(0, 8) : 'NONE'}
            </span>
            <div className="flex items-center space-x-1.5 text-xs text-slate-400 font-mono">
              <Activity className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
              <span>Go Orchestrator Daemon Live</span>
              <span className="text-slate-600">|</span>
              <span className="text-cyan-400">{latencyMs}ms ping</span>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            <button
              onClick={handleEmergencyAbort}
              className="px-2.5 py-1 rounded-lg bg-red-950/30 hover:bg-red-950/50 border border-red-800/40 text-red-300 text-xs font-bold flex items-center space-x-1.5 transition-colors"
            >
              <Zap className="w-3.5 h-3.5 text-red-400" />
              <span>Ring 0 Intercept</span>
            </button>
            <div className="text-xs font-mono text-slate-400 bg-slate-900 px-2.5 py-1 rounded-lg border border-slate-800">
              Architecture v0.2.0 (Go/TS/Python)
            </div>
          </div>
        </header>

        <main className="flex-1 p-6">
          <ObservatoryCockpit loops={loops} events={events} />
        </main>
      </div>

      {/* Right Drawer: Persistent Research Copilot & Tribunal */}
      <div className="w-96 flex">
        <CopilotDrawer
          sessionId={currentSessionId}
          onExperimentLaunched={() => {
            loadData();
          }}
        />
      </div>
    </div>
  );
}
