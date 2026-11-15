/** Chunk-load failures have one recovery that actually works: reload the
 * document so the browser fetches the current HTML/chunk map. A plain remount
 * retries the same stale URL. */

const CHUNK_ERROR_RE =
  /ChunkLoadError|Loading chunk|Loading CSS chunk|Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module/i;

export function isChunkLoadError(error: unknown): boolean {
  if (!error) return false;
  if (typeof error === 'string') return CHUNK_ERROR_RE.test(error);
  if (error instanceof Error) {
    return error.name === 'ChunkLoadError' || CHUNK_ERROR_RE.test(error.message);
  }
  return CHUNK_ERROR_RE.test(String(error));
}
