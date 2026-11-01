import { describe, expect, it } from 'vitest';
import {
  alignedRows,
  chartSummary,
  extents,
  formatValue,
  plotPath,
  pointText,
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
  });
});
