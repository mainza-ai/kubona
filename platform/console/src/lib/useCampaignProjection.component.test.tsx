import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { useCampaignProjection } from './useCampaignProjection';
import type { CampaignState } from '@/types';

function campaign(system: string, updated_at: string): CampaignState {
  return {
    session_id: 's1',
    research_goal: 'probe',
    target_system: system,
    quantum_factors: {},
    classical_anchor: {},
    trajectory: [],
    branches: [],
    epistemics: null,
    notebook: null,
    updated_at,
  } as unknown as CampaignState;
}

function ok(data: unknown) {
  return { ok: true, status: 200, json: async () => data };
}

function refused(status: number, message: string) {
  return {
    ok: false,
    status,
    text: async () => message,
    json: async () => ({ message }),
  };
}

function StaleProbe() {
  const controls = useCampaignProjection('s1');
  const [reloads, setReloads] = React.useState(0);
  return (
    <div>
      <button
        type="button"
        aria-label="write-then-reload"
        onClick={() => {
          controls.setCampaign(campaign('new', '2026-11-01T10:05:00Z'));
          void controls.reload().then(() => setReloads((n) => n + 1));
        }}
      />
      <span data-testid="system">{controls.campaign?.target_system ?? 'none'}</span>
      <span data-testid="reloads">{reloads}</span>
    </div>
  );
}

function RefusalProbe() {
  const controls = useCampaignProjection('s1');
  const [reloads, setReloads] = React.useState(0);
  return (
    <div>
      <button
        type="button"
        aria-label="reload"
        onClick={() => { void controls.reload().then(() => setReloads((n) => n + 1)); }}
      />
      <span data-testid="system">{controls.campaign?.target_system ?? 'none'}</span>
      <span data-testid="failure">{controls.failure?.kind ?? 'none'}</span>
      <span data-testid="reloads">{reloads}</span>
    </div>
  );
}

afterEach(() => vi.unstubAllGlobals());

describe('campaign projection ordering in a real browser', () => {
  it('ignores a stale read that lands after a newer write', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ok(campaign('old', '2026-11-01T10:00:00Z'))));

    const screen = await render(<StaleProbe />);
    await expect.element(screen.getByTestId('system')).toHaveTextContent('old');

    await screen.getByRole('button', { name: 'write-then-reload' }).click();
    await expect.element(screen.getByTestId('reloads')).toHaveTextContent('1');
    expect(screen.getByTestId('system').element().textContent).toBe('new');
    await screen.unmount();
  });

  it('keeps the snapshot and exposes a refusal instead of erasing it', async () => {
    let call = 0;
    vi.stubGlobal('fetch', vi.fn(async () => {
      call += 1;
      return call === 1
        ? ok(campaign('kept', '2026-11-01T10:00:00Z'))
        : refused(403, 'campaign read refused');
    }));

    const screen = await render(<RefusalProbe />);
    await expect.element(screen.getByTestId('system')).toHaveTextContent('kept');

    await screen.getByRole('button', { name: 'reload' }).click();
    await expect.element(screen.getByTestId('failure')).toHaveTextContent('forbidden');
    await expect.element(screen.getByTestId('reloads')).toHaveTextContent('1');
    expect(screen.getByTestId('system').element().textContent).toBe('kept');
    await screen.unmount();
  });
});
