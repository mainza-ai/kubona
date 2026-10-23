import { describe, expect, it } from 'vitest';
import { divergenceBand, ghostBandHeights, replayFrame, shouldCollapse } from './vault-view';

// Pinned view-models: ghost band, gap-tint legend, replay scrubber,
// duplicate-collapse window (R-06/R-07/R-11/R-15).

describe('vault view models', () => {
  it('ghostBandHeights orders upper > mid > lower above the floor', () => {
    // μ above the anchor floor so no rung hits the max(0, ·) clamp.
    const g = ghostBandHeights(-1.0, 0.005, -1.137, 0.174);
    expect(g.upper).toBeGreaterThan(g.mid);
    expect(g.mid).toBeGreaterThan(g.lower);
    expect(g.mid).toBeGreaterThan(0);
  });
  it('ghostBandHeights clamps rungs below the physical floor', () => {
    // Visual heights cannot go below the ground-state floor: a sealed μ
    // at the anchor with sub-floor σ collapses the lower rungs to it.
    const g = ghostBandHeights(-1.137, 0.005, -1.137, 0.174);
    expect(g.mid).toBeCloseTo(0, 9);
    expect(g.lower).toBe(0);
    expect(g.upper).toBeGreaterThan(0);
  });

  it('ghostBandHeights tolerates degenerate std', () => {
    const g = ghostBandHeights(-1.1, 0, -1.137, 0.174);
    expect(Number.isFinite(g.upper)).toBe(true);
    expect(Number.isFinite(g.mid)).toBe(true);
    expect(Number.isFinite(g.lower)).toBe(true);
  });

  it('divergenceBand matches the plan cutoffs', () => {
    expect(divergenceBand(0)).toBe('normal');
    expect(divergenceBand(0.99)).toBe('normal');
    expect(divergenceBand(1)).toBe('notable');
    expect(divergenceBand(3)).toBe('notable');
    expect(divergenceBand(3.01)).toBe('anomalous');
    expect(divergenceBand(NaN)).toBe('normal');
  });

  it('replayFrame picks renderable frames and rejects blanks', () => {
    const entries = [
      { event_type: 'prediction_sealed', prediction_hash: 'a1b2c3d4e5', expected_mean: -1.1, expected_std: 0.01 },
      { event_type: 'prediction_evaluated', prediction_hash: 'a1b2c3d4e5', surprise_z: 2.5, level: 'notable' },
      { event_type: 'annotation', text: 'hi' },
    ];
    const f0 = replayFrame(entries, 0)!;
    expect(f0.mean).toBeCloseTo(-1.1, 9);
    expect(f0.z).toBeNull();
    const f1 = replayFrame(entries, 1)!;
    expect(f1.z).toBeCloseTo(2.5, 9);
    expect(f1.level).toBe('notable');
    expect(replayFrame(entries, 2)).toBeNull();
    expect(replayFrame(entries, 9)).toBeNull();
    expect(replayFrame([], 0)).toBeNull();
  });

  it('shouldCollapse keys on source+level inside the window', () => {
    const base = { key: { source: 'vault', level: 'notable' }, at: 1000 };
    expect(shouldCollapse(base, { source: 'vault', level: 'notable' }, 1000 + 60_000)).toBe(true);
    expect(shouldCollapse(base, { source: 'vault', level: 'anomalous' }, 1000 + 60_000)).toBe(false);
    expect(shouldCollapse(base, { source: 'other', level: 'notable' }, 1000 + 60_000)).toBe(false);
    expect(shouldCollapse(base, { source: 'vault', level: 'notable' }, 1000 + 5 * 60_000 + 1)).toBe(false);
    expect(shouldCollapse(null, { source: 'vault', level: 'notable' }, 1000)).toBe(false);
  });
});
