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

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockPlan
    } as Response);

    const res = await planWorkflow({
      session_id: 'sess-abc',
      workflow: testWorkflow
    });

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(res.ok).toBe(true);
    expect(res.workflow_id).toBe('wf-123');
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

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockRun
    } as Response);

    const res = await runWorkflow({
      session_id: 'sess-abc',
      workflow: testWorkflow,
      force: true
    });

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(res.ok).toBe(true);
    expect(res.steps_completed).toBe(1);
    expect(res.spent_usd).toBe(0.5);
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
