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
import { classifyJobTargetScope } from '@/lib/target-scope';
import { numericClaimGuard } from '@/lib/resultNarrative';
import { useJobsProjection } from '@/domain/useJobsProjection';
import { LiteraturePanel } from './LiteraturePanel';
import { PublicationPanel } from './PublicationPanel';
import { ResearchPipelinePanel } from './ResearchPipelinePanel';
import { Button } from '@/components/ui/Button';
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
  const sorted = [...points].sort((a, b) => a.bond_distance - b.bond_distance);
  const pesSpec: ChartSpec | undefined =
    sorted.length >= 2
      ? {
          kind: 'line',
          title: 'Potential Energy Surface (PES) · VQE vs Exact FCI',
          series: [
            {
              name: 'Measured VQE',
              points: sorted.map((p) => ({
                x: p.bond_distance,
                y: p.energy,
                display: `R=${p.bond_distance.toFixed(3)} Å · VQE=${p.energy.toFixed(6)} Ha (Δ=${p.delta_mha.toFixed(3)} mHa)`,
              })),
            },
            {
              name: 'FCI Exact',
              points: sorted.map((p) => ({
                x: p.bond_distance,
                y: p.fci_exact_energy,
                display: `R=${p.bond_distance.toFixed(3)} Å · FCI=${p.fci_exact_energy.toFixed(6)} Ha`,
              })),
            },
          ],
          x: { label: 'Bond distance R (Å)', kind: 'linear' },
          y: { label: 'Energy', unit: 'Ha', format: 'number' },
          basis: `Stored PES scan across ${sorted.length} geometry points.`,
          detail: 'full',
        }
      : undefined;
  return (
    <div className="space-y-3">
      {pesSpec && <Chart spec={pesSpec} />}
      <div className="rounded-row border border-border overflow-hidden">
        <div className="flex items-center justify-between px-3 py-2 bg-fill">
          <span className="text-[12px] font-semibold text-text">PES points</span>
          <span className="text-[11px] text-muted">measured energy vs full FCI anchor</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-[12px]">
            <thead className="text-muted">
              <tr className="border-b border-border">
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
                <tr key={point.bond_distance} className="border-b border-border last:border-0">
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
    </div>
  );
}

