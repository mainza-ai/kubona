import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ArenaHud } from './ArenaHud';

const baseProps = {
  targetLabel: 'H2',
  swarmDetail: 'idle',
  view: 'orbital' as const,
  onViewChange: () => {},
  spendUsd: 0,
  spendCapUsd: 150,
  sessionElapsed: '00:00:00',
  onRecenter: () => {},
};

describe('ArenaHud workbench kill switch in a real browser', () => {
  it('hides the workbench tab when disabled and shows it when enabled', async () => {
    const hidden = await render(<ArenaHud {...baseProps} showWorkbench={false} />);
    await expect.element(hidden.getByRole('tab', { name: 'Orbital Space' })).toBeVisible();
    expect(hidden.container.textContent).not.toContain('Workbench');

    const shown = await render(<ArenaHud {...baseProps} showWorkbench={true} />);
    await expect.element(shown.getByRole('tab', { name: 'Workbench' })).toBeVisible();
  });

  it('opens the campaign switcher and toggles the experiment lab', async () => {
    const onOpenCampaigns = vi.fn();
    const onOpenExperimentLab = vi.fn();
    const screen = await render(
      <ArenaHud
        {...baseProps}
        view="workbench"
        campaignTitle="Alpha Campaign"
        onOpenCampaigns={onOpenCampaigns}
        experimentLabOpen={true}
        onOpenExperimentLab={onOpenExperimentLab}
      />,
    );

    await expect
      .element(screen.getByRole('tab', { name: 'Workbench' }))
      .toHaveAttribute('aria-selected', 'true');

    const campaign = screen.getByRole('button', { name: 'Switch campaign' });
    await expect.element(campaign).toBeVisible();
    await expect.element(campaign).toHaveTextContent('Alpha Campaign');
    expect(campaign.element().getAttribute('title')).toBe('Campaign: Alpha Campaign');
    await campaign.click();
    expect(onOpenCampaigns).toHaveBeenCalledTimes(1);

    const lab = screen.getByRole('button', { name: 'Open Experiment & Discovery Lab' });
    await expect.element(lab).toHaveAttribute('aria-expanded', 'true');
    await lab.click();
    expect(onOpenExperimentLab).toHaveBeenCalledTimes(1);
  });
});
