'use client';

import React, { useEffect, useRef, useState, useMemo, useCallback } from 'react';
import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { 
  Sparkles, 
  RotateCcw, 
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
  evaluatePrediction,
  fetchLatestPrediction,
  sealPrediction,
  uploadObservatoryAsset,
  type PredictionEvaluation,
  type SealedPrediction,
} from '@/lib/api';
import { useAttention } from '@/lib/attention-context';
import {
  buildTerrainHeightField,
  encodeDepthEXR,
  renderTerrainDepthGL,
} from '@/lib/depth-encode';
import {
  MORSE_WIDTH_A,
  dissociationEnergyHartree,
  equilibriumBondDistanceAngstrom,
  morseEnergyHartree,
  morseVisualHeight,
} from '@/lib/morse';

export interface SpatialCanvas3DProps {
  campaign?: CampaignState | null;
  lensImageUrl?: string | null;
  lensWatermark?: string;
  lensPromptUsed?: string | null;
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
  lensHistory = [],
  onSelectPoint,
  onRequestAtlasLens,
  onRequestDepthChisel,
  isGeneratingLens = false,
  lensError = null,
  airGapped = false,
  spendLimitUSD = 150,
  currentSpendUSD = 0,
}: SpatialCanvas3DProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showGhostContour, setShowGhostContour] = useState(true);
  const [selectedCoordinates, setSelectedCoordinates] = useState<{ r: number; energy: number } | null>(null);
  const [hoveredCoordinates, setHoveredCoordinates] = useState<{ r: number; energy: number } | null>(null);

  // P3: ledger-bound pre-registration. `sealed` is the latest sealed
  // expectation for this session (null = ghost stays illustrative);
  // `evaluation` is its latest scoring, rendered as the z-badge.
  const [sealed, setSealed] = useState<SealedPrediction | null>(null);
  const [evaluation, setEvaluation] = useState<PredictionEvaluation | null>(null);
  const [sealMeanStr, setSealMeanStr] = useState('');
  const [sealStdStr, setSealStdStr] = useState('');
  const [sealBusy, setSealBusy] = useState(false);
  const [sealMsg, setSealMsg] = useState<string | null>(null);

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
  // visualHeight(sealedMean) when bound — the ledger's μ, not an offset.
  const sealedMean = sealed?.expected_mean ?? null;

  // P3: pull the ledger's latest sealed prediction + evaluation whenever
  // the session changes. Ghost and badge re-bind from this state.
  // Session-change resets happen during render (campaign-key guard below),
  // so the effect body never calls setState synchronously.
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
        }
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
    setSealMsg(null);
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
  const selectionPinRef = useRef<THREE.Mesh | null>(null);
  const diamondMeshRef = useRef<THREE.Mesh | null>(null);

  // Props ref to give event listeners access to latest props without re-creating scene
  const propsRef = useRef({
    equilibriumR,
    fciGround,
    rho,
    targetSystem,
    onSelectPoint,
    computeEnergy,
    computeVisualHeight,
  });
  useEffect(() => {
    propsRef.current = {
      equilibriumR,
      fciGround,
      rho,
      targetSystem,
      onSelectPoint,
      computeEnergy,
      computeVisualHeight,
    };
  });

  // Toggle Vault Ghost contour visibility without touching camera/controls
  useEffect(() => {
    if (ghostMeshRef.current) {
      ghostMeshRef.current.visible = showGhostContour;
    }
  }, [showGhostContour]);

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

    // Update ghost plane in-place. Bound (P3): flat plane at the sealed μ
    // height — the ledger's expectation, repositioned whenever it changes.
    // Unbound: the illustrative +0.12 offset over the live surface.
    if (ghostMeshRef.current) {
      const gGeo = ghostMeshRef.current.geometry;
      const gPos = gGeo.attributes.position;
      const boundHeight =
        sealedMean != null && Number.isFinite(sealedMean)
          ? computeVisualHeight(sealedMean)
          : null;
      for (let i = 0; i < gPos.count; i++) {
        if (boundHeight != null) {
          gPos.setY(i, boundHeight);
        } else {
          const gx = gPos.getX(i);
          const gz = gPos.getZ(i);
          const gDeltaR = gx * 0.4;
          gPos.setY(i, computeVisualHeight(computeEnergy(gDeltaR, gz)) + 0.12);
        }
      }
      gPos.needsUpdate = true;
      gGeo.computeVertexNormals();
    }
  }, [computeEnergy, computeVisualHeight, equilibriumR, fciGround, rho, sealedMean]);

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
    } catch {
      return;
    }

    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
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

          gl_FragColor = vec4(surfaceColor * (diff * 0.85 + 0.35), 0.94);
        }
      `,
      transparent: true,
      side: THREE.DoubleSide,
    });

    const terrainMesh = new THREE.Mesh(terrainGeo, vsupMaterial);
    scene.add(terrainMesh);
    terrainMeshRef.current = terrainMesh;

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

    // Pre-Registration Vault Ghost Plane
    const ghostGeo = new THREE.PlaneGeometry(worldWidth * 0.8, worldDepth * 0.8, 16, 16);    ghostGeo.rotateX(-Math.PI / 2);
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

    // Selection Indicator Beacon Pin
    const pinGeo = new THREE.CylinderGeometry(0.02, 0.02, 0.8, 16);
    const pinMat = new THREE.MeshBasicMaterial({ color: 0xffb300 });
    const selectionPin = new THREE.Mesh(pinGeo, pinMat);
    selectionPin.visible = false;
    scene.add(selectionPin);
    selectionPinRef.current = selectionPin;

    // Raycasting for interactive click & hover
    const raycaster = new THREE.Raycaster();
    const pointer = new THREE.Vector2();

    // Separate Drag/Orbit from Click
    let pointerDownPos = { x: 0, y: 0 };
    let isPointerDown = false;

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
        const deltaR = pt.x * 0.4;
        const curEq = propsRef.current.equilibriumR;
        const rVal = Number((curEq + deltaR).toFixed(4));
        const eVal = Number(propsRef.current.computeEnergy(deltaR, pt.z).toFixed(5));

        setSelectedCoordinates({ r: rVal, energy: eVal });
        selectionPin.position.set(pt.x, pt.y + 0.3, pt.z);
        selectionPin.visible = true;

        if (propsRef.current.onSelectPoint) {
          propsRef.current.onSelectPoint({
            bondDistance: rVal,
            energy: eVal,
            label: `${propsRef.current.targetSystem} PES (R=${rVal}Å)`,
          });
        }
      }
    };

    const handlePointerMove = (e: MouseEvent) => {
      // Don't raycast hover while orbiting/dragging
      if (isPointerDown) return;

      const rect = canvas.getBoundingClientRect();
      pointer.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

      raycaster.setFromCamera(pointer, camera);
      const intersects = raycaster.intersectObject(terrainMesh);
      if (intersects.length > 0) {
        const pt = intersects[0].point;
        const deltaR = pt.x * 0.4;
        const curEq = propsRef.current.equilibriumR;
        const rVal = curEq + deltaR;
        const eVal = propsRef.current.computeEnergy(deltaR, pt.z);
        setHoveredCoordinates({ r: rVal, energy: eVal });
      } else {
        setHoveredCoordinates(null);
      }
    };

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
    let animId: number;
    const startTime = performance.now();

    const animate = () => {
      animId = requestAnimationFrame(animate);
      const elapsed = (performance.now() - startTime) / 1000.0;

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
      resizeObserver.disconnect();
      canvas.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('pointerup', handlePointerUp);
      canvas.removeEventListener('mousemove', handlePointerMove);

      controls.dispose();
      terrainGeo.dispose();
      vsupMaterial.dispose();
      diamondGeo.dispose();
      diamondMat.dispose();
      anchorRingGeo.dispose();
      anchorRingMat.dispose();
      ghostGeo.dispose();
      ghostMat.dispose();
      pinGeo.dispose();
      pinMat.dispose();
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
          <span className="font-code text-[11px] text-slate-300">
            FCI Anchor: {fciGround.toFixed(4)} Ha
          </span>
          <span className="text-white/20">|</span>
          <div className="flex items-center gap-1.5 text-[11px] font-code text-amber-300">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
            <span>{`ρ = ${rho.toFixed(2)}`}</span>
          </div>
        </div>

        {/* Viewport Control Badges */}
        <div className="flex items-center gap-2 pointer-events-auto">
          {/* Ghost contour toggle.
              P3: bound to the ledger's latest sealed prediction (flat plane
              at sealed μ) when one exists; illustrative +0.12 offset wireframe
              otherwise. The Vault panel below seals and evaluates. */}
          <button
            onClick={() => setShowGhostContour((prev) => !prev)}
            title={
              sealed
                ? `Sealed expectation μ=${sealed.expected_mean} ± ${sealed.expected_std} Ha (${sealed.prediction_hash.slice(0, 8)}…)`
                : 'No sealed prediction yet — ghost is an illustrative placeholder; seal one in the Vault panel'
            }
            className={`px-3 py-1.5 rounded-full border text-[11px] font-code transition-all flex items-center gap-1.5 backdrop-blur-xl shadow-lg ${
              showGhostContour
                ? 'bg-cyan-500/15 border-cyan-400/40 text-cyan-300'
                : 'bg-white/5 border-white/10 text-slate-400 hover:text-white'
            }`}
          >
            <Layers className="w-3.5 h-3.5" />
            <span>{sealed ? 'Vault Ghost (μ bound)' : 'Vault Ghost (illustrative)'}</span>
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
              <div className="text-[10px] text-slate-500 pt-1 border-t border-white/10">
                Click terrain to pin coordinates into active campaign.
              </div>
            </div>
          ) : (
            <div className="font-code text-xs text-slate-400">
              Hover or click anywhere on the PES landscape to read exact Hamiltonian coordinates.
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
                ) : (
                  <span>Cost: 500 cr ($0.50) · Marble 2</span>
                )}
              </div>
            </div>

            <button
              onClick={() => onRequestAtlasLens?.(captureSnapshotDataURL())}
              disabled={airGapped || isGeneratingLens}
              title={
                airGapped
                  ? 'Sovereign Air-Gap active: external cloud synthesis disabled'
                  : 'Pose the live terrain snapshot via Atlas Marble 2'
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
                  : 'Render terrain depth offscreen, upload as an Atlas asset, chisel the current view'
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
          {lensImageUrl.startsWith('http') && (
            <div className="w-56 h-32 rounded-lg overflow-hidden border border-cyan-500/30 shadow-2xl relative group bg-black/60 backdrop-blur-sm">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={lensImageUrl}
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
