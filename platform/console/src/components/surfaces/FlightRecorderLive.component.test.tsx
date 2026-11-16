import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
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

function liveText(): string {
  return document.querySelector('[aria-live="polite"]')?.textContent ?? '';
}

describe('FlightRecorder live region in a real browser', () => {
  it('announces the current step politely and stays visually hidden', async () => {
    await render(<FlightRecorder campaign={campaignWithSteps(3)} />);

    // Initial index 3 clamps to the last of 3 milestones.
    await expect.poll(liveText).toContain('Step 3 of 3');
    const live = document.querySelector('[aria-live="polite"]') as HTMLElement;
    expect(live.getAttribute('role')).toBe('status');
    expect(live.className).toContain('sr-only');

    // Both figures load through the lazy chart boundary.
    await expect.poll(() => document.querySelectorAll('[data-testid="k-chart"]').length).toBe(2);
    const text = document.body.textContent ?? '';
    expect(text).toContain('Error against FCI');
    expect(text).toContain('Chemical-accuracy limit');
  });

  it('updates the announcement only when the step changes (scrub)', async () => {
    const screen = await render(<FlightRecorder campaign={campaignWithSteps(3)} />);
    await expect.poll(liveText).toContain('Step 3 of 3');

    const scrubber = screen.getByLabelText('Playback scrubber');
    (scrubber.element() as HTMLInputElement).focus();
    await userEvent.keyboard('{Home}');

    await expect.poll(liveText).toContain('Step 1 of 3');
  });
});
