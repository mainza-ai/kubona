/** Numeric-claim guard for generated research prose.
 *
 * The manuscript is generated from a structured measurement, but generated
 * prose is still untrusted: a future LLM/provider path must not introduce a
 * number that is absent from the result record. This guard extracts every
 * numeric token and compares it with the values the record actually carries.
 * Callers render a safe draft (or refuse) when `ok` is false.
 */
export function numericClaims(text: string): string[] {
  return text.match(/-?\d+(?:\.\d+)?/g) ?? [];
}

export function unsupportedNumericClaims(text: string, allowed: Iterable<number>): string[] {
  const values = [...allowed].filter((v) => Number.isFinite(v));
  const seen = new Set<string>();
  const unsupported: string[] = [];
  for (const token of numericClaims(text)) {
    if (seen.has(token)) continue;
    seen.add(token);
    const n = Number(token);
    if (!Number.isFinite(n)) { unsupported.push(token); continue; }
    const decimals = token.includes('.') ? token.split('.')[1].length : 0;
    const tolerance = 0.5 * 10 ** (-decimals) + 1e-9;
    const within = values.some((v) => Math.abs(v - n) <= Math.max(tolerance, Math.abs(v) * 1e-12));
    if (!within) unsupported.push(token);
  }
  return unsupported;
}

export function numericClaimGuard(
  text: string,
  allowed: Iterable<number>,
): { ok: boolean; unsupported: string[] } {
  const unsupported = unsupportedNumericClaims(text, allowed);
  return { ok: unsupported.length === 0, unsupported };
}
