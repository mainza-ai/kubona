import type { ProvenanceKind } from '@/components/common/ProvenanceBadge';
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

export type WorkbenchComparisonData = {
  columns: string[];
  rows: { label: string; values: string[] }[];
};

export type WorkbenchRunItem = {
  id: string;
  label: string;
  status: string;
  metric?: string;
  at?: string;
};

export type WorkbenchObjectMeta = {
  subjectKind?: string;
  subjectId?: string;
  sourceDomain?: string;
  sourceId?: string;
  jobId?: string;
  revises?: string[];
  runCount?: number;
  runs?: WorkbenchRunItem[];
  status?: string;
  activeFlow?: boolean;
  hero?: { value: string; label: string; tone?: 'neutral' | 'positive' | 'warning' };
  callout?: { tone: 'supports' | 'contested' | 'info'; summary: string };
  code?: { language: string; content: string; label?: string };
  comparison?: WorkbenchComparisonData;
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
  /** What the card's values are and are not; always shown with the facts. */
  basis?: string;
  /** Source references recorded for this object; URLs are validated before render. */
  sources?: { label: string; url?: string }[];
  /** Where the numbers on this card come from. */
  provenance?: ProvenanceKind;
  /** Authoritative record this card projects; layout never owns content. */
  ref?: { domain: string; id: string };
  /** Transient structural metadata for linking, folding, and specialized card morphologies. */
  meta?: WorkbenchObjectMeta;
};

export type WorkbenchPins = Record<string, boolean>;
export type WorkbenchRect = { x: number; y: number; width: number; height: number };

export const WORKBENCH_GRID = 8;
export const WORKBENCH_CLEARANCE = 16;
export const DEFAULT_CAMERA: WorkbenchCamera = { x: 36, y: 36, zoom: 0.82 };
export const WORKBENCH_BOUNDS = { width: 1800, height: 1500 };

export function snapToGrid(value: number, step = WORKBENCH_GRID): number {
  const snapped = Math.round(value / step) * step;
  return snapped === 0 ? 0 : snapped;
}

export function boardPin(dx: number, dy: number): boolean {
  return Math.hypot(dx, dy) > 2;
}

export function rectsOverlap(a: WorkbenchRect, b: WorkbenchRect, gap = 0): boolean {
  return (
    a.x < b.x + b.width + gap &&
    a.x + a.width + gap > b.x &&
    a.y < b.y + b.height + gap &&
    a.y + a.height + gap > b.y
  );
}

export function intersectionArea(a: WorkbenchRect, b: WorkbenchRect): number {
  const xOverlap = Math.max(0, Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x));
  const yOverlap = Math.max(0, Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y));
  return xOverlap * yOverlap;
}

export function totalOverlapArea(objects: readonly Pick<WorkbenchObject, 'position' | 'size'>[]): number {
  let area = 0;
  for (let i = 0; i < objects.length; i += 1) {
    const a: WorkbenchRect = { ...objects[i].position, ...objects[i].size };
    for (let j = i + 1; j < objects.length; j += 1) {
      const b: WorkbenchRect = { ...objects[j].position, ...objects[j].size };
      area += intersectionArea(a, b);
    }
  }
  return Math.round(area * 100) / 100;
}

export function freeSpot(
  box: { width: number; height: number },
  preferred: { x: number; y: number },
  obstacles: readonly WorkbenchRect[],
  clearance = WORKBENCH_CLEARANCE,
  bounds?: { minX?: number; maxX?: number; minY?: number; maxY?: number },
): { x: number; y: number } {
  const clampPoint = (px: number, py: number) => ({
    x: bounds?.minX != null || bounds?.maxX != null
      ? Math.max(bounds.minX ?? -Infinity, Math.min(bounds.maxX ?? Infinity, px))
      : px,
    y: bounds?.minY != null || bounds?.maxY != null
      ? Math.max(bounds.minY ?? -Infinity, Math.min(bounds.maxY ?? Infinity, py))
      : py,
  });
  const base = clampPoint(snapToGrid(preferred.x), snapToGrid(preferred.y));
  const fits = (x: number, y: number) => {
    const rect: WorkbenchRect = { x, y, width: box.width, height: box.height };
    return !obstacles.some((obstacle) => rectsOverlap(rect, obstacle, clearance));
  };
  if (fits(base.x, base.y)) return base;

  const step = WORKBENCH_GRID * 3;
  const dirs: readonly [number, number][] = [
    [0, 1], [1, 0], [0, -1], [-1, 0],
    [1, 1], [-1, 1], [1, -1], [-1, -1],
  ];
  for (let ring = 1; ring <= 28; ring += 1) {
    for (const [dx, dy] of dirs) {
      const candidate = clampPoint(base.x + dx * ring * step, base.y + dy * ring * step);
      if (fits(candidate.x, candidate.y)) return candidate;
    }
    for (let offset = -ring + 1; offset < ring; offset += 1) {
      const ringCandidates: [number, number][] = [
        [offset, ring],
        [offset, -ring],
        [ring, offset],
        [-ring, offset],
      ];
      for (const [ox, oy] of ringCandidates) {
        const candidate = clampPoint(base.x + ox * step, base.y + oy * step);
        if (fits(candidate.x, candidate.y)) return candidate;
      }
    }
  }
  const bottom = obstacles.reduce((maxY, o) => Math.max(maxY, o.y + o.height), 0);
  return clampPoint(base.x, snapToGrid(bottom + clearance));
}

export function dropSpot(
  moved: WorkbenchRect,
  obstacles: readonly WorkbenchRect[],
  clearance = WORKBENCH_CLEARANCE,
  bounds?: { minX?: number; maxX?: number; minY?: number; maxY?: number },
): { x: number; y: number } {
  return freeSpot({ width: moved.width, height: moved.height }, { x: moved.x, y: moved.y }, obstacles, clearance, bounds);
}

export function layoutBounds(objects: readonly Pick<WorkbenchObject, 'position' | 'size'>[]): WorkbenchRect {
  if (objects.length === 0) return { x: 0, y: 0, width: WORKBENCH_BOUNDS.width, height: WORKBENCH_BOUNDS.height };
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const object of objects) {
    minX = Math.min(minX, object.position.x);
    minY = Math.min(minY, object.position.y);
    maxX = Math.max(maxX, object.position.x + object.size.width);
    maxY = Math.max(maxY, object.position.y + object.size.height);
  }
  return { x: minX, y: minY, width: Math.max(1, maxX - minX), height: Math.max(1, maxY - minY) };
}

export function contractPlacement(
  objects: readonly Pick<WorkbenchObject, 'position' | 'size'>[],
  padding = 360,
): { minX: number; minY: number; maxX: number; maxY: number; width: number; height: number } {
  const box = layoutBounds(objects);
  const minX = Math.min(-400, Math.floor(box.x - padding));
  const minY = Math.min(-400, Math.floor(box.y - padding));
  const maxX = Math.max(WORKBENCH_BOUNDS.width, Math.ceil(box.x + box.width + padding));
  const maxY = Math.max(WORKBENCH_BOUNDS.height, Math.ceil(box.y + box.height + padding));
  return { minX, minY, maxX, maxY, width: maxX - Math.min(0, minX), height: maxY - Math.min(0, minY) };
}

function finite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

