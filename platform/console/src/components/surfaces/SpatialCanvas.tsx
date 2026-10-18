'use client';

import React, { useState, useMemo, useEffect } from 'react';
import { Atom, Code, Copy, Check } from 'lucide-react';
import { CampaignState } from '@/types';
import { fetchMolecularADMET } from '@/lib/api';

interface SpatialCanvasProps {
  campaign?: CampaignState | null;
}

export function SpatialCanvas({ campaign }: SpatialCanvasProps) {
  // Discovery target
  const targetSystem = campaign?.target_system || 'H2';
  const isLiH = targetSystem.toUpperCase().includes('LIH');
  
  const systemDisplayName = useMemo(() => {
    const raw = campaign?.quantum_factors?.target_molecule || campaign?.target_system || 'H2';
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
    const formula = campaign?.quantum_factors?.molecular_formula || campaign?.quantum_factors?.target_molecule || 'H2';
    const matches = (formula.match(/([A-Z][a-z]?)/g) || []).filter((el) => validElements.includes(el));
    if (matches.length >= 2 && matches[0] !== matches[1]) {
      return [matches[0], matches[1]];
    }
    if (matches.length >= 1) {
      return [`${matches[0]}₁`, `${matches[0]}₂`];
    }
    return ['H₁', 'H₂'];
  }, [campaign?.quantum_factors?.molecular_formula, campaign?.quantum_factors?.target_molecule]);

  // Equilibrium bond distance
  const equilibriumR = useMemo(() => {
    if (campaign?.quantum_factors?.bond_distance) return campaign.quantum_factors.bond_distance;
    const sys = (campaign?.target_system || '').toUpperCase();
    if (sys.includes('LIH')) return 1.5957;
    if (sys.includes('BEH')) return 1.3300;
    if (sys.includes('CO')) return 1.1280;
    if (sys.includes('N2')) return 1.0980;
    if (sys.includes('H2O')) return 0.9580;
    return 0.7414;
  }, [campaign?.quantum_factors?.bond_distance, campaign?.target_system]);

  const campaignBondDist = campaign?.quantum_factors?.bond_distance;
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

  const targetSmiles = useMemo(() => {
    if (campaign?.smiles) return campaign.smiles;
    const sys = (campaign?.quantum_factors?.target_molecule || campaign?.target_system || 'H2').toUpperCase();
    if (sys.includes('LIH')) return '[Li+].[H-]';
    if (sys.includes('H2O')) return 'O';
    if (sys.includes('CO2')) return 'O=C=O';
    if (sys.includes('CO')) return '[C-]#[O+]';
    if (sys.includes('N2')) return 'N#N';
    return '[H][H]';
  }, [campaign?.smiles, campaign?.quantum_factors?.target_molecule, campaign?.target_system]);

  const [admetProfile, setAdmetProfile] = useState<Record<string, unknown> | null>(null);
  const [admetLoading, setAdmetLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    if (!targetSmiles) return;
    setAdmetLoading(true);
    fetchMolecularADMET(targetSmiles)
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
  }, [targetSmiles]);

  // Compute energy on the fly based on physical Morse potential anchored to PySCF FCI
  const energyCalculated = useMemo(() => {
    const Re = equilibriumR;
    const anchor = campaign?.classical_anchor;
    const anchorComputed = anchor?.anchor_computed === true;
    const fciRaw = anchor?.fci_exact_energy;
    const fciFinite = typeof fciRaw === 'number' && Number.isFinite(fciRaw);
    // No anchor → no Δ, no invented baseline. Model energy renders as
    // Morse extrapolation only when an anchor exists; otherwise UNVERIFIED.
    const fciGround = anchorComputed && fciFinite ? (fciRaw as number) : null;
    const De = fciGround != null ? Math.abs(fciGround) * 0.15 : 0;
    const a = 1.44; // Potential width parameter
    const deltaR = bondDistance - Re;
    const morseTerm = fciGround != null ? De * Math.pow(1 - Math.exp(-a * deltaR), 2) : 0;
    const energy = fciGround != null ? fciGround + morseTerm : null;
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
  }, [bondDistance, equilibriumR, campaign?.classical_anchor]);

  // Atom positions for SVG physical rendering
  const atomOffset = useMemo(() => {
    // Map bond distance in Angstroms to pixel distance
    return Math.round(bondDistance * 110);
  }, [bondDistance]);

  // Dynamic Circuit parser from session QuantumFactors / OpenQASM
  const circuitData = useMemo(() => {
    const qf = campaign?.quantum_factors;
    const defaultQubits = isLiH ? 4 : 2;
    const numQubits = (typeof qf?.qubit_count === 'number' && Number.isFinite(qf.qubit_count) ? qf.qubit_count : defaultQubits);
    const rawQasm = (qf?.qasm || '').trim();
    // No-circuit refusal state: without QASM there is no circuit to parse.
    // The schematic below renders only from live QASM; otherwise it refuses.
    const hasCircuit = rawQasm !== '';
    // Schematic fallback QASM is illustrative only and watermarked as such
    // in every copyable string — never presented as session data.
    const qasmText = hasCircuit ? rawQasm : (() => {
      const q = numQubits;
      let s = `// schematic — illustrative, not session data (no circuit in this campaign)\nOPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[${q}];\ncreg c[${q}];\n`;
      s += 'h q[0];\n';
      for (let i = 0; i < q; i++) s += `rz(${(0.785 * (i + 1)).toFixed(3)}) q[${i}];\n`;
      for (let i = 0; i < q - 1; i++) s += `cx q[${i}], q[${i + 1}];\n`;
      for (let i = 0; i < q; i++) s += `measure q[${i}] -> c[${i}];\n`;
      return s;
    })();

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
          gates.push({ id: `cx-c-${c}-${col}`, type: 'CX_CTRL', label: '•', qubit: c, col, targetQubit: t, color: 'text-indigo-400' });
          gates.push({ id: `cx-t-${t}-${col}`, type: 'CX_TGT', label: '⊕', qubit: t, col, targetQubit: c, color: 'text-indigo-400' });
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
          gates.push({ id: `h-${q}-${col}`, type: 'H', label: 'H', qubit: q, col, color: 'bg-blue-950/80 border-blue-500/60 text-blue-300' });
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
            ? 'bg-cyan-950/80 border-cyan-500/60 text-cyan-300'
            : name === 'RZ'
            ? 'bg-emerald-950/80 border-emerald-500/60 text-emerald-300'
            : 'bg-purple-950/80 border-purple-500/60 text-purple-300';
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
          gates.push({ id: `m-${q}-${col}`, type: 'MEASURE', label: '⏱ M', qubit: q, col, color: 'bg-amber-950/80 border-amber-500/60 text-amber-300' });
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
      ansatzTitle: qf?.ansatz_family || (isLiH ? 'TwoLocal / RyRz Entangling' : 'UCCSD Parameterized'),
      depth: typeof depthRaw === 'number' && Number.isFinite(depthRaw) ? depthRaw : (isLiH ? 8 : 4),
      depthLive: typeof depthRaw === 'number' && Number.isFinite(depthRaw),
      twoQubitGates: typeof twoQRaw === 'number' && Number.isFinite(twoQRaw) ? twoQRaw : (numQubits - 1),
      twoQLive: typeof twoQRaw === 'number' && Number.isFinite(twoQRaw),
    };
  }, [campaign?.quantum_factors, isLiH]);

  const hamiltonianString = useMemo(() => {
    const fciRaw = campaign?.classical_anchor?.fci_exact_energy;
    const anchorComputed = campaign?.classical_anchor?.anchor_computed === true;
    const fciOk = anchorComputed && typeof fciRaw === 'number' && Number.isFinite(fciRaw);
    const fciLabel = fciOk ? (fciRaw as number).toFixed(4) : 'UNVERIFIED';
    const prefix = '// schematic coefficients — illustrative, anchor value live where shown\n';
    const n = circuitData.numQubits;
    if (n === 2) {
      return `${prefix}H = ${fciLabel} I + 0.3979 Z₀ - 0.3979 Z₁ - 0.0112 Z₀Z₁ + 0.1809 X₀X₁`;
    }
    if (n === 4) {
      return `${prefix}H = ${fciLabel} I + 0.1712 Z₀ - 0.2227 Z₁ + 0.1686 Z₂ - 0.2227 Z₃ + 0.1205 Z₀Z₁ + 0.1658 Z₁Z₂ + 0.0453 X₀X₁Y₂Y₃`;
    }
    const terms = [`${fciLabel} I`];
    for (let i = 0; i < n; i++) {
      const coeff = (0.25 / (i + 1)).toFixed(4);
      terms.push(`${i % 2 === 0 ? '+' : '-'} ${coeff} Z${i}`);
    }
    terms.push(`+ 0.0412 Z₀Z₁`);
    return `${prefix}H = ${terms.join(' ')}`;
  }, [campaign?.classical_anchor?.anchor_computed, campaign?.classical_anchor?.fci_exact_energy, circuitData.numQubits]);

  return (
    <div className="relative min-h-[680px] rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0F19] to-[#07090E] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-8 select-none">
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
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Atom className="w-5 h-5 text-cyan-400 animate-float-gentle" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <h2 className="text-base font-semibold text-white tracking-tight">
                {systemDisplayName}
              </h2>
              <span className="text-[11px] font-mono font-medium px-2 py-0.5 rounded-full bg-cyan-950/80 text-cyan-300 border border-cyan-800/50">
                Ground State PES
              </span>
            </div>
            <p className="text-xs text-slate-300 mt-0.5">
              Direct physical manipulation of nuclear coordinates and electronic structure
            </p>
            <p className="text-[10px] font-code text-slate-500 mt-0.5">
              2-atom schematic only — larger systems still render two bubbles
            </p>
          </div>
        </div>

        {/* Calm Truth Status Indicator */}
        <div className="flex items-center space-x-2.5 px-3.5 py-1.5 rounded-2xl bg-black/40 border border-white/10 backdrop-blur-xl">
          <div className={`w-2 h-2 rounded-full ${energyCalculated.anchored ? 'bg-emerald-400 shadow-[0_0_12px_rgba(52,211,153,0.8)]' : 'bg-amber-400 shadow-[0_0_12px_rgba(251,191,36,0.8)]'}`} />
          <span className="text-xs font-medium text-slate-200">
            {energyCalculated.anchored
              ? 'Anchored at computed FCI'
              : energyCalculated.displaced
                ? 'Morse extrapolation — not measured at this R'
                : 'Unverified anchor (bridge unavailable)'}
          </span>
          <span className="text-xs text-slate-400">•</span>
          <span className="text-xs font-mono font-semibold text-slate-200">
            {energyCalculated.deltaMHa != null && Number.isFinite(energyCalculated.deltaMHa) ? `Δ = ${energyCalculated.deltaMHa.toFixed(2)} mHa` : 'Δ = UNVERIFIED (no anchor)'}
          </span>
        </div>
      </div>

      {/* Central Living Discovery Stage (Depth 1: The Molecular Target) */}
      {disclosureDepth === 1 && (
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
              className="absolute h-0.5 bg-gradient-to-r from-cyan-400 via-emerald-400 to-cyan-400 opacity-60 transition-all duration-200"
              style={{ width: `${atomOffset}px` }}
            />

            {/* Left Nucleus */}
            <div 
              className="absolute flex flex-col items-center transform -translate-x-1/2 transition-all duration-200"
              style={{ left: `calc(50% - ${atomOffset / 2}px)` }}
            >
              <div className="w-14 h-14 rounded-full bg-gradient-to-br from-cyan-400 to-blue-600 shadow-xl flex items-center justify-center text-white font-bold text-base border-2 border-white/20">
                {atom1}
              </div>
              <span className="text-[10px] font-mono text-cyan-300 mt-2 font-medium">Nuclear Core</span>
            </div>

            {/* Right Nucleus */}
            <div 
              className="absolute flex flex-col items-center transform translate-x-1/2 transition-all duration-200"
              style={{ left: `calc(50% + ${atomOffset / 2}px)` }}
            >
              <div className="w-14 h-14 rounded-full bg-gradient-to-br from-emerald-400 to-teal-600 shadow-xl flex items-center justify-center text-white font-bold text-base border-2 border-white/20">
                {atom2}
              </div>
              <span className="text-[10px] font-mono text-emerald-300 mt-2 font-medium">Nuclear Core</span>
            </div>

            {/* Bond Length Badge (Floating on bond center) */}
            <div className="absolute -top-4 px-3 py-1 rounded-full bg-black/70 border border-white/15 backdrop-blur-md text-[11px] font-mono font-bold text-cyan-200 shadow-md">
              R = {typeof bondDistance === 'number' && Number.isFinite(bondDistance) ? bondDistance.toFixed(4) : 'UNVERIFIED'} Å
            </div>
          </div>

          {/* Direct Manipulation Tactile Bond Slider */}
          <div className="w-full max-w-md mt-6 space-y-3">
            <div className="flex justify-between items-center text-xs text-slate-300 font-medium">
              <span>Bond Length Control</span>
              <button 
                onClick={() => setBondDistance(equilibriumR)}
                className="text-xs text-cyan-400 hover:text-cyan-300 transition-colors font-semibold hover:underline"
              >
                Reset to Equilibrium ({equilibriumR} Å)
              </button>
            </div>
            <input 
              type="range"
              min={Math.max(0.3, parseFloat((equilibriumR * 0.45).toFixed(3)))}
              max={parseFloat((equilibriumR * 2.2).toFixed(3))}
              step="0.005"
              value={bondDistance}
              onChange={(e) => setBondDistance(parseFloat(e.target.value))}
              className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-400 focus:outline-none focus:ring-2 focus:ring-cyan-400/50"
            />
            <div className="flex justify-between text-xs font-mono text-slate-300 font-medium">
              <span>Compressed (Repulsive)</span>
              <span className="text-emerald-300 font-bold">Equilibrium Valley</span>
              <span>Dissociated</span>
            </div>
          </div>

          {/* Molecular Pharmacology & ADMET Profile (RDKit Bridge Anchored) */}
          <div className="w-full max-w-md mt-5 p-4 rounded-2xl bg-black/40 border border-white/[0.08] backdrop-blur-xl space-y-3">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px] flex items-center space-x-2">
                <Atom className="w-4 h-4 text-emerald-400" />
                <span>Molecular Pharmacology &amp; ADMET</span>
              </span>
              <span className="text-slate-400 font-mono text-[10px]">
                SMILES: <code className="text-cyan-300">{targetSmiles}</code>
              </span>
            </div>

            {admetLoading && (
              <div className="p-3 text-center text-xs text-slate-400 font-mono">
                Profiling molecular pharmacology via RDKit bridge...
              </div>
            )}

            {admetProfile && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-mono">
                <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.05]">
                  <div className="text-[10px] text-slate-400 uppercase">Mol Weight</div>
                  <div className="text-xs font-semibold text-white mt-0.5">
                    {typeof admetProfile.molecular_weight === 'number' ? (admetProfile.molecular_weight as number).toFixed(2) : '—'} Da
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.05]">
                  <div className="text-[10px] text-slate-400 uppercase">cLogP</div>
                  <div className="text-xs font-semibold text-white mt-0.5">
                    {typeof admetProfile.clogp === 'number' ? (admetProfile.clogp as number).toFixed(2) : '—'}
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.05]">
                  <div className="text-[10px] text-slate-400 uppercase">TPSA</div>
                  <div className="text-xs font-semibold text-white mt-0.5">
                    {typeof admetProfile.tpsa === 'number' ? (admetProfile.tpsa as number).toFixed(1) : '—'} Å²
                  </div>
                </div>

                <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.05]">
                  <div className="text-[10px] text-slate-400 uppercase">Synthesis</div>
                  <div className="text-xs font-semibold text-emerald-300 mt-0.5 truncate" title={String(admetProfile.synthesizability || '')}>
                    {String(admetProfile.synthesizability || 'High').split(' ')[0]}
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Depth 2: Dynamic Variational Quantum Circuit Flow */}
      {disclosureDepth === 2 && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-6">
          <div className="w-full max-w-3xl bg-black/50 border border-white/10 rounded-2xl p-6 backdrop-blur-xl space-y-6">
            <div className="flex flex-wrap items-center justify-between border-b border-white/[0.08] pb-3 gap-3">
              <div>
                <div className="flex items-center space-x-2">
                  <h3 className="text-sm font-semibold text-white">{circuitData.ansatzTitle}</h3>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-md bg-cyan-950/80 text-cyan-300 border border-cyan-800/50">
                    {circuitData.numQubits} QUBITS
                  </span>
                </div>
                <p className="text-xs text-slate-400 mt-0.5">
                  Dynamic parameterized circuit mapped for {targetSystem} (Depth: {circuitData.depth}{circuitData.depthLive ? '' : ' (illustrative)'} · {circuitData.twoQubitGates} CNOT{circuitData.twoQLive ? '' : ' (illustrative)'})
                </p>
              </div>

              <div className="flex items-center space-x-2">
                {/* View Mode Toggle */}
                <div className="flex items-center p-0.5 rounded-xl bg-black/60 border border-white/10 text-xs">
                  <button
                    onClick={() => setCircuitViewMode('schematic')}
                    className={`px-2.5 py-1 rounded-lg transition-all font-medium ${
                      circuitViewMode === 'schematic' ? 'bg-white/15 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'
                    }`}
                  >
                    Schematic
                  </button>
                  <button
                    onClick={() => setCircuitViewMode('qasm')}
                    className={`px-2.5 py-1 rounded-lg transition-all font-medium flex items-center space-x-1 ${
                      circuitViewMode === 'qasm' ? 'bg-white/15 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'
                    }`}
                  >
                    <Code className="w-3 h-3" />
                    <span>OpenQASM</span>
                  </button>
                </div>

                <span className="text-xs font-mono text-cyan-400 bg-cyan-950/60 px-2.5 py-1 rounded-lg border border-cyan-800/40">
                  Depth = {circuitData.depth}{circuitData.depthLive ? '' : ' (EST)'} | {circuitData.twoQubitGates} CNOT{circuitData.twoQLive ? '' : ' (EST)'}
                </span>
              </div>
            </div>

            {circuitViewMode === 'schematic' ? (
              !circuitData.hasCircuit ? (
                <div className="p-4 rounded-xl border border-rose-500/30 bg-rose-950/30 text-xs text-rose-200">
                  No circuit in this campaign — nothing to render. Propose a circuit to populate this view.
                </div>
              ) : (
              <div className="space-y-4 overflow-x-auto py-2 scrollbar-thin">
                {Array.from({ length: circuitData.numQubits }).map((_, qIdx) => {
                  return (
                    <div key={`wire-${qIdx}`} className="flex items-center space-x-3 min-w-[560px]">
                      <span className="text-xs font-mono text-slate-400 w-16 font-bold shrink-0">
                        q[{qIdx}] |0⟩
                      </span>
                      <div className="flex-1 h-0.5 bg-slate-700 flex items-center justify-around relative px-4">
                        {Array.from({ length: circuitData.totalCols }).map((__, cIdx) => {
                          const gate = circuitData.gates.find((g) => g.qubit === qIdx && g.col === cIdx);
                          if (!gate) {
                            return <div key={`empty-${qIdx}-${cIdx}`} className="w-12 h-6" />;
                          }
                          if (gate.type === 'CX_CTRL') {
                            return (
                              <div
                                key={gate.id}
                                className="w-3.5 h-3.5 rounded-full bg-indigo-500 shadow-md ring-2 ring-white/20 z-10"
                                title={`CNOT Control to q[${gate.targetQubit}]`}
                              />
                            );
                          }
                          if (gate.type === 'CX_TGT') {
                            return (
                              <div
                                key={gate.id}
                                className="w-5 h-5 rounded-full border-2 border-indigo-400 flex items-center justify-center text-[10px] font-bold text-indigo-300 bg-black z-10"
                                title={`CNOT Target from q[${gate.targetQubit}]`}
                              >
                                +
                              </div>
                            );
                          }
                          return (
                            <div
                              key={gate.id}
                              className={`px-2.5 py-1 rounded-xl border text-[11px] font-mono font-bold shadow-lg z-10 ${gate.color}`}
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
                    <p className="text-xs text-slate-400 text-center font-sans pt-2">
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
                <div className="flex items-center justify-between text-xs text-slate-400">
                  <span className="font-mono">OpenQASM 2.0 Intermediate Representation {!circuitData.hasCircuit ? '(schematic — illustrative)' : ''}</span>
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(circuitData.rawQasm).catch(() => {});
                      setCopiedQasm(true);
                      setTimeout(() => setCopiedQasm(false), 2000);
                    }}
                    className="flex items-center space-x-1.5 px-2.5 py-1 rounded-lg bg-white/[0.05] hover:bg-white/10 text-slate-300 text-xs transition-all border border-white/10"
                  >
                    {copiedQasm ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    <span>{copiedQasm ? 'Copied' : 'Copy QASM'}</span>
                  </button>
                </div>
                <pre className="p-4 rounded-xl bg-black/80 border border-white/[0.08] text-xs font-mono text-cyan-300 overflow-x-auto max-h-64 leading-relaxed scrollbar-thin">
                  {circuitData.rawQasm}
                </pre>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Depth 3: Exact Molecular Integrals & Hamiltonian */}
      {disclosureDepth === 3 && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-6">
          <div className="w-full max-w-2xl bg-black/60 border border-white/10 rounded-2xl p-6 backdrop-blur-xl space-y-4">
            <h3 className="text-sm font-semibold text-white">Full Configuration Interaction (FCI) Benchmark</h3>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="p-4 rounded-xl bg-white/[0.03] border border-white/05 space-y-1">
                <span className="text-[11px] text-slate-300 uppercase tracking-wider font-semibold">Exact FCI Anchor</span>
                <div className="text-base font-mono font-bold text-emerald-400">
                  {energyCalculated.anchorFci != null && Number.isFinite(energyCalculated.anchorFci) ? `${energyCalculated.anchorFci.toFixed(6)} Ha` : 'UNVERIFIED'}
                </div>
                <span className="text-xs text-slate-300 font-sans">{energyCalculated.anchorComputed && energyCalculated.anchorFci != null ? 'PySCF-computed anchor' : 'No anchor computed — bridge unavailable'}</span>
              </div>

              <div className="p-4 rounded-xl bg-white/[0.03] border border-white/05 space-y-1">
                <span className="text-[11px] text-slate-300 uppercase tracking-wider font-semibold">Classical Hartree-Fock</span>
                <div className="text-base font-mono font-bold text-slate-200">
                  {energyCalculated.rhf != null && Number.isFinite(energyCalculated.rhf) ? `${energyCalculated.rhf.toFixed(6)} Ha` : 'UNVERIFIED (no RHF baseline)'}
                </div>
                <span className="text-xs text-slate-300 font-sans">Mean-field baseline{energyCalculated.rhf == null ? ' — not measured' : ''}</span>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-black/40 border border-white/05 space-y-2">
              <span className="text-xs font-semibold text-slate-200">Second-Quantized Hamiltonian Mapping</span>
              <p className="text-[10px] font-code text-slate-500">Illustrative Pauli form — coefficients schematic, anchor value live</p>
              <p className="text-xs font-mono text-cyan-300 leading-relaxed overflow-x-auto whitespace-nowrap scrollbar-thin">
                {hamiltonianString}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Bottom Floating Control Pill (3-Depth Progressive Disclosure Switcher) */}
      <div className="relative z-10 flex flex-wrap items-center justify-between gap-4 pt-4 border-t border-white/[0.08]">
        {/* Real-time energy readouts */}
        <div className="flex items-center space-x-6 text-xs font-mono">
          <div>
            <span className="text-slate-300 font-medium">Morse model at R: </span>
            <span className="text-white font-bold">{energyCalculated.modelEnergy != null && Number.isFinite(energyCalculated.modelEnergy) ? `${energyCalculated.modelEnergy.toFixed(5)} Ha` : 'UNVERIFIED'}</span>
          </div>
          <div>
            <span className="text-slate-300 font-medium">FCI anchor: </span>
            <span className="text-emerald-400 font-bold">{energyCalculated.anchorFci != null && Number.isFinite(energyCalculated.anchorFci) ? `${energyCalculated.anchorFci.toFixed(5)} Ha` : 'UNVERIFIED'}</span>
          </div>
        </div>

        {/* 3-Depth Progressive Disclosure Segmented Switch */}
        <div className="flex items-center p-1 rounded-2xl bg-black/60 border border-white/10 backdrop-blur-xl shadow-inner">
          <button
            onClick={() => setDisclosureDepth(1)}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
              disclosureDepth === 1
                ? 'bg-white/20 text-white shadow-sm font-semibold'
                : 'text-slate-300 hover:text-white'
            }`}
          >
            1. Physical System
          </button>
          <button
            onClick={() => setDisclosureDepth(2)}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
              disclosureDepth === 2
                ? 'bg-white/20 text-white shadow-sm font-semibold'
                : 'text-slate-300 hover:text-white'
            }`}
          >
            2. Quantum Circuit
          </button>
          <button
            onClick={() => setDisclosureDepth(3)}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-medium transition-all ${
              disclosureDepth === 3
                ? 'bg-white/20 text-white shadow-sm font-semibold'
                : 'text-slate-300 hover:text-white'
            }`}
          >
            3. Exact Integrals
          </button>
        </div>
      </div>
    </div>
  );
}
