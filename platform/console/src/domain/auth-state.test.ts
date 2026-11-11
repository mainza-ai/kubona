import { beforeEach, describe, expect, it } from 'vitest';
import {
  getAuthPhase,
  isMultiLabAuthRequired,
  resetAuthStateForTests,
  setAuthPhase,
  setMultiLabAuthRequired,
  shouldDeferProtectedRead,
} from './auth-state';

describe('auth-state gating', () => {
  beforeEach(() => {
    resetAuthStateForTests();
  });

  it('allows protected reads when multi-lab auth is not required', () => {
    expect(isMultiLabAuthRequired()).toBe(false);
    expect(getAuthPhase()).toBe('anonymous');
    expect(shouldDeferProtectedRead(false)).toBe(false);
  });

  it('defers protected reads while multi-lab session is anonymous and unblocks once authenticated or bearer-configured', () => {
    setMultiLabAuthRequired(true);
    expect(isMultiLabAuthRequired()).toBe(true);
    expect(shouldDeferProtectedRead(false)).toBe(true);
    expect(shouldDeferProtectedRead(true)).toBe(false);

    setAuthPhase('authenticated');
    expect(shouldDeferProtectedRead(false)).toBe(false);
  });
});
