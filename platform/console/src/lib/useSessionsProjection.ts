'use client';

import { useCallback, useState } from 'react';
import type { Session } from '@/types';

/** Owns the session-list projection. The server remains authoritative. */
export function useSessionsProjection() {
  const [sessions, setSessions] = useState<Session[]>([]);

  const upsert = useCallback((session: Session, prepend = false) => {
    setSessions((previous) => {
      const index = previous.findIndex((item) => item.id === session.id);
      if (index === -1) return prepend ? [session, ...previous] : [...previous, session];
      const next = [...previous];
      next[index] = session;
      return next;
    });
  }, []);

  return { sessions, setSessions, upsert };
}
