import type { NextConfig } from "next";

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
};

export default nextConfig;
