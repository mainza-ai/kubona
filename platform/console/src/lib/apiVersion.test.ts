import { describe, expect, it } from 'vitest';
import { versionedApiPath } from './api';

describe('versionedApiPath', () => {
  it('rewrites only legacy /api paths when the rollout flag is on', () => {
    expect(versionedApiPath('/api/sessions', true)).toBe('/api/v1/sessions');
    expect(versionedApiPath('/api/events/stream?session_id=s', true)).toBe('/api/v1/events/stream?session_id=s');
    expect(versionedApiPath('/api/v1/sessions', true)).toBe('/api/v1/sessions');
    expect(versionedApiPath('/api/sessions', false)).toBe('/api/sessions');
    expect(versionedApiPath('/health', true)).toBe('/health');
  });
});
