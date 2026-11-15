import type { Need, NeedKind } from '@/types';

/**
 * Typed needs (C2): a failure names a missing precondition and exactly one
 * remedy from a closed vocabulary. The contract file
 * `platform/contracts/need-kinds.json` is the single source; the conformance
 * test reads it and fails on any drift, so this map cannot silently fork.
 *
 * A job with no `need` has no known remedy — the error text stands alone.
 * That is the honest default: unknown never becomes a made-up next step.
 */

export const NEED_KINDS: readonly NeedKind[] = [
  'credentials',
  'budget_exceeded',
  'backend_offline',
  'quota_exhausted',
  'approval',
  'anchor_missing',
  'retry',
];

export type NeedRemedySurface = 'settings' | 'guardrails' | 'copilot';

export interface NeedRemedy {
  id: string;
  label: string;
  surface: NeedRemedySurface;
  section?: string;
}

export const NEED_REMEDIES: Record<NeedKind, NeedRemedy> = {
  credentials: {
    id: 'open-provider-settings',
    label: 'Open provider settings',
    surface: 'settings',
    section: 'models',
  },
  budget_exceeded: {
    id: 'review-spend-mandate',
    label: 'Review spend & mandates',
    surface: 'guardrails',
  },
  backend_offline: {
    id: 'open-quantum-settings',
    label: 'Open hardware settings',
    surface: 'settings',
    section: 'quantum',
  },
  quota_exhausted: {
    id: 'open-quantum-settings',
    label: 'Open hardware settings',
    surface: 'settings',
    section: 'quantum',
  },
  approval: {
    id: 'open-approvals',
    label: 'Open approvals',
    surface: 'guardrails',
  },
  anchor_missing: {
    id: 'ask-for-anchor',
    label: 'Ask for a classical anchor',
    surface: 'copilot',
  },
  retry: {
    id: 'resubmit-run',
    label: 'Resubmit run',
    surface: 'copilot',
  },
};

const NEED_KIND_SET = new Set<string>(NEED_KINDS);

export function isNeedKind(value: unknown): value is NeedKind {
  return typeof value === 'string' && NEED_KIND_SET.has(value);
}

/** Validate a wire need; an unknown kind is treated as absent (fail-closed). */
export function isNeed(value: unknown): value is Need {
  if (!value || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return isNeedKind(record.kind) && typeof record.reason === 'string' && record.reason.trim() !== '';
}

/**
 * The one sentence a reviewer reads. The server's reason is used verbatim;
 * when it is missing (older server or truncated row) we say so rather than
 * substituting a plausible cause.
 */
export function needSentence(need: Need): string {
  const reason = typeof need.reason === 'string' ? need.reason.trim() : '';
  return reason || 'Cause not recorded.';
}

/** The one remedy for a need, or null when the kind is not catalogued. */
export function remedyFor(kind: string): NeedRemedy | null {
  return isNeedKind(kind) ? NEED_REMEDIES[kind] : null;
}

/** True when the need still blocks work; `past` needs are history only. */
export function isLiveNeed(need: Need | null | undefined): need is Need {
  return Boolean(need && !need.past && isNeed(need));
}
