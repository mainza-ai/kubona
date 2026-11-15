'use client';

import { useSyncExternalStore } from 'react';

/**
 * D3 — one reduced-motion answer for JS and CSS-adjacent code. Motion is
 * reduced when EITHER the OS preference or the in-app `data-reduce-motion`
 * attribute says so; the two never disagree in the UI.
 *
 * CSS already collapses animations and transitions globally for both signals
 * (see styles/axes.css + globals.css); this module is for imperative motion:
 * camera glides, rAF loops, scroll behaviour.
 */

export const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

export function reducedMotionMediaMatches(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
  try {
    return window.matchMedia(REDUCED_MOTION_QUERY).matches;
  } catch {
    return false;
  }
}

export function reducedMotionAttrSet(): boolean {
  return typeof document !== 'undefined' && document.documentElement?.dataset.reduceMotion === 'true';
}

/** Current answer, pure read (safe to call outside React). */
export function prefersReducedMotion(): boolean {
  return reducedMotionMediaMatches() || reducedMotionAttrSet();
}

function subscribe(onChange: () => void): () => void {
  if (typeof window === 'undefined') return () => {};
  let media: MediaQueryList | null = null;
  const onMedia = () => onChange();
  if (typeof window.matchMedia === 'function') {
    try {
      media = window.matchMedia(REDUCED_MOTION_QUERY);
      media.addEventListener?.('change', onMedia);
    } catch {
      media = null;
    }
  }
  let observer: MutationObserver | null = null;
  if (typeof MutationObserver === 'function' && typeof document !== 'undefined') {
    observer = new MutationObserver(() => onChange());
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-reduce-motion'] });
  }
  return () => {
    media?.removeEventListener?.('change', onMedia);
    observer?.disconnect();
  };
}

/** Reactive answer for render-time decisions; SSR defaults to no reduction. */
export function useReducedMotion(): boolean {
  return useSyncExternalStore(subscribe, prefersReducedMotion, () => false);
}
