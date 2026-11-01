import { describe, expect, it } from 'vitest';
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
});
