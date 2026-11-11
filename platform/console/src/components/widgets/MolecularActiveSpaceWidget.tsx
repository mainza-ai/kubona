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
      <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
        <div className="flex items-center space-x-3 border-b border-[var(--color-border)] pb-3">
          <div className="p-2 rounded-xl bg-[var(--color-soft-lilac-wash)] border border-[var(--color-soft-lilac-edge)] text-[var(--color-soft-lilac-ink)]">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-[var(--color-text)]">Molecular Active Space</h3>
            <p className="text-xs text-[var(--color-muted)]">Orbital and electron counts come from the campaign proposal.</p>
          </div>
        </div>
        <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-[var(--color-text)]">No active-space counts are recorded for this campaign.</p>
          <p className="text-[12px] text-[var(--color-muted)]">
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
    <div data-provenance="proposed" className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--color-border)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-soft-lilac-wash)] border border-[var(--color-soft-lilac-edge)] text-[var(--color-soft-lilac-ink)]">
            <Atom className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-[var(--color-text)]">Molecular Active Space</h3>
            <p className="text-xs text-[var(--color-muted)]">{`${moleculeName} • CASSCF(${activeElectrons}e, ${activeOrbitals}o)`}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {onOpenObservatory && (
            <button
              type="button"
              onClick={onOpenObservatory}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-[var(--color-fill)] border border-[var(--color-border)] hover:border-[var(--color-accent-edge)] hover:bg-[var(--color-fill-hover)] text-xs font-medium text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-colors focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:outline-none"
            >
              <Box className="w-3.5 h-3.5 text-[var(--color-accent-ink)]" />
              <span>Open 3D Observatory</span>
            </button>
          )}
          <ProvenanceBadge kind="proposed" detail="Active-space counts come from the campaign proposal." />
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-4 space-y-3 font-mono text-xs">
          <div className="flex justify-between text-[var(--color-muted)]">
            <span>Total electrons / orbitals</span>
            <span data-metric className="text-[var(--color-text)]">{totalE} e / {totalO} o</span>
          </div>
          <div className="flex justify-between text-[var(--color-muted)]">
            <span>Active space</span>
            <span data-metric className="text-[var(--color-soft-lilac-ink)]">{activeElectrons} e / {activeOrbitals} o</span>
          </div>
          <div className="flex justify-between text-[var(--color-muted)]">
            <span>Frozen core</span>
            <span data-metric className="text-[var(--color-text)]">{frozenElectrons} e / {frozenOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-[var(--color-muted)]">
            <span>Virtual uncorrelated</span>
            <span data-metric className="text-[var(--color-text)]">{virtualOrbitals} orb</span>
          </div>
          <div className="flex justify-between text-[var(--color-muted)] pt-2 border-t border-[var(--color-border)]">
            <span>HOMO-LUMO gap</span>
            <span data-metric className="text-[var(--color-success-ink)]">{gap != null ? `${gap.toFixed(3)} Ha` : 'not recorded'}</span>
          </div>
          <p className="text-[12px] text-[var(--color-muted)] font-sans leading-relaxed pt-1">
            Per-orbital energies are not shown because the campaign records no orbital spectrum. Counts above are proposal fields, not a measurement.
          </p>
        </div>

        <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-4 flex flex-col justify-between space-y-4">
          <div className="space-y-3">
            <div className="text-xs font-semibold text-[var(--color-text)] flex items-center space-x-1.5">
              <Sliders className="w-3.5 h-3.5 text-[var(--color-soft-lilac-ink)]" />
              <span>CASSCF Active-Space Partitioning</span>
            </div>
            <div>
              <div className="flex justify-between text-xs font-mono text-[var(--color-muted)] mb-1">
                <span>Active Electrons (n_e):</span>
                <span className="text-[var(--color-soft-lilac-ink)] font-bold">{activeElectrons}</span>
              </div>
              <input
                type="range"
                aria-label="Active electrons"
                min="2"
                max={Math.max(4, totalE)}
                step="2"
                value={activeElectrons}
                onChange={handleElectronsChange}
                className="w-full accent-[var(--color-accent)] bg-[var(--color-fill)] rounded-lg h-1.5 cursor-pointer"
              />
            </div>
            <div>
              <div className="flex justify-between text-xs font-mono text-[var(--color-muted)] mb-1">
                <span>Active Orbitals (n_o):</span>
                <span className="text-[var(--color-soft-lilac-ink)] font-bold">{activeOrbitals}</span>
              </div>
              <input
                type="range"
                aria-label="Active orbitals"
                min="2"
                max={Math.max(4, totalO)}
                step="1"
                value={activeOrbitals}
                onChange={handleOrbitalsChange}
                className="w-full accent-[var(--color-accent)] bg-[var(--color-fill)] rounded-lg h-1.5 cursor-pointer"
              />
            </div>
          </div>
          <p className="text-[12px] text-[var(--color-muted)] leading-relaxed">
            Slider changes update the proposal only. They do not imply a CASSCF run or a measured active space.
          </p>
        </div>
      </div>
    </div>
  );
}
