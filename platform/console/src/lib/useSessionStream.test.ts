import { describe, expect, it } from 'vitest';
import { mergeSessionEvents } from './useSessionStream';
import type { LedgerEvent } from '@/types';

const event = (id: number, type = 'tick') => ({ id, event_type: type }) as LedgerEvent;

describe('mergeSessionEvents', () => {
  it('deduplicates replay overlap by id and keeps ledger order', () => {
    expect(mergeSessionEvents([event(2), event(1)], [event(2, 'new'), event(3)])).toEqual([
      event(1),
      event(2, 'new'),
      event(3),
    ]);
  });

  it('caps the retained projection', () => {
    const merged = mergeSessionEvents([event(1), event(2)], [event(3)], 2);
    expect(merged.map((item) => item.id)).toEqual([2, 3]);
  });
});
