'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { LayoutGrid, List, Maximize2, Minus, Plus } from 'lucide-react';
import type { CampaignState, Job, Loop } from '@/types';
import { fetchCanvasLayout, fetchOpenObligations, fetchWorkbenchLayout, saveWorkbenchLayout, type ObligationRow } from '@/lib/api';
import { requestOperation } from '@/lib/generated/request';
import { Chart } from '@/components/data/DataChart';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { Badge } from '@/components/ui/Badge';
import { usePollingResource } from '@/domain/use-polling-resource';
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
  onOpenObject?: (kind: WorkbenchObjectKind) => void;
};

export function Workbench({ campaign, sessionId, loops = [], jobs: suppliedJobs, obligations: suppliedObligations, onOpenObject }: Props) {
  const hostRef = useRef<HTMLDivElement>(null);
  const revision = useRef(0);
  const loaded = useRef(false);
  const drag = useRef<{ x: number; y: number; camera: WorkbenchCamera } | null>(null);
  const cardDrag = useRef<{ id: string; x: number; y: number; originX: number; originY: number } | null>(null);
  const sizeDrag = useRef<{ id: string; x: number; y: number; width: number; height: number } | null>(null);
  const areaDrag = useRef<{ x: number; y: number; originX: number; originY: number } | null>(null);
  const areaResize = useRef<{ x: number; y: number; width: number; height: number } | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [camera, setCamera] = useState<WorkbenchCamera>({ x: 36, y: 36, zoom: 0.82 });
  const [selected, setSelected] = useState<string | null>(null);
  const [positions, setPositions] = useState<WorkbenchPositions>({});
  const [sizes, setSizes] = useState<WorkbenchSizes>({});
  const { data: fetchedJobs } = usePollingResource(
    async () => {
      if (!sessionId) return [];
      const data = await requestOperation('fetchJobs', { query: { session_id: sessionId } });
      return data.jobs as unknown as Job[];
    },
    { intervalMs: 5000 },
  );
  const jobs = useMemo(() => suppliedJobs ?? fetchedJobs ?? [], [suppliedJobs, fetchedJobs]);
  const { data: fetchedObligations } = usePollingResource(async () => {
    const out = await fetchOpenObligations();
    if (!out.ok) throw new Error(out.error);
    return out.obligations;
  }, { intervalMs: 15_000 });
  const obligations = useMemo(() => suppliedObligations ?? fetchedObligations ?? [], [suppliedObligations, fetchedObligations]);
  const baseObjects = useMemo(() => buildWorkbench(campaign, loops, jobs, obligations), [campaign, loops, jobs, obligations]);
  const links = useMemo(() => buildWorkbenchLinks(baseObjects), [baseObjects]);
  const [viewMode, setViewMode] = useState<'canvas' | 'list'>('canvas');
  const [areas, setAreas] = useState<WorkbenchArea[]>([]);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [assignments, setAssignments] = useState<Record<string, string>>({});
  const [write, setWrite] = useState<WriteSettlement>(IDLE_WRITE);
  const objects = useMemo(
    () => baseObjects.map((object) => ({
      ...object,
      position: positions[object.id] ?? object.position,
      size: sizes[object.id] ?? object.size,
    })),
    [baseObjects, positions, sizes],
  );
  const areaBounds = useMemo(() => {
    if (objects.length === 0) return null;
    const minX = Math.min(...objects.map((object) => object.position.x)) - 24;
    const minY = Math.min(...objects.map((object) => object.position.y)) - 48;
    const maxX = Math.max(...objects.map((object) => object.position.x + object.size.width)) + 24;
    const maxY = Math.max(...objects.map((object) => object.position.y + object.size.height)) + 24;
    return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
  }, [objects]);

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
        revision.current = saved.revision;
        const doc = readWorkbenchDocument(saved.document);
        setCamera(doc.camera);
        setPositions(doc.positions);
        setSizes(doc.sizes);
        setAreas(doc.areas);
        setAssignments(doc.assignments);
        loaded.current = true;
        return;
      }
      const legacy = await fetchCanvasLayout(sessionId);
      if (!live) return;
      revision.current = 0;
      setCamera(readCamera(legacy));
      setPositions(readPositions(legacy));
      setSizes(readSizes(legacy));
      loaded.current = true;
    })();
    return () => {
      live = false;
    };
  }, [sessionId]);

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
        workbenchDocument(sessionId, baseObjects, positions, sizes, camera, links, areas, assignments) as unknown as Record<string, unknown>,
      ).then((out) => {
        if (out.ok) {
          if (typeof out.revision === 'number') revision.current = out.revision;
          setWrite(settleWrite(requestId, 'applied', `revision ${revision.current}`));
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
  }, [camera, positions, sizes, sessionId, baseObjects, links, areas, assignments]);

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
    if (objects.length <= 24 || viewport.width === 0) return null;
    const margin = 600;
    const left = -camera.x / camera.zoom - margin;
    const top = -camera.y / camera.zoom - margin;
    const right = left + viewport.width / camera.zoom + margin * 2;
    const bottom = top + viewport.height / camera.zoom + margin * 2;
    return new Set(objects.filter((object) => (
      object.position.x + object.size.width >= left &&
      object.position.x <= right &&
      object.position.y + object.size.height >= top &&
      object.position.y <= bottom
    )).map((object) => object.id));
  }, [objects, viewport, camera]);

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
      setCamera((current) => {
        const zoom = Math.min(2, Math.max(0.35, current.zoom * factor));
        return {
          x: px - ((px - current.x) / current.zoom) * zoom,
          y: py - ((py - current.y) / current.zoom) * zoom,
          zoom,
        };
      });
    };
    host.addEventListener('wheel', onWheel, { passive: false });
    return () => host.removeEventListener('wheel', onWheel);
  }, []);

  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return;
    drag.current = { x: event.clientX, y: event.clientY, camera };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const onPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    const dx = event.clientX - drag.current.x;
    const dy = event.clientY - drag.current.y;
    setCamera({ ...drag.current.camera, x: drag.current.camera.x + dx, y: drag.current.camera.y + dy });
  };

  const onPointerUp = () => {
    drag.current = null;
  };

  const onCardPointerDown = (event: React.PointerEvent<HTMLElement>, object: WorkbenchObject) => {
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    cardDrag.current = {
      id: object.id,
      x: event.clientX,
      y: event.clientY,
      originX: object.position.x,
      originY: object.position.y,
    };
  };

  const onCardPointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const current = cardDrag.current;
    if (!current) return;
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    setPositions((previous) => ({
      ...previous,
      [current.id]: {
        x: Math.max(-400, Math.min(1800, Math.round((current.originX + dx) * 100) / 100)),
        y: Math.max(-400, Math.min(1200, Math.round((current.originY + dy) * 100) / 100)),
      },
    }));
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

  const onAreaPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    const area = areas[0];
    if (!area || (event.target as HTMLElement).dataset.resize === 'area') return;
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    areaDrag.current = { x: event.clientX, y: event.clientY, originX: area.rect.x, originY: area.rect.y };
  };

  const onAreaPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const current = areaDrag.current;
    if (!current) return;
    const dx = (event.clientX - current.x) / camera.zoom;
    const dy = (event.clientY - current.y) / camera.zoom;
    setAreas((previous) => previous.map((area, index) => index === 0
      ? { ...area, rect: { ...area.rect, x: current.originX + dx, y: current.originY + dy } }
      : area));
  };

  const onAreaResizePointerDown = (event: React.PointerEvent<HTMLSpanElement>) => {
    const area = areas[0];
    if (!area) return;
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    areaResize.current = { x: event.clientX, y: event.clientY, width: area.rect.width, height: area.rect.height };
  };

  const onAreaResizePointerMove = (event: React.PointerEvent<HTMLSpanElement>) => {
    const current = areaResize.current;
    if (!current) return;
    const width = Math.max(320, current.width + (event.clientX - current.x) / camera.zoom);
    const height = Math.max(220, current.height + (event.clientY - current.y) / camera.zoom);
    setAreas((previous) => previous.map((area, index) => index === 0
      ? { ...area, rect: { ...area.rect, width, height } }
      : area));
  };

  return (
    <section
      ref={hostRef}
      className="k-workbench"
      data-testid="workbench"
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
          onClick={() => setAreas((current) => current.length > 0 ? [] : [{ id: 'area-science', title: 'Scientific chain', rect: areaBounds! }])}
          title="Group all objects into a spatial area, or clear the area"
        >
          {areas.length > 0 ? 'Ungroup' : 'Group all'}
        </Button>
        {areas[0] && (
          <input
            className="k-workbench-area-title"
            aria-label="Area title"
            value={areas[0].title}
            onChange={(event) => setAreas((previous) => previous.map((area, index) => index === 0 ? { ...area, title: event.target.value } : area))}
          />
        )}
        {selected && areas[0] && (
          <Button
            type="button"
            variant="ghost"
            size="compact"
            onClick={() => setAssignments((previous) => {
              const next = { ...previous };
              if (next[selected] === areas[0].id) delete next[selected];
              else next[selected] = areas[0].id;
              return next;
            })}
          >
            {assignments[selected] === areas[0].id ? 'Unassign' : 'Assign to area'}
          </Button>
        )}
        <span className="k-workbench-state" role="status" aria-live="polite">
          {writeSettlementLabel(write) || (sessionId ? 'view saved' : 'no campaign selected')}
        </span>
      </div>

      {objects.length === 0 ? (
        <EmptyState title="No campaign on the workbench" className="k-workbench-empty">
          Create or open a campaign to give the science a place to stand.
        </EmptyState>
      ) : (
        viewMode === 'list' ? (
          <div className="k-workbench-list">
            <table>
              <caption>Campaign workbench objects — text equivalent of the canvas</caption>
              <thead><tr><th scope="col">Kind</th><th scope="col">Object</th><th scope="col">Recorded facts</th><th scope="col">Open</th></tr></thead>
              <tbody>
                {objects.map((object) => (
                  <tr key={object.id}>
                    <th scope="row">{object.label}</th>
                    <td><strong>{object.title}</strong><span>{object.detail}</span></td>
                    <td>{object.facts.map((fact) => `${fact.label}: ${fact.value}`).join(' · ') || 'No facts recorded'}</td>
                    <td>{onOpenObject ? <Button type="button" size="compact" variant="ghost" onClick={() => onOpenObject(object.kind)}>Open</Button> : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
        <div
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
              style={{ left: area.rect.x, top: area.rect.y, width: area.rect.width, height: area.rect.height }}
              onPointerDown={onAreaPointerDown}
              onPointerMove={onAreaPointerMove}
              onPointerUp={() => { areaDrag.current = null; }}
              onPointerCancel={() => { areaDrag.current = null; }}
            >
              <span>{area.title}</span>
              <span
                className="k-workbench-area-resize"
                data-resize="area"
                aria-hidden="true"
                onPointerDown={onAreaResizePointerDown}
                onPointerMove={onAreaResizePointerMove}
                onPointerUp={() => { areaResize.current = null; }}
                onPointerCancel={() => { areaResize.current = null; }}
              />
            </div>
          ))}
          <svg className="k-workbench-links" width={WORKBENCH_BOUNDS.width} height={WORKBENCH_BOUNDS.height} aria-hidden="true">
            {links.map((link) => {
              const source = objects.find((object) => object.id === link.source);
              const target = objects.find((object) => object.id === link.target);
              if (!source || !target) return null;
              const x1 = source.position.x + source.size.width;
              const y1 = source.position.y + source.size.height / 2;
              const x2 = target.position.x;
              const y2 = target.position.y + target.size.height / 2;
              return <path key={link.id} data-kind={link.kind} d={`M${x1},${y1} C${x1 + 40},${y1} ${x2 - 40},${y2} ${x2},${y2}`} />;
            })}
          </svg>
          {objects.map((object) => viewport && visibleIds && !visibleIds.has(object.id) ? (
            <div
              key={object.id}
              className="k-workbench-placeholder"
              style={{ left: object.position.x, top: object.position.y, width: object.size.width, height: object.size.height }}
              aria-hidden="true"
            />
          ) : (
            <WorkCard
              key={object.id}
              object={object}
              selected={selected === object.id}
              assigned={Boolean(areas[0] && assignments[object.id] === areas[0].id)}
              onSelect={() => setSelected(object.id)}
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
          ))}
        </div>
        )
      )}
    </section>
  );
}

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
  onSelect: () => void;
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
      onClick={onSelect}
      onFocus={onSelect}
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
