'use client';

import { fetchBackendStatus } from '@/lib/api';
import { usePollingResource } from './use-polling-resource';

export function useBackendProjection() {
  return usePollingResource(fetchBackendStatus, { intervalMs: 15_000 });
}
