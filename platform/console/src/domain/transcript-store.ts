'use client';

import { useMemo, useSyncExternalStore } from 'react';
import type { ChatMessage } from '@/types';

type Updater<T> = T | ((previous: T) => T);

type TranscriptState = {
  messages: ChatMessage[];
  loading: boolean;
  streamingId: string | null;
  error: string | null;
};

type TranscriptStore = {
  getSnapshot: () => TranscriptState;
  subscribe: (listener: () => void) => () => void;
  setMessages: (value: Updater<ChatMessage[]>) => void;
  setLoading: (value: Updater<boolean>) => void;
  setStreamingId: (value: Updater<string | null>) => void;
  setError: (value: Updater<string | null>) => void;
};

const MAX_STORES = 8;
const stores = new Map<string, TranscriptStore>();

function createStore(): TranscriptStore {
  let state: TranscriptState = { messages: [], loading: false, streamingId: null, error: null };
  const listeners = new Set<() => void>();
  const publish = (patch: Partial<TranscriptState>) => {
    state = { ...state, ...patch };
    for (const listener of listeners) listener();
  };
  const resolve = <T,>(value: Updater<T>, previous: T): T =>
    typeof value === 'function' ? (value as (previous: T) => T)(previous) : value;
  return {
    getSnapshot: () => state,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    setMessages: (value) => publish({ messages: resolve(value, state.messages) }),
    setLoading: (value) => publish({ loading: resolve(value, state.loading) }),
    setStreamingId: (value) => publish({ streamingId: resolve(value, state.streamingId) }),
    setError: (value) => publish({ error: resolve(value, state.error) }),
  };
}

function getStore(key: string): TranscriptStore {
  const existing = stores.get(key);
  if (existing) return existing;
  const store = createStore();
  if (stores.size >= MAX_STORES) {
    const oldest = stores.keys().next().value;
    if (oldest !== undefined && oldest !== key) stores.delete(oldest);
  }
  stores.set(key, store);
  return store;
}

/** One transcript owner per session; composer and inspector share it. */
export function useTranscript(sessionId: string | null | undefined) {
  const key = sessionId || 'none';
  const store = useMemo(() => getStore(key), [key]);
  const state = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  return {
    ...state,
    errorMessage: state.error,
    setMessages: store.setMessages,
    setLoading: store.setLoading,
    setStreamingId: store.setStreamingId,
    setErrorMessage: store.setError,
  };
}

/** Test-only: module state must not leak between suites. */
export function resetTranscriptStoresForTests() {
  stores.clear();
}
