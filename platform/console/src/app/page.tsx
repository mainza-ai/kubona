'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Sparkles, Plus, Zap, Info, HelpCircle, Trash2, Clock, Box, X } from 'lucide-react';
import { Session, Loop, LedgerEvent, CampaignState, RunPhase, ProbeId } from '@/types';
import { fetchSessions, createSession, deleteSession, fetchLoops, fetchEvents, fetchCampaignState, API_BASE, authHeaders, configureApiBase, apiFetch, UNAUTH_EVENT } from '@/lib/api';
import { SHORTCUTS, SHORTCUT_TARGETS, KEY_TO_SHORTCUT, type ShortcutKey } from '@/lib/shortcuts';
import { CopilotDrawer } from '@/components/CopilotDrawer';
import { AboutLabModal } from '@/components/AboutLabModal';
import { CommandPalette } from '@/components/CommandPalette';
import { KeyboardShortcutsModal } from '@/components/KeyboardShortcutsModal';
import ConfirmDialog from '@/components/common/ConfirmDialog';
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
import { FlightRecorderCard } from '@/components/FlightRecorderCard';
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
  const mm = String(Math.floor((s % 60) / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

// Session-expired banner: subscribes to UNAUTH_EVENT (dispatched by api.ts
// on any HTTP 401) and surfaces "log in again" with a jump straight to the
// mesh→labs AuthPanel. Dismissible; unsubscribes on unmount. Exported for
// tests — Home mounts exactly one instance below.
export function SessionExpiredBanner({ onOpenLabs }: { onOpenLabs: () => void }) {
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const show = () => setVisible(true);
    window.addEventListener(UNAUTH_EVENT, show);
    return () => window.removeEventListener(UNAUTH_EVENT, show);
  }, []);
  if (!visible) return null;
  return (
    <div
      data-testid="auth-expired-banner"
      role="alert"
      className="flex items-center justify-between gap-3 px-4 py-2 bg-amber-950/90 border-b border-amber-500/40 text-amber-100 text-xs"
    >
      <span>Session expired — log in again.</span>
      <span className="flex items-center gap-2 shrink-0">
        <button
          data-testid="auth-banner-login"
          onClick={onOpenLabs}
          className="px-2.5 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 hover:bg-cyan-500/25 transition-all"
        >
          Log in (Mesh → Labs)
        </button>
        <button
          data-testid="auth-banner-dismiss"
          onClick={() => setVisible(false)}
          aria-label="Dismiss session expired banner"
          className="p-1 rounded-lg text-amber-200/70 hover:text-amber-100 hover:bg-white/10 transition-all"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </span>
    </div>
  );
}

// Verbatim failure for a refused campaign creation. createSession resolves
// null on refusal (no body to quote) and the modal cannot render it (owns
// its own close), so the shell keeps the modal open and renders this text
// itself — never a silent close.
export function describeCreateFailure(result: Session | null, error: unknown): string | null {
  if (result) return null;
  if (error !== undefined && error !== null) {
    const detail = error instanceof Error ? error.message : String(error);
    return `Create campaign failed: ${detail}`;
  }
  return 'Create campaign failed: backend refused (createSession returned null) — nothing was created.';
}

// Deep-link URL sync (Wave-2 shell): the dock surface (+ the active sub-tab
// for proofs/mesh/arena) round-trips through ?surface= & ?tab= so views are
// bookmarkable and palette/dock navigation updates the URL without reload.
// Unknown values fall back to defaults — never a crash. Pure (no window
// access) so tests can exercise it without mounting Home.
export type ProofsTab = 'manuscripts' | 'tribunal' | 'gallery' | 'circuits';
export type MeshTab = 'mesh' | 'guardrails' | 'models' | 'labs' | 'settings';

export interface ShellParams {
  surface: DockSurface;
  proofsTab: ProofsTab;
  meshTab: MeshTab;
  arenaView: ArenaView;
}

