import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { QuantumSettings } from './QuantumSettings';

const BACKENDS = [
  {
    provider: 'local-simulator',
    display_name: 'Local simulator',
    modality: 'simulator',
    device: 'local',
    credential_env: '',
    credential_state: 'ready',
    telemetry: 'measured',
    executable_now: true,
  },
  {
    provider: 'ibm-qiskit',
    display_name: 'IBM Quantum',
    modality: 'superconducting',
    device: 'ibm_heron',
    credential_env: 'KUBONA_IBM_QISKIT_TOKEN',
    credential_state: 'credentialless',
    telemetry: 'declared',
    executable_now: false,
  },
  {
    provider: 'neutral-atom-quera-pasqal',
    display_name: 'Neutral atom (not wired)',
    modality: 'neutral_atom',
    device: 'unwired',
    credential_env: '',
    credential_state: 'unavailable',
    telemetry: 'declared',
    executable_now: false,
  },
];

type QuantumStub = {
  settings?: Record<string, unknown>;
  /** Body answered by PUT /api/quantum/settings. */
  save?: { ok: boolean; body: unknown };
  canary?: unknown;
};

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function refusal(status: number, message: string) {
  const body = JSON.stringify({ error: message });
  return { ok: false, status, text: async () => body, json: async () => ({ error: message }) };
}

// The surface reads the real transport wrappers through the shared
// quantum-settings projection, so every route is answered at fetch level.
function installFetch(stub: QuantumStub = {}) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  const settings = stub.settings ?? {
    default_backend: 'local-simulator',
    harness_backends: {},
  };
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ method, path, body: init?.body });
    switch (path) {
      case '/api/quantum/backends':
        return json({ backends: BACKENDS });
      case '/api/quantum/settings':
        if (method === 'PUT') {
          const save = stub.save ?? { ok: true, body: { status: 'saved', settings } };
          return save.ok
            ? json(save.body)
            : refusal(400, String((save.body as { error?: string }).error ?? 'refused'));
        }
        return json(settings);
      case '/api/quantum/canary':
        return json(stub.canary ?? { ok: true, results: { 'local-simulator': {} }, cached: false });
      default:
        throw new Error(`unexpected fetch ${method} ${path}`);
    }
  });
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('QuantumSettings provider selection in a real browser', () => {
  it('lists providers with credential and catalog honesty, never a key field', async () => {
    installFetch();

    const screen = await render(<QuantumSettings />);
    await expect.element(screen.getByText('IBM Quantum')).toBeVisible();

    const text = screen.container.textContent ?? '';
    expect(text).toContain('NO CREDENTIALS');
    expect(text).toContain('CATALOG');
    // Secrets are never accepted here: no key/password input exists, and the
    // footer states the rule.
    expect(screen.container.querySelectorAll('input[type="password"]')).toHaveLength(0);
    expect(text).toContain('never accepted here');
  });

  it('saves the selected default through the validated route', async () => {
    const calls = installFetch();

    const screen = await render(<QuantumSettings />);
    const ibm = screen.getByRole('radio', { name: /IBM Quantum/ });
    await expect.element(ibm).toBeVisible();
    // Credentialless-by-design is allowed: plan before the token arrives.
    await ibm.click();
    await expect.element(ibm).toBeChecked();

    const save = screen.getByRole('button', { name: /^save selection$/i });
    await expect.element(save).toBeEnabled();
    await save.click();

    await expect.poll(() => calls.filter((c) => c.method === 'PUT').length).toBe(1);
    const put = calls.find((c) => c.method === 'PUT')!;
    expect(put.path).toBe('/api/quantum/settings');
    expect(JSON.parse(String(put.body))).toEqual({
      default_backend: 'ibm-qiskit',
      harness_backends: {},
    });

    await expect.poll(() => screen.container.textContent).toContain('saved');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Quantum provider selection saved.');
    expect(text).toContain('applied · quantum provider selection');
  });

  it('surfaces backend refusals verbatim', async () => {
    installFetch({ save: { ok: false, body: { error: 'unknown default_backend "nope"' } } });

    const screen = await render(<QuantumSettings />);
    const ibm = screen.getByRole('radio', { name: /IBM Quantum/ });
    await expect.element(ibm).toBeVisible();
    // Make the form dirty (select IBM), then the save refuses verbatim.
    await ibm.click();
    await screen.getByRole('button', { name: /^save selection$/i }).click();

    await expect.poll(() => screen.container.textContent).toContain('unknown default_backend');
    expect(screen.container.textContent).toContain('refused · unknown default_backend');
  });

  it('runs the canary and reports honestly', async () => {
    const calls = installFetch();

    const screen = await render(<QuantumSettings />);
    const canary = screen.getByLabelText('Run live canary check');
    await expect.element(canary).toBeVisible();
    await canary.click();

    await expect.poll(() => screen.container.textContent).toContain('canary ran');
    expect(calls.filter((c) => c.path === '/api/quantum/canary')).toHaveLength(1);
    // The measured results are disclosed, not just a "ran" claim.
    const details = screen.container.querySelector('[data-testid="canary-results"]');
    expect(details).not.toBeNull();
    expect(details!.textContent).toContain('local-simulator');
  });
});

