import { describe, expect, it } from 'vitest';
import { routeAlert, routeWorst, type DirectorInput } from './attention';

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
