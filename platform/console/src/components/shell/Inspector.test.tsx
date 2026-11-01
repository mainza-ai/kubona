import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
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

describe('Inspector', () => {
  it('offers open/focus/remove actions for a selected object', async () => {
    const object: WorkbenchObject = { id: 'campaign', kind: 'campaign', label: 'Campaign', title: 'H2', detail: 'goal', position: { x: 0, y: 0 }, size: { width: 10, height: 10 }, facts: [] };
    const onFocusObject = vi.fn();
    const onRemoveObject = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Inspector open tab="details" onTab={() => {}} onClose={() => {}} campaign={campaign} selectedObject={object} jobs={[]} copilot={<div />} onFocusObject={onFocusObject} onRemoveObject={onRemoveObject} />);
    });
    const focus = tree!.root.findAllByType('button').find((button) => button.props.children === 'Focus');
    const remove = tree!.root.findAllByType('button').find((button) => button.props.children === 'Remove from canvas');
    await act(async () => { focus!.props.onClick(); remove!.props.onClick(); });
    expect(onFocusObject).toHaveBeenCalledWith(object);
    expect(onRemoveObject).toHaveBeenCalledWith(object);
  });

  it('is absent while closed and exposes real tabs when open', async () => {
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Inspector open={false} tab="details" onTab={() => {}} onClose={onClose} campaign={campaign} selectedObject={null} jobs={[]} copilot={<div>copilot</div>} />,
      );
    });
    expect(tree!.toJSON()).toBeNull();
    await act(async () => {
      tree!.update(
        <Inspector open tab="details" onTab={() => {}} onClose={onClose} campaign={campaign} selectedObject={null} jobs={[]} copilot={<div>copilot</div>} />,
      );
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Find the ground state');
    const copilotTab = tree!.root.findAllByType('button').find((button) => button.props.children === 'Copilot');
    expect(copilotTab).toBeDefined();
  });
});
