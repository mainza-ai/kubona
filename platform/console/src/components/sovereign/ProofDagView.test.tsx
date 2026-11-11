import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ELK from 'elkjs/lib/elk.bundled.js';
import type { CampaignState } from '@/types';
import { ProofDagView } from './ProofDagView';
import {
  buildElkGraphInput,
  buildFallbackDagLayout,
  extractElkLayoutResult,
} from './proof-dag-layout';

vi.mock('@/lib/api', () => ({
  fetchQirCompile: vi.fn(async () => null),
}));

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

const origWorker = globalThis.Worker;

afterEach(() => {
  globalThis.Worker = origWorker;
  vi.restoreAllMocks();
});

describe('proof-dag-layout and ProofDagView', () => {
  it('computes deterministic sync-fallback layering, detects cycles, and groups wire_cuts partitions', () => {
    const fallback = buildFallbackDagLayout(
      dagCampaign.quantum_factors!.dag_nodes as unknown[],
      dagCampaign.quantum_factors!.dag_edges as unknown[],
      dagCampaign.quantum_factors!.wire_cuts as Record<string, unknown>[],
    );
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
    const fallback = buildFallbackDagLayout(
      dagCampaign.quantum_factors!.dag_nodes as unknown[],
      dagCampaign.quantum_factors!.dag_edges as unknown[],
      dagCampaign.quantum_factors!.wire_cuts as Record<string, unknown>[],
    );
    const elk = new ELK();
    const elkInput = buildElkGraphInput(
      fallback.nodes,
      fallback.edges,
      dagCampaign.quantum_factors!.wire_cuts as Record<string, unknown>[],
    );
    const laidOut = await elk.layout(elkInput);
    const result = extractElkLayoutResult(laidOut, fallback.nodes, fallback.edges, false);

    expect(result.engine).toBe('elk-worker');
    expect(result.clusters).toHaveLength(2);
    expect(result.edgePaths.e0).toContain('M');
    expect(result.pos.n0.x).toBeLessThan(result.pos.n3.x);
  }, 15_000);

  it('renders sync-fallback when Worker is unavailable and swaps cleanly when a mocked Worker posts ELK layout', async () => {
    // 1. No Worker in environment -> renders sync-fallback cleanly
    // @ts-expect-error test override
    delete globalThis.Worker;
    let noWorkerTree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      noWorkerTree = TestRenderer.create(<ProofDagView campaign={dagCampaign} />);
    });
    const rootNoWorker = noWorkerTree!.root.findByProps({ 'data-testid': 'proof-dag' });
    expect(rootNoWorker.props['data-layout-engine']).toBe('sync-fallback');
    expect(noWorkerTree!.root.findAllByProps({ 'data-testid': 'dag-cluster' })).toHaveLength(2);
    await act(async () => { noWorkerTree!.unmount(); });

    // 2. Mocked Worker -> swaps to elk-worker layout once worker posts response
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
    globalThis.Worker = FakeWorker as unknown as typeof Worker;

    let workerTree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      workerTree = TestRenderer.create(<ProofDagView campaign={dagCampaign} />);
    });
    expect(workerTree!.root.findByProps({ 'data-testid': 'proof-dag' }).props['data-layout-engine']).toBe('sync-fallback');

    const fallback = buildFallbackDagLayout(
      dagCampaign.quantum_factors!.dag_nodes as unknown[],
      dagCampaign.quantum_factors!.dag_edges as unknown[],
      dagCampaign.quantum_factors!.wire_cuts as Record<string, unknown>[],
    );
    await act(async () => {
      onMsg!({
        data: {
          seq: postedSeq,
          layout: { ...fallback, engine: 'elk-worker' },
        },
      } as MessageEvent);
    });
    expect(workerTree!.root.findByProps({ 'data-testid': 'proof-dag' }).props['data-layout-engine']).toBe('elk-worker');
    await act(async () => { workerTree!.unmount(); });
  });
});
