'use client';

import { useSyncExternalStore } from 'react';
import { API_PROBLEM_EVENT, type ApiProblem } from '@/lib/api';
import { boundedSignature } from '@/lib/api-problem';

/**
 * 3.9 — session-scoped API-problem state for the StatusStrip chip.
 *
 * The latest non-dismissed problem is held here. Dismissal is keyed by the
 * problem signature in sessionStorage: the same refusal does not nag again
 * this session, while a different path/status/message surfaces immediately.
 * A blocking auth state is NOT stored here — that keeps its role="alert".
 */

export const API_PROBLEM_DISMISSED_KEY = 'kubona.api-problems.dismissed.v1';
export const API_PROBLEM_DISMISSED_CAP = 100;

let latest: ApiProblem | null = null;
let dismissed: Set<string> | null = null;
let listening = false;
const listeners = new Set<() => void>();

function readDismissed(): Set<string> {
  if (dismissed) return dismissed;
  dismissed = new Set();
  try {
    if (typeof window === 'undefined') return dismissed;
    const raw = window.sessionStorage.getItem(API_PROBLEM_DISMISSED_KEY);
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        for (const value of parsed) {
          if (typeof value === 'string' && value) dismissed.add(value);
        }
      }
    }
  } catch {
    // No storage / corrupt JSON: dismissal degrades to in-memory only.
  }
  return dismissed;
}

function writeDismissed(): void {
  try {
    if (typeof window === 'undefined') return;
    const values = [...readDismissed()].slice(-API_PROBLEM_DISMISSED_CAP);
    window.sessionStorage.setItem(API_PROBLEM_DISMISSED_KEY, JSON.stringify(values));
  } catch {
    // Private mode/quota: keep the in-memory set.
  }
}

function emit(): void {
  for (const listener of listeners) listener();
}

function onProblem(event: Event): void {
  const detail = (event as CustomEvent<ApiProblem>).detail;
  if (!detail || typeof detail.path !== 'string') return;
  if (readDismissed().has(boundedSignature(detail))) return;
  latest = detail;
  emit();
}

function ensureListener(): void {
  if (listening || typeof window === 'undefined' || typeof window.addEventListener !== 'function') return;
  window.addEventListener(API_PROBLEM_EVENT, onProblem);
  listening = true;
}

function subscribe(listener: () => void): () => void {
  ensureListener();
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): ApiProblem | null {
  return latest;
}

function getServerSnapshot(): ApiProblem | null {
  return null;
}

export function dismissApiProblem(problem: ApiProblem | null = latest): void {
  if (!problem) return;
  readDismissed().add(boundedSignature(problem));
  writeDismissed();
  if (latest === problem) latest = null;
  emit();
}

/** Test hook: forget the latest problem, dismissals, and the window listener. */
export function resetApiProblemForTests(): void {
  if (listening && typeof window !== 'undefined' && typeof window.removeEventListener === 'function') {
    window.removeEventListener(API_PROBLEM_EVENT, onProblem);
  }
  listening = false;
  latest = null;
  dismissed = null;
  listeners.clear();
}

export function useApiProblem(): ApiProblem | null {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