export function readCamera(raw: Record<string, unknown> | null): WorkbenchCamera {
  const workbench = raw?.workbench;
  const source = workbench && typeof workbench === 'object' ? workbench : raw;
  if (!source || typeof source !== 'object') return DEFAULT_CAMERA;
  const camera = (source as { camera?: unknown }).camera;
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

// Negative values are the explicit not-evaluated sentinel for credibility
// amplitudes and derived campaign quantities. Never render them as 0.000.
function notEvaluated(raw: unknown, digits: number): string {
  return typeof raw === 'number' && Number.isFinite(raw) && raw >= 0
    ? raw.toFixed(digits)
    : 'not evaluated';
}

function recordedSources(sources: string[] | undefined): { label: string; url?: string }[] {
  return (sources ?? [])
    .filter((source) => typeof source === 'string' && source.trim().length > 0)
    .slice(0, 4)
    .map((source) => {
      const label = source.trim();
      try {
        const parsed = new URL(label);
        if (parsed.protocol === 'http:' || parsed.protocol === 'https:') return { label, url: label };
      } catch {
        // A non-URL source reference stays visible as text, never as a link.
      }
      return { label };
    });
}

function energyChart(campaign: CampaignState): ChartSpec | undefined {
  const trajectory = campaign.trajectory ?? [];
  const points = trajectory
    .filter((step) => Number.isFinite(step.step) && Number.isFinite(step.energy))
    .map((step) => ({ x: step.step, y: step.energy, display: `${step.energy.toFixed(5)} Ha` }));
  if (points.length === 0) return undefined;
  const anchor = campaign.classical_anchor;
  const anchorEnergy = anchor?.fci_exact_energy;
  const anchored = anchor?.anchor_computed === true && typeof anchorEnergy === 'number' && Number.isFinite(anchorEnergy);

  const energies = points.map((p) => p.y).sort((a, b) => a - b);
  const medianEnergy = energies[Math.floor(energies.length / 2)]!;
  const refEnergy = anchored && anchorEnergy != null ? anchorEnergy : medianEnergy;
  const outlierThreshold = Math.max(1.5, Math.abs(refEnergy) * 0.8);
  const inScope = points.filter((p) => Math.abs(p.y - refEnergy) <= outlierThreshold).map((p) => p.y);
  const outlierCount = points.length - inScope.length;
  const shouldClamp = outlierCount > 0 && inScope.length >= 2;
  let ySpec: ChartSpec['y'] = { label: 'Energy', unit: 'Ha', format: 'number' };
  if (shouldClamp) {
    const low = Math.min(...inScope, anchored && anchorEnergy != null ? anchorEnergy : Infinity);
    const high = Math.max(...inScope, anchored && anchorEnergy != null ? anchorEnergy : -Infinity);
    const pad = Math.max(0.005, (high - low) * 0.15);
    ySpec = { ...ySpec, min: low - pad, max: high + pad, clamp: true };
  }

  const series: ChartSpec['series'] = [{ name: 'VQE energy', points }];
  if (anchored && anchorEnergy != null) {
    series.push({
      name: 'FCI anchor',
      points: points.map((point) => ({ x: point.x, y: anchorEnergy })),
    });
  }
  const baseBasis = anchored
    ? 'Measured trajectory against the computed FCI anchor.'
    : 'Measured trajectory; no computed FCI anchor, so no reference line.';
  return {
    kind: 'line',
    title: `Energy trajectory · ${campaign.target_system || 'campaign'}`,
    series,
    x: { label: 'Iteration', kind: 'linear' },
    y: ySpec,
    basis: shouldClamp
      ? `${baseBasis} ${outlierCount} off-scale step${outlierCount === 1 ? '' : 's'} flagged; framed to target band (raw values preserved in Exact values).`
      : baseBasis,
    detail: 'full',
  };
}

export function estimateCardHeight({
  detail = '',
  factsCount,
  basis = '',
  sourcesCount = 0,
  hasChart = false,
  hasHero = false,
  hasCode = false,
  comparisonRows = 0,
  runsCount = 0,
  minHeight = 160,
}: {
  detail?: string;
  factsCount: number;
  basis?: string;
  sourcesCount?: number;
  hasChart?: boolean;
  hasHero?: boolean;
  hasCode?: boolean;
  comparisonRows?: number;
  runsCount?: number;
  minHeight?: number;
}): number {
  const detailLines = Math.max(1, Math.ceil(detail.length / 42));
  const visibleFacts = Math.min(factsCount, 6);
  const expandRow = factsCount > 6 ? 26 : 0;
  const factsHeight = visibleFacts > 0 ? visibleFacts * 24 + 10 + expandRow : 0;
  const basisHeight = basis ? Math.ceil(basis.length / 44) * 18 + 10 : 0;
  const sourcesHeight = sourcesCount > 0 ? Math.min(sourcesCount, 4) * 18 + 8 : 0;
  const chartHeight = hasChart ? 260 : 0;
  const heroHeight = hasHero ? 56 : 0;
  const codeHeight = hasCode ? 168 : 0;
  const comparisonHeight = comparisonRows > 0 ? 48 + Math.min(comparisonRows, 5) * 28 : 0;
  const runsHeight = runsCount > 0 ? Math.min(runsCount, 4) * 26 + 18 : 0;
  return snapToGrid(Math.max(
    minHeight,
    68 + detailLines * 19 + heroHeight + factsHeight + basisHeight + sourcesHeight + chartHeight + codeHeight + comparisonHeight + runsHeight + 24,
  ));
}

export function reflowUnpinnedObjects(
  objects: readonly WorkbenchObject[],
  pins: WorkbenchPins,
  measuredSizes: WorkbenchSizes,
  gap = 20,
): WorkbenchObject[] {
  if (objects.length <= 1) return [...objects];
  const byId = new Map<string, WorkbenchObject>();
  for (const object of objects) {
    const measured = measuredSizes[object.id];
    const effectiveHeight = !pins[object.id] && measured
      ? Math.max(object.size.height, snapToGrid(measured.height))
      : object.size.height;
    byId.set(object.id, effectiveHeight !== object.size.height
      ? { ...object, size: { ...object.size, height: effectiveHeight } }
      : object);
  }
  const obstacles: WorkbenchRect[] = [];
  const unpinned: WorkbenchObject[] = [];
  for (const object of byId.values()) {
    if (pins[object.id]) {
      obstacles.push({
        x: object.position.x,
        y: object.position.y,
        width: object.size.width,
        height: object.size.height,
      });
    } else {
      unpinned.push(object);
    }
  }
  unpinned.sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x);
  const shifted = new Map<string, { x: number; y: number }>();
  for (const item of unpinned) {
    const x = item.position.x;
    const width = item.size.width;
    const height = item.size.height;
    let y = item.position.y;
    let dirty = true;
    let guard = 0;
    while (dirty && guard <= obstacles.length) {
      dirty = false;
      guard += 1;
      for (const obs of obstacles) {
        if (rectsOverlap({ x, y, width, height }, obs, gap)) {
          y = snapToGrid(obs.y + obs.height + gap);
          dirty = true;
        }
      }
    }
    if (y !== item.position.y) shifted.set(item.id, { x, y });
    obstacles.push({ x, y, width, height });
  }
  if (shifted.size === 0 && Object.keys(measuredSizes).length === 0) return [...objects];
  return objects.map((object) => {
    const current = byId.get(object.id) ?? object;
    const pos = shifted.get(object.id);
    return pos ? { ...current, position: pos } : current;
  });
}

const REPEATABLE_JOB_TYPES = new Set([
  'thermodynamic_limit',
  'circuit_run',
  'spin_chain',
  'vqe_repeats',
]);

const RUN_HISTORY_CAP = 3;

function parseJsonRecord(raw: unknown): Record<string, unknown> {
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Record<string, unknown>;
  if (typeof raw === 'string' && raw.trim()) {
    try {
      const parsed = JSON.parse(raw) as unknown;
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed as Record<string, unknown>;
    } catch {
      return {};
    }
  }
  return {};
}

function normalizeFactKey(label: string): string {
  return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}

export function factKeyOverlapRatio(
  aFacts: readonly { label: string }[],
  bFacts: readonly { label: string }[],
): number {
  const aKeys = new Set(aFacts.map((f) => normalizeFactKey(f.label)).filter(Boolean));
  const bKeys = new Set(bFacts.map((f) => normalizeFactKey(f.label)).filter(Boolean));
  if (aKeys.size === 0 || bKeys.size === 0) return 0;
  let shared = 0;
  for (const key of aKeys) {
    if (bKeys.has(key)) shared += 1;
  }
  return shared / Math.min(aKeys.size, bKeys.size);
}

function dedupHeadline(text: string, headline: string): string {
  const clean = text.trim();
  const target = headline.trim();
  if (!clean || !target || clean === target) return clean;
  const escaped = target.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const stripped = clean
    .replace(new RegExp(`^${escaped}\\s*[·:\\-–—]\\s*`, 'i'), '')
    .replace(new RegExp(`\\s*[·:\\-–—]\\s*${escaped}$`, 'i'), '')
    .trim();
  return stripped || clean;
}

function jobResultNumber(result: Record<string, unknown>, keys: readonly string[]): number | null {
  for (const key of keys) {
    const val = result[key];
    if (typeof val === 'number' && Number.isFinite(val)) return val;
  }
  return null;
}

function mergeDistinctFacts(
  target: { label: string; value: string }[],
  incoming: readonly { label: string; value: string }[],
): { label: string; value: string }[] {
  const seen = new Set(target.map((f) => normalizeFactKey(f.label)));
  const out = [...target];
  for (const fact of incoming) {
    const key = normalizeFactKey(fact.label);
    if (!seen.has(key)) {
      seen.add(key);
      out.push(fact);
    }
  }
  return out;
}

