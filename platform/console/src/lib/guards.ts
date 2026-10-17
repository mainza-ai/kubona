'use client';

/**
 * Shared record guard: the bridge returns params/result as objects or JSON
 * strings (large payloads). One helper so JobDetailWidget + ProofDagView
 * cannot drift into divergent coercions.
 */
export function asRecord(v: unknown): Record<string, unknown> {
  if (v && typeof v === 'object') return v as Record<string, unknown>;
  if (typeof v === 'string') {
    try {
      const parsed: unknown = JSON.parse(v);
      if (parsed && typeof parsed === 'object') return parsed as Record<string, unknown>;
    } catch {
      /* fall through to empty */
    }
  }
  return {};
}

export function fmtMoney(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? `$${v.toFixed(2)}` : '—';
}

export function fmtFixed(v: unknown, digits: number): string {
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(digits) : '—';
}
