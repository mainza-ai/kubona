import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CodesignCockpit } from './CodesignCockpit';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('CodesignCockpit read-only', () => {
  it('renders TQ + frontier without job submission', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<CodesignCockpit />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('TQ scoring');
    expect(text).toContain('MAP-Elites frontier');
    expect(text).toContain('Kappa sweep');
    expect(text).toContain('Families');
    expect(text).toContain('DQI loss bound');
    expect(text).toContain('Domain packs');
    expect(text).not.toMatch(/submit job/i);
  });
});
