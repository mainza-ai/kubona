import { fileURLToPath } from 'node:url';

// One module-resolution contract for every Vitest project. Keeping the alias
// and React dedupe here means the Node and browser suites cannot drift apart.
export const resolve = {
  dedupe: ['react', 'react-dom', 'scheduler'],
  alias: {
    '@': fileURLToPath(new URL('./src', import.meta.url)),
  },
};
