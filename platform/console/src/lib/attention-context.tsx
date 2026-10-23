'use client';

// Attention Director context (P4): one shared alert channel from any surface
// to the CheckInStrip, with presence gating and ambient sound. Safe without
// a provider (no-op stub) so surfaces render standalone in tests.

import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { ambientAudio } from './ambient-audio';
import {
  routeAlert,
  shouldCollapse,
  createPrecisionTracker,
  recordOutcome,
  shouldTighten,
  type AlertLevel,
  type DirectorInput,
  type DirectorRoute,
  type PrecisionTracker,
  type PresenceMode,
} from './attention';

export interface DirectorAlert extends DirectorRoute {
  id: number;
  message: string;
  source: string;
  at: number;
  /** Consecutive collapsed duplicates (R-11): same source+level within the window. */
  duplicates: number;
}

interface AttentionContextValue {
  alert: DirectorAlert | null;
  presence: PresenceMode;
  setPresence: (p: PresenceMode) => void;
  notify: (input: DirectorInput) => void;
  clear: () => void;
  /** Resolve the current alert, recording whether the human acted on it.
   * Feeds per-source precision; sources below 0.8 precision get tighter
   * thresholds (R-11 alarm-precision tracking). */
  resolve: (acted: boolean) => void;
  /** Sources currently under tightened thresholds. */
  tightened: string[];
  muted: boolean;
  toggleMute: () => void;
}

const AttentionContext = createContext<AttentionContextValue | null>(null);

const STUB: AttentionContextValue = {
  alert: null,
  presence: 'hands',
  setPresence: () => {},
  notify: () => {},
  clear: () => {},
  resolve: () => {},
  tightened: [],
  muted: true,
  toggleMute: () => {},
};

export function useAttention(): AttentionContextValue {
  return useContext(AttentionContext) ?? STUB;
}

export function AttentionProvider({ children }: { children: React.ReactNode }) {
  const [alert, setAlert] = useState<DirectorAlert | null>(null);
  const [presence, setPresence] = useState<PresenceMode>('hands');
  const [muted, setMuted] = useState(ambientAudio.muted);
  const idRef = useRef(0);

  // Autoplay policy: unlock on first gesture, once. Headless-safe.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const unlock = () => ambientAudio.unlock();
    window.addEventListener('pointerdown', unlock, { once: true });
    return () => window.removeEventListener('pointerdown', unlock);
  }, []);

  const [tracker, setTracker] = useState<PrecisionTracker>(() => createPrecisionTracker());
  const trackerRef = useRef(tracker);
  trackerRef.current = tracker;
  const alertRef = useRef<DirectorAlert | null>(null);
  alertRef.current = alert;

  const notify = useCallback(
    (input: DirectorInput) => {
      const route = routeAlert(input, presence);
      if (route.suppressed && route.channel === 'ambient') return;
      // R-11 auto-tighten: sources with precision below 0.8 escalate one
      // band so the director stops crying wolf at glance level.
      const routed =
        trackerRef.current && shouldTighten(trackerRef.current, input.source) && route.channel === 'glance'
          ? { ...route, channel: 'nudge' as const, reason: `${route.reason} (tightened: low precision)` }
          : route;
      setAlert((prev) => {
        // R-11 duplicate collapse: same source+level inside the window
        // refreshes the visible alert instead of stacking a new one.
        if (
          prev &&
          shouldCollapse(
            { key: { source: prev.source, level: prev.level }, at: prev.at },
            { source: input.source, level: routed.level },
            Date.now(),
          )
        ) {
          return { ...prev, at: Date.now(), duplicates: prev.duplicates + 1 };
        }
        idRef.current += 1;
        return {
          ...routed,
          id: idRef.current,
          message: `${input.source}: ${routed.reason}`,
          source: input.source,
          at: Date.now(),
          duplicates: 0,
        };
      });
      ambientAudio.play(routed.sound);
    },
    [presence],
  );

  const clear = useCallback(() => setAlert(null), []);

  const resolve = useCallback((acted: boolean) => {
    const prev = alertRef.current;
    if (prev && trackerRef.current) {
      const next: PrecisionTracker = {
        acted: { ...trackerRef.current.acted },
        dismissed: { ...trackerRef.current.dismissed },
      };
      recordOutcome(next, prev.source, acted);
      setTracker(next);
    }
    setAlert(null);
  }, []);

  const tightened = useMemo(() => {
    const sources = new Set<string>([...Object.keys(tracker.acted), ...Object.keys(tracker.dismissed)]);
    return [...sources].filter((s) => shouldTighten(tracker, s));
  }, [tracker]);

  const toggleMute = useCallback(() => {
    setMuted((m) => {
      ambientAudio.setMuted(!m);
      return !m;
    });
  }, []);

  const value = useMemo(
    () => ({ alert, presence, setPresence, notify, clear, resolve, tightened, muted, toggleMute }),
    [alert, presence, notify, clear, resolve, tightened, muted, toggleMute],
  );
  return <AttentionContext.Provider value={value}>{children}</AttentionContext.Provider>;
}

export type { AlertLevel, DirectorInput, PresenceMode };
