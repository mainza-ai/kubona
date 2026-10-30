import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { WorkflowPanel } from './WorkflowPanel';
import ConfirmDialog from './ConfirmDialog';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({ planWorkflow: vi.fn(), runWorkflow: vi.fn() }));

describe('WorkflowPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('plans, confirms by name, and runs through the backend', async () => {
    vi.mocked(api.planWorkflow).mockResolvedValue({
      ok: true, dry_run: true, workflow_id: 'wf-1', steps: [], unresolved_references: [], compensations_planned: [],
    });
    vi.mocked(api.runWorkflow).mockResolvedValue({
      ok: true, workflow_id: 'wf-1', steps_completed: 1, step_records: [], spent_usd: 0, last_results: {}, compensations: [],
    });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<WorkflowPanel sessionId="s1" />); });
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Plan'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.planWorkflow).toHaveBeenCalled());
    expect(JSON.stringify(tree!.toJSON())).toContain('wf-1');

    await act(async () => {
      tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Run workflow'))!.props.onClick();
    });
    const gate = tree!.root.findByType(ConfirmDialog);
    await act(async () => {
      gate.findByType('input').props.onChange({ target: { value: 'campaign-audit' } });
    });
    await act(async () => { await tree!.root.findByType(ConfirmDialog).props.onConfirm(); });
    await vi.waitFor(() => expect(api.runWorkflow).toHaveBeenCalled());
    expect(JSON.stringify(tree!.toJSON())).toContain('completed ');
  });
});
