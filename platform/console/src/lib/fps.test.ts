import { describe, expect, it } from 'vitest';
import { createFpsAccumulator, pixelRatioStepDown, COARSE_DPR_CAP, DESKTOP_DPR_CAP, MIN_DPR } from './fps';

describe('fps accumulator (R-23)', () => {
  it('reports fps once a second elapses', () => {
    const acc = createFpsAccumulator();
    let out = null;
    // 60 frames in 1000ms -> 60fps.
    for (let i = 0; i <= 60; i++) {
      out = acc.tick(i * (1000 / 60)) ?? out;
    }
    expect(out).not.toBeNull();
    expect(out!.frames).toBeGreaterThanOrEqual(60);
    expect(out!.fps).toBeGreaterThan(50);
    expect(out!.fps).toBeLessThan(70);
  });

  it('returns null before a full window and recovers after reset', () => {
    const acc = createFpsAccumulator();
    expect(acc.tick(0)).toBeNull();
    expect(acc.tick(500)).toBeNull();
    acc.reset();
    expect(acc.tick(0)).toBeNull();
  });
});

describe('pixelRatioStepDown (Wave-3 fps-driven DPR)', () => {
  it('holds steady at healthy fps', () => {
    expect(pixelRatioStepDown(2, 60)).toBe(2);
    expect(pixelRatioStepDown(1.5, 45)).toBe(1.5);
  });

  it('steps down one notch (0.25) on mild sag', () => {
    expect(pixelRatioStepDown(2, 30)).toBe(1.75);
    expect(pixelRatioStepDown(1.5, 44.9)).toBe(1.25);
  });

  it('steps down two notches (0.5) on hard sag', () => {
    expect(pixelRatioStepDown(2, 20)).toBe(1.5);
    expect(pixelRatioStepDown(1.25, 10)).toBe(MIN_DPR);
  });

  it('floors at MIN_DPR and never below', () => {
    expect(pixelRatioStepDown(1, 5)).toBe(MIN_DPR);
    expect(pixelRatioStepDown(MIN_DPR, 5)).toBe(MIN_DPR);
  });

  it('passes non-finite inputs through untouched', () => {
    expect(pixelRatioStepDown(NaN, 20)).toBeNaN();
    expect(pixelRatioStepDown(2, NaN)).toBe(2);
  });

  it('exports the documented DPR caps', () => {
    expect(COARSE_DPR_CAP).toBe(1.5);
    expect(DESKTOP_DPR_CAP).toBe(2);
    expect(MIN_DPR).toBe(1);
  });
});
