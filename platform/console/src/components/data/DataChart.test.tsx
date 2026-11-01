import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { Chart } from './DataChart';
import type { ChartSpec } from './chart';

const spec: ChartSpec = {
  kind: 'line',
  title: 'Energy trajectory',
  x: { label: 'Iteration', kind: 'linear' },
  y: { label: 'Energy', format: 'number' },
  basis: 'Measured trajectory',
  series: [
    { name: 'VQE', points: [{ x: 1, y: -1.1 }, { x: 2, y: -1.2 }] },
    { name: 'FCI', points: [{ x: 1, y: -1.13 }, { x: 2, y: -1.13 }] },
  ],
};

describe('Chart', () => {
  it('names the plot and keeps every exact value in a table', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Chart spec={spec} />);
    });
    const svg = tree!.root.findByType('svg');
    expect(svg.props.role).toBe('img');
    expect(svg.props['aria-label']).toContain('Energy trajectory');
    expect(svg.props['aria-label']).toContain('low');
    const table = tree!.root.findByType('table');
    expect(table.findAllByType('tbody')[0]!.findAllByType('tr')).toHaveLength(2);
    expect(tree!.root.findAllByType('details')).toHaveLength(1);
  });

  it('draws a gap instead of a zero for a null point', () => {
    const withGap: ChartSpec = {
      ...spec,
      series: [{ name: 'VQE', points: [{ x: 1, y: 1 }, { x: 2, y: null }, { x: 3, y: 2 }] }],
    };
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Chart spec={withGap} />);
    });
    const rows = tree!.root.findByType('tbody').findAllByType('tr');
    expect(rows[1]!.findAllByType('td')[0]!.children.join('')).toContain('—');
  });
});
