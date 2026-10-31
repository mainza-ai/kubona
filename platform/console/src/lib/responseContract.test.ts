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

  it('accepts live chat, job, workflow, auth, and cost payloads', () => {
    expect(validateResponse('messages.list', { messages: [] })).toEqual([]);
    expect(validateResponse('jobs.list', { jobs: [] })).toEqual([]);
    expect(validateResponse('job', { job: { id: 'j1', session_id: 's1', status: 'QUEUED' } })).toEqual([]);
    expect(validateResponse('auth.whoami', { authenticated: true, tenant_id: 'lab' })).toEqual([]);
    expect(
      validateResponse('cost.summary', {
        cap_usd: 10,
        spent_usd: 1.5,
        remaining_usd: null,
        active_provider: 'local',
        currency: 'USD',
        by_provider: {},
      }),
    ).toEqual([]);
    expect(
      validateResponse('workflow.plan', {
        ok: true,
        dry_run: true,
        workflow_id: 'wf',
        steps: [],
        unresolved_references: [],
        compensations_planned: [],
      }),
    ).toEqual([]);
  });

  it('rejects a cost payload without the spend anchor', () => {
    const issues = validateResponse('cost.summary', {
      cap_usd: 10,
      active_provider: 'local',
      currency: 'USD',
    });
    expect(issues.some((i) => i.path === 'cost.summary.spent_usd')).toBe(true);
  });

  it('accepts chat, autoresearch, metasearch, codesign, LLM, and HF payloads', () => {
    expect(validateResponse('chat.send', { user: {}, assistant: {} })).toEqual([]);
    expect(validateResponse('chat.stream.done', { user: {}, assistant: {} })).toEqual([]);
    expect(validateResponse('tribunal.run', { tribunal: { turns: [] }, assistant_message: {} })).toEqual([]);
    expect(validateResponse('chat.launch', { job: {}, loop: {} })).toEqual([]);
    expect(validateResponse('autoresearch.harnesses', { harnesses: [], docker_runner: null })).toEqual([]);
    expect(validateResponse('autoresearch.loop', { id: 'l1', session_id: 's1', status: 'running' })).toEqual([]);
    expect(validateResponse('autoresearch.loops', { loops: [] })).toEqual([]);
    expect(validateResponse('autoresearch.step', { step: {}, state: {} })).toEqual([]);
    expect(validateResponse('metasearch.proposals', { proposals: [] })).toEqual([]);
    expect(validateResponse('codesign.evaluate', { ok: true, spec_kind: 'qasm', result: {} })).toEqual([]);
    expect(
      validateResponse('codesign.batch', { ok: true, spec_kind: 'qasm', frontier: {}, details: [] }),
    ).toEqual([]);
    expect(validateResponse('codesign.verified', { total: { toffoli: 7 } })).toEqual([]);
    expect(validateResponse('codesign.packs', { ok: true, packs: [] })).toEqual([]);
    expect(validateResponse('codesign.families', { ok: true })).toEqual([]);
    expect(validateResponse('codesign.rerun', { ok: true })).toEqual([]);
    expect(validateResponse('llm.models', { models: [] })).toEqual([]);
    expect(validateResponse('hf.search', { models: [] })).toEqual([]);
    expect(validateResponse('hf.details', { model: null })).toEqual([]);
    expect(validateResponse('hf.download', { task: null })).toEqual([]);
    expect(validateResponse('hf.downloads', { downloads: [] })).toEqual([]);
  });

  it('rejects a codesign verified payload with no numeric anchor', () => {
    expect(validateResponse('codesign.verified', { total: {} })).toContainEqual({
      path: 'codesign.verified.total.toffoli',
      message: 'missing required field',
    });
  });

  it('accepts vault, ledger, and observatory lens payloads', () => {
    expect(validateResponse('ledger.verify', { ok: true, fault_count: 0 })).toEqual([]);
    expect(
      validateResponse('vault.lease', {
        lease: {
          lease_id: 'l1',
          scope: 'read',
          ttl_seconds: 60,
          lease_signature: 'sig',
          issued_at: 'now',
          expires_at: 'later',
        },
      }),
    ).toEqual([]);
    expect(validateResponse('lens.request', { ok: true, operation_id: 'op' })).toEqual([]);
    expect(validateResponse('lens.resolve', { ok: true, done: false })).toEqual([]);
    expect(validateResponse('lens.history', { ok: true, operations: [] })).toEqual([]);
    expect(validateResponse('lens.status', { ok: true })).toEqual([]);
    expect(validateResponse('vault.latest', { ok: true, sealed: null })).toEqual([]);
    expect(validateResponse('vault.calibration', { ok: true, evaluations: 0 })).toEqual([]);
    expect(validateResponse('vault.history', { ok: true, predictions: [] })).toEqual([]);
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
