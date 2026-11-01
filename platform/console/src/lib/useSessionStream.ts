'use client';

import { useEffect, useRef, useState } from 'react';
import { API_BASE } from './api';
import type { LedgerEvent } from '@/types';

export type StreamStatus = 'idle' | 'connecting' | 'live' | 'closed';

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
      `${API_BASE}/api/events/stream?session_id=${encodeURIComponent(sessionId)}`,
      { withCredentials: true },
    );
    let errors = 0;
    const onTelemetry = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(String(event.data)) as {
          events?: LedgerEvent[];
          domain?: string;
          kind?: string;
          revision?: number;
        };
        if (!Array.isArray(payload.events) || payload.events.length === 0) return;
        errors = 0;
        if (typeof payload.revision === 'number' && Number.isFinite(payload.revision)) {
          const next = payload.revision;
          if (next <= revision.current) return;
          if (shouldRequestSnapshot(revision.current, next) && gapHandler.current) {
            gapHandler.current(revision.current, next);
          }
          revision.current = next;
        }
        handler.current(payload.events);
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
    };
  }, [sessionId]);

  return state.sessionId === sessionId && state.status === 'live';
}
