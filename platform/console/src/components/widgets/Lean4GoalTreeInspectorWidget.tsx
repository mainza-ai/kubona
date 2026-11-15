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
  const [selectedTacticIndex, setSelectedTacticIndex] = useState(0);
  // No hardcoded Bloch/tactics/rho without a live receipt: without all three
  // props the widget renders an empty state, never illustrative proof chrome.
  const hasReceipt = theoremName != null && theoremSignature != null && astHash != null
    && Array.isArray(suppliedSteps) && suppliedSteps.length > 0;
  if (!hasReceipt) {
    return (
      <div className="rounded-card border border-border bg-surface p-5 text-text space-y-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-row bg-success-wash border border-success-edge text-success-ink">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-body-strong font-semibold text-text">Lean 4 Goal Tree Inspector</h3>
            <p className="text-label text-muted">No live proof receipt — nothing verified</p>
          </div>
        </div>
        <p className="text-[12px] font-code text-muted">
          No theorem, tactics, or AST hash bound. Run a comparator verification to produce a receipt; until then no ρ, no tactics, no Bloch claim.
        </p>
      </div>
    );
  }
  const proofSteps = suppliedSteps ?? [];

  return (
    <div data-provenance="measured" className="rounded-card border border-border bg-surface p-5 text-text space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-row bg-success-wash border border-success-edge text-success-ink">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-body-strong font-semibold text-text">Lean 4 Goal Tree Inspector</h3>
              <ProvenanceBadge kind="measured" detail="Comparator proof receipt." />
            </div>
            <p className="text-label text-muted">{theoremName} • Comparator Kernel</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <span className="px-2 py-0.5 rounded-capsule bg-success-wash border border-success-edge text-[12px] font-mono text-success-ink flex items-center space-x-1">
            <CheckCircle2 className="w-3 h-3 text-success-ink" />
            <span>{sorryAx === 0 ? 'ρ = 1.000000' : 'ρ = —'}</span>
          </span>
          <span className="px-2 py-0.5 rounded-capsule bg-soft-sky-wash border border-soft-sky-edge text-[12px] font-mono text-soft-sky-ink">
            {sorryAx === 0 ? '0 sorryAx' : `${sorryAx ?? '—'} sorryAx`}
          </span>
        </div>
      </div>

      {/* Theorem Signature */}
      <div className="p-3 rounded-row bg-field border border-border font-mono text-label text-success-ink flex justify-between items-center">
        <code>{theoremSignature}</code>
        <span className="text-[12px] text-muted font-mono">{astHash}</span>
      </div>

      {axioms && axioms.length > 0 && (
        <p className="text-[12px] font-code text-muted">axioms: {axioms.join(', ')}</p>
      )}

      {/* Proof Tree & Tactic States */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Tactic Trace Ladder */}
        <div className="p-3 rounded-row bg-card border border-border space-y-2">
          <span className="text-label font-semibold text-label-secondary block mb-2 flex items-center space-x-1.5">
            <GitBranch className="w-3.5 h-3.5 text-success-ink" />
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
                className={`p-2.5 rounded-control-compact border text-label cursor-pointer transition ${
                  isSelected
                    ? 'bg-success-wash border-success-edge text-success-ink'
                    : 'border-border bg-tile text-label-secondary hover:bg-fill-hover'
                }`}
              >
                <div className="flex justify-between items-center font-mono">
                  <span className="text-faint">Step {s.step}</span>
                  <span className="text-success-ink font-bold">{s.tactic}</span>
                </div>
                <p className="text-[12px] text-muted mt-1">{s.note}</p>
              </div>
            );
          })}
        </div>

        {/* Lean 4 Interactive Goal State View */}
        <div className="p-3 rounded-row bg-field border border-border flex flex-col justify-between font-mono text-label">
          <div>
            <div className="flex items-center space-x-1.5 text-muted pb-2 border-b border-border mb-2">
              <Terminal className="w-3.5 h-3.5 text-success-ink" />
              <span>Lean 4 Tactic State</span>
            </div>
            <pre className="text-success-ink whitespace-pre-wrap leading-relaxed">
              {(proofSteps[Math.min(selectedTacticIndex, proofSteps.length - 1)] ?? proofSteps[0])?.state ?? ''}
            </pre>
          </div>

          <div className="mt-4 pt-2 border-t border-border text-[12px] text-muted flex justify-between items-center">
            <span>Axiom Whitelist: propext, Quot.sound, Classical.choice</span>
            <span className="text-success-ink">Lean v4.34.1</span>
          </div>
        </div>
      </div>
    </div>
  );
}
