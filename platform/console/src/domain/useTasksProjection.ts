'use client';

import { useCallback } from 'react';
import { fetchTasksResult } from '@/lib/api';
import { useSharedResource } from './shared-resource';
import type { Task } from '@/types';

/** One page-level owner for the session task projection. */
export function useTasksProjection(sessionId: string | null) {
  const load = useCallback(async () => {
    if (!sessionId) return [];
    const result = await fetchTasksResult(sessionId);
    if (!result.ok) throw new Error(result.error);
    return result.data;
  }, [sessionId]);
  const { data, status, error, refresh } = useSharedResource<Task[]>(`tasks:${sessionId ?? 'none'}`, load, { intervalMs: 15000 });
  return { tasks: data ?? [], status, error, refresh };
}
