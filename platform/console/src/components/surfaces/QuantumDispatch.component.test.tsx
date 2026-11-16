import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { QuantumDispatch } from './QuantumDispatch';

const BACKENDS = [
  { provider: 'local-simulator', display_name: 'Local simulator', modality: 'simulator', device: 'local', credential_env: '', credential_state: 'ready', telemetry: 'measured', executable_now: true, dispatch_ready: true, input_format: 'qasm' },
  { provider: 'dwave-annealing', display_name: 'D-Wave annealing (local exact/SA)', modality: 'annealing', device: 'local/SA', credential_env: 'KUBONA_DWAVE_ANNEALING_LEAP_TOKEN', credential_state: 'ready', telemetry: 'declared', executable_now: true, dispatch_ready: true, input_format: 'qubo' },
  { provider: 'ibm-qiskit', display_name: 'IBM Quantum', modality: 'superconducting', device: 'ibm_heron', credential_env: 'KUBONA_IBM_QISKIT_TOKEN', credential_state: 'credentialless', telemetry: 'declared', executable_now: false, dispatch_ready: false, input_format: 'qasm' },
];

const DEFAULT_QUBO = `[
  [-1.0, 2.0],
  [0.0, -1.0]
]`;

type Call = { method: string; path: string; body: Record<string, unknown> | undefined };

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function refusal(status: number, message: string) {
  const body = JSON.stringify({ error: message });
  return { ok: false, status, text: async () => body, json: async () => ({ error: message }) };
}

type DispatchStub = {
  /** Answer for the Nth POST /api/quantum/jobs (estimate then dispatch). */
  submit: (body: Record<string, unknown> | undefined, n: number) => unknown;
  /** Answer for GET /api/quantum/jobs/{id}; a throw is a transport loss. */
  status?: () => unknown;
  /** Answer for POST /api/quantum/jobs/{id}/cancel. */
  cancel?: () => unknown;
  /** Answer for POST /api/mandates/verify. */
  mandate?: () => unknown;
};

// The surface reads the real transport wrappers through the shared quantum
// projection, so every route is answered at fetch level.
function installFetch(stub: DispatchStub) {
  const calls: Call[] = [];
  const submits: Call[] = [];
  let statusCalls = 0;
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body === undefined ? undefined : JSON.parse(String(init.body)) as Record<string, unknown>;
    calls.push({ method, path, body });
    switch (true) {
      case path === '/api/quantum/backends':
        return json({ backends: BACKENDS });
      case path === '/api/quantum/settings':
        return json({ default_backend: 'local-simulator', harness_backends: {} });
      case path === '/api/quantum/jobs' && method === 'POST': {
        submits.push({ method, path, body });
        return stub.submit(body, submits.length - 1);
      }
      case path.endsWith('/cancel'):
        return stub.cancel
          ? stub.cancel()
          : json({ ok: true, job_id: 'j1', status: 'cancelled', provider_recall_attempted: false, provider_recall_status: '' });
      case path.startsWith('/api/quantum/jobs/'):
        statusCalls += 1;
        return stub.status ? stub.status() : json({ ok: true, job_id: 'j1', status: 'completed' });
      case path === '/api/ledger/export':
        return json({ ok: true, session_id: 's1', events: [], markdown: '' });
      case path === '/api/mandates/verify':
        return stub.mandate ? stub.mandate() : json({ valid: true });
      default:
        throw new Error(`unexpected fetch ${method} ${path}`);
    }
  });
  // `stub` is returned so a test can flip a route mid-flight (transport loss).
  return { calls, submits, stub, statusCalls: () => statusCalls };
}

function estimateBtn(screen: Awaited<ReturnType<typeof render>>) {
  return screen.getByRole('button', { name: /Estimate \(dry run\)/ });
}

function dispatchBtn(screen: Awaited<ReturnType<typeof render>>) {
  // Wave-2 money copy: the Dispatch button restates the already-computed
  // estimate at 2dp with the provider, so match the stable prefix only.
  return screen.getByRole('button', { name: /^Dispatch/ });
}

function disabled(locator: { element(): Element }): boolean {
  return (locator.element() as HTMLButtonElement).disabled;
}

