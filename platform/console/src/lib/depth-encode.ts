import * as THREE from 'three';
import { EXRExporter } from 'three/examples/jsm/exporters/EXRExporter.js';
import { morseEnergyHartree, morseVisualHeight } from './morse';
import type { PinholeIntrinsics } from './depth';

// Planar-depth encoder (P1): renders the ledger-grounded Morse terrain
// offscreen from a RUB camera and exports a top-left-aligned single-channel
// float EXR (uncompressed — see the ZIP interop pin in depth.test.ts).
//
// Pure logic (height field, coverage mask, intrinsics) is factored for
// hermetic tests; the WebGL glue stays thin and untested by construction
// (jsdom has no GL). Drift guard: buildTerrainHeightField is pinned against
// @/lib/morse in depth-encode.test.ts, so the encoder can never silently
// diverge from the surfaces.

export interface EncoderCamera {
  position: [number, number, number];
  quaternion: [number, number, number, number]; // XYZW, camera-to-world, rub
  fovDeg?: number;
}

/** Pinhole intrinsics from vertical FOV (docs' Chisel-editor convention). */
export function pinholeFromFov(fovDeg: number, width: number, height: number): PinholeIntrinsics {
  const f = height / (2 * Math.tan(((fovDeg * Math.PI) / 180) / 2));
  return { fx: f, fy: f, cx: width / 2, cy: height / 2, width, height };
}

/**
 * Terrain height field in world units, row-major top-down (row 0 = far -Z).
 * Must match SpatialCanvas3D's mesh vertex-for-vertex: same grid, same
 * world size, same deltaR mapping (x * 0.4), same visual-height mapping.
 */
export function buildTerrainHeightField(
  gridN: number,
  worldSize: number,
  dissociationHa: number,
  groundHa: number,
): Float32Array {
  const out = new Float32Array(gridN * gridN);
  for (let j = 0; j < gridN; j++) {
    for (let i = 0; i < gridN; i++) {
      const x = -worldSize / 2 + (worldSize * i) / (gridN - 1);
      const z = -worldSize / 2 + (worldSize * j) / (gridN - 1);
      const energy = morseEnergyHartree(x * 0.4, z, dissociationHa, groundHa);
      out[j * gridN + i] = morseVisualHeight(energy, groundHa, dissociationHa);
    }
  }
  return out;
}

/**
 * Convert a bottom-up RGBA float readback (R = view-space z, G = coverage)
 * into a top-down planar-depth field. Uncovered pixels (sky/void) become
 * 0, never an invented surface — the Atlas contract reads 0/NaN as
 * no-surface.
 */
export function topDownDepthFromReadback(
  pixelsBottomUp: Float32Array,
  width: number,
  height: number,
): Float32Array {
  const out = new Float32Array(width * height);
  for (let y = 0; y < height; y++) {
    const srcY = height - 1 - y;
    for (let x = 0; x < width; x++) {
      const src = (srcY * width + x) * 4;
      const covered = pixelsBottomUp[src + 1] > 0.5;
      const z = pixelsBottomUp[src];
      out[y * width + x] = covered && Number.isFinite(z) && z > 0 ? z : 0;
    }
  }
  return out;
}

/** Encode a top-down planar-depth field as an uncompressed float EXR. */
export async function encodeDepthEXR(depthTopDown: Float32Array, width: number, height: number): Promise<Uint8Array> {
  const rgba = new Float32Array(width * height * 4);
  // Atlas scanline 0 = image BOTTOM (verified against real Atlas depth
  // bytes: floor-close values sit at scanline 0). So texture row 0 must be
  // the image bottom: flip top-down input to bottom-up for export.
  for (let y = 0; y < height; y++) {
    const srcY = height - 1 - y;
    for (let x = 0; x < width; x++) {
      const z = depthTopDown[srcY * width + x];
      rgba.set([z, 1, 0, 1], (y * width + x) * 4);
    }
  }
  const tex = new THREE.DataTexture(rgba, width, height, THREE.RGBAFormat, THREE.FloatType);
  tex.needsUpdate = true;
  // Wave-3 heap hygiene: EXRExporter.parse reads the texture synchronously,
  // so dispose the GPU-side DataTexture immediately after parse — otherwise
  // every chisel dispatch leaks one 1280x720 RGBA float texture.
  try {
    return new EXRExporter().parse(tex, { compression: 0 });
  } finally {
    tex.dispose();
  }
}

/**
 * Thin WebGL glue: render the terrain depth offscreen from a RUB camera.
 * Untested by construction (jsdom has no GL) — keep it boring: the exact
 * same height field as buildTerrainHeightField, a depth+coverage shader,
 * and topDownDepthFromReadback for the CPU handoff. Browser-only — no
 * Python mirror exists; encoder-shaped bytes were live-verified through
 * the backend lane instead (R-31: 65×65 EXR → SUCCEEDED chisel).
 */
export function renderTerrainDepthGL(
  heightsTopDown: Float32Array,
  gridN: number,
  worldSize: number,
  cam: EncoderCamera,
  width = 1280,
  height = 720,
): Float32Array {
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: false, alpha: false });
  renderer.setSize(width, height, false);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(cam.fovDeg ?? 45, width / height, 0.1, 100);
  camera.position.set(...cam.position);
  camera.quaternion.set(cam.quaternion[0], cam.quaternion[1], cam.quaternion[2], cam.quaternion[3]);

  const geo = new THREE.PlaneGeometry(worldSize, worldSize, gridN - 1, gridN - 1);
  geo.rotateX(-Math.PI / 2);
  const pos = geo.attributes.position;
  // Index contract (pinned in depth-encode.test.ts via real PlaneGeometry):
  // vertex iy*gridN+ix sits at x=-w/2+w*ix/(N-1), z=-w/2+w*iy/(N-1),
  // exactly the layout buildTerrainHeightField emits. Copy, never recompute.
  for (let i = 0; i < pos.count; i++) {
    pos.setY(i, heightsTopDown[i]);
  }
  pos.needsUpdate = true;
  geo.computeVertexNormals();

  const mat = new THREE.ShaderMaterial({
    vertexShader: `
      varying float vViewZ;
      void main() {
        vec4 mv = modelViewMatrix * vec4(position, 1.0);
        vViewZ = -mv.z;
        gl_Position = projectionMatrix * mv;
      }`,
    fragmentShader: `
      varying float vViewZ;
      void main() {
        gl_FragColor = vec4(vViewZ, 1.0, 0.0, 1.0);
      }`,
  });
  const target = new THREE.WebGLRenderTarget(width, height, {
    type: THREE.FloatType,
    format: THREE.RGBAFormat,
    depthBuffer: true,
  });
  scene.add(new THREE.Mesh(geo, mat));
  renderer.setClearColor(0x000000, 0);
  renderer.setRenderTarget(target);
  renderer.render(scene, camera);

  const pixels = new Float32Array(width * height * 4);
  renderer.readRenderTargetPixels(target, 0, 0, width, height, pixels as unknown as THREE.TypedArray);

  target.dispose();
  geo.dispose();
  mat.dispose();
  renderer.dispose();
  return topDownDepthFromReadback(pixels, width, height);
}
