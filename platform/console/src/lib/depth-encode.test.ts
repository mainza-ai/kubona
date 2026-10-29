import * as THREE from 'three';
import { describe, expect, it, vi } from 'vitest';
import { buildTerrainHeightField, encodeDepthEXR, pinholeFromFov, topDownDepthFromReadback } from './depth-encode';
import { parseLinearDepthEXR } from './depth';
import { dissociationEnergyHartree, morseEnergyHartree, morseVisualHeight } from './morse';

// P1 encoder pins (hermetic, no GL): math parity with @/lib/morse, the
// PlaneGeometry index contract, coverage semantics, intrinsics, and the
// full EXR round-trip through the production decoder.

describe('depth encoder', () => {
  it('buildTerrainHeightField matches morse.ts point-for-point', () => {
    const N = 9;
    const W = 4.0;
    const de = dissociationEnergyHartree('LiH');
    const ground = -7.882;
    const field = buildTerrainHeightField(N, W, de, ground);
    for (const [i, j] of [[0, 0], [4, 4], [8, 8], [2, 6], [7, 1]] as const) {
      const x = -W / 2 + (W * i) / (N - 1);
      const z = -W / 2 + (W * j) / (N - 1);
      const want = morseVisualHeight(morseEnergyHartree(x * 0.4, z, de, ground), ground, de);
      expect(field[j * N + i]).toBeCloseTo(want, 6);
    }
  });

  it('PlaneGeometry vertex order matches the height-field layout', () => {
    // Real PlaneGeometry (no GL needed): vertex iy*N+ix must sit at
    // x=-w/2+w*ix/(N-1), z=-w/2+w*iy/(N-1) after rotateX(-PI/2).
    const N = 5;
    const W = 4.0;
    const geo = new THREE.PlaneGeometry(W, W, N - 1, N - 1);
    geo.rotateX(-Math.PI / 2);
    const pos = geo.attributes.position;
    expect(pos.count).toBe(N * N);
    for (let j = 0; j < N; j++) {
      for (let i = 0; i < N; i++) {
        const k = j * N + i;
        expect(pos.getX(k)).toBeCloseTo(-W / 2 + (W * i) / (N - 1), 9);
        expect(pos.getZ(k)).toBeCloseTo(-W / 2 + (W * j) / (N - 1), 9);
      }
    }
    geo.dispose();
  });

  it('topDownDepthFromReadback flips rows and zeroes uncovered pixels', () => {
    // Bottom-up RGBA readback: row 0 = [z=1 covered, NaN covered, z=2 uncovered].
    const w = 3;
    const h = 2;
    const bottomUp = new Float32Array([
      1, 1, 0, 1, NaN, 1, 0, 1, 2, 0, 0, 1,
      5, 1, 0, 1, 0, 0, 0, 1, -3, 1, 0, 1,
    ]);
    const top = topDownDepthFromReadback(bottomUp, w, h);
    // Top-down row 0 = readback row 1: [5 covered, 0 uncovered, 0 negative].
    expect(Array.from(top.slice(0, 3))).toEqual([5, 0, 0]);
    // Top-down row 1 = readback row 0: [1 covered, 0 NaN, 0 uncovered].
    expect(Array.from(top.slice(3, 6))).toEqual([1, 0, 0]);
  });

  it('pinholeFromFov follows the Chisel-editor convention', () => {
    const intr = pinholeFromFov(45, 1280, 720);
    expect(intr).toEqual({
      fx: 720 / (2 * Math.tan(Math.PI / 8)),
      fy: 720 / (2 * Math.tan(Math.PI / 8)),
      cx: 640,
      cy: 360,
      width: 1280,
      height: 720,
    });
  });

  it('encoded EXR decodes to identical top-down depths', async () => {
    const w = 8;
    const h = 6;
    const depth = new Float32Array(w * h);
    for (let i = 0; i < depth.length; i++) depth[i] = 1 + (i % 7) * 0.5;
    depth[0] = 0; // sky stays sky
    const bytes = await encodeDepthEXR(depth, w, h);
    const buf = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
    const back = parseLinearDepthEXR(buf);
    expect(back.width).toBe(w);
    expect(back.height).toBe(h);
    for (let i = 0; i < depth.length; i++) {
      // Half-float EXR: relative error up to ~5e-4 per the Atlas docs.
      expect(back.data[i]).toBeCloseTo(depth[i], 2);
    }
  });

  it('disposes the DataTexture after EXR parse (no heap leak per chisel)', async () => {
    const disposeSpy = vi.spyOn(THREE.DataTexture.prototype, 'dispose');
    const w = 4;
    const h = 4;
    const depth = new Float32Array(w * h).fill(2.5);
    await encodeDepthEXR(depth, w, h);
    expect(disposeSpy).toHaveBeenCalledTimes(1);
    disposeSpy.mockRestore();
  });
});
