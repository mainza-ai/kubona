'use client';

import { useSyncExternalStore } from 'react';
import { apiFailureFrom, type ApiFailure } from '@/lib/api-errors';

export class ResourceRefusal<T> extends Error {
  constructor(
    message: string,
    readonly data: T | null = null,
  ) {
    super(message);
    this.name = 'ResourceRefusal';
  }
}

export type ResourceStatus = 'idle' | 'loading' | 'ready' | 'stale' | 'refused';

export type ResourceState<T> = {
  status: ResourceStatus;
  data: T | null;
  error: string | null;
  failure: ApiFailure | null;
  revision: number;
  updatedAt: number;
  /** Current wait before the next poll; grows on failure. */
  intervalMs: number;
};

export type PollingStore<T> = {
  getSnapshot: () => ResourceState<T>;
  subscribe: (listener: () => void) => () => void;
  refresh: () => void;
  setLoader: (load: () => Promise<T>) => void;
  setPollMs: (ms: number) => void;
  reset: () => void;
  dispose: () => void;
};

type Options<T> = {
  intervalMs: number;
  hiddenIntervalMs?: number;
  maxBackoffMs?: number;
  /** A terminal resource (settled job, closed incident) stops polling. */
  shouldContinue?: (data: T) => boolean;
};

/** Ends polling without treating the resource as a transport failure. */
export class StopPolling extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'StopPolling';
  }
}

/**
 * One owner per shared resource. Polling is visibility-gated, serialized,
 * backoff-bounded, and never invents data on failure: the previous snapshot
 * stays with status `stale` and the error attached.
 */
export function createPollingStore<T>(
  load: () => Promise<T>,
  { intervalMs, hiddenIntervalMs = 30_000, maxBackoffMs = 30_000, shouldContinue }: Options<T>,
): PollingStore<T> {
  let state: ResourceState<T> = {
    status: 'idle',
    data: null,
    error: null,
    failure: null,
    revision: 0,
    updatedAt: 0,
    intervalMs,
  };
  const listeners = new Set<() => void>();
  let loader = load;
  let baseIntervalMs = intervalMs;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inFlight = false;
  let pendingRefresh = false;
  let active = false;
  let generation = 0;

  const publish = (patch: Partial<ResourceState<T>>) => {
    state = { ...state, ...patch };
    for (const listener of listeners) listener();
  };
  const clearTimer = () => {
    if (timer) clearTimeout(timer);
    timer = null;
  };
  const schedule = (ms: number) => {
    clearTimer();
    if (active) timer = setTimeout(tick, ms);
  };
  const hidden = () => typeof document !== 'undefined' && document.hidden;

  const tick = async () => {
    if (!active) return;
    if (hidden()) {
      if (baseIntervalMs > 0) schedule(hiddenIntervalMs);
      return;
    }
    if (inFlight) return;
    inFlight = true;
    const gen = generation;
    if (state.data === null) publish({ status: 'loading', error: null });
    try {
      const data = await loader();
      if (!active || gen !== generation) return;
      publish({
        status: 'ready',
        data,
        error: null,
        failure: null,
        revision: state.revision + 1,
        updatedAt: Date.now(),
        intervalMs: baseIntervalMs,
      });
      if (shouldContinue?.(data) === false || baseIntervalMs <= 0) clearTimer();
      else schedule(baseIntervalMs);
    } catch (err) {
      if (!active || gen !== generation) return;
      if (err instanceof StopPolling) {
        publish({ status: state.data === null ? 'refused' : 'stale', error: err.message, failure: null });
        clearTimer();
        return;
      }
      const next = Math.min(maxBackoffMs, Math.max(intervalMs, Math.round(state.intervalMs * 1.5)));
      const refusal = err instanceof ResourceRefusal ? (err.data as T | null) : null;
      const data = refusal ?? state.data;
      publish({
        status: data === null ? 'refused' : 'stale',
        data,
        error: err instanceof Error ? err.message : String(err),
        failure: apiFailureFrom(err),
        intervalMs: next,
      });
      if (baseIntervalMs > 0) schedule(next);
      else clearTimer();
    } finally {
      if (gen === generation) {
        inFlight = false;
        if (pendingRefresh && active) {
          pendingRefresh = false;
          clearTimer();
          void tick();
        }
      }
    }
  };

  const onVisibility = () => {
    if (!active) return;
    if (!hidden() && baseIntervalMs > 0) {
      clearTimer();
      void tick();
    } else {
      clearTimer();
    }
  };

  const start = () => {
    if (active) return;
    generation += 1;
    inFlight = false;
    pendingRefresh = false;
    active = true;
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);
    void tick();
  };
  const stop = () => {
    generation += 1;
    inFlight = false;
    pendingRefresh = false;
    active = false;
    clearTimer();
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
  };

  return {
    getSnapshot: () => state,
    subscribe: (listener) => {
      listeners.add(listener);
      if (listeners.size === 1) start();
      return () => {
        listeners.delete(listener);
        if (listeners.size === 0) stop();
      };
    },
    setLoader: (next) => { loader = next; },
    setPollMs: (ms) => {
      baseIntervalMs = Math.max(0, ms);
      if (!active) return;
      clearTimer();
      if (baseIntervalMs > 0) schedule(baseIntervalMs);
    },
    reset: () => {
      state = {
        status: 'idle',
        data: null,
        error: null,
        failure: null,
        revision: 0,
        updatedAt: 0,
        intervalMs: baseIntervalMs,
      };
    },
    refresh: () => {
      if (!active) return;
      if (inFlight) {
        pendingRefresh = true;
        return;
      }
      pendingRefresh = false;
      clearTimer();
      void tick();
    },
    dispose: stop,
  };
}

/** Read a domain store without letting a component own its fetch state. */
export function useResource<T>(store: PollingStore<T>): ResourceState<T> {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
