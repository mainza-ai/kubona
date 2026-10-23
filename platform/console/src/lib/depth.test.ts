import { DataTexture, FloatType, RGBAFormat } from 'three';
import { EXRExporter } from 'three/examples/jsm/exporters/EXRExporter.js';
import { describe, expect, it } from 'vitest';
import { depthStats, parseLinearDepthEXR, unprojectPlanarDepth } from './depth';

// P2 groundwork, hermetic: export a synthetic planar-depth field to EXR
// in-test, decode it through the production path, and prove the exact
// row-order mapping the Atlas docs mandate (CPU rows are bottom-up).

async function roundTrip(rowsBottomUp: number[][], w: number, h: number): Promise<Float32Array> {
  const raw = new Float32Array(w * h * 4);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const z = rowsBottomUp[y][x];
      raw.set([z, z, z, 1], (y * w + x) * 4);
    }
  }
  const tex = new DataTexture(raw, w, h, RGBAFormat, FloatType);
  tex.needsUpdate = true;
  // Interop pin: three r186 EXRExporter defaults to ZIP compression, which
  // this same version's EXRLoader misdecodes (garbage subnormals). P1 must
  // encode depth uncompressed (or ZIPS) so every decoder — ours and
  // Atlas's — reads identical pixels. Compression is a transport detail,
  // never part of the depth contract under test.
  const bytes = await new EXRExporter().parse(tex, { compression: 0 });
  const buf = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
  return parseLinearDepthEXR(buf).data;
}

describe('planar depth pipeline', () => {
  it('round-trips EXR with the documented bottom-up row flip', async () => {
    // Texture row 0 = image BOTTOM (GL convention), row 1 = image TOP.
    const topDown = await roundTrip(
      [
        [10, 10, 10, 10], // bottom row
        [2, 2, 2, 2], // top row
      ],
      4,
      2,
    );
    // Production output is top-down image rows: top row first.
    expect(Array.from(topDown.slice(0, 4))).toEqual([2, 2, 2, 2]);
    expect(Array.from(topDown.slice(4, 8))).toEqual([10, 10, 10, 10]);
  });

  it('unprojects the optical center to (0, 0, z)', () => {
    const w = 1280;
    const h = 720;
    const depth = new Float32Array(w * h).fill(NaN);
    depth[360 * w + 640] = 3.5; // principal point
    depth[360 * w + 641] = 3.5; // one pixel right
    const { points, count } = unprojectPlanarDepth(depth, w, h, {
      fx: 900,
      fy: 900,
      cx: 640,
      cy: 360,
      width: w,
      height: h,
    });
    expect(count).toBe(2);
    expect(points[0]).toBeCloseTo(0, 9);
    expect(points[1]).toBeCloseTo(0, 9);
    expect(points[2]).toBeCloseTo(3.5, 9);
    expect(points[3]).toBeCloseTo(3.5 / 900, 9);
    expect(points[5]).toBeCloseTo(3.5, 9);
  });

  it('skips NaN, infinite, and non-positive depths', () => {
    const depth = new Float32Array([1.0, NaN, Infinity, 0, -2, 2.0]);
    const { count } = unprojectPlanarDepth(depth, 3, 2, {
      fx: 1,
      fy: 1,
      cx: 0,
      cy: 0,
      width: 3,
      height: 2,
    });
    expect(count).toBe(2);
    const stats = depthStats(depth);
    expect(stats).toEqual({ finite: 2, nan: 4, min: 1, max: 2 });
  });
});
