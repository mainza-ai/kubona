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
  },
  resolve: {
    dedupe: ['react', 'react-dom', 'scheduler'],
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
});
