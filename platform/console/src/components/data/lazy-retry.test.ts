import { describe, expect, it } from 'vitest';
import { isChunkLoadError } from './lazy-retry';

describe('isChunkLoadError', () => {
  it('recognises the browser and Next chunk failure shapes', () => {
    expect(isChunkLoadError(new Error('Failed to fetch dynamically imported module: /_next/x.js'))).toBe(true);
    expect(isChunkLoadError(new Error('Loading chunk 42 failed'))).toBe(true);
    expect(isChunkLoadError(Object.assign(new Error('x'), { name: 'ChunkLoadError' }))).toBe(true);
    expect(isChunkLoadError('Importing a module script failed.')).toBe(true);
  });

  it('does not treat an ordinary render error as a stale chunk', () => {
    expect(isChunkLoadError(new Error('Cannot read properties of null'))).toBe(false);
    expect(isChunkLoadError(null)).toBe(false);
    expect(isChunkLoadError(undefined)).toBe(false);
  });
});
