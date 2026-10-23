import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { describe, expect, it } from 'vitest';
import { AttentionProvider, useAttention } from './attention-context';

// P4: provider routes notifications into shared alert state with presence
// gating; surfaces without a provider get the safe no-op stub.

function Probe({ onReady }: { onReady: (v: ReturnType<typeof useAttention>) => void }) {
  const v = useAttention();
  React.useEffect(() => {
    onReady(v);
  }, [v, onReady]);
  return <div data-alert={v.alert ? `${v.alert.channel}:${v.alert.level}` : 'none'} />;
}

describe('attention context', () => {
  it('notifies anomalous and suppresses notable when away', async () => {
    let api: ReturnType<typeof useAttention> | null = null;
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
        </AttentionProvider>,
      );
    });
    expect(tree!.toJSON()).toBeTruthy();

    await act(async () => {
      api!.notify({ level: 'anomalous', source: 'vault' });
    });
    expect(api!.alert?.channel).toBe('summon');

    await act(async () => {
      api!.setPresence('away');
    });
    await act(async () => {
      api!.clear();
      api!.notify({ level: 'notable', source: 'vault' });
    });
    // Away drops non-summons: no alert stored.
    expect(api!.alert).toBe(null);
  });

  it('stub is safe without a provider', async () => {
    let api: ReturnType<typeof useAttention> | null = null;
    await act(async () => {
      TestRenderer.create(<Probe onReady={(v) => { api = v; }} />);
    });
    expect(api!.alert).toBe(null);
    // Must not throw.
    api!.notify({ level: 'anomalous', source: 'vault' });
    api!.toggleMute();
    expect(api!.alert).toBe(null);
  });
});
