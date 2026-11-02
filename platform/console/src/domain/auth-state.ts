'use client';

import { useSyncExternalStore } from 'react';

export type AuthPhase = 'anonymous' | 'checking' | 'authenticated' | 'expired' | 'forbidden';

let phase: AuthPhase = 'anonymous';
const listeners = new Set<() => void>();

export function getAuthPhase(): AuthPhase {
  return phase;
}

export function setAuthPhase(next: AuthPhase): void {
  if (next === phase) return;
  phase = next;
  for (const listener of listeners) listener();
}

export function useAuthPhase(): AuthPhase {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getAuthPhase,
    getAuthPhase,
  );
}

/** Test-only: module state must not leak between suites. */
export function resetAuthStateForTests(): void {
  phase = 'anonymous';
  listeners.clear();
}
