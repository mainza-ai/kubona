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

  it('renders the verified decoder card from the served document', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      const url = String(input);
      if (url.includes('/api/codesign/verified')) {
        return new Response(
          JSON.stringify({
            program: 'paper-scale RS decoder',
            version: 1,
            generated: '2026-10-20',
            generator: 'gen',
            labels: ['measured on simulator'],
            total: { toffoli: 3928980, qubits: 112168, pin: 'slice60' },
            components: [
              { name: 'bm_walk_70r', toffoli: 2422140, scope: 'standalone', pin: 'slice58' },
            ],
            proofs: ['slice60 end-to-end (FLAG=1)'],
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
      await act(async () => {});
      const text = textOf(root!.root);
      expect(text).toContain('Verified RS decoder');
      expect(text).toContain('3,928,980');
      expect(text).toContain('112,168');
      expect(text).toContain('bm_walk_70r');
      expect(text).toContain('measured on simulator');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('shows the verified rail refusal instead of inventing numbers', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async () =>
      new Response(JSON.stringify({ code: 'unavailable', message: 'verified results unavailable' }), {
        status: 503,
      });
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      await act(async () => {});
      const text = textOf(root!.root);
      expect(text).toContain('Verified RS decoder');
      expect(text).toContain('verified results unavailable');
      expect(text).not.toContain('3,928,980');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('runs an evaluation and renders validity + counts + factory', async () => {
    const realFetch = globalThis.fetch;
    const seen: string[] = [];
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown, init?: { body?: unknown }) => {
      const url = String(input);
      seen.push(url);
      if (url.includes('/api/codesign/evaluate')) {
        const body = JSON.parse(String((init?.body as string) ?? '{}'));
        expect(body.spec_kind).toBe('and2');
        return new Response(
          JSON.stringify({
            ok: true,
            spec_kind: 'and2',
            result: {
              valid: true, toffoli: 1, peak_qubits: 3, eps_bound_97_5: 0.3,
              depth: 1, toffoli_depth: 1, active_qubits: 3,
              kappa_sweep: { 1: 3, 2: 30, 3: 300 },
              factory: { factory: 'gf2019_ccz', serial_seconds_est: 0.0011, within_failure_budget: true },
            },
          }),
          { status: 200 }
        );
      }
      if (url.includes('/api/codesign/verified')) {
        return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
      }
      return new Response('not found', { status: 404 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      const run = root!.root.findByProps({ children: 'Run evaluation' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      const text = textOf(root!.root);
      expect(seen.some((u) => u.includes('/api/codesign/evaluate'))).toBe(true);
      expect(text).toContain('VALID');
      expect(text).toContain('within accuracy budget');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('refuses to send malformed artifact JSON or partial factory spec', async () => {
    const realFetch = globalThis.fetch;
    let calls = 0;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      if (String(input).includes('/api/codesign/evaluate')) calls += 1;
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      const artifact = root!.root.findByProps({ 'aria-label': 'Artifact JSON' });
      await act(async () => {
        (artifact.props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: 'not json{{{' } });
      });
      const run = root!.root.findByProps({ children: 'Run evaluation' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root!.root)).toContain('artifact must be JSON');
      expect(calls).toBe(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });
});
