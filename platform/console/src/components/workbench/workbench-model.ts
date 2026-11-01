import type { CampaignState } from '@/types';
import type { ChartSpec } from '@/components/data/chart';

export type WorkbenchCamera = { x: number; y: number; zoom: number };
export type WorkbenchObjectKind =
  | 'campaign'
  | 'hypothesis'
  | 'measurement'
  | 'circuit'
  | 'chart'
  | 'evidence'
  | 'proof'
  | 'manuscript';

export type WorkbenchObject = {
  id: string;
  kind: WorkbenchObjectKind;
  label: string;
  title: string;
  detail: string;
  position: { x: number; y: number };
  size: { width: number; height: number };
  facts: { label: string; value: string }[];
  chart?: ChartSpec;
};

export const DEFAULT_CAMERA: WorkbenchCamera = { x: 36, y: 36, zoom: 0.82 };
export const WORKBENCH_BOUNDS = { width: 1500, height: 760 };

function finite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

export function readCamera(raw: Record<string, unknown> | null): WorkbenchCamera {
  const workbench = raw?.workbench;
  if (!workbench || typeof workbench !== 'object') return DEFAULT_CAMERA;
  const camera = (workbench as { camera?: unknown }).camera;
  if (!camera || typeof camera !== 'object') return DEFAULT_CAMERA;
  const { x, y, zoom } = camera as Record<string, unknown>;
  if (!finite(x) || !finite(y) || !finite(zoom)) return DEFAULT_CAMERA;
  return { x, y, zoom: Math.min(2, Math.max(0.35, zoom)) };
}

export function cameraPayload(camera: WorkbenchCamera): Record<string, unknown> {
  return {
    version: 1,
    camera: {
      x: Math.round(camera.x * 100) / 100,
      y: Math.round(camera.y * 100) / 100,
      zoom: Math.round(camera.zoom * 1000) / 1000,
    },
  };
}

function value(raw: unknown, fallback = 'UNVERIFIED'): string {
  if (typeof raw === 'number' && Number.isFinite(raw)) return String(raw);
  if (typeof raw === 'string' && raw.trim()) return raw;
  return fallback;
}

function fixed(raw: unknown, digits: number, fallback = 'UNVERIFIED'): string {
  return typeof raw === 'number' && Number.isFinite(raw) ? raw.toFixed(digits) : fallback;
}

function energyChart(campaign: CampaignState): ChartSpec | undefined {
  const trajectory = campaign.trajectory ?? [];
  const points = trajectory
    .filter((step) => Number.isFinite(step.step) && Number.isFinite(step.energy))
    .map((step) => ({ x: step.step, y: step.energy, display: `${step.energy.toFixed(5)} Ha` }));
  if (points.length === 0) return undefined;
  const anchor = campaign.classical_anchor;
  const anchored = anchor?.anchor_computed === true && Number.isFinite(anchor.fci_exact_energy);
  const series: ChartSpec['series'] = [{ name: 'VQE energy', points }];
  if (anchored) {
    series.push({
      name: 'FCI anchor',
      points: points.map((point) => ({ x: point.x, y: anchor.fci_exact_energy })),
    });
  }
  return {
    kind: 'line',
    title: `Energy trajectory · ${campaign.target_system || 'campaign'}`,
    series,
    x: { label: 'Iteration', kind: 'linear' },
    y: { label: 'Energy', unit: 'Ha', format: 'number' },
    basis: anchored
      ? 'Measured trajectory against the computed FCI anchor.'
      : 'Measured trajectory; no computed FCI anchor, so no reference line.',
    detail: 'full',
  };
}

/**
 * Build display objects from the authoritative campaign. No content is
 * duplicated: this is a read-only projection; later slices replace the
 * card bodies with the domain stores' own objects.
 */
