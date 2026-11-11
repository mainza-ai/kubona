'use client';

import React, { useState, useMemo, useEffect, useCallback } from 'react';
import dynamic from 'next/dynamic';
import { Atom, Code, Copy, Check, Orbit } from 'lucide-react';
import { CampaignState } from '@/types';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import { fetchCanvasLayout, saveCanvasLayout, fetchMolecularADMET, requestObservatoryLens, fetchObservatoryLensStatus, resolveObservatoryLens, fetchLensHistory, type LensHistoryOp } from '@/lib/api';
import { dissociationEnergyHartree, equilibriumBondDistanceAngstrom, morseEnergyHartree } from '@/lib/morse';

// Wave-3 code-split: three.js + EXR must not load on first paint for users
// who never open 3D. The observatory (SpatialCanvas3D, which statically
// pulls three + OrbitControls + the depth-encode chisel path) loads only
// when canvasMode flips to 'observatory'. ssr:false because WebGL has no
// server render; the loading fallback is an honest skeleton, never blank.
// The illustrative/schematic gating below (canvasMode === 'observatory'
// branch) is unchanged — only the import mechanism moved.
const SpatialCanvas3D = dynamic(
  () => import('./SpatialCanvas3D').then((m) => m.SpatialCanvas3D),
  {
    ssr: false,
    loading: () => (
      <div
        role="status"
        aria-label="Loading 3D observatory"
        className="flex-1 flex items-center justify-center min-h-[520px] rounded-2xl border border-[var(--color-border)] bg-[var(--color-canvas)] animate-pulse"
      >
        <span className="text-xs font-mono text-[var(--color-muted)]">Loading 3D observatory…</span>
      </div>
    ),
  },
);

interface SpatialCanvasProps {
  campaign?: CampaignState | null;
}

