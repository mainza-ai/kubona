'use client';

import { useSyncExternalStore } from 'react';

let live = false;
const listeners = new Set<() => void>();

export function setSessionStreamLive(next: boolean): void {
  if (live === next) return;
  live = next;
  for (const listener of listeners) listener();
}

export function getSessionStreamLive(): boolean {
  return live;
}

export function useSessionStreamLive(): boolean {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getSessionStreamLive,
    getSessionStreamLive,
  );
}

export function resetStreamStateForTests(): void {
  live = false;
  listeners.clear();
}
