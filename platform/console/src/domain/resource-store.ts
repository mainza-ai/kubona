'use client';

import { useSyncExternalStore } from 'react';

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
    revision: 0,
    updatedAt: 0,
    intervalMs,
  };
  const listeners = new Set<() => void>();
  let loader = load;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inFlight = false;
  let active = false;

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
      schedule(hiddenIntervalMs);
      return;
    }
    if (inFlight) return;
    inFlight = true;
    if (state.data === null) publish({ status: 'loading', error: null });
    try {
      const data = await loader();
      if (!active) return;
      publish({
        status: 'ready',
        data,
        error: null,
        revision: state.revision + 1,
        updatedAt: Date.now(),
        intervalMs,
      });
      if (shouldContinue?.(data) === false) clearTimer();
      else schedule(intervalMs);
    } catch (err) {
      if (!active) return;
      if (err instanceof StopPolling) {
        publish({ status: state.data === null ? 'refused' : 'stale', error: err.message });
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
        intervalMs: next,
      });
      schedule(next);
    } finally {
      inFlight = false;
    }
  };

  const onVisibility = () => {
    if (!active) return;
    if (!hidden()) {
      clearTimer();
      void tick();
    } else {
      clearTimer();
    }
  };

  const start = () => {
    if (active) return;
    active = true;
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);
    void tick();
  };
  const stop = () => {
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
    refresh: () => {
      if (!active) return;
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
