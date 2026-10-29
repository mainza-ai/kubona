import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.{ts,tsx}'],
    server: {
      // One transformed module graph for tests: react, react-dom and the
      // renderer resolve through the same pipeline so hook dispatchers
      // cannot split across two React instances under pnpm symlinks.
      deps: { inline: [/^react($|\/)/, /^react-dom($|\/)/, /^scheduler$/] },
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/**/*.d.ts'],
      // Hold-the-line floor, measured 2026-10-29: 54.5/55.3/52.6/57.1.
      // One-point epsilon below the measured values prevents flaky red on
      // test-order noise. Raise after a sustained green run; never lower
      // it to make a red pass.
      thresholds: { statements: 53, branches: 54, functions: 51, lines: 56 },
    },
  },
  resolve: {
    dedupe: ['react', 'react-dom', 'scheduler'],
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
});