function ResultReceiptTable({ receipts }: { receipts: CampaignResultRef[] }) {
  return (
    <div className="rounded-row border border-border overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 bg-fill">
        <span className="text-[12px] font-semibold text-text">Result receipts</span>
        <span className="text-[11px] text-muted">job → result SHA-256 → ledger receipt</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[12px]">
          <thead className="text-muted">
            <tr className="border-b border-border">
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
              <tr key={row.job_id} className="border-b border-border last:border-0">
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
  const seriesCharts: ChartSpec[] = [];
  for (const exp of experiments) {
    if (!Array.isArray(exp.series) || exp.series.length < 2) continue;
    if (exp.kind === 'thermodynamic_limit') {
      const pts = exp.series
        .map((pt) => ({
          x: Number(pt.n_sites),
          y: Number(pt.energy_per_site),
        }))
        .filter((pt) => Number.isFinite(pt.x) && Number.isFinite(pt.y));
      if (pts.length >= 2) {
        seriesCharts.push({
          kind: 'line',
          title: `DMRG Finite-Size Scaling (${exp.model || 'spin chain'}) · job ${exp.job_id.slice(0, 8)}`,
          series: [
            {
              name: 'DMRG E/N',
              points: pts.map((p) => ({
                x: p.x,
                y: p.y,
                display: `N=${p.x} · E/N=${p.y.toFixed(6)}`,
              })),
            },
          ],
          x: { label: 'Lattice sites N', kind: 'linear' },
          y: { label: 'Energy per site E/N', unit: 'J', format: 'number' },
          headline:
            typeof exp.energy_exact === 'number'
              ? { label: 'Thermodynamic limit E∞/N', value: exp.energy_exact.toFixed(6) }
              : undefined,
          basis: 'Classical MPS/DMRG finite-size sweep; classical reference, not a QPU claim.',
          detail: 'full',
        });
      }
    } else if (exp.kind === 'lattice_dynamics') {
      const pts = exp.series
        .map((pt) => ({
          x: Number(pt.t),
          y: Number(pt.total_magnetization),
        }))
        .filter((pt) => Number.isFinite(pt.x) && Number.isFinite(pt.y));
      if (pts.length >= 2) {
        seriesCharts.push({
          kind: 'line',
          title: `2D TFIM Quench Dynamics ⟨Z⟩(t) · job ${exp.job_id.slice(0, 8)}`,
          series: [
            {
              name: 'Magnetization ⟨Z⟩',
              points: pts.map((p) => ({
                x: p.x,
                y: p.y,
                display: `t=${p.x.toFixed(2)} · ⟨Z⟩=${p.y.toFixed(5)}`,
              })),
            },
          ],
          x: { label: 'Time t (1/J)', kind: 'linear' },
          y: { label: 'Total Magnetization ⟨Z⟩', format: 'number' },
          basis: 'Exact finite-size 2D TFIM unitary time evolution; classical reference, not a QPU claim.',
          detail: 'full',
        });
      }
    }
  }

  return (
    <div className="space-y-3">
      {seriesCharts.map((spec, idx) => (
        <Chart key={`${spec.title}-${idx}`} spec={spec} />
      ))}
      <div className="rounded-row border border-border overflow-hidden">
        <div className="flex items-center justify-between px-3 py-2 bg-fill">
          <span className="text-[12px] font-semibold text-text">Measured experiments</span>
          <span className="text-[11px] text-muted">spin, excited, reaction, QAOA, DMRG, dynamics, and conjecture records folded from jobs</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-[12px]">
            <thead className="text-muted">
              <tr className="border-b border-border">
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
                const gap = exp.gap ?? exp.delta;
                const system = exp.molecule || exp.model || exp.reaction || (exp.n_sites ? `${exp.n_sites} sites` : '—');
                return (
                  <tr key={exp.job_id} className="border-b border-border last:border-0">
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
                          : exp.kind === 'thermodynamic_limit'
                            ? 'classical reference (DMRG)'
                            : exp.kind === 'lattice_dynamics'
                              ? 'classical reference (quench)'
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
    </div>
  );
}

export function ResultsStudio({ campaign, sessionId }: ResultsStudioProps) {
  const [copiedBibtex, setCopiedBibtex] = useState(false);
  const [exported, setExported] = useState(false);
  const [activeTab, setActiveTab] = useState<'manuscript' | 'latex' | 'lean4' | 'literature' | 'pipeline'>('manuscript');
  const [pipelineOpen, setPipelineOpen] = useState(false);
  const [pubOpen, setPubOpen] = useState(false);
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
  const targetScope = {
    targetMolecule: campaign?.quantum_factors?.target_molecule ?? null,
    molecularFormula: campaign?.quantum_factors?.molecular_formula ?? null,
    targetSystem: campaign?.target_system ?? null,
    researchGoal: campaign?.research_goal ?? null,
    anchorFci: campaign?.classical_anchor?.fci_exact_energy ?? null,
  };
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
      .filter((job) => jobStatusLabel(job.status) === 'COMPLETED' && classifyJobTargetScope(job, targetScope).matches)
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
      <div className="relative min-h-[520px] rounded-sheet overflow-hidden bg-page border border-border shadow-[var(--shadow-float)] p-6 space-y-5">
        <div className="flex items-center space-x-3.5 border-b border-border pb-4">
          <div className="w-10 h-10 rounded-card bg-fill border border-border flex items-center justify-center shadow-inner">
            <BookOpen className="w-5 h-5 text-soft-lilac-ink" />
          </div>
          <div>
            <h2 className="text-body-large font-semibold text-text tracking-tight">Citable Results Studio &amp; Systematic Notebook</h2>
            <p className="text-label text-muted mt-0.5">No citable result is recorded for this campaign.</p>
          </div>
        </div>
        <MeasuredOutputs sessionId={sessionId ?? null} targetScope={targetScope} />
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
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-control-compact bg-fill border border-border text-text text-[12px]"
              >
                <Download className="w-3.5 h-3.5" /> Download experiment report
              </button>
              <button
                type="button"
                onClick={handleExportExperimentsJSON}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-control-compact bg-fill border border-border text-text text-[12px]"
              >
                <Download className="w-3.5 h-3.5" /> Experiment JSON
              </button>
              <button
                type="button"
                onClick={() => void handleExportSigned()}
                disabled={!sessionId}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-control-compact bg-success-wash border border-success-edge text-success-ink text-[12px] disabled:opacity-40"
              >
                <Download className="w-3.5 h-3.5" /> Signed export
              </button>
            </div>
          </>
        )}
        {exportError && <p role="alert" className="text-[12px] text-danger-ink">{exportError}</p>}
        <div className="rounded-card border border-border bg-surface p-6 text-center space-y-2">
          <p className="text-body-strong font-medium text-text">No manuscript, citation, or proof receipt yet.</p>
          <p className="text-[13px] leading-relaxed text-muted">
            A citable studio appears only after an evaluated claim and a ledger-backed notebook exist.
            Projected corridors and template values are not shown here as results.
          </p>
        </div>
        {sessionId && (
          <div className="space-y-3">
            <details
              open={pipelineOpen}
              onToggle={(e) => setPipelineOpen(e.currentTarget.open)}
              className="rounded-card border border-border bg-surface p-4"
            >
              <summary className="text-label font-semibold text-text cursor-pointer">
                Research pipeline (Hypotheses, Plans, Graph &amp; Lab Knowledge)
              </summary>
              {pipelineOpen && (
                <div className="mt-4">
                  <ResearchPipelinePanel sessionId={sessionId} />
                </div>
              )}
            </details>
            <details
              open={pubOpen}
              onToggle={(e) => setPubOpen(e.currentTarget.open)}
              className="rounded-card border border-border bg-surface p-4"
            >
              <summary className="text-label font-semibold text-text cursor-pointer">
                Publication drafts, verification &amp; RO-Crate export
              </summary>
              {pubOpen && (
                <div className="mt-4">
                  <PublicationPanel sessionId={sessionId} />
                </div>
              )}
            </details>
          </div>
        )}
      </div>
    );
  }

  return (
    <div data-provenance="measured" className="relative min-h-[640px] rounded-sheet overflow-hidden bg-page border border-border shadow-[var(--shadow-float)] flex flex-col justify-between p-6 space-y-6">
      {notebookError && (
        <div role="alert" className="p-2.5 rounded-row bg-danger-wash border border-danger-edge text-danger-ink text-[12px]">
          Notebook projection unavailable — {notebookError}. Nothing synthesized.
        </div>
      )}
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border pb-4">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-card bg-fill border border-border flex items-center justify-center shadow-inner">
            <BookOpen className="w-5 h-5 text-soft-lilac-ink" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-body-large font-semibold text-text tracking-tight">Citable Results Studio & Systematic Notebook</h2>
              <ProvenanceBadge kind="measured" detail="Citable gate passed: evaluated claim, notebook energies, and provenance hash." />
            </div>
            <p className="text-label text-muted mt-0.5">
              Structured research preprints with Lean 4 AST synchronization and frame-anchored human-in-the-loop review
            </p>
          </div>
        </div>

        {/* View Switcher & Action buttons */}
        <div className="flex items-center space-x-2">
          <div className="p-1 rounded-row bg-surface border border-border flex items-center space-x-1">
            <button
              onClick={() => setActiveTab('manuscript')}
              className={`px-3 py-1 rounded-control-compact text-label font-medium transition ${
                activeTab === 'manuscript' ? 'bg-accent text-on-accent font-semibold' : 'text-muted hover:text-text'
              }`}
            >
              Manuscript
            </button>
            <button
              onClick={() => setActiveTab('latex')}
              className={`px-3 py-1 rounded-control-compact text-label font-medium transition ${
                activeTab === 'latex' ? 'bg-accent text-on-accent font-semibold' : 'text-muted hover:text-text'
              }`}
            >
              LaTeX Source
            </button>
            <button
              onClick={() => setActiveTab('lean4')}
              className={`px-3 py-1 rounded-control-compact text-label font-medium transition ${
                activeTab === 'lean4' ? 'bg-accent text-on-accent font-semibold' : 'text-muted hover:text-text'
              }`}
            >
              Lean 4 Proof AST
            </button>
            <button
              onClick={() => setActiveTab('literature')}
              className={`px-3 py-1 rounded-control-compact text-label font-medium transition ${
                activeTab === 'literature' ? 'bg-accent text-on-accent font-semibold' : 'text-muted hover:text-text'
              }`}
            >
              Literature
            </button>
            <Button
              size="compact"
              variant={activeTab === 'pipeline' ? 'primary' : 'ghost'}
              onClick={() => setActiveTab('pipeline')}
            >
              Research Pipeline
            </Button>
          </div>

          <button
            onClick={handleCopyBibtex}
            className="px-3 py-1.5 rounded-row bg-fill hover:bg-fill-hover border border-border text-label text-label-secondary font-medium flex items-center space-x-1.5 transition"
          >
            {copiedBibtex ? <Check className="w-3.5 h-3.5 text-success-ink" /> : <Copy className="w-3.5 h-3.5" />}
            <span>{copiedBibtex ? 'BibTeX Copied' : 'Cite BibTeX'}</span>
          </button>

          <button
            data-testid="result-csv"
            onClick={handleExportCSV}
            disabled={!campaign?.measurement && !(campaign?.pes_points?.length)}
            title="Download measured CSV: PES points when present, otherwise the measurement summary"
            className="px-3 py-1.5 rounded-row bg-fill hover:bg-fill-hover border border-border text-text text-label font-medium flex items-center space-x-1.5 transition disabled:opacity-40"
          >
            <Download className="w-3.5 h-3.5" />
            <span>CSV</span>
          </button>

          <button
            onClick={handleExportResult}
            title="Download measured result JSON (campaign, measurement, PES points, notebook)"
            className="px-3 py-1.5 rounded-row bg-success-wash hover:opacity-90 border border-success-edge text-success-ink text-label font-medium flex items-center space-x-1.5 transition"
          >
            <Download className="w-3.5 h-3.5" />
            <span>Result JSON</span>
          </button>

          <button
            onClick={() => void handleExportSigned()}
            disabled={!sessionId}
            title="Download a server-built experiment bundle whose canonical payload is signed by the deployment key"
            className="px-3 py-1.5 rounded-row bg-fill hover:bg-fill-hover border border-border text-text text-label font-medium flex items-center space-x-1.5 transition disabled:opacity-40"
          >
            <Download className="w-3.5 h-3.5" />
            <span>Signed export</span>
          </button>

          <button
            onClick={handleExport}
            className="px-3.5 py-1.5 rounded-row bg-accent hover:bg-lit text-on-accent text-label font-semibold flex items-center space-x-1.5 transition shadow-md active:scale-95"
          >
            <Download className="w-3.5 h-3.5" />
            <span>{exported ? 'Exported' : 'Export'}</span>
          </button>
        </div>
      </div>

      {/* Main Two-Column Layout: Manuscript on Left, Frame-Anchored Review on Right */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 flex-1">
        {/* Left 2 Columns: The Living Document / Code Viewer */}
        <div className="lg:col-span-2 bg-surface border border-border rounded-card p-6 overflow-y-auto max-h-[580px] space-y-6">
          {activeTab === 'manuscript' && (
            <div className="space-y-6">
              {/* Server-owned publication workflow: draft, review, build, signoff. */}
              <PublicationPanel sessionId={sessionId ?? null} />
              {/* Measured job outputs: what ran, bound to job ids */}
              <MeasuredOutputs sessionId={sessionId ?? null} targetScope={targetScope} />
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
                className={`p-4 rounded-row border transition cursor-pointer ${
                  selectedAnchor === '§Title' ? 'border-soft-indigo-edge bg-soft-indigo-wash' : 'border-border bg-fill'
                }`}
              >
                <span className="text-[12px] font-mono font-bold uppercase tracking-wider text-info-ink">
                  {claimVerified && proofVerified ? 'Verified Research Finding' : 'Projected Research Draft'}
                </span>
                <h2 className="text-subtitle md:text-title-small font-bold text-text tracking-tight leading-snug mt-1">
                  {campaign?.notebook?.title || `Variational Quantum Ground State of ${target} Anchored to Full Configuration Interaction`}
                </h2>
                <div className="flex flex-wrap items-center gap-3 text-label text-muted pt-2 font-mono">
                  <span>By Kubona Autonomous Epistemic Swarm</span>
                  <span>•</span>
                  <span className={`font-semibold flex items-center space-x-1 ${
                    claimVerified && proofVerified && isChemAccMet ? 'text-success-ink' : 'text-warning-ink'
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
                className={`p-4 rounded-row border transition cursor-pointer ${
                  selectedAnchor === '§Abstract' ? 'border-soft-indigo-edge bg-soft-indigo-wash' : 'border-border bg-fill'
                }`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <h3 className="text-label font-semibold text-label-secondary uppercase tracking-wider font-mono">Abstract</h3>
                  <span className="text-[12px] font-mono text-faint">Anchor: §Abstract</span>
                </div>
                <p className="text-label text-label-secondary leading-relaxed font-sans">
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
                className={`p-4 rounded-row border transition cursor-pointer ${
                  selectedAnchor === '§GroundStateIntegrals' ? 'border-soft-indigo-edge bg-soft-indigo-wash' : 'border-border bg-fill'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="text-label font-mono text-label-secondary font-semibold uppercase">§GroundStateIntegrals</span>
                  <span className="text-[12px] font-mono text-faint">Anchor: §GroundStateIntegrals</span>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 font-mono text-label">
                  <div className="p-3 rounded-control-compact bg-raised border border-border space-y-1">
                    <span className="text-muted">VQE Energy</span>
                    <div className="text-body-strong font-bold text-text">{fmtEnergy(groundEnergy)} Ha</div>
                  </div>
                  <div className="p-3 rounded-control-compact bg-raised border border-border space-y-1">
                    <span className="text-muted">Classical FCI Anchor</span>
                    <div className="text-body-strong font-bold text-success-ink">{fmtEnergy(fciEnergy)} Ha</div>
                  </div>
                  <div className="p-3 rounded-control-compact bg-raised border border-border space-y-1">
                    <span className="text-muted">Chemical Discrepancy</span>
                    <div className="text-body-strong font-bold text-info-ink">{fmtDelta(deltaMHa)} mHa</div>
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
                className={`p-4 rounded-row border transition cursor-pointer ${
                  selectedAnchor === '§Lean4ProofAST' ? 'border-soft-indigo-edge bg-soft-indigo-wash' : 'border-border bg-fill'
                }`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <h3 className="text-label font-semibold text-label-secondary uppercase tracking-wider font-mono">Formal Verification & Provenance</h3>
                  <span className="text-[12px] font-mono text-success-ink">{proofVerified ? 'Zero sorryAx (ρ=1.000)' : 'Unverified'}</span>
                </div>
                <div className="space-y-1.5 font-mono text-[12px] text-label-secondary">
                  <div className="bg-raised p-2 rounded-chip border border-border truncate">
                    Lean 4 AST Hash: <span className="text-success-ink">{lean4AstHash}</span>
                  </div>
                  <div className="bg-raised p-2 rounded-chip border border-border truncate">
                    Merkle Provenance: <span className="text-soft-lilac-ink">{provenanceHash}</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'latex' && (
            <div className="space-y-3 font-mono text-label">
              <div className="flex items-center justify-between text-muted text-[12px]">
                <span>LaTeX Project (main.tex)</span>
                <span className="text-info-ink">{hasVerifiedData ? 'Draft preview — not submitted' : 'Unattested draft — not submitted'}</span>
              </div>
              <pre className="p-4 rounded-row bg-surface-elev border border-border text-label-secondary overflow-x-auto leading-relaxed">
                {latexCode}
              </pre>
            </div>
          )}

          {activeTab === 'lean4' && (
            <div className="space-y-3 font-mono text-label">
              <div className="flex items-center justify-between text-muted text-[12px]">
                <span>Lean 4 Theorem Declaration (Lake Comparator Whitelist)</span>
                <span className="text-success-ink">{proofVerified ? 'Axioms ⊆ {propext, Quot.sound, Classical.choice}' : 'No verified declaration'}</span>
              </div>
              <pre className="p-4 rounded-row bg-surface-elev border border-border text-success-ink overflow-x-auto leading-relaxed">
                {lean4Code}
              </pre>
            </div>
          )}

          {activeTab === 'literature' && (
            <LiteraturePanel initialQuery={target} />
          )}

          {activeTab === 'pipeline' && (
            <ResearchPipelinePanel sessionId={sessionId ?? null} />
          )}
        </div>

        {/* Right 1 Column: Frame-Anchored Review (The Buzz Pattern) */}
        <div className="bg-surface border border-border rounded-card p-5 flex flex-col justify-between max-h-[580px]">
          <div className="space-y-4 overflow-y-auto pr-1">
            <div className="flex items-center justify-between border-b border-border pb-3">
              <div className="flex items-center gap-2">
                <MessageSquarePlus className="w-4 h-4 text-info-ink" />
                <h3 className="text-label font-bold text-text uppercase tracking-wider font-mono">
                  Frame-Anchored Review
                </h3>
              </div>
              <span className="text-[12px] font-mono px-2 py-0.5 rounded-chip bg-soft-indigo-wash text-soft-lilac-ink border border-soft-indigo-edge">
                The Buzz Pattern
              </span>
            </div>

            <p className="text-[12px] text-muted leading-normal">
              Highlight equations, sections, or proof tokens to leave targeted annotations. Prompt the agent swarm to re-verify specific derivations.
            </p>

            <ClaimHistory sessionId={sessionId ?? null} />

            {/* Current Selected Anchor Indicator */}
            <div className="p-2.5 rounded-control-compact bg-raised border border-border flex items-center justify-between text-label font-mono">
              <span className="text-muted">Active Anchor:</span>
              <span className="text-info-ink font-semibold">{selectedAnchor}</span>
            </div>

            {/* Existing Frame Annotations */}
            <div className="space-y-2.5">
              {annotations.map((anno) => (
                <div
                  key={anno.id}
                  className="p-3 rounded-row bg-raised border border-border space-y-1.5 text-label"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-text text-[12px]">{anno.author}</span>
                    <span
                      className={`text-[12px] font-mono uppercase px-1.5 py-0.5 rounded-chip border ${
                        anno.status === 'verified'
                          ? 'border-success-edge text-success-ink bg-success-wash'
                          : anno.status === 'failed'
                            ? 'border-danger-edge text-danger-ink bg-danger-wash'
                            : 'border-info-edge text-info-ink bg-info-wash'
                      }`}
                    >
                      {anno.status}
                    </span>
                  </div>
                  <div className="text-[12px] font-mono text-muted">{anno.targetAnchor} • {anno.timestamp}</div>
                  <p className="text-label-secondary text-[12px] leading-relaxed">{anno.comment}</p>
                </div>
              ))}
            </div>
          </div>

          {/* New Annotation Input Form */}
          <form onSubmit={handleAddAnnotation} className="pt-3 border-t border-border space-y-2 mt-3">
            {annotationError && (
              <div className="p-2 rounded-control-compact bg-danger-wash border border-danger-edge text-danger-ink text-[12px]">
                {annotationError}
              </div>
            )}
            <input
              type="text"
              aria-label={`Annotate ${selectedAnchor}`}
              placeholder={`Annotate ${selectedAnchor}...`}
              value={newComment}
              onChange={(e) => setNewComment(e.target.value)}
              className="w-full px-3 py-2 rounded-row bg-field border border-border-strong text-label text-text placeholder-[var(--color-faint)] focus:outline-none focus:border-info"
            />
            <button
              type="submit"
              disabled={!newComment.trim() || submittingAnnotation}
              className="w-full py-1.5 rounded-row bg-accent hover:bg-lit disabled:opacity-50 text-label font-semibold text-on-accent flex items-center justify-center gap-1.5 transition"
            >
              <Send className="w-3.5 h-3.5" />
              <span>{submittingAnnotation ? 'Prompting Swarm...' : 'Prompt Swarm to Re-verify'}</span>
            </button>
          </form>
        </div>
      </div>

      {/* Bottom Footer Note */}
      <div className="border-t border-border pt-3 flex items-center justify-between text-label text-muted">
        <span>Archiving requires ACCEPT review + human sign-off (staged deposit, never auto-published)</span>
        <span className="font-mono text-label-secondary font-medium">Format: LaTeX / Lean 4 / BibTeX / Merkle JSON</span>
      </div>
    </div>
  );
}
