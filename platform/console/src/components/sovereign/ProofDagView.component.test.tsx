import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import ELK from 'elkjs/lib/elk.bundled.js';
import type { CampaignState } from '@/types';
import { ProofDagView } from './ProofDagView';
import {
  buildElkGraphInput,
  buildFallbackDagLayout,
  extractElkLayoutResult,
} from './proof-dag-layout';

// ProofDagView renders a layered SVG over the real edge set, falling back to a
// deterministic synchronous layout and upgrading to the ELK worker layout when
// the worker answers. The browser project lets the SVG, the cluster groups and
// the node selection be observed as real DOM.
const dagCampaign: CampaignState = {
  session_id: 'dag-1',
  research_goal: 'Partition circuit',
  target_system: 'H2',
  quantum_factors: {
    dag_nodes: [
      { id: 'n0', label: 'H q[0]', kind: 'gate', fragment: 0 },
      { id: 'n1', label: 'CNOT q[0],q[1]', kind: 'gate', fragment: 0 },
      { id: 'n2', label: 'RZ(0.4) q[1]', kind: 'gate', fragment: 1 },
      { id: 'n3', label: 'Measure', kind: 'measure', fragment: 1 },
    ],
    dag_edges: [
      { id: 'e0', source: 'n0', target: 'n1' },
      { id: 'e1', source: 'n1', target: 'n2' },
      { id: 'e2', source: 'n2', target: 'n3' },
    ],
    wire_cuts: [
      { cut_type: 'wire', total_cuts: 1, fragments: [0, 1], measured: false },
    ],
  },
} as unknown as CampaignState;

const dagNodes = dagCampaign.quantum_factors!.dag_nodes as unknown[];
const dagEdges = dagCampaign.quantum_factors!.dag_edges as unknown[];
const wireCuts = dagCampaign.quantum_factors!.wire_cuts as Record<string, unknown>[];

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('proof-dag-layout and ProofDagView in a real browser', () => {
  it('computes deterministic sync-fallback layering, detects cycles, and groups wire_cuts partitions', () => {
    const fallback = buildFallbackDagLayout(dagNodes, dagEdges, wireCuts);
    expect(fallback.engine).toBe('sync-fallback');
    expect(fallback.hasCycle).toBe(false);
    expect(fallback.clusters).toHaveLength(2);
    expect(fallback.pos.n0.x).toBeLessThan(fallback.pos.n1.x);
    expect(fallback.pos.n1.x).toBeLessThan(fallback.pos.n2.x);
    expect(fallback.edgePaths.e0).toContain('M');

    const cyclic = buildFallbackDagLayout(
      [{ id: 'a' }, { id: 'b' }],
      [{ source: 'a', target: 'b' }, { source: 'b', target: 'a' }],
      [],
    );
    expect(cyclic.hasCycle).toBe(true);
  });

  it('computes compound orthogonal layout via elkjs and extracts partition clusters', async () => {
    const fallback = buildFallbackDagLayout(dagNodes, dagEdges, wireCuts);
    const elk = new ELK();
    const elkInput = buildElkGraphInput(fallback.nodes, fallback.edges, wireCuts);
    const laidOut = await elk.layout(elkInput);
    const result = extractElkLayoutResult(laidOut, fallback.nodes, fallback.edges, false);

    expect(result.engine).toBe('elk-worker');
    expect(result.clusters).toHaveLength(2);
    expect(result.edgePaths.e0).toContain('M');
    expect(result.pos.n0.x).toBeLessThan(result.pos.n3.x);
  }, 15_000);

  it('renders sync-fallback when Worker is unavailable, with selectable nodes', async () => {
    // The component treats a missing Worker as "no background layout
    // available" and keeps the sync fallback.
    vi.stubGlobal('Worker', undefined);
    const screen = await render(<ProofDagView campaign={dagCampaign} />);

    const root = screen.container.querySelector('[data-testid="proof-dag"]') as HTMLElement;
    expect(root).not.toBeNull();
    expect(root.getAttribute('data-layout-engine')).toBe('sync-fallback');
    expect(screen.container.querySelectorAll('[data-testid="dag-cluster"]')).toHaveLength(2);

    const svg = screen.container.querySelector('svg');
    expect(svg?.getAttribute('aria-label')).toBe('Circuit DAG: 4 nodes, 3 edges');

    // Real pointer interaction: clicking a node opens its record card.
    const node = screen.getByRole('button', { name: 'Select circuit node n1' });
    expect(node.element().getAttribute('aria-pressed')).toBe('false');
    await node.click();
    await expect.poll(() => node.element().getAttribute('aria-pressed')).toBe('true');
    await expect
      .poll(() => screen.container.textContent)
      .toContain('no provenance receipt on this campaign');
    expect(screen.container.textContent).toContain('id n1 · gate');

    await screen.unmount();
  });

  it('swaps cleanly to the elk-worker layout when the worker posts a response', async () => {
    let postedSeq = 0;
    let onMsg: ((ev: MessageEvent) => void) | null = null;
    class FakeWorker {
      onmessage: ((ev: MessageEvent) => void) | null = null;
      onerror: (() => void) | null = null;
      postMessage(data: { seq: number }) {
        postedSeq = data.seq;
        onMsg = this.onmessage;
      }
      terminate() {}
    }
    vi.stubGlobal('Worker', FakeWorker as unknown as typeof Worker);

    const screen = await render(<ProofDagView campaign={dagCampaign} />);
    const root = screen.container.querySelector('[data-testid="proof-dag"]') as HTMLElement;
    // The synchronous fallback is painted on frame 1, before any worker reply.
    expect(root.getAttribute('data-layout-engine')).toBe('sync-fallback');
    await expect.poll(() => postedSeq).toBeGreaterThan(0);

    const fallback = buildFallbackDagLayout(dagNodes, dagEdges, wireCuts);
    onMsg!({
      data: {
        seq: postedSeq,
        layout: { ...fallback, engine: 'elk-worker' },
      },
    } as MessageEvent);

    await expect.poll(() => root.getAttribute('data-layout-engine')).toBe('elk-worker');
    expect(screen.container.querySelectorAll('[data-testid="dag-cluster"]')).toHaveLength(2);

    await screen.unmount();
  });
});
