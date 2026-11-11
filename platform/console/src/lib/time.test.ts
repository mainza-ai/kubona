import { describe, expect, it } from 'vitest';
import { formatElapsed } from './time';

describe('formatElapsed', () => {
  it('renders minutes and seconds cleanly past 60s without % 60 bug', () => {
    const start = '2026-11-11T10:00:00.000Z';
    expect(formatElapsed(start, Date.parse('2026-11-11T10:00:09.000Z'))).toBe('00:00:09');
    expect(formatElapsed(start, Date.parse('2026-11-11T10:01:05.000Z'))).toBe('00:01:05');
    expect(formatElapsed(start, Date.parse('2026-11-11T11:23:45.000Z'))).toBe('01:23:45');
  });

  it('guards invalid or future timestamps with 00:00:00', () => {
    expect(formatElapsed('not-a-date', Date.now())).toBe('00:00:00');
    const future = '2026-11-11T12:00:00.000Z';
    expect(formatElapsed(future, Date.parse('2026-11-11T11:00:00.000Z'))).toBe('00:00:00');
  });
});
