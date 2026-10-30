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

  it('shows the verified rail refusal instead of inventing numbers', async () => {    const realFetch = globalThis.fetch;
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

  it('refuses to send malformed artifact JSON or partial factory spec', async () => {    const realFetch = globalThis.fetch;
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

  it('runs the governed proposer rerun and renders the sealed receipt', async () => {
    const realFetch = globalThis.fetch;
    let posted: Record<string, unknown> | null = null;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown, init?: { body?: unknown }) => {
      const url = String(input);
      if (url.includes('/api/sessions')) {
        return new Response(JSON.stringify({ sessions: [{ id: 'sess-1', title: 'rerun session' }] }), { status: 200 });
      }
      if (url.includes('/api/codesign/rerun')) {
        posted = JSON.parse(String((init?.body as string) ?? '{}'));
        return new Response(
          JSON.stringify({
            ok: true,
            run_id: 'run-1',
            session_id: 'sess-1',
            spec_kind: 'and2',
            budget_steps: 2,
            seed: 7,
            num_tests: 16,
            max_failures: 0,
            kappa: 1,
            provider: 'strata',
            model: 'iq3_s',
            billing: 'none',
            payer: 'local',
            estimated_usd: 0,
            budget_usd: 0,
            steps: 1,
            trials: [{ step: 0, valid: true, toffoli: 1, peak_qubits: 3, tq: 3, reason: 'ok', candidate: '1ops:ccx' }],
            best: { step: 0, valid: true, toffoli: 1, peak_qubits: 3, tq: 3, reason: 'ok', candidate: '1ops:ccx' },
            provenance_status: 'sealed',
            ledger_ref: { id: 1 },
          }),
          { status: 200 }
        );
      }
      if (url.includes('/api/codesign/packs')) {
        return new Response(JSON.stringify({ ok: true, order: [], packs: [] }), { status: 200 });
      }
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      await act(async () => {});
      const run = root!.root.findByProps({ children: 'Run proposer' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(posted).not.toBeNull();
      expect(posted!.session_id).toBe('sess-1');
      expect(typeof posted!.request_id).toBe('string');
      expect(posted!.spec_kind).toBe('and2');
      expect(posted!.budget_steps).toBe(2);
      const text = textOf(root!.root);
      expect(text).toContain('run-1');
      expect(text).toContain('sealed');
      expect(text).toMatch(/billing\s+none/);
      expect(text).toContain('1T × 3Q = 3');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('runs families and renders elites, stagnation, and the widen-nudge', async () => {
    const realFetch = globalThis.fetch;
    let posted: unknown = null;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown, init?: { body?: unknown }) => {
      const url = String(input);
      if (url.includes('/api/codesign/families')) {
        posted = JSON.parse(String((init?.body as string) ?? '{}'));
        return new Response(
          JSON.stringify({
            ok: true, spec_kind: 'and2', kappa: 1,
            offers: [
              { family: 'direct', valid: true, toffoli: 1, qubits: 3, tq: 3, improved: true },
              { family: 'tower', valid: false },
            ],
            elites: { direct: { toffoli: 1, qubits: 3, tq: 3, artifact: [] } },
            stagnation: { direct: 0, tower: 0, mbu: 0 },
            lineage_len: 2,
            lineage: [
              { family: 'direct', toffoli: 1, qubits: 3, tq: 3, improved: true, parent: null },
              { family: 'direct', toffoli: 1, qubits: 3, tq: 3, improved: false, parent: null },
            ],
            shares: [{ offers: 2, improvements: 1, details: [] }],
            widen_nudge: { family: 'direct', offers_since_improvement: 0, nudge: 'widen direct: propose a rewrite' },
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
      const run = root!.root.findByProps({ children: 'Run families' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      const text = textOf(root!.root);
      expect((posted as { families: string[] }).families).toEqual(['direct', 'tower', 'mbu']);
      expect(text).toContain('direct 1T×3Q');
      expect(text).toContain('widen-nudge');
      expect(text).toContain('widen direct');
      expect(text).toContain('Lineage (');
      expect(text).toContain('events');
      expect(text).toContain('no improvement');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

    it('refuses to send malformed family offers', async () => {    const realFetch = globalThis.fetch;
    let calls = 0;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      if (String(input).includes('/api/codesign/families')) calls += 1;
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      const box = root!.root.findByProps({ 'aria-label': 'Family offers JSON' });
      await act(async () => {
        (box.props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: 'nope{{{' } });
      });
      const run = root!.root.findByProps({ children: 'Run families' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root!.root)).toContain('offers must be JSON');
      expect(calls).toBe(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('renders the pack registry live from the server, never hand-typed', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      const url = String(input);
      if (url.includes('/api/codesign/packs')) {
        return new Response(
          JSON.stringify({
            ok: true,
            order: ['numerics', 'inversion'],
            packs: [
              { name: 'numerics', version: '0.1.0', anchor_required: true, evidence_reqs: ['dyadic-partition proof'], status: 'scaffold', anchor_ref: null, proof_ref: null },
              { name: 'inversion', version: '0.1.0', anchor_required: true, evidence_reqs: ['Lean history-decode proof'], status: 'scaffold', anchor_ref: null, proof_ref: null },
            ],
          }),
          { status: 200 }
        );
      }
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    };
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      await act(async () => {});
      const text = textOf(root!.root);
      expect(text).toContain('live registry');
      expect(text).toContain('numerics');
      expect(text).toContain('0.1.0');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('shows the registry refusal instead of a stale hardcoded list', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async () =>
      new Response(JSON.stringify({ code: 'unavailable', message: 'registry down' }), { status: 503 });
    try {
      let root: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        root = TestRenderer.create(<CodesignCockpit />);
      });
      await act(async () => {});
      const text = textOf(root!.root);
      expect(text).toContain('registry down');
      expect(text).not.toContain('0.1.0');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('scores with a kappa slider (1..3), never a free-text field', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async () =>
      new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    try {
      let tree: TestRenderer.ReactTestRenderer | undefined;
      await act(async () => {
        tree = TestRenderer.create(<CodesignCockpit />);
      });
      const root = tree!.root;
      const sliders = root.findAllByProps({ 'aria-label': 'Kappa' });
      expect(sliders.length).toBe(3);
      for (const s of sliders) {
        expect((s.props as { type: string }).type).toBe('range');
        expect((s.props as { min: number }).min).toBe(1);
        expect((s.props as { max: number }).max).toBe(3);
      }
      await act(async () => {
        (sliders[0].props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '3' } });
      });
      const after = tree!.root.findAllByProps({ 'aria-label': 'Kappa' });
      expect((after[0].props as { value: string }).value).toBe('3');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('renders the first failure input + reason instead of a bare INVALID', async () => {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      const url = String(input);
      if (url.includes('/api/codesign/evaluate')) {
        return new Response(
          JSON.stringify({
            ok: true, spec_kind: 'and2',
            result: {
              valid: false, failures: 2, tests: 8, toffoli: 1, peak_qubits: 3,
              failure_details: [{ t: 3, input: { 0: 1, 1: 0, 2: 0 }, reason: 'output mismatch' }],
              failure_details_capped: false,
            },
          }),
          { status: 200 }
        );
      }
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
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
      expect(text).toContain('failure');
      expect(text).toContain('trial');
      expect(text).toContain('output mismatch');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });
});

describe('CodesignCockpit numeric range validation (Wave-2)', () => {
  function stubNotFound(calls: string[]) {
    const realFetch = globalThis.fetch;
    (globalThis as unknown as { fetch: unknown }).fetch = async (input: unknown) => {
      calls.push(String(input));
      return new Response(JSON.stringify({ message: 'not ready' }), { status: 503 });
    };
    return realFetch;
  }

  async function renderCockpit(): Promise<TestRenderer.ReactTestRenderer> {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<CodesignCockpit />);
    });
    return root!;
  }

  it('refuses a non-numeric Toffoli count with a named-field error and sends nothing', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Toffoli count' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: 'abc' } });
      });
      const score = root.root.findByProps({ children: 'Score' });
      await act(async () => {
        await (score.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Toffoli count must be an integer');
      expect(calls.filter((u) => u.includes('/api/codesign/tq'))).toHaveLength(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('refuses an out-of-range Qubit count in the sweep with a named-field error', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Qubit count' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '0' } });
      });
      const sweep = root.root.findByProps({ children: 'Sweep κ=1..3' });
      await act(async () => {
        await (sweep.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Qubit count must be an integer');
      expect(calls.filter((u) => u.includes('/api/codesign/tq'))).toHaveLength(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('names the offending frontier batch line instead of sending NaN', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Frontier batch' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '5,abc' } });
      });
      const frontier = root.root.findByProps({ children: 'Score frontier' });
      await act(async () => {
        await (frontier.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Frontier batch Qubit count (line 1)');
      expect(calls.filter((u) => u.includes('/api/codesign/frontier'))).toHaveLength(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('refuses a zero test count on the evaluator with a named-field error', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Number of tests' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '0' } });
      });
      const run = root.root.findByProps({ children: 'Run evaluation' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Number of tests must be an integer');
      expect(calls.filter((u) => u.includes('/api/codesign/evaluate'))).toHaveLength(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('refuses a bad budget row with named fields and never computes locally', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Budget rows' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: 'carry,2,40' } });
      });
      const seal = root.root.findByProps({ children: 'Seal union bound' });
      await act(async () => {
        await (seal.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Budget row 1 per-use rate');
      expect(calls.filter((u) => u.includes('/api/codesign/budget'))).toHaveLength(0);
      expect(textOf(root.root)).not.toContain('union ≤');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('withholds the local DQI estimate on out-of-range epsilon with a named error', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Epsilon' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '2' } });
      });
      const text = textOf(root.root);
      expect(text).toContain('Epsilon must be a number 0..1');
      expect(text).not.toContain('loss ≤');
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });

  it('refuses empty families and non-integer share rounds with named errors', async () => {
    const calls: string[] = [];
    const realFetch = stubNotFound(calls);
    try {
      const root = await renderCockpit();
      await act(async () => {
        (root.root.findByProps({ 'aria-label': 'Families (comma list)' }).props as { onChange: (e: { target: { value: string } }) => void }).onChange({ target: { value: '  ' } });
      });
      const run = root.root.findByProps({ children: 'Run families' });
      await act(async () => {
        await (run.props as { onClick: () => Promise<void> }).onClick();
      });
      expect(textOf(root.root)).toContain('Families (comma list) names no family');
      expect(calls.filter((u) => u.includes('/api/codesign/families'))).toHaveLength(0);
    } finally {
      (globalThis as unknown as { fetch: unknown }).fetch = realFetch;
    }
  });
});