function extractComparisonRows(
  raw: Record<string, unknown>,
): { columns: string[]; rows: { label: string; values: string[] }[] } | null {
  const comp = raw.comparison;
  if (!comp || typeof comp !== 'object' || Array.isArray(comp)) return null;
  const colsRaw = (comp as { columns?: unknown }).columns;
  const rowsRaw = (comp as { rows?: unknown }).rows;
  if (!Array.isArray(rowsRaw) || rowsRaw.length === 0) return null;
  const columns = Array.isArray(colsRaw) ? colsRaw.map((c) => String(c)) : [];
  const rows: { label: string; values: string[] }[] = [];
  for (const item of rowsRaw) {
    if (!item || typeof item !== 'object') continue;
    const label = String((item as { label?: unknown; name?: unknown }).label ?? (item as { name?: unknown }).name ?? '').trim();
    const valuesRaw = (item as { values?: unknown }).values;
    if (!label || !Array.isArray(valuesRaw)) continue;
    rows.push({ label, values: valuesRaw.map((v) => String(v)) });
  }
  return rows.length > 0 ? { columns, rows } : null;
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
  pins: WorkbenchPins = {},
): WorkbenchObject[] {
  if (!campaign) return [];
  const qf = campaign.quantum_factors;
  const anchor = campaign.classical_anchor;
  const epistemics = campaign.epistemics;
  const notebook = campaign.notebook;
  const anchored = anchor?.anchor_computed === true;
  const chart = energyChart(campaign);

  // 4-column top-aligned Causal Spine:
  // Col 0 (x=0):    Intent & Identity (Campaign, Hypothesis, Molecule)
  // Col 1 (x=384):  Ansatz & Proof (Circuit, Proof)
  // Col 2 (x=768):  Evidence & Verification (Measurement, Figures, Evidence)
  // Col 3 (x=1216): Factors & Manuscript (Quantum factors, Manuscript, Notes)
  const COL_X = [0, 384, 768, 1216] as const;
  const COL_W = [356, 356, 420, 360] as const;
  const ROW_GAP = 24;
  const colY = [0, 0, 0, 0];
  const objects: WorkbenchObject[] = [];

  const placeSpine = (
    col: 0 | 1 | 2 | 3,
    spec: Omit<WorkbenchObject, 'position' | 'size'> & { minHeight?: number },
  ) => {
    const width = COL_W[col];
    const height = estimateCardHeight({
      detail: spec.detail,
      factsCount: spec.facts.length,
      basis: spec.basis,
      sourcesCount: spec.sources?.length ?? 0,
      hasChart: Boolean(spec.chart),
      hasHero: Boolean(spec.meta?.hero),
      hasCode: Boolean(spec.meta?.code),
      comparisonRows: spec.meta?.comparison?.rows.length ?? 0,
      runsCount: spec.meta?.runs?.length ?? 0,
      minHeight: spec.minHeight ?? (spec.chart ? 360 : 176),
    });
    objects.push({
      ...spec,
      position: { x: COL_X[col], y: colY[col] },
      size: { width, height },
    });
    colY[col] += snapToGrid(height + ROW_GAP);
  };

  const campaignHeadline = value(qf?.target_molecule || campaign.target_system, '').trim();
  const evidenceSources = recordedSources(epistemics?.sources);

  placeSpine(0, {
    id: 'campaign',
    kind: 'campaign',
    label: 'Campaign',
    title: campaignHeadline || 'Untitled campaign',
    detail: campaign.research_goal || 'No research goal recorded.',
    facts: [
      { label: 'Target', value: value(campaign.target_system) },
      { label: 'Status', value: value(epistemics?.status) },
      { label: 'Anchor', value: anchored ? 'computed' : 'UNVERIFIED' },
      { label: 'Spend', value: typeValue(sessionSpendUsd) },
      { label: 'Est. spend', value: typeValue(qf?.estimated_spend_usd) },
    ],
  });

  placeSpine(0, {
    id: 'hypothesis',
    kind: 'hypothesis',
    label: 'Hypothesis',
    title: value(epistemics?.statement, 'No evaluated claim yet'),
    detail: evidenceSources.length > 0
      ? `The claim is a projection of the campaign epistemics, citing ${evidenceSources.length} source${evidenceSources.length === 1 ? '' : 's'} (${evidenceSources.map((s) => s.label).join(', ')}).`
      : 'The claim is a projection of the campaign epistemics, not a chat message.',
    basis: 'α, β, and γ are the campaign epistemic fractions. They are not a truth score or a confidence interval.',
    sources: evidenceSources,
    facts: [
      { label: 'State', value: value(epistemics?.status) },
      { label: 'Corroborated α', value: notEvaluated(epistemics?.alpha_corroborated, 3) },
      { label: 'Contested β', value: notEvaluated(epistemics?.beta_contested, 3) },
      { label: 'Unverified γ', value: notEvaluated(epistemics?.gamma_unverified, 3) },
    ],
    ...(epistemics?.status ? {
      meta: {
        status: epistemics.status,
        callout: {
          tone: /contested|contradict/i.test(epistemics.status)
            ? 'contested'
            : /verified/i.test(epistemics.status)
              ? 'supports'
              : 'info',
          summary: `${epistemics.status} · α=${notEvaluated(epistemics.alpha_corroborated, 2)} · β=${notEvaluated(epistemics.beta_contested, 2)}`,
        },
      },
    } : {}),
  });

  placeSpine(0, {
    id: 'molecule',
    kind: 'molecule',
    label: 'Molecule',
    title: value(qf?.target_molecule || campaign.target_system, 'No molecule'),
    detail: `${value(qf?.molecular_formula, 'formula not recorded')} · bond ${fixed(qf?.bond_distance, 4)} Å`,
    facts: [
      { label: 'Active electrons', value: value(qf?.active_electrons) },
      { label: 'Active orbitals', value: value(qf?.active_orbitals) },
      { label: 'Anchor', value: anchored ? `${fixed(anchor?.fci_exact_energy, 5)} Ha` : 'UNVERIFIED' },
    ],
  });

  placeSpine(1, {
    id: 'circuit',
    kind: 'circuit',
    label: 'Circuit',
    title: value(qf?.ansatz_family, 'No ansatz recorded'),
    detail: `${value(qf?.fermion_mapping, 'mapping not recorded')} · ${value(qf?.recommended_backend, 'backend not chosen')}`,
    facts: [
      { label: 'Qubits', value: value(qf?.qubit_count) },
      { label: 'Depth', value: value(qf?.circuit_depth) },
      { label: 'Two-qubit gates', value: value(qf?.two_qubit_gates) },
    ],
    ...(qf?.qasm ? { meta: { code: { language: 'qasm', label: 'OpenQASM', content: qf.qasm } } } : {}),
  });

  placeSpine(1, {
    id: 'proof',
    kind: 'proof',
    label: 'Proof',
    title: notebook?.provenance_hash ? 'Provenance recorded' : 'No provenance receipt',
    detail: notebook?.provenance_hash || 'No ledger-backed notebook is attached to this campaign.',
    facts: [
      { label: 'Verdict', value: value(notebook?.epistemic_verdict) },
      { label: 'Chemical accuracy', value: notebook?.chemical_accuracy ? 'PASS' : 'not claimed' },
      { label: 'Claim', value: campaign.measurement?.claim_id || 'not sealed' },
      { label: 'Job', value: campaign.measurement?.job_id?.slice(0, 8) || 'not recorded' },
    ],
    ...(campaign.measurement?.job_id ? { meta: { jobId: campaign.measurement.job_id } } : {}),
  });

  placeSpine(2, {
    id: 'measurement',
    kind: 'measurement',
    label: 'Measurement',
    title: campaign.measurement?.method || (anchored ? `${value(anchor?.method)} anchor` : 'No measured anchor'),
    detail: campaign.measurement
      ? `${fixed(campaign.measurement.energy, 6)} Ha ± ${fixed(campaign.measurement.uncertainty_mha / 1000, 6)} Ha VQE against ${fixed(campaign.measurement.fci_exact_energy, 6)} Ha FCI.`
      : anchored
        ? `${fixed(anchor?.vqe_expectation_energy, 5)} Ha VQE against ${fixed(anchor?.fci_exact_energy, 5)} Ha FCI.`
        : 'The campaign carries no computed classical anchor; every value stays UNVERIFIED.',
    basis: campaign.measurement
      ? `Measured ${campaign.measurement.measured ? 'shot' : 'statevector'} value; exact optimizer ${fixed(campaign.measurement.energy_exact, 6)} Ha. Δ is the measured VQE−FCI discrepancy, not an uncertainty.`
      : anchored
        ? `Δ is the recorded VQE−FCI discrepancy in mHa, not an uncertainty. The chemical-accuracy limit is ${fixed(anchor?.chemical_accuracy_limit, 2)} mHa. Source: ${value(anchor?.anchor_source, 'not recorded')}.`
        : 'No computed anchor means no reference energy, discrepancy, or chemical-accuracy verdict is available.',
    facts: campaign.measurement
      ? [
          { label: 'FCI', value: `${fixed(campaign.measurement.fci_exact_energy, 6)} Ha` },
          { label: 'VQE', value: `${fixed(campaign.measurement.energy, 6)} Ha` },
          { label: 'Uncertainty', value: `± ${fixed(campaign.measurement.uncertainty_mha, 3)} mHa` },
          { label: 'Δ', value: `${fixed(campaign.measurement.delta_mha, 3)} mHa` },
          { label: 'Accuracy limit', value: '1.60 mHa' },
          { label: 'Accuracy', value: campaign.measurement.chemical_accuracy ? 'PASS' : 'FAIL' },
          { label: 'Admissibility', value: campaign.measurement.below_fci ? 'POINT BELOW FCI' : 'above FCI' },
          { label: 'Converged', value: campaign.measurement.converged ? 'yes' : 'no (evaluation cap)' },
          { label: 'Claim', value: campaign.measurement.claim_id || 'not sealed' },
          { label: 'Backend', value: campaign.measurement.backend || 'not recorded' },
          { label: 'Shots', value: campaign.measurement.shots ? String(campaign.measurement.shots) : 'not recorded' },
          { label: 'Optimizer path', value: campaign.measurement.optimizer_history?.length ? `${campaign.measurement.optimizer_history.length} points` : 'not recorded' },
          { label: 'Provider', value: campaign.measurement.provider || campaign.measurement.backend || 'not recorded' },
          { label: 'Hardware', value: campaign.measurement.hardware ? 'yes' : 'no (simulator)' },
          { label: 'CI95', value: campaign.measurement.ci95_mha != null ? `${fixed(campaign.measurement.ci95_mha, 3)} mHa` : 'not recorded' },
          { label: 'Usage', value: campaign.measurement.usage_seconds != null ? `${fixed(campaign.measurement.usage_seconds, 2)} s` : 'not recorded' },
        ]
      : [
          { label: 'FCI', value: anchored ? `${fixed(anchor?.fci_exact_energy, 5)} Ha` : 'UNVERIFIED' },
          { label: 'VQE', value: anchored ? `${fixed(anchor?.vqe_expectation_energy, 5)} Ha` : 'UNVERIFIED' },
          { label: 'Δ', value: anchored ? `${fixed(anchor?.discrepancy_mha, 2)} mHa` : 'UNVERIFIED' },
          { label: 'Accuracy limit', value: anchored ? `${fixed(anchor?.chemical_accuracy_limit, 2)} mHa` : 'UNVERIFIED' },
          { label: 'Accuracy', value: anchored ? (anchor?.chemical_accuracy_passed ? 'PASS' : 'FAIL') : 'UNVERIFIED' },
          { label: 'UNCERTAINTY', value: 'not recorded' },
        ],
    ...((campaign.measurement || anchored) ? {
      meta: {
        ...(campaign.measurement?.job_id ? { jobId: campaign.measurement.job_id } : {}),
        ...(campaign.measurement
          ? {
              hero: {
                value: `${fixed(campaign.measurement.energy, 5)} Ha`,
                label: `Δ ${fixed(campaign.measurement.delta_mha, 2)} mHa vs FCI (${campaign.measurement.chemical_accuracy ? 'PASS' : 'above 1.60 mHa'})`,
                tone: campaign.measurement.chemical_accuracy ? 'positive' as const : 'warning' as const,
              },
            }
          : anchored && typeof anchor?.vqe_expectation_energy === 'number'
            ? {
                hero: {
                  value: `${fixed(anchor.vqe_expectation_energy, 5)} Ha`,
                  label: `Δ ${fixed(anchor.discrepancy_mha, 2)} mHa vs FCI (${anchor.chemical_accuracy_passed ? 'PASS' : 'above limit'})`,
                  tone: anchor.chemical_accuracy_passed ? 'positive' as const : 'warning' as const,
                },
              }
            : {}),
      },
    } : {}),
  });

  if (chart) {
    const traj = campaign.trajectory ?? [];
    const firstE = traj[0]?.energy;
    const lastE = traj[traj.length - 1]?.energy;
    const chartFacts: { label: string; value: string }[] = traj.length > 0
      ? [
          { label: 'Steps', value: String(traj.length) },
          ...(typeof firstE === 'number' && Number.isFinite(firstE) ? [{ label: 'Initial', value: `${firstE.toFixed(5)} Ha` }] : []),
          ...(typeof lastE === 'number' && Number.isFinite(lastE) ? [{ label: 'Latest', value: `${lastE.toFixed(5)} Ha` }] : []),
        ]
      : [];
    placeSpine(2, {
      id: 'chart',
      kind: 'chart',
      label: 'Figure',
      title: 'Energy trajectory',
      detail: traj.length > 1 && typeof firstE === 'number' && typeof lastE === 'number'
        ? `${traj.length} iterations · ${firstE.toFixed(5)} → ${lastE.toFixed(5)} Ha.`
        : dedupHeadline(chart.basis || 'Measured trajectory', campaignHeadline),
      facts: chartFacts,
      chart,
      minHeight: 380,
    });
  }

  if ((campaign.pes_points?.length ?? 0) > 0) {
    const points = campaign.pes_points!;
    placeSpine(2, {
      id: 'pes-chart',
      kind: 'chart',
      label: 'Figure',
      title: `Potential energy surface · ${campaign.target_system || 'campaign'}`,
      detail: 'Measured energy vs bond distance, with the classical FCI anchor.',
      facts: [],
      minHeight: 360,
      chart: {
        kind: 'line',
        title: `Potential energy surface · ${campaign.target_system || 'campaign'}`,
        series: [
          { name: 'VQE energy', points: points.map((p) => ({ x: p.bond_distance, y: p.energy, display: `${p.energy.toFixed(6)} Ha` })) },
          { name: 'FCI anchor', points: points.map((p) => ({ x: p.bond_distance, y: p.fci_exact_energy, display: `${p.fci_exact_energy.toFixed(6)} Ha` })) },
        ],
        x: { label: 'Bond distance (Å)', kind: 'linear' },
        y: { label: 'Energy', unit: 'Ha', format: 'number' },
        basis: 'Measured VQE points against the computed FCI anchor; uncertainty is recorded per point in the job result.',
        detail: 'full',
      },
    });
  }

  placeSpine(2, {
    id: 'evidence',
    kind: 'evidence',
    label: 'Evidence',
    title: value(epistemics?.status, 'No epistemic state'),
    detail: (epistemics?.sources?.length ?? 0) > 0
      ? `${epistemics!.sources.length} source reference${epistemics!.sources.length === 1 ? '' : 's'} recorded.`
      : 'No source references are recorded for this claim.',
    basis: 'P(correlated) is a campaign quantity, not a truth verdict. The source list is a reference index; no quote-level excerpt is stored here.',
    sources: recordedSources(epistemics?.sources),
    facts: [
      { label: 'Corroborated α', value: notEvaluated(epistemics?.alpha_corroborated, 3) },
      { label: 'Contested β', value: notEvaluated(epistemics?.beta_contested, 3) },
      { label: 'Unverified γ', value: notEvaluated(epistemics?.gamma_unverified, 3) },
      { label: 'P(correlated)', value: notEvaluated(epistemics?.p_correlated, 3) },
      { label: 'Literature drift', value: typeof epistemics?.literature_drift_mha === 'number' && epistemics.literature_drift_mha >= 0 ? `${fixed(epistemics.literature_drift_mha, 2)} mHa` : 'not evaluated' },
      { label: 'Sources', value: String(epistemics?.sources?.length ?? 0) },
    ],
  });

  const qfDiagnostics: { label: string; value: string }[] = [
    ...(qf?.fermion_mapping ? [{ label: 'Mapping', value: qf.fermion_mapping }] : []),
    ...(typeof qf?.bond_distance === 'number' && Number.isFinite(qf.bond_distance)
      ? [{ label: 'Bond distance', value: `${fixed(qf.bond_distance, 4)} Å` }]
      : []),
    ...(typeof qf?.meyer_wallach_q === 'number' && Number.isFinite(qf.meyer_wallach_q)
      ? [{ label: 'Meyer-Wallach Q', value: fixed(qf.meyer_wallach_q, 3) }]
      : []),
    ...(typeof qf?.gradient_variance === 'number' && Number.isFinite(qf.gradient_variance)
      ? [{ label: 'Gradient var.', value: qf.gradient_variance.toExponential(2) }]
      : []),
    ...(qf?.barren_plateau_risk ? [{ label: 'Plateau risk', value: qf.barren_plateau_risk }] : []),
    ...(typeof qf?.clifford_fidelity === 'number' && Number.isFinite(qf.clifford_fidelity)
      ? [{ label: 'Clifford fid.', value: fixed(qf.clifford_fidelity, 3) }]
      : []),
    ...(typeof qf?.synthesized === 'boolean' ? [{ label: 'Synthesized', value: qf.synthesized ? 'yes' : 'no' }] : []),
  ];

  const fallbackQfFacts = [
    { label: 'Mapping', value: value(qf?.fermion_mapping) },
    { label: 'Qubits', value: value(qf?.qubit_count) },
    { label: 'Depth', value: value(qf?.circuit_depth) },
    { label: 'Two-qubit gates', value: value(qf?.two_qubit_gates) },
    { label: 'Backend', value: value(qf?.recommended_backend) },
    { label: 'Estimated spend', value: typeValue(qf?.estimated_spend_usd) },
  ];

  const comparisonSummary = anchored && anchor
    ? {
        columns: ['Method', 'Energy (Ha)', 'Δ vs FCI'],
        rows: [
          ...(typeof anchor.rhf_mean_field_energy === 'number' && Number.isFinite(anchor.rhf_mean_field_energy) && typeof anchor.fci_exact_energy === 'number'
            ? [{
                label: 'RHF mean-field',
                values: [fixed(anchor.rhf_mean_field_energy, 5), `${((anchor.rhf_mean_field_energy - anchor.fci_exact_energy) * 1000).toFixed(2)} mHa`],
              }]
            : []),
          ...(typeof (campaign.measurement?.energy ?? anchor.vqe_expectation_energy) === 'number' && typeof anchor.fci_exact_energy === 'number'
            ? [{
                label: 'VQE',
                values: [
                  fixed(campaign.measurement?.energy ?? anchor.vqe_expectation_energy, 5),
                  `${fixed(campaign.measurement?.delta_mha ?? anchor.discrepancy_mha, 2)} mHa`,
                ],
              }]
            : []),
          ...(typeof anchor.fci_exact_energy === 'number'
            ? [{ label: anchor.method || 'FCI anchor', values: [fixed(anchor.fci_exact_energy, 5), '0.00 mHa (ref)'] }]
            : []),
        ],
      }
    : undefined;

  placeSpine(3, {
    id: 'quantum-factors',
    kind: 'table',
    label: 'Table',
    title: qfDiagnostics.length >= 2 ? 'Ansatz & trainability' : 'Quantum factors',
    detail: qfDiagnostics.length >= 2
      ? 'Trainability and parameter metrics deduplicated against Spine cards.'
      : 'Exact campaign fields as recorded; no derived values.',
    facts: qfDiagnostics.length >= 2 ? qfDiagnostics : fallbackQfFacts,
    ...(comparisonSummary && comparisonSummary.rows.length > 0 ? { meta: { comparison: comparisonSummary } } : {}),
  });

  placeSpine(3, {
    id: 'manuscript',
    kind: 'manuscript',
    label: 'Manuscript',
    title: value(notebook?.title, 'No manuscript yet'),
    detail: 'The manuscript is a projection of the notebook; unverified drafts stay labelled.',
    facts: [
      { label: 'Sections', value: String(notebook?.sections?.length ?? 0) },
      { label: 'Δ', value: fixed(notebook?.delta_mha, 2) + ' mHa' },
    ],
  });

  if (notebook?.sections?.length) {
    placeSpine(3, {
      id: 'notebook-outline',
      kind: 'note',
      label: 'Note',
      title: notebook.title || 'Notebook outline',
      detail: 'Recorded section titles; the manuscript card owns the prose.',
      facts: notebook.sections.slice(0, 5).map((section, index) => ({
        label: `Section ${index + 1}`,
        value: section,
      })),
    });
  }

  // Lower Runs & Checks band starts strictly below the tallest Spine column,
  // expanding horizontally to 5–6 balanced columns when many jobs/runs exist so
  // the canvas remains compact rather than building an elongated single tower.
  const runsTop = snapToGrid(Math.max(...colY) + 16);
  const denseRuns = loops.length + jobs.length + obligations.length + tasks.length > 16;
  const runColX = denseRuns ? [...COL_X, 1600, 1984] : [...COL_X];
  const runColW = denseRuns ? [...COL_W, 356, 356] : [...COL_W];
  const runColY = runColX.map(() => runsTop);
  const spineObstacles: WorkbenchRect[] = objects.map((o) => ({
    x: o.position.x,
    y: o.position.y,
    width: o.size.width,
    height: o.size.height,
  }));
  const placeRun = (
    preferredCol: 0 | 1 | 2 | 3,
    spec: Omit<WorkbenchObject, 'position' | 'size'> & { minHeight?: number },
  ) => {
    const totalOps = loops.length + jobs.length + obligations.length + tasks.length;
    let col: number = preferredCol;
    if (totalOps > 4) {
      for (let candidate = 0; candidate < runColY.length; candidate += 1) {
        if (runColY[candidate]! < runColY[col]!) col = candidate;
      }
    }
    const width = runColW[col] ?? COL_W[0];
    const height = estimateCardHeight({
      detail: spec.detail,
      factsCount: spec.facts.length,
      basis: spec.basis,
      sourcesCount: spec.sources?.length ?? 0,
      hasChart: Boolean(spec.chart),
      hasHero: Boolean(spec.meta?.hero),
      hasCode: Boolean(spec.meta?.code),
      comparisonRows: spec.meta?.comparison?.rows.length ?? 0,
      runsCount: spec.meta?.runs?.length ?? 0,
      minHeight: spec.minHeight ?? 176,
    });
    const x = runColX[col] ?? COL_X[0];
    let y = runColY[col] ?? runsTop;
    for (const obs of spineObstacles) {
      if (rectsOverlap({ x, y, width, height }, obs, ROW_GAP)) {
        y = Math.max(y, snapToGrid(obs.y + obs.height + ROW_GAP));
      }
    }
    const rect: WorkbenchRect = { x, y, width, height };
    spineObstacles.push(rect);
    objects.push({
      ...spec,
      position: { x, y },
      size: { width, height },
    });
    runColY[col] = snapToGrid(y + height + ROW_GAP);
  };

  loops.slice(0, 160).forEach((loop) => {
    const state = String(loop.status || 'unknown');
    placeRun(0, {
      id: `agent-run-${loop.id}`,
      kind: 'agent_run',
      ref: { domain: 'loop', id: loop.id },
      label: 'Agent run',
      title: `Run ${loop.id.slice(0, 8)}`,
      detail: state === 'running' ? 'Agent is working; progress comes from the server record.' : 'Settled or paused; no live claim without a running record.',
      facts: [
        { label: 'State', value: state },
        { label: 'Step', value: `${loop.step_count} / ${loop.max_steps}` },
        { label: 'Diverged', value: loop.diverged ? 'YES' : 'no' },
      ],
      meta: { status: state, activeFlow: state === 'running' },
    });
  });

  type ParsedJob = {
    job: Job;
    params: Record<string, unknown>;
    result: Record<string, unknown>;
    revises: string | null;
    order: number;
  };
  const parsedJobs: ParsedJob[] = jobs.slice(0, 160).map((job, order) => {
    const params = parseJsonRecord(job.params_json);
    const result = parseJsonRecord(job.result_json);
    const revRaw = params.revises ?? result.revises ?? (job as { revises?: unknown }).revises;
    const revises = typeof revRaw === 'string' && revRaw.trim() ? revRaw.trim() : null;
    return { job, params, result, revises, order };
  });

  const jobById = new Map(parsedJobs.map((entry) => [entry.job.id, entry]));
  const rootOfChain = (entry: ParsedJob): string | null => {
    let cur: ParsedJob | undefined = entry;
    const seen = new Set<string>([entry.job.id]);
    let parentId = cur.revises;
    let foundParent = false;
    while (parentId && !seen.has(parentId)) {
      seen.add(parentId);
      foundParent = true;
      const next = jobById.get(parentId);
      if (!next) return parentId;
      cur = next;
      parentId = cur.revises;
    }
    return foundParent ? cur.job.id : null;
  };

  const chartCard = objects.find((object) => object.id === 'chart');
  const jobGroups = new Map<string, ParsedJob[]>();
  for (const entry of parsedJobs) {
    const { job, params, result } = entry;
    const isPinned = pins[`job-${job.id}`] === true;

    const factorSource = parseJsonRecord(result.factors ?? result.parameters);
    const factorFacts = Object.entries(factorSource).map(([k, v]) => ({ label: k, value: value(v) }));
    if (chartCard && factorFacts.length > 0 && factKeyOverlapRatio(chartCard.facts, factorFacts) >= 0.5) {
      chartCard.facts = mergeDistinctFacts(chartCard.facts, factorFacts);
      chartCard.size = {
        ...chartCard.size,
        height: estimateCardHeight({
          detail: chartCard.detail,
          factsCount: chartCard.facts.length,
          basis: chartCard.basis,
          sourcesCount: chartCard.sources?.length ?? 0,
          hasChart: true,
          minHeight: 380,
        }),
      };
    }

    const comp = extractComparisonRows(result);
    const isComparison = (job.job_type === 'comparison' || result.kind === 'comparison') && comp !== null;
    if (isComparison && !isPinned) {
      const holderId = String(params.holder ?? params.target_card ?? result.holder ?? 'measurement');
      const holder = objects.find((object) => object.id === holderId);
      if (holder) {
        holder.meta = { ...holder.meta, comparison: comp };
        holder.facts = mergeDistinctFacts(holder.facts, [
          { label: 'Comparison', value: `${comp.rows.length} rows folded` },
        ]);
        holder.size = {
          ...holder.size,
          height: estimateCardHeight({
            detail: holder.detail,
            factsCount: holder.facts.length,
            basis: holder.basis,
            sourcesCount: holder.sources?.length ?? 0,
            hasChart: Boolean(holder.chart),
          }),
        };
        continue;
      }
    }

    let groupKey: string;
    const chainRoot = rootOfChain(entry);
    const explicitSeries = params.root_job_id ?? params.series_id ?? params.hypothesis_id ?? params.campaign_id;
    if (isPinned) {
      groupKey = `pin:${job.id}`;
    } else if (chainRoot) {
      groupKey = `chain:${chainRoot}`;
    } else if (REPEATABLE_JOB_TYPES.has(job.job_type)) {
      const scope = String(params.hypothesis_id ?? params.campaign_id ?? params.model ?? params.molecule ?? params.target ?? '');
      groupKey = `repeat:${job.job_type}:${scope}`;
    } else if (typeof explicitSeries === 'string' && explicitSeries.trim()) {
      groupKey = `series:${job.job_type}:${explicitSeries.trim()}`;
    } else {
      groupKey = `solo:${job.id}`;
    }

    const bucket = jobGroups.get(groupKey);
    if (bucket) bucket.push(entry);
    else jobGroups.set(groupKey, [entry]);
  }

  for (const group of jobGroups.values()) {
    group.sort((a, b) => {
      const ta = Date.parse(a.job.created_at || '') || 0;
      const tb = Date.parse(b.job.created_at || '') || 0;
      return ta !== tb ? ta - tb : a.order - b.order;
    });
    const root = group[0];
    const latest = group[group.length - 1];
    const cleanTitle = dedupHeadline(latest.job.job_type || 'Unnamed job', campaignHeadline) || 'Unnamed job';
    const backend = value(
      latest.params.requested_backend ?? latest.params.backend ?? latest.result.backend ?? latest.result.resolved_backend,
      'not recorded',
    );
    const shots = value(latest.params.shots ?? latest.result.shots, 'not recorded');
    const latestStatus = jobStatusLabel(latest.job.status);

    if (group.length === 1) {
      placeRun(1, {
        id: `job-${root.job.id}`,
        kind: 'job',
        ref: { domain: 'job', id: root.job.id },
        label: 'Job',
        title: cleanTitle,
        detail: `Record ${root.job.id.slice(0, 8)} · ${root.job.compiler_used || 'compiler not recorded'}`,
        facts: [
          { label: 'Status', value: latestStatus },
          { label: 'Backend', value: backend },
          { label: 'Shots', value: shots },
          { label: 'Created', value: root.job.created_at || 'not recorded' },
          { label: 'Completed', value: root.job.completed_at || 'not yet' },
          { label: 'Result', value: root.job.result_json ? 'recorded' : 'not recorded' },
        ],
        meta: { jobId: root.job.id, status: latestStatus, activeFlow: latestStatus === 'RUNNING' },
      });
      continue;
    }

    const firstEnergy = jobResultNumber(root.result, ['energy', 'ground_energy']);
    const lastEnergy = jobResultNumber(latest.result, ['energy', 'ground_energy']);
    const firstDelta = jobResultNumber(root.result, ['delta_mha', 'discrepancy_mha']);
    const lastDelta = jobResultNumber(latest.result, ['delta_mha', 'discrepancy_mha']);
    const measuredDelta = firstEnergy !== null && lastEnergy !== null
      ? `${(lastEnergy - firstEnergy) >= 0 ? '+' : ''}${(lastEnergy - firstEnergy).toFixed(5)} Ha`
      : firstDelta !== null && lastDelta !== null
        ? `${(lastDelta - firstDelta) >= 0 ? '+' : ''}${(lastDelta - firstDelta).toFixed(2)} mHa`
        : null;

    const recentSlice = group.slice(-RUN_HISTORY_CAP).map((entry) => `${entry.job.id.slice(0, 8)} (${jobStatusLabel(entry.job.status)})`);
    const olderCount = Math.max(0, group.length - RUN_HISTORY_CAP);
    const historyTail = olderCount > 0 ? `+${olderCount} older runs · Recent: ${recentSlice.join(' → ')}` : `Recent runs: ${recentSlice.join(' → ')}`;

    const runRows: WorkbenchRunItem[] = group.slice(-6).reverse().map((entry) => {
      const energy = jobResultNumber(entry.result, ['energy', 'ground_energy']);
      const delta = jobResultNumber(entry.result, ['delta_mha', 'discrepancy_mha']);
      const metric = energy !== null
        ? `${energy.toFixed(5)} Ha`
        : delta !== null
          ? `${delta.toFixed(2)} mHa`
          : undefined;
      return {
        id: entry.job.id,
        label: `${cleanTitle.toLowerCase().replaceAll(/\s+/g, '_')}_${entry.job.id.slice(0, 6)}`,
        status: jobStatusLabel(entry.job.status),
        metric,
        at: entry.job.completed_at || entry.job.created_at || undefined,
      };
    });

    placeRun(1, {
      id: `job-${root.job.id}`,
      kind: 'job',
      ref: { domain: 'job', id: root.job.id },
      label: 'Job',
      title: cleanTitle,
      detail: `Latest ${latest.job.id.slice(0, 8)} (${latest.job.compiler_used || 'compiler not recorded'}) · ${historyTail}`,
      facts: [
        { label: 'Status', value: latestStatus },
        { label: 'Runs', value: `${group.length} (${group.length - 1} folded)` },
        { label: 'Updated', value: latest.job.completed_at || latest.job.created_at || 'not recorded' },
        ...(measuredDelta ? [{ label: 'Delta', value: measuredDelta }] : []),
        { label: 'Backend', value: backend },
        { label: 'Shots', value: shots },
        { label: 'Result', value: latest.job.result_json ? 'recorded' : 'not recorded' },
      ],
      meta: {
        jobId: root.job.id,
        revises: group.map((entry) => entry.job.id),
        runCount: group.length,
        runs: runRows,
        status: latestStatus,
        activeFlow: latestStatus === 'RUNNING',
      },
    });
  }

  obligations.slice(0, 32).forEach((obligation) => {
    placeRun(2, {
      id: `approval-${obligation.id}`,
      kind: 'approval',
      ref: { domain: 'obligation', id: obligation.id },
      label: 'Approval',
      title: `${obligation.subject_kind} · ${obligation.subject_id.slice(0, 12)}`,
      detail: `${obligation.owner} · ${obligation.disposition}`,
      facts: [
        { label: 'Close ready', value: obligation.close_ready ? 'YES' : 'no' },
        { label: 'Cases', value: obligation.required_cases.length ? obligation.required_cases.join(', ') : 'none' },
        { label: 'Authorities', value: obligation.required_authorities.length ? obligation.required_authorities.join(', ') : 'none' },
      ],
      meta: {
        subjectKind: obligation.subject_kind,
        subjectId: obligation.subject_id,
        status: obligation.disposition,
      },
    });
  });

  tasks.slice(0, 160).forEach((task) => {
    placeRun(3, {
      id: `task-${task.id}`,
      kind: 'task',
      ref: { domain: 'task', id: task.id },
      label: 'Task',
      title: task.title,
      detail: task.source_domain ? `${task.source_domain} · ${task.source_id}` : 'No source record',
      facts: [
        { label: 'Status', value: task.status },
        { label: 'Source', value: task.source_domain || 'none' },
        { label: 'Updated', value: task.updated_at || 'not recorded' },
      ],
      meta: {
        sourceDomain: task.source_domain,
        sourceId: task.source_id,
        status: task.status,
      },
    });
  });

  const hasMeasurement = Boolean(notebook || epistemics || anchored || (campaign.trajectory?.length ?? 0) > 0);
  const provenanceFor = (object: WorkbenchObject): ProvenanceKind => {
    switch (object.kind) {
      case 'campaign':
        return hasMeasurement ? 'measured' : 'proposed';
      case 'hypothesis':
      case 'evidence':
        return epistemics ? 'measured' : 'proposed';
      case 'measurement':
        return anchored ? 'measured' : 'proposed';
      case 'proof':
      case 'manuscript':
      case 'note':
        return notebook ? 'measured' : 'proposed';
      case 'circuit':
      case 'table':
      case 'molecule':
        return 'proposed';
      case 'chart':
      case 'agent_run':
      case 'job':
      case 'approval':
      case 'task':
        return 'measured';
      default:
        return 'catalog';
    }
  };
  return objects.map((object) => ({
    ...object,
    provenance: object.provenance ?? provenanceFor(object),
    ref: object.ref ?? defaultRef(object, campaign),
  }));
}

