'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import dynamic from 'next/dynamic';
import { SurfaceLoading } from '@/components/shell/SurfaceLoading';
import { Sparkles, Plus, Zap, Info, HelpCircle, Trash2, Clock, Box, X } from 'lucide-react';
import { Session, LedgerEvent, RunPhase, ProbeId } from '@/types';
import { fetchSessionsResult, fetchSession, createSession, deleteSession, createTask, updateTaskStatus, fetchLoopsResult, fetchEventsResult, fetchCampaignState, API_BASE, CONSOLE_WORKBENCH_ENABLED, authHeaders, configureApiBase, apiFetch, UNAUTH_EVENT, API_PROBLEM_EVENT, type ApiProblem } from '@/lib/api';
import { useJobsProjection } from '@/domain/useJobsProjection';
import { useObligationsProjection } from '@/domain/useObligationsProjection';
import { useTasksProjection } from '@/domain/useTasksProjection';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { useAuthPhase } from '@/domain/auth-state';
import { settleWrite, writeSettlementLabel } from '@/domain/write-settlement';
import { Inspector, type InspectorTab } from '@/components/shell/Inspector';
import type { WorkbenchObject, WorkbenchObjectKind } from '@/components/workbench/workbench-model';
import type { WorkbenchHandle } from '@/components/workbench/Workbench';
import { mergeSessionEvents, useSessionStream } from '@/lib/useSessionStream';
import { useCampaignProjection } from '@/lib/useCampaignProjection';
import { useLoopsProjection } from '@/lib/useLoopsProjection';
import { useSessionsProjection } from '@/lib/useSessionsProjection';
import { SHORTCUTS, SHORTCUT_TARGETS, KEY_TO_SHORTCUT, type ShortcutKey } from '@/lib/shortcuts';
import { CopilotDrawer, type CopilotHandle } from '@/components/CopilotDrawer';
import { Composer } from '@/components/shell/Composer';
import { AboutLabModal } from '@/components/AboutLabModal';
import { CommandPalette } from '@/components/CommandPalette';
import { KeyboardShortcutsModal } from '@/components/KeyboardShortcutsModal';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { NewCampaignModal } from '@/components/modals/NewCampaignModal';
import { ModeRail, type ShellMode } from '@/components/shell/ModeRail';
import { ArenaHud, ArenaView } from '@/components/sovereign/ArenaHud';
import { SwarmUniverse } from '@/components/sovereign/SwarmUniverse';
import { ProofDagView } from '@/components/sovereign/ProofDagView';
import { ActionGate } from '@/components/sovereign/ActionGate';
import { AttentionProvider } from '@/lib/attention-context';
import { isMeteredBackend } from '@/lib/approvalPacket';
import type { CanvasWidgetSpec, CanvasWidgetKind } from '@/components/sovereign/WidgetStage';
import { FlightRecorderCard } from '@/components/FlightRecorderCard';
import { Workbench } from '@/components/workbench/Workbench';
import { StatusStrip } from '@/components/shell/StatusStrip';

