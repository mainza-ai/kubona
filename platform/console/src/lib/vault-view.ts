import { morseVisualHeight } from './morse';
import {
  divergenceBand as bandForAbsZ,
  shouldCollapse,
  type CollapseKey,
  type DivBand,
} from './attention';

// Pure view-models for the Vault panel, ghost band, gap tint legend and
// replay scrubber (R-06/R-07/R-15). Everything renderable is derived here
// and pinned by tests; the Three.js glue stays thin.
export { bandForAbsZ as divergenceBand, shouldCollapse };
export type { CollapseKey, DivBand };

export interface GhostBand {
  /** Visual heights: upper (μ+σ), mid (μ), lower (μ−σ). */
  upper: number;
  mid: number;
  lower: number;
}

/** Ghost band heights for a sealed (μ, σ): visualHeight applied per rung,
 * since the energy→height mapping is nonlinear — do NOT offset in energy space. */
export function ghostBandHeights(
  meanHa: number,
  stdHa: number,
  groundHa: number,
  dissociationHa: number,
): GhostBand {
  const mid = morseVisualHeight(meanHa, groundHa, dissociationHa);
  return {
    upper: morseVisualHeight(meanHa + Math.abs(stdHa), groundHa, dissociationHa),
    mid,
    lower: morseVisualHeight(meanHa - Math.abs(stdHa), groundHa, dissociationHa),
  };
}

export interface HistoryEntry {
  event_type?: string;
  prediction_hash?: string;
  expected_mean?: number;
  expected_std?: number;
  surprise_z?: number;
  level?: string;
  rho?: number;
}

export interface ReplayFrame {
  label: string;
  mean: number | null;
  std: number | null;
  z: number | null;
  level: string | null;
  rho: number | null;
}

/** Pick a scrubber frame from prediction history (newest-first). Null when
 * the index is out of range or the entry carries nothing renderable. */
export function replayFrame(entries: HistoryEntry[], index: number): ReplayFrame | null {
  const e = entries[index];
  if (!e) return null;
  const mean = typeof e.expected_mean === 'number' ? e.expected_mean : null;
  const std = typeof e.expected_std === 'number' ? e.expected_std : null;
  const z = typeof e.surprise_z === 'number' ? e.surprise_z : null;
  if (mean == null && z == null && e.rho == null) return null;
  const hash = typeof e.prediction_hash === 'string' ? e.prediction_hash.slice(0, 8) : '????';
  return {
    label: `${e.event_type ?? 'event'} ${hash}`,
    mean,
    std,
    z,
    level: typeof e.level === 'string' ? e.level : null,
    rho: typeof e.rho === 'number' ? e.rho : null,
  };
}
