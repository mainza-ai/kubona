import { describe, expect, it } from 'vitest';
import { createFpsAccumulator } from './fps';

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