const SHELL_DEFAULTS: ShellParams = {
  surface: 'arena',
  proofsTab: 'manuscripts',
  meshTab: 'mesh',
  arenaView: 'orbital',
};

const VALID_SURFACES: DockSurface[] = ['arena', 'recorder', 'manifold', 'fleet', 'proofs', 'mesh'];
const VALID_PROOFS_TABS: ProofsTab[] = ['manuscripts', 'tribunal', 'gallery', 'circuits'];
const VALID_MESH_TABS: MeshTab[] = ['mesh', 'guardrails', 'models', 'labs', 'settings'];
const VALID_ARENA_VIEWS: ArenaView[] = ['orbital', 'proof-dag', 'hardware-qpu'];

export function parseShellParams(search: string): ShellParams {
  const out: ShellParams = { ...SHELL_DEFAULTS };
  try {
    const params = new URLSearchParams(search);
    const surface = params.get('surface');
    if (surface !== null && (VALID_SURFACES as string[]).includes(surface)) {
      out.surface = surface as DockSurface;
    }
    const tab = params.get('tab');
    if (tab === null) return out;
    if (out.surface === 'proofs' && (VALID_PROOFS_TABS as string[]).includes(tab)) {
      out.proofsTab = tab as ProofsTab;
    } else if (out.surface === 'mesh' && (VALID_MESH_TABS as string[]).includes(tab)) {
      out.meshTab = tab as MeshTab;
    } else if (out.surface === 'arena' && (VALID_ARENA_VIEWS as string[]).includes(tab)) {
      out.arenaView = tab as ArenaView;
    }
    return out;
  } catch {
    return { ...SHELL_DEFAULTS };
  }
}

/** Sub-tab carried in ?tab= for the given surface (null = no tab param). */
export function tabForSurface(surface: DockSurface, tabs: Pick<ShellParams, 'proofsTab' | 'meshTab' | 'arenaView'>): string | null {
  switch (surface) {
    case 'proofs':
      return tabs.proofsTab;
    case 'mesh':
      return tabs.meshTab;
    case 'arena':
      return tabs.arenaView;
    default:
      return null;
  }
}

