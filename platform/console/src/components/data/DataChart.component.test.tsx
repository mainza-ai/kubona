import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { Chart } from './DataChart';
import type { ChartSpec } from './chart';

// Real-browser port of DataChart.test.tsx. The assertions keep the same
// contract — accessible name/description, the exact-values table, the
// all-null refusal, the compact table, actionable evidence, the null gap,
// embedded title suppression, tick thinning, zero-only anchoring, the
// hover tooltip grace and the clamp markers — but they now read real DOM
// nodes and drive real pointer/keyboard input in Chromium.

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

const tickTexts = (root: ParentNode) =>
  Array.from(root.querySelectorAll('.k-chart-tick')).map((node) => node.textContent ?? '');

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

describe('Chart in a real browser', () => {
  it('names the plot and keeps every exact value in a table', async () => {
    const screen = await render(<Chart spec={spec} />);
    const svg = screen.getByRole('img').element();
    expect(svg.getAttribute('role')).toBe('img');
    // D5: the computed summary must be announced alongside the title, so it
    // lives in a <desc> referenced by aria-describedby (aria-labelledby wins
    // over aria-label and would otherwise silence the summary).
    const describedBy = svg.getAttribute('aria-describedby');
    expect(describedBy).toBeTruthy();
    const desc = document.getElementById(describedBy!);
    expect(desc?.tagName.toLowerCase()).toBe('desc');
    expect(desc?.textContent).toContain('Energy trajectory');
    expect(desc?.textContent).toContain('low');
    const bodyRows = screen.container.querySelectorAll('tbody tr');
    expect(bodyRows).toHaveLength(2);
    expect(screen.container.querySelectorAll('details')).toHaveLength(1);
  });

  it('refuses an all-null chart instead of drawing empty axes (D5)', async () => {
    const empty: ChartSpec = {
      ...spec,
      basis: 'Awaiting the first measured shot.',
      series: [{ name: 'VQE', points: [{ x: 1, y: null }, { x: 2, y: null }] }],
    };
    const screen = await render(<Chart spec={empty} />);
    const refusal = screen.getByTestId('k-chart-unavailable');
    await expect.element(refusal).toHaveAttribute('role', 'status');
    expect(screen.container.querySelectorAll('svg')).toHaveLength(0);
    expect(screen.container.textContent).toContain('Chart unavailable');
    expect(screen.container.textContent).toContain('Awaiting the first measured shot.');
  });

  it('keeps the exact-values table in compact mode (D5)', async () => {
    const screen = await render(<Chart spec={{ ...spec, compact: true }} />);
    expect(screen.container.querySelectorAll('details')).toHaveLength(1);
    expect(screen.container.querySelector('table')).not.toBeNull();
  });

  it('renders evidence as actionable buttons when the consumer provides onEvidence (D5)', async () => {
    const onEvidence = vi.fn();
    const withEvidence: ChartSpec = {
      ...spec,
      series: [{
        name: 'VQE',
        points: [{ x: 1, y: -1.1, evidence: [{ key: 'ev-1', label: 'Ledger receipt' }] }],
      }],
    };
    const screen = await render(<Chart spec={withEvidence} onEvidence={onEvidence} />);
    // The table is inside <details>; open it with a real click first.
    await screen.getByText('Exact values').click();
    const button = screen.getByTestId('k-chart-evidence-button').first();
    await expect.element(button).toBeVisible();
    await button.click();
    expect(onEvidence).toHaveBeenCalledWith({ key: 'ev-1', label: 'Ledger receipt' });
  });

  it('draws a gap instead of a zero for a null point', async () => {
    const withGap: ChartSpec = {
      ...spec,
      series: [{ name: 'VQE', points: [{ x: 1, y: 1 }, { x: 2, y: null }, { x: 3, y: 2 }] }],
    };
    const screen = await render(<Chart spec={withGap} />);
    const rows = screen.container.querySelectorAll('tbody tr');
    expect(rows[1]?.querySelectorAll('td')[0]?.textContent).toContain('—');
  });

  it('suppresses duplicate visible title when embedded while keeping accessible aria-labelledby', async () => {
    const screen = await render(<Chart spec={spec} embedded />);
    const header = screen.container.querySelector('header')!;
    expect(header.className).toContain('k-chart-head-embedded');
    const hiddenWrap = header.querySelector('.sr-only')!;
    expect(hiddenWrap).not.toBeNull();
    const figcaption = hiddenWrap.querySelector('figcaption')!;
    expect(figcaption.textContent).toBe('Energy trajectory');
    const svg = screen.container.querySelector('svg')!;
    expect(svg.getAttribute('aria-labelledby')).toBe(figcaption.id);
  });

  it('flips bars horizontally on long category labels and thins > 12 X ticks on lines', async () => {
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
    const barScreen = await render(<Chart spec={barSpec} />);
    expect(barScreen.container.querySelector('figure')?.getAttribute('data-horizontal')).toBe('true');
    const ticks = tickTexts(barScreen.container);
    expect(ticks.some((text) => text.includes('ibm_marrakesh_h'))).toBe(true);
    await barScreen.unmount();

    const crowdedLine: ChartSpec = {
      kind: 'line',
      title: 'Dense trace',
      series: [{
        name: 'Energy',
        points: Array.from({ length: 20 }, (_, i) => ({ x: `S${i + 1}`, y: -1 - i * 0.01 })),
      }],
    };
    const lineScreen = await render(<Chart spec={crowdedLine} />);
    const renderedKeys = tickTexts(lineScreen.container).filter((text) => text.startsWith('S'));
    expect(renderedKeys.length).toBeLessThanOrEqual(8);
  });

  it('anchors zero-only bars on [0, 1] and expands horizontal gutters for long labels', async () => {
    const zeroBars: ChartSpec = {
      kind: 'bars',
      title: 'Zero counts',
      series: [{ name: 'Shots', points: [{ x: '00', y: 0 }, { x: '11', y: 0 }] }],
    };
    const zeroScreen = await render(<Chart spec={zeroBars} />);
    const yTicks = tickTexts(zeroScreen.container);
    expect(yTicks).toContain('0');
    expect(yTicks).not.toContain('-1');
    await zeroScreen.unmount();

    const longBarSpec: ChartSpec = {
      kind: 'bars',
      title: 'Backend error',
      series: [{ name: 'Error', points: [{ x: 'ibm_marrakesh_heron_v2', y: 0.012 }] }],
    };
    const longScreen = await render(<Chart spec={longBarSpec} />);
    const ticks = Array.from(longScreen.container.querySelectorAll('.k-chart-tick'));
    expect(ticks.map((node) => node.textContent ?? '')).toContain('ibm_marrakesh_heron_v2');
    const valNode = ticks.find((node) => (node.textContent ?? '') === '0.012');
    expect(valNode).toBeDefined();
    expect(Number(valNode!.getAttribute('x'))).toBeLessThan(640);
  });

  it('shows pointer/focus hover tooltip and hides it after 160ms Hover.hide grace', async () => {
    const barSpec: ChartSpec = {
      kind: 'bars',
      title: 'Energies',
      series: [{ name: 'VQE', points: [{ x: 'run1', y: -1.11, evidence: [{ key: 'job', label: 'job:123' }] }] }],
    };
    const screen = await render(<Chart spec={barSpec} />);
    const rect = screen.container.querySelector('rect')!;
    await userEvent.hover(rect);
    const tip = screen.getByTestId('k-chart-tip');
    await expect.element(tip).toBeVisible();
    expect(tip.element().querySelector('strong')?.textContent).toBe('-1.11');
    expect(tip.element().querySelector('.k-chart-tip-evidence')?.textContent).toContain('job:123');

    // The grace is 160 ms: 100 ms after leaving the tip is still on screen.
    await userEvent.unhover(rect);
    await sleep(80);
    expect(screen.container.querySelector('[data-testid="k-chart-tip"]')).not.toBeNull();
    await expect.poll(
      () => screen.container.querySelector('[data-testid="k-chart-tip"]'),
      { timeout: 1000 },
    ).toBeNull();
  });

  it('keeps the keyboard path to the same tooltip', async () => {
    const barSpec: ChartSpec = {
      kind: 'bars',
      title: 'Energies',
      series: [{ name: 'VQE', points: [{ x: 'run1', y: -1.11 }] }],
    };
    const screen = await render(<Chart spec={barSpec} />);
    const rect = screen.container.querySelector('rect')!;
    rect.focus();
    await expect.element(screen.getByTestId('k-chart-tip')).toBeVisible();
    rect.blur();
    await expect.poll(
      () => screen.container.querySelector('[data-testid="k-chart-tip"]'),
      { timeout: 1000 },
    ).toBeNull();
  });

  it('clamps off-scale outliers with accessible diamond markers while keeping exact raw table values', async () => {
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
    const screen = await render(<Chart spec={clampedSpec} />);
    expect(screen.container.querySelectorAll('[data-outlier="true"]')).toHaveLength(1);
    const tableText = Array.from(screen.container.querySelectorAll('table td'))
      .map((cell) => cell.textContent ?? '')
      .join(' ');
    expect(tableText).toContain('-7.85000 Ha');
  });
});
