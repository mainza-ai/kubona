import { describe, expect, it } from 'vitest';
import { alignedRows, categoryKeys, type ChartSpec } from './chart';
import { CHART_HEIGHT, CHART_MARGINS, computeChartLayout } from './chart-layout';

function spec(overrides: Partial<ChartSpec> = {}): ChartSpec {
  return {
    kind: 'bars',
    title: 'Energy',
    series: [
      {
        name: 'measured',
        points: [
          { x: 'a', y: 1 },
          { x: 'b', y: 2 },
          { x: 'c', y: 3 },
        ],
      },
    ],
    ...overrides,
  };
}

function layoutOf(input: ChartSpec) {
  const rows = alignedRows(input);
  const keys = categoryKeys(input);
  return computeChartLayout(input, rows, keys, false);
}

describe('chart layout geometry', () => {
  it('places a vertical plot inside the fixed viewport', () => {
    const layout = layoutOf(spec());
    expect(layout.horizontal).toBe(false);
    expect(layout.width).toBe(680);
    expect(layout.svgHeight).toBe(CHART_HEIGHT);
    expect(layout.plotLeft).toBe(CHART_MARGINS.left);
    expect(layout.plotW).toBe(600);
    expect(layout.plotH).toBe(238);
    // High value at the top margin, low value at the bottom of the plot.
    expect(layout.y(layout.yHigh)).toBeCloseTo(CHART_MARGINS.top, 6);
    expect(layout.y(layout.yLow)).toBeCloseTo(CHART_MARGINS.top + layout.plotH, 6);
    // Bands are centered; lines run edge to edge.
    expect(layout.xBand(0)).toBeCloseTo(162, 6);
    expect(layout.xBand(2)).toBeCloseTo(562, 6);
    expect(layout.xLine(0)).toBeCloseTo(62, 6);
    expect(layout.xLine(2)).toBeCloseTo(662, 6);
  });

  it('expands the left gutter for long horizontal labels and uses the vertical plot', () => {
    const layout = layoutOf(spec({ horizontal: true, series: [{ name: 'm', points: [{ x: 'a-very-long-category', y: 1 }, { x: 'b', y: 2 }] }] }));
    expect(layout.horizontal).toBe(true);
    expect(layout.plotLeft).toBeGreaterThan(CHART_MARGINS.left);
    expect(layout.plotLeft).toBeLessThanOrEqual(220);
    expect(layout.plotW).toBe(680 - layout.plotLeft - layout.plotRight);
    // Horizontal band centers on rows.
    expect(layout.yBand(0)).toBeCloseTo(CHART_MARGINS.top + layout.plotH / 4, 6);
  });

  it('grows the viewport when horizontal categories need more rows', () => {
    const points = Array.from({ length: 12 }, (_, i) => ({ x: `k${i}`, y: i }));
    const layout = layoutOf(spec({ horizontal: true, series: [{ name: 'm', points }] }));
    expect(layout.svgHeight).toBeGreaterThan(CHART_HEIGHT);
    expect(layout.plotH).toBe(layout.svgHeight - CHART_MARGINS.top - CHART_MARGINS.bottom);
  });

  it('expands a flat zero series instead of dividing by zero', () => {
    const layout = layoutOf(spec({ series: [{ name: 'm', points: [{ x: 'a', y: 0 }, { x: 'b', y: 0 }] }] }));
    expect(layout.safeLow).toBe(0);
    expect(layout.safeHigh).toBeGreaterThan(0);
    expect(layout.ySpan).toBe(layout.yHigh - layout.yLow);
  });

  it('centers a single-key line instead of dividing by zero', () => {
    const layout = layoutOf(spec({ kind: 'line', series: [{ name: 'm', points: [{ x: 'only', y: 1 }] }] }));
    expect(layout.xLine(0)).toBeCloseTo(layout.plotLeft + layout.plotW / 2, 6);
  });
});
