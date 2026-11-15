import { API_BASE, apiFetch, authHeaders } from '@/lib/api';

export type WritePhase = 'idle' | 'accepted' | 'admitted' | 'applied' | 'refused' | 'unknown';

export type WriteSettlement = {
  phase: WritePhase;
  requestId: string | null;
  detail: string;
};

export function newWriteRequestId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `write-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export const IDLE_WRITE: WriteSettlement = { phase: 'idle', requestId: null, detail: '' };

export function acceptWrite(requestId: string, detail = 'request accepted'): WriteSettlement {
  return { phase: 'accepted', requestId, detail };
}

export function settleWrite(
  requestId: string,
  phase: Exclude<WritePhase, 'idle' | 'accepted'>,
  detail: string,
): WriteSettlement {
  return { phase, requestId, detail };
}

/** Accepted is not applied; unknown never renders as success. */
export function writeSettlementLabel(write: WriteSettlement): string {
  switch (write.phase) {
    case 'accepted':
      return `accepted · ${write.detail}`;
    case 'admitted':
      return `admitted · ${write.detail}`;
    case 'applied':
      return `applied · ${write.detail}`;
    case 'refused':
      return `refused · ${write.detail}`;
    case 'unknown':
      return `unknown · ${write.detail}`;
    default:
      return '';
  }
}


type OperationReceipt = {
  request_id?: string;
  status?: 'in_progress' | 'complete';
  status_code?: number;
  disposition?: 'admitted' | 'applied' | 'no_op' | 'refused' | 'outcome_unknown';
  reason?: string;
  receipt?: unknown;
};

async function readOperationReceipt(requestId: string, sessionId: string): Promise<OperationReceipt | null> {
  const path = `/api/operations/${encodeURIComponent(requestId)}?session_id=${encodeURIComponent(sessionId)}`;
  const res = await apiFetch(`${API_BASE}${path}`, { cache: 'no-store', headers: authHeaders() });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`operation receipt refused (HTTP ${res.status})`);
  return (await res.json()) as OperationReceipt;
}

export type ReconciledOperation = {
  phase: 'admitted' | 'applied' | 'refused' | 'unknown';
  requestId: string;
  detail: string;
  statusCode?: number;
  receipt?: unknown;
};

/** Honest copy when a launch reply never came back. Never claims the job was absent. */
export const UNCONFIRMED_LAUNCH =
  'The launch reply was lost. Whether a job was created is unknown. Retry stays disabled until this request is reconciled.';

export function launchJobId(receipt: unknown): string | null {
  if (!receipt || typeof receipt !== 'object') return null;
  const job = (receipt as { job?: { id?: unknown } }).job;
  if (job?.id == null || job.id === '') return null;
  return String(job.id);
}

function receiptDetail(body: unknown, code: number): string {
  if (typeof body === 'string') return body.slice(0, 200);
  if (body && typeof body === 'object') {
    const record = body as { message?: unknown; error?: unknown; status?: unknown };
    return String(record.message ?? record.error ?? record.status ?? `HTTP ${code}`).slice(0, 200);
  }
  return `HTTP ${code}`;
}

/** Reconcile a write and keep the receipt body so a lost launch can be read back. */
export async function reconcileOperationFull(
  requestId: string,
  sessionId: string,
  attempts = 8,
  delayMs = 250,
): Promise<ReconciledOperation> {
  let sawAdmitted = false;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const receipt = await readOperationReceipt(requestId, sessionId);
      if (receipt?.status === 'in_progress' || receipt?.disposition === 'admitted') {
        sawAdmitted = true;
      }
      if (receipt?.status === 'complete') {
        const code = receipt.status_code ?? 0;
        const detail = receipt.reason || receiptDetail(receipt.receipt, code);
        let phase: ReconciledOperation['phase'] = code >= 400 ? 'refused' : 'applied';
        if (receipt.disposition === 'refused') phase = 'refused';
        if (receipt.disposition === 'outcome_unknown') phase = 'unknown';
        if (receipt.disposition === 'no_op' || receipt.disposition === 'applied') phase = 'applied';
        return {
          phase,
          requestId,
          detail,
          statusCode: code,
          receipt: receipt.receipt,
        };
      }
    } catch {
      // Keep checking; a transient receipt read is not an outcome.
    }
    if (attempt < attempts) await new Promise((resolve) => setTimeout(resolve, delayMs));
  }
  if (sawAdmitted) {
    return { phase: 'admitted', requestId, detail: 'queued' };
  }
  return {
    phase: 'unknown',
    requestId,
    detail: `operation ${requestId.slice(0, 12)} not confirmed after reconciliation`,
  };
}

/** Reconcile an accepted-but-unconfirmed write against its server receipt. */
export async function reconcileOperation(
  requestId: string,
  sessionId: string,
  attempts = 8,
  delayMs = 250,
): Promise<WriteSettlement> {
  const settled = await reconcileOperationFull(requestId, sessionId, attempts, delayMs);
  return settleWrite(requestId, settled.phase, settled.detail);
}
