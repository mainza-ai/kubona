// Lightweight FPS meter for the 60 FPS gate (R-23). Pure accumulator is
// unit-tested; the rAF loop is a thin hook. Overhead: one timestamp per
// frame, one state update per second.

export interface FpsSample {
  fps: number;
  frames: number;
  windowMs: number;
}

export function createFpsAccumulator() {
  let count = 0;
  let start = 0;
  return {
    /** Feed a rAF timestamp (ms). Returns a sample once >=1000ms elapsed. */
    tick(nowMs: number): FpsSample | null {
      if (count === 0) start = nowMs;
      count += 1;
      const elapsed = nowMs - start;
      if (elapsed >= 1000) {
        const sample = { fps: (count * 1000) / elapsed, frames: count, windowMs: elapsed };
        count = 0;
        return sample;
      }
      return null;
    },
    reset() {
      count = 0;
      start = 0;
    },
  };
}

export type FpsAccumulator = ReturnType<typeof createFpsAccumulator>;
