import { NextResponse } from 'next/server';

// Runtime backend URL for the browser bundle. NEXT_PUBLIC_* values are
// baked at build time; this route is evaluated per request, so the same
// image can point at any backend without a rebuild:
//
//   KUBONA_CONSOLE_API_URL=http://kubonad:8080  (compose / proxy setups)
//
// Falls back to the build-time default (local dev). Never serves secrets:
// the operator bearer token stays build-time / operator-managed (see U3).
export async function GET() {
  const apiUrl =
    process.env.KUBONA_CONSOLE_API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    'http://localhost:8080';
  return NextResponse.json({ apiUrl });
}
