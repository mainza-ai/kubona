'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Sparkles, Plus, Zap, Info, HelpCircle, Trash2, Clock, Box, X } from 'lucide-react';
import { Session, Loop, LedgerEvent, CampaignState, RunPhase, ProbeId } from '@/types';
import { fetchSessions, createSession, deleteSession, fetchLoops, fetchEvents, fetchCampaignState, API_BASE, authHeaders, configureApiBase, apiFetch } from '@/lib/api';
import { CopilotDrawer } from '@/components/CopilotDrawer';
import { AboutLabModal } from '@/components/AboutLabModal';
import { CommandPalette } from '@/components/CommandPalette';
import { KeyboardShortcutsModal } from '@/components/KeyboardShortcutsModal';
import { NewCampaignModal } from '@/components/modals/NewCampaignModal';
import { SovereignDock, DockSurface } from '@/components/sovereign/SovereignDock';
import { ArenaHud, ArenaView } from '@/components/sovereign/ArenaHud';
import { SwarmUniverse } from '@/components/sovereign/SwarmUniverse';
import { ProofDagView } from '@/components/sovereign/ProofDagView';
import { ActionGate } from '@/components/sovereign/ActionGate';
import { AttentionProvider } from '@/lib/attention-context';
import { isMeteredBackend } from '@/lib/approvalPacket';
import { WidgetStage, CanvasWidgetSpec, CanvasWidgetKind } from '@/components/sovereign/WidgetStage';
import { FlightRecorder } from '@/components/surfaces/FlightRecorder';
import { EpistemicManifold } from '@/components/surfaces/EpistemicManifold';
import { FleetRadar } from '@/components/surfaces/FleetRadar';
import { ResultsStudio } from '@/components/surfaces/ResultsStudio';
import { EpistemicTribunalRoom } from '@/components/surfaces/EpistemicTribunalRoom';
import { CognitiveMesh } from '@/components/surfaces/CognitiveMesh';
import { HuggingFaceModelHub } from '@/components/surfaces/HuggingFaceModelHub';
import { LabsView } from '@/components/surfaces/LabsView';
import { LLMSettings } from '@/components/surfaces/LLMSettings';
import { QuantumSettings } from '@/components/surfaces/QuantumSettings';
import { QuantumDispatch } from '@/components/surfaces/QuantumDispatch';
import { AutonomyCockpit } from '@/components/surfaces/AutonomyCockpit';
import { SpatialCanvas } from '@/components/surfaces/SpatialCanvas';
import { BackendStatusBar } from '@/components/common/BackendStatusBar';
import { CheckInStrip } from '@/components/sovereign/CheckInStrip';
import { GenerativeUIStudio } from '@/components/surfaces/GenerativeUIStudio';
import { CodesignCockpit } from '@/components/surfaces/CodesignCockpit';

