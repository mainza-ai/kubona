import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { Chart } from './DataChart';
import type { ChartSpec } from './chart';

afterEach(() => {
  vi.useRealTimers();
});

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

  it('suppresses duplicate visible title when embedded while keeping accessible aria-labelledby', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Chart spec={spec} embedded />);
    });
    const hiddenWrap = tree!.root.findByType('header').findByProps({ className: 'sr-only' });
    const figcaption = hiddenWrap.findByType('figcaption');
    expect(figcaption.children.join('')).toBe('Energy trajectory');
    const svg = tree!.root.findByType('svg');
    expect(svg.props['aria-labelledby']).toBe(figcaption.props.id);
    expect(tree!.root.findByType('header').props.className).toContain('k-chart-head-embedded');
  });

  it('flips bars horizontally on long category labels and thins > 12 X ticks on lines', () => {
    const barSpec: ChartSpec = {
      kind: 'bars',
      title: 'Backend error',
      series: [{
        name: 'Error',
        points: [
          { x: 'ibm_marrakesh_heron_v2', y: 0.012 },
          { x: 'local_simulator', y: 0.002 },
        ],
      }],
    };
    let barTree: TestRenderer.ReactTestRenderer;
    act(() => {
      barTree = TestRenderer.create(<Chart spec={barSpec} />);
    });
    const svg = barTree!.root.findByType('svg');
    expect(barTree!.root.findByType('figure').props['data-horizontal']).toBe('true');
    const ticks = svg.findAllByProps({ className: 'k-chart-tick' });
    expect(ticks.some((t) => String(t.children.join('')).includes('ibm_marrakesh_h'))).toBe(true);

    const crowdedLine: ChartSpec = {
      kind: 'line',
      title: 'Dense trace',
      series: [{
        name: 'Energy',
        points: Array.from({ length: 20 }, (_, i) => ({ x: `S${i + 1}`, y: -1 - i * 0.01 })),
      }],
    };
    let lineTree: TestRenderer.ReactTestRenderer;
    act(() => {
      lineTree = TestRenderer.create(<Chart spec={crowdedLine} />);
    });
    const renderedKeys = lineTree!.root
      .findByType('svg')
      .findAllByProps({ className: 'k-chart-tick' })
      .map((node) => node.children.join(''))
      .filter((txt) => txt.startsWith('S'));
    expect(renderedKeys.length).toBeLessThanOrEqual(8);
  });

  it('anchors zero-only bars on [0, 1] and expands horizontal gutters for long labels', () => {
    const zeroBars: ChartSpec = {
      kind: 'bars',
      title: 'Zero counts',
      series: [{ name: 'Shots', points: [{ x: '00', y: 0 }, { x: '11', y: 0 }] }],
    };
    let zeroTree: TestRenderer.ReactTestRenderer;
    act(() => {
      zeroTree = TestRenderer.create(<Chart spec={zeroBars} />);
    });
    const yTicks = zeroTree!.root
      .findByType('svg')
      .findAllByProps({ className: 'k-chart-tick' })
      .map((n) => n.children.join(''));
    expect(yTicks).toContain('0');
    expect(yTicks).not.toContain('-1');

    const longBarSpec: ChartSpec = {
      kind: 'bars',
      title: 'Backend error',
      series: [{ name: 'Error', points: [{ x: 'ibm_marrakesh_heron_v2', y: 0.012 }] }],
    };
    let longTree: TestRenderer.ReactTestRenderer;
    act(() => {
      longTree = TestRenderer.create(<Chart spec={longBarSpec} />);
    });
    const svg = longTree!.root.findByType('svg');
    const tickStrings = svg.findAllByProps({ className: 'k-chart-tick' }).map((n) => n.children.join(''));
    expect(tickStrings).toContain('ibm_marrakesh_heron_v2');
    const valNode = svg.findAllByProps({ className: 'k-chart-tick' }).find((n) => n.children.join('') === '0.012');
    expect(valNode).toBeDefined();
    expect(Number(valNode!.props.x)).toBeLessThan(640);
  });

  it('shows pointer/focus hover tooltip and hides it after 160ms Hover.hide grace', () => {
    vi.useFakeTimers();
    const barSpec: ChartSpec = {
      kind: 'bars',
      title: 'Energies',
      series: [{ name: 'VQE', points: [{ x: 'run1', y: -1.11, evidence: [{ key: 'job', label: 'job:123' }] }] }],
    };
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Chart spec={barSpec} />);
    });
    const rect = tree!.root.findByType('rect');
    act(() => {
      rect.props.onPointerEnter();
    });
    const tip = tree!.root.findByProps({ 'data-testid': 'k-chart-tip' });
    expect(tip.findByType('strong').children.join('')).toBe('-1.11');
    expect(tip.findByProps({ className: 'k-chart-tip-evidence' }).children.join('')).toContain('job:123');

    act(() => {
      rect.props.onPointerLeave();
      vi.advanceTimersByTime(100);
    });
    expect(tree!.root.findAllByProps({ 'data-testid': 'k-chart-tip' })).toHaveLength(1);

    act(() => {
      vi.advanceTimersByTime(65);
    });
    expect(tree!.root.findAllByProps({ 'data-testid': 'k-chart-tip' })).toHaveLength(0);
    act(() => {
      tree!.unmount();
    });
  });

  it('clamps off-scale outliers with accessible diamond markers while keeping exact raw table values', () => {
    const clampedSpec: ChartSpec = {
      kind: 'line',
      title: 'H2 energy trajectory',
      x: { label: 'Iteration', kind: 'linear' },
      y: { label: 'Energy', unit: 'Ha', format: 'number', min: -1.15, max: -1.1, clamp: true },
      basis: 'Measured trajectory. 1 off-scale step flagged.',
      series: [
        {
          name: 'VQE energy',
          points: [
            { x: 1, y: -1.129, display: '-1.12900 Ha' },
            { x: 2, y: -7.85, display: '-7.85000 Ha' },
            { x: 3, y: -1.136, display: '-1.13600 Ha' },
          ],
        },
      ],
    };
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Chart spec={clampedSpec} />);
    });
    const outliers = tree!.root.findAllByProps({ 'data-outlier': 'true' });
    expect(outliers).toHaveLength(1);
    const tableText = tree!.root
      .findByType('table')
      .findAllByType('td')
      .map((cell) => cell.children.join(''))
      .join(' ');
    expect(tableText).toContain('-7.85000 Ha');
  });
});
