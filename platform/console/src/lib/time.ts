/**
 * Formats an elapsed HH:MM:SS duration from an ISO timestamp.
 */
export function formatElapsed(createdAt: string, nowMs = Date.now()): string {
  const createdMs = new Date(createdAt).getTime();
  const ms = nowMs - createdMs;
  if (!Number.isFinite(ms) || ms < 0) return '00:00:00';
  const s = Math.floor(ms / 1000);
  const hh = String(Math.floor(s / 3600)).padStart(2, '0');
  const mm = String(Math.floor((s % 3600) / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}
