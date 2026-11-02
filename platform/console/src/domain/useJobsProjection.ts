'use client';

import { useCallback, useEffect } from 'react';
import { fetchJobsResult } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useSharedResource } from './shared-resource';
import { subscribeInvalidation } from './invalidation';
import { useSessionStreamLive } from './stream-state';
import type { Job } from '@/types';

/** One page-level owner for the session job projection. */
export function useJobsProjection(sessionId: string | null) {
  const load = useCallback(async () => {
    if (!sessionId) return [];
    const result = await fetchJobsResult(sessionId);
    if (!result.ok) throw new ApiFailureError(result.failure);
    return result.data;
  }, [sessionId]);
  const { data, status, error, failure, refresh, setPollMs } = useSharedResource<Job[]>(`jobs:${sessionId ?? 'none'}`, load, { intervalMs: 5000 });
  const live = useSessionStreamLive();
  useEffect(() => { setPollMs(live ? 0 : 5000); }, [live, setPollMs]);
  useEffect(() => subscribeInvalidation(['jobs'], refresh), [refresh]);
  return { jobs: data ?? [], status, error, failure, refresh };
}
