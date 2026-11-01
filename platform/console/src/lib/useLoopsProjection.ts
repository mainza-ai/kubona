'use client';

import { useCallback, useState } from 'react';
import { fetchLoops } from './api';
import type { Loop } from '@/types';

/** Owns the loop projection; every refresh still reads the server's state. */
export function useLoopsProjection(sessionId: string | null) {
  const [loops, setLoops] = useState<Loop[]>([]);
  const refresh = useCallback(async () => {
    const next = await fetchLoops(sessionId || undefined);
    setLoops(next);
    return next;
  }, [sessionId]);
  return { loops, setLoops, refresh };
}
