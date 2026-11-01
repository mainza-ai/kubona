'use client';

import { useCallback } from 'react';
import { fetchJobs } from '@/lib/api';
import { usePollingResource } from './use-polling-resource';
import type { Job } from '@/types';

/** One page-level owner for the session job projection. */
export function useJobsProjection(sessionId: string | null) {
  const load = useCallback(async () => (sessionId ? fetchJobs(sessionId) : []), [sessionId]);
  const { data, status, error, refresh } = usePollingResource<Job[]>(load, { intervalMs: 5000 });
  return { jobs: data ?? [], status, error, refresh };
}
