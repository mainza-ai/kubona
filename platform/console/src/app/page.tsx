'use client';

import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import dynamic from 'next/dynamic';
import { SurfaceLoading } from '@/components/shell/SurfaceLoading';
import { Sparkles, Plus, Zap, Info, HelpCircle, Trash2, Clock, X } from 'lucide-react';
import { Session, LedgerEvent, RunPhase, ProbeId } from '@/types';
import { fetchSessionsResult, fetchSession, createSession, deleteSession, createTask, updateTaskStatus, closeObligation, fetchLoopsResult, fetchEventsResult, fetchCampaignState, authWhoami, fetchAccess, wasRecentAuthFailure, hasOperatorBearer, type AccessState, ApiFailureError, API_BASE, CONSOLE_WORKBENCH_ENABLED, authHeaders, configureApiBase, apiFetch } from '@/lib/api';
import { useJobsProjection } from '@/domain/useJobsProjection';
import { useObligationsProjection } from '@/domain/useObligationsProjection';
import { useTasksProjection } from '@/domain/useTasksProjection';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { settleWrite, writeSettlementLabel } from '@/domain/write-settlement';
import { setAuthPhase, useAuthPhase } from '@/domain/auth-state';
import { subscribeInvalidation } from '@/domain/invalidation';
import { Inspector, type InspectorTab } from '@/components/shell/Inspector';
import { buildWorkbench, type WorkbenchObject, type WorkbenchObjectKind } from '@/components/workbench/workbench-model';
import type { WorkbenchHandle } from '@/components/workbench/Workbench';
import { mergeSessionEvents, useSessionStream } from '@/lib/useSessionStream';
import { useCampaignProjection } from '@/lib/useCampaignProjection';
import { useLoopsProjection } from '@/lib/useLoopsProjection';
import { useSessionsProjection } from '@/lib/useSessionsProjection';
import { SHORTCUTS, SHORTCUT_TARGETS, KEY_TO_SHORTCUT, type ShortcutKey } from '@/lib/shortcuts';
import { CopilotDrawer } from '@/components/CopilotDrawer';
import { CommandDock } from '@/components/shell/CommandDock';
import { useCopilotChat } from '@/domain/useCopilotChat';
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
import { Chart } from '@/components/data/LazyChart';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import { Button } from '@/components/ui/Button';
import { AttentionProvider } from '@/lib/attention-context';
import { isMeteredBackend } from '@/lib/approvalPacket';
import type { CanvasWidgetSpec, CanvasWidgetKind } from '@/components/sovereign/WidgetStage';
import { CodeBlock, CompareMatrix, Workbench } from '@/components/workbench/Workbench';
import { StatusStrip } from '@/components/shell/StatusStrip';
import { BannerStack, describeCreateFailure } from '@/components/shell/Banners';
import { KeyHint } from '@/components/ui/Controls';
import { Dialog } from '@/components/ui/Overlays';
import { CampaignHeader } from '@/components/shell/CampaignHeader';
import { runViewTransition } from '@/lib/appearance';
import { buildShellSearch, parseShellParams, tabForSurface, type DockSurface, type MeshTab } from '@/lib/shell-params';

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
const AppearanceSettings = dynamic(() => import('@/components/surfaces/AppearanceSettings').then((mod) => ({ default: mod.AppearanceSettings })), { ssr: false, loading: () => <SurfaceLoading label="appearance" /> });
const QuantumDispatch = dynamic(() => import('@/components/surfaces/QuantumDispatch').then((mod) => ({ default: mod.QuantumDispatch })), { ssr: false, loading: () => <SurfaceLoading label="quantum dispatch" /> });
const ExperimentPanel = dynamic(() => import('@/components/common/ExperimentPanel').then((mod) => ({ default: mod.ExperimentPanel })), { ssr: false, loading: () => <SurfaceLoading label="experiment launcher" /> });
const DiscoveryProtocolPanel = dynamic(() => import('@/components/common/DiscoveryProtocolPanel').then((mod) => ({ default: mod.DiscoveryProtocolPanel })), { ssr: false, loading: () => <SurfaceLoading label="discovery protocol" /> });
const AutonomyCockpit = dynamic(() => import('@/components/surfaces/AutonomyCockpit').then((mod) => ({ default: mod.AutonomyCockpit })), { ssr: false, loading: () => <SurfaceLoading label="guardrails" /> });
const SpatialCanvas = dynamic(() => import('@/components/surfaces/SpatialCanvas').then((mod) => ({ default: mod.SpatialCanvas })), { ssr: false, loading: () => <SurfaceLoading label="molecule workbench" /> });
const GenerativeUIStudio = dynamic(() => import('@/components/surfaces/GenerativeUIStudio').then((mod) => ({ default: mod.GenerativeUIStudio })), { ssr: false, loading: () => <SurfaceLoading label="gallery" /> });
const CodesignCockpit = dynamic(() => import('@/components/surfaces/CodesignCockpit').then((mod) => ({ default: mod.CodesignCockpit })), { ssr: false, loading: () => <SurfaceLoading label="codesign" /> });
const VaultLeasePanel = dynamic(() => import('@/components/common/VaultLeasePanel').then((mod) => ({ default: mod.VaultLeasePanel })), { ssr: false, loading: () => <SurfaceLoading label="vault leases" /> });
const SystemReadinessPanel = dynamic(() => import('@/components/common/SystemReadinessPanel').then((mod) => ({ default: mod.SystemReadinessPanel })), { ssr: false, loading: () => <SurfaceLoading label="readiness" /> });
const WidgetStage = dynamic(() => import('@/components/sovereign/WidgetStage').then((mod) => ({ default: mod.WidgetStage })), { ssr: false, loading: () => <SurfaceLoading label="widget" /> });

