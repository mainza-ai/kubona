'use client';

import React, { useState, useEffect, useRef } from 'react';
import { X, Sparkles, DollarSign } from 'lucide-react';
import { useFocusTrap } from '@/lib/useFocusTrap';

interface NewCampaignModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreate: (params: {
    title: string;
    targetMolecule: string;
    spendLimitUSD: number;
    fresh: boolean;
  }) => Promise<void>;
}

interface MoleculePreset {
  id: string;
  symbol: string;
  name: string;
  qubits: number;
  bondDist: string;
  defaultTitle: string;
  description: string;
}

const PRESETS: MoleculePreset[] = [
  {
    id: 'H2',
    symbol: 'H₂',
    name: 'Hydrogen Molecule',
    qubits: 2,
    bondDist: '0.7414 Å',
    defaultTitle: 'H2 Ground State Potential Energy Surface',
    description: 'Minimal 2-qubit benchmark for variational quantum eigensolver verification.',
  },
  {
    id: 'LiH',
    symbol: 'LiH',
    name: 'Lithium Hydride',
    qubits: 4,
    bondDist: '1.5957 Å',
    defaultTitle: 'Lithium Hydride Ground State Dissociation',
    description: '4-qubit heteronuclear diatomic molecule with non-trivial ionic bonding character.',
  },
  {
    id: 'BeH2',
    symbol: 'BeH₂',
    name: 'Beryllium Hydride',
    qubits: 6,
    bondDist: '1.3300 Å',
    defaultTitle: 'BeH2 Linear Potential Energy Surface',
    description: '6-qubit linear polyatomic molecule testing active-space reduction and depth pruning.',
  },
  {
    id: 'H2O',
    symbol: 'H₂O',
    name: 'Water Molecule',
    qubits: 8,
    bondDist: '0.9575 Å',
    defaultTitle: 'H2O Asymmetric Stretch Active Space',
    description: '8-qubit bent triatomic system with strong electron correlation in the valence shell.',
  },
  {
    id: 'N2',
    symbol: 'N₂',
    name: 'Dinitrogen',
    qubits: 6,
    bondDist: '1.0977 Å',
    defaultTitle: 'N2 Triple Bond Dissociation VQE',
    description: '6-qubit strongly correlated triple bond dissociation challenge for quantum chemistry.',
  },
  {
    id: 'CO',
    symbol: 'CO',
    name: 'Carbon Monoxide',
    qubits: 6,
    bondDist: '1.1283 Å',
    defaultTitle: 'CO Dipole Ground State Optimization',
    description: '6-qubit polar molecule requiring precise multi-reference electron correlation.',
  },
];

