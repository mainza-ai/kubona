'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { LedgerEvent } from '@/types';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export interface TelemetryState {
  events: LedgerEvent[];
  connected: boolean;
  error: string | null;
  lastHeartbeat: string | null;
}

// Transport decision (Q-29 follow-up, recorded 2026-10-09): REST polling is
// the normative console transport (page-level 3.5s loops + radar 1s poll).
// Rationale: oversight cadence needs no sub-second push; polling has no
// sticky connections through proxies and degrades obviously. This SSE hook
// against /api/events/stream is retained as progressive enhancement for the
// fleet radar ONLY, pending a load measurement (concurrent EventSource
// connections, proxy buffering behavior) — it is staged, not supported, and
// no surface may depend on it until that measurement lands.
export function useTelemetry(sessionId?: string, options: { enabled?: boolean } = {}) {
  const { enabled = true } = options;
  const [state, setState] = useState<TelemetryState>({
    events: [],
    connected: false,
    error: null,
    lastHeartbeat: null,
  });

  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const backoffRef = useRef<number>(1000);

  const connect = useCallback(() => {
    if (!enabled || typeof window === 'undefined') return;

    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }

    const url = sessionId
      ? `${API_BASE}/api/events/stream?session_id=${encodeURIComponent(sessionId)}`
      : `${API_BASE}/api/events/stream`;

    try {
      const es = new EventSource(url);
      eventSourceRef.current = es;

      es.addEventListener('connected', (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          setState((prev) => ({
            ...prev,
            connected: true,
            error: null,
            lastHeartbeat: data.timestamp || new Date().toISOString(),
          }));
          backoffRef.current = 1000; // Reset exponential backoff on success
        } catch {
          // handshake ack
          setState((prev) => ({ ...prev, connected: true, error: null }));
        }
      });

      es.addEventListener('telemetry', (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          if (Array.isArray(data.events)) {
            setState((prev) => {
              const existingIds = new Set(prev.events.map((ev) => ev.id));
              const newEvents = data.events.filter((ev: LedgerEvent) => !existingIds.has(ev.id));
              return {
                ...prev,
                events: [...newEvents, ...prev.events].slice(0, 100),
                lastHeartbeat: data.timestamp || new Date().toISOString(),
              };
            });
          }
        } catch (err) {
          console.error('Error parsing telemetry SSE event:', err);
        }
      });

      es.onerror = () => {
        setState((prev) => ({ ...prev, connected: false, error: 'Connection interrupted' }));
        es.close();
        eventSourceRef.current = null;

        // Auto-reconnect with exponential backoff (max 15s)
        const delay = backoffRef.current;
        backoffRef.current = Math.min(backoffRef.current * 1.5, 15000);
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, delay);
      };
    } catch (err) {
      setState((prev) => ({ ...prev, connected: false, error: String(err) }));
    }
  }, [sessionId, enabled]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, [connect]);

  return {
    ...state,
    reconnect: connect,
  };
}