export type WorkbenchViewLod = 'summary' | 'compact' | 'detail';

export type WorkbenchViewState = {
  camera: WorkbenchCamera;
  selected: string[];
  view_mode: 'canvas' | 'list';
  filters: Record<string, unknown>;
  panel: string;
  lod: WorkbenchViewLod;
};

export type WorkbenchDocument = {
  version: 2;
  session_id: string;
  objects: {
    id: string;
    kind: WorkbenchObjectKind;
    position: { x: number; y: number };
    size?: { width: number; height: number };
    sizePinned?: boolean;
    ref: { domain: string; id: string };
    area?: string | null;
    pinned?: boolean;
    overrides?: Record<string, unknown>;
  }[];
  links: WorkbenchLink[];
  areas: WorkbenchArea[];
  assignments?: Record<string, string>;
  hidden: string[];
};

const VALID_LODS = new Set<WorkbenchViewLod>(['summary', 'compact', 'detail']);

function normalizeViewLod(raw: unknown): WorkbenchViewLod {
  if (raw === 'overview') return 'summary';
  if (raw === 'standard') return 'compact';
  if (typeof raw === 'string' && VALID_LODS.has(raw as WorkbenchViewLod)) return raw as WorkbenchViewLod;
  return 'compact';
}

function roundPoint(point: { x: number; y: number }): { x: number; y: number } {
  return { x: Math.round(point.x * 100) / 100, y: Math.round(point.y * 100) / 100 };
}

