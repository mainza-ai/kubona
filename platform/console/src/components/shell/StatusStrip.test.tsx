import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { API_PROBLEM_EVENT } from '@/lib/api';
import { StatusStrip } from './StatusStrip';

vi.mock('@/components/common/BackendStatusBar', () => ({
  BackendStatusBar: () => React.createElement('span', null, 'backend ready'),
}));
vi.mock('@/components/sovereign/CheckInStrip', () => ({
  CheckInStrip: () => React.createElement('span', null, '0 loops · $0.00'),
}));

function stubWindow(): EventTarget {
  const target = new EventTarget();
  vi.stubGlobal('window', target);
  return target;
}

describe('StatusStrip problem chips (3.9)', () => {
  it('shows a refusal as a chip, dismisses it by signature, and re-arms on a different problem', () => {
    const target = stubWindow();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<StatusStrip loops={[]} />);
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'api-problem-chip' })).toHaveLength(0);

    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/loops', status: 500, message: 'failed', at: Date.now() },
      }));
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'api-problem-chip' })).toHaveLength(1);

    act(() => {
      tree.root.findByProps({ 'aria-label': 'Dismiss API refusal' }).props.onClick();
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'api-problem-chip' })).toHaveLength(0);

    // The identical problem stays dismissed this session.
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/loops', status: 500, message: 'failed', at: Date.now() },
      }));
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'api-problem-chip' })).toHaveLength(0);

    // A different signature is new information and surfaces.
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/jobs', status: 503, message: 'bridge down', at: Date.now() },
      }));
    });
    const chip = tree.root.findByProps({ 'data-testid': 'api-problem-chip' });
    expect(chip.props.title).toContain('bridge down');
  });

  it('renders contract drift as a warning chip, not a blocking alert', () => {
    const target = stubWindow();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<StatusStrip loops={[]} />);
    });
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: 'contract:jobs.list', status: null, message: 'response contract drifted: jobs: expected array', at: Date.now() },
      }));
    });
    const chip = tree.root.findByProps({ 'data-testid': 'contract-drift-chip' });
    expect(chip.props.role).toBe('status');
    expect(tree.root.findAllByProps({ 'data-testid': 'api-problem-chip' })).toHaveLength(0);
  });
});