const FlightRecorder = dynamic(() => import('@/components/surfaces/FlightRecorder').then((mod) => ({ default: mod.FlightRecorder })), { ssr: false, loading: () => <SurfaceLoading label="flight recorder" /> });
const EpistemicManifold = dynamic(() => import('@/components/surfaces/EpistemicManifold').then((mod) => ({ default: mod.EpistemicManifold })), { ssr: false, loading: () => <SurfaceLoading label="evidence" /> });
const FleetRadar = dynamic(() => import('@/components/surfaces/FleetRadar').then((mod) => ({ default: mod.FleetRadar })), { ssr: false, loading: () => <SurfaceLoading label="fleet" /> });
const ResultsStudio = dynamic(() => import('@/components/surfaces/ResultsStudio').then((mod) => ({ default: mod.ResultsStudio })), { ssr: false, loading: () => <SurfaceLoading label="results" /> });
const EpistemicTribunalRoom = dynamic(() => import('@/components/surfaces/EpistemicTribunalRoom').then((mod) => ({ default: mod.EpistemicTribunalRoom })), { ssr: false, loading: () => <SurfaceLoading label="tribunal" /> });
const CognitiveMesh = dynamic(() => import('@/components/surfaces/CognitiveMesh').then((mod) => ({ default: mod.CognitiveMesh })), { ssr: false, loading: () => <SurfaceLoading label="mesh" /> });
const HuggingFaceModelHub = dynamic(() => import('@/components/surfaces/HuggingFaceModelHub').then((mod) => ({ default: mod.HuggingFaceModelHub })), { ssr: false, loading: () => <SurfaceLoading label="model hub" /> });
const LabsView = dynamic(() => import('@/components/surfaces/LabsView').then((mod) => ({ default: mod.LabsView })), { ssr: false, loading: () => <SurfaceLoading label="laboratories" /> });
const LLMSettings = dynamic(() => import('@/components/surfaces/LLMSettings').then((mod) => ({ default: mod.LLMSettings })), { ssr: false, loading: () => <SurfaceLoading label="LLM settings" /> });
const QuantumSettings = dynamic(() => import('@/components/surfaces/QuantumSettings').then((mod) => ({ default: mod.QuantumSettings })), { ssr: false, loading: () => <SurfaceLoading label="quantum settings" /> });
const QuantumDispatch = dynamic(() => import('@/components/surfaces/QuantumDispatch').then((mod) => ({ default: mod.QuantumDispatch })), { ssr: false, loading: () => <SurfaceLoading label="quantum dispatch" /> });
const AutonomyCockpit = dynamic(() => import('@/components/surfaces/AutonomyCockpit').then((mod) => ({ default: mod.AutonomyCockpit })), { ssr: false, loading: () => <SurfaceLoading label="guardrails" /> });
const SpatialCanvas = dynamic(() => import('@/components/surfaces/SpatialCanvas').then((mod) => ({ default: mod.SpatialCanvas })), { ssr: false, loading: () => <SurfaceLoading label="molecule workbench" /> });
const GenerativeUIStudio = dynamic(() => import('@/components/surfaces/GenerativeUIStudio').then((mod) => ({ default: mod.GenerativeUIStudio })), { ssr: false, loading: () => <SurfaceLoading label="gallery" /> });
const CodesignCockpit = dynamic(() => import('@/components/surfaces/CodesignCockpit').then((mod) => ({ default: mod.CodesignCockpit })), { ssr: false, loading: () => <SurfaceLoading label="codesign" /> });
const VaultLeasePanel = dynamic(() => import('@/components/common/VaultLeasePanel').then((mod) => ({ default: mod.VaultLeasePanel })), { ssr: false, loading: () => <SurfaceLoading label="vault leases" /> });
const SystemReadinessPanel = dynamic(() => import('@/components/common/SystemReadinessPanel').then((mod) => ({ default: mod.SystemReadinessPanel })), { ssr: false, loading: () => <SurfaceLoading label="readiness" /> });
const WidgetStage = dynamic(() => import('@/components/sovereign/WidgetStage').then((mod) => ({ default: mod.WidgetStage })), { ssr: false, loading: () => <SurfaceLoading label="widget" /> });

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
  const [eventVisible, setEventVisible] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const authPhase = useAuthPhase();
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const show = () => setEventVisible(true);
    window.addEventListener(UNAUTH_EVENT, show);
    return () => window.removeEventListener(UNAUTH_EVENT, show);
  }, []);
  const visible = !dismissed && (authPhase === 'expired' || eventVisible);
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
          onClick={() => { setDismissed(true); setEventVisible(false); }}
          aria-label="Dismiss session expired banner"
          className="p-1 rounded-lg text-amber-200/70 hover:text-amber-100 hover:bg-white/10 transition-all"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </span>
    </div>
  );
}

// A single visible refusal for any API error, including wrappers that
// historically returned []/null. Empty data must never be the only signal.
export function ApiProblemBanner() {
  const [problem, setProblem] = useState<ApiProblem | null>(null);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onProblem = (event: Event) => {
      const detail = (event as CustomEvent<ApiProblem>).detail;
      if (detail) setProblem(detail);
    };
    window.addEventListener(API_PROBLEM_EVENT, onProblem);
    return () => window.removeEventListener(API_PROBLEM_EVENT, onProblem);
  }, []);
  if (!problem) return null;
  return (
    <div
      data-testid="api-problem-banner"
      role="alert"
      className="flex items-center justify-between gap-3 px-4 py-1.5 bg-rose-950/90 border-b border-rose-500/40 text-rose-100 text-[12px] font-code"
    >
      <span className="truncate">
        API refusal — {problem.path} · {problem.status ?? 'transport'} · {problem.message}. Some surfaces may show empty data until this is fixed.
      </span>
      <button
        type="button"
        onClick={() => setProblem(null)}
        aria-label="Dismiss API refusal"
        className="shrink-0 px-2 py-0.5 rounded border border-rose-400/30 text-rose-200 hover:bg-rose-500/20"
      >
        Dismiss
      </button>
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
export type DockSurface = 'arena' | 'recorder' | 'manifold' | 'fleet' | 'proofs' | 'mesh';
export type ProofsTab = 'manuscripts' | 'tribunal' | 'gallery' | 'circuits';
export type MeshTab = 'fleet' | 'mesh' | 'guardrails' | 'models' | 'labs' | 'settings';

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
  arenaView: 'workbench',
};