function roundSize(size: { width: number; height: number }): { width: number; height: number } {
  return { width: Math.round(size.width * 100) / 100, height: Math.round(size.height * 100) / 100 };
}

export function workbenchViewDocument(view: Partial<WorkbenchViewState>): WorkbenchViewState {
  const camera = view.camera ?? DEFAULT_CAMERA;
  const zoom = finite(camera.zoom) ? Math.min(2.5, Math.max(0.25, camera.zoom)) : DEFAULT_CAMERA.zoom;
  return {
    camera: {
      x: finite(camera.x) ? Math.round(camera.x * 100) / 100 : DEFAULT_CAMERA.x,
      y: finite(camera.y) ? Math.round(camera.y * 100) / 100 : DEFAULT_CAMERA.y,
      zoom: Math.round(zoom * 1000) / 1000,
    },
    selected: Array.isArray(view.selected) ? view.selected.filter((id): id is string => typeof id === 'string') : [],
    view_mode: view.view_mode === 'list' ? 'list' : 'canvas',
    filters: view.filters && typeof view.filters === 'object' ? view.filters : {},
    panel: typeof view.panel === 'string' ? view.panel.slice(0, 64) : '',
    lod: normalizeViewLod(view.lod),
  };
}

export function readWorkbenchView(
  raw: Record<string, unknown> | null | undefined,
  legacyLayout?: Record<string, unknown> | null,
): WorkbenchViewState {
  const source = raw && typeof raw === 'object' ? raw : null;
  let camera = DEFAULT_CAMERA;
  if (source?.camera && typeof source.camera === 'object') {
    const candidate = readCamera(source);
    camera = candidate;
  } else if (legacyLayout && typeof legacyLayout === 'object') {
    camera = readCamera(legacyLayout);
  }
  const selected = Array.isArray(source?.selected)
    ? source!.selected.filter((id): id is string => typeof id === 'string')
    : [];
  const view_mode = source?.view_mode === 'list' ? 'list' : 'canvas';
  const filters = source?.filters && typeof source.filters === 'object' && !Array.isArray(source.filters)
    ? (source.filters as Record<string, unknown>)
    : {};
  const panel = typeof source?.panel === 'string' ? source.panel.slice(0, 64) : '';
  const lod = normalizeViewLod(source?.lod);
  return { camera, selected, view_mode, filters, panel, lod };
}

