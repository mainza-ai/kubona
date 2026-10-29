'use client';

import React, { useState, useEffect, useRef } from 'react';
import { X, Sparkles, Atom,  DollarSign, Check } from 'lucide-react';
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
  const [isFresh, setIsFresh] = useState<boolean>(true);
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
        fresh: isFresh,
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
        className="w-full max-w-2xl bg-[#0C0F17]/95 border border-white/10 rounded-3xl shadow-2xl shadow-black/90 overflow-hidden transform transition-all animate-in fade-in zoom-in-95 duration-150 backdrop-blur-2xl flex flex-col max-h-[90vh]"
      >
        {/* Apple-style Window Header */}
        <div className="h-11 px-5 border-b border-white/[0.08] bg-white/[0.02] flex items-center justify-between select-none shrink-0">
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
          <span className="text-xs font-semibold text-slate-300 font-sans tracking-wide flex items-center space-x-1.5">
            <Sparkles className="w-3.5 h-3.5 text-cyan-400" />
            <span>New Scientific Research Campaign</span>
          </span>
          <div className="w-12" />
        </div>

        {/* Modal Form Body */}
        <form onSubmit={handleSubmit} className="p-6 overflow-y-auto space-y-6 flex-1">
          {/* Header Introduction */}
          <div>
            <h2 id="new-campaign-title" className="text-lg font-bold text-white tracking-tight">
              Initiate Discovery Exploration
            </h2>
            <p className="text-xs text-slate-300 mt-1 leading-relaxed">
              Configure your molecular target, establish classical ground truth anchors, and set sovereign guardrails.
            </p>
          </div>

          {/* Campaign Title / Hypothesis */}
          <div className="space-y-1.5">
            <label htmlFor="new-campaign-title-input" className="block text-xs font-semibold text-slate-200">
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
              className="w-full px-3.5 py-2.5 rounded-xl bg-black/60 border border-white/10 text-xs text-white placeholder-slate-400 focus:outline-none focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400/40 transition-all font-sans"
            />
            {titleError && (
              <p id="new-campaign-title-error" role="alert" data-testid="campaign-title-error" className="text-[11px] text-rose-300 font-sans">
                {titleError}
              </p>
            )}
          </div>

          {/* Molecule Target Presets */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="block text-xs font-semibold text-slate-200">
                Target Chemical System
              </label>
              <span className="text-[11px] text-slate-400 font-mono">
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
                        ? 'bg-cyan-950/60 border-cyan-500/60 ring-1 ring-cyan-500/30 text-white shadow-lg'
                        : 'bg-black/40 hover:bg-black/70 border-white/[0.08] hover:border-white/20 text-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-bold font-mono tracking-tight text-white">
                        {p.symbol}
                      </span>
                      <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-white/[0.06] text-cyan-300">
                        {p.qubits}q
                      </span>
                    </div>
                    <div className="text-[11px] font-medium text-slate-300 truncate">
                      {p.name}
                    </div>
                    <div className="text-[10px] text-slate-400 font-mono">
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
                className="w-full px-3.5 py-2 rounded-xl bg-black/40 border border-white/10 text-xs text-slate-200 placeholder-slate-400 focus:outline-none focus:border-cyan-400 transition-all font-mono"
              />
              {moleculeError && (
                <p id="new-campaign-molecule-error" role="alert" data-testid="campaign-molecule-error" className="mt-1 text-[11px] text-rose-300 font-sans">
                  {moleculeError}
                </p>
              )}
            </div>
          </div>

          {/* Initial State Mode: Fresh vs Pre-Computed Benchmark */}
          <div className="space-y-2">
            <label className="block text-xs font-semibold text-slate-200">
              Initial Campaign State
            </label>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {/* Fresh Discovery Canvas */}
              <div
                onClick={() => setIsFresh(true)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setIsFresh(true);
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={isFresh}
                aria-label="Fresh discovery canvas"
                className={`p-3.5 rounded-2xl border cursor-pointer transition-all space-y-1.5 ${
                  isFresh
                    ? 'bg-cyan-950/60 border-cyan-500/60 ring-1 ring-cyan-500/30 text-white'
                    : 'bg-black/40 hover:bg-black/60 border-white/[0.08] text-slate-300'
                }`}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <Atom className="w-4 h-4 text-cyan-400" />
                    <span className="text-xs font-bold text-white">Fresh Discovery Canvas</span>
                  </div>
                  {isFresh && <Check className="w-4 h-4 text-cyan-400" />}
                </div>
                <p className="text-[11px] text-slate-300 leading-relaxed font-sans">
                  Starts with an unrun, clean canvas. Awaiting user hypothesis and Research Tribunal formulation.
                </p>
                <span className="inline-block text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full bg-cyan-900/40 text-cyan-300 border border-cyan-700/40">
                  Recommended
                </span>
              </div>

              {/* Synthesize Benchmark Reference */}
              <div
                onClick={() => setIsFresh(false)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setIsFresh(false);
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={!isFresh}
                aria-label="Synthesize reference baseline"
                className={`p-3.5 rounded-2xl border cursor-pointer transition-all space-y-1.5 ${
                  !isFresh
                    ? 'bg-indigo-950/60 border-indigo-500/60 ring-1 ring-indigo-500/30 text-white'
                    : 'bg-black/40 hover:bg-black/60 border-white/[0.08] text-slate-300'
                }`}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <Sparkles className="w-4 h-4 text-indigo-400" />
                    <span className="text-xs font-bold text-white">Synthesize Reference Baseline</span>
                  </div>
                  {!isFresh && <Check className="w-4 h-4 text-indigo-400" />}
                </div>
                <p className="text-[11px] text-slate-300 leading-relaxed font-sans">
                  Pre-computes a full variational trajectory with chemical accuracy convergence proof and notebook.
                </p>
                <span className="inline-block text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full bg-indigo-900/40 text-indigo-300 border border-indigo-700/40">
                  Reference Demo
                </span>
              </div>
            </div>
          </div>

          {/* Spend Budget Ceiling */}
          <div className="p-4 rounded-2xl bg-black/40 border border-white/10 space-y-2">
            <div className="flex justify-between items-center text-xs">
              <span className="font-semibold text-slate-200 flex items-center space-x-1.5">
                <DollarSign className="w-3.5 h-3.5 text-emerald-400" />
                <span>Spend Limit Mandate</span>
              </span>
              <span className="font-mono text-emerald-400 font-bold text-xs">
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
              className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-emerald-400 focus:outline-none"
            />

            <div className="flex justify-between text-[11px] text-slate-400 font-mono">
              <span>$25.00</span>
              <span>$100.00 (Standard)</span>
              <span>$250.00</span>
            </div>
          </div>

          {/* Server refusal (verbatim, modal stays open) */}
          {submitError && (
            <div role="alert" data-testid="campaign-create-error" className="p-3 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-xs">
              Campaign creation refused — nothing created ({submitError}).
            </div>
          )}

          {/* Form Actions Footer */}
          <div className="pt-2 border-t border-white/[0.08] flex items-center justify-between">
            <button
              type="button"
              onClick={onClose}
              disabled={isSubmitting}
              className="px-4 py-2 rounded-xl text-xs font-medium text-slate-300 hover:text-white hover:bg-white/[0.04] transition-all disabled:opacity-50"
            >
              Cancel
            </button>

            <button
              type="submit"
              disabled={isSubmitting}
              className="px-5 py-2.5 rounded-xl bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 text-white text-xs font-semibold flex items-center space-x-2 transition-all shadow-lg active:scale-95 disabled:opacity-50"
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
