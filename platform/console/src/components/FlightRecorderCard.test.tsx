import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { FlightRecorderCard } from './FlightRecorderCard';

describe('FlightRecorderCard', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders nothing when sessionId is null', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId={null} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('renders telemetry and answers the 60-second auditor questions', async () => {    const mockTelemetry = {
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

    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/flight-recorder')) {
        return {
          ok: true,
          json: async () => mockTelemetry,
        };
      }
      return { ok: false, status: 404 };
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId="sess-flight-1" />);
    });

    const json = JSON.stringify(tree!.toJSON());
    expect(json).toContain('Auditor Flight Recorder');
    expect(json).toContain('BUDGET_EXHAUSTED');
    expect(json).toContain('Session spend ceiling reached or mandate exhausted');
    expect(json).toContain('150.00');
    expect(json).toContain('42.5');
    expect(json).toContain('18');
  });

  it('renders the measured fetch latency with its gate budget (within budget)', async () => {
    const mockTelemetry = {
      ok: true,
      session_id: 'sess-flight-1',
      tenant_id: 'default',
      terminal_verdict: 'SURVIVES',
      spent_usd: 1.0,
      spend_limit_usd: 150.0,
      duration_seconds: 5.0,
      step_count: 2,
      stop_reason: '',
      last_claims: [],
      recent_events: [],
    };
    // Deterministic clock: every read returns `now`; the fetch stub
    // advances it, so intervening reads cannot shift the measured delta.
    let now = 1000;
    const nowSpy = vi.spyOn(performance, 'now').mockImplementation(() => now);
    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/flight-recorder')) {
        now += 250;
        return { ok: true, json: async () => mockTelemetry };
      }
      return { ok: false, status: 404 };
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId="sess-flight-1" />);
    });

    // Serialized children split the line ("telemetry fetch: ","250",...),
    // so assert the fragments, not one contiguous string.
    const json = JSON.stringify(tree!.toJSON());
    expect(json).toContain('telemetry fetch:');
    expect(json).toContain('250');
    expect(json).toContain('gate budget 60,000 ms');
    expect(json).toContain('within budget');
    expect(json).not.toContain('over budget');
    nowSpy.mockRestore();
  });

  it('survives a malformed payload — null numerics render as —, never a toFixed crash', async () => {
    const malformed = {
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
    };

    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/flight-recorder')) {
        return {
          ok: true,
          json: async () => malformed,
        };
      }
      return { ok: false, status: 404 };
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId="sess-flight-1" />);
    });

    const json = JSON.stringify(tree!.toJSON());
    expect(json).toContain('Auditor Flight Recorder');
    expect(json).toContain('—');
    expect(json).not.toContain('NaN');
  });

  it('surfaces a null telemetry body as an error, never a render crash', async () => {
    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/flight-recorder')) {
        return {
          ok: true,
          json: async () => null,
        };
      }
      return { ok: false, status: 404 };
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId="sess-flight-1" />);
    });

    expect(JSON.stringify(tree!.toJSON())).toContain('Error:');
  });

  it('renders an over-budget fetch honestly — never a passing badge', async () => {
    const mockTelemetry = {
      ok: true,
      session_id: 'sess-flight-1',
      tenant_id: 'default',
      terminal_verdict: 'SURVIVES',
      spent_usd: 1.0,
      spend_limit_usd: 150.0,
      duration_seconds: 61.0,
      step_count: 2,
      stop_reason: '',
      last_claims: [],
      recent_events: [],
    };
    let now = 0;
    const nowSpy = vi.spyOn(performance, 'now').mockImplementation(() => now);
    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/flight-recorder')) {
        now += 61_500;
        return { ok: true, json: async () => mockTelemetry };
      }
      return { ok: false, status: 404 };
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<FlightRecorderCard sessionId="sess-flight-1" />);
    });

    const json = JSON.stringify(tree!.toJSON());
    expect(json).toContain('telemetry fetch:');
    expect(json).toContain('61500');
    expect(json).toContain('gate budget 60,000 ms');
    expect(json).toContain('over budget');
    expect(json).not.toContain('within budget');
    nowSpy.mockRestore();
  });
});
