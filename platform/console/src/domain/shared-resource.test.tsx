import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { useSharedResource } from './shared-resource';

function Probe({ load }: { load: () => Promise<{ n: number }> }) {
  const state = useSharedResource('probe', load, { intervalMs: 1000 });
  return <div>{state.data ? state.data.n : 'idle'}</div>;
}

describe('shared domain resource', () => {
  it('loads once for two simultaneous consumers and refreshes after disposal', async () => {
    const load = vi.fn(async () => ({ n: 7 }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<><Probe load={load} /><Probe load={load} /></>);
    });
    expect(load).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(tree!.toJSON())).toContain('7');

    await act(async () => {
      tree!.unmount();
    });
    await act(async () => {
      tree = TestRenderer.create(<Probe load={load} />);
    });
    expect(load).toHaveBeenCalledTimes(2);
  });
});
