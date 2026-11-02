import type { CampaignState, Job, Loop, Task } from '@/types';
import { jobStatusLabel } from '@/lib/job-status';
import type { ObligationRow } from '@/lib/api';
import type { ChartSpec } from '@/components/data/chart';

export type WorkbenchCamera = { x: number; y: number; zoom: number };
export type WorkbenchPositions = Record<string, { x: number; y: number }>;
export type WorkbenchSizes = Record<string, { width: number; height: number }>;
export type WorkbenchObjectKind =
  | 'campaign'
  | 'hypothesis'
  | 'measurement'
  | 'circuit'
  | 'chart'
  | 'evidence'
  | 'proof'
  | 'manuscript'
  | 'agent_run'
  | 'job'
  | 'table'
  | 'approval'
  | 'note'
  | 'molecule'
  | 'task';

export type WorkbenchLink = {
  id: string;
  source: string;
  target: string;
  kind: 'supports' | 'uses' | 'depends_on' | 'contradicts' | 'same_as' | 'thread';
};

export type WorkbenchArea = {
  id: string;
  title: string;
  rect: { x: number; y: number; width: number; height: number };
};

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
  /** Authoritative record this card projects; layout never owns content. */
  ref?: { domain: string; id: string };
};

export const DEFAULT_CAMERA: WorkbenchCamera = { x: 36, y: 36, zoom: 0.82 };
export const WORKBENCH_BOUNDS = { width: 1800, height: 1500 };

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
  return { x, y, zoom: Math.min(2.5, Math.max(0.25, zoom)) };
}

