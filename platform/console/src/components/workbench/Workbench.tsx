'use client';

import React, { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react';
import { LayoutGrid, List, Maximize2, Minus, Plus } from 'lucide-react';
import type { CampaignState, Job, Loop, Task } from '@/types';
import type { AccessState } from '@/lib/api';
import { fetchCanvasLayout, fetchWorkbenchLayout, fetchWorkbenchView, saveWorkbenchLayout, saveWorkbenchView, type ObligationRow } from '@/lib/api';
import { Chart } from '@/components/data/LazyChart';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { Badge } from '@/components/ui/Badge';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import { acceptWrite, newWriteRequestId, reconcileOperation, settleWrite, writeSettlementLabel, IDLE_WRITE, type WriteSettlement } from '@/domain/write-settlement';
import {
  AskReceiptBanner,
  ConfirmCard,
  type AskReceipt,
  type ConfirmGateConfig,
} from './ConfirmCard';
import {
  WORKBENCH_CLEARANCE,
  alignSelectedPositions,
  arrangeSelectedPositions,
  boardPin,
  buildWorkbench,
  buildWorkbenchLinks,
  comparisonCsv,
  contractPlacement,
  distributeSelectedPositions,
  dropSpot,
  filterWorkbenchObjects,
  freeSpot,
  layoutBounds,
  nextZoomTier,
  readPositions,
  readSizes,
  readWorkbenchDocument,
  readWorkbenchView,
  rectsOverlap,
  reflowUnpinnedObjects,
  resolveFocusObjectId,
  routeOrthogonalBusEdges,
  selectFocusedLinks,
  selectSpineChainLinks,
  snapToGrid,
  validateManualOverrideValue,
  workbenchDocument,
  workbenchViewDocument,
  type WorkbenchArea,
  type WorkbenchCamera,
  type WorkbenchComparisonData,
  type WorkbenchFilter,
  type WorkbenchLink,
  type WorkbenchObject,
  type WorkbenchObjectKind,
  type WorkbenchPins,
  type WorkbenchPositions,
  type WorkbenchRect,
  type WorkbenchSizes,
  type WorkbenchViewLod,
  type WorkbenchZoomTier,
} from './workbench-model';

type Props = {
  campaign?: CampaignState | null;
  error?: string | null;
  onRetry?: () => void;
  sessionId?: string;
  loops?: Loop[];
  jobs?: Job[];
  obligations?: ObligationRow[];
  tasks?: Task[];
  sessionSpendUsd?: number;
  recenterSignal?: number;
  onTaskStatus?: (taskId: string, status: 'open' | 'done') => void;
  onCloseObligation?: (id: string) => void | Promise<{ ok: boolean; error?: string } | void>;
  onSelectObject?: (object: WorkbenchObject | null) => void;
  onOpenObject?: (kind: WorkbenchObjectKind) => void;
  onLiftObject?: (object: WorkbenchObject) => void;
  onAskSelection?: (objects: WorkbenchObject[]) => void;
  onCreateCampaign?: () => void;
  confirmGate?: ConfirmGateConfig | null;
  onGateSettled?: (receipt: AskReceipt) => void;
  onInlineGateVisibility?: (ownedByCard: boolean) => void;
  localMode?: boolean;
  multiLab?: boolean;
  authError?: boolean;
  onSignIn?: () => void;
  access?: AccessState | null;
};

type EdgeVisibility = 'hybrid' | 'focus' | 'all' | 'off';

type WorkbenchSnapshot = {
  positions: WorkbenchPositions;
  sizes: WorkbenchSizes;
  pins: WorkbenchPins;
  hidden: string[];
  areas: WorkbenchArea[];
  assignments: Record<string, string>;
  storedLinks: WorkbenchLink[] | null;
  overrides: Record<string, Record<string, unknown>>;
};

type WorkbenchUndoEntry = {
  kind: 'remove' | 'move' | 'resize' | 'link' | 'arrange' | 'area' | 'override';
  label: string;
  title: string;
  snapshot: WorkbenchSnapshot;
};

const MAX_UNDO_ENTRIES = 30;

export type WorkbenchHandle = {
  focusObject: (id: string) => void;
  removeObject: (id: string) => void;
};

function lodForZoom(zoom: number): WorkbenchViewLod {
  if (zoom < 0.55) return 'summary';
  if (zoom < 0.95) return 'compact';
  return 'detail';
}

export const Workbench = React.forwardRef<WorkbenchHandle, Props>(function Workbench({
  campaign, error = null, onRetry, sessionId, loops = [], jobs: suppliedJobs, obligations: suppliedObligations, tasks: suppliedTasks, sessionSpendUsd, recenterSignal = 0, onTaskStatus, onCloseObligation, onSelectObject, onOpenObject, onLiftObject, onAskSelection, onCreateCampaign, confirmGate = null, onGateSettled, onInlineGateVisibility, localMode = false, multiLab = false, authError = false, onSignIn, access,
}, ref) {
  const hostRef = useRef<HTMLDivElement>(null);
  const worldRef = useRef<HTMLDivElement>(null);
  const revision = useRef(0);
  const viewRevision = useRef(0);
  const loaded = useRef(false);
  const layoutInFlight = useRef(false);
  const layoutPendingSave = useRef(false);
  const viewInFlight = useRef(false);
  const viewPendingSave = useRef(false);
  const drag = useRef<{ x: number; y: number; camera: WorkbenchCamera; moved: boolean } | null>(null);
  const marqueeDrag = useRef<{ startX: number; startY: number; curX: number; curY: number; additive: boolean } | null>(null);
  const cardDrag = useRef<{ ids: string[]; x: number; y: number; moved: boolean; origins: Record<string, { x: number; y: number }>; snapshot: WorkbenchSnapshot } | null>(null);
  const sizeDrag = useRef<{ id: string; x: number; y: number; width: number; height: number; snapshot: WorkbenchSnapshot } | null>(null);
  const areaDrag = useRef<{ id: string; x: number; y: number; originX: number; originY: number; childOrigins: Record<string, { x: number; y: number }> } | null>(null);
  const areaResize = useRef<{ id: string; x: number; y: number; width: number; height: number } | null>(null);
  const minimapDrag = useRef(false);
  const minimapCanvasRef = useRef<HTMLCanvasElement | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const viewSaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const heldTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const heldRef = useRef(false);
  const lastSavedDocument = useRef<string>('');
  const lastSavedView = useRef<string>('');
  const [camera, setCamera] = useState<WorkbenchCamera>({ x: 36, y: 36, zoom: 0.82 });
  const cameraRef = useRef(camera);
  const [zoomTier, setZoomTier] = useState<WorkbenchZoomTier>(() => nextZoomTier(0.82, 'standard'));
  const zoomCommit = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [draggingIds, setDraggingIds] = useState<Set<string>>(() => new Set());
  const [edgeVisibility, setEdgeVisibility] = useState<EdgeVisibility>('hybrid');
  const [filter, setFilter] = useState<WorkbenchFilter>('all');
  const [followLive, setFollowLive] = useState(false);
  const [held, setHeld] = useState(false);
  const [unseen, setUnseen] = useState(
    () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  );
  const [gateReceipts, setGateReceipts] = useState<Record<string, AskReceipt>>({});
  const [obligationReceipts, setObligationReceipts] = useState<Record<string, AskReceipt>>({});
  const [obligationErrors, setObligationErrors] = useState<Record<string, string>>({});
  const [spaceHeld, setSpaceHeld] = useState(false);
  const spaceHeldRef = useRef(false);
  const [marqueeBox, setMarqueeBox] = useState<{ left: number; top: number; width: number; height: number } | null>(null);
  const [pins, setPins] = useState<WorkbenchPins>({});
  const [positions, setPositions] = useState<WorkbenchPositions>({});
  const [sizes, setSizes] = useState<WorkbenchSizes>({});
  const [measuredSizes, setMeasuredSizes] = useState<WorkbenchSizes>({});
  const measuredSizesRef = useRef<WorkbenchSizes>({});
  const measureQueuedRef = useRef(false);
  const [overrides, setOverrides] = useState<Record<string, Record<string, unknown>>>({});
  const [storedLinks, setStoredLinks] = useState<WorkbenchLink[] | null>(null);
  const [layoutObjects, setLayoutObjects] = useState<{ id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[]>([]);
  const [settledSessionId, setSettledSessionId] = useState<string | null>(null);
  const hydrationSettled = !sessionId || settledSessionId === sessionId;
  const [linkKind, setLinkKind] = useState<WorkbenchLink['kind']>('supports');
  const jobs = useMemo(() => suppliedJobs ?? [], [suppliedJobs]);
  const obligations = useMemo(() => suppliedObligations ?? [], [suppliedObligations]);
  const tasks = useMemo(() => suppliedTasks ?? [], [suppliedTasks]);
  const baseObjects = useMemo(() => {
    const projected = buildWorkbench(campaign, loops, jobs, obligations, tasks, sessionSpendUsd, pins);
    if (!campaign || !hydrationSettled) return projected;
    const absorbed = new Set<string>();
    for (const object of projected) {
      absorbed.add(object.id);
      if (object.ref?.id) absorbed.add(`${object.ref.domain}-${object.ref.id}`);
      for (const revId of object.meta?.revises ?? []) {
        absorbed.add(revId);
        absorbed.add(`job-${revId}`);
      }
      for (const run of object.meta?.runs ?? []) {
        absorbed.add(run.id);
        absorbed.add(`job-${run.id}`);
      }
    }
    const obstacles: WorkbenchRect[] = projected.map((object) => ({
      x: object.position.x,
      y: object.position.y,
      width: object.size.width,
      height: object.size.height,
    }));
    const unavailable: WorkbenchObject[] = [];
    for (const object of layoutObjects) {
      if (!object.ref || absorbed.has(object.id)) continue;
      const box: WorkbenchRect = { ...object.position, ...object.size };
      const overlaps = (object.position.x === 0 && object.position.y === 0)
        || obstacles.some((obs) => rectsOverlap(box, obs, WORKBENCH_CLEARANCE));
      const pos = overlaps
        ? freeSpot(object.size, { x: 0, y: Math.max(0, ...obstacles.map((o) => o.y + o.height)) + WORKBENCH_CLEARANCE }, obstacles, WORKBENCH_CLEARANCE)
        : object.position;
      obstacles.push({ ...pos, ...object.size });
      unavailable.push({
        id: object.id,
        kind: object.kind,
        label: 'Unavailable',
        title: `${object.ref.domain} record ${object.ref.id.slice(0, 12)}`,
        detail: 'This reference is preserved in the layout but is not in the live projection.',
        position: pos,
        size: object.size,
        facts: [
          { label: 'Domain', value: object.ref.domain },
          { label: 'Record', value: object.ref.id },
        ],
        ref: object.ref,
      });
    }
    return [...projected, ...unavailable];
  }, [campaign, hydrationSettled, loops, jobs, obligations, tasks, sessionSpendUsd, pins, layoutObjects]);
  const baseObjectsRef = useRef(baseObjects);
  useEffect(() => {
    baseObjectsRef.current = baseObjects;
  }, [baseObjects]);
  const links = useMemo(() => storedLinks ?? buildWorkbenchLinks(baseObjects), [storedLinks, baseObjects]);
  const [viewMode, setViewMode] = useState<'canvas' | 'list'>('canvas');
  const [areas, setAreas] = useState<WorkbenchArea[]>([]);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [assignments, setAssignments] = useState<Record<string, string>>({});
  const [hidden, setHidden] = useState<string[]>([]);
  const [undoStack, setUndoStack] = useState<WorkbenchUndoEntry[]>([]);
  const [activeAreaId, setActiveAreaId] = useState<string | null>(null);
  const [renamingAreaId, setRenamingAreaId] = useState<string | null>(null);
  const [conflictRevision, setConflictRevision] = useState<number | null>(null);
  const [write, setWrite] = useState<WriteSettlement>(IDLE_WRITE);
  useEffect(() => {
    if (!campaign) onSelectObject?.(null);
  }, [campaign, onSelectObject]);

  const measureCard = useCallback((id: string, width: number, height: number) => {
    const prev = measuredSizesRef.current[id];
    if (prev && Math.abs(prev.height - height) < 2 && Math.abs(prev.width - width) < 2) return;
    measuredSizesRef.current = { ...measuredSizesRef.current, [id]: { width, height } };
    if (measureQueuedRef.current) return;
    measureQueuedRef.current = true;
    queueMicrotask(() => {
      measureQueuedRef.current = false;
      setMeasuredSizes({ ...measuredSizesRef.current });
    });
  }, []);

  const objects = useMemo(() => {
    const merged = baseObjects.map((object) => ({
      ...object,
      position: pins[object.id] && positions[object.id] ? positions[object.id]! : object.position,
      size: sizes[object.id] ?? object.size,
    }));
    return reflowUnpinnedObjects(merged, pins, measuredSizes);
  }, [baseObjects, pins, positions, sizes, measuredSizes]);

  const lod = lodForZoom(camera.zoom);
  const worldEnvelope = useMemo(() => contractPlacement(objects), [objects]);
  const objectsRef = useRef(objects);
  useEffect(() => {
    objectsRef.current = objects;
  }, [objects]);
  const selectObject = useCallback((object: WorkbenchObject, additive = false) => {
    setSelected((previous) => {
      if (!additive) return [object.id];
      return previous.includes(object.id)
        ? previous.filter((id) => id !== object.id)
        : [...previous, object.id];
    });
    onSelectObject?.(object);
  }, [onSelectObject]);

  const visibleObjects = useMemo(() => objects.filter((object) => !hidden.includes(object.id)), [objects, hidden]);
  const filteredObjects = useMemo(() => filterWorkbenchObjects(visibleObjects, filter), [visibleObjects, filter]);
  const filterCounts = useMemo(() => ({
    all: visibleObjects.length,
    spine: filterWorkbenchObjects(visibleObjects, 'spine').length,
    jobs: filterWorkbenchObjects(visibleObjects, 'jobs').length,
    issues: filterWorkbenchObjects(visibleObjects, 'issues').length,
  }), [visibleObjects]);
  const visibleObjectMap = useMemo(() => new Map(filteredObjects.map((object) => [object.id, object])), [filteredObjects]);
  const inlineGateOwned = useMemo(
    () => Boolean(confirmGate && viewMode === 'canvas' && filteredObjects.some((item) => item.kind === 'circuit')),
    [confirmGate, viewMode, filteredObjects],
  );
  useEffect(() => {
    onInlineGateVisibility?.(inlineGateOwned);
    return () => { onInlineGateVisibility?.(false); };
  }, [inlineGateOwned, onInlineGateVisibility]);
  useEffect(() => () => {
    if (heldTimer.current) {
      clearTimeout(heldTimer.current);
      heldTimer.current = null;
    }
  }, []);
  const areaById = useMemo(() => new Map(areas.map((area) => [area.id, area])), [areas]);
  const areaChildCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const object of visibleObjects) {
      const areaId = assignments[object.id];
      if (areaId) counts[areaId] = (counts[areaId] ?? 0) + 1;
    }
    return counts;
  }, [visibleObjects, assignments]);
  const activeArea = areas.find((area) => area.id === activeAreaId) ?? areas[0] ?? null;

  const captureSnapshot = useCallback((): WorkbenchSnapshot => ({
    positions: { ...positions },
    sizes: { ...sizes },
    pins: { ...pins },
    hidden: [...hidden],
    areas: areas.map((area) => ({ ...area, rect: { ...area.rect } })),
    assignments: { ...assignments },
    storedLinks: storedLinks ? storedLinks.map((link) => ({ ...link })) : null,
    overrides: Object.fromEntries(Object.entries(overrides).map(([k, v]) => [k, { ...v }])),
  }), [positions, sizes, pins, hidden, areas, assignments, storedLinks, overrides]);

  const pushUndo = useCallback((
    kind: WorkbenchUndoEntry['kind'],
    label: string,
    title: string,
    snapshot: WorkbenchSnapshot,
  ) => {
    setUndoStack((previous) => [...previous.slice(-(MAX_UNDO_ENTRIES - 1)), { kind, label, title, snapshot }]);
  }, []);

  const undoLast = useCallback(() => {
    setUndoStack((previous) => {
      if (previous.length === 0) return previous;
      const top = previous[previous.length - 1];
      const snap = top.snapshot;
      setPositions(snap.positions);
      setSizes(snap.sizes);
      setPins(snap.pins);
      setHidden(snap.hidden);
      setAreas(snap.areas);
      setAssignments(snap.assignments);
      setStoredLinks(snap.storedLinks);
      setOverrides(snap.overrides);
      return previous.slice(0, -1);
    });
  }, []);

  const updateCardOverride = useCallback((objectId: string, cellKey: string, nextValue: string | null) => {
    pushUndo('override', 'Undo edit', 'Undo manual table override (Z)', captureSnapshot());
    setOverrides((previous) => {
      const current = { ...(previous[objectId] ?? {}) };
      if (nextValue === null) {
        delete current[cellKey];
      } else {
        current[cellKey] = { value: nextValue, provenance: 'MANUAL', updated_at: new Date().toISOString() };
      }
      if (Object.keys(current).length === 0) {
        const next = { ...previous };
        delete next[objectId];
        return next;
      }
      return { ...previous, [objectId]: current };
    });
  }, [pushUndo, captureSnapshot]);

  const areaBounds = useMemo(() => {
    if (visibleObjects.length === 0) return null;
    const minX = Math.min(...visibleObjects.map((object) => object.position.x)) - 24;
    const minY = Math.min(...visibleObjects.map((object) => object.position.y)) - 48;
    const maxX = Math.max(...visibleObjects.map((object) => object.position.x + object.size.width)) + 24;
    const maxY = Math.max(...visibleObjects.map((object) => object.position.y + object.size.height)) + 24;
    return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
  }, [visibleObjects]);

  const removeSelected = useCallback(() => {
    if (selected.length === 0) return;
    pushUndo('remove', 'Undo remove', 'Restore the last removed card', captureSnapshot());
    setHidden((previous) => Array.from(new Set([...previous, ...selected])));
    setSelected([]);
    onSelectObject?.(null);
  }, [selected, onSelectObject, pushUndo, captureSnapshot]);

  const customLink = useCallback(() => {
    if (selected.length !== 2) return;
    const [source, target] = selected;
    const current = storedLinks ?? buildWorkbenchLinks(baseObjects);
    if (current.some((link) => link.source === source && link.target === target && link.kind === linkKind)) return;
    pushUndo('link', 'Undo link', 'Undo link edit (Z)', captureSnapshot());
    setStoredLinks([...current, { id: `${source}-${target}-${linkKind}`, source, target, kind: linkKind }]);
  }, [selected, linkKind, baseObjects, storedLinks, pushUndo, captureSnapshot]);

  const removeLink = useCallback((id: string) => {
    pushUndo('link', 'Undo link', 'Undo link removal (Z)', captureSnapshot());
    setStoredLinks((previous) => (previous ?? buildWorkbenchLinks(baseObjects)).filter((link) => link.id !== id));
  }, [baseObjects, pushUndo, captureSnapshot]);

  const holdCanvas = useCallback(() => {
    if (!heldRef.current) {
      heldRef.current = true;
      setHeld(true);
    }
    if (heldTimer.current) clearTimeout(heldTimer.current);
    heldTimer.current = setTimeout(() => {
      heldTimer.current = null;
      heldRef.current = false;
      setHeld(false);
    }, 1000);
  }, []);

  const commitCameraNext = useCallback((next: WorkbenchCamera) => {
    cameraRef.current = next;
    setCamera(next);
    setZoomTier((curr) => nextZoomTier(next.zoom, curr));
  }, []);

  useImperativeHandle(ref, () => ({
    focusObject: (id: string) => {
      const resolvedId = resolveFocusObjectId(id, objects) ?? id;
      const object = objects.find((item) => item.id === resolvedId);
      if (!object) return;
      setFilter('all');
      setHidden((previous) => previous.filter((item) => item !== object.id));
      selectObject(object, false);
      const host = hostRef.current;
      if (!host) return;
      const zoom = Math.min(1.5, Math.max(0.25, camera.zoom));
      commitCameraNext({
        x: host.clientWidth / 2 - (object.position.x + object.size.width / 2) * zoom,
        y: host.clientHeight / 2 - (object.position.y + object.size.height / 2) * zoom,
        zoom,
      });
    },
    removeObject: (id: string) => {
      pushUndo('remove', 'Undo remove', 'Restore the last removed card', captureSnapshot());
      setHidden((previous) => previous.includes(id) ? previous : [...previous, id]);
      setSelected((previous) => previous.filter((item) => item !== id));
      onSelectObject?.(null);
    },
  }), [objects, camera.zoom, selectObject, onSelectObject, pushUndo, captureSnapshot, commitCameraNext]);

  const applyCamera = useCallback((next: WorkbenchCamera) => {
    cameraRef.current = next;
    if (worldRef.current) {
      worldRef.current.style.transform = `translate3d(${next.x}px, ${next.y}px, 0) scale(${next.zoom})`;
    }
    setZoomTier((curr) => nextZoomTier(next.zoom, curr));
  }, []);

  const commitCamera = useCallback(() => {
    commitCameraNext(cameraRef.current);
  }, [commitCameraNext]);

  useEffect(() => {
    cameraRef.current = camera;
    if (worldRef.current) {
      worldRef.current.style.transform = `translate3d(${camera.x}px, ${camera.y}px, 0) scale(${camera.zoom})`;
    }
  }, [camera]);

  const fitRect = useCallback((box: WorkbenchRect) => {
    const host = hostRef.current;
    if (!host) return;
    const width = host.clientWidth;
    const height = host.clientHeight;
    if (!width || !height) return;
    const padLeft = 48;
    const padRight = width > 640 ? 212 : 48;
    const padTop = height > 320 ? 32 : 20;
    const padBottom = height > 420 ? 148 : 36;
    const usableW = Math.max(120, width - padLeft - padRight);
    const usableH = Math.max(120, height - padTop - padBottom);
    const zoom = Math.min(1.15, Math.max(0.25, Math.min(usableW / Math.max(1, box.width), usableH / Math.max(1, box.height))));
    commitCameraNext({
      x: Math.round(padLeft + (usableW - box.width * zoom) / 2 - box.x * zoom),
      y: Math.round(padTop + (usableH - box.height * zoom) / 2 - box.y * zoom),
      zoom,
    });
  }, [commitCameraNext]);

  const fit = useCallback(() => {
    const box = layoutBounds(objectsRef.current.filter((item) => !hidden.includes(item.id)));
    fitRect(box);
  }, [hidden, fitRect]);

  const firstRecenterRef = useRef(recenterSignal);
  useEffect(() => {
    if (recenterSignal === firstRecenterRef.current) return;
    firstRecenterRef.current = recenterSignal;
    fit();
  }, [recenterSignal, fit]);

  const applySaved = useCallback((
    saved: { revision: number; document: Record<string, unknown> },
    savedView?: { view_revision: number; view: Record<string, unknown> } | null,
  ) => {
    revision.current = saved.revision;
    const doc = readWorkbenchDocument(saved.document);
    const view = readWorkbenchView(savedView?.view ?? null, saved.document);
    viewRevision.current = savedView?.view_revision ?? 0;

    setPins(doc.pins);
    setPositions(doc.positions);
    setSizes(doc.pinnedSizes);
    setAreas(doc.areas);
    setAssignments(doc.assignments);
    setHidden(doc.hidden);
    setOverrides(doc.overrides);
    setStoredLinks(doc.links);
    setLayoutObjects(doc.objects);
    commitCameraNext(view.camera);
    setSelected(view.selected);
    setViewMode(view.view_mode);
    setConflictRevision(null);

    if (sessionId) {
      lastSavedDocument.current = JSON.stringify(
        workbenchDocument(
          sessionId,
          baseObjectsRef.current,
          doc.positions,
          doc.pinnedSizes,
          doc.pins,
          doc.links ?? [],
          doc.areas,
          doc.assignments,
          doc.hidden,
          doc.overrides,
        ),
      );
    }
    lastSavedView.current = JSON.stringify(
      workbenchViewDocument({
        camera: view.camera,
        selected: view.selected,
        view_mode: view.view_mode,
        lod: lodForZoom(view.camera.zoom),
      }),
    );
    loaded.current = true;
  }, [sessionId]);

  useEffect(() => {
    loaded.current = false;
    if (!sessionId) {
      lastSavedDocument.current = '';
      lastSavedView.current = '';
      loaded.current = true;
      return;
    }
    let live = true;
    const hydrateTimer = setTimeout(() => {
      if (live) setSettledSessionId(sessionId);
    }, 250);
    void (async () => {
      const [savedLayout, savedView] = await Promise.all([
        fetchWorkbenchLayout(sessionId),
        typeof fetchWorkbenchView === 'function' ? fetchWorkbenchView(sessionId) : Promise.resolve(null),
      ]);
      if (!live) return;
      if (savedLayout && savedLayout.revision > 0) {
        applySaved(savedLayout, savedView);
        return;
      }
      const legacy = await fetchCanvasLayout(sessionId);
      if (!live) return;
      revision.current = 0;
      const legacyPos = readPositions(legacy);
      const legacySizes = readSizes(legacy);
      const legacyPins = Object.fromEntries(Object.keys(legacyPos).map((id) => [id, true]));
      const view = readWorkbenchView(savedView?.view ?? null, legacy);
      viewRevision.current = savedView?.view_revision ?? 0;

      setPins(legacyPins);
      setPositions(legacyPos);
      setSizes(legacySizes);
      setAreas([]);
      setAssignments({});
      setHidden([]);
      setOverrides({});
      setStoredLinks(null);
      setLayoutObjects([]);
      setCamera(view.camera);
      setSelected(view.selected);
      setViewMode(view.view_mode);
      setConflictRevision(null);

      lastSavedDocument.current = JSON.stringify(
        workbenchDocument(sessionId, baseObjectsRef.current, legacyPos, legacySizes, legacyPins),
      );
      lastSavedView.current = JSON.stringify(
        workbenchViewDocument({
          camera: view.camera,
          selected: view.selected,
          view_mode: view.view_mode,
          lod: lodForZoom(view.camera.zoom),
        }),
      );
      loaded.current = true;
    })();
    return () => {
      live = false;
      clearTimeout(hydrateTimer);
    };
  }, [sessionId, applySaved]);

  const reloadLatest = useCallback(async () => {
    if (!sessionId) return;
    const [savedLayout, savedView] = await Promise.all([
      fetchWorkbenchLayout(sessionId),
      typeof fetchWorkbenchView === 'function' ? fetchWorkbenchView(sessionId) : Promise.resolve(null),
    ]);
    if (savedLayout && savedLayout.revision > 0) applySaved(savedLayout, savedView);
  }, [sessionId, applySaved]);

  // Semantic layout save channel (D1): single-flight, CAS-guarded, never carries camera or unpinned derived cards.
  const flushLayoutSaveRef = useRef<() => void>(() => {});
  const flushLayoutSave = useCallback(() => {
    if (!sessionId || !loaded.current) return;
    const doc = workbenchDocument(
      sessionId,
      baseObjectsRef.current,
      positions,
      sizes,
      pins,
      storedLinks ?? [],
      areas,
      assignments,
      hidden,
      overrides,
    ) as unknown as Record<string, unknown>;
    const serialized = JSON.stringify(doc);
    if (serialized === lastSavedDocument.current) return;
    if (layoutInFlight.current) {
      layoutPendingSave.current = true;
      return;
    }
    layoutInFlight.current = true;
    layoutPendingSave.current = false;
    const requestId = newWriteRequestId();
    setWrite(acceptWrite(requestId, 'saving layout'));
    void saveWorkbenchLayout(sessionId, revision.current, doc, requestId)
      .then((out) => {
        if (out.ok) {
          lastSavedDocument.current = serialized;
          if (typeof out.revision === 'number') revision.current = out.revision;
          setWrite(settleWrite(requestId, 'applied', `revision ${revision.current}`));
        } else if (out.conflict) {
          setConflictRevision(out.currentRevision ?? null);
          setWrite(settleWrite(requestId, 'refused', out.error || 'layout changed elsewhere'));
        } else if (out.unknown) {
          setWrite(settleWrite(requestId, 'unknown', 'save outcome not confirmed'));
          void reconcileOperation(requestId, sessionId).then(setWrite);
        } else {
          setWrite(settleWrite(requestId, 'refused', out.error || 'layout save refused'));
        }
      })
      .finally(() => {
        layoutInFlight.current = false;
        if (layoutPendingSave.current) {
          layoutPendingSave.current = false;
          flushLayoutSaveRef.current();
        }
      });
  }, [sessionId, positions, sizes, pins, storedLinks, areas, assignments, hidden, overrides]);

  useEffect(() => {
    flushLayoutSaveRef.current = flushLayoutSave;
  }, [flushLayoutSave]);

  useEffect(() => {
    if (!sessionId || !loaded.current) return;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      flushLayoutSave();
    }, 450);
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current);
    };
  }, [sessionId, positions, sizes, pins, storedLinks, areas, assignments, hidden, overrides, baseObjects, flushLayoutSave]);

  // Ephemeral view save channel (D1): camera, selection, mode, LOD -> /api/workbench/view only.
  const flushViewSaveRef = useRef<() => void>(() => {});
  const flushViewSave = useCallback(() => {
    if (!sessionId || !loaded.current || typeof saveWorkbenchView !== 'function') return;
    const view = workbenchViewDocument({ camera, selected, view_mode: viewMode, lod }) as unknown as Record<string, unknown>;
    const serialized = JSON.stringify(view);
    if (serialized === lastSavedView.current) return;
    if (viewInFlight.current) {
      viewPendingSave.current = true;
      return;
    }
    viewInFlight.current = true;
    viewPendingSave.current = false;
    const requestId = newWriteRequestId();
    void saveWorkbenchView(sessionId, viewRevision.current, view, requestId)
      .then((out) => {
        if (out.ok) {
          lastSavedView.current = serialized;
          if (typeof out.view_revision === 'number') viewRevision.current = out.view_revision;
        } else if (out.conflict && typeof fetchWorkbenchView === 'function') {
          void fetchWorkbenchView(sessionId).then((latest) => {
            if (latest) viewRevision.current = latest.view_revision;
          });
        }
      })
      .finally(() => {
        viewInFlight.current = false;
        if (viewPendingSave.current) {
          viewPendingSave.current = false;
          flushViewSaveRef.current();
        }
      });
  }, [sessionId, camera, selected, viewMode, lod]);

  useEffect(() => {
    flushViewSaveRef.current = flushViewSave;
  }, [flushViewSave]);

  useEffect(() => {
    if (!sessionId || !loaded.current) return;
    if (viewSaveTimer.current) clearTimeout(viewSaveTimer.current);
    viewSaveTimer.current = setTimeout(() => {
      flushViewSave();
    }, 350);
    return () => {
      if (viewSaveTimer.current) clearTimeout(viewSaveTimer.current);
    };
  }, [sessionId, camera, selected, viewMode, lod, flushViewSave]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const update = () => setViewport({ width: host.clientWidth, height: host.clientHeight });
    update();
    const observer = new ResizeObserver(update);
    observer.observe(host);
    return () => observer.disconnect();
  }, []);

  const visibleIds = useMemo(() => {
    if (filteredObjects.length <= 24 || viewport.width === 0) return null;
    const margin = 160;
    const left = -camera.x / camera.zoom - margin;
    const top = -camera.y / camera.zoom - margin;
    const right = left + viewport.width / camera.zoom + margin * 2;
    const bottom = top + viewport.height / camera.zoom + margin * 2;
    return new Set(filteredObjects.filter((object) => (
      object.position.x + object.size.width >= left &&
      object.position.x <= right &&
      object.position.y + object.size.height >= top &&
      object.position.y <= bottom
    )).map((object) => object.id));
  }, [filteredObjects, viewport, camera]);

  const liveTarget = useMemo(() => {
    const isLiveStatus = (value: string | undefined) => /live|running|active|working|queued|pending/i.test(value ?? '');
    return (
      visibleObjects.find((obj) => obj.kind === 'agent_run' && (isLiveStatus(obj.meta?.status) || isLiveStatus(obj.detail)))
      ?? visibleObjects.find((obj) => obj.kind === 'job' && (isLiveStatus(obj.meta?.status) || (obj.meta?.runs ?? []).some((run) => isLiveStatus(run.status))))
      ?? visibleObjects.find((obj) => obj.kind === 'agent_run')
      ?? visibleObjects.find((obj) => obj.kind === 'job')
      ?? null
    );
  }, [visibleObjects]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    let hidden = typeof document !== 'undefined' && document.visibilityState === 'hidden';
    let offscreen = false;
    const onVisibility = () => {
      hidden = document.visibilityState === 'hidden';
      setUnseen(hidden || offscreen);
    };
    document.addEventListener('visibilitychange', onVisibility);
    let observer: IntersectionObserver | null = null;
    const host = hostRef.current;
    if (host && typeof IntersectionObserver === 'function') {
      observer = new IntersectionObserver((entries) => {
        const entry = entries[0];
        if (!entry) return;
        offscreen = !entry.isIntersecting;
        setUnseen(hidden || offscreen);
      });
      observer.observe(host);
    }
    return () => {
      document.removeEventListener('visibilitychange', onVisibility);
      observer?.disconnect();
    };
  }, []);

  useEffect(() => {
    if (!followLive || unseen || viewMode !== 'canvas' || held || viewport.width === 0 || viewport.height === 0 || !liveTarget) return;
    if (drag.current || cardDrag.current || sizeDrag.current || minimapDrag.current) return;
    const cx = liveTarget.position.x + liveTarget.size.width / 2;
    const cy = liveTarget.position.y + liveTarget.size.height / 2;
    const cur = cameraRef.current;
    const sx = cx * cur.zoom + cur.x;
    const sy = cy * cur.zoom + cur.y;
    const insideBand = sx >= viewport.width * 0.16
      && sx <= viewport.width * 0.84
      && sy >= viewport.height * 0.16
      && sy <= viewport.height * 0.80;
    if (insideBand) return;
    const nextX = Math.round(viewport.width / 2 - cx * cur.zoom);
    const nextY = Math.round(viewport.height / 2 - cy * cur.zoom);
    if (Math.hypot(nextX - cur.x, nextY - cur.y) <= 4) return;
    const rafId = requestAnimationFrame(() => {
      commitCameraNext({ x: nextX, y: nextY, zoom: cameraRef.current.zoom });
    });
    return () => cancelAnimationFrame(rafId);
  }, [followLive, unseen, viewMode, held, viewport, liveTarget, camera, commitCameraNext]);

  // Native, non-passive wheel listener: React's delegated wheel handler is
  // passive, so preventDefault there would be ignored (and log a warning).
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      holdCanvas();
      setFollowLive(false);
      const rect = host.getBoundingClientRect();
      const px = event.clientX - rect.left;
      const py = event.clientY - rect.top;
      const factor = Math.exp(-event.deltaY * 0.0015);
      const current = cameraRef.current;
      const zoom = Math.min(2.5, Math.max(0.25, current.zoom * factor));
      applyCamera({
        x: px - ((px - current.x) / current.zoom) * zoom,
        y: py - ((py - current.y) / current.zoom) * zoom,
        zoom,
      });
      if (zoomCommit.current) clearTimeout(zoomCommit.current);
      zoomCommit.current = setTimeout(commitCamera, 120);
    };
    host.addEventListener('wheel', onWheel, { passive: false });
    return () => {
      host.removeEventListener('wheel', onWheel);
      if (zoomCommit.current) clearTimeout(zoomCommit.current);
    };
  }, [applyCamera, commitCamera, holdCanvas]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const isEditable = (target: EventTarget | null) => {
      const el = target as HTMLElement | null;
      return Boolean(el && (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName) || el.isContentEditable));
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.code === 'Space' && !event.repeat && !isEditable(event.target)) {
        spaceHeldRef.current = true;
        setSpaceHeld(true);
      }
    };
    const onKeyUp = (event: KeyboardEvent) => {
      if (event.code === 'Space') {
        spaceHeldRef.current = false;
        setSpaceHeld(false);
      }
    };
    const onBlur = () => {
      spaceHeldRef.current = false;
      setSpaceHeld(false);
    };
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('keyup', onKeyUp);
    window.addEventListener('blur', onBlur);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('keyup', onKeyUp);
      window.removeEventListener('blur', onBlur);
    };
  }, []);

  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    const onBackground = event.target === event.currentTarget || event.target === worldRef.current;
    if (!spaceHeldRef.current && !onBackground) return;
    if (event.shiftKey && !spaceHeldRef.current && onBackground) {
      const rect = event.currentTarget.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;
      marqueeDrag.current = { startX: x, startY: y, curX: x, curY: y, additive: Boolean(event.metaKey || event.ctrlKey) };
      setMarqueeBox({ left: x, top: y, width: 0, height: 0 });
      event.currentTarget.setPointerCapture?.(event.pointerId);
      return;
    }
    drag.current = { x: event.clientX, y: event.clientY, camera: cameraRef.current, moved: false };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };

  const onPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (marqueeDrag.current) {
      const rect = event.currentTarget.getBoundingClientRect();
      const curX = event.clientX - rect.left;
      const curY = event.clientY - rect.top;
      marqueeDrag.current.curX = curX;
      marqueeDrag.current.curY = curY;
      setMarqueeBox({
        left: Math.min(marqueeDrag.current.startX, curX),
        top: Math.min(marqueeDrag.current.startY, curY),
        width: Math.abs(curX - marqueeDrag.current.startX),
        height: Math.abs(curY - marqueeDrag.current.startY),
      });
      return;
    }
    if (!drag.current) return;
    const dx = event.clientX - drag.current.x;
    const dy = event.clientY - drag.current.y;
    if (Math.hypot(dx, dy) > 2) {
      drag.current.moved = true;
      setFollowLive(false);
    }
    holdCanvas();
    applyCamera({ ...drag.current.camera, x: drag.current.camera.x + dx, y: drag.current.camera.y + dy });
  };

  const onPointerUp = (event?: React.PointerEvent<HTMLDivElement>) => {
    if (marqueeDrag.current) {
      const m = marqueeDrag.current;
      marqueeDrag.current = null;
      setMarqueeBox(null);
      const minX = (Math.min(m.startX, m.curX) - cameraRef.current.x) / cameraRef.current.zoom;
      const minY = (Math.min(m.startY, m.curY) - cameraRef.current.y) / cameraRef.current.zoom;
      const maxX = (Math.max(m.startX, m.curX) - cameraRef.current.x) / cameraRef.current.zoom;
      const maxY = (Math.max(m.startY, m.curY) - cameraRef.current.y) / cameraRef.current.zoom;
      const box: WorkbenchRect = { x: minX, y: minY, width: Math.max(2, maxX - minX), height: Math.max(2, maxY - minY) };
      const hits = visibleObjects
        .filter((object) => rectsOverlap(box, { ...object.position, ...object.size }))
        .map((object) => object.id);
      setSelected((previous) => (m.additive ? Array.from(new Set([...previous, ...hits])) : hits));
      return;
    }
    if (!drag.current) return;
    const wasMoved = drag.current.moved;
    drag.current = null;
    if (!wasMoved && event && (event.target === event.currentTarget || event.target === worldRef.current) && !event.shiftKey) {
      setSelected([]);
      onSelectObject?.(null);
    } else if (wasMoved) {
      commitCamera();
    }
  };

  const onCardPointerDown = (event: React.PointerEvent<HTMLElement>, object: WorkbenchObject) => {
    if (spaceHeldRef.current) {
      drag.current = { x: event.clientX, y: event.clientY, camera: cameraRef.current, moved: false };
      hostRef.current?.setPointerCapture?.(event.pointerId);
      return;
    }
    event.stopPropagation();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const group = selected.includes(object.id) ? selected : [object.id];
    if (!selected.includes(object.id)) selectObject(object, false);
    cardDrag.current = {
      ids: group,
      x: event.clientX,
      y: event.clientY,
      moved: false,
      snapshot: captureSnapshot(),
      origins: Object.fromEntries(group.map((id) => {
        const item = objects.find((candidate) => candidate.id === id) ?? object;
        return [id, { x: item.position.x, y: item.position.y }];
      })),
    };
  };

  const onCardPointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const current = cardDrag.current;
    if (!current) return;
    if (!current.moved && !boardPin(event.clientX - current.x, event.clientY - current.y)) return;
    holdCanvas();
    if (!current.moved) {
      current.moved = true;
      pushUndo('move', 'Undo move', 'Undo card move (Z)', current.snapshot);
      setDraggingIds(new Set(current.ids));
      setPins((previous) => {
        const next = { ...previous };
        for (const id of current.ids) next[id] = true;
        return next;
      });
    }
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    const minX = worldEnvelope.minX;
    const maxX = Math.max(worldEnvelope.maxX, 2400);
    const minY = worldEnvelope.minY;
    const maxY = Math.max(worldEnvelope.maxY, 2000);
    setPositions((previous) => {
      const next = { ...previous };
      for (const id of current.ids) {
        const origin = current.origins[id];
        if (!origin) continue;
        next[id] = {
          x: Math.max(minX, Math.min(maxX, Math.round((origin.x + dx) * 100) / 100)),
          y: Math.max(minY, Math.min(maxY, Math.round((origin.y + dy) * 100) / 100)),
        };
      }
      return next;
    });
  };

  const onCardPointerUp = () => {
    const activeDrag = cardDrag.current;
    cardDrag.current = null;
    if (activeDrag?.moved || sizeDrag.current) setDraggingIds(new Set());
    if (activeDrag?.moved) {
      const movedSet = new Set(activeDrag.ids);
      const stationary = visibleObjects
        .filter((item) => !movedSet.has(item.id))
        .map((item) => ({ x: item.position.x, y: item.position.y, width: item.size.width, height: item.size.height }));
      setPositions((previous) => {
        const next = { ...previous };
        const placedObstacles = [...stationary];
        for (const id of activeDrag.ids) {
          const item = visibleObjects.find((candidate) => candidate.id === id);
          if (!item) continue;
          const pos = next[id] ?? item.position;
          const settled = dropSpot(
            { x: pos.x, y: pos.y, width: item.size.width, height: item.size.height },
            placedObstacles,
            WORKBENCH_CLEARANCE,
            { minX: worldEnvelope.minX, maxX: Math.max(worldEnvelope.maxX, 2400), minY: worldEnvelope.minY, maxY: Math.max(worldEnvelope.maxY, 2000) },
          );
          next[id] = settled;
          placedObstacles.push({ ...settled, width: item.size.width, height: item.size.height });
        }
        return next;
      });
      setPins((previous) => {
        const next = { ...previous };
        for (const id of activeDrag.ids) next[id] = true;
        return next;
      });
    }
    if (sizeDrag.current) {
      const resizedId = sizeDrag.current.id;
      sizeDrag.current = null;
      setSizes((previous) => {
        const size = previous[resizedId];
        if (!size) return previous;
        return { ...previous, [resizedId]: { width: snapToGrid(size.width), height: snapToGrid(size.height) } };
      });
      setPins((previous) => ({ ...previous, [resizedId]: true }));
    }
  };

  const onResizePointerDown = (event: React.PointerEvent<HTMLElement>, object: WorkbenchObject) => {
    event.stopPropagation();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const snapshot = captureSnapshot();
    pushUndo('resize', 'Undo resize', 'Undo card resize (Z)', snapshot);
    setDraggingIds(new Set([object.id]));
    sizeDrag.current = {
      id: object.id,
      x: event.clientX,
      y: event.clientY,
      width: object.size.width,
      height: object.size.height,
      snapshot,
    };
  };

  const onResizePointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const current = sizeDrag.current;
    if (!current) return;
    holdCanvas();
    const width = Math.max(220, Math.min(900, current.width + (event.clientX - current.x) / camera.zoom));
    const height = Math.max(140, Math.min(900, current.height + (event.clientY - current.y) / camera.zoom));
    setSizes((previous) => ({
      ...previous,
      [current.id]: { width: Math.round(width * 100) / 100, height: Math.round(height * 100) / 100 },
    }));
  };

  const onAreaPointerDown = (event: React.PointerEvent<HTMLDivElement>, area: WorkbenchArea) => {
    if ((event.target as HTMLElement).dataset.resize === 'area') return;
    if ((event.target as HTMLElement).closest('button, input')) return;
    event.stopPropagation();
    setActiveAreaId(area.id);
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const childOrigins: Record<string, { x: number; y: number }> = {};
    for (const obj of visibleObjects) {
      if (assignments[obj.id] === area.id) {
        childOrigins[obj.id] = { x: obj.position.x, y: obj.position.y };
      }
    }
    areaDrag.current = { id: area.id, x: event.clientX, y: event.clientY, originX: area.rect.x, originY: area.rect.y, childOrigins };
  };

  const onAreaPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const current = areaDrag.current;
    if (!current) return;
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    setAreas((previous) => previous.map((area) => area.id === current.id
      ? { ...area, rect: { ...area.rect, x: current.originX + dx, y: current.originY + dy } }
      : area));
    const childIds = Object.keys(current.childOrigins);
    if (childIds.length > 0) {
      setPins((previous) => {
        const next = { ...previous };
        for (const id of childIds) next[id] = true;
        return next;
      });
      setPositions((previous) => {
        const next = { ...previous };
        for (const id of childIds) {
          const origin = current.childOrigins[id];
          if (origin) next[id] = { x: Math.round((origin.x + dx) * 100) / 100, y: Math.round((origin.y + dy) * 100) / 100 };
        }
        return next;
      });
    }
  };

  const onAreaResizePointerDown = (event: React.PointerEvent<HTMLSpanElement>, area: WorkbenchArea) => {
    event.stopPropagation();
    setActiveAreaId(area.id);
    event.currentTarget.setPointerCapture?.(event.pointerId);
    areaResize.current = { id: area.id, x: event.clientX, y: event.clientY, width: area.rect.width, height: area.rect.height };
  };

  const onAreaResizePointerMove = (event: React.PointerEvent<HTMLSpanElement>) => {
    const current = areaResize.current;
    if (!current) return;
    const width = Math.max(320, current.width + (event.clientX - current.x) / camera.zoom);
    const height = Math.max(220, current.height + (event.clientY - current.y) / camera.zoom);
    setAreas((previous) => previous.map((area) => area.id === current.id
      ? { ...area, rect: { ...area.rect, width, height } }
      : area));
  };

  const createArea = useCallback((title: string, rect: { x: number; y: number; width: number; height: number }, memberIds: string[] = []) => {
    pushUndo('area', 'Undo area', 'Undo area change (Z)', captureSnapshot());
    const id = newWriteRequestId();
    setAreas((previous) => [...previous, { id, title, rect }]);
    if (memberIds.length > 0) {
      setAssignments((previous) => {
        const next = { ...previous };
        for (const memberId of memberIds) next[memberId] = id;
        return next;
      });
    }
    setActiveAreaId(id);
  }, [pushUndo, captureSnapshot]);

  const deleteArea = useCallback((id: string) => {
    pushUndo('area', 'Undo area', 'Undo area removal (Z)', captureSnapshot());
    setAreas((previous) => previous.filter((area) => area.id !== id));
    setAssignments((previous) => Object.fromEntries(Object.entries(previous).filter(([, areaId]) => areaId !== id)));
    setActiveAreaId(areas.find((area) => area.id !== id)?.id ?? null);
    setRenamingAreaId((curr) => (curr === id ? null : curr));
  }, [areas, pushUndo, captureSnapshot]);

  const groupSelected = useCallback(() => {
    const items = visibleObjects.filter((object) => selected.includes(object.id));
    if (items.length === 0) return;
    const minX = Math.min(...items.map((object) => object.position.x)) - 24;
    const minY = Math.min(...items.map((object) => object.position.y)) - 48;
    const maxX = Math.max(...items.map((object) => object.position.x + object.size.width)) + 24;
    const maxY = Math.max(...items.map((object) => object.position.y + object.size.height)) + 24;
    createArea(`Area ${areas.length + 1}`, { x: minX, y: minY, width: maxX - minX, height: maxY - minY }, items.map((item) => item.id));
  }, [visibleObjects, selected, areas.length, createArea]);

  const selectedObjects = useMemo(
    () => visibleObjects.filter((object) => selected.includes(object.id)),
    [visibleObjects, selected],
  );

  const applySelectionTransform = useCallback((nextPositions: WorkbenchPositions) => {
    const ids = Object.keys(nextPositions);
    if (ids.length === 0) return;
    pushUndo('arrange', 'Undo arrange', 'Undo selection layout (Z)', captureSnapshot());
    setPins((previous) => {
      const next = { ...previous };
      for (const id of ids) next[id] = true;
      return next;
    });
    setPositions((previous) => ({ ...previous, ...nextPositions }));
  }, [pushUndo, captureSnapshot]);

  const focusTargets = useMemo(
    () => (hoveredId && !selected.includes(hoveredId) ? [...selected, hoveredId] : selected),
    [selected, hoveredId],
  );

  const focusedLinkIds = useMemo(
    () => new Set(selectFocusedLinks(links, focusTargets).map((link) => link.id)),
    [links, focusTargets],
  );

  const visibleLinks = useMemo(() => {
    if (edgeVisibility === 'off') return [];
    if (edgeVisibility === 'all') {
      return links
        .filter((link) => visibleObjectMap.has(link.source) && visibleObjectMap.has(link.target))
        .slice(0, 48);
    }
    const focused = selectFocusedLinks(links, focusTargets);
    if (edgeVisibility === 'focus') return focused;
    const seen = new Set<string>();
    const combined: WorkbenchLink[] = [];
    for (const link of [...selectSpineChainLinks(links), ...focused]) {
      if (seen.has(link.id)) continue;
      if (!visibleObjectMap.has(link.source) || !visibleObjectMap.has(link.target)) continue;
      seen.add(link.id);
      combined.push(link);
    }
    return combined;
  }, [edgeVisibility, links, focusTargets, visibleObjectMap]);

  const routedLinks = useMemo(
    () => routeOrthogonalBusEdges(visibleLinks, visibleObjectMap, draggingIds),
    [visibleLinks, visibleObjectMap, draggingIds],
  );

  const topUndo = undoStack.length > 0 ? undoStack[undoStack.length - 1] : null;

  const minimapBounds = useMemo(() => {
    const b = layoutBounds(filteredObjects);
    return {
      minX: b.x - 40,
      minY: b.y - 40,
      spanW: Math.max(480, b.width + 80),
      spanH: Math.max(320, b.height + 80),
    };
  }, [filteredObjects]);

  const panFromMinimap = useCallback((clientX: number, clientY: number, el: HTMLCanvasElement) => {
    const rect = el.getBoundingClientRect();
    const rectW = rect.width || 176;
    const rectH = rect.height || 118;
    const { minX, minY, spanW, spanH } = minimapBounds;
    const scale = Math.min(rectW / spanW, rectH / spanH);
    const offX = (rectW - spanW * scale) / 2;
    const offY = (rectH - spanH * scale) / 2;
    const wx = minX + (clientX - rect.left - offX) / scale;
    const wy = minY + (clientY - rect.top - offY) / scale;
    const vw = viewport.width || 1280;
    const vh = viewport.height || 720;
    const cur = cameraRef.current;
    holdCanvas();
    setFollowLive(false);
    commitCameraNext({
      x: Math.round(vw / 2 - wx * cur.zoom),
      y: Math.round(vh / 2 - wy * cur.zoom),
      zoom: cur.zoom,
    });
  }, [minimapBounds, viewport.width, viewport.height, holdCanvas, commitCameraNext]);

  useEffect(() => {
    if (viewMode !== 'canvas' || filteredObjects.length === 0) return;
    const canvas = minimapCanvasRef.current;
    if (!canvas || typeof canvas.getContext !== 'function') return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const w = 176;
    const h = 118;
    const dpr = typeof window !== 'undefined' ? Math.min(2, window.devicePixelRatio || 1) : 1;
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const styles = typeof getComputedStyle === 'function' ? getComputedStyle(canvas) : null;
    const tok = (name: string, fallback: string) => styles?.getPropertyValue(name).trim() || fallback;
    const infoColor = tok('--color-info', '#8fbcf7');
    const successColor = tok('--color-success', '#7fcaa1');
    const borderStrong = tok('--color-border-strong', 'rgba(255,255,255,0.18)');
    const textColor = tok('--color-text', '#f4f4f6');
    const neutralFill = tok('--color-control', 'rgba(255,255,255,0.12)');

    const { minX, minY, spanW, spanH } = minimapBounds;
    const scale = Math.min(w / spanW, h / spanH);
    const offX = (w - spanW * scale) / 2;
    const offY = (h - spanH * scale) / 2;
    const selectedSet = new Set(selected);

    for (const object of filteredObjects) {
      const rx = offX + (object.position.x - minX) * scale;
      const ry = offY + (object.position.y - minY) * scale;
      const rw = Math.max(2, object.size.width * scale);
      const rh = Math.max(2, object.size.height * scale);
      const isSelected = selectedSet.has(object.id);
      ctx.globalAlpha = object.provenance === 'measured' || object.provenance === 'proposed' ? 0.36 : 0.72;
      ctx.fillStyle = object.provenance === 'measured'
        ? successColor
        : object.provenance === 'proposed'
          ? infoColor
          : neutralFill;
      ctx.fillRect(rx, ry, rw, rh);
      ctx.globalAlpha = 1;
      ctx.lineWidth = isSelected ? 1.5 : 0.75;
      ctx.strokeStyle = isSelected
        ? textColor
        : object.provenance === 'measured'
          ? successColor
          : object.provenance === 'proposed'
            ? infoColor
            : borderStrong;
      ctx.strokeRect(rx, ry, rw, rh);
    }

    const vx = offX + (-camera.x / camera.zoom - minX) * scale;
    const vy = offY + (-camera.y / camera.zoom - minY) * scale;
    const vw = ((viewport.width || 1280) / camera.zoom) * scale;
    const vh = ((viewport.height || 720) / camera.zoom) * scale;
    ctx.globalAlpha = 0.14;
    ctx.fillStyle = infoColor;
    ctx.fillRect(vx, vy, vw, vh);
    ctx.globalAlpha = 1;
    ctx.lineWidth = 1.25;
    ctx.strokeStyle = infoColor;
    ctx.strokeRect(vx, vy, vw, vh);
  }, [viewMode, filteredObjects, selected, camera, viewport.width, viewport.height, minimapBounds]);

  const selectionBarStyle = useMemo(() => {
    if (viewMode !== 'canvas' || selectedObjects.length === 0) return null;
    const vw = viewport.width || 1280;
    const vh = viewport.height || 720;
    const minX = Math.min(...selectedObjects.map((o) => o.position.x)) * camera.zoom + camera.x;
    const maxX = Math.max(...selectedObjects.map((o) => o.position.x + o.size.width)) * camera.zoom + camera.x;
    const minY = Math.min(...selectedObjects.map((o) => o.position.y)) * camera.zoom + camera.y;
    const centerX = Math.max(180, Math.min(vw - 180, (minX + maxX) / 2));
    const topY = Math.max(12, Math.min(Math.max(12, vh - 140), minY - 48));
    return { left: centerX, top: topY };
  }, [viewMode, selectedObjects, viewport, camera]);

  return (
    <section
      className="k-workbench"
      data-testid="workbench"
      data-object-count={objects.length}
      data-loop-count={loops.length}
      data-job-count={jobs.length}
      data-obligation-count={obligations.length}
      aria-label="Campaign workbench"
      tabIndex={0}
      onKeyDown={(event) => {
        const target = event.target as HTMLElement;
        const editing = Boolean(target && (/^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName) || target.isContentEditable));
        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z' && !editing) {
          event.preventDefault();
          undoLast();
          return;
        }
        if (event.metaKey || event.ctrlKey || event.altKey) return;
        if (event.key.toLowerCase() === 'z' && !editing && undoStack.length > 0) {
          event.preventDefault();
          undoLast();
        }
        if (event.key === '0') {
          event.preventDefault();
          fit();
        }
        if (event.key === '1') {
          event.preventDefault();
          holdCanvas();
          commitCameraNext({ ...cameraRef.current, zoom: 1 });
        }
        if ((event.key === 'Delete' || event.key === 'Backspace') && selected.length > 0 && !editing) {
          event.preventDefault();
          removeSelected();
        }
      }}
    >
      <div className="k-workbench-bar" aria-label="Workbench controls">
        <Button type="button" variant="ghost" size="compact" onClick={fit} title="Fit the whole workbench (0)">
          <Maximize2 size={14} /> Fit
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          aria-label="Zoom out"
          onClick={() => {
            holdCanvas();
            const current = cameraRef.current;
            commitCameraNext({ ...current, zoom: Math.max(0.25, current.zoom / 1.2) });
          }}
          title="Zoom out"
        >
          <Minus size={14} />
        </Button>
        <span className="k-workbench-zoom">{Math.round(camera.zoom * 100)}%</span>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          aria-label="Zoom in"
          onClick={() => {
            holdCanvas();
            const current = cameraRef.current;
            commitCameraNext({ ...current, zoom: Math.min(2, current.zoom * 1.2) });
          }}
          title="Zoom in"
        >
          <Plus size={14} />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          aria-pressed={viewMode === 'list'}
          onClick={() => setViewMode((current) => (current === 'canvas' ? 'list' : 'canvas'))}
          title="Switch between canvas and list (text equivalent)"
        >
          {viewMode === 'canvas' ? <List size={14} /> : <LayoutGrid size={14} />}
          {viewMode === 'canvas' ? 'List' : 'Canvas'}
        </Button>
        <div className="k-workbench-filters" role="group" aria-label="Filter workbench objects">
          {([
            ['all', 'All', filterCounts.all],
            ['spine', 'Spine', filterCounts.spine],
            ['jobs', 'Jobs', filterCounts.jobs],
            ['issues', 'Issues', filterCounts.issues],
          ] as const).map(([key, label, count]) => (
            <button
              key={key}
              type="button"
              className="k-workbench-filter-chip"
              data-filter={key}
              aria-pressed={filter === key}
              onClick={() => setFilter(key)}
            >
              {`${label} ${count}`}
            </button>
          ))}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          disabled={viewMode === 'list' || !liveTarget}
          aria-pressed={followLive}
          data-testid="workbench-follow-live"
          onClick={() => setFollowLive((curr) => !curr)}
          title="Follow active job or loop while canvas is idle"
        >
          Follow live
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          disabled={viewMode === 'list'}
          onClick={() => setEdgeVisibility((curr) => (curr === 'hybrid' ? 'all' : curr === 'all' ? 'off' : curr === 'off' ? 'focus' : 'hybrid'))}
          title="Cycle edge visibility: Hybrid (spine + focused), All, Off, or Focus (hover/select only)"
        >
          Edges: {edgeVisibility}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          disabled={!areaBounds || viewMode === 'list'}
          onClick={() => createArea(`Area ${areas.length + 1}`, areaBounds!)}
          title="Group all objects into a spatial area"
        >
          Group all
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          disabled={selected.length === 0 || viewMode === 'list'}
          onClick={groupSelected}
          title="Group the selected cards into a spatial area"
        >
          Group selected
        </Button>
        {selected.length === 2 && (
          <span className="flex items-center gap-1">
            <select
              aria-label="Link kind"
              value={linkKind}
              onChange={(event) => setLinkKind(event.target.value as WorkbenchLink['kind'])}
              className="h-7 rounded-lg border border-[var(--color-border)] bg-[var(--color-field)] px-1 text-[12px] text-[var(--color-label-secondary)]"
            >
              <option value="supports">supports</option>
              <option value="uses">uses</option>
              <option value="depends_on">depends on</option>
              <option value="contradicts">contradicts</option>
              <option value="same_as">same as</option>
              <option value="thread">thread</option>
            </select>
            <Button type="button" variant="ghost" size="compact" onClick={customLink} title="Link the two selected cards">Link</Button>
          </span>
        )}
        {activeArea && (
          <Button
            type="button"
            variant="ghost"
            size="compact"
            onClick={() => deleteArea(activeArea.id)}
            title="Delete the active area; cards stay on the canvas"
          >
            Delete area
          </Button>
        )}
        {activeArea && (
          <input
            className="k-workbench-area-title"
            aria-label="Area title"
            value={activeArea.title}
            onChange={(event) => setAreas((previous) => previous.map((area) => area.id === activeArea.id ? { ...area, title: event.target.value } : area))}
          />
        )}
        {selected.length > 0 && activeArea && (
          <Button
            type="button"
            variant="ghost"
            size="compact"
            onClick={() => setAssignments((previous) => {
              const next = { ...previous };
              const areaId = activeArea.id;
              const allAssigned = selected.every((id) => previous[id] === areaId);
              for (const id of selected) {
                if (allAssigned) delete next[id];
                else next[id] = areaId;
              }
              return next;
            })}
          >
            {selected.every((id) => assignments[id] === activeArea.id) ? 'Unassign' : 'Assign to area'}
          </Button>
        )}
        {selected.length > 0 && (
          <Button type="button" variant="ghost" size="compact" onClick={removeSelected} title="Remove the selected cards from the canvas; records are not deleted">
            {selected.length > 1 ? `Remove (${selected.length})` : 'Remove'}
          </Button>
        )}
        {topUndo && (
          <Button type="button" variant="ghost" size="compact" onClick={undoLast} title={topUndo.title}>
            {topUndo.label}
          </Button>
        )}
        {conflictRevision !== null && (
          <Button type="button" size="compact" onClick={() => void reloadLatest()} title="Reload the latest server layout; local unsaved layout changes are discarded">
            Reload layout
          </Button>
        )}
        <span className="k-workbench-state" role="status" aria-live="polite">
          {writeSettlementLabel(write) || (sessionId ? 'view saved' : 'no campaign selected')}
        </span>
      </div>

      <div className="k-workbench-stage">
      <div
        ref={hostRef}
        className="k-workbench-viewport"
        data-lod={lod}
        data-zoom-tier={zoomTier}
        data-held={held ? 'true' : undefined}
        data-unseen={unseen ? 'true' : undefined}
        data-pan-mode={spaceHeld ? 'true' : undefined}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={() => onPointerUp()}
      >
      {filteredObjects.length === 0 ? (
        <EmptyState
          title={
            error && objects.length === 0
              ? 'Campaign unavailable'
              : objects.length === 0
                ? 'No campaign on the workbench'
                : visibleObjects.length > 0 && filter !== 'all'
                  ? 'No matching workbench objects'
                  : 'All workbench objects are hidden'
          }
          className="k-workbench-empty"
        >
          {error && objects.length === 0 ? (
            <>
              <span>{authError ? 'Sign in to open this campaign.' : error}</span>
              {authError && onSignIn ? (
                <Button type="button" size="compact" data-testid="workbench-sign-in" onClick={onSignIn}>Sign in</Button>
              ) : onRetry ? (
                <Button type="button" size="compact" data-testid="retry-campaign" onClick={onRetry}>Retry campaign read</Button>
              ) : null}
            </>
          ) : objects.length === 0 ? (
            <div className="space-y-2">
              <p>{localMode ? 'Local private mode — no sign-in is required to start.' : multiLab ? 'Multi-lab workspace — sign in to create or open a campaign.' : 'Create or open a campaign to give the science a place to stand.'}</p>
              <div className="flex flex-wrap justify-center gap-1.5 text-[12px] font-mono">
                <span className={`px-2 py-0.5 rounded-full border ${access ? 'border-[var(--color-success-edge)] bg-[var(--color-success-wash)] text-[var(--color-success)]' : 'border-[var(--color-border)] bg-[var(--color-fill)] text-[var(--color-muted)]'}`}>
                  {access ? 'backend connected' : 'checking backend'}
                </span>
                <span className={`px-2 py-0.5 rounded-full border ${access?.llm_local ? 'border-[var(--color-accent-edge)] bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]' : 'border-[var(--color-warning-edge)] bg-[var(--color-warning-wash)] text-[var(--color-warning)]'}`}>
                  {access ? (access.llm_local ? 'local LLM' : 'cloud LLM — sign-in required') : 'checking LLM'}
                </span>
                <span className="px-2 py-0.5 rounded-full border border-[var(--color-border)] bg-[var(--color-fill)] text-[var(--color-muted)]">
                  {access?.role === 'lab_admin' ? 'lab admin' : access?.role === 'operator' ? 'operator' : 'researcher'}
                </span>
                <span className="px-2 py-0.5 rounded-full border border-[var(--color-border)] bg-[var(--color-fill)] text-[var(--color-muted)]">no campaign yet</span>
              </div>
              {multiLab && !sessionId && onSignIn ? (
                <Button type="button" size="compact" data-testid="workbench-sign-in" onClick={onSignIn}>Sign in</Button>
              ) : onCreateCampaign ? (
                <Button type="button" size="compact" onClick={onCreateCampaign} data-testid="workbench-create-campaign">
                  Create your first campaign
                </Button>
              ) : null}
            </div>
          ) : visibleObjects.length > 0 && filter !== 'all' ? (
            <div className="space-y-2">
              <p>Filter &ldquo;{filter}&rdquo; matched 0 of {visibleObjects.length} cards.</p>
              <Button type="button" size="compact" onClick={() => setFilter('all')}>Show all cards</Button>
            </div>
          ) : 'Use Undo remove or the list view to restore an object.'}
        </EmptyState>
      ) : (
        viewMode === 'list' ? (
          <div className="k-workbench-list">
            <table>
              <caption>Campaign workbench objects — text equivalent of the canvas</caption>
              <thead><tr><th scope="col">Kind</th><th scope="col">Object</th><th scope="col">Recorded facts</th><th scope="col">Open</th></tr></thead>
              <tbody>
                {filteredObjects.map((object, index) => (
                  <tr key={object.id} data-provenance={object.provenance ?? 'catalog'} className={index >= 18 ? 'k-deferred-row' : undefined}>
                    <th scope="row">
                      <div className="flex items-center gap-1.5">
                        <span>{object.label}</span>
                        <ProvenanceBadge kind={object.provenance ?? 'catalog'} />
                      </div>
                    </th>
                    <td>
                      <strong>{object.title}</strong>
                      <span>{object.detail}</span>
                      {object.basis && <span>{object.basis}</span>}
                      {object.sources?.length ? (
                        <span>Sources: {object.sources.map((source) => source.label).join(' · ')}</span>
                      ) : null}
                    </td>
                    <td data-metric>{object.facts.map((fact) => `${fact.label}: ${fact.value}`).join(' · ') || 'No facts recorded'}</td>
                    <td>{onLiftObject ? <Button type="button" size="compact" variant="ghost" onClick={() => onLiftObject(object)}>Open</Button> : onOpenObject ? <Button type="button" size="compact" variant="ghost" onClick={() => onOpenObject(object.kind)}>Open</Button> : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {links.length > 0 && (
              <div className="k-workbench-hidden" aria-label="Workbench links">
                <strong>Links</strong>
                {links.map((link) => {
                  const source = objects.find((object) => object.id === link.source);
                  const target = objects.find((object) => object.id === link.target);
                  return (
                    <button key={link.id} type="button" onClick={() => removeLink(link.id)} title="Remove this link; the cards and records are not deleted">
                      {source?.label ?? link.source} → {link.kind} → {target?.label ?? link.target} · Remove
                    </button>
                  );
                })}
              </div>
            )}
            {hidden.length > 0 && (
              <div className="k-workbench-hidden" aria-label="Hidden objects">
                <strong>Hidden objects</strong>
                {hidden.map((id) => {
                  const object = objects.find((item) => item.id === id);
                  return object ? (
                    <button key={id} type="button" onClick={() => setHidden((previous) => previous.filter((item) => item !== id))}>
                      Restore {object.label}: {object.title}
                    </button>
                  ) : null;
                })}
              </div>
            )}
          </div>
        ) : (
        <div
          ref={worldRef}
          className="k-workbench-world"
          style={{
            width: worldEnvelope.width,
            height: worldEnvelope.height,
            transform: `translate3d(${camera.x}px, ${camera.y}px, 0) scale(${camera.zoom})`,
          }}
        >
          {areas.map((area) => {
            const count = areaChildCounts[area.id] ?? 0;
            const isRenaming = renamingAreaId === area.id;
            return (
              <div
                key={area.id}
                className="k-workbench-area"
                data-selected={activeArea?.id === area.id ? 'true' : undefined}
                style={{ left: area.rect.x, top: area.rect.y, width: area.rect.width, height: area.rect.height }}
                onPointerDown={(event) => onAreaPointerDown(event, area)}
                onPointerMove={onAreaPointerMove}
                onPointerUp={() => { areaDrag.current = null; }}
                onPointerCancel={() => { areaDrag.current = null; }}
              >
                <span>{area.title}</span>
                <div className="k-workbench-area-chrome">
                  <span className="k-workbench-area-count" aria-label={`${count} cards`}>{count}</span>
                  {isRenaming ? (
                    <input
                      className="k-workbench-area-inline-input"
                      aria-label={`Rename ${area.title}`}
                      value={area.title}
                      autoFocus
                      onPointerDown={(event) => event.stopPropagation()}
                      onChange={(event) => setAreas((previous) => previous.map((item) => item.id === area.id ? { ...item, title: event.target.value } : item))}
                      onBlur={() => setRenamingAreaId(null)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter' || event.key === 'Escape') {
                          event.preventDefault();
                          setRenamingAreaId(null);
                        }
                      }}
                    />
                  ) : null}
                  <button
                    type="button"
                    title="Zoom to fit this area"
                    onPointerDown={(event) => event.stopPropagation()}
                    onClick={(event) => { event.stopPropagation(); fitRect(area.rect); }}
                  >
                    Fit
                  </button>
                  <button
                    type="button"
                    title="Rename this area"
                    onPointerDown={(event) => event.stopPropagation()}
                    onClick={(event) => { event.stopPropagation(); setActiveAreaId(area.id); setRenamingAreaId((curr) => (curr === area.id ? null : area.id)); }}
                  >
                    Rename
                  </button>
                  <button
                    type="button"
                    title="Remove this area; cards stay on the canvas"
                    onPointerDown={(event) => event.stopPropagation()}
                    onClick={(event) => { event.stopPropagation(); deleteArea(area.id); }}
                  >
                    Remove
                  </button>
                </div>
                <span
                  className="k-workbench-area-resize"
                  data-resize="area"
                  aria-hidden="true"
                  onPointerDown={(event) => onAreaResizePointerDown(event, area)}
                  onPointerMove={onAreaResizePointerMove}
                  onPointerUp={() => { areaResize.current = null; }}
                  onPointerCancel={() => { areaResize.current = null; }}
                />
              </div>
            );
          })}
          <svg className="k-workbench-links" width={worldEnvelope.width} height={worldEnvelope.height} aria-hidden="true">
            {routedLinks.map(({ link, d, activeFlow }) => (
              <path
                key={link.id}
                data-kind={link.kind}
                data-edge-role={edgeVisibility === 'focus' || focusedLinkIds.has(link.id) ? 'focus' : 'rest'}
                data-flow={activeFlow ? 'live' : undefined}
                d={d}
              />
            ))}
          </svg>
          {filteredObjects.map((object) => {
            if (viewport && visibleIds && !visibleIds.has(object.id)) return null;
            const assignedArea = assignments[object.id] ? areaById.get(assignments[object.id]) : undefined;
            const sessionGateKey = sessionId || 'default';
            const obId = object.kind === 'approval' && object.ref ? object.ref.id : '';
            return (
              <WorkCard
                key={object.id}
                object={object}
                explicitSize={Boolean(sizes[object.id])}
                overview={zoomTier === 'overview'}
                selected={selected.includes(object.id)}
                assignedAreaTitle={assignedArea?.title}
                cardOverrides={overrides[object.id]}
                onUpdateOverride={(cellKey, nextValue) => updateCardOverride(object.id, cellKey, nextValue)}
                onHoverChange={(hovering) => setHoveredId(hovering ? object.id : (curr) => (curr === object.id ? null : curr))}
                onMeasure={measureCard}
                onSelect={(additive) => selectObject(object, additive)}
                onPointerDown={(event) => onCardPointerDown(event, object)}
                onPointerMove={onCardPointerMove}
                onPointerUp={onCardPointerUp}
                onResizePointerDown={(event) => onResizePointerDown(event, object)}
                onResizePointerMove={onResizePointerMove}
                onResizePointerUp={onCardPointerUp}
                onOpen={onLiftObject ? () => onLiftObject(object) : onOpenObject ? () => onOpenObject(object.kind) : undefined}
                taskStatus={object.kind === 'task' && object.ref ? tasks.find((task) => task.id === object.ref!.id)?.status : undefined}
                approvalReady={object.kind === 'approval' && object.ref ? Boolean(obligations.find((item) => item.id === object.ref!.id)?.close_ready) : false}
                confirmGate={object.kind === 'circuit' ? confirmGate : null}
                gateReceipt={object.kind === 'circuit' ? gateReceipts[sessionGateKey] ?? null : null}
                onGateSettled={object.kind === 'circuit' ? (receipt) => {
                  setGateReceipts((prev) => ({ ...prev, [sessionGateKey]: receipt }));
                  onGateSettled?.(receipt);
                } : undefined}
                approvalReceipt={obId ? obligationReceipts[obId] ?? null : null}
                approvalError={obId ? obligationErrors[obId] ?? null : null}
                onCloseApproval={obId && onCloseObligation ? async () => {
                  const res = await onCloseObligation(obId);
                  if (res && !res.ok) {
                    setObligationErrors((prev) => ({ ...prev, [obId]: res.error || 'Obligation could not be closed' }));
                    return;
                  }
                  setObligationErrors((prev) => {
                    if (!(obId in prev)) return prev;
                    const next = { ...prev };
                    delete next[obId];
                    return next;
                  });
                  setObligationReceipts((prev) => ({
                    ...prev,
                    [obId]: {
                      outcome: 'closed',
                      label: 'Obligation closed',
                      detail: `${obId.slice(0, 8)} verified closed`,
                      at: new Date().toISOString(),
                    },
                  }));
                } : undefined}
                onTaskStatus={object.kind === 'task' && object.ref && onTaskStatus ? () => {
                  const task = tasks.find((item) => item.id === object.ref!.id);
                  if (task) onTaskStatus(task.id, task.status === 'done' ? 'open' : 'done');
                } : undefined}
                onNudge={(dx, dy) => {
                  pushUndo('move', 'Undo move', 'Undo card nudge (Z)', captureSnapshot());
                  setPins((previous) => ({ ...previous, [object.id]: true }));
                  setPositions((previous) => ({
                    ...previous,
                    [object.id]: {
                      x: (previous[object.id]?.x ?? object.position.x) + dx,
                      y: (previous[object.id]?.y ?? object.position.y) + dy,
                    },
                  }));
                }}
              />
            );
          })}
        </div>
        )
      )}
      {marqueeBox && (
        <div
          className="k-workbench-marquee"
          aria-hidden="true"
          style={{ left: marqueeBox.left, top: marqueeBox.top, width: marqueeBox.width, height: marqueeBox.height }}
        />
      )}
      {selectionBarStyle && (
        <div
          className="k-workbench-selection-bar"
          role="toolbar"
          aria-label="Selection actions"
          style={{ left: selectionBarStyle.left, top: selectionBarStyle.top }}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <span className="k-workbench-selection-count">{selectedObjects.length} selected</span>
          {selectedObjects.length >= 2 && (
            <>
              <Button type="button" variant="ghost" size="compact" onClick={() => applySelectionTransform(alignSelectedPositions(selectedObjects, 'left'))}>
                Align
              </Button>
              {selectedObjects.length >= 3 && (
                <Button type="button" variant="ghost" size="compact" onClick={() => applySelectionTransform(distributeSelectedPositions(selectedObjects))}>
                  Distribute
                </Button>
              )}
              <Button type="button" variant="ghost" size="compact" onClick={() => applySelectionTransform(arrangeSelectedPositions(selectedObjects))}>
                Arrange
              </Button>
            </>
          )}
          <Button type="button" variant="ghost" size="compact" onClick={groupSelected}>
            Group area
          </Button>
          {onAskSelection && (
            <Button type="button" size="compact" onClick={() => onAskSelection(selectedObjects)}>
              Ask Copilot
            </Button>
          )}
        </div>
      )}
      </div>
      {viewMode === 'canvas' && filteredObjects.length > 0 && (
        <canvas
          ref={minimapCanvasRef}
          className="k-workbench-minimap"
          data-testid="workbench-minimap"
          role="region"
          aria-label="Workbench minimap"
          width={176}
          height={118}
          onPointerDown={(event) => {
            event.stopPropagation();
            minimapDrag.current = true;
            event.currentTarget.setPointerCapture?.(event.pointerId);
            panFromMinimap(event.clientX, event.clientY, event.currentTarget);
          }}
          onPointerMove={(event) => {
            if (!minimapDrag.current) return;
            event.stopPropagation();
            panFromMinimap(event.clientX, event.clientY, event.currentTarget);
          }}
          onPointerUp={(event) => {
            minimapDrag.current = false;
            event.stopPropagation();
          }}
          onPointerCancel={() => { minimapDrag.current = false; }}
          onClick={(event) => event.stopPropagation()}
        />
      )}
      </div>
    </section>
  );
});

