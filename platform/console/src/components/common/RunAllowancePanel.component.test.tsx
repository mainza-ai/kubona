import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { hexToBytes } from '@noble/hashes/utils.js';
import { RunAllowancePanel } from './RunAllowancePanel';
import { canonicalAllowanceScope } from '@/lib/runAllowance';

// Same hash wiring lib/custody.ts installs; the real signer runs in-browser.
ed.hashes.sha512 = (...m: Uint8Array[]) => sha512(ed.etc.concatBytes(...m));

const allowance = {
  id: 'ra-12345678',
  tenant_id: 'lab-a',
  session_id: 'sess-1',
  backend: 'ibm_fez',
  job_types: ['circuit_run'],
  max_per_job_usd: 1,
  max_total_usd: 3,
  remaining_usd: 3,
  max_jobs: 3,
  jobs_used: 0,
  shots_cap: 1024,
  status: 'active' as const,
  issued_at: '2026-11-14T20:00:00Z',
  expires_at: '2026-11-15T04:00:00Z',
  signer_pubkey_hex: 'ab',
  scope_digest: 'sha256:x',
};

type Call = { method: string; path: string; query: string; body?: Record<string, unknown> };

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

function installFetch(list: unknown[]) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;
    calls.push({ method, path: parsed.pathname, query: parsed.search, body });
    if (method === 'GET' && parsed.pathname === '/api/run-allowances') {
      return ok({ ok: true, allowances: list, count: list.length });
    }
    if (method === 'POST' && parsed.pathname === '/api/run-allowances') {
      return ok({ ok: true, allowance });
    }
    if (method === 'POST' && parsed.pathname === `/api/run-allowances/${allowance.id}/revoke`) {
      return ok({ ok: true, id: allowance.id, status: 'revoked' });
    }
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${method} ${parsed.pathname}` }),
      text: async () => `unexpected ${method} ${parsed.pathname}`,
    };
  });
  return calls;
}

beforeEach(() => {
  window.localStorage.clear();
});
afterEach(() => vi.unstubAllGlobals());

describe('RunAllowancePanel in a real browser (C11)', () => {
  it('signs the canonical scope and mints a bounded allowance', async () => {
    const onSelect = vi.fn();
    const calls = installFetch([]);
    const screen = await render(
      <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="ibm_fez" perJobEstimateUsd={1} onSelect={onSelect} />,
    );

    await expect.poll(() => calls.filter((c) => c.method === 'GET').length).toBeGreaterThan(0);
    const listed = calls.find((c) => c.method === 'GET')!;
    expect(listed.path).toBe('/api/run-allowances');
    expect(new URLSearchParams(listed.query).get('session_id')).toBe('sess-1');

    await screen.getByTestId('run-allowance-mint').click();
    await expect.element(screen.getByTestId('run-allowance-receipt')).toMatchTextContent(/Allowance active/);

    const minted = calls.find((c) => c.method === 'POST' && c.path === '/api/run-allowances')!.body!;
    expect(minted.backend).toBe('ibm_fez');
    expect(minted.job_types).toEqual(['circuit_run']);
    expect(minted.max_per_job_usd).toBe(1);
    expect(minted.max_total_usd).toBe(3);
    expect(minted.max_jobs).toBe(3);
    expect(minted.shots_cap).toBe(1024);
    expect(String(minted.expires_at)).not.toMatch(/\.\d{3}Z$/);

    // Rebuild the exact bytes the operator signed and verify the Ed25519
    // signature with the signer's public key (real custody, not a stub).
    const canonical = canonicalAllowanceScope({
      tenantId: 'lab-a',
      sessionId: 'sess-1',
      campaignId: 'sess-1',
      backend: String(minted.backend),
      jobTypes: minted.job_types as string[],
      maxPerJobUsd: Number(minted.max_per_job_usd),
      maxTotalUsd: Number(minted.max_total_usd),
      maxJobs: Number(minted.max_jobs),
      shotsCap: Number(minted.shots_cap),
      expiresAt: String(minted.expires_at),
      nonce: String(minted.nonce),
    });
    expect(canonical).toContain('"max_per_job_micro_usd":1000000');
    expect(canonical).toContain('"max_total_micro_usd":3000000');
    expect(canonical).toContain('"backend":"ibm_fez"');
    expect(canonical).toContain('"job_types":["circuit_run"]');
    expect(canonical).not.toMatch(/\d{3}Z"/); // expiry has no milliseconds

    expect(String(minted.signer_pubkey_hex)).toHaveLength(64);
    expect(String(minted.signature_hex)).toHaveLength(128);
    expect(
      ed.verify(
        hexToBytes(String(minted.signature_hex)),
        new TextEncoder().encode(canonical),
        hexToBytes(String(minted.signer_pubkey_hex)),
      ),
    ).toBe(true);

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: allowance.id }));
  });

  it('lists active allowances for selection and revocation', async () => {
    const onSelect = vi.fn();
    const calls = installFetch([allowance]);
    const screen = await render(
      <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="ibm_fez" perJobEstimateUsd={1} onSelect={onSelect} />,
    );

    const radio = screen.getByRole('radio', { name: 'Use allowance ra-12345' });
    await expect.element(radio).toBeVisible();
    await radio.click();
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: allowance.id }));

    await screen.getByRole('button', { name: 'Revoke allowance ra-12345' }).click();
    await expect
      .poll(() => calls.filter((c) => c.method === 'POST' && c.path.endsWith('/revoke')).length)
      .toBeGreaterThan(0);

    const revoked = calls.find((c) => c.method === 'POST' && c.path.endsWith('/revoke'))!;
    expect(revoked.path).toBe(`/api/run-allowances/${allowance.id}/revoke`);
    expect(revoked.body).toMatchObject({ session_id: 'sess-1', reason: 'revoked from the launch card' });
  });

  it('refuses to offer an allowance for a simulator backend', async () => {
    installFetch([]);
    const screen = await render(
      <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="local_simulator" perJobEstimateUsd={0} onSelect={() => {}} />,
    );

    await expect.element(screen.getByTestId('run-allowance-ineligible')).toBeVisible();
    expect(screen.container.querySelector('[data-testid="run-allowance"]')).toBeNull();
  });
});
