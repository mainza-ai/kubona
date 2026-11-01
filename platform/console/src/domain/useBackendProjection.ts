'use client';

import { fetchBackendStatus } from '@/lib/api';
import { useSharedResource } from './shared-resource';

export function useBackendProjection() {
  return useSharedResource('backend', fetchBackendStatus, { intervalMs: 15_000 });
}
