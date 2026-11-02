'use client';

import React, { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react';
import { LayoutGrid, List, Maximize2, Minus, Plus } from 'lucide-react';
import type { CampaignState, Job, Loop, Task } from '@/types';
import { fetchCanvasLayout, fetchWorkbenchLayout, saveWorkbenchLayout, type ObligationRow } from '@/lib/api';
import { Chart } from '@/components/data/LazyChart';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { Badge } from '@/components/ui/Badge';
import { acceptWrite, newWriteRequestId, settleWrite, writeSettlementLabel, IDLE_WRITE, type WriteSettlement } from '@/domain/write-settlement';
import {
  WORKBENCH_BOUNDS,
  buildWorkbench,
  buildWorkbenchLinks,
  readCamera,
  readPositions,
  readSizes,
  readWorkbenchDocument,
  workbenchDocument,
  type WorkbenchArea,
  type WorkbenchCamera,
  type WorkbenchLink,
  type WorkbenchObject,
  type WorkbenchObjectKind,
  type WorkbenchPositions,
  type WorkbenchSizes,
} from './workbench-model';

type Props = {
  campaign?: CampaignState | null;
  sessionId?: string;
  loops?: Loop[];
  jobs?: Job[];
  obligations?: ObligationRow[];
  tasks?: Task[];
  onSelectObject?: (object: WorkbenchObject | null) => void;
  onOpenObject?: (kind: WorkbenchObjectKind) => void;
};

export type WorkbenchHandle = {
  focusObject: (id: string) => void;
  removeObject: (id: string) => void;
};

export const Workbench = React.forwardRef<WorkbenchHandle, Props>(function Workbench({
  campaign, sessionId, loops = [], jobs: suppliedJobs, obligations: suppliedObligations, tasks: suppliedTasks, onSelectObject, onOpenObject,
}, ref) {
  const hostRef = useRef<HTMLDivElement>(null);
  const worldRef = useRef<HTMLDivElement>(null);
  const revision = useRef(0);
  const loaded = useRef(false);
  const drag = useRef<{ x: number; y: number; camera: WorkbenchCamera } | null>(null);
  const cardDrag = useRef<{ ids: string[]; x: number; y: number; origins: Record<string, { x: number; y: number }> } | null>(null);
  const sizeDrag = useRef<{ id: string; x: number; y: number; width: number; height: number } | null>(null);
  const areaDrag = useRef<{ id: string; x: number; y: number; originX: number; originY: number } | null>(null);
  const areaResize = useRef<{ id: string; x: number; y: number; width: number; height: number } | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [camera, setCamera] = useState<WorkbenchCamera>({ x: 36, y: 36, zoom: 0.82 });
  const cameraRef = useRef(camera);
  const zoomCommit = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [positions, setPositions] = useState<WorkbenchPositions>({});
  const [sizes, setSizes] = useState<WorkbenchSizes>({});
  const [storedLinks, setStoredLinks] = useState<WorkbenchLink[] | null>(null);
  const [layoutObjects, setLayoutObjects] = useState<{ id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[]>([]);
  const [linkKind, setLinkKind] = useState<WorkbenchLink['kind']>('supports');
  const jobs = useMemo(() => suppliedJobs ?? [], [suppliedJobs]);
  const obligations = useMemo(() => suppliedObligations ?? [], [suppliedObligations]);
  const tasks = useMemo(() => suppliedTasks ?? [], [suppliedTasks]);
  const baseObjects = useMemo(() => {
    const projected = buildWorkbench(campaign, loops, jobs, obligations, tasks);
    const known = new Set(projected.map((object) => object.id));
    const unavailable: WorkbenchObject[] = layoutObjects
      .filter((object) => !known.has(object.id) && object.ref)
      .map((object) => ({
        id: object.id,
        kind: object.kind,
        label: 'Unavailable',
        title: `${object.ref!.domain} record ${object.ref!.id.slice(0, 12)}`,
        detail: 'This reference is preserved in the layout but is not in the live projection.',
        position: object.position,
        size: object.size,
        facts: [
          { label: 'Domain', value: object.ref!.domain },
          { label: 'Record', value: object.ref!.id },
        ],
        ref: object.ref,
      }));
    return [...projected, ...unavailable];
  }, [campaign, loops, jobs, obligations, tasks, layoutObjects]);
  const links = useMemo(() => storedLinks ?? buildWorkbenchLinks(baseObjects), [storedLinks, baseObjects]);
  const [viewMode, setViewMode] = useState<'canvas' | 'list'>('canvas');
  const [areas, setAreas] = useState<WorkbenchArea[]>([]);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [assignments, setAssignments] = useState<Record<string, string>>({});
  const [hidden, setHidden] = useState<string[]>([]);
  const [lastRemoved, setLastRemoved] = useState<string[]>([]);
  const [activeAreaId, setActiveAreaId] = useState<string | null>(null);
  const [conflictRevision, setConflictRevision] = useState<number | null>(null);
  const [write, setWrite] = useState<WriteSettlement>(IDLE_WRITE);
  useEffect(() => {
    if (!campaign) onSelectObject?.(null);
  }, [campaign, onSelectObject]);

  const objects = useMemo(
    () => baseObjects.map((object) => ({
      ...object,
      position: positions[object.id] ?? object.position,
      size: sizes[object.id] ?? object.size,
    })),
    [baseObjects, positions, sizes],
  );
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
  const activeArea = areas.find((area) => area.id === activeAreaId) ?? areas[0] ?? null;

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
    setHidden((previous) => Array.from(new Set([...previous, ...selected])));
    setLastRemoved(selected);
    setSelected([]);
    onSelectObject?.(null);
  }, [selected, onSelectObject]);

  const undoRemove = useCallback(() => {
    if (lastRemoved.length === 0) return;
    setHidden((previous) => previous.filter((id) => !lastRemoved.includes(id)));
    setLastRemoved([]);
  }, [lastRemoved]);

  const customLink = useCallback(() => {
    if (selected.length !== 2) return;
    const [source, target] = selected;
    setStoredLinks((previous) => {
      const current = previous ?? buildWorkbenchLinks(baseObjects);
      if (current.some((link) => link.source === source && link.target === target && link.kind === linkKind)) return current;
      return [...current, { id: `${source}-${target}-${linkKind}`, source, target, kind: linkKind }];
    });
  }, [selected, linkKind, baseObjects]);

  const removeLink = useCallback((id: string) => {
    setStoredLinks((previous) => (previous ?? buildWorkbenchLinks(baseObjects)).filter((link) => link.id !== id));
  }, [baseObjects]);

  useImperativeHandle(ref, () => ({
    focusObject: (id: string) => {
      const object = objects.find((item) => item.id === id);
      const host = hostRef.current;
      if (!object || !host) return;
      const zoom = Math.min(1.5, Math.max(0.25, camera.zoom));
      setCamera({
        x: host.clientWidth / 2 - (object.position.x + object.size.width / 2) * zoom,
        y: host.clientHeight / 2 - (object.position.y + object.size.height / 2) * zoom,
        zoom,
      });
    },
    removeObject: (id: string) => {
      setHidden((previous) => previous.includes(id) ? previous : [...previous, id]);
      setLastRemoved([id]);
      setSelected((previous) => previous.filter((item) => item !== id));
      onSelectObject?.(null);
    },
  }), [objects, camera.zoom, onSelectObject]);

  const applyCamera = useCallback((next: WorkbenchCamera) => {
    cameraRef.current = next;
    if (worldRef.current) {
      worldRef.current.style.transform = `translate3d(${next.x}px, ${next.y}px, 0) scale(${next.zoom})`;
    }
  }, []);

  const commitCamera = useCallback(() => {
    setCamera(cameraRef.current);
  }, []);

  useEffect(() => {
    cameraRef.current = camera;
    if (worldRef.current) {
      worldRef.current.style.transform = `translate3d(${camera.x}px, ${camera.y}px, 0) scale(${camera.zoom})`;
    }
  }, [camera]);

  const fit = useCallback(() => {
    const host = hostRef.current;
    if (!host) return;
    const width = host.clientWidth;
    const height = host.clientHeight;
    if (!width || !height) return;
    const zoom = Math.min(1.2, Math.max(0.35, Math.min((width - 64) / WORKBENCH_BOUNDS.width, (height - 64) / WORKBENCH_BOUNDS.height)));
    setCamera({
      x: (width - WORKBENCH_BOUNDS.width * zoom) / 2,
      y: (height - WORKBENCH_BOUNDS.height * zoom) / 2,
      zoom,
    });
  }, []);

  const applySaved = useCallback((saved: { revision: number; document: Record<string, unknown> }) => {
    revision.current = saved.revision;
    const doc = readWorkbenchDocument(saved.document);
    setCamera(doc.camera);
    setPositions(doc.positions);
    setSizes(doc.sizes);
    setAreas(doc.areas);
    setAssignments(doc.assignments);
    setHidden(doc.hidden);
    setLayoutObjects(doc.objects);
    setConflictRevision(null);
    loaded.current = true;
  }, []);

  useEffect(() => {
    loaded.current = false;
    if (!sessionId) {
      loaded.current = true;
      return;
    }
    let live = true;
    void (async () => {
      const saved = await fetchWorkbenchLayout(sessionId);
      if (!live) return;
      if (saved && saved.revision > 0) {
        applySaved(saved);
        return;
      }
      const legacy = await fetchCanvasLayout(sessionId);
      if (!live) return;
      revision.current = 0;
      setCamera(readCamera(legacy));
      setPositions(readPositions(legacy));
      setSizes(readSizes(legacy));
      setLayoutObjects([]);
      loaded.current = true;
    })();
    return () => {
      live = false;
    };
  }, [sessionId, applySaved]);

  const reloadLatest = useCallback(async () => {
    if (!sessionId) return;
    const saved = await fetchWorkbenchLayout(sessionId);
    if (saved && saved.revision > 0) applySaved(saved);
  }, [sessionId, applySaved]);

  // Save the camera only, never a duplicate of the campaign content.
  useEffect(() => {
    if (!sessionId || !loaded.current) return;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      const requestId = newWriteRequestId();
      setWrite(acceptWrite(requestId, 'saving view'));
      void saveWorkbenchLayout(
        sessionId,
        revision.current,
        workbenchDocument(sessionId, baseObjects, positions, sizes, camera, links, areas, assignments, hidden) as unknown as Record<string, unknown>,
      ).then((out) => {
        if (out.ok) {
          if (typeof out.revision === 'number') revision.current = out.revision;
          setWrite(settleWrite(requestId, 'applied', `revision ${revision.current}`));
        } else if (out.conflict) {
          setConflictRevision(out.currentRevision ?? null);
          setWrite(settleWrite(requestId, 'refused', out.error || 'layout changed elsewhere'));
        } else if (out.unknown) {
          setWrite(settleWrite(requestId, 'unknown', 'save outcome not confirmed'));
        } else {
          setWrite(settleWrite(requestId, 'refused', out.error || 'layout save refused'));
        }
      });
    }, 650);
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current);
    };
  }, [camera, positions, sizes, sessionId, baseObjects, links, areas, assignments, hidden]);

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
    if (visibleObjects.length <= 24 || viewport.width === 0) return null;
    const margin = 160;
    const left = -camera.x / camera.zoom - margin;
    const top = -camera.y / camera.zoom - margin;
    const right = left + viewport.width / camera.zoom + margin * 2;
    const bottom = top + viewport.height / camera.zoom + margin * 2;
    return new Set(visibleObjects.filter((object) => (
      object.position.x + object.size.width >= left &&
      object.position.x <= right &&
      object.position.y + object.size.height >= top &&
      object.position.y <= bottom
    )).map((object) => object.id));
  }, [visibleObjects, viewport, camera]);

  // Native, non-passive wheel listener: React's delegated wheel handler is
  // passive, so preventDefault there would be ignored (and log a warning).
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
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
  }, [applyCamera, commitCamera]);

  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return;
    drag.current = { x: event.clientX, y: event.clientY, camera: cameraRef.current };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const onPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    const dx = event.clientX - drag.current.x;
    const dy = event.clientY - drag.current.y;
    applyCamera({ ...drag.current.camera, x: drag.current.camera.x + dx, y: drag.current.camera.y + dy });
  };

  const onPointerUp = () => {
    if (!drag.current) return;
    drag.current = null;
    commitCamera();
  };

  const onCardPointerDown = (event: React.PointerEvent<HTMLElement>, object: WorkbenchObject) => {
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    const group = selected.includes(object.id) ? selected : [object.id];
    if (!selected.includes(object.id)) selectObject(object, false);
    cardDrag.current = {
      ids: group,
      x: event.clientX,
      y: event.clientY,
      origins: Object.fromEntries(group.map((id) => {
        const item = objects.find((candidate) => candidate.id === id) ?? object;
        return [id, { x: item.position.x, y: item.position.y }];
      })),
    };
  };

  const onCardPointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const current = cardDrag.current;
    if (!current) return;
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    setPositions((previous) => {
      const next = { ...previous };
      for (const id of current.ids) {
        const origin = current.origins[id];
        if (!origin) continue;
        next[id] = {
          x: Math.max(-400, Math.min(1800, Math.round((origin.x + dx) * 100) / 100)),
          y: Math.max(-400, Math.min(1200, Math.round((origin.y + dy) * 100) / 100)),
        };
      }
      return next;
    });
  };

  const onCardPointerUp = () => {
    cardDrag.current = null;
    sizeDrag.current = null;
  };

  const onResizePointerDown = (event: React.PointerEvent<HTMLElement>, object: WorkbenchObject) => {
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    sizeDrag.current = {
      id: object.id,
      x: event.clientX,
      y: event.clientY,
      width: object.size.width,
      height: object.size.height,
    };
  };

  const onResizePointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const current = sizeDrag.current;
    if (!current) return;
    const width = Math.max(220, Math.min(900, current.width + (event.clientX - current.x) / camera.zoom));
    const height = Math.max(140, Math.min(900, current.height + (event.clientY - current.y) / camera.zoom));
    setSizes((previous) => ({
      ...previous,
      [current.id]: { width: Math.round(width * 100) / 100, height: Math.round(height * 100) / 100 },
    }));
  };

  const onAreaPointerDown = (event: React.PointerEvent<HTMLDivElement>, area: WorkbenchArea) => {
    if ((event.target as HTMLElement).dataset.resize === 'area') return;
    event.stopPropagation();
    setActiveAreaId(area.id);
    event.currentTarget.setPointerCapture(event.pointerId);
    areaDrag.current = { id: area.id, x: event.clientX, y: event.clientY, originX: area.rect.x, originY: area.rect.y };
  };

  const onAreaPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const current = areaDrag.current;
    if (!current) return;
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    setAreas((previous) => previous.map((area) => area.id === current.id
      ? { ...area, rect: { ...area.rect, x: current.originX + dx, y: current.originY + dy } }
      : area));
  };

  const onAreaResizePointerDown = (event: React.PointerEvent<HTMLSpanElement>, area: WorkbenchArea) => {
    event.stopPropagation();
    setActiveAreaId(area.id);
    event.currentTarget.setPointerCapture(event.pointerId);
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

  const createArea = useCallback((title: string, rect: { x: number; y: number; width: number; height: number }) => {
    const id = newWriteRequestId();
    setAreas((previous) => [...previous, { id, title, rect }]);
    setActiveAreaId(id);
  }, []);

  const groupSelected = useCallback(() => {
    const items = visibleObjects.filter((object) => selected.includes(object.id));
    if (items.length === 0) return;
    const minX = Math.min(...items.map((object) => object.position.x)) - 24;
    const minY = Math.min(...items.map((object) => object.position.y)) - 48;
    const maxX = Math.max(...items.map((object) => object.position.x + object.size.width)) + 24;
    const maxY = Math.max(...items.map((object) => object.position.y + object.size.height)) + 24;
    createArea(`Area ${areas.length + 1}`, { x: minX, y: minY, width: maxX - minX, height: maxY - minY });
  }, [visibleObjects, selected, areas.length, createArea]);

  return (
    <section
      ref={hostRef}
      className="k-workbench"
      data-testid="workbench"
      data-object-count={objects.length}
      data-loop-count={loops.length}
      data-job-count={jobs.length}
      data-obligation-count={obligations.length}
      aria-label="Campaign workbench"
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={(event) => {
        if (event.metaKey || event.ctrlKey || event.altKey) return;
        if (event.key === '0') {
          event.preventDefault();
          fit();
        }
        if (event.key === '1') {
          event.preventDefault();
          setCamera((current) => ({ ...current, zoom: 1 }));
        }
        if ((event.key === 'Delete' || event.key === 'Backspace') && selected.length > 0) {
          const target = event.target as HTMLElement;
          if (!/^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName) && !target.isContentEditable) {
            event.preventDefault();
            removeSelected();
          }
        }
      }}
    >
      <div className="k-workbench-bar" aria-label="Workbench controls">
        <Button type="button" variant="ghost" size="compact" onClick={fit} title="Fit the whole workbench (0)">
          <Maximize2 size={14} /> Fit
        </Button>
        <Button type="button" variant="ghost" size="compact" aria-label="Zoom out" onClick={() => setCamera((current) => ({ ...current, zoom: Math.max(0.35, current.zoom / 1.2) }))} title="Zoom out">
          <Minus size={14} />
        </Button>
        <span className="k-workbench-zoom">{Math.round(camera.zoom * 100)}%</span>
        <Button type="button" variant="ghost" size="compact" aria-label="Zoom in" onClick={() => setCamera((current) => ({ ...current, zoom: Math.min(2, current.zoom * 1.2) }))} title="Zoom in">
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
              className="h-7 rounded-lg border border-white/10 bg-black/30 px-1 text-[12px] text-slate-300"
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
            onClick={() => {
              const id = activeArea.id;
              setAreas((previous) => previous.filter((area) => area.id !== id));
              setAssignments((previous) => Object.fromEntries(Object.entries(previous).filter(([, areaId]) => areaId !== id)));
              setActiveAreaId(areas.find((area) => area.id !== id)?.id ?? null);
            }}
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
        {lastRemoved.length > 0 && (
          <Button type="button" variant="ghost" size="compact" onClick={undoRemove} title="Restore the last removed card">
            Undo remove
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

      {visibleObjects.length === 0 ? (
        <EmptyState
          title={objects.length === 0 ? 'No campaign on the workbench' : 'All workbench objects are hidden'}
          className="k-workbench-empty"
        >
          {objects.length === 0 ? 'Create or open a campaign to give the science a place to stand.' : 'Use Undo remove or the list view to restore an object.'}
        </EmptyState>
      ) : (
        viewMode === 'list' ? (
          <div className="k-workbench-list">
            <table>
              <caption>Campaign workbench objects — text equivalent of the canvas</caption>
              <thead><tr><th scope="col">Kind</th><th scope="col">Object</th><th scope="col">Recorded facts</th><th scope="col">Open</th></tr></thead>
              <tbody>
                {visibleObjects.map((object) => (
                  <tr key={object.id}>
                    <th scope="row">{object.label}</th>
                    <td><strong>{object.title}</strong><span>{object.detail}</span></td>
                    <td>{object.facts.map((fact) => `${fact.label}: ${fact.value}`).join(' · ') || 'No facts recorded'}</td>
                    <td>{onOpenObject ? <Button type="button" size="compact" variant="ghost" onClick={() => onOpenObject(object.kind)}>Open</Button> : '—'}</td>
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
            width: WORKBENCH_BOUNDS.width,
            height: WORKBENCH_BOUNDS.height,
            transform: `translate3d(${camera.x}px, ${camera.y}px, 0) scale(${camera.zoom})`,
          }}
        >
          {areas.map((area) => (
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
          ))}
          <svg className="k-workbench-links" width={WORKBENCH_BOUNDS.width} height={WORKBENCH_BOUNDS.height} aria-hidden="true">
            {links.map((link) => {
              const source = visibleObjects.find((object) => object.id === link.source);
              const target = visibleObjects.find((object) => object.id === link.target);
              if (!source || !target) return null;
              const x1 = source.position.x + source.size.width;
              const y1 = source.position.y + source.size.height / 2;
              const x2 = target.position.x;
              const y2 = target.position.y + target.size.height / 2;
              return <path key={link.id} data-kind={link.kind} d={`M${x1},${y1} C${x1 + 40},${y1} ${x2 - 40},${y2} ${x2},${y2}`} />;
            })}
          </svg>
          {visibleObjects.map((object) => (viewport && visibleIds && !visibleIds.has(object.id) ? null : (
            <WorkCard
              key={object.id}
              object={object}
              selected={selected.includes(object.id)}
              assigned={Boolean(areas[0] && assignments[object.id] === areas[0].id)}
              onSelect={(additive) => selectObject(object, additive)}
              onPointerDown={(event) => onCardPointerDown(event, object)}
              onPointerMove={onCardPointerMove}
              onPointerUp={onCardPointerUp}
              onResizePointerDown={(event) => onResizePointerDown(event, object)}
              onResizePointerMove={onResizePointerMove}
              onResizePointerUp={onCardPointerUp}
              onOpen={onOpenObject ? () => onOpenObject(object.kind) : undefined}
              onNudge={(dx, dy) => setPositions((previous) => ({
                ...previous,
                [object.id]: {
                  x: (previous[object.id]?.x ?? object.position.x) + dx,
                  y: (previous[object.id]?.y ?? object.position.y) + dy,
                },
              }))}
            />
          )))}
        </div>
        )
      )}
    </section>
  );
});

function WorkCard({
  object,
  selected,
  assigned,
  onSelect,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onResizePointerDown,
  onResizePointerMove,
  onResizePointerUp,
  onOpen,
  onNudge,
}: {
  object: WorkbenchObject;
  selected: boolean;
  assigned: boolean;
  onSelect: (additive: boolean) => void;
  onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: () => void;
  onResizePointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onResizePointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onResizePointerUp: () => void;
  onOpen?: () => void;
  onNudge: (dx: number, dy: number) => void;
}) {
  return (
    <article
      className="k-workbench-card"
      data-kind={object.kind}
      data-selected={selected ? 'true' : undefined}
      style={{
        left: object.position.x,
        top: object.position.y,
        width: object.size.width,
        height: object.size.height,
      }}
      tabIndex={0}
      aria-label={`${object.label}: ${object.title}`}
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
        {assigned && <Badge tone="info">area</Badge>}
        <h3>{object.title}</h3>
        <p>{object.detail}</p>
      </header>
      {object.facts.length > 0 && (
        <dl>
          {object.facts.map((fact) => (
            <div key={fact.label}>
              <dt>{fact.label}</dt>
              <dd>{fact.value}</dd>
            </div>
          ))}
        </dl>
      )}
      {object.chart && (
        <div className="k-workbench-chart">
          <Chart spec={object.chart} />
        </div>
      )}
      {selected && onOpen && (
        <Button
          type="button"
          size="compact"
          className="k-workbench-open"
          onClick={(event) => {
            event.stopPropagation();
            onOpen();
          }}
        >
          Open
        </Button>
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
