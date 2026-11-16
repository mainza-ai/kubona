import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { hexToBytes } from '@noble/hashes/utils.js';
import { HardwareLaunchPanel } from './HardwareLaunchPanel';
import { computeCanonicalPayload } from '@/lib/mandates';

// Same hash wiring lib/mandates.ts installs; the real custody signer runs in-browser.
ed.hashes.sha512 = (...m: Uint8Array[]) => sha512(ed.etc.concatBytes(...m));

type Call = { method: string; path: string; body?: Record<string, unknown> };

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

function installFetch(routes: {
  preparePath: string;
  prepare: unknown;
  launchPath: string;
  launchJob: Record<string, unknown>;
  polledJob: Record<string, unknown>;
  receipt: string;
}) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;
    calls.push({ method, path: parsed.pathname, body });
    if (method === 'POST' && parsed.pathname === routes.preparePath) return ok(routes.prepare);
    if (method === 'POST' && parsed.pathname === '/api/mandates/verify') return ok({ valid: true, receipt: routes.receipt });
    if (method === 'POST' && parsed.pathname === routes.launchPath) return ok({ job: routes.launchJob });
    if (method === 'GET' && parsed.pathname.startsWith('/api/jobs/')) return ok({ job: routes.polledJob });
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${method} ${parsed.pathname}` }),
      text: async () => `unexpected ${method} ${parsed.pathname}`,
    };
  });
  return calls;
}

function assertSignedMandate(body: Record<string, unknown>, jobHash: string) {
  expect(body.tenant_id).toBe('lab');
  expect(body.job_hash).toBe(jobHash);
  expect(body.max_budget_usd).toBe(1);
  expect(String(body.expiry_timestamp)).not.toMatch(/\.\d{3}Z$/);
  expect(String(body.public_key_hex)).toHaveLength(64);
  expect(String(body.signature_hex)).toHaveLength(128);
  const canonical = computeCanonicalPayload('lab', jobHash, 1, String(body.expiry_timestamp));
  expect(ed.verify(hexToBytes(String(body.signature_hex)), canonical, hexToBytes(String(body.public_key_hex)))).toBe(true);
}

function job(id: string, jobType: string, hardwareJobId: string) {
  return {
    id,
    session_id: 's',
    status: 'JOB_STATUS_COMPLETED',
    job_type: jobType,
    params_json: {},
    created_at: '',
    result_json: { hardware_job_id: hardwareJobId },
  };
}

const sourceJob = {
  id: 'job-spin-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain',
  params_json: {}, created_at: '',
  result_json: { kind: 'spin_chain', model: 'tfim', n_sites: 4 },
};

afterEach(() => vi.unstubAllGlobals());

describe('HardwareLaunchPanel in a real browser', () => {
  it('prepares the exact binding, signs it, and launches the governed job', async () => {
    const calls = installFetch({
      preparePath: '/api/experiments/spin-chain/hardware/prepare',
      prepare: { binding: 'binding-abc', spec: { requested_backend: 'ibm_fez', shots: 1024, max_cost_usd: 1 } },
      launchPath: '/api/experiments/spin-chain/hardware',
      launchJob: job('job-hw-new', 'spin_chain_hardware', 'vendor-1'),
      polledJob: job('job-hw-new', 'spin_chain_hardware', 'vendor-1'),
      receipt: 'receipt-1',
    });

    const screen = await render(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={sourceJob as never} />);
    await screen.getByRole('button', { name: 'Prepare binding' }).click();
    await expect.poll(() => document.body.textContent).toContain('binding-abc');

    const prepareCall = calls.find((c) => c.path === '/api/experiments/spin-chain/hardware/prepare')!;
    expect(prepareCall.body).toEqual({
      session_id: 's',
      source_job_id: 'job-spin-source',
      backend: 'ibm_fez',
      shots: 1024,
    });

    await screen.getByRole('button', { name: 'Sign & launch' }).click();
    await expect.poll(() => document.body.textContent).toContain('vendor-1');

    const verifyCall = calls.find((c) => c.path === '/api/mandates/verify')!;
    assertSignedMandate(verifyCall.body!, 'binding-abc');

    const launchCall = calls.find((c) => c.path === '/api/experiments/spin-chain/hardware')!;
    expect(launchCall.body!).toMatchObject({
      session_id: 's',
      source_job_id: 'job-spin-source',
      mandate_ref: 'receipt-1',
      backend: 'ibm_fez',
      shots: 1024,
      budget_usd: 1,
      caller_jurisdictions: ['ZM'],
      caller_export_class: 'public',
      export_reviewed: true,
    });
    expect(String(launchCall.body!.request_id)).toMatch(/^ui-spin-hw-/);

    // The status poll completes the governed launch before the test ends.
    await expect.poll(() => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/jobs/')).length, { timeout: 5000 }).toBe(1);
  }, 10_000);

  it('routes a VQE source through the VQE hardware prepare and launch rails', async () => {
    const vqeSource = {
      id: 'job-vqe-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_run',
      params_json: {}, created_at: '', result_json: { kind: 'vqe', molecule: 'H2', theta: [0.1], ansatz_qasm: 'OPENQASM 2.0; qreg q[1];' },
    };
    const calls = installFetch({
      preparePath: '/api/experiments/vqe/hardware/prepare',
      prepare: { binding: 'binding-vqe', spec: { shots: 1024, max_cost_usd: 1 } },
      launchPath: '/api/experiments/vqe/hardware',
      launchJob: job('job-vqe-hw', 'hardware_experiment', 'vendor-vqe'),
      polledJob: job('job-vqe-hw', 'hardware_experiment', 'vendor-vqe'),
      receipt: 'receipt-vqe',
    });

    const screen = await render(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={vqeSource as never} />);
    await screen.getByRole('button', { name: 'Prepare binding' }).click();
    await expect.poll(() => document.body.textContent).toContain('binding-vqe');

    const prepareCall = calls.find((c) => c.path === '/api/experiments/vqe/hardware/prepare')!;
    expect(prepareCall.body).toEqual({
      session_id: 's',
      source_job_id: 'job-vqe-source',
      backend: 'ibm_fez',
      shots: 1024,
      loop: false,
      max_iterations: 8,
    });

    await screen.getByRole('button', { name: 'Sign & launch' }).click();
    await expect.poll(() => document.body.textContent).toContain('vendor-vqe');

    const launchCall = calls.find((c) => c.path === '/api/experiments/vqe/hardware')!;
    expect(launchCall.body!).toMatchObject({
      session_id: 's',
      source_job_id: 'job-vqe-source',
      mandate_ref: 'receipt-vqe',
      backend: 'ibm_fez',
      shots: 1024,
      budget_usd: 1,
      loop: false,
      max_iterations: 8,
      caller_jurisdictions: ['ZM'],
      caller_export_class: 'public',
      export_reviewed: true,
    });
    expect(String(launchCall.body!.request_id)).toMatch(/^ui-vqe-hw-/);

    await expect.poll(() => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/jobs/')).length, { timeout: 5000 }).toBe(1);
  }, 10_000);

  it('prepares and launches a hardware PES with pricing evidence', async () => {
    const pesSource = {
      id: 'job-pes-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_pes',
      params_json: {}, created_at: '',
      result_json: { kind: 'pes', molecule: 'H2', points: [{ bond_distance: 0.7 }, { bond_distance: 0.8 }] },
    };
    const calls = installFetch({
      preparePath: '/api/experiments/pes/hardware/prepare',
      prepare: {
        binding: 'binding-pes',
        spec: { requested_backend: 'ibm_fez', shots: 1024, max_cost_usd: 1 },
        pricing: { available: true, plan: 'open', pricing_type: 'free', estimated_usd: 0 },
      },
      launchPath: '/api/experiments/pes/hardware',
      launchJob: job('job-pes-hw', 'hardware_pes', 'vendor-pes'),
      polledJob: job('job-pes-hw', 'hardware_pes', 'vendor-pes'),
      receipt: 'receipt-pes',
    });

    const screen = await render(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={pesSource as never} />);
    await screen.getByRole('button', { name: 'Prepare binding' }).click();
    await expect.poll(() => document.body.textContent).toContain('estimated $0');

    const prepareCall = calls.find((c) => c.path === '/api/experiments/pes/hardware/prepare')!;
    expect(prepareCall.body).toEqual({
      session_id: 's',
      source_job_id: 'job-pes-source',
      backend: 'ibm_fez',
      shots: 1024,
    });

    await screen.getByRole('button', { name: 'Sign & launch' }).click();
    await expect.poll(() => document.body.textContent).toContain('vendor-pes');

    const verifyCall = calls.find((c) => c.path === '/api/mandates/verify')!;
    assertSignedMandate(verifyCall.body!, 'binding-pes');

    const launchCall = calls.find((c) => c.path === '/api/experiments/pes/hardware')!;
    expect(launchCall.body!).toMatchObject({
      session_id: 's',
      source_job_id: 'job-pes-source',
      mandate_ref: 'receipt-pes',
      backend: 'ibm_fez',
      shots: 1024,
      budget_usd: 1,
      caller_jurisdictions: ['ZM'],
      caller_export_class: 'public',
      export_reviewed: true,
    });
    expect(String(launchCall.body!.request_id)).toMatch(/^ui-pes-hw-/);

    await expect.poll(() => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/jobs/')).length, { timeout: 5000 }).toBe(1);
  }, 10_000);
});
