import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { FlightRecorder } from './FlightRecorder';
import type { CampaignState } from '@/types';

function campaignWith(trajectory: CampaignState['trajectory'], anchorComputed = true): CampaignState {
  return {
    session_id: 's',
    research_goal: 'g',
    target_system: 'H2',
    quantum_factors: { ansatz_family: 'UCCSD' },
    classical_anchor: { fci_exact_energy: -1.13727, chemical_accuracy_limit: 1.6, anchor_computed: anchorComputed },
    trajectory,
    branches: [],
    epistemics: {},
    notebook: {},
    updated_at: '',
  } as unknown as CampaignState;
}

describe('FlightRecorder measured points', () => {
  it('labels measured observations distinctly from optimizer steps', () => {
    const campaign = campaignWith([
      { step: 1, energy: -1.05, loss: 0.4, theta: [0.1], grad_norm: 0.1, watchdog: 'normal', timestamp: '' },
      { step: 2, energy: -1.13699, loss: 0.0003, theta: [], grad_norm: 0, watchdog: 'measured_krylov', timestamp: '' },
    ]);
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorder campaign={campaign} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Measured Point 2 (measured_krylov)');
    expect(text).toContain('an observation, not an optimizer step');
    expect(text).toContain('Baseline H2 Hypothesis');
  });

  it('withholds Δ and breakthrough labels when anchor_computed is false', () => {
    const campaign = campaignWith(
      [{ step: 1, energy: 0, loss: 0, theta: [0], grad_norm: 0, watchdog: 'normal', timestamp: '' }],
      false
    );
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorder campaign={campaign} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('UNVERIFIED (no measurement)');
    expect(text).not.toContain('(Accuracy Met)');
  });

  it('labels the story card by milestone position, not by raw iteration number', () => {
    // Audit 2026-10-21: the header read "Iteration 15 of 7" — a global
    // iteration counter over a milestone count (15 > 7). The position
    // ("Milestone 1 of 2") and the global counter ("Iteration 15") are
    // distinct facts; the header must state both, coherently.
    const campaign = campaignWith([
      { step: 15, energy: -1.05, loss: 0.4, theta: [0.1], grad_norm: 0.1, watchdog: 'normal', timestamp: '' },
      { step: 30, energy: -1.13699, loss: 0.0003, theta: [], grad_norm: 0, watchdog: 'measured_krylov', timestamp: '' },
    ]);
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<FlightRecorder campaign={campaign} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    // Serialized children split the label ("Milestone ","2"," of ","2").
    expect(text).toContain('"Milestone ","2"," of ","2"');
    expect(text).toContain('" · Iteration ","30"');
    expect(text).not.toContain('"Iteration ","30"," of "');
  });
});
