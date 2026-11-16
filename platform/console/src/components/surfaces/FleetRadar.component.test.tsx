import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { FleetRadar } from './FleetRadar';

type Call = { url: string; method: string };

const localSimulator = {
  name: 'local_simulator',
  type: 'sim',
  qubits: 32,
  t1_us: 'inf',
  status: 'ONLINE',
  credential_state: 'ready',
  queue_depth: 0,
  cost_per_shot: 0,
  jurisdiction: 'LOCAL',
  itar_ear: 'EXEMPT',
};

const ibmHeron = {
  name: 'ibm_heron',
  type: 'qpu',
  qubits: 156,
  t1_us: '180',
  status: 'READY',
  credential_state: 'credentialless',
  queue_depth: 14,
  cost_per_shot: 0.0016,
  jurisdiction: 'US',
  itar_ear: 'EAR99',
};

const costSummary = {
  cap_usd: 10,
  spent_usd: 0,
  remaining_usd: 10,
  unmetered_count: 0,
  by_provider: {},
  active_provider: 'local',
  local_bypass: true,
  heuristic: false,
  currency: 'USD',
  track_only: false,
};

function bodyText(): string {
  return document.body.textContent ?? '';
}

function ok(data: unknown) {
  return { ok: true, status: 200, json: async () => data };
}

type Routes = {
  fleet: () => unknown | Promise<unknown>;
  radar?: () => unknown;
  cost?: () => unknown;
};

/** Stub the daemon by URL and record every request the surface made. */
function stubDaemon({ fleet, radar = () => ({ radar_endpoints: [] }), cost = () => costSummary }: Routes) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const target = String(url);
    calls.push({ url: target, method: (init?.method ?? 'GET').toUpperCase() });
    if (target.includes('/api/fleet/calibration/refresh')) return ok({ ok: true });
    if (target.includes('/api/fleet')) return ok(await fleet());
    if (target.includes('/api/radar')) return ok(await radar());
    if (target.includes('/api/cost/summary')) return ok(await cost());
    throw new Error(`unexpected fetch ${target}`);
  }));
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('FleetRadar credential dimension in a real browser', () => {
  it('badges credentialless backends without merging them into down', async () => {
    stubDaemon({ fleet: () => ({ backends: [localSimulator, ibmHeron] }) });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('Fleet capability matrix');
    await expect.poll(() => document.querySelectorAll('[data-testid="k-chart"]').length).toBe(1);
    const text = bodyText();
    expect(text).toContain('Cost/shot');
    // The credentialless device keeps its own status AND gains the badge.
    expect(text).toContain('READY');
    expect(text).toContain('NO CREDENTIALS');
    // The ready simulator gets no badge.
    expect(text.match(/NO CREDENTIALS/g)).toHaveLength(1);
  });

  it('shows no badge when the daemon predates credential_state', async () => {
    // Daemon from before the credential dimension: the field is absent entirely.
    const legacyHeron: Record<string, unknown> = { ...ibmHeron };
    delete legacyHeron.credential_state;
    stubDaemon({ fleet: () => ({ backends: [legacyHeron] }) });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('ibm_heron');
    expect(bodyText()).not.toContain('NO CREDENTIALS');
  });

  it('labels radar liveness honestly with backoff (H5)', async () => {
    stubDaemon({ fleet: () => ({ backends: [localSimulator] }) });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('Adaptive radar');
    const text = bodyText();
    expect(text).toContain('backs off');
    expect(text).not.toContain('1000ms Live Telemetry Radar');
  });

  it('frames tiers as a declared catalog, not live data (H10)', async () => {
    stubDaemon({ fleet: () => ({ backends: [] }) });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('declared catalog');
    expect(bodyText()).toContain('declared — not live canary data');
  });

  it('offers an explicit manual refresh in addition to auto-poll', async () => {
    const calls = stubDaemon({ fleet: () => ({ backends: [localSimulator] }) });
    const screen = await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('local_simulator');
    const fleetCalls = () => calls.filter((call) => call.url.includes('/api/fleet')).length;
    const callsBefore = fleetCalls();
    await screen.getByRole('button', { name: 'Refresh now' }).click();

    await expect.poll(fleetCalls).toBeGreaterThan(callsBefore);
  });

  it('chips UNPROVENANCED cards on the grid and cost when telemetry is absent (H6)', async () => {
    stubDaemon({
      fleet: () => ({
        backends: [
          { name: 'mystery_sim', type: 'sim', qubits: 32, t1_us: 'inf', status: 'ONLINE', queue_depth: 0, cost_per_shot: 0, jurisdiction: 'LOCAL', itar_ear: 'EXEMPT' },
        ],
      }),
    });
    const screen = await render(<FleetRadar />);

    // Card grid chip renders without expanding.
    await expect.poll(bodyText).toContain('UNPROVENANCED');
    expect(bodyText()).not.toContain('CATALOG');
    // Cost line chip renders after expanding the card.
    await screen.getByRole('button', { name: /Deep Physics/ }).click();
    await expect.poll(bodyText).toContain('Cost per Shot');
    expect(bodyText()).toContain('(UNPROVENANCED)');
  });

  it('tags declared-catalog cards CATALOG and leaves measured ones untagged', async () => {
    stubDaemon({
      fleet: () => ({
        backends: [
          { ...localSimulator, telemetry: 'measured' },
          { ...ibmHeron, telemetry: 'declared', catalog_verified: null },
        ],
      }),
    });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('ibm_heron');
    // Exactly one CATALOG tag: queue/coherence on the Heron card are
    // vendor catalog, not live measurement. Absent telemetry (old
    // daemons) renders no tag rather than a false claim either way.
    expect(bodyText().match(/CATALOG/g)).toHaveLength(1);
  });
});

describe('FleetRadar loading + stale chrome (Wave-3) in a real browser', () => {
  it('shows a loading skeleton on first load and hides it once the fleet settles', async () => {
    // Pending fleet: first paint is the honest skeleton alongside the empty grid.
    stubDaemon({ fleet: () => new Promise(() => undefined) });
    const pending = await render(<FleetRadar />);

    await expect.poll(() => pending.container.querySelectorAll('[data-testid="fleet-radar-skeleton"]').length).toBe(1);
    expect(bodyText()).not.toContain('local_simulator');
    await pending.unmount();

    // Settled fleet: skeleton gone, devices listed.
    stubDaemon({ fleet: () => ({ backends: [localSimulator] }) });
    const settled = await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('local_simulator');
    expect(settled.container.querySelectorAll('[data-testid="fleet-radar-skeleton"]')).toHaveLength(0);
  });

  it('badges stale data when the radar backoff lags past nominal', async () => {
    // Refused radar grows the poll interval past 1s nominal → stale badge.
    stubDaemon({
      fleet: () => ({ backends: [] }),
      radar: () => ({ refused: true, refuse_reason: 'radar down' }),
    });
    await render(<FleetRadar />);

    await expect.poll(() => document.querySelectorAll('[data-testid="fleet-radar-stale"]').length).toBe(1);
    const text = bodyText();
    expect(text).toContain('Stale');
    expect(text).toContain('1.5s interval');
    expect(text).toContain('Radar refused — radar down');
  });

  it('shows no stale badge on a nominal 1s poll', async () => {
    stubDaemon({ fleet: () => ({ backends: [localSimulator] }) });
    await render(<FleetRadar />);

    await expect.poll(bodyText).toContain('local_simulator');
    expect(document.querySelectorAll('[data-testid="fleet-radar-stale"]')).toHaveLength(0);
  });
});
