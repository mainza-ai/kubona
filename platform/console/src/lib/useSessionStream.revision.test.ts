import { describe, expect, it } from 'vitest';
import { shouldRequestSnapshot } from './useSessionStream';

describe('session stream revision gap', () => {
  it('requests a snapshot only after a forward gap', () => {
    expect(shouldRequestSnapshot(0, 1)).toBe(false);
    expect(shouldRequestSnapshot(3, 4)).toBe(false);
    expect(shouldRequestSnapshot(3, 5)).toBe(true);
    expect(shouldRequestSnapshot(5, 5)).toBe(false);
    expect(shouldRequestSnapshot(5, 4)).toBe(false);
  });
});
