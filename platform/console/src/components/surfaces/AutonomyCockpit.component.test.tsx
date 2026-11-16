import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { AutonomyCockpit } from './AutonomyCockpit';
import type { CampaignState, Session } from '@/types';

// The cockpit derives its Sentinel badge from live predicates only, mints a
// real Ed25519 mandate through browser custody, and gates the air-gap and
// emergency abort behind typed confirmations. These tests drive the real DOM
// against a transport-level daemon stub — no module mocking, so the signing,
// minting, and launch rails are the production ones.

type Call = { url: string; method: string; body: string | undefined };
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

/**
 * Stub the daemon by exact pathname (query ignored) and record every call.
 * A route may throw to simulate an unreachable transport.
 */
function stubDaemon(routes: Record<string, Route>, fallback: Route = () => refuse(404, { message: 'not found' })) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown, init?: RequestInit) => {
      const url = String(input);
      const path = new URL(url, 'http://localhost:3000').pathname;
      const call = { url, method: (init?.method ?? 'GET').toUpperCase(), body: init?.body as string | undefined };
      calls.push(call);
      const route = routes[path];
      if (route) return route(call);
      return fallback(call);
    }),
  );
  return calls;
}

const SESSION: Session = {
  id: 'sess-1',
  title: 't',
  tenant_id: 'lab-alpha',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  status: 'active',
  spend_limit_usd: 150,
  current_spend_usd: 0,
};

function campaign(): CampaignState {
  return {
    session_id: 'sess-1',
    target_system: 'H2',
    quantum_factors: {
      estimated_spend_usd: 12.5,
      qasm: 'OPENQASM 2.0;\nqreg q[1];\n',
      recommended_backend: 'ibm_brisbane',
    },
  } as unknown as CampaignState;
}

interface Scenario {
  airGapped: boolean | null;
  reachable: boolean;
  ready: boolean;
  readyMessage?: string;
}

/** The baseline daemon: live settings + readiness, plus a healthy proposal rail. */
function scenarioRoutes(s: Scenario): Record<string, Route> {
  return {
    '/api/llm/settings': (call) => {
      if (call.method === 'PATCH') return ok({ status: 'ok', changed: true, settings: { air_gapped: false } });
      if (s.airGapped === null) throw new Error('settings unreachable');
      return ok({ air_gapped: s.airGapped });
    },
    '/api/health': () => {
      if (!s.reachable) throw new Error('connection refused');
      return ok({ service: 'kubonad-test', version: '0.0.0' });
    },
    '/api/ready': () => {
      if (!s.reachable) throw new Error('connection refused');
      if (!s.ready) return refuse(503, { message: s.readyMessage || 'not ready' });
      return ok({ ready: true });
    },
    '/api/metasearch/proposals': () => ok({ proposals: [] }),
    // B1: the surface mints a server-owned plan before launching.
    '/api/proposals': () =>
      ok({ ok: true, proposal: { id: 'pr_auto', digest: 'sha256:auto', body: { backend: 'ibm_brisbane' } } }, 201),
    '/api/chat/launch': () => ok({ job: { id: 'job-auto-1' }, loop: { id: 'loop-auto-1' } }),
    '/api/mandates/verify': () => ok({ valid: true }),
  };
}

async function renderCockpit(session: Session | null = SESSION) {
  return render(<AutonomyCockpit campaign={campaign()} session={session} />);
}

const text = (screen: Awaited<ReturnType<typeof render>>) => screen.container.textContent ?? '';

