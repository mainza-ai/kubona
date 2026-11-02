'use client';

import { useCallback, useEffect, useMemo, useSyncExternalStore } from 'react';
import { createPollingStore, type PollingStore, type ResourceState } from './resource-store';

type Options<T> = {
  intervalMs: number;
  hiddenIntervalMs?: number;
  maxBackoffMs?: number;
  shouldContinue?: (data: T) => boolean;
};

type Entry<T> = {
  key: string;
  store: PollingStore<T>;
};

const registry = new Map<string, Entry<unknown>>();
const refCounts = new Map<string, number>();

function acquire<T>(key: string, load: () => Promise<T>, options: Options<T>): Entry<T> {
  const existing = registry.get(key) as Entry<T> | undefined;
  if (existing) {
    if ((refCounts.get(key) ?? 0) === 0) existing.store.reset();
    existing.store.setLoader(load);
    return existing;
  }
  const entry: Entry<T> = {
    key,
    store: createPollingStore(load, options),
  };
  registry.set(key, entry as Entry<unknown>);
  return entry;
}

/** One ref-counted store per domain key. Stops and resets at refs=0. */
export function useSharedResource<T>(key: string, load: () => Promise<T>, options: Options<T>) {
  // Creates an idle store; network starts only in subscribe().
  const entry = useMemo(() => acquire(key, load, options), [key]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { entry.store.setLoader(load); }, [entry, load]);

  const subscribe = useCallback((listener: () => void) => {
    refCounts.set(key, (refCounts.get(key) ?? 0) + 1);
    const unsubscribe = entry.store.subscribe(listener);
    return () => {
      unsubscribe();
      const next = Math.max(0, (refCounts.get(key) ?? 1) - 1);
      refCounts.set(key, next);
      if (next === 0) {
        entry.store.dispose();
        entry.store.reset();
      }
    };
  }, [entry, key]);

  const getSnapshot = useCallback((): ResourceState<T> => entry.store.getSnapshot(), [entry]);
  const state = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { ...state, refresh: entry.store.refresh, setPollMs: entry.store.setPollMs };
}

/** Test-only: shared module state must not leak across isolated suites. */
export function resetSharedResourcesForTests() {
  for (const entry of registry.values()) {
    entry.store.dispose();
    entry.store.reset();
  }
  registry.clear();
  refCounts.clear();
}
