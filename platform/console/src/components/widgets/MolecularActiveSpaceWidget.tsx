'use client';

import React, { useState } from 'react';
import { Atom, Box, Sliders } from 'lucide-react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

interface MolecularActiveSpaceWidgetProps {
  moleculeName?: string;
  numElectrons?: number;
  numOrbitals?: number;
  homoLumoGapHa?: number;
  onActiveSpaceChange?: (activeElectrons: number, activeOrbitals: number) => void;
  onOpenObservatory?: () => void;
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
  onOpenObservatory,
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
      <div className="rounded-card border border-border bg-surface p-5 text-text space-y-4">
        <div className="flex items-center space-x-3 border-b border-border pb-3">
          <div className="p-2 rounded-row bg-soft-lilac-wash border border-soft-lilac-edge text-soft-lilac-ink">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-body-strong font-semibold text-text">Molecular Active Space</h3>
            <p className="text-label text-muted">Orbital and electron counts come from the campaign proposal.</p>
          </div>
        </div>
        <div className="rounded-row border border-border bg-card p-5 text-center space-y-1.5">
          <p className="text-label font-medium text-text">No active-space counts are recorded for this campaign.</p>
          <p className="text-[12px] text-muted">
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
    <div data-provenance="proposed" className="rounded-card border border-border bg-surface p-5 text-text space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-row bg-soft-lilac-wash border border-soft-lilac-edge text-soft-lilac-ink">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-body-strong font-semibold text-text">Molecular Active Space</h3>
            <p className="text-label text-muted">{`${moleculeName} • CASSCF(${activeElectrons}e, ${activeOrbitals}o)`}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {onOpenObservatory && (
            <button
              type="button"
              onClick={onOpenObservatory}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-capsule bg-fill border border-border hover:border-accent-edge hover:bg-fill-hover text-label font-medium text-label-secondary hover:text-text transition-colors focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
            >
              <Box className="w-3.5 h-3.5 text-accent-ink" />
              <span>Open 3D Observatory</span>
            </button>
          )}
          <ProvenanceBadge kind="proposed" detail="Active-space counts come from the campaign proposal." />
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="rounded-row border border-border bg-card p-4 space-y-3 font-mono text-label">
          <div className="flex justify-between text-muted">
            <span>Total electrons / orbitals</span>
            <span data-metric className="text-text">{totalE} e / {totalO} o</span>
          </div>
          <div className="flex justify-between text-muted">
            <span>Active space</span>
            <span data-metric className="text-soft-lilac-ink">{activeElectrons} e / {activeOrbitals} o</span>
          </div>
          <div className="flex justify-between text-muted">
            <span>Frozen core</span>
            <span data-metric className="text-text">{frozenElectrons} e / {frozenOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-muted">
            <span>Virtual uncorrelated</span>
            <span data-metric className="text-text">{virtualOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-muted pt-2 border-t border-border">
            <span>HOMO-LUMO gap</span>
            <span data-metric className="text-success-ink">{gap != null ? `${gap.toFixed(3)} Ha` : 'not recorded'}</span>
          </div>
          <p className="text-[12px] text-muted font-sans leading-relaxed pt-1">
            Per-orbital energies are not shown because the campaign records no orbital spectrum. Counts above are proposal fields, not a measurement.
          </p>
        </div>

        <div className="rounded-row border border-border bg-card p-4 flex flex-col justify-between space-y-4">
          <div className="space-y-3">
            <div className="text-label font-semibold text-text flex items-center space-x-1.5">
              <Sliders className="w-3.5 h-3.5 text-soft-lilac-ink" />
              <span>CASSCF Active-Space Partitioning</span>
            </div>
            <div>
              <div className="flex justify-between text-label font-mono text-muted mb-1">
                <span>Active Electrons (n_e):</span>
                <span className="text-soft-lilac-ink font-bold">{activeElectrons}</span>
              </div>
              <input
                type="range"
                aria-label="Active electrons"
                min="2"
                max={Math.max(4, totalE)}
                step="2"
                value={activeElectrons}
                onChange={handleElectronsChange}
                className="w-full accent-accent bg-fill rounded-control-compact h-1.5 cursor-pointer"
              />
            </div>
            <div>
              <div className="flex justify-between text-label font-mono text-muted mb-1">
                <span>Active Orbitals (n_o):</span>
                <span className="text-soft-lilac-ink font-bold">{activeOrbitals}</span>
              </div>
              <input
                type="range"
                aria-label="Active orbitals"
                min="2"
                max={Math.max(4, totalO)}
                step="1"
                value={activeOrbitals}
                onChange={handleOrbitalsChange}
                className="w-full accent-accent bg-fill rounded-control-compact h-1.5 cursor-pointer"
              />
            </div>
          </div>
          <p className="text-[12px] text-muted leading-relaxed">
            Slider changes update the proposal only. They do not imply a CASSCF run or a measured active space.
          </p>
        </div>
      </div>
    </div>
  );
}
