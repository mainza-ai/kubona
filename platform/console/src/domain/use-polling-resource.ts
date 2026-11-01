'use client';

import { useEffect, useState } from 'react';
import { createPollingStore, useResource, type PollingStore } from './resource-store';

/**
 * Own a polling resource for one mounted surface. Use a module store when a
 * resource truly has multiple consumers; this keeps single-consumer surfaces
 * from leaking stale test/app state across mounts.
 */
export function usePollingResource<T>(
  load: () => Promise<T>,
  options: { intervalMs: number; hiddenIntervalMs?: number; maxBackoffMs?: number },
) {
  const [current] = useState<PollingStore<T>>(() => createPollingStore(load, options));
  useEffect(() => () => current.dispose(), [current]);
  const state = useResource(current);
  return { ...state, refresh: current.refresh };
}