function WorkCard({
  object,
  explicitSize,
  overview = false,
  selected,
  assignedAreaTitle,
  cardOverrides,
  onUpdateOverride,
  onHoverChange,
  onMeasure,
  onSelect,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onResizePointerDown,
  onResizePointerMove,
  onResizePointerUp,
  onOpen,
  taskStatus,
  approvalReady = false,
  confirmGate = null,
  gateReceipt = null,
  onGateSettled,
  approvalReceipt = null,
  approvalError = null,
  onTaskStatus,
  onCloseApproval,
  onNudge,
}: {
  object: WorkbenchObject;
  explicitSize: boolean;
  overview?: boolean;
  selected: boolean;
  assignedAreaTitle?: string;
  cardOverrides?: Record<string, unknown>;
  onUpdateOverride?: (cellKey: string, nextValue: string | null) => void;
  onHoverChange: (hovering: boolean) => void;
  onMeasure: (id: string, width: number, height: number) => void;
  onSelect: (additive: boolean) => void;
  onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: () => void;
  onResizePointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onResizePointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onResizePointerUp: () => void;
  onOpen?: () => void;
  taskStatus?: string;
  approvalReady?: boolean;
  confirmGate?: ConfirmGateConfig | null;
  gateReceipt?: AskReceipt | null;
  onGateSettled?: (receipt: AskReceipt) => void;
  approvalReceipt?: AskReceipt | null;
  approvalError?: string | null;
  onTaskStatus?: () => void;
  onCloseApproval?: () => void;
  onNudge: (dx: number, dy: number) => void;
}) {
  const cardRef = useRef<HTMLElement>(null);
  const [factsExpanded, setFactsExpanded] = useState(false);

  useEffect(() => {
    const node = cardRef.current;
    if (!node || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver((entries) => {
      const rect = entries[0]?.contentRect;
      if (!rect) return;
      onMeasure(object.id, rect.width, Math.max(node.offsetHeight, node.scrollHeight, rect.height));
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [object.id, onMeasure]);

  const maxCollapsedFacts = 6;
  const visibleFacts = factsExpanded || object.facts.length <= maxCollapsedFacts
    ? object.facts
    : object.facts.slice(0, maxCollapsedFacts);
  const hiddenFactsCount = object.facts.length - visibleFacts.length;
  const contested = /contested|contradict|diverg/i.test(`${object.title} ${object.meta?.status ?? ''}`);

  return (
    <article
      ref={cardRef}
      className="k-workbench-card"
      data-kind={object.kind}
      data-provenance={object.provenance ?? 'catalog'}
      data-contested={contested ? 'true' : undefined}
      data-selected={selected ? 'true' : undefined}
      style={{
        left: object.position.x,
        top: object.position.y,
        width: object.size.width,
        ...(explicitSize ? { height: object.size.height } : { minHeight: object.size.height }),
      }}
      tabIndex={0}
      aria-label={`${object.label}: ${object.title}`}
      onPointerEnter={() => onHoverChange(true)}
      onPointerLeave={() => onHoverChange(false)}
      onClick={(event) => onSelect(Boolean(event?.metaKey || event?.ctrlKey || event?.shiftKey))}
      onKeyDown={(event) => {
        const step = event.shiftKey ? 50 : 10;
        const moves: Record<string, [number, number]> = {
          ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step],
        };
        const move = moves[event.key];
        if (move) {
          event.preventDefault();
          onNudge(move[0], move[1]);
        }
        if (event.key === ' ') {
          event.preventDefault();
          onSelect(event.metaKey || event.ctrlKey || event.shiftKey);
        }
        if (event.key === 'Enter' && onOpen) {
          event.preventDefault();
          onOpen();
        }
      }}
    >
      <header
        className="k-workbench-drag"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <Badge className="k-workbench-kind" tone={object.kind === 'chart' ? 'info' : 'neutral'}>{object.label}</Badge>
        {assignedAreaTitle !== undefined && <Badge tone="info">area · {assignedAreaTitle}</Badge>}
        <ProvenanceBadge kind={object.provenance ?? 'catalog'} />
        <h3>{object.title}</h3>
        <p>{object.detail}</p>
      </header>
      {object.meta?.hero && (
        <div className="k-workbench-hero" data-tone={object.meta.hero.tone ?? 'neutral'}>
          <strong data-metric>{object.meta.hero.value}</strong>
          <span>{object.meta.hero.label}</span>
        </div>
      )}
      {object.meta?.callout && (
        <div className="k-workbench-callout" data-tone={object.meta.callout.tone}>
          <span>{object.meta.callout.summary}</span>
        </div>
      )}
      {object.kind === 'task' && onTaskStatus && (
        <label
          className="k-workbench-checklist"
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={taskStatus === 'done'}
            onChange={() => onTaskStatus()}
          />
          <span>{taskStatus === 'done' ? 'Completed' : 'Mark task done'}</span>
        </label>
      )}
      {visibleFacts.length > 0 && (
        <dl>
          {visibleFacts.map((fact) => (
            <React.Fragment key={fact.label}>
              <dt>{fact.label}</dt>
              <dd data-metric>{fact.value}</dd>
            </React.Fragment>
          ))}
        </dl>
      )}
      {object.facts.length > maxCollapsedFacts && (
        <button
          type="button"
          className="k-workbench-facts-more"
          aria-expanded={factsExpanded}
          onClick={(event) => {
            event.stopPropagation();
            setFactsExpanded((prev) => !prev);
          }}
        >
          {factsExpanded ? 'Show fewer facts' : `+${hiddenFactsCount} more facts`}
        </button>
      )}
      {object.meta?.runs && object.meta.runs.length > 0 && (
        <ul className="k-workbench-runs" aria-label={`Run timeline for ${object.title}`}>
          {object.meta.runs.map((run) => (
            <li key={run.id} className="k-workbench-part-row" data-status={run.status.toLowerCase()}>
              <span className="k-workbench-part-id">{run.label}</span>
              <span className="k-workbench-part-status">{run.status}</span>
              {run.metric && <span className="k-workbench-part-metric" data-metric>{run.metric}</span>}
            </li>
          ))}
        </ul>
      )}
      {object.meta?.comparison && object.meta.comparison.rows.length > 0 && (
        <CompareMatrix
          comparison={object.meta.comparison}
          title={object.title}
          cardOverrides={cardOverrides}
          onUpdateOverride={onUpdateOverride}
        />
      )}
      {object.meta?.code && object.meta.code.content && (
        <CodeBlock code={object.meta.code} />
      )}
      {object.basis && <p className="k-workbench-basis">{object.basis}</p>}
      {object.sources?.length ? (
        <ul className="k-workbench-sources" aria-label={`Sources for ${object.title}`}>
          {object.sources.map((source) => (
            <li key={source.label}>
              {source.url
                ? <a href={source.url} target="_blank" rel="noreferrer">{source.label}</a>
                : <span>{source.label}</span>}
            </li>
          ))}
        </ul>
      ) : null}
      {object.chart && (
        <div className="k-workbench-chart" data-metric data-provenance={object.provenance ?? 'catalog'}>
          <Chart spec={overview ? { ...object.chart, detail: 'overview' } : object.chart} embedded />
        </div>
      )}
      {(confirmGate || gateReceipt) && (
        <ConfirmCard gate={confirmGate} receipt={gateReceipt} onSettled={onGateSettled} />
      )}
      {approvalError && (
        <div role="alert" className="k-confirm-error">{approvalError}</div>
      )}
      {approvalReceipt && <AskReceiptBanner receipt={approvalReceipt} />}
      {selected && (onOpen || onTaskStatus || (onCloseApproval && approvalReady && !approvalReceipt)) && (
        <div className="k-workbench-actions">
          {onOpen && (
            <Button
              type="button"
              size="compact"
              className="k-workbench-open"
              onClick={(event) => {
                event.stopPropagation();
                onOpen();
              }}
            >
              {object.kind === 'approval' ? 'Review approval' : 'Open'}
            </Button>
          )}
          {onCloseApproval && approvalReady && !approvalReceipt && (
            <Button
              type="button"
              size="compact"
              data-testid="close-approval"
              onClick={(event) => {
                event.stopPropagation();
                onCloseApproval();
              }}
            >
              Close approval
            </Button>
          )}
          {onTaskStatus && (
            <Button
              type="button"
              size="compact"
              variant="ghost"
              onClick={(event) => {
                event.stopPropagation();
                onTaskStatus();
              }}
            >
              {taskStatus === 'done' ? 'Reopen task' : 'Mark done'}
            </Button>
          )}
        </div>
      )}
      {selected && (
        <span
          className="k-workbench-resize"
          aria-hidden="true"
          onPointerDown={onResizePointerDown}
          onPointerMove={onResizePointerMove}
          onPointerUp={onResizePointerUp}
          onPointerCancel={onResizePointerUp}
        />
      )}
    </article>
  );
}

const CODE_TOKEN_RE = /(\/\/.*|#.*|\b(?:OPENQASM|include|qreg|creg|gate|cx|cz|h|x|y|z|rx|ry|rz|u3|measure|def|return|import|from|for|in)\b|-?\d+(?:\.\d+)?)/g;

function highlightCodeLine(line: string) {
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  let key = 0;
  for (const match of line.matchAll(CODE_TOKEN_RE)) {
    const at = match.index ?? 0;
    if (at > cursor) parts.push(line.slice(cursor, at));
    const token = match[0];
    const kind = token.startsWith('//') || token.startsWith('#')
      ? 'comment'
      : /^-?\d/.test(token)
        ? 'number'
        : 'keyword';
    parts.push(<span key={key++} className={`k-code-${kind}`}>{token}</span>);
    cursor = at + token.length;
  }
  if (cursor < line.length) parts.push(line.slice(cursor));
  return parts.length > 0 ? parts : ' ';
}

const CODE_CAP_LINES = 16;

export function CodeBlock({
  code,
  initialExpanded = false,
}: {
  code: { language: string; content: string; label?: string };
  initialExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(initialExpanded);
  const [copied, setCopied] = useState(false);
  const lines = useMemo(() => code.content.split(/\r?\n/), [code.content]);
  const capped = !expanded && lines.length > CODE_CAP_LINES;
  const shown = capped ? lines.slice(0, CODE_CAP_LINES) : lines;

  return (
    <div
      className="k-workbench-code"
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <div className="k-workbench-code-head">
        <span>{code.label || code.language.toUpperCase()}</span>
        <div className="k-workbench-code-actions">
          <span>{lines.length} lines</span>
          <button
            type="button"
            onClick={() => {
              void navigator.clipboard?.writeText(code.content).then(
                () => { setCopied(true); setTimeout(() => setCopied(false), 1400); },
                () => setCopied(false),
              );
            }}
          >
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>
      <pre data-language={code.language}>
        <code>
          {shown.map((line, index) => (
            <span key={index} className="k-workbench-code-line">
              {highlightCodeLine(line)}
            </span>
          ))}
        </code>
      </pre>
      {lines.length > CODE_CAP_LINES && (
        <button
          type="button"
          className="k-workbench-code-more"
          aria-expanded={expanded}
          onClick={() => setExpanded((prev) => !prev)}
        >
          {expanded ? 'Collapse to 16 lines' : `Show all ${lines.length} lines`}
        </button>
      )}
    </div>
  );
}

function parseComparableNumber(cell: string): number | null {
  const match = cell.trim().match(/^(-?\d+(?:\.\d+)?)\s*([a-zA-Z%]*)$/);
  if (!match) return null;
  const n = Number(match[1]);
  return Number.isFinite(n) ? n : null;
}

function unitSuffix(cell: string): string {
  const match = cell.trim().match(/^-?\d+(?:\.\d+)?\s*(.*)$/);
  return match ? match[1].trim() : '';
}

export function CompareMatrix({
  comparison,
  title,
  cardOverrides,
  onUpdateOverride,
}: {
  comparison: WorkbenchComparisonData;
  title: string;
  cardOverrides?: Record<string, unknown>;
  onUpdateOverride?: (cellKey: string, nextValue: string | null) => void;
}) {
  const [editingCell, setEditingCell] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [cellError, setCellError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const rowStats = useMemo(() => comparison.rows.map((row) => {
    const nums = row.values.map(parseComparableNumber);
    const units = row.values.map(unitSuffix);
    const allNums = nums.every((n): n is number => n !== null);
    const sameUnit = units.every((u) => u === units[0]);
    if (!allNums || !sameUnit || nums.length < 2) return null;
    const min = Math.min(...nums);
    const max = Math.max(...nums);
    return { nums, min, max, span: max - min };
  }), [comparison.rows]);

  const commitCell = (cellKey: string) => {
    if (!onUpdateOverride) return;
    const checked = validateManualOverrideValue(draft);
    if (!checked.ok) {
      setCellError(checked.error);
      return;
    }
    setCellError(null);
    onUpdateOverride(cellKey, checked.value);
    setEditingCell(null);
  };

  return (
    <div
      className="k-workbench-compare"
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <div className="k-workbench-compare-head">
        <span>Comparison matrix</span>
        <button
          type="button"
          className="k-chart-copy"
          onClick={() => {
            void navigator.clipboard?.writeText(comparisonCsv(comparison, cardOverrides)).then(
              () => { setCopied(true); setTimeout(() => setCopied(false), 1400); },
              () => setCopied(false),
            );
          }}
        >
          {copied ? 'Copied' : 'Copy CSV'}
        </button>
      </div>
      {cellError && (
        <div role="alert" className="k-workbench-compare-error">{cellError}</div>
      )}
      <div className="k-workbench-compare-wrap">
        <table>
          <caption className="sr-only">{title} comparison matrix</caption>
          <thead>
            <tr>
              {comparison.columns.map((column, colIdx) => (
                <th key={column} scope="col" className={colIdx === 0 ? 'k-compare-corner' : undefined}>
                  {column}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {comparison.rows.map((row, rIdx) => {
              const stats = rowStats[rIdx];
              return (
                <tr key={row.label}>
                  <th scope="row" title={row.label}>{row.label}</th>
                  {row.values.map((measuredVal, cIdx) => {
                    const cellKey = `${rIdx}:${cIdx}`;
                    const ov = cardOverrides?.[cellKey] as { value?: unknown } | undefined;
                    const manualValue = typeof ov?.value === 'string' && ov.value ? ov.value : null;
                    const numeric = stats ? stats.nums[cIdx] : null;
                    const isBest = Boolean(stats && stats.span > 0 && numeric === stats.min);
                    const relPct = stats && stats.span > 0 && numeric !== null
                      ? Math.max(8, Math.round(((stats.max - numeric) / stats.span) * 100))
                      : 0;
                    const isEditing = editingCell === cellKey;
                    const colName = comparison.columns[cIdx + 1] || `Col ${cIdx + 1}`;
                    return (
                      <td
                        key={cellKey}
                        data-provenance={manualValue ? 'proposed' : 'measured'}
                        data-manual={manualValue ? 'true' : undefined}
                      >
                        {isEditing ? (
                          <div className="k-compare-editor">
                            <input
                              value={draft}
                              aria-label={`Override ${row.label} ${colName}`}
                              onChange={(event) => setDraft(event.target.value)}
                              onKeyDown={(event) => {
                                if (event.key === 'Enter') {
                                  event.preventDefault();
                                  commitCell(cellKey);
                                } else if (event.key === 'Escape') {
                                  event.preventDefault();
                                  setEditingCell(null);
                                  setCellError(null);
                                }
                              }}
                            />
                            <button type="button" onClick={() => commitCell(cellKey)}>Save</button>
                            {manualValue && onUpdateOverride && (
                              <button
                                type="button"
                                onClick={() => {
                                  onUpdateOverride(cellKey, null);
                                  setEditingCell(null);
                                  setCellError(null);
                                }}
                              >
                                Revert
                              </button>
                            )}
                          </div>
                        ) : (
                          <div className="k-compare-cell">
                            {relPct > 0 && !manualValue && (
                              <span className="k-compare-bar" style={{ width: `${relPct}%` }} aria-hidden="true" />
                            )}
                            <span className="k-compare-val" data-metric>
                              {isBest && !manualValue && <i className="k-compare-best" title="Lowest measured value in row" />}
                              {manualValue ?? measuredVal}
                            </span>
                            {manualValue && (
                              <span className="k-compare-manual-meta" data-provenance="measured">
                                <span className="k-compare-manual-badge">MANUAL</span>
                                <span className="k-compare-orig" title={`Measured value: ${measuredVal}`}>orig {measuredVal}</span>
                              </span>
                            )}
                            {onUpdateOverride && (
                              <button
                                type="button"
                                className="k-compare-edit-btn"
                                aria-label={`Edit ${row.label} ${colName}`}
                                onClick={() => {
                                  setEditingCell(cellKey);
                                  setDraft(manualValue ?? measuredVal);
                                  setCellError(null);
                                }}
                              >
                                Edit
                              </button>
                            )}
                          </div>
                        )}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
