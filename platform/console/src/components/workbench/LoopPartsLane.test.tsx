import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import * as api from '@/lib/api';
import { LoopPartsLane } from './LoopPartsLane';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return { ...actual, fetchLoopParts: vi.fn(), steerLoop: vi.fn() };
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node && typeof node === 'object' && 'children' in node) {
    const children = (node as { children?: unknown }).children;
    if (Array.isArray(children)) return children.map(textOf).join(' ');
  }
  return '';
}

describe('LoopPartsLane (C1)', () => {
  it('renders helper columns with the active part and the job need', async () => {
    vi.mocked(api.fetchLoopParts).mockResolvedValue({
      ok: true,
      data: [
        { part_id: 'p1', loop_id: 'l1', helper: 'lead', title: 'Optimizer step 1', state: 'done', began_ms: 1, now: '‖∇‖ = 0.05' },
        { part_id: 'p2', loop_id: 'l1', helper: 'quantum_job', title: 'Dispatched circuit_run', state: 'failed', began_ms: 2, job_status: 'failed', need: { kind: 'backend_offline', reason: 'bridge unreachable' } },
      ],
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<LoopPartsLane loopId="l1" live />);
    });
    await act(async () => {
      await Promise.resolve();
    });
    const lane = tree.root.findByProps({ 'data-testid': 'run-parts' });
    const text = textOf(lane);
    expect(text).toContain('lead');
    expect(text).toContain('quantum job');
    expect(text).toContain('Optimizer step 1');
    expect(text).toContain('bridge unreachable');
    expect(lane.findAllByProps({ 'data-active': 'true' })).toHaveLength(0);
  });

  it('loads a settled run only on request', async () => {
    vi.mocked(api.fetchLoopParts).mockClear();
    vi.mocked(api.fetchLoopParts).mockResolvedValue({ ok: true, data: [] });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<LoopPartsLane loopId="l3" live={false} />);
    });
    expect(api.fetchLoopParts).not.toHaveBeenCalled();
    await act(async () => {
      tree.root.findByProps({ 'data-testid': 'run-parts-open' }).props.onClick({ stopPropagation: () => {} });
    });
    expect(api.fetchLoopParts).toHaveBeenCalledWith('l3');
  });

  it('steers a live run with an explicit context disclosure (C10)', async () => {
    vi.mocked(api.fetchLoopParts).mockResolvedValue({
      ok: true,
      data: [
        { part_id: 'p1', loop_id: 'l1', helper: 'lead', title: 'Optimizer step 1', state: 'running', began_ms: 1 },
      ],
    });
    vi.mocked(api.steerLoop).mockResolvedValue({
      ok: true,
      steer: { id: 'steer-12345678', loop_id: 'l1', text: 'shorter ansatz', status: 'pending', created_at: '' },
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<LoopPartsLane loopId="l1" live />);
    });
    await act(async () => { await Promise.resolve(); });
    const box = tree.root.findByProps({ 'data-testid': 'run-steer' });
    expect(textOf(box)).toContain('Applied at the next step boundary');
    expect(textOf(box)).toContain('active: Optimizer step 1');
    const input = tree.root.findByProps({ 'aria-label': 'Steer this run' });
    await act(async () => { input.props.onChange({ target: { value: 'shorter ansatz' } }); });
    await act(async () => {
      tree.root.findByProps({ 'data-testid': 'run-steer-submit' }).props.onClick();
    });
    await act(async () => { await Promise.resolve(); });
    expect(api.steerLoop).toHaveBeenCalledWith('l1', 'shorter ansatz');
    expect(textOf(tree.root.findByProps({ 'data-testid': 'run-steer-receipt' }))).toContain('next step boundary');
  });

  it('never renders an unreadable projection as empty', async () => {
    vi.mocked(api.fetchLoopParts).mockResolvedValue({
      ok: false,
      error: 'loops refused',
      failure: { path: '/api/loops/l2/parts', status: 401, kind: 'auth', message: 'login required', at: Date.now() },
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<LoopPartsLane loopId="l2" live />);
    });
    await act(async () => {
      await Promise.resolve();
    });
    const refused = tree.root.findByProps({ 'data-testid': 'run-parts-refused' });
    expect(textOf(refused)).toContain('unavailable');
    expect(tree.root.findAllByProps({ 'data-testid': 'run-parts' })).toHaveLength(0);
  });
});
