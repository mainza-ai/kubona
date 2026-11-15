import type { ApiProblem } from '@/lib/api';

/**
 * 3.9 — API/contract signals are status, not alarms. A refused request or a
 * drifted response contract is observable but non-blocking: it becomes a chip
 * in the StatusStrip. Only a blocking state (expired auth) keeps role="alert".
 *
 * Dismissal is keyed by the problem's signature, so the same problem stays
 * dismissed for the session while a genuinely different one re-appears.
 */

export const CONTRACT_PROBLEM_PREFIX = 'contract:';

export function problemSignature(problem: ApiProblem): string {
  return `${problem.path}|${problem.status ?? 'transport'}|${problem.message}`;
}

export function isContractDrift(problem: ApiProblem | null | undefined): boolean {
  return Boolean(problem && problem.path.startsWith(CONTRACT_PROBLEM_PREFIX));
}

export function contractNameOf(problem: ApiProblem): string {
  return problem.path.startsWith(CONTRACT_PROBLEM_PREFIX)
    ? problem.path.slice(CONTRACT_PROBLEM_PREFIX.length)
    : problem.path;
}

/** Short chip label; the full message lives in the title attribute. */
export function problemChipLabel(problem: ApiProblem): string {
  if (isContractDrift(problem)) return `contract drift · ${contractNameOf(problem)}`;
  return `API ${problem.status ?? 'transport'} · ${problem.path}`;
}

/** A signature is a bounded opaque key; cap the message part for storage. */
export function boundedSignature(problem: ApiProblem): string {
  return problemSignature({ ...problem, message: problem.message.slice(0, 160) });
}
