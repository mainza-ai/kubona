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

// Wave-3 mobile step-down (R-23 follow-up): pure DPR policy for the 3D
// observatory. The rAF loop feeds measured fps; the component applies the
// returned ratio via renderer.setPixelRatio. Coarse-pointer devices start at
// COARSE_DPR_CAP (1.5); desktop starts at DESKTOP_DPR_CAP (2). Thresholds:
// >=45fps holds (no churn), 25–45fps steps down one notch (0.25), <25fps
// steps down two notches (0.5). Floors at MIN_DPR so text stays legible.
export const COARSE_DPR_CAP = 1.5;
export const DESKTOP_DPR_CAP = 2;
export const MIN_DPR = 1;

export function pixelRatioStepDown(current: number, fps: number, min = MIN_DPR): number {
  if (!Number.isFinite(current) || !Number.isFinite(fps)) return current;
  if (fps >= 45) return current;
  const step = fps < 25 ? 0.5 : 0.25;
  const next = Math.round((current - step) * 4) / 4;
  return Math.max(min, next);
}
