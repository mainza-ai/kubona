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

  it('collapses duplicate alerts instead of stacking (R-11)', async () => {
    let api: ReturnType<typeof useAttention> | null = null;
    await act(async () => {
      TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
        </AttentionProvider>,
      );
    });
    // 'eyes' lets notable through (hands would suppress it pre-collapse).
    await act(async () => {
      api!.setPresence('eyes');
    });
    await act(async () => {
      api!.notify({ level: 'notable', source: 'vault' });
    });
    const firstId = api!.alert!.id;
    await act(async () => {
      api!.notify({ level: 'notable', source: 'vault' });
    });
    // Same id (no new alert), duplicate count incremented.
    expect(api!.alert!.id).toBe(firstId);
    expect(api!.alert!.duplicates).toBe(1);
    await act(async () => {
      api!.notify({ level: 'notable', source: 'vault' });
    });
    expect(api!.alert!.duplicates).toBe(2);
    // Different source does not collapse.
    await act(async () => {
      api!.notify({ level: 'notable', source: 'lab' });
    });
    expect(api!.alert!.id).not.toBe(firstId);
    expect(api!.alert!.duplicates).toBe(0);
  });

  it('switches presence through the dial values', async () => {
    let api: ReturnType<typeof useAttention> | null = null;
    await act(async () => {
      TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
        </AttentionProvider>,
      );
    });
    expect(api!.presence).toBe('hands');
    for (const p of ['eyes', 'ear', 'away', 'hands'] as const) {
      await act(async () => {
        api!.setPresence(p);
      });
      expect(api!.presence).toBe(p);
    }
  });

  it('tracks precision and tightens low-precision sources (R-11)', async () => {
    let api: ReturnType<typeof useAttention> | null = null;
    await act(async () => {
      TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
        </AttentionProvider>,
      );
    });
    // 1 acted + 4 dismissed = 5 rated, precision 0.2 -> tightened.
    // Use anomalous so alerts survive default 'hands' presence.
    await act(async () => {
      api!.notify({ level: 'anomalous', source: 'vault' });
    });
    await act(async () => {
      api!.resolve(true);
    });
    for (let i = 0; i < 4; i++) {
      await act(async () => {
        api!.notify({ level: 'anomalous', source: 'vault' });
      });
      await act(async () => {
        api!.resolve(false);
      });
    }
    expect(api!.tightened).toContain('vault');
  });
});
