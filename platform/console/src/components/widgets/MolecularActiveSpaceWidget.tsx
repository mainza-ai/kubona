'use client';

import React, { useState } from 'react';
import { Atom, Sliders } from 'lucide-react';

interface MolecularActiveSpaceWidgetProps {
  moleculeName?: string;
  numElectrons?: number;
  numOrbitals?: number;
  homoLumoGapHa?: number;
  onActiveSpaceChange?: (activeElectrons: number, activeOrbitals: number) => void;
}

export function MolecularActiveSpaceWidget({
  moleculeName = 'FeMoco (Nitrogenase [Fe7MoS9C])',
  numElectrons = 54,
  numOrbitals = 54,
  homoLumoGapHa = 0.042,
  onActiveSpaceChange,
}: MolecularActiveSpaceWidgetProps) {
  const [activeElectrons, setActiveElectrons] = useState(10);
  const [activeOrbitals, setActiveOrbitals] = useState(10);
  const [selectedOrbital, setSelectedOrbital] = useState<number>(5);
  const [viewState, setViewState] = useState<'orbitals' | 'electrons' | 'density'>('orbitals');

  const orbitalLevels = [
    { idx: 1, name: 'HOMO-4', energy: -0.485, occ: 2, sym: 'a1g', isCore: true },
    { idx: 2, name: 'HOMO-3', energy: -0.412, occ: 2, sym: 't1u', isCore: true },
    { idx: 3, name: 'HOMO-2', energy: -0.365, occ: 2, sym: 'eg', isCore: false },
    { idx: 4, name: 'HOMO-1', energy: -0.320, occ: 2, sym: 't2g', isCore: false },
    { idx: 5, name: 'HOMO', energy: -0.280, occ: 2, sym: 't2g', isCore: false },
    { idx: 6, name: 'LUMO', energy: -0.238, occ: 0, sym: 'eg*', isCore: false },
    { idx: 7, name: 'LUMO+1', energy: -0.190, occ: 0, sym: 't1u*', isCore: false },
    { idx: 8, name: 'LUMO+2', energy: -0.142, occ: 0, sym: 'a1g*', isCore: true },
  ];

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
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">3D Molecular Active Space Viewer (A2UI)<span className="ml-2 align-middle text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">Illustrative</span></h3>
            <p className="text-xs text-white/50">{moleculeName} • CASSCF({activeElectrons}e, {activeOrbitals}o)</p>
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
        {/* Orbital Energy Ladder */}
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4 flex flex-col justify-between h-64">
          <div className="text-xs font-mono text-white/60 flex justify-between items-center mb-2">
            <span>Orbital Energy Level (Ha)</span>
            <span className="text-emerald-400">Gap: {homoLumoGapHa} Ha ({Math.round(homoLumoGapHa * 27.2114 * 100) / 100} eV)</span>
          </div>
          <div className="flex-1 flex flex-col-reverse justify-around relative py-2">
            {/* Visual HOMO-LUMO Gap line */}
            <div className="absolute top-[48%] left-0 right-0 border-b border-dashed border-emerald-500/40 flex justify-between text-[10px] text-emerald-400 px-2 font-mono">
              <span>Fermi Level E_F</span>
              <span>HOMO-LUMO Gap</span>
            </div>
            {orbitalLevels.map((lvl) => {
              const isSelected = selectedOrbital === lvl.idx;
              const isActive = !lvl.isCore;
              return (
                <div
                  key={lvl.idx}
                  onClick={() => setSelectedOrbital(lvl.idx)}
                  className={`flex items-center justify-between text-xs px-2 py-1 rounded cursor-pointer transition ${
                    isSelected
                      ? 'bg-purple-500/20 border border-purple-500/40 text-purple-200'
                      : isActive
                      ? 'hover:bg-white/[0.04] text-white/80'
                      : 'text-white/40'
                  }`}
                >
                  <div className="flex items-center space-x-2">
                    <span className="font-mono text-[10px] w-14">{lvl.name}</span>
                    <span className="text-[10px] text-white/40 font-mono">({lvl.sym})</span>
                  </div>
                  <div className="flex items-center space-x-3">
                    <div className="flex space-x-0.5">
                      {lvl.occ === 2 && (
                        <span className="text-purple-400 font-mono font-bold text-[11px]">↑↓</span>
                      )}
                      {lvl.occ === 1 && (
                        <span className="text-purple-400 font-mono font-bold text-[11px]">↑</span>
                      )}
                      {lvl.occ === 0 && (
                        <span className="text-white/20 font-mono text-[11px]">—</span>
                      )}
                    </div>
                    <span className="font-mono text-[10px] text-white/50">{lvl.energy.toFixed(3)}</span>
                  </div>
                </div>
              );
            })}
          </div>
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
                min="2"
                max="24"
                step="2"
                value={activeElectrons}
                onChange={handleElectronsChange}
                className="w-full accent-purple-500 bg-white/10 rounded-lg h-1.5 cursor-pointer"
              />
            </div>
            <div>
              <div className="flex justify-between text-xs font-mono text-white/60 mb-1">
                <span>Active Orbitals (n_o):</span>
                <span className="text-purple-300 font-bold">{activeOrbitals}</span>
              </div>
              <input
                type="range"
                min="2"
                max="24"
                step="1"
                value={activeOrbitals}
                onChange={handleOrbitalsChange}
                className="w-full accent-purple-500 bg-white/10 rounded-lg h-1.5 cursor-pointer"
              />
            </div>
          </div>

          <div className="p-3 rounded-lg bg-black/40 border border-white/[0.04] text-xs space-y-1.5">
            <div className="flex justify-between text-white/60">
              <span>Total Electrons / Orbitals:</span>
              <span className="font-mono text-white/80">{numElectrons} e / {numOrbitals} o</span>
            </div>
            <div className="flex justify-between text-white/60">
              <span>Frozen Core Orbitals:</span>
              <span className="font-mono text-white/80">{Math.max(0, (numElectrons - activeElectrons) / 2)}</span>
            </div>
            <div className="flex justify-between text-white/60">
              <span>Virtual Uncorrelated:</span>
              <span className="font-mono text-white/80">{Math.max(0, numOrbitals - activeOrbitals - Math.max(0, (numElectrons - activeElectrons) / 2))}</span>
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
