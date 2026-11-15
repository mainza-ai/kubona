import { defineConfig } from 'vitest/config';
import { playwright } from '@vitest/browser-playwright';
import { resolve } from './vitest.shared';

// D8: browser component tests run in a real Chromium page, so focus,
// keyboard, layout and CSS are observable. They live beside the unit suite
// but in their own project: `pnpm test` stays fast and browser-free, while
// `pnpm test:browser` runs every `*.component.test.tsx` file.
const channel = process.env.PLAYWRIGHT_CHANNEL;
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;

export default defineConfig({
  resolve,
  // Next inlines `process.env.*` at build time. Browser tests are not a Next
  // build, so provide the same client contract explicitly; without this the
  // first module that reads NEXT_PUBLIC_API_URL dies on `process is not defined`.
  define: {
    'process.env': JSON.stringify({
      NODE_ENV: 'test',
      NEXT_PUBLIC_API_URL: 'http://localhost:8080',
    }),
  },
  test: {
    include: ['src/**/*.component.test.tsx'],
    setupFiles: ['./src/test-setup.ts'],
    // One prebundled React (and the icon set that consumes it) prevents the
    // duplicate-context failure a cold optimizer run can produce.
    deps: {
      optimizer: {
        client: {
          include: ['react', 'react-dom', 'react/jsx-runtime', 'lucide-react', 'vitest-browser-react'],
        },
      },
    },
    browser: {
      enabled: true,
      headless: true,
      provider: playwright({
        launchOptions: {
          ...(channel ? { channel } : {}),
          ...(executablePath ? { executablePath } : {}),
        },
      }),
      instances: [{ browser: 'chromium' }],
    },
  },
});
