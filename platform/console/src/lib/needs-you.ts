import type { ObligationRow } from '@/lib/api';

/**
 * "Needs you" projection (report §3.1).
 *
 * An obligation that has not been settled IS the reviewer's queue: it stays
 * in the shell until a decision removes it. This module is pure — it turns
 * the server's open-obligation rows into ordered tray items and computes
 * what is new since the reviewer last looked.
 *
 * Honesty rules:
 * - `reason` is the server's verbatim blocking text (an unknown's
 *   description, else a close-refusal reason), never a client guess.
 * - `waitingSince` is null when the row carries no readable clock; the UI
 *   renders "waiting unknown", never a substitute time.
 * - Ordering is a v1 heuristic and says so: decidable rows first (a decision
 *   can happen now), then longest-waiting first. The report's ideal ordering
 *   (cost of waiting × irreversibility) needs pricing and blast-radius inputs
 *   that obligation rows do not carry; when they exist, this function is the
 *   single place to change.
 */

export type NeedSubjectClass = 'job' | 'loop' | 'campaign' | 'task' | 'claim' | 'proof' | 'other';

export interface NeedItem {
  /** The obligation id — the unit the server decides closure on. */
  id: string;
  subjectKind: string;
  subjectId: string;
  subjectClass: NeedSubjectClass;
  owner: string;
  /** `decidable`: close rules would pass now. `blocked`: they would refuse. */
  state: 'decidable' | 'blocked';
  /** Verbatim server evidence, first blocking reason; null when none recorded. */
  reason: string | null;
  /** Every recorded blocking reason, deduped in server order. */
  reasons: string[];
  /** ISO timestamp from the row, or null when unreadable. */
  waitingSince: string | null;
  /** Stable per-content key: changes when the obligation changes. */
  signature: string;
}

const SUBJECT_CLASSES: Array<{ className: NeedSubjectClass; pattern: RegExp }> = [
  { className: 'job', pattern: /(^|[-_:])job|quantum[-_]?run|hardware/i },
  { className: 'loop', pattern: /loop|autoresearch|agent[-_]?run/i },
  { className: 'campaign', pattern: /campaign|session|replan/i },
  { className: 'task', pattern: /task|todo/i },
  { className: 'claim', pattern: /claim|hypothesis|tribunal/i },
  { className: 'proof', pattern: /proof|receipt|evidence|manuscript/i },
];

/** Classify an obligation's subject kind for navigation and labelling. */
export function classifySubject(kind: string): NeedSubjectClass {
  const value = (kind ?? '').trim();
  if (!value) return 'other';
  for (const entry of SUBJECT_CLASSES) {
    if (entry.pattern.test(value)) return entry.className;
  }
  return 'other';
}

function parseTime(raw: string | null | undefined): number | null {
  if (typeof raw !== 'string' || raw.trim() === '') return null;
  const ms = Date.parse(raw);
  return Number.isFinite(ms) ? ms : null;
}

/** Every server-recorded blocking reason, deduped in server order. */
export function blockingReasons(row: ObligationRow): string[] {
  const out: string[] = [];
  const push = (value: unknown) => {
    if (typeof value !== 'string') return;
    const text = value.trim();
    if (text && !out.includes(text)) out.push(text);
  };
  for (const unknown of row.unknowns_detail ?? []) push(unknown?.description);
  for (const reason of row.close_reasons ?? []) push(reason);
  return out;
}

function signatureOf(row: ObligationRow): string {
  const stamp = row.updated_at ?? row.created_at ?? '';
  return `${row.id}@${stamp}`;
}

/**
 * Project open obligations into tray items. Closed rows are dropped — a
 * decision is what removes a row, so a decided item never lingers.
 */
export function buildNeedsYou(rows: readonly ObligationRow[]): NeedItem[] {
  const items: NeedItem[] = [];
  for (const row of rows) {
    if (!row || !row.id) continue;
    if (row.disposition === 'closed') continue;
    const reasons = blockingReasons(row);
    const waitingSince =
      typeof row.created_at === 'string' && parseTime(row.created_at) !== null ? row.created_at : null;
    items.push({
      id: row.id,
      subjectKind: row.subject_kind ?? '',
      subjectId: row.subject_id ?? '',
      subjectClass: classifySubject(row.subject_kind ?? ''),
      owner: row.owner ?? '',
      state: row.close_ready === true ? 'decidable' : 'blocked',
      reason: reasons[0] ?? null,
      reasons,
      waitingSince,
      signature: signatureOf(row),
    });
  }
  // Decidable first; within a group the longest-waiting first. Rows with an
  // unreadable clock sort after rows with one (we cannot claim they waited
  // longer), and ties fall back to id for determinism.
  return items.sort((a, b) => {
    if (a.state !== b.state) return a.state === 'decidable' ? -1 : 1;
    const at = parseTime(a.waitingSince);
    const bt = parseTime(b.waitingSince);
    if (at !== null && bt !== null && at !== bt) return at - bt;
    if (at !== null && bt === null) return -1;
    if (at === null && bt !== null) return 1;
    return a.id.localeCompare(b.id);
  });
}

/** Items the reviewer has not looked at yet. */
export function unseenNeedsYou(items: readonly NeedItem[], seen: ReadonlySet<string>): NeedItem[] {
  return items.filter((item) => !seen.has(item.signature));
}

/** True once a row has waited at least one hour. Unknown clocks never escalate. */
export function needsEscalation(item: Pick<NeedItem, 'waitingSince'>, nowMs = Date.now()): boolean {
  const since = parseTime(item.waitingSince);
  return since !== null && nowMs - since >= 3_600_000;
}

/** Short waiting label; refuses to invent a time when the row has none. */
export function waitingLabel(item: Pick<NeedItem, 'waitingSince'>, nowMs = Date.now()): string {
  const since = parseTime(item.waitingSince);
  if (since === null) return 'waiting unknown';
  const deltaMs = Math.max(0, nowMs - since);
  const minutes = Math.floor(deltaMs / 60_000);
  if (minutes < 1) return 'waiting <1m';
  if (minutes < 60) return `waiting ${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `waiting ${hours}h`;
  return `waiting ${Math.floor(hours / 24)}d`;
}
