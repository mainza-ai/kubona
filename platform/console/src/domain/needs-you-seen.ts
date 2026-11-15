'use client';

import { useCallback, useSyncExternalStore } from 'react';

/**
 * Seen-state for the "Needs you" tray (report §3.1: seen-state lives in its
 * own store). A tray item is new until the reviewer opens the tray with it
 * present; the badge counts new items only.
 *
 * The store is device-local presentation state (tier A of the settings
 * vocabulary): losing it re-raises items, it never hides work. Nothing here
 * decides anything — a decided item disappears because the server stops
 * returning it, not because it was marked seen.
 *
 * Implemented as an external store so the first render can hydrate from an
 * empty server snapshot and then adopt sessionStorage without a
 * setState-in-effect cascade or a hydration mismatch.
 */

export const NEEDS_YOU_SEEN_KEY = 'kubona.needs-you.seen.v1';
/** Bounded so a long-lived browser cannot grow the store without limit. */
export const NEEDS_YOU_SEEN_CAP = 500;

const EMPTY: Set<string> = new Set();

let cache: Set<string> | null = null;
const listeners = new Set<() => void>();

function readStorage(): string | null {
  try {
    if (typeof window === 'undefined') return null;
    return window.sessionStorage.getItem(NEEDS_YOU_SEEN_KEY);
  } catch {
    return null;
  }
}

function writeStorage(value: string): void {
  try {
    if (typeof window === 'undefined') return;
    window.sessionStorage.setItem(NEEDS_YOU_SEEN_KEY, value);
  } catch {
    // Private-mode/quota refusal: seen-state degrades to in-memory only.
  }
}

/** Parse a serialized set; malformed content is empty (raise, never hide). */
export function parseSeenSignatures(raw: string | null): Set<string> {
  if (!raw) return new Set();
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((value): value is string => typeof value === 'string' && value !== ''));
  } catch {
    return new Set();
  }
}

export function serializeSeenSignatures(seen: ReadonlySet<string>): string {
  return JSON.stringify([...seen].slice(-NEEDS_YOU_SEEN_CAP));
}

/**
 * Drop signatures that are no longer active so the set stays bounded and a
 * returning obligation counts as new again. `null` means "list not loaded
 * yet" — pruning then would erase the viewer's history on every reload.
 */
export function pruneSeenSignatures(seen: ReadonlySet<string>, active: readonly string[] | null): Set<string> {
  if (active === null) return new Set(seen);
  const live = new Set(active);
  const out = new Set<string>();
  for (const signature of seen) {
    if (live.has(signature)) out.add(signature);
  }
  return out;
}

function getSnapshot(): Set<string> {
  if (cache === null) cache = parseSeenSignatures(readStorage());
  return cache;
}

function getServerSnapshot(): Set<string> {
  return EMPTY;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function commit(next: Set<string>): void {
  cache = next;
  writeStorage(serializeSeenSignatures(next));
  for (const listener of listeners) listener();
}

/** Test hook: forget the module cache so a fresh read hits sessionStorage. */
export function resetNeedsYouSeenForTests(): void {
  cache = null;
  listeners.clear();
}

export function useNeedsYouSeen() {
  const seen = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const markSeen = useCallback((signatures: readonly string[]) => {
    const current = getSnapshot();
    let changed = false;
    const next = new Set(current);
    for (const signature of signatures) {
      if (!signature || next.has(signature)) continue;
      next.add(signature);
      changed = true;
    }
    if (changed) commit(next);
  }, []);

  const prune = useCallback((active: readonly string[] | null) => {
    if (active === null) return;
    const current = getSnapshot();
    const next = pruneSeenSignatures(current, active);
    if (next.size !== current.size) commit(next);
  }, []);

  return { seen, markSeen, prune };
}
