/** Pure chart geometry: viewport, plot box, scale functions and ticks. Keeping
 * this out of the renderer makes the layout testable without a DOM and stops
 * DataChart from being a 1,000-line unit. */

import {
  extents,
  formatValue,
  shouldFlipHorizontal,
  thinnedTickIndices,
  ticks,
  type ChartPoint,
  type ChartSpec,
} from './chart';

export const CHART_WIDTH = 680;
export const CHART_HEIGHT = 300;
export const CHART_MARGINS = { top: 18, right: 18, bottom: 44, left: 62 };
export const CHART_H_LEFT = 128;
export const CHART_PLOT_W = CHART_WIDTH - CHART_MARGINS.left - CHART_MARGINS.right;
export const CHART_PLOT_H = CHART_HEIGHT - CHART_MARGINS.top - CHART_MARGINS.bottom;

export type ChartLayout = {
  width: number;
  height: number;
  margins: typeof CHART_MARGINS;
  svgHeight: number;
  compact: boolean;
  overview: boolean;
  horizontal: boolean;
  hasHorizontalValueLabels: boolean;
  maxKeyChars: number;
  maxValChars: number;
  maxHorizontalLabelChars: number;
  plotLeft: number;
  plotRight: number;
  plotW: number;
  plotH: number;
  zeroAnchored: boolean;
  safeLow: number;
  safeHigh: number;
  ticksY: number[];
  yLow: number;
  yHigh: number;
  ySpan: number;
  xTickIndices: number[];
  y: (value: number) => number;
  xVal: (value: number) => number;
  xBand: (index: number) => number;
  yBand: (index: number) => number;
  xLine: (index: number) => number;
};

export function computeChartLayout(
  spec: ChartSpec,
  rows: ReadonlyArray<{ points: (ChartPoint | undefined)[] }>,
  keys: readonly string[],
  compact: boolean,
): ChartLayout {
  const overview = spec.detail === 'overview';
  const horizontal = shouldFlipHorizontal(spec, keys as string[]);
  const hasHorizontalValueLabels = horizontal && !compact && spec.series.length === 1 && rows.length <= 14;
  const maxKeyChars = keys.reduce((m, k) => Math.max(m, k.length), 0);
  const maxValChars = hasHorizontalValueLabels
    ? rows.reduce((m, r) => {
        const pt = r.points[0];
        return pt && pt.y != null ? Math.max(m, formatValue(pt.y, spec.y).length) : m;
      }, 0)
    : 0;
  const svgHeight = horizontal
    ? Math.max(CHART_HEIGHT, CHART_MARGINS.top + CHART_MARGINS.bottom + keys.length * 28)
    : CHART_HEIGHT;
  const plotLeft = horizontal
    ? Math.min(220, Math.max(CHART_H_LEFT, Math.ceil(maxKeyChars * 7.2 + 20)))
    : CHART_MARGINS.left;
  const plotRight = hasHorizontalValueLabels
    ? Math.max(CHART_MARGINS.right, Math.min(112, Math.ceil(maxValChars * 7 + 16)))
    : CHART_MARGINS.right;
  const plotW = Math.max(160, CHART_WIDTH - plotLeft - plotRight);
  const plotH = svgHeight - CHART_MARGINS.top - CHART_MARGINS.bottom;
  const maxHorizontalLabelChars = Math.max(16, Math.floor((plotLeft - 18) / 7.2));

  const zeroAnchored =
    spec.kind === 'bars' || spec.kind === 'stacked' || spec.kind === 'area' || spec.kind === 'range';
  const range = extents(spec);
  const rawLow = range.low ?? 0;
  const rawHigh = range.high ?? 1;
  const safeLow = rawLow === rawHigh ? (zeroAnchored && rawLow === 0 ? 0 : rawLow - 1) : rawLow;
  const safeHigh = rawLow === rawHigh ? (zeroAnchored && rawLow === 0 ? 1 : rawHigh + 1) : rawHigh;
  const ticksY = ticks(safeLow, safeHigh, compact ? 3 : 4);
  const yLow = ticksY[0] ?? safeLow;
  const yHigh = ticksY[ticksY.length - 1] ?? safeHigh;
  const ySpan = yHigh === yLow ? 1 : yHigh - yLow;

  const y = (value: number) =>
    CHART_MARGINS.top + plotH - ((value - yLow) / ySpan) * plotH;
  const xVal = (value: number) => plotLeft + ((value - yLow) / ySpan) * plotW;
  const xBand = (index: number) => {
    const step = plotW / Math.max(1, keys.length);
    return plotLeft + step * index + step / 2;
  };
  const yBand = (index: number) => {
    const step = plotH / Math.max(1, keys.length);
    return CHART_MARGINS.top + step * index + step / 2;
  };
  const xLine = (index: number) =>
    plotLeft + (keys.length <= 1 ? plotW / 2 : (plotW * index) / (keys.length - 1));

  return {
    width: CHART_WIDTH,
    height: CHART_HEIGHT,
    margins: CHART_MARGINS,
    svgHeight,
    compact,
    overview,
    horizontal,
    hasHorizontalValueLabels,
    maxKeyChars,
    maxValChars,
    maxHorizontalLabelChars,
    plotLeft,
    plotRight,
    plotW,
    plotH,
    zeroAnchored,
    safeLow,
    safeHigh,
    ticksY,
    yLow,
    yHigh,
    ySpan,
    xTickIndices: thinnedTickIndices(keys.length),
    y,
    xVal,
    xBand,
    yBand,
    xLine,
  };
}
