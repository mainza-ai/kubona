import { describe, expect, it } from 'vitest';
import {
  alignedRows,
  chartSummary,
  extents,
  formatValue,
  plotPath,
  pointText,
  shouldFlipHorizontal,
  thinnedTickIndices,
  ticks,
  valuesCsv,
  type ChartSpec,
} from './chart';

const spec: ChartSpec = {
  kind: 'line',
  title: 'Energy trajectory',
  x: { label: 'Iteration', kind: 'linear' },
  y: { label: 'Energy', format: 'number' },
  basis: 'Measured trajectory',
  series: [
    {
      name: 'VQE',
      points: [
        { x: 1, y: -1.1, display: '-1.10000 Ha' },
        { x: 2, y: null },
        { x: 3, y: -1.2 },
      ],
    },
    { name: 'FCI', points: [{ x: 1, y: -1.13 }, { x: 2, y: -1.13 }, { x: 3, y: -1.13 }] },
  ],
};

describe('chart spec', () => {
  it('keeps null as a gap instead of turning it into zero', () => {
    const rows = alignedRows(spec);
    expect(rows[1]?.points[0]?.y).toBeNull();
    expect(pointText(rows[1]?.points[0], spec.y)).toBe('—');
    expect(extents(spec)).toEqual({ low: -1.2, high: -1.1 });
  });

  it('formats values with the declared unit', () => {
    expect(formatValue(5400, { format: 'duration' })).toBe('1h 30m');
    expect(formatValue(42, { format: 'percent' })).toBe('42%');
    expect(formatValue(1200, { format: 'bytes' })).toBe('1.2 KB');
    expect(formatValue(12.5, { format: 'money', currency: 'USD' })).toContain('12.5');
  });

  it('exports the exact source strings to CSV', () => {
    expect(valuesCsv(spec)).toBe(
      '"","VQE","FCI"\n"1","-1.10000 Ha","-1.13"\n"2","—","-1.13"\n"3","-1.2","-1.13"',
    );
  });

  it('summarizes the plot for assistive technology', () => {
    const summary = chartSummary(spec);
    expect(summary).toContain('Energy trajectory');
    expect(summary).toContain('Iteration');
    expect(summary).toContain('line');
    expect(summary).toContain('low');
  });

  it('rounds ticks and never invents a curve peak', () => {
    expect(ticks(-1.2, -1.1, 4).length).toBeGreaterThan(1);
    const path = plotPath([
      { x: 0, y: 10 },
      { x: 10, y: 0 },
      { x: 20, y: 10 },
    ]);
    expect(path.startsWith('M0,10')).toBe(true);
    expect(path).toContain('C');

    // No invented peak or dip: a Bézier lies inside the hull of its control
    // points, so every control y must stay within its segment's y-range.
    const datasets: Array<Array<{ x: number; y: number }>> = [
      [{ x: 0, y: 10 }, { x: 10, y: 0 }, { x: 20, y: 10 }],
      [{ x: 0, y: 0 }, { x: 1, y: 0.1 }, { x: 2, y: 9.9 }, { x: 3, y: 10 }, { x: 4, y: 10 }],
      [{ x: 0, y: 5 }, { x: 1, y: 5 }, { x: 2, y: 5 }, { x: 3, y: 1 }, { x: 4, y: 8 }],
      [{ x: 0, y: -1.137 }, { x: 0.5, y: -1.1 }, { x: 0.74, y: -1.137 }, { x: 1.5, y: -0.9 }, { x: 3, y: -0.93 }],
      [{ x: 0, y: 1 }, { x: 1, y: 3 }, { x: 1, y: 7 }, { x: 2, y: 4 }],
    ];
    for (const data of datasets) {
      const d = plotPath(data);
      const segments = d.slice(d.indexOf(',') + 1).match(/[CL][^CL]+/g) ?? [];
      expect(segments).toHaveLength(data.length - 1);
      segments.forEach((segment, k) => {
        const y0 = data[k]!.y;
        const y1 = data[k + 1]!.y;
        const lo = Math.min(y0, y1) - 0.011;
        const hi = Math.max(y0, y1) + 0.011;
        const nums = segment.slice(1).split(',').map(Number);
        const ys = segment[0] === 'C' ? [nums[1]!, nums[3]!, nums[5]!] : [nums[1]!];
        for (const y of ys) {
          expect(y).toBeGreaterThanOrEqual(lo);
          expect(y).toBeLessThanOrEqual(hi);
        }
      });
    }
    // A segment with no x-extent is drawn straight, never interpolated.
    expect(plotPath([{ x: 0, y: 1 }, { x: 1, y: 3 }, { x: 1, y: 7 }, { x: 2, y: 4 }])).toContain('L1,7');
  });

  it('flips bars horizontally on long category labels or > 8 categories, and thins crowded ticks', () => {
    const shortSpec: ChartSpec = { kind: 'bars', title: 'Short', series: [] };
    expect(shouldFlipHorizontal(shortSpec, ['A', 'B', 'C'])).toBe(false);
    expect(shouldFlipHorizontal(shortSpec, ['Very long hardware backend label', 'Short'])).toBe(true);
    expect(shouldFlipHorizontal(shortSpec, Array.from({ length: 9 }, (_, i) => `K${i}`))).toBe(true);
    expect(shouldFlipHorizontal({ kind: 'line', title: 'Line', series: [] }, Array.from({ length: 15 }, (_, i) => `Long label number ${i}`))).toBe(false);

    const small = thinnedTickIndices(5);
    expect(small).toEqual([0, 1, 2, 3, 4]);
    const crowded = thinnedTickIndices(24, 6);
    expect(crowded.length).toBeLessThanOrEqual(7);
    expect(crowded[0]).toBe(0);
    expect(crowded[crowded.length - 1]).toBe(23);
  });
});
