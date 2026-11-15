import type { LoopPart } from '@/types';

/**
 * C1 — run lanes. A run is time-ordered columns of parts, one column per
 * helper, so parallel helpers are visibly parallel. Ordering is by recorded
 * start time; helper columns follow the closed vocabulary order, with any
 * unknown helper appended in first-seen order (never dropped silently).
 */
export const RUN_HELPER_ORDER = ['lead', 'tool', 'quantum_job', 'literature', 'prover'] as const;

export interface RunColumn {
  helper: string;
  parts: LoopPart[];
}

export function columnsOf(parts: readonly LoopPart[]): RunColumn[] {
  const ordered = [...parts].sort((a, b) => {
    if (a.began_ms !== b.began_ms) return a.began_ms - b.began_ms;
    return a.part_id.localeCompare(b.part_id);
  });
  const byHelper = new Map<string, LoopPart[]>();
  const unknown: string[] = [];
  for (const part of ordered) {
    const helper = part.helper || 'unknown';
    if (!byHelper.has(helper)) {
      byHelper.set(helper, []);
      if (!(RUN_HELPER_ORDER as readonly string[]).includes(helper)) unknown.push(helper);
    }
    byHelper.get(helper)!.push(part);
  }
  const helpers = [...RUN_HELPER_ORDER.filter((helper) => byHelper.has(helper)), ...unknown];
  return helpers.map((helper) => ({ helper, parts: byHelper.get(helper)! }));
}

/** The part the agent marker sits on: first running, else first waiting. */
export function activePart(parts: readonly LoopPart[]): LoopPart | null {
  return parts.find((part) => part.state === 'running') ?? parts.find((part) => part.state === 'waiting') ?? null;
}