export function NewCampaignModal({ isOpen, onClose, onCreate }: NewCampaignModalProps) {
  const [selectedMolecule, setSelectedMolecule] = useState<string>('H2');
  const [customMolecule, setCustomMolecule] = useState<string>('');
  const [title, setTitle] = useState<string>('H2 Ground State Potential Energy Surface');
  const [spendLimit, setSpendLimit] = useState<number>(100);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  // Inline validation state: invalid input blocks onCreate (never a silent
  // fallback to the preset), and a server/transport refusal renders verbatim
  // with the modal kept OPEN.
  const [titleError, setTitleError] = useState<string | null>(null);
  const [moleculeError, setMoleculeError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(isOpen, modalRef);

  // Auto-update title when selecting preset if title was defaulted
  const handleSelectPreset = (preset: MoleculePreset) => {
    setSelectedMolecule(preset.id);
    setCustomMolecule('');
    setMoleculeError(null);
    setTitle(preset.defaultTitle);
    setTitleError(null);
  };

  // Inline validators (Wave-2): trimmed title >= 3 chars; a non-empty custom
  // molecule must look like a chemical formula or SMILES. An invalid custom
  // value is an error next to the field — never a silent fallback to the
  // selected preset.
  const validateTitle = (v: string): string | null =>
    v.trim().length >= 3 ? null : 'Campaign name must be at least 3 characters (leading/trailing spaces ignored).';

  const validateCustomMolecule = (v: string): string | null => {
    const t = v.trim();
    if (t === '') return null;
    if (t.length > 200) {
      return `Custom molecule is ${t.length} characters (max 200) — shorten it or clear the field to use the preset.`;
    }
    if (!/[A-Za-z]/.test(t)) {
      return `Custom molecule "${t}" must contain an element symbol — fix it or clear the field to use the preset.`;
    }
    if (!/^[A-Za-z0-9@+\-\[\]\(\)=#$%\/\\.:*]+$/.test(t)) {
      return `Custom molecule "${t}" is not a valid formula/SMILES (no spaces; allowed: letters, digits, []()=#+@/%\\.:*-) — fix it or clear the field to use the preset.`;
    }
    return null;
  };

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isSubmitting) return;
    // Inline gate: refuse to send while a field is invalid. No silent
    // fallback — an invalid custom molecule never degrades to the preset.
    const tErr = validateTitle(title);
    const mErr = validateCustomMolecule(customMolecule);
    setTitleError(tErr);
    setMoleculeError(mErr);
    if (tErr || mErr) return;
    const finalMolecule = customMolecule.trim() ? customMolecule.trim() : selectedMolecule;
    const finalTitle = title.trim();

    setIsSubmitting(true);
    setSubmitError(null);
    try {
      await onCreate({
        title: finalTitle,
        targetMolecule: finalMolecule,
        spendLimitUSD: spendLimit,
        fresh: true,
      });
      onClose();
    } catch (err) {
      // Server/transport refusal: keep the modal OPEN and show it verbatim.
      setSubmitError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md transition-opacity duration-200"
      onClick={(e) => {
        if (e.target === e.currentTarget && !isSubmitting) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="new-campaign-title"
    >
      <div
        ref={modalRef}
        className="w-full max-w-2xl bg-[var(--color-float)] border border-[var(--color-border-strong)] rounded-3xl shadow-[var(--shadow-float)] overflow-hidden transform transition-all animate-in fade-in zoom-in-95 duration-150 backdrop-blur-2xl flex flex-col max-h-[90vh]"
      >
        {/* Apple-style Window Header */}
        <div className="h-11 px-5 border-b border-[var(--color-border)] bg-[var(--color-surface)] flex items-center justify-between select-none shrink-0">
          <div className="flex items-center space-x-2">
            <button
              onClick={onClose}
              disabled={isSubmitting}
              aria-label="Close dialog"
              className="w-3 h-3 rounded-full bg-[#FF5F56] hover:brightness-110 active:brightness-90 transition-all flex items-center justify-center group disabled:opacity-50"
            >
              <X className="w-2 h-2 text-black/70 opacity-0 group-hover:opacity-100 transition-opacity" />
            </button>
            <div className="w-3 h-3 rounded-full bg-[#FFBD2E] opacity-75" />
            <div className="w-3 h-3 rounded-full bg-[#27C93F] opacity-75" />
          </div>
          <span className="text-xs font-semibold text-[var(--color-label-secondary)] font-sans tracking-wide flex items-center space-x-1.5">
            <Sparkles className="w-3.5 h-3.5 text-[var(--color-accent-ink)]" />
            <span>New Scientific Research Campaign</span>
          </span>
          <div className="w-12" />
        </div>

        {/* Modal Form Body */}
        <form onSubmit={handleSubmit} className="p-6 overflow-y-auto space-y-6 flex-1">
          {/* Header Introduction */}
          <div>
            <h2 id="new-campaign-title" className="text-lg font-bold text-[var(--color-text)] tracking-tight">
              Initiate Discovery Exploration
            </h2>
            <p className="text-xs text-[var(--color-muted)] mt-1 leading-relaxed">
              Configure your molecular target, establish classical ground truth anchors, and set sovereign guardrails.
            </p>
          </div>

          {/* Campaign Title / Hypothesis */}
          <div className="space-y-1.5">
            <label htmlFor="new-campaign-title-input" className="block text-xs font-semibold text-[var(--color-text)]">
              Research Campaign Name / Hypothesis
            </label>
            <input
              id="new-campaign-title-input"
              type="text"
              value={title}
              aria-label="Research campaign name or hypothesis"
              aria-invalid={titleError !== null}
              aria-describedby={titleError ? 'new-campaign-title-error' : undefined}
              onChange={(e) => {
                setTitle(e.target.value);
                if (titleError) setTitleError(validateTitle(e.target.value));
              }}
              placeholder="e.g., Lithium Hydride Ground State Dissociation"
              className="w-full px-3.5 py-2.5 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-accent)] focus:ring-1 focus:ring-[var(--color-accent-edge)] transition-all font-sans"
            />
            {titleError && (
              <p id="new-campaign-title-error" role="alert" data-testid="campaign-title-error" className="text-[12px] text-[var(--color-danger-ink)] font-sans">
                {titleError}
              </p>
            )}
          </div>

          {/* Molecule Target Presets */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="block text-xs font-semibold text-[var(--color-text)]">
                Target Chemical System
              </label>
              <span className="text-[12px] text-[var(--color-muted)] font-mono">
                PySCF ab-initio anchor available on run
              </span>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2.5">
              {PRESETS.map((p) => {
                const isSelected = selectedMolecule === p.id && !customMolecule;
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => handleSelectPreset(p)}
                    className={`p-3 rounded-2xl border text-left transition-all flex flex-col justify-between space-y-1 relative group ${
                      isSelected
                        ? 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] ring-1 ring-[var(--color-info-edge)] text-[var(--color-text)] shadow-sm'
                        : 'bg-[var(--color-card)] hover:bg-[var(--color-fill-hover)] border-[var(--color-border)] hover:border-[var(--color-border-strong)] text-[var(--color-label-secondary)]'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-bold font-mono tracking-tight text-[var(--color-text)]">
                        {p.symbol}
                      </span>
                      <span className="text-[12px] font-mono px-1.5 py-0.2 rounded bg-[var(--color-fill)] text-[var(--color-info-ink)]">
                        {p.qubits}q
                      </span>
                    </div>
                    <div className="text-[12px] font-medium text-[var(--color-label-secondary)] truncate">
                      {p.name}
                    </div>
                    <div className="text-[12px] text-[var(--color-muted)] font-mono">
                      R = {p.bondDist}
                    </div>
                  </button>
                );
              })}
            </div>

            {/* Custom Molecule Input */}
            <div className="pt-1">
              <label htmlFor="new-campaign-molecule-input" className="sr-only">
                Custom chemical formula or SMILES
              </label>
              <input
                id="new-campaign-molecule-input"
                type="text"
                value={customMolecule}
                aria-label="Custom chemical formula or SMILES"
                aria-invalid={moleculeError !== null}
                aria-describedby={moleculeError ? 'new-campaign-molecule-error' : undefined}
                onChange={(e) => {
                  setCustomMolecule(e.target.value);
                  setMoleculeError(validateCustomMolecule(e.target.value));
                  if (e.target.value.trim()) {
                    setTitle(`${e.target.value.trim()} Ground State Optimization`);
                    setTitleError(null);
                  }
                }}
                placeholder="Or specify custom chemical formula or SMILES (e.g. HF, CH4, Caffeine)..."
                className="w-full px-3.5 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-accent)] transition-all font-mono"
              />
              {moleculeError && (
                <p id="new-campaign-molecule-error" role="alert" data-testid="campaign-molecule-error" className="mt-1 text-[12px] text-[var(--color-danger-ink)] font-sans">
                  {moleculeError}
                </p>
              )}
            </div>
          </div>

          {/* Spend Budget Ceiling */}
          <div className="p-4 rounded-2xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-2">
            <div className="flex justify-between items-center text-xs">
              <span className="font-semibold text-[var(--color-text)] flex items-center space-x-1.5">
                <DollarSign className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
                <span>Spend Limit Mandate</span>
              </span>
              <span className="font-mono text-[var(--color-success-ink)] font-bold text-xs">
                ${spendLimit.toFixed(2)} Maximum
              </span>
            </div>

            <input
              type="range"
              min="25"
              max="250"
              step="25"
              value={spendLimit}
              onChange={(e) => setSpendLimit(parseFloat(e.target.value))}
              className="w-full h-2 bg-[var(--color-fill)] rounded-lg appearance-none cursor-pointer accent-[var(--color-success)] focus:outline-none"
            />

            <div className="flex justify-between text-[12px] text-[var(--color-muted)] font-mono">
              <span>$25.00</span>
              <span>$100.00 (Standard)</span>
              <span>$250.00</span>
            </div>
          </div>

          {/* Server refusal (verbatim, modal stays open) */}
          {submitError && (
            <div role="alert" data-testid="campaign-create-error" className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs">
              Campaign creation refused — nothing created ({submitError}).
            </div>
          )}

          {/* Form Actions Footer */}
          <div className="pt-2 border-t border-[var(--color-border)] flex items-center justify-between">
            <button
              type="button"
              onClick={onClose}
              disabled={isSubmitting}
              className="px-4 py-2 rounded-xl text-xs font-medium text-[var(--color-label-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-all disabled:opacity-50"
            >
              Cancel
            </button>

            <button
              type="submit"
              disabled={isSubmitting}
              className="px-5 py-2.5 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold flex items-center space-x-2 transition-all shadow-lg active:scale-95 disabled:opacity-50"
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span>{isSubmitting ? 'Creating Campaign...' : 'Create Research Campaign'}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
