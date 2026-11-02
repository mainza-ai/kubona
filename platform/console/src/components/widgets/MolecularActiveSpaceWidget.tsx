'use client';

import React, { useState } from 'react';
import { Atom, Sliders } from 'lucide-react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

interface MolecularActiveSpaceWidgetProps {
  moleculeName?: string;
  numElectrons?: number;
  numOrbitals?: number;
  homoLumoGapHa?: number;
  onActiveSpaceChange?: (activeElectrons: number, activeOrbitals: number) => void;
}

/**
 * Proposal-backed active-space summary. Orbital energies are deliberately
 * absent: the campaign does not record a per-orbital spectrum, and the old
 * generated ladder was a layout sketch, not chemistry.
 */
export function MolecularActiveSpaceWidget({
  moleculeName,
  numElectrons,
  numOrbitals,
  homoLumoGapHa,
  onActiveSpaceChange,
}: MolecularActiveSpaceWidgetProps) {
  const backed =
    typeof moleculeName === 'string' && moleculeName.trim() !== '' &&
    typeof numElectrons === 'number' && Number.isFinite(numElectrons) && numElectrons > 0 &&
    typeof numOrbitals === 'number' && Number.isFinite(numOrbitals) && numOrbitals > 0;

  const totalE = backed ? numElectrons : 0;
  const totalO = backed ? numOrbitals : 0;
  const [activeElectrons, setActiveElectrons] = useState(Math.min(10, totalE));
  const [activeOrbitals, setActiveOrbitals] = useState(Math.min(10, totalO));
  const [prevSpaceKey, setPrevSpaceKey] = useState(`${totalE}:${totalO}`);
  if (prevSpaceKey !== `${totalE}:${totalO}`) {
    setPrevSpaceKey(`${totalE}:${totalO}`);
    setActiveElectrons(Math.min(10, totalE));
    setActiveOrbitals(Math.min(10, totalO));
  }

  const gap = typeof homoLumoGapHa === 'number' && Number.isFinite(homoLumoGapHa) && homoLumoGapHa > 0
    ? homoLumoGapHa
    : null;

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

  if (!backed) {
    return (
      <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 text-white space-y-4">
        <div className="flex items-center space-x-3 border-b border-white/[0.08] pb-3">
          <div className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Molecular Active Space</h3>
            <p className="text-xs text-white/50">Orbital and electron counts come from the campaign proposal.</p>
          </div>
        </div>
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-white/80">No active-space counts are recorded for this campaign.</p>
          <p className="text-[12px] text-slate-400">
            The viewer appears when a campaign states its molecule, electron count, and orbital count; no molecule or gap default is substituted.
          </p>
        </div>
      </div>
    );
  }

  const frozenElectrons = Math.max(0, totalE - activeElectrons);
  const frozenOrbitals = Math.max(0, frozenElectrons / 2);
  const virtualOrbitals = Math.max(0, totalO - activeOrbitals - frozenOrbitals);

  return (
    <div data-provenance="proposed" className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 text-white space-y-4">
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Molecular Active Space</h3>
            <p className="text-xs text-white/50">{`${moleculeName} • CASSCF(${activeElectrons}e, ${activeOrbitals}o)`}</p>
          </div>
        </div>
        <ProvenanceBadge kind="proposed" detail="Active-space counts come from the campaign proposal." />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4 space-y-3 font-mono text-xs">
          <div className="flex justify-between text-white/60">
            <span>Total electrons / orbitals</span>
            <span data-metric className="text-white/80">{totalE} e / {totalO} o</span>
          </div>
          <div className="flex justify-between text-white/60">
            <span>Active space</span>
            <span data-metric className="text-purple-300">{activeElectrons} e / {activeOrbitals} o</span>
          </div>
          <div className="flex justify-between text-white/60">
            <span>Frozen core</span>
            <span data-metric className="text-white/80">{frozenElectrons} e / {frozenOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-white/60">
            <span>Virtual uncorrelated</span>
            <span data-metric className="text-white/80">{virtualOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-white/60 pt-2 border-t border-white/[0.04]">
            <span>HOMO-LUMO gap</span>
            <span data-metric className="text-emerald-300">{gap != null ? `${gap.toFixed(3)} Ha` : 'not recorded'}</span>
          </div>
          <p className="text-[12px] text-slate-400 font-sans leading-relaxed pt-1">
            Per-orbital energies are not shown because the campaign records no orbital spectrum. Counts above are proposal fields, not a measurement.
          </p>
        </div>

        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4 flex flex-col justify-between space-y-4">
          <div className="space-y-3">
            <div className="text-xs font-semibold text-white/80 flex items-center space-x-1.5">
              <Sliders className="w-3.5 h-3.5 text-purple-400" />
              <span>CASSCF Active-Space Partitioning</span>
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
                aria-label="Active orbitals"
                min="2"
                max={Math.max(4, totalO)}
                step="1"
                value={activeOrbitals}
                onChange={handleOrbitalsChange}
                className="w-full accent-purple-500 bg-white/10 rounded-lg h-1.5 cursor-pointer"
              />
            </div>
          </div>
          <p className="text-[12px] text-slate-400 leading-relaxed">
            Slider changes update the proposal only. They do not imply a CASSCF run or a measured active space.
          </p>
        </div>
      </div>
    </div>
  );
}