export default function Home() {
  const [access, setAccess] = useState<AccessState | null>(null);
  const authPhase = useAuthPhase();
  const authPhaseRef = useRef(authPhase);
  useEffect(() => {
    authPhaseRef.current = authPhase;
  }, [authPhase]);
  const bearerConfigured = typeof hasOperatorBearer === 'function' && hasOperatorBearer();
  const canReadObligations =
    access !== null &&
    (access.mode !== 'multi_lab' || authPhase === 'authenticated' || bearerConfigured);

  const { sessions, setSessions, upsert: upsertSession } = useSessionsProjection();
  const [currentSessionId, setCurrentSessionId] = useState<string>('');
  const prevPhase = useRef<RunPhase>('idle');
  const { loops, setLoops, refresh: refreshLoops } = useLoopsProjection(currentSessionId || null);
  const { jobs, status: jobsStatus, error: jobsError, refresh: refreshJobs } = useJobsProjection(currentSessionId || null);
  const { obligations, refresh: refreshObligations } = useObligationsProjection(canReadObligations);
  const { tasks, refresh: refreshTasks } = useTasksProjection(currentSessionId || null);
  const { campaign, setCampaign, reload: reloadCampaign, failure: campaignFailure } = useCampaignProjection(currentSessionId || null);
  const [events, setEvents] = useState<LedgerEvent[]>([]);

  // Sovereign shell state
  const [surface, setSurface] = useState<DockSurface>('arena');
  const [arenaView, setArenaView] = useState<ArenaView>('workbench');
  const [workbenchEnabled, setWorkbenchEnabled] = useState(true);
  const [recenterSignal, setRecenterSignal] = useState(0);
  const [proofsTab, setProofsTab] = useState<'manuscripts' | 'tribunal' | 'gallery' | 'circuits'>('manuscripts');
  const [meshTab, setMeshTab] = useState<MeshTab>('mesh');
  const [settingsSection, setSettingsSection] = useState<'models' | 'quantum' | 'dispatch' | 'vault' | 'readiness' | 'appearance'>('models');
  const [moleculeObjectViewOpen, setMoleculeObjectViewOpen] = useState(false);
  const [inlineGateOwned, setInlineGateOwned] = useState(false);
  const [liftedObject, setLiftedObject] = useState<WorkbenchObject | null>(null);
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
  const loopsAuthHaltedRef = useRef(false);

  const localFailure = useCallback((result: { ok: false; failure: { kind: string }; error: string }) => {
    if (result.failure.kind === 'auth') return null;
    return result.error;
  }, []);

  // Tick serialization (no overlapping polls) + outage-episode toast latch:
  // exactly one "backend unreachable" toast per outage, reset on success.
  const tickBusy = useRef(false);
  const outageNotified = useRef(false);

  // Modals & Panels Visibility
  const [isAboutOpen, setIsAboutOpen] = useState(false);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);
  const [isShortcutsOpen, setIsShortcutsOpen] = useState(false);
  const [isNewCampaignModalOpen, setIsNewCampaignModalOpen] = useState(false);
  const [isExperimentLabOpen, setIsExperimentLabOpen] = useState(false);
  // createSession null/refusal text, rendered verbatim while the modal stays
  // open. Mirrored in a ref because the modal calls onClose synchronously
  // after onCreate resolves — state alone would not have flushed yet.
  const [createError, setCreateError] = useState<string | null>(null);
  const createWrite = useWriteSettlement();
  const taskWrite = useWriteSettlement();
  const obligationWrite = useWriteSettlement();
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
  const workbenchRef = useRef<WorkbenchHandle>(null);
  const [showCampaignMenu, setShowCampaignMenu] = useState(false);
  const [campaignFilter, setCampaignFilter] = useState('');
  const [sessionListError, setSessionListError] = useState<string | null>(null);
  const [dataError, setDataError] = useState<string | null>(null);
  const campaignAuthError = campaignFailure?.status === 401 || campaignFailure?.status === 403;

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
    if (loopResult.ok) {
      loopsAuthHaltedRef.current = false;
      setLoops(loopResult.data);
    } else if (loopResult.failure.kind === 'auth' && !hasOperatorBearer() && authPhaseRef.current !== 'authenticated') {
      loopsAuthHaltedRef.current = true;
    }
    if (evtResult.ok) setEvents(evtResult.data);
    const readError = !loopResult.ok ? localFailure(loopResult) : !evtResult.ok ? localFailure(evtResult) : null;
    setDataError(readError);
  }, [currentSessionId, localFailure, setLoops, setSessions]);

  const prevAuthPhase = useRef(authPhase);
  useEffect(() => {
    const previous = prevAuthPhase.current;
    prevAuthPhase.current = authPhase;
    if (authPhase === 'authenticated' && previous !== 'authenticated') {
      loopsAuthHaltedRef.current = false;
      void refreshData();
    }
  }, [authPhase, refreshData]);

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
  useEffect(() => subscribeInvalidation(['loops'], () => { void refreshLoops(); }), [refreshLoops]);
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

  const currentSessionIdRef = useRef(currentSessionId);
  useEffect(() => {
    currentSessionIdRef.current = currentSessionId;
  }, [currentSessionId]);

  const prevSessionIdRef = useRef('');
  useEffect(() => {
    const previous = prevSessionIdRef.current;
    prevSessionIdRef.current = currentSessionId;
    if (previous && currentSessionId && previous !== currentSessionId) {
      void refreshData();
    }
  }, [currentSessionId, refreshData]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      // Runtime backend URL first (M5): same image, any backend.
      await configureApiBase();
      if (!mounted) return;
      setAuthPhase('checking');
      const [accessState, whoami] = await Promise.all([
        fetchAccess(),
        authWhoami(),
      ]);
      if (!mounted) return;
      if (!whoami) setAuthPhase('anonymous');
      setAccess(accessState);
      const workbenchOn = CONSOLE_WORKBENCH_ENABLED;
      setWorkbenchEnabled(workbenchOn);
      if (!workbenchOn) setArenaView((current) => (current === 'workbench' ? 'orbital' : current));
      const multiLabAnon =
        accessState?.mode === 'multi_lab' &&
        whoami?.authenticated !== true &&
        !hasOperatorBearer();
      if (multiLabAnon) loopsAuthHaltedRef.current = true;
      const [sessResult, loopResult] = await Promise.all([
        fetchSessionsResult(),
        multiLabAnon
          ? Promise.resolve({
              ok: false as const,
              error: 'Sign in required for multi-lab loops.',
              failure: {
                path: '/api/loops',
                status: 401,
                kind: 'auth' as const,
                message: 'Sign in required for multi-lab loops.',
                at: Date.now(),
              },
            })
          : fetchLoopsResult(),
      ]);
      if (!mounted) return;
      if (!sessResult.ok) {
        setSessionListError(sessResult.error);
        if (loopResult.ok) {
          loopsAuthHaltedRef.current = false;
          setLoops(loopResult.data);
        } else if (loopResult.failure.kind === 'auth' && !hasOperatorBearer() && authPhaseRef.current !== 'authenticated') {
          loopsAuthHaltedRef.current = true;
        }
        const authOnly = sessResult.failure.status === 401 || /multi-lab|unauthorized|log in/i.test(sessResult.error);
        setDataError(authOnly ? (loopResult.ok ? null : localFailure(loopResult)) : (loopResult.ok ? sessResult.error : localFailure(loopResult)));
        return;
      }
      setSessionListError(null);
      if (!loopResult.ok) {
        if (loopResult.failure.kind === 'auth' && !hasOperatorBearer() && authPhaseRef.current !== 'authenticated') {
          loopsAuthHaltedRef.current = true;
        }
        setDataError(localFailure(loopResult));
      } else {
        loopsAuthHaltedRef.current = false;
        setDataError(null);
      }
      const sessList = sessResult.sessions;
      let targetSessionId = currentSessionIdRef.current;
      setSessions(sessList);
      if (sessList.length > 0 && !targetSessionId) {
        targetSessionId = sessList[0].id;
        switchSession(sessList[0].id);
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
    return () => {
      mounted = false;
    };
  }, [localFailure, setLoops, setSessions, switchSession]);

  useEffect(() => {
    let mounted = true;
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
          if (!streamLive && !loopsAuthHaltedRef.current) {
            const loopResult = await fetchLoopsResult(currentSessionId || undefined);
            if (!mounted) return;
            if (!loopResult.ok) {
              if (loopResult.failure.kind === 'transport' || loopResult.failure.kind === 'server') {
                throw new ApiFailureError(loopResult.failure);
              }
              if (loopResult.failure.kind === 'auth' && !hasOperatorBearer() && authPhaseRef.current !== 'authenticated') {
                loopsAuthHaltedRef.current = true;
              }
              setDataError(localFailure(loopResult));
            } else {
              loopsAuthHaltedRef.current = false;
              setLoops(loopResult.data);
              setDataError(null);
            }
          }
          if (currentSessionId) {
            const freshSession = await fetchSession(currentSessionId);
            if (!mounted) return;
            if (freshSession) upsertSession(freshSession, true);
            if (!streamLive) {
              await reloadCampaign();
              if (!mounted) return;
            }
            let e = eventsRef.current;
            if (!streamLive) {
              const eventResult = await fetchEventsResult(currentSessionId);
              if (!mounted) return;
              if (!eventResult.ok) {
                if (eventResult.failure.kind === 'transport' || eventResult.failure.kind === 'server') {
                  throw new ApiFailureError(eventResult.failure);
                }
                setDataError(localFailure(eventResult));
              } else {
                e = eventResult.data;
                setEvents(e);
              }
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
        tickTimer = setTimeout(tick, document.hidden || atRest || streamLive ? 15000 : 3500);
      }
    };
    tickTimer = setTimeout(tick, 3500);
    return () => {
      mounted = false;
      clearTimeout(tickTimer);
    };
  }, [currentSessionId, runPhase, streamLive, reloadCampaign, localFailure, setLoops, upsertSession]);

  useEffect(() => {
    if (runPhase === 'idle') return;
    const clock = setInterval(() => {
      setRunElapsedSec((s) => s + 1);
    }, 1000);
    return () => clearInterval(clock);
  }, [runPhase]);
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
        if (!newSess && (sessionListError || wasRecentAuthFailure())) {
          failure = 'Sign in to create a campaign in this multi-lab workspace. Nothing was created.';
        } else {
          failure = describeCreateFailure(newSess, null);
        }
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
    [switchSession, setCampaign, upsertSession, createWrite, sessionListError]
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
    if (s === 'arena') setMoleculeObjectViewOpen(false);
    else setIsExperimentLabOpen(false);
  }, []);

  const openInspector = useCallback((tab?: InspectorTab) => {
    setIsExperimentLabOpen(false);
    if (tab) setInspectorTab(tab);
    setInspectorOpen(true);
  }, []);

  const toggleExperimentLab = useCallback((mode: 'open' | 'close' | 'toggle' = 'open') => {
    setInspectorOpen(false);
    setSurface('arena');
    setMoleculeObjectViewOpen(false);
    setIsExperimentLabOpen((prev) => (mode === 'toggle' ? !prev : mode === 'open'));
  }, []);

  const activeMode: ShellMode = surface === 'proofs'
    ? 'evidence'
    : surface === 'mesh'
      ? (meshTab === 'settings' ? 'settings' : 'operations')
      : surface === 'fleet'
        ? 'operations'
        : 'workbench';

  const handleSelectMode = useCallback((mode: ShellMode) => {
    const applyMode = () => {
      if (mode === 'workbench') {
        setSurface('arena');
        setArenaView('workbench');
        setMoleculeObjectViewOpen(false);
        return;
      }
      setIsExperimentLabOpen(false);
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
    };
    runViewTransition(applyMode, () => {
      if (typeof document !== 'undefined') {
        document.getElementById('kubona-main')?.focus();
      }
    });
  }, [surface]);

  const handleOpenWorkbenchObject = useCallback((kind: WorkbenchObjectKind) => {
    if (kind === 'chart' || kind === 'agent_run' || kind === 'job') setSurface('recorder');
    else if (kind === 'approval') { setSurface('mesh'); setMeshTab('guardrails'); }
    else if (kind === 'evidence') setSurface('manifold');
    else if (kind === 'molecule') { setSurface('arena'); setMoleculeObjectViewOpen(true); }
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
    const out = await createTask(currentSessionId, `Review ${object.title}`, ref.domain, ref.id, requestId);
    if (out.ok) {
      taskWrite.applied(requestId, `task ${out.task.id.slice(0, 8)} created`);
      refreshTasks();
      setToast({ key: Date.now(), text: `Task created for ${object.title}.` });
    } else {
      if (out.unknown) void taskWrite.reconcile(requestId, currentSessionId, 'task create not confirmed');
      else taskWrite.refused(requestId, out.error);
      setToast({ key: Date.now(), text: `Task ${out.unknown ? 'unconfirmed' : 'refused'}: ${out.error}` });
    }
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 4000);
  }, [currentSessionId, refreshTasks, taskWrite]);

  const handleCloseObligation = useCallback(async (id: string) => {
    const requestId = obligationWrite.begin('close obligation');
    const out = await closeObligation(id);
    if (out.ok && out.closed) {
      obligationWrite.applied(requestId, `obligation ${id.slice(0, 8)} closed`);
      refreshObligations();
      return { ok: true as const };
    }
    const reason = (out.reasons ?? []).join('; ') || out.error || 'not close-ready';
    obligationWrite.refused(requestId, reason);
    return { ok: false as const, error: reason };
  }, [obligationWrite, refreshObligations]);

  const handleTaskStatus = useCallback(async (taskId: string, status: 'open' | 'done') => {
    if (!currentSessionId) return;
    const requestId = taskWrite.begin(`task ${status}`);
    const out = await updateTaskStatus(taskId, currentSessionId, status, requestId);
    if (out.ok) {
      taskWrite.applied(requestId, `task ${status}`);
      refreshTasks();
    } else {
      if (out.unknown) void taskWrite.reconcile(requestId, currentSessionId, 'task update not confirmed');
      else taskWrite.refused(requestId, out.error);
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
        setSurface('arena'); setMoleculeObjectViewOpen(false); setArenaView('workbench'); break;
      case 'arena-orbital':
        setSurface('arena'); setMoleculeObjectViewOpen(false); setArenaView('orbital'); break;
      case 'arena-proof-dag':
        setSurface('arena'); setMoleculeObjectViewOpen(false); setArenaView('proof-dag'); break;
      case 'arena-hardware-qpu':
        setSurface('arena'); setMoleculeObjectViewOpen(false); setArenaView('hardware-qpu'); break;
      case 'experiment-lab':
        toggleExperimentLab(surface === 'arena' ? 'toggle' : 'open');
        break;
      case 'palette':
        setIsCommandPaletteOpen((v) => !v); break;
      case 'copilot':
        if (inspectorOpen) setInspectorOpen(false);
        else openInspector('copilot');
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
  }, [handleEmergencyAbort, handleSelectDockSurface, inspectorOpen, openInspector, surface, toggleExperimentLab]);

  // FleetRadar subscribes to a shared module resource store (useFleetProjection),
  // so rendering it inside Arena Hardware QPU shares the single poller cleanly
  // without redirecting away from Arena.
  const handleArenaViewChange = useCallback((v: ArenaView) => {
    setArenaView(v);
  }, []);

  const handleCopilotSurface = useCallback((surf: string) => {
    if (surf === 'surface1') setSurface('arena');
    else if (surf === 'surface7') { setSurface('proofs'); setProofsTab('manuscripts'); }
    else if (surf === 'labs') { setSurface('mesh'); setMeshTab('labs'); setInspectorOpen(false); }
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

  const copilotChat = useCopilotChat({
    sessionId: currentSessionId,
    campaign,
    requiresSignIn: Boolean(sessionListError && /multi-lab|unauthorized|log in/i.test(sessionListError)),
    onSessionChange: (id) => {
      setCurrentSessionId(id);
      refreshData();
    },
    onCampaignUpdated: (c) => setCampaign(c),
    onExperimentLaunched: () => {
      refreshData();
      void reloadCampaign();
    },
    onRunStateChange: handleRunStateChange,
    onMaterialize: handleMaterialize,
  });

  const copilotChatRef = useRef(copilotChat);
  useEffect(() => {
    copilotChatRef.current = copilotChat;
  }, [copilotChat]);

  const handleAsk = useCallback((text: string) => {
    openInspector('copilot');
    return copilotChatRef.current.send(text);
  }, [openInspector]);

  const handleTribunalAsk = useCallback((topic: string) => {
    openInspector('copilot');
    return copilotChatRef.current.conveneTribunal(topic);
  }, [openInspector]);

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

  const focusWorkbenchTarget = useCallback((id: string) => {
    setSurface('arena');
    setArenaView('workbench');
    setMoleculeObjectViewOpen(false);
    if (workbenchRef.current) {
      workbenchRef.current.focusObject(id);
    } else {
      setTimeout(() => workbenchRef.current?.focusObject(id), 0);
    }
  }, []);

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
      if (e.key === 'Escape' && isExperimentLabOpen && typeof document !== 'undefined' && !document.querySelector('dialog[open]')) {
        setIsExperimentLabOpen(false);
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
  }, [isExperimentLabOpen, navigateToTarget]);

  // Deep-link URL sync (Wave-2): read ?surface=&?tab= once on mount (plus
  // back/forward via popstate), and reflect every navigation with
  // replaceState (no reload). Palette/dock clicks all funnel through the
  // surface/tab setters above, so the write effect below covers them.
  const urlReady = useRef(false);
  useEffect(() => {
    if (typeof window === 'undefined' || urlReady.current) return;
    urlReady.current = true;
    const parsed = parseShellParams(window.location?.search ?? '');
    setSurface(parsed.surface);
    setProofsTab(parsed.proofsTab);
    setMeshTab(parsed.meshTab);
    setArenaView(parsed.arenaView);
  }, []);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onPopState = () => {
      const parsed = parseShellParams(window.location?.search ?? '');
      setSurface(parsed.surface);
      setProofsTab(parsed.proofsTab);
      setMeshTab(parsed.meshTab);
      setArenaView(parsed.arenaView);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);
  useEffect(() => {
    if (typeof window === 'undefined' || !urlReady.current) return;
    if (!window.location?.search && surface === 'arena' && arenaView === 'workbench') return;
    const tab = tabForSurface(surface, { proofsTab, meshTab, arenaView });
    const next = buildShellSearch(surface, tab);
    if (window.location?.search !== next && typeof window.history?.replaceState === 'function') {
      window.history.replaceState(null, '', `${window.location.pathname}${next}${window.location.hash}`);
    }
  }, [surface, proofsTab, meshTab, arenaView]);

  const session = sessions.find((s) => s.id === currentSessionId) || null;
  const confirmGate = useMemo(() => (
    gateOpen && campaign ? {
      campaign,
      session,
      onAuthorized: handleAuthorizePulse,
      onDenied: handleDenyPulse,
      onDowngrade: handleDowngradePulse,
    } : null
  ), [gateOpen, campaign, session, handleAuthorizePulse, handleDenyPulse, handleDowngradePulse]);
  const targetShort = campaign?.quantum_factors?.target_molecule || campaign?.target_system || '';
  const targetLabel = targetShort ? `${targetShort.slice(0, 28)} · Ground State Target` : 'No Target · Idle';

  const [liveWorkbenchSummary, setLiveWorkbenchSummary] = useState<{
    sessionId: string;
    items: Array<{ id: string; label: string; title: string; kind: WorkbenchObjectKind }>;
  } | null>(null);
  const handleWorkbenchObjectsChange = useCallback((nextObjects: WorkbenchObject[]) => {
    const items = nextObjects.map((object) => ({ id: object.id, label: object.label, title: object.title, kind: object.kind }));
    setLiveWorkbenchSummary((prev) => {
      if (
        prev &&
        prev.sessionId === currentSessionId &&
        prev.items.length === items.length &&
        prev.items.every((entry, idx) =>
          entry.id === items[idx]!.id &&
          entry.label === items[idx]!.label &&
          entry.title === items[idx]!.title &&
          entry.kind === items[idx]!.kind
        )
      ) {
        return prev;
      }
      return { sessionId: currentSessionId, items };
    });
  }, [currentSessionId]);

  const paletteObjects = useMemo(
    () => (
      liveWorkbenchSummary && liveWorkbenchSummary.sessionId === currentSessionId
        ? liveWorkbenchSummary.items
        : buildWorkbench(campaign, loops, jobs, obligations, tasks, session?.current_spend_usd)
          .map((object) => ({ id: object.id, label: object.label, title: object.title, kind: object.kind }))
    ),
    [liveWorkbenchSummary, currentSessionId, campaign, loops, jobs, obligations, tasks, session?.current_spend_usd],
  );

  const subToggle = (options: Array<{ id: string; label: string }>, value: string, onChange: (id: string) => void) => (
    <div className="flex items-center justify-center px-4 pt-4">
      <div className="bg-[var(--color-float)] border border-[var(--color-border-strong)] rounded-2xl sm:rounded-full p-1 flex flex-wrap items-center justify-center max-w-full shadow-[var(--shadow-float)]">
        {options.map((o) => (
          <button
            key={o.id}
            onClick={() => onChange(o.id)}
            className={`px-3.5 py-1.5 rounded-full text-xs font-medium transition-all ${
              value === o.id ? 'bg-[var(--color-fill-active)] text-[var(--color-text)]' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );

  return (
    <div className="flex h-dvh w-full bg-[var(--color-canvas)] text-[var(--color-text)] overflow-hidden font-sovereign">
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
        onToggleCopilot={() => {
          if (inspectorOpen) setInspectorOpen(false);
          else openInspector('copilot');
        }}
        onOpenShortcuts={() => setIsShortcutsOpen(true)}
        objects={paletteObjects}
        onOpenObject={(id) => {
          setSurface('arena');
          setArenaView('workbench');
          openInspector('details');
          setTimeout(() => workbenchRef.current?.focusObject(id), 0);
        }}
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
            <code className="rounded bg-[var(--color-fill)] text-[var(--color-text)] px-1 font-mono">{currentSessionId}</code> as
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
              <code className="rounded bg-[var(--color-fill)] text-[var(--color-text)] px-1 font-mono">{deleteDraft.id.slice(0, 8)}</code> to confirm.
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
          className="fixed top-6 left-1/2 -translate-x-1/2 z-[60] max-w-lg px-4 py-2.5 rounded-2xl bg-[var(--color-float)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs shadow-[var(--shadow-float)]"
        >
          {createError}
        </div>
      )}

      {/* Icon-only sovereign dock */}
      <ModeRail
        active={activeMode}
        onSelect={handleSelectMode}
        airGapSovereign={access ? access.llm_local !== false : true}
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
        <div className="absolute left-14 lg:left-20 bottom-6 z-50 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] rounded-2xl bg-[var(--color-surface-elev)] border border-[var(--color-border-strong)] shadow-[var(--shadow-float)] p-3 space-y-2">
          <div className="flex items-center justify-between px-1">
            <span className="text-[12px] font-code uppercase tracking-wider text-[var(--color-muted)]">
              Campaigns ({sessions.length})
            </span>
            <button
              onClick={() => { setShowCampaignMenu(false); handleCreateSession(); }}
              className="flex items-center gap-1 px-2 py-1 rounded-lg bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] hover:bg-[var(--color-fill-hover)] transition-all"
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
            className="w-full px-2.5 py-1.5 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-xs text-[var(--color-text)] placeholder-[var(--color-faint)] focus:outline-none focus:border-[var(--color-info)]"
          />
          <div className="max-h-64 overflow-y-auto space-y-3">
            {ordered.length === 0 && (
              sessionListError ? (
                /multi-lab|unauthorized|log in/i.test(sessionListError) ? (
                  <div role="status" className="text-[12px] text-[var(--color-warning-ink)] px-1 py-2 space-y-1">
                    <p>This is a multi-lab workspace. Sign in to see your lab’s campaigns.</p>
                    <button
                      type="button"
                      onClick={() => { setShowCampaignMenu(false); setSurface('mesh'); setMeshTab('labs'); setInspectorOpen(false); }}
                      className="text-[var(--color-info-ink)] underline underline-offset-2"
                    >
                      Sign in
                    </button>
                  </div>
                ) : (
                  <div role="alert" className="text-[12px] text-[var(--color-warning-ink)] px-1 py-2 space-y-1">
                    <p>Campaigns unavailable: {sessionListError}</p>
                    <button type="button" onClick={() => void refreshData()} className="text-[var(--color-info-ink)] underline underline-offset-2">
                      Retry
                    </button>
                  </div>
                )
              ) : (
                <div className="text-[12px] text-[var(--color-faint)] px-1 py-2">{sessions.length === 0 ? 'No campaigns yet.' : 'No campaigns match.'}</div>
              )
            )}
            {ordered.map(([tenant, list]) => (
              <div key={tenant} className="space-y-1.5">
                <div className="px-1 text-[12px] font-code uppercase tracking-wider text-[var(--color-faint)]">
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
                        ? 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] text-[var(--color-text)]'
                        : 'bg-[var(--color-surface)] border-[var(--color-border)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]'
                    }`}
                  >
                    <div className="font-semibold truncate">{s.title}</div>
                    <div className="text-[12px] font-code text-[var(--color-muted)] flex justify-between mt-0.5">
                      <span className="flex items-center gap-1">
                        <Clock className="w-3 h-3" />
                        {new Date(s.created_at).toLocaleDateString()}
                      </span>
                      <span className="text-[var(--color-warning-ink)]">${spend.toFixed(2)} / ${limit.toFixed(2)}</span>
                    </div>
                  </button>
                  <button
                    onClick={(e) => handleDeleteSession(e, s.id)}
                    aria-label={`Delete ${s.title}`}
                    className="absolute top-2 right-2 p-1.5 rounded-lg text-[var(--color-faint)] hover:text-[var(--color-danger-ink)] hover:bg-[var(--color-danger-wash)] opacity-0 group-hover:opacity-100 focus-visible:opacity-100 group-focus-within:opacity-100 transition-all"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              );
                })}
              </div>
            ))}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-[var(--color-border)] text-[12px]">
            <button onClick={() => { setShowCampaignMenu(false); handleEmergencyAbort(); }} className="flex items-center gap-1.5 text-[var(--color-danger-ink)] hover:opacity-80 transition-opacity">
              <Zap className="w-3.5 h-3.5" /> Ring 0 Intercept
            </button>
            <button onClick={() => { setShowCampaignMenu(false); setIsAboutOpen(true); }} className="flex items-center gap-1.5 text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-colors">
              <Info className="w-3.5 h-3.5" /> Lab Info
            </button>
            <button onClick={() => { setShowCampaignMenu(false); setIsShortcutsOpen(true); }} className="flex items-center gap-1 text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-colors">
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
        <BannerStack
          onOpenLabs={() => {
            setSurface('mesh');
            setMeshTab('labs');
          }}
        />
        {surface !== 'arena' && (
          <CampaignHeader
            campaign={campaign}
            session={session}
            statusText={runStatusText}
            mode={access?.mode}
            onOpenCampaigns={() => setShowCampaignMenu((value) => !value)}
          />
        )}
        {surface === 'arena' && (
          <div className="relative flex min-h-0 flex-1 overflow-hidden">
          <main id="kubona-main" tabIndex={-1} className="relative flex flex-col min-h-0 min-w-0 flex-1 overflow-hidden">
            <ArenaHud
              targetLabel={targetLabel}
              swarmDetail={runPhase === 'idle' ? 'idle · at rest' : `${loops.length} Loops Live`}
              view={arenaView}
              onViewChange={handleArenaViewChange}
              showWorkbench={workbenchEnabled}
              campaignTitle={session?.title}
              onOpenCampaigns={() => setShowCampaignMenu((v) => !v)}
              spendUsd={session?.current_spend_usd ?? 0}
              spendCapUsd={session?.spend_limit_usd ?? 150}
              sessionCreatedAt={session?.created_at}
              statusText={runStatusText}
              onRecenter={() => setRecenterSignal((n) => n + 1)}
              experimentLabOpen={isExperimentLabOpen}
              onOpenExperimentLab={!moleculeObjectViewOpen ? () => toggleExperimentLab('toggle') : undefined}
              onOpenMoleculeView={!moleculeObjectViewOpen ? () => setMoleculeObjectViewOpen(true) : undefined}
            />

            <div className="relative min-h-0 flex-1 overflow-hidden">
            {arenaView === 'workbench' && !moleculeObjectViewOpen && (
              <Workbench
                key={currentSessionId}
                ref={workbenchRef}
                campaign={campaign}
                error={campaignFailure?.message ?? null}
                authError={campaignAuthError}
                onSignIn={() => { setSurface('mesh'); setMeshTab('labs'); setInspectorOpen(false); }}
                onRetry={reloadCampaign}
                loops={loops}
                jobs={jobs}
                obligations={obligations}
                tasks={tasks}
                sessionSpendUsd={session?.current_spend_usd}
                recenterSignal={recenterSignal}
                onCreateCampaign={handleCreateSession}
                localMode={access?.anonymous_chat === true}
                multiLab={access?.mode === 'multi_lab'}
                access={access}
                onTaskStatus={(taskId, status) => { void handleTaskStatus(taskId, status); }}
                onCloseObligation={(id) => handleCloseObligation(id)}
                sessionId={currentSessionId || undefined}
                confirmGate={confirmGate}
                onInlineGateVisibility={setInlineGateOwned}
                onObjectsChange={handleWorkbenchObjectsChange}
                onLiftObject={(object) => {
                  if (object.kind === 'molecule') {
                    setMoleculeObjectViewOpen(true);
                    return;
                  }
                  setLiftedObject(object);
                }}
                onSelectObject={(object) => {
                  setSelectedWorkbenchObject(object);
                  if (object) {
                    openInspector('details');
                  }
                }}
                onOpenObject={handleOpenWorkbenchObject}
                onAskSelection={(selectedObjs) => {
                  const first = selectedObjs[0];
                  if (first) setSelectedWorkbenchObject(first);
                  openInspector('copilot');
                }}
              />
            )}
            {arenaView === 'orbital' && !moleculeObjectViewOpen && (
              <SwarmUniverse campaign={campaign} recenterSignal={recenterSignal} activeProbe={activeProbe} atRest={runPhase === 'idle'} />
            )}
            {arenaView === 'proof-dag' && !moleculeObjectViewOpen && (
              <ProofDagView campaign={campaign} />
            )}
            {arenaView === 'hardware-qpu' && !moleculeObjectViewOpen && (
              <div className="h-full overflow-y-auto overscroll-contain p-4 sm:p-6">
                <div className="max-w-6xl mx-auto">
                  <FleetRadar campaign={campaign} />
                </div>
              </div>
            )}
            {moleculeObjectViewOpen && (
              <div
                role="dialog"
                aria-label="Molecule object view"
                data-testid="molecule-object-view"
                className="absolute inset-0 overflow-y-auto overscroll-contain pt-6 px-6 pb-6 bg-[var(--color-canvas)]/95"
              >
                <div className="flex items-center justify-end max-w-6xl mx-auto pb-2">
                  <button
                    onClick={() => setMoleculeObjectViewOpen(false)}
                    aria-label="Close molecule object view"
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-[var(--color-fill)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-all"
                  >
                    <X className="w-3.5 h-3.5" /> Back to Workbench
                  </button>
                </div>
                <SpatialCanvas campaign={campaign} />
              </div>
            )}

            {/* On-demand canvas widget (co-pilot materialized, single slot) */}
            {!moleculeObjectViewOpen && (
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

            {/* Calibrated action gate — fallback when inline workbench circuit card gate is not active */}
            {gateOpen && !moleculeObjectViewOpen && !(arenaView === 'workbench' && inlineGateOwned) && (
              <ActionGate
                campaign={campaign}
                session={session}
                onAuthorized={handleAuthorizePulse}
                onDenied={handleDenyPulse}
                onDowngrade={handleDowngradePulse}
              />
            )}
            {gateNotice && (
              <div className="absolute bottom-6 left-6 z-20 w-[min(20rem,calc(100vw-3rem))] max-w-[calc(100vw-3rem)] p-3 rounded-2xl bg-[var(--color-float)] border border-[var(--color-border-strong)] text-[12px] text-[var(--color-label-secondary)] shadow-[var(--shadow-float)]">
                {gateNotice}
              </div>
            )}
            {toast && (
              <div
                key={toast.key}
                role="status"
                aria-live="polite"
                className="absolute bottom-6 left-1/2 -translate-x-1/2 z-20 px-4 py-2 rounded-full bg-[var(--color-float)] border border-[var(--color-border-strong)] text-[12px] font-code text-[var(--color-text)] shadow-[var(--shadow-float)]"
              >
                {toast.text}
              </div>
            )}
            </div>
          </main>
          {isExperimentLabOpen && (
            <>
              <div
                className="fixed inset-0 z-30 bg-[var(--color-canvas)]/70 backdrop-blur-[2px] lg:hidden"
                onClick={() => setIsExperimentLabOpen(false)}
                aria-hidden="true"
              />
              <aside
                aria-label="Experiment & Discovery Lab"
                className="fixed inset-y-0 right-0 z-40 w-[min(26rem,100%)] lg:static lg:z-auto lg:w-[420px] shrink-0 flex flex-col bg-[var(--color-surface-elev)] border-l border-[var(--color-border-strong)] shadow-[var(--shadow-float)] lg:shadow-none"
              >
                <div className="flex items-center justify-between gap-2 px-4 py-3 border-b border-[var(--color-border)] shrink-0">
                  <div className="flex items-center gap-2 min-w-0">
                    <Zap className="w-4 h-4 text-[var(--color-accent-ink)] shrink-0" />
                    <h2 className="text-xs font-code uppercase tracking-wider text-[var(--color-text)] truncate">
                      Experiment &amp; Discovery Lab
                    </h2>
                    <KeyHint>{SHORTCUTS.experimentLab}</KeyHint>
                  </div>
                  <button
                    type="button"
                    onClick={() => setIsExperimentLabOpen(false)}
                    aria-label="Close Experiment & Discovery Lab"
                    className="w-7 h-7 rounded-full flex items-center justify-center text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
                  >
                    <X className="w-4 h-4" />
                  </button>
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto overscroll-contain p-4 space-y-4">
                  <ExperimentPanel
                    sessionId={currentSessionId || null}
                    tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id}
                  />
                  <DiscoveryProtocolPanel sessionId={currentSessionId || null} />
                </div>
              </aside>
            </>
          )}
          </div>
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
                <FlightRecorder loops={loops} events={events} sessionId={currentSessionId || null} campaign={campaign} onLoopsChanged={refreshLoops} onSessionImported={handleSessionImported} />
              )}
              {surface === 'manifold' && <EpistemicManifold campaign={campaign} sessionId={currentSessionId || null} />}
              {surface === 'fleet' && <FleetRadar campaign={campaign} />}
              {surface === 'proofs' && proofsTab === 'manuscripts' && <ResultsStudio campaign={campaign} sessionId={currentSessionId || null} />}
              {surface === 'proofs' && proofsTab === 'tribunal' && (() => {
                const tribunalTarget = campaign?.target_system || campaign?.quantum_factors?.target_molecule || '';
                const tribunalSlug = tribunalTarget.toLowerCase().replace(/[^a-z0-9]/g, '');
                return (
                  <EpistemicTribunalRoom
                    sessionId={session?.id ?? ''}
                    targetSystem={tribunalTarget}
                    hypothesisId={campaign?.hypothesis_id || (tribunalSlug ? `hyp-qksd-${tribunalSlug}-01` : '')}
                    campaign={campaign}
                    onCampaignSynthesized={(c) => setCampaign(c)}
                  />
                );
              })()}
              {surface === 'proofs' && proofsTab === 'gallery' && (
                <GenerativeUIStudio
                  campaign={campaign}
                  tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id}
                  onOpenObservatory={() => {
                    setSurface('arena');
                    setMoleculeObjectViewOpen(true);
                  }}
                />
              )}
              {surface === 'proofs' && proofsTab === 'circuits' && <CodesignCockpit />}
              {surface === 'mesh' && meshTab === 'fleet' && <FleetRadar campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'mesh' && <CognitiveMesh campaign={campaign} />}
              {surface === 'mesh' && meshTab === 'guardrails' && <AutonomyCockpit campaign={campaign} session={session} />}
              {surface === 'mesh' && meshTab === 'models' && <HuggingFaceModelHub />}
              {surface === 'mesh' && meshTab === 'labs' && <LabsView />}
              {surface === 'mesh' && meshTab === 'settings' && (
                <div className="mx-auto max-w-4xl space-y-4">
                  <nav className="k-settings-nav" aria-label="Settings sections" data-testid="settings-nav">
                    {([
                      ['models', 'Models'],
                      ['quantum', 'Quantum providers'],
                      ['dispatch', 'Quantum dispatch'],
                      ['vault', 'Vault leases'],
                      ['readiness', 'System readiness'],
                      ['appearance', 'Appearance'],
                    ] as const).map(([id, label]) => (
                      <button
                        key={id}
                        type="button"
                        aria-current={settingsSection === id ? 'page' : undefined}
                        onClick={() => setSettingsSection(id)}
                        className={settingsSection === id ? 'is-active' : undefined}
                      >
                        {label}
                      </button>
                    ))}
                  </nav>
                  {settingsSection === 'models' && <LLMSettings />}
                  {settingsSection === 'quantum' && <QuantumSettings />}
                  {settingsSection === 'dispatch' && <QuantumDispatch sessionId={currentSessionId || null} tenantId={sessions.find((s) => s.id === currentSessionId)?.tenant_id} />}
                  {settingsSection === 'vault' && <VaultLeasePanel sessionId={currentSessionId || undefined} />}
                  {settingsSection === 'readiness' && <SystemReadinessPanel />}
                  {settingsSection === 'appearance' && <AppearanceSettings />}
                </div>
              )}
            </div>
          </main>
        )}
        {surface === 'arena' && (
          <CommandDock
            onSubmit={handleAsk}
            contextLabel={selectedWorkbenchObject ? `${selectedWorkbenchObject.label}: ${selectedWorkbenchObject.title}` : campaign?.target_system || null}
            onClearContext={() => setSelectedWorkbenchObject(null)}
            onAttach={copilotChat.attach}
            onDetach={copilotChat.detach}
            attachments={copilotChat.attachments}
            onOpenModel={() => { setSurface('mesh'); setMeshTab('models'); }}
            modelLabel="Model Hub"
            sessionId={currentSessionId || null}
            messageCount={copilotChat.messages.length}
            streaming={Boolean(copilotChat.streamingId)}
            busy={copilotChat.loading || copilotChat.runningTribunal}
            onStop={copilotChat.stop}
            followUps={copilotChat.followUps}
            onFollowUp={(key) => {
              openInspector('copilot');
              copilotChat.runFollowUp(key);
            }}
            onTribunal={handleTribunalAsk}
            targetDisplayName={copilotChat.targetDisplayName}
            onSessionChange={(id) => {
              setCurrentSessionId(id);
              refreshData();
            }}
            onOpenInspector={!inspectorOpen ? () => {
              openInspector('details');
            } : undefined}
            inspectorShortcut={SHORTCUTS.copilot}
          />
        )}
        <StatusStrip
          loops={loops}
          error={dataError}
          streamLive={streamLive}
          lastEventAt={events.length > 0 ? events[events.length - 1]?.timestamp : null}
        />
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
        onFocusObject={(object) => focusWorkbenchTarget(object.id)}
        onFocusJob={(job) => focusWorkbenchTarget(job.id)}
        onRemoveObject={(object) => workbenchRef.current?.removeObject(object.id)}
        copilot={
          <CopilotDrawer
            controller={copilotChat}
            dockComposer
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
            onCreateCampaign={handleCreateSession}
            requiresSignIn={Boolean(sessionListError && /multi-lab|unauthorized|log in/i.test(sessionListError))}
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
      {!inspectorOpen && surface !== 'arena' && (
        <button
          onClick={() => { openInspector('details'); }}
          className="fixed bottom-12 right-6 z-40 px-4 py-2.5 rounded-full bg-[var(--color-float)] border border-[var(--color-accent-edge)] hover:bg-[var(--color-fill-hover)] text-[var(--color-text)] shadow-[var(--shadow-float)] flex items-center space-x-2.5 group transition-colors duration-200"
          title={`Open Inspector (${SHORTCUTS.copilot})`}
        >
          <div className="relative">
            <Sparkles className="w-4 h-4 text-[var(--color-accent-ink)]" />
            <span className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-[var(--color-success)]" />
          </div>
          <span className="text-xs font-semibold text-[var(--color-label-secondary)] group-hover:text-[var(--color-text)]">Inspector</span>
          <KeyHint>{SHORTCUTS.copilot}</KeyHint>
        </button>
      )}
      <Dialog
        open={Boolean(liftedObject)}
        onClose={() => setLiftedObject(null)}
        title={liftedObject ? `${liftedObject.label}: ${liftedObject.title}` : ''}
        className="k-dialog-lift"
      >
        {liftedObject && (
          <div className="k-lift-body" data-provenance={liftedObject.provenance ?? 'catalog'}>
            <div className="k-lift-meta">
              <ProvenanceBadge kind={liftedObject.provenance ?? 'catalog'} />
              <div className="k-lift-actions">
                <Button
                  type="button"
                  size="compact"
                  variant="ghost"
                  onClick={() => {
                    setSelectedWorkbenchObject(liftedObject);
                    openInspector('copilot');
                    setLiftedObject(null);
                  }}
                >
                  Ask Copilot
                </Button>
                <Button
                  type="button"
                  size="compact"
                  onClick={() => {
                    const targetKind = liftedObject.kind;
                    const targetId = liftedObject.id;
                    setLiftedObject(null);
                    if (
                      targetKind === 'campaign' ||
                      targetKind === 'task' ||
                      targetKind === 'table' ||
                      targetKind === 'measurement' ||
                      targetKind === 'note'
                    ) {
                      focusWorkbenchTarget(targetId);
                    } else {
                      handleOpenWorkbenchObject(targetKind);
                    }
                  }}
                >
                  {({
                    chart: 'Jump to Flight Recorder',
                    agent_run: 'Jump to Flight Recorder',
                    job: 'Jump to Flight Recorder',
                    approval: 'Jump to Guardrails',
                    evidence: 'Jump to Evidence Manifold',
                    hypothesis: 'Jump to Evidence Manifold',
                    molecule: 'Open Molecule Workbench',
                    proof: 'Jump to Results Studio',
                    manuscript: 'Jump to Results Studio',
                    circuit: 'Jump to Proof DAG',
                    campaign: 'Focus on Workbench',
                    task: 'Focus on Workbench',
                    table: 'Focus on Workbench',
                    measurement: 'Focus on Workbench',
                    note: 'Focus on Workbench',
                  } as Partial<Record<typeof liftedObject.kind, string>>)[liftedObject.kind] ?? 'Focus on Workbench'}
                </Button>
              </div>
            </div>
            <p className="k-lift-detail">{liftedObject.detail}</p>
            {liftedObject.meta?.callout && (
              <div className="k-workbench-callout" data-tone={liftedObject.meta.callout.tone}>
                <span>{liftedObject.meta.callout.summary}</span>
              </div>
            )}
            {liftedObject.facts.length > 0 && (
              <dl className="k-lift-facts">
                {liftedObject.facts.map((fact) => (
                  <div key={fact.label}>
                    <dt>{fact.label}</dt>
                    <dd data-metric>{fact.value}</dd>
                  </div>
                ))}
              </dl>
            )}
            {liftedObject.chart && (
              <div className="k-lift-panel" data-metric>
                <Chart spec={liftedObject.chart} />
              </div>
            )}
            {liftedObject.meta?.comparison && liftedObject.meta.comparison.rows.length > 0 && (
              <CompareMatrix comparison={liftedObject.meta.comparison} title={liftedObject.title} />
            )}
            {liftedObject.meta?.code && liftedObject.meta.code.content && (
              <CodeBlock code={liftedObject.meta.code} initialExpanded />
            )}
            {liftedObject.kind === 'manuscript' && (
              <ResultsStudio campaign={campaign} sessionId={currentSessionId || null} />
            )}
            {liftedObject.basis && <p className="k-workbench-basis">{liftedObject.basis}</p>}
            {liftedObject.sources?.length ? (
              <ul className="k-workbench-sources" aria-label={`Sources for ${liftedObject.title}`}>
                {liftedObject.sources.map((source) => (
                  <li key={source.label}>
                    {source.url
                      ? <a href={source.url} target="_blank" rel="noreferrer">{source.label}</a>
                      : <span>{source.label}</span>}
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        )}
      </Dialog>
      </AttentionProvider>
    </div>
  );
}