describe('QuantumSettings labels + icon-button names (Wave-2) in a real browser', () => {
  it('names the icon-only reload/canary buttons and keeps their tooltips', async () => {
    const calls = installFetch({
      settings: { default_backend: 'local-simulator', harness_backends: { 'vqe-ansatz': 'local-simulator' } },
    });

    const screen = await render(<QuantumSettings />);
    const reload = screen.getByLabelText('Reload backends and saved selection');
    await expect.element(reload).toBeVisible();
    expect(reload.element().getAttribute('title')).toBe('Reload backends and saved selection');

    const canary = screen.getByLabelText('Run live canary check');
    expect(canary.element().getAttribute('title')).toContain('canary');

    // The named control is wired to a real refetch, not a decorative icon.
    const before = calls.filter((c) => c.path === '/api/quantum/backends').length;
    await reload.click();
    await expect
      .poll(() => calls.filter((c) => c.path === '/api/quantum/backends').length)
      .toBeGreaterThan(before);
  });

  it('associates harness labels with their inputs', async () => {
    installFetch({
      settings: { default_backend: 'local-simulator', harness_backends: { 'vqe-ansatz': 'local-simulator' } },
    });

    const screen = await render(<QuantumSettings />);

    // Per-row override select is named for its harness…
    const rowSelect = screen.getByLabelText('Backend for harness vqe-ansatz');
    await expect.element(rowSelect).toBeVisible();
    expect(rowSelect.element().tagName).toBe('SELECT');
    expect((rowSelect.element() as HTMLSelectElement).value).toBe('local-simulator');
    // …and the remove button is named for its harness too.
    const remove = screen.getByLabelText('Remove harness vqe-ansatz');
    expect(remove.element().tagName).toBe('BUTTON');
    // New-harness controls are labeled.
    const newName = screen.getByLabelText('New harness name');
    expect(newName.element().tagName).toBe('INPUT');
    const newBackend = screen.getByLabelText('Backend for new harness');
    expect(newBackend.element().tagName).toBe('SELECT');

    // Every label element is associated with a control: explicit htmlFor that
    // resolves, or an implicitly wrapped input/select.
    const labels = Array.from(screen.container.querySelectorAll('label'));
    expect(labels.length).toBeGreaterThan(0);
    for (const label of labels) {
      const htmlFor = label.getAttribute('for');
      if (htmlFor) {
        expect(screen.container.querySelector(`#${CSS.escape(htmlFor)}`)).not.toBeNull();
      } else {
        expect(label.querySelector('input, select')).not.toBeNull();
      }
    }
  });
});
