const JOB_STATUS_LABELS: Record<number, string> = {
  0: 'UNSPECIFIED',
  1: 'QUEUED',
  2: 'RUNNING',
  3: 'COMPLETED',
  4: 'FAILED',
  5: 'PAUSED',
  6: 'CANCELLED',
  7: 'INTERRUPTED',
  8: 'OUTCOME_UNKNOWN',
};

const KNOWN_LABELS = new Set(Object.values(JOB_STATUS_LABELS));

export function jobStatusLabel(status: unknown): string {
  if (typeof status === 'number') return JOB_STATUS_LABELS[status] ?? String(status);
  if (typeof status === 'string') {
    const upper = status.toUpperCase().replace(/^JOB_STATUS_/, '');
    if (KNOWN_LABELS.has(upper)) return upper;
    return status;
  }
  return 'UNKNOWN';
}

// Terminal matches the ledger: the row will not run again. Outcome-unknown
// is deliberately not terminal — the reservation stays until evidence or a
// signed resolution — but it is not still running either.
export function isTerminalJobStatus(status: unknown): boolean {
  const label = jobStatusLabel(status);
  return label === 'COMPLETED' || label === 'FAILED' || label === 'CANCELLED' || label === 'INTERRUPTED';
}

export function isOutcomeUnknown(status: unknown): boolean {
  return jobStatusLabel(status) === 'OUTCOME_UNKNOWN';
}
