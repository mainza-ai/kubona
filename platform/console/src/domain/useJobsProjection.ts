'use client';

import { useCallback } from 'react';
import { fetchJobsResult } from '@/lib/api';
import { useSharedResource } from './shared-resource';
import type { Job } from '@/types';

/** One page-level owner for the session job projection. */
export function useJobsProjection(sessionId: string | null) {
  const load = useCallback(async () => {
    if (!sessionId) return [];
    const result = await fetchJobsResult(sessionId);
    if (!result.ok) throw new Error(result.error);
    return result.data;
  }, [sessionId]);
  const { data, status, error, refresh } = useSharedResource<Job[]>(`jobs:${sessionId ?? 'none'}`, load, { intervalMs: 5000 });
  return { jobs: data ?? [], status, error, refresh };
}
