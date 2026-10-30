'use client';

import React, { useEffect, useRef, useState, useMemo, useCallback } from 'react';
import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { 
  Sparkles, 
  Maximize2, 
  Minimize2, 
  Layers, 
  ShieldCheck, 
  ShieldAlert, 
  Zap, 
  Eye,
  Camera
} from 'lucide-react';
import { CampaignState } from '@/types';
import {
  appendLedgerEvent,
  assertPrediction,
  evaluatePrediction,
  fetchCalibration,
  type AuthorCalibration,
  fetchLatestPrediction,
  fetchLedgerExport,
  fetchPredictionHistory,
  type HistoryPrediction,
  type PredictionAssertion,
  type PredictionEvaluation,
  type SealedPrediction,
  sealPrediction,
  uploadObservatoryAsset,
  lensImageProxyUrl,
} from '@/lib/api';
import { ghostBandHeights, replayFrame } from '@/lib/vault-view';
import { createFpsAccumulator, pixelRatioStepDown, COARSE_DPR_CAP, DESKTOP_DPR_CAP, type FpsAccumulator } from '@/lib/fps';
import { buildPinPayload, pinValuesFromHit } from '@/lib/pin';
import { useAttention } from '@/lib/attention-context';
// Wave-3 code-split: the depth-encode chisel path (three + EXRExporter via
// @/lib/depth-encode) is dynamically imported inside handleEncodeDepthChisel
// so it never joins the first-paint bundle — or even the initial 3D chunk —
// until the operator actually clicks "Depth Chisel".
import {
  MORSE_WIDTH_A,
  dissociationEnergyHartree,
  equilibriumBondDistanceAngstrom,
  morseEnergyHartree,
  morseVisualHeight,
} from '@/lib/morse';

// Wave-3 pure helpers (hermetic — unit-tested, no DOM/GL):
//
// - WEBGL_UNAVAILABLE_MESSAGE: the single fail-closed string the panel
//   renders when renderer creation throws or the GPU context is lost. The
//   panel replaces the live-looking HUD entirely — never a blank canvas
//   with live numbers.
// - roundHoverReading / hoverReadingChanged: hover state updates only when
//   the rounded (r, energy) actually changes, so mousemove storms collapse
//   to meaningful updates under the rAF throttle.
// - initialPixelRatioCap: coarse pointers (touch) start at 1.5, fine
//   pointers at 2. Exported for tests; the mount effect applies it.
// - touchProbeHint: coarse-pointer copy for the inspection probe.
export const WEBGL_UNAVAILABLE_MESSAGE = '3D unavailable — 2D workbench below';

export function roundHoverReading(r: number, energy: number): { r: number; energy: number } {
  return {
    r: Math.round(r * 10000) / 10000,
    energy: Math.round(energy * 100000) / 100000,
  };
}

export function hoverReadingChanged(
  prev: { r: number; energy: number } | null,
  next: { r: number; energy: number },
): boolean {
  if (!prev) return true;
  return prev.r !== next.r || prev.energy !== next.energy;
}

export function initialPixelRatioCap(devicePixelRatio: number, isCoarsePointer: boolean): number {
  const dpr = Number.isFinite(devicePixelRatio) && devicePixelRatio > 0 ? devicePixelRatio : 1;
  return Math.min(dpr, isCoarsePointer ? COARSE_DPR_CAP : DESKTOP_DPR_CAP);
}

export function touchProbeHint(isCoarsePointer: boolean): string {
  return isCoarsePointer
    ? 'Tap to pin, drag to orbit.'
    : 'Click terrain to pin coordinates into active campaign.';
}

export interface SpatialCanvas3DProps {
  campaign?: CampaignState | null;
  lensImageUrl?: string | null;
  lensWatermark?: string;
  lensPromptUsed?: string | null;
  // Actual reserved amount from the dispatch response. The ONLY cost figure
  // the UI may show: no hardcoded prices in idle chrome (F-14).
  lensCostUsd?: number | null;
  lensHistory?: Array<{
    operation_id: string;
    task_type?: string;
    cost_usd?: number;
    prompt_used?: string;
    resolved?: boolean;
  }>;
  onSelectPoint?: (point: { bondDistance: number; energy: number; label?: string }) => void;
  onRequestAtlasLens?: (snapshotDataUrl: string | null) => void;
  onRequestDepthChisel?: (
    depthAssetId: string,
    camera: { position: number[]; orientation: number[] },
  ) => void;
  isGeneratingLens?: boolean;
  lensError?: string | null;
  airGapped?: boolean;
  spendLimitUSD?: number;
  currentSpendUSD?: number;
}

