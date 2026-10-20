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

  it('seals the union bound via the server rail, never locally', async () => {
    const seen: string[] = [];
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown, init?: { body?: unknown }) => {
      const url = String(input);
      seen.push(url);
      if (url.includes('/api/codesign/budget')) {
        const body = JSON.parse(String((init?.body as string) ?? '{}'));
        expect(Array.isArray(body.entries)).toBe(true);
        expect(body.entries).toHaveLength(2);
        return new Response(
          JSON.stringify({
            ok: true,
            rows: [
              { source: 'carry', per_use_rate: 5e-7, uses: 40, contribution: 2e-5 },
              { source: 'history', per_use_rate: 2.4e-6, uses: 2, contribution: 4.8e-6 },
            ],
            union_bound: 2.48e-5,
          }),
          { status: 200 }
        );
      }
      return new Response('not found', { status: 404 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      const seal = root!.root.findByProps({ children: 'Seal union bound' });
      await act(async () => {
        await (seal.props as { onClick: () => Promise<void> }).onClick();
      });
      const text = textOf(root!.root);
      expect(seen.some((u) => u.includes('/api/codesign/budget'))).toBe(true);
      expect(text).toContain('server-sealed');
      expect(text).toContain('2.480e-5');
      expect(text).not.toContain('(local estimate)');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('surfaces server refusal on the budget rail', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async () =>
      new Response(JSON.stringify({ code: 'bad_request', message: 'entries must be a list of <=32: refusing' }), {
        status: 400,
      });
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      const seal = root!.root.findByProps({ children: 'Seal union bound' });
      await act(async () => {
        await (seal.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root!.root)).toContain('entries must be a list of <=32: refusing');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });
});
