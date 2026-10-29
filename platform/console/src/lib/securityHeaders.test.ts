import { describe, it, expect } from 'vitest';
import { hstsHeaderForApiUrl, isLoopbackApiUrl, HSTS_HEADER_VALUE } from './securityHeaders';

describe('HSTS predicate (non-local deployments only)', () => {
  it('omits HSTS for localhost (local dev exemption)', () => {
    expect(hstsHeaderForApiUrl('http://localhost:8080')).toBeNull();
  });

  it('omits HSTS for 127.0.0.1 and ::1', () => {
    expect(hstsHeaderForApiUrl('http://127.0.0.1:8080')).toBeNull();
    expect(hstsHeaderForApiUrl('http://[::1]:8080')).toBeNull();
    expect(isLoopbackApiUrl('http://127.0.0.1:9')).toBe(true);
  });

  it('omits HSTS when unset (localhost default)', () => {
    expect(hstsHeaderForApiUrl(undefined)).toBeNull();
    expect(hstsHeaderForApiUrl('')).toBeNull();
  });

  it('omits HSTS for unparseable URLs (fail toward dev, never pin)', () => {
    expect(hstsHeaderForApiUrl('not a url')).toBeNull();
  });

  it('sends the one-year HSTS value for non-local deployments', () => {
    expect(hstsHeaderForApiUrl('https://console.example.com')).toBe(HSTS_HEADER_VALUE);
    expect(HSTS_HEADER_VALUE).toContain('max-age=31536000');
    expect(HSTS_HEADER_VALUE).toContain('includeSubDomains');
  });
});