const badgeText = (screen: Awaited<ReturnType<typeof render>>) =>
  screen.getByTestId('sentinel-badge').element().textContent ?? '';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('AutonomyCockpit Sentinel badge (live predicate) in a real browser', () => {
  it('claims Protection Active only when backend is ready AND air-gap is engaged', async () => {
    stubDaemon(scenarioRoutes({ airGapped: true, reachable: true, ready: true }));
    const screen = await renderCockpit();

    await expect.poll(() => badgeText(screen)).toContain('Sentinel Protection Active');
    const badge = badgeText(screen);
    expect(badge).not.toContain('degraded');
    expect(badge).not.toContain('controls enabled');
  });

  it('renders degraded (never Active) when the air-gap is released', async () => {
    stubDaemon(scenarioRoutes({ airGapped: false, reachable: true, ready: true }));
    const screen = await renderCockpit();

    await expect.poll(() => badgeText(screen)).toContain('Sentinel degraded');
    const badge = badgeText(screen);
    expect(badge).toContain('air-gap released');
    expect(badge).not.toContain('Protection Active');
  });

  it('renders degraded with the backend reason when readiness 503s', async () => {
    stubDaemon(scenarioRoutes({ airGapped: true, reachable: true, ready: false, readyMessage: 'disk blocked' }));
    const screen = await renderCockpit();

    await expect.poll(() => badgeText(screen)).toContain('Sentinel degraded');
    const badge = badgeText(screen);
    expect(badge).toContain('disk blocked');
    expect(badge).not.toContain('Protection Active');
  });

  it('demotes to neutral controls copy when predicates are unreachable', async () => {
    stubDaemon(scenarioRoutes({ airGapped: null, reachable: false, ready: false }));
    const screen = await renderCockpit();

    await expect.poll(() => badgeText(screen)).toContain('Sentinel controls enabled');
    expect(badgeText(screen)).not.toContain('Protection Active');
    // No static assurance chrome remains anywhere on the surface.
    expect(text(screen)).not.toContain('Envelope Verified');
  });
});

describe('AutonomyCockpit envelope line (per-dispatch receipt) in a real browser', () => {
  async function openAttestation(screen: Awaited<ReturnType<typeof render>>) {
    const inspect = screen.getByRole('button', { name: /Inspect Cryptographic/ });
    await inspect.click();
    await expect.poll(() => text(screen)).toContain('Tenant: ');
  }

  it('states no envelope verified yet (never a static verified claim)', async () => {
    stubDaemon(scenarioRoutes({ airGapped: true, reachable: true, ready: true }));
    const tenantless: Session = { ...SESSION, tenant_id: undefined };
    const screen = await renderCockpit(tenantless);
    await openAttestation(screen);

    const body = text(screen);
    expect(body).toContain('No envelope verified yet');
    expect(body).toContain('unbound');
    expect(body).not.toContain('Envelope Verified Statelessly');
  });

  it('derives the envelope line from the verified dispatch receipt', async () => {
    stubDaemon(scenarioRoutes({ airGapped: true, reachable: true, ready: true }));
    const screen = await renderCockpit();

    // Real approval: browser-custody Ed25519 signing, server verification, a
    // server-owned plan, then a launch — never a mocked authorizeSpend.
    await screen.getByRole('button', { name: 'Approve once' }).click();
    await expect.poll(() => text(screen)).toContain('Dispatched job job-auto');

    await openAttestation(screen);
    const body = text(screen);
    expect(body).toContain('Envelope verified');
    // A real signature preview (16 hex chars), not a fixture constant.
    expect(body).toMatch(/\(sig [0-9a-f]{16}…\) for tenant lab-alpha/);
    expect(body).not.toContain('Envelope Verified Statelessly');
  });
});

