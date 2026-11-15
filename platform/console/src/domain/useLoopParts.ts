'use client';

import { useEffect } from 'react';
import { fetchLoopParts } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { subscribeInvalidation } from './invalidation';
import { useSharedResource } from './shared-resource';
import type { LoopPart } from '@/types';

/**
 * C1 — one loop's recorded run parts. Polls only while the run is live; the
 * session SSE stream's `loop_*` events (including `loop_part`) also trigger an
 * immediate refresh. An unreadable projection stays refused, never empty.
 */
export function useLoopParts(loopId: string | undefined, enabled: boolean, live: boolean) {
  const { data, status, error, refresh, setPollMs } = useSharedResource<LoopPart[]>(
    `loop-parts:${loopId ?? 'none'}`,
    async () => {
      if (!loopId || !enabled) return [];
      const out = await fetchLoopParts(loopId);
      if (!out.ok) throw new ApiFailureError(out.failure);
      return out.data;
    },
    { intervalMs: 5000 },
  );

  useEffect(() => {
    if (enabled) refresh();
  }, [enabled, refresh]);

  useEffect(() => {
    setPollMs(loopId && enabled && live ? 5000 : 0);
  }, [loopId, enabled, live, setPollMs]);

  useEffect(() => subscribeInvalidation(['loops'], refresh), [refresh]);

  return { parts: data ?? [], status, error, refresh };
}
