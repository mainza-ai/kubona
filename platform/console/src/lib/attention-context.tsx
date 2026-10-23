'use client';

// Attention Director context (P4): one shared alert channel from any surface
// to the CheckInStrip, with presence gating and ambient sound. Safe without
// a provider (no-op stub) so surfaces render standalone in tests.

import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { ambientAudio } from './ambient-audio';
import { routeAlert, type AlertLevel, type DirectorInput, type DirectorRoute, type PresenceMode } from './attention';

export interface DirectorAlert extends DirectorRoute {
  id: number;
  message: string;
  source: string;
  at: number;
}

interface AttentionContextValue {
  alert: DirectorAlert | null;
  presence: PresenceMode;
  setPresence: (p: PresenceMode) => void;
  notify: (input: DirectorInput) => void;
  clear: () => void;
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

  const notify = useCallback(
    (input: DirectorInput) => {
      const route = routeAlert(input, presence);
      if (route.suppressed && route.channel === 'ambient') return;
      idRef.current += 1;
      setAlert({
        ...route,
        id: idRef.current,
        message: `${input.source}: ${route.reason}`,
        source: input.source,
        at: Date.now(),
      });
      ambientAudio.play(route.sound);
    },
    [presence],
  );

  const clear = useCallback(() => setAlert(null), []);

  const toggleMute = useCallback(() => {
    setMuted((m) => {
      ambientAudio.setMuted(!m);
      return !m;
    });
  }, []);

  const value = useMemo(
    () => ({ alert, presence, setPresence, notify, clear, muted, toggleMute }),
    [alert, presence, notify, clear, muted, toggleMute],
  );
  return <AttentionContext.Provider value={value}>{children}</AttentionContext.Provider>;
}

export type { AlertLevel, DirectorInput, PresenceMode };