describe('AutonomyCockpit Wave-2 money copy + toggle staging in a real browser', () => {
  const airGapToggle = (screen: Awaited<ReturnType<typeof render>>) =>
    screen.container.querySelector('button[title^="Persists to intelligence"]') as HTMLButtonElement;

  const releaseButton = (screen: Awaited<ReturnType<typeof render>>) =>
    screen.getByRole('button', { name: 'Release air-gap' });

  it('Simulate Offline states the in-scope $0.00 amount (2dp)', async () => {
    stubDaemon(scenarioRoutes({ airGapped: true, reachable: true, ready: true }));
    const screen = await renderCockpit();

    const offline = screen.getByRole('button', { name: /Simulate Offline/ });
    await expect.element(offline).toHaveTextContent('Simulate Offline — $0.00');
  });

  it('Air-Gap toggle persists immediately with revert-on-failure: success reports persisted', async () => {
    const saved: string[] = [];
    stubDaemon({
      ...scenarioRoutes({ airGapped: true, reachable: true, ready: true }),
      '/api/llm/settings': (call) => {
        if (call.method === 'PATCH') {
          saved.push(String(call.body));
          return ok({ status: 'ok', changed: true, settings: { air_gapped: false } });
        }
        return ok({ air_gapped: true });
      },
    });
    const screen = await renderCockpit();

    const toggle = airGapToggle(screen);
    await expect.poll(() => toggle.disabled).toBe(false);
    await userEvent.click(toggle);

    // Releasing a live air-gap always routes through the typed confirmation.
    const release = releaseButton(screen);
    await expect.poll(() => (release.element() as HTMLButtonElement).disabled).toBe(false);
    await release.click();

    await expect.poll(() => text(screen)).toContain('released (persisted)');
    expect(saved).toHaveLength(1);
    expect(JSON.parse(saved[0]).air_gapped).toBe(false);
    // The badge follows the live predicate: released air-gap is degraded.
    await expect.poll(() => badgeText(screen)).toContain('Sentinel degraded');
  });

  it('Air-Gap refusal reverts the switch with a reverted banner (nothing silently kept)', async () => {
    stubDaemon({
      ...scenarioRoutes({ airGapped: true, reachable: true, ready: true }),
      '/api/llm/settings': (call) => {
        if (call.method === 'PATCH') return refuse(500, 'disk blocked');
        return ok({ air_gapped: true });
      },
    });
    const screen = await renderCockpit();

    const toggle = airGapToggle(screen);
    await expect.poll(() => toggle.disabled).toBe(false);
    await userEvent.click(toggle);

    const release = releaseButton(screen);
    await expect.poll(() => (release.element() as HTMLButtonElement).disabled).toBe(false);
    await release.click();

    await expect.poll(() => text(screen)).toContain('reverted');
    expect(text(screen)).toContain('disk blocked');
    // Reverted: the badge still derives the engaged predicate (no silent flip).
    expect(badgeText(screen)).toContain('Sentinel Protection Active');
  });
});

describe('AutonomyCockpit emergency abort typed gate in a real browser', () => {
  function abortRoutes(abort: Route): Record<string, Route> {
    return {
      ...scenarioRoutes({ airGapped: false, reachable: true, ready: true }),
      '/api/supervisor/abort': abort,
    };
  }

  async function openAbortGate(screen: Awaited<ReturnType<typeof render>>) {
    await screen.getByRole('button', { name: /Emergency Pause/ }).click();
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeVisible();
    return dialog;
  }

  it('opens a typed gate instead of native confirm, and closes on success', async () => {
    stubDaemon(abortRoutes(() => ok({ aborted_count: 2, abort_receipt: 'r1' })));
    const screen = await renderCockpit();

    const dialog = await openAbortGate(screen);
    expect(dialog.element().getAttribute('aria-label')).toContain('abort');

    const confirm = dialog.getByRole('button', { name: 'Abort all running work' });
    const input = dialog.getByRole('textbox');
    expect((confirm.element() as HTMLButtonElement).disabled).toBe(true);

    await input.fill('sess-1X');
    expect((confirm.element() as HTMLButtonElement).disabled).toBe(true);

    await input.fill('sess-1');
    await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(false);
    await confirm.click();

    await expect.poll(() => text(screen)).toContain('Ring 0 Intercept complete: 2 job(s)');
    expect(screen.container.querySelectorAll('[role="alertdialog"]')).toHaveLength(0);
  });

  it('keeps the dialog open on refusal with the reason inline', async () => {
    stubDaemon(abortRoutes(() => refuse(403, { message: 'forbidden' })));
    const screen = await renderCockpit();

    const dialog = await openAbortGate(screen);
    await dialog.getByRole('textbox').fill('sess-1');
    const confirm = dialog.getByRole('button', { name: 'Abort all running work' });
    await expect.poll(() => (confirm.element() as HTMLButtonElement).disabled).toBe(false);
    await confirm.click();

    await expect.poll(() => text(screen)).toContain('Abort refused');
    expect(screen.container.querySelectorAll('[role="alertdialog"]')).toHaveLength(1);
  });
});