export function readWorkbenchDocument(raw: Record<string, unknown> | null): {
  camera: WorkbenchCamera;
  pins: WorkbenchPins;
  objects: { id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[];
  positions: WorkbenchPositions;
  sizes: WorkbenchSizes;
  pinnedSizes: WorkbenchSizes;
  links: WorkbenchLink[] | null;
  areas: WorkbenchArea[];
  assignments: Record<string, string>;
  hidden: string[];
  overrides: Record<string, Record<string, unknown>>;
} {
  const camera = readCamera({ workbench: raw });
  const objects = Array.isArray(raw?.objects) ? raw.objects : [];
  const pins: WorkbenchPins = {};
  const positions: WorkbenchPositions = {};
  const sizes: WorkbenchSizes = {};
  const pinnedSizes: WorkbenchSizes = {};
  const rawLinks = Array.isArray(raw?.links) ? (raw.links as WorkbenchLink[]) : null;
  const links: WorkbenchLink[] | null = rawLinks && rawLinks.length > 0 ? rawLinks : null;
  const areas: WorkbenchArea[] = Array.isArray(raw?.areas) ? (raw.areas as WorkbenchArea[]) : [];
  const assignments: Record<string, string> = {};
  if (raw?.assignments && typeof raw.assignments === 'object' && !Array.isArray(raw.assignments)) {
    for (const [key, val] of Object.entries(raw.assignments as Record<string, unknown>)) {
      if (typeof val === 'string' && val) assignments[key] = val;
    }
  }
  const hidden: string[] = Array.isArray(raw?.hidden) ? raw!.hidden.filter((id): id is string => typeof id === 'string') : [];
  const overrides: Record<string, Record<string, unknown>> = {};
  const layoutObjects: { id: string; kind: WorkbenchObjectKind; ref?: { domain: string; id: string }; position: { x: number; y: number }; size: { width: number; height: number } }[] = [];
  for (const item of objects) {
    if (!item || typeof item !== 'object') continue;
    const object = item as { id?: unknown; position?: unknown; size?: unknown; pinned?: unknown; sizePinned?: unknown; overrides?: unknown };
    if (typeof object.id !== 'string') continue;
    const area = (object as { area?: unknown }).area;
    if (typeof area === 'string' && area) assignments[object.id] = area;
    const isPinned = object.pinned === true;
    let parsedPos: { x: number; y: number } | undefined;
    if (object.position && typeof object.position === 'object') {
      const { x, y } = object.position as Record<string, unknown>;
      if (finite(x) && finite(y)) {
        parsedPos = { x, y };
        if (isPinned) {
          pins[object.id] = true;
          positions[object.id] = parsedPos;
        }
      }
    }
    if (object.size && typeof object.size === 'object') {
      const { width, height } = object.size as Record<string, unknown>;
      if (finite(width) && finite(height) && width > 0 && height > 0) {
        sizes[object.id] = { width, height };
        if (object.sizePinned === true) pinnedSizes[object.id] = { width, height };
      }
    }
    if (object.overrides && typeof object.overrides === 'object' && !Array.isArray(object.overrides)) {
      overrides[object.id] = object.overrides as Record<string, unknown>;
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
          position: parsedPos ?? { x: 0, y: 0 },
          size: pinnedSizes[object.id] ?? { width: 356, height: 180 },
        });
      }
    }
  }
  return { camera, pins, objects: layoutObjects, positions, sizes, pinnedSizes, links, areas, assignments, hidden, overrides };
}

export const WORKBENCH_EDGE_CORNER_RADIUS = 12;
export const WORKBENCH_EDGE_GUTTER = 96;
export const WORKBENCH_FOCUS_LINK_CAP = 6;

