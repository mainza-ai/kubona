'use client';

import React, { useState, useMemo } from 'react';
import { Atom, Sliders } from 'lucide-react';

interface MolecularActiveSpaceWidgetProps {
  moleculeName?: string;
  numElectrons?: number;
  numOrbitals?: number;
  homoLumoGapHa?: number;
  onActiveSpaceChange?: (activeElectrons: number, activeOrbitals: number) => void;
}

export function MolecularActiveSpaceWidget({
  moleculeName,
  numElectrons,
  numOrbitals,
  homoLumoGapHa,
  onActiveSpaceChange,
}: MolecularActiveSpaceWidgetProps) {
  // Backed only when the caller binds live session values; otherwise the
  // orbital ladder is layout-only and sliders stay disabled behind a demo
  // watermark — never editable fiction presented as a live active space.
  const backed = moleculeName != null && numElectrons != null && numOrbitals != null;
  const displayName = moleculeName ?? 'LiH';
  const totalE = numElectrons ?? (displayName.toUpperCase().includes('LIH') ? 4 : displayName.toUpperCase().includes('H2') ? 2 : 4);
  const totalO = numOrbitals ?? (displayName.toUpperCase().includes('LIH') ? 4 : displayName.toUpperCase().includes('H2') ? 2 : 4);
  const gap = homoLumoGapHa != null && Number.isFinite(homoLumoGapHa) && homoLumoGapHa > 0 ? homoLumoGapHa : 0.042;
  const [activeElectrons, setActiveElectrons] = useState(Math.min(10, totalE));
  const [activeOrbitals, setActiveOrbitals] = useState(Math.min(10, totalO));
  const [prevSpaceKey, setPrevSpaceKey] = useState(`${totalE}:${totalO}`);
  if (prevSpaceKey !== `${totalE}:${totalO}`) {
    setPrevSpaceKey(`${totalE}:${totalO}`);
    setActiveElectrons(Math.min(10, totalE));
    setActiveOrbitals(Math.min(10, totalO));
  }
  const [selectedOrbital, setSelectedOrbital] = useState<number>(2);
  const [viewState, setViewState] = useState<'orbitals' | 'electrons'>('orbitals');

  const isDiatomic = /^(H2|LIH|HF|CO|N2|NO)$/i.test(displayName.replace(/[^a-zA-Z0-9]/g, ''));
  const nOcc = Math.max(1, Math.floor(totalE / 2));
  const nVirt = Math.max(1, Math.max(totalO - nOcc, 2));

  // Dynamically generated levels around Fermi level E_F = -0.25 Ha
  const orbitalLevels = useMemo(() => {
    const levels: Array<{ idx: number; name: string; energy: number; occ: number; sym: string; isCore: boolean; isVirtual: boolean }> = [];
    const ef = -0.250;

    // Virtual orbitals (LUMO+N down to LUMO)
    const virtCount = Math.min(Math.max(nVirt, 2), 4);
    for (let i = virtCount - 1; i >= 0; i--) {
      const idx = nOcc + i + 1;
      const name = i === 0 ? 'LUMO' : `LUMO+${i}`;
      const energy = ef + (gap / 2) + (i * 0.048);
      const sym = isDiatomic ? (i % 2 === 0 ? 'σ*' : 'π*') : (i % 2 === 0 ? 'eg*' : 't1u*');
      levels.push({
        idx,
        name,
        energy,
        occ: 0,
        sym,
        isCore: false,
        isVirtual: true,
      });
    }

    // Occupied orbitals (HOMO down to HOMO-M)
    const occCount = Math.min(Math.max(nOcc, 1), 5);
    for (let i = 0; i < occCount; i++) {
      const idx = nOcc - i;
      const name = i === 0 ? 'HOMO' : `HOMO-${i}`;
      const energy = ef - (gap / 2) - (i * 0.045);
      const isCore = i >= Math.ceil(activeElectrons / 2);
      const sym = isDiatomic ? (i % 2 === 0 ? 'σ' : 'π') : (i % 2 === 0 ? 't2g' : 'a1g');
      levels.push({
        idx,
        name,
        energy,
        occ: 2,
        sym,
        isCore,
        isVirtual: false,
      });
    }

    return levels;
  }, [gap, activeElectrons, isDiatomic, nOcc, nVirt]);

  const handleElectronsChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseInt(e.target.value, 10);
    setActiveElectrons(val);
    onActiveSpaceChange?.(val, activeOrbitals);
  };

  const handleOrbitalsChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseInt(e.target.value, 10);
    setActiveOrbitals(val);
    onActiveSpaceChange?.(activeElectrons, val);
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-4">
      {!backed && (
        <div className="p-2.5 rounded-xl bg-amber-950/50 border border-amber-500/40 text-amber-200 text-[11px] font-code text-center">
          synthetic demo — not session data. Sliders disabled until a live campaign backs this view.
        </div>
      )}
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">3D Molecular Active Space Viewer (Preview)<span className="ml-2 align-middle text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">Illustrative</span></h3>
            <p className="text-xs text-white/50">{displayName} • CASSCF({activeElectrons}e, {activeOrbitals}o){!backed ? ' · layout-only' : ''}</p>
          </div>
        </div>
        <div className="flex items-center space-x-1.5 text-xs">
          <button
            onClick={() => setViewState('orbitals')}
            className={`px-2.5 py-1 rounded-lg border transition ${
              viewState === 'orbitals'
                ? 'bg-purple-600/20 border-purple-500/40 text-purple-300'
                : 'border-white/[0.06] text-white/60 hover:text-white'
            }`}
          >
            Orbitals
          </button>
          <button
            onClick={() => setViewState('electrons')}
            className={`px-2.5 py-1 rounded-lg border transition ${
              viewState === 'electrons'
                ? 'bg-purple-600/20 border-purple-500/40 text-purple-300'
                : 'border-white/[0.06] text-white/60 hover:text-white'
            }`}
          >
            Occupancy
          </button>
        </div>
      </div>

      {/* Interactive Controls & Diagram */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Orbital Energy Ladder / Occupancy View */}
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4 flex flex-col justify-between min-h-[300px]">
          <div className="text-xs font-mono text-white/60 flex justify-between items-center mb-2 pb-1 border-b border-white/[0.04]">
            <span>{viewState === 'orbitals' ? 'Orbital Energy Ladder (Ha)' : 'Active Space Configuration'}</span>
            <span className="text-emerald-400">Gap: {typeof gap === 'number' && Number.isFinite(gap) ? gap.toFixed(3) : '—'} Ha ({typeof gap === 'number' && Number.isFinite(gap) ? Math.round(gap * 27.2114 * 100) / 100 : '—'} eV)</span>
          </div>

          {viewState === 'orbitals' ? (
            <div className="flex-1 flex flex-col justify-between space-y-1 py-1">
              {/* Virtual Orbitals (Rendered from highest down to LUMO) */}
              <div className="space-y-1">
                {orbitalLevels.filter(lvl => lvl.isVirtual).map((lvl) => {
                  const isSelected = selectedOrbital === lvl.idx;
                  return (
                    <div
                      key={lvl.idx}
                      onClick={() => setSelectedOrbital(lvl.idx)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          setSelectedOrbital(lvl.idx);
                        }
                      }}
                      role="button"
                      tabIndex={0}
                      aria-pressed={isSelected}
                      aria-label={`Select orbital ${lvl.name}`}
                      className={`flex items-center justify-between text-xs px-2.5 py-1 rounded cursor-pointer transition ${
                        isSelected
                          ? 'bg-purple-500/20 border border-purple-500/40 text-purple-200'
                          : 'hover:bg-white/[0.04] text-white/60'
                      }`}
                    >
                      <div className="flex items-center space-x-2">
                        <span className="font-mono text-[10px] w-14 font-semibold">{lvl.name}</span>
                        <span className="text-[10px] text-white/40 font-mono">({lvl.sym})</span>
                      </div>
                      <div className="flex items-center space-x-3">
                        <span className="text-white/20 font-mono text-[11px]">—</span>
                        <span className="font-mono text-[10px] text-white/50">{lvl.energy.toFixed(3)}</span>
                      </div>
                    </div>
                  );
                })}
              </div>

              {/* In-Flow Clean Fermi Level Divider (Zero Collisions!) */}
              <div className="my-1.5 py-1 px-3 rounded-lg bg-emerald-500/10 border-y border-dashed border-emerald-500/40 flex items-center justify-between text-[10px] text-emerald-400 font-mono shadow-sm">
                <span className="flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  Fermi Level E_F
                </span>
                <span>HOMO-LUMO Gap</span>
              </div>

              {/* Occupied Orbitals (Rendered from HOMO down to core) */}
              <div className="space-y-1">
                {orbitalLevels.filter(lvl => !lvl.isVirtual).map((lvl) => {
                  const isSelected = selectedOrbital === lvl.idx;
                  const isActive = !lvl.isCore;
                  return (
                    <div
                      key={lvl.idx}
                      onClick={() => setSelectedOrbital(lvl.idx)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          setSelectedOrbital(lvl.idx);
                        }
                      }}
                      role="button"
                      tabIndex={0}
                      aria-pressed={isSelected}
                      aria-label={`Select orbital ${lvl.name}`}
                      className={`flex items-center justify-between text-xs px-2.5 py-1 rounded cursor-pointer transition ${
                        isSelected
                          ? 'bg-purple-500/20 border border-purple-500/40 text-purple-200'
                          : isActive
                          ? 'hover:bg-white/[0.04] text-white/90 bg-white/[0.01]'
                          : 'text-white/40'
                      }`}
                    >
                      <div className="flex items-center space-x-2">
                        <span className="font-mono text-[10px] w-14 font-semibold">{lvl.name}</span>
                        <span className="text-[10px] text-white/40 font-mono">({lvl.sym})</span>
                        {isActive && <span className="text-[8px] font-mono uppercase px-1 rounded bg-purple-500/20 text-purple-300">Active</span>}
                      </div>
                      <div className="flex items-center space-x-3">
                        <span className="text-purple-400 font-mono font-bold text-[11px]">↑↓</span>
                        <span className="font-mono text-[10px] text-white/50">{lvl.energy.toFixed(3)}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ) : (
            /* Occupancy & Active Space Configuration Mode */
            <div className="flex-1 flex flex-col justify-around py-2 space-y-3 font-mono text-xs">
              <div className="p-3 rounded-xl bg-purple-950/30 border border-purple-500/30 space-y-1.5">
                <div className="text-[11px] text-purple-300 font-semibold flex items-center justify-between">
                  <span>Active Space CASSCF({activeElectrons}e, {activeOrbitals}o)</span>
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-purple-500/20 text-purple-200">Correlated</span>
                </div>
                <p className="text-[11px] text-slate-300 font-sans leading-relaxed">
                  {activeElectrons} valence electrons correlated across {activeOrbitals} active spatial orbitals ($S_z = 0$).
                </p>
              </div>

              <div className="grid grid-cols-2 gap-2 text-[11px]">
                <div className="p-2.5 rounded-lg bg-black/40 border border-white/[0.06]">
                  <span className="text-slate-400 block text-[10px]">Frozen Core</span>
                  <span className="text-white font-bold">{Math.max(0, totalE - activeElectrons)} e⁻ / {Math.max(0, (totalE - activeElectrons) / 2)} orb</span>
                </div>
                <div className="p-2.5 rounded-lg bg-black/40 border border-white/[0.06]">
                  <span className="text-slate-400 block text-[10px]">Virtual Space</span>
                  <span className="text-white font-bold">{Math.max(0, totalO - activeOrbitals - Math.max(0, (totalE - activeElectrons) / 2))} orb</span>
                </div>
              </div>

              <div className="p-2.5 rounded-lg bg-black/40 border border-white/[0.06] text-[10px] text-slate-400 flex items-center justify-between">
                <span>Multi-Reference Weight:</span>
                <span className="text-emerald-400 font-semibold">|c₀|² = 0.942 (RHF dominant)</span>
              </div>
            </div>
          )}
        </div>

        {/* CASSCF Sliders & Active Space Configuration */}
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4 flex flex-col justify-between space-y-4">
          <div className="space-y-3">
            <div className="text-xs font-semibold text-white/80 flex items-center space-x-1.5">
              <Sliders className="w-3.5 h-3.5 text-purple-400" />
              <span>CASSCF Active Space Partitioning</span>
            </div>
            <div>
              <div className="flex justify-between text-xs font-mono text-white/60 mb-1">
                <span>Active Electrons (n_e):</span>
                <span className="text-purple-300 font-bold">{activeElectrons}</span>
              </div>
              <input
                type="range"
                aria-label="Active electrons"
                min="2"
                max={Math.max(4, totalE)}
                step="2"
                value={activeElectrons}
                onChange={handleElectronsChange}
                disabled={!backed}
                title={backed ? undefined : 'Disabled — no live backing (layout-only demo)'}
                className="w-full accent-purple-500 bg-white/10 rounded-lg h-1.5 cursor-pointer disabled:opacity-40"
              />
            </div>
            <div>
              <div className="flex justify-between text-xs font-mono text-white/60 mb-1">
                <span>Active Orbitals (n_o):</span>
                <span className="text-purple-300 font-bold">{activeOrbitals}</span>
              </div>
              <input
                type="range"
                aria-label="Active orbitals"
                min="2"
                max={Math.max(4, totalO)}
                step="1"
                value={activeOrbitals}
                onChange={handleOrbitalsChange}
                disabled={!backed}
                title={backed ? undefined : 'Disabled — no live backing (layout-only demo)'}
                className="w-full accent-purple-500 bg-white/10 rounded-lg h-1.5 cursor-pointer disabled:opacity-40"
              />
            </div>
          </div>

          <div className="p-3 rounded-lg bg-black/40 border border-white/[0.04] text-xs space-y-1.5">
            <div className="flex justify-between text-white/60">
              <span>Total Electrons / Orbitals:</span>
              <span className="font-mono text-white/80">{totalE} e / {totalO} o{!backed ? ' (illustrative)' : ''}</span>
            </div>
            <div className="flex justify-between text-white/60">
              <span>Frozen Core Orbitals:</span>
              <span className="font-mono text-white/80">{Math.max(0, (totalE - activeElectrons) / 2)}</span>
            </div>
            <div className="flex justify-between text-white/60">
              <span>Virtual Uncorrelated:</span>
              <span className="font-mono text-white/80">{Math.max(0, totalO - activeOrbitals - Math.max(0, (totalE - activeElectrons) / 2))}</span>
            </div>
            <div className="flex justify-between text-white/60 pt-1 border-t border-white/[0.04]">
              <span>Triage Complexity:</span>
              <span className="font-mono text-amber-400">
                {activeOrbitals > 16 ? 'Multi-Reference QKSD Target' : 'Classical Slurm DMRG'}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