const VALID_SURFACES: DockSurface[] = ['arena', 'recorder', 'manifold', 'fleet', 'proofs', 'mesh'];
const VALID_PROOFS_TABS: ProofsTab[] = ['manuscripts', 'tribunal', 'gallery', 'circuits'];
const VALID_MESH_TABS: MeshTab[] = ['fleet', 'mesh', 'guardrails', 'models', 'labs', 'settings'];
const VALID_ARENA_VIEWS: ArenaView[] = ['workbench', 'orbital', 'proof-dag', 'hardware-qpu'];

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
  const { sessions, setSessions, upsert: upsertSession } = useSessionsProjection();
  const [currentSessionId, setCurrentSessionId] = useState<string>('');
  const bootstrappedSession = useRef(false);
  const prevPhase = useRef<RunPhase>('idle');
  const { loops, setLoops, refresh: refreshLoops } = useLoopsProjection(currentSessionId || null);
  const { jobs, status: jobsStatus, error: jobsError, refresh: refreshJobs } = useJobsProjection(currentSessionId || null);
  const { obligations } = useObligationsProjection();
  const { tasks, refresh: refreshTasks } = useTasksProjection(currentSessionId || null);
  const { campaign, setCampaign, reload: reloadCampaign } = useCampaignProjection(currentSessionId || null);
  const [events, setEvents] = useState<LedgerEvent[]>([]);
  const [nowTick, setNowTick] = useState(0);

  // Sovereign shell state
  const [surface, setSurface] = useState<DockSurface>('arena');
  const [arenaView, setArenaView] = useState<ArenaView>('workbench');
  const [workbenchEnabled, setWorkbenchEnabled] = useState(true);
  const [recenterSignal, setRecenterSignal] = useState(0);
  const [proofsTab, setProofsTab] = useState<'manuscripts' | 'tribunal' | 'gallery' | 'circuits'>('manuscripts');
  const [meshTab, setMeshTab] = useState<MeshTab>('mesh');
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
  const createWrite = useWriteSettlement();
  const taskWrite = useWriteSettlement();
  const createFailedRef = useRef(false);
  // Responsive shell (Wave-2): below lg the copilot is a closed-by-default
  // overlay drawer, not a fixed side panel. Default closed on narrow screens
  // via matchMedia at mount; the existing toggle reopens it as an overlay.
  // The workbench gets the room first. Copilot opens on demand (the button
  // below, the palette, or a run's question), never as a permanent column.
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>('details');
  const [isCopilotExpanded, setIsCopilotExpanded] = useState(false);
  const [selectedWorkbenchObject, setSelectedWorkbenchObject] = useState<WorkbenchObject | null>(null);
  const copilotRef = useRef<CopilotHandle>(null);
  const workbenchRef = useRef<WorkbenchHandle>(null);
  const pendingAsk = useRef<string | null>(null);
  const pendingAttach = useRef<Array<{ name: string; text: string }>>([]);
  const [showCampaignMenu, setShowCampaignMenu] = useState(false);
  const [campaignFilter, setCampaignFilter] = useState('');
  const [sessionListError, setSessionListError] = useState<string | null>(null);
  const [dataError, setDataError] = useState<string | null>(null);

  const refreshData = useCallback(async () => {
    const sid = currentSessionId || undefined;
    const [sessResult, loopResult, evtResult] = await Promise.all([
      fetchSessionsResult(),
      fetchLoopsResult(sid),
      fetchEventsResult(sid),
    ]);
    if (sessResult.ok) {
      setSessionListError(null);
      setSessions(sessResult.sessions);
    } else {
      setSessionListError(sessResult.error);
    }
    if (loopResult.ok) setLoops(loopResult.data);
    if (evtResult.ok) setEvents(evtResult.data);
    const readError = !loopResult.ok ? loopResult.error : !evtResult.ok ? evtResult.error : null;
    setDataError(readError);
  }, [currentSessionId, setLoops, setSessions]);

  const onTelemetry = useCallback((incoming: LedgerEvent[]) => {
    setEvents((previous) => mergeSessionEvents(previous, incoming));
  }, []);
  const onStreamGap = useCallback(() => {
    if (!currentSessionId) return;
    void fetchEventsResult(currentSessionId).then((result) => {
      if (result.ok) setEvents(result.data);
      else setDataError(result.error);
    });
  }, [currentSessionId]);
  const streamLive = useSessionStream(currentSessionId || null, onTelemetry, onStreamGap);
  const eventsRef = useRef<LedgerEvent[]>([]);
  useEffect(() => {
    eventsRef.current = events;
  }, [events]);

  const switchSession = useCallback((id: string) => {
    setCanvasWidget(null);
    setCurrentSessionId(id);
  }, []);

  // U2-a: an imported (or replayed) session becomes current — switch to
  // it and load its campaign, same as a fresh creation.
  const handleSessionImported = useCallback(async (sessionId: string) => {
    switchSession(sessionId);
    const camp = await fetchCampaignState(sessionId);
    if (camp) setCampaign(camp);
    refreshData();
  }, [switchSession, refreshData, setCampaign]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      // Runtime backend URL first (M5): same image, any backend.
      await configureApiBase();
      const workbenchOn = CONSOLE_WORKBENCH_ENABLED;
      setWorkbenchEnabled(workbenchOn);
      if (!workbenchOn) setArenaView((current) => (current === 'workbench' ? 'orbital' : current));
      const [sessResult, loopResult] = await Promise.all([
        fetchSessionsResult(),
        fetchLoopsResult(),
      ]);
      if (!mounted) return;
      if (!sessResult.ok) {
        setSessionListError(sessResult.error);
        if (loopResult.ok) setLoops(loopResult.data);
        setDataError(loopResult.ok ? sessResult.error : loopResult.error);
        return;
      }
      setSessionListError(null);
      if (!loopResult.ok) setDataError(loopResult.error);
      else setDataError(null);
      const sessList = sessResult.sessions;
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
      if (loopResult.ok) setLoops(loopResult.data);
      if (targetSessionId) {
        void fetchEventsResult(targetSessionId).then((result) => {
          if (!mounted) return;
          if (result.ok) setEvents(result.data);
          else setDataError(result.error);
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
          const loopResult = await fetchLoopsResult(currentSessionId || undefined);
          if (!mounted) return;
          if (!loopResult.ok) throw new Error(loopResult.error);
          setLoops(loopResult.data);
          setDataError(null);
          if (currentSessionId) {
            const freshSession = await fetchSession(currentSessionId);
            if (!mounted) return;
            if (freshSession) upsertSession(freshSession, true);
            const freshCampaign = await fetchCampaignState(currentSessionId);
            if (!mounted) return;
            if (freshCampaign) setCampaign(freshCampaign);
            let e = eventsRef.current;
            if (!streamLive) {
              const eventResult = await fetchEventsResult(currentSessionId);
              if (!mounted) return;
              if (!eventResult.ok) throw new Error(eventResult.error);
              e = eventResult.data;
              setEvents(e);
            }
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
  }, [currentSessionId, refreshData, switchSession, runPhase, streamLive, setCampaign, setLoops, setSessions, upsertSession]);
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
      let createdId: string | null = null;
      const requestId = createWrite.begin('creating campaign');
      try {
        const newSess = await createSession(
          params.title,
          params.spendLimitUSD,
          params.targetMolecule,
          params.fresh
        );
        failure = describeCreateFailure(newSess, null);
        if (!failure && newSess) {
          createdId = newSess.id;
          upsertSession(newSess, true);
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
      if (failure) {
        createWrite.refused(requestId, failure);
      } else {
        createWrite.applied(requestId, `campaign ${createdId?.slice(0, 8) ?? ''} created`);
        setToast({ key: Date.now(), text: writeSettlementLabel(settleWrite(requestId, 'applied', `campaign ${createdId?.slice(0, 8) ?? ''} created`)) });
        if (toastTimer.current) clearTimeout(toastTimer.current);
        toastTimer.current = setTimeout(() => setToast(null), 4000);
      }
    },
    [switchSession, setCampaign, upsertSession, createWrite]
  );

  // Wave-3: delete-campaign stages a typed ConfirmDialog (session short-id)
  // instead of a native confirm(). The deleteSession + session-list update
  // is unchanged; refusal stays open with an inline error.
  const [deleteDraft, setDeleteDraft] = useState<{ id: string; name: string } | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const deleteWrite = useWriteSettlement();

  const handleDeleteSession = useCallback(
    (e: React.MouseEvent, sessionId: string) => {
      e.stopPropagation();
      const sessionToDelete = sessions.find((s) => s.id === sessionId);
      const name = sessionToDelete?.title || 'this campaign';
      setDeleteError(null);
      setDeleteDraft({ id: sessionId, name });
    },
    [sessions]
  );

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteDraft || deleteBusy) return;
    const sessionId = deleteDraft.id;
    setDeleteBusy(true);
    setDeleteError(null);
    const requestId = deleteWrite.begin(`deleting campaign ${sessionId.slice(0, 8)}`);
    try {
      const success = await deleteSession(sessionId);
      if (!success) {
        // Fail-closed: stay open with an inline error, nothing removed.
        deleteWrite.refused(requestId, 'backend refused; nothing was deleted');
        setDeleteError('Delete refused by backend — nothing was deleted.');
        return;
      }
      deleteWrite.applied(requestId, `campaign ${sessionId.slice(0, 8)} deleted`);
      setToast({ key: Date.now(), text: writeSettlementLabel(settleWrite(requestId, 'applied', `campaign ${sessionId.slice(0, 8)} deleted`)) });
      if (toastTimer.current) clearTimeout(toastTimer.current);
      toastTimer.current = setTimeout(() => setToast(null), 4000);
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
      setDeleteDraft(null);
      setDeleteError(null);
    } finally {
      setDeleteBusy(false);
    }
  }, [deleteDraft, deleteBusy, currentSessionId, switchSession, setCampaign, setSessions, deleteWrite]);

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
      // Wave-3: inline toast instead of a blocking alert(). No session, no
      // dialog — nothing was dispatched.
      setToast({ key: Date.now(), text: 'No active session: open or create a session before triggering an abort.' });
      if (toastTimer.current) clearTimeout(toastTimer.current);
      toastTimer.current = setTimeout(() => setToast(null), 4000);
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
      // Wave-3: inline toast instead of a blocking alert(). The abort
      // receipt renders in the existing toast slot, auto-dismissed.
      setToast({ key: Date.now(), text: `Ring 0 Reflex Intercept complete: ${data.aborted_count} job(s) settled failed. Abort receipt: ${data.abort_receipt}` });
      if (toastTimer.current) clearTimeout(toastTimer.current);
      toastTimer.current = setTimeout(() => setToast(null), 6000);
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

  const activeMode: ShellMode = surface === 'proofs'
    ? 'evidence'
    : surface === 'mesh'
      ? (meshTab === 'settings' ? 'settings' : 'operations')
      : surface === 'fleet'
        ? 'operations'
        : 'workbench';

  const handleSelectMode = useCallback((mode: ShellMode) => {
    if (mode === 'workbench') {
      setSurface('arena');
      setArenaView('workbench');
      setShowLegacyCanvas(false);
      return;
    }
    if (mode === 'evidence') {
      setSurface('proofs');
      setProofsTab('manuscripts');
      return;
    }
    if (mode === 'operations') {
      setSurface('mesh');
      setMeshTab((current) => (surface === 'mesh' && current !== 'settings' ? current : 'fleet'));
      return;
    }
    setSurface('mesh');
    setMeshTab('settings');
  }, [surface]);

  useEffect(() => {
    if (!inspectorOpen || inspectorTab !== 'copilot' || !copilotRef.current) return;
    for (const item of pendingAttach.current) copilotRef.current.attach(item.name, item.text);
    pendingAttach.current = [];
    if (!pendingAsk.current) return;
    const text = pendingAsk.current;
    pendingAsk.current = null;
    // Attachments are React state; let the attach commits land before send
    // reads the transcript's attachment list.
    setTimeout(() => { void copilotRef.current?.submit(text); }, 0);
  }, [inspectorOpen, inspectorTab]);

  const handleAttach = useCallback((name: string, text: string) => {
    if (inspectorOpen && inspectorTab === 'copilot' && copilotRef.current) {
      copilotRef.current.attach(name, text);
      return;
    }
    pendingAttach.current = [...pendingAttach.current.filter((item) => item.name !== name), { name, text }];
    setInspectorOpen(true);
    setInspectorTab('copilot');
  }, [inspectorOpen, inspectorTab]);

  const handleDetach = useCallback((name: string) => {
    pendingAttach.current = pendingAttach.current.filter((item) => item.name !== name);
    copilotRef.current?.detach(name);
  }, []);

  const handleAsk = useCallback((text: string) => {
    if (inspectorOpen && inspectorTab === 'copilot' && copilotRef.current) {
      return copilotRef.current.submit(text);
    }
    pendingAsk.current = text;
    setInspectorOpen(true);
    setInspectorTab('copilot');
  }, [inspectorOpen, inspectorTab]);

  const handleOpenWorkbenchObject = useCallback((kind: WorkbenchObjectKind) => {
    if (kind === 'chart' || kind === 'agent_run' || kind === 'job') setSurface('recorder');
    else if (kind === 'approval') { setSurface('mesh'); setMeshTab('guardrails'); }
    else if (kind === 'evidence') setSurface('manifold');
    else if (kind === 'molecule') { setSurface('arena'); setShowLegacyCanvas(true); }
    else if (kind === 'proof' || kind === 'manuscript') {
      setSurface('proofs');
      setProofsTab('manuscripts');
    } else if (kind === 'circuit') {
      setSurface('arena');
      setArenaView('proof-dag');
    } else {
      setSurface('arena');
      setArenaView('workbench');
    }
  }, []);

  const handleCreateTask = useCallback(async (object: WorkbenchObject) => {
    if (!currentSessionId) return;
    const ref = object.ref ?? { domain: 'campaign', id: currentSessionId };
    const requestId = taskWrite.begin('create task');
    const out = await createTask(currentSessionId, `Review ${object.title}`, ref.domain, ref.id);
    if (out.ok) {
      taskWrite.applied(requestId, `task ${out.task.id.slice(0, 8)} created`);
      refreshTasks();
      setToast({ key: Date.now(), text: `Task created for ${object.title}.` });
    } else {
      taskWrite[out.unknown ? 'unknown' : 'refused'](requestId, out.error);
      setToast({ key: Date.now(), text: `Task ${out.unknown ? 'unconfirmed' : 'refused'}: ${out.error}` });
    }
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 4000);
  }, [currentSessionId, refreshTasks, taskWrite]);

  const handleTaskStatus = useCallback(async (taskId: string, status: 'open' | 'done') => {
    if (!currentSessionId) return;
    const requestId = taskWrite.begin(`task ${status}`);
    const out = await updateTaskStatus(taskId, currentSessionId, status);
    if (out.ok) {
      taskWrite.applied(requestId, `task ${status}`);
      refreshTasks();
    } else {
      taskWrite[out.unknown ? 'unknown' : 'refused'](requestId, out.error);
      setToast({ key: Date.now(), text: `Task update ${out.unknown ? 'unconfirmed' : 'refused'}: ${out.error}` });
      if (toastTimer.current) clearTimeout(toastTimer.current);
      toastTimer.current = setTimeout(() => setToast(null), 4000);
    }
  }, [currentSessionId, refreshTasks, taskWrite]);

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
      case 'arena-workbench':
        setSurface('arena'); setShowLegacyCanvas(false); setArenaView('workbench'); break;
      case 'arena-orbital':
        setSurface('arena'); setShowLegacyCanvas(false); setArenaView('orbital'); break;
      case 'arena-proof-dag':
        setSurface('arena'); setShowLegacyCanvas(false); setArenaView('proof-dag'); break;
      case 'palette':
        setIsCommandPaletteOpen((v) => !v); break;
      case 'copilot':
        setInspectorOpen((v) => !v);
        setInspectorTab('copilot');
        break;
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
    setArenaView(parsed.arenaView === 'hardware-qpu' ? 'workbench' : parsed.arenaView);
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
      setArenaView(parsed.arenaView === 'hardware-qpu' ? 'workbench' : parsed.arenaView);
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
      <div className="bg-[#0E1117] border border-white/10 rounded-2xl sm:rounded-full p-1 flex flex-wrap items-center justify-center max-w-full shadow-2xl">
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
    <div className="flex h-screen w-screen bg-[#07080B] text-slate-100 overflow-hidden font-sovereign">
      <a href="#kubona-main" className="k-skip-link">Skip to main content</a>
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
        onToggleCopilot={() => { setInspectorOpen(true); setInspectorTab('copilot'); }}
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
      {deleteDraft != null && (
        <ConfirmDialog
          open
          title={`Delete "${deleteDraft.name}"?`}
          body={
            <span>
              This will permanently remove its state and transcripts. Type{' '}
              <code className="rounded bg-white/10 px-1 font-mono">{deleteDraft.id.slice(0, 8)}</code> to confirm.
            </span>
          }
          requireText={deleteDraft.id.slice(0, 8)}
          confirmLabel="Delete campaign"
          danger
          busy={deleteBusy}
          error={deleteError}
          onConfirm={() => void handleConfirmDelete()}
          onCancel={() => {
            if (deleteBusy) return;
            setDeleteDraft(null);
            setDeleteError(null);
          }}
        />
      )}
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
      <ModeRail
        active={activeMode}
        onSelect={handleSelectMode}
        airGapSovereign
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
        <div className="absolute left-14 lg:left-20 bottom-6 z-50 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] rounded-2xl bg-[#0E1117] border border-white/10 shadow-2xl p-3 space-y-2">
          <div className="flex items-center justify-between px-1">
            <span className="text-[12px] font-code uppercase tracking-wider text-slate-400">
              Campaigns ({sessions.length})
            </span>
            <button
              onClick={() => { setShowCampaignMenu(false); handleCreateSession(); }}
              className="flex items-center gap-1 px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[12px] hover:bg-cyan-500/25 transition-all"
            >
              <Plus className="w-3 h-3" /> New <kbd className="font-code text-[12px]">{SHORTCUTS.newCampaign}</kbd>
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
              sessionListError ? (
                <div role="alert" className="text-[12px] text-amber-300/90 px-1 py-2 space-y-1">
                  <p>Campaigns unavailable: {sessionListError}</p>
                  <button type="button" onClick={() => void refreshData()} className="text-cyan-200 underline underline-offset-2">
                    Retry
                  </button>
                </div>
              ) : (
                <div className="text-[12px] text-slate-500 px-1 py-2">No campaigns match.</div>
              )
            )}
            {ordered.map(([tenant, list]) => (
              <div key={tenant} className="space-y-1.5">
                <div className="px-1 text-[12px] font-code uppercase tracking-wider text-slate-500">
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
                    <div className="text-[12px] font-code text-slate-400 flex justify-between mt-0.5">
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
          <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/[0.06] text-[12px]">
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
        <ApiProblemBanner />
        <StatusStrip
          loops={loops}
          error={dataError}
          streamLive={streamLive}
          lastEventAt={events.length > 0 ? events[events.length - 1]?.timestamp : null}
        />
        {surface === 'arena' && (
          <main id="kubona-main" tabIndex={-1} className="relative min-h-0 flex-1 overflow-hidden">
            <ArenaHud
              targetLabel={targetLabel}
              swarmDetail={runPhase === 'idle' ? 'idle · at rest' : `${loops.length} Loops Live`}
                view={arenaView}
                onViewChange={handleArenaViewChange}
                showWorkbench={workbenchEnabled}
              spendUsd={session?.current_spend_usd ?? 0}
              spendCapUsd={session?.spend_limit_usd ?? 150}
              sessionElapsed={session ? formatElapsed(session.created_at) : '00:00:00'}
              statusText={runStatusText}
              onRecenter={() => setRecenterSignal((n) => n + 1)}
            />

            {arenaView === 'workbench' && !showLegacyCanvas && (
              <Workbench
                key={currentSessionId}
                ref={workbenchRef}
                campaign={campaign}
                loops={loops}
                jobs={jobs}
                obligations={obligations}
                tasks={tasks}
                sessionId={currentSessionId || undefined}
                onSelectObject={(object) => {
                  setSelectedWorkbenchObject(object);
                  if (object) {
                    setInspectorTab('details');
                    setInspectorOpen(true);
                  }
                }}
                onOpenObject={handleOpenWorkbenchObject}
              />
            )}
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
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-white/5 border border-white/10 text-[12px] text-slate-300 hover:text-white transition-all"
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
                className="absolute bottom-6 right-6 z-20 w-9 h-9 rounded-full bg-[#0E1117] border border-white/10 hover:border-cyan-400/40 transition-all shadow-2xl flex items-center justify-center"
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
              <div className="absolute bottom-6 left-6 z-20 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] p-3 rounded-2xl bg-[#0E1117]/90 border border-white/10 text-[12px] text-slate-200 shadow-2xl">
                {gateNotice}
              </div>
            )}
            {toast && (
              <div
                key={toast.key}
                role="status"
                aria-live="polite"
                className="absolute bottom-6 left-1/2 -translate-x-1/2 z-20 px-4 py-2 rounded-full bg-[#0E1117] border border-cyan-400/25 text-[12px] font-code text-cyan-200 shadow-2xl animate-pulse"
              >
                {toast.text}
              </div>
            )}
          </main>
        )}

        {surface !== 'arena' && (
          <main id="kubona-main" tabIndex={-1} className="flex-1 overflow-y-auto">
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
                { id: 'fleet', label: 'Fleet Radar' },
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
              {surface === 'mesh' && meshTab === 'fleet' && <FleetRadar campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'mesh' && <CognitiveMesh campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'guardrails' && <AutonomyCockpit campaign={campaign} session={session} />}
              {surface === 'mesh' && meshTab === 'models' && <HuggingFaceModelHub />}
              {surface === 'mesh' && meshTab === 'labs' && <LabsView />}
              {surface === 'mesh' && meshTab === 'settings' && <LLMSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumSettings />}
              {surface === 'mesh' && meshTab === 'settings' && <QuantumDispatch sessionId={currentSessionId || null} tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id} />}
              {surface === 'mesh' && meshTab === 'settings' && <VaultLeasePanel sessionId={currentSessionId || undefined} />}
              {surface === 'mesh' && meshTab === 'settings' && <SystemReadinessPanel />}
            </div>
          </main>
        )}
        {surface === 'arena' && (
          <Composer
            onSubmit={handleAsk}
            contextLabel={selectedWorkbenchObject ? `${selectedWorkbenchObject.label}: ${selectedWorkbenchObject.title}` : campaign?.target_system || null}
            onClearContext={() => setSelectedWorkbenchObject(null)}
            onAttach={handleAttach}
            onDetach={handleDetach}
            onOpenModel={() => { setSurface('mesh'); setMeshTab('models'); }}
            modelLabel="Model Hub"
          />
        )}
      </div>

      <Inspector
        open={inspectorOpen}
        tab={inspectorTab}
        onTab={setInspectorTab}
        onClose={() => setInspectorOpen(false)}
        campaign={campaign}
        selectedObject={selectedWorkbenchObject}
        jobs={jobs}
        tasks={tasks}
        onCreateTask={handleCreateTask}
        onTaskStatus={handleTaskStatus}
        jobsStatus={jobsStatus}
        jobsError={jobsError}
        onRefreshJobs={refreshJobs}
        expanded={isCopilotExpanded}
        onOpenObject={(object) => handleOpenWorkbenchObject(object.kind)}
        onFocusObject={(object) => workbenchRef.current?.focusObject(object.id)}
        onRemoveObject={(object) => workbenchRef.current?.removeObject(object.id)}
        copilot={
          <CopilotDrawer
            ref={copilotRef}
            sessionId={currentSessionId}
            campaign={campaign}
            isExpanded={isCopilotExpanded}
            onToggleExpand={() => setIsCopilotExpanded((prev) => !prev)}
            onExperimentLaunched={() => {
              refreshData();
              void reloadCampaign();
            }}
            onCampaignUpdated={(c) => setCampaign(c)}
            onSelectSurface={handleCopilotSurface}
            onMaterialize={handleMaterialize}
            onRunStateChange={handleRunStateChange}
            onSessionChange={(id) => {
              setCurrentSessionId(id);
              refreshData();
            }}
            onClose={() => setInspectorOpen(false)}
          />
        }
      />
      {!inspectorOpen && (
        <button
          onClick={() => { setInspectorOpen(true); setInspectorTab('details'); }}
          className="fixed bottom-6 right-6 z-40 px-4 py-2.5 rounded-full bg-[#0C101A] border border-cyan-500/40 hover:border-cyan-400 text-white shadow-2xl flex items-center space-x-2.5 group transition-all duration-200 hover:scale-105 active:scale-95"
          title={`Open Inspector (${SHORTCUTS.copilot})`}
        >
          <div className="relative">
            <Sparkles className="w-4 h-4 text-cyan-400 animate-float-gentle" />
            <span className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
          </div>
          <span className="text-xs font-semibold text-slate-200 group-hover:text-white">Inspector</span>
          <kbd className="px-1.5 py-0.5 rounded bg-black/50 text-[12px] font-code text-cyan-300 border border-white/10">{SHORTCUTS.copilot}</kbd>
        </button>
      )}
      </AttentionProvider>
    </div>
  );
}