export function buildWorkbenchLinks(objects: WorkbenchObject[]): WorkbenchLink[] {
  const has = new Set(objects.map((object) => object.id));
  const out: WorkbenchLink[] = [];
  const seen = new Set<string>();
  const add = (source: string, target: string, kind: WorkbenchLink['kind'], id?: string) => {
    if (!has.has(source) || !has.has(target) || source === target) return;
    const key = `${source}->${target}:${kind}`;
    if (seen.has(key)) return;
    seen.add(key);
    out.push({ id: id ?? `${source}-${target}`, source, target, kind });
  };

  // Preserve primary causal spine ordering & IDs first
  const pairs: [string, string, WorkbenchLink['kind']][] = [
    ['campaign', 'hypothesis', 'supports'],
    ['campaign', 'circuit', 'uses'],
    ['campaign', 'measurement', 'supports'],
    ['circuit', 'measurement', 'depends_on'],
    ['measurement', 'evidence', 'supports'],
    ['evidence', 'proof', 'supports'],
    ['proof', 'manuscript', 'depends_on'],
    ['measurement', 'chart', 'uses'],
    ['campaign', 'molecule', 'uses'],
    ['molecule', 'circuit', 'uses'],
    ['circuit', 'quantum-factors', 'uses'],
    ['hypothesis', 'evidence', 'depends_on'],
    ['measurement', 'pes-chart', 'uses'],
    ['manuscript', 'notebook-outline', 'supports'],
  ];
  for (const [source, target, kind] of pairs) add(source, target, kind);

  const byRefId = new Map<string, WorkbenchObject>();
  for (const object of objects) {
    if (object.ref?.id) byRefId.set(`${object.ref.domain}:${object.ref.id}`, object);
    if (object.meta?.jobId) byRefId.set(`job:${object.meta.jobId}`, object);
  }
  const measurementObj = objects.find((o) => o.id === 'measurement');

  for (const object of objects) {
    if (object.kind === 'agent_run') {
      add('campaign', object.id, 'uses');
    } else if (object.kind === 'job') {
      add('circuit', object.id, 'uses');
      if (measurementObj?.meta?.jobId && (
        measurementObj.meta.jobId === object.meta?.jobId ||
        object.meta?.revises?.includes(measurementObj.meta.jobId)
      )) {
        add(object.id, measurementObj.id, 'supports');
      } else if (has.has('measurement')) {
        add(object.id, 'measurement', 'supports');
      }
      if (object.meta?.revises && object.meta.revises.length > 1) {
        const rootJobId = `job-${object.meta.revises[0]}`;
        if (rootJobId !== object.id) add(rootJobId, object.id, 'same_as');
      }
    } else if (object.kind === 'approval') {
      const subKind = object.meta?.subjectKind;
      const subId = object.meta?.subjectId;
      const targetMatch = subKind && subId
        ? byRefId.get(`${subKind}:${subId}`) ?? objects.find((o) => o.id === `${subKind}-${subId}` || o.id === subKind)
        : undefined;
      if (targetMatch) add(targetMatch.id, object.id, 'depends_on');
      else add('campaign', object.id, 'depends_on');
    } else if (object.kind === 'task') {
      const srcDomain = object.meta?.sourceDomain;
      const srcId = object.meta?.sourceId;
      const sourceMatch = srcDomain && srcId
        ? byRefId.get(`${srcDomain}:${srcId}`) ?? objects.find((o) => o.kind === srcDomain || o.id === `${srcDomain}-${srcId}` || o.id === srcDomain)
        : undefined;
      if (sourceMatch) add(sourceMatch.id, object.id, 'thread');
      else add('campaign', object.id, 'thread');
    }
  }
  return out;
}

export function selectFocusedLinks(
  links: readonly WorkbenchLink[],
  activeIds: readonly string[],
  cap = WORKBENCH_FOCUS_LINK_CAP,
): WorkbenchLink[] {
  if (activeIds.length === 0) return [];
  const active = new Set(activeIds);
  const direct = links.filter((link) => active.has(link.source) || active.has(link.target));
  return direct.slice(0, cap);
}

const PRIMARY_SPINE_REST_IDS = new Set([
  'campaign-hypothesis',
  'campaign-circuit',
  'circuit-measurement',
  'measurement-evidence',
  'evidence-proof',
  'proof-manuscript',
]);

export function selectSpineChainLinks(links: readonly WorkbenchLink[]): WorkbenchLink[] {
  return links.filter((link) => PRIMARY_SPINE_REST_IDS.has(link.id));
}

export type RoutedWorkbenchLink = {
  link: WorkbenchLink;
  d: string;
  activeFlow: boolean;
};

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

export function roundedOrthogonalPath(
  rawPoints: readonly { x: number; y: number }[],
  radius = WORKBENCH_EDGE_CORNER_RADIUS,
): string {
  const pts: { x: number; y: number }[] = [];
  for (const pt of rawPoints) {
    const prev = pts[pts.length - 1];
    if (!prev || Math.hypot(pt.x - prev.x, pt.y - prev.y) >= 1) pts.push(pt);
  }
  if (pts.length < 2) return '';
  if (pts.length === 2) {
    return `M${round2(pts[0].x)},${round2(pts[0].y)} L${round2(pts[1].x)},${round2(pts[1].y)}`;
  }
  const cmds: string[] = [`M${round2(pts[0].x)},${round2(pts[0].y)}`];
  for (let i = 1; i < pts.length - 1; i += 1) {
    const prev = pts[i - 1];
    const curr = pts[i];
    const next = pts[i + 1];
    const dx1 = curr.x - prev.x;
    const dy1 = curr.y - prev.y;
    const len1 = Math.hypot(dx1, dy1);
    const dx2 = next.x - curr.x;
    const dy2 = next.y - curr.y;
    const len2 = Math.hypot(dx2, dy2);
    const cross = dx1 * dy2 - dy1 * dx2;
    if (len1 < 1 || len2 < 1 || Math.abs(cross) < 1e-3) {
      cmds.push(`L${round2(curr.x)},${round2(curr.y)}`);
      continue;
    }
    const r = Math.min(radius, len1 / 2, len2 / 2);
    const ax = curr.x - (dx1 / len1) * r;
    const ay = curr.y - (dy1 / len1) * r;
    const bx = curr.x + (dx2 / len2) * r;
    const by = curr.y + (dy2 / len2) * r;
    cmds.push(`L${round2(ax)},${round2(ay)} Q${round2(curr.x)},${round2(curr.y)} ${round2(bx)},${round2(by)}`);
  }
  const last = pts[pts.length - 1];
  cmds.push(`L${round2(last.x)},${round2(last.y)}`);
  return cmds.join(' ');
}

export function routeOrthogonalBusEdges(
  links: readonly WorkbenchLink[],
  objectById: ReadonlyMap<string, WorkbenchObject>,
  draggingIds: ReadonlySet<string> = new Set(),
): RoutedWorkbenchLink[] {
  const gutterLanes = new Map<string, number>();
  const routed: RoutedWorkbenchLink[] = [];

  for (const link of links) {
    const src = objectById.get(link.source);
    const dst = objectById.get(link.target);
    if (!src || !dst) continue;

    const activeFlow = Boolean(src.meta?.activeFlow || dst.meta?.activeFlow);
    const isDragging = draggingIds.has(src.id) || draggingIds.has(dst.id);

    const srcRight = src.position.x + src.size.width;
    const srcMidY = src.position.y + src.size.height / 2;
    const srcCx = src.position.x + src.size.width / 2;
    const dstLeft = dst.position.x;
    const dstMidY = dst.position.y + dst.size.height / 2;
    const dstCx = dst.position.x + dst.size.width / 2;

    if (isDragging) {
      const mx = (srcRight + dstLeft) / 2;
      const d = roundedOrthogonalPath([
        { x: srcRight, y: srcMidY },
        { x: mx, y: srcMidY },
        { x: mx, y: dstMidY },
        { x: dstLeft, y: dstMidY },
      ]);
      routed.push({ link, d, activeFlow });
      continue;
    }

    // Stacked vertically in the same column: bottom-center -> top-center elbow
    if (Math.abs(srcCx - dstCx) < 90 && dst.position.y >= src.position.y + src.size.height - 8) {
      const sy = src.position.y + src.size.height;
      const dy = dst.position.y;
      const midY = (sy + dy) / 2;
      const d = roundedOrthogonalPath([
        { x: srcCx, y: sy },
        { x: srcCx, y: midY },
        { x: dstCx, y: midY },
        { x: dstCx, y: dy },
      ]);
      routed.push({ link, d, activeFlow });
      continue;
    }

    // Forward inter-column routing inside the 96px shared trunk gutter
    if (dstLeft >= srcRight + 16) {
      const gutterKey = `${Math.round(srcRight / 48)}:${Math.round(dstLeft / 48)}`;
      const laneIdx = gutterLanes.get(gutterKey) ?? 0;
      gutterLanes.set(gutterKey, laneIdx + 1);
      const span = Math.min(WORKBENCH_EDGE_GUTTER, Math.max(24, dstLeft - srcRight));
      const busOffset = Math.min(span - 12, 16 + (laneIdx % 5) * 12);
      const busX = srcRight + busOffset;
      const d = roundedOrthogonalPath([
        { x: srcRight, y: srcMidY },
        { x: busX, y: srcMidY },
        { x: busX, y: dstMidY },
        { x: dstLeft, y: dstMidY },
      ]);
      routed.push({ link, d, activeFlow });
      continue;
    }

    // Reverse or same-column fallback routing via outer gutter lane
    const outerLane = gutterLanes.get('outer') ?? 0;
    gutterLanes.set('outer', outerLane + 1);
    const outerX = Math.max(srcRight, dst.position.x + dst.size.width) + 28 + (outerLane % 4) * 10;
    const d = roundedOrthogonalPath([
      { x: srcRight, y: srcMidY },
      { x: outerX, y: srcMidY },
      { x: outerX, y: dstMidY },
      { x: dst.position.x + dst.size.width, y: dstMidY },
    ]);
    routed.push({ link, d, activeFlow });
  }

  return routed;
}

export function alignSelectedPositions(
  objects: readonly WorkbenchObject[],
  axis: 'left' | 'top' = 'left',
): WorkbenchPositions {
  if (objects.length < 2) return {};
  if (axis === 'left') {
    const targetX = snapToGrid(Math.min(...objects.map((o) => o.position.x)));
    const ordered = [...objects].sort((a, b) => a.position.y - b.position.y);
    const out: WorkbenchPositions = {};
    let yCursor = snapToGrid(ordered[0].position.y);
    for (const item of ordered) {
      const targetY = Math.max(yCursor, snapToGrid(item.position.y));
      out[item.id] = { x: targetX, y: targetY };
      yCursor = targetY + item.size.height + WORKBENCH_CLEARANCE;
    }
    return out;
  }
  const targetY = snapToGrid(Math.min(...objects.map((o) => o.position.y)));
  const ordered = [...objects].sort((a, b) => a.position.x - b.position.x);
  const out: WorkbenchPositions = {};
  let xCursor = snapToGrid(ordered[0].position.x);
  for (const item of ordered) {
    const targetX = Math.max(xCursor, snapToGrid(item.position.x));
    out[item.id] = { x: targetX, y: targetY };
    xCursor = targetX + item.size.width + WORKBENCH_CLEARANCE;
  }
  return out;
}

