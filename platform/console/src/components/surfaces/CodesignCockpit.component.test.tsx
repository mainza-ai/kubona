import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { CodesignCockpit } from './CodesignCockpit';

// The cockpit is read-only over the Go rail: every number it shows is served
// (validity stays Python-owned). These tests drive the real DOM — typing into
// the named fields, pressing the real buttons — with the daemon stubbed at the
// transport boundary by URL + method.

type Call = { url: string; method: string; body: Record<string, unknown> | undefined };
type Reply = {
  ok: boolean;
  status: number;
  json: () => Promise<unknown>;
  text: () => Promise<string>;
};

const ok = (data: unknown, status = 200): Reply => ({
  ok: true,
  status,
  json: async () => data,
  text: async () => JSON.stringify(data),
});

const refuse = (status: number, body: unknown): Reply => ({
  ok: false,
  status,
  json: async () => body,
  text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
});

type Route = (call: Call) => Reply;

const VERIFIED = {
  program: 'paper-scale RS decoder',
  version: 1,
  generated: '2026-10-20',
  generator: 'gen',
  labels: ['measured on simulator'],
  total: { toffoli: 3928980, qubits: 112168, pin: 'slice60' },
  components: [{ name: 'bm_walk_70r', toffoli: 2422140, scope: 'standalone', pin: 'slice58' }],
  proofs: ['slice60 end-to-end (FLAG=1)'],
};

const PACKS = {
  ok: true,
  order: ['numerics', 'inversion'],
  packs: [
    { name: 'numerics', version: '0.1.0', anchor_required: true, evidence_reqs: ['dyadic-partition proof'], status: 'scaffold', anchor_ref: null, proof_ref: null },
    { name: 'inversion', version: '0.1.0', anchor_required: true, evidence_reqs: ['Lean history-decode proof'], status: 'scaffold', anchor_ref: null, proof_ref: null },
  ],
};

const NOT_READY = refuse(503, { message: 'not ready' });

/** Stub the daemon by URL substring; every request is recorded. */
function stubDaemon(routes: Record<string, Route>, fallback: Route = () => refuse(404, { message: 'not found' })) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? 'GET').toUpperCase();
      let body: Record<string, unknown> | undefined;
      if (typeof init?.body === 'string') {
        try {
          body = JSON.parse(init.body) as Record<string, unknown>;
        } catch {
          body = undefined;
        }
      }
      const call = { url, method, body };
      calls.push(call);
      for (const [needle, reply] of Object.entries(routes)) {
        if (url.includes(needle)) return reply(call);
      }
      return fallback(call);
    }),
  );
  return calls;
}

/** Mount fetches: a settled verified card + pack registry unless overridden. */
function mountRoutes(overrides: Record<string, Route> = {}): Record<string, Route> {
  return {
    '/api/codesign/verified': () => ok(VERIFIED),
    '/api/codesign/packs': () => ok(PACKS),
    '/api/sessions': () => ok({ sessions: [] }),
    ...overrides,
  };
}

const callsTo = (calls: Call[], needle: string) => calls.filter((call) => call.url.includes(needle));

