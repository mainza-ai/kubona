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

  it('renders telemetry and answers the 60-second auditor questions', async () => {
    const mockTelemetry = {
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
});