function formatElapsed(createdAt: string): string {
  const ms = Date.now() - new Date(createdAt).getTime();
  if (!Number.isFinite(ms) || ms < 0) return '00:00:00';
  const s = Math.floor(ms / 1000);
  const hh = String(Math.floor(s / 3600)).padStart(2, '0');
  const mm = String(Math.floor((s % 3600) / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

export default function Home() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState<string>('');
  const bootstrappedSession = useRef(false);
  const prevPhase = useRef<RunPhase>('idle');
  const [campaign, setCampaign] = useState<CampaignState | null>(null);
  const [loops, setLoops] = useState<Loop[]>([]);
  const [events, setEvents] = useState<LedgerEvent[]>([]);
  const [nowTick, setNowTick] = useState(0);

  // Sovereign shell state
  const [surface, setSurface] = useState<DockSurface>('arena');
  const [arenaView, setArenaView] = useState<ArenaView>('orbital');
  const [recenterSignal, setRecenterSignal] = useState(0);
  const [proofsTab, setProofsTab] = useState<'manuscripts' | 'tribunal' | 'gallery' | 'circuits'>('manuscripts');
  const [meshTab, setMeshTab] = useState<'mesh' | 'guardrails' | 'models' | 'labs' | 'settings'>('mesh');
  const [showLegacyCanvas, setShowLegacyCanvas] = useState(false);
  const [gateState, setGateState] = useState<Record<string, 'authorized' | 'denied' | 'downgraded'>>({});
  const [gateNotice, setGateNotice] = useState<string | null>(null);
  const [canvasWidget, setCanvasWidget] = useState<CanvasWidgetSpec | null>(null);
  // Run-state machine (Phase 1 awareness): what the swarm is doing right now.
  const [runPhase, setRunPhase] = useState<RunPhase>('idle');
  const [runDetail, setRunDetail] = useState('');
  const [runElapsedSec, setRunElapsedSec] = useState(0);
  const runPhaseRef = useRef<RunPhase>('idle');
  const [toast, setToast] = useState<{ key: number; text: string } | null>(null);
  const lastToastEventId = useRef(0);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Modals & Panels Visibility
  const [isAboutOpen, setIsAboutOpen] = useState(false);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);
  const [isShortcutsOpen, setIsShortcutsOpen] = useState(false);
  const [isNewCampaignModalOpen, setIsNewCampaignModalOpen] = useState(false);
  const [showCopilot, setShowCopilot] = useState(true);
  const [isCopilotExpanded, setIsCopilotExpanded] = useState(false);
  const [showCampaignMenu, setShowCampaignMenu] = useState(false);

  const refreshData = useCallback(async () => {
    const sid = currentSessionId || undefined;
    const [sessList, loopList, evtList] = await Promise.all([
      fetchSessions(),
      fetchLoops(sid),
      fetchEvents(sid),
    ]);
    if (sessList.length > 0) {
      setSessions(sessList);
    }
    setLoops(loopList);
    setEvents(evtList);
  }, [currentSessionId]);

  useEffect(() => {
    if (currentSessionId) {
      fetchCampaignState(currentSessionId).then(setCampaign);
    }
  }, [currentSessionId]);

  const switchSession = useCallback((id: string) => {
    setCanvasWidget(null);
    setCurrentSessionId(id);
  }, []);

  // Focused loop refetch for override actions (U1-c): the server owns
  // loop state, so every accepted pause/resume/kill re-reads it.
  const refreshLoops = useCallback(async () => {
    const loopList = await fetchLoops(currentSessionId || undefined);
    setLoops(loopList);
  }, [currentSessionId]);

  // U2-a: an imported (or replayed) session becomes current — switch to
  // it and load its campaign, same as a fresh creation.
  const handleSessionImported = useCallback(async (sessionId: string) => {
    switchSession(sessionId);
    const camp = await fetchCampaignState(sessionId);
    if (camp) setCampaign(camp);
    refreshData();
  }, [switchSession, refreshData]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      // Runtime backend URL first (M5): same image, any backend.
      await configureApiBase();
      const [sessList, loopList] = await Promise.all([
        fetchSessions(),
        fetchLoops(),
      ]);
      if (!mounted) return;
      let targetSessionId = currentSessionId;
      if (sessList.length === 0) {
        if (!bootstrappedSession.current) {
          bootstrappedSession.current = true;
          const firstSess = await createSession('Quantum Research Campaign #1', 150.0);
          if (firstSess && mounted) {
            setSessions([firstSess]);
            targetSessionId = firstSess.id;
            switchSession(firstSess.id);
            fetchCampaignState(firstSess.id).then((c) => {
              if (mounted) setCampaign(c);
            });
          }
        }
      } else {
        setSessions(sessList);
        if (!currentSessionId) {
          targetSessionId = sessList[0].id;
          switchSession(sessList[0].id);
          fetchCampaignState(sessList[0].id).then((c) => {
            if (mounted) setCampaign(c);
          });
        }
      }
      setLoops(loopList);
      if (targetSessionId) {
        fetchEvents(targetSessionId).then((e) => {
          if (mounted) setEvents(e);
        });
      }
    })();

    // D4 (2.5b): aggregate heartbeat toasts (one per poll, never per event),
    // skip fetch work while the tab is hidden, back off to 15s at rest.
    // The 1s clock below is untouched (elapsed displays need it).
    let tickTimer: ReturnType<typeof setTimeout>;
    const tick = () => {
      if (!mounted) return;
      if (!document.hidden) {
        fetchLoops(currentSessionId || undefined).then((l) => { if (mounted) setLoops(l); });
        if (currentSessionId) {
          fetchEvents(currentSessionId).then((e) => {
            if (!mounted) return;
            setEvents(e);
            // Heartbeat toasts: while a run is live, surface fresh ledger events
            // on the canvas (max one per poll, auto-fading). Baseline on run start.
            if (prevPhase.current === 'idle' && runPhase !== 'idle') {
              if (e.length > 0) lastToastEventId.current = Math.max(...e.map((ev) => ev.id));
            } else if (runPhase !== 'idle' && e.length > 0) {
              const fresh = e.filter((ev) => ev.id > lastToastEventId.current);
              if (lastToastEventId.current > 0 && fresh.length > 0) {
                const last = fresh[fresh.length - 1];
                const label = String(last.event_type || 'event').replace(/_/g, ' ');
                const text = fresh.length === 1 ? `${label} · recorded` : `${fresh.length} new events — view recorder`;
                setToast({ key: last.id, text });
                if (toastTimer.current) clearTimeout(toastTimer.current);
                toastTimer.current = setTimeout(() => setToast(null), 4000);
              }
              lastToastEventId.current = Math.max(...e.map((ev) => ev.id));
            }
            prevPhase.current = runPhase;
          });
        }
      }
      const atRest = runPhaseRef.current === 'idle';
      tickTimer = setTimeout(tick, document.hidden || atRest ? 15000 : 3500);
    };
    tickTimer = setTimeout(tick, 3500);
    const clock = setInterval(() => {
      setNowTick((t) => t + 1);
      if (runPhaseRef.current !== 'idle') setRunElapsedSec((s) => s + 1);
    }, 1000);
    return () => {
      mounted = false;
      clearTimeout(tickTimer);
      clearInterval(clock);
    };
  }, [currentSessionId, refreshData, switchSession, runPhase]);
  const handleCreateSession = useCallback(() => {
    setIsNewCampaignModalOpen(true);
  }, []);

  const handleConfirmCreateCampaign = useCallback(
    async (params: {
      title: string;
      targetMolecule: string;
      spendLimitUSD: number;
      fresh: boolean;
    }) => {
      const newSess = await createSession(
        params.title,
        params.spendLimitUSD,
        params.targetMolecule,
        params.fresh
      );
      if (newSess) {
        setSessions((prev) => [newSess, ...prev]);
        switchSession(newSess.id);
        const newCamp = await fetchCampaignState(newSess.id);
        if (newCamp) {
          setCampaign(newCamp);
        }
      }
    },
    [switchSession]
  );

  const handleDeleteSession = useCallback(
    async (e: React.MouseEvent, sessionId: string) => {
      e.stopPropagation();
      const sessionToDelete = sessions.find((s) => s.id === sessionId);
      const name = sessionToDelete?.title || 'this campaign';
      if (!confirm(`Delete "${name}"? This will permanently remove its state and transcripts.`)) {
        return;
      }
      const success = await deleteSession(sessionId);
      if (success) {
        setSessions((prev) => {
          const filtered = prev.filter((s) => s.id !== sessionId);
          if (currentSessionId === sessionId) {
            const nextSession = filtered[0];
            if (nextSession) {
              switchSession(nextSession.id);
              fetchCampaignState(nextSession.id).then((c) => {
                if (c) setCampaign(c);
              });
            } else {
              switchSession('');
              setCampaign(null);
            }
          }
          return filtered;
        });
      }
    },
    [sessions, currentSessionId, switchSession]
  );

  const handleEmergencyAbort = useCallback(async () => {
    if (!currentSessionId) {
      alert('No active session: open or create a session before triggering an abort.');
      return;
    }
    if (confirm('EMERGENCY ABORT: Settle every unsettled job in this session as failed?')) {
      try {
        const res = await apiFetch(`${API_BASE}/api/supervisor/abort`, {
          method: 'POST',
          headers: authHeaders({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ session_id: currentSessionId }),
        });
        if (!res.ok) {
          throw new Error(`supervisor refused the abort (HTTP ${res.status})`);
        }
        const data = await res.json();
        alert(
          `Ring 0 Reflex Intercept complete: ${data.aborted_count} job(s) settled failed.\nAbort receipt: ${data.abort_receipt}`
        );
      } catch (err) {
        alert(`Abort failed and nothing was settled: ${err instanceof Error ? err.message : String(err)}`);
      }
    }
  }, [currentSessionId]);

  const handleSelectDockSurface = useCallback((s: DockSurface) => {
    setSurface(s);
    if (s === 'arena') setShowLegacyCanvas(false);
  }, []);

  const handleCopilotSurface = useCallback((surf: string) => {
    if (surf === 'surface1') setSurface('arena');
    else if (surf === 'surface7') { setSurface('proofs'); setProofsTab('manuscripts'); }
    else setSurface(surf as DockSurface);
  }, []);

  const handleMaterialize = useCallback((kind: CanvasWidgetKind, title: string, jobId?: string, sessionId?: string) => {
    setCanvasWidget({ kind, title, jobId, sessionId: sessionId ?? currentSessionId });
    setSurface('arena');
  }, [currentSessionId]);

  const handleRunStateChange = useCallback((phase: RunPhase, detail: string) => {
    runPhaseRef.current = phase;
    setRunPhase(phase);
    setRunDetail(detail);
    if (phase !== 'idle') setRunElapsedSec(0);
  }, []);

  // Prompt → probe mapping: the canvas lights the agent being asked.
  const probeForText = useCallback((text: string): ProbeId | null => {
    const t = text.toLowerCase();
    if (/proof|lean|theorem|formal|verif/.test(t)) return 'formalist';
    if (/circuit|qubit|backend|qasm|shots|compiler|transpil|heron|aquila/.test(t)) return 'compiler';
    if (/risk|plateau|fidelity|canary|critic|trap|variance/.test(t)) return 'critic';
    if (/scout/.test(t)) return 'theorist';
    if (/auditor/.test(t)) return 'compiler';
    if (/scribe/.test(t)) return 'formalist';
    if (/disconfirmation|officer/.test(t)) return 'critic';
    if (/energy|ansatz|vqe|pes|surface|molecule|h2|lih|beh2|ground state|tribunal|swarm/.test(t)) return 'theorist';
    return null;
  }, []);

  const handleAuthorizePulse = useCallback(() => {
    setGateState((g) => ({ ...g, [currentSessionId]: 'authorized' }));
    setGateNotice('Pulse authorized: mandate verified & recorded. Gate closed.');
    setTimeout(() => setGateNotice(null), 6000);
    refreshData();
  }, [currentSessionId, refreshData]);

  const handleDenyPulse = useCallback((reason: string) => {
    setGateState((g) => ({ ...g, [currentSessionId]: 'denied' }));
    setGateNotice(`Pulse denied (${reason}). Halted fail-closed — nothing dispatched.`);
    setTimeout(() => setGateNotice(null), 6000);
  }, [currentSessionId]);

  const handleDowngradePulse = useCallback(() => {
    setGateState((g) => ({ ...g, [currentSessionId]: 'downgraded' }));
    setGateNotice('Downgraded to local simulator — dispatch via chat launch or Simulate Offline (L1 auto-allow, $0.00).');
    setTimeout(() => setGateNotice(null), 6000);
  }, [currentSessionId]);

  const gateOpen =
    Boolean(campaign) &&
    isMeteredBackend(campaign?.quantum_factors?.recommended_backend) &&
    !gateState[currentSessionId];

  // Run-state derivation: overdue, lit probe, HUD status line.
  const runOverdue = runPhase !== 'idle' && runElapsedSec > 90;
  const activeProbe: ProbeId | null = runPhase === 'idle'
    ? null
    : runOverdue
      ? 'critic'
      : probeForText(runDetail);
  const runStatusText = runPhase === 'idle'
    ? null
    : runOverdue
      ? `still working · ${runElapsedSec}s`
      : runPhase === 'tribunal'
        ? `${runDetail} · ${runElapsedSec}s`
        : `reasoning · ${runElapsedSec}s`;

  const handleRequestGateFocus = useCallback(() => {
    setGateNotice(
      gateOpen
        ? 'Confirm in the Action Gate (bottom left) — the only place pulses authorize.'
        : 'Gate already decided for this session.'
    );
    setTimeout(() => setGateNotice(null), 6000);
  }, [gateOpen]);

  // Global Keyboard Shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const isCmdOrCtrl = e.metaKey || e.ctrlKey;

      if (isCmdOrCtrl && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsCommandPaletteOpen((prev) => !prev);
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
      } else if (isCmdOrCtrl && ['1', '2', '3', '4', '5', '6'].includes(e.key)) {
        e.preventDefault();
        const dockMap: Record<string, DockSurface> = {
          '1': 'arena', '2': 'recorder', '3': 'manifold',
          '4': 'fleet', '5': 'proofs', '6': 'mesh',
        };
        handleSelectDockSurface(dockMap[e.key]);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleCreateSession, handleEmergencyAbort, handleSelectDockSurface]);

  const session = sessions.find((s) => s.id === currentSessionId) || null;
  const targetShort = campaign?.quantum_factors?.target_molecule || campaign?.target_system || '';
  const targetLabel = targetShort ? `${targetShort.slice(0, 28)} · Ground State Target` : 'No Target · Idle';
  void nowTick;

  const subToggle = (options: Array<{ id: string; label: string }>, value: string, onChange: (id: string) => void) => (
    <div className="flex items-center justify-center pt-4">
      <div className="bg-[#0E1117]/85 backdrop-blur-xl border border-white/10 rounded-full p-1 flex items-center shadow-2xl">
        {options.map((o) => (
          <button
            key={o.id}
            onClick={() => onChange(o.id)}
            className={`px-3.5 py-1.5 rounded-full text-xs font-medium transition-all ${
              value === o.id ? 'bg-white/10 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );

  return (
    <div className="flex h-screen w-screen bg-[#07080B] text-slate-100 overflow-hidden font-sovereign select-none">
      <AttentionProvider>
      {/* Modals & Dialogs */}
      <AboutLabModal isOpen={isAboutOpen} onClose={() => setIsAboutOpen(false)} />
      <CommandPalette
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        onSelectSurface={(surfId) => {
          if (surfId === 'tribunal') {
            setSurface('proofs');
            setProofsTab('tribunal');
          } else if (surfId === 'gallery') {
            setSurface('proofs');
            setProofsTab('gallery');
          } else if (surfId === 'guardrails') {
            setSurface('mesh');
            setMeshTab('guardrails');
          } else if (surfId === 'models') {
            setSurface('mesh');
            setMeshTab('models');
          } else if (surfId === 'labs') {
            setSurface('mesh');
            setMeshTab('labs');
          } else if (surfId === 'settings') {
            setSurface('mesh');
            setMeshTab('settings');
          } else {
            handleSelectDockSurface(surfId as DockSurface);
          }
        }}
        onNewCampaign={handleCreateSession}
        onEmergencyAbort={handleEmergencyAbort}
        onOpenAbout={() => setIsAboutOpen(true)}
      />
      <KeyboardShortcutsModal
        isOpen={isShortcutsOpen}
        onClose={() => setIsShortcutsOpen(false)}
      />
      <NewCampaignModal
        isOpen={isNewCampaignModalOpen}
        onClose={() => setIsNewCampaignModalOpen(false)}
        onCreate={handleConfirmCreateCampaign}
      />

      {/* Icon-only sovereign dock */}
      <SovereignDock
        active={surface}
        onSelect={handleSelectDockSurface}
        airGapSovereign
        onOpenSettings={() => { setSurface('mesh'); setMeshTab('settings'); }}
        onOpenCampaigns={() => setShowCampaignMenu((v) => !v)}
      />

      {/* Campaign menu (avatar popover) */}
      {showCampaignMenu && (
        <div className="absolute left-20 bottom-6 z-50 w-80 rounded-2xl bg-[#0E1117]/95 backdrop-blur-2xl border border-white/10 shadow-2xl p-3 space-y-2">
          <div className="flex items-center justify-between px-1">
            <span className="text-[10px] font-code uppercase tracking-wider text-slate-400">
              Campaigns ({sessions.length})
            </span>
            <button
              onClick={() => { setShowCampaignMenu(false); handleCreateSession(); }}
              className="flex items-center gap-1 px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] hover:bg-cyan-500/25 transition-all"
            >
              <Plus className="w-3 h-3" /> New <kbd className="font-code text-[9px]">⌘N</kbd>
            </button>
          </div>
          <div className="max-h-64 overflow-y-auto space-y-1.5">
            {sessions.map((s) => {
              const isSelected = s.id === currentSessionId;
              const spend = s.current_spend_usd || 0;
              const limit = s.spend_limit_usd || 150;
              return (
                <div key={s.id} className="relative group">
                  <button
                    onClick={() => { switchSession(s.id); setShowCampaignMenu(false); }}
                    className={`w-full text-left p-2.5 pr-8 rounded-xl text-xs transition-all border ${
                      isSelected
                        ? 'bg-cyan-500/10 border-cyan-500/40 text-white'
                        : 'bg-black/40 border-white/[0.06] text-slate-300 hover:bg-white/[0.04]'
                    }`}
                  >
                    <div className="font-semibold truncate">{s.title}</div>
                    <div className="text-[10px] font-code text-slate-400 flex justify-between mt-0.5">
                      <span className="flex items-center gap-1">
                        <Clock className="w-3 h-3" />
                        {new Date(s.created_at).toLocaleDateString()}
                      </span>
                      <span className="text-amber-300">${spend.toFixed(2)} / ${limit.toFixed(2)}</span>
                    </div>
                  </button>
                  <button
                    onClick={(e) => handleDeleteSession(e, s.id)}
                    aria-label={`Delete ${s.title}`}
                    className="absolute top-2 right-2 p-1.5 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-rose-500/15 opacity-0 group-hover:opacity-100 transition-all"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              );
            })}
          </div>
          <div className="flex items-center justify-between pt-2 border-t border-white/[0.06] text-[11px]">
            <button onClick={() => { setShowCampaignMenu(false); handleEmergencyAbort(); }} className="flex items-center gap-1.5 text-red-300 hover:text-red-200 transition-colors">
              <Zap className="w-3.5 h-3.5" /> Ring 0 Intercept
            </button>
            <button onClick={() => { setShowCampaignMenu(false); setIsAboutOpen(true); }} className="flex items-center gap-1.5 text-slate-300 hover:text-white transition-colors">
              <Info className="w-3.5 h-3.5" /> Lab Info
            </button>
            <button onClick={() => { setShowCampaignMenu(false); setIsShortcutsOpen(true); }} className="flex items-center gap-1 text-slate-300 hover:text-white transition-colors">
              <HelpCircle className="w-3.5 h-3.5" /> <span className="font-code">?</span>
            </button>
          </div>
        </div>
      )}

      {/* Center stage */}
      <div className="flex-1 flex flex-col min-w-0 relative grid-bg">
        {/* U1-a: backend liveness always visible — distinguishes "empty"
            from "backend down" on every surface below. */}
        <BackendStatusBar />
        <CheckInStrip loops={loops} />
        {surface === 'arena' && (
          <main className="relative flex-1 h-full overflow-hidden">
            <ArenaHud
              targetLabel={targetLabel}
              swarmDetail={runPhase === 'idle' ? 'idle · at rest' : `${loops.length} Loops Live`}
              view={arenaView}
              onViewChange={setArenaView}
              spendUsd={session?.current_spend_usd ?? 0}
              spendCapUsd={session?.spend_limit_usd ?? 150}
              sessionElapsed={session ? formatElapsed(session.created_at) : '00:00:00'}
              statusText={runStatusText}
              onRecenter={() => setRecenterSignal((n) => n + 1)}
            />

            {arenaView === 'orbital' && !showLegacyCanvas && (
              <SwarmUniverse campaign={campaign} recenterSignal={recenterSignal} activeProbe={activeProbe} atRest={runPhase === 'idle'} />
            )}
            {arenaView === 'proof-dag' && !showLegacyCanvas && (
              <ProofDagView campaign={campaign} />
            )}
            {arenaView === 'hardware-qpu' && !showLegacyCanvas && (
              <div className="absolute inset-0 overflow-y-auto pt-20 px-6 pb-6">
                <FleetRadar campaign={campaign} />
              </div>
            )}
            {showLegacyCanvas && (
              <div className="absolute inset-0 overflow-y-auto pt-20 px-6 pb-6 bg-[#07080B]/95">
                <div className="flex items-center justify-end max-w-6xl mx-auto pb-2">
                  <button
                    onClick={() => setShowLegacyCanvas(false)}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-white/5 border border-white/10 text-[11px] text-slate-300 hover:text-white transition-all"
                  >
                    <X className="w-3.5 h-3.5" /> Back to Universe
                  </button>
                </div>
                <SpatialCanvas campaign={campaign} />
              </div>
            )}

            {/* Legacy canvas entry point */}
            {!showLegacyCanvas && (
              <button
                onClick={() => setShowLegacyCanvas(true)}
                title="Living Canvas (molecule workbench)"
                aria-label="Living Canvas (molecule workbench)"
                className="absolute bottom-6 right-6 z-20 w-9 h-9 rounded-full bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 hover:border-cyan-400/40 transition-all shadow-2xl flex items-center justify-center"
              >
                <Box className="w-4 h-4 text-cyan-400" />
              </button>
            )}

            {/* On-demand canvas widget (co-pilot materialized, single slot) */}
            {!showLegacyCanvas && (
              <WidgetStage
                widget={canvasWidget}
                campaign={campaign}
                gateOpen={gateOpen}
                spendUsd={session?.current_spend_usd ?? 0}
                spendCapUsd={session?.spend_limit_usd ?? 150}
                onDismiss={() => setCanvasWidget(null)}
                onRequestGateFocus={handleRequestGateFocus}
              />
            )}

            {/* Calibrated action gate — only while authorization is outstanding */}
            {gateOpen && !showLegacyCanvas && (
              <ActionGate
                campaign={campaign}
                session={session}
                onAuthorized={handleAuthorizePulse}
                onDenied={handleDenyPulse}
                onDowngrade={handleDowngradePulse}
              />
            )}
            {gateNotice && (
              <div className="absolute bottom-6 left-6 z-20 w-80 p-3 rounded-2xl bg-[#0E1117]/90 border border-white/10 text-[11px] text-slate-200 shadow-2xl">
                {gateNotice}
              </div>
            )}
            {toast && (
              <div
                key={toast.key}
                className="absolute bottom-6 left-1/2 -translate-x-1/2 z-20 px-4 py-2 rounded-full bg-[#0E1117]/90 backdrop-blur-xl border border-cyan-400/25 text-[11px] font-code text-cyan-200 shadow-2xl animate-pulse"
              >
                {toast.text}
              </div>
            )}
          </main>
        )}

        {surface !== 'arena' && (
          <main className="flex-1 overflow-y-auto">
            {surface === 'proofs' && subToggle(
              [
                { id: 'manuscripts', label: 'Manuscripts' },
                { id: 'tribunal', label: 'Tribunal' },
                { id: 'gallery', label: 'Preview Gallery' },
                { id: 'circuits', label: 'Circuit Codesign' },
              ],
              proofsTab,
              (id) => setProofsTab(id as 'manuscripts' | 'tribunal' | 'gallery' | 'circuits')
            )}
            {surface === 'mesh' && subToggle(
              [
                { id: 'mesh', label: 'Cognitive Mesh' },
                { id: 'guardrails', label: 'Guardrails' },
                { id: 'models', label: 'Model Hub' },
                { id: 'labs', label: 'Laboratories' },
                { id: 'settings', label: 'Sovereign Settings' },
              ],
              meshTab,
              (id) => setMeshTab(id as typeof meshTab)
            )}
            <div className="p-6 max-w-6xl mx-auto">
              {surface === 'recorder' && <FlightRecorder loops={loops} events={events} campaign={campaign} onLoopsChanged={refreshLoops} onSessionImported={handleSessionImported} />}
              {surface === 'manifold' && <EpistemicManifold campaign={campaign} sessionId={currentSessionId || null} />}
              {surface === 'fleet' && <FleetRadar campaign={campaign} />}
              {surface === 'proofs' && proofsTab === 'manuscripts' && <ResultsStudio campaign={campaign} sessionId={currentSessionId || null} />}
              {surface === 'proofs' && proofsTab === 'tribunal' && (
                <EpistemicTribunalRoom
                  sessionId={session?.id ?? ''}
                  targetSystem={campaign?.target_system || campaign?.quantum_factors?.target_molecule || 'LiH'}
                  hypothesisId={campaign?.hypothesis_id || `hyp-qksd-${(campaign?.quantum_factors?.target_molecule || campaign?.target_system || 'LiH').toLowerCase().replace(/[^a-z0-9]/g, '')}-01`}
                  campaign={campaign}
                  onCampaignSynthesized={(c) => setCampaign(c)}
                />
              )}
              {surface === 'proofs' && proofsTab === 'gallery' && <GenerativeUIStudio campaign={campaign} />}
              {surface === 'proofs' && proofsTab === 'circuits' && <CodesignCockpit />}
              {surface === 'mesh' && meshTab === 'mesh' && <CognitiveMesh campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'guardrails' && <AutonomyCockpit campaign={campaign} session={session} />}
              {surface === 'mesh' && meshTab === 'models' && <HuggingFaceModelHub />}
              {surface === 'mesh' && meshTab === 'labs' && <LabsView />}
              {surface === 'mesh' && meshTab === 'settings' && <LLMSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumDispatch sessionId={currentSessionId || null} />}
            </div>
          </main>
        )}
      </div>

      {/* Right: native co-pilot surface */}
      {showCopilot ? (
        <aside
          aria-label="Kubona co-pilot"
          className={`${
            isCopilotExpanded ? 'w-[640px] xl:w-[700px]' : 'w-96'
          } flex flex-col shrink-0 border-l border-white/5 bg-[#0A0C10]/95 backdrop-blur-2xl transition-all duration-300 shadow-2xl overflow-hidden`}
        >
          <CopilotDrawer
            sessionId={currentSessionId}
            campaign={campaign}
            isExpanded={isCopilotExpanded}
            onToggleExpand={() => setIsCopilotExpanded((prev) => !prev)}
            onExperimentLaunched={() => {
              refreshData();
              if (currentSessionId) fetchCampaignState(currentSessionId).then(setCampaign);
            }}
            onCampaignUpdated={(c) => setCampaign(c)}
            onSelectSurface={handleCopilotSurface}
            onMaterialize={handleMaterialize}
            onRunStateChange={handleRunStateChange}
            onSessionChange={(id) => {
              setCurrentSessionId(id);
              refreshData();
            }}
            onClose={() => setShowCopilot(false)}
          />
        </aside>
      ) : (
        <button
          onClick={() => setShowCopilot(true)}
          className="fixed bottom-6 right-6 z-40 px-4 py-2.5 rounded-full bg-[#0C101A] border border-cyan-500/40 hover:border-cyan-400 text-white shadow-2xl backdrop-blur-2xl flex items-center space-x-2.5 group transition-all duration-200 hover:scale-105 active:scale-95"
          title="Open Kubona Copilot (⌘J)"
        >
          <div className="relative">
            <Sparkles className="w-4 h-4 text-cyan-400 animate-float-gentle" />
            <span className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
          </div>
          <span className="text-xs font-semibold text-slate-200 group-hover:text-white">
            Kubona Copilot
          </span>
          <kbd className="px-1.5 py-0.5 rounded bg-black/50 text-[10px] font-code text-cyan-300 border border-white/10">
            ⌘J
          </kbd>
        </button>
      )}
      </AttentionProvider>
    </div>
  );
}