export function buildShellSearch(surface: DockSurface, tab: string | null): string {
  const params = new URLSearchParams();
  params.set('surface', surface);
  if (tab !== null && tab !== '') params.set('tab', tab);
  const s = params.toString();
  return s ? `?${s}` : '';
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
  // Tick serialization (no overlapping polls) + outage-episode toast latch:
  // exactly one "backend unreachable" toast per outage, reset on success.
  const tickBusy = useRef(false);
  const outageNotified = useRef(false);

  // Modals & Panels Visibility
  const [isAboutOpen, setIsAboutOpen] = useState(false);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);
  const [isShortcutsOpen, setIsShortcutsOpen] = useState(false);
  const [isNewCampaignModalOpen, setIsNewCampaignModalOpen] = useState(false);
  // createSession null/refusal text, rendered verbatim while the modal stays
  // open. Mirrored in a ref because the modal calls onClose synchronously
  // after onCreate resolves — state alone would not have flushed yet.
  const [createError, setCreateError] = useState<string | null>(null);
  const createFailedRef = useRef(false);
  // Responsive shell (Wave-2): below lg the copilot is a closed-by-default
  // overlay drawer, not a fixed side panel. Default closed on narrow screens
  // via matchMedia at mount; the existing toggle reopens it as an overlay.
  const [showCopilot, setShowCopilot] = useState(() => {
    if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
      try {
        if (window.matchMedia('(max-width: 1023.98px)').matches) return false;
      } catch {
        // matchMedia unavailable (tests/SSR) — fall through to open default.
      }
    }
    return true;
  });
  const [isCopilotExpanded, setIsCopilotExpanded] = useState(false);
  const [showCampaignMenu, setShowCampaignMenu] = useState(false);
  const [campaignFilter, setCampaignFilter] = useState('');

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
    // Hardened: serialized (no overlapping ticks — a slow backend cannot
    // stack polls) with a single outage toast per episode (reset on the
    // next successful tick), so a dead backend nags once, not every 3.5s.
    // The 1s clock below is untouched (elapsed displays need it).
    let tickTimer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      if (!mounted) return;
      if (tickBusy.current) {
        tickTimer = setTimeout(tick, 3500);
        return;
      }
      tickBusy.current = true;
      try {
        if (!document.hidden) {
          const loopList = await fetchLoops(currentSessionId || undefined);
          if (!mounted) return;
          setLoops(loopList);
          if (currentSessionId) {
            const e = await fetchEvents(currentSessionId);
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
          }
        }
        // A clean tick closes the outage episode: the next failure toasts again.
        outageNotified.current = false;
      } catch (err) {
        if (!outageNotified.current) {
          outageNotified.current = true;
          const detail = err instanceof Error ? err.message : String(err);
          setToast({ key: Date.now(), text: `Backend unreachable — retrying (${detail.slice(0, 120)})` });
          if (toastTimer.current) clearTimeout(toastTimer.current);
          toastTimer.current = setTimeout(() => { if (mounted) setToast(null); }, 4000);
        }
      } finally {
        tickBusy.current = false;
        if (!mounted) return;
        const atRest = runPhaseRef.current === 'idle';
        tickTimer = setTimeout(tick, document.hidden || atRest ? 15000 : 3500);
      }
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
      // Null (refusal) or throw (transport) keeps the modal OPEN with the
      // failure rendered verbatim — closing would be silent success theater
      // (cf. the 2026-10-21 agent-registration audit). The modal calls
      // onClose after onCreate resolves, so the failure is latched in
      // createFailedRef and the onClose below refuses to close on it.
      let failure: string | null = null;
      try {
        const newSess = await createSession(
          params.title,
          params.spendLimitUSD,
          params.targetMolecule,
          params.fresh
        );
        failure = describeCreateFailure(newSess, null);
        if (!failure && newSess) {
          setSessions((prev) => [newSess, ...prev]);
          switchSession(newSess.id);
          const newCamp = await fetchCampaignState(newSess.id);
          if (newCamp) {
            setCampaign(newCamp);
          }
        }
      } catch (err) {
        failure = describeCreateFailure(null, err);
      }
      createFailedRef.current = failure !== null;
      setCreateError(failure);
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

  // Emergency abort via typed ConfirmDialog (Wave-2): the destructive
  // request (endpoint, body, refusal/success handling) is identical to the
  // old native-confirm flow — only the acknowledgement is now typed
  // (session short-id) instead of a blocking confirm(). Failure text is the
  // same string, surfaced in-dialog so the user can retry or cancel.
  const [isAbortOpen, setIsAbortOpen] = useState(false);
  const [abortBusy, setAbortBusy] = useState(false);
  const [abortError, setAbortError] = useState<string | null>(null);

  const handleEmergencyAbort = useCallback(() => {
    if (!currentSessionId) {
      alert('No active session: open or create a session before triggering an abort.');
      return;
    }
    setAbortError(null);
    setIsAbortOpen(true);
  }, [currentSessionId]);

  const handleConfirmAbort = useCallback(async () => {
    if (!currentSessionId) return;
    setAbortBusy(true);
    setAbortError(null);
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
      setIsAbortOpen(false);
      alert(
        `Ring 0 Reflex Intercept complete: ${data.aborted_count} job(s) settled failed.\nAbort receipt: ${data.abort_receipt}`
      );
    } catch (err) {
      setAbortError(`Abort failed and nothing was settled: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setAbortBusy(false);
    }
  }, [currentSessionId]);

  const handleSelectDockSurface = useCallback((s: DockSurface) => {
    setSurface(s);
    if (s === 'arena') setShowLegacyCanvas(false);
  }, []);

  // Single dispatch for keyboard shortcuts AND palette selections: targets
  // resolve through SHORTCUT_TARGETS (lib/shortcuts) so the two cannot
  // drift — every handler branch names a palette item id and vice versa.
  const navigateToTarget = useCallback((target: string) => {
    switch (target) {
      case 'arena':
      case 'recorder':
      case 'manifold':
      case 'fleet':
      case 'proofs':
      case 'mesh':
        handleSelectDockSurface(target as DockSurface);
        break;
      case 'guardrails':
        setSurface('mesh'); setMeshTab('guardrails'); break;
      case 'models':
        setSurface('mesh'); setMeshTab('models'); break;
      case 'labs':
        setSurface('mesh'); setMeshTab('labs'); break;
      case 'settings':
        setSurface('mesh'); setMeshTab('settings'); break;
      case 'tribunal':
        setSurface('proofs'); setProofsTab('tribunal'); break;
      case 'gallery':
        setSurface('proofs'); setProofsTab('gallery'); break;
      case 'manuscripts':
        setSurface('proofs'); setProofsTab('manuscripts'); break;
      case 'circuits':
        setSurface('proofs'); setProofsTab('circuits'); break;
      case 'arena-orbital':
        setSurface('arena'); setShowLegacyCanvas(false); setArenaView('orbital'); break;
      case 'arena-proof-dag':
        setSurface('arena'); setShowLegacyCanvas(false); setArenaView('proof-dag'); break;
      case 'palette':
        setIsCommandPaletteOpen((v) => !v); break;
      case 'copilot':
        setShowCopilot((v) => !v); break;
      case 'about':
        setIsAboutOpen(true); break;
      case 'new':
        setIsNewCampaignModalOpen(true); break;
      case 'abort':
        void handleEmergencyAbort(); break;
      case 'help':
        setIsShortcutsOpen(true); break;
      default:
        break;
    }
  }, [handleEmergencyAbort, handleSelectDockSurface]);

  // FleetRadar double-mount fix: FleetRadar owns radar+fleet pollers, so
  // mounting it in the arena hardware-qpu view AND on the dock fleet
  // surface polls the backend twice. The arena tab now jumps to the single
  // dock instance instead of mounting a second one (simplest correct —
  // no lifted state, no second poller).
  const handleArenaViewChange = useCallback((v: ArenaView) => {
    if (v === 'hardware-qpu') {
      setSurface('fleet');
      return;
    }
    setArenaView(v);
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

  // Global keyboard shortcuts — driven by lib/shortcuts (KEY_TO_SHORTCUT +
  // SHORTCUT_TARGETS), the same table the palette labels. Digits are
  // meta-only (Ctrl+digits keep switching browser tabs on non-macOS);
  // letters keep Cmd-or-Ctrl. Inside text inputs only the palette toggle
  // survives (the escape hatch) so typing is never stolen. '?' keeps its
  // historic no-modifier behavior outside inputs.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName;
      const inField =
        tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el?.isContentEditable === true;
      const key = e.key.toLowerCase();
      if (key === '?') {
        if (!inField) {
          e.preventDefault();
          navigateToTarget('help');
        }
        return;
      }
      const shortcutName = KEY_TO_SHORTCUT[key];
      if (!shortcutName) return;
      const target = SHORTCUT_TARGETS[shortcutName as ShortcutKey];
      const isDigit = /^[0-9]$/.test(e.key);
      const modOk = isDigit ? e.metaKey && !e.ctrlKey : e.metaKey || e.ctrlKey;
      if (!modOk) return;
      if (inField && target !== 'palette') return;
      e.preventDefault();
      navigateToTarget(target);
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [navigateToTarget]);

  // Deep-link URL sync (Wave-2): read ?surface=&?tab= once on mount (plus
  // back/forward via popstate), and reflect every navigation with
  // replaceState (no reload). Palette/dock clicks all funnel through the
  // surface/tab setters above, so the write effect below covers them.
  // hardware-qpu is a redirect at runtime (arena view jumps to the dock
  // fleet surface), so a deep link carrying it lands on fleet.
  const urlReady = useRef(false);
  useEffect(() => {
    if (typeof window === 'undefined' || urlReady.current) return;
    urlReady.current = true;
    const parsed = parseShellParams(window.location?.search ?? '');
    const landedSurface = parsed.surface === 'arena' && parsed.arenaView === 'hardware-qpu'
      ? 'fleet'
      : parsed.surface;
    setSurface(landedSurface);
    setProofsTab(parsed.proofsTab);
    setMeshTab(parsed.meshTab);
    setArenaView(parsed.arenaView === 'hardware-qpu' ? 'orbital' : parsed.arenaView);
  }, []);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onPopState = () => {
      const parsed = parseShellParams(window.location?.search ?? '');
      const landedSurface = parsed.surface === 'arena' && parsed.arenaView === 'hardware-qpu'
        ? 'fleet'
        : parsed.surface;
      setSurface(landedSurface);
      setProofsTab(parsed.proofsTab);
      setMeshTab(parsed.meshTab);
      setArenaView(parsed.arenaView === 'hardware-qpu' ? 'orbital' : parsed.arenaView);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);
  useEffect(() => {
    if (typeof window === 'undefined' || !urlReady.current) return;
    const tab = tabForSurface(surface, { proofsTab, meshTab, arenaView });
    const next = buildShellSearch(surface, tab);
    if (window.location?.search !== next && typeof window.history?.replaceState === 'function') {
      window.history.replaceState(null, '', `${window.location.pathname}${next}${window.location.hash}`);
    }
  }, [surface, proofsTab, meshTab, arenaView]);

  const session = sessions.find((s) => s.id === currentSessionId) || null;
  const targetShort = campaign?.quantum_factors?.target_molecule || campaign?.target_system || '';
  const targetLabel = targetShort ? `${targetShort.slice(0, 28)} · Ground State Target` : 'No Target · Idle';
  void nowTick;

  const subToggle = (options: Array<{ id: string; label: string }>, value: string, onChange: (id: string) => void) => (
    <div className="flex items-center justify-center px-4 pt-4">
      <div className="bg-[#0E1117]/85 backdrop-blur-xl border border-white/10 rounded-2xl sm:rounded-full p-1 flex flex-wrap items-center justify-center max-w-full shadow-2xl">
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
        onSelectSurface={navigateToTarget}
        onNewCampaign={handleCreateSession}
        onEmergencyAbort={handleEmergencyAbort}
        onOpenAbout={() => setIsAboutOpen(true)}
        onToggleCopilot={() => setShowCopilot((v) => !v)}
        onOpenShortcuts={() => setIsShortcutsOpen(true)}
      />
      <KeyboardShortcutsModal
        isOpen={isShortcutsOpen}
        onClose={() => setIsShortcutsOpen(false)}
      />
      <ConfirmDialog
        open={isAbortOpen}
        title="Emergency abort"
        body={
          <>
            EMERGENCY ABORT: settle every unsettled job in session{' '}
            <code className="rounded bg-white/10 px-1 font-mono">{currentSessionId}</code> as
            failed? This cannot be undone.
          </>
        }
        requireText={currentSessionId.slice(0, 8) || undefined}
        confirmLabel="Abort all jobs"
        danger
        busy={abortBusy}
        error={abortError}
        onConfirm={() => void handleConfirmAbort()}
        onCancel={() => {
          if (abortBusy) return;
          setIsAbortOpen(false);
          setAbortError(null);
        }}
      />
      <NewCampaignModal
        isOpen={isNewCampaignModalOpen}
        // A refused creation latches createFailedRef: the modal calls onClose
        // right after onCreate resolves, and closing there would swallow the
        // verbatim failure. Refuse the close; the error renders below.
        onClose={() => {
          if (createFailedRef.current) return;
          setCreateError(null);
          setIsNewCampaignModalOpen(false);
        }}
        onCreate={handleConfirmCreateCampaign}
      />
      {isNewCampaignModalOpen && createError && (
        <div
          data-testid="create-campaign-error"
          role="alert"
          className="fixed top-6 left-1/2 -translate-x-1/2 z-[60] max-w-lg px-4 py-2.5 rounded-2xl bg-rose-950/95 border border-rose-500/40 text-rose-100 text-xs shadow-2xl"
        >
          {createError}
        </div>
      )}

      {/* Icon-only sovereign dock */}
      <SovereignDock
        active={surface}
        onSelect={handleSelectDockSurface}
        airGapSovereign
        onOpenSettings={() => { setSurface('mesh'); setMeshTab('settings'); }}
        onOpenCampaigns={() => setShowCampaignMenu((v) => !v)}
      />

      {/* Campaign menu (avatar popover) — grouped by tenant, filterable.
          Tenant ids ride on the already-loaded sessions (same pattern as
          LabsView's byLab grouping); no extra fetch, no lab-metadata
          dependency — LabsView stays the source of truth for lab state. */}
      {showCampaignMenu ? (() => {
        const q = campaignFilter.trim().toLowerCase();
        const matches = (s: Session) =>
          q === '' ||
          (s.title || '').toLowerCase().includes(q) ||
          s.id.toLowerCase().includes(q) ||
          (s.tenant_id || 'default').toLowerCase().includes(q);
        const groups = new Map<string, Session[]>();
        for (const s of sessions) {
          if (!matches(s)) continue;
          const tenant = s.tenant_id || 'default';
          if (!groups.has(tenant)) groups.set(tenant, []);
          groups.get(tenant)!.push(s);
        }
        const ordered = [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
        return (
        <div className="absolute left-14 lg:left-20 bottom-6 z-50 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] rounded-2xl bg-[#0E1117]/95 backdrop-blur-2xl border border-white/10 shadow-2xl p-3 space-y-2">
          <div className="flex items-center justify-between px-1">
            <span className="text-[10px] font-code uppercase tracking-wider text-slate-400">
              Campaigns ({sessions.length})
            </span>
            <button
              onClick={() => { setShowCampaignMenu(false); handleCreateSession(); }}
              className="flex items-center gap-1 px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] hover:bg-cyan-500/25 transition-all"
            >
              <Plus className="w-3 h-3" /> New <kbd className="font-code text-[9px]">{SHORTCUTS.newCampaign}</kbd>
            </button>
          </div>
          <input
            data-testid="campaign-filter"
            type="text"
            value={campaignFilter}
            onChange={(e) => setCampaignFilter(e.target.value)}
            placeholder="Filter by title or lab…"
            aria-label="Filter campaigns by title or lab"
            className="w-full px-2.5 py-1.5 rounded-xl bg-black/40 border border-white/10 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-cyan-400/50"
          />
          <div className="max-h-64 overflow-y-auto space-y-3">
            {ordered.length === 0 && (
              <div className="text-[11px] text-slate-500 px-1 py-2">No campaigns match.</div>
            )}
            {ordered.map(([tenant, list]) => (
              <div key={tenant} className="space-y-1.5">
                <div className="px-1 text-[10px] font-code uppercase tracking-wider text-slate-500">
                  Lab: {tenant} ({list.length})
                </div>
                {list.map((s) => {
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
            ))}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/[0.06] text-[11px]">
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
        );
      })() : null}

      {/* Center stage */}
      <div className="flex-1 flex flex-col min-w-0 relative grid-bg">
        {/* U1-a: backend liveness always visible — distinguishes "empty"
            from "backend down" on every surface below. */}
        <SessionExpiredBanner
          onOpenLabs={() => {
            setSurface('mesh');
            setMeshTab('labs');
          }}
        />
        <BackendStatusBar />
        <CheckInStrip loops={loops} />
        {surface === 'arena' && (
          <main className="relative flex-1 h-full overflow-hidden">
            <ArenaHud
              targetLabel={targetLabel}
              swarmDetail={runPhase === 'idle' ? 'idle · at rest' : `${loops.length} Loops Live`}
                view={arenaView}
                onViewChange={handleArenaViewChange}
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
            {/* hardware-qpu intentionally renders no second FleetRadar here:
                handleArenaViewChange jumps to the dock fleet surface instead
                (single mount, single poller — see FleetRadar double-mount). */}
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
              <div className="absolute bottom-6 left-6 z-20 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] p-3 rounded-2xl bg-[#0E1117]/90 border border-white/10 text-[11px] text-slate-200 shadow-2xl">
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
              {surface === 'recorder' && (
                <>
                  <FlightRecorder loops={loops} events={events} campaign={campaign} onLoopsChanged={refreshLoops} onSessionImported={handleSessionImported} />
                  {/* Auditor flight-recorder telemetry (60s gate) with measured
                      fetch latency — mounted, not dead code. */}
                  <div className="mt-4">
                    <FlightRecorderCard sessionId={currentSessionId || null} />
                  </div>
                </>
              )}
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
              {surface === 'proofs' && proofsTab === 'gallery' && <GenerativeUIStudio campaign={campaign} tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id} />}
              {surface === 'proofs' && proofsTab === 'circuits' && <CodesignCockpit />}
              {surface === 'mesh' && meshTab === 'mesh' && <CognitiveMesh campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'guardrails' && <AutonomyCockpit campaign={campaign} session={session} />}
              {surface === 'mesh' && meshTab === 'models' && <HuggingFaceModelHub />}
              {surface === 'mesh' && meshTab === 'labs' && <LabsView />}
              {surface === 'mesh' && meshTab === 'settings' && <LLMSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumDispatch sessionId={currentSessionId || null} tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id} />}
            </div>
          </main>
        )}
      </div>

      {/* Right: native co-pilot surface */}
      {/* Right: native co-pilot surface. Below lg this is an overlay drawer
          (closed by default at mount) over a dimmed backdrop; at lg+ it is
          the fixed side panel. The existing toggle opens/closes both. */}
      {showCopilot ? (
        <>
          <div
            aria-hidden
            onClick={() => setShowCopilot(false)}
            className="fixed inset-0 z-40 bg-black/60 lg:hidden"
          />
          <aside
            aria-label="Kubona co-pilot"
            className={`${
              isCopilotExpanded ? 'lg:w-[640px] xl:w-[700px]' : 'lg:w-96'
            } fixed inset-y-0 right-0 z-50 flex w-[min(24rem,calc(100vw-3rem))] flex-col shrink-0 border-l border-white/5 bg-[#0A0C10]/95 backdrop-blur-2xl transition-all duration-300 shadow-2xl overflow-hidden lg:static lg:z-auto`}
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
        </>
      ) : (
        <button
          onClick={() => setShowCopilot(true)}
          className="fixed bottom-6 right-6 z-40 px-4 py-2.5 rounded-full bg-[#0C101A] border border-cyan-500/40 hover:border-cyan-400 text-white shadow-2xl backdrop-blur-2xl flex items-center space-x-2.5 group transition-all duration-200 hover:scale-105 active:scale-95"
          title={`Open Kubona Copilot (${SHORTCUTS.copilot})`}
        >
          <div className="relative">
            <Sparkles className="w-4 h-4 text-cyan-400 animate-float-gentle" />
            <span className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
          </div>
          <span className="text-xs font-semibold text-slate-200 group-hover:text-white">
            Kubona Copilot
          </span>
          <kbd className="px-1.5 py-0.5 rounded bg-black/50 text-[10px] font-code text-cyan-300 border border-white/10">
            {SHORTCUTS.copilot}
          </kbd>
        </button>
      )}
      </AttentionProvider>
    </div>
  );
}