export function SpatialCanvas3D({
  campaign,
  lensImageUrl = null,
  lensWatermark = 'SYNTHETIC LENS — WORLD LABS ATLAS',
  lensPromptUsed = null,
  lensCostUsd = null,
  lensHistory = [],
  onSelectPoint,
  onRequestAtlasLens,
  onRequestDepthChisel,
  isGeneratingLens = false,
  lensError = null,
  airGapped = false,
}: SpatialCanvas3DProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showGhostContour, setShowGhostContour] = useState(true);
  // R-23 measured FPS (rAF timestamps through the pure accumulator;
  // state updates at most once per second so the meter costs nothing).
  const [fps, setFps] = useState<number | null>(null);
  const fpsRef = useRef<FpsAccumulator | null>(null);
  const [selectedCoordinates, setSelectedCoordinates] = useState<{ r: number; energy: number; ledgerHash: string | null } | null>(null);
  const [hoveredCoordinates, setHoveredCoordinates] = useState<{ r: number; energy: number } | null>(null);
  // Wave-3 WebGL fail-closed: renderer creation throws (or the GPU context
  // is lost) → this carries the message and the render below swaps the
  // entire live HUD for the fail-closed panel. Never a blank canvas with
  // live-looking numbers.
  const [webglError, setWebglError] = useState<string | null>(null);
  // Wave-3 mobile: coarse pointers (touch) get a 1.5 DPR cap + touch copy.
  // Lazy initializer (not an effect) so SSR/test-renderer (no window)
  // stays false without a set-state-in-effect.
  const [isCoarsePointer, setIsCoarsePointer] = useState(
    () =>
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(pointer: coarse)').matches,
  );
  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
    const mq = window.matchMedia('(pointer: coarse)');
    const onChange = (e: MediaQueryListEvent) => setIsCoarsePointer(e.matches);
    if (typeof mq.addEventListener === 'function') {
      mq.addEventListener('change', onChange);
      return () => mq.removeEventListener('change', onChange);
    }
    return undefined;
  }, []);

  // P3: ledger-bound pre-registration. `sealed` is the latest sealed
  // expectation for this session (null = ghost stays illustrative);
  // `evaluation` is its latest scoring, rendered as the z-badge.
  const [sealed, setSealed] = useState<SealedPrediction | null>(null);
  const [evaluation, setEvaluation] = useState<PredictionEvaluation | null>(null);
  const [assertion, setAssertion] = useState<PredictionAssertion | null>(null);
  const [sealMeanStr, setSealMeanStr] = useState('');
  const [sealStdStr, setSealStdStr] = useState('');
  const [sealBusy, setSealBusy] = useState(false);
  const [sealMsg, setSealMsg] = useState<string | null>(null);
  // R-15 replay: scrubbed history index (null = live). The ghost, z-badge
  // and Vault readouts render the scrubbed entry instead of the ledger tip.
  const [history, setHistory] = useState<HistoryPrediction[]>([]);
  const [replayIdx, setReplayIdx] = useState<number | null>(null);
  // R-09 formal assertion form (rho + evidence are caller judgment).
  const [assertRho, setAssertRho] = useState('');
  const [assertEvidence, setAssertEvidence] = useState('');
  const [assertMsg, setAssertMsg] = useState<string | null>(null);
  // R-08 calibration table (read-only; enforcement stays human-gated).
  const [calibration, setCalibration] = useState<Record<string, AuthorCalibration> | null>(null);
  // R-14 agent gaze trails (latest world.gaze annotations, newest last).
  const [gazes, setGazes] = useState<Array<{ position: [number, number, number]; quaternion: [number, number, number, number] }>>([]);

  // P4: evaluated surprises route to the Attention Director (CheckInStrip
  // pill + ambient sound), closing the watch loop the plan requires.
  const { notify } = useAttention();

  // Target system & equilibrium parameters (single source of truth:
  // @/lib/morse — shared with the 2D workbench so both surfaces agree).
  const targetSystem = campaign?.target_system || 'H2';
  const equilibriumR = useMemo(() => {
    if (campaign?.quantum_factors?.bond_distance) return campaign.quantum_factors.bond_distance;
    return equilibriumBondDistanceAngstrom(campaign?.target_system);
  }, [campaign?.quantum_factors?.bond_distance, campaign?.target_system]);

  const anchor = campaign?.classical_anchor;
  const anchorComputed = anchor?.anchor_computed === true;
  const fciRaw = anchor?.fci_exact_energy;
  const fciGround = anchorComputed && typeof fciRaw === 'number' && Number.isFinite(fciRaw) ? fciRaw : -1.137;
  const rho = campaign?.epistemics?.p_correlated ?? 0.88;

  // Sealed expectation level (null = unbound). The ghost plane renders at
  // visualHeight(sealedMean) when bound — the ledger's expectation, not an
  // invented offset. μ±σ band via ghostBandHeights (R-07).
  const sealedMean = sealed?.expected_mean ?? null;
  const sealedStd = sealed?.expected_std ?? null;

  // Replay view: when scrubbing, the ghost band, gap tint and badge render
  // the scrubbed entry; null replays the live ledger tip.
  const replayEntry = replayIdx == null ? null : replayFrame(history, replayIdx);
  const viewMean = replayEntry?.mean ?? sealedMean;
  const viewStd = replayEntry?.std ?? sealedStd;

  // P3: pull the ledger's latest sealed prediction + evaluation whenever
  // the session changes. Ghost and badge re-bind from this state.
  // Session-change resets happen during render (campaign-key guard); the
  // effect body only fetches.
  useEffect(() => {
    const sid = campaign?.session_id;
    if (!sid) return;
    let cancelled = false;
    fetchLatestPrediction(sid)
      .then((res) => {
        if (cancelled) return;
        if (res.ok) {
          setSealed(res.sealed ?? null);
          setEvaluation(res.evaluation ?? null);
          setAssertion(res.assertion ?? null);
        }
      })
      .catch(() => {});
    fetchCalibration(sid)
      .then((res) => {
        if (!cancelled && res.ok) setCalibration(res.authors ?? null);
      })
      .catch(() => {});
    fetchPredictionHistory(sid, 50)
      .then((res) => {
        if (cancelled) return;
        if (res.ok) {
          setHistory(res.predictions ?? []);
          setReplayIdx(null);
        }
      })
      .catch(() => {});
    // R-14 gaze trails: latest world.gaze annotations, newest last.
    fetchLedgerExport(sid)
      .then((res) => {
        if (cancelled || !res.ok || !res.events) return;
        const trails: Array<{ position: [number, number, number]; quaternion: [number, number, number, number] }> = [];
        for (const ev of res.events) {
          const payload = ev['payload'] as Record<string, unknown> | undefined;
          if (ev['event_type'] !== 'annotation' || !payload || payload['world_tool'] !== 'gaze') continue;
          const pos = payload['position'];
          const quat = payload['quaternion'];
          if (
            Array.isArray(pos) && pos.length === 3 && pos.every((n) => typeof n === 'number' && Number.isFinite(n)) &&
            Array.isArray(quat) && quat.length === 4 && quat.every((n) => typeof n === 'number' && Number.isFinite(n))
          ) {
            trails.push({ position: [pos[0], pos[1], pos[2]], quaternion: [quat[0], quat[1], quat[2], quat[3]] });
          }
          if (trails.length >= 8) break;
        }
        setGazes(trails.reverse());
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [campaign?.session_id]);

  // Reset P3 state during render when the session changes — same
  // campaign-key guard pattern as SpatialCanvas (no setState in effects).
  const sessionKey = campaign?.session_id || '';
  const [prevSessionKey, setPrevSessionKey] = useState('');
  if (sessionKey !== prevSessionKey) {
    setPrevSessionKey(sessionKey);
    setSealed(null);
    setEvaluation(null);
    setAssertion(null);
    setCalibration(null);
    setHistory([]);
    setReplayIdx(null);
    setGazes([]);
    setSealMsg(null);
    setAssertMsg(null);
  }

  const handleSeal = useCallback(async () => {
    const sid = campaign?.session_id;
    if (!sid || !anchorComputed || sealBusy) return;
    const mean = sealMeanStr.trim() === '' ? fciGround : Number(sealMeanStr);
    const std = sealStdStr.trim() === '' ? 0.005 : Number(sealStdStr);
    if (!Number.isFinite(mean) || !Number.isFinite(std) || std <= 0) {
      setSealMsg('μ and σ must be finite numbers with σ > 0');
      return;
    }
    setSealBusy(true);
    setSealMsg(null);
    try {
      const resp = await sealPrediction({
        sessionId: sid,
        targetSystem,
        method: 'VQE UCCSD + Qiskit Aer',
        quantity: 'ground_state_energy_ha',
        expectedMean: mean,
        expectedStd: std,
        author: 'console-operator',
        pCorrelated: rho,
      });
      if (!resp.ok) {
        setSealMsg(resp.error || 'Seal failed');
        return;
      }
      const latest = await fetchLatestPrediction(sid);
      if (latest.ok) {
        setSealed(latest.sealed ?? null);
        setEvaluation(latest.evaluation ?? null);
      }
      setSealMsg(`Sealed ${resp.prediction_hash?.slice(0, 12)}…${resp.std_floor_applied ? ' (σ floored to 1e-4)' : ''}`);
    } finally {
      setSealBusy(false);
    }
  }, [campaign?.session_id, anchorComputed, sealBusy, sealMeanStr, sealStdStr, fciGround, targetSystem, rho]);

  // The pinned terrain point is the operator's explicit measurement:
  // sealing an expectation then evaluating the pinned value closes a
  // real (if manual) pre-registration loop against the simulator surface.
  const handleEvaluatePinned = useCallback(async () => {
    const sid = campaign?.session_id;
    const measured = selectedCoordinates?.energy;
    if (!sid || !sealed || measured == null || sealBusy) return;
    setSealBusy(true);
    setSealMsg(null);
    try {
      const resp = await evaluatePrediction({
        sessionId: sid,
        predictionHash: sealed.prediction_hash,
        measuredValue: measured,
      });
      if (!resp.ok) {
        setSealMsg(resp.error || 'Evaluate failed');
        return;
      }
      const latest = await fetchLatestPrediction(sid);
      if (latest.ok) setEvaluation(latest.evaluation ?? null);
      notify({
        level: resp.level === 'anomalous' || resp.level === 'notable' ? resp.level : 'normal',
        source: 'vault',
      });
    } finally {
      setSealBusy(false);
    }
  }, [campaign?.session_id, sealed, selectedCoordinates?.energy, sealBusy, notify]);

  // R-09: formal assertion. rho and evidence are caller judgment typed in
  // by the operator — the server validates shape, lineage, and bounds, and
  // never invents credibility.
  const handleAssert = useCallback(async () => {
    const sid = campaign?.session_id;
    if (!sid || !sealed || !evaluation || sealBusy) return;
    const rho = Number(assertRho);
    const evidence = assertEvidence
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    if (!Number.isFinite(rho) || rho <= 0 || rho > 1) {
      setAssertMsg('rho must be a number in (0, 1]');
      return;
    }
    if (evidence.length === 0) {
      setAssertMsg('at least one evidence reference is required');
      return;
    }
    setSealBusy(true);
    setAssertMsg(null);
    try {
      const resp = await assertPrediction({
        sessionId: sid,
        predictionHash: sealed.prediction_hash,
        rho,
        evidenceRefs: evidence,
      });
      if (!resp.ok) {
        setAssertMsg(resp.error || 'Assert failed');
        return;
      }
      const latest = await fetchLatestPrediction(sid);
      if (latest.ok) setAssertion(latest.assertion ?? null);
      setAssertMsg(`Asserted ρ=${rho} on ${sealed.prediction_hash.slice(0, 8)}…`);
    } finally {
      setSealBusy(false);
    }
  }, [campaign?.session_id, sealed, evaluation, sealBusy, assertRho, assertEvidence]);

  // Physical molecular bond dissociation energy De in Hartrees
  // (single source of truth: @/lib/morse).
  const De = useMemo(
    () => dissociationEnergyHartree(campaign?.target_system),
    [campaign?.target_system],
  );

  // Physical potential energy function in Hartrees (shared @/lib/morse):
  // Morse potential along bond displacement deltaR (in Ångströms), plus
  // harmonic confinement along orthogonal mode deltaZ.
  const computeEnergy = useCallback(
    (deltaR: number, deltaZ: number = 0) =>
      morseEnergyHartree(deltaR, deltaZ, De, fciGround, MORSE_WIDTH_A),
    [De, fciGround],
  );

  // Visual elevation mapping (shared @/lib/morse): converts physical energy
  // above ground state (deltaE) to 3D mesh height without letting compressed
  // nuclear core repulsion explode into vertical cliffs.
  const computeVisualHeight = useCallback(
    (energy: number) => morseVisualHeight(energy, fciGround, De),
    [De, fciGround],
  );

  // Q-37: capture the live terrain as a 1280x720 PNG data URL for the
  // images2PosedRGBD path. Cover-crops (never stretches) to the exact
  // posing grid — mirroring the tutorial's resize-then-center-crop rule,
  // because rescaling after posing invalidates intrinsics. Null when the
  // canvas is unreadable; the backend fail-closes on missing imagery.
  const captureSnapshotDataURL = (): string | null => {
    const canvas = canvasRef.current;
    if (!canvas || canvas.width === 0 || canvas.height === 0) return null;
    try {
      const out = document.createElement('canvas');
      out.width = 1280;
      out.height = 720;
      const ctx = out.getContext('2d');
      if (!ctx) return null;
      const srcW = canvas.width;
      const srcH = canvas.height;
      const targetAspect = 1280 / 720;
      let sx = 0;
      let sy = 0;
      let sw = srcW;
      let sh = srcH;
      if (srcW / srcH > targetAspect) {
        sw = srcH * targetAspect;
        sx = (srcW - sw) / 2;
      } else {
        sh = srcW / targetAspect;
        sy = (srcH - sh) / 2;
      }
      ctx.drawImage(canvas, sx, sy, sw, sh, 0, 0, 1280, 720);
      return out.toDataURL('image/png');
    } catch {
      return null;
    }
  };

  // P1: depth-conditioned chisel. Renders the ledger-grounded terrain
  // depth offscreen from the CURRENT orbit pose, exports uncompressed EXR,
  // uploads as a project asset, then dispatches chisel through the parent
  // rail (spend + poll + promptUsed resolve stay in one place). The orbit
  // pose arrives as an argument from the click handler (event handlers may
  // read refs; render-phase callbacks may not).
  const [chiselBusy, setChiselBusy] = useState(false);
  const [chiselMsg, setChiselMsg] = useState<string | null>(null);

  const handleEncodeDepthChisel = useCallback(async (
    camPos: [number, number, number],
    camQuat: [number, number, number, number],
  ) => {
    const sid = campaign?.session_id;
    if (!sid || !anchorComputed || isGeneratingLens || chiselBusy) return;
    setChiselBusy(true);
    setChiselMsg(null);
    try {
      // Wave-3: three + EXRExporter enter only here (dynamic import), so
      // the chisel path never costs first paint — or initial 3D open.
      const { buildTerrainHeightField, renderTerrainDepthGL, encodeDepthEXR } =
        await import('@/lib/depth-encode');
      const N = 65;
      const W = 4.0;
      const heights = buildTerrainHeightField(N, W, De, fciGround);
      const depth = renderTerrainDepthGL(
        heights,
        N,
        W,
        { position: camPos, quaternion: camQuat },
        1280,
        720,
      );
      const exr = await encodeDepthEXR(depth, 1280, 720);
      let bin = '';
      const CHUNK = 0x8000;
      for (let i = 0; i < exr.length; i += CHUNK) {
        bin += String.fromCharCode(...exr.subarray(i, i + CHUNK));
      }
      const dataUrl = `data:image/x-exr;base64,${btoa(bin)}`;
      const up = await uploadObservatoryAsset({
        sessionId: sid,
        dataBase64: dataUrl,
        displayName: `kubona-pes-${targetSystem}.exr`,
      });
      if (!up.ok || !up.asset_id) {
        setChiselMsg(up.error || 'Depth upload failed');
        return;
      }
      onRequestDepthChisel?.(up.asset_id, { position: [...camPos], orientation: [...camQuat] });
    } catch (e) {
      setChiselMsg((e as Error).message || 'Depth encode failed');
    } finally {
      setChiselBusy(false);
    }
  }, [campaign?.session_id, anchorComputed, isGeneratingLens, chiselBusy, De, fciGround, targetSystem, onRequestDepthChisel]);

  // Stable references for Three.js objects
  const sceneRef = useRef<THREE.Scene | null>(null);
  const cameraRef = useRef<THREE.PerspectiveCamera | null>(null);
  const rendererRef = useRef<THREE.WebGLRenderer | null>(null);
  const controlsRef = useRef<OrbitControls | null>(null);
  const terrainMeshRef = useRef<THREE.Mesh | null>(null);
  const ghostMeshRef = useRef<THREE.Mesh | null>(null);
  // R-07 μ±σ band: translucent wireframe shells above/below the μ plane.
  const ghostUpperRef = useRef<THREE.Mesh | null>(null);
  const ghostLowerRef = useRef<THREE.Mesh | null>(null);
  const vsupMaterialRef = useRef<THREE.ShaderMaterial | null>(null);
  // R-14 agent gaze trails (ArrowHelpers owned by a group).
  const gazeGroupRef = useRef<THREE.Group | null>(null);
  const selectionPinRef = useRef<THREE.Mesh | null>(null);
  const diamondMeshRef = useRef<THREE.Mesh | null>(null);

  // Props ref to give event listeners access to latest props without re-creating scene
  const propsRef = useRef({
    equilibriumR,
    fciGround,
    rho,
    targetSystem,
    sessionId: campaign?.session_id ?? null,
    onSelectPoint,
    appendLedgerEvent,
    computeEnergy,
    computeVisualHeight,
  });
  useEffect(() => {
    propsRef.current = {
      equilibriumR,
      fciGround,
      rho,
      targetSystem,
      sessionId: campaign?.session_id ?? null,
      onSelectPoint,
      appendLedgerEvent,
      computeEnergy,
      computeVisualHeight,
    };
  });

  // Toggle Vault Ghost contour visibility without touching camera/controls
  useEffect(() => {
    if (ghostMeshRef.current) {
      ghostMeshRef.current.visible = showGhostContour;
    }
    if (ghostUpperRef.current) {
      ghostUpperRef.current.visible = showGhostContour && viewMean != null;
    }
    if (ghostLowerRef.current) {
      ghostLowerRef.current.visible = showGhostContour && viewMean != null;
    }
  }, [showGhostContour, viewMean]);

  // R-14: rebuild agent gaze arrows from the latest world.gaze annotations
  // (cap 8, newest last). Cones point along the agent quaternion (-Z forward).
  useEffect(() => {
    const group = gazeGroupRef.current;
    if (!group) return;
    for (const c of [...group.children]) {
      group.remove(c);
      c.traverse((o) => {
        const mesh = o as THREE.Mesh;
        if (mesh.isMesh) {
          mesh.geometry.dispose();
          (mesh.material as THREE.Material).dispose();
        }
      });
    }
    const up = new THREE.Vector3(0, 0, -1);
    for (const g of gazes.slice(-8)) {
      const dir = up.clone().applyQuaternion(new THREE.Quaternion(g.quaternion[0], g.quaternion[1], g.quaternion[2], g.quaternion[3]));
      if (dir.lengthSq() < 1e-12) continue;
      dir.normalize();
      const arrow = new THREE.ArrowHelper(dir, new THREE.Vector3(g.position[0], g.position[1], g.position[2]), 0.6, 0x7dd3fc, 0.15, 0.08);
      const line = arrow.line.material as THREE.Material;
      const cone = arrow.cone.material as THREE.Material;
      line.transparent = true;
      line.opacity = 0.55;
      cone.transparent = true;
      cone.opacity = 0.55;
      group.add(arrow);
    }
  }, [gazes]);

  // Update terrain mesh in-place if chemical parameters change (without camera reset!)
  useEffect(() => {
    const terrain = terrainMeshRef.current;
    if (!terrain) return;
    const geo = terrain.geometry;
    const posAttr = geo.attributes.position;
    const energyArray = geo.attributes.energy?.array as Float32Array;
    const confidenceArray = geo.attributes.confidence?.array as Float32Array;
    if (!posAttr || !energyArray || !confidenceArray) return;

    for (let i = 0; i < posAttr.count; i++) {
      const x = posAttr.getX(i);
      const z = posAttr.getZ(i);
      const deltaR = x * 0.4;
      const energy = computeEnergy(deltaR, z);
      const height = computeVisualHeight(energy);
      posAttr.setY(i, height);

      energyArray[i] = energy;
      const distFromEquilibrium = Math.sqrt(x * x + z * z);
      const conf = Math.max(0.1, Math.min(1.0, rho * Math.exp(-0.8 * distFromEquilibrium)));
      confidenceArray[i] = conf;
    }

    posAttr.needsUpdate = true;
    (geo.attributes.energy as THREE.BufferAttribute).needsUpdate = true;
    (geo.attributes.confidence as THREE.BufferAttribute).needsUpdate = true;
    geo.computeVertexNormals();

    // Update ghost band in-place (R-06/R-07). Bound: flat μ plane plus
    // μ±σ wireframe shells from the ledger (or the scrubbed replay entry);
    // the terrain shader tints the divergence gap via uSealedMean/uSealedStd.
    // Unbound: the illustrative +0.12 offset over the live surface.
    const band =
      viewMean != null && Number.isFinite(viewMean) && viewStd != null && Number.isFinite(viewStd) && viewStd > 0
        ? ghostBandHeights(viewMean, viewStd, fciGround, De)
        : null;
    const setPlaneY = (mesh: THREE.Mesh | null, y: number | null, fallback: (i: number, gx: number, gz: number) => number) => {
      if (!mesh) return;
      const g = mesh.geometry;
      const p = g.attributes.position;
      for (let i = 0; i < p.count; i++) {
        if (y != null) {
          p.setY(i, y);
        } else {
          p.setY(i, fallback(i, p.getX(i), p.getZ(i)));
        }
      }
      p.needsUpdate = true;
      g.computeVertexNormals();
    };
    const illustrative = (_i: number, gx: number, gz: number) =>
      computeVisualHeight(computeEnergy(gx * 0.4, gz)) + 0.12;
    setPlaneY(ghostMeshRef.current, band ? band.mid : null, illustrative);
    setPlaneY(ghostUpperRef.current, band ? band.upper : null, illustrative);
    setPlaneY(ghostLowerRef.current, band ? band.lower : null, illustrative);
    if (ghostUpperRef.current) ghostUpperRef.current.visible = showGhostContour && band != null;
    if (ghostLowerRef.current) ghostLowerRef.current.visible = showGhostContour && band != null;

    // Gap tint (R-06): |z| bands painted onto the terrain. Same cutoffs as
    // divergenceBand() — the GLSL below mirrors them.
    const mat = vsupMaterialRef.current;
    if (mat) {
      mat.uniforms.uSealedMean.value = viewMean ?? 0;
      mat.uniforms.uSealedStd.value = viewStd && viewStd > 0 ? viewStd : 1;
      mat.uniforms.uShowGap.value = band != null ? 1 : 0;
    }
  }, [computeEnergy, computeVisualHeight, equilibriumR, fciGround, rho, viewMean, viewStd, showGhostContour, De]);

  // Core Three.js Lifecycle: Mounts ONCE
  useEffect(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({
        canvas,
        antialias: true,
        alpha: true,
        powerPreference: 'high-performance',
        // Q-37: readable pixels for the snapshot path. Without this the
        // drawing buffer may be cleared before toDataURL runs.
        preserveDrawingBuffer: true,
      });
    } catch (e) {
      // Wave-3 fail-closed: no GL → no live-looking HUD. The panel below
      // replaces the whole observatory chrome (message constant is exported
      // for tests: WEBGL_UNAVAILABLE_MESSAGE). Deferred to a microtask so
      // the mount effect never setStates synchronously (render-phase rules).
      const message = (e as Error)?.message || 'WebGL unavailable';
      queueMicrotask(() => setWebglError(message));
      return;
    }

    // Wave-3 mobile: coarse pointers cap at 1.5 DPR; desktop caps at 2.
    // Recomputed here (not from state) so the mount-time value is exact
    // even before the coarse-pointer effect above commits.
    const coarseNow =
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(pointer: coarse)').matches;
    renderer.setPixelRatio(initialPixelRatioCap(window.devicePixelRatio || 1, coarseNow));
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.toneMappingExposure = 1.1;
    rendererRef.current = renderer;

    const scene = new THREE.Scene();
    scene.background = new THREE.Color(0x07080b);
    sceneRef.current = scene;

    const camera = new THREE.PerspectiveCamera(
      45,
      container.clientWidth / container.clientHeight,
      0.1,
      100
    );
    camera.position.set(3.5, 3.2, 4.5);
    cameraRef.current = camera;

    const controls = new OrbitControls(camera, canvas);
    controls.enableDamping = true;
    controls.dampingFactor = 0.05;
    controls.maxPolarAngle = Math.PI / 2 + 0.05;
    controls.minDistance = 1.5;
    controls.maxDistance = 12.0;
    controls.target.set(0, 0, 0);
    controlsRef.current = controls;

    // Diegetic Lighting
    const ambientLight = new THREE.AmbientLight(0xffffff, 0.45);
    scene.add(ambientLight);

    const keyLight = new THREE.DirectionalLight(0x00e5ff, 1.2);
    keyLight.position.set(5, 8, 4);
    scene.add(keyLight);

    const fillLight = new THREE.DirectionalLight(0xffb300, 0.6);
    fillLight.position.set(-5, 4, -4);
    scene.add(fillLight);

    // Initial Physical Morse Potential Terrain
    const gridSize = 64;
    const worldWidth = 4.0;
    const worldDepth = 4.0;
    const terrainGeo = new THREE.PlaneGeometry(worldWidth, worldDepth, gridSize, gridSize);
    terrainGeo.rotateX(-Math.PI / 2);

    const posAttr = terrainGeo.attributes.position;
    const energyArray = new Float32Array(posAttr.count);
    const confidenceArray = new Float32Array(posAttr.count);

    for (let i = 0; i < posAttr.count; i++) {
      const x = posAttr.getX(i);
      const z = posAttr.getZ(i);
      const deltaR = x * 0.4;
      const energy = propsRef.current.computeEnergy(deltaR, z);
      const height = propsRef.current.computeVisualHeight(energy);
      posAttr.setY(i, height);

      energyArray[i] = energy;
      const distFromEquilibrium = Math.sqrt(x * x + z * z);
      const conf = Math.max(0.1, Math.min(1.0, propsRef.current.rho * Math.exp(-0.8 * distFromEquilibrium)));
      confidenceArray[i] = conf;
    }

    terrainGeo.setAttribute('energy', new THREE.BufferAttribute(energyArray, 1));
    terrainGeo.setAttribute('confidence', new THREE.BufferAttribute(confidenceArray, 1));
    terrainGeo.computeVertexNormals();

    const vsupMaterial = new THREE.ShaderMaterial({
      uniforms: {
        uSealedMean: { value: propsRef.current.fciGround },
        uSealedStd: { value: 1.0 },
        uShowGap: { value: 0 },
      },
      vertexShader: `
        attribute float energy;
        attribute float confidence;
        varying vec3 vWorldPosition;
        varying vec3 vNormal;
        varying float vEnergy;
        varying float vConfidence;

        void main() {
          vEnergy = energy;
          vConfidence = confidence;
          vNormal = normalize(normalMatrix * normal);
          vec4 worldPos = modelMatrix * vec4(position, 1.0);
          vWorldPosition = worldPos.xyz;
          gl_Position = projectionMatrix * viewMatrix * worldPos;
        }
      `,
      fragmentShader: `
        varying vec3 vWorldPosition;
        varying vec3 vNormal;
        varying float vEnergy;
        varying float vConfidence;
        uniform float uSealedMean;
        uniform float uSealedStd;
        uniform float uShowGap;

        void main() {
          vec3 lightDir = normalize(vec3(0.5, 1.0, 0.4));
          float diff = max(dot(vNormal, lightDir), 0.15);

          // Topographic Isolines (concentric potential contour rings)
          float contour = mod(vWorldPosition.y * 4.0, 0.4);
          float line = smoothstep(0.028, 0.0, abs(contour - 0.2));

          // Base VSUP colors
          vec3 confidentColor = mix(vec3(0.0, 0.9, 1.0), vec3(1.0, 0.7, 0.0), clamp(vWorldPosition.y * 0.7, 0.0, 1.0));
          vec3 uncertainColor = vec3(0.09, 0.12, 0.18);

          // Value-suppression
          vec3 surfaceColor = mix(uncertainColor, confidentColor, vConfidence);
          surfaceColor += vec3(0.0, 0.8, 1.0) * line * 0.6 * vConfidence;

          // Wireframe grid lines on terrain
          vec2 grid = abs(fract(vWorldPosition.xz * 2.0 - 0.5) - 0.5) / fwidth(vWorldPosition.xz * 2.0);
          float gridLine = 1.0 - min(min(grid.x, grid.y), 1.0);
          surfaceColor += vec3(0.12, 0.18, 0.25) * gridLine * 0.35;

          // R-06 surprise gap tint: divergence of this vertex from the
          // sealed expectation. Cutoffs mirror divergenceBand() in
          // vault-view.ts (|z|<1 cyan keep, 1-3 amber, >3 rose).
          if (uShowGap > 0.5 && uSealedStd > 0.0) {
            float z = (vEnergy - uSealedMean) / uSealedStd;
            float az = abs(z);
            vec3 gapColor = vec3(0.0);
            float gapAmt = 0.0;
            if (az > 3.0) {
              gapColor = vec3(1.0, 0.15, 0.35);
              gapAmt = 0.55;
            } else if (az >= 1.0) {
              gapColor = vec3(1.0, 0.65, 0.1);
              gapAmt = 0.45;
            }
            surfaceColor = mix(surfaceColor, gapColor, gapAmt * vConfidence);
          }

          gl_FragColor = vec4(surfaceColor * (diff * 0.85 + 0.35), 0.94);
        }
      `,
      transparent: true,
      side: THREE.DoubleSide,
    });

  const terrainMesh = new THREE.Mesh(terrainGeo, vsupMaterial);
  scene.add(terrainMesh);
  terrainMeshRef.current = terrainMesh;
  vsupMaterialRef.current = vsupMaterial;

    // Coordinate Grid Base
    const gridHelper = new THREE.GridHelper(worldWidth * 1.5, 24, 0x00e5ff, 0x1e293b);
    gridHelper.position.y = -0.05;
    scene.add(gridHelper);

    // Classical Ground State Anchor Beacon
    const anchorGroup = new THREE.Group();
    anchorGroup.position.set(0, 0, 0);

    const diamondGeo = new THREE.OctahedronGeometry(0.09, 0);
    const diamondMat = new THREE.MeshStandardMaterial({
      color: 0x00e676,
      emissive: 0x00e676,
      emissiveIntensity: 0.8,
      roughness: 0.2,
      metalness: 0.8,
    });
    const diamondMesh = new THREE.Mesh(diamondGeo, diamondMat);
    diamondMesh.position.y = 0.15;
    anchorGroup.add(diamondMesh);
    diamondMeshRef.current = diamondMesh;

    const anchorRingGeo = new THREE.RingGeometry(0.12, 0.15, 32);
    anchorRingGeo.rotateX(-Math.PI / 2);
    const anchorRingMat = new THREE.MeshBasicMaterial({ color: 0x00e676, side: THREE.DoubleSide });
    const anchorRing = new THREE.Mesh(anchorRingGeo, anchorRingMat);
    anchorRing.position.y = 0.02;
    anchorGroup.add(anchorRing);
    scene.add(anchorGroup);

    // Tier-1 Pavilion Shell (P5): procedural architectural frame — obsidian
    // floor, titanium pillars, quartz window glow, ceiling ring. Pure local
    // geometry: the room exists with or without Atlas, so air-gap keeps a
    // walkable world. All pieces disposed in cleanup below.
    const pavilion = new THREE.Group();
    const floorGeo = new THREE.CircleGeometry(6.5, 48);
    floorGeo.rotateX(-Math.PI / 2);
    const floorMat = new THREE.MeshStandardMaterial({ color: 0x0b0e14, roughness: 0.35, metalness: 0.75 });
    const floorMesh = new THREE.Mesh(floorGeo, floorMat);
    floorMesh.position.y = -0.08;
    pavilion.add(floorMesh);

    const pillarGeo = new THREE.CylinderGeometry(0.06, 0.09, 3.0, 12);
    const pillarMat = new THREE.MeshStandardMaterial({ color: 0x3a4356, roughness: 0.4, metalness: 0.85 });
    const glowGeo = new THREE.PlaneGeometry(2.4, 1.5);
    const glowMat = new THREE.MeshBasicMaterial({ color: 0x00e5ff, transparent: true, opacity: 0.07, side: THREE.DoubleSide });
    for (let k = 0; k < 6; k++) {
      const a = (k / 6) * Math.PI * 2;
      const px = Math.cos(a) * 4.2;
      const pz = Math.sin(a) * 4.2;
      const pillar = new THREE.Mesh(pillarGeo, pillarMat);
      pillar.position.set(px, 1.42, pz);
      pavilion.add(pillar);
      const glow = new THREE.Mesh(glowGeo, glowMat);
      const na = a + Math.PI / 6;
      glow.position.set(Math.cos(na) * 4.2, 1.5, Math.sin(na) * 4.2);
      glow.lookAt(0, 1.5, 0);
      pavilion.add(glow);
    }
    const ringGeo = new THREE.TorusGeometry(4.2, 0.045, 12, 64);
    ringGeo.rotateX(Math.PI / 2);
    const ringMat = new THREE.MeshBasicMaterial({ color: 0x00e5ff, transparent: true, opacity: 0.35 });
    const ringMesh = new THREE.Mesh(ringGeo, ringMat);
    ringMesh.position.y = 2.95;
    pavilion.add(ringMesh);
    scene.add(pavilion);

    // Pre-Registration Vault Ghost Plane + μ±σ band (R-07). Three planes
    // sharing one footprint: translucent μ sheet + faint wireframe shells.
    // In-place heights are set by the update effect; unbound, only the mid
    // plane shows (illustrative +0.12 offset over the live surface).
    const ghostGeo = new THREE.PlaneGeometry(worldWidth * 0.8, worldDepth * 0.8, 16, 16);
    ghostGeo.rotateX(-Math.PI / 2);
    const ghostPos = ghostGeo.attributes.position;
    for (let i = 0; i < ghostPos.count; i++) {
      const gx = ghostPos.getX(i);
      const gz = ghostPos.getZ(i);
      const gDeltaR = gx * 0.4;
      ghostPos.setY(i, propsRef.current.computeVisualHeight(propsRef.current.computeEnergy(gDeltaR, gz)) + 0.12);
    }
    ghostGeo.computeVertexNormals();

    const ghostMat = new THREE.MeshBasicMaterial({
      color: 0x00e5ff,
      wireframe: true,
      transparent: true,
      opacity: 0.35,
    });
    const ghostMesh = new THREE.Mesh(ghostGeo, ghostMat);
    scene.add(ghostMesh);
    ghostMeshRef.current = ghostMesh;

    const bandMatOpts = { color: 0x00e5ff, wireframe: true, transparent: true, opacity: 0.15 };
    const ghostUpper = new THREE.Mesh(ghostGeo.clone(), new THREE.MeshBasicMaterial(bandMatOpts));
    const ghostLower = new THREE.Mesh(ghostGeo.clone(), new THREE.MeshBasicMaterial(bandMatOpts));
    ghostUpper.visible = false;
    ghostLower.visible = false;
    scene.add(ghostUpper);
    scene.add(ghostLower);
    ghostUpperRef.current = ghostUpper;
    ghostLowerRef.current = ghostLower;

    // Selection Indicator Beacon Pin + provenance stain halo (R-12).
    // Every pin is a human touch: the click appends a touched:true ledger
    // record and the amber ring marks the stained coordinates.
    const pinGeo = new THREE.CylinderGeometry(0.02, 0.02, 0.8, 16);
    const pinMat = new THREE.MeshBasicMaterial({ color: 0xffb300 });
    const selectionPin = new THREE.Mesh(pinGeo, pinMat);
    selectionPin.visible = false;
    scene.add(selectionPin);
    selectionPinRef.current = selectionPin;
    const haloGeo = new THREE.TorusGeometry(0.14, 0.015, 8, 32);
    haloGeo.rotateX(Math.PI / 2);
    const haloMat = new THREE.MeshBasicMaterial({ color: 0xffb300, transparent: true, opacity: 0.85 });
    const stainHalo = new THREE.Mesh(haloGeo, haloMat);
    stainHalo.visible = false;
    scene.add(stainHalo);

    // R-14 agent gaze trails: one ArrowHelper per recent world.gaze,
    // rebuilt by the gaze effect below. Empty until agents look.
    const gazeGroup = new THREE.Group();
    scene.add(gazeGroup);
    gazeGroupRef.current = gazeGroup;

    // Raycasting for interactive click & hover
    const raycaster = new THREE.Raycaster();
    const pointer = new THREE.Vector2();

    // Separate Drag/Orbit from Click
    let pointerDownPos = { x: 0, y: 0 };
    let isPointerDown = false;
    // Declared early: the context-lost handler (below) must cancel the
    // loop, and `let` bindings are TDZ-sensitive.
    let animId = 0;

    const handlePointerDown = (e: MouseEvent) => {
      isPointerDown = true;
      pointerDownPos = { x: e.clientX, y: e.clientY };
    };

    const handlePointerUp = (e: MouseEvent) => {
      if (!isPointerDown) return;
      isPointerDown = false;
      const dragDist = Math.hypot(e.clientX - pointerDownPos.x, e.clientY - pointerDownPos.y);
      if (dragDist > 5) {
        // User was rotating/panning/dragging! Ignore click!
        return;
      }

      const rect = canvas.getBoundingClientRect();
      pointer.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

      raycaster.setFromCamera(pointer, camera);
      const intersects = raycaster.intersectObject(terrainMesh);
      if (intersects.length > 0) {
        const pt = intersects[0].point;
        // R-24: coordinate + payload math lives in lib/pin (unit-tested);
        // this handler only positions meshes and appends the record.
        const pin = pinValuesFromHit(pt.x, pt.z, propsRef.current.equilibriumR, propsRef.current.computeEnergy);
        const rVal = pin.r;
        const eVal = pin.energy;

        // R-12/R-24: every pin is a human touch. The coordinates render
        // immediately; the stain (touched:true ledger record) lands async
        // and its hash turns the pin into a click-to-ledger-ID.
        setSelectedCoordinates({ r: rVal, energy: eVal, ledgerHash: null });
        selectionPin.position.set(pt.x, pt.y + 0.3, pt.z);
        selectionPin.visible = true;
        stainHalo.position.set(pt.x, pt.y + 0.02, pt.z);
        stainHalo.visible = true;

        const sid = propsRef.current.sessionId;
        if (sid) {
          propsRef.current
            .appendLedgerEvent(sid, 'annotation', buildPinPayload({ r: rVal, energy: eVal }))
            .then((res) => {
              if (res.ok && res.record_hash) {
                setSelectedCoordinates((prev) => (prev ? { ...prev, ledgerHash: res.record_hash as string } : prev));
              }
            })
            .catch(() => {
              /* stain is advisory; the pin stands without it */
            });
        }

        if (propsRef.current.onSelectPoint) {
          propsRef.current.onSelectPoint({
            bondDistance: rVal,
            energy: eVal,
            label: `${propsRef.current.targetSystem} PES (R=${rVal}Å)`,
          });
        }
      }
    };

    // Wave-3 hover: rAF-coalesce the mousemove raycast. Pointer events only
    // stash NDC; a single rAF per frame does the raycast + setState, and
    // setState fires only when the rounded (r, energy) actually changes.
    // This collapses mousemove storms to ≤1 raycast/frame with no HUD churn.
    let hoverRaf = 0;
    const pendingPointer = { x: 0, y: 0, has: false };
    const lastHoverRef: { current: { r: number; energy: number } | null } = { current: null };
    const runHoverRaycast = () => {
      hoverRaf = 0;
      if (!pendingPointer.has) return;
      pendingPointer.has = false;
      pointer.x = pendingPointer.x;
      pointer.y = pendingPointer.y;
      raycaster.setFromCamera(pointer, camera);
      const intersects = raycaster.intersectObject(terrainMesh);
      if (intersects.length > 0) {
        const pt = intersects[0].point;
        const deltaR = pt.x * 0.4;
        const curEq = propsRef.current.equilibriumR;
        const rounded = roundHoverReading(
          curEq + deltaR,
          propsRef.current.computeEnergy(deltaR, pt.z),
        );
        if (hoverReadingChanged(lastHoverRef.current, rounded)) {
          lastHoverRef.current = rounded;
          setHoveredCoordinates(rounded);
        }
      } else if (lastHoverRef.current !== null) {
        lastHoverRef.current = null;
        setHoveredCoordinates(null);
      }
    };

    const handlePointerMove = (e: MouseEvent) => {
      // Don't raycast hover while orbiting/dragging
      if (isPointerDown) return;

      const rect = canvas.getBoundingClientRect();
      pendingPointer.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      pendingPointer.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
      pendingPointer.has = true;
      if (hoverRaf === 0) {
        hoverRaf = requestAnimationFrame(runHoverRaycast);
      }
    };

    // Wave-3 fail-closed: a lost GPU context can never leave a blank canvas
    // under a live-looking HUD. Prevent the default (which would allow a
    // silent restore into a stale scene) and swap to the panel instead.
    const handleContextLost = (e: Event) => {
      e.preventDefault();
      cancelAnimationFrame(animId);
      if (hoverRaf !== 0) {
        cancelAnimationFrame(hoverRaf);
        hoverRaf = 0;
      }
      setWebglError('GPU context lost');
    };
    canvas.addEventListener('webglcontextlost', handleContextLost);

    canvas.addEventListener('pointerdown', handlePointerDown);
    window.addEventListener('pointerup', handlePointerUp);
    canvas.addEventListener('mousemove', handlePointerMove);

    // Resize Handler
    const handleResize = () => {
      if (!container || !renderer || !camera) return;
      const width = container.clientWidth;
      const height = container.clientHeight;
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
      renderer.setSize(width, height);
    };

    const resizeObserver = new ResizeObserver(handleResize);
    resizeObserver.observe(container);

    // Animation Render Loop
    const startTime = performance.now();
    if (!fpsRef.current) fpsRef.current = createFpsAccumulator();
    const fpsAcc = fpsRef.current;

    const animate = () => {
      animId = requestAnimationFrame(animate);
      const elapsed = (performance.now() - startTime) / 1000.0;
      const sample = fpsAcc.tick(performance.now());
      if (sample) {
        setFps(Math.round(sample.fps));
        // Wave-3 fps-driven step-down: when measured fps sags, drop the
        // pixel ratio a notch (pure policy in @/lib/fps). One-way per
        // sample so a single bad second can't collapse to MIN_DPR.
        const stepped = pixelRatioStepDown(renderer.getPixelRatio(), sample.fps);
        if (stepped !== renderer.getPixelRatio()) renderer.setPixelRatio(stepped);
      }

      if (diamondMeshRef.current) {
        diamondMeshRef.current.rotation.y = elapsed * 0.8;
        diamondMeshRef.current.position.y = 0.14 + Math.sin(elapsed * 2.0) * 0.02;
      }

      controls.update();
      renderer.render(scene, camera);
    };

    animate();

    return () => {
      cancelAnimationFrame(animId);
      if (hoverRaf !== 0) cancelAnimationFrame(hoverRaf);
      resizeObserver.disconnect();
      canvas.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('pointerup', handlePointerUp);
      canvas.removeEventListener('mousemove', handlePointerMove);
      canvas.removeEventListener('webglcontextlost', handleContextLost);

      controls.dispose();
      terrainGeo.dispose();
      vsupMaterial.dispose();
      diamondGeo.dispose();
      diamondMat.dispose();
      anchorRingGeo.dispose();
      anchorRingMat.dispose();
      ghostGeo.dispose();
      ghostMat.dispose();
      ghostUpper.geometry.dispose();
      (ghostUpper.material as THREE.Material).dispose();
      ghostLower.geometry.dispose();
      (ghostLower.material as THREE.Material).dispose();
      pinGeo.dispose();
      pinMat.dispose();
      haloGeo.dispose();
      haloMat.dispose();
      gazeGroup.children.slice().forEach((c) => {
        gazeGroup.remove(c);
        c.traverse((o) => {
          const mesh = o as THREE.Mesh;
          if (mesh.isMesh) {
            mesh.geometry.dispose();
            (mesh.material as THREE.Material).dispose();
          }
        });
      });
      gridHelper.dispose();
      floorGeo.dispose();
      floorMat.dispose();
      pillarGeo.dispose();
      pillarMat.dispose();
      glowGeo.dispose();
      glowMat.dispose();
      ringGeo.dispose();
      ringMat.dispose();
      renderer.dispose();
    };
  }, []); // Run ONCE on mount! Never tear down on state re-renders!

  // Wave-3 fail-closed: renderer creation threw or the GPU context was
  // lost. Render the panel INSTEAD of the canvas + live HUD — a blank
  // WebGL canvas under live-looking numbers is the failure mode this
  // replaces. The 2D workbench (parent) stays mounted below.
  if (webglError) {
    return (
      <div
        role="alert"
        className="relative w-full h-full min-h-[520px] rounded-2xl overflow-hidden bg-[#07080B] border border-amber-400/30 flex flex-col items-center justify-center gap-2 p-8 text-center"
      >
        <span className="text-sm font-medium text-amber-200">{WEBGL_UNAVAILABLE_MESSAGE}</span>
        <span className="text-[11px] font-mono text-slate-400 max-w-sm">
          The GPU renderer could not start ({webglError}). Your campaign data
          is intact — continue in the 2D workbench.
        </span>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className={`relative w-full h-full min-h-[520px] rounded-2xl overflow-hidden bg-[#07080B] border border-white/10 ${
        isFullscreen ? 'fixed inset-0 z-50 rounded-none' : ''
      }`}
    >
      {/* Three.js Rendering Canvas */}
      <canvas ref={canvasRef} className="w-full h-full block cursor-crosshair" />

      {/* Diegetic Top HUD: Target System & Analytical Ground Truth */}
      <div className="absolute top-4 left-4 right-4 flex items-center justify-between pointer-events-none">
        <div className="flex items-center gap-3 pointer-events-auto bg-[#0E1117]/85 backdrop-blur-xl border border-white/10 rounded-full px-4 py-2 shadow-2xl">
          <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse" />
          <span className="font-code text-xs text-white font-medium tracking-wide">
            {targetSystem} Potential Energy Surface
          </span>
          <span className="text-white/20">|</span>
          <span className="font-code text-[11px] text-cyan-400">
            Re: {equilibriumR.toFixed(4)} Å
          </span>
          <span className="text-white/20">|</span>
          <span className="font-code text-[11px] text-slate-300" title={anchorComputed ? 'Computed classical anchor' : 'No computed anchor: terrain is illustrative'}>
            FCI Anchor: {anchorComputed ? `${fciGround.toFixed(4)} Ha` : 'UNVERIFIED'}
          </span>
          <span className="text-white/20">|</span>
          <div className="flex items-center gap-1.5 text-[11px] font-code text-amber-300">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
            <span title={anchorComputed ? 'Correlated fraction from the live campaign' : 'No computed anchor — confidence uncomputed'}>{anchorComputed && campaign?.epistemics?.p_correlated != null ? `ρ = ${rho.toFixed(2)}` : 'ρ uncomputed'}</span>
          </div>
          <span className="text-white/20">|</span>
          <span className="font-code text-[11px] text-slate-400" title="Measured render-loop rate (R-23)">
            {fps == null ? '… fps' : `${fps} fps`}
          </span>
        </div>

        {/* Viewport Control Badges */}
        <div className="flex items-center gap-2 pointer-events-auto">
        {/* Ghost contour toggle.
            Bound (P3+R-07): translucent μ plane with ±σ wireframe shells from
            the ledger (or the replay scrub position). Unbound: illustrative
            +0.12 offset wireframe. Gap tint (R-06) paints |z| bands on the
            terrain while bound: cyan &lt;1 · amber 1–3 · rose &gt;3. */}
          <button
            onClick={() => setShowGhostContour((prev) => !prev)}
            title={
              sealed
                ? `Sealed expectation μ=${sealed.expected_mean} ± ${sealed.expected_std} Ha (${sealed.prediction_hash.slice(0, 8)}…); amber/rose = divergence gap`
                : 'No sealed prediction yet — ghost is an illustrative placeholder; seal one in the Vault panel'
            }
            className={`px-3 py-1.5 rounded-full border text-[11px] font-code transition-all flex items-center gap-1.5 backdrop-blur-xl shadow-lg ${
              showGhostContour
                ? 'bg-cyan-500/15 border-cyan-400/40 text-cyan-300'
                : 'bg-white/5 border-white/10 text-slate-400 hover:text-white'
            }`}
          >
          <Layers className="w-3.5 h-3.5" />
          <span>{sealed ? 'Vault Ghost (μ±σ bound)' : 'Vault Ghost (illustrative)'}</span>
        </button>

          {/* Fullscreen toggle */}
          <button
            onClick={() => setIsFullscreen((prev) => !prev)}
            title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
            className="w-8 h-8 rounded-full bg-[#0E1117]/80 backdrop-blur-xl border border-white/10 text-slate-300 hover:text-white flex items-center justify-center transition-all shadow-lg"
          >
            {isFullscreen ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
          </button>
        </div>
      </div>

      {/* Diegetic Bottom HUD: Coordinates & Generative Lens Trigger */}
      <div className="absolute bottom-4 left-4 right-4 flex items-end justify-between pointer-events-none">
        <div className="pointer-events-auto flex flex-col gap-2 max-w-sm">
        {/* Raycasted Coordinates Readout */}
        <div className="bg-[#0E1117]/90 backdrop-blur-xl border border-white/10 rounded-xl p-3 shadow-2xl">
          <div className="text-[10px] uppercase font-code tracking-wider text-slate-400 mb-1 flex items-center gap-1.5">
            <Sparkles className="w-3 h-3 text-cyan-400" />
            <span>Spatial Inspection Probe</span>
          </div>
          {!anchorComputed && (
            <div className="mb-1 text-[10px] font-code text-amber-300 border border-amber-400/40 rounded px-1.5 py-0.5">
              UNVERIFIED terrain — illustrative
            </div>
          )}
          {hoveredCoordinates || selectedCoordinates ? (
            <div className="space-y-1 font-code text-xs">
              <div className="flex justify-between gap-4">
                <span className="text-slate-400">Bond Dist (R):</span>
                <span className="text-white font-medium">
                  {(hoveredCoordinates?.r || selectedCoordinates?.r || 0).toFixed(4)} Å
                </span>
              </div>
                <div className="flex justify-between gap-4">
                <span className="text-slate-400">Energy V(R):</span>
                <span className="text-cyan-300 font-medium">
                  {(hoveredCoordinates?.energy || selectedCoordinates?.energy || 0).toFixed(5)} Ha
                </span>
              </div>
              {!hoveredCoordinates && selectedCoordinates?.ledgerHash && (
                <div className="flex justify-between gap-4">
                  <span className="text-slate-400">Ledger stain:</span>
                  <span className="text-amber-300/90 font-medium truncate" title={selectedCoordinates.ledgerHash}>
                    #{selectedCoordinates.ledgerHash.slice(0, 12)}…
                  </span>
                </div>
              )}
              <div className="text-[10px] text-slate-500 pt-1 border-t border-white/10">
                {touchProbeHint(isCoarsePointer)}
              </div>
            </div>
          ) : (
            <div className="font-code text-xs text-slate-400">
              {isCoarsePointer
                ? 'Tap to pin, drag to orbit — Hamiltonian coordinates read out here.'
                : 'Hover or click anywhere on the PES landscape to read exact Hamiltonian coordinates.'}
            </div>
          )}
        </div>

        {/* Vault Pre-registration (P3): seal expectations before compute,
            evaluate pinned measurements against them. Ghost binds to the
            sealed μ; the badge renders the ledger-scored surprise. */}
        <div className="bg-[#0E1117]/90 backdrop-blur-xl border border-white/10 rounded-xl p-3 shadow-2xl">
          <div className="text-[10px] uppercase font-code tracking-wider text-slate-400 mb-1 flex items-center gap-1.5">
            <ShieldCheck className="w-3 h-3 text-emerald-400" />
            <span>Vault Pre-registration</span>
            {sealed && (
              <span className="text-cyan-300 normal-case">{sealed.prediction_hash.slice(0, 8)}…</span>
            )}
          </div>
          {sealed ? (
            <div className="font-code text-xs space-y-1">
              <div className="flex justify-between gap-4">
                <span className="text-slate-400">Sealed μ ± σ:</span>
                <span className="text-white font-medium">
                  {sealed.expected_mean.toFixed(5)} ± {sealed.expected_std.toFixed(5)} Ha
                </span>
              </div>
              {evaluation && (
                <div
                  title={`Brier ${evaluation.brier_score.toExponential(2)} · measured ${evaluation.measured_value.toFixed(5)} Ha`}
                  className={`rounded-lg px-2 py-1.5 text-[11px] font-medium border ${
                    evaluation.level === 'anomalous'
                      ? 'bg-rose-500/10 border-rose-400/40 text-rose-200 animate-pulse'
                      : evaluation.level === 'notable'
                        ? 'bg-amber-500/10 border-amber-400/30 text-amber-300'
                        : 'bg-emerald-500/10 border-emerald-400/30 text-emerald-300'
                  }`}
                >
                  {`z = ${evaluation.surprise_z.toFixed(3)} · ${evaluation.level ?? 'unscored'}`}
                </div>
              )}
            </div>
          ) : (
            <div className="font-code text-xs text-slate-500">
              {anchorComputed
                ? 'No sealed prediction — ghost is illustrative.'
                : 'No FCI anchor — sealing refused (nothing to anchor μ to).'}
            </div>
          )}
          <div className="flex items-center gap-2 mt-2">
            <input
              value={sealMeanStr}
              onChange={(e) => setSealMeanStr(e.target.value)}
              placeholder={`μ ${fciGround.toFixed(4)}`}
              title="Expected mean energy in Hartrees (defaults to the FCI anchor)"
              disabled={!anchorComputed || sealBusy}
              inputMode="decimal"
              className="w-24 bg-white/5 border border-white/10 rounded px-2 py-1 text-xs font-code text-white placeholder:text-slate-600 disabled:opacity-40"
            />
            <input
              value={sealStdStr}
              onChange={(e) => setSealStdStr(e.target.value)}
              placeholder="σ 0.005"
              title="Expected spread in Hartrees (floor 1e-4 enforced server-side)"
              disabled={!anchorComputed || sealBusy}
              inputMode="decimal"
              className="w-24 bg-white/5 border border-white/10 rounded px-2 py-1 text-xs font-code text-white placeholder:text-slate-600 disabled:opacity-40"
            />
            <button
              onClick={handleSeal}
              disabled={!anchorComputed || sealBusy}
              title={anchorComputed ? 'Seal expectation in the Vault ledger' : 'Requires a computed FCI anchor'}
              className="px-3 py-1 rounded-lg text-xs font-medium bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-semibold disabled:opacity-40 disabled:cursor-not-allowed transition-all"
            >
              {sealBusy ? '…' : 'Seal'}
            </button>
          </div>
          <button
            onClick={handleEvaluatePinned}
            disabled={!sealed || !selectedCoordinates || sealBusy}
            title={
              !sealed
                ? 'Seal an expectation first'
                : !selectedCoordinates
                  ? 'Pin a terrain point first (click) — the pin is the measurement'
                  : `Score pinned E=${selectedCoordinates.energy} Ha against sealed μ`
            }
            className="mt-2 w-full px-3 py-1.5 rounded-lg text-xs font-medium border border-cyan-400/30 text-cyan-300 hover:bg-cyan-500/10 disabled:opacity-40 disabled:cursor-not-allowed transition-all"
          >
            Evaluate pinned point
          </button>
          {assertion && (
            <div className="mt-1.5 rounded-lg px-2 py-1 text-[11px] font-code bg-violet-500/10 border border-violet-400/30 text-violet-200" title={`Evidence: ${(assertion.evidence_refs ?? []).join(', ') || '—'}`}>
              Asserted ρ={typeof assertion.rho === 'number' ? assertion.rho.toFixed(2) : '?'}
            </div>
          )}
          {sealed && evaluation && !assertion && (
            <div className="mt-2 rounded-lg border border-white/10 bg-white/[0.03] p-2">
              <div className="text-[10px] uppercase font-code tracking-wider text-slate-500 mb-1">Formal assertion (R-09)</div>
              <div className="flex items-center gap-2">
                <input
                  value={assertRho}
                  onChange={(e) => setAssertRho(e.target.value)}
                  placeholder="ρ 0.90"
                  title="Credibility you assert — judged by you, never computed"
                  disabled={sealBusy}
                  inputMode="decimal"
                  className="w-20 bg-white/5 border border-white/10 rounded px-2 py-1 text-xs font-code text-white placeholder:text-slate-600 disabled:opacity-40"
                />
                <input
                  value={assertEvidence}
                  onChange={(e) => setAssertEvidence(e.target.value)}
                  placeholder="evidence refs, comma-separated"
                  title="Evidence references this claim rests on (required)"
                  disabled={sealBusy}
                  className="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-2 py-1 text-xs font-code text-white placeholder:text-slate-600 disabled:opacity-40"
                />
                <button
                  onClick={handleAssert}
                  disabled={sealBusy}
                  title="Mint a prediction_asserted event binding hash + rho + evidence"
                  className="px-3 py-1 rounded-lg text-xs font-medium bg-violet-500/80 hover:bg-violet-400 text-white disabled:opacity-40 disabled:cursor-not-allowed transition-all"
                >
                  Assert
                </button>
              </div>
              {assertMsg && <div className="mt-1 text-[11px] font-code text-rose-300">{assertMsg}</div>}
            </div>
          )}
          {calibration && Object.keys(calibration).length > 0 && (
            <div className="mt-2 rounded-lg border border-white/10 bg-white/[0.03] p-2">
              <div className="text-[10px] uppercase font-code tracking-wider text-slate-500 mb-1">Author calibration (read-only)</div>
              {Object.entries(calibration).slice(0, 4).map(([author, c]) => (
                <div key={author} className="flex justify-between gap-2 text-[11px] font-code text-slate-300">
                  <span className="truncate">{author || 'unknown'}</span>
                  <span>
                    {c.evaluated}/{c.sealed} eval
                    {typeof c.mean_brier_score === 'number' ? ` · B ${c.mean_brier_score.toExponential(1)}` : ''}
                    {typeof c.mean_abs_z === 'number' ? ` · |z| ${c.mean_abs_z.toFixed(2)}` : ''}
                  </span>
                </div>
              ))}
            </div>
          )}
          {history.length > 0 && (
            <div className="mt-2 rounded-lg border border-white/10 bg-white/[0.03] p-2">
              <div className="text-[10px] uppercase font-code tracking-wider text-slate-500 mb-1">
                Trajectory replay {replayEntry ? `(step ${history.length - (replayIdx ?? 0)}/${history.length})` : '(live)'}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setReplayIdx((i) => (i == null ? history.length - 1 : Math.min(history.length - 1, i + 1)))}
                  disabled={replayIdx != null && replayIdx >= history.length - 1}
                  title="Step one event older (time travel is newest-first: older = forward in time)"
                  className="px-2 py-1 rounded-md text-[11px] font-code border border-white/10 text-slate-300 hover:bg-white/5 disabled:opacity-40"
                >
                  ‹ older
                </button>
                <button
                  onClick={() => setReplayIdx((i) => (i == null ? null : i - 1 < 0 ? null : i - 1))}
                  disabled={replayIdx == null}
                  title="Step one event newer"
                  className="px-2 py-1 rounded-md text-[11px] font-code border border-white/10 text-slate-300 hover:bg-white/5 disabled:opacity-40"
                >
                  newer ›
                </button>
                <button
                  onClick={() => setReplayIdx(null)}
                  disabled={replayIdx == null}
                  title="Return to the live ledger tip"
                  className="px-2 py-1 rounded-md text-[11px] font-code border border-cyan-400/30 text-cyan-300 hover:bg-cyan-500/10 disabled:opacity-40"
                >
                  live
                </button>
              </div>
              {replayEntry && (
                <div className="mt-1 text-[11px] font-code text-slate-300 truncate" title={replayEntry.label}>
                  ◷ {replayEntry.label}
                  {replayEntry.z != null ? ` · z=${replayEntry.z.toFixed(2)}` : ''}
                  {replayEntry.mean != null ? ` · μ=${replayEntry.mean.toFixed(4)}` : ''}
                  {replayEntry.level ? ` · ${replayEntry.level}` : ''}
                </div>
              )}
            </div>
          )}
          {gazes.length > 0 && (
            <div className="mt-1 text-[10px] font-code text-slate-500">
              {gazes.length} agent gaze{gazes.length === 1 ? '' : 's'} in view
            </div>
          )}
          {sealMsg && (
            <div className={`mt-1.5 text-[11px] font-code ${sealMsg.startsWith('Sealed') ? 'text-emerald-300' : 'text-rose-300'}`}>
              {sealMsg}
            </div>
          )}
        </div>
        </div>

        {/* Generative Lens (Atlas Marble 2) Action Panel */}
        <div className="pointer-events-auto flex flex-col items-end gap-2">
          {lensError && (
            <div className="bg-rose-950/80 border border-rose-500/40 text-rose-200 text-xs px-3 py-1.5 rounded-lg backdrop-blur-xl flex items-center gap-2 max-w-xs shadow-xl">
              <ShieldAlert className="w-4 h-4 text-rose-400 shrink-0" />
              <span>{lensError}</span>
            </div>
          )}

          <div className="bg-[#0E1117]/90 backdrop-blur-xl border border-white/10 rounded-xl p-3 shadow-2xl flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <div className="text-right">
              <div className="text-xs font-medium text-white flex items-center gap-1.5 justify-end">
                <Camera className="w-3.5 h-3.5 text-cyan-400" />
                <span>World Labs Atlas Lens</span>
              </div>
              <div className="text-[10px] font-code text-slate-400">
                {airGapped ? (
                  <span className="text-amber-400">Air-Gap Locked (Offline)</span>
                ) : isGeneratingLens ? (
                  <span>Dispatching — reserving against session cap…</span>
                ) : (
                  <span>On-demand generative lens · Marble 2</span>
                )}
              </div>
            </div>

            <button
              onClick={() => onRequestAtlasLens?.(captureSnapshotDataURL())}
              disabled={airGapped || isGeneratingLens}
              title={
                airGapped
                  ? 'Sovereign Air-Gap active: external cloud synthesis disabled'
                  : 'Pose the live terrain snapshot via Atlas Marble 2 (billed to the session cap on dispatch)'
              }
              className={`px-3.5 py-2 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 shadow-lg ${
                airGapped
                  ? 'bg-white/5 border border-white/10 text-slate-500 cursor-not-allowed'
                  : isGeneratingLens
                    ? 'bg-cyan-500/20 border border-cyan-400/30 text-cyan-300 animate-pulse'
                    : 'bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-semibold'
              }`}
            >
              {isGeneratingLens ? (
                <>
                  <Zap className="w-3.5 h-3.5 animate-spin" />
                  <span>Synthesizing...</span>
                </>
              ) : (
                <>
                  <Eye className="w-3.5 h-3.5" />
                  <span>Synthesize Lens</span>
                </>
              )}
            </button>
          </div>

          {/* P1: depth-conditioned chisel from the ledger-grounded terrain. */}
          <button
            onClick={() => {
              const cam = cameraRef.current;
              if (!cam) return;
              const pos = cam.position.toArray();
              const quat = cam.quaternion.toArray();
              void handleEncodeDepthChisel(
                [pos[0], pos[1], pos[2]],
                [quat[0], quat[1], quat[2], quat[3]],
              );
            }}
            disabled={airGapped || isGeneratingLens || chiselBusy || !anchorComputed}
            title={
              airGapped
                ? 'Sovereign Air-Gap active: external cloud synthesis disabled'
                : !anchorComputed
                  ? 'Requires a computed FCI anchor (depth needs ground truth)'
                  : 'Render terrain depth offscreen, upload as an Atlas asset, chisel the current view (billed to the session cap on dispatch)'
            }
            className="px-3.5 py-2 rounded-lg text-xs font-medium transition-all flex items-center justify-center gap-1.5 shadow-lg border border-cyan-400/30 text-cyan-300 hover:bg-cyan-500/10 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Layers className="w-3.5 h-3.5" />
            <span>{chiselBusy ? 'Encoding depth…' : 'Depth Chisel (P1)'}</span>
          </button>
          {chiselMsg && (
            <div className="text-[11px] font-code text-rose-300">{chiselMsg}</div>
          )}
          {/* P6: our reservation ledger — rated invoices live on the Atlas
              billing dashboard, never here. */}
          {lensHistory.length > 0 && (
            <details className="text-[11px] font-code">
              <summary className="text-slate-400 cursor-pointer hover:text-slate-200">
                Reservations ({lensHistory.length}) — not billed amounts
              </summary>
              <div className="mt-1 space-y-1 max-h-32 overflow-y-auto">
                {lensHistory.map((op) => (
                  <div key={op.operation_id} className="text-slate-500 truncate" title={op.prompt_used || op.operation_id}>
                    <span className="text-slate-300">{op.operation_id.slice(0, 8)}…</span>
                    {' · '}{op.task_type ?? 'lens'}
                    {op.cost_usd != null ? ` · $${op.cost_usd.toFixed(2)} reserved` : ''}
                    {op.resolved ? ' · resolved' : ' · pending'}
                  </div>
                ))}
              </div>
            </details>
          )}
          </div>
        </div>
      </div>

      {/* Atlas Synthetic Lens Non-Evidential Watermark & Preview */}
      {lensImageUrl && (
        <div className="absolute top-16 right-4 z-20 flex flex-col items-end gap-2">
          <div className="bg-black/80 border border-white/20 text-[10px] font-code text-slate-300 px-2.5 py-1 rounded backdrop-blur-md tracking-wider">
            [{lensWatermark}]
          </div>
          {lensPromptUsed && (
            <div className="bg-black/80 border border-white/20 text-[10px] font-code text-slate-400 px-2.5 py-1 rounded backdrop-blur-md max-w-56 truncate" title={lensPromptUsed}>
              promptUsed: {lensPromptUsed.slice(0, 80)}
            </div>
          )}
          {lensCostUsd != null && (
            <div className="bg-black/80 border border-white/20 text-[10px] font-code text-slate-300 px-2.5 py-1 rounded backdrop-blur-md tracking-wider">
              Reserved ${lensCostUsd.toFixed(2)} against session cap
            </div>
          )}
          {lensImageUrl.startsWith('http') && (
            <div className="w-56 h-32 rounded-lg overflow-hidden border border-cyan-500/30 shadow-2xl relative group bg-black/60 backdrop-blur-sm">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={lensImageProxyUrl(lensImageUrl) ?? lensImageUrl}
                alt="Atlas Marble 2 Observatory Synthetic Lens"
                className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
              />
              <div className="absolute bottom-1 right-1.5 text-[9px] font-mono text-cyan-300/80 bg-black/70 px-1 rounded">
                Atlas Lens
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
