'use client';

import React, { useState } from 'react';
import {
  DEFAULT_APPEARANCE,
  TINT_OPTIONS,
  loadAppearance,
  runViewTransition,
  saveAppearance,
  type AppearancePreferences,
  type DensityMode,
  type ThemeMode,
} from '@/lib/appearance';

export function AppearanceSettings() {
  const [prefs, setPrefs] = useState<AppearancePreferences>(() => loadAppearance());

  const update = (patch: Partial<AppearancePreferences>) => {
    const next = { ...prefs, ...patch };
    const shouldAnimate =
      (patch.theme !== undefined && patch.theme !== prefs.theme) ||
      (patch.contrast !== undefined && patch.contrast !== prefs.contrast);
    const apply = () => {
      setPrefs(next);
      saveAppearance(next);
    };
    if (shouldAnimate && !next.reduceMotion) {
      runViewTransition(apply);
    } else {
      apply();
    }
  };

  return (
    <section aria-label="Appearance settings" className="k-surface p-5 space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold text-[var(--color-text)]">Appearance</h3>
          <p className="text-[12px] text-[var(--color-muted)]">
            Warm-graphite surface ladder, light mode inversion, accent tints, and accessible legibility axes.
          </p>
        </div>
        <button
          type="button"
          className="k-button"
          onClick={() => update(DEFAULT_APPEARANCE)}
        >
          Reset defaults
        </button>
      </div>

      <div className="space-y-2">
        <span className="k-field-label">Theme</span>
        <div className="k-segmented" role="radiogroup" aria-label="Theme mode">
          {(['dark', 'light'] as ThemeMode[]).map((mode) => (
            <button
              key={mode}
              type="button"
              role="radio"
              aria-checked={prefs.theme === mode}
              onClick={() => update({ theme: mode })}
              className={`k-segmented-item ${prefs.theme === mode ? 'is-selected' : ''}`}
            >
              {mode === 'dark' ? 'Dark graphite' : 'Light paper'}
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        <span className="k-field-label">Accent tint</span>
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Accent tint">
          {TINT_OPTIONS.map((opt) => {
            const active = prefs.tint === opt.id;
            return (
              <button
                key={opt.id}
                type="button"
                role="radio"
                aria-checked={active}
                onClick={() => update({ tint: opt.id })}
                className={`inline-flex items-center gap-2 rounded-xl border px-3 py-1.5 text-[12px] transition-colors ${
                  active
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                    : 'border-[var(--color-border)] bg-[var(--color-fill)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]'
                }`}
              >
                <span
                  aria-hidden="true"
                  className="h-3 w-3 rounded-full border border-black/20"
                  style={{ background: opt.swatchVar }}
                />
                {opt.label}
              </button>
            );
          })}
        </div>
      </div>

      <div className="space-y-2">
        <span className="k-field-label">Density</span>
        <div className="k-segmented" role="radiogroup" aria-label="Interface density">
          {(['comfortable', 'compact'] as DensityMode[]).map((density) => (
            <button
              key={density}
              type="button"
              role="radio"
              aria-checked={prefs.density === density}
              onClick={() => update({ density })}
              className={`k-segmented-item ${prefs.density === density ? 'is-selected' : ''}`}
            >
              {density === 'comfortable' ? 'Comfortable' : 'Compact'}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-wrap gap-6 pt-1">
        <label className="k-choice k-toggle cursor-pointer">
          <input
            type="checkbox"
            className="k-choice-input"
            checked={prefs.contrast}
            onChange={(e) => update({ contrast: e.target.checked })}
          />
          <span className="k-toggle-track" aria-hidden="true" />
          <span>Increase contrast</span>
        </label>

        <label className="k-choice k-toggle cursor-pointer">
          <input
            type="checkbox"
            className="k-choice-input"
            checked={prefs.reduceMotion}
            onChange={(e) => update({ reduceMotion: e.target.checked })}
          />
          <span className="k-toggle-track" aria-hidden="true" />
          <span>Reduce motion</span>
        </label>
      </div>
    </section>
  );
}
