import { describe, expect, it } from 'vitest';
import { coalesceTelemetryFrames, mergeSessionEvents, parseTelemetryPayload } from './useSessionStream';
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

describe('parseTelemetryPayload', () => {
  it('accepts the current events array and the legacy single-event frame', () => {
    expect(parseTelemetryPayload({ events: [event(4)] })).toEqual([event(4)]);
    expect(parseTelemetryPayload({ event: event(5) })).toEqual([event(5)]);
    expect(parseTelemetryPayload({ events: [{ id: Number.NaN }, event(6)] })).toEqual([event(6)]);
    expect(parseTelemetryPayload({ events: [] })).toEqual([]);
  });
});

describe('coalesceTelemetryFrames', () => {
  it('keeps ledger history but lets the latest projection frame win per domain id', () => {
    const frame = (id: number, payloadId: string, value: string) => ({
      id,
      event_type: 'job_updated',
      payload: { id: payloadId, value },
    }) as unknown as LedgerEvent;
    const out = coalesceTelemetryFrames([
      { domain: 'jobs', events: [frame(1, 'job-a', 'queued')] },
      { domain: 'jobs', events: [frame(2, 'job-a', 'running')] },
      { domain: 'ledger', events: [frame(3, 'job-a', 'queued')] },
      { domain: 'ledger', events: [frame(4, 'job-a', 'running')] },
    ]);
    expect(out.map((event) => event.id)).toEqual([2, 3, 4]);
    expect((out[0] as unknown as { payload: { value: string } }).payload.value).toBe('running');
  });
});
