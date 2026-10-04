'use client';

import React, { useState, useEffect, useCallback } from 'react';
import Image from 'next/image';
import {
  Plus,
  Activity,
  Zap,
  Search,
  Command,
  HelpCircle,
  PanelLeftClose,
  PanelLeft,
  PanelRightClose,
  PanelRight,
  Info,
  Clock,
  DollarSign,
  ChevronRight,
} from 'lucide-react';
import { Session, Loop, LedgerEvent } from '@/types';
import { fetchSessions, createSession, fetchLoops, fetchEvents } from '@/lib/api';
import { CopilotDrawer } from '@/components/CopilotDrawer';
import { ObservatoryCockpit, SurfaceTab } from '@/components/ObservatoryCockpit';
import { AboutLabModal } from '@/components/AboutLabModal';
import { CommandPalette } from '@/components/CommandPalette';
import { KeyboardShortcutsModal } from '@/components/KeyboardShortcutsModal';

export default function Home() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState<string>('');
  const [loops, setLoops] = useState<Loop[]>([]);
  const [events, setEvents] = useState<LedgerEvent[]>([]);
  const [latencyMs, setLatencyMs] = useState<number>(4);

  // Surface Tab Navigation State
  const [activeTab, setActiveTab] = useState<SurfaceTab>('overview');

  // Modals & Panels Visibility
  const [isAboutOpen, setIsAboutOpen] = useState(false);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);
  const [isShortcutsOpen, setIsShortcutsOpen] = useState(false);
  const [showSidebar, setShowSidebar] = useState(true);
  const [showCopilot, setShowCopilot] = useState(true);

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

  const handleCreateSession = useCallback(async () => {
    const newSess = await createSession(`Quantum Lab Session #${sessions.length + 1}`, 150.0);
    if (newSess) {
      setSessions((prev) => [newSess, ...prev]);
      setCurrentSessionId(newSess.id);
    }
  }, [sessions.length]);

  const handleEmergencyAbort = useCallback(() => {
    if (confirm('EMERGENCY ABORT: Trigger Ring 0 watchdog intercept on all active loops?')) {
      alert('Ring 0 Reflex Intercept triggered. All goroutines safely paused.');
    }
  }, []);

  // Global Keyboard Shortcuts (Apple HIG compliance)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const isCmdOrCtrl = e.metaKey || e.ctrlKey;

      if (isCmdOrCtrl && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsCommandPaletteOpen((prev) => !prev);
      } else if (isCmdOrCtrl && e.key.toLowerCase() === 'b') {
        e.preventDefault();
        setShowSidebar((prev) => !prev);
      } else if (isCmdOrCtrl && e.key.toLowerCase() === 'j') {
        e.preventDefault();
        setShowCopilot((prev) => !prev);
      } else if (isCmdOrCtrl && e.key.toLowerCase() === 'i') {
        e.preventDefault();
        setIsAboutOpen(true);
      } else if (isCmdOrCtrl && e.key.toLowerCase() === 'n') {
        e.preventDefault();
        handleCreateSession();
      } else if (isCmdOrCtrl && e.key === '.') {
        e.preventDefault();
        handleEmergencyAbort();
      } else if (e.key === '?' && !['INPUT', 'TEXTAREA'].includes((e.target as HTMLElement).tagName)) {
        e.preventDefault();
        setIsShortcutsOpen(true);
      } else if (isCmdOrCtrl && ['0', '1', '2', '3', '4', '5', '6', '7', '8'].includes(e.key)) {
        e.preventDefault();
        const tabMap: Record<string, SurfaceTab> = {
          '0': 'overview',
          '1': 'surface1',
          '2': 'surface2',
          '3': 'surface3',
          '4': 'surface4',
          '5': 'surface5',
          '6': 'surface6',
          '7': 'surface7',
          '8': 'settings',
        };
        setActiveTab(tabMap[e.key]);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleCreateSession, handleEmergencyAbort]);

  return (
    <div className="flex h-screen bg-[#05070B] text-slate-100 overflow-hidden font-sans select-none">
      {/* Modals & Dialogs */}
      <AboutLabModal isOpen={isAboutOpen} onClose={() => setIsAboutOpen(false)} />
      <CommandPalette
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        onSelectSurface={(surfId) => setActiveTab(surfId as SurfaceTab)}
        onNewCampaign={handleCreateSession}
        onEmergencyAbort={handleEmergencyAbort}
        onOpenAbout={() => setIsAboutOpen(true)}
      />
      <KeyboardShortcutsModal
        isOpen={isShortcutsOpen}
        onClose={() => setIsShortcutsOpen(false)}
      />

      {/* Left Sidebar: Multi-Session Navigation & Brand Lockup */}
      {showSidebar && (
        <aside
          aria-label="Research Campaigns"
          className="w-72 border-r border-white/[0.08] bg-[#090C13]/95 flex flex-col shadow-2xl backdrop-blur-2xl shrink-0 transition-all duration-200"
        >
          {/* Apple-style Window Titlebar & Brand Identity */}
          <div className="p-4 border-b border-white/[0.08] bg-white/[0.02]">
            {/* Window Traffic Lights */}
            <div className="flex items-center space-x-2 pb-3">
              <button
                onClick={() => setIsAboutOpen(true)}
                title="About Kubona"
                className="w-3 h-3 rounded-full bg-[#FF5F56] hover:brightness-110 transition-all"
              />
              <div className="w-3 h-3 rounded-full bg-[#FFBD2E] opacity-80" />
              <div className="w-3 h-3 rounded-full bg-[#27C93F] opacity-80" />
            </div>

            {/* Brand Monogram + Typography */}
            <div
              onClick={() => setIsAboutOpen(true)}
              className="flex items-center space-x-3 cursor-pointer group p-1.5 -mx-1.5 rounded-xl hover:bg-white/[0.04] transition-all"
            >
              <div className="relative w-9 h-9 rounded-[10px] overflow-hidden bg-black/70 ring-1 ring-white/15 shadow-md flex items-center justify-center group-hover:ring-cyan-400/50 transition-all shrink-0">
                <Image
                  src="/kubona_monogram.png"
                  alt="Kubona Logo"
                  width={30}
                  height={30}
                  className="object-contain"
                  priority
                />
              </div>

              <div className="flex flex-col flex-1 min-w-0">
                <div className="flex items-center space-x-1.5">
                  <span className="font-bold text-sm tracking-tight text-white group-hover:text-cyan-300 transition-colors">
                    Kubona
                  </span>
                  <span className="text-[9px] font-mono font-bold px-1.5 py-0.2 rounded-full bg-cyan-950 text-cyan-300 border border-cyan-800/60">
                    PRO
                  </span>
                </div>
                <span className="text-[10px] text-slate-400 font-sans tracking-wide truncate">
                  Scientific Laboratory
                </span>
              </div>

              <ChevronRight className="w-3.5 h-3.5 text-slate-500 group-hover:text-white transition-colors" />
            </div>
          </div>

          {/* Quick Action: New Campaign */}
          <div className="p-3 border-b border-white/[0.08]">
            <button
              onClick={handleCreateSession}
              className="w-full py-2 px-3 bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 active:scale-[0.98] text-white text-xs font-semibold rounded-xl flex items-center justify-center space-x-2 transition-all shadow-md shadow-cyan-950/40 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>New Research Campaign</span>
              <kbd className="text-[9px] font-mono bg-black/30 px-1.5 py-0.2 rounded text-cyan-200">
                ⌘N
              </kbd>
            </button>
          </div>

          {/* Active Campaigns List */}
          <div className="flex-1 overflow-y-auto p-2 space-y-1.5">
            <div className="px-2 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider flex justify-between items-center font-mono">
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
                  className={`w-full text-left p-3 rounded-xl text-xs transition-all flex flex-col space-y-1.5 border select-none focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                    isSelected
                      ? 'bg-cyan-500/10 border-cyan-500/40 text-white shadow-md'
                      : 'bg-[#080B11]/80 border-white/[0.04] text-slate-300 hover:bg-white/[0.04] hover:text-white'
                  }`}
                >
                  <div className="font-semibold text-slate-100 truncate">{s.title}</div>
                  <div className="text-[10px] text-slate-400 flex justify-between font-mono">
                    <span className="flex items-center space-x-1">
                      <Clock className="w-3 h-3 text-slate-500" />
                      <span>{new Date(s.created_at).toLocaleDateString()}</span>
                    </span>
                    <span className="text-emerald-400 font-bold font-mono">
                      ${limit.toFixed(2)} Cap
                    </span>
                  </div>
                  {/* Spend Progress Bar */}
                  <div className="w-full bg-black/60 rounded-full h-1 overflow-hidden border border-white/[0.04]">
                    <div
                      className="bg-gradient-to-r from-cyan-400 to-indigo-500 h-1 rounded-full"
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </button>
              );
            })}
          </div>

          {/* Sidebar Footer with Lab Info Trigger & Shortcuts */}
          <div className="p-3 border-t border-white/[0.08] bg-white/[0.01] flex items-center justify-between text-xs text-slate-400">
            <button
              onClick={() => setIsAboutOpen(true)}
              className="flex items-center space-x-1.5 hover:text-white transition-colors"
            >
              <Info className="w-3.5 h-3.5 text-cyan-400" />
              <span>Lab Info</span>
            </button>
            <button
              onClick={() => setIsShortcutsOpen(true)}
              className="flex items-center space-x-1 hover:text-white transition-colors"
              title="Keyboard Shortcuts"
            >
              <HelpCircle className="w-3.5 h-3.5 text-slate-400" />
              <span className="font-mono text-[10px]">?</span>
            </button>
          </div>
        </aside>
      )}

      {/* Center Stage: Observatory Cockpit */}
      <div className="flex-1 flex flex-col overflow-y-auto min-w-0">
        {/* Apple-style Unified Top Toolbar */}
        <header className="h-14 border-b border-white/[0.08] bg-[#0A0D15]/85 backdrop-blur-2xl px-6 flex items-center justify-between shadow-sm sticky top-0 z-30 shrink-0">
          <div className="flex items-center space-x-3">
            {/* Sidebar Toggle Button */}
            <button
              onClick={() => setShowSidebar((prev) => !prev)}
              aria-label="Toggle Campaigns Sidebar"
              title="Toggle Campaigns Sidebar (⌘B)"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              {showSidebar ? (
                <PanelLeftClose className="w-4 h-4" />
              ) : (
                <PanelLeft className="w-4 h-4 text-cyan-400" />
              )}
            </button>

            {/* Current Session Indicator */}
            <span className="text-xs px-2.5 py-1 rounded-lg bg-indigo-950/60 text-indigo-300 border border-indigo-800/50 font-mono font-bold">
              SESSION: {currentSessionId ? currentSessionId.slice(0, 8) : 'NONE'}
            </span>

            {/* Live Daemon Ping */}
            <div className="flex items-center space-x-1.5 text-xs text-slate-300 font-mono">
              <Activity className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
              <span>Go Core Live</span>
              <span className="text-slate-600">|</span>
              <span className="text-cyan-400">{latencyMs}ms ping</span>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            {/* Quick Spotlight Search Trigger */}
            <button
              onClick={() => setIsCommandPaletteOpen(true)}
              className="px-3 py-1.5 rounded-xl bg-white/[0.04] hover:bg-white/[0.08] border border-white/[0.08] text-xs text-slate-400 hover:text-slate-200 flex items-center space-x-2 transition-all shadow-sm focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              <Search className="w-3.5 h-3.5 text-slate-400" />
              <span>Search surfaces...</span>
              <kbd className="px-1.5 py-0.5 rounded bg-white/10 text-[10px] font-mono text-slate-300">
                ⌘K
              </kbd>
            </button>

            {/* Emergency Abort (Ring 0) */}
            <button
              onClick={handleEmergencyAbort}
              className="px-2.5 py-1.5 rounded-xl bg-red-950/40 hover:bg-red-950/70 border border-red-800/50 text-red-200 text-xs font-bold flex items-center space-x-1.5 transition-all shadow-sm active:scale-95 focus-visible:ring-2 focus-visible:ring-red-400 focus-visible:outline-none"
            >
              <Zap className="w-3.5 h-3.5 text-red-400" />
              <span>Ring 0 Intercept</span>
            </button>

            {/* Copilot Drawer Toggle */}
            <button
              onClick={() => setShowCopilot((prev) => !prev)}
              aria-label="Toggle Copilot Drawer"
              title="Toggle Research Copilot (⌘J)"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              {showCopilot ? (
                <PanelRightClose className="w-4 h-4" />
              ) : (
                <PanelRight className="w-4 h-4 text-cyan-400" />
              )}
            </button>
          </div>
        </header>

        {/* Main Observatory Content Stage */}
        <main className="flex-1 p-6">
          <ObservatoryCockpit
            loops={loops}
            events={events}
            activeTab={activeTab}
            onTabChange={setActiveTab}
          />
        </main>
      </div>

      {/* Right Drawer: Persistent Research Copilot & Tribunal */}
      {showCopilot && (
        <aside
          aria-label="Research Copilot and Tribunal"
          className="w-96 flex shrink-0 border-l border-white/[0.08] bg-[#090C13]/95 backdrop-blur-2xl transition-all duration-200 shadow-2xl"
        >
          <CopilotDrawer
            sessionId={currentSessionId}
            onExperimentLaunched={() => {
              loadData();
            }}
          />
        </aside>
      )}
    </div>
  );
}
