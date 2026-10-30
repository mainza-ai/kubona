import { describe, it, expect } from 'vitest';
import { grantCanonical, revokeCanonical } from './grants';

describe('grant canonical payloads', () => {
  it('sorts scopes and truncates expiry to UTC seconds', () => {
    const got = new TextDecoder().decode(grantCanonical('lab-a', 'lab-b', ['write', 'read'], '2030-01-01T00:00:00.999Z'));
    expect(got).toBe('grant|lab-a|lab-b|read,write|2030-01-01T00:00:00Z');
  });

  it('binds revocation to id and reason', () => {
    expect(new TextDecoder().decode(revokeCanonical('g1', 'mistake'))).toBe('grant-revoke|g1|mistake');
  });
});
