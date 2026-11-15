import { describe, expect, it, vi } from 'vitest';
import {
  clearWorkflowPlan,
  getWorkflowPlan,
  publishWorkflowPlan,
  resetWorkflowPlanForTests,
  setWorkflowPlanTaskIds,
} from './workflow-plan-store';

describe('workflow plan store (C8)', () => {
  it('publishes a draft, attaches task ids, and clears', () => {
    resetWorkflowPlanForTests();
    const listener = vi.fn();
    publishWorkflowPlan({
      workflowId: 'wf-1',
      name: 'plan',
      steps: [{ id: 's1', tool: 't' }],
      unresolved: [],
      createdAt: 42,
    });
    expect(getWorkflowPlan()?.createdAt).toBe(42);
    setWorkflowPlanTaskIds(['t1', 't2']);
    expect(getWorkflowPlan()?.taskIds).toEqual(['t1', 't2']);
    clearWorkflowPlan();
    expect(getWorkflowPlan()).toBeNull();
    void listener;
  });

  it('ignores task ids with no published plan', () => {
    resetWorkflowPlanForTests();
    setWorkflowPlanTaskIds(['t1']);
    expect(getWorkflowPlan()).toBeNull();
  });
});
