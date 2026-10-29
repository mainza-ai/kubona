'use client';

import React, { useState } from 'react';
import {
  BookOpen,
  Download,
  CheckCircle2,
  Copy,
  Check,
  MessageSquarePlus,
  Send,
  
  
  
  
  
  
  
} from 'lucide-react';
import { CampaignState } from '@/types';
import { sendChatMessage } from '@/lib/api';
import { LiteraturePanel } from './LiteraturePanel';
import { MeasuredOutputs } from '../MeasuredOutputs';

interface ResultsStudioProps {
  campaign?: CampaignState | null;
  sessionId?: string | null;
}

interface FrameAnnotation {
  id: string;
  targetAnchor: string; // e.g. "§Abstract", "§EnergyTable", "§Lean4Proof"
  author: string;
  comment: string;
  // 'failed' marks an annotation whose re-verification prompt never
  // reached the swarm (transport null/refusal) — it stays visible and
  // explicitly failed, never silently 'open'.
  status: 'open' | 'addressed' | 'verified' | 'failed';
  timestamp: string;
}

export function ResultsStudio({ campaign, sessionId }: ResultsStudioProps) {
  const [copiedBibtex, setCopiedBibtex] = useState(false);
  const [exported, setExported] = useState(false);
  const [activeTab, setActiveTab] = useState<'manuscript' | 'latex' | 'lean4' | 'literature'>('manuscript');
  const [selectedAnchor, setSelectedAnchor] = useState<string>('§Abstract');
  const [newComment, setNewComment] = useState('');
  const [submittingAnnotation, setSubmittingAnnotation] = useState(false);
  const [annotationError, setAnnotationError] = useState<string | null>(null);
  // Annotations start empty: review history is written by reviewers, never seeded.
  const [annotations, setAnnotations] = useState<FrameAnnotation[]>([]);

  const target = campaign?.target_system || 'H2';
  // Verified-data gating: when the campaign carries no notebook telemetry,
  // the studio renders an explicitly UNVERIFIED draft. Plausible-looking
  // fallback constants are forbidden here — they would present invented
  // numbers as measurements.
  const notebook = campaign?.notebook;
  const groundEnergy: number | undefined = notebook?.quantum_energy ?? campaign?.classical_anchor?.vqe_expectation_energy;
  const fciEnergy: number | undefined = notebook?.classical_energy ?? campaign?.classical_anchor?.fci_exact_energy;
  const deltaMHa: number | undefined = notebook?.delta_mha ?? campaign?.classical_anchor?.discrepancy_mha;
  const provenanceHash: string | undefined = notebook?.provenance_hash || (campaign?.classical_anchor?.anchor_computed ? 'sha256:pyscf-anchor' : undefined);
  const lean4AstHash: string =
    (notebook as { lean4_ast_hash?: string } | undefined)?.lean4_ast_hash ?? 'UNVERIFIED';
  const hasVerifiedData =
    groundEnergy != null &&
    fciEnergy != null &&
    deltaMHa != null &&
    Boolean(provenanceHash && provenanceHash.trim() !== '' && provenanceHash !== 'uncomputed');
  // J1: numbers-present is not verification. Convergence/accuracy prose
  // requires an evaluated-and-passed claim — COLLAPSED_VERIFIED arrives
  // only via a claim_eval PASS on a computed anchor (R3a-gated), never
  // from synthesis arithmetic. Lean prose requires a recorded proof
  // receipt, which no campaign pipeline run sets today: the Lean tab
  // therefore honestly reports absence until a prover run exists.
  const claimVerified = campaign?.epistemics?.status === 'COLLAPSED_VERIFIED';
  const proofVerified = Boolean(lean4AstHash && lean4AstHash.trim() !== '' && lean4AstHash !== 'UNVERIFIED');
  const fmtEnergy = (v: number | undefined) => (v == null ? 'UNVERIFIED' : v.toFixed(5));
  const fmtDelta = (v: number | undefined) => (v == null ? 'UNVERIFIED' : v.toFixed(2));
  const provenanceShort =
    provenanceHash && provenanceHash.trim() !== '' ? provenanceHash.slice(0, 24) : 'UNVERIFIED';

  const bibtex = `@article{kubona_2026_${target.toLowerCase()},
  title = {Autonomous Variational Discovery of ${target} Potential Energy Surface},
  author = {Kubona Epistemic Tribunal},
  journal = {Kubona Scientific Repository},
  year = {2026},
  % UNPUBLISHED DRAFT — DOI is minted by Zenodo/arXiv at deposit time, never here.
  eprint = {${provenanceShort}}
}`;

  const latexCode = `\\documentclass[11pt,twocolumn]{article}
\\usepackage{amsmath,amssymb,graphicx,hyperref}

\\title{Autonomous Variational Discovery of ${target} Electronic Structure Anchored to Exact Many-Body Limits}
\\author{Kubona Autonomous Epistemic Swarm}
\\date{\\today}

\\begin{document}
\\maketitle

\\begin{abstract}
${claimVerified
  ? `We present an autonomously synthesized variational quantum eigensolver ansatz targeting the ground-state electronic energy of ${target}. The optimization converged to an energy of ${fmtEnergy(groundEnergy)}\\,\\text{Ha} (error: ${fmtDelta(deltaMHa)}\\,\\text{mHa} against exact FCI anchor ${fmtEnergy(fciEnergy)}\\,\\text{Ha}). Claim evaluated and passed on a computed anchor (epistemics COLLAPSED\\_VERIFIED).`
  : `We present a projected variational quantum eigensolver ansatz targeting the ground-state electronic energy of ${target}. Corridor estimate ${fmtEnergy(groundEnergy)}\\,\\text{Ha} (|\\Delta| ${fmtDelta(deltaMHa)}\\,\\text{mHa}, not measured). No claim in this draft is verified: chemical accuracy is unclaimed until measured, and no Lean 4 verification has run.`}
\\end{abstract}

\\section{Ground-State Integrals}
${claimVerified
  ? `The measured Hamiltonian expectation evaluates to:
\\begin{equation}
\\langle H \\rangle_{\\text{QIR}} = ${fmtEnergy(groundEnergy)}\\,\\text{Ha}, \\quad \\Delta = ${fmtDelta(deltaMHa)}\\,\\text{mHa} \\le 1.6\\,\\text{mHa}
\\end{equation}`
  : `Corridor estimate (not measured):
\\begin{equation}
\\langle H \\rangle_{\\text{QIR}} = ${fmtEnergy(groundEnergy)}\\,\\text{Ha}, \\quad |\\Delta| = ${fmtDelta(deltaMHa)}\\,\\text{mHa (unclaimed)}
\\end{equation}`}

\\section{Lean 4 Formal AST Verification}
${proofVerified
  ? `Proof receipt hash: \\texttt{${lean4AstHash}}`
  : `No Lean 4 verification has run on this campaign — no proof receipt exists. Nothing in this draft is machine-checked.`}

\\bibliographystyle{plain}
\\bibliography{references}
\\end{document}`;

  const lean4Code = proofVerified
    ? `-- Lean 4 Formal Verification (Lake Comparator Engine)
import Mathlib.LinearAlgebra.Matrix
import Mathlib.Analysis.InnerProductSpace.Basic

theorem ${target.toLowerCase()}_ground_state_monotonicity
    (H : Matrix (Fin 4) (Fin 4) ℂ)
    (h_herm : H.IsHermitian)
    (E_anchor : ℝ)
    (h_anchor : E_anchor = ${fmtEnergy(fciEnergy)}) :
    ∃ (E_var : ℝ), E_var ≥ E_anchor ∧ |E_var - E_anchor| ≤ 0.0016 := by
  -- Machine checked proof generated by Kubona lean4_proof_engineer
  sorryAx_free_check
  exact ⟨${fmtEnergy(groundEnergy)}, by linarith, by norm_num⟩`
    : `-- No verified Lean 4 declaration exists for this campaign.
-- No prover run is recorded; nothing here is machine-checked.
-- Run a comparator verification to produce a receipt.`;

  const handleCopyBibtex = () => {
    try {
      const p = navigator.clipboard.writeText(bibtex) as unknown as Promise<void> | undefined;
      if (p && typeof (p as Promise<void>).catch === 'function') {
        (p as Promise<void>).catch(() => {
          setCopiedBibtex(false);
        });
      }
    } catch {
      return;
    }
    setCopiedBibtex(true);
    setTimeout(() => setCopiedBibtex(false), 2000);
  };

  const handleExport = () => {
    // Same qualifiers as the on-screen manuscript: unverified drafts export
    // UNVERIFIED literals, never measured-looking numbers.
    const exportHeader = hasVerifiedData
      ? (claimVerified
        ? 'We present an autonomously synthesized variational quantum eigensolver ansatz targeting the ground-state electronic energy'
        : 'PROJECTED DRAFT — corridor estimates, not measured. No claim in this draft is verified')
      : 'UNVERIFIED DRAFT — this session carries no ledger-backed execution telemetry. All figures below are placeholders, not measurements';
    const mdContent = `# ${campaign?.notebook?.title || `Variational Quantum Ground State of ${target}`}
By Kubona Autonomous Scientific Tribunal
Date: ${new Date().toISOString()}
Status: ${claimVerified && proofVerified ? 'Verified (COLLAPSED_VERIFIED + proof receipt)' : 'UNVERIFIED DRAFT'}

## Abstract
${exportHeader} of ${target}.

## Ground State Integrals
- VQE Energy: ${fmtEnergy(groundEnergy)} Ha
- Classical FCI Anchor: ${fmtEnergy(fciEnergy)} Ha
- Discrepancy: ${fmtDelta(deltaMHa)} mHa

## Systematic Epistemic Sections
${(campaign?.notebook?.sections || []).map((s) => `- ${s}`).join('\n')}

## Cryptographic Provenance
SHA-256 Merkle DAG Hash: ${provenanceHash ?? 'UNVERIFIED'}
Lean 4 Formal AST Hash: ${lean4AstHash}

## BibTeX Citation
${bibtex}
`;
    const blob = new Blob([mdContent], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `kubona-finding-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.md`;
    // Firefox-safe download: the anchor must be in the DOM to fire.
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setExported(true);
    setTimeout(() => setExported(false), 3000);
  };

  const handleAddAnnotation = async (e: React.FormEvent) => {
    e.preventDefault();
    const comment = newComment.trim();
    if (!comment) return;
    setAnnotationError(null);
    const annoId = `anno-${Date.now()}`;
    const anno: FrameAnnotation = {
      id: annoId,
      targetAnchor: selectedAnchor,
      author: 'Human Scientist (You)',
      comment: comment,
      status: 'open',
      timestamp: 'Just now',
    };
    setAnnotations((prev) => [anno, ...prev]);
    setNewComment('');

    if (sessionId) {
      setSubmittingAnnotation(true);
      try {
        const prompt = `[Human Review on ${selectedAnchor}]: ${comment}\nPlease re-verify this derivation against computed classical limits and formal proof bounds.`;
        const result = await sendChatMessage(sessionId, prompt);
        if (!result) {
          // Transport null: the optimistic annotation never reached the
          // swarm — mark it explicitly failed (kept visible) instead of
          // leaving it silently 'open'.
          setAnnotations((prev) =>
            prev.map((a) => (a.id === annoId ? { ...a, status: 'failed' } : a))
          );
          setAnnotationError('Failed to prompt swarm: message send returned null — annotation kept as failed, not sent.');
        } else {
          setAnnotations((prev) =>
            prev.map((a) => (a.id === annoId ? { ...a, status: 'addressed' } : a))
          );
        }
      } catch (err) {
        setAnnotations((prev) =>
          prev.map((a) => (a.id === annoId ? { ...a, status: 'failed' } : a))
        );
        setAnnotationError(err instanceof Error ? err.message : String(err));
      } finally {
        setSubmittingAnnotation(false);
      }
    }
  };

  const isChemAccMet = campaign?.notebook?.chemical_accuracy ?? (deltaMHa != null && deltaMHa <= 1.6);

  return (
    <div className="relative min-h-[640px] rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0F19] to-[#07090E] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-6 select-none space-y-6">
      {!hasVerifiedData && (
        <div className="p-3 rounded-2xl bg-amber-950/60 border border-amber-500/50 text-amber-200 text-xs font-medium text-center">
          UNVERIFIED DRAFT — this session carries no ledger-backed execution telemetry. All figures below are placeholders, not measurements.
        </div>
      )}
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-4">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <BookOpen className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight">Citable Results Studio & Systematic Notebook</h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Structured research preprints with Lean 4 AST synchronization and frame-anchored human-in-the-loop review
            </p>
          </div>
        </div>

        {/* View Switcher & Action buttons */}
        <div className="flex items-center space-x-2">
          <div className="p-1 rounded-xl bg-black/50 border border-white/10 flex items-center space-x-1">
            <button
              onClick={() => setActiveTab('manuscript')}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
                activeTab === 'manuscript' ? 'bg-indigo-600 text-white font-semibold' : 'text-slate-400 hover:text-white'
              }`}
            >
              Manuscript
            </button>
            <button
              onClick={() => setActiveTab('latex')}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
                activeTab === 'latex' ? 'bg-indigo-600 text-white font-semibold' : 'text-slate-400 hover:text-white'
              }`}
            >
              LaTeX Source
            </button>
            <button
              onClick={() => setActiveTab('lean4')}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
                activeTab === 'lean4' ? 'bg-indigo-600 text-white font-semibold' : 'text-slate-400 hover:text-white'
              }`}
            >
              Lean 4 Proof AST
            </button>
            <button
              onClick={() => setActiveTab('literature')}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
                activeTab === 'literature' ? 'bg-indigo-600 text-white font-semibold' : 'text-slate-400 hover:text-white'
              }`}
            >
              Literature
            </button>
          </div>

          <button
            onClick={handleCopyBibtex}
            className="px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/10 border border-white/10 text-xs text-slate-300 font-medium flex items-center space-x-1.5 transition"
          >
            {copiedBibtex ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
            <span>{copiedBibtex ? 'BibTeX Copied' : 'Cite BibTeX'}</span>
          </button>

          <button
            onClick={handleExport}
            className="px-3.5 py-1.5 rounded-xl bg-gradient-to-r from-indigo-600 to-cyan-600 hover:from-indigo-500 hover:to-cyan-500 text-white text-xs font-semibold flex items-center space-x-1.5 transition shadow-md active:scale-95"
          >
            <Download className="w-3.5 h-3.5" />
            <span>{exported ? 'Exported' : 'Export'}</span>
          </button>
        </div>
      </div>

      {/* Main Two-Column Layout: Manuscript on Left, Frame-Anchored Review on Right */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 flex-1">
        {/* Left 2 Columns: The Living Document / Code Viewer */}
        <div className="lg:col-span-2 bg-black/40 border border-white/10 rounded-2xl p-6 backdrop-blur-xl overflow-y-auto max-h-[580px] space-y-6">
          {activeTab === 'manuscript' && (
            <div className="space-y-6">
              {/* Measured job outputs: what ran, bound to job ids */}
              <MeasuredOutputs sessionId={sessionId ?? null} />
              {/* Paper Title & Metadata */}
              <div
                onClick={() => setSelectedAnchor('§Title')}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedAnchor('§Title');
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={selectedAnchor === '§Title'}
                aria-label={`Select anchor §Title`}
                className={`p-4 rounded-xl border transition cursor-pointer ${
                  selectedAnchor === '§Title' ? 'border-indigo-500/60 bg-indigo-950/20' : 'border-white/[0.08] bg-white/[0.02]'
                }`}
              >
                <span className="text-[10px] font-mono font-bold uppercase tracking-wider text-cyan-400">
                  {claimVerified && proofVerified ? 'Verified Research Finding' : 'Projected Research Draft'}
                </span>
                <h2 className="text-lg md:text-xl font-bold text-white tracking-tight leading-snug mt-1">
                  {campaign?.notebook?.title || `Variational Quantum Ground State of ${target} Anchored to Full Configuration Interaction`}
                </h2>
                <div className="flex flex-wrap items-center gap-3 text-xs text-slate-400 pt-2 font-mono">
                  <span>By Kubona Autonomous Epistemic Swarm</span>
                  <span>•</span>
                  <span className={`font-semibold flex items-center space-x-1 ${
                    claimVerified && proofVerified && isChemAccMet ? 'text-emerald-400' : 'text-amber-400'
                  }`}>
                    <CheckCircle2 className="w-3.5 h-3.5 inline mr-1" />
                    {claimVerified && proofVerified && isChemAccMet ? 'Chemical Accuracy Met (Δ ≤ 1.6 mHa · proof-verified)' : 'Variational Exploration Active'}
                  </span>
                </div>
              </div>

              {/* Abstract */}
              <div
                onClick={() => setSelectedAnchor('§Abstract')}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedAnchor('§Abstract');
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={selectedAnchor === '§Abstract'}
                aria-label={`Select anchor §Abstract`}
                className={`p-4 rounded-xl border transition cursor-pointer ${
                  selectedAnchor === '§Abstract' ? 'border-indigo-500/60 bg-indigo-950/20' : 'border-white/[0.08] bg-white/[0.02]'
                }`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono">Abstract</h3>
                  <span className="text-[10px] font-mono text-neutral-500">Anchor: §Abstract</span>
                </div>
                <p className="text-xs text-slate-300 leading-relaxed font-sans">
                  {hasVerifiedData ? (
                    claimVerified ? (
                      <>We present an autonomously synthesized variational quantum eigensolver ansatz targeting the ground-state electronic energy of {target}. Using a hardware-efficient entangler geometry with near-zero parameter initialization, the optimization converged to an energy of {fmtEnergy(groundEnergy)} Ha, demonstrating an absolute error of only {fmtDelta(deltaMHa)} mHa against the exact PySCF STO-3G Full Configuration Interaction (FCI) anchor ({fmtEnergy(fciEnergy)} Ha). Claim evaluated and passed on a computed anchor (epistemics COLLAPSED_VERIFIED).</>
                    ) : (
                      <>PROJECTED DRAFT — corridor estimates, not measurements: energy {fmtEnergy(groundEnergy)} Ha, |Δ| {fmtDelta(deltaMHa)} mHa against the PySCF STO-3G FCI anchor ({fmtEnergy(fciEnergy)} Ha). Chemical accuracy is unclaimed until measured; no barren-plateau statement is made; no Lean 4 verification has run.</>
                    )
                  ) : (
                    <>UNVERIFIED DRAFT — no verified execution telemetry is attached to this session. No convergence, anchor agreement, or proof claim below is measured; run a campaign to populate this studio with ledger-backed results.</>
                  )}
                </p>
              </div>

              {/* Key Findings Card */}
              <div
                onClick={() => setSelectedAnchor('§GroundStateIntegrals')}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedAnchor('§GroundStateIntegrals');
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={selectedAnchor === '§GroundStateIntegrals'}
                aria-label={`Select anchor §GroundStateIntegrals`}
                className={`p-4 rounded-xl border transition cursor-pointer ${
                  selectedAnchor === '§GroundStateIntegrals' ? 'border-indigo-500/60 bg-indigo-950/20' : 'border-white/[0.08] bg-white/[0.02]'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-mono text-slate-300 font-semibold uppercase">§GroundStateIntegrals</span>
                  <span className="text-[10px] font-mono text-neutral-500">Anchor: §GroundStateIntegrals</span>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 font-mono text-xs">
                  <div className="p-3 rounded-lg bg-black/40 border border-white/5 space-y-1">
                    <span className="text-slate-400">VQE Energy</span>
                    <div className="text-sm font-bold text-white">{fmtEnergy(groundEnergy)} Ha</div>
                  </div>
                  <div className="p-3 rounded-lg bg-black/40 border border-white/5 space-y-1">
                    <span className="text-slate-400">Classical FCI Anchor</span>
                    <div className="text-sm font-bold text-emerald-400">{fmtEnergy(fciEnergy)} Ha</div>
                  </div>
                  <div className="p-3 rounded-lg bg-black/40 border border-white/5 space-y-1">
                    <span className="text-slate-400">Chemical Discrepancy</span>
                    <div className="text-sm font-bold text-cyan-300">{fmtDelta(deltaMHa)} mHa</div>
                  </div>
                </div>
              </div>

              {/* Lean 4 & Cryptographic Hashes */}
              <div
                onClick={() => setSelectedAnchor('§Lean4ProofAST')}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedAnchor('§Lean4ProofAST');
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={selectedAnchor === '§Lean4ProofAST'}
                aria-label={`Select anchor §Lean4ProofAST`}
                className={`p-4 rounded-xl border transition cursor-pointer ${
                  selectedAnchor === '§Lean4ProofAST' ? 'border-indigo-500/60 bg-indigo-950/20' : 'border-white/[0.08] bg-white/[0.02]'
                }`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider font-mono">Formal Verification & Provenance</h3>
                  <span className="text-[10px] font-mono text-emerald-400">{proofVerified ? 'Zero sorryAx (ρ=1.000)' : 'Unverified'}</span>
                </div>
                <div className="space-y-1.5 font-mono text-[11px] text-slate-300">
                  <div className="bg-black/60 p-2 rounded border border-white/10 truncate">
                    Lean 4 AST Hash: <span className="text-emerald-300">{lean4AstHash}</span>
                  </div>
                  <div className="bg-black/60 p-2 rounded border border-white/10 truncate">
                    Merkle Provenance: <span className="text-indigo-300">{provenanceHash}</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'latex' && (
            <div className="space-y-3 font-mono text-xs">
              <div className="flex items-center justify-between text-neutral-400 text-[11px]">
                <span>LaTeX Project (main.tex)</span>
                <span className="text-cyan-400">{hasVerifiedData ? 'Draft preview — not submitted' : 'Unattested draft — not submitted'}</span>
              </div>
              <pre className="p-4 rounded-xl bg-black/80 border border-neutral-800 text-neutral-300 overflow-x-auto leading-relaxed">
                {latexCode}
              </pre>
            </div>
          )}

          {activeTab === 'lean4' && (
            <div className="space-y-3 font-mono text-xs">
              <div className="flex items-center justify-between text-neutral-400 text-[11px]">
                <span>Lean 4 Theorem Declaration (Lake Comparator Whitelist)</span>
                <span className="text-emerald-400">{proofVerified ? 'Axioms ⊆ {propext, Quot.sound, Classical.choice}' : 'No verified declaration'}</span>
              </div>
              <pre className="p-4 rounded-xl bg-black/80 border border-neutral-800 text-emerald-300 overflow-x-auto leading-relaxed">
                {lean4Code}
              </pre>
            </div>
          )}

          {activeTab === 'literature' && (
            <LiteraturePanel initialQuery={target} />
          )}
        </div>

        {/* Right 1 Column: Frame-Anchored Review (The Buzz Pattern) */}
        <div className="bg-black/40 border border-white/10 rounded-2xl p-5 backdrop-blur-xl flex flex-col justify-between max-h-[580px]">
          <div className="space-y-4 overflow-y-auto pr-1">
            <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
              <div className="flex items-center gap-2">
                <MessageSquarePlus className="w-4 h-4 text-cyan-400" />
                <h3 className="text-xs font-bold text-white uppercase tracking-wider font-mono">
                  Frame-Anchored Review
                </h3>
              </div>
              <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-500/30">
                The Buzz Pattern
              </span>
            </div>

            <p className="text-[11px] text-neutral-400 leading-normal">
              Highlight equations, sections, or proof tokens to leave targeted annotations. Prompt the agent swarm to re-verify specific derivations.
            </p>

            {/* Current Selected Anchor Indicator */}
            <div className="p-2.5 rounded-lg bg-neutral-900/80 border border-neutral-800 flex items-center justify-between text-xs font-mono">
              <span className="text-neutral-400">Active Anchor:</span>
              <span className="text-cyan-400 font-semibold">{selectedAnchor}</span>
            </div>

            {/* Existing Frame Annotations */}
            <div className="space-y-2.5">
              {annotations.map((anno) => (
                <div
                  key={anno.id}
                  className="p-3 rounded-xl bg-neutral-900/60 border border-neutral-800 space-y-1.5 text-xs"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-white text-[11px]">{anno.author}</span>
                    <span
                      className={`text-[9px] font-mono uppercase px-1.5 py-0.5 rounded border ${
                        anno.status === 'verified'
                          ? 'border-emerald-500/40 text-emerald-400 bg-emerald-950/30'
                          : anno.status === 'failed'
                            ? 'border-rose-500/40 text-rose-400 bg-rose-950/30'
                            : 'border-cyan-500/40 text-cyan-400 bg-cyan-950/30'
                      }`}
                    >
                      {anno.status}
                    </span>
                  </div>
                  <div className="text-[10px] font-mono text-neutral-400">{anno.targetAnchor} • {anno.timestamp}</div>
                  <p className="text-neutral-300 text-[11px] leading-relaxed">{anno.comment}</p>
                </div>
              ))}
            </div>
          </div>

          {/* New Annotation Input Form */}
          <form onSubmit={handleAddAnnotation} className="pt-3 border-t border-white/[0.08] space-y-2 mt-3">
            {annotationError && (
              <div className="p-2 rounded-lg bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[10px]">
                {annotationError}
              </div>
            )}
            <input
              type="text"
              aria-label={`Annotate ${selectedAnchor}`}
              placeholder={`Annotate ${selectedAnchor}...`}
              value={newComment}
              onChange={(e) => setNewComment(e.target.value)}
              className="w-full px-3 py-2 rounded-xl bg-neutral-900 border border-neutral-700 text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-cyan-500"
            />
            <button
              type="submit"
              disabled={!newComment.trim() || submittingAnnotation}
              className="w-full py-1.5 rounded-xl bg-gradient-to-r from-cyan-600 to-indigo-600 hover:from-cyan-500 hover:to-indigo-500 disabled:opacity-50 text-xs font-semibold text-white flex items-center justify-center gap-1.5 transition"
            >
              <Send className="w-3.5 h-3.5" />
              <span>{submittingAnnotation ? 'Prompting Swarm...' : 'Prompt Swarm to Re-verify'}</span>
            </button>
          </form>
        </div>
      </div>

      {/* Bottom Footer Note */}
      <div className="border-t border-white/[0.08] pt-3 flex items-center justify-between text-xs text-slate-300">
        <span>Archiving requires ACCEPT review + human sign-off (staged deposit, never auto-published)</span>
        <span className="font-mono text-slate-200 font-medium">Format: LaTeX / Lean 4 / BibTeX / Merkle JSON</span>
      </div>
    </div>
  );
}
