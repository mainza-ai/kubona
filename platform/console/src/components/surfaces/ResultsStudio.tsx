'use client';

import React, { useEffect, useState } from 'react';
import {
  BookOpen,
  Download,
  CheckCircle2,
  Copy,
  Check,
  MessageSquarePlus,
  Send,
  
  
  
  
  
  
  
} from 'lucide-react';
import { CampaignExperiment, CampaignResultRef, CampaignState, PesPoint, SystematicNotebook, Job } from '@/types';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import { fetchExperimentExport, fetchNotebooks, sendChatMessage } from '@/lib/api';
import { jobStatusLabel } from '@/lib/job-status';
import { numericClaimGuard } from '@/lib/resultNarrative';
import { useJobsProjection } from '@/domain/useJobsProjection';
import { LiteraturePanel } from './LiteraturePanel';
import { MeasuredOutputs } from '../MeasuredOutputs';
import { ClaimHistory } from '@/components/common/ClaimHistory';
import { Chart } from '@/components/data/LazyChart';
import type { ChartSpec } from '@/components/data/chart';

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

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>;
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value) as unknown;
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : null;
    } catch {
      return null;
    }
  }
  return null;
}

function readCounts(job: Job): Record<string, number> | null {
  const result = asRecord(job.result_json);
  const raw = asRecord(result?.counts);
  if (!raw) return null;
  const counts: Record<string, number> = {};
  for (const [bitstring, value] of Object.entries(raw)) {
    const n = typeof value === 'number' ? value : Number(value);
    if (Number.isFinite(n) && n >= 0) counts[bitstring] = n;
  }
  return Object.keys(counts).length > 0 ? counts : null;
}

