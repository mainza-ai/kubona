'use client';

import { useSyncExternalStore } from 'react';

export type AuthPhase = 'anonymous' | 'checking' | 'authenticated' | 'expired' | 'forbidden';

let phase: AuthPhase = 'anonymous';
let multiLabAuthRequired = false;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

export function getAuthPhase(): AuthPhase {
  return phase;
}

export function setAuthPhase(next: AuthPhase): void {
  if (next === phase) return;
  phase = next;
  emit();
}

export function isMultiLabAuthRequired(): boolean {
  return multiLabAuthRequired;
}

export function setMultiLabAuthRequired(required: boolean): void {
  if (required === multiLabAuthRequired) return;
  multiLabAuthRequired = required;
  emit();
}

export function shouldDeferProtectedRead(hasBearer = false): boolean {
  return multiLabAuthRequired && phase !== 'authenticated' && !hasBearer;
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

export function useMultiLabAuthRequired(): boolean {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    isMultiLabAuthRequired,
    isMultiLabAuthRequired,
  );
}

/** Test-only: module state must not leak between suites. */
export function resetAuthStateForTests(): void {
  phase = 'anonymous';
  multiLabAuthRequired = false;
  listeners.clear();
}