export function buildWorkbench(campaign: CampaignState | null | undefined): WorkbenchObject[] {
  if (!campaign) return [];
  const qf = campaign.quantum_factors;
  const anchor = campaign.classical_anchor;
  const epistemics = campaign.epistemics;
  const notebook = campaign.notebook;
  const anchored = anchor?.anchor_computed === true;
  const chart = energyChart(campaign);
  const objects: WorkbenchObject[] = [
    {
      id: 'campaign',
      kind: 'campaign',
      label: 'Campaign',
      title: value(qf?.target_molecule || campaign.target_system, 'Untitled campaign'),
      detail: campaign.research_goal || 'No research goal recorded.',
      position: { x: 0, y: 0 },
      size: { width: 380, height: 200 },
      facts: [
        { label: 'Target', value: value(campaign.target_system) },
        { label: 'Status', value: value(epistemics?.status) },
        { label: 'Hypothesis', value: value(campaign.hypothesis_id, 'not recorded') },
      ],
    },
    {
      id: 'hypothesis',
      kind: 'hypothesis',
      label: 'Hypothesis',
      title: value(epistemics?.statement, 'No evaluated claim yet'),
      detail: 'The claim is a projection of the campaign epistemics, not a chat message.',
      position: { x: 0, y: 240 },
      size: { width: 380, height: 200 },
      facts: [
        { label: 'State', value: value(epistemics?.status) },
        { label: 'Corroborated α', value: fixed(epistemics?.alpha_corroborated, 3) },
        { label: 'Contested β', value: fixed(epistemics?.beta_contested, 3) },
      ],
    },
    {
      id: 'circuit',
      kind: 'circuit',
      label: 'Circuit',
      title: value(qf?.ansatz_family, 'No ansatz recorded'),
      detail: `${value(qf?.fermion_mapping, 'mapping not recorded')} · ${value(qf?.recommended_backend, 'backend not chosen')}`,
      position: { x: 420, y: 0 },
      size: { width: 380, height: 200 },
      facts: [
        { label: 'Qubits', value: value(qf?.qubit_count) },
        { label: 'Depth', value: value(qf?.circuit_depth) },
        { label: 'Two-qubit gates', value: value(qf?.two_qubit_gates) },
      ],
    },
    {
      id: 'measurement',
      kind: 'measurement',
      label: 'Measurement',
      title: anchored ? 'Classical anchor computed' : 'No measured anchor',
      detail: anchored
        ? `${value(anchor?.method)} · Δ ${fixed(anchor?.discrepancy_mha, 2)} mHa`
        : 'The campaign carries no computed classical anchor; every value stays UNVERIFIED.',
      position: { x: 420, y: 240 },
      size: { width: 380, height: 200 },
      facts: [
        { label: 'FCI', value: anchored ? `${fixed(anchor?.fci_exact_energy, 5)} Ha` : 'UNVERIFIED' },
        { label: 'VQE', value: anchored ? `${fixed(anchor?.vqe_expectation_energy, 5)} Ha` : 'UNVERIFIED' },
        { label: 'Δ', value: anchored ? `${fixed(anchor?.discrepancy_mha, 2)} mHa` : 'UNVERIFIED' },
      ],
    },
    {
      id: 'evidence',
      kind: 'evidence',
      label: 'Evidence',
      title: value(epistemics?.status, 'No epistemic state'),
      detail: 'Corroboration, contest, and uncertainty are kept separate from a verdict.',
      position: { x: 840, y: 470 },
      size: { width: 380, height: 200 },
      facts: [
        { label: 'Unverified γ', value: fixed(epistemics?.gamma_unverified, 3) },
        { label: 'P(correlated)', value: fixed(epistemics?.p_correlated, 3) },
        { label: 'Sources', value: String(epistemics?.sources?.length ?? 0) },
      ],
    },
    {
      id: 'proof',
      kind: 'proof',
      label: 'Proof',
      title: notebook?.provenance_hash ? 'Provenance recorded' : 'No provenance receipt',
      detail: notebook?.provenance_hash || 'No ledger-backed notebook is attached to this campaign.',
      position: { x: 0, y: 480 },
      size: { width: 380, height: 190 },
      facts: [
        { label: 'Verdict', value: value(notebook?.epistemic_verdict) },
        { label: 'Chemical accuracy', value: notebook?.chemical_accuracy ? 'PASS' : 'not claimed' },
      ],
    },
    {
      id: 'manuscript',
      kind: 'manuscript',
      label: 'Manuscript',
      title: value(notebook?.title, 'No manuscript yet'),
      detail: 'The manuscript is a projection of the notebook; unverified drafts stay labelled.',
      position: { x: 420, y: 480 },
      size: { width: 380, height: 190 },
      facts: [
        { label: 'Sections', value: String(notebook?.sections?.length ?? 0) },
        { label: 'Δ', value: fixed(notebook?.delta_mha, 2) + ' mHa' },
      ],
    },
  ];
  if (chart) {
    objects.push({
      id: 'chart',
      kind: 'chart',
      label: 'Figure',
      title: chart.title,
      detail: chart.basis || 'Measured trajectory',
      position: { x: 840, y: 0 },
      size: { width: 620, height: 430 },
      facts: [],
      chart,
    });
  }
  return objects;
}