/** Backends arrive asynchronously; the estimate button stays disabled until they do. */
async function renderDispatch(props: React.ComponentProps<typeof QuantumDispatch>) {
  const screen = await render(<QuantumDispatch {...props} />);
  await expect.poll(() => disabled(estimateBtn(screen)), { timeout: 3000 }).toBe(false);
  return screen;
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('QuantumDispatch human flow in a real browser', () => {
  it('estimates without spending, then dispatches with a fresh idempotency key', async () => {
    const stub = installFetch({
      submit: (_body, n) => n === 0
        ? json({ ok: true, estimate: { estimated_usd: 0 }, fidelity: { admitted: true, skipped: 'simulator-class backend' } })
        : json({ ok: true, job_id: 'j1', queued: false, session_id: 's1' }),
    });

    const screen = await renderDispatch({ sessionId: 's1' });

    // Estimate (dry run).
    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Estimated cost:');
    expect(screen.container.textContent).toContain('0.000000');
    expect(stub.submits).toHaveLength(1);
    expect(stub.submits[0].body).toMatchObject({ dry_run: true, provider: 'local-simulator' });

    // Dispatch (zero estimate needs no mandate).
    const go = dispatchBtn(screen);
    expect(go.element().textContent).toContain('Dispatch — $0.00 on local-simulator');
    expect(disabled(go)).toBe(false);
    await go.click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('j1');

    expect(stub.submits).toHaveLength(2);
    const dispatch = stub.submits[1].body!;
    expect(dispatch.dry_run).toBe(false);
    expect('job_id' in dispatch).toBe(false);
    expect(typeof dispatch.idempotency_key).toBe('string');
    expect(String(dispatch.idempotency_key).length).toBeGreaterThan(8);
    // The status poll then resolved the job through the real route.
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('status: completed');
  });

  it('gates non-zero dispatch behind the mandate card', async () => {
    const stub = installFetch({
      submit: (_body, n) => n === 0
        ? json({ ok: true, estimate: { estimated_usd: 1.6 }, fidelity: { admitted: true } })
        : json({ ok: true, job_id: 'j2', queued: true, session_id: 's1' }),
      status: () => json({ ok: true, job_id: 'j2', status: 'completed' }),
    });

    const screen = await renderDispatch({ sessionId: 's1', tenantId: 'lab-test' });
    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('AP2 High-Friction Spend Mandate Card');

    // ...and dispatch stays disabled until signing.
    const go = dispatchBtn(screen);
    expect(go.element().textContent).toContain('Dispatch — $1.60 on local-simulator');
    expect(disabled(go)).toBe(true);
    expect(screen.container.textContent).toContain('sign the mandate card above first');

    // Sign through the real Ed25519 custody path + server verification stub.
    await screen.getByRole('button', { name: /Authorize via Ed25519 Card/ }).click();
    // A signed mandate replaces the card with the bound dispatch (the card
    // itself only ever renders the pre-authorization proposal).
    await expect.poll(() => disabled(go), { timeout: 5000 }).toBe(false);
    expect(screen.container.textContent).not.toContain('AP2 High-Friction Spend Mandate Card');

    await go.click();
    await expect.poll(() => stub.submits.length, { timeout: 3000 }).toBe(2);
    const dispatch = stub.submits[1].body!;
    expect(dispatch.dry_run).toBe(false);
    expect(typeof dispatch.mandate_ref).toBe('string');
    // The dispatch carries the real signed mandate, not an opaque placeholder.
    const mandate = JSON.parse(String(dispatch.mandate_ref)) as Record<string, unknown>;
    expect(mandate.tenant_id).toBe('lab-test');
    expect(mandate.max_budget_usd).toBe(1.6);
    expect(typeof mandate.job_hash).toBe('string');
    expect(String(mandate.signature_hex).length).toBeGreaterThan(16);
    // The verifier was actually consulted before dispatch was unlocked.
    expect(stub.calls.filter((c) => c.path === '/api/mandates/verify')).toHaveLength(1);
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('j2');
  });

  it('renders refusals verbatim', async () => {
    installFetch({ submit: () => refusal(400, 'unknown provider "nope"') });

    const screen = await renderDispatch({ sessionId: 's1' });
    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('unknown provider "nope"');
    expect(screen.container.textContent).toContain('Estimate refused:');
  });

  it('cancels with the provider-recall disclosure', async () => {
    installFetch({
      submit: (_body, n) => n === 0
        ? json({ ok: true, estimate: { estimated_usd: 0 }, fidelity: { admitted: true } })
        : json({ ok: true, job_id: 'j5', queued: true, session_id: 's1' }),
      status: () => json({ ok: true, job_id: 'j5', status: 'queued' }),
      cancel: () => json({
        ok: true,
        job_id: 'j5',
        status: 'cancelled',
        provider_recall_attempted: true,
        provider_recall_status: 'recall_pending',
      }),
    });

    const screen = await renderDispatch({ sessionId: 's1' });
    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Estimated cost:');
    await dispatchBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('j5');

    await screen.getByRole('button', { name: 'Cancel job' }).click();
    // The cancel never claims the vendor stopped: it discloses the recall.
    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('Fenced + vendor recall attempted (recall_pending)');
    expect(screen.container.textContent).toContain('running work may complete underneath');
  });

  it('pauses polling after repeated transport loss and resumes on demand', async () => {
    const stub = installFetch({
      submit: (_body, n) => n === 0
        ? json({ ok: true, estimate: { estimated_usd: 0 }, fidelity: { admitted: true, skipped: 'simulator-class backend' } })
        : json({ ok: true, job_id: 'j9', queued: true, session_id: 's1' }),
      status: () => json({ ok: true, job_id: 'j9', status: 'completed' }),
    });

    const screen = await renderDispatch({ sessionId: 's1' });
    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Estimated cost:');
    await dispatchBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('j9');

    // From here the backend goes dark: every status fetch is a transport
    // failure. Fake timers drive the chained poll timeouts.
    vi.useFakeTimers();
    stub.stub.status = () => { throw new TypeError('network down'); };

    const callsBefore = stub.statusCalls();
    await screen.getByRole('button', { name: 'Refresh' }).click();
    // Backoff chain: 7.5s → 11.25s → 15s → 15s; five consecutive failures
    // exhaust the transport budget.
    for (let i = 0; i < 6; i++) {
      await vi.advanceTimersByTimeAsync(16000);
    }
    await vi.advanceTimersByTimeAsync(0);
    expect(stub.statusCalls()).toBeGreaterThan(callsBefore);
    expect(screen.container.textContent).toContain('polling paused');
    expect(screen.container.textContent).toContain('Resume');

    // Paused: no further transport attempts while paused.
    const callsAtPause = stub.statusCalls();
    await vi.advanceTimersByTimeAsync(60000);
    expect(stub.statusCalls()).toBe(callsAtPause);

    // Resume restarts the poll chain (one immediate attempt).
    await screen.getByRole('button', { name: 'Resume' }).click();
    await vi.advanceTimersByTimeAsync(0);
    expect(stub.statusCalls()).toBeGreaterThan(callsAtPause);
    expect(screen.container.textContent).not.toContain('polling paused');
  });

  it('switches input mode to QUBO matrix when dwave-annealing is selected', async () => {
    installFetch({ submit: () => json({ ok: true, estimate: { estimated_usd: 0 } }) });

    const screen = await renderDispatch({ sessionId: 's1' });
    expect(screen.container.textContent).toContain('OpenQASM circuit');

    await screen.getByRole('combobox', { name: /Backend/ }).selectOptions('dwave-annealing');

    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('QUBO matrix (JSON 2-D array)');
    expect(screen.container.textContent).toContain('Samples / Reads');
    const program = screen.getByRole('textbox', { name: 'QUBO matrix (JSON 2-D array)' }).element() as HTMLTextAreaElement;
    expect(program.value.trim()).toBe(DEFAULT_QUBO.trim());
  });

  it('dispatch button restates the priced estimate at 2dp with provider (quote line stays 6dp)', async () => {
    installFetch({
      submit: () => json({ ok: true, estimate: { estimated_usd: 1.6 }, fidelity: { admitted: true } }),
    });

    const screen = await renderDispatch({ sessionId: 's1' });
    // Unpriced: generic label, no invented figure.
    expect(dispatchBtn(screen).element().textContent).toContain('Dispatch job');
    expect(dispatchBtn(screen).element().textContent).not.toContain('$');

    await estimateBtn(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Estimated cost:');
    // Priced from the ALREADY-COMPUTED estimate: amount 2dp + provider.
    expect(dispatchBtn(screen).element().textContent).toContain('Dispatch — $1.60 on local-simulator');
    // Documented exception: the dry-run quote line itself stays 6dp.
    expect(screen.container.textContent).toContain('1.600000');

    // Stale quote drops back to the generic label. The provider select cannot
    // be used for this: non-executable backends render as disabled options
    // (catalog honesty), so the program is edited instead — the quote
    // fingerprint covers provider, program, shots and session alike.
    expect(screen.getByRole('option', { name: /IBM Quantum/ }).element()).toHaveProperty('disabled', true);
    await screen.getByRole('textbox', { name: 'OpenQASM circuit' }).fill('OPENQASM 2.0;\n// edited after the estimate');
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Estimate is stale');
    expect(dispatchBtn(screen).element().textContent).toContain('Dispatch job');
    expect(dispatchBtn(screen).element().textContent).not.toContain('$');
    expect(disabled(dispatchBtn(screen))).toBe(true);
  });
});
