import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  RESPONSE_SCHEMAS,
  contract,
  reportResponseContract,
  resetResponseContractReports,
  validateResponse,
} from './responseContract';

afterEach(() => {
  resetResponseContractReports();
  vi.restoreAllMocks();
});

describe('response contracts', () => {
  it('accepts the live sessions envelope and rejects a missing collection', () => {
    expect(validateResponse('sessions.list', { sessions: [] })).toEqual([]);
    expect(validateResponse('sessions.list', {})).toContainEqual({
      path: 'sessions.list.sessions',
      message: 'missing required field',
    });
  });

  it('accepts empty event and loop collections', () => {
    expect(validateResponse('events.list', { events: [] })).toEqual([]);
    expect(validateResponse('loops.list', { loops: [] })).toEqual([]);
  });

  it('rejects a session missing id and a non-finite number', () => {
    const issues = validateResponse('session', { id: 's1', title: 't', status: 'ready', created_at: 'now' });
    expect(issues.some((i) => i.path === 'session.updated_at')).toBe(true);
    expect(validateResponse('events.list', { events: [{ id: Number.NaN, timestamp: 't', session_id: 's', event_type: 'x', payload: {} }] }))
      .toContainEqual({ path: 'events.list.events[0].id', message: 'expected finite number, got NaN' });
  });

  it('keeps extra backend fields legal', () => {
    expect(
      validateResponse('capabilities', {
        catalog_digest: 'abc',
        harnesses: ['fixed'],
        budget: {},
        flags: {},
        future_field: { nested: true },
      }),
    ).toEqual([]);
  });

  it('validates the backend projected fleet and quantum catalog', () => {
    expect(validateResponse('fleet', { backends: [], refused: true })).toEqual([]);
    expect(
      validateResponse('quantum.backends', {
        backends: [
          {
            provider: 'local-simulator',
            display_name: 'Local',
            modality: 'simulator',
            device: 'local',
            credential_state: 'ready',
            telemetry: 'measured',
            executable_now: true,
          },
        ],
      }),
    ).toEqual([]);
  });

  it('reports drift once per contract and returns the payload unchanged', () => {
    const logs = vi.spyOn(console, 'error').mockImplementation(() => {});
    const payload = { not_sessions: [] };
    expect(contract('sessions.list', payload)).toBe(payload);
    contract('sessions.list', payload);
    expect(logs).toHaveBeenCalledTimes(1);
    expect(logs.mock.calls[0][0]).toContain('[response-contract] sessions.list drifted');
  });

  it('does not report valid payloads', () => {
    const logs = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(reportResponseContract('canvas.layout', {})).toEqual([]);
    expect(logs).not.toHaveBeenCalled();
  });

  it('has a non-empty object schema for every declared contract', () => {
    for (const [name, schema] of Object.entries(RESPONSE_SCHEMAS)) {
      expect(name.length, name).toBeGreaterThan(0);
      expect(schema.type, name).toBe('object');
    }
  });
});
