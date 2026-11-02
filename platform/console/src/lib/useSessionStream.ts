'use client';

import { useEffect, useRef, useState } from 'react';
import { API_BASE, apiFetch, authHeaders, hasOperatorBearer, versionedApiPath } from './api';
import { useAuthPhase } from '@/domain/auth-state';
import { setSessionStreamLive } from '@/domain/stream-state';
import { domainsForEventTypes, publishInvalidation } from '@/domain/invalidation';
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

/**
 * One session telemetry stream. The caller owns the projection; this hook
 * owns reconnection and exposes whether the stream is authoritative so the
 * polling fallback can yield while frames arrive.
 */
export function shouldRequestSnapshot(previous: number, next: number): boolean {
  return previous > 0 && next > previous + 1;
}

export function streamRetryDelay(attempt: number, random = Math.random): number {
  const base = Math.min(30_000, 1_000 * 1.6 ** Math.max(0, attempt));
  return Math.round(base * (0.75 + random() * 0.5));
}

export type ParsedSseFrame = { event: string; data: string; id?: string };

function parseSseFrame(raw: string): ParsedSseFrame {
  let event = 'message';
  let id: string | undefined;
  const data: string[] = [];
  for (const line of raw.split('\n')) {
    if (!line || line.startsWith(':')) continue;
    if (line.startsWith('event:')) event = line.slice(6).trim();
    else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
    else if (line.startsWith('id:')) id = line.slice(3).trim();
  }
  return { event, data: data.join('\n'), id };
}

export function takeSseFrames(buffer: string): { frames: ParsedSseFrame[]; rest: string } {
  const text = buffer.replace(/\r\n/g, '\n');
  const frames: ParsedSseFrame[] = [];
  let rest = text;
  let split = rest.indexOf('\n\n');
  while (split >= 0) {
    const raw = rest.slice(0, split);
    rest = rest.slice(split + 2);
    if (!raw.trim()) continue;
    const parsed = parseSseFrame(raw);
    if (parsed.data || parsed.event !== 'message') frames.push(parsed);
    split = rest.indexOf('\n\n');
  }
  return { frames, rest };
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

  const authPhase = useAuthPhase();
  useEffect(() => {
    if (!sessionId || typeof window === 'undefined') return;
    if (!hasOperatorBearer() && authPhase !== 'authenticated') return;
    revision.current = 0;
    let stopped = false;
    let attempt = 0;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let controller: AbortController | null = null;
    let pending: TelemetryFrame[] = [];
    let frameHandle = 0;
    const flush = () => {
      frameHandle = 0;
      const frames = pending;
      pending = [];
      const batch = coalesceTelemetryFrames(frames);
      if (batch.length > 0) {
        handler.current(batch);
        publishInvalidation(domainsForEventTypes(batch.map((event) => String(event.event_type ?? ''))));
      }
    };
    const enqueue = (incoming: TelemetryFrame) => {
      pending.push(incoming);
      if (frameHandle === 0) frameHandle = requestAnimationFrame(flush);
    };
    const handleFrame = (frame: ParsedSseFrame) => {
      if (frame.event !== 'telemetry' || !frame.data) return;
      try {
        const payload = JSON.parse(frame.data) as {
          events?: unknown;
          event?: unknown;
          domain?: string;
          kind?: string;
          revision?: number;
        };
        const incoming = parseTelemetryPayload(payload);
        if (incoming.length === 0) return;
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
    const scheduleReconnect = () => {
      if (stopped) return;
      const delay = streamRetryDelay(attempt++);
      setSessionStreamLive(false);
      setState({ sessionId, status: 'connecting' });
      retryTimer = setTimeout(() => { void connect(); }, delay);
    };
    const connect = async (): Promise<void> => {
      if (stopped) return;
      controller = new AbortController();
      const cursor = revision.current > 0 ? `&last_event_id=${revision.current}` : '';
      try {
        const res = await apiFetch(
          `${API_BASE}${versionedApiPath(`/api/events/stream?session_id=${encodeURIComponent(sessionId)}${cursor}`)}`,
          { signal: controller.signal, headers: authHeaders(), cache: 'no-store' },
        );
        if (!res.ok) throw new Error(`stream refused (HTTP ${res.status})`);
        if (!res.body) throw new Error('stream body unavailable');
        attempt = 0;
        setSessionStreamLive(true);
        setState({ sessionId, status: 'live' });
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (!stopped) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const parsed = takeSseFrames(buffer);
          buffer = parsed.rest;
          for (const frame of parsed.frames) handleFrame(frame);
        }
      } catch {
        // Abort on unmount is expected; everything else retries below.
      }
      if (stopped) return;
      scheduleReconnect();
    };
    void connect();
    return () => {
      stopped = true;
      setSessionStreamLive(false);
      controller?.abort();
      if (retryTimer) clearTimeout(retryTimer);
      if (frameHandle !== 0) cancelAnimationFrame(frameHandle);
      pending = [];
    };
  }, [sessionId, authPhase]);

  return state.sessionId === sessionId && state.status === 'live';
}
