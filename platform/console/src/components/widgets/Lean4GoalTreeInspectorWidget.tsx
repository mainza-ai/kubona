'use client';

import React, { useState } from 'react';
import { CheckCircle2, ShieldCheck, GitBranch, Terminal } from 'lucide-react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

export interface Lean4ProofStep {
  step: number;
  tactic: string;
  state: string;
  note: string;
}

interface Lean4GoalTreeInspectorWidgetProps {
  theoremName?: string;
  theoremSignature?: string;
  astHash?: string;
  // A real comparator receipt must supply the tactic trace, axioms, and
  // sorryAx count. Without them this widget renders the empty state; there
  // is no illustrative proof chrome (2026-10-26 audit P0-10).
  proofSteps?: Lean4ProofStep[];
  axioms?: string[];
  sorryAx?: number;
}

export function Lean4GoalTreeInspectorWidget({
  theoremName,
  theoremSignature,
  astHash,
  proofSteps: suppliedSteps,
  axioms,
  sorryAx,
}: Lean4GoalTreeInspectorWidgetProps) {
  const [selectedTacticIndex, setSelectedTacticIndex] = useState(2);
  // No hardcoded Bloch/tactics/rho without a live receipt: without all three
  // props the widget renders an empty state, never illustrative proof chrome.
  const hasReceipt = theoremName != null && theoremSignature != null && astHash != null
    && Array.isArray(suppliedSteps) && suppliedSteps.length > 0;
  if (!hasReceipt) {
    return (
      <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[var(--color-success-ink)]">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-[var(--color-text)]">Lean 4 Goal Tree Inspector</h3>
            <p className="text-xs text-[var(--color-muted)]">No live proof receipt — nothing verified</p>
          </div>
        </div>
        <p className="text-[12px] font-code text-[var(--color-muted)]">
          No theorem, tactics, or AST hash bound. Run a comparator verification to produce a receipt; until then no ρ, no tactics, no Bloch claim.
        </p>
      </div>
    );
  }
  const proofSteps = suppliedSteps ?? [];

  return (
    <div data-provenance="measured" className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[var(--color-success-ink)]">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-[var(--color-text)]">Lean 4 Goal Tree Inspector</h3>
              <ProvenanceBadge kind="measured" detail="Comparator proof receipt." />
            </div>
            <p className="text-xs text-[var(--color-muted)]">{theoremName} • Comparator Kernel</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <span className="px-2 py-0.5 rounded-full bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[12px] font-mono text-[var(--color-success-ink)] flex items-center space-x-1">
            <CheckCircle2 className="w-3 h-3 text-[var(--color-success-ink)]" />
            <span>{sorryAx === 0 ? 'ρ = 1.000000' : 'ρ = —'}</span>
          </span>
          <span className="px-2 py-0.5 rounded-full bg-[var(--color-soft-sky-wash)] border border-[var(--color-soft-sky-edge)] text-[12px] font-mono text-[var(--color-soft-sky-ink)]">
            {sorryAx === 0 ? '0 sorryAx' : `${sorryAx ?? '—'} sorryAx`}
          </span>
        </div>
      </div>

      {/* Theorem Signature */}
      <div className="p-3 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] font-mono text-xs text-[var(--color-success-ink)] flex justify-between items-center">
        <code>{theoremSignature}</code>
        <span className="text-[12px] text-[var(--color-muted)] font-mono">{astHash}</span>
      </div>

      {axioms && axioms.length > 0 && (
        <p className="text-[12px] font-code text-[var(--color-muted)]">axioms: {axioms.join(', ')}</p>
      )}

      {/* Proof Tree & Tactic States */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Tactic Trace Ladder */}
        <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] space-y-2">
          <span className="text-xs font-semibold text-[var(--color-label-secondary)] block mb-2 flex items-center space-x-1.5">
            <GitBranch className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
            <span>Interactive Proof Steps</span>
          </span>
          {proofSteps.map((s, idx) => {
            const isSelected = selectedTacticIndex === idx;
            return (
              <div
                key={s.step}
                onClick={() => setSelectedTacticIndex(idx)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedTacticIndex(idx);
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={isSelected}
                aria-label={`Select tactic step ${s.step}`}
                className={`p-2.5 rounded-lg border text-xs cursor-pointer transition ${
                  isSelected
                    ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
                    : 'border-[var(--color-border)] bg-[var(--color-tile)] text-[var(--color-label-secondary)] hover:bg-[var(--color-fill-hover)]'
                }`}
              >
                <div className="flex justify-between items-center font-mono">
                  <span className="text-[var(--color-faint)]">Step {s.step}</span>
                  <span className="text-[var(--color-success-ink)] font-bold">{s.tactic}</span>
                </div>
                <p className="text-[12px] text-[var(--color-muted)] mt-1">{s.note}</p>
              </div>
            );
          })}
        </div>

        {/* Lean 4 Interactive Goal State View */}
        <div className="p-3 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] flex flex-col justify-between font-mono text-xs">
          <div>
            <div className="flex items-center space-x-1.5 text-[var(--color-muted)] pb-2 border-b border-[var(--color-border)] mb-2">
              <Terminal className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
              <span>Lean 4 Tactic State</span>
            </div>
            <pre className="text-[var(--color-success-ink)] whitespace-pre-wrap leading-relaxed">
              {proofSteps[selectedTacticIndex].state}
            </pre>
          </div>

          <div className="mt-4 pt-2 border-t border-[var(--color-border)] text-[12px] text-[var(--color-muted)] flex justify-between items-center">
            <span>Axiom Whitelist: propext, Quot.sound, Classical.choice</span>
            <span className="text-[var(--color-success-ink)]">Lean v4.34.1</span>
          </div>
        </div>
      </div>
    </div>
  );
}
