'use client';

import { useEffect, useRef, useState } from 'react';
import { API_BASE, versionedApiPath } from './api';
import type { LedgerEvent } from '@/types';

export type StreamStatus = 'idle' | 'connecting' | 'live' | 'closed';

export function parseTelemetryPayload(event: unknown): LedgerEvent[] {
  if (!event || typeof event !== 'object') return [];
  const payload = event as { events?: unknown; event?: unknown };
  if (Array.isArray(payload.events)) {
    return payload.events.filter((item): item is LedgerEvent => Boolean(item) && Number.isFinite((item as LedgerEvent).id));
  }
  if (payload.event && Number.isFinite((payload.event as LedgerEvent).id)) {
    return [payload.event as LedgerEvent];
  }
  return [];
}

type TelemetryFrame = { domain?: string; events: LedgerEvent[] };

function payloadId(event: LedgerEvent): string | number | null {
  const payload = (event as { payload?: unknown }).payload;
  if (!payload || typeof payload !== 'object') return null;
  const id = (payload as { id?: unknown }).id;
  return typeof id === 'string' || typeof id === 'number' ? id : null;
}

/** Coalesce projection frames by domain + record id; ledger frames stay by id. */
export function coalesceTelemetryFrames(frames: TelemetryFrame[]): LedgerEvent[] {
  const byKey = new Map<string, LedgerEvent>();
  for (const frame of frames) {
    for (const event of frame.events) {
      const domain = frame.domain ?? 'ledger';
      const id = domain === 'ledger' ? event.id : payloadId(event) ?? event.id;
      byKey.set(`${domain}:${id}`, event);
    }
  }
  return Array.from(byKey.values()).sort((a, b) => a.id - b.id);
}

/** Merge by ledger id; the SSE replay cursor and poll overlap are idempotent. */
export function mergeSessionEvents(
  previous: LedgerEvent[],
  incoming: LedgerEvent[],
  cap = 200,
): LedgerEvent[] {
  const byId = new Map(previous.map((event) => [event.id, event]));
  for (const event of incoming) {
    if (event && typeof event.id === 'number') byId.set(event.id, event);
  }
  return Array.from(byId.values())
    .sort((a, b) => a.id - b.id)
    .slice(-cap);
}

export function hasSessionCookie(): boolean {
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some((part) => part.trim().startsWith('kubona_session='));
}

/**
 * One session telemetry stream. The caller owns the projection; this hook
 * owns reconnection and exposes whether the stream is authoritative so the
 * polling fallback can yield while frames arrive.
 */
export function shouldRequestSnapshot(previous: number, next: number): boolean {
  return previous > 0 && next > previous + 1;
}

export function useSessionStream(
  sessionId: string | null,
  onEvents: (events: LedgerEvent[]) => void,
  onGap?: (previous: number, next: number) => void,
): boolean {
  const [state, setState] = useState<{ sessionId: string | null; status: StreamStatus }>({
    sessionId: null,
    status: 'idle',
  });
  const revision = useRef(0);
  const handler = useRef(onEvents);
  const gapHandler = useRef(onGap);
  useEffect(() => {
    handler.current = onEvents;
    gapHandler.current = onGap;
  }, [onEvents, onGap]);

  useEffect(() => {
    if (!sessionId || typeof window === 'undefined' || typeof EventSource === 'undefined') {
      return;
    }
    if (!hasSessionCookie()) {
      return;
    }
    revision.current = 0;
    const source = new EventSource(
      `${API_BASE}${versionedApiPath(`/api/events/stream?session_id=${encodeURIComponent(sessionId)}`)}`,
      { withCredentials: true },
    );
    let errors = 0;
    let pending: TelemetryFrame[] = [];
    let frame = 0;
    const flush = () => {
      frame = 0;
      const frames = pending;
      pending = [];
      const batch = coalesceTelemetryFrames(frames);
      if (batch.length > 0) handler.current(batch);
    };
    const enqueue = (incoming: TelemetryFrame) => {
      pending.push(incoming);
      if (frame === 0) frame = requestAnimationFrame(flush);
    };
    const onTelemetry = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(String(event.data)) as {
          events?: unknown;
          event?: unknown;
          domain?: string;
          kind?: string;
          revision?: number;
        };
        const incoming = parseTelemetryPayload(payload);
        if (incoming.length === 0) return;
        errors = 0;
        if (typeof payload.revision === 'number' && Number.isFinite(payload.revision)) {
          const next = payload.revision;
          if (next <= revision.current) return;
          if (shouldRequestSnapshot(revision.current, next) && gapHandler.current) {
            gapHandler.current(revision.current, next);
          }
          revision.current = next;
        }
        enqueue({ domain: payload.domain, events: incoming });
      } catch {
        // Malformed frame: the next replay/poll reconciles it.
      }
    };
    source.addEventListener('telemetry', onTelemetry as EventListener);
    source.addEventListener('open', () => setState({ sessionId, status: 'live' }));
    source.onerror = () => {
      errors += 1;
      setState({ sessionId, status: 'connecting' });
      if (errors >= 3) {
        source.close();
        setState({ sessionId, status: 'closed' });
      }
    };
    return () => {
      source.removeEventListener('telemetry', onTelemetry as EventListener);
      source.close();
      if (frame !== 0) cancelAnimationFrame(frame);
      pending = [];
    };
  }, [sessionId]);

  return state.sessionId === sessionId && state.status === 'live';
}
