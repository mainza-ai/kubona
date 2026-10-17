import { describe, it, expect } from 'vitest';
import {
  canonicalAdminParams,
  adminParamsHash,
  adminPayload,
  signAdminAction,
  goFloat,
} from './adminAction';
import { getOrCreateSessionKeyPair } from './mandates';

// Golden vectors from the Go toolchain (2026-10-17):
//   go run /tmp/vec/main.go  (json.Marshal + sha256 over the same maps)
// If any vector fails, the TS canonicalization drifted from Go and every
// envelope the console mints would be rejected — fix the builder, never
// the vectors.
describe('adminAction canonicalization (U1-b)', () => {
  it('matches Go map JSON byte-for-byte', () => {
    expect(canonicalAdminParams({ ceiling_usd: 150 })).toBe('{"ceiling_usd":150}');
    expect(canonicalAdminParams({ ceiling_usd: 49.5 })).toBe('{"ceiling_usd":49.5}');
    expect(canonicalAdminParams({ status: 'suspended' })).toBe('{"status":"suspended"}');
    expect(
      canonicalAdminParams({ tenant_id: 'lab-alpha', amount_usd: 25.5, reason: 'failed QPU run, no result' })
    ).toBe('{"amount_usd":25.5,"reason":"failed QPU run, no result","tenant_id":"lab-alpha"}');
  });

  it('matches Go ParamsHashOf (hex sha256)', () => {
    expect(adminParamsHash({ ceiling_usd: 150 })).toBe(
      '95169ffc05abd4ab865019f68df37924f29d682856c4c4e9b929b74069e0e891'
    );
    expect(adminParamsHash({ ceiling_usd: 49.5 })).toBe(
      'fa4de6772fb3bf4f7170679c181d4555dfa2d279a969d245264dd0ecf629871a'
    );
    expect(adminParamsHash({ status: 'suspended' })).toBe(
      '78db1913590003d8f19b7d3bf6b4a8ecc7ff2d06825e842d209e9a0e29a9bb5f'
    );
    expect(
      adminParamsHash({ tenant_id: 'lab-alpha', amount_usd: 25.5, reason: 'failed QPU run, no result' })
    ).toBe('e499f2401366e7075ec16ce84cfe3105d7ed57ddbcfed244dcbb4747a5bdae56');
  });

  it('formats the CanonicalPayload exactly like Go', () => {
    expect(adminPayload('lab-a', 'set_ceiling', 'abc123', '2026-10-17T19:30:00Z')).toBe(
      'lab-a|set_ceiling|abc123|2026-10-17T19:30:00Z'
    );
  });

  it('goFloat refuses non-finite values (no NaN/Infinity envelopes)', () => {
    expect(() => goFloat(NaN)).toThrow();
    expect(() => goFloat(Infinity)).toThrow();
  });

  it('mints a well-formed envelope bound to the exact params', () => {
    const kp = getOrCreateSessionKeyPair();
    const env = signAdminAction({
      tenantId: 'lab-a',
      action: 'set_ceiling',
      params: { ceiling_usd: 150 },
      keyPair: kp,
      ttlSeconds: 300,
    });
    expect(env.tenant_id).toBe('lab-a');
    expect(env.action).toBe('set_ceiling');
    expect(env.params_hash).toBe('95169ffc05abd4ab865019f68df37924f29d682856c4c4e9b929b74069e0e891');
    expect(env.expiry).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/);
    expect(env.signature_hex).toMatch(/^[0-9a-f]{128}$/);
  });

  it('refuses empty tenant/action (no unbound envelopes)', () => {
    const kp = getOrCreateSessionKeyPair();
    expect(() => signAdminAction({ tenantId: '', action: 'set_ceiling', params: {}, keyPair: kp })).toThrow();
  });
});