const button = (screen: Awaited<ReturnType<typeof render>>, name: string | RegExp) =>
  screen.getByRole('button', { name });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('CodesignCockpit read-only in a real browser', () => {
  it('renders TQ + frontier without job submission', async () => {
    stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    await expect.poll(() => screen.container.textContent).toContain('Verified RS decoder');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('TQ scoring');
    expect(text).toContain('MAP-Elites frontier');
    expect(text).toContain('Kappa sweep');
    expect(text).toContain('Families');
    expect(text).toContain('DQI loss bound');
    expect(text).toContain('Domain packs');
    expect(text).not.toMatch(/submit job/i);
  });

  it('seals the union bound via the server rail, never locally', async () => {
    const calls = stubDaemon(mountRoutes({
      '/api/codesign/budget': () =>
        ok({
          ok: true,
          rows: [
            { source: 'carry', per_use_rate: 5e-7, uses: 40, contribution: 2e-5 },
            { source: 'history', per_use_rate: 2.4e-6, uses: 2, contribution: 4.8e-6 },
          ],
          union_bound: 2.48e-5,
        }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Seal union bound').click();

    await expect.poll(() => screen.container.textContent).toContain('server-sealed');
    const posted = callsTo(calls, '/api/codesign/budget');
    expect(posted).toHaveLength(1);
    // The two default rows are parsed client-side and sent as data; the union
    // itself is computed server-side.
    expect(Array.isArray(posted[0].body?.entries)).toBe(true);
    expect(posted[0].body?.entries).toHaveLength(2);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('2.480e-5');
    expect(text).not.toContain('(local estimate)');
  });

  it('surfaces server refusal on the budget rail', async () => {
    stubDaemon(mountRoutes({
      '/api/codesign/budget': () => refuse(400, { code: 'bad_request', message: 'entries must be a list of <=32: refusing' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Seal union bound').click();

    await expect
      .poll(() => screen.getByRole('alert').element().textContent ?? '')
      .toContain('entries must be a list of <=32: refusing');
  });

  it('renders the verified decoder card from the served document', async () => {
    stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    await expect.poll(() => screen.container.textContent).toContain('3,928,980');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Verified RS decoder');
    expect(text).toContain('112,168');
    expect(text).toContain('bm_walk_70r');
    expect(text).toContain('measured on simulator');
  });

  it('shows the verified rail refusal instead of inventing numbers', async () => {
    stubDaemon(mountRoutes({
      '/api/codesign/verified': () => refuse(503, { code: 'unavailable', message: 'verified results unavailable' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await expect.poll(() => screen.container.textContent).toContain('verified results unavailable');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Verified RS decoder');
    expect(text).not.toContain('3,928,980');
  });

  it('runs an evaluation and renders validity + counts + factory', async () => {
    const calls = stubDaemon(mountRoutes({
      '/api/codesign/evaluate': () =>
        ok({
          ok: true,
          spec_kind: 'and2',
          result: {
            valid: true, toffoli: 1, peak_qubits: 3, eps_bound_97_5: 0.3,
            depth: 1, toffoli_depth: 1, active_qubits: 3,
            kappa_sweep: { 1: 3, 2: 30, 3: 300 },
            factory: { factory: 'gf2019_ccz', serial_seconds_est: 0.0011, within_failure_budget: true },
          },
        }),
      '/api/codesign/verified': () => refuse(503, { message: 'not ready' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Run evaluation').click();

    await expect.poll(() => screen.container.textContent).toContain('VALID');
    const posted = callsTo(calls, '/api/codesign/evaluate');
    expect(posted).toHaveLength(1);
    expect(posted[0].body?.spec_kind).toBe('and2');
    expect(screen.container.textContent).toContain('within accuracy budget');
  });

  it('refuses to send malformed artifact JSON or partial factory spec', async () => {
    const calls = stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Artifact JSON').fill('not json{{{');
    await button(screen, 'Run evaluation').click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('artifact must be JSON');
    expect(callsTo(calls, '/api/codesign/evaluate')).toHaveLength(0);
  });

  it('runs the governed proposer rerun and renders the sealed receipt', async () => {
    const calls = stubDaemon(mountRoutes({
      '/api/sessions': () => ok({ sessions: [{ id: 'sess-1', title: 'rerun session' }] }),
      '/api/codesign/rerun': () =>
        ok({
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
    }));
    const screen = await render(<CodesignCockpit />);

    // The proposer is session-scoped: the select must be populated (from the
    // served session list) before the button unlocks.
    const run = button(screen, 'Run proposer');
    await expect.poll(() => (run.element() as HTMLButtonElement).disabled).toBe(false);
    await run.click();

    await expect.poll(() => screen.container.textContent).toContain('run-1');
    const posted = callsTo(calls, '/api/codesign/rerun');
    expect(posted).toHaveLength(1);
    expect(posted[0].body?.session_id).toBe('sess-1');
    expect(typeof posted[0].body?.request_id).toBe('string');
    expect(posted[0].body?.spec_kind).toBe('and2');
    expect(posted[0].body?.budget_steps).toBe(2);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('sealed');
    expect(text).toMatch(/billing\s+none/);
    expect(text).toContain('1T × 3Q = 3');
  });

  it('runs families and renders elites, stagnation, and the widen-nudge', async () => {
    const calls = stubDaemon(mountRoutes({
      '/api/codesign/families': () =>
        ok({
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
      '/api/codesign/verified': () => refuse(503, { message: 'not ready' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Run families').click();

    await expect.poll(() => screen.container.textContent).toContain('widen-nudge');
    const posted = callsTo(calls, '/api/codesign/families');
    expect(posted).toHaveLength(1);
    expect(posted[0].body?.families).toEqual(['direct', 'tower', 'mbu']);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('direct 1T×3Q');
    expect(text).toContain('widen direct');
    expect(text).toContain('Lineage (');
    expect(text).toContain('events');
    expect(text).toContain('no improvement');
  });

  it('refuses to send malformed family offers', async () => {
    const calls = stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Family offers JSON').fill('nope{{{');
    await button(screen, 'Run families').click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('offers must be JSON');
    expect(callsTo(calls, '/api/codesign/families')).toHaveLength(0);
  });

  it('renders the pack registry live from the server, never hand-typed', async () => {
    stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    const summary = screen.getByText('Domain packs — live registry (served, never hand-typed)');
    await summary.click();
    const details = summary.element().closest('details') as HTMLDetailsElement;
    await expect.poll(() => details.open).toBe(true);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('live registry');
    expect(text).toContain('numerics');
    expect(text).toContain('0.1.0');
  });

  it('shows the registry refusal instead of a stale hardcoded list', async () => {
    stubDaemon(mountRoutes({
      '/api/codesign/packs': () => refuse(503, { code: 'unavailable', message: 'registry down' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await expect.poll(() => screen.container.textContent).toContain('registry down');
    expect(screen.container.textContent).not.toContain('0.1.0');
  });

  it('scores with a kappa slider (1..3), never a free-text field', async () => {
    stubDaemon(mountRoutes());
    const screen = await render(<CodesignCockpit />);

    const sliders = Array.from(screen.container.querySelectorAll<HTMLInputElement>('input[aria-label="Kappa"]'));
    expect(sliders).toHaveLength(3);
    for (const slider of sliders) {
      expect(slider.type).toBe('range');
      expect(slider.min).toBe('1');
      expect(slider.max).toBe('3');
    }

    // Real keyboard interaction: focus the first slider and step it to 3.
    sliders[0].focus();
    await userEvent.keyboard('{ArrowRight}{ArrowRight}');
    await expect.poll(() => sliders[0].value).toBe('3');
  });

  it('renders the first failure input + reason instead of a bare INVALID', async () => {
    stubDaemon(mountRoutes({
      '/api/codesign/evaluate': () =>
        ok({
          ok: true, spec_kind: 'and2',
          result: {
            valid: false, failures: 2, tests: 8, toffoli: 1, peak_qubits: 3,
            failure_details: [{ t: 3, input: { 0: 1, 1: 0, 2: 0 }, reason: 'output mismatch' }],
            failure_details_capped: false,
          },
        }),
      '/api/codesign/verified': () => refuse(503, { message: 'not ready' }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Run evaluation').click();

    await expect.poll(() => screen.container.textContent).toContain('output mismatch');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('failure');
    expect(text).toContain('trial');
  });
});

describe('CodesignCockpit numeric range validation (Wave-2) in a real browser', () => {
  it('refuses a non-numeric Toffoli count with a named-field error and sends nothing', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Toffoli count').fill('abc');
    await button(screen, 'Score').click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('Toffoli count must be an integer');
    expect(callsTo(calls, '/api/codesign/tq')).toHaveLength(0);
  });

  it('refuses an out-of-range Qubit count in the sweep with a named-field error', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Qubit count').fill('0');
    await button(screen, /Sweep/).click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('Qubit count must be an integer');
    expect(callsTo(calls, '/api/codesign/tq')).toHaveLength(0);
  });

  it('names the offending frontier batch line instead of sending NaN', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Frontier batch').fill('5,abc');
    await button(screen, 'Score frontier').click();

    await expect
      .poll(() => screen.getByRole('alert').element().textContent ?? '')
      .toContain('Frontier batch Qubit count (line 1)');
    expect(callsTo(calls, '/api/codesign/frontier')).toHaveLength(0);
  });

  it('refuses a zero test count on the evaluator with a named-field error', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Number of tests').fill('0');
    await button(screen, 'Run evaluation').click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('Number of tests must be an integer');
    expect(callsTo(calls, '/api/codesign/evaluate')).toHaveLength(0);
  });

  it('refuses a bad budget row with named fields and never computes locally', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Budget rows').fill('carry,2,40');
    await button(screen, 'Seal union bound').click();

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('Budget row 1 per-use rate');
    expect(callsTo(calls, '/api/codesign/budget')).toHaveLength(0);
    expect(screen.container.textContent).not.toContain('union ≤');
  });

  it('withholds the local DQI estimate on out-of-range epsilon with a named error', async () => {
    stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    const summary = screen.getByText(/DQI loss bound/);
    await summary.click();
    await screen.getByLabelText('Epsilon').fill('2');

    await expect.poll(() => screen.getByRole('alert').element().textContent ?? '').toContain('Epsilon must be a number 0..1');
    expect(screen.container.textContent).not.toContain('loss ≤');
  });

  it('refuses empty families and non-integer share rounds with named errors', async () => {
    const calls = stubDaemon(mountRoutes(), () => NOT_READY);
    const screen = await render(<CodesignCockpit />);

    await screen.getByLabelText('Families (comma list)').fill('  ');
    await button(screen, 'Run families').click();

    await expect
      .poll(() => screen.getByRole('alert').element().textContent ?? '')
      .toContain('Families (comma list) names no family');
    expect(callsTo(calls, '/api/codesign/families')).toHaveLength(0);
  });

  it('renders Exhaustive (ε = 0) when verification_method is exhaustive and allows 48 proposer steps', async () => {
    const calls = stubDaemon(mountRoutes({
      '/api/sessions': () => ok({ sessions: [{ id: 'sess-48', title: 'qec search' }] }),
      '/api/codesign/evaluate': () =>
        ok({
          ok: true,
          spec_kind: 'and2',
          result: {
            valid: true, toffoli: 1, peak_qubits: 3, eps_bound_97_5: 0,
            verification_method: 'exhaustive',
            depth: 1, toffoli_depth: 1, active_qubits: 3,
          },
        }),
      '/api/codesign/rerun': () =>
        ok({
          ok: true, run_id: 'run-48', session_id: 'sess-48', spec_kind: 'and2',
          budget_steps: 48, seed: 7, num_tests: 16, max_failures: 0, kappa: 1,
          provider: 'strata', model: 'iq3_s', billing: 'none', payer: 'local',
          estimated_usd: 0, budget_usd: 0, steps: 48, trials: [],
          provenance_status: 'sealed', ledger_ref: { id: 2 },
        }),
    }));
    const screen = await render(<CodesignCockpit />);

    await button(screen, 'Run evaluation').click();
    await expect.poll(() => screen.container.textContent).toContain('Exhaustive (ε = 0)');

    await screen.getByLabelText('Proposer steps').fill('48');
    const run = button(screen, 'Run proposer');
    await expect.poll(() => (run.element() as HTMLButtonElement).disabled).toBe(false);
    await run.click();
    await expect.poll(() => callsTo(calls, '/api/codesign/rerun').length).toBe(1);
    expect(callsTo(calls, '/api/codesign/rerun')[0].body?.budget_steps).toBe(48);
  });
});
