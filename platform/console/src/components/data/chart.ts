/** One typed spec for every Kubona chart. Values are numbers; display strings
 * are kept for the exact-values table, never re-parsed for drawing. */

export type ChartKind =
  | 'bars'
  | 'stacked'
  | 'line'
  | 'area'
  | 'range'
  | 'donut'
  | 'radial'
  | 'radar'
  | 'heat'
  | 'spark';

export type ChartPoint = {
  x: string | number;
  /** null is a gap. It is never drawn or treated as zero. */
  y: number | null;
  /** The top of a range. */
  y2?: number;
  /** The value as its source wrote it, shown verbatim in the table. */
  display?: string;
  evidence?: { key: string; label: string; url?: string; file?: boolean; origin?: string }[];
};

export type ChartSpec = {
  kind: ChartKind;
  title: string;
  series: { name: string; points: ChartPoint[] }[];
  x?: { label?: string; kind?: 'category' | 'time' | 'linear' };
  y?: {
    label?: string;
    unit?: string;
    format?: 'number' | 'money' | 'duration' | 'percent' | 'bytes';
    currency?: string;
    min?: number;
    max?: number;
    clamp?: boolean;
  };
  basis?: string;
  headline?: { label: string; value: string };
  detail?: 'full' | 'overview' | 'tile';
  compact?: boolean;
  horizontal?: boolean;
  stack?: boolean;
  values?: false;
};

const CHART_COLORS = 8;

export function seriesColor(index: number): string {
  if (index < CHART_COLORS) return `var(--color-chart-${index + 1})`;
  return 'var(--color-chart-other)';
}

export function formatValue(
  value: number | null | undefined,
  y?: ChartSpec['y'],
): string {
  if (value == null || !Number.isFinite(value)) return '—';
  switch (y?.format) {
    case 'money':
      return new Intl.NumberFormat(undefined, {
        style: 'currency',
        currency: y.currency || 'USD',
        maximumFractionDigits: 2,
      }).format(value);
    case 'percent':
      return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value)}%`;
    case 'duration': {
      const seconds = Math.max(0, value);
      if (seconds < 60) return `${Math.round(seconds)}s`;
      if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`;
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.round((seconds % 3600) / 60);
      return `${hours}h ${minutes}m`;
    }
    case 'bytes': {
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let next = Math.max(0, value);
      let unit = 0;
      while (next >= 1000 && unit < units.length - 1) {
        next /= 1000;
        unit += 1;
      }
      return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(next)} ${units[unit]}`;
    }
    default:
      return new Intl.NumberFormat(undefined, { maximumFractionDigits: 6 }).format(value);
  }
}

export function pointText(point: ChartPoint | undefined, y?: ChartSpec['y']): string {
  if (!point) return '—';
  if (point.display) return point.display;
  if (point.y2 != null) return `${formatValue(point.y, y)}–${formatValue(point.y2, y)}`;
  return formatValue(point.y, y);
}

export function xText(value: string | number): string {
  return String(value);
}

/** Categories in first-seen order. Time/linear axes sort by their own key. */
export function categoryKeys(spec: ChartSpec): string[] {
  const seen = new Set<string>();
  const keys: string[] = [];
  for (const series of spec.series) {
    for (const point of series.points) {
      const key = String(point.x);
      if (!seen.has(key)) {
        seen.add(key);
        keys.push(key);
      }
    }
  }
  if (spec.x?.kind === 'time') {
    keys.sort((a, b) => new Date(a).getTime() - new Date(b).getTime());
  } else if (spec.x?.kind === 'linear') {
    keys.sort((a, b) => Number(a) - Number(b));
  }
  return keys;
}

export function alignedRows(spec: ChartSpec): {
  key: string;
  label: string;
  points: (ChartPoint | undefined)[];
}[] {
  return categoryKeys(spec).map((key) => ({
    key,
    label: xText(key),
    points: spec.series.map((series) => series.points.find((point) => String(point.x) === key)),
  }));
}

export function extents(spec: ChartSpec): { low: number | null; high: number | null } {
  if (
    spec.y?.clamp &&
    typeof spec.y.min === 'number' &&
    Number.isFinite(spec.y.min) &&
    typeof spec.y.max === 'number' &&
    Number.isFinite(spec.y.max) &&
    spec.y.max > spec.y.min
  ) {
    return { low: spec.y.min, high: spec.y.max };
  }
  const values: number[] = [];
  for (const series of spec.series) {
    for (const point of series.points) {
      if (point.y != null && Number.isFinite(point.y)) values.push(point.y);
      if (point.y2 != null && Number.isFinite(point.y2)) values.push(point.y2);
    }
  }
  if (values.length === 0) return { low: null, high: null };
  let low = Math.min(spec.y?.min ?? Infinity, ...values);
  let high = Math.max(spec.y?.max ?? -Infinity, ...values);
  if (spec.kind === 'bars' || spec.kind === 'stacked' || spec.kind === 'area' || spec.kind === 'range') {
    low = Math.min(low, 0);
    high = Math.max(high, 0);
  }
  return { low, high };
}

function niceStep(span: number, count: number): number {
  if (!Number.isFinite(span) || span <= 0) return 1;
  const rough = span / Math.max(1, count);
  const power = 10 ** Math.floor(Math.log10(rough));
  const fraction = rough / power;
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
  return nice * power;
}

export function ticks(low: number, high: number, count = 4): number[] {
  if (!Number.isFinite(low) || !Number.isFinite(high) || low === high) return [low];
  const step = niceStep(high - low, count);
  const start = Math.floor(low / step) * step;
  const end = Math.ceil(high / step) * step;
  const out: number[] = [];
  for (let value = start; value <= end + step * 0.5; value += step) out.push(Number(value.toFixed(12)));
  return out;
}

export function chartSummary(spec: ChartSpec): string {
  const axes = [spec.x?.label, spec.y?.label].filter(Boolean).join('; ');
  const range = extents(spec);
  const low = range.low == null ? '' : formatValue(range.low, spec.y);
  const high = range.high == null ? '' : formatValue(range.high, spec.y);
  return [spec.title, axes, `${spec.kind}${low && high ? `; low ${low}; high ${high}` : ''}`]
    .filter(Boolean)
    .join('. ');
}

export function valuesCsv(spec: ChartSpec): string {
  const rows = alignedRows(spec);
  const escape = (value: string) => `"${value.replaceAll('"', '""')}"`;
  const header = ['', ...spec.series.map((series) => series.name)];
  const lines = [header.map(escape).join(',')];
  for (const row of rows) {
    lines.push(
      [
        row.label,
        ...row.points.map((point) => pointText(point, spec.y)),
      ]
        .map(escape)
        .join(','),
    );
  }
  return lines.join('\n');
}

