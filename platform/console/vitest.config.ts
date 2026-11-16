import { defineConfig } from 'vitest/config';
import { resolve } from './vitest.shared';

export default defineConfig({
  test: {
    environment: 'node',
    // D8: browser component tests run under vitest.browser.config.ts.
    exclude: ['**/node_modules/**', '**/.next/**', 'src/**/*.component.test.tsx'],
    // Fork isolation: thread workers intermittently tear down while a
    // CopilotDrawer console log is still pending (Vitest EnvironmentTeardownError),
    // turning a fully green suite into a false red. Forks keep each file's
    // console/RPC lifecycle self-contained; the same 576 assertions run.
    pool: 'forks',
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    server: {
      // One transformed module graph for tests: react, react-dom and the
      // renderer resolve through the same pipeline so hook dispatchers
      // cannot split across two React instances under pnpm symlinks.
      deps: { inline: [/^react($|\/)/, /^react-dom($|\/)/, /^scheduler$/] },
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'json-summary'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/**/*.d.ts'],
      // Floors are enforced after merging node + browser coverage in
      // scripts/merge-coverage.mjs; a node-only run cannot see browser tests.

    },
  },
  resolve,
});
