import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { FlightRecorder } from './FlightRecorder';
import type { CampaignState, LedgerEvent } from '@/types';

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

function bodyText(): string {
  return document.body.textContent ?? '';
}

afterEach(() => vi.unstubAllGlobals());

describe('FlightRecorder measured points in a real browser', () => {
  it('labels measured observations distinctly from optimizer steps', async () => {
    const campaign = campaignWith([
      { step: 1, energy: -1.05, loss: 0.4, theta: [0.1], grad_norm: 0.1, watchdog: 'normal', timestamp: '' },
      { step: 2, energy: -1.13699, loss: 0.0003, theta: [], grad_norm: 0, watchdog: 'measured_krylov', timestamp: '' },
    ]);
    await render(<FlightRecorder campaign={campaign} />);

    // The story card clamps to the last milestone (the measured point).
    await expect.poll(bodyText).toContain('Measured Point 2 (measured_krylov)');
    expect(bodyText()).toContain('an observation, not an optimizer step');
    // The baseline milestone is still reachable from the tick roster.
    expect(bodyText()).toContain('Baseline H2 Hypothesis');
  });

  it('withholds Δ and breakthrough labels when anchor_computed is false', async () => {
    const campaign = campaignWith(
      [{ step: 1, energy: 0, loss: 0, theta: [0], grad_norm: 0, watchdog: 'normal', timestamp: '' }],
      false
    );
    await render(<FlightRecorder campaign={campaign} />);

    await expect.poll(bodyText).toContain('UNVERIFIED (no measurement)');
    expect(bodyText()).not.toContain('(Accuracy Met)');
  });

  it('states "no live loops" (via the override roster) when the loop roster is empty', async () => {
    const campaign = campaignWith([]);
    await render(<FlightRecorder loops={[]} events={[]} campaign={campaign} onLoopsChanged={() => {}} />);

    await expect.poll(bodyText).toContain('no live loops');
  });

  it('pages older ledger events through the cursor endpoint', async () => {
    const calls: Array<{ url: string; method: string }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? 'GET' });
      return {
        ok: true,
        status: 200,
        json: async () => ({
          events: [{ id: 1, timestamp: '2026-11-01T10:00:00Z', session_id: 's', event_type: 'older', payload: {} }],
        }),
      };
    });

    const screen = await render(
      <FlightRecorder
        campaign={campaignWith([])}
        sessionId="s"
        events={[{ id: 2, timestamp: '2026-11-01T10:01:00Z', session_id: 's', event_type: 'newer', payload: {} } as LedgerEvent]}
      />,
    );

    // The cursor button lives inside a closed <details>; open it as a user would.
    await screen.getByText('Ledger history · 1 loaded').click();
    await screen.getByRole('button', { name: 'Load older events' }).click();

    await expect.poll(bodyText).toContain('older');
    const cursorCall = calls.find((call) => call.url.includes('/api/events'));
    expect(cursorCall).toBeTruthy();
    expect(cursorCall!.method).toBe('GET');
    expect(cursorCall!.url).toContain('session_id=s');
    // Cursor is the smallest loaded id (2), not the count.
    expect(cursorCall!.url).toContain('before_id=2');
    expect(bodyText()).toContain('Ledger history · 2 loaded');
  });

  it('labels the story card by milestone position, not by raw iteration number', async () => {
    // Audit 2026-10-21: the header read "Iteration 15 of 7" — a global
    // iteration counter over a milestone count (15 > 7). The position
    // ("Milestone 1 of 2") and the global counter ("Iteration 15") are
    // distinct facts; the header must state both, coherently.
    const campaign = campaignWith([
      { step: 15, energy: -1.05, loss: 0.4, theta: [0.1], grad_norm: 0.1, watchdog: 'normal', timestamp: '' },
      { step: 30, energy: -1.13699, loss: 0.0003, theta: [], grad_norm: 0, watchdog: 'measured_krylov', timestamp: '' },
    ]);
    await render(<FlightRecorder campaign={campaign} />);

    await expect.poll(bodyText).toContain('Milestone 2 of 2 · Iteration 30');
    expect(bodyText()).not.toContain('Iteration 30 of');
  });
});
