'use client';

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Maximize2, Minus, Plus } from 'lucide-react';
import type { CampaignState } from '@/types';
import { fetchCanvasLayout, saveCanvasLayout } from '@/lib/api';
import { Chart } from '@/components/data/DataChart';
import {
  WORKBENCH_BOUNDS,
  buildWorkbench,
  readCamera,
  readPositions,
  workbenchPayload,
  type WorkbenchCamera,
  type WorkbenchObject,
  type WorkbenchPositions,
} from './workbench-model';

type Props = {
  campaign?: CampaignState | null;
  sessionId?: string;
};

export function Workbench({ campaign, sessionId }: Props) {
  const hostRef = useRef<HTMLDivElement>(null);
  const baseLayout = useRef<Record<string, unknown>>({});
  const loaded = useRef(false);
  const drag = useRef<{ x: number; y: number; camera: WorkbenchCamera } | null>(null);
  const cardDrag = useRef<{ id: string; x: number; y: number; originX: number; originY: number } | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [camera, setCamera] = useState<WorkbenchCamera>({ x: 36, y: 36, zoom: 0.82 });
  const [selected, setSelected] = useState<string | null>(null);
  const [positions, setPositions] = useState<WorkbenchPositions>({});
  const [status, setStatus] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const objects = buildWorkbench(campaign).map((object) => ({
    ...object,
    position: positions[object.id] ?? object.position,
  }));

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
    fetchCanvasLayout(sessionId).then((layout) => {
      if (!live) return;
      baseLayout.current = layout ?? {};
      setCamera(readCamera(layout));
      setPositions(readPositions(layout));
      loaded.current = true;
    });
    return () => {
      live = false;
    };
  }, [sessionId]);

  // Save the camera only, never a duplicate of the campaign content.
  useEffect(() => {
    if (!sessionId || !loaded.current) return;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      setSaving(true);
      void saveCanvasLayout(sessionId, {
        ...baseLayout.current,
        workbench: workbenchPayload(camera, positions),
      }).then((out) => {
        setSaving(false);
        if (!out.ok) setStatus(out.error || 'layout save refused');
        else setStatus(null);
      });
    }, 650);
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current);
    };
  }, [camera, positions, sessionId]);

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
        <button type="button" onClick={fit} title="Fit the whole workbench (0)">
          <Maximize2 size={14} /> Fit
        </button>
        <button type="button" onClick={() => setCamera((current) => ({ ...current, zoom: Math.max(0.35, current.zoom / 1.2) }))} title="Zoom out">
          <Minus size={14} />
        </button>
        <span className="k-workbench-zoom">{Math.round(camera.zoom * 100)}%</span>
        <button type="button" onClick={() => setCamera((current) => ({ ...current, zoom: Math.min(2, current.zoom * 1.2) }))} title="Zoom in">
          <Plus size={14} />
        </button>
        <span className="k-workbench-state" role="status" aria-live="polite">
          {saving ? 'saving view…' : status || (sessionId ? 'view saved' : 'no campaign selected')}
        </span>
      </div>

      {objects.length === 0 ? (
        <div className="k-workbench-empty">
          <strong>No campaign on the workbench</strong>
          <span>Create or open a campaign to give the science a place to stand.</span>
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
          <svg className="k-workbench-links" width={WORKBENCH_BOUNDS.width} height={WORKBENCH_BOUNDS.height} aria-hidden="true">
            {objects
              .filter((object) => object.id !== 'campaign' && object.id !== 'hypothesis')
              .map((object) => {
                const origin = objects.find((candidate) => candidate.id === (object.kind === 'measurement' || object.kind === 'circuit' ? 'campaign' : 'hypothesis'));
                if (!origin) return null;
                const x1 = origin.position.x + origin.size.width;
                const y1 = origin.position.y + origin.size.height / 2;
                const x2 = object.position.x;
                const y2 = object.position.y + object.size.height / 2;
                return <path key={object.id} d={`M${x1},${y1} C${x1 + 40},${y1} ${x2 - 40},${y2} ${x2},${y2}`} />;
              })}
          </svg>
          {objects.map((object) => (
            <WorkCard
              key={object.id}
              object={object}
              selected={selected === object.id}
              onSelect={() => setSelected(object.id)}
              onPointerDown={(event) => onCardPointerDown(event, object)}
              onPointerMove={onCardPointerMove}
              onPointerUp={onCardPointerUp}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function WorkCard({
  object,
  selected,
  onSelect,
  onPointerDown,
  onPointerMove,
  onPointerUp,
}: {
  object: WorkbenchObject;
  selected: boolean;
  onSelect: () => void;
  onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: () => void;
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
    >
      <header
        className="k-workbench-drag"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <span className="k-workbench-kind">{object.label}</span>
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
    </article>
  );
}
