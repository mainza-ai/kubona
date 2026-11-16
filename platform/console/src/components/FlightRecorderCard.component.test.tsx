import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { FlightRecorderCard } from './FlightRecorderCard';

function bodyText(): string {
  return document.body.textContent ?? '';
}

function ok(data: unknown) {
  return { ok: true, status: 200, json: async () => data };
}

function stubTelemetry(payload: unknown, onFetch?: () => void) {
  const fetchStub = vi.fn(async (url: string) => {
    if (String(url).includes('/flight-recorder')) {
      onFetch?.();
      return ok(payload);
    }
    return { ok: false, status: 404, json: async () => ({}) };
  });
  vi.stubGlobal('fetch', fetchStub);
  return fetchStub;
}

const telemetry = {
  ok: true,
  session_id: 'sess-flight-1',
  tenant_id: 'default',
  terminal_verdict: 'BUDGET_EXHAUSTED',
  spent_usd: 150.0,
  spend_limit_usd: 150.0,
  duration_seconds: 42.5,
  step_count: 18,
  stop_reason: 'Session spend ceiling reached or mandate exhausted',
  last_claims: [
    {
      id: 'clm-sess-0001',
      statement: 'H2 ground state below -1.137 Hartree',
      verdict: 'SURVIVES',
      evidence_json: '{}',
    },
  ],
  recent_events: [],
};

const survives = {
  ...telemetry,
  terminal_verdict: 'SURVIVES',
  spent_usd: 1.0,
  duration_seconds: 5.0,
  step_count: 2,
  stop_reason: '',
  last_claims: [],
};

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('FlightRecorderCard in a real browser', () => {
  it('renders nothing when sessionId is null', async () => {
    const fetchStub = stubTelemetry(telemetry);
    const screen = await render(<FlightRecorderCard sessionId={null} />);
    expect(screen.container.innerHTML).toBe('');
    expect(fetchStub).not.toHaveBeenCalled();
  });

  it('renders telemetry and answers the 60-second auditor questions', async () => {
    stubTelemetry(telemetry);
    await render(<FlightRecorderCard sessionId="sess-flight-1" />);

    await expect.poll(bodyText).toContain('Auditor Flight Recorder');
    const text = bodyText();
    expect(text).toContain('BUDGET_EXHAUSTED');
    expect(text).toContain('Session spend ceiling reached or mandate exhausted');
    expect(text).toContain('150.00');
    expect(text).toContain('42.5');
    expect(text).toContain('18');
    // The auditor summary names the recorded claim and its verdict.
    expect(text).toContain('clm-sess-0001');
    expect(text).toContain('SURVIVES');
  });

  it('renders the measured fetch latency with its gate budget (within budget)', async () => {
    // Deterministic clock: every read returns `now`; the fetch stub advances
    // it, so intervening reads cannot shift the measured delta.
    let now = 1000;
    vi.spyOn(performance, 'now').mockImplementation(() => now);
    stubTelemetry(survives, () => {
      now += 250;
    });

    await render(<FlightRecorderCard sessionId="sess-flight-1" />);

    await expect.poll(() => document.querySelector('[data-testid="flight-recorder-latency"]')?.textContent ?? '')
      .toContain('telemetry fetch:');
    const latency = document.querySelector('[data-testid="flight-recorder-latency"]')?.textContent ?? '';
    expect(latency).toContain('250');
    expect(latency).toContain('gate budget 60,000 ms');
    expect(latency).toContain('within budget');
    expect(latency).not.toContain('over budget');
    expect(latency).not.toContain('NaN');
  });

  it('survives a malformed payload — null numerics render as —, never a toFixed crash', async () => {
    stubTelemetry({
      ok: true,
      session_id: 'sess-flight-1',
      tenant_id: 'default',
      terminal_verdict: 'SURVIVES',
      spent_usd: null,
      spend_limit_usd: undefined,
      duration_seconds: null,
      step_count: 2,
      stop_reason: '',
      last_claims: null,
      recent_events: null,
    });

    await render(<FlightRecorderCard sessionId="sess-flight-1" />);

    await expect.poll(bodyText).toContain('Auditor Flight Recorder');
    const text = bodyText();
    expect(text).toContain('—');
    expect(text).not.toContain('NaN');
  });

  it('surfaces a null telemetry body as an error, never a render crash', async () => {
    stubTelemetry(null);

    await render(<FlightRecorderCard sessionId="sess-flight-1" />);

    await expect.poll(bodyText).toContain('Error:');
    expect(bodyText()).toContain('null body');
  });

  it('renders an over-budget fetch honestly — never a passing badge', async () => {
    let now = 0;
    vi.spyOn(performance, 'now').mockImplementation(() => now);
    stubTelemetry(survives, () => {
      now += 61_500;
    });

    await render(<FlightRecorderCard sessionId="sess-flight-1" />);

    await expect.poll(() => document.querySelector('[data-testid="flight-recorder-latency"]')?.textContent ?? '')
      .toContain('telemetry fetch:');
    const latency = document.querySelector('[data-testid="flight-recorder-latency"]')?.textContent ?? '';
    expect(latency).toContain('61500');
    expect(latency).toContain('gate budget 60,000 ms');
    expect(latency).toContain('over budget');
    expect(latency).not.toContain('within budget');
  });
});
