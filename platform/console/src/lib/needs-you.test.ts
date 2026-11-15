import { describe, expect, it } from 'vitest';
import {
  blockingReasons,
  buildNeedsYou,
  classifySubject,
  unseenNeedsYou,
  waitingLabel,
} from './needs-you';
import type { ObligationRow } from './api';

function row(overrides: Partial<ObligationRow> = {}): ObligationRow {
  return {
    id: 'ob_1',
    subject_kind: 'job',
    subject_id: 'job-1',
    owner: 'session:s1',
    disposition: 'open',
    required_cases: ['receipt'],
    required_authorities: ['reviewer'],
    close_ready: false,
    close_reasons: [],
    ...overrides,
  };
}

describe('classifySubject', () => {
  it('maps server subject kinds to navigable classes', () => {
    expect(classifySubject('job')).toBe('job');
    expect(classifySubject('hardware-job')).toBe('job');
    expect(classifySubject('autoresearch_loop')).toBe('loop');
    expect(classifySubject('campaign')).toBe('campaign');
    expect(classifySubject('task')).toBe('task');
    expect(classifySubject('tribunal-run')).toBe('claim');
    expect(classifySubject('proof-receipt')).toBe('proof');
    expect(classifySubject('')).toBe('other');
    expect(classifySubject('quarantine-review')).toBe('other');
  });
});

describe('blockingReasons', () => {
  it('prefers verbatim unknown descriptions and keeps refusal reasons after them', () => {
    const reasons = blockingReasons(
      row({
        unknowns_detail: [{ id: 'u1', description: 'budget approval pending' }],
        close_reasons: ['missing reviewer authority', 'budget approval pending'],
      }),
    );
    // Deduped, server order preserved: nothing invented, nothing repeated.
    expect(reasons).toEqual(['budget approval pending', 'missing reviewer authority']);
  });

  it('ignores blank text instead of rendering an empty sentence', () => {
    expect(blockingReasons(row({ close_reasons: ['  ', ''] }))).toEqual([]);
  });
});

describe('buildNeedsYou', () => {
  it('drops closed rows — a decision removes the item', () => {
    const items = buildNeedsYou([row({ id: 'a' }), row({ id: 'b', disposition: 'closed' })]);
    expect(items.map((item) => item.id)).toEqual(['a']);
  });

  it('orders decidable rows first, then longest-waiting, then unknown clocks', () => {
    const items = buildNeedsYou([
      row({ id: 'blocked-old', close_ready: false, created_at: '2026-01-01T00:00:00Z' }),
      row({ id: 'blocked-no-clock', close_ready: false, created_at: null }),
      row({ id: 'ready-new', close_ready: true, created_at: '2026-06-01T00:00:00Z' }),
      row({ id: 'ready-old', close_ready: true, created_at: '2026-02-01T00:00:00Z' }),
    ]);
    expect(items.map((item) => item.id)).toEqual(['ready-old', 'ready-new', 'blocked-old', 'blocked-no-clock']);
    expect(items.find((item) => item.id === 'blocked-no-clock')?.waitingSince).toBeNull();
  });

  it('carries the state, reason, and a content signature that changes with the row', () => {
    const updated = '2026-11-14T10:00:00Z';
    const [item] = buildNeedsYou([
      row({
        id: 'ob_9',
        subject_kind: 'claim',
        subject_id: 'claim-7',
        close_ready: true,
        unknowns_detail: [{ id: 'u1', description: 'anchor missing' }],
        created_at: '2026-11-13T09:00:00Z',
        updated_at: updated,
      }),
    ]);
    expect(item).toMatchObject({
      id: 'ob_9',
      subjectClass: 'claim',
      subjectId: 'claim-7',
      state: 'decidable',
      reason: 'anchor missing',
      waitingSince: '2026-11-13T09:00:00Z',
      signature: `ob_9@${updated}`,
    });
  });

  it('refuses to invent a waiting time when the clock is unreadable', () => {
    const [item] = buildNeedsYou([row({ created_at: 'not-a-time' })]);
    expect(item.waitingSince).toBeNull();
    expect(item.reason).toBeNull();
  });
});

describe('unseenNeedsYou', () => {
  it('counts only signatures the reviewer has not looked at', () => {
    const items = buildNeedsYou([
      row({ id: 'a', updated_at: '2026-11-14T10:00:00Z' }),
      row({ id: 'b', updated_at: '2026-11-14T10:00:00Z' }),
    ]);
    const seen = new Set([items[0].signature]);
    expect(unseenNeedsYou(items, seen).map((item) => item.id)).toEqual([items[1].id]);
  });
});

describe('waitingLabel', () => {
  const now = Date.parse('2026-11-14T12:00:00Z');
  it('renders an honest waiting duration', () => {
    expect(waitingLabel({ waitingSince: '2026-11-14T11:59:30Z' }, now)).toBe('waiting <1m');
    expect(waitingLabel({ waitingSince: '2026-11-14T11:55:00Z' }, now)).toBe('waiting 5m');
    expect(waitingLabel({ waitingSince: '2026-11-14T09:00:00Z' }, now)).toBe('waiting 3h');
    expect(waitingLabel({ waitingSince: '2026-11-11T12:00:00Z' }, now)).toBe('waiting 3d');
  });

  it('never invents a time when the row has none', () => {
    expect(waitingLabel({ waitingSince: null }, now)).toBe('waiting unknown');
    expect(waitingLabel({ waitingSince: 'garbage' }, now)).toBe('waiting unknown');
  });
});
