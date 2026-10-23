import * as THREE from 'three';
import { EXRLoader } from 'three/examples/jsm/loaders/EXRLoader.js';

// Planar-depth CPU pipeline (P2 groundwork, zero live spend).
//
// Contract (Atlas docs: Cameras and posed images): depth pixels are optical
// axis view-space z (NOT ray length); NaN means no surface; EXRLoader
// exposes rows bottom-up, so CPU code maps image-space y -> height-1-y.
// Measurement and hit-testing resolve against KUBONA's OWN depth buffer —
// never against model-reconstructed depth.

export interface PinholeIntrinsics {
  fx: number;
  fy: number;
  cx: number;
  cy: number;
  width: number;
  height: number;
}

/**
 * CPU-decode a single-channel planar-depth EXR into TOP-DOWN rows (image
 * convention). Multi-channel files read the R channel. NaN (sky/void) and
 * non-positive values are preserved for the caller to skip.
 */
export function parseLinearDepthEXR(buffer: ArrayBuffer): {
  width: number;
  height: number;
  data: Float32Array;
} {
  // No forced data type: linear-depth EXRs are half-float pixels per the
  // Atlas contract (relative rounding up to ~5e-4); float EXRs decode
  // natively. Forcing FloatType misreads half bytes as garbage.
  const loader = new EXRLoader();
  const tex = loader.parse(buffer) as unknown as {
    data: Float32Array | Uint16Array;
    width: number;
    height: number;
  };
  const { width, height } = tex;
  let flat: Float32Array;
  if (tex.data instanceof Float32Array) {
    flat = tex.data;
  } else {
    flat = new Float32Array(tex.data.length);
    for (let i = 0; i < tex.data.length; i++) {
      flat[i] = THREE.DataUtils.fromHalfFloat(tex.data[i]);
    }
  }
  const channels = Math.max(1, Math.round(flat.length / (width * height)));
  const out = new Float32Array(width * height);
  for (let y = 0; y < height; y++) {
    const srcY = height - 1 - y; // bottom-up decoder rows -> top-down image
    for (let x = 0; x < width; x++) {
      out[y * width + x] = flat[(srcY * width + x) * channels];
    }
  }
  return { width, height, data: out };
}

/**
 * Unproject planar view-space depth to camera-space XYZ point cloud.
 * x = (u-cx)/fx * z; y = -(v-cy)/fy * z (image y grows down, camera y up).
 * Skips NaN / infinite / z <= 0 (sky and voids per the depth contract).
 */
export function unprojectPlanarDepth(
  depthTopDown: Float32Array,
  width: number,
  height: number,
  intr: PinholeIntrinsics,
): { points: Float32Array; count: number } {
  const pts: number[] = [];
  for (let v = 0; v < height; v++) {
    for (let u = 0; u < width; u++) {
      const z = depthTopDown[v * width + u];
      if (!Number.isFinite(z) || z <= 0) continue;
      pts.push(((u - intr.cx) / intr.fx) * z, (-(v - intr.cy) / intr.fy) * z, z);
    }
  }
  return { points: new Float32Array(pts), count: pts.length / 3 };
}

/** Finite-pixel statistics for validation gates and HUD readouts. */
export function depthStats(depth: Float32Array): {
  finite: number;
  nan: number;
  min: number;
  max: number;
} {
  let finite = 0;
  let nan = 0;
  let min = Infinity;
  let max = -Infinity;
  for (let i = 0; i < depth.length; i++) {
    const z = depth[i];
    if (!Number.isFinite(z) || z <= 0) {
      nan++;
      continue;
    }
    finite++;
    if (z < min) min = z;
    if (z > max) max = z;
  }
  return { finite, nan, min, max };
}
