'use client';

import React, { useState } from 'react';
import { CheckCircle2, ShieldCheck, GitBranch, Terminal } from 'lucide-react';

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
      <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Lean 4 Goal Tree Inspector</h3>
            <p className="text-xs text-white/50">No live proof receipt — nothing verified</p>
          </div>
        </div>
        <p className="text-[11px] font-code text-slate-400">
          No theorem, tactics, or AST hash bound. Run a comparator verification to produce a receipt; until then no ρ, no tactics, no Bloch claim.
        </p>
      </div>
    );
  }
  const proofSteps = suppliedSteps ?? [];

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Lean 4 Goal Tree Inspector (Preview)<span className="ml-2 align-middle text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">Illustrative</span></h3>
            <p className="text-xs text-white/50">{theoremName} • Comparator Kernel</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <span className="px-2 py-0.5 rounded-full bg-emerald-500/20 border border-emerald-500/40 text-[11px] font-mono text-emerald-300 flex items-center space-x-1">
            <CheckCircle2 className="w-3 h-3 text-emerald-400" />
            <span>{sorryAx === 0 ? 'ρ = 1.000000' : 'ρ = —'}</span>
          </span>
          <span className="px-2 py-0.5 rounded-full bg-blue-500/10 border border-blue-500/20 text-[10px] font-mono text-blue-300">
            {sorryAx === 0 ? '0 sorryAx' : `${sorryAx ?? '—'} sorryAx`}
          </span>
        </div>
      </div>

      {/* Theorem Signature */}
      <div className="p-3 rounded-xl bg-black/80 border border-white/[0.06] font-mono text-xs text-emerald-300/90 flex justify-between items-center">
        <code>{theoremSignature}</code>
        <span className="text-[10px] text-white/40 font-mono">{astHash}</span>
      </div>

      {axioms && axioms.length > 0 && (
        <p className="text-[10px] font-code text-slate-400">axioms: {axioms.join(', ')}</p>
      )}

      {/* Proof Tree & Tactic States */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Tactic Trace Ladder */}
        <div className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.06] space-y-2">
          <span className="text-xs font-semibold text-white/70 block mb-2 flex items-center space-x-1.5">
            <GitBranch className="w-3.5 h-3.5 text-emerald-400" />
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
                    ? 'bg-emerald-500/15 border-emerald-500/40 text-emerald-200'
                    : 'border-white/[0.04] bg-black/40 text-white/70 hover:bg-white/[0.04]'
                }`}
              >
                <div className="flex justify-between items-center font-mono">
                  <span className="text-white/40">Step {s.step}</span>
                  <span className="text-emerald-300 font-bold">{s.tactic}</span>
                </div>
                <p className="text-[11px] text-white/50 mt-1">{s.note}</p>
              </div>
            );
          })}
        </div>

        {/* Lean 4 Interactive Goal State View */}
        <div className="p-3 rounded-xl bg-black/90 border border-white/[0.06] flex flex-col justify-between font-mono text-xs">
          <div>
            <div className="flex items-center space-x-1.5 text-white/40 pb-2 border-b border-white/[0.06] mb-2">
              <Terminal className="w-3.5 h-3.5 text-emerald-400" />
              <span>Lean 4 Tactic State</span>
            </div>
            <pre className="text-emerald-300/90 whitespace-pre-wrap leading-relaxed">
              {proofSteps[selectedTacticIndex].state}
            </pre>
          </div>

          <div className="mt-4 pt-2 border-t border-white/[0.06] text-[10px] text-white/40 flex justify-between items-center">
            <span>Axiom Whitelist: propext, Quot.sound, Classical.choice</span>
            <span className="text-emerald-400">Lean v4.34.1</span>
          </div>
        </div>
      </div>
    </div>
  );
}