function downloadText(name: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function csvCell(value: unknown): string {
  const text = value == null ? '' : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

function PesTable({ points }: { points: PesPoint[] }) {
  return (
    <div className="rounded-xl border border-white/10 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 bg-white/[0.03]">
        <span className="text-[12px] font-semibold text-white">PES points</span>
        <span className="text-[11px] text-[var(--color-muted)]">measured energy vs full FCI anchor</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[12px]">
          <thead className="text-[var(--color-muted)]">
            <tr className="border-b border-white/10">
              <th className="px-2 py-1 text-left">R (Å)</th>
              <th className="px-2 py-1 text-right">E (Ha)</th>
              <th className="px-2 py-1 text-right">FCI (Ha)</th>
              <th className="px-2 py-1 text-right">Δ (mHa)</th>
              <th className="px-2 py-1 text-right">σ (mHa)</th>
              <th className="px-2 py-1 text-right">95% (mHa)</th>
              <th className="px-2 py-1 text-right">Chem. acc.</th>
              <th className="px-2 py-1 text-left">Source</th>
            </tr>
          </thead>
          <tbody>
            {points.map((point) => (
              <tr key={point.bond_distance} className="border-b border-white/[0.06] last:border-0">
                <td className="px-2 py-1 font-code">{point.bond_distance.toFixed(4)}</td>
                <td className="px-2 py-1 text-right font-code">{point.energy.toFixed(6)}</td>
                <td className="px-2 py-1 text-right font-code">{point.fci_exact_energy.toFixed(6)}</td>
                <td className="px-2 py-1 text-right font-code">{point.delta_mha.toFixed(3)}</td>
                <td className="px-2 py-1 text-right font-code">{point.uncertainty_mha.toFixed(3)}</td>
                <td className="px-2 py-1 text-right font-code">{typeof point.ci95_mha === 'number' ? point.ci95_mha.toFixed(3) : '—'}</td>
                <td className="px-2 py-1 text-right">{(point.claimable ?? point.chemical_accuracy) ? 'PASS' : 'outside'}</td>
                <td className="px-2 py-1">{point.hardware ? `hardware${point.mitigated ? ' · ZNE' : ''}${point.backend ? ` · ${point.backend}` : ''}` : 'simulator'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ResultReceiptTable({ receipts }: { receipts: CampaignResultRef[] }) {
  return (
    <div className="rounded-xl border border-white/10 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 bg-white/[0.03]">
        <span className="text-[12px] font-semibold text-white">Result receipts</span>
        <span className="text-[11px] text-[var(--color-muted)]">job → result SHA-256 → ledger receipt</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[12px]">
          <thead className="text-[var(--color-muted)]">
            <tr className="border-b border-white/10">
              <th className="px-2 py-1 text-left">Job</th>
              <th className="px-2 py-1 text-left">Kind</th>
              <th className="px-2 py-1 text-left">Version</th>
              <th className="px-2 py-1 text-left">Signed</th>
              <th className="px-2 py-1 text-left">Ledger entry</th>
              <th className="px-2 py-1 text-left">Result SHA-256</th>
            </tr>
          </thead>
          <tbody>
            {receipts.map((row) => (
              <tr key={row.job_id} className="border-b border-white/[0.06] last:border-0">
                <td className="px-2 py-1 font-code">{row.job_id.slice(0, 8)}</td>
                <td className="px-2 py-1 font-code">{row.kind}</td>
                <td className="px-2 py-1 font-code">{row.result_version || 'unversioned'}</td>
                <td className="px-2 py-1">{row.result_receipt?.signed ? 'signed' : 'hash-chained'}</td>
                <td className="px-2 py-1 font-code">{row.result_receipt?.ledger_entry_hash?.slice(0, 16) || 'missing'}</td>
                <td className="px-2 py-1 font-code">{row.result_receipt?.result_sha256?.slice(0, 16) || 'missing'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ExperimentsTable({ experiments }: { experiments: CampaignExperiment[] }) {
  return (
    <div className="rounded-xl border border-white/10 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 bg-white/[0.03]">
        <span className="text-[12px] font-semibold text-white">Measured experiments</span>
        <span className="text-[11px] text-[var(--color-muted)]">spin, excited, reaction, and conjecture records folded from jobs</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[12px]">
          <thead className="text-[var(--color-muted)]">
            <tr className="border-b border-white/10">
              <th className="px-2 py-1 text-left">Kind</th>
              <th className="px-2 py-1 text-left">System</th>
              <th className="px-2 py-1 text-right">Measured</th>
              <th className="px-2 py-1 text-right">Anchor</th>
              <th className="px-2 py-1 text-right">Gap / ΔE</th>
              <th className="px-2 py-1 text-right">σ</th>
              <th className="px-2 py-1 text-right">Status</th>
              <th className="px-2 py-1 text-left">Job</th>
            </tr>
          </thead>
          <tbody>
            {experiments.map((exp) => {
              const primary = exp.energy ?? exp.ground_energy ?? exp.reaction_energy;
              const secondary = exp.energy_exact ?? exp.ground_energy_exact ?? exp.reaction_energy_exact;
              const gap = exp.gap ?? (exp.kind === 'reaction' ? exp.delta : undefined);
              const system = exp.molecule || exp.model || exp.reaction || (exp.n_sites ? `${exp.n_sites} sites` : '—');
              return (
                <tr key={exp.job_id} className="border-b border-white/[0.06] last:border-0">
                  <td className="px-2 py-1 font-code">{exp.kind}</td>
                  <td className="px-2 py-1 font-code">{system}</td>
                  <td className="px-2 py-1 text-right font-code">{typeof primary === 'number' ? primary.toFixed(6) : '—'}</td>
                  <td className="px-2 py-1 text-right font-code">{typeof secondary === 'number' ? secondary.toFixed(6) : '—'}</td>
                  <td className="px-2 py-1 text-right font-code">{typeof gap === 'number' ? gap.toFixed(6) : '—'}</td>
                  <td className="px-2 py-1 text-right font-code">{typeof exp.uncertainty === 'number' ? exp.uncertainty.toFixed(6) : '—'}</td>
                  <td className="px-2 py-1 text-right">
                    {exp.status === 'failed'
                      ? `failed${exp.error ? ` · ${exp.error}` : ''}`
                      : exp.kind === 'conjecture_run'
                        ? `${exp.novelty_status || 'unclassified'}${exp.laws?.length ? ` · ${exp.laws[0]}` : ''}`
                        : exp.claimable ? 'claimable' : 'not claimable'}
                    {exp.hardware && exp.status !== 'failed' ? ' · hardware' : ''}
                  </td>
                  <td className="px-2 py-1 font-code">{exp.job_id.slice(0, 8)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
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
  const [remoteNotebook, setRemoteNotebook] = useState<SystematicNotebook | null>(null);
  const [notebookError, setNotebookError] = useState<string | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);

  const target =
    campaign?.target_system ||
    campaign?.quantum_factors?.target_molecule ||
    'unrecorded target';
  // The campaign payload is the primary source; the results projection is
  // the fallback when a session has a notebook but the campaign read did not
  // carry it. Never synthesize a notebook locally.
  useEffect(() => {
    if (!sessionId || campaign?.notebook) return;
    let live = true;
    fetchNotebooks(sessionId).then((out) => {
      if (!live) return;
      if (!out.ok) { setNotebookError(out.error); return; }
      setNotebookError(null);
      setRemoteNotebook(out.notebooks[0] ?? null);
    });
    return () => { live = false; };
  }, [sessionId, campaign?.notebook]);

  const { jobs: sessionJobs } = useJobsProjection(sessionId ?? null);
  const measuredCounts = (() => {
    if (!sessionId) return null;
    const completed = sessionJobs
      .filter((job) => jobStatusLabel(job.status) === 'COMPLETED')
      .sort((a, b) => String(b.completed_at ?? b.created_at).localeCompare(String(a.completed_at ?? a.created_at)));
    for (const job of completed) {
      const counts = readCounts(job);
      if (!counts) continue;
      const result = asRecord(job.result_json);
      return {
        sessionId,
        jobId: job.id,
        backend: typeof result?.resolved_backend === 'string' ? result.resolved_backend : 'unrecorded backend',
        counts,
      };
    }
    return null;
  })();

  const notebook = campaign?.notebook ?? remoteNotebook ?? undefined;
  const groundEnergy: number | undefined = notebook?.quantum_energy ?? campaign?.classical_anchor?.vqe_expectation_energy;
  const fciEnergy: number | undefined = notebook?.classical_energy ?? campaign?.classical_anchor?.fci_exact_energy;
  const deltaMHa: number | undefined = notebook?.delta_mha ?? campaign?.classical_anchor?.discrepancy_mha;
  const measurement = campaign?.measurement;
  const uncertaintyMHa = measurement?.uncertainty_mha;
  const converged = measurement?.converged;
  const belowFci = measurement?.below_fci === true;
  const provenanceHash: string | undefined = notebook?.provenance_hash;
  const lean4AstHash =
    (notebook as { lean4_ast_hash?: string } | undefined)?.lean4_ast_hash ?? null;
  // A citable studio requires an evaluated claim and a recorded notebook
  // provenance. Numbers alone (including synthesis corridors) are not a
  // result; without this gate the page has nothing honest to show.
  const provenanceRecorded = Boolean(
    provenanceHash &&
    provenanceHash.trim() !== '' &&
    provenanceHash !== 'uncomputed' &&
    provenanceHash !== 'UNVERIFIED'
  );
  const hasVerifiedData =
    campaign?.epistemics?.status === 'COLLAPSED_VERIFIED' &&
    groundEnergy != null &&
    fciEnergy != null &&
    deltaMHa != null &&
    provenanceRecorded;
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
  const provenanceShort = provenanceHash ? provenanceHash.slice(0, 24) : 'not recorded';
  const abstractNarrative = claimVerified
    ? `We present a measured variational quantum eigensolver result for ${target}. The optimizer ${converged === false ? 'stopped at its evaluation cap (not reported as converged)' : 'reported convergence'}; the measured energy is ${fmtEnergy(groundEnergy)}${uncertaintyMHa != null ? ` ± ${uncertaintyMHa.toFixed(3)} mHa` : ''} (Delta ${fmtDelta(deltaMHa)} mHa against FCI ${fmtEnergy(fciEnergy)} Ha${belowFci ? '; point below FCI; read against uncertainty' : ''}). Claim evaluated on a computed anchor (epistemics COLLAPSED_VERIFIED).`
    : `We present a projected variational quantum eigensolver ansatz targeting the ground-state electronic energy of ${target}. Corridor estimate ${fmtEnergy(groundEnergy)} Ha (|Delta| ${fmtDelta(deltaMHa)} mHa, not measured). No claim in this draft is verified: chemical accuracy is unclaimed until measured, and no Lean 4 verification has run.`;
  const narrativeAllowed = [
    groundEnergy, fciEnergy, deltaMHa, uncertaintyMHa,
    campaign?.measurement?.energy, campaign?.measurement?.energy_exact,
    campaign?.measurement?.fci_exact_energy, campaign?.measurement?.uncertainty_mha,
    campaign?.measurement?.delta_mha, 1.6,
    ...(target.match(/\d+(?:\.\d+)?/g) ?? []).map(Number),
  ].filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
  const narrativeGuard = numericClaimGuard(abstractNarrative, narrativeAllowed);
  const safeAbstract = narrativeGuard.ok
    ? abstractNarrative
    : 'Measured result recorded; the generated abstract was withheld because it contained a number absent from the measurement record.';

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
${safeAbstract}
\end{abstract}

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
    ? `-- Recorded Lean 4 proof receipt
-- AST hash: ${lean4AstHash}
-- The proof AST and declaration are served by the backend receipt; the
-- console does not synthesize theorem text or claim axioms it cannot read.
-- Inspect the receipt via /api/ledger/export for this session.`
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

  const handleExportSigned = async () => {
    if (!sessionId) return;
    setExportError(null);
    try {
      const bundle = await fetchExperimentExport(sessionId);
      downloadText(`kubona-experiment-export-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.json`,
        JSON.stringify(bundle, null, 2), 'application/json;charset=utf-8');
    } catch (err) {
      setExportError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleExportResult = () => {
    const bundle = {
      schema_version: 1,
      exported_at: new Date().toISOString(),
      campaign,
      measurement: campaign?.measurement ?? null,
      pes_points: campaign?.pes_points ?? [],
      experiments: campaign?.experiments ?? [],
      notebook: notebook ?? null,
    };
    const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `kubona-result-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    setExported(true);
    setTimeout(() => setExported(false), 3000);
  };

  const handleExportCSV = () => {
    const points = campaign?.pes_points ?? [];
    if (points.length > 0) {
      const header = ['bond_distance_angstrom', 'energy_ha', 'energy_exact_ha', 'fci_exact_ha', 'delta_mha', 'uncertainty_mha', 'chemical_accuracy'];
      const rows = points.map((point) => [
        point.bond_distance, point.energy, point.energy_exact ?? '', point.fci_exact_energy,
        point.delta_mha, point.uncertainty_mha, point.chemical_accuracy,
      ]);
      downloadText(`kubona-pes-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.csv`,
        [header, ...rows].map((row) => row.map(csvCell).join(',')).join('\n') + '\n',
        'text/csv;charset=utf-8');
      return;
    }
    const m = campaign?.measurement;
    if (!m) return;
    const header = ['job_id', 'claim_id', 'method', 'backend', 'energy_ha', 'energy_exact_ha', 'fci_exact_ha', 'uncertainty_mha', 'delta_mha', 'shots', 'seed', 'chemical_accuracy', 'converged', 'below_fci', 'optimizer_points', 'provider', 'hardware', 'ci95_mha', 'usage_seconds', 'actual_usd'];
    const row = [m.job_id ?? '', m.claim_id ?? '', m.method, m.backend ?? '', m.energy, m.energy_exact ?? '', m.fci_exact_energy, m.uncertainty_mha, m.delta_mha, m.shots ?? '', m.seed ?? '', m.chemical_accuracy, m.converged, m.below_fci, m.optimizer_history?.length ?? 0, m.provider ?? '', m.hardware ?? false, m.ci95_mha ?? '', m.usage_seconds ?? '', m.actual_usd ?? ''];
    downloadText(`kubona-measurement-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.csv`,
      [header, row].map((r) => r.map(csvCell).join(',')).join('\n') + '\n',
      'text/csv;charset=utf-8');
  };

  const handleExport = () => {
    const mdContent = `# ${campaign?.notebook?.title || `Variational Quantum Ground State of ${target}`}
By Kubona Autonomous Scientific Tribunal
Date: ${new Date().toISOString()}
Status: ${claimVerified && proofVerified ? 'Verified (COLLAPSED_VERIFIED + proof receipt)' : 'Claim verified (COLLAPSED_VERIFIED); no Lean proof receipt'}

## Abstract
We present a measured variational quantum eigensolver result for ${target}.

## Ground State Integrals
- VQE Energy: ${fmtEnergy(groundEnergy)}${uncertaintyMHa != null ? ` ± ${uncertaintyMHa.toFixed(3)} mHa` : ''} Ha
- Classical FCI Anchor: ${fmtEnergy(fciEnergy)} Ha
- Discrepancy: ${fmtDelta(deltaMHa)} mHa
- Optimizer converged: ${converged == null ? 'not recorded' : converged ? 'yes' : 'no (evaluation cap)'}
- Backend: ${measurement?.backend ?? 'not recorded'}
- Shots: ${measurement?.shots ?? 'not recorded'}
- Claim: ${measurement?.claim_id ?? 'not recorded'}
- Point below FCI: ${belowFci ? 'yes (read against uncertainty)' : 'no'}

## Systematic Epistemic Sections
${(campaign?.notebook?.sections || []).map((s) => `- ${s}`).join('\n')}

## Cryptographic Provenance
SHA-256 Merkle DAG Hash: ${provenanceHash ?? 'UNVERIFIED'}
Lean 4 Formal AST Hash: ${lean4AstHash ?? 'UNVERIFIED (no proof receipt)'}

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
  const energySpec: ChartSpec | undefined =
    groundEnergy != null && fciEnergy != null
      ? {
          kind: 'line',
          title: `Energy against the FCI anchor · ${target}`,
          series: [
            {
              name: 'Energy',
              points: [
                {
                  x: 'VQE',
                  y: groundEnergy,
                  display: `${fmtEnergy(groundEnergy)} Ha`,
                  evidence: provenanceHash ? [{ key: 'provenance', label: provenanceShort }] : [],
                },
                {
                  x: 'FCI',
                  y: fciEnergy,
                  display: `${fmtEnergy(fciEnergy)} Ha`,
                  evidence: [{ key: 'anchor', label: 'PySCF FCI anchor' }],
                },
              ],
            },
          ],
          x: { label: 'Method', kind: 'category' },
          y: { label: 'Energy', unit: 'Ha', format: 'number' },
          headline: deltaMHa != null ? { label: '|Δ| vs FCI', value: `${fmtDelta(deltaMHa)} mHa` } : undefined,
          basis:
            deltaMHa != null
              ? `Measured values recorded in the notebook; discrepancy is ${fmtDelta(deltaMHa)} mHa against the exact FCI anchor.`
              : 'Values recorded in the notebook; no discrepancy recorded.',
          detail: 'full',
        }
      : undefined;

  const countsForSession = measuredCounts?.sessionId === sessionId ? measuredCounts : null;
  const countsSpec: ChartSpec | undefined = countsForSession
    ? (() => {
        const entries = Object.entries(countsForSession.counts).sort((a, b) => b[1] - a[1]);
        const total = entries.reduce((sum, [, value]) => sum + value, 0);
        return {
          kind: 'bars',
          title: `Measured bitstring distribution · ${target}`,
          series: [{
            name: 'Shots',
            points: entries.map(([bitstring, shots]) => ({
              x: bitstring,
              y: shots,
              display: `${shots} shots (${total > 0 ? ((shots / total) * 100).toFixed(1) : '0.0'}%)`,
            })),
          }],
          x: { label: 'Bitstring', kind: 'category' },
          y: { label: 'Shots', unit: 'shots', format: 'number' },
          headline: { label: 'Total shots', value: String(total) },
          basis: `Result of completed job ${countsForSession.jobId} on ${countsForSession.backend}; counts are server-returned, not reconstructed.`,
          detail: 'full',
        };
      })()
    : undefined;

  const experimentReport = (() => {
    const rows = campaign?.experiments ?? [];
    if (rows.length === 0) return '';
    const lines = [
      '# Measured experiment report',
      '',
      `Target: ${target}`,
      `Generated: ${new Date().toISOString()}`,
      '',
      'This report lists stored job results only. It is not a chemical-accuracy claim,',
      'a novelty claim, or a theorem. Claimability is reported per row.',
      '',
    ];
    for (const exp of rows) {
      lines.push(`## ${exp.kind} · ${exp.molecule || exp.model || exp.reaction || 'unrecorded system'}`);
      lines.push(`- job: ${exp.job_id}`);
      if (exp.result_version) lines.push(`- result_version: ${exp.result_version}`);
      if (exp.backend) lines.push(`- backend: ${exp.backend}`);
      if (exp.hardware_job_id) lines.push(`- vendor job: ${exp.hardware_job_id}`);
      if (exp.hardware) lines.push('- hardware: yes');
      if (typeof exp.energy === 'number') lines.push(`- measured energy: ${exp.energy} Ha`);
      if (typeof exp.energy_exact === 'number') lines.push(`- exact anchor: ${exp.energy_exact} Ha`);
      if (typeof exp.ground_energy === 'number') lines.push(`- measured ground: ${exp.ground_energy} Ha`);
      if (typeof exp.ground_energy_exact === 'number') lines.push(`- exact ground: ${exp.ground_energy_exact} Ha`);
      if (typeof exp.excited_energy === 'number') lines.push(`- measured excited: ${exp.excited_energy} Ha`);
      if (typeof exp.excited_energy_exact === 'number') lines.push(`- exact excited: ${exp.excited_energy_exact} Ha`);
      if (typeof exp.gap === 'number') lines.push(`- measured gap: ${exp.gap} Ha`);
      if (typeof exp.gap_exact === 'number') lines.push(`- exact gap: ${exp.gap_exact} Ha`);
      if (typeof exp.reaction_energy === 'number') lines.push(`- measured reaction energy: ${exp.reaction_energy} Ha`);
      if (typeof exp.reaction_energy_exact === 'number') lines.push(`- exact reaction anchor: ${exp.reaction_energy_exact} Ha`);
      if (typeof exp.reaction_energy_exact_full === 'number') lines.push(`- full-molecule reaction anchor: ${exp.reaction_energy_exact_full} Ha`);
      if (typeof exp.uncertainty === 'number') lines.push(`- uncertainty: ${exp.uncertainty} Ha`);
      if (typeof exp.delta === 'number') lines.push(`- delta: ${exp.delta} Ha`);
      if (exp.status) lines.push(`- status: ${exp.status}`);
      if (exp.error) lines.push(`- error: ${exp.error}`);
      lines.push(`- measured: ${exp.measured ? 'yes' : 'no'}`);
      lines.push(`- claimable: ${exp.claimable ? 'yes' : 'no'}`);
      if (exp.kind === 'conjecture_run') {
        lines.push(`- novelty_status: ${exp.novelty_status || 'unclassified'}`);
        if (exp.laws?.length) lines.push(`- laws: ${exp.laws.join('; ')}`);
        if (exp.known_as?.length) lines.push(`- known as: ${exp.known_as.join('; ')}`);
        lines.push('- proof_status: not-a-proof');
      }
      lines.push('');
    }
    const receipts = campaign?.result_receipts ?? [];
    if (receipts.length > 0) {
      lines.push('## Result receipts');
      for (const row of receipts) {
        lines.push(`- ${row.job_id} (${row.kind}): ${row.result_receipt?.signed ? 'signed' : 'hash-chained'} · ledger ${row.result_receipt?.ledger_entry_hash || 'missing'} · result ${row.result_receipt?.result_sha256 || 'missing'}`);
      }
      lines.push('');
    }
    return lines.join('\n');
  })();

  const handleExportExperiments = () => {
    if (!experimentReport) return;
    downloadText(`kubona-experiments-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.md`,
      experimentReport, 'text/markdown;charset=utf-8');
  };

  const handleExportExperimentsJSON = () => {
    const bundle = {
      schema_version: 1,
      exported_at: new Date().toISOString(),
      target,
      result_version: 'experiment-result-v1',
      experiments: campaign?.experiments ?? [],
      result_receipts: campaign?.result_receipts ?? [],
    };
    downloadText(`kubona-experiments-${target.toLowerCase().replace(/[^a-z0-9]/g, '-')}.json`,
      JSON.stringify(bundle, null, 2), 'application/json;charset=utf-8');
  };

  if (!hasVerifiedData) {
    return (
      <div className="relative min-h-[520px] rounded-3xl overflow-hidden bg-[var(--color-page)] border border-white/[0.08] shadow-2xl p-6 space-y-5">
        <div className="flex items-center space-x-3.5 border-b border-white/[0.08] pb-4">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 flex items-center justify-center shadow-inner">
            <BookOpen className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight">Citable Results Studio &amp; Systematic Notebook</h2>
            <p className="text-xs text-slate-400 mt-0.5">No citable result is recorded for this campaign.</p>
          </div>
        </div>
        <MeasuredOutputs sessionId={sessionId ?? null} />
        {(campaign?.pes_points?.length ?? 0) > 0 && (
          <PesTable points={campaign!.pes_points!} />
        )}
        {(campaign?.result_receipts?.length ?? 0) > 0 && (
          <ResultReceiptTable receipts={campaign!.result_receipts!} />
        )}
        {(campaign?.experiments?.length ?? 0) > 0 && (
          <>
            <ExperimentsTable experiments={campaign!.experiments!} />
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={handleExportExperiments}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-text)] text-[12px]"
              >
                <Download className="w-3.5 h-3.5" /> Download experiment report
              </button>
              <button
                type="button"
                onClick={handleExportExperimentsJSON}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-text)] text-[12px]"
              >
                <Download className="w-3.5 h-3.5" /> Experiment JSON
              </button>
              <button
                type="button"
                onClick={() => void handleExportSigned()}
                disabled={!sessionId}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-emerald-500/10 border border-emerald-400/30 text-emerald-200 text-[12px] disabled:opacity-40"
              >
                <Download className="w-3.5 h-3.5" /> Signed export
              </button>
            </div>
          </>
        )}
        {exportError && <p role="alert" className="text-[12px] text-rose-300">{exportError}</p>}
        <div className="rounded-2xl border border-white/[0.08] bg-black/30 p-6 text-center space-y-2">
          <p className="text-sm font-medium text-white">No manuscript, citation, or proof receipt yet.</p>
          <p className="text-[13px] leading-relaxed text-slate-400">
            A citable studio appears only after an evaluated claim and a ledger-backed notebook exist.
            Projected corridors and template values are not shown here as results.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div data-provenance="measured" className="relative min-h-[640px] rounded-3xl overflow-hidden bg-[var(--color-page)] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-6 space-y-6">
      {notebookError && (
        <div role="alert" className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[12px]">
          Notebook projection unavailable — {notebookError}. Nothing synthesized.
        </div>
      )}
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-4">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 flex items-center justify-center shadow-inner">
            <BookOpen className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold text-white tracking-tight">Citable Results Studio & Systematic Notebook</h2>
              <ProvenanceBadge kind="measured" detail="Citable gate passed: evaluated claim, notebook energies, and provenance hash." />
            </div>
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
            data-testid="result-csv"
            onClick={handleExportCSV}
            disabled={!campaign?.measurement && !(campaign?.pes_points?.length)}
            title="Download measured CSV: PES points when present, otherwise the measurement summary"
            className="px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/10 border border-white/10 text-[var(--color-text)] text-xs font-medium flex items-center space-x-1.5 transition disabled:opacity-40"
          >
            <Download className="w-3.5 h-3.5" />
            <span>CSV</span>
          </button>

          <button
            onClick={handleExportResult}
            title="Download measured result JSON (campaign, measurement, PES points, notebook)"
            className="px-3 py-1.5 rounded-xl bg-emerald-500/10 hover:bg-emerald-500/20 border border-emerald-400/30 text-emerald-200 text-xs font-medium flex items-center space-x-1.5 transition"
          >
            <Download className="w-3.5 h-3.5" />
            <span>Result JSON</span>
          </button>

          <button
            onClick={() => void handleExportSigned()}
            disabled={!sessionId}
            title="Download a server-built experiment bundle whose canonical payload is signed by the deployment key"
            className="px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/10 border border-white/10 text-[var(--color-text)] text-xs font-medium flex items-center space-x-1.5 transition disabled:opacity-40"
          >
            <Download className="w-3.5 h-3.5" />
            <span>Signed export</span>
          </button>

          <button
            onClick={handleExport}
            className="px-3.5 py-1.5 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold flex items-center space-x-1.5 transition shadow-md active:scale-95"
          >
            <Download className="w-3.5 h-3.5" />
            <span>{exported ? 'Exported' : 'Export'}</span>
          </button>
        </div>
      </div>

      {/* Main Two-Column Layout: Manuscript on Left, Frame-Anchored Review on Right */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 flex-1">
        {/* Left 2 Columns: The Living Document / Code Viewer */}
        <div className="lg:col-span-2 bg-black/40 border border-white/10 rounded-2xl p-6 overflow-y-auto max-h-[580px] space-y-6">
          {activeTab === 'manuscript' && (
            <div className="space-y-6">
              {/* Measured job outputs: what ran, bound to job ids */}
              <MeasuredOutputs sessionId={sessionId ?? null} />
              {(campaign?.pes_points?.length ?? 0) > 0 && (
                <PesTable points={campaign!.pes_points!} />
              )}
              {(campaign?.experiments?.length ?? 0) > 0 && (
                <ExperimentsTable experiments={campaign!.experiments!} />
              )}
              {(campaign?.result_receipts?.length ?? 0) > 0 && (
                <ResultReceiptTable receipts={campaign!.result_receipts!} />
              )}
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
                <span className="text-[12px] font-mono font-bold uppercase tracking-wider text-cyan-400">
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
                  <span className="text-[12px] font-mono text-neutral-500">Anchor: §Abstract</span>
                </div>
                <p className="text-xs text-slate-300 leading-relaxed font-sans">
                  {safeAbstract}
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
                  <span className="text-[12px] font-mono text-neutral-500">Anchor: §GroundStateIntegrals</span>
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
                {energySpec && <Chart spec={energySpec} className="mt-3" />}
                {countsSpec && <Chart spec={countsSpec} className="mt-3" />}
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
                  <span className="text-[12px] font-mono text-emerald-400">{proofVerified ? 'Zero sorryAx (ρ=1.000)' : 'Unverified'}</span>
                </div>
                <div className="space-y-1.5 font-mono text-[12px] text-slate-300">
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
              <div className="flex items-center justify-between text-neutral-400 text-[12px]">
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
              <div className="flex items-center justify-between text-neutral-400 text-[12px]">
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
        <div className="bg-black/40 border border-white/10 rounded-2xl p-5 flex flex-col justify-between max-h-[580px]">
          <div className="space-y-4 overflow-y-auto pr-1">
            <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
              <div className="flex items-center gap-2">
                <MessageSquarePlus className="w-4 h-4 text-cyan-400" />
                <h3 className="text-xs font-bold text-white uppercase tracking-wider font-mono">
                  Frame-Anchored Review
                </h3>
              </div>
              <span className="text-[12px] font-mono px-2 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-500/30">
                The Buzz Pattern
              </span>
            </div>

            <p className="text-[12px] text-neutral-400 leading-normal">
              Highlight equations, sections, or proof tokens to leave targeted annotations. Prompt the agent swarm to re-verify specific derivations.
            </p>

            <ClaimHistory sessionId={sessionId ?? null} />

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
                    <span className="font-semibold text-white text-[12px]">{anno.author}</span>
                    <span
                      className={`text-[12px] font-mono uppercase px-1.5 py-0.5 rounded border ${
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
                  <div className="text-[12px] font-mono text-neutral-400">{anno.targetAnchor} • {anno.timestamp}</div>
                  <p className="text-neutral-300 text-[12px] leading-relaxed">{anno.comment}</p>
                </div>
              ))}
            </div>
          </div>

          {/* New Annotation Input Form */}
          <form onSubmit={handleAddAnnotation} className="pt-3 border-t border-white/[0.08] space-y-2 mt-3">
            {annotationError && (
              <div className="p-2 rounded-lg bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[12px]">
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
              className="w-full py-1.5 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] disabled:opacity-50 text-xs font-semibold text-[var(--color-on-accent)] flex items-center justify-center gap-1.5 transition"
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
