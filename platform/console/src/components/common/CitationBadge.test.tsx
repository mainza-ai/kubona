import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CitationBadge } from './CitationBadge';
import type { CitationReport } from '@/types';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node && typeof node === 'object' && 'children' in node) {
    const children = (node as { children?: unknown }).children;
    if (Array.isArray(children)) return children.map(textOf).join(' ');
  }
  return '';
}

describe('CitationBadge (B5)', () => {
  it('states verified claims plainly', () => {
    const report: CitationReport = {
      observations: [{ id: 'o1', tool: 'get_job' }],
      claims: [
        { number: '-1.13727', citation: 'o1', status: 'verified' },
        { number: '1024', citation: 'o1', status: 'verified' },
      ],
      verified: 2,
      unverified: 0,
    };
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CitationBadge report={report} />);
    });
    const badge = tree.root.findByProps({ 'data-testid': 'citation-badge' });
    expect(badge.props['data-state']).toBe('verified');
    expect(textOf(badge)).toContain('2/2 numeric claims verified');
    expect(textOf(badge)).not.toContain('unverified');
  });

  it('names unverified claims instead of hiding them', () => {
    const report: CitationReport = {
      observations: [{ id: 'o1', tool: 'get_job' }],
      claims: [
        { number: '-1.13727', citation: 'o1', status: 'verified' },
        { number: '0.7414', status: 'uncited' },
        { number: '5.5', citation: 'o9', status: 'unknown_observation' },
      ],
      verified: 1,
      unverified: 2,
    };
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CitationBadge report={report} />);
    });
    const badge = tree.root.findByProps({ 'data-testid': 'citation-badge' });
    expect(badge.props['data-state']).toBe('partial');
    expect(textOf(badge)).toContain('1/3 numeric claims verified');
    expect(textOf(badge)).toContain('2 unverified');
    expect(badge.props.title).toContain('0.7414 — no citation');
    expect(badge.props.title).toContain('[o9] — unknown observation id');
  });

  it('renders nothing without a report', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CitationBadge report={null} />);
    });
    expect(tree.toJSON()).toBeNull();
  });
});
