import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { Inspector } from './Inspector';
import type { CampaignState } from '@/types';
import type { WorkbenchObject } from '@/components/workbench/workbench-model';

const campaign = {
  session_id: 's1',
  target_system: 'H2',
  research_goal: 'Find the ground state',
  hypothesis_id: 'hyp-1',
  quantum_factors: { recommended_backend: 'local_simulator' },
  classical_anchor: { anchor_computed: true, method: 'PySCF FCI', fci_exact_energy: -1.13727, vqe_expectation_energy: -1.13, discrepancy_mha: 7.27 },
  epistemics: { status: 'PROJECTED', statement: 'Claim pending', alpha_corroborated: 0.8, beta_contested: 0.1, gamma_unverified: 0.1, sources: ['ledger:1'] },
  notebook: { provenance_hash: 'sha256:abc' },
} as unknown as CampaignState;

describe('Inspector in a real browser', () => {
  it('offers open/focus/remove actions for a selected object', async () => {
    const object: WorkbenchObject = { id: 'campaign', kind: 'campaign', label: 'Campaign', title: 'H2', detail: 'goal', position: { x: 0, y: 0 }, size: { width: 10, height: 10 }, facts: [] };
    const onFocusObject = vi.fn();
    const onRemoveObject = vi.fn();
    const screen = await render(
      <Inspector open tab="details" onTab={() => {}} onClose={() => {}} campaign={campaign} selectedObject={object} jobs={[]} copilot={<div />} onFocusObject={onFocusObject} onRemoveObject={onRemoveObject} />,
    );
    await screen.getByRole('button', { name: 'Focus' }).click();
    await screen.getByRole('button', { name: 'Remove from canvas' }).click();
    expect(onFocusObject).toHaveBeenCalledWith(object);
    expect(onRemoveObject).toHaveBeenCalledWith(object);
  });

  it('is absent while closed and exposes real tabs when open', async () => {
    const onClose = vi.fn();
    const screen = await render(
      <Inspector open={false} tab="details" onTab={() => {}} onClose={onClose} campaign={campaign} selectedObject={null} jobs={[]} copilot={<div>copilot</div>} />,
    );
    expect(screen.container.childElementCount).toBe(0);
    await screen.rerender(
      <Inspector open tab="details" onTab={() => {}} onClose={onClose} campaign={campaign} selectedObject={null} jobs={[]} copilot={<div>copilot</div>} />,
    );
    await expect.element(screen.getByText('Find the ground state')).toBeVisible();
    await expect.element(screen.getByRole('tab', { name: 'Copilot' })).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Details' }).element().getAttribute('aria-selected')).toBe('true');
  });

  it('creates a task from a selected record', async () => {
    const object: WorkbenchObject = { id: 'job-j1', kind: 'job', label: 'Job', title: 'Run 1', detail: '', position: { x: 0, y: 0 }, size: { width: 10, height: 10 }, facts: [], ref: { domain: 'job', id: 'j1' } };
    const onCreateTask = vi.fn();
    const screen = await render(
      <Inspector open tab="details" onTab={() => {}} onClose={() => {}} campaign={campaign} selectedObject={object} jobs={[]} onCreateTask={onCreateTask} copilot={<div />} />,
    );
    await screen.getByRole('button', { name: 'Create task' }).click();
    expect(onCreateTask).toHaveBeenCalledWith(object);
  });

  it('toggles a selected task open or done', async () => {
    const object: WorkbenchObject = { id: 'task-t1', kind: 'task', label: 'Task', title: 'Review anchor', detail: '', position: { x: 0, y: 0 }, size: { width: 10, height: 10 }, facts: [], ref: { domain: 'task', id: 't1' } };
    const onTaskStatus = vi.fn();
    const screen = await render(
      <Inspector open tab="details" onTab={() => {}} onClose={() => {}} campaign={campaign} selectedObject={object} jobs={[]} tasks={[{ id: 't1', session_id: 's1', title: 'Review anchor', status: 'open', source_domain: 'measurement', source_id: 'm1', created_at: '', updated_at: '' }]} onTaskStatus={onTaskStatus} copilot={<div />} />,
    );
    await screen.getByRole('button', { name: 'Mark done' }).click();
    expect(onTaskStatus).toHaveBeenCalledWith('t1', 'done');
  });

  it('makes Jobs tab rows clickable and invokes onFocusJob', async () => {
    const onFocusJob = vi.fn();
    const job = {
      id: 'job-focus-1',
      session_id: 's1',
      job_type: 'vqe',
      status: 2,
      params_json: {},
      created_at: '2026-11-01T00:00:00Z',
    };
    const screen = await render(
      <Inspector
        open
        tab="jobs"
        onTab={() => {}}
        onClose={() => {}}
        campaign={campaign}
        selectedObject={null}
        jobs={[job]}
        onFocusJob={onFocusJob}
        copilot={<div />}
      />,
    );
    await screen.getByTestId('inspector-job-job-focus-1').click();
    expect(onFocusJob).toHaveBeenCalledWith(job);
  });
});