export function readPositions(raw: Record<string, unknown> | null): WorkbenchPositions {
  const workbench = raw?.workbench;
  if (!workbench || typeof workbench !== 'object') return {};
  const objects = (workbench as { objects?: unknown }).objects;
  if (!objects || typeof objects !== 'object') return {};
  const out: WorkbenchPositions = {};
  for (const [id, value] of Object.entries(objects as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue;
    const { x, y } = value as Record<string, unknown>;
    if (finite(x) && finite(y) && Math.abs(x) <= 1e6 && Math.abs(y) <= 1e6) {
      out[id] = { x: Math.round(x * 100) / 100, y: Math.round(y * 100) / 100 };
    }
  }
  return out;
}

export function readSizes(raw: Record<string, unknown> | null): WorkbenchSizes {
  const workbench = raw?.workbench;
  if (!workbench || typeof workbench !== 'object') return {};
  const objects = (workbench as { objects?: unknown }).objects;
  if (!objects || typeof objects !== 'object') return {};
  const out: WorkbenchSizes = {};
  for (const [id, value] of Object.entries(objects as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue;
    const { width, height } = value as Record<string, unknown>;
    if (finite(width) && finite(height) && width >= 160 && width <= 4096 && height >= 72 && height <= 4096) {
      out[id] = { width: Math.round(width * 100) / 100, height: Math.round(height * 100) / 100 };
    }
  }
  return out;
}

export function workbenchPayload(
  camera: WorkbenchCamera,
  positions: WorkbenchPositions,
  sizes: WorkbenchSizes = {},
): Record<string, unknown> {
  return {
    version: 1,
    camera: {
      x: Math.round(camera.x * 100) / 100,
      y: Math.round(camera.y * 100) / 100,
      zoom: Math.round(camera.zoom * 1000) / 1000,
    },
    objects: Object.fromEntries(
      Object.entries(positions).map(([id, position]) => [
        id,
        {
          x: Math.round(position.x * 100) / 100,
          y: Math.round(position.y * 100) / 100,
          ...(sizes[id]
            ? {
                width: Math.round(sizes[id]!.width * 100) / 100,
                height: Math.round(sizes[id]!.height * 100) / 100,
              }
            : {}),
        },
      ]),
    ),
  };
}

function defaultRef(object: WorkbenchObject, campaign: CampaignState): { domain: string; id: string } {
  const session = campaign.session_id;
  const notebook = campaign.notebook;
  switch (object.kind) {
    case 'campaign':
      return { domain: 'campaign', id: session };
    case 'proof':
      return { domain: 'proof', id: notebook?.provenance_hash || `${session}:proof` };
    case 'manuscript':
    case 'note':
      return { domain: 'notebook', id: notebook?.id || `${session}:notebook` };
    default:
      return { domain: 'campaign', id: `${session}:${object.kind}` };
  }
}

function value(raw: unknown, fallback = 'UNVERIFIED'): string {
  if (typeof raw === 'number' && Number.isFinite(raw)) return String(raw);
  if (typeof raw === 'string' && raw.trim()) return raw;
  return fallback;
}

function typeValue(raw: unknown): string {
  if (typeof raw === 'number' && Number.isFinite(raw)) return `$${raw.toFixed(2)}`;
  return value(raw);
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
export function buildWorkbench(
  campaign: CampaignState | null | undefined,
  loops: Loop[] = [],
  jobs: Job[] = [],
  obligations: ObligationRow[] = [],
  tasks: Task[] = [],
  sessionSpendUsd?: number,
): WorkbenchObject[] {
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
        { label: 'Anchor', value: anchored ? 'computed' : 'UNVERIFIED' },
        { label: 'Spend', value: typeValue(sessionSpendUsd) },
        { label: 'Est. spend', value: typeValue(qf?.estimated_spend_usd) },
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
  loops.slice(0, 160).forEach((loop, index) => {
    const state = String(loop.status || 'unknown');
    objects.push({
      id: `agent-run-${loop.id}`,
      kind: 'agent_run',
      ref: { domain: 'loop', id: loop.id },
      label: 'Agent run',
      title: `Run ${loop.id.slice(0, 8)}`,
      detail: state === 'running' ? 'Agent is working; progress comes from the server record.' : 'Settled or paused; no live claim without a running record.',
      position: { x: 0, y: 720 + index * 210 },
      size: { width: 380, height: 180 },
      facts: [
        { label: 'State', value: state },
        { label: 'Step', value: `${loop.step_count} / ${loop.max_steps}` },
        { label: 'Diverged', value: loop.diverged ? 'YES' : 'no' },
      ],
    });
  });
  jobs.slice(0, 160).forEach((job, index) => {
    let params: Record<string, unknown> = {};
    try {
      params = typeof job.params_json === 'string' ? JSON.parse(job.params_json) as Record<string, unknown> : job.params_json || {};
    } catch {
      params = {};
    }
    objects.push({
      id: `job-${job.id}`,
      kind: 'job',
      ref: { domain: 'job', id: job.id },
      label: 'Job',
      title: job.job_type || 'Unnamed job',
      detail: `Record ${job.id.slice(0, 8)} · ${job.compiler_used || 'compiler not recorded'}`,
      position: { x: 840, y: 720 + index * 210 },
      size: { width: 380, height: 180 },
      facts: [
        { label: 'Status', value: jobStatusLabel(job.status) },
        { label: 'Backend', value: value(params.requested_backend ?? params.backend, 'not recorded') },
        { label: 'Shots', value: value(params.shots, 'not recorded') },
        { label: 'Created', value: job.created_at || 'not recorded' },
        { label: 'Completed', value: job.completed_at || 'not yet' },
        { label: 'Result', value: job.result_json ? 'recorded' : 'not recorded' },
      ],
    });
  });
  objects.push({
    id: 'quantum-factors',
    kind: 'table',
    label: 'Table',
    title: 'Quantum factors',
    detail: 'Exact campaign fields as recorded; no derived values.',
    position: { x: 1260, y: 0 },
    size: { width: 380, height: 430 },
    facts: [
      { label: 'Molecule', value: value(qf?.target_molecule) },
      { label: 'Mapping', value: value(qf?.fermion_mapping) },
      { label: 'Qubits', value: value(qf?.qubit_count) },
      { label: 'Depth', value: value(qf?.circuit_depth) },
      { label: 'Two-qubit gates', value: value(qf?.two_qubit_gates) },
      { label: 'Backend', value: value(qf?.recommended_backend) },
      { label: 'Estimated spend', value: typeValue(qf?.estimated_spend_usd) },
    ],
  });
  obligations.slice(0, 32).forEach((obligation, index) => {
    objects.push({
      id: `approval-${obligation.id}`,
      kind: 'approval',
      ref: { domain: 'obligation', id: obligation.id },
      label: 'Approval',
      title: `${obligation.subject_kind} · ${obligation.subject_id.slice(0, 12)}`,
      detail: `${obligation.owner} · ${obligation.disposition}`,
      position: { x: 1260, y: 470 + index * 210 },
      size: { width: 380, height: 180 },
      facts: [
        { label: 'Close ready', value: obligation.close_ready ? 'YES' : 'no' },
        { label: 'Cases', value: obligation.required_cases.length ? obligation.required_cases.join(', ') : 'none' },
        { label: 'Authorities', value: obligation.required_authorities.length ? obligation.required_authorities.join(', ') : 'none' },
      ],
    });
  });
  tasks.slice(0, 160).forEach((task, index) => {
    objects.push({
      id: `task-${task.id}`,
      kind: 'task',
      ref: { domain: 'task', id: task.id },
      label: 'Task',
      title: task.title,
      detail: task.source_domain ? `${task.source_domain} · ${task.source_id}` : 'No source record',
      position: { x: 840, y: 1140 + index * 210 },
      size: { width: 380, height: 180 },
      facts: [
        { label: 'Status', value: task.status },
        { label: 'Source', value: task.source_domain || 'none' },
        { label: 'Updated', value: task.updated_at || 'not recorded' },
      ],
    });
  });
  objects.push({
    id: 'molecule',
    kind: 'molecule',
    label: 'Molecule',
    title: value(qf?.target_molecule || campaign.target_system, 'No molecule'),
    detail: `${value(qf?.molecular_formula, 'formula not recorded')} · bond ${fixed(qf?.bond_distance, 4)} Å`,
    position: { x: 0, y: 1140 },
    size: { width: 380, height: 200 },
    facts: [
      { label: 'Active electrons', value: value(qf?.active_electrons) },
      { label: 'Active orbitals', value: value(qf?.active_orbitals) },
      { label: 'Anchor', value: anchored ? `${fixed(anchor?.fci_exact_energy, 5)} Ha` : 'UNVERIFIED' },
    ],
  });
  if (notebook?.sections?.length) {
    objects.push({
      id: 'notebook-outline',
      kind: 'note',
      label: 'Note',
      title: notebook.title || 'Notebook outline',
      detail: 'Recorded section titles; the manuscript card owns the prose.',
      position: { x: 1260, y: 890 },
      size: { width: 380, height: 190 },
      facts: notebook.sections.slice(0, 5).map((section, index) => ({
        label: `Section ${index + 1}`,
        value: section,
      })),
    });
  }
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
  return objects.map((object) => ({ ...object, ref: object.ref ?? defaultRef(object, campaign) }));
}

export type WorkbenchDocument = {
  version: 2;
  session_id: string;
  objects: {
    id: string;
    kind: WorkbenchObjectKind;
    position: { x: number; y: number };
    size: { width: number; height: number };
    ref: { domain: string; id: string };
    area?: string | null;
  }[];
  links: WorkbenchLink[];
  areas: WorkbenchArea[];
  hidden: string[];
  camera: WorkbenchCamera;
  selected: [];
};

export function readWorkbenchDocument(raw: Record<string, unknown> | null): {
  camera: WorkbenchCamera;
  objects: { id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[];
  positions: WorkbenchPositions;
  sizes: WorkbenchSizes;
  links: WorkbenchLink[];
  areas: WorkbenchArea[];
  assignments: Record<string, string>;
  hidden: string[];
} {
  const camera = readCamera({ workbench: raw });
  const objects = Array.isArray(raw?.objects) ? raw.objects : [];
  const positions: WorkbenchPositions = {};
  const sizes: WorkbenchSizes = {};
  const links: WorkbenchLink[] = Array.isArray(raw?.links) ? (raw.links as WorkbenchLink[]) : [];
  const areas: WorkbenchArea[] = Array.isArray(raw?.areas) ? (raw.areas as WorkbenchArea[]) : [];
  const assignments: Record<string, string> = {};
  const hidden: string[] = Array.isArray(raw?.hidden) ? raw!.hidden.filter((id): id is string => typeof id === 'string') : [];
  const layoutObjects: { id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[] = [];
  for (const item of objects) {
    if (!item || typeof item !== 'object') continue;
    const object = item as { id?: unknown; position?: unknown; size?: unknown };
    if (typeof object.id !== 'string') continue;
    const area = (object as { area?: unknown }).area;
    if (typeof area === 'string' && area) assignments[object.id] = area;
    if (object.position && typeof object.position === 'object') {
      const { x, y } = object.position as Record<string, unknown>;
      if (finite(x) && finite(y)) positions[object.id] = { x, y };
    }
    if (object.size && typeof object.size === 'object') {
      const { width, height } = object.size as Record<string, unknown>;
      if (finite(width) && finite(height) && width > 0 && height > 0) sizes[object.id] = { width, height };
    }
    const ref = (item as { ref?: unknown }).ref;
    const kind = (item as { kind?: unknown }).kind;
    if (ref && typeof ref === 'object') {
      const domain = (ref as { domain?: unknown }).domain;
      const refId = (ref as { id?: unknown }).id;
      if (typeof domain === 'string' && typeof refId === 'string') {
        layoutObjects.push({
          id: object.id,
          kind: typeof kind === 'string' ? kind as WorkbenchObjectKind : 'note',
          ref: { domain, id: refId },
          position: positions[object.id] ?? { x: 0, y: 0 },
          size: sizes[object.id] ?? { width: 380, height: 180 },
        });
      }
    }
  }
  return { camera, objects: layoutObjects, positions, sizes, links, areas, assignments, hidden };
}

export function buildWorkbenchLinks(objects: WorkbenchObject[]): WorkbenchLink[] {
  const has = new Set(objects.map((object) => object.id));
  const pairs: [string, string, WorkbenchLink['kind']][] = [
    ['campaign', 'hypothesis', 'supports'],
    ['campaign', 'circuit', 'uses'],
    ['campaign', 'measurement', 'supports'],
    ['circuit', 'measurement', 'depends_on'],
    ['measurement', 'evidence', 'supports'],
    ['evidence', 'proof', 'supports'],
    ['proof', 'manuscript', 'depends_on'],
    ['measurement', 'chart', 'uses'],
  ];
  return pairs
    .filter(([source, target]) => has.has(source) && has.has(target))
    .map(([source, target, kind]) => ({ id: `${source}-${target}`, source, target, kind }));
}

export function workbenchDocument(
  sessionId: string,
  objects: WorkbenchObject[],
  positions: WorkbenchPositions,
  sizes: WorkbenchSizes,
  camera: WorkbenchCamera,
  links: WorkbenchLink[] = buildWorkbenchLinks(objects),
  areas: WorkbenchArea[] = [],
  assignments: Record<string, string> = {},
  hidden: string[] = [],
): WorkbenchDocument {
  return {
    version: 2,
    session_id: sessionId,
    objects: objects.map((object) => ({
      id: object.id,
      kind: object.kind,
      position: positions[object.id] ?? object.position,
      size: sizes[object.id] ?? object.size,
      ref: object.ref ?? { domain: 'campaign', id: sessionId },
      area: assignments[object.id] ?? null,
    })),
    links,
    areas,
    hidden,
    camera,
    selected: [],
  };
}
