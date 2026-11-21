import { NextRequest } from 'next/server';

// Same-origin image proxy. The browser's CSP keeps img-src to 'self'; the
// Next server forwards the session cookie to the Go rail, which validates the
// World Labs URL, redirects, content type, and size before fetching bytes.
const ALLOWED = /^(https:\/\/([a-z0-9-]+\.)*worldlabs\.ai\/|https:\/\/storage\.googleapis\.com\/wlt-marble-outputs-prod\/)/i;

export async function GET(req: NextRequest) {
  const raw = req.nextUrl.searchParams.get('url') || '';
  if (!ALLOWED.test(raw)) {
    return new Response('lens image URL must be HTTPS on a worldlabs.ai or wlt-marble-outputs-prod host', {
      status: 400,
      headers: { 'Content-Type': 'text/plain; charset=utf-8', 'X-Content-Type-Options': 'nosniff' },
    });
  }
  const base = (
    process.env.KUBONA_INTERNAL_API_URL ||
    process.env.KUBONA_CONSOLE_API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    'http://localhost:8080'
  ).replace(/\/$/, '');
  const headers: Record<string, string> = {
    cookie: req.headers.get('cookie') || '',
  };
  const auth = req.headers.get('authorization');
  if (auth) headers.authorization = auth;
  let upstream: Response;
  try {
    upstream = await fetch(`${base}/api/observatory/lens/image?url=${encodeURIComponent(raw)}`, {
      cache: 'no-store',
      headers,
      signal: AbortSignal.timeout(15_000),
    });
  } catch (err) {
    return new Response(`lens image upstream unreachable: ${err instanceof Error ? err.message : String(err)}`, {
      status: 502,
      headers: { 'Content-Type': 'text/plain; charset=utf-8', 'X-Content-Type-Options': 'nosniff' },
    });
  }
  if (!upstream.ok) {
    return new Response(await upstream.text(), {
      status: upstream.status,
      headers: { 'Content-Type': 'text/plain; charset=utf-8', 'X-Content-Type-Options': 'nosniff' },
    });
  }
  return new Response(upstream.body, {
    status: 200,
    headers: {
      'Content-Type': upstream.headers.get('content-type') || 'image/png',
      'X-Content-Type-Options': 'nosniff',
      'Cache-Control': 'private, max-age=60',
    },
  });
}
