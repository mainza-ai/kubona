import { describe, expect, it } from 'vitest';
import { divergenceBand, routeAlert, routeWorst, shouldCollapse, type DirectorInput } from './attention';

// P4 director pins: interrupt hierarchy, presence gating, worst-pick.

describe('attention director', () => {
  const eye = 'eyes' as const;

  it('routes z-bands to ambient / glance / summon', () => {
    expect(routeAlert({ level: 'normal', source: 'vault' }, eye).channel).toBe('ambient');
    expect(routeAlert({ level: 'notable', source: 'vault' }, eye).channel).toBe('glance');
    const s = routeAlert({ level: 'anomalous', source: 'vault' }, eye);
    expect(s.channel).toBe('summon');
    expect(s.sound).toBe('chime');
    expect(s.suppressed).toBe(false);
  });

  it('escalates on spend ratio and deadlock', () => {
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 85, spendCap: 100 }, eye).channel).toBe('nudge');
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 100, spendCap: 100 }, eye).channel).toBe('summon');
    expect(routeAlert({ level: 'normal', source: 'tribunal', deadlocked: true }, eye).channel).toBe('nudge');
    // Spend escalation wins over a quiet z-band.
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 90, spendCap: 100 }, eye).sound).toBe('tick');
  });

  it('hands suppresses non-critical channels (flagged, not silent)', () => {
    const g = routeAlert({ level: 'notable', source: 'vault' }, 'hands');
    expect(g.channel).toBe('ambient');
    expect(g.suppressed).toBe(true);
    const n = routeAlert({ level: 'normal', source: 'lab', spendUsed: 85, spendCap: 100 }, 'hands');
    expect(n.channel).toBe('ambient');
    expect(n.suppressed).toBe(true);
    const s = routeAlert({ level: 'anomalous', source: 'vault' }, 'hands');
    expect(s.channel).toBe('summon');
    expect(s.suppressed).toBe(false);
  });

  it('away drops everything below summon (flagged)', () => {
    expect(routeAlert({ level: 'notable', source: 'vault' }, 'away').suppressed).toBe(true);
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 85, spendCap: 100 }, 'away').suppressed).toBe(true);
    expect(routeAlert({ level: 'anomalous', source: 'vault' }, 'away').suppressed).toBe(false);
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 100, spendCap: 100 }, 'away').suppressed).toBe(false);
  });

  it('ear keeps glance and nudge audible-visible', () => {
    expect(routeAlert({ level: 'notable', source: 'vault' }, 'ear').channel).toBe('glance');
    expect(routeAlert({ level: 'normal', source: 'lab', spendUsed: 85, spendCap: 100 }, 'ear').channel).toBe('nudge');
  });

  it('routeWorst picks the highest channel', () => {
    const inputs: DirectorInput[] = [
      { level: 'normal', source: 'a' },
      { level: 'notable', source: 'b' },
      { level: 'anomalous', source: 'c' },
    ];
    expect(routeWorst(inputs, eye)?.channel).toBe('summon');
    expect(routeWorst([], eye)).toBe(null);
  });
});

describe('divergence bands and duplicate collapse', () => {
  it('divergenceBand matches the plan cutoffs', () => {
    expect(divergenceBand(0)).toBe('normal');
    expect(divergenceBand(0.99)).toBe('normal');
    expect(divergenceBand(1)).toBe('notable');
    expect(divergenceBand(3)).toBe('notable');
    expect(divergenceBand(3.01)).toBe('anomalous');
    expect(divergenceBand(NaN)).toBe('normal');
  });

  it('shouldCollapse keys on source+level inside the window', () => {
    const base = { key: { source: 'vault', level: 'notable' }, at: 1000 };
    expect(shouldCollapse(base, { source: 'vault', level: 'notable' }, 1000 + 60_000)).toBe(true);
    expect(shouldCollapse(base, { source: 'vault', level: 'anomalous' }, 1000 + 60_000)).toBe(false);
    expect(shouldCollapse(base, { source: 'other', level: 'notable' }, 1000 + 60_000)).toBe(false);
    expect(shouldCollapse(base, { source: 'vault', level: 'notable' }, 1000 + 5 * 60_000 + 1)).toBe(false);
    expect(shouldCollapse(null, { source: 'vault', level: 'notable' }, 1000)).toBe(false);
  });
});

describe('gate batching and precision tracking (R-11)', () => {
  it('batchGateRequests coalesces same-kind gates inside the window', async () => {
    const { batchGateRequests } = await import('./attention');
    const now = 100000;
    const batches = batchGateRequests(
      [
        { id: 'a', kind: 'approve', label: 'Approve', at: now - 1000 },
        { id: 'b', kind: 'approve', label: 'Approve', at: now - 2000 },
        { id: 'c', kind: 'spend', label: 'Spend', at: now - 1000 },
        { id: 'old', kind: 'approve', label: 'Approve', at: now - 999999 },
      ],
      60000,
      now,
    );
    expect(batches).toHaveLength(2);
    const approve = batches.find((b) => b.kind === 'approve')!;
    expect(approve.count).toBe(2);
    expect(approve.ids).toEqual(['a', 'b']);
  });

  it('precision tracker needs 5 samples, tightens below 0.8', async () => {
    const { createPrecisionTracker, recordOutcome, precisionFor, shouldTighten } = await import('./attention');
    const t = createPrecisionTracker();
    expect(precisionFor(t, 'vault')).toBeNull();
    for (let i = 0; i < 4; i++) recordOutcome(t, 'vault', true);
    expect(precisionFor(t, 'vault')).toBeNull();
    recordOutcome(t, 'vault', false);
    recordOutcome(t, 'vault', false);
    recordOutcome(t, 'vault', false);
    // 4 acted / 7 total ≈ 0.57 → tighten.
    expect(precisionFor(t, 'vault')).toBeCloseTo(4 / 7, 9);
    expect(shouldTighten(t, 'vault')).toBe(true);
    expect(shouldTighten(t, 'unknown')).toBe(false);
  });
});
