import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ArenaHud } from './ArenaHud';

function render(showWorkbench: boolean) {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(
      <ArenaHud
        targetLabel="H2"
        swarmDetail="idle"
        view="orbital"
        onViewChange={() => {}}
        spendUsd={0}
        spendCapUsd={150}
        sessionElapsed="00:00:00"
        onRecenter={() => {}}
        showWorkbench={showWorkbench}
      />,
    );
  });
  return JSON.stringify(tree!.toJSON());
}

describe('ArenaHud workbench kill switch', () => {
  it('hides the workbench tab when disabled', () => {
    expect(render(false)).not.toContain('Workbench');
    expect(render(true)).toContain('Workbench');
  });

  it('renders campaign switcher and experiment lab expanded state when provided', () => {
    const onOpenCampaigns = vi.fn();
    const onOpenExperimentLab = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ArenaHud
          targetLabel="H2"
          swarmDetail="idle"
          view="workbench"
          onViewChange={() => {}}
          spendUsd={0}
          spendCapUsd={150}
          onRecenter={() => {}}
          campaignTitle="Alpha Campaign"
          onOpenCampaigns={onOpenCampaigns}
          experimentLabOpen={true}
          onOpenExperimentLab={onOpenExperimentLab}
        />,
      );
    });
    const campaignBtn = tree!.root.findByProps({ 'aria-label': 'Switch campaign' });
    const labBtn = tree!.root.findByProps({ 'aria-label': 'Open Experiment & Discovery Lab' });
    expect(labBtn.props['aria-expanded']).toBe(true);
    act(() => {
      campaignBtn.props.onClick();
      labBtn.props.onClick();
    });
    expect(onOpenCampaigns).toHaveBeenCalledTimes(1);
    expect(onOpenExperimentLab).toHaveBeenCalledTimes(1);
    act(() => { tree!.unmount(); });
  });
});