/**
 * Smooth path through points ordered along x that never overshoots the data:
 * monotone piecewise-cubic Hermite interpolation after Fritsch & Carlson,
 * "Monotone Piecewise Cubic Interpolation", SIAM J. Numer. Anal. 17(2), 1980.
 * Each Hermite segment is emitted as the equivalent cubic Bézier, whose
 * control points stay within the segment's y-range, so no peak or dip is
 * invented between measured points. Segments without positive x-extent are
 * drawn straight rather than interpolated.
 */
export function plotPath(points: readonly { x: number; y: number }[]): string {
  const n = points.length;
  if (n === 0) return '';
  const start = `M${round(points[0]!.x)},${round(points[0]!.y)}`;
  if (n === 1) return start;
  if (n === 2) return `${start}L${round(points[1]!.x)},${round(points[1]!.y)}`;

  // 1. Secant slope of every segment (0 where x does not advance).
  const widths: number[] = [];
  const secants: number[] = [];
  for (let k = 0; k < n - 1; k += 1) {
    const h = points[k + 1]!.x - points[k]!.x;
    widths.push(h);
    secants.push(h > 0 ? (points[k + 1]!.y - points[k]!.y) / h : 0);
  }

  // 2. Initial tangents: one-sided at the ends, secant average inside, and
  //    flat at local extrema (neighbouring secants disagree in sign).
  const tangents: number[] = new Array(n);
  tangents[0] = secants[0]!;
  tangents[n - 1] = secants[n - 2]!;
  for (let k = 1; k < n - 1; k += 1) {
    const before = secants[k - 1]!;
    const after = secants[k]!;
    tangents[k] = before * after <= 0 ? 0 : (before + after) / 2;
  }

  // 3. Flat segments pin both endpoint tangents; otherwise restrict
  //    (alpha, beta) to the circle of radius 3 — the sufficient monotonicity
  //    region from Fritsch & Carlson.
  for (let k = 0; k < n - 1; k += 1) {
    const delta = secants[k]!;
    if (delta === 0) {
      tangents[k] = 0;
      tangents[k + 1] = 0;
      continue;
    }
    const alpha = tangents[k]! / delta;
    const beta = tangents[k + 1]! / delta;
    const radius = Math.hypot(alpha, beta);
    if (radius > 3) {
      const tau = 3 / radius;
      tangents[k] = tau * alpha * delta;
      tangents[k + 1] = tau * beta * delta;
    }
  }

  // 4. Hermite segment (p0, m0, p1, m1) over width h == Bézier with control
  //    points p0 + (h/3, m0 h/3) and p1 - (h/3, m1 h/3).
  let path = start;
  for (let k = 0; k < n - 1; k += 1) {
    const a = points[k]!;
    const b = points[k + 1]!;
    const h = widths[k]!;
    if (!(h > 0)) {
      path += `L${round(b.x)},${round(b.y)}`;
      continue;
    }
    const third = h / 3;
    path += `C${round(a.x + third)},${round(a.y + third * tangents[k]!)},${round(b.x - third)},${round(b.y - third * tangents[k + 1]!)},${round(b.x)},${round(b.y)}`;
  }
  return path;
}

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

/** Auto-flip categorical bar charts horizontally when labels are long or numerous. */
export function shouldFlipHorizontal(spec: ChartSpec, keys: readonly string[]): boolean {
  if (spec.kind !== 'bars' && spec.kind !== 'stacked') return false;
  return Boolean(spec.horizontal || keys.length > 8 || keys.some((key) => key.length > 14));
}

/** Deterministic X-axis tick indices when series length exceeds readable density (> 12). */
export function thinnedTickIndices(total: number, maxLabels = 8): number[] {
  if (total <= 0) return [];
  if (total <= 12) return Array.from({ length: total }, (_, index) => index);
  const step = Math.ceil(total / Math.max(2, maxLabels));
  const out: number[] = [];
  for (let index = 0; index < total; index += step) out.push(index);
  const last = total - 1;
  if (out[out.length - 1] !== last) {
    if (last - out[out.length - 1]! < Math.ceil(step / 2)) out.pop();
    out.push(last);
  }
  return out;
}
