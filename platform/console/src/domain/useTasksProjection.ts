'use client';

import { useCallback, useEffect } from 'react';
import { fetchTasksResult } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useSharedResource } from './shared-resource';
import { subscribeInvalidation } from './invalidation';
import { useSessionStreamLive } from './stream-state';
import type { Task } from '@/types';

/** One page-level owner for the session task projection. */
export function useTasksProjection(sessionId: string | null) {
  const load = useCallback(async () => {
    if (!sessionId) return [];
    const result = await fetchTasksResult(sessionId);
    if (!result.ok) throw new ApiFailureError(result.failure);
    return result.data;
  }, [sessionId]);
  const { data, status, error, failure, refresh, setPollMs } = useSharedResource<Task[]>(`tasks:${sessionId ?? 'none'}`, load, { intervalMs: 15000 });
  const live = useSessionStreamLive();
  useEffect(() => { setPollMs(live ? 0 : 15000); }, [live, setPollMs]);
  useEffect(() => subscribeInvalidation(['tasks'], refresh), [refresh]);
  return { tasks: data ?? [], status, error, failure, refresh };
}