export function SpatialCanvas({ campaign }: SpatialCanvasProps) {
  const [canvasMode, setCanvasMode] = useState<'workbench' | 'observatory'>('workbench');
  const [layoutStatus, setLayoutStatus] = useState<string | null>(null);
  const [canvasCamera, setCanvasCamera] = useState<{
    position: [number, number, number];
    target: [number, number, number];
  } | null>(null);
  // Persisted canvas layout is session-scoped: display mode plus the
  // operator-arranged 3D camera (position + orbit target).
  useEffect(() => {
    const sid = campaign?.session_id;
    if (!sid) return;
    let live = true;
    fetchCanvasLayout(sid).then((layout) => {
      if (!live || !layout) return;
      if (layout.canvas_mode === 'observatory' || layout.canvas_mode === 'workbench') {
        setCanvasMode(layout.canvas_mode);
      }
      const cam = layout.canvas_camera as { position?: unknown; target?: unknown } | undefined;
      const isVec3 = (v: unknown): v is [number, number, number] =>
        Array.isArray(v) && v.length === 3 && v.every((n) => typeof n === 'number' && Number.isFinite(n));
      if (cam && isVec3(cam.position) && isVec3(cam.target)) {
        setCanvasCamera({ position: cam.position, target: cam.target });
      }
      setLayoutStatus('layout restored');
    });
    return () => { live = false; };
  }, [campaign?.session_id]);

  // Discovery target
  const targetSystem = campaign?.target_system || 'No target';
  
  const systemDisplayName = useMemo(() => {
    const raw = campaign?.quantum_factors?.target_molecule || campaign?.target_system || 'No target';
    const upper = raw.toUpperCase();
    if (upper === 'H2' || upper === 'H₂' || upper.includes('HYDROGEN')) return 'Hydrogen Molecule (H₂)';
    if (upper.includes('LIH') || upper.includes('LITHIUM')) return 'Lithium Hydride (LiH)';
    if (upper.includes('BEH2') || upper.includes('BERYLLIUM')) return 'Beryllium Hydride (BeH₂)';
    if (upper.includes('H2O') || upper.includes('WATER')) return 'Water Molecule (H₂O)';
    if (upper.includes('N2') || upper.includes('NITROGEN')) return 'Dinitrogen (N₂)';
    if (upper.includes('CO2')) return 'Carbon Dioxide (CO₂)';
    if (upper.includes('CO')) return 'Carbon Monoxide (CO)';
    return raw;
  }, [campaign?.target_system, campaign?.quantum_factors?.target_molecule]);

  const [atom1, atom2] = useMemo(() => {
    const mol = (campaign?.quantum_factors?.molecular_formula || campaign?.quantum_factors?.target_molecule || '').toUpperCase();
    if (mol.includes('LIH') || mol === 'LIH') return ['Li', 'H'];
    if (mol.includes('BEH') || mol === 'BEH2') return ['Be', 'H'];
    if (mol.includes('H2O')) return ['H', 'O'];
    if (mol.includes('N2')) return ['N₁', 'N₂'];
    if (mol.includes('CO2') || mol === 'CO') return ['C', 'O'];
    if (mol.includes('HF')) return ['H', 'F'];
    if (mol.includes('NH3')) return ['N', 'H'];
    if (mol.includes('CH4')) return ['C', 'H'];

    // Whitelist valid chemical symbols strictly from formula (never arbitrary English words)
    const validElements = ['H', 'He', 'Li', 'Be', 'B', 'C', 'N', 'O', 'F', 'Ne', 'Na', 'Mg', 'Al', 'Si', 'P', 'S', 'Cl', 'Br', 'I'];
    const formula = campaign?.quantum_factors?.molecular_formula || campaign?.quantum_factors?.target_molecule || '';
    const matches = (formula.match(/([A-Z][a-z]?)/g) || []).filter((el) => validElements.includes(el));
    if (matches.length >= 2 && matches[0] !== matches[1]) {
      return [matches[0], matches[1]];
    }
    if (matches.length >= 1) {
      return [`${matches[0]}₁`, `${matches[0]}₂`];
    }
    return ['?', '?'];
  }, [campaign?.quantum_factors?.molecular_formula, campaign?.quantum_factors?.target_molecule]);

  // Equilibrium bond distance (single source of truth: @/lib/morse —
  // shared with the 3D observatory so both surfaces agree).
  const equilibriumR = useMemo(() => {
    if (campaign?.quantum_factors?.bond_distance) return campaign.quantum_factors.bond_distance;
    return equilibriumBondDistanceAngstrom(campaign?.target_system);
  }, [campaign?.quantum_factors?.bond_distance, campaign?.target_system]);

  const campaignBondDist = campaign?.quantum_factors?.bond_distance;
  // H9: equilibriumR falls back to the Morse table when the campaign carries
  // no bond distance — the reset label says so.
  const equilibriumFromTable = campaignBondDist == null;
  const [userBondDistance, setUserBondDistance] = useState<number | null>(null);
  const [prevCampaignKey, setPrevCampaignKey] = useState<string>('');

  const currentCampaignKey = `${campaign?.session_id || ''}-${campaign?.target_system || ''}`;
  if (currentCampaignKey !== prevCampaignKey) {
    setPrevCampaignKey(currentCampaignKey);
    setUserBondDistance(null);
  }

  const bondDistance = userBondDistance ?? (campaignBondDist || equilibriumR);
  const setBondDistance = (val: number) => setUserBondDistance(val);

  const [disclosureDepth, setDisclosureDepth] = useState<1 | 2 | 3>(1);
  const [circuitViewMode, setCircuitViewMode] = useState<'schematic' | 'qasm'>('schematic');
  const [copiedQasm, setCopiedQasm] = useState(false);

  // H8: hardcoded map is a fallback only — the display tags it (inferred)
  // whenever the campaign carries no SMILES of its own.
  const smilesInferred = !campaign?.smiles;
  const targetSmiles = useMemo(() => {
    if (campaign?.smiles) return campaign.smiles;
    const sys = (campaign?.quantum_factors?.target_molecule || campaign?.target_system || 'No target').toUpperCase();
    if (sys.includes('LIH')) return '[Li+].[H-]';
    if (sys.includes('H2O')) return 'O';
    if (sys.includes('CO2')) return 'O=C=O';
    if (sys.includes('CO')) return '[C-]#[O+]';
    if (sys.includes('N2')) return 'N#N';
    return '[H][H]';
  }, [campaign?.smiles, campaign?.quantum_factors?.target_molecule, campaign?.target_system]);

  const [admetProfile, setAdmetProfile] = useState<Record<string, unknown> | null>(null);
  const [admetLoading, setAdmetLoading] = useState(false);
  const [admetSmiles, setAdmetSmiles] = useState<string | null>(null);

  // Compiler-honest scalars: the React Compiler reads `campaign` as a whole
  // object through optional chaining, so the lens callback depends on these
  // scalars instead of `campaign?.x` expressions (P0c lint fix).
  const lensSessionId = campaign?.session_id;
  const lensTargetSystem = campaign?.target_system || 'No target';

  const [isGeneratingLens, setIsGeneratingLens] = useState(false);
  const [lensError, setLensError] = useState<string | null>(null);
  const [lensImageUrl, setLensImageUrl] = useState<string | null>(null);
  const [lensPromptUsed, setLensPromptUsed] = useState<string | null>(null);
  // Actual reserved amount from the dispatch response — the only cost
  // figure the UI may show. No hardcoded prices in idle chrome (F-14).
  const [lensCostUsd, setLensCostUsd] = useState<number | null>(null);
  const [lensHistory, setLensHistory] = useState<LensHistoryOp[]>([]);
  const [lensOpId, setLensOpId] = useState<string | null>(null);

  const handleRequestAtlasLens = useCallback(async (snapshotDataUrl: string | null) => {
    const sid = lensSessionId;
    if (!sid) {
      setLensError('No active campaign session to debit spend');
      return;
    }
    setIsGeneratingLens(true);
    setLensError(null);
    setLensCostUsd(null);
    try {
      const resp = await requestObservatoryLens({
        sessionId: sid,
        targetSystem: lensTargetSystem,
        // Q-37: live terrain snapshot when capturable; otherwise undefined
        // and the backend fail-closes (no stock imagery, ever).
        imageUrl: snapshotDataUrl ?? undefined,
      });
      if (!resp.ok) {
        setLensError(resp.error || 'Failed to synthesize lens');
        setIsGeneratingLens(false);
      } else if (resp.operation_id) {
        setLensImageUrl(null);
        setLensPromptUsed(null);
        setLensCostUsd(resp.cost_usd ?? null);
        setLensOpId(resp.operation_id);
      } else {
        setIsGeneratingLens(false);
      }
    } catch (e) {
      setLensError((e as Error).message);
      setIsGeneratingLens(false);
    }
  }, [lensSessionId, lensTargetSystem]);

  // P1: depth-conditioned chisel from an uploaded encoder depth asset.
  // Shares the lens rail (spend, poll, resolve) with the snapshot path.
  const handleRequestDepthChisel = useCallback(async (
    depthAssetId: string,
    camera: { position: number[]; orientation: number[] },
  ) => {
    const sid = lensSessionId;
    if (!sid) {
      setLensError('No active campaign session to debit spend');
      return;
    }
    setIsGeneratingLens(true);
    setLensError(null);
    setLensCostUsd(null);
    try {
      const resp = await requestObservatoryLens({
        sessionId: sid,
        targetSystem: lensTargetSystem,
        taskType: 'atlasChisel',
        depthAssetId,
        camera,
      });
      if (!resp.ok) {
        setLensError(resp.error || 'Failed to dispatch chisel');
        setIsGeneratingLens(false);
      } else if (resp.operation_id) {
        setLensImageUrl(null);
        setLensPromptUsed(null);
        setLensCostUsd(resp.cost_usd ?? null);
        setLensOpId(resp.operation_id);
      } else {
        setIsGeneratingLens(false);
      }
    } catch (e) {
      setLensError((e as Error).message);
      setIsGeneratingLens(false);
    }
  }, [lensSessionId, lensTargetSystem]);

  // P6: per-session reservation ledger (our records, not rated invoices).
  // Session-change resets happen during render (campaign-key guard); the
  // effect body only fetches. Key is normalized to '' so undefined sessions
  // don't retrigger every render.
  const [prevLensSession, setPrevLensSession] = useState<string | null>(null);
  const lensSessionKey = lensSessionId ?? '';
  if (lensSessionKey !== (prevLensSession ?? '')) {
    setPrevLensSession(lensSessionKey);
    setLensHistory([]);
    setLensPromptUsed(null);
    setLensCostUsd(null);
  }
  const refreshLensHistory = useCallback(async (sid: string) => {
    try {
      const h = await fetchLensHistory(sid);
      if (h.ok) setLensHistory(h.operations ?? []);
    } catch {
      /* history is advisory */
    }
  }, []);

  useEffect(() => {
    if (!lensSessionId) return;
    let cancelled = false;
    // setState lives in the promise callback (same as the poll effect),
    // never synchronously in the effect body.
    fetchLensHistory(lensSessionId)
      .then((h) => {
        if (!cancelled && h.ok) setLensHistory(h.operations ?? []);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [lensSessionId]);

  useEffect(() => {
    if (!lensOpId || lensImageUrl) return;
    let cancelled = false;
    // Bounded poll: 40 attempts x 3 s (2 min ceiling) with a terminal
    // timeout error — never an open-ended interval. P1 adds backoff +
    // Retry-After honoring for 429s.
    let attempts = 0;
    const maxAttempts = 40;
    const interval = setInterval(async () => {
      if (typeof document !== 'undefined' && document.hidden) return;
      try {
        attempts += 1;
        if (attempts > maxAttempts) {
          if (!cancelled) {
            setLensError('Timed out waiting for the Atlas operation (2 min): keep the operation ID and check the World Labs console, or retry with the same request');
            setLensOpId(null);
            setIsGeneratingLens(false);
          }
          clearInterval(interval);
          return;
        }
        const data = await fetchObservatoryLensStatus(lensOpId);
        if (cancelled) return;
        if (data.error) {
          const errMsg = typeof data.error === 'string'
            ? data.error
            : (data.error as { message?: string })?.message || 'Atlas operation failed';
          setLensError(errMsg);
          setLensOpId(null);
          setIsGeneratingLens(false);
          clearInterval(interval);
          return;
        }
        if (data.done) {
          const resp = data.response as { frames?: Array<{ imageAsset?: { url?: string } }> } | undefined;
          const url = resp?.frames?.[0]?.imageAsset?.url;
          if (url) {
            setLensImageUrl(url);
          } else {
            setLensError('Atlas operation completed but returned no image frame');
          }
          setLensOpId(null);
          setIsGeneratingLens(false);
          clearInterval(interval);
          // P1: persist + surface the model-consumed prompt via resolve.
          const sid = lensSessionId;
          const opId = lensOpId;
          if (sid && opId) {
            resolveObservatoryLens({ sessionId: sid, operationId: opId })
              .then((r) => {
                if (!cancelled && r.ok && r.prompt_used) setLensPromptUsed(r.prompt_used);
                void refreshLensHistory(sid);
              })
              .catch(() => {
                void refreshLensHistory(sid);
              });
          }
        }
      } catch {
        // keep polling
      }
    }, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [lensOpId, lensImageUrl, lensSessionId, refreshLensHistory]);

  // Adjust state during render when the target changes (same pattern as
  // the campaign-key guard above): reset profile + loading synchronously,
  // then the effect below fetches exactly once per target. This keeps
  // setState out of the effect body (react-hooks/set-state-in-effect).
  if (targetSmiles !== admetSmiles) {
    setAdmetSmiles(targetSmiles);
    setAdmetProfile(null);
    setAdmetLoading(!!targetSmiles);
  }

  useEffect(() => {
    let cancelled = false;
    if (!admetSmiles) return;
    fetchMolecularADMET(admetSmiles)
      .then((data) => {
        if (!cancelled && data && !data.error) {
          setAdmetProfile(data);
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setAdmetLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [admetSmiles]);

  // Compute energy on the fly from the shared physical Morse potential
  // (@/lib/morse) anchored to PySCF FCI. Same numbers as the 3D
  // observatory by construction (P0c: retired the ad-hoc |FCI|*0.15,
  // a=1.44 workbench-only parameterization).
  const energyCalculated = useMemo(() => {
    const Re = equilibriumR;
    const anchor = campaign?.classical_anchor;
    const anchorComputed = anchor?.anchor_computed === true;
    const fciRaw = anchor?.fci_exact_energy;
    const fciFinite = typeof fciRaw === 'number' && Number.isFinite(fciRaw);
    // No anchor → no Δ, no invented baseline. Model energy renders as
    // Morse extrapolation only when an anchor exists; otherwise UNVERIFIED.
    const fciGround = anchorComputed && fciFinite ? (fciRaw as number) : null;
    const De = dissociationEnergyHartree(campaign?.target_system);
    const deltaR = bondDistance - Re;
    const energy = fciGround != null ? morseEnergyHartree(deltaR, 0, De, fciGround) : null;
    const rhfRaw = anchor?.rhf_mean_field_energy;
    const rhfFinite = typeof rhfRaw === 'number' && Number.isFinite(rhfRaw);
    const rhfEnergy = anchorComputed && rhfFinite ? (rhfRaw as number) : null;
    const deltaMHa = fciGround != null && energy != null ? Math.abs(energy - fciGround) * 1000.0 : null;
    const nearEquilibrium = Math.abs(deltaR) < 0.01;
    return {
      anchorFci: fciGround,
      anchorComputed,
      modelEnergy: energy,
      rhf: rhfEnergy,
      deltaMHa,
      // Verified chrome only when the anchor was actually computed AND the
      // slider sits at equilibrium (where the model equals the anchor).
      // Anywhere else this is Morse extrapolation, labeled as such.
      anchored: anchorComputed && fciGround != null && nearEquilibrium,
      displaced: !nearEquilibrium,
    };
  }, [bondDistance, equilibriumR, campaign?.classical_anchor, campaign?.target_system]);

  // Atom positions for SVG physical rendering
  const atomOffset = useMemo(() => {
    // Map bond distance in Angstroms to pixel distance
    return Math.round(bondDistance * 110);
  }, [bondDistance]);

  // Dynamic Circuit parser from session QuantumFactors / OpenQASM
  const circuitData = useMemo(() => {
    const qf = campaign?.quantum_factors;
    const numQubits = (typeof qf?.qubit_count === 'number' && Number.isFinite(qf.qubit_count) ? qf.qubit_count : 0);
    const rawQasm = (qf?.qasm || '').trim();
    // No-circuit refusal state: without QASM there is no circuit to parse.
    // No schematic fallback is generated.
    const hasCircuit = rawQasm !== '';
    const qasmText = rawQasm;

    interface GateOp {
      id: string;
      type: string;
      label: string;
      qubit: number;
      col: number;
      targetQubit?: number;
      color: string;
    }
    const gates: GateOp[] = [];
    interface Conn {
      col: number;
      fromQubit: number;
      toQubit: number;
    }
    const connections: Conn[] = [];
    const colPointers = new Array(numQubits).fill(0);

    const lines = qasmText.split('\n').map((l) => l.trim().replace(/;$/, ''));
    for (const line of lines) {
      if (!line || line.startsWith('//') || line.startsWith('OPENQASM') || line.startsWith('include') || line.startsWith('qreg') || line.startsWith('creg')) {
        continue;
      }
      const cxMatch = line.match(/^cx\s+q\[(\d+)\],\s*q\[(\d+)\]/i);
      if (cxMatch) {
        const c = parseInt(cxMatch[1], 10);
        const t = parseInt(cxMatch[2], 10);
        if (c < numQubits && t < numQubits) {
          const col = Math.max(colPointers[c], colPointers[t]);
          gates.push({ id: `cx-c-${c}-${col}`, type: 'CX_CTRL', label: '•', qubit: c, col, targetQubit: t, color: 'text-[var(--color-accent-ink)]' });
          gates.push({ id: `cx-t-${t}-${col}`, type: 'CX_TGT', label: '⊕', qubit: t, col, targetQubit: c, color: 'text-[var(--color-accent-ink)]' });
          connections.push({ col, fromQubit: Math.min(c, t), toQubit: Math.max(c, t) });
          colPointers[c] = col + 1;
          colPointers[t] = col + 1;
        }
        continue;
      }
      const hMatch = line.match(/^h\s+q\[(\d+)\]/i);
      if (hMatch) {
        const q = parseInt(hMatch[1], 10);
        if (q < numQubits) {
          const col = colPointers[q];
          gates.push({ id: `h-${q}-${col}`, type: 'H', label: 'H', qubit: q, col, color: 'bg-[var(--color-soft-sky-wash)] border-[var(--color-soft-sky-edge)] text-[var(--color-soft-sky-ink)]' });
          colPointers[q] = col + 1;
        }
        continue;
      }
      const rotMatch = line.match(/^(rz|ry|rx)\(([^)]+)\)\s+q\[(\d+)\]/i);
      if (rotMatch) {
        const name = rotMatch[1].toUpperCase();
        const angle = parseFloat(rotMatch[2]);
        const angleDisplay = isNaN(angle) ? rotMatch[2] : angle.toFixed(2);
        const q = parseInt(rotMatch[3], 10);
        if (q < numQubits) {
          const col = colPointers[q];
          const color = name === 'RY' 
            ? 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] text-[var(--color-info-ink)]'
            : name === 'RZ'
            ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
            : 'bg-[var(--color-soft-lilac-wash)] border-[var(--color-soft-lilac-edge)] text-[var(--color-soft-lilac-ink)]';
          gates.push({
            id: `${name}-${q}-${col}`,
            type: name,
            label: `${name}(${angleDisplay})`,
            qubit: q,
            col,
            color,
          });
          colPointers[q] = col + 1;
        }
        continue;
      }
      const mMatch = line.match(/^measure\s+q\[(\d+)\]/i);
      if (mMatch) {
        const q = parseInt(mMatch[1], 10);
        if (q < numQubits) {
          const col = colPointers[q];
          gates.push({ id: `m-${q}-${col}`, type: 'MEASURE', label: '⏱ M', qubit: q, col, color: 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]' });
          colPointers[q] = col + 1;
        }
        continue;
      }
    }

    const totalCols = Math.max(1, ...colPointers);
    const depthRaw = qf?.circuit_depth;
    const twoQRaw = qf?.two_qubit_gates;
    return {
      numQubits,
      totalCols,
      gates,
      connections,
      rawQasm: qasmText,
      hasCircuit,
      ansatzTitle: qf?.ansatz_family || 'Ansatz not recorded',
      depth: typeof depthRaw === 'number' && Number.isFinite(depthRaw) ? depthRaw : 0,
      depthLive: typeof depthRaw === 'number' && Number.isFinite(depthRaw),
      twoQubitGates: typeof twoQRaw === 'number' && Number.isFinite(twoQRaw) ? twoQRaw : 0,
      twoQLive: typeof twoQRaw === 'number' && Number.isFinite(twoQRaw),
    };
  }, [campaign?.quantum_factors]);

  // The campaign records no Hamiltonian coefficients. Keep the section's
  // empty state rather than printing a schematic Pauli expansion.
  const hamiltonianString = '// Hamiltonian coefficients not recorded for this campaign.';

  if (!campaign) {
    return (
      <div className="relative min-h-[560px] sm:min-h-[680px] rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border-strong)] shadow-[var(--shadow-float)] flex items-center justify-center p-8 text-center">
        <div className="space-y-2">
          <h2 className="text-base font-semibold text-[var(--color-text)]">Molecular Workbench</h2>
          <p className="text-xs text-[var(--color-muted)]">No campaign proposal is selected. Create or open a campaign to give the workbench a target.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="relative min-h-[560px] sm:min-h-[680px] rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border-strong)] shadow-[var(--shadow-float)] flex flex-col justify-between p-4 sm:p-8 select-none">
      {/* Ambient Radial Discovery Lighting */}
      <div 
        className="absolute inset-0 pointer-events-none opacity-40 transition-opacity duration-1000"
        style={{
          background: `radial-gradient(circle 450px at 50% 45%, ${energyCalculated.anchored ? 'rgba(16, 185, 129, 0.15)' : 'rgba(245, 158, 11, 0.15)'}, transparent 75%)`,
        }}
      />

      {/* Top Floating Instrument Bar: Goal & Truth Indicator */}
      <div className="relative z-10 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] flex items-center justify-center shadow-inner">
            <Atom className="w-5 h-5 text-[var(--color-accent-ink)] animate-float-gentle" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <h2 className="text-base font-semibold text-[var(--color-text)] tracking-tight">
                {systemDisplayName}
              </h2>
              <span className="text-[12px] font-mono font-medium px-2 py-0.5 rounded-full bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)]">
                Ground State PES
              </span>
              <ProvenanceBadge kind="model" detail="Morse potential visualization; measured anchors are labeled separately." />
            </div>
            <p className="text-xs text-[var(--color-label-secondary)] mt-0.5">
              Direct physical manipulation of nuclear coordinates and electronic structure
            </p>
            <p className="text-[12px] font-code text-[var(--color-faint)] mt-0.5">
              2-atom schematic only — larger systems still render two bubbles
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 sm:gap-3">
          {/* Mode Switcher: 2D Workbench vs 3D Observatory */}
          <div role="group" aria-label="Canvas mode" className="flex items-center p-1 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] shadow-inner">
            <button
              onClick={() => setCanvasMode('workbench')}
              aria-pressed={canvasMode === 'workbench'}
              className={`px-3 py-1 rounded-xl text-xs font-medium transition-all ${
                canvasMode === 'workbench'
                  ? 'bg-[var(--color-fill-hover)] text-[var(--color-text)] font-semibold shadow-sm'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              2D Workbench
            </button>
            <button
              onClick={() => setCanvasMode('observatory')}
              aria-pressed={canvasMode === 'observatory'}
              className={`px-3 py-1 rounded-xl text-xs font-medium transition-all flex items-center gap-1.5 ${
                canvasMode === 'observatory'
                  ? 'bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)] font-semibold shadow-sm'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              <Orbit className="w-3.5 h-3.5 text-[var(--color-accent-ink)]" />
              <span>3D Observatory</span>
            </button>
          </div>

          <button
            type="button"
            onClick={() => {
              const sid = campaign?.session_id;
              if (!sid) { setLayoutStatus('no session — layout not saved'); return; }
              void saveCanvasLayout(sid, {
                canvas_mode: canvasMode,
                ...(canvasCamera ? { canvas_camera: canvasCamera } : {}),
              }).then((out) => {
                setLayoutStatus(out.ok ? 'layout saved' : `layout save refused (${out.error ?? 'unknown'})`);
              });
            }}
            className="px-3 py-1.5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] text-xs text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]"
          >
            Save layout
          </button>
          {layoutStatus && <span className="text-[12px] font-code text-[var(--color-muted)]">{layoutStatus}</span>}

          {/* Calm Truth Status Indicator */}
          <div className="flex max-w-full flex-wrap items-center gap-x-2.5 gap-y-1 px-3 py-1.5 rounded-2xl bg-[var(--color-float)] border border-[var(--color-border-strong)]">
            <div className={`w-2 h-2 rounded-full ${energyCalculated.anchored ? 'bg-[var(--color-success)] shadow-[0_0_12px_rgba(52,211,153,0.8)]' : 'bg-[var(--color-warning)] shadow-[0_0_12px_rgba(251,191,36,0.8)]'}`} />
            <ProvenanceBadge kind={energyCalculated.anchored ? 'measured' : 'model'} detail={energyCalculated.anchored ? 'At the computed FCI anchor.' : 'Morse extrapolation from a model.'} />
            <span className="text-xs font-medium text-[var(--color-text)]">
              {energyCalculated.anchored
                ? 'Anchored at computed FCI'
                : energyCalculated.displaced
                  ? 'Morse extrapolation — not measured at this R'
                  : 'Unverified anchor (bridge unavailable)'}
            </span>
            <span className="text-xs text-[var(--color-muted)]">•</span>
            <span className="text-xs font-mono font-semibold text-[var(--color-text)]">
              {energyCalculated.deltaMHa != null && Number.isFinite(energyCalculated.deltaMHa) ? `Δ = ${energyCalculated.deltaMHa.toFixed(2)} mHa` : 'Δ = UNVERIFIED (no anchor)'}
            </span>
          </div>
        </div>
      </div>

      {/* 3D Observatory Representation Mode */}
      {canvasMode === 'observatory' && (
        <div className="relative z-10 flex-1 my-4">
          <SpatialCanvas3D
            campaign={campaign}
            initialCamera={canvasCamera}
            onCameraChange={setCanvasCamera}
            onSelectPoint={(pt) => setBondDistance(pt.bondDistance)}
            onRequestAtlasLens={handleRequestAtlasLens}
            onRequestDepthChisel={handleRequestDepthChisel}
            isGeneratingLens={isGeneratingLens}
            lensError={lensError}
            lensImageUrl={lensImageUrl}
            lensPromptUsed={lensPromptUsed}
            lensCostUsd={lensCostUsd}
            lensHistory={lensHistory}
          />
        </div>
      )}

      {/* Central Living Discovery Stage (Depth 1: The Molecular Target) */}
      {canvasMode === 'workbench' && disclosureDepth === 1 && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-6">
          {/* Tactile Molecular Visualization */}
          <div className="relative w-full max-w-md h-56 flex items-center justify-center">
            {/* Electron Density Wave Cloud */}
            <div 
              className="absolute w-64 h-32 rounded-full opacity-30 blur-2xl transition-all duration-300"
              style={{
                background: energyCalculated.anchored
                  ? 'radial-gradient(ellipse at center, rgba(16, 185, 129, 0.4), transparent)'
                  : 'radial-gradient(ellipse at center, rgba(245, 158, 11, 0.4), transparent)',
                width: `${atomOffset + 120}px`,
              }}
            />

            {/* Intersite Covalent Bond Wire with Length Indicator */}
            <div 
              className="absolute h-0.5 bg-gradient-to-r from-[var(--color-accent)] via-[var(--color-success)] to-[var(--color-accent)] opacity-60 transition-all duration-200"
              style={{ width: `${atomOffset}px` }}
            />

            {/* Left Nucleus */}
            <div 
              className="absolute flex flex-col items-center transform -translate-x-1/2 transition-all duration-200"
              style={{ left: `calc(50% - ${atomOffset / 2}px)` }}
            >
              <div className="w-14 h-14 rounded-full bg-gradient-to-br from-cyan-500 to-blue-600 shadow-xl flex items-center justify-center text-[var(--color-on-accent)] font-bold text-base border-2 border-[var(--color-border-strong)]">
                {atom1}
              </div>
              <span className="text-[12px] font-mono text-[var(--color-info-ink)] mt-2 font-medium">Nuclear Core</span>
            </div>

            {/* Right Nucleus */}
            <div 
              className="absolute flex flex-col items-center transform translate-x-1/2 transition-all duration-200"
              style={{ left: `calc(50% + ${atomOffset / 2}px)` }}
            >
              <div className="w-14 h-14 rounded-full bg-gradient-to-br from-emerald-500 to-teal-600 shadow-xl flex items-center justify-center text-[var(--color-on-accent)] font-bold text-base border-2 border-[var(--color-border-strong)]">
                {atom2}
              </div>
              <span className="text-[12px] font-mono text-[var(--color-success-ink)] mt-2 font-medium">Nuclear Core</span>
            </div>

            {/* Bond Length Badge (Floating on bond center) */}
            <div className="absolute -top-4 px-3 py-1 rounded-full bg-[var(--color-float)] border border-[var(--color-border-strong)] text-[12px] font-mono font-bold text-[var(--color-info-ink)] shadow-md">
              R = {typeof bondDistance === 'number' && Number.isFinite(bondDistance) ? bondDistance.toFixed(4) : 'UNVERIFIED'} Å
            </div>
          </div>

          {/* Direct Manipulation Tactile Bond Slider */}
          <div className="w-full max-w-md mt-6 space-y-3">
            <div className="flex justify-between items-center text-xs text-[var(--color-label-secondary)] font-medium">
              <span>Bond Length Control</span>
              <button 
                onClick={() => setBondDistance(equilibriumR)}
                className="text-xs text-[var(--color-accent-ink)] hover:underline transition-colors font-semibold"
              >
                Reset to Equilibrium ({equilibriumR} Å{equilibriumFromTable ? ' (table)' : ''})
              </button>
            </div>
            <input 
              type="range"
              min={Math.max(0.3, parseFloat((equilibriumR * 0.45).toFixed(3)))}
              max={parseFloat((equilibriumR * 2.2).toFixed(3))}
              step="0.005"
              value={bondDistance}
              onChange={(e) => setBondDistance(parseFloat(e.target.value))}
              className="w-full h-2 bg-[var(--color-fill)] rounded-lg appearance-none cursor-pointer accent-[var(--color-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-edge)]"
            />
            <div className="flex justify-between text-xs font-mono text-[var(--color-label-secondary)] font-medium">
              <span>Compressed (Repulsive)</span>
              <span className="text-[var(--color-success-ink)] font-bold">Equilibrium Valley</span>
              <span>Dissociated</span>
            </div>
          </div>

          {/* Molecular Pharmacology & ADMET Profile (RDKit Bridge Anchored) */}
          <div className="w-full max-w-md mt-5 p-4 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono text-[12px] flex items-center space-x-2">
                <Atom className="w-4 h-4 text-[var(--color-success-ink)]" />
                <span>Molecular Pharmacology &amp; ADMET</span>
                <ProvenanceBadge kind="measured" detail="RDKit bridge profile." />
              </span>
              <span className="text-[var(--color-muted)] font-mono text-[12px]">
                SMILES: <code className="text-[var(--color-info-ink)]">{targetSmiles}</code>{smilesInferred ? ' (inferred)' : ''}
              </span>
            </div>

            {admetLoading && (
              <div className="p-3 text-center text-xs text-[var(--color-muted)] font-mono">
                Profiling molecular pharmacology via RDKit bridge...
              </div>
            )}

            {admetProfile && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-mono">
                <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
                  <div className="text-[12px] text-[var(--color-muted)] uppercase">Mol Weight</div>
                  <div className="text-xs font-semibold text-[var(--color-text)] mt-0.5">
                    {typeof admetProfile.molecular_weight === 'number' ? (admetProfile.molecular_weight as number).toFixed(2) : '—'} Da
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
                  <div className="text-[12px] text-[var(--color-muted)] uppercase">cLogP</div>
                  <div className="text-xs font-semibold text-[var(--color-text)] mt-0.5">
                    {typeof admetProfile.clogp === 'number' ? (admetProfile.clogp as number).toFixed(2) : '—'}
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
                  <div className="text-[12px] text-[var(--color-muted)] uppercase">TPSA</div>
                  <div className="text-xs font-semibold text-[var(--color-text)] mt-0.5">
                    {typeof admetProfile.tpsa === 'number' ? (admetProfile.tpsa as number).toFixed(1) : '—'} Å²
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
                  <div className="text-[12px] text-[var(--color-muted)] uppercase">Synthesis</div>
                    <div className="text-xs font-semibold text-[var(--color-success-ink)] mt-0.5 truncate" title={String(admetProfile.synthesizability ?? '')}>
                      {String(admetProfile.synthesizability ?? '—').split(' ')[0]}
                    </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Depth 2: Dynamic Variational Quantum Circuit Flow */}
      {canvasMode === 'workbench' && disclosureDepth === 2 && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-6">
          <div className="w-full max-w-3xl bg-[var(--color-surface)] border border-[var(--color-border)] rounded-2xl p-6 space-y-6">
            <div className="flex flex-wrap items-center justify-between border-b border-[var(--color-border)] pb-3 gap-3">
              <div>
                <div className="flex items-center space-x-2">
                  <h3 className="text-sm font-semibold text-[var(--color-text)]">{circuitData.ansatzTitle}</h3>
                  <span className="text-[12px] font-mono px-2 py-0.5 rounded-md bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)]">
                    {circuitData.numQubits} QUBITS
                  </span>
                </div>
                <p className="text-xs text-[var(--color-muted)] mt-0.5">
                  {circuitData.hasCircuit
                    ? `Proposed circuit for ${targetSystem} (Depth: ${circuitData.depthLive ? circuitData.depth : '—'} · ${circuitData.twoQLive ? `${circuitData.twoQubitGates} CNOT` : 'CNOT count not recorded'})`
                    : `No circuit proposal is recorded for ${targetSystem}.`}
                </p>
              </div>

              <div className="flex items-center space-x-2">
                {/* View Mode Toggle */}
                <div className="flex items-center p-0.5 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-xs">
                  <button
                    onClick={() => setCircuitViewMode('schematic')}
                    className={`px-2.5 py-1 rounded-lg transition-all font-medium ${
                      circuitViewMode === 'schematic' ? 'bg-[var(--color-surface-elev)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
                    }`}
                  >
                    Schematic
                  </button>
                  <button
                    onClick={() => setCircuitViewMode('qasm')}
                    className={`px-2.5 py-1 rounded-lg transition-all font-medium flex items-center space-x-1 ${
                      circuitViewMode === 'qasm' ? 'bg-[var(--color-surface-elev)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
                    }`}
                  >
                    <Code className="w-3 h-3" />
                    <span>OpenQASM</span>
                  </button>
                </div>

                <span className="text-xs font-mono text-[var(--color-info-ink)] bg-[var(--color-info-wash)] px-2.5 py-1 rounded-lg border border-[var(--color-info-edge)]">
                  Depth = {circuitData.depthLive ? circuitData.depth : '—'} | {circuitData.twoQLive ? `${circuitData.twoQubitGates} CNOT` : 'CNOT —'}
                </span>
              </div>
            </div>

            {circuitViewMode === 'schematic' ? (
              !circuitData.hasCircuit ? (
                <div className="p-4 rounded-xl border border-[var(--color-danger-edge)] bg-[var(--color-danger-wash)] text-xs text-[var(--color-danger-ink)]">
                  No circuit in this campaign — nothing to render. Propose a circuit to populate this view.
                </div>
              ) : (
              <div className="space-y-4 overflow-x-auto py-2 scrollbar-thin">
                {Array.from({ length: circuitData.numQubits }).map((_, qIdx) => {
                  return (
                    <div key={`wire-${qIdx}`} className="flex items-center space-x-3 min-w-[560px]">
                      <span className="text-xs font-mono text-[var(--color-muted)] w-16 font-bold shrink-0">
                        q[{qIdx}] |0⟩
                      </span>
                      <div className="flex-1 h-0.5 bg-[var(--color-border-strong)] flex items-center justify-around relative px-4">
                        {Array.from({ length: circuitData.totalCols }).map((__, cIdx) => {
                          const gate = circuitData.gates.find((g) => g.qubit === qIdx && g.col === cIdx);
                          if (!gate) {
                            return <div key={`empty-${qIdx}-${cIdx}`} className="w-12 h-6" />;
                          }
                          if (gate.type === 'CX_CTRL') {
                            return (
                              <div
                                key={gate.id}
                                className="w-3.5 h-3.5 rounded-full bg-[var(--color-accent)] shadow-md ring-2 ring-[var(--color-border-strong)] z-10"
                                title={`CNOT Control to q[${gate.targetQubit}]`}
                              />
                            );
                          }
                          if (gate.type === 'CX_TGT') {
                            return (
                              <div
                                key={gate.id}
                                className="w-5 h-5 rounded-full border-2 border-[var(--color-accent-edge)] flex items-center justify-center text-[12px] font-bold text-[var(--color-accent-ink)] bg-[var(--color-surface)] z-10"
                                title={`CNOT Target from q[${gate.targetQubit}]`}
                              >
                                +
                              </div>
                            );
                          }
                          return (
                            <div
                              key={gate.id}
                              className={`px-2.5 py-1 rounded-xl border text-[12px] font-mono font-bold shadow-lg z-10 ${gate.color}`}
                            >
                              {gate.label}
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}

                {(() => {
                  const gv = campaign?.quantum_factors?.gradient_variance;
                  const gvOk = typeof gv === 'number' && Number.isFinite(gv);
                  return (
                    <p className="text-xs text-[var(--color-muted)] text-center font-sans pt-2">
                      {gvOk
                        ? `Gradient variance ${(gv as number).toExponential(2)} — barren-plateau status bound to measured variance.`
                        : 'Barren-plateau status unattested — no gradient variance measured.'}
                    </p>
                  );
                })()}
              </div>
              )
            ) : (
              <div className="space-y-3">
                {circuitData.hasCircuit ? (
                  <>
                    <div className="flex items-center justify-between text-xs text-[var(--color-muted)]">
                      <span className="font-mono">OpenQASM 2.0 proposed circuit</span>
                      <button
                        onClick={() => {
                          navigator.clipboard.writeText(circuitData.rawQasm).catch(() => {});
                          setCopiedQasm(true);
                          setTimeout(() => setCopiedQasm(false), 2000);
                        }}
                        className="flex items-center space-x-1.5 px-2.5 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] text-[var(--color-label-secondary)] text-xs transition-all border border-[var(--color-border)]"
                      >
                        {copiedQasm ? <Check className="w-3.5 h-3.5 text-[var(--color-success-ink)]" /> : <Copy className="w-3.5 h-3.5" />}
                        <span>{copiedQasm ? 'Copied' : 'Copy QASM'}</span>
                      </button>
                    </div>
                    <pre className="p-4 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-xs font-mono text-[var(--color-text)] overflow-x-auto max-h-64 leading-relaxed scrollbar-thin">
                      {circuitData.rawQasm}
                    </pre>
                  </>
                ) : (
                  <div className="p-4 rounded-xl border border-[var(--color-danger-edge)] bg-[var(--color-danger-wash)] text-xs text-[var(--color-danger-ink)]">
                    No circuit proposal is recorded for this campaign.
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Depth 3: Exact Molecular Integrals & Hamiltonian */}
      {canvasMode === 'workbench' && disclosureDepth === 3 && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-6">
          <div className="w-full max-w-2xl bg-[var(--color-surface)] border border-[var(--color-border)] rounded-2xl p-6 space-y-4">
            <h3 className="text-sm font-semibold text-[var(--color-text)]">Full Configuration Interaction (FCI) Benchmark</h3>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="p-4 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-1">
                <span className="text-[12px] text-[var(--color-label-secondary)] uppercase tracking-wider font-semibold">Exact FCI Anchor</span>
                <div className="text-base font-mono font-bold text-[var(--color-success-ink)]">
                  {energyCalculated.anchorFci != null && Number.isFinite(energyCalculated.anchorFci) ? `${energyCalculated.anchorFci.toFixed(6)} Ha` : 'UNVERIFIED'}
                </div>
                <span className="text-xs text-[var(--color-muted)] font-sans">{energyCalculated.anchorComputed && energyCalculated.anchorFci != null ? 'PySCF-computed anchor' : 'No anchor computed — bridge unavailable'}</span>
              </div>

              <div className="p-4 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-1">
                <span className="text-[12px] text-[var(--color-label-secondary)] uppercase tracking-wider font-semibold">Classical Hartree-Fock</span>
                <div className="text-base font-mono font-bold text-[var(--color-text)]">
                  {energyCalculated.rhf != null && Number.isFinite(energyCalculated.rhf) ? `${energyCalculated.rhf.toFixed(6)} Ha` : 'UNVERIFIED (no RHF baseline)'}
                </div>
                <span className="text-xs text-[var(--color-muted)] font-sans">Mean-field baseline{energyCalculated.rhf == null ? ' — not measured' : ''}</span>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] space-y-2">
              <span className="text-xs font-semibold text-[var(--color-text)]">Second-Quantized Hamiltonian Mapping</span>
              <p className="text-[12px] font-code text-[var(--color-faint)]">Coefficient provenance</p>
              <p className="text-xs font-mono text-[var(--color-info-ink)] leading-relaxed overflow-x-auto whitespace-nowrap scrollbar-thin">
                {hamiltonianString}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Bottom Floating Control Pill (3-Depth Progressive Disclosure Switcher) */}
      {canvasMode === 'workbench' && (
        <div className="relative z-10 flex flex-wrap items-center justify-between gap-4 pt-4 border-t border-[var(--color-border)]">
          {/* Real-time energy readouts */}
          <div className="flex items-center space-x-6 text-xs font-mono">
            <div>
              <span className="text-[var(--color-muted)] font-medium">Morse model at R: </span>
              <span className="text-[var(--color-text)] font-bold">{energyCalculated.modelEnergy != null && Number.isFinite(energyCalculated.modelEnergy) ? `${energyCalculated.modelEnergy.toFixed(5)} Ha` : 'UNVERIFIED'}</span>
            </div>
            <div>
              <span className="text-[var(--color-muted)] font-medium">FCI anchor: </span>
              <span className="text-[var(--color-success-ink)] font-bold">{energyCalculated.anchorFci != null && Number.isFinite(energyCalculated.anchorFci) ? `${energyCalculated.anchorFci.toFixed(5)} Ha` : 'UNVERIFIED'}</span>
            </div>
          </div>

          {/* 3-Depth Progressive Disclosure Segmented Switch */}
          <div role="group" aria-label="Canvas mode" className="flex items-center p-1 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] shadow-inner">
            <button
              onClick={() => setDisclosureDepth(1)}
              className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
                disclosureDepth === 1
                  ? 'bg-[var(--color-fill-hover)] text-[var(--color-text)] shadow-sm font-semibold'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              1. Physical System
            </button>
            <button
              onClick={() => setDisclosureDepth(2)}
              className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
                disclosureDepth === 2
                  ? 'bg-[var(--color-fill-hover)] text-[var(--color-text)] shadow-sm font-semibold'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              2. Quantum Circuit
            </button>
            <button
              onClick={() => setDisclosureDepth(3)}
              className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
                disclosureDepth === 3
                  ? 'bg-[var(--color-fill-hover)] text-[var(--color-text)] shadow-sm font-semibold'
                  : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
              }`}
            >
              3. Exact Integrals
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
