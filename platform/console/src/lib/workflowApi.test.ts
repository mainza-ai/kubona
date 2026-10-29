import { describe, it, expect, vi, beforeEach } from 'vitest';
import { planWorkflow, runWorkflow } from './api';
import type { WorkflowDefinition } from '@/types';

describe('Workflow API Client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  const testWorkflow: WorkflowDefinition = {
    name: 'test-vqe',
    trigger: 'manual',
    budget_ceiling_usd: 5.0,
    steps: [
      { id: 's1', tool: 'compiler/transpile_circuit', args: { qasm: 'OPENQASM 2.0;' } }
    ]
  };

  it('planWorkflow calls POST /api/workflows/plan and parses response', async () => {
    const mockPlan = {
      ok: true,
      dry_run: true,
      workflow_id: 'wf-123',
      steps: testWorkflow.steps,
      unresolved_references: [],
      compensations_planned: []
    };

    const seen: Array<{ url: string; init?: RequestInit }> = [];
    global.fetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
      seen.push({ url: String(url), init });
      return {
        ok: true,
        json: async () => mockPlan
      } as Response;
    });

    const res = await planWorkflow({
      session_id: 'sess-abc',
      workflow: testWorkflow
    });

    expect(global.fetch).toHaveBeenCalledTimes(1);
    // Success-path shape: URL substring + method + parsed JSON body (CodesignCockpit pattern).
    expect(seen[0].url).toContain('/api/workflows/plan');
    expect(seen[0].init?.method).toBe('POST');
    const body = JSON.parse(String(seen[0].init?.body ?? '{}'));
    expect(body.session_id).toBe('sess-abc');
    expect(body.workflow.name).toBe('test-vqe');
    expect(Array.isArray(body.workflow.steps)).toBe(true);
    expect(body.workflow.steps[0]).toMatchObject({ id: 's1', tool: 'compiler/transpile_circuit' });
    expect(res.ok).toBe(true);
    expect(res.workflow_id).toBe('wf-123');
    expect(res.dry_run).toBe(true);
    expect(Array.isArray(res.unresolved_references)).toBe(true);
  });

  it('runWorkflow calls POST /api/workflows/run and parses response', async () => {
    const mockRun = {
      ok: true,
      workflow_id: 'wf-123',
      steps_completed: 1,
      step_records: ['rec-1'],
      spent_usd: 0.5,
      last_results: { s1: { ok: true } },
      compensations: []
    };

    const seen: Array<{ url: string; init?: RequestInit }> = [];
    global.fetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
      seen.push({ url: String(url), init });
      return {
        ok: true,
        json: async () => mockRun
      } as Response;
    });

    const res = await runWorkflow({
      session_id: 'sess-abc',
      workflow: testWorkflow,
      force: true
    });

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(seen[0].url).toContain('/api/workflows/run');
    expect(seen[0].init?.method).toBe('POST');
    const body = JSON.parse(String(seen[0].init?.body ?? '{}'));
    expect(body.session_id).toBe('sess-abc');
    expect(body.workflow.name).toBe('test-vqe');
    expect(body.force).toBe(true);
    expect(res.ok).toBe(true);
    expect(res.steps_completed).toBe(1);
    expect(res.spent_usd).toBe(0.5);
    expect(res.step_records).toEqual(['rec-1']);
    expect(res.last_results).toMatchObject({ s1: { ok: true } });
  });

  it('runWorkflow throws honest error on refusal', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => 'ceiling exceeded: budget 5.0 exhausted'
    } as Response);

    await expect(
      runWorkflow({
        session_id: 'sess-abc',
        workflow: testWorkflow
      })
    ).rejects.toThrow('workflow run refused (HTTP 403): ceiling exceeded');
  });
});

describe('dev-token fail-closed guard (UI-03)', () => {
  // NEXT_PUBLIC_* values ship inside the client JS bundle by construction —
  // a real operator token here is a published secret. Local dev (`dev-token`)
  // is the only legitimate use; production must unset it and log in through
  // AuthPanel (Ed25519 session cookie) instead. A production build with the
  // token set fails closed; KUBONA_ALLOW_DEV_TOKEN=1 is the explicit,
  // auditable escape hatch for a throwaway local demo build only. Rotate is
  // operator-side: never bake a real token into a shipped bundle.
  // (Guard lives in src/lib/api.ts, owned read-only here — this suite only
  // asserts the throw; the prose above mirrors the guard comment.)
  it('throws on production import with the token set and no escape hatch', async () => {
    vi.resetModules();
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN', 'dev-token');
    vi.stubEnv('KUBONA_ALLOW_DEV_TOKEN', '');
    await expect(import('./api')).rejects.toThrow(/refusing a production build/);
    vi.unstubAllEnvs();
    vi.resetModules();
  });
});
