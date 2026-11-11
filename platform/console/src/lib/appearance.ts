'use client';

import { flushSync } from 'react-dom';
import {
  APPEARANCE_PREPAINT_SCRIPT,
  APPEARANCE_STORAGE_KEY,
} from './appearance-prepaint';

export { APPEARANCE_PREPAINT_SCRIPT, APPEARANCE_STORAGE_KEY };

export type ThemeMode = 'dark' | 'light';
export type AccentTint =
  | 'graphite'
  | 'sky'
  | 'sage'
  | 'rose'
  | 'amber'
  | 'teal'
  | 'lavender'
  | 'orchid';
export type DensityMode = 'comfortable' | 'compact';

export interface AppearancePreferences {
  theme: ThemeMode;
  tint: AccentTint;
  contrast: boolean;
  density: DensityMode;
  reduceMotion: boolean;
}

export const DEFAULT_APPEARANCE: AppearancePreferences = {
  theme: 'dark',
  tint: 'graphite',
  contrast: false,
  density: 'comfortable',
  reduceMotion: false,
};

export const TINT_OPTIONS: ReadonlyArray<{ id: AccentTint; label: string; swatchVar: string }> = [
  { id: 'graphite', label: 'Graphite', swatchVar: 'var(--color-tint-graphite)' },
  { id: 'sky', label: 'Sky', swatchVar: 'var(--color-tint-sky)' },
  { id: 'sage', label: 'Sage', swatchVar: 'var(--color-tint-sage)' },
  { id: 'teal', label: 'Teal', swatchVar: 'var(--color-tint-teal)' },
  { id: 'amber', label: 'Amber', swatchVar: 'var(--color-tint-amber)' },
  { id: 'rose', label: 'Rose', swatchVar: 'var(--color-tint-rose)' },
  { id: 'lavender', label: 'Lavender', swatchVar: 'var(--color-tint-lavender)' },
  { id: 'orchid', label: 'Orchid', swatchVar: 'var(--color-tint-orchid)' },
];

const VALID_THEMES = new Set<ThemeMode>(['dark', 'light']);
const VALID_TINTS = new Set<AccentTint>(TINT_OPTIONS.map((t) => t.id));
const VALID_DENSITY = new Set<DensityMode>(['comfortable', 'compact']);

export function normalizeAppearance(raw: unknown): AppearancePreferences {
  if (!raw || typeof raw !== 'object') return DEFAULT_APPEARANCE;
  const obj = raw as Record<string, unknown>;
  return {
    theme: VALID_THEMES.has(obj.theme as ThemeMode) ? (obj.theme as ThemeMode) : 'dark',
    tint: VALID_TINTS.has(obj.tint as AccentTint) ? (obj.tint as AccentTint) : 'graphite',
    contrast: obj.contrast === true,
    density: VALID_DENSITY.has(obj.density as DensityMode) ? (obj.density as DensityMode) : 'comfortable',
    reduceMotion: obj.reduceMotion === true,
  };
}

export function loadAppearance(): AppearancePreferences {
  if (typeof window === 'undefined' || !window.localStorage) return DEFAULT_APPEARANCE;
  try {
    const raw = window.localStorage.getItem(APPEARANCE_STORAGE_KEY);
    if (!raw) return DEFAULT_APPEARANCE;
    return normalizeAppearance(JSON.parse(raw));
  } catch {
    return DEFAULT_APPEARANCE;
  }
}

export function applyAppearanceToDocument(prefs: AppearancePreferences, doc: Document = document): void {
  const root = doc.documentElement;
  if (prefs.theme === 'light') {
    root.dataset.theme = 'light';
    root.classList.remove('dark');
  } else {
    root.dataset.theme = 'dark';
    root.classList.add('dark');
  }

  if (prefs.tint && prefs.tint !== 'graphite') {
    root.dataset.tint = prefs.tint;
  } else {
    delete root.dataset.tint;
  }

  if (prefs.contrast) {
    root.dataset.contrast = 'true';
  } else {
    delete root.dataset.contrast;
  }

  if (prefs.density === 'compact') {
    root.dataset.density = 'compact';
  } else {
    delete root.dataset.density;
  }

  if (prefs.reduceMotion) {
    root.dataset.reduceMotion = 'true';
  } else {
    delete root.dataset.reduceMotion;
  }
}

export function saveAppearance(prefs: AppearancePreferences): void {
  const normalized = normalizeAppearance(prefs);
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      window.localStorage.setItem(APPEARANCE_STORAGE_KEY, JSON.stringify(normalized));
    } catch {
      // Ignore quota / privacy mode storage errors cleanly.
    }
  }
  if (typeof document !== 'undefined') {
    applyAppearanceToDocument(normalized, document);
  }
}

interface ViewTransitionHandle {
  finished: Promise<void>;
  skipTransition?: () => void;
}

type ViewTransitionDoc = Document & {
  startViewTransition?: (cb: () => void) => ViewTransitionHandle;
};

let activeViewTransition: ViewTransitionHandle | null = null;

/**
 * Runs a DOM/React state transition inside `document.startViewTransition` with
 * an atomic `flushSync` commit, cancelling any overlapping in-flight transition
 * via `skipTransition()`, and bypassing view transitions when reduced motion
 * is requested via OS setting or `data-reduce-motion="true"`.
 */
export function runViewTransition(
  apply: () => void,
  onFinished?: () => void,
): void {
  if (typeof document === 'undefined') {
    apply();
    onFinished?.();
    return;
  }
  const doc = document as ViewTransitionDoc;
  const reduceMotionAttr = doc.documentElement?.dataset.reduceMotion === 'true';
  const reduceMotionMedia =
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (
    reduceMotionAttr ||
    reduceMotionMedia ||
    typeof doc.startViewTransition !== 'function'
  ) {
    apply();
    onFinished?.();
    return;
  }
  activeViewTransition?.skipTransition?.();
  const transition = doc.startViewTransition(() => {
    flushSync(apply);
  });
  activeViewTransition = transition;
  transition.finished
    .catch(() => {})
    .finally(() => {
      if (activeViewTransition === transition) {
        activeViewTransition = null;
      }
      onFinished?.();
    });
}