export function distributeSelectedPositions(
  objects: readonly WorkbenchObject[],
): WorkbenchPositions {
  if (objects.length < 3) return {};
  const minX = Math.min(...objects.map((o) => o.position.x));
  const maxX = Math.max(...objects.map((o) => o.position.x));
  const minY = Math.min(...objects.map((o) => o.position.y));
  const maxY = Math.max(...objects.map((o) => o.position.y));
  const horizontal = maxX - minX >= maxY - minY;

  if (horizontal) {
    const sorted = [...objects].sort((a, b) => a.position.x - b.position.x);
    const totalWidths = sorted.reduce((acc, o) => acc + o.size.width, 0);
    const endX = sorted[sorted.length - 1].position.x + sorted[sorted.length - 1].size.width;
    const span = Math.max(endX - sorted[0].position.x, totalWidths + WORKBENCH_CLEARANCE * (sorted.length - 1));
    const gap = Math.max(WORKBENCH_CLEARANCE, (span - totalWidths) / (sorted.length - 1));
    const out: WorkbenchPositions = {};
    let cursor = snapToGrid(sorted[0].position.x);
    for (const item of sorted) {
      out[item.id] = { x: snapToGrid(cursor), y: snapToGrid(item.position.y) };
      cursor += item.size.width + gap;
    }
    return out;
  }

  const sorted = [...objects].sort((a, b) => a.position.y - b.position.y);
  const totalHeights = sorted.reduce((acc, o) => acc + o.size.height, 0);
  const endY = sorted[sorted.length - 1].position.y + sorted[sorted.length - 1].size.height;
  const span = Math.max(endY - sorted[0].position.y, totalHeights + WORKBENCH_CLEARANCE * (sorted.length - 1));
  const gap = Math.max(WORKBENCH_CLEARANCE, (span - totalHeights) / (sorted.length - 1));
  const out: WorkbenchPositions = {};
  let cursor = snapToGrid(sorted[0].position.y);
  for (const item of sorted) {
    out[item.id] = { x: snapToGrid(item.position.x), y: snapToGrid(cursor) };
    cursor += item.size.height + gap;
  }
  return out;
}

export function arrangeSelectedPositions(
  objects: readonly WorkbenchObject[],
): WorkbenchPositions {
  if (objects.length < 2) return {};
  const originX = snapToGrid(Math.min(...objects.map((o) => o.position.x)));
  const originY = snapToGrid(Math.min(...objects.map((o) => o.position.y)));
  const cols = Math.min(3, Math.ceil(Math.sqrt(objects.length)));
  const sorted = [...objects].sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x);
  const colW = new Array<number>(cols).fill(0);
  for (let i = 0; i < sorted.length; i += 1) {
    colW[i % cols] = Math.max(colW[i % cols], sorted[i].size.width);
  }
  const colX = new Array<number>(cols).fill(originX);
  for (let c = 1; c < cols; c += 1) {
    colX[c] = snapToGrid(colX[c - 1] + colW[c - 1] + 24);
  }
  const colY = new Array<number>(cols).fill(originY);
  const out: WorkbenchPositions = {};
  for (let i = 0; i < sorted.length; i += 1) {
    const c = i % cols;
    const item = sorted[i];
    out[item.id] = { x: colX[c], y: snapToGrid(colY[c]) };
    colY[c] += snapToGrid(item.size.height + 24);
  }
  return out;
}

export const COMPARISON_SPAN_LIMIT_BYTES = 8192;

export function validateManualOverrideValue(raw: string): { ok: true; value: string } | { ok: false; error: string } {
  const trimmed = raw.trim();
  if (!trimmed) return { ok: false, error: 'Override cannot be empty.' };
  const bytes = new TextEncoder().encode(trimmed).byteLength;
  if (bytes > COMPARISON_SPAN_LIMIT_BYTES) {
    return { ok: false, error: `Override exceeds ${COMPARISON_SPAN_LIMIT_BYTES} byte provenance limit (${bytes} bytes).` };
  }
  if (trimmed.length > 128) return { ok: false, error: 'Cell override must be 128 characters or fewer.' };
  return { ok: true, value: trimmed };
}

export function comparisonCsv(
  comparison: WorkbenchComparisonData,
  overrides?: Record<string, unknown>,
): string {
  const esc = (cell: string) => `"${cell.replaceAll('"', '""')}"`;
  const lines = [comparison.columns.map(esc).join(',')];
  comparison.rows.forEach((row, rIdx) => {
    const cells = [
      row.label,
      ...row.values.map((val, cIdx) => {
        const key = `${rIdx}:${cIdx}`;
        const ov = overrides?.[key] as { value?: unknown } | undefined;
        return typeof ov?.value === 'string' && ov.value ? `${ov.value} [MANUAL]` : val;
      }),
    ];
    lines.push(cells.map(esc).join(','));
  });
  return lines.join('\n');
}

export function workbenchDocument(
  sessionId: string,
  objects: WorkbenchObject[],
  positions: WorkbenchPositions,
  sizes: WorkbenchSizes,
  pins: WorkbenchPins = {},
  links: WorkbenchLink[] = [],
  areas: WorkbenchArea[] = [],
  assignments: Record<string, string> = {},
  hidden: string[] = [],
  overrides: Record<string, Record<string, unknown>> = {},
): WorkbenchDocument {
  const referenced = new Set<string>(hidden);
  for (const link of links) {
    referenced.add(link.source);
    referenced.add(link.target);
  }
  for (const [id, areaId] of Object.entries(assignments)) {
    if (areaId) referenced.add(id);
  }

  const semanticObjects: WorkbenchDocument['objects'] = [];
  for (const object of objects) {
    const isPinned = pins[object.id] === true;
    const customSize = sizes[object.id];
    const customOverride = overrides[object.id];
    const hasOverride = Boolean(customOverride && Object.keys(customOverride).length > 0);
    if (!isPinned && !customSize && !hasOverride && !referenced.has(object.id)) {
      continue;
    }
    const pos = roundPoint(positions[object.id] ?? object.position);
    semanticObjects.push({
      id: object.id,
      kind: object.kind,
      position: pos,
      ...(customSize ? { size: roundSize(customSize), sizePinned: true } : {}),
      ref: object.ref ?? { domain: 'campaign', id: sessionId },
      area: assignments[object.id] ?? null,
      ...(isPinned ? { pinned: true } : {}),
      ...(hasOverride ? { overrides: customOverride } : {}),
    });
  }

  return {
    version: 2,
    session_id: sessionId,
    objects: semanticObjects,
    links,
    areas,
    ...(Object.keys(assignments).length > 0 ? { assignments } : {}),
    hidden,
  };
}

export type WorkbenchZoomTier = 'overview' | 'standard' | 'detail';
export type WorkbenchFilter = 'all' | 'spine' | 'jobs' | 'issues';

const SPINE_FILTER_KINDS = new Set<WorkbenchObjectKind>([
  'campaign',
  'hypothesis',
  'proof',
  'circuit',
  'molecule',
  'chart',
  'table',
  'manuscript',
  'measurement',
]);

const JOB_FILTER_KINDS = new Set<WorkbenchObjectKind>(['job', 'agent_run', 'task']);

/**
 * Asymmetric hysteresis LOD transitions so hovering near a zoom boundary never thrashes:
 * - enters overview below 0.50, exits above 0.60
 * - enters detail above 1.05, exits below 0.95
 */
export function nextZoomTier(zoom: number, current: WorkbenchZoomTier): WorkbenchZoomTier {
  if (!Number.isFinite(zoom)) return current;
  if (current === 'overview') {
    if (zoom > 1.08) return 'detail';
    if (zoom > 0.60) return 'standard';
    return 'overview';
  }
  if (current === 'detail') {
    if (zoom < 0.50) return 'overview';
    if (zoom < 0.95) return 'standard';
    return 'detail';
  }
  if (zoom < 0.50) return 'overview';
  if (zoom > 1.05) return 'detail';
  return 'standard';
}

export function isIssueWorkbenchObject(object: WorkbenchObject): boolean {
  if (object.kind === 'approval') return true;
  if (/contested|contradict|diverg/i.test(`${object.title} ${object.meta?.status ?? ''}`)) return true;
  if (object.meta?.hero?.tone === 'warning') return true;
  if (object.meta?.callout?.tone === 'contested') return true;
  if (object.meta?.runs?.some((r) => /failed|error/i.test(r.status))) return true;
  return object.facts.some((f) => /failed|contested/i.test(f.value));
}

export function filterWorkbenchObjects(
  objects: readonly WorkbenchObject[],
  filter: WorkbenchFilter,
): WorkbenchObject[] {
  if (filter === 'all') return [...objects];
  if (filter === 'spine') return objects.filter((o) => SPINE_FILTER_KINDS.has(o.kind));
  if (filter === 'jobs') return objects.filter((o) => JOB_FILTER_KINDS.has(o.kind));
  return objects.filter(isIssueWorkbenchObject);
}

/**
 * Resolves a raw object id or job id (including folded repeat jobs stored in meta.runs)
 * to the visible canvas card id that owns it.
 */
export function resolveFocusObjectId(
  rawId: string,
  objects: readonly WorkbenchObject[],
): string | null {
  if (!rawId) return null;
  const direct = objects.find((o) => o.id === rawId || o.id === `job-${rawId}`);
  if (direct) return direct.id;
  const owner = objects.find((o) =>
    o.ref?.id === rawId || o.meta?.runs?.some((r) => r.id === rawId),
  );
  return owner?.id ?? null;
}
