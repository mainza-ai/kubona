import type { NextConfig } from "next";
import { hstsHeaderForApiUrl } from "./src/lib/securityHeaders";

// Startup env guard: the console talks to the bridge at NEXT_PUBLIC_API_URL.
// Unset means the localhost default below — loud in dev, not silent.
if (!process.env.NEXT_PUBLIC_API_URL) {
  console.warn(
    '[kubona-console] NEXT_PUBLIC_API_URL unset — defaulting to http://localhost:8080. Set it explicitly for any non-local bridge.'
  );
}

const nextConfig: NextConfig = {
  // D1: standalone output for the production console image
  // (Dockerfile.console copies .next/standalone + .next/static).
  output: 'standalone',

  // Security headers (2026-10-26 audit UI-04). CSP keeps the script/style
  // allowances Next needs while blocking framing, object embedding, and
  // cross-origin form posts. connect-src stays scheme-wide because the Go
  // API URL is runtime-configurable and SSE uses ws/wss.
  // HSTS (Phase-2 credibility): sent only for non-local deployments.
  // Same resolution as the /api/config route (deploy override first,
  // build-time default, localhost fallback); loopback targets omit it so
  // local dev can never be HSTS-pinned.
  //
  // Wave-3 CSP img-src investigation (2026-10-29) — decision: route remote
  // lens imagery through the backend proxy (same-origin), do NOT widen
  // img-src. Provenance, read from the code:
  // - SpatialCanvas.tsx (lens poll) reads the frame URL verbatim:
  //   `resp.frames?.[0]?.imageAsset?.url` — whatever the Atlas operations
  //   API returned, passed straight into `lensImageUrl`.
  // - SpatialCanvas3D.tsx (lens preview) renders `<img src={lensImageUrl}>`
  //   only when it starts with 'http'.
  // - The status route (platform/core/pkg/api/observatory_lens.go,
  //   handleObservatoryLensStatus) proxies Atlas bytes verbatim, so the URL
  //   host is Atlas-chosen, not ours. Atlas docs (`url = baseUrl +
  //   contentPath`, plus signedQuery/read-prefix auth) confirm the host is
  //   per-project/region and NOT pinnable — docs use a placeholder host and
  //   repo fixtures use https://cdn.example. The only host provable from
  //   this repo is the API base itself
  //   (https://api.atlas-beta.worldlabs.ai, WLT_API_BASE_URL override),
  //   which is where control-plane calls go — NOT where image bytes live.
  //   Allowlisting that host would imply a guarantee the code cannot keep.
  // Hence: img-src stays `'self' data: blob:`. Dev keeps working because
  // snapshots ride as data: URLs, HUD/local assets are same-origin, and
  // connect-src already covers the localhost bridge + ws(s). Remote https
  // lens frames fail closed under this CSP (blocked, no exfil) until the
  // backend serves them same-origin (fetch with WLT-Api-Key server-side,
  // re-serve bytes) — that proxy is a backend change, out of scope for
  // this console-only Wave.
  async headers() {
    const apiUrl =
      process.env.KUBONA_CONSOLE_API_URL || process.env.NEXT_PUBLIC_API_URL;
    const hsts = hstsHeaderForApiUrl(apiUrl);
    const csp = [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline'",
      "style-src 'self' 'unsafe-inline'",
      "img-src 'self' data: blob:",
      "font-src 'self' data:",
      "connect-src 'self' http: https: ws: wss:",
      "object-src 'none'",
      "base-uri 'self'",
      "form-action 'self'",
      "frame-ancestors 'none'",
    ].join('; ');
    return [
      {
        source: '/(.*)',
        headers: [
          { key: 'Content-Security-Policy', value: csp },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          ...(hsts ? [{ key: 'Strict-Transport-Security', value: hsts }] : []),
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
        ],
      },
    ];
  },
};

export default nextConfig;
