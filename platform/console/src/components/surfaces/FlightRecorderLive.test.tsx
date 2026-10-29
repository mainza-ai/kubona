import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { FlightRecorder } from './FlightRecorder';
import type { CampaignState } from '@/types';

function campaignWithSteps(n: number): CampaignState {
  return {
    session_id: 's',
    research_goal: 'g',
    target_system: 'H2',
    quantum_factors: { ansatz_family: 'UCCSD' },
    classical_anchor: { fci_exact_energy: -1.13727, chemical_accuracy_limit: 1.6, anchor_computed: true },
    trajectory: Array.from({ length: n }, (_, i) => ({
      step: i + 1,
      energy: -1.05 - i * 0.01,
      loss: 0.4,
      theta: [0.1],
      grad_norm: 0.1,
      watchdog: 'normal',
      timestamp: '',
    })),
    branches: [],
    epistemics: {},
    notebook: {},
    updated_at: '',
  } as unknown as CampaignState;
}

describe('FlightRecorder live region (Wave-2)', () => {
  it('announces the current step politely and stays visually hidden', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorder campaign={campaignWithSteps(3)} />);
    });
    const live = tree!.root.findByProps({ 'aria-live': 'polite' });
    expect(live.props.role).toBe('status');
    expect(String(live.props.className)).toContain('sr-only');
    // Initial index 3 clamps to the last of 3 milestones.
    expect(String(live.props.children)).toContain('Step 3 of 3');
  });

  it('updates the announcement only when the step changes (scrub)', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorder campaign={campaignWithSteps(3)} />);
    });
    const scrubber = tree!.root.findByProps({ 'aria-label': 'Playback scrubber' });
    await act(async () => {
      scrubber.props.onChange({ target: { value: '0' } });
    });
    const live = tree!.root.findByProps({ 'aria-live': 'polite' });
    expect(String(live.props.children)).toContain('Step 1 of 3');
  });
});
