// Security-header predicates (Phase-2 console credibility).
//
// HSTS is a production-only header: sending it on a localhost dev server is
// harmless in most browsers but can pin a throwaway port in hosted dev
// proxies, so the console omits it for loopback API targets and sends it
// everywhere else. No invented numbers: max-age 31536000 is the documented
// one-year value; includeSubDomains matches the single-origin console.
export const HSTS_HEADER_VALUE = 'max-age=31536000; includeSubDomains';

/** Loopback hosts are local dev — HSTS stays off so it can never pin them. */
export function isLoopbackApiUrl(apiUrl: string): boolean {
  try {
    const host = new URL(apiUrl).hostname.toLowerCase().replace(/^\[|\]$/g, '');
    return host === 'localhost' || host === '127.0.0.1' || host === '::1';
  } catch {
    // Unparseable URL: the startup default is http://localhost:8080, so
    // treat it as local (omit HSTS) rather than risk pinning a dev box.
    return true;
  }
}

/**
 * HSTS value for a deployment, or null when the header must be omitted
 * (loopback/local dev). Blank input falls back to the localhost default,
 * mirroring the /api/config route.
 */
export function hstsHeaderForApiUrl(apiUrl: string | undefined): string | null {
  const url = (apiUrl ?? '').trim() || 'http://localhost:8080';
  return isLoopbackApiUrl(url) ? null : HSTS_HEADER_VALUE;
}
